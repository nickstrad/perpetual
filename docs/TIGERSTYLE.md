# TigerStyle for Go

This guide adapts the supplied TigerStyle principles to this Go platform.
It supports [GUIDANCE.md](../GUIDANCE.md), where understandable engineering remains the project's central goal.
Preserve correctness and recovery guarantees, explain the design, then optimize measured bottlenecks.
Prefer decisions a solo maintainer can explain, test, and revise confidently.

TigerStyle supplies inspiration, not a requirement to reproduce TigerBeetle's implementation constraints.
See the [upstream guide](https://github.com/tigerbeetle/tigerbeetle/blob/main/docs/TIGER_STYLE.md).

## State and control flow

Give each mutable state structure one clear owner.
Make lifecycle phases, action ownership, and uncertain outcomes explicit.
Keep state decisions together; move external effects behind small, concrete boundaries.
Prefer straightforward branches and early returns over clever Boolean expressions.
Use iteration for lifecycle traversal and untrusted nesting.
Bound work per iteration, response size, retained output, and queued requests.
Long-running service loops need explicit shutdown behavior and bounded work between checks.

Do not assume a failed response means an operation failed.
Distinguish requested effects, accepted work, confirmed completion, and missing evidence.
Never hide those distinctions behind a generic success flag.
Comments must explain why ordering or recovery constraints matter.

## A deterministic decision core

Deterministic means identical inputs produce identical decisions under the same code version.
An event describes something observed; an effect requests work outside the core.
The production coordinator and simulator must call the same decision code.
That code accepts state and events, then returns decisions and requested effects.
It performs no networking, filesystem operations, process launches, or real-time waits.

Supply time, identifiers, and random choices explicitly when they influence decisions.
Use ordered collections or sorted keys when iteration affects behavior.
Never depend on Go map iteration order.
Keep goroutines and uncontrolled channel selection outside the simulated decision core.
Real adapters can use concurrency and report completions as explicit events.
Document event ordering and durable-acknowledgment requirements at those boundaries.

Simulation checks logical behavior, not real kernel scheduling or storage durability.
Keep real integration and machine validation alongside it.
See [testing research](TEST_RESEARCH.md) for the proposed scope and growth path.

## Errors and invariants

An invariant is a rule that internal program state must always satisfy.
An assertion checks such a rule and reports a programming defect when violated.
Operating errors include unavailable guests, invalid requests, disk failures, and expired certificates.
Handle those errors explicitly and return useful context.
Never panic because a user supplied invalid data or an operation timed out.

Assert internal relationships after validating external inputs.
Useful examples include output ordering, reserved ownership, and published snapshot completeness.
Keep important invariant checks enabled in production.
Use separate checks when separate failure messages clarify the violated rule.
Pair checks around meaningful boundaries when each location supplies independent evidence.
Avoid assertion quotas; assertions must express actual understanding.

Go has no built-in assertion statement.
We may add a small internal helper when the first invariant needs reuse.
The helper remains proposed until implementation records actual code and tests.
This illustrative contract uses a distinguishable panic value:

```go
package invariant

type Violation struct {
	Message string
}

func Check(condition bool, message string) {
	if !condition {
		panic(Violation{Message: message})
	}
}
```

Example usage checks already-validated internal state:

```go
invariant.Check(collectedSeq <= durableSeq, "cursor exceeds durable output")
```

Keep predicates free of side effects and messages free of sensitive values.
Do not recover an invariant failure and continue using potentially corrupted state.
In simulation, capture the failure only to record the trace and fail the test.
In production, an invariant failure must stop the affected service process.
Preserve detached guest execution when the control-plane service fails.

Go HTTP servers recover handler panics instead of necessarily terminating the process.
Therefore, plain handler `panic` does not establish this failure policy.
Keep coordinator assertions on owned service workers outside HTTP panic recovery.
If handlers can assert, install an explicit fatal boundary for `invariant.Violation`.
That boundary must terminate the service instead of returning an ordinary response.
Verify worker and handler failure behavior with subprocess tests before relying on it.
See [Go HTTP handler behavior](https://pkg.go.dev/net/http#Handler).

Use ordinary `error` returns for expected failures; preserve causes when adding context.
Check failures from writes, synchronization, process waits, and resource cleanup.
Document cleanup failures without erasing the original operation's error.

## Bounds, types, and resources

Use `int` for Go slice indexes, lengths, and APIs that require it.
Use explicit widths for persisted counters and wire fields when their ranges matter.
Use `time.Duration` for elapsed time; include units in serialized field names.
Check range and overflow before narrowing conversions or adding untrusted sizes.
Distinguish indexes, counts, byte offsets, and sequence numbers in names and types.

Ordinary Go allocation and garbage collection remain acceptable.
Bound admission, buffers, output retention, and worker counts instead of banning allocation.
Preallocate known collections where that improves clarity or measured performance.
Make ownership of byte slices explicit when handing data to another goroutine.
Copy mutable buffers when ownership would otherwise overlap.
Never copy structures containing active locks.

Acquire resources near their cleanup registration.
Use `defer` for ordinary lexical ownership, avoiding unbounded defers inside loops.
Each goroutine needs an owner, shutdown path, and completion observation.
Each child process needs identity checks, output handling, and a termination policy.
Request cancellation must not cancel accepted work that has independent ownership.

## Go conventions and readable functions

Use `gofmt`; accept its tabs and layout.
Use Go naming conventions: exported `MixedCaps`, unexported `mixedCaps`, and readable package names.
Preserve conventional initialisms such as `HTTP`, `ID`, and `URL`.
Prefer domain-specific names over generic helpers such as `HandleEverything`.
Keep variables within their smallest useful scope.
Use option structures when adjacent parameters could easily swap meanings.
Follow Go conventions for context placement and error returns.

Aim for functions readers can understand without scrolling through unrelated responsibilities.
Treat approximately 70 lines as a review prompt, not an automatic extraction rule.
Prefer lines around 100 columns when practical; preserve readable Go formatting.
Do not fragment clear code merely to meet arbitrary numeric limits.
Keep orchestration visible even when leaf operations move into helpers.
Choose pointers for ownership and mutation semantics, not a universal byte-size threshold.

Comments explain reasons, contracts, failure boundaries, and surprising choices.
Tests explain their scenario, injected fault, invariant, and expected recovery.
Commit messages describe the problem and resulting behavior.
Follow [Effective Go](https://go.dev/doc/effective_go) where project requirements do not justify exceptions.

## Performance and dependencies

Estimate memory, disk, network, and CPU costs before committing to architectural choices.
Measure real workloads before adding caches, pooling, or custom allocation strategies.
Batch operations only when batching preserves documented durability and latency boundaries.
Never delay a required acknowledgment boundary merely to improve benchmark numbers.

Prefer the standard library and a small set of justified dependencies.
Existing planned dependencies need not disappear to imitate a zero-dependency policy.
Pin versions and explain what maintenance burden each dependency removes.
Prefer Go for substantial shared tooling when it improves portability and testing.
Small shell scripts remain appropriate for transparent host setup and image construction.

## Testing and review

Astra owns testing design, implementation, simulation, and the living [testing guide](../TESTING.md).
Production owners provide testable boundaries and fix defects in their components.
The lead reviews interactions, verifies evidence, and updates plans before committing.
Add one readable simulation scenario alongside each new simulated behavior.
Preserve failing event traces as focused regression cases.
Use condition-independence checks where compound decisions can hide meaningful gaps.
Do not confuse statement coverage, successful simulation, or assertions with correctness proofs.

Fix known correctness failures before building dependent features.
Record genuine scope deferrals with their consequences and owners.
Do not disguise unresolved defects as successful milestones or impose an impossible debt slogan.
The goal is a platform whose behavior and limitations remain understandable as it grows.
