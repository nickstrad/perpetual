# 3. Architecture after detached commands

Read the [slice plan](3-detached-commands.md) and [diagram conventions](README.md#reading-the-architecture-diagrams).
Shared terms live in [the simulator's place](README.md#the-simulators-place).
This cumulative target includes earlier slices; it does not claim implementation.

## Changed since slice 2

- `agent-plane` gains command dispatch and a polling collector.
- Arrow (2) gains commands, action inspection, and polling watch.
- Arrow (9) gains dispatch, inspection by identifier, and output polling.
- `vmagent` gains guest decisions, a command runner, a journal, and numbered output.
- PostgreSQL gains action ownership and output cursors.
- Host files gain collected output, saved through arrow (7).
- The simulator drives guest decisions and holds pending external completions.

## Production at this point

The first diagram shows who connects to whom.
The second shows the decision loop inside `agent-plane`; arrows keep their numbers.

<!-- draw-visual: diagrams/architecture-topology-slice-3.mmd -->
```text
┌────────┐     ┌────────────────┐    ┌───────────┐    ┌────────────────────┐           ┌────────┐
│Terminal├──1─►│ perpetual CLI  ├─2─►│agent-plane├─9─►│      vmagent       ├─launches─►│Command*│
└────┬───┘     └────────────────┘    └─────┬─────┘    └────────────────────┘           └────────┘
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
 (1) Terminal runs perpetual commands; may disconnect at any time*
 (2) HTTP request/response: lifecycle, command*, action inspection*, polling watch*
 (3) requests, completions, and expired deadlines become events
 (4) decisions return effects and replies; they perform no I/O
 (5) adapters report confirmed, failed, or unknown outcomes
 (6) bounded database transactions
 (7) disks, network devices, process evidence; save collected output*
 (8) launch, verify, and terminate detached processes; Firecracker runs the VM
 (9) HTTP request/response to vmagent: health, dispatch*, inspect*, poll output*
 (10) SSH session; still the only interactive terminal path
   *  new in this slice
```

| Actor | Runs in | Lifetime | Responsibilities at this point |
| --- | --- | --- | --- |
| Terminal | operator's shell | outside the platform | runs commands; from slice 2 also carries SSH sessions |
| `perpetual` CLI | host | short-lived process | formats requests, prints responses; from slice 2 launches the SSH client |
| `agent-plane` | host | long-lived service, one per host | lifecycle, health, SSH discovery, command dispatch*, polling collector* |
| shared decisions | inside `agent-plane`; from slice 3 also inside `vmagent` | code without I/O | turn events into effects and replies |
| real adapters | inside `agent-plane` | code performing I/O | perform effects; report completions |
| PostgreSQL | host | independent service | machine records, action ownership*, output cursors* |
| host files and network | host | filesystem and devices | disks, process evidence, collected output* |
| Firecracker | host | detached process per VM | runs each VM |
| guest init | each VM | first guest process | starts services, reaps children |
| `vmagent` | each VM | guest service | health, command runner*, retries*; guest decisions* drive real process and file effects; journal*, numbered output* |
| Command* | each VM | child process group | runs regardless of callers |
| guest SSH server | each VM | guest service | interactive sessions |

Polling carries all platform output; interactive terminals still use SSH.
The collector saves host output before advancing its database cursor.

## What survives which failure

Columns list failures this slice introduces; cells state planned guarantees.
Earlier failure columns still apply; see the previous architecture file.

| State | Terminal disconnects | Acceptance acknowledgment lost on arrow (9) | `agent-plane` restarts mid-command |
| --- | --- | --- | --- |
| `agent-plane` memory | kept | kept | lost |
| Action record and command slot | kept | kept as uncertain | kept |
| Output cursor and host output files | kept | kept | kept; collection resumes from the cursor |
| Guest journal | kept | kept; answers inspection | kept |
| Guest retained output | kept within retention | kept within retention | older output may become a reported gap |
| Command | keeps running | keeps running | keeps running |

## Simulator at this point

<!-- draw-visual: diagrams/architecture-simulator-slice-3.mmd -->
```text
┌────────────────────────────┐
│      Scenario script       ├──────faults───────┐
└──────────────┬─────────────┘                   │
               │                                 │
            events                               │
               ▼                                 ▼
┌────────────────────────────┐         ┌──────────────────┐
│        Event queue         │◄───5────┤Simulated adapters│
└──────────────┬─────────────┘         └──────────────────┘
               │                                 ▲
               3                                 │
               ▼                                 │
┌────────────────────────────┐                   │
│SAME host + guest* decisions├─────────4─────────┘
└──────────────┬─────────────┘
               │
             state
               ▼
┌────────────────────────────┐
│      Invariant checks      │
└──────────────┬─────────────┘
               ▼
┌────────────────────────────┐
│       Trace recorder       │
└────────────────────────────┘
```

```text
 (3) (4) (5) match the production arrows of the same number
 adapters:
     stand in for (6) to (9): lifecycle, guest acceptance*,
     journal*, launch*, and output*; completions can wait
     while the control plane is unavailable*
   *  new in this slice
```

The model cannot establish real process creation or journal durability.

## Real and simulated boundaries

Each row names a production arrow, its substitute, and the validating real check.
Rows list boundaries this slice adds; earlier rows still apply.

| Arrow | Real boundary | Simulated substitute | Faults the simulator injects | Real check that validates it |
| --- | --- | --- | --- | --- |
| (9) | Dispatch and acceptance over guest HTTP | Simulated guest acceptance | Acceptance acknowledgment lost | Lose a real acknowledgment; inspect the original identifier |
| guest | Journal files | Journal that always survives | none; durability is assumed | Real files with controlled crashes |
| guest | Child process launch | Launch results and gaps | Launch evidence missing | Actual children started by `vmagent` |
| (9), (7) | Output collection and host output files | Numbered deliveries and a cursor | Delayed output; interrupted collection; retention gaps | Interrupt real collection; recover order or reported gaps |
| none | `agent-plane` availability | Scripted outage | Guest finishes while the control plane is down | Restart the real control plane during a command |

## Learning checkpoint

Trace one command beyond its caller and control-plane connection lifetimes.
Trace walkthrough 1 of the [plan](3-detached-commands.md#walkthroughs) along arrows (2), (6), (9), and (7).
Explain walkthrough 2 by naming the arrow that lost its acknowledgment.
