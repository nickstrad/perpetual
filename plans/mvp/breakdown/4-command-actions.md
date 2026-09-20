# 4. Files, input, and command control

Requires [slice 3](3-detached-commands.md).
See the [cumulative architecture](4-command-actions_architecture.md).

## Actors and actions

An actor is a module or entity that performs actions within this slice.
Earlier actors remain; this table lists who acts on files, input, and cancellation.

| Platform actor | Where it lives | Actions in this slice |
| --- | --- | --- |
| Terminal | Operator's shell | Writes files, sends input, reads results, cancels unwanted work |
| `perpetual` CLI | Host, short-lived process | Sends bounded reads and writes, sequenced input, end-of-input, cancellation, optional timeout |
| `agent-plane` | Host, single long-lived service | Reserves write identifiers; routes file actions beside a running command; enforces limits; keeps command ownership until verified termination |
| Action decisions | Shared code, host and guest | Order input, settle cancel-versus-exit races, apply deadlines, keep uncertainty explicit |
| PostgreSQL | Host, independent service | Keeps action records, limits, uncertain deliveries, and cursors |
| `vmagent` files | Inside each VM | Reads within bounds; stages, publishes, and journals each file replacement |
| `vmagent` runner | Inside each VM | Feeds sequenced input; closes input; enforces deadlines; terminates and verifies the process group |
| Command | Inside each VM, process group | Reads standard input; may exit naturally during a cancellation |
| SSH session | Terminal to guest, outside the platform | May edit the same files unseen by the journal |

| Simulator actor | Replaces | Actions in this slice |
| --- | --- | --- |
| Seeded chooser | Hand-ordered scripts, once those pass | Picks one valid pending event per step from a repeatable seed |
| Event queue | Arrival order on host and guest | Offers only causally valid events; enforces step and time limits |
| Action decisions | Nothing; identical production code | Decide exactly as production does |
| Simulated guest | Runner, pipes, files, process groups | Completes, loses acknowledgments, and leaves file publication gaps |
| Invariant checks | A reader's careful review | Assert one terminal outcome, no automatic resend, held ownership |
| Trace recorder | Scattered logs | Stores the seed and every actual choice for replay |

## Point of this slice

A caller supplies files, interacts with running work, and explicitly ends unwanted commands.
Standard input carries bytes into a command.
Cancellation requests termination; a timeout sets an execution deadline.

## Walkthroughs

These walkthroughs illustrate planned behavior; nothing here exists yet.
Discussion questions below may still change individual steps.
Notation follows [slice 1](1-durable-requests.md#walkthroughs).
W1 and X1 name action identifiers; `#n` numbers one input message.

### Walkthrough 1: write a file, run a command, read results

<!-- draw-visual: diagrams/4-command-actions-walkthrough-1.mmd -->
```text
┌──────────┐     ┌─────┐     ┌─────────────┐     ┌────────────┐     ┌─────────┐     ┌─────────┐
│ Terminal │     │ CLI │     │ agent-plane │     │ PostgreSQL │     │ vmagent │     │ Command │
└─────┬────┘     └──┬──┘     └──────┬──────┘     └──────┬─────┘     └────┬────┘     └────┬────┘
      │             │               │                   │                │               │
      │ 1. write file               │                   │                │               │
      ├────────────►│               │                   │                │               │
      │             │               │                   │                │               │
      │             │ 2. write W1   │                   │                │               │
      │             ├──────────────►│                   │                │               │
      │             │               │                   │                │               │
      │             │               │ 3. save W1        │                │               │
      │             │               ├──────────────────►│                │               │
      │             │               │                   │                │               │
      │             │               │ 4. write W1, bounded               │               │
      │             │               ├───────────────────────────────────►│               │
      │             │               │                   │                │               │
      │             │               │                   │   ┌─────────────────────────┐  │
      │             │               │                   │   │ stage, publish, journal │  │
      │             │               │                   │   └─────────────────────────┘  │
      │             │               │                   │                │               │
      │             │               │ 5. W1 done        │                │               │
      │             │               │◄───────────────────────────────────┤               │
      │             │               │                   │                │               │
      │             │               │ 6. W1 done        │                │               │
      │             │               ├──────────────────►│                │               │
      │             │               │                   │                │               │
      │             │ 7. W1 done    │                   │                │               │
      │             │◄──────────────┤                   │                │               │
      │             │               │                   │                │               │
      │ 8. exec on a│               │                   │                │               │
      ├────────────►│               │                   │                │               │
      │             │               │                   │                │               │
      │             │ 9. command X1 │                   │                │               │
      │             ├──────────────►│                   │                │               │
      │             │               │                   │                │               │
      │             │               │ 10. reserve slot  │                │               │
      │             │               ├──────────────────►│                │               │
      │             │               │                   │                │               │
      │             │               │ 11. dispatch X1   │                │               │
      │             │               ├───────────────────────────────────►│               │
      │             │               │                   │                │               │
      │             │               │                   │                │ 12. launch    │
      │             │               │                   │                ├──────────────►│
      │             │               │                   │                │               │
      │             │               │                   │                │  ┌─────────────────────────┐
      │             │               │                   │                │  │ use file, write results │
      │             │               │                   │                │  └─────────────────────────┘
      │             │               │                   │                │               │
     ┌────────────────────────────────────────────────────────────────────────────────────┐
     │                     X1 still runs, file actions stay available                     │
     └────────────────────────────────────────────────────────────────────────────────────┘
      │             │               │                   │                │               │
      │ 13. read file               │                   │                │               │
      ├────────────►│               │                   │                │               │
      │             │               │                   │                │               │
      │             │ 14. bounded read                  │                │               │
      │             ├──────────────►│                   │                │               │
      │             │               │                   │                │               │
      │             │               │ 15. read within limit              │               │
      │             │               ├───────────────────────────────────►│               │
      │             │               │                   │                │               │
      │             │               │ 16. bounded bytes │                │               │
      │             │               │◄───────────────────────────────────┤               │
      │             │               │                   │                │               │
      │             │ 17. bytes     │                   │                │               │
      │             │◄──────────────┤                   │                │               │
      │             │               │                   │                │               │
      │ 18. print bytes             │                   │                │               │
      │◄────────────┤               │                   │                │               │
      │             │               │                   │                │               │
```

The write gets its identifier before any bytes travel.
A retried W1 returns the journaled outcome without writing again.
Reads and writes stay bounded; large transfers belong to SSH tools.

### Walkthrough 2: send input, lose an acknowledgment, close input

<!-- draw-visual: diagrams/4-command-actions-walkthrough-2.mmd -->
```text
┌─────┐     ┌─────────────┐     ┌────────────┐     ┌─────────┐     ┌─────────┐
│ CLI │     │ agent-plane │     │ PostgreSQL │     │ vmagent │     │ Command │
└──┬──┘     └──────┬──────┘     └──────┬─────┘     └────┬────┘     └────┬────┘
   │               │                   │                │               │
   │ 1. input 1 for X1                 │                │               │
   ├──────────────►│                   │                │               │
   │               │                   │                │               │
   │               │ 2. input 1        │                │               │
   │               ├───────────────────────────────────►│               │
   │               │                   │                │               │
   │               │                   │                │ 3. stdin bytes│
   │               │                   │                ├──────────────►│
   │               │                   │                │               │
   │               │ 4. ack 1          │                │               │
   │               │◄───────────────────────────────────┤               │
   │               │                   │                │               │
   │ 5. 1 delivered│                   │                │               │
   │◄──────────────┤                   │                │               │
   │               │                   │                │               │
   │ 6. input 2    │                   │                │               │
   ├──────────────►│                   │                │               │
   │               │                   │                │               │
   │               │ 7. input 2        │                │               │
   │               ├───────────────────────────────────►│               │
   │               │                   │                │               │
   │               │                   │                │ 8. stdin bytes│
   │               │                   │                ├──────────────►│
   │               │                   │                │               │
   │               │ 9. ack 2          │                │               │
   │               │×───────────────────────────────────┤               │
   │               │                   │                │               │
   │               │ 10. 2 uncertain   │                │               │
   │               ├──────────────────►│                │               │
   │               │                   │                │               │
   │ 11. 2 uncertain                   │                │               │
   │◄──────────────┤                   │                │               │
   │               │                   │                │               │
   │ ┌────────────────────────────┐    │                │               │
   │ │ never resend automatically │    │                │               │
   │ └────────────────────────────┘    │                │               │
   │               │                   │                │               │
   │ 12. end of input                  │                │               │
   ├──────────────►│                   │                │               │
   │               │                   │                │               │
   │               │ 13. close input   │                │               │
   │               ├───────────────────────────────────►│               │
   │               │                   │                │               │
   │               │                   │                │ 14. close stdin
   │               │                   │                ├──────────────►│
   │               │                   │                │               │
   │               │ 15. input closed  │                │               │
   │               │◄───────────────────────────────────┤               │
   │               │                   │                │               │
   │ 16. input closed                  │                │               │
   │◄──────────────┤                   │                │               │
   │               │                   │                │               │
```

Resending #2 automatically could deliver the same bytes twice.
Which acknowledgments prove delivery remains a discussion question below.
Closing input is an explicit action, never a side effect of detaching.

### Walkthrough 3: cancellation races natural completion

<!-- draw-visual: diagrams/4-command-actions-walkthrough-3.mmd -->
```text
┌─────┐     ┌─────────────┐     ┌────────────┐     ┌─────────┐     ┌─────────┐
│ CLI │     │ agent-plane │     │ PostgreSQL │     │ vmagent │     │ Command │
└──┬──┘     └──────┬──────┘     └──────┬─────┘     └────┬────┘     └────┬────┘
   │               │                   │                │               │
   │ 1. cancel X1  │                   │                │               │
   ├──────────────►│                   │                │               │
   │               │                   │                │               │
   │               │                   │                │ 2. exit 0     │
   │               │                   │                │◄──────────────┤
   │               │                   │                │               │
   │               │                   │       ┌─────────────────┐      │
   │               │                   │       │ journal: exit 0 │      │
   │               │                   │       └─────────────────┘      │
   │               │                   │                │               │
   │               │ 3. cancel X1      │                │               │
   │               ├───────────────────────────────────►│               │
   │               │                   │                │               │
   │               │ 4. already finished: exit 0        │               │
   │               │◄───────────────────────────────────┤               │
   │               │                   │                │               │
   │               │ 5. X1 succeeded   │                │               │
   │               ├──────────────────►│                │               │
   │               │                   │                │               │
   │ 6. outcome: exit 0                │                │               │
   │◄──────────────┤                   │                │               │
   │               │                   │                │               │
  ┌──────────────────────────────────────────────────────────────────────┐
  │                other order: the cancel arrives first                 │
  └──────────────────────────────────────────────────────────────────────┘
   │               │                   │                │               │
   │ 7. cancel X1  │                   │                │               │
   ├──────────────►│                   │                │               │
   │               │                   │                │               │
   │               │ 8. cancel X1      │                │               │
   │               ├───────────────────────────────────►│               │
   │               │                   │                │               │
   │               │                   │                │ 9. kill group │
   │               │                   │                ├──────────────►│
   │               │                   │                │               │
   │               │                   │                │ 10. group gone│
   │               │                   │                │◄──────────────┤
   │               │                   │                │               │
   │               │                   │      ┌────────────────────┐    │
   │               │                   │      │ journal: cancelled │    │
   │               │                   │      └────────────────────┘    │
   │               │                   │                │               │
   │               │ 11. cancelled, verified            │               │
   │               │◄───────────────────────────────────┤               │
   │               │                   │                │               │
   │               │ 12. X1 cancelled  │                │               │
   │               ├──────────────────►│                │               │
   │               │                   │                │               │
   │ 13. outcome: cancelled            │                │               │
   │◄──────────────┤                   │                │               │
   │               │                   │                │               │
```

Either order records exactly one terminal outcome, and both callers see it.
The command slot stays owned until termination is verified.

## Platform work

- Add bounded file reads and journaled file replacement.
- Add sequenced input and explicit end-of-input handling.
- Add cancellation, execution deadlines, and verified process-group termination.
- Enforce per-action and aggregate resource limits.
- Keep file operations available while a platform command runs.

Input acknowledgment loss must not trigger automatic resend.
File replacement recovery must account for manual SSH edits.
Keep command ownership until termination or explicit uncertainty resolution.

## Simulator work

A seed initializes a repeatable sequence of generated choices.
Add bounded seeded choices among valid pending events after handwritten scenarios work.
Exercise cancellation versus completion, input acknowledgment loss, and file publication gaps.
Reuse the existing trace recorder; preserve actual event choices alongside seeds.
Enforce causal ordering, event limits, and logical-time limits.

No automatic trace reducer or generic scenario language is required.
Keep useful failures as short, readable regression scenarios.

### Simulator walkthrough

Handwritten scripts come first; a seeded chooser then explores competing orders.
The chooser never invents events; it only orders what is already pending.

```text
 pending events, all causally valid          chooser             recorded choice
+----------------------------------+
| 1. X1 exits with status 0        |
| 2. cancel X1 reaches the guest   | ---- seed 42 picks ---->  2. cancel X1
| 3. acknowledgment for input #2   |
+----------------------------------+
 then it picks again among what remains, within step and time limits

 the trace keeps the seed and every choice, so replay needs no generator
```

The same three events produce different stories under different seeds.

```text
 seed 42:  cancel --> exit 0 --> ack lost     outcome: whichever the guest recorded first
 seed 7:   exit 0 --> cancel --> ack lost     outcome: exit 0; cancel returns that outcome
 seed 19:  ack lost --> exit 0 --> cancel     input #2 stays uncertain in every order
```

This scenario follows seed 42.

<!-- draw-visual: diagrams/4-command-actions-simulator.mmd -->
```text
┌─────────┐     ┌───────┐     ┌───────────┐     ┌───────────┐     ┌──────────────┐
│ Chooser │     │ Queue │     │ Decisions │     │ Sim guest │     │ Checks+trace │
└────┬────┘     └───┬───┘     └─────┬─────┘     └─────┬─────┘     └───────┬──────┘
     │              │               │                 │                   │
     ┌──────────────────────────────┐                 │                   │
     │ pending: exit, cancel, ack 2 │                 │                   │
     └──────────────────────────────┘                 │                   │
     │              │               │                 │                   │
     │ 1. seed 42: cancel           │                 │                   │
     ├─────────────►│               │                 │                   │
     │              │               │                 │                   │
     │              │ 2. deliver cancel               │                   │
     │              ├──────────────►│                 │                   │
     │              │               │                 │                   │
     │              │               │ 3. effect: kill group               │
     │              │               ├────────────────►│                   │
     │              │               │                 │                   │
     │ 4. next: exit 0              │                 │                   │
     ├─────────────►│               │                 │                   │
     │              │               │                 │                   │
     │              │ 5. deliver exit 0               │                   │
     │              ├──────────────►│                 │                   │
     │              │               │                 │                   │
     │              ┌───────────────────────────────┐ │                   │
     │              │ keep the one recorded outcome │ │                   │
     │              └───────────────────────────────┘ │                   │
     │              │               │                 │                   │
     │ 6. fault: lose ack 2         │                 │                   │
     ├───────────────────────────────────────────────►│                   │
     │              │               │                 │                   │
     │              │ 7. deliver timeout              │                   │
     │              ├──────────────►│                 │                   │
     │              │               │                 │                   │
     │              │┌──────────────────────────────┐ │                   │
     │              ││ input 2 uncertain, no resend │ │                   │
     │              │└──────────────────────────────┘ │                   │
     │              │               │                 │                   │
     │              │               │ 8. state + effects                  │
     │              │               ├────────────────────────────────────►│
     │              │               │                 │                   │
     │              │               │                 │      ┌─────────────────────────┐
     │              │               │                 │      │ one outcome? slot held? │
     │              │               │                 │      └─────────────────────────┘
     │              │               │                 │                   │
     │              │               │                 │     ┌────────────────────────────┐
     │              │               │                 │     │ trace: seed + every choice │
     │              │               │                 │     └────────────────────────────┘
     │              │               │                 │                   │
```

A failing seed becomes a short handwritten regression scenario.
Generated schedules cover modeled events, not arbitrary goroutine interleavings.

## Demonstration and evidence

Write a file, execute a command using it, and retrieve bounded results.
Send input, close input, and cancel another command while inspecting actual children.
Prove timeout and output limits do not silently discard terminal status.
Replay competing events and explain each retained uncertain outcome.
Test actual rename, synchronization, pipe, and process-group boundaries.

## Discuss before expansion

Which input acknowledgments establish delivery, and which remain ambiguous?
How should manual SSH changes constrain file recovery?
Which competing events justify randomized ordering beyond fixed scenarios?

Terra leads file and process integration; Sol extends request paths.
Astra owns ordering contracts, simulator generation, and all corresponding tests.
The lead reviews bounds and uncertainty before acceptance.

Broader cross-component recovery follows in slice 5.
Reference scope: old M5 and remaining command-facing CLI integration.

