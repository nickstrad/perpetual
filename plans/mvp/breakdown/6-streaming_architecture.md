# 6. Architecture after optional streaming

Read the [slice plan](6-streaming.md) and [diagram conventions](README.md#reading-the-architecture-diagrams).
Shared terms live in [the simulator's place](README.md#the-simulators-place).
This cumulative target includes earlier slices; it does not claim implementation.

## Changed since slice 5

- Only arrow (9) changes: configured HTTPS and an optional WebSocket.
- The collector gains stream attachment, automatic fallback, and explicit error reporting.
- `agent-plane` tracks upgraded connections during shutdown.
- Cursors become transport-neutral; nothing else in PostgreSQL changes.
- Arrows (1), (2), and (10) stay unchanged; the CLI never streams.
- The simulator gains transport switching, duplicates, drops, and delayed final status.

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
 (2) HTTP request/response only; watch still polls agent-plane
 (3) requests, completions, and expired deadlines become events
 (4) decisions return effects and replies; they perform no I/O
 (5) adapters report confirmed, failed, or unknown outcomes
 (6) database transactions, reconnection, inspection of unknown commits
 (7) disks, network devices, process evidence, collected output
 (8) process control and verification; Firecracker runs the VM
 (9) HTTP or configured HTTPS*; optional WS/WSS attached to an existing action*
 (10) SSH session; still the only interactive terminal path
   *  new in this slice
```

| Actor | Runs in | Lifetime | Responsibilities at this point |
| --- | --- | --- | --- |
| Terminal | operator's shell | outside the platform | runs commands; from slice 2 also carries SSH sessions |
| `perpetual` CLI | host | short-lived process | formats requests, prints responses; from slice 2 launches the SSH client |
| `agent-plane` | host | long-lived service, one per host | lifecycle, all actions, reconciler, database link, polling/stream collector*, upgraded-connection tracking* |
| shared decisions | inside `agent-plane`; from slice 3 also inside `vmagent` | code without I/O | turn events into effects and replies |
| real adapters | inside `agent-plane` | code performing I/O | perform effects; report completions |
| PostgreSQL | host | independent service | intent, identities, actions, transport-neutral cursors* |
| host files and network | host | filesystem and devices | disks, process evidence, collected output |
| Firecracker | host | detached process per VM | runs each VM |
| guest init | each VM | first guest process | starts services, reaps children |
| `vmagent` | each VM | guest service | HTTP/HTTPS*, optional WS/WSS*; stream path* attaches to existing actions; starts nothing; guest decisions drive real process and file effects; journal, numbered output |
| Command | each VM | child process group | runs regardless of callers |
| guest SSH server | each VM | guest service | interactive sessions |

Only `agent-plane` connects through WebSockets; CLI watch continues polling.

## What survives which failure

Columns list failures this slice introduces; cells state planned guarantees.
Earlier failure columns still apply; see the previous architecture file.

| State | Upgrade rejected on arrow (9) | Stream drops | `agent-plane` shuts down |
| --- | --- | --- | --- |
| WebSocket connection | never opens | lost; reattach or poll | closed deliberately; tracked |
| Output cursor | unchanged | kept; collection resumes from it | kept |
| Guest retained output | kept within retention | kept within retention | kept within retention |
| Input in flight | not affected | uncertain; never retransmitted automatically | uncertain; never retransmitted automatically |
| Command | keeps running | keeps running | keeps running |

## Simulator at this point

<!-- draw-visual: diagrams/architecture-simulator-slice-6.mmd -->
```text
┌───────────────────────────┐
│       Seeded chooser      ├──────faults────────┐
└─────────────┬─────────────┘                    │
              │                                  │
           events                                │
              ▼                                  ▼
┌───────────────────────────┐         ┌────────────────────┐
│   Queue + logical clock   │◄───5────┤Simulated transport*│
└─────────────┬─────────────┘         └────────────────────┘
              │                                  ▲
              3                                  │
              ▼                                  │
┌───────────────────────────┐                    │
│       SAME decisions      ├─────────4──────────┘
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
     joins the simulated world; stands in for (9):
     polling and stream delivery*, duplicates*, drops*,
     reconnections*, delayed final status*
   *  new in this slice
```

The harness models delivery only; it simulates no encryption or wire protocol.

## Real and simulated boundaries

Each row names a production arrow, its substitute, and the validating real check.
Rows list boundaries this slice adds; earlier rows still apply.

| Arrow | Real boundary | Simulated substitute | Faults the simulator injects | Real check that validates it |
| --- | --- | --- | --- | --- |
| (9) | WebSocket upgrade | Accepted or rejected attachment | Upgrade rejected | Real local servers performing upgrades |
| (9) | Stream delivery | Deliveries that duplicate or drop | Duplicate output; dropped connection; late final status | Real framing, interruption, and resumed collection |
| (9) | HTTPS and secure WebSockets | Not simulated | none | Real certificates and configured trust |
| none | Shutdown of upgraded connections | Not simulated | none | Real service shutdown with open streams |

## Learning checkpoint

Explain why changing a connection never changes the command's identity.
Trace walkthrough 1 of the [plan](6-streaming.md#walkthroughs); every step stays on arrow (9).
Explain walkthrough 2 using the survival table.
