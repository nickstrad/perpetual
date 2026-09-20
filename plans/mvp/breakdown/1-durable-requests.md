# 1. Durable requests and the first simulator

Read the [roadmap](README.md) and [cumulative architecture](1-durable-requests_architecture.md).
Starting point: the existing boot prototype; no platform components exist yet.

## Actors and actions

An actor is a module or entity that performs actions within this slice.
The terminal stands for whoever types commands; the platform never sees a person.

| Platform actor | Where it lives | Actions in this slice |
| --- | --- | --- |
| Terminal | Operator's shell | Runs `perpetual`; repeats a command when no reply arrives |
| `perpetual` CLI | Host, short-lived process | Chooses the request identifier; sends register and inspect; prints records |
| `agent-plane` | Host, single long-lived service | Accepts HTTP requests; turns them into events; performs effects; replies |
| Admission decisions | Shared code inside `agent-plane` | Classify a request as new, matching retry, or conflict; emit reservation effects |
| PostgreSQL adapter | Inside `agent-plane` | Runs bounded transactions; reports committed, aborted, or unknown outcomes |
| PostgreSQL | Host, independent service | Keeps machine intent and request ownership across every restart |

| Simulator actor | Replaces | Actions in this slice |
| --- | --- | --- |
| Scenario script | Terminal, CLI, and real timing | Lists events, faults, and restarts in one explicit order |
| Event queue | HTTP arrival and completion order | Delivers exactly one scripted event per step |
| Admission decisions | Nothing; identical production code | Decide exactly as production does |
| Simulated store | PostgreSQL and its adapter | Keeps committed records apart from memory lost at restart |
| Invariant checks | A reader's careful review | Assert one identity, unchanged parameters, and rejected conflicts |
| Trace recorder | Scattered logs | Writes readable steps; replay compares normalized traces |

## Point of this slice

A caller registers machine intent and retrieves the same record after retrying.
Registration records requested configuration; it does not claim a machine already exists.
This gives the simulator real production behavior before virtualization complicates debugging.

## Walkthroughs

These walkthroughs illustrate planned behavior; nothing here exists yet.
Discussion questions below may still change individual steps.
Read each diagram downward; every column is one actor.
An arrow carries one numbered action from its sender to its receiver.
A cross ends a lost message; a wide note marks crashes and restarts.
A note over one actor is work that actor performs alone.
Diagrams are generated; read [diagrams](diagrams/README.md) before changing one.
The PostgreSQL column includes its adapter inside `agent-plane`.
R1 names a request identifier; P and P2 name differing parameters.

### Walkthrough 1: register, then inspect

<!-- draw-visual: diagrams/1-durable-requests-walkthrough-1.mmd -->
```text
┌──────────┐     ┌─────┐     ┌─────────────┐     ┌───────────┐     ┌────────────┐
│ Terminal │     │ CLI │     │ agent-plane │     │ Decisions │     │ PostgreSQL │
└─────┬────┘     └──┬──┘     └──────┬──────┘     └─────┬─────┘     └──────┬─────┘
      │             │               │                  │                  │
      │ 1. register a               │                  │                  │
      ├────────────►│               │                  │                  │
      │             │               │                  │                  │
      │  ┌──────────────────────┐   │                  │                  │
      │  │ choose request id R1 │   │                  │                  │
      │  └──────────────────────┘   │                  │                  │
      │             │               │                  │                  │
      │             │ 2. register R1, P                │                  │
      │             ├──────────────►│                  │                  │
      │             │               │                  │                  │
      │             │               │ 3. event: register R1               │
      │             │               ├─────────────────►│                  │
      │             │               │                  │                  │
      │             │               │ 4. effect: reserve                  │
      │             │               │◄─────────────────┤                  │
      │             │               │                  │                  │
      │             │               │ 5. transaction: reserve R1          │
      │             │               ├────────────────────────────────────►│
      │             │               │                  │                  │
      │             │               │ 6. committed     │                  │
      │             │               │◄────────────────────────────────────┤
      │             │               │                  │                  │
      │             │               │ 7. event: committed                 │
      │             │               ├─────────────────►│                  │
      │             │               │                  │                  │
      │             │               │ 8. reply: registered                │
      │             │               │◄─────────────────┤                  │
      │             │               │                  │                  │
      │             │ 9. record: registered            │                  │
      │             │◄──────────────┤                  │                  │
      │             │               │                  │                  │
      │ 10. print record            │                  │                  │
      │◄────────────┤               │                  │                  │
      │             │               │                  │                  │
      │ 11. inspect a               │                  │                  │
      ├────────────►│               │                  │                  │
      │             │               │                  │                  │
      │             │ 12. inspect a │                  │                  │
      │             ├──────────────►│                  │                  │
      │             │               │                  │                  │
      │             │               │ 13. read record  │                  │
      │             │               ├────────────────────────────────────►│
      │             │               │                  │                  │
      │             │               │ 14. intent only, no machine         │
      │             │               │◄────────────────────────────────────┤
      │             │               │                  │                  │
      │             │ 15. same record                  │                  │
      │             │◄──────────────┤                  │                  │
      │             │               │                  │                  │
      │ 16. print record            │                  │                  │
      │◄────────────┤               │                  │                  │
      │             │               │                  │                  │
```

Decisions never touch the database; they only return effects and replies.
The record says "registered"; it never claims a machine exists.

### Walkthrough 2: lost reply, retry, restart, conflicting reuse

Decision steps match walkthrough 1 and stay hidden until the conflict.

<!-- draw-visual: diagrams/1-durable-requests-walkthrough-2.mmd -->
```text
┌──────────┐     ┌─────┐     ┌─────────────┐     ┌───────────┐     ┌────────────┐
│ Terminal │     │ CLI │     │ agent-plane │     │ Decisions │     │ PostgreSQL │
└─────┬────┘     └──┬──┘     └──────┬──────┘     └─────┬─────┘     └──────┬─────┘
      │             │               │                  │                  │
      │ 1. register a               │                  │                  │
      ├────────────►│               │                  │                  │
      │             │               │                  │                  │
      │             │ 2. register R1, P                │                  │
      │             ├──────────────►│                  │                  │
      │             │               │                  │                  │
      │             │               │ 3. transaction: reserve R1          │
      │             │               ├────────────────────────────────────►│
      │             │               │                  │                  │
      │             │               │ 4. committed     │                  │
      │             │               │◄────────────────────────────────────┤
      │             │               │                  │                  │
      │             │ 5. registered │                  │                  │
      │             │×──────────────┤                  │                  │
      │             │               │                  │                  │
      │ 6. no reply │               │                  │                  │
      │◄────────────┤               │                  │                  │
      │             │               │                  │                  │
      │ 7. retry R1 │               │                  │                  │
      ├────────────►│               │                  │                  │
      │             │               │                  │                  │
      │             │ 8. register R1, P                │                  │
      │             ├──────────────►│                  │                  │
      │             │               │                  │                  │
      │             │               │ 9. look up R1    │                  │
      │             │               ├────────────────────────────────────►│
      │             │               │                  │                  │
      │             │               │ 10. found, P matches                │
      │             │               │◄────────────────────────────────────┤
      │             │               │                  │                  │
      │             │ 11. same record                  │                  │
      │             │◄──────────────┤                  │                  │
      │             │               │                  │                  │
     ┌─────────────────────────────────────────────────────────────────────┐
     │          agent-plane restarts: memory lost, records remain          │
     └─────────────────────────────────────────────────────────────────────┘
      │             │               │                  │                  │
      │ 12. inspect a               │                  │                  │
      ├────────────►│               │                  │                  │
      │             │               │                  │                  │
      │             │ 13. inspect a │                  │                  │
      │             ├──────────────►│                  │                  │
      │             │               │                  │                  │
      │             │               │ 14. read record  │                  │
      │             │               ├────────────────────────────────────►│
      │             │               │                  │                  │
      │             │               │ 15. committed record                │
      │             │               │◄────────────────────────────────────┤
      │             │               │                  │                  │
      │             │ 16. same record                  │                  │
      │             │◄──────────────┤                  │                  │
      │             │               │                  │                  │
      │ 17. reuse R1, P2            │                  │                  │
      ├────────────►│               │                  │                  │
      │             │               │                  │                  │
      │             │ 18. register R1, P2              │                  │
      │             ├──────────────►│                  │                  │
      │             │               │                  │                  │
      │             │               │ 19. event: R1, P2│                  │
      │             │               ├─────────────────►│                  │
      │             │               │                  │                  │
      │             │               │ 20. reply: conflict                 │
      │             │               │◄─────────────────┤                  │
      │             │               │                  │                  │
      │             │ 21. conflict  │                  │                  │
      │             │◄──────────────┤                  │                  │
      │             │               │                  │                  │
```

The retry finds R1 instead of creating a second identity.
Restart loses only memory; inspection still returns the committed record.
Reusing R1 with different parameters fails rather than overwriting intent.

## Platform work

- Connect a minimal CLI, host HTTP service, and PostgreSQL storage.
- Preserve request identifiers and reject conflicting reuse with different parameters.
- Separate shared admission decisions from database effects and their completion events.
- Establish bounded connections, transactions, migrations, and single-host service ownership.
- Recover recorded requests after restarting the control plane.

Keep the prototype untouched until the replacement boot path proves itself.
Expose registration and inspection only; do not publish pretend command execution.

## Simulator work

Build a package-local queue that delivers explicitly ordered events.
Model committed records separately from memory lost during service restart.
Run the same admission decisions that handle real registration requests.
Record readable traces and replay the chosen events without random generation.

First scenario: commit registration, lose its response, retry, restart, then inspect.
Assert one identity, unchanged parameters, and rejection of conflicting retries.
Include pending commits whose outcomes remain unknown after connection loss.
A missing observation must not automatically permit conflicting work.

### Simulator walkthrough

The simulator replaces the terminal, network, and database with scripted substitutes.
Decisions stay untouched; they cannot tell which execution is calling them.
At this level, the script can cut the request path in three places.

```text
 Terminal + CLI ---request---> agent-plane ---reserve---> PostgreSQL
 Terminal + CLI <---reply----- agent-plane <---outcome--- PostgreSQL
                     ^              ^              ^
                     |              |              |
               [reply lost]    [restart:     [connection lost:
                               memory gone]   commit outcome unknown]
```

The first scenario replays walkthrough 2 without a terminal or database.
Every arrow leaving the script is one scripted event or fault.

<!-- draw-visual: diagrams/1-durable-requests-simulator.mmd -->
```text
┌────────┐     ┌─────────────┐     ┌───────────┐     ┌───────────┐     ┌──────────────┐
│ Script │     │ Event queue │     │ Decisions │     │ Sim store │     │ Checks+trace │
└────┬───┘     └──────┬──────┘     └─────┬─────┘     └─────┬─────┘     └───────┬──────┘
     │                │                  │                 │                   │
     │ 1. register R1, P                 │                 │                   │
     ├───────────────►│                  │                 │                   │
     │                │                  │                 │                   │
     │                │ 2. deliver register                │                   │
     │                ├─────────────────►│                 │                   │
     │                │                  │                 │                   │
     │                │                  │ 3. effect: reserve                  │
     │                │                  ├────────────────►│                   │
     │                │                  │                 │                   │
     │                │                  │           ┌───────────┐             │
     │                │                  │           │ commit R1 │             │
     │                │                  │           └───────────┘             │
     │                │                  │                 │                   │
     │                │ 4. completion: committed           │                   │
     │                │◄───────────────────────────────────┤                   │
     │                │                  │                 │                   │
     │                │ 5. deliver committed               │                   │
     │                ├─────────────────►│                 │                   │
     │                │                  │                 │                   │
     │                │                  │ 6. effect: reply│                   │
     │                │                  ├────────────────►│                   │
     │                │                  │                 │                   │
     │ 7. fault: lose that reply         │                 │                   │
     ├────────────────────────────────────────────────────►│                   │
     │                │                  │                 │                   │
     │ 8. retry R1, P │                  │                 │                   │
     ├───────────────►│                  │                 │                   │
     │                │                  │                 │                   │
     │                │ 9. deliver retry │                 │                   │
     │                ├─────────────────►│                 │                   │
     │                │                  │                 │                   │
     │                │                  │ 10. effect: look up                 │
     │                │                  ├────────────────►│                   │
     │                │                  │                 │                   │
     │                │ 11. completion: found, P matches   │                   │
     │                │◄───────────────────────────────────┤                   │
     │                │                  │                 │                   │
     │                │ 12. deliver found│                 │                   │
     │                ├─────────────────►│                 │                   │
     │                │                  │                 │                   │
     │                │      ┌───────────────────────┐     │                   │
     │                │      │ decide: same identity │     │                   │
     │                │      └───────────────────────┘     │                   │
     │                │                  │                 │                   │
    ┌───────────────────────────────────────────────────────────────────────────┐
    │        scripted restart: memory discarded, committed records kept         │
    └───────────────────────────────────────────────────────────────────────────┘
     │                │                  │                 │                   │
     │                │  ┌────────────────────────────────┐│                   │
     │                │  │ rebuild from committed records ││                   │
     │                │  └────────────────────────────────┘│                   │
     │                │                  │                 │                   │
     │ 13. inspect a  │                  │                 │                   │
     ├───────────────►│                  │                 │                   │
     │                │                  │                 │                   │
     │                │ 14. deliver inspect                │                   │
     │                ├─────────────────►│                 │                   │
     │                │                  │                 │                   │
     │ 15. register R1, P2               │                 │                   │
     ├───────────────►│                  │                 │                   │
     │                │                  │                 │                   │
     │                │ 16. deliver R1, P2                 │                   │
     │                ├─────────────────►│                 │                   │
     │                │                  │                 │                   │
     │                │         ┌──────────────────┐       │                   │
     │                │         │ decide: conflict │       │                   │
     │                │         └──────────────────┘       │                   │
     │                │                  │                 │                   │
     │                │                  │ 17. state + effects                 │
     │                │                  ├────────────────────────────────────►│
     │                │                  │                 │                   │
     │                │                  │                 │     ┌────────────────────────────┐
     │                │                  │                 │     │ one identity? P unchanged? │
     │                │                  │                 │     └────────────────────────────┘
     │                │                  │                 │                   │
     │                │                  │                 │       ┌────────────────────────┐
     │                │                  │                 │       │ write trace for replay │
     │                │                  │                 │       └────────────────────────┘
     │                │                  │                 │                   │
```

Checks and trace recording run after every delivered event, not only last.
A second script loses the connection before the commit outcome arrives.
Decisions must then keep R1 pending instead of admitting conflicting work.
Replaying the same script twice must produce the same normalized trace.

## Demonstration and evidence

Show registration and inspection through the actual CLI and PostgreSQL.
Compare simulated reservation outcomes against real concurrent database transactions.
Replay the same scenario twice and compare normalized traces.
Temporarily break an invariant and verify the test detects that specific defect.
Restore correct behavior; retain the detector scenario.
Introduce useful test commands and automated checks with this first behavior.

## Discuss before expansion

Which registration states clearly distinguish records from provisioned machines?
What evidence resolves a pending registration without duplicating its identity?
Which trace format can a reader follow without understanding harness internals?

Astra leads contracts and simulation; Terra owns persistence; Sol integrates the request path.
Luna high can handle settled entrypoints and configuration scaffolding.
The lead validates the whole demonstration and updates [state](STATE.md).

Booting machines, guest actions, and randomized schedules wait for later slices.
Reference scope: old M0/M1, adapted around registration rather than premature command execution.

