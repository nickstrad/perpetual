# Substance of an implementation design

Read this when creating or substantially revising a plan. Organize sections to suit the feature, but do not leave the following decisions for implementation agents to guess.

## Starting point and boundary

Record date, source revision, current implementation status, governing documents, and the matching state path. State the smallest useful user demonstration and the exact behavior delivered. Identify excluded later-slice behavior and explain material differences from historical references. Separate requirements, selected proposals, unresolved decisions, and verified facts.

Resolve discussion questions explicitly. Record the chosen option, rationale, tradeoff, and evidence that would challenge it. Do not leave every routine implementation choice open. A material unresolved contract blocks only dependent assignments; name that dependency.

## User and component contracts

Specify requests, responses, CLI behavior, identifiers, schemas, defaults, validation, size limits, normalization, timeout units, retry semantics, status and error classifications, and compatibility. Include concrete successful, duplicate, conflicting, malformed, overloaded, and uncertain examples where relevant. Say what success proves.

Map actors and responsibilities. Name package and file paths; identify dependency direction, state owners, resource lifetimes, and public interfaces. Describe startup, readiness, normal operation, cancellation, shutdown, restart, and deployment/migration handling. Prefer existing modules and small interfaces to generic frameworks.

## TigerStyle and state transitions

Provide a table of states, events, preconditions, next state, effects, reply, and invariant. Explain the commitment point for each durable change and the evidence needed before dependent work. Keep expected operating errors distinct from programming defects. Cover stale observations and lost acknowledgments.

Specify numeric ranges, units, queue/buffer/worker bounds, overflow handling, retry count and total budget, lock order, goroutine owners, channel closure, cleanup, and process termination. Explain how fatal invariants stop the service even inside recovered HTTP handlers. Describe determinism and how mutable inputs remain unchanged.

## Substantial code sketches

Use several coherent blocks that show the actual difficult design. Usually include:

- Shared domain types, one or more full pure decision functions, and important invariants.
- Event/effect definitions and a meaningful transition path through production and simulation.
- Storage schema/constraints and transaction sequence with commit uncertainty and recovery.
- A real adapter or handler path showing errors, cancellation, ownership, and cleanup.
- Representative behavioral tests, MC/DC cases, and a fault scenario with independent expected outcomes.

Each block names the destination file and says **proposed, not executed**. Include imports when they clarify dependencies. Identify omitted helpers and give their contracts. Pseudocode is acceptable when explicitly labeled; do not present placeholder bodies as finished implementation. Avoid writing a speculative whole codebase in Markdown. Never compile or test these blocks in the design phase.

## Failure design and evidence

For each meaningful cut point, record what may have happened externally, surviving durable/volatile state, legal recovery evidence, forbidden actions, and the specific test. Distinguish service crash, adapter disconnect, database crash, cancellation, and ordinary returned errors. Bound progress under explicit health and delivery assumptions.

Separate independent test oracles from implementation helpers. Plan MC/DC pairs for handwritten decisions, with short-circuit and infeasible-condition treatment. Add fuzzing for named parser/arithmetic risks. Design deterministic trace metadata, actual-choice replay, and checks after transitions. Identify real components that must validate simulated contracts.

Specify fixture ownership, teardown, readiness handshakes, fault injection location, and how missing prerequisites fail. Name intended commands and exact pass conditions; label them planned until execution. State simulation limitations and remaining evidence gaps.

## Delegation and TDD

Make a dependency-ordered work table and concrete work packets. Every packet needs:

- Task ID, role, chosen model and alternative, effort, and why that tier fits.
- Exact owned files, read-only dependencies, overlapping-file handoff, and predecessor gates.
- Required behavior, skeleton/interface shape where useful, and prohibited shortcuts.
- Test case IDs, required red evidence, expected green evidence, and reviewer.
- A concise dispatch prompt that works without the original conversation.

Release production work only after its test-and-contract gate. Establish interfaces and fixtures as setup, then observe meaningful failures, implement, refactor, and review. Small independent packets may run in parallel when contracts are settled and files do not overlap. Tests can evolve when the contract changes; do not freeze a flawed suite or let implementers silently weaken it.

## Completion and resumption

Map every acceptance requirement to task, test/evidence, and reviewer. Include exact proposed commands, demonstration steps, and expected results. State what counts as ready for review and accepted. Preserve findings, unresolved risks, and the latest next action in the paired state file. Put durable delivered status in project direction only after acceptance.
