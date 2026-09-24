# Project direction

Updated: 2026-09-24. Status: slice 1 registration implemented and locally validated on the implementation branch; slices 2–7 remain planned.

The architecture below describes the target platform. The implemented surface is
durable registration and inspection only; it does not provision machines or run guest actions.

The repository teaches understandable systems engineering through virtual-machine orchestration.
Callers request concrete actions; the platform does not choose their workflows.
`agent-plane` manages machine lifecycle, durable host state, and guest action routing.
`vmagent` runs inside each guest and exposes concrete actions through HTTP/HTTPS.
Optional WebSocket connections attach to existing actions without restarting execution.
SSH handles interactive access; accepted commands survive caller disconnection.

PostgreSQL replaces SQLite in the active design, not historical plan revisions.
The control plane uses concurrent connections while preserving explicit admission ownership.
A package-local fixed simulator exercises shared production decisions using controlled events and faults.
The simulator requires no database; real integration tests require isolated PostgreSQL.
Testing direction updated 2026-09-22: use MC/DC as the coverage standard within basic behavior testing, targeted native Go fuzzing, and deterministic simulation.
Compose business logic from pure functions and apply the invariant, boundary, and resource rules in [TESTING.md](../../TESTING.md).
The assigned test owner owns testing; the lead validates results, maintains the plan, and commits changes.

Slice 1 now has Go CLI/service binaries, PostgreSQL migrations and an adapter,
a bounded coordinator, strict HTTP decoding, and deterministic replay.
The [slice 1 design](../../plans/mvp/breakdown/1-durable-requests-plan.md) owns the contract;
[testing](../../TESTING.md) records actual evidence and coverage limits.
Docker development commands retain local data; integration tests own disposable
PostgreSQL 18 fixtures. [Durable registration boundaries](durable-registration.md)
records gate, uncertainty, startup snapshot, and fixture findings.
Independent review has no unresolved material correctness findings. The
[acceptance record](../testing/durable-requests-acceptance.md) records passing local
checks and residual coverage limits. Full MC/DC is not established; later slices
remain unimplemented. This branch has not been merged.
Update this status whenever a slice's implementation or acceptance changes.
Seven vertical slices pair useful platform behavior with incremental simulator development.
Each slice includes a cumulative diagram and questions for later refinement.
Each slice plan lists its actors and walks through their planned workflows.
Diagrams render from Mermaid sources; see [text diagram rendering](text-diagram-rendering.md).
The breakdown controls implementation order; v1.5 remains a detailed reference.

## Authoritative documents

| Document | Use |
| --- | --- |
| [GUIDANCE.md](../../GUIDANCE.md) | Understandability, maintainability, and engineering priorities |
| [CLAUDE.md](../../CLAUDE.md) | Shared agent instructions; root AGENTS.md links here |
| [MVP breakdown](../../plans/mvp/breakdown/README.md) | Active sequence, seven plans, paired cumulative diagrams, and iteration rules |
| `.state/` (untracked) | One append-only activity file per set of agent work; cleaned up after its knowledge is saved; see [CLAUDE.md](../../CLAUDE.md#working-state) |
| [Target architecture](../../plans/1-mvp/v1.5_architecture.md) | Full-platform responsibilities, connections, and major concerns |
| [Detailed reference](../../plans/1-mvp/v1.5.md) | Earlier contracts, suggested implementation details, and historical planning tracker |
| [TESTING.md](../../TESTING.md) | Direct requirements for MC/DC, simulation, real-effect validation, and acceptance evidence; planned command interfaces |
| [TEST_RESEARCH.md](../TEST_RESEARCH.md) | Simulation feasibility, boundaries, alternatives, and learning sequence |
| [TIGERSTYLE.md](../TIGERSTYLE.md) | Go coding conventions and invariant handling |

Earlier plan revisions preserve design history; they do not override the active plan.
Update this summary when decisions change, without duplicating detailed implementation contracts.
