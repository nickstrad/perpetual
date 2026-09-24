---
name: detailed-design
description: Write fully developed implementation designs for perpetual, with substantial code sketches, TigerStyle contracts, model assignments, TDD handoffs, independent review, and resumable plan state. Also resume or execute an existing plan when requested.
---

# Detailed design

Make the design concrete enough that an implementation agent can work without inventing behavior, concurrency rules, or acceptance criteria. Prefer understandable engineering to machinery added merely to support a planning process.

## Choose the requested phase

- **Design or revise a plan:** write the design, code sketches, test specifications, assignments, and state. Do not implement application code, create executable test suites, compile snippets, run application tests, start services, or provision fixtures. Document checks, source inspection, and skill validation are appropriate. Label every code sketch **proposed, not executed**.
- **Implement an approved/requested plan:** follow [execution.md](references/execution.md). Design permission alone does not authorize implementation. Existing implementation authorization remains valid across context changes; do not repeatedly request it.
- **Resume:** read the plan and its `plan_state.md`, inspect actual work and live workers, then continue the authorized phase. Do not trust an old task status without reconciling it with the files and evidence.

## Ground the work

Locate the repository root. Read `docs/knowledge/index.md`, relevant knowledge entries, `GUIDANCE.md`, `TESTING.md`, `docs/TIGERSTYLE.md`, the active breakdown, and the current slice. Read `docs/TEST_RESEARCH.md` for coordination or simulated boundaries. Use `plans/1-mvp/v1.5.md` as contract reference, not the work sequence. Inspect existing code, instructions, and state before proposing replacements.

Expand the current slice only. Explain its boundary and acceptance evidence before writing the details. Resolve routine choices and state their rationale. Ask only about material unresolved behavior; while waiting, develop independent portions. Distinguish a proposed choice from an accepted requirement. A request to produce a design authorizes documenting proposed resolutions, not pretending the user already approved them.

General repository guides use roles: design owner, test owner, production owner, reviewer, lead. Keep model policy here and explicit model assignments in each generated plan. Do not spread model ownership rules into `AGENTS.md`, `CLAUDE.md`, `GUIDANCE.md`, `TESTING.md`, style guides, research, or knowledge notes.

## Write the design

Read [design-doc.md](references/design-doc.md) and cover its required substance. Use the requested filename; otherwise use `<slice>-plan.md` beside its source slice. Include substantial, coherent code blocks for the hard paths, not just a signature list. State each snippet's intended file, dependencies, assumptions, omitted helpers, and errors. Keep simple pieces brief; spend detail on ownership, bounds, state transitions, recovery, SQL ordering, and testable contracts.

TigerStyle is a design constraint from the beginning. Explain who owns mutable state, why each invariant holds, what stops on invariant failure, and how expected errors recover. Pass observations, identifiers, and time into pure decisions. Place I/O and goroutines in adapters with explicit lifetimes. Preserve uncertainty, bound work, and specify overflow and resource cleanup. Treat `docs/TIGERSTYLE.md` as authoritative; do not invent assertion quotas or arbitrary function limits.

Design TDD before delegating production: observable contracts, independence pairs for MC/DC, faults, independent expected outcomes, real boundary checks, and intended failure messages. The test owner may create narrow interfaces, shared types, and deliberately failing skeletons during implementation. Tests must fail for missing behavior after scaffolding compiles; missing dependencies or syntax errors are setup failures, not useful red evidence.

## Assign models in the plan

Use the user's preferred routing below. These are preferences, not claims that every host exposes these model names.

| Work | Preferred model | Alternative when working in the other environment |
| --- | --- | --- |
| Design, difficult contracts, ambiguous recovery, TDD suites and simulation | Astra | Fable; Opus if Fable is unavailable |
| Bounded production implementation with settled contracts | Sol | Opus |
| Straightforward scaffolding or documentation with little design judgment | Luna, high effort | Sonnet, high effort |
| Independent review of design, tests, implementation, and evidence | Fresh Astra reviewer | Fresh Fable or Opus reviewer |

Use high reasoning effort for substantive design, test design, implementation, and review when supported, unless the user selects another setting. Never silently lower the requested Luna/Sonnet high setting. Escalate uncertain concurrency or recovery choices to the design/test tier; a short file does not imply an easy task.

Every plan lists a chosen primary, environment alternative, effort, responsibility, file ownership, prerequisites, and completion evidence for each task. Before dispatch, resolve the chosen name to an actually available model ID and record it in state. Do not invent a Fable API ID or treat a role label as a running agent. If the preferred model is unavailable, use a listed available alternative when authorized and disclose it; ask only when no suitable authorized assignment exists. Never claim cross-provider delegation happened without a real tool/run.

The test owner authors and maintains test suites, simulator, fixtures, and `TESTING.md`. Implementers make production pass those contracts; contract changes go through the test/design owner and lead. A distinct reviewer challenges both tests and production. The lead integrates, validates evidence, updates plans and knowledge, and handles authorized commits.

## Keep work resumable

Every generated plan links its unique repository-root `.state/<plan-slug>/plan_state.md`. Initialize it from [the state template](assets/plan_state.md). This is the lead-owned plan ledger; individual workers also keep their own `.state/<work-slug>.md` as repository instructions require. State is untracked and never committed. Do not put multiple plans into one shared `plan_state.md`.

The ledger tracks **all** active design, testing, implementation, review, and integration work. Give tasks stable IDs. Append timestamped snapshots and events; never rewrite earlier entries. The latest snapshot is authoritative. Record the actual model/run, owned files, branch/worktree, dependencies, red/green/review evidence, pending commands/processes, blockers, and exact next action. Record authorization and design decisions so changing tools does not lose them. Never store secrets.

Checkpoint before delegation or starting a long command, after each contract/status/evidence change, after a worker response, and before clearing context or yielding. Only the lead edits the shared ledger; workers report and append to their own logs. On resume, reconcile live workers before reassigning ownership. If state is absent on another checkout, reconstruct it from plans, diffs and durable evidence; mark uncertainty rather than inventing execution history.

## Finish the requested phase

For design work, verify links, internal consistency, model routing, task dependencies, snippet/prose agreement, and that each acceptance item has an owner and evidence target. Do not execute the snippets as a shortcut to design confidence. Record checks actually performed, unresolved decisions, and the next authorized step. A completed design is not an implemented slice.

For implementation work, follow the review and acceptance gates in [execution.md](references/execution.md). Do not report planned commands as passing or mark a slice delivered on an agent's assertion alone.
