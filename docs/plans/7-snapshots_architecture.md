# 7. Architecture after snapshots and restoration

Read the [slice plan](7-snapshots.md) and [diagram conventions](README.md#reading-the-architecture-diagrams).
Shared terms live in [the simulator's place](README.md#the-simulators-place).
This cumulative target includes earlier slices; it does not claim implementation.

## Changed since slice 6

- `agent-plane` gains snapshot and restore workflows.
- Arrow (2) gains snapshot and restore requests.
- Arrow (7) gains snapshot staging and publication.
- Arrow (8) gains pause, capture, and start from saved state.
- Arrow (9) gains drain and identity bootstrap.
- PostgreSQL gains snapshot records; its history stays outside every snapshot.
- The simulator gains drain races, publication failures, and old boot observations.

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
 (1) Terminal runs perpetual commands; finishes SSH work before snapshotting*
 (2) HTTP request/response: all actions, snapshot*, restore*
 (3) requests, completions, and expired deadlines become events
 (4) decisions return effects and replies; they perform no I/O
 (5) adapters report confirmed, failed, or unknown outcomes
 (6) database transactions, reconnection, inspection of unknown commits
 (7) disks, evidence, output; stage and publish snapshots*; restore disks*
 (8) process control; pause*, capture memory and device state*, start from saved state*
 (9) HTTP/HTTPS, optional WS/WSS: all actions, drain*, bootstrap a fresh boot identity*
 (10) SSH session; invisible to draining*
   *  new in this slice
```

| Actor | Runs in | Lifetime | Responsibilities at this point |
| --- | --- | --- | --- |
| Terminal | operator's shell | outside the platform | runs commands; from slice 2 also carries SSH sessions |
| `perpetual` CLI | host | short-lived process | formats requests, prints responses; from slice 2 launches the SSH client |
| `agent-plane` | host | long-lived service, one per host | lifecycle, all actions, recovery, collection, snapshot workflow*, restore workflow* |
| shared decisions | inside `agent-plane`; from slice 3 also inside `vmagent` | code without I/O | turn events into effects and replies |
| real adapters | inside `agent-plane` | code performing I/O | perform effects; report completions |
| PostgreSQL | host | independent service | intent, identities, actions, cursors, snapshot records* |
| host files and network | host | filesystem and devices | disks, evidence, output, snapshot staging*, published snapshots* |
| Firecracker | host | detached process per VM | pause, capture* |
| guest init | each VM | first guest process | starts services, reaps children |
| `vmagent` | each VM | guest service | actions, drain*, bootstrap*; guest decisions drive real process and file effects; journal, numbered output |
| Command | each VM | child process group | runs regardless of callers |
| guest SSH server | each VM | guest service | interactive sessions |

PostgreSQL history stays outside snapshots; restoration reuses the original machine.
Restore changes boot identity and reconnects; snapshots never rewind host history.

## What survives which failure

Columns list failures this slice introduces; cells state planned guarantees.
Earlier failure columns still apply; see the previous architecture file.

| State | Drain deadline expires | Crash before publication on arrow (7) | Termination confirmation lost on arrow (8) | Restoration fails |
| --- | --- | --- | --- | --- |
| PostgreSQL rows and host history | kept | kept | kept | kept; never rewound |
| Snapshot staging | none yet | kept as incomplete; never restorable | not affected | not affected |
| Published snapshot | none | none | kept; complete | kept |
| Machine process | keeps running; obligations recorded | paused; inspected after restart | unknown until inspected | stays stopped |
| Original disk | kept | kept | kept | must be kept; method is an open question |
| Boot identity | unchanged | unchanged | unchanged | no new identity published |

## Simulator at this point

<!-- draw-visual: diagrams/architecture-simulator-slice-7.mmd -->
```text
┌───────────────────────────┐
│       Seeded chooser      ├─────faults──────┐
└─────────────┬─────────────┘                 │
              │                               │
           events                             │
              ▼                               ▼
┌───────────────────────────┐         ┌───────────────┐
│   Queue + logical clock   │◄───5────┤Simulated world│
└─────────────┬─────────────┘         └───────────────┘
              │                               ▲
              3                               │
              ▼                               │
┌───────────────────────────┐                 │
│       SAME decisions      ├────────4────────┘
└─────────────┬─────────────┘
              │
            state
              ▼
┌───────────────────────────┐
│Invariant + progress checks│
└─────────────┬─────────────┘
              ▼
┌───────────────────────────┐
│       Trace recorder      │
└───────────────────────────┘
```

```text
 (3) (4) (5) match the production arrows of the same number
 adapters:
     stands in for (6) to (9): all prior effects, artifact
     status*, publication and restore failures*,
     old boot observations*
   *  new in this slice
```

The simulator models artifact status, not memory images or Firecracker internals.

## Real and simulated boundaries

Each row names a production arrow, its substitute, and the validating real check.
Rows list boundaries this slice adds; earlier rows still apply.

| Arrow | Real boundary | Simulated substitute | Faults the simulator injects | Real check that validates it |
| --- | --- | --- | --- | --- |
| (9) | Guest drain | Drain completions | New request races drain admission | Real drain with running commands and writes |
| (8) | Firecracker pause, capture, and restore | Artifact status only | Partial artifact; restoration fails | Actual snapshot and restoration on real guests |
| (7) | Snapshot publication | Staging versus published status | Crash before publication | Interrupt real publication; reject incomplete sets |
| (8) | Termination confirmation | Separate completion event | Confirmation lost | Real termination verified separately |
| (9) | Bootstrap and boot identity | Reports tagged with an old identity | Old boot observation after restore | Restore, then verify fresh identity and preserved files |

## Learning checkpoint

Explain snapshot publication, termination, and restored identity as separate confirmations.
Trace walkthrough 1 of the [plan](7-snapshots.md#walkthroughs) along arrows (9), (8), (7), and (6).
Explain walkthrough 3 using the survival table.
