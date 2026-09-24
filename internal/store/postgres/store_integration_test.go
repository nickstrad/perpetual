//go:build integration

package postgres

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net"
	"perpetual/internal/invariant"
	"perpetual/internal/registration"
	"perpetual/internal/testfixture"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixtureRequest(n int, name string) registration.Request {
	p := registration.Parameters{Name: name, Image: "base", VCPUs: 1, MemoryMiB: 512, DiskMiB: 1024}
	r, err := registration.NewRequest(registration.RequestID(fmt.Sprintf("%08d-1111-4111-8111-111111111111", n)), registration.MachineID(fmt.Sprintf("%08d-2222-4222-8222-222222222222", n)), p)
	if err != nil {
		panic(err)
	}
	return r
}
func fixtureStore(t *testing.T, limit uint64) (*Store, *pgxpool.Pool, string) {
	t.Helper()
	pool, dsn := testfixture.Database(t)
	s, err := New(pool, Options{MaxRegistrations: limit, LockTimeout: 150 * time.Millisecond, StatementTimeout: 2 * time.Second, OperationTimeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("P11 fresh migration: %v", err)
	}
	return s, pool, dsn
}
func counts(t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var rows, used int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM machine_registrations), used FROM registration_gate WHERE singleton_id=1").Scan(&rows, &used); err != nil {
		t.Fatal(err)
	}
	if rows != want || used != want {
		t.Fatalf("durable rows=%d counter=%d want=%d", rows, used, want)
	}
}
func TestP01ConcurrentMatchingRetry(t *testing.T) {
	s, pool, _ := fixtureStore(t, 2)
	r := fixtureRequest(1, "demo-a")
	other := r
	other.CandidateID = fixtureRequest(2, "other").CandidateID
	a, b := raceReserve(t, s, r, other)
	if !((a.Kind == registration.OutcomeCreated && b.Kind == registration.OutcomeExisting) || (b.Kind == registration.OutcomeCreated && a.Kind == registration.OutcomeExisting)) {
		t.Fatalf("classifications %v/%v", a.Kind, b.Kind)
	}
	if a.Record != b.Record {
		t.Fatal("matching attempts published different winners")
	}
	counts(t, pool, 1)
}
func raceReserve(t *testing.T, s *Store, a, b registration.Request) (registration.Outcome, registration.Outcome) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	out := make(chan registration.Outcome, 2)
	for _, r := range []registration.Request{a, b} {
		go func(r registration.Request) { <-start; out <- s.Reserve(ctx, r) }(r)
	}
	close(start)
	next := func() registration.Outcome {
		select {
		case r := <-out:
			return r
		case <-ctx.Done():
			t.Fatal("concurrent reservations exceeded bound")
			return registration.Outcome{}
		}
	}
	return next(), next()
}
func TestP02ConflictingRequest(t *testing.T) {
	s, pool, _ := fixtureStore(t, 2)
	r := fixtureRequest(1, "demo-a")
	first := s.Reserve(context.Background(), r)
	if first.Kind != registration.OutcomeCreated {
		t.Fatalf("initial=%v", first.Kind)
	}
	changed := r
	changed.Parameters.MemoryMiB = 1024
	changed.Fingerprint, _ = registration.Fingerprint(changed.Parameters)
	got := s.Reserve(context.Background(), changed)
	if got.Kind != registration.OutcomeRequestConflict || got.Record != (registration.Record{}) {
		t.Fatalf("changed intent=%+v", got)
	}
	record, found, err := s.LookupRequest(context.Background(), r.ID)
	if err != nil || !found || record != first.Record {
		t.Fatalf("winner changed: %+v %v %v", record, found, err)
	}
	counts(t, pool, 1)
}
func TestP03ConcurrentNameConflict(t *testing.T) {
	s, pool, _ := fixtureStore(t, 2)
	a, b := raceReserve(t, s, fixtureRequest(1, "same"), fixtureRequest(2, "same"))
	assertKinds(t, a, b, registration.OutcomeCreated, registration.OutcomeNameConflict)
	counts(t, pool, 1)
}
func TestP04ConcurrentCapacity(t *testing.T) {
	s, pool, _ := fixtureStore(t, 1)
	a, b := raceReserve(t, s, fixtureRequest(1, "one"), fixtureRequest(2, "two"))
	assertKinds(t, a, b, registration.OutcomeCreated, registration.OutcomeCapacity)
	counts(t, pool, 1)
	winner := a
	if b.Kind == registration.OutcomeCreated {
		winner = b
	}
	request := fixtureRequest(1, "one")
	if winner.Record.RequestID != request.ID {
		request = fixtureRequest(2, "two")
	}
	retry := s.Reserve(context.Background(), request)
	if retry.Kind != registration.OutcomeExisting || retry.Record != winner.Record {
		t.Fatalf("retry at capacity=%+v", retry)
	}
}
func assertKinds(t *testing.T, a, b registration.Outcome, x, y registration.OutcomeKind) {
	t.Helper()
	if !((a.Kind == x && b.Kind == y) || (a.Kind == y && b.Kind == x)) {
		t.Fatalf("got kinds %v/%v want %v/%v", a.Kind, b.Kind, x, y)
	}
}
func pending(t *testing.T, pool *pgxpool.Pool, r registration.Request) pgx.Tx {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(ctx)
	})
	if _, err := tx.Exec(ctx, "SELECT singleton_id FROM registration_gate WHERE singleton_id=1 FOR UPDATE"); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO machine_registrations(request_id,machine_id,name,image,vcpus,memory_mib,disk_mib,fingerprint_version,fingerprint,state) VALUES($1,$2,$3,$4,$5,$6,$7,1,$8,'registered')`, string(r.ID), string(r.CandidateID), r.Parameters.Name, r.Parameters.Image, int32(r.Parameters.VCPUs), int64(r.Parameters.MemoryMiB), int64(r.Parameters.DiskMiB), r.Fingerprint[:])
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "UPDATE registration_gate SET used=used+1 WHERE singleton_id=1"); err != nil {
		t.Fatal(err)
	}
	return tx
}
func TestP05PendingGate(t *testing.T) {
	for _, commit := range []bool{true, false} {
		t.Run(fmt.Sprint("commit=", commit), func(t *testing.T) {
			s, pool, _ := fixtureStore(t, 2)
			r := fixtureRequest(1, "demo-a")
			tx := pending(t, pool, r)
			if _, found, err := s.LookupRequest(context.Background(), r.ID); err != nil || found {
				t.Fatalf("uncommitted row observation found=%v err=%v", found, err)
			}
			changed := r
			changed.Parameters.MemoryMiB = 1024
			changed.Fingerprint, _ = registration.Fingerprint(changed.Parameters)
			for _, req := range []registration.Request{r, changed} {
				got := s.Reserve(context.Background(), req)
				if got.Kind != registration.OutcomeBusy {
					t.Fatalf("pending gate bypass: %v", got.Kind)
				}
			}
			if commit {
				if err := tx.Commit(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := tx.Rollback(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			retry := s.Reserve(context.Background(), r)
			want := registration.OutcomeExisting
			if !commit {
				want = registration.OutcomeCreated
			}
			if retry.Kind != want {
				t.Fatalf("resolved retry=%v want=%v", retry.Kind, want)
			}
			if retry.Record.MachineID != r.CandidateID {
				t.Fatal("wrong durable identity")
			}
			if got := s.Reserve(context.Background(), changed); got.Kind != registration.OutcomeRequestConflict {
				t.Fatalf("changed retry=%v", got.Kind)
			}
			counts(t, pool, 1)
		})
	}
}

// lossConn parses backend framing but never invents a database result. Only the
// successful COMMIT CommandComplete is withheld, after the real server sends it.
type lossConn struct {
	net.Conn
	armed    *atomic.Bool
	observed chan struct{}
	once     *sync.Once
	buffer   []byte
}

func (c *lossConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(c.buffer) == 0 {
		var head [5]byte
		if _, err := io.ReadFull(c.Conn, head[:]); err != nil {
			return 0, err
		}
		n := binary.BigEndian.Uint32(head[1:])
		if n < 4 || n > 1<<20 {
			return 0, fmt.Errorf("unsupported backend frame size %d", n)
		}
		frame := make([]byte, int(n)+1)
		copy(frame, head[:])
		if _, err := io.ReadFull(c.Conn, frame[5:]); err != nil {
			return 0, err
		}
		if head[0] == 'C' && string(frame[5:]) == "COMMIT\x00" && c.armed.CompareAndSwap(true, false) {
			c.once.Do(func() { close(c.observed) })
			_ = c.Conn.Close()
			return 0, io.EOF
		}
		c.buffer = frame
	}
	n := copy(p, c.buffer)
	c.buffer = c.buffer[n:]
	return n, nil
}
func TestP06ActualCommitResponseLoss(t *testing.T) {
	s, pool, dsn := fixtureStore(t, 2)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	var armed atomic.Bool
	observed := make(chan struct{})
	var once sync.Once
	cfg.ConnConfig.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		c, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &lossConn{Conn: c, armed: &armed, observed: observed, once: &once}, nil
	}
	faultPool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer faultPool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := faultPool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	faulty, err := New(faultPool, Options{MaxRegistrations: 2, LockTimeout: time.Second, StatementTimeout: 2 * time.Second, OperationTimeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	armed.Store(true)
	r := fixtureRequest(1, "demo-a")
	got := faulty.Reserve(ctx, r)
	select {
	case <-observed:
	case <-ctx.Done():
		t.Fatal("fault did not observe actual backend COMMIT response")
	}
	if got.Kind != registration.OutcomeUnknown || got.Record != (registration.Record{}) {
		t.Fatalf("lost COMMIT ack=%+v", got)
	}
	record, found, err := s.LookupRequest(ctx, r.ID)
	if err != nil || !found || record.MachineID != r.CandidateID {
		t.Fatalf("real commit not persisted: %+v %v %v", record, found, err)
	}
	retry := s.Reserve(ctx, r)
	if retry.Kind != registration.OutcomeExisting || retry.Record != record {
		t.Fatalf("retry=%+v", retry)
	}
	counts(t, pool, 1)
}
func TestP07CancellationAndReadCapacity(t *testing.T) {
	s, pool, _ := fixtureStore(t, 3)
	first := fixtureRequest(1, "existing")
	if got := s.Reserve(context.Background(), first); got.Kind != registration.OutcomeCreated {
		t.Fatal(got.Kind)
	}
	tx := pending(t, pool, fixtureRequest(2, "pending"))
	defer tx.Rollback(context.Background())
	// Establish an actual server wait before canceling. A pre-canceled context
	// alone would only cover local rejection before transaction acquisition.
	waitingStore, err := New(pool, Options{MaxRegistrations: 3, LockTimeout: time.Second, StatementTimeout: 2 * time.Second, OperationTimeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	blockedCtx, abort := context.WithCancel(context.Background())
	defer abort()
	blockedDone := make(chan registration.Outcome, 1)
	go func() { blockedDone <- waitingStore.Reserve(blockedCtx, fixtureRequest(4, "active-cancel")) }()
	observationCtx, observationCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer observationCancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		if err := pool.QueryRow(observationCtx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND pid<>pg_backend_pid()").Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case got := <-blockedDone:
			t.Fatalf("reservation resolved before cancellation handshake: %v", got.Kind)
		case <-observationCtx.Done():
			t.Fatal("reservation did not reach gate wait")
		case <-ticker.C:
		}
	}
	abort()
	select {
	case got := <-blockedDone:
		if got.Kind != registration.OutcomeUnavailable {
			t.Fatalf("canceled active reservation=%v", got.Kind)
		}
	case <-observationCtx.Done():
		t.Fatal("active reservation did not honor cancellation")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := s.Reserve(ctx, fixtureRequest(3, "cancelled")); got.Kind != registration.OutcomeUnavailable && got.Kind != registration.OutcomeBusy {
		t.Fatalf("cancelled=%v", got.Kind)
	}
	deadline, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if _, found, err := s.LookupRequest(deadline, first.ID); err != nil || !found {
		t.Fatalf("inspection blocked behind mutation: found=%v err=%v", found, err)
	}
}
func TestP10OwnedDatabaseRestart(t *testing.T) {
	s, pool, _ := fixtureStore(t, 2)
	r := fixtureRequest(1, "demo-a")
	got := s.Reserve(context.Background(), r)
	if got.Kind != registration.OutcomeCreated {
		t.Fatal(got.Kind)
	}
	old, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	testfixture.Restart(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := old.Conn().Ping(ctx); err == nil {
		t.Error("old server connection unexpectedly survived restart")
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		record, found, err := s.LookupRequest(ctx, r.ID)
		if err == nil && found {
			if record != got.Record {
				t.Fatal("restart changed durable registration")
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("pool failed to reconnect: %v", err)
		case <-ticker.C:
		}
	}
	counts(t, pool, 1)
}
func TestP11MigrationGuards(t *testing.T) {
	t.Run("repeat-and-limit", func(t *testing.T) {
		s, pool, _ := fixtureStore(t, 2)
		if err := s.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		wrong, err := New(pool, Options{MaxRegistrations: 3, LockTimeout: 500 * time.Millisecond, StatementTimeout: time.Second, OperationTimeout: 2 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		if err := wrong.Migrate(context.Background()); err == nil {
			t.Fatal("accepted mismatched configured limit")
		}
	})
	for _, sql := range []string{"UPDATE schema_migrations SET checksum='altered'", "INSERT INTO schema_migrations(version,checksum) VALUES (999,'future')"} {
		t.Run(sql, func(t *testing.T) {
			s, pool, _ := fixtureStore(t, 2)
			if _, err := pool.Exec(context.Background(), sql); err != nil {
				t.Fatal(err)
			}
			if err := s.Migrate(context.Background()); err == nil {
				t.Fatal("startup accepted schema/counter corruption")
			}
		})
	}
	t.Run("constraints", func(t *testing.T) {
		_, pool, _ := fixtureStore(t, 2)
		for _, assignment := range []string{"name=''", "image='a/b'", "vcpus=0", "memory_mib=127", "disk_mib=1023", "fingerprint_version=2", "fingerprint=decode('00','hex')", "state='running'"} {
			t.Run(assignment, func(t *testing.T) {
				r := fixtureRequest(1, "demo-a")
				tx := pending(t, pool, r)
				if _, err := tx.Exec(context.Background(), "UPDATE machine_registrations SET "+assignment); err == nil {
					t.Fatal("database accepted invalid direct mutation")
				}
				_ = tx.Rollback(context.Background())
			})
		}
	})
}
func TestP08RealDeadlock(t *testing.T) {
	_, pool, _ := fixtureStore(t, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Rollback(context.Background())
	b, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Rollback(context.Background())
	for _, tx := range []pgx.Tx{a, b} {
		if _, err := tx.Exec(ctx, "SET deadlock_timeout='50ms'"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Exec(ctx, "SELECT pg_advisory_xact_lock(11001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Exec(ctx, "SELECT pg_advisory_xact_lock(11002)"); err != nil {
		t.Fatal(err)
	}
	type result struct {
		tx  pgx.Tx
		err error
	}
	results := make(chan result, 2)
	go func() { _, err := a.Exec(ctx, "SELECT pg_advisory_xact_lock(11002)"); results <- result{a, err} }()
	go func() { _, err := b.Exec(ctx, "SELECT pg_advisory_xact_lock(11001)"); results <- result{b, err} }()
	success, deadlock := 0, 0
	for i := 0; i < 2; i++ {
		var completed result
		select {
		case completed = <-results:
		case <-ctx.Done():
			t.Fatal("deadlock outcomes exceeded bound")
		}
		if completed.err == nil {
			success++
		} else {
			var pgErr *pgconn.PgError
			if errors.As(completed.err, &pgErr) && pgErr.Code == "40P01" {
				deadlock++
			} else {
				t.Errorf("unexpected contention result: %v", completed.err)
			}
		}
		// Only this completed Exec's connection is used; the other goroutine may
		// still be returning its result. Server abort releases victim locks before
		// Go response delivery, so neither result is required to arrive first.
		_ = completed.tx.Rollback(ctx)
	}
	if success != 1 || deadlock != 1 {
		t.Fatalf("real deadlock successes=%d aborts=%d; want one each", success, deadlock)
	}

	// The production retry classifier is tested separately; this proves the real
	// server fault, not that the adapter retried these fixture-owned transactions.
}
func TestFixtureFingerprintLiteral(t *testing.T) {
	r := fixtureRequest(1, "demo-a")
	if hex.EncodeToString(r.Fingerprint[:]) != "ff70ad39bb4ef4857ee39460ababfbfc1baeca31b43a61cd676ccd854b258808" {
		t.Fatal("fixture canonicalization differs from independent literal")
	}
}

func TestP08BoundedAbortRetriesPreserveIdentity(t *testing.T) {
	for _, c := range []struct {
		name, code string
		failures   int
		want       registration.OutcomeKind
		attempts   int
	}{{"serialization-eventual", "40001", 2, registration.OutcomeCreated, 3}, {"serialization-exhausted", "40001", 10, registration.OutcomeUnavailable, 3}, {"deadlock-classified", "40P01", 2, registration.OutcomeCreated, 3}, {"nonretryable", "23514", 2, registration.OutcomeUnavailable, 1}} {
		t.Run(c.name, func(t *testing.T) {
			s, pool, _ := fixtureStore(t, 2)
			r := fixtureRequest(1, "demo-a")
			// Sequence increments survive rollback and therefore independently count
			// actual transaction attempts. This is a controlled server SQLSTATE fault;
			// TestP08RealDeadlock separately establishes actual lock contention.
			sql := fmt.Sprintf(`CREATE SEQUENCE test_attempts;
CREATE FUNCTION fail_reservation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE attempt bigint;
BEGIN
 IF NEW.request_id::text <> '%s' OR NEW.machine_id::text <> '%s' THEN RAISE EXCEPTION 'retry identity changed'; END IF;
 attempt := nextval('test_attempts');
 IF attempt <= %d THEN RAISE EXCEPTION 'controlled transaction abort' USING ERRCODE = '%s'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER inject_abort BEFORE INSERT ON machine_registrations FOR EACH ROW EXECUTE FUNCTION fail_reservation();`, r.ID, r.CandidateID, c.failures, c.code)
			if _, err := pool.Exec(context.Background(), sql); err != nil {
				t.Fatal(err)
			}
			got := s.Reserve(context.Background(), r)
			if got.Kind != c.want {
				t.Fatalf("retry classification=%v want=%v", got.Kind, c.want)
			}
			var attempts int
			if err := pool.QueryRow(context.Background(), "SELECT last_value FROM test_attempts").Scan(&attempts); err != nil {
				t.Fatal(err)
			}
			if attempts != c.attempts {
				t.Fatalf("transaction attempts=%d want=%d", attempts, c.attempts)
			}
			wantCount := 0
			if c.want == registration.OutcomeCreated {
				wantCount = 1
				if got.Record.MachineID != r.CandidateID {
					t.Fatal("retry replaced candidate")
				}
			}
			counts(t, pool, wantCount)
		})
	}
}

func TestP11CounterCorruptionIsInvariant(t *testing.T) {
	s, pool, _ := fixtureStore(t, 2)
	if _, err := pool.Exec(context.Background(), "UPDATE registration_gate SET used=1"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, ok := recover().(invariant.Violation); !ok {
			t.Error("counter corruption must stop startup with typed invariant")
		}
	}()
	_ = s.Migrate(context.Background())
}
func TestP11MigrationWaitsForPriorCommitBeforeCounting(t *testing.T) {
	_, pool, _ := fixtureStore(t, 2)
	s, err := New(pool, Options{MaxRegistrations: 2, LockTimeout: time.Second, StatementTimeout: 2 * time.Second, OperationTimeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	tx := pending(t, pool, fixtureRequest(1, "demo-a"))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		defer func() {
			if v := recover(); v != nil {
				done <- fmt.Errorf("migration invariant after valid prior commit: %v", v)
			}
		}()
		done <- s.Migrate(ctx)
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND pid<>pg_backend_pid()").Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("migration completed before gate handshake: %v", err)
		case <-ctx.Done():
			t.Fatal("migration never waited on prior transaction")
		case <-ticker.C:
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("migration did not recover after prior commit")
	}
	counts(t, pool, 1)
}
func TestP11CandidateCollisionIsUnavailable(t *testing.T) {
	s, pool, _ := fixtureStore(t, 2)
	first := fixtureRequest(1, "first")
	winner := s.Reserve(context.Background(), first)
	if winner.Kind != registration.OutcomeCreated {
		t.Fatal(winner.Kind)
	}
	second := fixtureRequest(2, "second")
	second.CandidateID = first.CandidateID
	got := s.Reserve(context.Background(), second)
	if got.Kind != registration.OutcomeUnavailable || got.Record != (registration.Record{}) {
		t.Fatalf("candidate collision=%+v", got)
	}
	record, found, err := s.LookupRequest(context.Background(), first.ID)
	if err != nil || !found || record != winner.Record {
		t.Fatal("candidate collision damaged original winner")
	}
	counts(t, pool, 1)
}
