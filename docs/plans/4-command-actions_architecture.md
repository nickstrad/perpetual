# 4. Architecture after files, input, and cancellation

Read the [slice plan](4-command-actions.md) and [diagram conventions](README.md#reading-the-architecture-diagrams).
Shared terms live in [the simulator's place](README.md#the-simulators-place).
This cumulative target includes earlier slices; it does not claim implementation.

## Changed since slice 3

- Arrow (2) gains bounded file actions, input, end-of-input, cancellation, and timeouts.
- Arrow (9) carries those actions to `vmagent`, beside a running command.
- `vmagent` gains a file handler; its runner gains input, deadlines, group termination.
- PostgreSQL gains limits and uncertain input deliveries.
- Arrow (10) now competes: SSH sessions may edit files the journal misses.
- The simulator gains a seeded chooser, bounded schedules, and competing completion orders.

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
 (2) HTTP request/response: lifecycle, command, watch, file read/write*, input*,
     end-of-input*, cancellation*, optional timeout*
 (3) requests, completions, and expired deadlines become events
 (4) decisions return effects and replies; they perform no I/O
 (5) adapters report confirmed, failed, or unknown outcomes
 (6) bounded database transactions
 (7) disks, network devices, process evidence, collected output
 (8) launch, verify, and terminate detached processes; Firecracker runs the VM
 (9) HTTP request/response to vmagent: health, commands, files*, input*, cancel*
 (10) SSH session; manual edits bypass the journal*
   *  new in this slice
```

| Actor | Runs in | Lifetime | Responsibilities at this point |
| --- | --- | --- | --- |
| Terminal | operator's shell | outside the platform | runs commands; from slice 2 also carries SSH sessions |
| `perpetual` CLI | host | short-lived process | formats requests, prints responses; from slice 2 launches the SSH client |
| `agent-plane` | host | long-lived service, one per host | lifecycle, commands, polling collector, files*, input*, cancellation*, resource limits* |
| shared decisions | inside `agent-plane`; from slice 3 also inside `vmagent` | code without I/O | turn events into effects and replies |
| real adapters | inside `agent-plane` | code performing I/O | perform effects; report completions |
| PostgreSQL | host | independent service | machine/action records, limits*, uncertain deliveries*, output cursors |
| host files and network | host | filesystem and devices | disks, process evidence, collected output |
| Firecracker | host | detached process per VM | runs each VM |
| guest init | each VM | first guest process | starts services, reaps children |
| `vmagent` | each VM | guest service | runner, files*, input*, cancel*; guest decisions drive process, pipe*, and file effects; journal, numbered output |
| Command | each VM | child process group | reads standard input* |
| guest SSH server | each VM | guest service | may edit the same files* |

File actions stay available while a platform command runs.

## What survives which failure

Columns list failures this slice introduces; cells state planned guarantees.
Earlier failure columns still apply; see the previous architecture file.

| State | Input acknowledgment lost on arrow (9) | File replacement interrupted | Cancellation races natural exit |
| --- | --- | --- | --- |
| PostgreSQL action records | kept; input marked uncertain | kept; write outcome unresolved | kept; one terminal outcome |
| Guest journal | kept | may be incomplete; destination gets inspected | kept; first recorded outcome stands |
| Destination file | not affected | old or new contents; unseen SSH edits can prevent certainty | not affected |
| Input bytes | delivery unknown; never resent automatically | not affected | not affected |
| Command and its slot | keeps running; slot held | keeps running | ended; slot freed after verified termination |

## Simulator at this point

<!-- draw-visual: diagrams/architecture-simulator-slice-4.mmd -->
```text
┌────────────────┐
│Seeded chooser* ├──────faults───────┐
└────────┬───────┘                   │
         │                           │
      events                         │
         ▼                           ▼
┌────────────────┐         ┌──────────────────┐
│  Event queue   │◄───5────┤Simulated adapters│
└────────┬───────┘         └──────────────────┘
         │                           ▲
         3                           │
         ▼                           │
┌────────────────┐                   │
│ SAME decisions ├─────────4─────────┘
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
     stand in for (6) to (9): lifecycle, commands, files*,
     input*, cancellation*; competing completion orders*
     and publication gaps*
   *  new in this slice
```

Handwritten scripts remain; the chooser only orders valid pending events.
Generated schedules cover modeled events, not arbitrary Go goroutine execution.

## Real and simulated boundaries

Each row names a production arrow, its substitute, and the validating real check.
Rows list boundaries this slice adds; earlier rows still apply.

| Arrow | Real boundary | Simulated substitute | Faults the simulator injects | Real check that validates it |
| --- | --- | --- | --- | --- |
| (9), guest | Standard input pipe | Sequenced deliveries | Input acknowledgment lost | Actual pipes and end-of-input |
| guest | File staging, rename, and sync | Publication status | Gap between staging and publication | Actual rename and synchronization boundaries |
| guest | Process-group termination | Exit and cancel completions | Cancellation races natural exit | Actual process groups and inspected children |
| none | Execution deadlines | Logical clock | Deadline competes with exit | Real timeout that keeps terminal status |
| (10) | Manual SSH edits | Not simulated | none | Documented limit: recovery cannot exclude unseen edits |

## Learning checkpoint

Reproduce an ordering failure and identify the invariant that detects it.
Trace walkthrough 1 of the [plan](4-command-actions.md#walkthroughs) along arrows (2), (6), and (9).
Explain walkthrough 3 by naming which completion reached arrow (5) first.
