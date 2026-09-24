# Early deterministic simulation

## Decision and status

Build our own small deterministic harness alongside the first production coordination code.
Deterministic means identical controlled inputs produce identical decisions and event traces.
Simulation replaces selected external effects with controlled representations of their behavior.
The harness must execute the same decision code used by production.
This approach is feasible for perpetual's admission, lifecycle, and recovery coordination.
It avoids simulating whole operating systems or Firecracker internals.
Use ordinary Go and its standard library for the baseline harness.
Keep the custom event scheduler local to its first owning package.
Building these bounded pieces serves the project's learning purpose.
No patched runtime, hosted service, or additional simulator dependency is required.

This document records research and proposed implementation boundaries on 2026-09-18.
No coordinator, simulator, test target, or CI implementation exists yet.
PostgreSQL is the selected state backend; database fixtures remain unimplemented.
The custom simulator remains database-free.
[TESTING.md](../TESTING.md) defines MC/DC coverage, simulation requirements, real-effect validation, and maintenance.
The [MVP breakdown](plans/README.md) pairs platform slices with simulator growth.
The [project direction](knowledge/project-direction.md) entry records which slices have been delivered.
The assigned test owner maintains the harness, scenarios, fixtures, invariant checks, and testing documentation.
Production owners implement shared decisions and real adapters in collaboration with the test owner.
The lead independently validates results and updates the plan and tracker.

## Lessons from primary sources

TigerBeetle replaces external interactions while executing production code under controlled time.
Its protocol-aware tests inspect internal safety and recovery behavior.
This supports checking intermediate states, beyond final API responses.
We do not need its consensus protocol or physical storage determinism.
See [Protocol-Aware Deterministic Simulation Testing](https://tigerbeetle.com/blog/2026-08-20-protocol-aware-dst/).

Dropbox separated control decisions from concurrent external work.
Its focused planner tests exercised real planning without performing actual filesystem operations.
Broader simulation still needed separate native filesystem and network tests.
These examples support starting with a narrow production boundary.
See [Testing sync at Dropbox](https://dropbox.tech/infrastructure/-testing-our-new-sync-engine).

TigerBeetle's liveness tests preserve some failures while requiring healthy components' progress.
Apply that distinction to independent machines and recovery paths.
See [Simulation Testing For Liveness](https://tigerbeetle.com/blog/2023-07-06-simulation-testing-for-liveness/).

SQLite combines injected failures, crash checks, regression cases, and coverage analysis.
Its MC/DC discussion motivates requiring evidence that each condition independently affects a decision, alongside behavior and failure tests.
Those techniques remain necessary around our real storage boundaries.
Simulation supplements them. See [SQLite testing](https://www.sqlite.org/testing.html).
SQLite supplies testing inspiration only; it is not the selected backend.

The following design is our recommendation, inferred from these examples.
Their results do not establish perpetual's correctness or implementation cost.

## Tools considered

This survey informed our choice; no tools were installed or benchmarked.
The [DST resource collection](https://github.com/ivanyu/awesome-deterministic-simulation-testing) indexes useful approaches and case studies.
It serves as a discovery aid, not evidence of individual tools' compatibility.

WebAssembly is a portable execution format with its own runtime environment.
Polar Signals combined WebAssembly, Go runtime modifications, and the `faketime` build tag.
Their article reports occasional failures to reproduce execution with identical seeds.
Its runtime maintenance and execution constraints exceed our selected scope.
We choose an explicit application scheduler instead.
See [(Mostly) Deterministic Simulation Testing in Go](https://www.polarsignals.com/blog/posts/2024/05/28/mostly-dst-in-go).

Fuzzing generates varied inputs to discover failures.

| Tool | Useful capability | Decision |
| --- | --- | --- |
| [Gosim](https://github.com/jellevandenhooff/gosim) | Experimental simulation of Go programs, networking, filesystems, and machines | Broader alternative; Go 1.26 compatibility remains unverified |
| [Rapid](https://github.com/flyingmutant/rapid) | Structured input generation and automatic failing-case minimization | Optional later helper; not our event scheduler |
| [Go fuzzing](https://go.dev/doc/security/fuzz/) | Native input mutation, coverage guidance, and failure minimization | Optional later generation of byte-encoded event schedules |
| [Go synctest](https://pkg.go.dev/testing/synctest@go1.26.0) | Fake time and synchronization inside isolated concurrent tests | Useful for adapter tests; no seeded scheduler guarantee |

Gosim documents limited simulated APIs and a changing experimental interface.
A minimum Go version in `go.mod` would not establish our toolchain compatibility.
Assess an exact release and required APIs before any future adoption.

Shrinking means reducing a failing input while preserving its failure.
Rapid could eventually supply generators and shrinking without replacing our scheduler.
Native fuzzing could mutate schedule bytes consumed by our deterministic event loop.
Each fuzz invocation must create fresh scenario state and enforce execution bounds.
Neither input generator controls real network, process, or goroutine scheduling.
The custom simulator remains responsible for legal events, effects, and reproducible execution.

## Small Go architecture

A coordinator owns decisions about state changes and requested external work.
An event reports a request, observation, completion, or expired deadline.
An effect requests external work, such as reserving an action.
An adapter executes effects through PostgreSQL, transport, processes, or other real components.

Use an ordinary Go function with explicit state and typed events.
The following signature illustrates the boundary; it is not implemented code.

```go
func Step(state State, event Event) (State, []Effect)
```

`State` contains current knowledge, pending work, and necessary decision data.
Changing this value does not itself commit durable storage.
Persistence effects receive explicit completion events before dependent actions proceed.
Use concrete event and effect types for the current feature.
Avoid a universal framework or stringly typed command language.

```text
Production requests and adapter results
                 |
                 v
       Shared production Step
                 |
                 v
           Requested effects
           /               \
Real adapters           Simulated adapters
           \               /
            Completion events
```

Production and simulation each supply their own event delivery loop.
The shared function must actually serve production requests.
A test-only copy of admission rules would test the wrong implementation.
Simulated adapters represent external contracts; they must not reproduce coordinator decisions.

One owner serializes each coordinator's decisions.
External operations run separately and report their results as events.
An unreachable guest must not block another guest's decisions.
Keep the initial coordinator scoped to admission; expand with implemented behavior.
Preserve actual database constraints even when coordinator decisions also reject conflicts.

## Minimum scheduler and replay

A scheduler chooses which pending event executes next.
Logical time is an integer clock advanced by the harness.
A seed initializes the generator choosing event orders and injected faults.
A trace records the resulting inputs, effects, completions, and assertions.

Begin with these small pieces:

1. An explicit event queue with stable event identifiers.
2. Logical time, deadline events, and documented ordering for simultaneous events.
3. A local random generator used for every generated choice.
4. Named fault boundaries and readable transition traces.
5. Simulated atomic reservations and controlled response delivery.
6. Restart support separating volatile state from modeled durable records.

Build them incrementally, explaining each addition through an observable failure.
First deliver fixed events; then add logical deadlines and explicit response loss.
Next preserve records across restarts and produce readable replay traces.
Only then introduce seeded choices among legal pending events.
This progression teaches scheduling, causality, and uncertainty through actual production behavior.
Do not turn the exercise into a runtime or virtual-machine emulator.

Use handwritten scenarios before adding randomized variations.
Let randomness choose only valid pending events and allowed fault outcomes.
Preserve causal dependencies; completions cannot precede their corresponding effects.
Bound event counts and logical time to detect stalled scenarios.

For replay, record revision, toolchain, harness version, configuration, seed, and initial state.
Pin the generator algorithm and version relevant to recorded seeds.
Also retain the actual event and fault trace.
Replaying a trace should bypass random choice generation.
Check that replayed events remain applicable to the current pending effects.
Fail clearly when code changes make an old trace incompatible.
Seeds alone need not reproduce execution after implementation changes.
Keep important failures as readable regression scenarios.

Compare two runs' normalized traces and outcomes during harness validation.
Exclude diagnostic timestamps and temporary paths from that comparison.
Such comparisons detect accidental nondeterminism; they cannot prove its complete absence.

## Crash and pending-effect semantics

Durable records represent effects confirmed committed by the simulated storage contract.
Volatile state represents information lost when the coordinator process crashes.
External resources can outlive that crash, just as guests can survive production outages.

Do not interpret every pending effect as cancelled during a crash.
An operation may finish even when its caller never receives confirmation.
Model these cases separately:

| Boundary | Allowed crash outcome | Restart consequence |
| --- | --- | --- |
| Effect not dispatched | No external change | Reconsider from durable intent |
| Reservation transaction pending | Atomic commit or no commit | Inspect the stored identifier |
| Commit succeeded, response lost | Reservation remains durable | Matching retry returns existing state |
| Guest request already dispatched | Guest may accept or continue | Inspect its original action identifier |
| Process launched, evidence missing | Process may exist without confirmed ownership | Preserve uncertainty; prohibit blind replay |

The scheduler records which external outcomes occurred, even when acknowledgments disappear.
Restart reconstructs the coordinator using production recovery logic and durable records.
It does not restore the previous in-memory object as a shortcut.
Discard old process callbacks; correlate later observations through valid identities and requests.
Idempotent inspection can resume; ambiguous mutations require their documented recovery rules.
An action identifier stays stable across retries and inspection.

Initially, storage transactions have atomic all-or-nothing modeled outcomes.
This assumption covers coordinator reasoning, not actual filesystem durability.
Actual PostgreSQL and journal crash tests must validate their respective storage contracts.
Power failures, cache loss, and torn storage writes remain outside this simulator.
An `agent-plane` crash leaves modeled PostgreSQL running unless separately faulted.
Keep database restart, connection loss, and coordinator restart as distinct events.
Lost commit acknowledgments require outcome inspection through stable request identifiers.
Do not assume a missing row proves an in-flight transaction already aborted.
Simulate transaction rejection and retry only according to the selected adapter contract.
The simulator does not implement PostgreSQL's locking or query engine.

## First production slice

Slice 1 defines boundaries, event ordering, trace metadata, and minimal harness scaffolding.
Scaffolding alone does not establish tested production behavior.
That slice supplies real registration decisions alongside their first simulation scenario.
It introduces `make test-sim` against that shared production core.
That command remains planned until its scenarios execute and produce verification evidence.

1. Submit registration A with a particular request fingerprint.
2. Commit its reservation, then lose the caller's response.
3. Retry A using the identical fingerprint.
4. Reuse A with different parameters and submit competing registration B.
5. Crash the coordinator while retaining durable reservation records.
6. Restart through production recovery, then inspect and retry A.

Assert one reservation and one stable identity for A.
Matching retries return recorded state; conflicting fingerprints fail.
B cannot acquire A's reserved machine identity.
Restart cannot turn missing evidence into permission for conflicting work.
Check these properties throughout execution, not only after the final event.
Before guest execution exists, this proves nothing about actual command launch deduplication.

Run matching contract cases against real PostgreSQL reservation operations.
Compare returned classifications and persisted ownership, including uniqueness violations.
Do not infer correct SQL from a passing in-memory adapter.
Use concurrent connections to check conflicting ownership and unrelated-machine progress.
Include concurrent capacity admission, cancellation, and bounded transaction-abort retries.
Guest effects must never execute inside retried database transactions.

Real integration uses disposable PostgreSQL databases with production migrations and driver settings.
Record the pinned server version, isolation policy, and durability configuration.
Database restart tests require an exclusively owned server instance.
An isolated database on a shared server cannot authorize restarting that server.
Test service restart with PostgreSQL alive, then database restart and connection recovery separately.
Fixture setup remains planned; missing prerequisites must fail explicit integration checks.
See [PostgreSQL isolation](https://www.postgresql.org/docs/current/transaction-iso.html) and [SQLSTATE errors](https://www.postgresql.org/docs/current/errcodes-appendix.html).

## Independent safety and progress checks

A safety invariant forbids a bad outcome throughout a scenario.
A progress check requires an observable result under explicitly stated operating conditions.
Assertions must express requirements independently of the implementation's calculations.
Avoid a second function that merely repeats `Step` to predict its output.

Check ownership, request identity, durable ordering, and permitted recovery evidence.
Later, check stale boot rejection and output cursors against durable event records.
Track issued execution effects independently when testing duplicate prevention.
An intentionally unresolved command may retain its slot indefinitely.

For progress, stop relevant faults and deliver required completions fairly.
Fair delivery means eligible work cannot remain postponed forever.
Require the expected result within stated event and logical-time bounds.
Also keep machine A unreachable while requiring healthy machine B's progress.
Do not repeatedly restart B to conceal stalled recovery.
Report the final state, pending events, and assumptions when bounds expire.

## Growth and real-component coverage

| Slice | Simulation addition | Separate real evidence |
| --- | --- | --- |
| 1 | Fixed scheduler, replay, registration reservations | PostgreSQL concurrency, constraints, reconnecting, and restart |
| 2 | Lifecycle intent, logical deadlines, effect completion | Processes, networking, two actual guests |
| 3 | Guest acceptance, launch uncertainty, collection | Journals, files, subprocesses, detachment |
| 4 | Competing file/input/cancel events, seeded schedules | Files, pipes, process termination, limits |
| 5 | Independent failures, stale observations, progress | Service, guest-agent, and database restart behavior |
| 6 | Transport switching and equivalent outcomes | Actual HTTP/HTTPS/WebSockets and fallback |
| 7 | Drain, publication, restore identity | Actual snapshots, restoration, and artifact checks |

Use shared contract cases to compare simulated and real adapters where possible.
Document intentional differences and unsupported fault types beside each adapter.
Simulation cannot replace race detection, integration tests, or privileged validation.
Real tests must check effect ordering across adapter boundaries too.

## Go limitations and maintenance cost

Keep goroutines, real I/O, and wall-clock reads outside the deterministic core.
Supply identifiers and time explicitly; sort map keys before order-sensitive decisions.
Go leaves map iteration unspecified and chooses among ready channel cases pseudo-randomly.
A seeded application generator controls neither behavior. See the [Go specification](https://go.dev/ref/spec).

`testing/synctest` provides fake time and synchronization for isolated concurrent tests.
Its documented isolation discourages real networks and external processes.
It does not promise seeded control over goroutine scheduling.
Use it for appropriate adapter tests, alongside the explicit coordinator scheduler.
See [Go 1.26 synctest documentation](https://pkg.go.dev/testing/synctest@go1.26.0).

For solo development, adapter maintenance is the main continuing cost.
Starting before coordination spreads across goroutines reduces later restructuring.
Keep the harness beside its first owning package.
Extract shared helpers only when another real feature needs them.
Avoid emulating operating systems, arbitrary disk corruption, or Firecracker internals.
Defer generalized scenario languages, automatic minimization, and large seed farms.
Do not defer readable failure traces or deterministic replay.

The first acceptance gate requires one production scenario and one meaningful fault.
Require repeatable replay, independent invariants, and real reservation contract checks.
Expand only when new production behavior supplies corresponding risks and assertions.
Keep every modeled assumption visible in [TESTING.md](../TESTING.md).
