# Durable requests: implementation evidence

Verified locally on 2026-09-24, Linux amd64, Go 1.26.8, pgx 5.11.0, and the
PostgreSQL 18.6 image pinned in [versions.env](../../scripts/versions.env).
The server reported Read Committed, `fsync=on`, `synchronous_commit=on`, and
`full_page_writes=on`. This records local execution, not a GitHub CI result.

| Acceptance | Executed evidence |
| --- | --- |
| A01–A03: immutable registration and conflicts | T01–T04 core/wire cases, actual CLI/API process demonstration, PostgreSQL P01–P04 and persisted row/counter checks |
| A04–A05: restart and uncertain outcomes | S01–S04/S07/S08; P05 pending commit and abort; P06 actual COMMIT response withheld from pgx; real HTTP reply loss, service kill/restart and independent database restart |
| A06: uniqueness and capacity | Concurrent real connections and S05/S06; matching retry at capacity; original identity and count independently checked |
| A07–A08: shared decisions and replay | Production `Step`/`DecideAdmission`, S01–S10, two recorded-trace replays, impossible-event rejection, temporary production retry-identity mutation detected by the independent oracle and restored |
| A09: real storage boundary | P01–P11 with owned PostgreSQL, actual deadlock, server-injected aborts with sequence-counted attempts and unchanged request/candidate, migration lock-wait snapshot regression |
| A10–A11: bounds and fatal invariants | Queue/worker saturation, admission-time budgets, active lock-wait cancellation, listener capacity, strict decoding, bounded fixtures and shutdown, worker/HTTP fatal subprocesses and second-process ownership rejection |
| A12: integration and maintainable evidence | Actual CLI demonstration, independent review, full tests/race/static/fuzz checks, concrete decision mapping and explicit remaining coverage gaps |

Executed commands and results:

| Command/check | Result |
| --- | --- |
| Build targets | Both binaries built successfully |
| `make test test-race vet` | Passed fast tests, simulation, real PostgreSQL/process integration, race detection, and static checks |
| `make coverage` | Complete instrumented suite passed; 75.7% aggregate statement coverage |
| `make test-fuzz` | All four named targets passed, each with 30 seconds of discovery, two workers, bounded minimization and total runtime |
| Real deadlock regression, `-count=20` | Passed with exactly one deadlock victim and one survivor, independent of result-delivery order |
| Stronger P07 cancellation regression | Passed after observing the real lock wait before canceling the blocked operation |
| Development Docker targets | Up/readiness/settings, down/up persistence, and removal of the owned validation container/network/volume passed |
| Document and Git checks | Local links and whitespace checked; work state and generated artifacts remain ignored |

Tests first failed against compiling unimplemented core, adapter, and boundary
skeletons. Independent review found and closed malformed CLI success acceptance,
impossible replay events, startup snapshot ordering, cancellation and shutdown
ownership, and ambiguous commit classification defects. Regression tests cover
the fixes. The reviewer reported no unresolved material correctness findings.

The first combined run exposed a deadlock test that incorrectly assumed the
victim reported first; its corrected oracle and repeated run passed. An initial
fuzz invocation exhausted a 45-second outer budget while compiling and fuzzing;
the final 120-second outer limit retains the 30-second discovery budget. An early
race build overlapped addition of race-tag helper files; the coherent final run
passed. These attempts are not counted as successful checks.

Statement coverage does **not** establish MC/DC. The
[decision inventory](decisions.md) records actual condition pairs, infeasible
combinations, and remaining SQL error-site, cleanup and orchestration gaps. The
profile also excludes code executed inside separately built subprocesses; their
behavior is checked by real process tests. Race runs build those binaries with
`-race`. No complete MC/DC or power-loss durability claim is made.

Simulation models coordination and atomic storage outcomes. It does not validate
PostgreSQL internals, filesystem power loss, kernel scheduling, guest execution,
or machine provisioning. Those are outside this registration slice.
