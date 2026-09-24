# 7. Snapshots, restoration, and the MVP walkthrough

Requires [slice 6](6-streaming.md).
See the [cumulative architecture](7-snapshots_architecture.md).

## Actors and actions

An actor is a module or entity that performs actions within this slice.
Earlier actors remain; this table lists who acts on snapshots and restoration.

| Platform actor | Where it lives | Actions in this slice |
| --- | --- | --- |
| Terminal | Operator's shell | Finishes SSH work first; requests snapshot and restore |
| `perpetual` CLI | Host, short-lived process | Sends snapshot and restore; inspects their operations |
| `agent-plane` snapshot workflow | Host, inside `agent-plane` | Reserves the operation; closes admission; waits for drain; captures into staging; publishes complete snapshots; confirms termination separately |
| `agent-plane` restore workflow | Host, inside `agent-plane` | Requires the stopped original and its allocation; restores files; starts from saved state; issues a fresh boot identity; reopens admission |
| Snapshot decisions | Shared code, host and guest | Order each boundary; reject incomplete artifacts and old boot observations |
| PostgreSQL | Host, independent service | Records snapshots and operations; stays outside every snapshot |
| Host snapshot files | Host filesystem | Separate staging from published, complete snapshots |
| Firecracker | Host, one detached process per machine | Pauses; captures memory and device state; terminates; starts from saved state |
| `vmagent` | Inside each VM | Drains platform work; blocks new mutations; accepts a fresh identity after restore |

| Simulator actor | Replaces | Actions in this slice |
| --- | --- | --- |
| Seeded chooser | Hand-ordered scripts, once those pass | Fails each boundary; races requests against drain; delivers old boot observations |
| Event queue | Arrival order on host and guest | Orders drain, capture, publication, termination, and restore completions |
| Snapshot decisions | Nothing; identical production code | Decide exactly as production does |
| Simulated world | Artifacts, processes, guests | Tracks artifact status only, never memory images |
| Invariant checks | A reader's careful review | Assert incomplete artifacts never restore; history never rewinds; stale boots lose |
| Trace recorder | Scattered logs | Replays failures of recovery itself |

## Point of this slice

A caller saves a consistent machine boundary and later restores that machine.
A snapshot contains saved memory, device state, and matching disk contents.
Draining blocks new platform mutations while existing work finishes.

## Walkthroughs

These walkthroughs illustrate planned behavior; nothing here exists yet.
Discussion questions below may still change individual steps.
Notation follows [slice 1](durable_requests/README.md#walkthroughs).
s1 names the snapshot; B1 and B2 name successive boot identities.

### Walkthrough 1: drain, capture, publish, terminate

<!-- draw-visual: diagrams/7-snapshots-walkthrough-1.mmd -->
```text
┌─────┐     ┌─────────────┐     ┌────────────┐     ┌────────────┐     ┌─────────────┐     ┌─────────┐
│ CLI │     │ agent-plane │     │ PostgreSQL │     │ Host files │     │ Firecracker │     │ vmagent │
└──┬──┘     └──────┬──────┘     └──────┬─────┘     └──────┬─────┘     └──────┬──────┘     └────┬────┘
   │               │                   │                  │                  │                 │
   │ 1. snapshot a │                   │                  │                  │                 │
   ├──────────────►│                   │                  │                  │                 │
   │               │                   │                  │                  │                 │
   │               │ 2. reserve snapshot                  │                  │                 │
   │               ├──────────────────►│                  │                  │                 │
   │               │                   │                  │                  │                 │
   │               │ 3. drain: close mutation admission   │                  │                 │
   │               ├──────────────────────────────────────────────────────────────────────────►│
   │               │                   │                  │                  │                 │
   │               │                   │                  │                  │      ┌─────────────────────┐
   │               │                   │                  │                  │      │ finish running work │
   │               │                   │                  │                  │      └─────────────────────┘
   │               │                   │                  │                  │                 │
   │               │ 4. drained        │                  │                  │                 │
   │               │◄──────────────────────────────────────────────────────────────────────────┤
   │               │                   │                  │                  │                 │
   │   ┌───────────────────────┐       │                  │                  │                 │
   │   │ collect final results │       │                  │                  │                 │
   │   └───────────────────────┘       │                  │                  │                 │
   │               │                   │                  │                  │                 │
   │               │ 5. pause, capture memory + devices   │                  │                 │
   │               ├────────────────────────────────────────────────────────►│                 │
   │               │                   │                  │                  │                 │
   │               │                   │                  │ 6. artifacts     │                 │
   │               │                   │                  │◄─────────────────┤                 │
   │               │                   │                  │                  │                 │
   │               │ 7. copy matching disk to staging     │                  │                 │
   │               ├─────────────────────────────────────►│                  │                 │
   │               │                   │                  │                  │                 │
   │               │ 8. publish complete snapshot         │                  │                 │
   │               ├─────────────────────────────────────►│                  │                 │
   │               │                   │                  │                  │                 │
   │               │ 9. s1 published   │                  │                  │                 │
   │               ├──────────────────►│                  │                  │                 │
   │               │                   │                  │                  │                 │
   │               │ 10. terminate machine                │                  │                 │
   │               ├────────────────────────────────────────────────────────►│                 │
   │               │                   │                  │                  │                 │
   │               │ 11. exit confirmed│                  │                  │                 │
   │               │◄────────────────────────────────────────────────────────┤                 │
   │               │                   │                  │                  │                 │
   │               │ 12. a stopped     │                  │                  │                 │
   │               ├──────────────────►│                  │                  │                 │
   │               │                   │                  │                  │                 │
   │ 13. s1 done   │                   │                  │                  │                 │
   │◄──────────────┤                   │                  │                  │                 │
   │               │                   │                  │                  │                 │
```

Publication and termination receive separate durable confirmations.
Draining cannot see SSH sessions; the terminal must finish those first.

### Walkthrough 2: restore the same machine

<!-- draw-visual: diagrams/7-snapshots-walkthrough-2.mmd -->
```text
┌─────┐     ┌─────────────┐     ┌────────────┐     ┌────────────┐     ┌─────────────┐     ┌─────────┐
│ CLI │     │ agent-plane │     │ PostgreSQL │     │ Host files │     │ Firecracker │     │ vmagent │
└──┬──┘     └──────┬──────┘     └──────┬─────┘     └──────┬─────┘     └──────┬──────┘     └────┬────┘
   │               │                   │                  │                  │                 │
   │ 1. restore a  │                   │                  │                  │                 │
   ├──────────────►│                   │                  │                  │                 │
   │               │                   │                  │                  │                 │
   │               │ 2. a stopped?     │                  │                  │                 │
   │               ├──────────────────►│                  │                  │                 │
   │               │                   │                  │                  │                 │
   │               │ 3. reserve, B2    │                  │                  │                 │
   │               ├──────────────────►│                  │                  │                 │
   │               │                   │                  │                  │                 │
   │               │ 4. restore disk from s1              │                  │                 │
   │               ├─────────────────────────────────────►│                  │                 │
   │               │                   │                  │                  │                 │
   │               │ 5. start from saved memory + devices │                  │                 │
   │               ├────────────────────────────────────────────────────────►│                 │
   │               │                   │                  │                  │                 │
   │               │ 6. bootstrap: you are boot B2        │                  │                 │
   │               ├──────────────────────────────────────────────────────────────────────────►│
   │               │                   │                  │                  │                 │
   │               │ 7. ready as B2    │                  │                  │                 │
   │               │◄──────────────────────────────────────────────────────────────────────────┤
   │               │                   │                  │                  │                 │
   │               │ 8. a ready        │                  │                  │                 │
   │               ├──────────────────►│                  │                  │                 │
   │               │                   │                  │                  │                 │
   │      ┌──────────────────┐         │                  │                  │                 │
   │      │ reopen admission │         │                  │                  │                 │
   │      └──────────────────┘         │                  │                  │                 │
   │               │                   │                  │                  │                 │
   │               │ 9. delayed report tagged B1          │                  │                 │
   │               │◄──────────────────────────────────────────────────────────────────────────┤
   │               │                   │                  │                  │                 │
   │ ┌───────────────────────────┐     │                  │                  │                 │
   │ │ reject: B1 is not current │     │                  │                  │                 │
   │ └───────────────────────────┘     │                  │                  │                 │
   │               │                   │                  │                  │                 │
   │ 10. a restored│                   │                  │                  │                 │
   │◄──────────────┤                   │                  │                  │                 │
   │               │                   │                  │                  │                 │
```

Restore also requires the machine's saved network allocation.
Files return with the disk; host history stays exactly where it was.
The restored guest believed B1 until bootstrap replaced that identity.

### Walkthrough 3: agent-plane crashes between capture and publication

<!-- draw-visual: diagrams/7-snapshots-walkthrough-3.mmd -->
```text
            ┌─────────────┐     ┌────────────┐     ┌────────────┐     ┌─────────────┐
            │ agent-plane │     │ PostgreSQL │     │ Host files │     │ Firecracker │
            └──────┬──────┘     └──────┬─────┘     └──────┬─────┘     └──────┬──────┘
                   │                   │                  │                  │
                   │ 1. pause, capture memory + devices   │                  │
                   ├────────────────────────────────────────────────────────►│
                   │                   │                  │                  │
                   │                   │                  │ 2. artifacts     │
                   │                   │                  │◄─────────────────┤
                   │                   │                  │                  │
                  ┌───────────────────────────────────────────────────────────┐
                  │           agent-plane crashes before publishing           │
                  └───────────────────────────────────────────────────────────┘
                   │                   │                  │                  │
                  ┌───────────────────────────────────────────────────────────┐
                  │                   agent-plane restarts                    │
                  └───────────────────────────────────────────────────────────┘
                   │                   │                  │                  │
                   │ 3. read unfinished│                  │                  │
                   ├──────────────────►│                  │                  │
                   │                   │                  │                  │
                   │ 4. s1 unpublished │                  │                  │
                   │◄──────────────────┤                  │                  │
                   │                   │                  │                  │
                   │ 5. inspect staging│                  │                  │
                   ├─────────────────────────────────────►│                  │
                   │                   │                  │                  │
                   │ 6. incomplete set │                  │                  │
                   │◄─────────────────────────────────────┤                  │
                   │                   │                  │                  │
                   │ 7. s1 failed      │                  │                  │
                   ├──────────────────►│                  │                  │
                   │                   │                  │                  │
      ┌──────────────────────────┐     │                  │                  │
      │ s1 can never be restored │     │                  │                  │
      └──────────────────────────┘     │                  │                  │
                   │                   │                  │                  │
                   │ 8. inspect paused machine            │                  │
                   ├────────────────────────────────────────────────────────►│
                   │                   │                  │                  │
                   │ 9. process alive, paused             │                  │
                   │◄────────────────────────────────────────────────────────┤
                   │                   │                  │                  │
┌─────────────────────────────────────┐│                  │                  │
│ ownership and cleanup stay recorded ││                  │                  │
└─────────────────────────────────────┘│                  │                  │
                   │                   │                  │                  │
```

Staging never counts as a snapshot, whatever files it contains.
How recovery treats the paused machine remains a discussion question below.

## Platform work

- Drain platform commands and writes before capturing snapshot artifacts.
- Publish only complete snapshots, then confirm machine termination separately.
- Restore the original stopped machine using its saved allocation.
- Establish fresh boot identity and connections before reopening admission.
- Preserve host history and saved snapshots independently from machine deletion.

Users must finish interactive SSH work before snapshotting.
PostgreSQL and collected host history remain outside guest snapshots.
Failed drain or publication must leave ownership and cleanup obligations explicit.

## Simulator work

Add drain, capture, publication, termination, and restoration effects to existing decisions.
Fail each boundary and race new requests against drain admission.
Deliver old boot observations after restoration and verify rejection.
Check incomplete artifacts never become eligible for restoration.
Test recovery failure itself where publication or ownership guarantees depend on it.

The simulator models artifact status, not memory images or Firecracker internals.
Actual snapshot restoration remains required acceptance evidence.

### Simulator walkthrough

Snapshotting is a chain of boundaries; the simulator can cut every link.

```text
 drain --> capture --> publish --> terminate --> restore --> bootstrap --> reopen
   ^          ^           ^            ^            ^            ^
 [request   [partial   [crash       [confirmation [fails; the  [old boot B1
  races the  artifact]  before       lost]         original     observation
  drain]                publishing]                disk kept]   arrives]
```

Artifacts carry a status, never real memory contents.

```text
 staging:    [disk ok] [memory ok] [devices --]     status: incomplete, never restorable
 published:  [disk ok] [memory ok] [devices ok]     status: complete, restorable
```

This scenario races a write against drain, then fails publication.

<!-- draw-visual: diagrams/7-snapshots-simulator.mmd -->
```text
┌─────────┐     ┌───────┐     ┌───────────┐     ┌───────────┐     ┌──────────────┐
│ Chooser │     │ Queue │     │ Decisions │     │ Sim world │     │ Checks+trace │
└────┬────┘     └───┬───┘     └─────┬─────┘     └─────┬─────┘     └───────┬──────┘
     │              │               │                 │                   │
     │ 1. snapshot a│               │                 │                   │
     ├─────────────►│               │                 │                   │
     │              │               │                 │                   │
     │              │ 2. deliver snapshot             │                   │
     │              ├──────────────►│                 │                   │
     │              │               │                 │                   │
     │              │               │ 3. effect: drain│                   │
     │              │               ├────────────────►│                   │
     │              │               │                 │                   │
     │ 4. race: write W9            │                 │                   │
     ├─────────────►│               │                 │                   │
     │              │               │                 │                   │
     │              │ 5. deliver W9 │                 │                   │
     │              ├──────────────►│                 │                   │
     │              │               │                 │                   │
     │              │  ┌──────────────────────────┐   │                   │
     │              │  │ reject: admission closed │   │                   │
     │              │  └──────────────────────────┘   │                   │
     │              │               │                 │                   │
     │              │ 6. completion: drained          │                   │
     │              │◄────────────────────────────────┤                   │
     │              │               │                 │                   │
     │              │ 7. deliver drained              │                   │
     │              ├──────────────►│                 │                   │
     │              │               │                 │                   │
     │              │               │ 8. effect: capture                  │
     │              │               ├────────────────►│                   │
     │              │               │                 │                   │
     │              │ 9. completion: captured         │                   │
     │              │◄────────────────────────────────┤                   │
     │              │               │                 │                   │
     │              │ 10. deliver captured            │                   │
     │              ├──────────────►│                 │                   │
     │              │               │                 │                   │
     │              │               │ 11. effect: publish                 │
     │              │               ├────────────────►│                   │
     │              │               │                 │                   │
     │ 12. fault: publish fails midway                │                   │
     ├───────────────────────────────────────────────►│                   │
     │              │               │                 │                   │
     │              │               │       ┌────────────────────┐        │
     │              │               │       │ status: incomplete │        │
     │              │               │       └────────────────────┘        │
     │              │               │                 │                   │
     │ 13. restore from s1          │                 │                   │
     ├─────────────►│               │                 │                   │
     │              │               │                 │                   │
     │              │ 14. deliver restore             │                   │
     │              ├──────────────►│                 │                   │
     │              │               │                 │                   │
     │              │   ┌───────────────────────┐     │                   │
     │              │   │ reject: s1 incomplete │     │                   │
     │              │   └───────────────────────┘     │                   │
     │              │               │                 │                   │
     │              │               │ 15. state + effects                 │
     │              │               ├────────────────────────────────────►│
     │              │               │                 │                   │
     │              │               │                 │    ┌──────────────────────────────┐
     │              │               │                 │    │ incomplete never restorable? │
     │              │               │                 │    └──────────────────────────────┘
     │              │               │                 │                   │
```

A later script lets restoration succeed, then delivers a B1 health report.
Decisions must reject that report because the current boot is B2.
Actual snapshot restoration remains required acceptance evidence.

## Demonstration and evidence

Create files, finish commands, snapshot, stop, restore, and inspect the same machine.
Verify fresh identity, preserved files, and unchanged historical host records.
Interrupt publication and restoration; reject incomplete or conflicting outcomes.
Run the cumulative two-machine walkthrough and all relevant earlier regressions.
Audit each core boundary for remaining testability gaps and scoped refactors.
Update testing documentation and knowledge using actual observed results.

## Discuss before expansion

Which artifacts and confirmations establish a complete, restorable snapshot?
What recovery preserves the original disk when restoration fails?
Which remaining limitations must the final walkthrough explicitly teach?

The design and test owners establish snapshot contracts, restoration decisions, and testing; assign models when expanding this slice.
Production owners address scoped integration defects.
The documentation owner records settled walkthroughs; the lead performs final acceptance.

Cloning, live migration, automatic expiry, and multi-host coordination remain deferred.
Reference scope: old M9/M10.

