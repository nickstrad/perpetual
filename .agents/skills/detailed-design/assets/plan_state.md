# Plan state: PLAN_SLUG

Plan: REPOSITORY_RELATIVE_PLAN_PATH
Goal: CONCRETE_OUTCOME
Scope: CURRENT_SLICE_AND_PHASE
Ledger owner: LEAD_IDENTITY
Authorization: USER_AUTHORIZED_PHASE_AND_CONSTRAINTS
Created: UTC_TIMESTAMP

This is an ignored, append-only ledger. Replace the template fields when creating a new file. After initialization, append history; never edit an earlier snapshot. Only the lead writes this ledger. Workers own separate `.state/<work-slug>.md` files. No credentials or private runtime data.

## UTC_TIMESTAMP — Initial snapshot

Repository/worktree: PATH
Branch and source revision: BRANCH_AND_REVISION
Working-tree changes at handoff: FILES_AND_OWNERS
Plan status: draft / ready / implementing / review / accepted
Implemented behavior: NONE_OR_VERIFIED_BEHAVIOR

| Task | Phase/status | Owner role | Chosen model / actual model ID / effort | Worker/run | Owned files | Dependencies | Evidence | Exact next action |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| TASK_ID | planned | ROLE | PLANNED_MODEL; not dispatched | none | PATHS | GATES | none | ACTION |

Decisions and rationale: ACCEPTED_AND_PROPOSED_DECISIONS
Unresolved questions/risks: ITEMS_WITH_OWNERS
Checks actually run: COMMAND_CWD_REVISION_EXIT_RESULT_EVIDENCE
Red/green/review evidence: OBSERVED_RESULTS_ONLY
Live commands/processes/resources: IDS_OWNERS_AND_SAFE_RESUMPTION
Next action after context clear: ONE_CONCRETE_STARTING_ACTION
Durable updates still needed: PLAN_KNOWLEDGE_TESTING_GUIDE

## UTC_TIMESTAMP — Activity or replacement snapshot

Append the change, why it happened, affected task and contract IDs, actual evidence, remaining risk, and next action. Append a complete fresh task snapshot at phase boundaries or when handing off context so resumption does not depend on reconstructing a long log. A later snapshot supersedes earlier statuses without erasing history.
