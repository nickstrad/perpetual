# Durable registration boundaries

Recorded 2026-09-24 during slice 1 implementation. The
[implementation plan](../plans/durable_requests/plan.md) owns the
API and resource contracts; [TESTING.md](../../TESTING.md) owns verification status.

Registration records intent only. PostgreSQL is authoritative for request IDs,
machine IDs, names, and the retained-record count. The service keeps bounded,
temporary jobs; it does not reconstruct an authoritative registration cache on
restart. The simulator calls the same `Step` and admission functions as production.

An absent lookup cannot establish that a pending transaction aborted. Every
mutation locks the singleton registration gate, then looks up the request in a
separate Read Committed statement. That second statement observes the previous
gate owner's committed result. Matching retries return the stored machine ID,
even when capacity is full. Inspection does not acquire the mutation gate.

Startup must also count registrations in a separate statement after locking the
gate. Combining the lock and count in one query can mix the updated locked row
with a snapshot taken before a pending commit, falsely reporting corruption. The
real migration-wait regression covers this boundary.

Commit response loss and pending transactions are separate test cases. The real
driver fault wrapper observes PostgreSQL's COMMIT completion frame, withholds it
from pgx, and verifies the record through another connection. Transaction retries
are restricted to confirmed aborts and retain the original request and candidate.

The Docker development database persists across `make dev-db-down` and
`make dev-db-up`; `make dev-db-reset` deletes that project's data. Integration
tests create separate owned containers. They serialize Go test packages because
database-restart cases affect the whole fixture server, while concurrent
reservation tests still run concurrent connections within a package. Never run
restart checks against an operator's database.

Simulation establishes coordination behavior, not PostgreSQL storage internals,
power-loss durability, or future guest execution. Replay rejects impossible event
sequences. A source export without Git metadata records revision unavailability
explicitly instead of inventing a revision or failing otherwise valid tests.
