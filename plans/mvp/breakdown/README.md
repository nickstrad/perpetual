# MVP slices: platform and simulator together

Start here, then read the next unfinished slice.
The [project direction](../../../docs/knowledge/project-direction.md) entry records which slices are delivered.
A vertical slice delivers one useful behavior across its required components.
Each numbered plan pairs platform work with corresponding simulator growth.
Each matching architecture shows the cumulative target after that slice.
Each plan opens with its actors, platform and simulator, and their actions.
Walkthroughs then show those actors performing the slice's workflows and simulator runs.
These diagrams describe planned outcomes, not currently implemented software.

This breakdown replaces the giant plan as the implementation sequence.
The [v1.5 reference](../../1-mvp/v1.5.md) retains detailed contracts and historical reasoning.
Its old milestone numbering does not dictate the new order.
Keep its safety guarantees unless discussion explicitly changes them.
Resolve conflicts before implementation; never silently weaken recovery behavior.

## Reading map

| Slice | User-visible result | Simulator grows by | Cumulative architecture |
| --- | --- | --- | --- |
| [1: Durable requests](1-durable-requests.md) | Register and inspect machine intent across retries | Fixed events, durable reservations, restart, readable replay | [Diagram 1](1-durable-requests_architecture.md) |
| [2: Live machines](2-live-machines.md) | Boot, inspect, SSH, stop, and delete independent machines | Logical deadlines, lifecycle effects, partial cleanup | [Diagram 2](2-live-machines_architecture.md) |
| [3: Detached commands](3-detached-commands.md) | Execute, disconnect, and watch the same command later | Guest acceptance, launch uncertainty, output ordering | [Diagram 3](3-detached-commands_architecture.md) |
| [4: Working with commands](4-command-actions.md) | Transfer bounded files, send input, and cancel commands | Competing actions, deadlines, bounded seeded schedules | [Diagram 4](4-command-actions_architecture.md) |
| [5: Recovery across failures](5-recovery.md) | Recover after independent service failures without duplicating work | Independent crashes, stale observations, bounded healthy-machine progress | [Diagram 5](5-recovery_architecture.md) |
| [6: Optional streaming](6-streaming.md) | Stream existing commands with polling fallback | Transport changes, duplicate delivery, reconnect schedules | [Diagram 6](6-streaming_architecture.md) |
| [7: Snapshots and restoration](7-snapshots.md) | Save and restore stopped execution boundaries | Drain races, publication failures, restored identities | [Diagram 7](7-snapshots_architecture.md) |

Follow the numbered order; each slice builds on its predecessor.
Slice 5 broadens recovery testing; earlier slices already need safe failure handling.
We move SSH and basic watch earlier to make those slices independently useful.
We postpone detailed file maps and endpoint decisions until their slice needs them.

## The simulator's place

An event reports a request, completion, or expired deadline.
An effect requests external work, such as saving a reservation.
An adapter performs that work through a real component or controlled substitute.
An invariant states a rule that must remain true.
A trace records events, effects, faults, and outcomes for inspection and replay.

Production and simulation call identical decision code.
Production adapters contact PostgreSQL, processes, files, and guests.
Simulated adapters model selected contracts without running those components.
The simulator chooses event delivery; it does not control Go's runtime scheduler.

```text
 PRODUCTION                                  SIMULATION
 requests + real completions                 scripted/generated events
              |                                         |
              v                                         v
      shared decision code                     same decision code
              |                                         |
              v                                         v
       real adapters                           simulated adapters
              |                                         |
              +-- completion events            +-- controlled completions
```

These are separate executions, not a testing service inside production.
Use ordinary Go; no hosted testing platform or modified runtime is required.
Real integration tests must check assumptions that simulation cannot establish.
Start with fixed scenarios, then add randomness when competing behaviors justify it.
Do not build a general simulator before implementing its first production behavior.

## Reading the architecture diagrams

Every cumulative architecture file follows one layout and these conventions.
A change list comes first, so each slice's difference stays visible.
A trailing `*` marks anything new in that slice.
Numbered arrows keep their numbers across all seven files.
The arrow key beneath each diagram explains every number.
Walkthroughs, survival tables, and fault tables cite those same numbers.
The actor table gives each actor's placement and lifetime.
A service keeps running; a short-lived process exits after one command.
A detached process outlives its launcher; code runs inside another actor.
Boxes show responsibility boundaries, not additional deployed services.

These rules hold in every architecture file once their slice introduces them.
Test executions reuse production decisions, not a second copy of their rules.
Controlled adapters supply outcomes; real integration checks validate those adapter assumptions.
Actual VM tests verify process, networking, image, and lifecycle behavior separately.
The database and guest processes outlive control-plane connections.
From slice 3, the collector saves host output before advancing its database cursor.
Keep previous slice scenarios working as responsibilities expand.
Survival tables state planned guarantees, never observed results.

Sequence, topology, and loop diagrams are generated from [Mermaid sources](diagrams/README.md).
Never edit their rendered text; change the source and regenerate.
The `draw-visual` skill explains how; timelines and cut-point maps stay hand-written.

## How we iterate

1. Read the next slice and explain its diagram together.
2. Resolve its discussion questions and identify the smallest useful demonstration.
3. Expand only that slice into implementation tasks, contracts, and specific tests.
4. Build platform behavior and simulator coverage together.
5. Review real evidence, remaining limits, and the next learning step.

Track in-progress work in an untracked `.state/` file; [CLAUDE.md](../../../CLAUDE.md#working-state) explains.
Keep later slices mid-level until their dependencies become concrete.
Each slice needs a demonstration, meaningful fault, and repeatable test evidence.
A failing replay should explain the broken rule, not merely dump internal state.
No slice completes through simulated evidence alone when real effects matter.

## Ownership and boundaries

Astra owns all testing, simulator development, and [TESTING.md](../../../TESTING.md).
Astra also handles high-leverage contracts and difficult recovery decisions.
Sol integrates bounded workflows; Terra implements operating-system and persistence boundaries.
Luna high handles clear scaffolding and settled documentation tasks.
The lead reviews, independently validates, records delivered slices, and handles commits.
Assign exact files and tasks only when expanding the current slice.

Preserve PostgreSQL, one control plane per host, and independent guest execution.
The CLI uses request/response; only host-to-guest connections gain optional streaming.
SSH remains the interactive terminal path.
Security hardening, multiple control-plane hosts, and whole-system simulation remain outside this MVP.

Read [guidance](../../../GUIDANCE.md), [Go style](../../../docs/TIGERSTYLE.md), and [research](../../../docs/TEST_RESEARCH.md) when expanding work.

