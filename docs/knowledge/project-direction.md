# Project direction

Recorded: 2026-09-18. Status: accepted design direction; platform implementation remains pending.

The repository teaches understandable systems engineering through virtual-machine orchestration.
Callers request concrete actions; the platform does not choose their workflows.
`agent-plane` manages machine lifecycle, durable host state, and guest action routing.
`vmagent` runs inside each guest and exposes concrete actions through HTTP/HTTPS.
Optional WebSocket connections attach to existing actions without restarting execution.
SSH handles interactive access; accepted commands survive caller disconnection.

PostgreSQL replaces SQLite in the active design, not historical plan revisions.
The control plane uses concurrent connections while preserving explicit admission ownership.
A planned custom simulator exercises shared production decisions using controlled events and faults.
The simulator requires no database; real integration tests require isolated PostgreSQL.
Testing direction updated 2026-09-22: use MC/DC as the coverage standard within basic behavior testing, targeted native Go fuzzing, and deterministic simulation.
Compose business logic from pure functions and apply the invariant, boundary, and resource rules in [TESTING.md](../../TESTING.md).
Astra owns testing; the lead validates results, maintains the plan, and commits changes.

No simulator, PostgreSQL integration, or project-specific test targets exist yet.
The repository contains documentation and tooling only; no Go code exists yet.
The earlier Firecracker SDK boot prototype was removed on 2026-09-20, uncommitted.
Implementation starts from scratch, following the breakdown.
Slice status on 2026-09-20: seven slices planned at mid-level; none implemented or accepted.
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
