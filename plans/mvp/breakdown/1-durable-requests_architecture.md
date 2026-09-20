# 1. Architecture after durable requests

Read the [slice plan](1-durable-requests.md) and [diagram conventions](README.md#reading-the-architecture-diagrams).
Shared terms live in [the simulator's place](README.md#the-simulators-place).
This cumulative target includes earlier slices; it does not claim implementation.

## New in this slice

- Nothing precedes this slice, so no `*` markers appear below.
- Terminal, `perpetual` CLI, `agent-plane`, and PostgreSQL appear together.
- The decision loop appears: events, decisions, effects, adapters, completions.
- The simulator appears: script, queue, simulated store, checks, and trace.

## Production at this point

The first diagram shows who connects to whom.
The second shows the decision loop inside `agent-plane`; arrows keep their numbers.

<!-- draw-visual: diagrams/architecture-topology-slice-1.mmd -->
```text
┌────────┐    ┌─────────────┐    ┌───────────┐    ┌──────────┐
│Terminal├─1─►│perpetual CLI├─2─►│agent-plane├─6─►│PostgreSQL│
└────────┘    └─────────────┘    └───────────┘    └──────────┘
```

<!-- draw-visual: diagrams/architecture-decision-loop.mmd -->
```text
┌────────────────┐
│     events     │◄───────┐
└────────┬───────┘        │
         │                │
         3                │
         ▼                │
┌────────────────┐        │
│shared decisions│  5 completions
└────────┬───────┘        │
         │                │
     4 effects            │
         ▼                │
┌────────────────┐        │
│ real adapters  ├────────┘
└────────────────┘
```

```text
 (1) Terminal runs perpetual commands; repeats one when no reply arrives
 (2) HTTP request/response: register intent, inspect record
 (3) requests and completions become events
 (4) decisions return effects and replies; they perform no I/O
 (5) adapters report committed, aborted, or unknown outcomes
 (6) bounded database transactions
```

| Actor | Runs in | Lifetime | Responsibilities at this point |
| --- | --- | --- | --- |
| Terminal | operator's shell | outside the platform | runs commands; from slice 2 also carries SSH sessions |
| `perpetual` CLI | host | short-lived process | formats requests, prints responses; from slice 2 launches the SSH client |
| `agent-plane` | host | long-lived service, one per host | registration, inspection; single-host service ownership |
| shared decisions | inside `agent-plane`; from slice 3 also inside `vmagent` | code without I/O | turn events into effects and replies |
| real adapters | inside `agent-plane` | code performing I/O | perform effects; report completions |
| PostgreSQL | host | independent service | machine intent, request ownership |

No guest exists yet; registration records intent rather than successful provisioning.
No Firecracker integration, running guest, or command execution exists yet.

## What survives which failure

Columns list failures this slice introduces; cells state planned guarantees.

| State | Reply lost on arrow (2) | `agent-plane` restarts | Database connection lost on arrow (6) |
| --- | --- | --- | --- |
| `agent-plane` memory | kept | lost | kept |
| PostgreSQL committed rows | kept | kept | kept |
| Transaction in flight | not affected | outcome unknown until inspected | outcome unknown until inspected |
| Request identifier held by the caller | kept; the retry reuses it | kept | kept |

## Simulator at this point

<!-- draw-visual: diagrams/architecture-simulator-slice-1.mmd -->
```text
┌────────────────┐
│Scenario script ├─────faults──────┐
└────────┬───────┘                 │
         │                         │
      events                       │
         ▼                         ▼
┌────────────────┐         ┌───────────────┐
│  Event queue   │◄───5────┤Simulated store│
└────────┬───────┘         └───────────────┘
         │                         ▲
         3                         │
         ▼                         │
┌────────────────┐                 │
│ SAME decisions ├────────4────────┘
└────────┬───────┘
         │
       state
         ▼
┌────────────────┐
│Invariant checks│
└────────┬───────┘
         ▼
┌────────────────┐
│ Trace recorder │
└────────────────┘
```

```text
 (3) (4) (5) match the production arrows of the same number
 adapters:
     stands in for (6): committed records kept apart
     from memory; loses replies and connections on cue
```

## Real and simulated boundaries

Each row names a production arrow, its substitute, and the validating real check.
Later slices add rows; these rows keep applying.

| Arrow | Real boundary | Simulated substitute | Faults the simulator injects | Real check that validates it |
| --- | --- | --- | --- | --- |
| (2) | HTTP reply path | Script withholds the reply | Reply lost after commit | Retry through the actual CLI and service |
| (6) | PostgreSQL adapter and transactions | Simulated store | Commit outcome unknown after connection loss | Concurrent real transactions compared with simulated reservation outcomes |
| none | `agent-plane` process memory | Discarded state, rebuilt by production reconstruction | Restart between any two events | Restart the real service, then inspect |

## Learning checkpoint

Follow a request from admission through persistence, response loss, and retry.
Trace walkthrough 1 of the [plan](1-durable-requests.md#walkthroughs) along arrows (1) through (6).
Explain walkthrough 2 by naming the arrow each fault cuts.
A real PostgreSQL test must verify the reservation contract independently.
