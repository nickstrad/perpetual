# 2. Live machines and lifecycle simulation

Requires [slice 1](1-durable-requests.md).
See the [cumulative architecture](2-live-machines_architecture.md).

## Actors and actions

An actor is a module or entity that performs actions within this slice.
Actors from [slice 1](1-durable-requests.md#actors-and-actions) remain; this table lists their new actions.

| Platform actor | Where it lives | Actions in this slice |
| --- | --- | --- |
| Terminal | Operator's shell | Runs `perpetual`; carries the interactive SSH session |
| `perpetual` CLI | Host, short-lived process | Sends start, inspect, stop, delete; fetches SSH details; launches the local SSH client |
| `agent-plane` | Host, single long-lived service | Owns each lifecycle operation; prepares disks and networks; launches and verifies processes; polls health |
| Lifecycle decisions | Shared code inside `agent-plane` | Choose the next lifecycle effect; set boot deadlines; refuse launches lacking ownership evidence |
| PostgreSQL | Host, independent service | Keeps intent, allocations, boot identities, and process identities |
| Host files and network | Host filesystem and devices | Hold disks, network allocations, and process evidence for verified owners only |
| Firecracker | Host, one detached process per machine | Runs the guest; keeps running when `agent-plane` stops |
| Guest init | Inside each VM | Starts `vmagent` and the SSH server; configures networking; reaps orphaned children |
| `vmagent` | Inside each VM | Reports health tagged with its boot identity; runs no commands yet |
| Guest SSH server | Inside each VM | Accepts terminal sessions without involving `agent-plane` |

| Simulator actor | Replaces | Actions in this slice |
| --- | --- | --- |
| Scenario script | Terminal, CLI, and real timing | Orders events, crashes, cleanup interruptions, and clock advances |
| Queue + logical clock | Arrival order and wall-clock time | Delivers events; fires deadlines only when the script advances time |
| Lifecycle decisions | Nothing; identical production code | Decide exactly as production does |
| Simulated host | Firecracker, network setup, cleanup, guest health | Returns documented results and failures; finishes launches after a crash |
| Invariant checks | A reader's careful review | Assert distinct allocations, no unexplained second launch, verified cleanup only |
| Trace recorder | Scattered logs | Records logical time beside each event, effect, and fault |

## Point of this slice

A caller boots two machines and controls either without disrupting the other.
SSH provides immediate useful access before the platform supports command actions.
A boot identity distinguishes each execution of a machine.

## Walkthroughs

These walkthroughs illustrate planned behavior; nothing here exists yet.
Discussion questions below may still change individual steps.
Notation follows [slice 1](1-durable-requests.md#walkthroughs).
Decision steps stay hidden; `agent-plane` still routes every choice through them.

### Walkthrough 1: boot machine a and open SSH

<!-- draw-visual: diagrams/2-live-machines-walkthrough-1.mmd -->
```text
┌──────────┐     ┌─────┐     ┌─────────────┐     ┌────────────┐     ┌─────────────┐     ┌──────┐
│ Terminal │     │ CLI │     │ agent-plane │     │ PostgreSQL │     │ Firecracker │     │ VM a │
└─────┬────┘     └──┬──┘     └──────┬──────┘     └──────┬─────┘     └──────┬──────┘     └───┬──┘
      │             │               │                   │                  │                │
      │ 1. start a  │               │                   │                  │                │
      ├────────────►│               │                   │                  │                │
      │             │               │                   │                  │                │
      │             │ 2. start a    │                   │                  │                │
      │             ├──────────────►│                   │                  │                │
      │             │               │                   │                  │                │
      │             │               │ 3. reserve, B1    │                  │                │
      │             │               ├──────────────────►│                  │                │
      │             │               │                   │                  │                │
      │             │ 4. accepted   │                   │                  │                │
      │             │◄──────────────┤                   │                  │                │
      │             │               │                   │                  │                │
      │             │  ┌──────────────────────────┐     │                  │                │
      │             │  │ prepare disk and network │     │                  │                │
      │             │  └──────────────────────────┘     │                  │                │
      │             │               │                   │                  │                │
      │             │               │ 5. launch detached process           │                │
      │             │               ├─────────────────────────────────────►│                │
      │             │               │                   │                  │                │
      │             │               │ 6. record owner   │                  │                │
      │             │               ├──────────────────►│                  │                │
      │             │               │                   │                  │                │
      │             │               │                   │                  │ 7. boot guest  │
      │             │               │                   │                  ├───────────────►│
      │             │               │                   │                  │                │
      │             │               │                   │                  │     ┌──────────────────────┐
      │             │               │                   │                  │     │ init starts services │
      │             │               │                   │                  │     └──────────────────────┘
      │             │               │                   │                  │                │
      │             │               │ 8. health?        │                  │                │
      │             │               ├──────────────────────────────────────────────────────►│
      │             │               │                   │                  │                │
      │             │               │ 9. healthy, boot B1                  │                │
      │             │               │◄──────────────────────────────────────────────────────┤
      │             │               │                   │                  │                │
      │             │               │ 10. a ready       │                  │                │
      │             │               ├──────────────────►│                  │                │
      │             │               │                   │                  │                │
      │ 11. ssh a   │               │                   │                  │                │
      ├────────────►│               │                   │                  │                │
      │             │               │                   │                  │                │
      │             │ 12. SSH details?                  │                  │                │
      │             ├──────────────►│                   │                  │                │
      │             │               │                   │                  │                │
      │             │ 13. address   │                   │                  │                │
      │             │◄──────────────┤                   │                  │                │
      │             │               │                   │                  │                │
      │┌─────────────────────────┐  │                   │                  │                │
      ││ launch local SSH client │  │                   │                  │                │
      │└─────────────────────────┘  │                   │                  │                │
      │             │               │                   │                  │                │
      │ 14. SSH session, agent-plane relays nothing     │                  │                │
      ├────────────────────────────────────────────────────────────────────────────────────►│
      │             │               │                   │                  │                │
```

The start request returns early; the operation outlives that HTTP exchange.
Boot identity B1 exists before any guest report gets trusted.
SSH bytes travel between terminal and guest; `agent-plane` only supplied the address.

### Walkthrough 2: stop and delete a while b keeps running

<!-- draw-visual: diagrams/2-live-machines-walkthrough-2.mmd -->
```text
┌──────────┐     ┌─────┐     ┌─────────────┐     ┌────────────┐     ┌───────────────┐     ┌──────┐
│ Terminal │     │ CLI │     │ agent-plane │     │ PostgreSQL │     │ Firecracker a │     │ VM b │
└─────┬────┘     └──┬──┘     └──────┬──────┘     └──────┬─────┘     └───────┬───────┘     └───┬──┘
      │             │               │                   │                   │                 │
      │ 1. stop a   │               │                   │                   │                 │
      ├────────────►│               │                   │                   │                 │
      │             │               │                   │                   │                 │
      │             │ 2. stop a     │                   │                   │                 │
      │             ├──────────────►│                   │                   │                 │
      │             │               │                   │                   │                 │
      │             │               │ 3. reserve stop   │                   │                 │
      │             │               ├──────────────────►│                   │                 │
      │             │               │                   │                   │                 │
      │             │               │ 4. terminate verified process         │                 │
      │             │               ├──────────────────────────────────────►│                 │
      │             │               │                   │                   │                 │
      │             │               │ 5. exit confirmed │                   │                 │
      │             │               │◄──────────────────────────────────────┤                 │
      │             │               │                   │                   │                 │
      │             │               │ 6. a stopped      │                   │                 │
      │             │               ├──────────────────►│                   │                 │
      │             │               │                   │                   │                 │
      │ 7. delete a │               │                   │                   │                 │
      ├────────────►│               │                   │                   │                 │
      │             │               │                   │                   │                 │
      │             │ 8. delete a   │                   │                   │                 │
      │             ├──────────────►│                   │                   │                 │
      │             │               │                   │                   │                 │
      │             │               │ 9. a stopped?     │                   │                 │
      │             │               ├──────────────────►│                   │                 │
      │             │               │                   │                   │                 │
      │             ┌────────────────────────────────┐  │                   │                 │
      │             │ remove only a's verified files │  │                   │                 │
      │             └────────────────────────────────┘  │                   │                 │
      │             │               │                   │                   │                 │
      │             │               │ 10. a deleted     │                   │                 │
      │             │               ├──────────────────►│                   │                 │
      │             │               │                   │                   │                 │
      │             │               │ 11. health?       │                   │                 │
      │             │               ├────────────────────────────────────────────────────────►│
      │             │               │                   │                   │                 │
      │             │               │ 12. healthy, untouched                │                 │
      │             │               │◄────────────────────────────────────────────────────────┤
      │             │               │                   │                   │                 │
      │ 13. SSH session to b continues throughout       │                   │                 │
      ├──────────────────────────────────────────────────────────────────────────────────────►│
      │             │               │                   │                   │                 │
```

Stopping and deleting remain separate, explicitly requested operations.
Cleanup touches only files whose ownership `agent-plane` verified.

### Walkthrough 3: agent-plane crashes during launch

<!-- draw-visual: diagrams/2-live-machines-walkthrough-3.mmd -->
```text
┌─────────────┐     ┌────────────┐     ┌─────────────┐     ┌──────┐
│ agent-plane │     │ PostgreSQL │     │ Firecracker │     │ VM a │
└──────┬──────┘     └──────┬─────┘     └──────┬──────┘     └───┬──┘
       │                   │                  │                │
       │ 1. reserve, B1    │                  │                │
       ├──────────────────►│                  │                │
       │                   │                  │                │
       │ 2. launch detached process           │                │
       ├─────────────────────────────────────►│                │
       │                   │                  │                │
      ┌─────────────────────────────────────────────────────────┐
      │     agent-plane crashes before recording the owner      │
      └─────────────────────────────────────────────────────────┘
       │                   │                  │                │
       │                   │                  │ 3. boot continues
       │                   │                  ├───────────────►│
       │                   │                  │                │
      ┌─────────────────────────────────────────────────────────┐
      │                  agent-plane restarts                   │
      └─────────────────────────────────────────────────────────┘
       │                   │                  │                │
       │ 4. read unfinished operations        │                │
       ├──────────────────►│                  │                │
       │                   │                  │                │
       │ 5. start a, B1, owner unknown        │                │
       │◄──────────────────┤                  │                │
       │                   │                  │                │
       │ 6. inspect process evidence          │                │
       ├─────────────────────────────────────►│                │
       │                   │                  │                │
       │ 7. process for B1 alive              │                │
       │◄─────────────────────────────────────┤                │
       │                   │                  │                │
       │ 8. record owner   │                  │                │
       ├──────────────────►│                  │                │
       │                   │                  │                │
       │ 9. health?        │                  │                │
       ├──────────────────────────────────────────────────────►│
       │                   │                  │                │
       │ 10. healthy, boot B1                 │                │
       │◄──────────────────────────────────────────────────────┤
       │                   │                  │                │
       │ 11. a ready       │                  │                │
       ├──────────────────►│                  │                │
       │                   │                  │                │
```

No second launch happens; ownership evidence resolved the unknown first.
Which evidence identifies that process remains a discussion question below.
An unreachable guest would stay unknown, never silently become stopped.

## Platform work

- Turn registered intent into disks, network allocations, and detached Firecracker processes.
- Build a guest image containing initialization, vmagent health, and SSH services.
- Add start, inspect, stop, and explicit deletion through the control plane.
- Record process ownership and boot identity before accepting guest observations.
- Add SSH discovery and local client launch without relaying terminal traffic.

Control-plane shutdown must leave machines running.
Restart must inspect owned processes before creating replacements or releasing resources.
Unreachable never means confirmed stopped; cleanup touches only verified owned resources.

## Simulator work

Logical time advances through controlled events instead of real sleeping.
Add lifecycle effects, boot deadlines, health observations, and resource ownership.
Model a process launch completing after the requesting service crashes.
Delay health, fail network setup, and interrupt cleanup at named boundaries.
Check distinct allocations and prohibit duplicate launches without resolved ownership evidence.

Do not simulate CPUs, kernels, or packet routing.
The simulator represents their documented results and failure boundaries.

### Simulator walkthrough

The simulator now owns time as well as event order.
Deadlines fire only when the script advances its logical clock.

```text
 logical time    t0            t1             t2                 t30
                 |-------------|--------------|-------------------|
 events       start a      launch done    health report      boot deadline
                               ^              ^                   ^
 script may:            [crash agent-plane [delay beyond     [advance clock
                         just before]       the deadline]     without sleeping]
```

Each lifecycle boundary is a named place where the script can interrupt.

```text
 reserve --> disk --> network --> launch --> record owner --> health --> ready
    ^         ^          ^           ^             ^             ^
 [commit   [cleanup   [setup      [crash after  [acknowledgment [delayed past
  unknown]  cut short]  fails]      launch]       lost]           deadline]
```

This scenario replays walkthrough 3, then delays health past the deadline.

<!-- draw-visual: diagrams/2-live-machines-simulator.mmd -->
```text
┌────────┐     ┌───────────────┐     ┌───────────┐     ┌──────────┐     ┌──────────────┐
│ Script │     │ Queue + clock │     │ Decisions │     │ Sim host │     │ Checks+trace │
└────┬───┘     └───────┬───────┘     └─────┬─────┘     └─────┬────┘     └───────┬──────┘
     │                 │                   │                 │                  │
     │ 1. start a      │                   │                 │                  │
     ├────────────────►│                   │                 │                  │
     │                 │                   │                 │                  │
     │                 │ 2. deliver start  │                 │                  │
     │                 ├──────────────────►│                 │                  │
     │                 │                   │                 │                  │
     │                 │                   │ 3. effect: launch B1               │
     │                 │                   ├────────────────►│                  │
     │                 │                   │                 │                  │
     │                 │ 4. deadline t30   │                 │                  │
     │                 │◄──────────────────┤                 │                  │
     │                 │                   │                 │                  │
    ┌────────────────────────────────────────────────────────────────────────────┐
    │                      scripted crash: memory discarded                      │
    └────────────────────────────────────────────────────────────────────────────┘
     │                 │                   │                 │                  │
     │                 │                   │     ┌────────────────────────┐     │
     │                 │                   │     │ launch finishes anyway │     │
     │                 │                   │     └────────────────────────┘     │
     │                 │                   │                 │                  │
    ┌────────────────────────────────────────────────────────────────────────────┐
    │              scripted restart: rebuild from committed records              │
    └────────────────────────────────────────────────────────────────────────────┘
     │                 │                   │                 │                  │
     │                 │                   │ 5. effect: inspect                 │
     │                 │                   ├────────────────►│                  │
     │                 │                   │                 │                  │
     │                 │ 6. completion: B1 alive             │                  │
     │                 │◄────────────────────────────────────┤                  │
     │                 │                   │                 │                  │
     │                 │ 7. deliver alive  │                 │                  │
     │                 ├──────────────────►│                 │                  │
     │                 │                   │                 │                  │
     │                 │     ┌────────────────────────────┐  │                  │
     │                 │     │ adopt process, no relaunch │  │                  │
     │                 │     └────────────────────────────┘  │                  │
     │                 │                   │                 │                  │
     │ 8. fault: delay health              │                 │                  │
     ├──────────────────────────────────────────────────────►│                  │
     │                 │                   │                 │                  │
     │ 9. clock to t30 │                   │                 │                  │
     ├────────────────►│                   │                 │                  │
     │                 │                   │                 │                  │
     │                 │ 10. deliver t30   │                 │                  │
     │                 ├──────────────────►│                 │                  │
     │                 │                   │                 │                  │
     │                 │  ┌──────────────────────────────────┐                  │
     │                 │  │ report late boot, keep ownership │                  │
     │                 │  └──────────────────────────────────┘                  │
     │                 │                   │                 │                  │
     │                 │ 11. completion: healthy B1          │                  │
     │                 │◄────────────────────────────────────┤                  │
     │                 │                   │                 │                  │
     │                 │ 12. deliver health│                 │                  │
     │                 ├──────────────────►│                 │                  │
     │                 │                   │                 │                  │
     │                 │                   │ 13. state + effects                │
     │                 │                   ├───────────────────────────────────►│
     │                 │                   │                 │                  │
     │                 │                   │                 │┌───────────────────────────────────┐
     │                 │                   │                 ││ one launch? distinct allocations? │
     │                 │                   │                 │└───────────────────────────────────┘
     │                 │                   │                 │                  │
```

The simulated host returns documented results; it runs no kernel or network.
Real guests must confirm those results before this slice counts as accepted.

## Demonstration and evidence

Boot two real guests and establish SSH sessions.
Stop and delete one while the other remains reachable.
Restart the control plane while both run; verify identities and continued operation.
Replay partial-boot failures and verify resource ownership remains explainable.
Compare simulated lifecycle contracts with real process and network behavior.

## Discuss before expansion

How will restart identify a process when launch acknowledgment disappears?
Which resources require confirmed termination before release?
What guest initialization proves SSH availability and correct child-process handling?

Terra leads host and image boundaries; Sol integrates lifecycle workflows.
Luna high handles the settled SSH wrapper; Astra owns lifecycle tests and simulation.
The lead requires actual machine evidence before acceptance.

Platform command execution waits for slice 3.
Reference scope: old M2/M3 and SSH work from M6, with early lifecycle recovery.

