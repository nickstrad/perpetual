# Repository agent instructions

Start repository work with [the knowledge index](docs/knowledge/index.md).
Read entries relevant to your task before investigating or changing behavior.
Read [GUIDANCE.md](GUIDANCE.md) before planning or changing this repository.
Its central principle is understandable, maintainable engineering over cleverness or unnecessary machinery.
Read [TESTING.md](TESTING.md) before designing tests or changing tested behavior.
Follow [Go TigerStyle](docs/TIGERSTYLE.md) for implementation and invariant handling.
Read [simulation research](docs/TEST_RESEARCH.md) before changing coordination or simulated boundaries.
Follow the [MVP breakdown](docs/plans/README.md).
Expand only the current slice after discussing its boundaries and acceptance evidence.
Use the [current slice design](docs/plans/durable_requests/plan.md) for detailed contracts.
Superseded plan revisions remain in Git history.

The assigned test owner maintains testing and `TESTING.md` throughout implementation.
Production owners supply behavior contracts and collaborate on necessary testability refactors.
The lead assistant performs final review, validates evidence, updates plans, and handles commits.
Use the [detailed-design skill](.agents/skills/detailed-design/SKILL.md) for detailed designs and TDD delegation.
Keep model-selection policy in that skill and explicit model assignments in individual plans; general guides describe roles.
Keep testing documentation and relevant code changes together in commits.
Never describe planned checks as implemented or report unexecuted tests as passing.

## Working state

Each set of agent work owns one state file: `.state/<work-slug>.md`.
Git ignores that folder; state files are never committed.
A state file lets work resume after context clears or AI tool changes.
Start it with the goal, scope, and owner of that work.
Append dated activity while working; never rewrite earlier entries.
Record decisions, changed behavior, checks actually run, unresolved risks, and the next step.
Never reuse one file across separate chunks of work; start a fresh file.
Each detailed plan also has a lead-owned `.state/<plan-slug>/plan_state.md` ledger, linked from the plan.
It tracks all active tasks and their evidence across workers and tools; append snapshots without rewriting history.
Workers keep their own work files and report changes to the lead instead of concurrently editing the plan ledger.
Check `.state/` before starting; never edit or delete another set of work's file.

Finished state files may stay until someone cleans them up.
A file becomes garbage once its work is committed and nobody must resume it.
Before deleting one, read it for useful information and save that knowledge first.
Project findings go into `docs/knowledge/`; plan changes go into the plans.
Machine and tooling findings go into the VM knowledge store through `kb`.
Delivered slice status belongs in [project direction](docs/knowledge/project-direction.md), not in a state file.

## Knowledge

Record useful, lasting project knowledge in `docs/knowledge/` for future agents and humans.
Use a standalone `*.md` file for text-only summaries.
Give knowledge containing artifacts its own descriptively named folder.
Artifacts include supporting documents, scripts, datasets, and nested folders.
Every folder beneath `docs/knowledge/`, including its root, must contain `README.md`.
Explain each folder's contents, purpose, and safe usage in its README.
Update `docs/knowledge/index.md` whenever adding, moving, or removing entries.
Index every file and folder, including nested artifacts and README files.
Keep knowledge current; distinguish verified findings, proposals, and unresolved questions.
Link authoritative plans and guides instead of duplicating their evolving details.
Never store credentials, private runtime data, or unreviewed generated dumps.

`AGENTS.md` links to this file so both tools share these instructions.
Keep that relative symlink intact; do not create divergent instruction copies.
