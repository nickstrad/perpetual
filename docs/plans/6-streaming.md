# 6. Optional streaming without changing execution

Requires [slice 5](5-recovery.md).
See the [cumulative architecture](6-streaming_architecture.md).

## Actors and actions

An actor is a module or entity that performs actions within this slice.
Only the host-to-guest connection changes; every other actor behaves as before.

| Platform actor | Where it lives | Actions in this slice |
| --- | --- | --- |
| Terminal and CLI | Operator's shell and host process | Unchanged; watch still polls `agent-plane` through request/response |
| `agent-plane` collector | Host, inside `agent-plane` | Attaches a WebSocket to an existing action; falls back in automatic mode; reports errors in explicit mode; tracks upgraded connections during shutdown |
| Transport decisions | Shared code, host and guest | Keep one ordering and one cursor whatever carries the bytes |
| PostgreSQL | Host, independent service | Keeps transport-neutral cursors and action records |
| `vmagent` HTTP paths | Inside each VM | Keep serving polling exactly as before |
| `vmagent` stream path | Inside each VM | Upgrades one connection; attaches by action identifier; never starts or cancels work |
| Command | Inside each VM, process group | Runs unaware of whichever transport carries its output |
| Configured certificates | Host and guest configuration | Let HTTPS and secure WebSockets verify the configured peer |

| Simulator actor | Replaces | Actions in this slice |
| --- | --- | --- |
| Seeded chooser | Hand-ordered scripts, once those pass | Switches transports, duplicates deliveries, drops connections, delays final status |
| Event queue | Arrival order on host and guest | Orders deliveries from both transports |
| Transport decisions | Nothing; identical production code | Decide exactly as production does |
| Simulated transport | Polling and streaming delivery | Delivers, duplicates, or drops; holds no durable state |
| Invariant checks | A reader's careful review | Assert identical retained outcomes across transport schedules |
| Trace recorder | Scattered logs | Replays mixed-transport runs in the existing format |

## Point of this slice

Long commands can stream output while keeping ordinary request/response available.
A WebSocket upgrades one HTTP connection for ongoing messages in both directions.
HTTPS encrypts HTTP; secure WebSockets use its encrypted connection.

## Walkthroughs

These walkthroughs illustrate planned behavior; nothing here exists yet.
Discussion questions below may still change individual steps.
Notation follows [slice 1](durable_requests/README.md#walkthroughs).
X1 names the command; `#n` means numbered output position n.

### Walkthrough 1: automatic mode streams, drops, then resumes by polling

<!-- draw-visual: diagrams/6-streaming-walkthrough-1.mmd -->
```text
┌─────┐     ┌─────────────┐     ┌────────────┐     ┌─────────┐     ┌─────────┐
│ CLI │     │ agent-plane │     │ PostgreSQL │     │ vmagent │     │ Command │
└──┬──┘     └──────┬──────┘     └──────┬─────┘     └────┬────┘     └────┬────┘
   │               │                   │                │               │
   │ 1. command X1 │                   │                │               │
   ├──────────────►│                   │                │               │
   │               │                   │                │               │
   │               │ 2. dispatch X1    │                │               │
   │               ├───────────────────────────────────►│               │
   │               │                   │                │               │
   │               │                   │                │ 3. launch     │
   │               │                   │                ├──────────────►│
   │               │                   │                │               │
   │               │ 4. upgrade: attach X1 after 0      │               │
   │               ├───────────────────────────────────►│               │
   │               │                   │                │               │
   │               │ 5. upgrade accepted                │               │
   │               │◄───────────────────────────────────┤               │
   │               │                   │                │               │
   │               │                   │                │ 6. 1..3       │
   │               │                   │                │◄──────────────┤
   │               │                   │                │               │
   │               │ 7. stream 1..3    │                │               │
   │               │◄───────────────────────────────────┤               │
   │               │                   │                │               │
   │      ┌──────────────────┐         │                │               │
   │      │ save host output │         │                │               │
   │      └──────────────────┘         │                │               │
   │               │                   │                │               │
   │               │ 8. cursor = 3     │                │               │
   │               ├──────────────────►│                │               │
   │               │                   │                │               │
  ┌──────────────────────────────────────────────────────────────────────┐
  │                    stream drops, X1 keeps running                    │
  └──────────────────────────────────────────────────────────────────────┘
   │               │                   │                │               │
   │               │                   │                │ 9. 4..5       │
   │               │                   │                │◄──────────────┤
   │               │                   │                │               │
   │               │ 10. poll after cursor 3            │               │
   │               ├───────────────────────────────────►│               │
   │               │                   │                │               │
   │               │ 11. 4..5          │                │               │
   │               │◄───────────────────────────────────┤               │
   │               │                   │                │               │
   │               │ 12. cursor = 5    │                │               │
   │               ├──────────────────►│                │               │
   │               │                   │                │               │
   │ 13. watch X1  │                   │                │               │
   ├──────────────►│                   │                │               │
   │               │                   │                │               │
   │ 14. 1..5 in order                 │                │               │
   │◄──────────────┤                   │                │               │
   │               │                   │                │               │
```

The cursor never recorded which transport delivered each position.
Attachment, loss, and fallback touched the connection, never the command.

### Walkthrough 2: the guest rejects the upgrade

<!-- draw-visual: diagrams/6-streaming-walkthrough-2.mmd -->
```text
┌─────┐     ┌─────────────┐     ┌─────────┐     ┌─────────┐
│ CLI │     │ agent-plane │     │ vmagent │     │ Command │
└──┬──┘     └──────┬──────┘     └────┬────┘     └────┬────┘
   │               │                 │               │
   │               │ 1. upgrade: attach X1           │
   │               ├────────────────►│               │
   │               │                 │               │
   │               │ 2. upgrade rejected             │
   │               │◄────────────────┤               │
   │               │                 │               │
   │ ┌───────────────────────────┐   │               │
   │ │ automatic mode: fall back │   │               │
   │ └───────────────────────────┘   │               │
   │               │                 │               │
   │               │ 3. poll after cursor 0          │
   │               ├────────────────►│               │
   │               │                 │               │
   │               │ 4. 1..3         │               │
   │               │◄────────────────┤               │
   │               │                 │               │
  ┌───────────────────────────────────────────────────┐
  │      same rejection, explicit streaming mode      │
  └───────────────────────────────────────────────────┘
   │               │                 │               │
   │               │ 5. upgrade: attach X1           │
   │               ├────────────────►│               │
   │               │                 │               │
   │               │ 6. upgrade rejected             │
   │               │◄────────────────┤               │
   │               │                 │               │
   │     ┌───────────────────┐       │               │
   │     │ no silent polling │       │               │
   │     └───────────────────┘       │               │
   │               │                 │               │
   │ 7. inspect X1 │                 │               │
   ├──────────────►│                 │               │
   │               │                 │               │
   │ 8. running, transport error     │               │
   │◄──────────────┤                 │               │
   │               │                 │               │
   │               │                 │┌──────────────────────────────┐
   │               │                 ││ never restarted or cancelled │
   │               │                 │└──────────────────────────────┘
   │               │                 │               │
```

Automatic mode hides the rejection behind working polling.
Explicit mode reports the rejection; it never silently changes transport.

## Platform work

- Attach host-to-guest WebSockets to existing action identifiers.
- Share output ordering and input semantics with ordinary HTTP paths.
- In automatic mode, fall back after rejected upgrades or interrupted streams.
- Add configured HTTPS and secure WebSocket support.
- Track upgraded connections explicitly during service shutdown.

The CLI continues request/response with agent-plane, including watch.
Explicit streaming mode reports transport errors rather than silently choosing polling.
Stream attachment, reconnection, and closure never start or cancel command execution.
SSH remains the interactive terminal path.

## Simulator work

Model transport delivery separately from durable action and output state.
Switch delivery between polling and streaming within an existing execution.
Duplicate output delivery, drop connections, and delay final status.
Check identical retained outcomes across transport schedules.
Keep uncertainty rules for input rather than promising automatic retransmission.

Do not simulate encryption or implement a second wire protocol inside the harness.
Real local servers must verify upgrades, certificates, framing, and connection shutdown.

### Simulator walkthrough

Transport delivery lives apart from durable action and output state.
The same execution must retain the same outcome under every transport schedule.

```text
 durable guest output        #1  #2  #3  #4  #5  [final]

 schedule P, polling only    [#1 #2]      [#3 #4]      [#5 final]
 schedule M, mixed           ~#1 #2 #2~   X dropped    [#3 #4 #5]   ~final, late~

 retained host outcome       #1  #2  #3  #4  #5  [final]     identical for P and M

 [ ] polled response     ~ ~ streamed messages     X connection dropped
```

This scenario follows schedule M.

<!-- draw-visual: diagrams/6-streaming-simulator.mmd -->
```text
┌─────────┐     ┌───────┐     ┌───────────┐     ┌───────────────┐     ┌──────────────┐
│ Chooser │     │ Queue │     │ Decisions │     │ Sim transport │     │ Checks+trace │
└────┬────┘     └───┬───┘     └─────┬─────┘     └───────┬───────┘     └───────┬──────┘
     │              │               │                   │                     │
     │              │               │ 1. effect: attach stream                │
     │              │               ├──────────────────►│                     │
     │              │               │                   │                     │
     │              │ 2. delivery: 1..2 streamed        │                     │
     │              │◄──────────────────────────────────┤                     │
     │              │               │                   │                     │
     │              │ 3. deliver 1..2                   │                     │
     │              ├──────────────►│                   │                     │
     │              │               │                   │                     │
     │ 4. fault: duplicate 2        │                   │                     │
     ├─────────────────────────────────────────────────►│                     │
     │              │               │                   │                     │
     │              │ 5. delivery: 2 again              │                     │
     │              │◄──────────────────────────────────┤                     │
     │              │               │                   │                     │
     │              │ 6. deliver 2  │                   │                     │
     │              ├──────────────►│                   │                     │
     │              │               │                   │                     │
     │             ┌──────────────────────────────────┐ │                     │
     │             │ ignore duplicate, cursor stays 2 │ │                     │
     │             └──────────────────────────────────┘ │                     │
     │              │               │                   │                     │
     │ 7. fault: drop connection    │                   │                     │
     ├─────────────────────────────────────────────────►│                     │
     │              │               │                   │                     │
     │              │               │ 8. effect: poll after 2                 │
     │              │               ├──────────────────►│                     │
     │              │               │                   │                     │
     │              │ 9. delivery: 3..5 polled          │                     │
     │              │◄──────────────────────────────────┤                     │
     │              │               │                   │                     │
     │              │ 10. deliver 3..5                  │                     │
     │              ├──────────────►│                   │                     │
     │              │               │                   │                     │
     │ 11. fault: delay final status│                   │                     │
     ├─────────────────────────────────────────────────►│                     │
     │              │               │                   │                     │
     │              │ 12. delivery: final, late         │                     │
     │              │◄──────────────────────────────────┤                     │
     │              │               │                   │                     │
     │              │ 13. deliver final                 │                     │
     │              ├──────────────►│                   │                     │
     │              │               │                   │                     │
     │              │               │ 14. state + effects                     │
     │              │               ├────────────────────────────────────────►│
     │              │               │                   │                     │
     │              │               │                   │     ┌───────────────────────────────┐
     │              │               │                   │     │ same outcome as polling only? │
     │              │               │                   │     └───────────────────────────────┘
     │              │               │                   │                     │
```

The harness models delivery only; it implements no second wire protocol.
Real local servers must verify upgrades, certificates, framing, and shutdown.

## Demonstration and evidence

Run equivalent command scenarios through polling and streaming.
Interrupt a stream and resume collection without duplicate execution or hidden gaps.
Reject an automatic-mode upgrade and observe successful polling fallback.
Verify explicit streaming reports that rejection without restarting or cancelling execution.
Validate configured trust and shutdown with actual HTTP/HTTPS servers.
Replay a mixed-transport scenario using the existing trace format.

## Discuss before expansion

What qualifies a command for streaming rather than polling?
How will collectors avoid conflicting ownership while switching transports?
Which certificate and connection failures should trigger fallback or explicit errors?

Production owners implement transport integration and configured encrypted transport boundaries.
The test owner maintains equivalence checks, simulator scenarios, and real-server testing; assign models when expanding this slice.
The lead validates that transport changes preserve execution semantics.

Certificate issuance and broader security hardening remain outside this slice.
Reference scope: old M7.
