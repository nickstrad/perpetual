# Repository agent instructions

Start repository work with [the knowledge index](docs/knowledge/index.md).
Read entries relevant to your task before investigating or changing behavior.
Read [GUIDANCE.md](GUIDANCE.md) before planning or changing this repository.
Its central principle is understandable, maintainable engineering over cleverness or unnecessary machinery.
Read [TESTING.md](TESTING.md) before designing tests or changing tested behavior.
Follow [Go TigerStyle](docs/TIGERSTYLE.md) for implementation and invariant handling.
Read [simulation research](docs/TEST_RESEARCH.md) before changing coordination or simulated boundaries.
Follow the [MVP breakdown](plans/mvp/breakdown/README.md) and its [state tracker](plans/mvp/breakdown/STATE.md).
Expand only the current slice after discussing its boundaries and acceptance evidence.
Use [v1.5](plans/1-mvp/v1.5.md) for detailed reference, not the active implementation sequence.

Astra owns testing and maintains `TESTING.md` throughout implementation.
Production owners supply behavior contracts and collaborate on necessary testability refactors.
The lead assistant performs final review, validates evidence, updates plans, and handles commits.
Keep testing documentation and relevant code changes together in commits.
Never describe planned checks as implemented or report unexecuted tests as passing.

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
