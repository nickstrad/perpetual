# 3. Commands that survive callers

Requires [slice 2](2-live-machines.md).
See the [cumulative architecture](3-detached-commands_architecture.md).

## Actors and actions

An actor is a module or entity that performs actions within this slice.
Earlier actors remain; this table lists who acts on commands.

| Platform actor | Where it lives | Actions in this slice |
| --- | --- | --- |
| Terminal | Operator's shell | Starts a command, disconnects, and watches later |
| `perpetual` CLI | Host, short-lived process | Chooses the action identifier; submits; may exit; polls with `--watch/-w <vm>` |
| `agent-plane` dispatch | Host, inside `agent-plane` | Reserves the guest's single command slot; saves the request; forwards it; inspects that identifier after uncertainty |
| `agent-plane` collector | Host, inside `agent-plane` | Polls numbered output without any caller attached; saves output before advancing its cursor |
| Command decisions | Shared code, host and guest | Decide admission, retry answers, uncertainty, and slot release |
| PostgreSQL | Host, independent service | Keeps action ownership, status, and output cursors |
| Host output files | Host filesystem | Hold collected output for later watchers |
| `vmagent` | Inside each VM | Journals acceptance; launches the child; numbers output; records the outcome; answers retries from its journal |
| Guest journal | Inside each VM, on disk | Preserves acceptance, launch evidence, and outcomes across HTTP lifetimes |
| Command | Inside each VM, child process | Runs and exits regardless of callers or collectors |

| Simulator actor | Replaces | Actions in this slice |
| --- | --- | --- |
| Scenario script | Terminal, CLI, and real timing | Orders requests, lost acknowledgments, outages, and delayed output |
| Event queue | Arrival order on host and guest | Holds pending external completions until the script releases them |
| Host decisions | Nothing; identical production code | Decide dispatch, collection, and uncertainty |
| Guest decisions | Nothing; identical production code | Decide guest admission and retry answers |
| Simulated adapters | Guest HTTP, journal, child process, output | Accept requests, lose replies, leave launch gaps, delay output |
| Invariant checks | A reader's careful review | Assert one execution per identifier, held slots, and honest cursors |
| Trace recorder | Scattered logs | Shows host and guest steps in one ordered story |

## Point of this slice

A caller starts a command, disconnects, and later inspects the same execution.
Polling means repeatedly requesting status or output.
A journal records durable guest evidence; a cursor tracks collected output position.

## Walkthroughs

These walkthroughs illustrate planned behavior; nothing here exists yet.
Discussion questions below may still change individual steps.
Notation follows [slice 1](1-durable-requests.md#walkthroughs).
Output `#n` means the numbered output position n.

### Walkthrough 1: start, detach, collect, watch later

<!-- draw-visual: diagrams/3-detached-commands-walkthrough-1.mmd -->
```text
┌──────────┐     ┌─────┐     ┌─────────────┐     ┌────────────┐     ┌─────────┐     ┌─────────┐
│ Terminal │     │ CLI │     │ agent-plane │     │ PostgreSQL │     │ vmagent │     │ Command │
└─────┬────┘     └──┬──┘     └──────┬──────┘     └──────┬─────┘     └────┬────┘     └────┬────┘
      │             │               │                   │                │               │
      │ 1. exec on a│               │                   │                │               │
      ├────────────►│               │                   │                │               │
      │             │               │                   │                │               │
      │  ┌─────────────────────┐    │                   │                │               │
      │  │ choose action id X1 │    │                   │                │               │
      │  └─────────────────────┘    │                   │                │               │
      │             │               │                   │                │               │
      │             │ 2. command X1 │                   │                │               │
      │             ├──────────────►│                   │                │               │
      │             │               │                   │                │               │
      │             │               │ 3. reserve slot   │                │               │
      │             │               ├──────────────────►│                │               │
      │             │               │                   │                │               │
      │             │               │ 4. dispatch X1    │                │               │
      │             │               ├───────────────────────────────────►│               │
      │             │               │                   │                │               │
      │             │               │                   │      ┌───────────────────┐     │
      │             │               │                   │      │ journal: accepted │     │
      │             │               │                   │      └───────────────────┘     │
      │             │               │                   │                │               │
      │             │               │                   │                │ 5. launch     │
      │             │               │                   │                ├──────────────►│
      │             │               │                   │                │               │
      │             │               │ 6. accepted       │                │               │
      │             │               │◄───────────────────────────────────┤               │
      │             │               │                   │                │               │
      │             │ 7. X1 accepted│                   │                │               │
      │             │◄──────────────┤                   │                │               │
      │             │               │                   │                │               │
      │ 8. print X1 │               │                   │                │               │
      │◄────────────┤               │                   │                │               │
      │             │               │                   │                │               │
     ┌────────────────────────────────────────────────────────────────────────────────────┐
     │                  terminal disconnects, the command keeps running                   │
     └────────────────────────────────────────────────────────────────────────────────────┘
      │             │               │                   │                │               │
      │             │               │                   │                │ 9. 1..3       │
      │             │               │                   │                │◄──────────────┤
      │             │               │                   │                │               │
      │             │               │ 10. poll after cursor 0            │               │
      │             │               ├───────────────────────────────────►│               │
      │             │               │                   │                │               │
      │             │               │ 11. output 1..3   │                │               │
      │             │               │◄───────────────────────────────────┤               │
      │             │               │                   │                │               │
      │             │   ┌───────────────────────┐       │                │               │
      │             │   │ save host output file │       │                │               │
      │             │   └───────────────────────┘       │                │               │
      │             │               │                   │                │               │
      │             │               │ 12. cursor = 3    │                │               │
      │             │               ├──────────────────►│                │               │
      │             │               │                   │                │               │
      │             │               │                   │                │ 13. exit 0    │
      │             │               │                   │                │◄──────────────┤
      │             │               │                   │                │               │
      │             │               │                   │       ┌──────────────────┐     │
      │             │               │                   │       │ journal: outcome │     │
      │             │               │                   │       └──────────────────┘     │
      │             │               │                   │                │               │
      │             │               │ 14. poll after cursor 3            │               │
      │             │               ├───────────────────────────────────►│               │
      │             │               │                   │                │               │
      │             │               │ 15. final status: exit 0           │               │
      │             │               │◄───────────────────────────────────┤               │
      │             │               │                   │                │               │
      │             │               │ 16. free slot     │                │               │
      │             │               ├──────────────────►│                │               │
      │             │               │                   │                │               │
     ┌────────────────────────────────────────────────────────────────────────────────────┐
     │                              later, from any terminal                              │
     └────────────────────────────────────────────────────────────────────────────────────┘
      │             │               │                   │                │               │
      │ 17. --watch a               │                   │                │               │
      ├────────────►│               │                   │                │               │
      │             │               │                   │                │               │
      │             │ 18. watch X1  │                   │                │               │
      │             ├──────────────►│                   │                │               │
      │             │               │                   │                │               │
      │             │ 19. output, exit                  │                │               │
      │             │◄──────────────┤                   │                │               │
      │             │               │                   │                │               │
      │ 20. output  │               │                   │                │               │
      │◄────────────┤               │                   │                │               │
      │             │               │                   │                │               │
```

The command never depended on the terminal, the CLI, or an HTTP connection.
The collector gathered output while nobody watched.
Watch reads host output through its own cursor; it owns nothing.

### Walkthrough 2: the acceptance acknowledgment disappears

<!-- draw-visual: diagrams/3-detached-commands-walkthrough-2.mmd -->
```text
┌─────┐     ┌─────────────┐     ┌────────────┐     ┌─────────┐     ┌─────────┐
│ CLI │     │ agent-plane │     │ PostgreSQL │     │ vmagent │     │ Command │
└──┬──┘     └──────┬──────┘     └──────┬─────┘     └────┬────┘     └────┬────┘
   │               │                   │                │               │
   │ 1. command X1 │                   │                │               │
   ├──────────────►│                   │                │               │
   │               │                   │                │               │
   │               │ 2. reserve slot   │                │               │
   │               ├──────────────────►│                │               │
   │               │                   │                │               │
   │               │ 3. dispatch X1    │                │               │
   │               ├───────────────────────────────────►│               │
   │               │                   │                │               │
   │               │                   │      ┌───────────────────┐     │
   │               │                   │      │ journal: accepted │     │
   │               │                   │      └───────────────────┘     │
   │               │                   │                │               │
   │               │                   │                │ 4. launch child
   │               │                   │                ├──────────────►│
   │               │                   │                │               │
   │               │ 5. accepted       │                │               │
   │               │×───────────────────────────────────┤               │
   │               │                   │                │               │
   │               │ 6. X1 uncertain   │                │               │
   │               ├──────────────────►│                │               │
   │               │                   │                │               │
   │ 7. X1 outcome uncertain           │                │               │
   │◄──────────────┤                   │                │               │
   │               │                   │                │               │
   │               │ 8. inspect X1     │                │               │
   │               ├───────────────────────────────────►│               │
   │               │                   │                │               │
   │               │ 9. X1 running     │                │               │
   │               │◄───────────────────────────────────┤               │
   │               │                   │                │               │
   │               │ 10. X1 running    │                │               │
   │               ├──────────────────►│                │               │
   │               │                   │                │               │
   │ 11. retry command X1              │                │               │
   ├──────────────►│                   │                │               │
   │               │                   │                │               │
   │ 12. existing X1: running          │                │               │
   │◄──────────────┤                   │                │               │
   │               │                   │                │               │
```

Inspection reuses X1; it never dispatches replacement work.
The slot stays reserved while the outcome remains uncertain.
Had `vmagent` restarted meanwhile, X1 would stay uncertain rather than relaunch.

### Walkthrough 3: agent-plane restarts during execution

<!-- draw-visual: diagrams/3-detached-commands-walkthrough-3.mmd -->
```text
      ┌─────────────┐     ┌────────────┐     ┌─────────┐     ┌─────────┐
      │ agent-plane │     │ PostgreSQL │     │ vmagent │     │ Command │
      └──────┬──────┘     └──────┬─────┘     └────┬────┘     └────┬────┘
             │                   │                │               │
            ┌──────────────────────────────────────────────────────┐
            │   agent-plane stops, collection pauses at cursor 3   │
            └──────────────────────────────────────────────────────┘
             │                   │                │               │
             │                   │                │ 1. output 4..9│
             │                   │                │◄──────────────┤
             │                   │                │               │
             │                   │                │ 2. exit 0     │
             │                   │                │◄──────────────┤
             │                   │                │               │
             │                   │     ┌──────────────────────┐   │
             │                   │     │ retention drops 4..5 │   │
             │                   │     └──────────────────────┘   │
             │                   │                │               │
            ┌──────────────────────────────────────────────────────┐
            │                 agent-plane restarts                 │
            └──────────────────────────────────────────────────────┘
             │                   │                │               │
             │ 3. read active actions             │               │
             ├──────────────────►│                │               │
             │                   │                │               │
             │ 4. X1 running, cursor 3            │               │
             │◄──────────────────┤                │               │
             │                   │                │               │
             │ 5. inspect X1     │                │               │
             ├───────────────────────────────────►│               │
             │                   │                │               │
             │ 6. X1 finished: exit 0             │               │
             │◄───────────────────────────────────┤               │
             │                   │                │               │
             │ 7. poll after cursor 3             │               │
             ├───────────────────────────────────►│               │
             │                   │                │               │
             │ 8. gap 4..5, output 6..9           │               │
             │◄───────────────────────────────────┤               │
             │                   │                │               │
┌─────────────────────────┐      │                │               │
│ save output, record gap │      │                │               │
└─────────────────────────┘      │                │               │
             │                   │                │               │
             │ 9. cursor = 9, X1 succeeded        │               │
             ├──────────────────►│                │               │
             │                   │                │               │
```

Collection resumes from the committed cursor, not from memory.
Retention discarded #4 and #5; the record reports that gap explicitly.
The terminal status survives even when some output does not.

## Platform work

- Add command requests through CLI, agent-plane, and the guest HTTP server.
- Reserve one active platform command per guest using stable action identifiers.
- Record guest acceptance, launch evidence, output, and terminal outcomes.
- Collect numbered output independently from attached callers.
- Add action inspection and polling watch through `--watch/-w <vm>`.

Bound output retention immediately; report discarded output as explicit gaps.
Return existing outcomes for matching retries rather than starting new commands.
Keep uncertain launch outcomes explicit and retain their ownership.
Provide basic control-plane restart recovery now, not after later feature work.

## Simulator work

Extend shared decisions across host dispatch, guest admission, and collection.
Reuse guest decision code where simulated safety claims concern guest behavior.
Model accepted requests, lost acknowledgments, launch gaps, and delayed output.
Allow guest work to finish while the control plane remains unavailable.
Check collected positions against durable output and preserve uncertain reservations.

The model cannot establish real process creation or journal durability.
Test those boundaries through actual children, files, and controlled crashes.

### Simulator walkthrough

Decisions now run on both sides, so the simulator drives both.
Guest work can complete while the script keeps the control plane unavailable.

```text
 guest durable output    #1  #2  #3  #4  #5  #6  [final status]
                                  ^
 host collected cursor  ----------+     collected through #3

 checked after every step:
   the cursor never passes durable guest output
   discarded output appears as a reported gap, never as silence
   an uncertain launch keeps its command slot
```

This scenario combines walkthroughs 2 and 3 without a guest or database.

<!-- draw-visual: diagrams/3-detached-commands-simulator.mmd -->
```text
┌────────┐     ┌───────┐     ┌─────────────┐     ┌──────────────┐     ┌──────────┐     ┌────────┐
│ Script │     │ Queue │     │ Host decide │     │ Guest decide │     │ Adapters │     │ Checks │
└────┬───┘     └───┬───┘     └──────┬──────┘     └───────┬──────┘     └─────┬────┘     └────┬───┘
     │             │                │                    │                  │               │
     │ 1. exec X1  │                │                    │                  │               │
     ├────────────►│                │                    │                  │               │
     │             │                │                    │                  │               │
     │             │ 2. deliver X1  │                    │                  │               │
     │             ├───────────────►│                    │                  │               │
     │             │                │                    │                  │               │
     │             │                │ 3. effect: reserve slot               │               │
     │             │                ├──────────────────────────────────────►│               │
     │             │                │                    │                  │               │
     │             │                │ 4. effect: dispatch X1                │               │
     │             │                ├──────────────────────────────────────►│               │
     │             │                │                    │                  │               │
     │             │ 5. guest event: X1 arrives          │                  │               │
     │             │◄───────────────────────────────────────────────────────┤               │
     │             │                │                    │                  │               │
     │             │ 6. deliver X1 arrives               │                  │               │
     │             ├────────────────────────────────────►│                  │               │
     │             │                │                    │                  │               │
     │             │                │                    │ 7. journal+run   │               │
     │             │                │                    ├─────────────────►│               │
     │             │                │                    │                  │               │
     │ 8. fault: lose the acceptance reply               │                  │               │
     ├─────────────────────────────────────────────────────────────────────►│               │
     │             │                │                    │                  │               │
    ┌────────────────────────────────────────────────────────────────────────────────────────┐
    │                       scripted outage: control plane unavailable                       │
    └────────────────────────────────────────────────────────────────────────────────────────┘
     │             │                │                    │                  │               │
     │             │ 9. guest event: output, then exit 0 │                  │               │
     │             │◄───────────────────────────────────────────────────────┤               │
     │             │                │                    │                  │               │
     │             │ 10. deliver exit 0                  │                  │               │
     │             ├────────────────────────────────────►│                  │               │
     │             │                │                    │                  │               │
     │             │                │                    │ 11. journal exit │               │
     │             │                │                    ├─────────────────►│               │
     │             │                │                    │                  │               │
    ┌────────────────────────────────────────────────────────────────────────────────────────┐
    │                    scripted restart: rebuild from committed records                    │
    └────────────────────────────────────────────────────────────────────────────────────────┘
     │             │                │                    │                  │               │
     │             │   ┌─────────────────────────┐       │                  │               │
     │             │   │ X1 uncertain, slot kept │       │                  │               │
     │             │   └─────────────────────────┘       │                  │               │
     │             │                │                    │                  │               │
     │             │                │ 12. effect: inspect X1                │               │
     │             │                ├──────────────────────────────────────►│               │
     │             │                │                    │                  │               │
     │             │ 13. completion: X1 finished         │                  │               │
     │             │◄───────────────────────────────────────────────────────┤               │
     │             │                │                    │                  │               │
     │             │ 14. deliver done                    │                  │               │
     │             ├───────────────►│                    │                  │               │
     │             │                │                    │                  │               │
     │             │                │ 15. state + effects│                  │               │
     │             │                ├──────────────────────────────────────────────────────►│
     │             │                │                    │                  │               │
     │             │                │                    │                  ┌───────────────────────────────┐
     │             │                │                    │                  │ one execution? honest cursor? │
     │             │                │                    │                  └───────────────────────────────┘
     │             │                │                    │                  │               │
```

The simulated journal always survives; real file durability needs real crash tests.
Real children, files, and controlled crashes must confirm these adapter assumptions.

## Demonstration and evidence

Run a real command, detach watch, and reconnect without restarting execution.
Restart the control plane during execution and recover the original action.
Lose acceptance acknowledgment and inspect the original identifier safely.
Interrupt output collection; recover ordered output or explain retention gaps.
Show separate commands progressing across both guests.

## Discuss before expansion

Which launch evidence permits inspection but forbids automatic replay?
How do output limits interact with slow or disconnected collectors?
What should watch display when a command's outcome remains uncertain?

Astra leads guest admission and output contracts; Sol integrates dispatch and watch.
Terra supplies process boundaries; Astra owns simulator and real-process testing.
The lead reviews detach and restart behavior before acceptance.

Interactive terminals remain SSH's responsibility.
Files, input, explicit cancellation, and richer deadlines arrive in slice 4.
Reference scope: old M4 and basic M6 watch, with essential M8 safety.

