# Slice 1 implementation design: durable requests

Status: current slice 1 design, written 2026-09-23 against repository revision `1ae6480`.
Go, SQL, shell, and JSON examples are design sketches, not execution evidence.
[Project direction](../../knowledge/project-direction.md) records implementation status;
[TESTING.md](../../../TESTING.md) records available checks and their limits.

Source slice: [durable requests](README.md). Architecture: [slice 1](architecture.md). Sequence: [MVP breakdown](../README.md).
Resume ledger: [plan_state.md](../../../.state/1-durable-requests/plan_state.md), local and ignored by Git.
Planning workflow: [detailed-design skill](../../../.agents/skills/detailed-design/SKILL.md).
Governing documents: [guidance](../../../GUIDANCE.md), [TigerStyle](../../TIGERSTYLE.md), [testing](../../../TESTING.md), and [simulation research](../../TEST_RESEARCH.md).

## 1. Outcome and scope

A caller registers a named machine configuration using a stable request identifier, loses the response, retries, and receives the original machine identity. The caller can inspect that registration after the control plane restarts. Reusing the request identifier with different parameters returns a conflict. Two registrations cannot own the same machine name.

The demonstration runs the real CLI, HTTP service, and PostgreSQL adapter. A deterministic simulator exercises the same decisions, including a transaction that may still commit after its connection disappears. Real PostgreSQL checks validate the storage assumptions independently.

This slice creates an intent record. It does not boot a VM, reserve running-machine memory, create network devices, invoke Firecracker, or execute commands. Guest and machine lifecycle work belongs to later slices. The only host resource capacity introduced here is a bound on retained registration records, named `max_registrations` so it cannot be mistaken for running capacity.

At the design baseline, the repository had documentation and tooling only. The paths below describe this slice’s responsibilities. Do not add a generic scheduler framework or future lifecycle columns merely because later slices will need them.

### 1.1 Proposed resolutions of the slice's discussion questions

| Question | Selected design proposal | Reason and acceptance evidence |
| --- | --- | --- |
| What distinguishes registration from provisioning? | Durable record state is `registered`; response includes `provisioned: false` | Inspection cannot suggest a running guest; API and CLI tests check the distinction |
| What resolves an uncertain commit? | Inspect by original request ID; if absent, retry the same reservation transaction through the same database gate | Absence alone is not rollback evidence; real pending-transaction tests cover both later commit and later abort |
| What trace can a reader follow? | Versioned JSON Lines with event IDs, effect IDs, chosen faults, observations, and outcomes; concise text rendering on failure | Replay consumes recorded choices, checks causality, and compares normalized traces |
| What is the minimum surface? | `PUT /v1/registrations/{request_id}`, two read routes, and readiness | Names registration explicitly without prematurely implementing the historical create/start operation API |
| How much concurrency is necessary? | One owned coordinator loop, bounded external workers, and a short database gate for registration mutations | Decisions stay deterministic; database constraints remain authoritative; inspections do not take the mutation gate |

These resolutions define the registration contract. They do not establish runtime validation.

The registration-specific HTTP surface deliberately refines the older `POST /v1/machines -> 202 Operation` reference. Future provisioning may add that operation surface without changing the meaning of registration. The global registration gate trades mutation throughput for simple bounded coordination; it is not a design for future independent guest execution.

### 1.2 Acceptance requirements

| ID | Required observable result |
| --- | --- |
| A01 | First successful registration creates exactly one durable record and returns its immutable machine ID |
| A02 | Matching retries return the original record even when the registration limit is full |
| A03 | Reused request ID with changed parameters and reused machine name with another ID are rejected without mutation |
| A04 | Service restart preserves committed records; no response loss or restart creates another identity |
| A05 | Unknown commits retain uncertainty; an absent read never authorizes a conflicting overwrite |
| A06 | Concurrent reservations obey uniqueness and the configured registration limit |
| A07 | Production and fixed simulation execute the same admission and transition functions |
| A08 | Repeat replay produces equivalent normalized traces and outcomes; a deliberate invariant defect is detected |
| A09 | Real PostgreSQL outcomes agree with the simulated reservation contract, including pending commit and restart cases |
| A10 | Requests, connections, transactions, queues, retries, traces, and shutdown have enforced bounds |
| A11 | Invariant violations stop the affected service; malformed input and database failures remain expected errors |
| A12 | Actual CLI/API demonstration, MC/DC mapping, race/static checks, and honest documentation support acceptance |

## 2. Contracts visible to the caller

### 2.1 Identity and immutable parameters

`request_id` is a caller-owned canonical lowercase UUID string: exactly 36 ASCII bytes, hexadecimal digits and the four standard hyphen positions. It is an opaque key; acceptance does not depend on its UUID version. The CLI creates a random UUID when omitted and prints it to stderr **before sending the mutation**. Explicit `--request-id` is supported for repeatable scripts. The caller must retain this key after any uncertain result.

`machine_id` is a separately generated UUID candidate supplied to the decision core by the service adapter. A losing retry's candidate is discarded. Only a committed record's ID is published as authoritative. The persisted winner must always be returned; never synthesize a fresh machine ID when handling a retry.

The request parameters are a comparable Go value with no mutable slices, maps, or pointer-owned children. Request IDs, candidate IDs, timestamps, and local timeouts are excluded from the parameter fingerprint. Include the schema version and every behavior-affecting parameter. Compare the normalized parameter fields as well as the fingerprint so even a hash collision cannot silently authorize changed intent.

| Parameter | Validation / default |
| --- | --- |
| `name` | 1–63 ASCII bytes; `[a-z0-9]` first and last, `[a-z0-9-]` internally; exact case, no implicit lowercasing |
| `image` | 1–128 ASCII bytes; letters, digits, `.`, `_`, `-`; opaque image name, not a path and not existence-checked in this slice |
| `vcpus` | Integer 1–64; CLI default 1 |
| `memory_mib` | Integer 128–1,048,576; CLI default 512 |
| `disk_mib` | Integer 1,024–16,777,216; CLI default 1,024 |

All fields are required in the HTTP body. The CLI applies defaults before sending them. The server accepts neither `null` fields nor duplicate JSON keys, unknown fields, fractional integers, overflowed integers, or trailing JSON values. Decoding uses presence-aware wire fields and a small strict object decoder; plain `encoding/json.Unmarshal` alone is insufficient to reject duplicate keys. The body limit applies before full allocation. Fuzz this boundary during implementation.

These ranges bound representable intent. They are not a promise that a later host can provision the requested resources. Arithmetic involving units uses checked conversions; no allocation is sized from these values in slice 1.

### 2.2 Routes and status semantics

| Route | Success | Important failures |
| --- | --- | --- |
| `PUT /v1/registrations/{request_id}` | `201` on newly committed record; `200` on matching existing record | `400` invalid input; `409` identifier/name conflict; `429` retained-record capacity; `503` busy/unavailable/uncertain |
| `GET /v1/registrations/{request_id}` | `200` committed registration | `404` no committed record observed; `503` unavailable |
| `GET /v1/machines/{machine_id}` | `200` committed registration by machine ID | `404` not observed; `503` unavailable |
| `GET /healthz` | `200` startup completed and a bounded database probe succeeds | `503` starting, stopping, incompatible schema, or unavailable database |

A `404` is a read observation. It makes no claim about an in-flight write. Retrying a mutation always uses `PUT` with the original request ID and original parameters. There is no HTTP endpoint for force-clearing reservations in this slice.

Proposed, not executed — request body:

```json
{
  "name": "demo-a",
  "image": "base",
  "vcpus": 1,
  "memory_mib": 512,
  "disk_mib": 1024
}
```

Proposed, not executed — successful registration response:

```json
{
  "request_id": "11111111-1111-4111-8111-111111111111",
  "machine_id": "22222222-2222-4222-8222-222222222222",
  "state": "registered",
  "provisioned": false,
  "parameters": {"name":"demo-a","image":"base","vcpus":1,"memory_mib":512,"disk_mib":1024},
  "created_at": "2026-09-23T00:00:00Z"
}
```

Proposed, not executed — uncertain write response:

```json
{
  "error": {
    "code": "outcome_unknown",
    "message": "Registration may have committed; inspect or retry with this request ID and the same parameters.",
    "request_id": "11111111-1111-4111-8111-111111111111"
  }
}
```

Use stable error codes: `invalid_request`, `request_conflict`, `name_conflict`, `registration_capacity`, `busy`, `unavailable`, `outcome_unknown`, `not_found`. Do not include connection strings or raw SQL in responses. Internal logs preserve diagnostic classification without private request dumps. A network failure at the CLI is uncertain even if the service never received the request; the CLI cannot know that from a missing response.

### 2.3 CLI and exit behavior

Proposed, not executed — intended commands:

```sh
perpetual machine register --request-id 11111111-1111-4111-8111-111111111111 \
  --name demo-a --image base --vcpus 1 --memory-mib 512 --disk-mib 1024
perpetual registration inspect 11111111-1111-4111-8111-111111111111
perpetual machine inspect 22222222-2222-4222-8222-222222222222
```

Successful stdout is one JSON object followed by a newline. Human diagnostics and the generated request ID go to stderr. Exit codes: `0` success, `2` invalid flags/input, `3` conflict, `4` not found, `5` capacity/busy/unavailable, `6` uncertain mutation. A timed-out read is unavailable; a timed-out mutation is uncertain. The CLI sends a mutation once per invocation. Do not hide uncertainty behind automatic HTTP retries or create a new ID after a failed send.

## 3. Structure and ownership

Use a local Go module named `perpetual` until a public module path is selected. Standard library for CLI/HTTP/JSON; use the `pgx/v5` driver and pool for PostgreSQL. During setup, pin an available Go 1.26 patch release, exact driver release, and PostgreSQL 18 fixture image digest. Record those resolved versions in committed build/fixture configuration. Do not invent version pins while writing an unexecuted design.

| Proposed path | Responsibility | Dependencies |
| --- | --- | --- |
| `cmd/perpetual/main.go` | Small CLI entrypoint and exit status | `internal/cli` |
| `cmd/agent-plane/main.go` | Config, process ownership, startup/shutdown, wiring | Config, hostlock, registration, store, HTTP |
| `internal/config/config.go` | Pure parsing/validation of supplied settings | Standard library |
| `internal/invariant/invariant.go` | Typed production invariant failure | Standard library |
| `internal/registration/types.go` | Immutable domain types and bounded classifications | Standard library |
| `internal/registration/validate.go` | Parameter validation and canonical fingerprint | Standard library |
| `internal/registration/admission.go` | Pure classification against an explicit observation | Domain and invariant |
| `internal/registration/step.go` | Pure request/completion transition function | Domain and invariant |
| `internal/registration/service.go` | One coordinator loop, bounded jobs/effects and result routing | Domain; narrow store interface |
| `internal/registration/sim_test.go`, `sim_store_test.go`, `trace_test.go` | Package-local fixed scheduler and modeled store, replay | Production decision functions |
| `internal/store/postgres/store.go`, `reserve.go`, `migrate.go` | Pool, transaction contract, schema versioning | Domain, pgx |
| `internal/store/postgres/migrations/001_registrations.sql` | Constraints and registration gate | PostgreSQL |
| `internal/hostlock/lock_linux.go` | Owned process file lock on Linux | Standard syscall/file boundary |
| `internal/httpapi/server.go`, `decode.go` | Bounded request parsing, JSON/error mapping, fatal boundary | Domain service interface |
| `internal/cli/run.go`, `client.go` | Injected I/O and HTTP client; stable request identity | Domain wire contracts |
| `internal/testfixture/`, `scripts/test-postgres.sh` | Disposable owned PostgreSQL and process helpers | Test owner only |
| `Makefile`, `.github/workflows/test.yml` | Executable developer/CI check targets | Actual implemented suites |

Companion tests live beside each package. Extract shared test fixtures only for actual reuse; the scheduler remains local to registration. Do not introduce an event bus, repository abstraction per table, service locator, or dependency injection framework.

### 3.1 Resource bounds

| Resource | Initial bound / handling |
| --- | --- |
| HTTP listen address | `127.0.0.1:7777`; configurable |
| Request body / response body | 4 KiB / 16 KiB; reject oversize, including streamed chunked bodies |
| HTTP read header / read / write / idle deadlines | 2 s / 5 s / 10 s / 30 s |
| Concurrent accepted connections | 128; listener admission caps before a handler is created |
| Coordinator jobs / input queue | 128 / 128; nonblocking overload classification |
| Database effect workers / pool connections | 4 / 8; validate workers < pool size to leave inspection capacity |
| Database lock / statement / total operation budget | 1 s / 5 s / 8 s; monotonic service-owned context |
| Confirmed-abort attempts | At most 3 within the original 8 s budget, no recursive retry |
| `max_registrations` | 1,024; persisted in gate row; startup rejects mismatched configuration |
| Shutdown grace | 10 s; stop admission, finish bounded work, cancel/join workers, close pool, release lock |
| Simulation steps / serialized trace | 256 steps / 1 MiB per baseline scenario; explicit failure at bound |

Validate positive counts and safe narrowing conversions. Never derive an unbounded channel, slice, goroutine count, or buffer from external input. All long loops check cancellation or a fixed work budget. These defaults are proposed starting values; tune only with evidence and matching tests.

## 4. Core design and TigerStyle contracts

### 4.1 Domain and admission

Proposed, not executed — `internal/registration/types.go` and `admission.go`. Imports and elementary constructors are omitted; use `time.Time` and the project's typed invariant helper. `ValidateParameters` rejects invalid input before these internal types enter admission.

```go
type RequestID string
type MachineID string

type Parameters struct {
	Name      string
	Image     string
	VCPUs     uint32
	MemoryMiB uint64
	DiskMiB   uint64
}

type Request struct {
	ID          RequestID
	CandidateID MachineID
	Parameters  Parameters
	Fingerprint [32]byte
}

type Record struct {
	RequestID   RequestID
	MachineID   MachineID
	Parameters  Parameters
	Fingerprint [32]byte
	CreatedAt   time.Time
}

type AdmissionKind uint8

const (
	AdmissionCreate AdmissionKind = iota + 1
	AdmissionExisting
	AdmissionRequestConflict
	AdmissionNameConflict
	AdmissionCapacity
)

// Observations for a mutation come from a transaction holding the gate.
// Absence outside that gate is not permission to create a registration.
type Observation struct {
	HasRequest bool
	Record     Record
	NameTaken  bool
	Used       uint64
	Limit      uint64
}

type Admission struct {
	Kind   AdmissionKind
	Record Record // Set only for AdmissionExisting.
}

func DecideAdmission(request Request, observed Observation) Admission {
	invariant.Check(observed.Limit > 0, "registration limit must be positive")
	invariant.Check(observed.Used <= observed.Limit, "registration count exceeds limit")
	if observed.HasRequest {
		invariant.Check(observed.Record.RequestID == request.ID, "lookup returned another request")
		if observed.Record.Parameters != request.Parameters {
			return Admission{Kind: AdmissionRequestConflict}
		}
		if observed.Record.Fingerprint != request.Fingerprint {
			return Admission{Kind: AdmissionRequestConflict}
		}
		return Admission{Kind: AdmissionExisting, Record: observed.Record}
	}
	if observed.NameTaken {
		return Admission{Kind: AdmissionNameConflict}
	}
	if observed.Used == observed.Limit {
		return Admission{Kind: AdmissionCapacity}
	}
	return Admission{Kind: AdmissionCreate}
}
```

The order is part of the contract: a matching retry succeeds before name or capacity admission for new records. Each guard has an independent meaning and failure message. Invalid external data returns an ordinary error in validation. An impossible count or mismatched lookup identity is an internal defect, not a user conflict.

The observation carries values, not a lock object. The storage adapter owns the gate and must enforce that precondition. Pure code cannot manufacture database isolation. Simulation models the same precondition rather than calling `DecideAdmission` on an arbitrary absent read.

Proposed, not executed — `internal/registration/validate.go`, canonicalization excerpt:

```go
// CanonicalParameters is deliberately a struct: field order and field set are
// explicit. Changing the encoding requires a version/migration decision.
type CanonicalParameters struct {
	Version   uint32 `json:"version"`
	Name      string `json:"name"`
	Image     string `json:"image"`
	VCPUs     uint32 `json:"vcpus"`
	MemoryMiB uint64 `json:"memory_mib"`
	DiskMiB   uint64 `json:"disk_mib"`
}

func Fingerprint(parameters Parameters) ([32]byte, error) {
	if err := ValidateParameters(parameters); err != nil {
		return [32]byte{}, err
	}
	canonical := CanonicalParameters{
		Version: 1, Name: parameters.Name, Image: parameters.Image,
		VCPUs: parameters.VCPUs, MemoryMiB: parameters.MemoryMiB,
		DiskMiB: parameters.DiskMiB,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return [32]byte{}, fmt.Errorf("encode registration parameters: %w", err)
	}
	return sha256.Sum256(encoded), nil
}
```

This encoding is application-owned, versioned canonicalization, not a claim of compliance with a general canonical JSON standard. Golden examples should use literal expected encoded bytes/digests; tests must not compute the expected digest with `Fingerprint` itself.

### 4.2 Events, effects, and completion certainty

Each admitted HTTP operation gets an internal job ID and an effect ID, distinct from durable request identity. Generate a service epoch outside the core and use a checked monotonically increasing sequence within that epoch. The service owns the routing map. A response waiter is a one-element buffered channel; a disconnected caller cannot block the coordinator.

Proposed, not executed — `internal/registration/step.go`, core shape:

```go
type Phase uint8
const (
	PhaseNew Phase = iota
	PhaseReserving
	PhaseResolved
	PhaseUncertain
)

type OutcomeKind uint8
const (
	OutcomeCreated OutcomeKind = iota + 1
	OutcomeExisting
	OutcomeRequestConflict
	OutcomeNameConflict
	OutcomeCapacity
	OutcomeBusy       // This attempt is known not to have committed.
	OutcomeUnavailable // This attempt is known not to have committed.
	OutcomeUnknown    // May have committed; never release durable ownership.
)

type Outcome struct {
	Kind   OutcomeKind
	Record Record // Valid only for Created or Existing.
}

type State struct {
	Phase    Phase
	EffectID EffectID
	Request  Request
	Outcome  Outcome
}

type Event interface{ registrationEvent() }
type Submit struct{ Request Request; EffectID EffectID }
type Completed struct{ EffectID EffectID; Outcome Outcome }
type Effect interface{ registrationEffect() }
type Reserve struct{ EffectID EffectID; Request Request }
type Reply struct{ Outcome Outcome }

func Step(state State, event Event) (State, []Effect) {
	switch event := event.(type) {
	case Submit:
		invariant.Check(state.Phase == PhaseNew, "job submitted twice")
		next := State{
			Phase: PhaseReserving, EffectID: event.EffectID, Request: event.Request,
		}
		return next, []Effect{Reserve{EffectID: event.EffectID, Request: event.Request}}
	case Completed:
		invariant.Check(state.Phase == PhaseReserving, "completion without pending effect")
		invariant.Check(state.EffectID == event.EffectID, "completion for another effect")
		CheckOutcome(state.Request, event.Outcome)
		next := state
		next.Phase = PhaseResolved
		if event.Outcome.Kind == OutcomeUnknown {
			next.Phase = PhaseUncertain
		}
		next.Outcome = event.Outcome
		return next, []Effect{Reply{Outcome: event.Outcome}}
	default:
		invariant.Check(false, "unsupported registration event")
		return state, nil // Unreachable; Check must not return on false.
	}
}
```

Omitted declarations: `EffectID` contains epoch/sequence; marker methods restrict event/effect variants to this package. `CheckOutcome` accepts only the declared enum range; success requires matching request ID, immutable parameters/fingerprint, a nonempty canonical machine ID and valid UTC timestamp. Non-success outcomes must not publish an authoritative record. Each relationship gets its own assertion.

The coordinator discards stale completions from a previous service epoch **before** calling `Step`. An impossible duplicate/misrouted completion from the current owned worker contract is an invariant defect. HTTP retries are separate jobs sharing a durable request ID; they are not repeat `Submit` events on one state. Do not grow an unbounded in-memory history of request IDs.

`PhaseUncertain` records uncertainty for this response. Finishing the HTTP job does not cancel the external transaction or release its database locks. PostgreSQL owns the unresolved transaction until commit/abort. A subsequent job uses the original request ID and gate; it never uses local absence to overwrite another reservation.

| Current phase / event | Transition and external action | Observable result |
| --- | --- | --- |
| New / valid Submit | Reserving; request one database effect | No success before persistence |
| Reserving / Created or Existing | Resolved; reply with committed record | 201 or 200 |
| Reserving / confirmed rejection | Resolved; reply with stable error code | No record created by this attempt |
| Reserving / Unknown | Uncertain; reply with request ID only | Caller must inspect/retry original identity |
| Any live job / caller disconnect | Waiter detaches; owned effect still bounded by service budget | No implied cancellation of accepted work |
| Any / process crash | Volatile jobs are discarded | Database outcomes remain independent |
| Restart | Construct a fresh coordinator after lock/schema checks | Reads query durable records; retries re-enter the gate |

### 4.3 Invariants and fatal handling

Core state uses value semantics. `Step`, validation and admission must not mutate their inputs. Ordered event delivery is explicit; no clocks, random reads, goroutines, environment access, or I/O enter those functions.

Required invariants: one committed request maps to one immutable machine identity; a name has one owner; retained count never exceeds limit; no successful reply precedes a committed/observed record; no stale effect can complete a different job. Database constraints and independent simulator checks support these rules, but neither replaces the other.

Use a typed `invariant.Violation` as described by TigerStyle. Keep coordinator assertions in its owned goroutine outside `net/http` recovery. A top-level worker boundary logs the violated rule and causes the service process to exit nonzero; it must never resume that worker. Wrap HTTP handlers with a fatal boundary for the same typed violation, since Go otherwise recovers handler panics. Other unexpected panics should also terminate rather than leave a partially healthy service. Test both coordinator and handler paths in subprocesses. Validation failures, database errors and context deadlines must not panic.

## 5. PostgreSQL durability boundary

### 5.1 Schema and migration

Proposed, not executed — `internal/store/postgres/migrations/001_registrations.sql`:

```sql
CREATE TABLE schema_migrations (
    version bigint PRIMARY KEY CHECK (version > 0),
    checksum text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE registration_gate (
    singleton_id smallint PRIMARY KEY CHECK (singleton_id = 1),
    used bigint NOT NULL CHECK (used >= 0),
    max_registrations bigint NOT NULL CHECK (max_registrations > 0),
    CHECK (used <= max_registrations)
);

CREATE TABLE machine_registrations (
    request_id uuid PRIMARY KEY,
    machine_id uuid NOT NULL UNIQUE,
    name text NOT NULL UNIQUE
        CHECK (octet_length(name) BETWEEN 1 AND 63)
        CHECK (name ~ '^[a-z0-9]([a-z0-9-]*[a-z0-9])?$'),
    image text NOT NULL
        CHECK (octet_length(image) BETWEEN 1 AND 128)
        CHECK (image ~ '^[A-Za-z0-9._-]+$'),
    vcpus integer NOT NULL CHECK (vcpus BETWEEN 1 AND 64),
    memory_mib bigint NOT NULL CHECK (memory_mib BETWEEN 128 AND 1048576),
    disk_mib bigint NOT NULL CHECK (disk_mib BETWEEN 1024 AND 16777216),
    fingerprint_version smallint NOT NULL CHECK (fingerprint_version = 1),
    fingerprint bytea NOT NULL CHECK (octet_length(fingerprint) = 32),
    state text NOT NULL CHECK (state = 'registered'),
    created_at timestamptz NOT NULL DEFAULT now()
);
```

Create the singleton gate with `used = 0` and the configured limit in the same initialization transaction; bind the value as a parameter. Include the migration's version and checksum in that transaction. Do not run migrations per request. Start only with the exact supported schema version/checksum; reject an unknown future schema or altered already-applied migration.

During startup, verify the gate's persisted limit matches configuration and `used` equals the actual record count under the gate. The initial 1,024-record bound makes this full check reasonable. The count relationship is cross-table and therefore an application invariant; the SQL `CHECK` alone cannot prove it. Manual SQL mutation is outside the supported write API, and detected inconsistency stops startup. No delete/reset API or counter repair shortcut is introduced here.

Use one migration transaction, bounded statement/lock timeouts, the host process lock, and a migration-specific database advisory transaction lock to prevent concurrent initialization against the same dedicated database. This advisory lock is only migration coordination; runtime reservations use the visible singleton row. Set its constant key in one named declaration, with a comment explaining ownership. Do not create database roles or modify an operator's server as incidental startup work.

### 5.2 Reservation transaction

Every mutation follows this order under Read Committed:

1. Begin a bounded transaction; set transaction-local lock and statement timeouts.
2. Lock the singleton `registration_gate` row with `SELECT ... FOR UPDATE`.
3. In a **subsequent statement**, look up the original request ID. If it exists, return matching existing intent or conflict using the pure decision function.
4. If absent, query the requested name and gather the gate count/limit. Call `DecideAdmission` using those observations.
5. For new admission, insert the complete immutable record with the supplied candidate machine ID, then increment `used` with a guarded update. Check the affected-row count and returned values.
6. Commit. Return `OutcomeCreated` only after successful commit acknowledgment. Inspect or re-enter this same protocol after ambiguity.

Read Committed takes a fresh snapshot for each statement. The lookup after acquiring the gate observes the outcome of the earlier gate owner; it must not share a pre-wait snapshot in one elaborate SQL expression. PostgreSQL's [transaction isolation documentation](https://www.postgresql.org/docs/current/transaction-iso.html) explains statement snapshots and uniqueness interactions. This plan chooses an explicit gate so the recovery proof does not depend on interpreting an absent lookup during a pending insert.

Only registration transactions hold this gate. They perform no HTTP, filesystem, guest, or process effect while holding it. Inspect routes read committed rows without the gate. Acquire the gate before any registration row mutation and use one lock order; [PostgreSQL locking guidance](https://www.postgresql.org/docs/current/explicit-locking.html) describes why consistent ordering matters.

The gate serializes new registrations across names. A pending transaction can delay other mutations up to their lock budget; return `busy` rather than wait indefinitely. This limitation is deliberate for this small slice. It must not become the synchronization mechanism for independent machine work in later slices.

Proposed, not executed — `internal/store/postgres/reserve.go`, **pseudocode with Go-shaped signatures**. The helpers below have contracts specified after the block; this is not a paste-ready driver implementation.

```go
func (store *Store) Reserve(ctx context.Context, request Request) Outcome {
    for attempt := 1; attempt <= 3; attempt++ {
        outcome, retryableAbort := store.reserveOnce(ctx, request)
        if !retryableAbort {
            return outcome
        }
        if ctx.Err() != nil {
            return Outcome{Kind: OutcomeUnavailable}
        }
    }
    return Outcome{Kind: OutcomeUnavailable}
}

func (store *Store) reserveOnce(ctx context.Context, request Request) (Outcome, bool) {
    tx, err := store.beginReadCommitted(ctx)
    if err != nil {
        return unavailableOrBusy(err), isConfirmedRetryableAbort(err)
    }
    defer store.rollbackOrDiscard(tx) // Bounded cleanup; preserve original failure.

    gate, err := store.lockRegistrationGate(ctx, tx)
    if err != nil {
        return unavailableOrBusy(err), isConfirmedRetryableAbort(err)
    }
    observed, err := store.observeAfterGate(ctx, tx, gate, request)
    if err != nil {
        return unavailableOrBusy(err), isConfirmedRetryableAbort(err)
    }
    admission := registration.DecideAdmission(request, observed)
    if admission.Kind != registration.AdmissionCreate {
        // No writes were issued. Existing records are immutable and committed.
        return outcomeFromAdmission(admission), false
    }

    record, err := store.insertRegistration(ctx, tx, request)
    if err != nil {
        return classifyInsertFailure(err), isConfirmedRetryableAbort(err)
    }
    if err := store.incrementRegistrationCount(ctx, tx, gate); err != nil {
        return unavailableOrBusy(err), isConfirmedRetryableAbort(err)
    }
    if err := tx.Commit(ctx); err != nil {
        if isConfirmedRetryableAbort(err) {
            return Outcome{Kind: OutcomeUnavailable}, true
        }
        if isConfirmedCommitRollback(err) {
            return Outcome{Kind: OutcomeUnavailable}, false
        }
        // COMMIT may have reached PostgreSQL. Connection loss is not rollback.
        return Outcome{Kind: OutcomeUnknown}, false
    }
    return Outcome{Kind: OutcomeCreated, Record: record}, false
}
```

Helper contracts and details that implementation must preserve:

- `beginReadCommitted` acquires one pooled connection within the operation budget and uses `SET LOCAL` values inside the transaction. No caller-controlled SQL interpolation.
- `lockRegistrationGate` requires exactly one gate row and checks its internal bounds. Missing/multiple rows or an impossible counter is an invariant failure.
- `observeAfterGate` performs the request lookup after gate acquisition as a distinct statement. For an existing request, it returns that record; otherwise it obtains name ownership. No absence result escapes as admission permission without this gate.
- `insertRegistration` uses explicit columns, parameters, `INSERT ... RETURNING`, and checked type conversions. Never use `ON CONFLICT DO UPDATE` to replace intent. A conflicting random machine ID is a returned internal availability error with diagnostic context, no new candidate silently generated mid-retry. A uniqueness failure inconsistent with the held gate and prior reads is an internal invariant defect under the supported single-writer protocol; catch it through the fatal boundary rather than pretending admission succeeded.
- `incrementRegistrationCount` updates `used = used + 1` with `used < max_registrations`, returns the new value, and requires exactly one changed row. Since limits fit signed `bigint`, validated values cannot overflow.
- `rollbackOrDiscard` uses a fresh bounded cleanup context, records a rollback failure without erasing the original outcome, and removes an unusable connection from reuse. It never changes `OutcomeUnknown` to a known failure.
- Retry only SQLSTATE `40001` and `40P01` when the adapter has confirmed transaction abort. At most three attempts share the original request, candidate ID, and overall budget. All retries are database-only. An ordinary lock timeout returns `busy`; a general connection failure after `COMMIT` was sent returns `outcome_unknown`.
- A failure before any `COMMIT` dispatch cannot autonomously commit this adapter's transaction; rollback/connection disposal still needs bounded cleanup. Be conservative when driver return values cannot establish where the failure occurred.
- The production driver integration must verify these classifications. The pseudocode is a contract outline, not evidence about exact pgx error behavior.

### 5.3 Why unknown results are safe

Suppose transaction T1 holds the gate, inserts R1, sends COMMIT, and loses its connection before the service receives an answer. A plain lookup from another connection can temporarily return no row. The next mutation T2 must still acquire the same gate.

If T1 commits, T2's subsequent request lookup sees the winning record. The same parameters return that record; different parameters conflict. If T1 aborts, T2 acquires the gate and may create a record under the original request ID. If T1 remains unresolved, T2 waits only to its lock deadline and returns busy without writing. An insert or counter increment cannot survive alone because they share a transaction.

If T1 had not acquired the gate, another transaction may win first. That is legal concurrency: only the committed winner defines the intent. The unique request ID, machine ID, and name constraints defend against duplicates independently of timing. A failure or timeout never authorizes bypassing them.

### 5.4 Reads and restart

`LookupRequest` and `LookupMachine` are bounded read-only queries returning `(Record, found, error)`. Read errors are distinct from absence. The service never turns a connection error into `404`.

Restart constructs an empty coordinator through the normal production constructor after lock, migration, gate consistency and pool checks. Inspection queries PostgreSQL; registration retries query it under the mutation gate. There is no authoritative memory cache to reload, and no need to scan all request IDs into RAM. Startup may be unavailable while an older registration transaction resolves; it must not reset the gate or counter to force readiness.

## 6. Service, transport, and process boundaries

### 6.1 Narrow interfaces and lifetime

Proposed, not executed — `internal/registration/service.go`:

```go
type Store interface {
    Reserve(context.Context, Request) Outcome
    LookupRequest(context.Context, RequestID) (Record, bool, error)
    LookupMachine(context.Context, MachineID) (Record, bool, error)
}

type Service interface {
    Register(context.Context, Request) Outcome
    InspectRequest(context.Context, RequestID) (Record, bool, error)
    InspectMachine(context.Context, MachineID) (Record, bool, error)
}
```

The HTTP handler creates a validated domain request with a candidate ID. `Register` attempts bounded admission into the coordinator. If the caller cancels before enqueue, return a known local rejection. Once admitted, the service owns the database effect and its eight-second context, derived from the service lifecycle rather than the HTTP request. The handler may stop waiting; it does not cancel or duplicate the effect.

The coordinator alone owns job state and effect correlation. Workers own one adapter call at a time and return one completion. Completion delivery has reserved capacity at least equal to the number of active workers, independent of the request queue. The loop must not block waiting to dispatch an effect to a full worker queue while workers are trying to report completions. Keep pending effects in a bounded owned list and select between available dispatch and completions; do not launch a goroutine per blocked send.

Each job has at most one active reservation effect and one eventual reply. Reject new admission when job capacity is exhausted. Remove a finished job after delivering or discarding its response; durable ownership remains in PostgreSQL. Reads use a separately bounded path and the pool's remaining capacity. The shared admission function still runs inside every real reservation transaction.

Every worker has service-owned cancellation and a join path. Workers never close a shared channel; the owning loop closes dispatch after admission stops. Cleanup order is specified below. Do not assert that arbitrary Go `select` ordering is deterministic: the simulator controls its own explicit event order, while production applies the same transitions to its observed order.

### 6.2 HTTP handling sketch

Proposed, not executed — `internal/httpapi/server.go`. `DecodeRegistration`, `NewRequest`, `WriteOutcome`, and error helpers follow the contracts in sections 2 and 4; this excerpt shows the effect boundary.

```go
func (server *Server) PutRegistration(w http.ResponseWriter, r *http.Request) {
    requestID, err := ParseRequestID(r.PathValue("request_id"))
    if err != nil {
        WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
        return
    }
    r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
    parameters, err := DecodeRegistration(r.Body)
    if err != nil {
        WriteError(w, http.StatusBadRequest, "invalid_request", "invalid registration body")
        return
    }
    candidateID, err := server.newMachineID()
    if err != nil {
        WriteError(w, http.StatusServiceUnavailable, "unavailable", "identity generation failed")
        return
    }
    request, err := NewRequest(requestID, candidateID, parameters)
    if err != nil {
        WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
        return
    }
    outcome := server.service.Register(r.Context(), request)
    // Failure to write a reply does not undo a committed registration.
    if err := WriteOutcome(w, requestID, outcome); err != nil {
        server.logReplyFailure(requestID, err)
    }
}
```

Read and write failures are handled and bounded. Success/error body encoding happens before writing response headers; response encoding failure cannot produce a misleading success. The strict decoder rejects unsupported content types and malformed/trailing input. The service controls what happens after enqueue; the handler is not the owner of accepted work.

### 6.3 Startup and shutdown

Startup order:

1. Parse supplied config without effects; reject invalid bounds and missing database configuration.
2. Open the canonical host lock file under the configured runtime directory and take a nonblocking exclusive `flock`. Default `/run/perpetual/agent-plane.lock`; tests use an owned temporary directory. Never unlink the lock file while another process could hold its inode. Keep the descriptor close-on-exec.
3. Connect the bounded pool; migrate/verify the dedicated database, gate and schema.
4. Construct the coordinator and worker set. Start the bounded listener and readiness route.

On SIGTERM/SIGINT or fatal startup failure: stop new admission/readiness, stop accepting connections, let admitted work finish within the ten-second grace, cancel remaining service operations, join workers, close the pool, and finally release the host lock. If join cannot complete by its bound, report the failure and terminate the service; process exit closes owned sockets. Do not declare an unacknowledged write aborted. No guest cleanup exists in this slice.

A second service using the canonical lock path must fail before migrations or request handling. The lock is host process ownership, not distributed leadership. The MVP remains one control plane per host, with one dedicated database. Choosing a different runtime path deliberately does not establish a supported second leader.

## 7. Deterministic simulation design

Keep the first scheduler in registration test files. It operates on a list of typed events, explicit pending effects, and a modeled store. There is no random generator, real clock, goroutine scheduler emulation, real database, or real network. Use supplied fixed IDs and timestamps.

The modeled store has three separate pieces: committed records/counter, one gate owner with its uncommitted candidate state, and pending deliveries. A commit atomically publishes both record and counter. A rollback publishes neither. Commit outcome and acknowledgment delivery are separate choices. A service crash discards jobs/epoch/waiters, while a database transaction already dispatched can still resolve. A database crash is a separate event that discards uncommitted work and preserves modeled committed records.

The simulator runs production `Step`, `DecideAdmission`, validation and reconstruction paths. It models storage outcomes and gate waiting, not PostgreSQL query execution. Assertions independently examine recorded replies, issued effects, durable identities, immutable parameters, count, and gate ownership. They must not call `DecideAdmission` to predict the correct result.

### 7.1 Required fixed scenarios

| Case | Ordered fault sequence | Forbidden result / required progress |
| --- | --- | --- |
| S01 | Submit R1; commit; deliver storage completion; drop HTTP reply; retry R1; restart; inspect | One machine identity throughout; matching retry returns original record |
| S02 | Submit R1; hold transaction before resolution; lose connection; observe absent row; retry same ID; release original commit | No second record or overwritten parameters; original committed identity returned |
| S03 | As S02, but retry changes parameters | Pending/busy before resolution; conflict after original commit |
| S04 | As S02, original transaction aborts | Matching retry may create exactly one record; no leaked counter |
| S05 | Two IDs race for one name | One winner, one name conflict, one capacity increment |
| S06 | Capacity has one free slot; submit two distinct names | Exactly one new commit; matching retry succeeds at full capacity |
| S07 | Crash before dispatch, after dispatch, after commit, before reply | No assumption that a dispatched effect was canceled; restart uses production reconstruction |
| S08 | Deliver completion from old epoch after restart | No reply or state mutation to a new job |
| S09 | Faults stop; deliver eligible work fairly | Requests resolve within explicit event budget; report pending work if they do not |
| S10 | Replay incompatible version or nonexistent effect ID | Clear incompatibility error; no invented event or silently ignored mismatch |

Each scenario checks invariants after every delivered event and persistence transition. Fair progress claims state that PostgreSQL eventually resolves the gate owner and healthy completions are delivered. S02/S03 may remain unresolved while that assumption is deliberately withheld; report uncertainty rather than manufacturing progress.

Proposed, not executed — scenario sketch in `internal/registration/sim_test.go`. Helper names describe the intended harness API; the test owner may refine them while preserving these observations.

```go
func TestPendingCommitCannotAuthorizeConflictingReuse(t *testing.T) {
    h := NewHarness(t, FixedConfig())
    original := FixtureRequest("request-a", "machine-a", "demo-a")
    conflict := original
    conflict.Parameters.MemoryMiB = 1024
    conflict.Fingerprint = FixtureFingerprintForChangedMemory()

    first := h.Submit(original)
    h.DispatchReserve(first)
    h.HoldBeforeCommit(first)
    h.LoseStorageReply(first)
    h.AssertReply(first, OutcomeUnknown)
    h.AssertLookupAbsent(original.ID) // Observation, not permission.

    second := h.Submit(conflict)
    h.DispatchReserve(second)
    h.AssertWaitingForGate(second)
    h.Commit(first)
    h.DeliverEligibleCompletions()

    h.AssertReply(second, OutcomeRequestConflict)
    h.AssertCommittedRecord(original.ID, original.CandidateID, original.Parameters)
    h.AssertCommittedCount(1)
    h.AssertNoDuplicateIdentity()
}
```

Fixture helpers map readable symbolic names to valid UUIDs; they do not bypass input validation. The changed-memory fingerprint is a literal fixture value independently established from the specified encoding. `LoseStorageReply` models an unknown completion without resolving the held database transaction. Harness bookkeeping must permit that transaction to commit after the job has returned uncertainty.

### 7.2 Trace and replay

Header fields: format/harness version, source revision plus dirty-tree indicator, Go toolchain, scenario name, configuration, initial committed records and pending transactions. Each step has a monotonically increasing event number, event/effect ID, chosen delivery/fault, observation or completion, requested effects and visible replies. Exclude secrets, connection strings and arbitrary runtime data.

Write JSON Lines for machine replay and render a concise text explanation on failure. Enforce event/byte bounds before appending. Record actual choices; replay never regenerates a scenario from a seed. Normalize only nonsemantic diagnostics such as temporary paths and wall-clock log times. Do not normalize away request IDs, machine IDs, parameters, event order, or persisted timestamps chosen by the scenario.

Reconstruct fresh state twice from the same trace and compare normalized trace and outcome. Reject unsupported versions and causally impossible deliveries. Retain short regression scenarios for useful failures. During implementation, temporarily remove or break a specific ownership check, demonstrate that the independent oracle detects the forbidden result, restore the correct code and rerun the focused check. Record both results; do not commit the deliberate defect.

## 8. Test design and failure evidence

The test owner writes executable suites during implementation, before releasing their dependent production packets. This section specifies those suites; actual execution evidence belongs in the testing guide. Assertions derive from the contracts above. Test setup may create compiling types/interfaces and explicit unimplemented skeletons. A compile error or missing database is not useful TDD red evidence.

### 8.1 Basic tests and MC/DC inventory

| Test group | Cases and independent oracle | Requirement |
| --- | --- | --- |
| T01 Validation | IDs malformed at each structural boundary; name/image below/at/above byte limits; absent/null/duplicate/unknown JSON fields; integer limits and overflow; trailing values | A03, A10 |
| T02 Canonicalization | Literal canonical bytes/digest for fixed fixtures; differing whitespace/key order gives same parameters; every parameter change changes compared intent; caller input unchanged | A02, A03 |
| T03 Admission | New, existing, parameter mismatch, fingerprint mismatch, name conflict, capacity; retry at capacity; invariant states fail with distinct messages | A01–A06, A11 |
| T04 Step | One reserve then one reply; no premature success; unknown remains uncertain; invalid internal event/phase/effect/outcome fails; input unchanged | A05, A07, A11 |
| T05 Service | Enqueue/cancel boundary, detached waiter, full queues, completion under saturation, shutdown and worker joins, stale epoch routing, inspection while mutations wait | A04, A05, A10 |
| T06 HTTP/CLI | Actual local round trips for every status; generated ID printed before send; one mutation send; bounded bodies; canceled write yields uncertainty; no fake provisioned status | A01–A05, A10 |
| T07 Store contract | Shared cases against simulated and PostgreSQL adapters; persisted winner/counter and classification, not just returned error | A01–A06, A09 |
| T08 Fatal/ownership | Real subprocess worker and handler violations exit nonzero; invalid input stays alive; second lock owner rejected; shutdown releases lock | A10, A11 |
| T09 Trace/simulation | S01–S10; twice-replayed normalized equivalence; byte/event bounds; deliberate defect detected | A07–A09 |
| T10 Migration/restart | Fresh/repeated migration, checksum/future-version rejection, gate consistency; service restart distinct from database restart | A04, A09, A10 |

For the proposed admission function, use these explicit independence cases. Each `if` is a separate single-condition decision; do not report this table as covering the rest of the code.

| Case | Has request | Parameters equal | Fingerprint equal | Name taken | Used / limit | Expected |
| --- | --- | --- | --- | --- | --- | --- |
| C01 | false | not evaluated | not evaluated | false | 1 / 2 | Create |
| C02 | true | true | true | false | 1 / 2 | Existing |
| C03 | true | false | true | false | 1 / 2 | Request conflict |
| C04 | true | true | false | false | 1 / 2 | Request conflict |
| C05 | false | not evaluated | not evaluated | true | 1 / 2 | Name conflict |
| C06 | false | not evaluated | not evaluated | false | 2 / 2 | Capacity |
| C07 | true | true | true | true | 1 / 1 | Existing |

Pairs: C01/C02 for `HasRequest`; C02/C03 for parameter equality; C02/C04 for fingerprint equality; C01/C05 for name ownership; C01/C06 for capacity. C07 establishes retry precedence. In C01, the one retained record belongs to an unrelated request/name; in C02 it is the matched record. NameTaken is only queried for absent requests and is ignored for existing ones. These observations avoid a fictitious existing record with a zero retained count. Artificial fingerprint mismatch is a defensive input to the decision test, not an external accepted request. Separate assertion cases cover zero limit, used above limit, and wrong lookup ID; fatal predicates belong in the decision inventory too.

Proposed, not executed — `internal/registration/admission_test.go`, representative table structure. `BaseRequest` and `BaseRecord` provide explicit fixed valid fixtures. The full suite must add every row above and invariant cases.

```go
func TestAdmissionPreservesExistingIntent(t *testing.T) {
    request := BaseRequest()
    record := BaseRecord()
    changed := request
    changed.Parameters.MemoryMiB = 1024
    cases := []struct {
        name string
        request Request
        observed Observation
        want AdmissionKind
    }{
        {
            name: "matching retry at full capacity",
            request: request,
            observed: Observation{
                HasRequest: true, Record: record, NameTaken: true, Used: 1, Limit: 1,
            },
            want: AdmissionExisting,
        },
        {
            name: "changed parameters cannot overwrite recorded intent",
            request: changed,
            observed: Observation{
                HasRequest: true, Record: record, NameTaken: true, Used: 1, Limit: 1,
            },
            want: AdmissionRequestConflict,
        },
    }
    for _, test := range cases {
        t.Run(test.name, func(t *testing.T) {
            before := test.observed
            got := DecideAdmission(test.request, test.observed)
            if got.Kind != test.want {
                t.Fatalf("admission = %v, want %v", got.Kind, test.want)
            }
            if test.observed != before {
                t.Fatal("admission mutated its observation")
            }
            if test.want == AdmissionExisting && got.Record != record {
                t.Fatal("retry did not return the literal recorded winner")
            }
            if test.want != AdmissionExisting && got.Record != (Record{}) {
                t.Fatal("rejection published a record")
            }
        })
    }
}
```

The test owner maintains a decision inventory covering validation, adapters, configuration, handlers, service orchestration, and cleanup as those functions are implemented. Label case pairs, actual evaluated conditions, infeasible combinations with reasons, and uncovered gaps. Go statement coverage is separate evidence. A planned or unexecuted case contributes no coverage.

Fuzz targets: strict request decoding, identifier/name validation, canonical encoding round trips, and pure admission/transition inputs constructed from reachable states. Seed malformed/empty/limit values and stable fixtures. Bound each input and each run; retain minimized failures with their fix. Real PostgreSQL, subprocesses, and sockets stay outside fuzz targets. Fuzzing supplements explicit condition pairs.

### 8.2 Real PostgreSQL fixtures and concurrency

Use an explicitly owned disposable PostgreSQL 18 instance for restart tests, with the production migrations and driver. Other integration cases may share that owned server with isolated per-test databases. Record the pinned image digest, server version, Read Committed setting, `fsync=on`, `synchronous_commit=on`, and `full_page_writes=on`. Refuse restart operations unless the fixture owns the server. Missing prerequisites fail a requested integration target.

Fixture startup uses readiness probes, not fixed sleeps. Every child/container/database has an owner and bounded teardown. A failure keeps useful sanitized logs and reports surviving resources. A fixture must not read an arbitrary operator DSN and assume permission to destroy or restart it.

| Real case | Coordination/fault mechanism | Evidence to retain |
| --- | --- | --- |
| P01 matching concurrent retry | Distinct connections start on a barrier | One row, one counter increment, both responses identify the same record |
| P02 conflicting request ID | Same ID, different parameters; control first gate owner | Winner unchanged; loser conflicts |
| P03 same machine name | Different IDs/candidates, same name | One name owner; no leaked registration count |
| P04 final capacity slot | Gate configured with one slot; race two names | One commit, one capacity rejection; retry winner still succeeds |
| P05 pending before commit | First connection holds gate after insert; second connection verifies absent committed row; retry through production adapter | Retry cannot mutate before gate resolves; test both explicit commit and rollback of first connection |
| P06 lost commit acknowledgment | A test-only transport boundary lets COMMIT complete at the backend but withholds its response from the adapter | Adapter reports unknown; inspection/retry finds original identity; no second insert |
| P07 cancellation/lock timeout | Hold gate using a fixture-owned session; cancel/wait using explicit handshake | Bounded busy/cancel response and no write; inspect unrelated committed row still works |
| P08 confirmed transaction abort | Controlled real contention/deadlock where feasible; separate driver classification tests | Real abort evidence plus bounded retries using original identity; never mislabel mock errors as real server coverage |
| P09 service crash | Kill owned service after handshake at selected request/commit boundary; keep PostgreSQL alive | Restart and inspect/retry original ID; count stable |
| P10 database restart | Restart only owned fixture, while service/pool exists | Old connection fails honestly; reconnect and inspect previously committed records |
| P11 migration/constraints | Fresh/repeat/future schema, invalid direct inserts in a rollback-owned transaction | Schema guard and database constraints reject violations |

P06 uses a test-only `net.Conn` wrapper supplied through the production driver's connection configuration. Run that adapter test with one isolated connection and plaintext transport to the owned local fixture (`sslmode=disable` for this fixture only). The wrapper reads PostgreSQL backend frames with bounded framing buffers and normally forwards their exact bytes. Arm it after migration/setup and before the one reservation under test. When it reads the backend `CommandComplete` frame whose tag is `COMMIT`, signal `commit_observed`, withhold that frame and the following readiness bytes from the driver, close the connection, and return EOF. A separate real connection then verifies the committed row/counter. The adapter has not received the successful commit response and must classify its result as unknown.

The test owner verifies the exact pgx configuration seam and PostgreSQL framing while implementing this fixture; unsupported framing fails the fixture clearly. Bound all handshake waits and frame lengths, pass initialization frames unchanged, and never enable this wrapper in production wiring. This makes the fault location observable without a sleep or a fabricated `Commit` result. P05 establishes the pending-lock hazard; P06 separately validates real driver behavior when an actual successful backend response is lost. It does not claim packet-level network simulation.

For P08, production classification tests can cover SQLSTATE handling deterministically, but a fabricated error is not evidence of PostgreSQL contention. Report each layer separately. Bound retries by both attempts and original context deadline; test exhaustion and preservation of request/candidate identity.

### 8.3 Failure matrix

| Cut point | What can survive | Legal recovery | Forbidden conclusion |
| --- | --- | --- | --- |
| Before request enqueue | No accepted service job | Caller may repeat same ID | No need to invent a replacement ID |
| After enqueue, caller disconnects | Owned effect may still run | Inspect or retry original ID | Caller cancellation rolled back acceptance |
| Transaction waiting for gate | Existing owner may commit | Wait to bound, then busy; retry later | Missing row makes the gate unnecessary |
| Insert before commit | Uncommitted record/counter | Transaction commit or rollback | Publish machine identity as confirmed |
| COMMIT sent, response lost | Record may already be durable | Read; if absent, re-enter gate | Connection error proves rollback |
| Commit acknowledged, HTTP reply lost | Committed record | Matching retry returns same identity | Register another machine automatically |
| Process crash with dispatched transaction | DB transaction may outlive process | Fresh constructor and same gate | Restore the old in-memory job or assume it aborted |
| Database restart | Committed state subject to actual database durability contract | Reconnect, validate schema, inspect | Simulation established filesystem/power-loss durability |
| Invariant violation | Service state may be invalid | Terminate service; diagnose and restart safely | Return an ordinary HTTP error and continue |

## 9. Delegation and TDD work packets

Model assignments are explicit here rather than in general repository guidance. They define work ownership; actual handoffs and results belong in the local ledger. Use the [skill's routing policy](../../../.agents/skills/detailed-design/SKILL.md) when resolving runtime availability.

For Codex, select Astra for design/testing/review, Sol for substantial production work, and Luna high for settled scaffolding. For Claude, use Fable for the highest-tier tasks when exposed by that environment, otherwise Opus; use Opus for substantial implementation and Sonnet high for settled scaffolding. Do not guess a Fable model identifier. Record the actual available ID and effort in state before dispatch. Highest-tier test and reviewer roles are separate agent runs even if they use the same model.

### 9.1 Task order and file ownership

All assigned reasoning effort is high where supported. A runtime lacking the requested effort/model is recorded and resolved according to the skill before dispatch. The lead remains responsible for integration and acceptance.

| Task | Primary / Claude alternative | Owned files and deliverable | Prerequisite | Gate |
| --- | --- | --- | --- | --- |
| DR00 Setup | Luna high / Sonnet high | `go.mod`, `go.sum`, binary/package shells, non-test build target, config example; resolve version pins | Implementation requested | Compiling empty module and real pinned prerequisites; no behavior claims |
| DR01 Core test contract | Astra / Fable, fallback Opus | Domain types/interfaces/skeletons, T01-core/T02-core/T03/T04 tests, initial MC/DC inventory, `TESTING.md` | DR00 | Meaningful red assertions for core behavior; accepted signatures |
| DR02 Pure core | Sol / Opus | `registration/{types,validate,admission,step}.go`, `invariant.go`; type ownership explicitly transferred from DR01 | DR01 red gate | T01-core/T02-core/T03/T04 green; immutable inputs and fatal contracts reviewed |
| DR03 Storage and simulator tests | Astra / Fable, fallback Opus | T07/T09/T10, S01–S10, P01–P11 fixtures, PostgreSQL skeleton signatures, `TESTING.md`, test targets and CI | DR01 contracts; DR02 for final production-core scenarios | Storage assertions fail for absent adapter behavior; deterministic harness and fault controls have evidence |
| DR04 PostgreSQL production | Sol / Opus; escalate unresolved recovery to Astra/Fable | `store/postgres/*.go`, migration SQL, exact dependency additions by handoff | DR03 storage red gate | Adapter cases P01–P08/P10/P11 green; P09/service integration deferred to DR06 and final integration |
| DR05 Boundary test contract | Astra / Fable, fallback Opus | T01-wire/T02-wire/T05/T06/T08, service/HTTP/CLI/config/lock skeleton signatures, subprocess fixtures and `TESTING.md` | DR01 contracts; may overlap DR04 after signatures settle | Meaningful red assertions for transport, ownership, bounds, fatal handling |
| DR06 Service and HTTP | Sol / Opus | `registration/service.go`, `httpapi/*.go`, `hostlock/lock_linux.go`, `cmd/agent-plane/main.go` | DR05 red gate; DR04 and DR07 configuration for process integration | T01-wire/T02-wire/T05/T06-http/T08 and P09 green; combined CLI/service evidence after DR07 |
| DR07 CLI and configuration | Luna high / Sonnet high | `cli/*.go`, `config/config.go`, `cmd/perpetual/main.go`, settled examples | DR05 red gate and frozen service/wire contracts | T06-cli/config green against local HTTP fixtures; full CLI/service integration after DR06 |
| DR08 Independent review | Fresh Astra / fresh Fable, fallback Opus | Review findings and validation evidence; request fixes from owners | DR02, DR04, DR06, DR07 and complete DR03/DR05 suites | Requirements, tests, code, MC/DC and real evidence reviewed; material findings resolved |
| DR09 Integration/acceptance | Lead, preferably Astra / Fable or Opus | Plan, state, project direction, knowledge, final testing-guide integration; authorized commit | DR08 | A01–A12 have actual evidence and honest limitations |

Test subgroup boundaries make these gates executable: **T01-core** validates already-decoded domain fields; **T01-wire** covers strict JSON/body decoding; **T02-core** covers canonical domain encoding; **T02-wire** checks equivalent wire representations; **T06-http** tests the API; **T06-cli/config** tests CLI/config through local HTTP fixtures. P10 first checks the production adapter/pool across an owned database restart; its full service demonstration waits for DR06. The test owner records combined CLI/service results after DR06 and DR07, before DR08. No packet depends on a later component to satisfy its own green gate.

DR03 and DR05 share the test owner and may be separate sequential sessions. They must not edit `TESTING.md`, Makefile or shared fixtures concurrently. Production workers may run their tests but do not own acceptance-oracle changes. DR07 delivers configuration parsing first, then its CLI. DR06 and DR07 may develop in parallel against settled wire/config interfaces, but DR06 process-startup tests wait for the DR07 configuration handoff. DR07 CLI tests use a local HTTP fixture and do not wait for DR06, so this dependency has no cycle. Only one worker owns a given skeleton or shared contract at a time.

### 9.2 Dispatch packets

**DR00 — setup.** Read sections 1–3 and the repository guides. Resolve exact supported toolchain, driver and fixture pins through available authoritative sources/tool metadata. Create only the module and specified shells/build/config setup. Leave behavior unimplemented. Report actual build evidence, selected versions, changed paths and anything unavailable. Do not mark a test target passing before it has real cases. Transfer Makefile testing sections to the test owner.

**DR01 — test and core contracts.** Read sections 2–4 and 8. Implement T01-core/T02-core/T03/T04 against explicit types and compiling skeletons. Strict HTTP decoding belongs to DR05/DR06, not this gate. Keep expected values independent. List the first failures by contract ID and command, including what they prove is absent. A test that fails only because a symbol is missing is still setup. Write the initial decision inventory and update `TESTING.md` to describe only implemented commands/cases. Transfer production file ownership when signatures are accepted.

**DR02 — pure production decisions.** Implement the accepted DR01 interfaces and sections 2/4. Work only on owned production files. Do not weaken case expectations, read real time in decisions, mutate shared input, or turn invalid client data into a panic. Request test-owner review of a discovered contract defect. Deliver assigned green commands, actual results, invariant explanation and changes to the design if necessary.

**DR03 — storage and simulator evidence.** Read sections 5, 7 and 8. Establish production adapter skeletons, shared reservation cases, deterministic model and trace helpers, owned fixtures, P01–P11, and executable test targets. Preserve pending transactions across modeled service crashes. Design P06's handshake/proxy before claiming commit-loss coverage. Add real adapter tests first and show behavioral red failures. Simulator checks must call production decisions and use independent oracles. Report limitations separately from completed cases.

**DR04 — persistence.** Read sections 3–5, P01–P11 and DR03 red evidence. Implement the exact gate/lookup/write/commit protocol and pinned production driver. Keep retry count and total budget explicit. Never overwrite on request conflict, infer rollback from connection loss, or retry a non-database effect. Run adapter cases P01–P08/P10/P11 and report driver-specific error classification. P09 and the service-backed P10 demonstration wait for DR06; they are not prerequisites of this packet. Escalate any unresolved safety rule to the design/test owner before continuing dependent work.

**DR05 — boundary tests.** Read sections 2, 3.1, 4.3, 6 and T01-wire/T02-wire/T05/T06/T08. Establish narrow skeleton interfaces and failing tests for cancel-before/after-enqueue, completion delivery under saturation, two process owners, real fatal worker/HTTP behavior, CLI ID persistence and status mapping. Use owned subprocesses and explicit handshakes. Transfer service files to DR06 and CLI/config files to DR07 only after the shared contracts are stable.

**DR06 — service and HTTP.** Implement the bounded owner loop, worker routing, handler/decoder, host lock and service wiring. Prove the loop cannot deadlock because completion and request admission share a saturated queue. Keep invariant failures outside HTTP recovery or terminate explicitly. Accepted work uses the service budget. Close resources in the documented order, observe worker exit, and preserve commit uncertainty on shutdown. Work against DR05 tests; coordinate interface changes through the lead/test owner.

**DR07 — CLI/config.** Implement the settled commands, parsing and validation with injected streams/client. Print a generated ID before the first mutation attempt, send once, preserve the original ID on uncertainty, and keep JSON stdout distinct from diagnostics. Avoid adding retries or changing HTTP semantics. If an assignment reveals concurrency or uncertain-effect design decisions, return that issue to the lead rather than expanding this small packet.

**DR08 — independent reviewer.** Read the design and actual diff/evidence without accepting the implementers' completion claims. Review test expectations as well as code. Inspect missing MC/DC pairs, short-circuit conditions, model/real mismatch, unsafe commit retries, stale completion routing, fatal boundaries and resource leaks. Run focused verification needed to support findings. Record locations, concrete failure modes, unresolved risk and whether each acceptance item has evidence. Request fixes from the responsible owners, then recheck changed risk areas.

**DR09 — lead acceptance.** Reconcile the working tree and all worker reports; validate integrated evidence and final reviewer findings. Update the source plan's status, current plan, testing guide and delivered status only where results support the claim. Save lasting findings in indexed knowledge entries. Keep test documentation with the related implementation in any authorized commit. Preserve state until no one needs to resume the work.

### 9.3 Red, green and review evidence

For every production packet, retain: contract/test IDs; exact command and working directory; revision or diff identity; observed failure reason before implementation; passing result after implementation; and reviewer finding/closure. Setup failures, skips and planned checks are not red/green evidence. A later relevant edit invalidates earlier evidence for the changed behavior and needs a focused rerun.

Tests are not immutable scripture. If a suite encodes the wrong requirement, the implementer reports it, the test/design owner revises the contract and oracle with rationale, and the lead records that decision before accepting the changed behavior. Do not silently reduce coverage to accommodate a defect.

## 10. Acceptance commands and demonstration

The following targets are planned. Introduce them only when they execute their promised checks and propagate failure status.

| Command | Required contents / evidence |
| --- | --- |
| `make test-fast` | Pure/unit/HTTP-in-process cases and committed fuzz seeds; explicitly names excluded external suites |
| `make test-sim` | S01–S10 plus trace replay twice and model invariants; no external services |
| `make test-integration` | Owned PostgreSQL P01–P11 and subprocess/CLI/service checks; fail when prerequisites are absent |
| `make test` | Complete unprivileged suite, including real owned PostgreSQL integration and simulation |
| `make test-race` | Concurrent portions of that suite under `-race`, with actual fixtures |
| `make vet` | Static diagnostics on production and tests |
| `make coverage` | Executed decision/condition mapping, infeasible pairs and gaps; statement report distinctly labeled |
| `make test-fuzz` | Named target, 30 s discovery budget, 2 workers, bounded minimization and total process timeout; reproduce saved failures |

CI provisions the owned database fixture and records toolchain/server/driver versions. Run complete unprivileged tests, race/static checks, committed fuzz seeds and bounded selected discovery targets. Keep traces and sanitized fault logs as failure artifacts. Do not add VM validation or pretend machine evidence is needed for registration-only acceptance.

The final real demonstration is sequential and uses a disposable fixture:

1. Start PostgreSQL and `agent-plane`; observe readiness, exact schema and configured limits.
2. Register `demo-a` with a supplied request ID through the actual CLI. Save its machine ID and registration JSON.
3. Repeat the same request; require the exact same stored identity and parameters, with one database row/count increment.
4. Change one parameter under that request ID; require conflict, then inspect unchanged intent.
5. Register a different request ID with `demo-a`; require name conflict.
6. Lose the real reply after commit using the test-controlled response boundary; retry original ID and observe the same record.
7. Terminate and restart the owned service while PostgreSQL remains running; inspect by request and machine ID.
8. Run the pending-commit fixture in both commit and abort directions. Inspect its real durable rows and compare the classifications with the corresponding simulation cases.
9. Restart only the owned PostgreSQL fixture and verify reconnecting inspection. Record this independently from the service-restart result.
10. Replay the recorded simulation twice; compare normalized results and show the deliberate-defect detector evidence with correct code restored.

| Acceptance | Primary evidence | Owner / review |
| --- | --- | --- |
| A01–A03 | T01–T04/T06, P01–P04, CLI demonstration 2–5 | Test owner + production owners; DR08 |
| A04–A05 | T05/T10, S01–S04/S07/S08, P05/P06/P09/P10 | Test owner + persistence/service owners; DR08 |
| A06 | T07, S05/S06, P01–P04 and persisted counter | Persistence owner; DR08 |
| A07–A08 | T09, S01–S10, same-function call paths and deliberate-defect result | Test owner; DR08 |
| A09 | P01–P11 compared against modeled contract and explicit limitations | Test owner + persistence owner; DR08 |
| A10–A11 | Boundary/fuzz/race checks, T05/T08/T10, subprocess exits and resource teardown | Service/test owners; DR08 |
| A12 | Full evidence table, MC/DC inventory, actual commands, final demonstration | Lead DR09 |

Acceptance requires resolving material reviewer findings and satisfying the actual checks, not merely completing the task table. Report any unexecuted evidence as missing. Simulation does not establish PostgreSQL internals, power-loss durability, OS scheduling behavior, networking correctness beyond exercised local transports, or future guest execution guarantees.

## 11. State, context clearing, and next action

The lead owns `.state/1-durable-requests/plan_state.md`. It records every active task including design, suites, implementations, review and integration. Workers maintain separate append-only `.state/<work-slug>.md` files and report to the lead; they do not concurrently edit the shared plan ledger.

Each checkpoint records task ID/status, actual model ID/effort/run, owned files, dependencies, working tree/worktree, actual red/green/review evidence, pending processes, unresolved risks, authorized phase and the exact next action. Append a fresh complete snapshot before clearing context. A planned model assignment is never a claimed live worker.

If switching from Codex to Claude, read the same plan and state, inspect the actual files and evidence, reconcile live processes/agents, then choose available assignments from section 9. Do not repeat completed checks without a changed revision or unresolved risk. If this local state is absent in another checkout, recreate it from the plan and durable evidence and explicitly mark execution history unknown.

Use project direction and the testing guide to establish the delivered state before resuming. The plan defines contracts and acceptance requirements; it does not establish a passing runtime or CI result.
