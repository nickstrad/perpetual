# Durable-request decision inventory

This maps handwritten production decisions to concrete tests. Go's coverage
profile measures statements, not MC/DC; no repository-wide MC/DC percentage is
claimed. Test helpers, generated code and third-party code are outside production
coverage scope. Tests named below exist and have been executed; final aggregate
command results are recorded in [TESTING.md](../../TESTING.md).

For compound guards, start from the stated valid baseline and alter one condition
at a time. Conditions after a short-circuiting false/true operand are not counted
as evaluated merely because their input has a value.

| Production decisions | Cases, evaluated conditions and independent pairs |
| --- | --- |
| `registration.ValidateRequestID` / `ValidateMachineID`: length, four separators, lowercase hex | `T01Identifiers`: canonical, all-zero and alphabetic UUIDs versus short/long/uppercase; corrupt each of the 36 bytes independently. Every separator and digit position is reached. |
| `ValidateParameters`: name/image byte limits, endpoint/inner character classes, numeric ranges | `T01ParameterBoundaries`: minima/maxima versus adjacent outside bounds; empty, Unicode, uppercase name, internal/endpoint hyphens, underscore/path rejection; each numeric field varied with all others valid. |
| `NewRequest`: request ID, candidate ID, parameters | `T01NewRequestValidation`: valid baseline versus each independently invalid field. `Fingerprint` validation uses valid/invalid canonical cases. |
| Canonical encoding and fingerprint | `T02CanonicalLiteral`: literal bytes/digest; every parameter varied independently. `T02WireCanonicalEquivalence` checks different wire ordering. Native fuzz seeds/discovery test bounded deterministic encoding. |
| `DecideAdmission`: HasRequest | `T03AdmissionIndependence` C01/C02; downstream absent-request decisions are not evaluated in C02. |
| Existing parameter equality / fingerprint equality | C02/C03 and C02/C04 respectively; the other equality remains true. |
| NameTaken / Used==Limit | C01/C05 and C01/C06 respectively, with HasRequest false. C07 proves matching retry precedes both guards. |
| Admission limit/count/lookup invariants | `T03AdmissionInvariants`: zero limit, count above limit and wrong lookup identity versus C01/C02. |
| `Step`: event variant, submission phase, pending phase, effect correlation | `T04Transitions` and `T04TransitionInvariants`: valid Submit/Completed, repeated Submit, completion without pending state, mismatched effect, nil event. |
| Effect identity conjunction / epoch filtering | `T04EffectIdentityConditions`: valid epoch+sequence versus each invalid component. `T04FreshStateAndEpochRouting`: current/old epoch. Same-epoch zero sequence is routed to the owner invariant, not discarded as stale. |
| Outcome range, success union, uncertainty and success relationships | `T04Transitions`: all eight kinds. `T04TransitionInvariants`: below/above enum, non-success record, wrong request/parameters/fingerprint/machine/timestamp/timezone independently. `FuzzPureCore` constructs reachable states and checks deterministic immutable decisions. |
| `NewService`: store, epoch, positive job/queue/worker counts, workers<=jobs, positive budget | `T05ConstructorConditionPairs`: valid baseline and each invalid input field (the MaxJobs/Workers predicates are coupled; see exclusion below); `T05InvalidBounds` adds invalid combinations. |
| `Register`: canceled, stopping, job capacity, enqueue, detached waiter | `T05CancelBeforeEnqueue`, `T05DetachedCallerDoesNotCancelAcceptedWork`, `T05ReadIndependentAndShutdownRejects`, `T05CompletionUnderSaturation`, `T05StopAdmissionDrainsAcceptedWork`. No accepted mutation inherits caller cancellation. |
| Worker deadline and shutdown draining | `T05BudgetIncludesQueuedTime`: a controlled worker holds the slot past both jobs' original budgets; queued job never invokes storage. `T05ShutdownDeadlineCancelsAndJoins`: deadline returns an error, cancellation completes the worker, subsequent join succeeds. |
| Coordinator completion effects / stale filtering / fatal failure | T04 core checks and S08 cover shared transitions/filter. `T08WorkerInvariantTerminates` invokes an actual service worker returning an invalid success; child exits nonzero. Completion under saturation has an independent completion channel and bounded progress check. |
| Read forwarding and readiness | `T05ReadErrorPreserved`, `T05ReadIndependentAndShutdownRejects`, `T05StopAdmissionDrainsAcceptedWork`; HTTP read found/absent/error cases remain distinct. |
| Listener token admission, accept failure, close, once-only release | `cmd/agent-plane.TestT05ListenerCapacityAndClose`: one occupied connection blocks underlying second Accept, repeated close releases one token, underlying failure returns its slot, listener close unblocks Accept. |
| Strict decoder I/O/bounds/object/field/closing/trailing/presence/type/domain guards | `T01WireStrictness`, `T01WireErrorExits`: reader error, 4097-byte body, nonobject/null/empty, malformed/truncated object, every missing/null field, duplicate literal/escaped keys, unknown fields, fractions/negative/overflow, field type errors and domain invalidity. |
| HTTP route/prefix/empty suffix/slash/method/ID guards | `T06HTTPGuardDecisions`, `T06HTTPInvalidHasNoEffects`: all three routes, unknown route, empty/nested suffix, invalid IDs and disallowed methods. Rejections assert no mutation calls. |
| Content type / decoding / candidate generation / NewRequest | `T06HTTPGuardDecisions`: valid JSON versus parse error/wrong media type, injected generation error and malformed candidate. Wire tests cover decoding exits. |
| HTTP outcome switch and read/readiness outcomes | `T06HTTPOutcomes` exercises all eight classifications and exact public codes/statuses; `T06HTTPReadClassifications` covers found/absent/error and health success/failure; `T06HTTPRealRoundTrip` exercises actual local transport. |
| Fatal HTTP panic recovery | `T08HTTPInvariantTerminates`: actual local HTTP child invokes typed invariant and exits through fatal boundary instead of net/http recovery. |
| CLI command selection, flags, narrowing, validation, generated/supplied ID | `T06CLIRejectsInvalidBeforeSend`, `T06CLIGeneratedIdentityBeforeSingleSend`, `T06CLIErrorDecisionPairs`: valid register/inspect versus invalid shape/flags/domain/ID/CPU overflow, generation error, ID output failure and nil context/streams. |
| CLI endpoint compound guard | `T06CLIErrorDecisionPairs`: valid default/http/https versus parse error, wrong scheme, missing host, credentials, query, fragment independently. |
| CLI send/read/status/response validation | `T06CLIStatusMapping`, `T06CLIInspectRoutes`, `T06CLIReadFailureAndResponseBound`: status branches, read timeout, malformed/empty/oversize body. `T06CLIRejectsMismatchedSuccess`: independently mismatched request/machine/parameters and missing/null provisioned. `T06CLIErrorDecisionPairs`: invalid IDs/state/provisioned/parameters/timestamp and stdout failure. |
| Mutation certainty after invalid response | `T06CLIMalformedMutationReplyPreservesUncertainty`: malformed/unrecognized 503 and oversize bodies preserve original request ID and exit 6. `T06CLIStatusMapping`: recognized invalid/conflict/capacity/busy/unavailable responses establish the documented known rejection. |
| Config lookup/default, URL, address, absolute path | `T06ConfigDefaultsAndBounds`, `T06ConfigDecisionPairs`: nil lookup, required DB URL; valid postgres/postgresql versus parse/scheme/host failures; host/port parse, empty/zero/overflow; relative runtime path. |
| Config count parsing/range/cross-field and timeout order | Same cases independently vary each count with valid baseline; zero/negative/overflow/above-cap, positive override, workers>=pool; duration parse/zero/negative, lock>=statement and statement>=operation independently. |
| Hostlock path/directory/open/flock/close | `T08LockErrorPaths`: empty path, parent-file mkdir failure and directory-as-file open failure. `T08ProcessLock`: actual second-process lock rejection, preserved inode, repeated close, reacquisition after release. Process integration repeats actual service ownership. |
| Store constructor pool/limit/timeouts | `T07StoreConstructorPairs`: valid baseline versus nil pool, zero/overflow limit, lock<=0, statement<=lock, operation<=statement and >24h individually. |
| Reservation gate/read/admission/write/commit | P01 matching concurrent retries; P02 changed intent; P03 name race; P04 final-slot race/retry; P05 pending transaction commit/abort with absent reads and blocked same/conflicting retries; P07 cancellation/independent read; P11 candidate collision returns unavailable with original record/counter retained. |
| Before-commit classification: SQLSTATE and driver error | `T07BeforeCommitClassification`: 40001, 40P01, 55P03, other server error and transport error. P07 exercises actual lock timeout/cancellation. |
| Commit classification: confirmed abort versus ambiguity | `T07CommitErrorCertainty`: confirmed rollback, 40001/40P01, 40003/08007, generic server/transport errors. P06 withholds the actual backend COMMIT CommandComplete via pgx DialFunc: unknown result, independent durable row, matching retry. Synthetic classifier cases do not substitute for P06. |
| Retry/no-retry and exhaustion | `P08BoundedAbortRetriesPreserveIdentity`: server trigger raises 40001/40P01, sequence counts actual attempts across rollback (eventual third-attempt success, three-attempt exhaustion, one-attempt nonretryable rejection); trigger verifies original request/candidate on every attempt. `P08RealDeadlock` separately creates actual two-session lock contention and requires exactly one 40P01 plus one success regardless of delivery order. |
| Migration fresh/repeat/version/checksum/limit/count | `P11MigrationGuards`, `P11CounterCorruptionIsInvariant`; `P11MigrationWaitsForPriorCommitBeforeCounting` observes the real lock wait before committing older work and checks the later fresh count. Constraint cases reject invalid direct changes in rollback-owned transactions. |
| Pool/service/process restart and CLI/API composition | P10 holds an old connection through owned DB restart, observes failure, reconnects and inspects unchanged data. `TestProcessCLIRegistrationRestartAndReplyLoss` builds actual binaries, verifies register/retry/conflicts, cuts a real successful HTTP reply, kills/restarts service, tests two process owners and independently restarts DB while service stays alive. |

## Infeasible combinations and remaining gaps

These distinctions prevent an unjustified whole-program coverage claim.

- Canonical/CLI parameter structs contain only strings and integers; their
  `json.Marshal` error branches cannot be reached with these fixed field types.
  The HTTP encoding helper has broader static type `any`, but supported callers
  provide only those fixed response/error structs.
- The JSON token decoder cannot return a non-string object key after accepting
  `{`; similarly a successful closing-token read after `More()==false` must
  return `}`. Malformed token/closing **errors** are exercised. Treat the impossible
  alternative token-type branches as defensive, not covered.
- Go 1.26.8 `crypto/rand.Read` returns a full buffer and nil error or terminates the
  process on entropy failure (verified in the pinned standard-library source).
  Injected ID-generator errors cover the callers' expected-error branches; the
  wrapper's `rand.Read` nonnil-error exit is infeasible for that implementation.
- Validated SQL values plus UUID/NOT NULL/CHECK constraints exclude negative
  counts, out-of-range resource fields, malformed stored UUIDs and wrong-length
  digests in supported writes. Defensive scan/gate assertions remain enabled.
  Cross-table count corruption and machine-candidate collision are independently
  exercised; the former is fatal, the latter an expected availability failure.
- Current-epoch owner-map corruption, uint64 effect-sequence exhaustion, and
  impossible effect shapes cannot arise from this slice's bounded owned worker
  protocol. T04 checks core mismatches; a real worker invariant subprocess checks
  fatal behavior. Exhausting 2^64 owned jobs is not attempted.
- `NewService` has a coupled constructor guard: changing `MaxJobs<=0` to true
  while keeping `Workers>0` necessarily also makes `Workers>MaxJobs` true.
  No unique-cause independence pair exists for that predicate with all other
  Boolean values fixed. The test varies one input field and proves rejection;
  it does not claim an independent-condition pair for this infeasible combination.
- `parseCount` parser failure precedes numeric guards. `strconv.ParseUint` syntax
  errors return zero and range errors return MaxUint64, coupling the otherwise
  unevaluated zero/above-maximum values to the parse error. Tests cover the error
  exit and separately vary zero and above-maximum **after successful parsing**;
  they do not assert unique-cause MC/DC for the coupled parse-error predicate.
  Similarly a failed duration parse returns zero; later positivity is masked.
- **Residual error-path gaps:** the suite does not force a separate database
  failure at every migration/reservation SQL statement, rollback logging failure,
  entropy process termination, every process-startup allocation/socket failure,
  or every file-close/log-write failure. Generic error classification is tested,
  but that does not establish condition independence at every individual call
  site. The exact gaps remain visible rather than being counted as covered.
- **Residual orchestration pairs:** queue-send rejection distinct from full-job
  rejection is exercised under saturation, but no coverage instrument proves the
  scheduler took each select arm. A cancellation concurrent with acquisition of
  the admission mutex is guarded in production; the existing canceled-input and
  accepted-disconnect cases do not independently force every instruction-level
  interleaving. Go race checks complement these contracts.

## Simulation evidence

The fixed scheduler calls production `NewState`, `Step`, `DecideAdmission` and
`IsCurrentEpoch`. It records concrete effect IDs, initial committed/pending state,
actual choices and observations, actual revision/dirty metadata (explicitly
unknown for source exports), and the pinned toolchain. Each trace is replayed twice
without regenerating choices. S10 rejects incompatible versions, nonexistent or
repeated/expired dispatch, invalid completions/reply loss, and event/byte overflow.
Database restart aborts gate owners **and waiters**; service restart preserves
already-dispatched transactions. A test-only panic boundary retains the failed
action and preceding trace before failing the scenario.

Temporarily returning a retry candidate instead of the persisted Existing winner
in production admission made S01 fail `success published before matching
durability`. The exact source was restored and the entire simulator passed again.
This actual mutation evidence supplements the permanent independent-oracle test.
