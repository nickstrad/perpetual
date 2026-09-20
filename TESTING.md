# Testing perpetual

Tests establish observable behavior and expose failures before users encounter them.
This living guide defines the strategy and records its current limits.
The [MVP breakdown](plans/mvp/breakdown/README.md) pairs features with simulator development.
[Project direction](docs/knowledge/project-direction.md) records delivered slices; this guide records test evidence.
The [architecture](plans/1-mvp/v1.5_architecture.md) defines the intended platform behavior.
The [simulation research](docs/TEST_RESEARCH.md) defines our focused early harness proposal.

## Current status

Repository inspection on 2026-09-18 found only the existing boot prototype.
`main.go` and its dependencies do not implement the planned platform.
No test files, Makefile targets, validation scripts, or CI workflow exist yet.
The selected early simulation approach also remains unimplemented.
PostgreSQL is the selected state backend; its testing fixtures remain unimplemented.
CI means continuous integration: automated checks executed after repository changes.
This document establishes direction; it does not establish passing tests.
No runtime checks were executed for this documentation change.
Everything below describes planned work unless explicitly marked implemented.

## Ownership

Astra owns test design, implementation, fixtures, validation scripts, CI, and this guide.
A fixture supplies controlled data or resources needed by a test.
Production workers own application changes and necessary refactors within assigned scopes.
A refactor changes code structure while preserving intended behavior.
Astra identifies testing obstacles and agrees boundary changes with production owners.
Production workers provide behavior contracts, failure boundaries, and reproduction details.
Astra implements the corresponding checks and reports observed results.
The lead reviews integration and independently performs final validation.
The lead alone updates the shared plan, tracker, and acceptance status.
Coordinate shared files before editing; keep independent work in separate files.
These development roles introduce no named assistant products into guest images.

## Terms and priorities

A behavior test checks externally meaningful results from a specific scenario.
An invariant is a rule that must remain true throughout execution.
A safety check verifies that forbidden behavior never occurs during the test.
A liveness check verifies progress under stated operating conditions.
A bounded-progress check requires progress within an explicit time or step limit.
A regression test reproduces a defect and detects its return.
Fault injection deliberately fails a selected operation to examine recovery.

Start with important behaviors, their invariants, and their likely failure boundaries.
Prefer actual files, disposable PostgreSQL databases, local servers, and helper subprocesses.
Replace privileged effects and selected failures through small, explicit boundaries.
Assert persisted records, output, process effects, and returned outcomes.
Avoid assertions that merely mirror private functions or incidental call order.
Add infrastructure when a concrete test requires it.
Keep fixtures smaller than the behavior they help explain.

### Source lessons

SQLite combines ordinary checks, injected failures, crash testing, and regression cases.
Its coverage discussion distinguishes executed statements from independently tested conditions.
Adopt those complementary perspectives proportionately. See [SQLite testing](https://www.sqlite.org/testing.html).
SQLite provides testing inspiration only; PostgreSQL stores platform state.

TigerBeetle separates safety checks from progress checks under sufficiently healthy conditions.
Its liveness tests preserve some failures while requiring unaffected components' progress.
Apply that distinction to independent machines and recovery paths.
See [Simulation Testing For Liveness](https://tigerbeetle.com/blog/2023-07-06-simulation-testing-for-liveness/).

TigerBeetle also checks internal invariants using production code under controlled external interactions.
See [Protocol-Aware Deterministic Simulation Testing](https://tigerbeetle.com/blog/2026-08-20-protocol-aware-dst/).
Dropbox demonstrates focused planning tests sharing production decision logic.
See [Testing sync at Dropbox](https://dropbox.tech/infrastructure/-testing-our-new-sync-engine).
Our [research proposal](docs/TEST_RESEARCH.md) applies these ideas to early coordinator testing.

These sources inspire our choices; they do not establish perpetual's correctness.
We do not inherit their coverage results or simulation infrastructure.

## Checks that matter

An action identifier names one requested operation throughout retries and inspection.
A boot identifier distinguishes separate executions of the same machine.
A journal stores durable guest acceptance and outcome evidence.
A cursor records the last output sequence collected or delivered.

| Behavior | Safety check | Progress check and conditions |
| --- | --- | --- |
| Command retry | Lost acceptance never causes duplicate execution | Recover recorded outcomes once the original guest responds |
| Concurrent admission | Only one command owns each guest's execution slot | A confirmed terminal outcome eventually releases its slot |
| Caller detachment | Disconnecting never cancels accepted execution | A healthy collector obtains completion without attached viewers |
| Control-plane restart | Shutdown leaves guest processes running | Reconciliation resumes collection with reachable guests and writable storage |
| Independent machines | Cleanup never touches another machine's resources | Healthy machines remain usable while another stays unreachable |
| Output recovery | Committed cursors never exceed durable output | Replay reaches terminal status, reporting any retention gaps |
| Guest restart | Unfinished journal entries never trigger blind replay | Inspection reports uncertainty within its configured request deadline |
| Snapshot publication | Incomplete artifacts never become restorable snapshots | Drain timeout resolves without leaving admission permanently closed |
| Restoration | Old boot observations cannot alter current state | Verified restore opens admission after fresh identity bootstrap |
| Input delivery | Uncertain input acknowledgment never causes automatic resend | Definite delivery or explicit uncertainty becomes observable |

An uncertain action may intentionally retain its command slot.
Do not assert eventual execution when safe recovery lacks sufficient evidence.
Commands without execution deadlines may legitimately continue indefinitely.
Every progress test states required health, available evidence, and its expected endpoint.

## Layers and fixtures

### Focused behavior checks

Use table cases for validation, identity matching, allocation, and state decisions.
Test limits immediately below, at, and above each configured boundary.
Exercise malformed requests, conflicting identifiers, and stale observations.
Keep assertions tied to public behavior and documented invariants.

### Real local integration

Integration tests combine components to check their shared behavior.
Use disposable PostgreSQL databases for constraints, concurrent reservations, migrations, and reconnecting.
Use separate connections to race conflicting and independent reservations.
Verify both ownership exclusion and progress for unrelated machines.
Use actual journals and output files for publication and recovery checks.
Use local HTTP, HTTPS, and WebSocket servers with temporary certificates.
Run identical outcome cases through polling and streaming.
Include failed upgrades, disconnects, replay, expired output, and final collection.
Use actual helper processes for detachment, stdin, deadlines, and cancellation.
Substitute only virtualization and privileged networking where local integration requires it.
Validate substitutes against real components when relevant capabilities become available.

### PostgreSQL integration setup

Pure decision tests and the custom simulator require no database server.
Real store tests require an explicitly configured disposable PostgreSQL fixture.
Pin the tested server version and record its durability and isolation settings.
Use the same driver and migrations as production.
Each test run owns its database names, credentials, and cleanup scope.
Never discover or reuse an operator's application database implicitly.
Restart tests require an exclusively owned server instance, not merely another database.
Publish fixture setup and teardown commands when their implementation exists.
Missing prerequisites must fail explicit integration checks, not silently skip them.
CI must provision its disposable fixture before running the full suite.

Check uniqueness, conditional updates, transaction rollback, and concurrent capacity admission.
Exercise exhausted connection pools, cancelled waits, lock contention, and reconnection.
Test recognized transaction-abort retries under the selected isolation policy.
Bound retries and preserve request identifiers throughout recovery.
An interrupted commit acknowledgment can leave its outcome unknown.
Inspect durable request records before deciding whether further mutation is safe.
A missing row does not prove an in-flight transaction already aborted.
Keep guest execution outside database transaction retries.
PostgreSQL documents isolation semantics and structured SQLSTATE errors.
See [transaction isolation](https://www.postgresql.org/docs/current/transaction-iso.html) and [error codes](https://www.postgresql.org/docs/current/errcodes-appendix.html).

### Real-machine validation

Privileged checks require actual virtualization, networking access, and disposable guest resources.
Verify boot, SSH, two-machine independence, process survival, snapshots, and restoration.
Include service-manager shutdown behavior; process detachment alone cannot establish survival.
Verify guest child handling and cleanup against actual operating-system behavior.
Local substitutes cannot establish kernel, Firecracker, image, or host-network correctness.
An explicitly requested validation must report missing prerequisites as failure or blockage.
Never report skipped virtualization checks as passing machine coverage.

### Resource and timing discipline

Each test owns its temporary directories, listeners, child processes, and cleanup.
Never use an operator's live runtime directory or existing guests as fixtures.
Verify exact ownership before signaling processes or removing network resources.
Use explicit readiness handshakes instead of sleeping for assumed startup durations.
Inject clocks only where application timing decisions need control.
Real subprocesses still need bounded deadlines to detect hangs.
Record the deadline, observed phase, and surviving resources when a check times out.
Check process, listener, and file cleanup even after expected failures.

## Failure and crash testing

Returning an error exercises an error path within a surviving process.
A crash also removes memory, interrupts coordination, and bypasses ordinary cleanup.
Terminate relevant subprocesses, then reconstruct state through the actual recovery paths.
For guest journals and output files, reopen persisted files after process termination.
An `agent-plane` crash normally leaves PostgreSQL running; test that distinction explicitly.
Separately restart the disposable database and verify connection recovery and durable records.
Neither crash implies guest termination or safe replay of uncertain operations.
Place explicit handshakes around selected boundaries to target the intended interruption.

Begin with these fault locations:

1. Guest acceptance persisted, before process identity publication.
2. Command started, before acceptance reaches the host.
3. Host output appended, before its collection cursor commits.
4. File replacement published, before its result enters the journal.
5. Snapshot artifacts written, before the completed manifest publishes.
6. Snapshot published, before machine termination receives durable confirmation.
7. Restore launched, before fresh boot identity opens admission.

After each interruption, restart the affected component using its existing files.
Check durable effects, preserved ownership, duplicate prevention, and explicit uncertainty.
Inject one-time and persistent write, rename, synchronization, or transport failures where relevant.
Begin with one failure per scenario; combine failures when risks justify them.
Also fail recovery itself when that boundary controls accepted durability guarantees.

Process termination does not simulate power loss or lost filesystem caches.
Injected filesystem errors do not establish every storage device's behavior.
Record these limits beside durability results.
Avoid building a filesystem simulator without a specific unresolved durability requirement.

## Condition independence

A condition is one Boolean input to a decision.
Condition independence means changing one condition can change that decision's result.
MC/DC means Modified Condition/Decision Coverage, a stronger coverage discipline.
Use its independence idea for critical admission and recovery decisions.
We do not claim measured or formal MC/DC coverage.

Consider admission after resolving duplicate identifiers:

```text
allow = ready && slot_free && !draining
```

Each pair below changes only one input from the baseline.

| Case | ready | slot_free | draining | allow | Pair |
| --- | --- | --- | --- | --- | --- |
| Baseline | true | true | false | true | None |
| Not ready | false | true | false | false | Baseline and not ready |
| Slot occupied | true | false | false | false | Baseline and slot occupied |
| Draining | true | true | true | false | Baseline and draining |

Check both rejection and absence of external effects for denied requests.
Also verify matching retries return recorded outcomes despite occupied slots or draining.
Apply similar pairs to identity checks and snapshot publication prerequisites.
Review feasible input combinations; coupled conditions may prevent simple independent pairs.
Go's usual coverage reports measure statements, not condition independence or formal MC/DC.
Use coverage to locate unexecuted statements and guide review.
High statement coverage cannot establish complete behavior, branch, or recovery coverage.
Do not remove defensive checks simply to raise coverage percentages.

## Progress after failures

First, inject selected faults while checking safety invariants.
Then restore the conditions necessary for the chosen component's progress.
Keep unrelated failures fixed when healthy components should tolerate them.
Require the specified observable result within a documented bound.

For example, keep machine A unreachable while machine B stays healthy.
Require B's reconciliation and completed-action collection within the test's chosen deadline.
Ensure A remains unresolved without blocking B's admission or collection.
Do not periodically restart B to conceal stalled recovery.
Separately test recovery when A becomes reachable with matching identities.

Choose bounds from configured deadlines, polling intervals, and expected work.
Document scheduling allowances for real processes and slow test hosts.
A finite test establishes observed bounded progress under those assumptions.
It does not prove termination under every possible schedule or permanent failure.

## Generated cases and simulation

Fuzzing generates varied inputs to expose unexpected behavior.
Start with bounded decoding, cursor, and size-limit cases when implementations exist.
Keep every reproduced defect as a small regression case.
A seed initializes a repeatable sequence of generated choices.
Record seeds, input traces, configured faults, versions, and failing assertions.
Seeds do not control Go scheduling, real network timing, or subprocess execution.

Deterministic simulation controls modeled time, scheduling, randomness, and external effects.
Start a focused harness alongside the first production coordination code.
Build a custom event scheduler using ordinary Go and its standard library.
Keep it local to the first owning package and grow it incrementally.
This bounded implementation also teaches event ordering, causality, and failure recovery.
No patched runtime, hosted service, or external simulator is the baseline.
A coordinator makes state decisions and requests effects from external components.
Both production and simulation must call the identical decision function.
The simulator schedules typed events, logical deadlines, and controlled effect completions.
It must not copy production admission rules into a separate toy implementation.

Slice 1 establishes boundaries and exercises actual registration and reservation decisions.
Scaffolding alone never counts as production validation.
First commit registration, lose its response, then retry and restart.
Assert stable identity, one reservation, conflicting-request rejection, and preserved registration ownership.
Compare reservation contracts against real PostgreSQL behavior using concurrent connections.
Expand through lifecycle, guest acceptance, collection, recovery, and snapshots as implemented.

Crashes discard volatile coordinator state while retaining modeled durable records.
Already dispatched effects may finish even when their acknowledgments disappear.
Restart through production recovery logic, inspecting existing identifiers before ambiguous mutations.
Keep pending-effect outcomes and allowed storage assumptions explicit.

Record revision, toolchain, generator version, configuration, seed, and complete event trace.
Replay recorded events directly; seeds alone may change meaning after code changes.
Control all ordering inside the core, including map traversal and simultaneous events.
Do not rely on ordinary goroutine scheduling for deterministic replay.
Check independent invariants after each transition and progress under stated health assumptions.

Full operating-system and Firecracker simulation remain outside the selected approach.
Actual storage, processes, transports, and virtualization still require their separate tests.
Keep fault models small and compare adapter contracts against real implementations.
See [TEST_RESEARCH.md](docs/TEST_RESEARCH.md) for boundaries, crash semantics, and milestone acceptance.

## Commands and delivery

No project-specific test command is currently implemented or verified.
Ordinary Go commands can inspect the prototype when its toolchain resolves.
Without test files, successful package testing supplies no planned-platform behavior evidence.
The following commands are planned interfaces, not runnable instructions today.

| Planned command | Purpose |
| --- | --- |
| `make test-fast` | Short Go tests with explicitly documented omissions |
| `make test` | Complete unprivileged suite using its disposable PostgreSQL fixture |
| `make test-race` | Detect exercised data races across the unprivileged suite |
| `make test-integration` | Real component checks requiring disposable PostgreSQL |
| `make test-sim` | Custom deterministic coordinator scenarios and replay, first delivered in slice 1 |
| `make test-fuzz` | Bounded runs of explicitly named fuzz targets |
| `make coverage` | Generate statement coverage for review |
| `make vet` | Static Go diagnostics |
| `make validate` | Preflight followed by required real-machine validation |

A race detector finds exercised races; a passing run cannot exclude every race.
Simulation scenarios remain part of `make test` once their production core exists.
`make test-sim` selects those scenarios for focused iteration and replay.
Fast checks and simulation remain database-free; full testing includes PostgreSQL integration.
Fuzz commands must document target names, duration, and reproduction syntax once implemented.
Fast mode must name omitted checks; full unprivileged testing includes them.
CI initially runs complete unprivileged tests, race checks, and static diagnostics.
Keep privileged validation separate until an appropriate runner exists.
CI must preserve useful failure logs and propagate failing exit statuses.
Never implement success placeholders for missing checks.

Introduce testing with each implemented behavior, starting with slice 1 boundaries and tooling.
Add real database and API cases with slice 1 implementation.
Add process and lifecycle checks when those components become executable.
Add recovery checks alongside journals and collectors, before later integration milestones.
Keep privileged milestone evidence distinct from local test results.
Slice 7's audit checks remaining gaps; it does not postpone earlier failure testing.

## Maintenance and acceptance

For each change, record its behavior, invariant, failure boundary, and verification scope.
Astra updates relevant fixtures, regression cases, and this guide as needed.
Update command status only after execution establishes its actual behavior.
For failures, retain the exact command, observed result, and smallest reproduction.
Include relevant seed, trace, fault location, versions, and diagnostic artifact paths.
Keep logs free of secrets and unrelated operator data.

The lead checks the combined changes and performs appropriate independent validation.
Record remaining limitations and missing privileged evidence before accepting affected milestones.
Do not repeat checks without changed code, failures, or unresolved concerns.
Documentation changes need link and consistency checks, not unrelated runtime execution.
Remove obsolete fixtures when behavior changes; retain relevant historical regression protection.
Keep this guide synchronized with implemented commands and accepted architecture decisions.
Commit guide updates alongside the testing changes they explain.
