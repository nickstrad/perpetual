# Testing perpetual

Build two complementary forms of tests: basic behavior tests with MC/DC coverage and targeted fuzzing, and deterministic simulation of coordination and recovery.
Validate external effects against real components as part of that work.
Use the [MVP breakdown](plans/mvp/breakdown/README.md) to grow tests with each production slice.

Slice 1 now has executable core, service, HTTP, CLI/configuration, host-lock,
PostgreSQL, subprocess and fixed-simulation suites. The core and boundary tests
first failed against compiling skeletons; the current focused suites pass.
S01–S10 replay actual traces twice, and a temporary production identity mutation
was detected before restoring the exact source. Real PostgreSQL tests exercise
pending commits, actual COMMIT-response loss, contention, bounded abort retries,
constraints and restart. An actual CLI/service process demonstration covers reply
loss, service crash/restart and database restart independently. Final aggregate
race/static/fuzz/coverage checks are recorded by the lead during acceptance.

The [decision inventory](docs/testing/decisions.md) lists concrete cases,
short-circuit limitations, infeasible alternatives and residual error-path gaps.
Go statement coverage does not establish repository-wide MC/DC; no such percentage
is claimed. No VM execution is covered by this registration-only slice.
Record delivered slices in [project direction](docs/knowledge/project-direction.md).
The [simulation research](docs/TEST_RESEARCH.md) records influences, alternatives, and detailed modeling assumptions.

## Build business logic from pure functions

Follow [Go TigerStyle](docs/TIGERSTYLE.md) when making code testable:

- Compose business rules from small, named pure functions. A pure function returns the same result for the same inputs and has no externally visible side effects. Keep inputs unchanged, including data shared through slices, maps, and pointers.
- Pass state, observations, time, identifiers, and random choices explicitly. Return the next state and requested effects; perform database, filesystem, network, and process operations in adapters outside the decision code.
- Keep orchestration readable. Separate validation, decisions, and effect execution without fragmenting a clear operation into trivial helpers or building a generic framework.
- Give mutable state and resources clear owners. Serialize decisions where required; let external work report completion through explicit events. A requested write is not a committed write.
- Bound work, queues, buffers, output, and retries. Make units, deadlines, overflow checks, and uncertain outcomes explicit.
- Keep meaningful invariant checks enabled in production. Check relationships such as exclusive reservations and output cursors never exceeding durable output, at the boundaries where they must hold.

An invariant failure indicates a programming defect and must stop the affected service.
Invalid input, unavailable guests, and I/O failures are expected errors with explicit recovery paths.
Test both categories; do not turn operating errors into assertion failures.
Verify fatal invariant handling through subprocess tests, including HTTP handlers whose panics Go would otherwise recover.
Avoid assertion quotas and arbitrary function-size limits; each check and helper must explain a real rule.

## 1. Basic testing: behavior and MC/DC coverage

Use Modified Condition/Decision Coverage (MC/DC) as the coverage standard for handwritten production Go decisions.
Cover every feasible decision and condition in that scope; track any remaining gaps explicitly.
Cover validation, admission, identity checks, state transitions, recovery rules, and decisions inside effect adapters.
Start with ordinary table-driven Go tests and explicit expected outcomes.
Test pure functions directly, then test their composition through the production entrypoints that use them.

For each decision:

1. Identify its Boolean conditions and exercise both decision outcomes.
2. Exercise each condition as both true and false. Include every reachable function entry and exit, including error returns.
3. Show a pair of cases where changing one condition changes the decision while the other condition values stay fixed. Label the pair in the test table or nearby comments.
4. Assert the required result, next state, and requested effects. Check that rejected requests produce no prohibited effects.

For example, after resolving matching retries, a new command might require:

```text
allow = ready && slot_free && !draining
```

| Case | ready | slot_free | draining | allow | Independence pair |
| --- | --- | --- | --- | --- | --- |
| A | true | true | false | true | Baseline |
| B | false | true | false | false | A/B: ready |
| C | true | false | false | false | A/C: slot_free |
| D | true | true | true | false | A/D: draining |

Also test that a matching retry returns the recorded outcome while the slot is occupied or admission is draining.
The independence pairs establish coverage of this decision; they do not establish the correctness of the surrounding workflow.

Keep a reviewable mapping from production decisions and conditions to executed test cases.
Account for short-circuit evaluation; an input value alone does not show that its condition was evaluated.
When predicates call other predicates, cover the decisions inside those functions too.
For coupled or unreachable conditions, simplify the expression when that improves the design, or document the exact infeasible pair and why it cannot occur.
Report such exclusions and uncovered decisions; do not count them as covered or remove defensive checks to improve a percentage.
Exclude generated and third-party code from the project coverage scope explicitly.

Begin with reviewed case mappings; add automated MC/DC measurement only after verifying what the tool measures.
Go's ordinary coverage output measures statements and cannot establish MC/DC.
Report coverage only for the code and cases actually checked, without claiming a repository-wide result from a few examples.

Coverage is one requirement within basic testing. Also:

- Test values below, at, and above limits; empty and malformed input; overflow; conflicting identifiers; and stale observations.
- Assert behavior and invariants independently of implementation calculations. Do not copy the production algorithm into the expected-result code.
- Add a regression case for every fixed defect, including defects found through fuzzing.
- Run the race detector on exercised concurrent code and static checks on production and tests. Simulation and MC/DC do not replace these checks.

### Fuzz input boundaries and pure logic

Use Go's built-in fuzzing through `FuzzXxx(*testing.F)` tests in `*_test.go`; no separate fuzzing framework is needed.
Add a target when varied inputs can expose a named risk and the expected property is clear.
Fuzzing supplements the explicit MC/DC pairs; generated coverage does not establish condition independence.

| Target, when implemented | Properties to check |
| --- | --- |
| Request, identifier, cursor, and journal decoding | Malformed or truncated input returns an error without panicking; accepted values satisfy format and size constraints |
| Encoding and decoding | Valid values survive a round trip under documented normalization rules |
| Size, offset, and deadline calculations | Boundary values cannot wrap, bypass limits, or request prohibited effects |
| Pure admission and transition functions | Inputs remain unchanged; identical inputs give identical results; valid transitions preserve invariants and rejected requests produce no prohibited effects |

Generate reachable internal states through valid operations; test malformed external input separately.
Keep each target fast and deterministic, with fresh state per invocation and bounded input sizes and work.
Exercise invalid input rather than filtering away the cases the parser must reject.
Keep real databases, subprocesses, and network services in the real-effect checks.

Seed targets with `f.Add` examples covering normal use, empty input, malformed data, and limits.
Retain minimized failures in `testdata/fuzz/<FuzzName>/` with the fix; ordinary `go test` reruns the seed corpus.
Record the failing input and reproduction command, not just a random seed.
Use the [native Go fuzzing guide](https://go.dev/doc/security/fuzz/) for target and corpus mechanics.

Once a target exists, run one named target in one package with a fixed budget.
The following remain illustrative generic commands; `make test-fuzz` now executes
the four concrete slice1 targets described below:

```sh
go test ./path/to/package -run='^$' -fuzz='^FuzzDecodeRequest$' -fuzztime=30s -parallel=2
go test ./path/to/package -run='^FuzzDecodeRequest/<corpus-hash>$'
```

Bound minimization and the overall job timeout too; longer local or scheduled runs can explore beyond the CI budget.
If later simulation work benefits from fuzzed event sequences, feed bounded inputs into the existing deterministic scheduler and preserve its replay trace.
Keep that extension within the planned progression to generated schedules in slice 4.

## 2. Simulation testing: coordination and recovery

Build a small event scheduler in ordinary Go alongside the first production coordinator.
Production and simulation must call identical decision functions, including guest decisions when the test makes claims about guest behavior.
Keep the harness local to its first package until another implemented feature needs shared helpers.

Represent requests and completions as typed events, and external work as effects.
Use simulated adapters to control effect outcomes without a database, guest, or real clock.
Start with fixed event sequences; introduce logical deadlines in slice 2 and bounded seeded schedules in slice 4.
Keep event ordering explicit, including simultaneous events and order-sensitive map traversal.
Do not depend on goroutine scheduling for reproducibility.

For every scenario:

- State the initial conditions, fault location, forbidden outcomes, and expected recovery.
- Check safety invariants after every transition. Examples include one reservation per identity, no duplicate execution, and no publication before its durability prerequisites.
- Separate volatile state from modeled durable records. Crash by discarding memory and restart through production recovery; already dispatched effects may still complete after the crash.
- Preserve uncertainty when an acknowledgment disappears. Inspect stable request identifiers before retrying ambiguous mutations. A missing record does not prove a pending transaction aborted.
- Require progress within explicit event and logical-time bounds under stated health and delivery assumptions. Allow unresolved work to retain its reservation when safe recovery lacks evidence.
- Save readable events, effects, faults, outcomes, and the failed assertion. Include initial state, configuration, revision, toolchain, harness version, and generator version and seed when used.
- Replay the actual recorded choices without regenerating them. Reject incompatible traces clearly; retain important failures as short regression scenarios.

Deliver eligible healthy events fairly during progress checks; do not repeatedly restart a component to conceal stalled recovery.
Model storage transactions as atomic commit-or-no-commit outcomes, with response delivery controlled separately.
Treat control-plane restart, database restart, and connection loss as distinct faults.
The simulator covers coordination contracts, not PostgreSQL internals, filesystem power-loss behavior, kernel scheduling, packet routing, encryption, or Firecracker internals.
Validate those external boundaries through the real checks below.

### Implement the scope already planned

Follow the numbered slices; expand only the current slice after discussing its boundaries and acceptance evidence.
Slice 1 is being implemented; slices 2–7 remain planned. Keep earlier scenarios running as later behavior is added.

| Slice | Add to simulation | Required evidence |
| --- | --- | --- |
| [1. Durable requests](plans/mvp/breakdown/1-durable-requests.md) | Fixed events, reservations, lost replies, pending commits, restart, and trace replay | One stable identity across retry and restart; conflicting reuse rejected; unknown commits do not permit conflicting work; real PostgreSQL contracts agree |
| [2. Live machines](plans/mvp/breakdown/2-live-machines.md) | Logical deadlines, lifecycle effects, delayed health, launch uncertainty, and partial cleanup | Distinct allocations; no duplicate launch without resolved ownership; real boot, SSH, and two-machine independence |
| [3. Detached commands](plans/mvp/breakdown/3-detached-commands.md) | Guest acceptance, lost acknowledgment, launch gaps, and output collection | No blind replay; accepted execution survives caller detachment and control-plane outage; cursors never exceed durable output; real process and journal checks |
| [4. Working with commands](plans/mvp/breakdown/4-command-actions.md) | File/input/cancel races, deadlines, and bounded seeded schedules | Correct cancel-versus-complete behavior; no automatic resend of uncertain input; bounded output; real file, pipe, and process effects |
| [5. Recovery](plans/mvp/breakdown/5-recovery.md) | Independent crashes, stale observations, and bounded progress | Healthy machine B progresses while A remains unreachable; separate real service, guest-agent, and database restarts |
| [6. Streaming](plans/mvp/breakdown/6-streaming.md) | Polling/streaming changes, duplicate delivery, and reconnect schedules | Equivalent retained outcomes without restarting execution; real HTTP/HTTPS/WebSocket behavior and fallback |
| [7. Snapshots](plans/mvp/breakdown/7-snapshots.md) | Drain races, capture/publication failures, termination, and restored identities | Incomplete artifacts cannot restore; old boot observations are rejected; real snapshot restoration succeeds |

For slice 1, commit registration, lose the reply, retry, restart, and inspect.
Also lose the connection while a commit remains pending and submit conflicting requests.
Compare real PostgreSQL reservation outcomes using concurrent connections.
Replay the same scenario twice and compare normalized traces and outcomes.
Temporarily break a tested invariant to confirm the scenario detects that defect, then restore correct behavior.
Harness scaffolding alone does not satisfy this acceptance gate.

## Validate real effects

Use shared contract cases for simulated and real adapters where practical.
Check returned classifications, persisted state, effect ordering, and resource ownership.
Document model assumptions and intentional differences beside each adapter.
Add the following checks when their production slice introduces the corresponding boundary:

| Boundary | Checks to implement |
| --- | --- |
| PostgreSQL | Production migrations and driver; uniqueness, concurrent reservations and capacity admission, rollback, contention, cancellation, bounded transaction-abort retries, lost commit acknowledgment, and reconnecting |
| Journals and output files | Actual writes, synchronization, publication, retention, and recovery after process termination; output saved before cursor advancement |
| Processes and lifecycle | Actual helper children, detachment, stdin, deadlines, cancellation, and cleanup; service-manager shutdown preserves accepted guest work |
| Transports | Local HTTP/HTTPS/WebSocket servers; trust, failed upgrades, disconnects, replay, fallback, and equivalent polling/streaming outcomes |
| Machines and snapshots | Actual boot, SSH, two-machine independence, snapshot publication, restoration, and fresh boot identity |

Keep guest execution outside retried database transactions.
Test crashes separately from returned errors: terminate the relevant process and recover using its existing durable files or records.
Target interruptions with explicit handshakes, including acceptance before launch evidence, output before cursor commit, and artifacts before publication.
Fail recovery itself where a durability or ownership guarantee depends on it.
Process termination does not establish power-loss durability.

Each test must own its temporary files, listeners, children, database fixture, and cleanup scope.
Use an explicitly configured disposable PostgreSQL fixture with a pinned server version and recorded isolation and durability settings.
Database restart checks require an exclusively owned server instance.
Never borrow an operator's database, runtime directory, or guests implicitly.
Use readiness signals instead of startup sleeps; bound waits and report pending work and surviving resources on timeout.
Verify cleanup after expected failures and check ownership before signaling or deleting resources.
Missing prerequisites must fail an explicitly requested check; skipped VM checks are not passing machine evidence.

## Deliver and maintain the checks

The Makefile now exposes these executable commands. `make help` lists them and
the separate development-database helpers. Missing Docker or Go prerequisites
fail explicitly; there are no successful placeholder checks.

| Command | Implemented behavior |
| --- | --- |
| `make build` | Build both actual binaries into `bin/` |
| `make test-fast` | Unit/local HTTP/subprocess ownership/fatal cases and committed fuzz seeds; excludes PostgreSQL-tagged tests and fixed simulations |
| `make test-sim` | S01–S10, lifecycle regressions, recorded replay, trace limits/metadata and invariant-failure trace capture; no database |
| `make test-integration` | Owned disposable Docker PostgreSQL 18.6, P01–P11 and actual CLI/service process demonstration |
| `make test` | Fast, simulation and real integration suites |
| `make test-race` | Complete tagged suite under `-race` with its real owned PostgreSQL fixture |
| `make vet` | Static diagnostics including integration test code |
| `make coverage` | Execute tagged suite, output `artifacts/statements.out`, label statement report separately, print reviewed condition mapping and gaps |
| `make test-fuzz` | `FuzzIdentifiers`, `FuzzCanonicalization`, `FuzzDecodeRegistration`, `FuzzPureCore`: each 30s discovery, two workers, 5s minimization, 60s Go timeout and 120s outer process limit (including cold compilation) |

`make validate` remains future machine work; it is not a target for this slice.
Native fuzz seed cases run in ordinary tests. Reproduce a saved native fuzz failure
with `go test <package> -run '^<FuzzName>/<corpus-hash>$'`; retained minimized inputs
belong in that package's `testdata/fuzz/` directory.

`scripts/test-postgres.sh` creates an exclusively owned container and per-case
databases. `scripts/versions.env` pins the official PostgreSQL 18.6 image digest.
It prints server version/isolation/fsync/synchronous_commit/full_page_writes,
waits on TCP readiness, binds a fixed loopback host port across restart and cleans
up the container. P06 wraps the real pgx connection to suppress an observed
successful COMMIT frame; it does not fabricate a transaction result. P08 separates
actual server deadlock from controlled server-raised SQLSTATE retry accounting.
Fixture packages run with `-p 1` because their restart cases share a server;
concurrent operations **within** packages and race detection remain active.
Failures retain sanitized server logs under `/tmp/perpetual-pg-logs.*`.
Docker setup/readiness/inspection/cleanup commands have separate timeouts; the
fixture test command has a 360s outer limit and 5s forced-termination grace.
Teardown failure reports the surviving container name and retained log path.

The process demonstration builds the actual binaries in an owned temporary
folder, uses readiness/commit/lock-wait handshakes, enforces bounded child waits,
checks graceful exit, and never restarts an operator-provided database.
When the enclosing suite uses `-race`, both child binaries are also built with
`-race`. Ordinary statement profiles do not merge those subprocesses or measure
uninstrumented cross-package calls; low command-main/invariant percentages are
not evidence that their process contracts were omitted. The
persistent development helpers (`dev-db-up`, `dev-db-down`, `dev-db-env`,
`dev-db-reset`) use a separate fixture and are not substitutes for integration
test ownership.

Observed on 2026-09-24: `make test test-race vet` completed successfully, including
P01–P11 and the actual process demonstration; `make coverage` also completed.
The process race case separately passed with both child binaries instrumented.
The corrected real-deadlock oracle passed 20 consecutive executions; a later
P07 extension independently observed a real gate wait before canceling the active
reservation and passed its focused rerun. Initial red/setup/fault logs remain in
local state for review; setup failures are not counted as behavioral evidence.
`make test-fuzz` completed successfully for all four named targets, each with
30 seconds of discovery and the documented worker/minimization bounds. The first
attempt timed out during cold instrumentation compilation; the corrected outer
compile/run budget passed without weakening the discovery budget.

Continuous integration must provision fixtures and run complete unprivileged tests, race checks, and static diagnostics.
Run committed fuzz corpus cases in the normal suite. Once targets exist, add bounded fuzz discovery runs for selected targets and retain failure inputs as artifacts.
Retain useful failure logs and propagate failure exit statuses; never implement successful placeholders for missing checks.
Keep privileged validation evidence separate until a suitable runner exists.

Before accepting a behavior change, check its observable contract, MC/DC evidence, relevant simulation scenarios, and real effects.
Record exact commands and observed results, remaining coverage gaps, model limitations, and any missing machine evidence.
Keep reproductions and traces free of secrets and unrelated operator data.
Update this guide with the testing changes it describes; mark commands implemented only after running them.
Documentation-only changes need link and consistency checks, not unrelated runtime tests.
