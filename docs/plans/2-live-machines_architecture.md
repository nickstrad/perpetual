# 2. Architecture after live machines

Read the [slice plan](2-live-machines.md) and [diagram conventions](README.md#reading-the-architecture-diagrams).
Shared terms live in [the simulator's place](README.md#the-simulators-place).
This cumulative target includes earlier slices; it does not claim implementation.

## Changed since slice 1

- `agent-plane` gains lifecycle operations, health polling, and SSH discovery.
- Adapters gain arrows (7), (8), and (9): host resources, Firecracker, guest HTTP.
- Each VM appears with guest init, `vmagent` health, and an SSH server.
- Arrow (10) appears: the terminal reaches the guest SSH server directly.
- PostgreSQL additionally keeps allocations, boot identities, and process identities.
- Expired deadlines join requests and completions as events on arrow (3).
- The simulator gains a logical clock, deadlines, and a simulated host.

## Production at this point

The first diagram shows who connects to whom.
The second shows the decision loop inside `agent-plane`; arrows keep their numbers.

<!-- draw-visual: diagrams/architecture-topology-slice-2.mmd -->
```text
┌────────┐     ┌─────────────────┐    ┌───────────┐    ┌─────────────────────┐
│Terminal├──1─►│  perpetual CLI  ├─2─►│agent-plane├─9─►│       vmagent*      │
└────┬───┘     └─────────────────┘    └─────┬─────┘    └─────────────────────┘
     │                                      │                     ▲
     │                                      │                  runs VM
     │                                      │                     │
     │         ┌─────────────────┐          │          ┌──────────┴──────────┐
     └─────10─►│guest SSH server*│          ├───────8─►│     Firecracker*    │
               └─────────────────┘          │          └─────────────────────┘
                                            │
                                            │          ┌─────────────────────┐
                                            ├───────7─►│host files + network*│
                                            │          └─────────────────────┘
                                            │
                                            │          ┌─────────────────────┐
                                            └───────6─►│      PostgreSQL     │
                                                       └─────────────────────┘
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
 (2) HTTP request/response: register, inspect, start*, stop*, delete*, SSH details*
 (3) requests, completions, and expired deadlines* become events
 (4) decisions return effects and replies; they perform no I/O
 (5) adapters report confirmed, failed, or unknown outcomes
 (6) bounded database transactions
 (7) prepare and remove disks, network devices, and process evidence*
 (8) launch, verify, and terminate detached processes*; Firecracker runs the VM*
 (9) HTTP request/response to vmagent: health*
 (10) SSH session from the terminal's SSH client, through host networking*;
      agent-plane supplies the address and relays nothing
   *  new in this slice
```

| Actor | Runs in | Lifetime | Responsibilities at this point |
| --- | --- | --- | --- |
| Terminal | operator's shell | outside the platform | runs commands; from slice 2 also carries SSH sessions |
| `perpetual` CLI | host | short-lived process | formats requests, prints responses; from slice 2 launches the SSH client |
| `agent-plane` | host | long-lived service, one per host | registration, lifecycle*, health polling*, SSH discovery* |
| shared decisions | inside `agent-plane`; from slice 3 also inside `vmagent` | code without I/O | turn events into effects and replies |
| real adapters | inside `agent-plane` | code performing I/O | perform effects; report completions |
| PostgreSQL | host | independent service | intent, allocations*, boot/process identities* |
| host files and network* | host | filesystem and devices | disks, network devices, process evidence |
| Firecracker* | host | detached process per VM | runs each VM |
| guest init* | each VM | first guest process | starts services, reaps children |
| `vmagent`* | each VM | guest service | health tagged with boot identity; no platform command runner yet |
| guest SSH server* | each VM | guest service | interactive sessions |

Guest health and SSH exist; platform command execution does not.

## What survives which failure

Columns list failures this slice introduces; cells state planned guarantees.
Earlier failure columns still apply; see the previous architecture file.

| State | `agent-plane` restarts | Launch acknowledgment lost on arrow (8) | Guest unreachable on arrow (9) |
| --- | --- | --- | --- |
| `agent-plane` memory | lost | kept | kept |
| PostgreSQL committed rows | kept | kept; process owner still unrecorded | kept; never rewritten to stopped |
| Host files and network allocation | kept | kept; released only after verified ownership | kept |
| Firecracker process and guest | keeps running | keeps running; adopted after inspection | unknown; never assumed stopped |
| SSH session on arrow (10) | keeps running | not affected | unknown |

## Simulator at this point

<!-- draw-visual: diagrams/architecture-simulator-slice-2.mmd -->
```text
┌──────────────────────┐
│   Scenario script    ├─────faults──────┐
└───────────┬──────────┘                 │
            │                            │
         events                          │
            ▼                            ▼
┌──────────────────────┐         ┌───────────────┐
│Queue + logical clock*│◄───5────┤Simulated host*│
└───────────┬──────────┘         └───────────────┘
            │                            ▲
            3                            │
            ▼                            │
┌──────────────────────┐                 │
│    SAME decisions    ├────────4────────┘
└───────────┬──────────┘
            │
          state
            ▼
┌──────────────────────┐
│   Invariant checks   │
└───────────┬──────────┘
            ▼
┌──────────────────────┐
│    Trace recorder    │
└──────────────────────┘
```

```text
 (3) (4) (5) match the production arrows of the same number
 adapters:
     stands in for (6) to (9): reservations, process*,
     network*, cleanup*, and health* results;
     a launch may finish after a crash*
   *  new in this slice
```

The simulator models documented results; it runs no CPU, kernel, or packet routing.

## Real and simulated boundaries

Each row names a production arrow, its substitute, and the validating real check.
Rows list boundaries this slice adds; earlier rows still apply.

| Arrow | Real boundary | Simulated substitute | Faults the simulator injects | Real check that validates it |
| --- | --- | --- | --- | --- |
| (7) | Disk, network, and cleanup steps | Simulated host results | Network setup fails; cleanup interrupted at named boundaries | Real partial-boot and cleanup runs on the host |
| (8) | Firecracker launch and process ownership | Launch that can outlive a crash | Launch completes after `agent-plane` crashes; acknowledgment lost | Restart the real control plane while two guests run |
| (9) | Guest health over HTTP | Scripted health reports | Health delayed past the boot deadline | Boot real guests from the image; verify health |
| none | Wall-clock deadlines | Logical clock | Deadline fires only when the script advances time | Real boot timeout against a real guest |
| (10) | SSH | Not simulated | none | Real SSH sessions to two guests |

## Learning checkpoint

Locate each resource owner and explain an interrupted boot without guessing.
Trace walkthrough 1 of the [plan](2-live-machines.md#walkthroughs) along arrows (1) through (10).
Explain walkthrough 3 by naming which arrow lost its acknowledgment.
