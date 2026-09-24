# Project direction

Recorded: 2026-09-18. Status: accepted design direction; platform implementation remains pending.

The repository teaches understandable systems engineering through virtual-machine orchestration.
Callers request concrete actions; the platform does not choose their workflows.
`agent-plane` manages machine lifecycle, durable host state, and guest action routing.
`vmagent` runs inside each guest and exposes concrete actions through HTTP/HTTPS.
Optional WebSocket connections attach to existing actions without restarting execution.
SSH handles interactive access; accepted commands survive caller disconnection.

PostgreSQL is the state backend in the active design.
The control plane uses concurrent connections while preserving explicit admission ownership.
A planned custom simulator exercises shared production decisions using controlled events and faults.
The simulator requires no database; real integration tests require isolated PostgreSQL.
Testing direction updated 2026-09-22: use MC/DC as the coverage standard within basic behavior testing, targeted native Go fuzzing, and deterministic simulation.
Compose business logic from pure functions and apply the invariant, boundary, and resource rules in [TESTING.md](../../TESTING.md).
The assigned test owner maintains testing; the lead validates results, maintains the plan, and commits changes.

No simulator, PostgreSQL integration, or project-specific test targets exist yet.
The repository contains documentation and tooling only; no Go code exists yet.
The earlier Firecracker SDK boot prototype was removed on 2026-09-20.
Implementation starts from scratch, following the breakdown.
Slice status on 2026-09-20: seven slices planned at mid-level; none implemented or accepted.
Detailed planning updated 2026-09-23: slice 1 now has a [concrete implementation design](../plans/durable_requests/plan.md).
Its proposed snippets and acceptance checks have not been executed; all seven slices remain unimplemented.
The [planning workflow](planning-workflow.md) describes shared project skills and per-plan resumption.
Update this status whenever a slice's implementation or acceptance changes.
Seven vertical slices pair useful platform behavior with incremental simulator development.
Each slice includes a cumulative diagram and questions for later refinement.
Each slice plan lists its actors and walks through their planned workflows.
Diagrams render from Mermaid sources; see [text diagram rendering](text-diagram-rendering.md).
The breakdown controls implementation order; each slice retains its latest plan and visuals.

## Authoritative documents

| Document | Use |
| --- | --- |
| [GUIDANCE.md](../../GUIDANCE.md) | Understandability, maintainability, and engineering priorities |
| [CLAUDE.md](../../CLAUDE.md) | Shared agent instructions; root AGENTS.md links here |
| [MVP breakdown](../plans/README.md) | Active sequence, seven plans, paired cumulative diagrams, and iteration rules |
| `.state/` (untracked) | One append-only activity file per set of agent work; cleaned up after its knowledge is saved; see [CLAUDE.md](../../CLAUDE.md#working-state) |
| [Durable requests design](../plans/durable_requests/plan.md) | Current slice 1 contracts and acceptance requirements |
| [Durable requests architecture](../plans/durable_requests/architecture.md) | Slice 1 responsibilities, connections, and simulator boundaries |
| [TESTING.md](../../TESTING.md) | Direct requirements for MC/DC, simulation, real-effect validation, and acceptance evidence; planned command interfaces |
| [TEST_RESEARCH.md](../TEST_RESEARCH.md) | Simulation feasibility, boundaries, alternatives, and learning sequence |
| [TIGERSTYLE.md](../TIGERSTYLE.md) | Go coding conventions and invariant handling |

Superseded plan revisions are retained in Git history, not in the current documentation tree.
Update this summary when decisions change, without duplicating detailed implementation contracts.
