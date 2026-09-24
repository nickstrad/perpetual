# Execute and resume a designed plan

Use only when implementation is requested or already authorized. A request to write a plan ends with a plan.

## Resume without guessing

Read the plan, its latest state snapshot and subsequent events, relevant worker logs, and governing guidance. Check the working tree, branch/worktree, live agents, pending processes, and referenced evidence. Resolve ownership before starting another worker. Existing changes are not evidence that their tests passed. Completed tests are not proof that later edits passed them.

Record the actual available model IDs, effort settings, and environment alternatives chosen from the skill's routing policy. Preserve user overrides. If a platform cannot select a named model or start a subagent, state that limitation and use an explicitly permitted alternative; do not simulate an independent review by relabeling your own pass.

## Contract and red phase

Dispatch the highest-tier assigned test owner first. Supply the design's behavior contracts, acceptance IDs, file boundaries, and intended failure cases. The test owner writes suites, fixtures, simulator, test targets, and testing documentation. They may establish shared types, narrow interfaces, and empty implementations to give tests stable entrypoints. Coordinate shared-file ownership with the production owner before writing them.

Distinguish setup from TDD evidence. Missing package declarations, broken imports, missing services, and invalid fixture configuration are setup failures. Compile the suite against explicit unimplemented skeletons, then show assertions failing for absent behavior. A bounded `ErrNotImplemented` return can be suitable for an expected-error API, but must never become a production success or satisfy a test accidentally. Do not use `t.Skip` as red evidence.

Write expected outcomes from the requirements. The oracle must not call the production decision to compute its expected result. Prove fault controls reach the intended boundary using handshakes. Record the command, revision/diff identity, case IDs, actual failure reason, exit status, and evidence location. Release dependent implementation when the contract is stable and the red result is meaningful.

## Implementation and refactor phase

Dispatch bounded packets to their assigned production models. Include only the relevant design sections, accepted interfaces, case IDs, owned files, predecessor evidence, and state instructions. Give every goroutine, transaction, and buffer an owner; enforce the plan's TigerStyle bounds and uncertainty rules.

Implement the behavior, run assigned tests, then refactor while retaining evidence. Production owners can suggest test or contract improvements, but the test owner and lead review changes that alter acceptance or weaken an oracle. Do not change expected results merely to make the suite green.

Parallelize only independent work with settled interfaces and distinct file ownership. Shared types, migration ordering, Makefile, CI, and `TESTING.md` need explicit handoffs. Keep the test owner available for discrepancies; do not let every worker edit the simulator.

Each worker reports changed files, behavior, exact checks/results, failures, limitations, and next step. The lead appends the report and updated task snapshot to `plan_state.md`; worker logs stay separate. A worker's completion message means ready for review, not accepted.

## Independent review and acceptance

Assign a separate reviewer at the highest suitable tier. They read the requirements, relevant design, diff, tests, and raw evidence. They challenge both implementation and test oracles: missing condition pairs, accidental test-only behavior, hidden nondeterminism, fatal invariants caught by HTTP, unsafe retries, cleanup, and unvalidated external assumptions.

The reviewer runs focused checks needed to validate the change, not an arbitrary repeated full suite. Report concrete findings with file locations, behavior at risk, and required evidence. Fix material defects before dependent work; send test corrections through the test owner. Request another review of changed risk areas.

The lead validates integration and acceptance evidence, records checks actually run, and updates the plan, durable knowledge, delivered status, and testing guide as appropriate. Never mark simulated success as proof of real SQL/process/filesystem behavior. Commit only when authorized; keep relevant testing documentation and implementation together.

## Context handoff

Before yielding or clearing context, append a timestamped snapshot of every active task, live worker/run, owned files, pending commands, decisions, evidence, blockers, and precise next action. Include authorization already granted and any remaining boundary that needs user input. Never rely on chat memory alone.
