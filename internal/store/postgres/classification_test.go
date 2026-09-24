package postgres

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"perpetual/internal/registration"
	"testing"
	"time"
)

func TestT07CommitErrorCertainty(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		want  registration.OutcomeKind
		retry bool
	}{{"serialization", &pgconn.PgError{Code: "40001"}, registration.OutcomeUnavailable, true}, {"deadlock", &pgconn.PgError{Code: "40P01"}, registration.OutcomeUnavailable, true}, {"confirmed-rollback", pgx.ErrTxCommitRollback, registration.OutcomeUnavailable, false}, {"statement-completion-unknown", &pgconn.PgError{Code: "40003"}, registration.OutcomeUnknown, false}, {"transaction-resolution-unknown", &pgconn.PgError{Code: "08007"}, registration.OutcomeUnknown, false}, {"connection", errors.New("connection lost"), registration.OutcomeUnknown, false}, {"other-server-error", &pgconn.PgError{Code: "23514"}, registration.OutcomeUnknown, false}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, retry := classifyCommitError(c.err)
			if got.Kind != c.want || retry != c.retry || got.Record != (registration.Record{}) {
				t.Fatalf("outcome=%+v retry=%v want=%v/%v", got, retry, c.want, c.retry)
			}
		})
	}
}
func TestT07BeforeCommitClassification(t *testing.T) {
	for _, c := range []struct {
		err   error
		want  registration.OutcomeKind
		retry bool
	}{{&pgconn.PgError{Code: "40001"}, registration.OutcomeUnavailable, true}, {&pgconn.PgError{Code: "40P01"}, registration.OutcomeUnavailable, true}, {&pgconn.PgError{Code: "55P03"}, registration.OutcomeBusy, false}, {&pgconn.PgError{Code: "23514"}, registration.OutcomeUnavailable, false}, {errors.New("transport error"), registration.OutcomeUnavailable, false}} {
		got, retry := classifyBeforeCommit(c.err)
		if got.Kind != c.want || retry != c.retry {
			t.Errorf("classification=%v/%v want=%v/%v", got.Kind, retry, c.want, c.retry)
		}
	}
}

func TestT07StoreConstructorPairs(t *testing.T) {
	valid := Options{MaxRegistrations: 1, LockTimeout: time.Millisecond, StatementTimeout: 2 * time.Millisecond, OperationTimeout: 3 * time.Millisecond}
	pool := &pgxpool.Pool{}
	for _, c := range []struct {
		name    string
		edit    func(*Options)
		nilPool bool
	}{{"nil-pool", func(*Options) {}, true}, {"zero-limit", func(o *Options) { o.MaxRegistrations = 0 }, false}, {"overflow-limit", func(o *Options) { o.MaxRegistrations = ^uint64(0) }, false}, {"zero-lock", func(o *Options) { o.LockTimeout = 0 }, false}, {"statement-order", func(o *Options) { o.StatementTimeout = time.Millisecond }, false}, {"operation-order", func(o *Options) { o.OperationTimeout = 2 * time.Millisecond }, false}, {"operation-cap", func(o *Options) { o.OperationTimeout = 25 * time.Hour }, false}} {
		t.Run(c.name, func(t *testing.T) {
			options := valid
			c.edit(&options)
			p := pool
			if c.nilPool {
				p = nil
			}
			if _, err := New(p, options); err == nil {
				t.Fatal("invalid constructor accepted")
			}
		})
	}
	if _, err := New(pool, valid); err != nil {
		t.Fatal(err)
	}
}
