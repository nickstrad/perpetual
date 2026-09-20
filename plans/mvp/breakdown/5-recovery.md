# 5. Recovery across independent failures

Requires [slice 4](4-command-actions.md).
See the [cumulative architecture](5-recovery_architecture.md).

## Actors and actions

An actor is a module or entity that performs actions within this slice.
No new component appears; existing actors gain recovery actions.

| Platform actor | Where it lives | Actions in this slice |
| --- | --- | --- |
| Terminal and CLI | Operator's shell and host process | Inspect machines and actions; see stale and unresolved work reported honestly |
| `agent-plane` reconciler | Host, inside `agent-plane` | Reads unfinished records; verifies processes; compares guest identities; inspects existing identifiers; reopens admission per machine |
| `agent-plane` database link | Host, inside `agent-plane` | Detects database loss; blocks durable mutations; reconnects within bounds; inspects unknown commits |
| Recovery decisions | Shared code, host and guest | Classify evidence as resolved, stale, or uncertain before choosing further work |
| PostgreSQL | Host, independent service | May restart alone; keeps committed intent and evidence |
| Firecracker | Host, one detached process per machine | Keeps guests running through every control-plane failure |
| `vmagent` | Inside each VM | May restart alone; takes a fresh guest-process identity; marks unfinished actions uncertain |
| VM a and VM b | Independent guests | One stays unreachable; the other must remain fully usable |

| Simulator actor | Replaces | Actions in this slice |
| --- | --- | --- |
| Seeded chooser | Hand-ordered scripts, once those pass | Schedules independent crashes between ordinary events |
| Queue + logical clock | Arrival order and wall-clock time | Delivers healthy machines' events fairly; measures progress bounds |
| Recovery decisions | Nothing; identical production code | Decide exactly as production does |
| Simulated world | Database, guests, processes, transports | Keeps durable storage apart from each component's volatile memory |
| Invariant checks | A reader's careful review | Run during faults, not only after recovery |
| Progress check | Hoping things finish | Requires machine b's outcome within stated bounds and assumptions |
| Trace recorder | Scattered logs | Saves one combined failure for exact replay |

## Point of this slice

A caller can understand surviving work after independent component failures.
Reconciliation compares persisted intent with current observations before choosing further work.
Earlier slices already recover their introduced effects; this slice combines those guarantees.

## Walkthroughs

These walkthroughs illustrate planned behavior; nothing here exists yet.
Discussion questions below may still change individual steps.
Notation follows [slice 1](1-durable-requests.md#walkthroughs).
X1 and Y1 name commands on machines a and b.

### Walkthrough 1: agent-plane restarts; a is unreachable, b is healthy

<!-- draw-visual: diagrams/5-recovery-walkthrough-1.mmd -->
```text
┌─────┐     ┌─────────────┐     ┌────────────┐     ┌──────┐     ┌──────┐
│ CLI │     │ agent-plane │     │ PostgreSQL │     │ VM a │     │ VM b │
└──┬──┘     └──────┬──────┘     └──────┬─────┘     └───┬──┘     └───┬──┘
   │               │                   │               │            │
  ┌──────────────────────────────────────────────────────────────────┐
  │      agent-plane restarts, database and guests kept running      │
  └──────────────────────────────────────────────────────────────────┘
   │               │                   │               │            │
   │               │ 1. read unfinished│               │            │
   │               ├──────────────────►│               │            │
   │               │                   │               │            │
   │               │ 2. a: X1, b: Y1   │               │            │
   │               │◄──────────────────┤               │            │
   │               │                   │               │            │
   │               │ 3. inspect X1     │               │            │
   │               ├──────────────────────────────────×│            │
   │               │                   │               │            │
   │┌─────────────────────────────┐    │               │            │
   ││ a unreachable: X1 uncertain │    │               │            │
   │└─────────────────────────────┘    │               │            │
   │               │                   │               │            │
   │               │ 4. identity? inspect Y1           │            │
   │               ├───────────────────────────────────────────────►│
   │               │                   │               │            │
   │               │ 5. same boot, Y1 running          │            │
   │               │◄───────────────────────────────────────────────┤
   │               │                   │               │            │
   │               │ 6. b reconciled   │               │            │
   │               ├──────────────────►│               │            │
   │               │                   │               │            │
   │┌─────────────────────────────┐    │               │            │
   ││ reopen admission for b only │    │               │            │
   │└─────────────────────────────┘    │               │            │
   │               │                   │               │            │
   │ 7. inspect a, b                   │               │            │
   ├──────────────►│                   │               │            │
   │               │                   │               │            │
   │ 8. a stale, b ready               │               │            │
   │◄──────────────┤                   │               │            │
   │               │                   │               │            │
   │               │ 9. bounded retry  │               │            │
   │               ├──────────────────────────────────×│            │
   │               │                   │               │            │
   │               │ 10. poll Y1 output│               │            │
   │               ├───────────────────────────────────────────────►│
   │               │                   │               │            │
   │               │ 11. output, exit 0│               │            │
   │               │◄───────────────────────────────────────────────┤
   │               │                   │               │            │
```

Machine a's silence never becomes "stopped" or "safe to rerun".
Machine b reopens alone; it never waits for a.
Inspection shows a's last observation together with its age.

### Walkthrough 2: PostgreSQL restarts under a running command

<!-- draw-visual: diagrams/5-recovery-walkthrough-2.mmd -->
```text
  ┌─────┐     ┌─────────────┐     ┌────────────┐     ┌─────────┐     ┌─────────┐
  │ CLI │     │ agent-plane │     │ PostgreSQL │     │ vmagent │     │ Command │
  └──┬──┘     └──────┬──────┘     └──────┬─────┘     └────┬────┘     └────┬────┘
     │               │                   │                │               │
     │               │ 1. cursor = 3     │                │               │
     │               ├──────────────────×│                │               │
     │               │                   │                │               │
    ┌──────────────────────────────────────────────────────────────────────┐
    │         PostgreSQL restarts, that commit outcome is unknown          │
    └──────────────────────────────────────────────────────────────────────┘
     │               │                   │                │               │
     │               │                   │                │ 2. 4..6       │
     │               │                   │                │◄──────────────┤
     │               │                   │                │               │
     │ 3. write W2   │                   │                │               │
     ├──────────────►│                   │                │               │
     │               │                   │                │               │
     │ 4. unavailable│                   │                │               │
     │◄──────────────┤                   │                │               │
     │               │                   │                │               │
┌──────────────────────────────────────────┐              │               │
│ no durable mutation without the database │              │               │
└──────────────────────────────────────────┘              │               │
     │               │                   │                │               │
     │               │ 5. reconnect, bounded              │               │
     │               ├──────────────────►│                │               │
     │               │                   │                │               │
     │               │ 6. cursor = 3 committed?           │               │
     │               ├──────────────────►│                │               │
     │               │                   │                │               │
     │               │ 7. yes            │                │               │
     │               │◄──────────────────┤                │               │
     │               │                   │                │               │
    ┌──────────────────────────────────┐ │                │               │
    │ reconcile, then reopen admission │ │                │               │
    └──────────────────────────────────┘ │                │               │
     │               │                   │                │               │
     │               │ 8. poll after cursor 3             │               │
     │               ├───────────────────────────────────►│               │
     │               │                   │                │               │
     │               │ 9. 4..6           │                │               │
     │               │◄───────────────────────────────────┤               │
     │               │                   │                │               │
     │               │ 10. cursor = 6    │                │               │
     │               ├──────────────────►│                │               │
     │               │                   │                │               │
```

Connection loss proved nothing about the commit, so `agent-plane` asked.
The guest kept working; only new durable mutations waited.

### Walkthrough 3: vmagent restarts; a stale observation arrives

<!-- draw-visual: diagrams/5-recovery-walkthrough-3.mmd -->
```text
       ┌─────────────┐     ┌────────────┐     ┌─────────┐     ┌─────────┐
       │ agent-plane │     │ PostgreSQL │     │ vmagent │     │ Command │
       └──────┬──────┘     └──────┬─────┘     └────┬────┘     └────┬────┘
              │                   │                │               │
              │                   │                │ 1. launch X1  │
              │                   │                ├──────────────►│
              │                   │                │               │
             ┌──────────────────────────────────────────────────────┐
             │         vmagent restarts as guest process G2         │
             └──────────────────────────────────────────────────────┘
              │                   │                │               │
              │                   │    ┌────────────────────────┐  │
              │                   │    │ journal: X1 unfinished │  │
              │                   │    └────────────────────────┘  │
              │                   │                │               │
              │ 2. inspect X1     │                │               │
              ├───────────────────────────────────►│               │
              │                   │                │               │
              │ 3. G2: X1 uncertain, not rerun     │               │
              │◄───────────────────────────────────┤               │
              │                   │                │               │
              │ 4. X1 uncertain, slot kept         │               │
              ├──────────────────►│                │               │
              │                   │                │               │
              │ 5. delayed report tagged old boot B0               │
              │◄───────────────────────────────────┤               │
              │                   │                │               │
┌───────────────────────────┐     │                │               │
│ reject: B0 is not current │     │                │               │
└───────────────────────────┘     │                │               │
              │                   │                │               │
```

X1 stays uncertain until evidence, or an operator, resolves it.
Which observations close uncertainty remains a discussion question below.

## Platform work

- Reconcile interrupted lifecycle operations, guest actions, and output collection together.
- Distinguish control-plane restart, guest-agent restart, database restart, and machine loss.
- Reject stale boot observations and preserve uncertain execution evidence.
- Reconnect PostgreSQL and inspect unknown commits before dependent mutations.
- Keep healthy guests usable when another guest remains unreachable.

Database loss blocks new durable mutations without stopping accepted guest work.
Recovery must not convert missing evidence into permission for duplicate execution.
Report stale observations and unresolved work honestly through inspection commands.

## Simulator work

Expand the existing fault schedule across independently failing components.
Keep simulated durable storage separate from each component's volatile memory.
Check invariants during faults, not only after recovery finishes.

A progress check requires an outcome within explicit bounds and health assumptions.
Leave machine A unreachable while requiring healthy machine B to complete work.
Deliver eligible healthy events fairly, without repeatedly restarting B.
Report pending events and violated assumptions when progress bounds expire.

### Simulator walkthrough

Each component now fails alone, on its own line of the schedule.
`X` marks a crash; dots mark downtime.

```text
 logical time     t0                                                 t60
                  |---------------------------------------------------|
 agent-plane      ======X..........==================================
 PostgreSQL       ===================X.......========================
 vmagent on a     ===X...............................................   stays unreachable
 vmagent on b     ===================================================
 command Y1 on b      [submitted]------------------[must finish]------|  progress bound
```

Every simulated component keeps two kinds of state.

```text
+-- agent-plane ----+   +-- PostgreSQL -------+   +-- vmagent ----------+
| volatile: memory  |   | volatile: sessions  |   | volatile: memory    |
|   lost at crash   |   |   lost at crash     |   |   lost at crash     |
| durable: none     |   | durable: committed  |   | durable: journal    |
|                   |   |   rows survive      |   |   entries survive   |
+-------------------+   +---------------------+   +---------------------+
 restart always rebuilds from durable state, never from a saved memory object
```

This scenario keeps a unreachable while b must still finish Y1.

<!-- draw-visual: diagrams/5-recovery-simulator.mmd -->
```text
┌─────────┐     ┌───────────────┐     ┌───────────┐     ┌───────────┐     ┌──────────────┐
│ Chooser │     │ Queue + clock │     │ Decisions │     │ Sim world │     │ Checks+trace │
└────┬────┘     └───────┬───────┘     └─────┬─────┘     └─────┬─────┘     └───────┬──────┘
     │                  │                   │                 │                   │
     │ 1. fault: a unreachable              │                 │                   │
     ├───────────────────────────────────────────────────────►│                   │
     │                  │                   │                 │                   │
     │ 2. exec Y1 on b  │                   │                 │                   │
     ├─────────────────►│                   │                 │                   │
     │                  │                   │                 │                   │
     │                  │ 3. deliver Y1     │                 │                   │
     │                  ├──────────────────►│                 │                   │
     │                  │                   │                 │                   │
     │                  │                   │ 4. effect: dispatch                 │
     │                  │                   ├────────────────►│                   │
     │                  │                   │                 │                   │
     │                  │                   │                 │       ┌───────────────────────┐
     │                  │                   │                 │       │ bound: Y1 done by t60 │
     │                  │                   │                 │       └───────────────────────┘
     │                  │                   │                 │                   │
    ┌──────────────────────────────────────────────────────────────────────────────┐
    │         chosen fault: agent-plane crashes, store and guests persist          │
    └──────────────────────────────────────────────────────────────────────────────┘
     │                  │                   │                 │                   │
     │                  │     ┌────────────────────────────┐  │                   │
     │                  │     │ rebuild from durable state │  │                   │
     │                  │     └────────────────────────────┘  │                   │
     │                  │                   │                 │                   │
     │                  │                   │ 5. effect: inspect a, b             │
     │                  │                   ├────────────────►│                   │
     │                  │                   │                 │                   │
     │                  │ 6. completion: Y1 running           │                   │
     │                  │◄────────────────────────────────────┤                   │
     │                  │                   │                 │                   │
     │                  │                   │       ┌────────────────────┐        │
     │                  │                   │       │ a: nothing arrives │        │
     │                  │                   │       └────────────────────┘        │
     │                  │                   │                 │                   │
     │ 7. fair pick: b  │                   │                 │                   │
     ├─────────────────►│                   │                 │                   │
     │                  │                   │                 │                   │
     │                  │ 8. completion: Y1 exit 0            │                   │
     │                  │◄────────────────────────────────────┤                   │
     │                  │                   │                 │                   │
     │                  │ 9. deliver exit 0 │                 │                   │
     │                  ├──────────────────►│                 │                   │
     │                  │                   │                 │                   │
     │                  │                   │ 10. state + effects, every step     │
     │                  │                   ├────────────────────────────────────►│
     │                  │                   │                 │                   │
     │                  │ 11. clock reaches t60               │                   │
     │                  ├────────────────────────────────────────────────────────►│
     │                  │                   │                 │                   │
     │                  │                   │                 │     ┌────────────────────────────┐
     │                  │                   │                 │     │ Y1 done? else list pending │
     │                  │                   │                 │     └────────────────────────────┘
     │                  │                   │                 │                   │
```

An expired bound reports pending events and the violated assumption.
A fair schedule cannot starve b by restarting it repeatedly.

## Demonstration and evidence

Restart agent-plane while PostgreSQL and guests continue running.
Separately restart an exclusively owned test database and verify reconnection.
Restart vmagent and inspect unfinished commands without blindly relaunching them.
Compare simulated recovery with real service, journal, transport, and database checks.
Reproduce one combined failure through its saved trace.

## Discuss before expansion

Which observations close uncertainty, and which require operator intervention?
What scheduling assumptions make a healthy-machine progress deadline meaningful?
Which bounded retries avoid turning an unavailable guest into host-wide starvation?

Astra leads recovery decisions and testing.
Production owners implement scoped fixes in their existing components.
The lead reviews uncertainty classifications and independent-machine progress.

Streaming follows after polling recovery becomes trustworthy.
Reference scope: old M8, moved before optional transport expansion.

