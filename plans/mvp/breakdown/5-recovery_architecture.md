# 5. Architecture after recovery across failures

Read the [slice plan](5-recovery.md) and [diagram conventions](README.md#reading-the-architecture-diagrams).
Shared terms live in [the simulator's place](README.md#the-simulators-place).
This cumulative target includes earlier slices; it does not claim implementation.

## Changed since slice 4

- `agent-plane` gains a reconciler and a database link blocking mutations during loss.
- No arrow appears; every existing arrow gains a recovery path.
- Arrow (6) gains bounded reconnection and inspection of unknown commits.
- Arrows (8) and (9) gain identity checks before any further work.
- `vmagent` restart takes a fresh identity and marks unfinished actions uncertain.
- The simulator gains independent crashes, stale observations, and bounded progress checks.

## Production at this point

The first diagram shows who connects to whom.
The second shows the decision loop inside `agent-plane`; arrows keep their numbers.

<!-- draw-visual: diagrams/architecture-topology-slice-4-to-7.mmd -->
```text
┌────────┐     ┌────────────────┐    ┌───────────┐    ┌────────────────────┐           ┌───────┐
│Terminal├──1─►│ perpetual CLI  ├─2─►│agent-plane├─9─►│      vmagent       ├─launches─►│Command│
└────┬───┘     └────────────────┘    └─────┬─────┘    └────────────────────┘           └───────┘
     │                                     │                     ▲
     │                                     │                  runs VM
     │                                     │                     │
     │         ┌────────────────┐          │          ┌──────────┴─────────┐
     └─────10─►│guest SSH server│          ├───────8─►│    Firecracker     │
               └────────────────┘          │          └────────────────────┘
                                           │
                                           │          ┌────────────────────┐
                                           ├───────7─►│host files + network│
                                           │          └────────────────────┘
                                           │
                                           │          ┌────────────────────┐
                                           └───────6─►│     PostgreSQL     │
                                                      └────────────────────┘
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
 (1) Terminal runs perpetual commands
 (2) HTTP request/response: all actions; inspection reports stale and unresolved work*
 (3) requests, completions, and expired deadlines become events
 (4) decisions return effects and replies; they perform no I/O
 (5) adapters report confirmed, failed, or unknown outcomes
 (6) database transactions; reconnect within bounds*; inspect unknown commits*
 (7) disks, network devices, process evidence, collected output
 (8) process control; verify surviving processes against saved identities*
 (9) HTTP to vmagent: all actions; compare guest identities*; inspect existing identifiers*
 (10) SSH session
   *  new in this slice
```

| Actor | Runs in | Lifetime | Responsibilities at this point |
| --- | --- | --- | --- |
| Terminal | operator's shell | outside the platform | runs commands; from slice 2 also carries SSH sessions |
| `perpetual` CLI | host | short-lived process | formats requests, prints responses; from slice 2 launches the SSH client |
| `agent-plane` | host | long-lived service, one per host | lifecycle, all actions, polling collector, reconciler*, database link* |
| shared decisions | inside `agent-plane`; from slice 3 also inside `vmagent` | code without I/O | turn events into effects and replies |
| real adapters | inside `agent-plane` | code performing I/O | perform effects; report completions |
| PostgreSQL | host | independent service | durable intent, action evidence*, identities, cursors, stale observation times* |
| host files and network | host | filesystem and devices | disks, process evidence, collected output |
| Firecracker | host | detached process per VM | runs each VM |
| guest init | each VM | first guest process | starts services, reaps children |
| `vmagent` | each VM | guest service | actions, own restart recovery*; guest decisions drive real process and file effects; journal, numbered output |
| Command | each VM | child process group | runs regardless of callers |
| guest SSH server | each VM | guest service | interactive sessions |

Recovery expands across components; earlier slices already required basic crash safety.

## What survives which failure

Columns list failures this slice introduces; cells state planned guarantees.
Earlier failure columns still apply; see the previous architecture file.

| State | `agent-plane` restarts | PostgreSQL restarts | `vmagent` restarts | Machine lost |
| --- | --- | --- | --- | --- |
| `agent-plane` memory | lost; rebuilt by reconciliation | kept; durable mutations blocked | kept | kept |
| PostgreSQL committed rows | kept | kept; in-flight commits unknown until inspected | kept | kept; its actions become interrupted |
| Host files | kept | kept | kept | kept |
| Firecracker process and guest | keeps running | keeps running | keeps running | lost; never inferred from silence alone |
| `vmagent` memory | kept | kept | lost; fresh guest-process identity | lost |
| Guest journal and retained output | kept | kept; output stays bounded while collection waits | kept; unfinished actions become uncertain | kept on the machine disk |
| Command | keeps running | keeps running | uncertain; never rerun from the journal | ended |
| SSH session | keeps running | keeps running | keeps running | ended |

## Simulator at this point

<!-- draw-visual: diagrams/architecture-simulator-slice-5.mmd -->
```text
┌────────────────────────────┐
│       Seeded chooser       ├─────faults───────┐
└──────────────┬─────────────┘                  │
               │                                │
            events                              │
               ▼                                ▼
┌────────────────────────────┐         ┌────────────────┐
│   Queue + logical clock    │◄───5────┤Simulated world*│
└──────────────┬─────────────┘         └────────────────┘
               │                                ▲
               3                                │
               ▼                                │
┌────────────────────────────┐                  │
│       SAME decisions       ├────────4─────────┘
└──────────────┬─────────────┘
               │
             state
               ▼
┌────────────────────────────┐
│Invariant + progress checks*│
└──────────────┬─────────────┘
               ▼
┌────────────────────────────┐
│       Trace recorder       │
└────────────────────────────┘
```

```text
 (3) (4) (5) match the production arrows of the same number
 adapters:
     stands in for (6) to (9): host*, guest*, and database*
     failures; stale observations*; durable storage kept
     apart from each volatile memory*
   *  new in this slice
```

Invariants run during faults; the progress check bounds healthy-machine outcomes.

## Real and simulated boundaries

Each row names a production arrow, its substitute, and the validating real check.
Rows list boundaries this slice adds; earlier rows still apply.

| Arrow | Real boundary | Simulated substitute | Faults the simulator injects | Real check that validates it |
| --- | --- | --- | --- | --- |
| none | `agent-plane` process | Crash and production rebuild | Crash between any two events | Restart the real service while PostgreSQL and guests run |
| (6) | PostgreSQL service | Store that restarts with unknown commits | Database restart; commit outcome unknown | Restart an exclusively owned test database; verify reconnection |
| (9) | `vmagent` process | Guest restart with a fresh identity | Guest-agent restart; stale boot observation | Restart real `vmagent`; inspect unfinished commands |
| (9) | Guest reachability | Machine a silent; fair delivery for machine b | One guest unreachable indefinitely | Two real machines with one unreachable |

## Learning checkpoint

Distinguish uncertainty from failure while unaffected machines continue making progress.
Trace walkthrough 1 of the [plan](5-recovery.md#walkthroughs) along arrows (6) and (9).
Use the survival table to explain walkthroughs 2 and 3.
