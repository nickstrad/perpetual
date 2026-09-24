package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"perpetual/internal/invariant"
	"perpetual/internal/registration"
)

const maxAbortAttempts = 3

// PostgreSQL SQLSTATE codes this adapter distinguishes.
const (
	sqlStateUniqueViolation      = "23505"
	sqlStateLockNotAvailable     = "55P03"
	sqlStateSerializationFailure = "40001"
	sqlStateDeadlockDetected     = "40P01"
)

// Reserve repeats only confirmed PostgreSQL transaction aborts. The original
// request and candidate survive every attempt; no external effect is retried.
func (s *Store) Reserve(ctx context.Context, request registration.Request) registration.Outcome {
	// registration.NewRequest validated these fields. A request that fails here
	// did not come from it, which is a caller defect rather than an operating error.
	invariant.Check(registration.ValidateRequestID(string(request.ID)) == nil, "reservation request ID invalid")
	invariant.Check(registration.ValidateMachineID(string(request.CandidateID)) == nil, "reservation candidate ID invalid")
	fingerprint, err := registration.Fingerprint(request.Parameters)
	invariant.Check(err == nil && fingerprint == request.Fingerprint, "reservation fingerprint mismatch")
	ctx, cancel := context.WithTimeout(ctx, s.options.OperationTimeout)
	defer cancel()
	for attempt := 0; attempt < maxAbortAttempts; attempt++ {
		if ctx.Err() != nil {
			return registration.Outcome{Kind: registration.OutcomeUnavailable}
		}
		outcome, retry := s.reserveOnce(ctx, request)
		if !retry {
			return outcome
		}
	}
	return registration.Outcome{Kind: registration.OutcomeUnavailable}
}

func (s *Store) reserveOnce(ctx context.Context, request registration.Request) (registration.Outcome, bool) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return classifyBeforeCommit(err)
	}
	defer rollback(tx)
	if err := setLocalTimeouts(ctx, tx, s.options); err != nil {
		return classifyBeforeCommit(err)
	}
	used, limit, err := lockGate(ctx, tx)
	if err != nil {
		return classifyBeforeCommit(err)
	}
	// Startup verified the configured limit; a change while running is a defect.
	invariant.Check(uint64(limit) == s.options.MaxRegistrations, "registration gate limit changed")

	// This is a distinct Read Committed statement after acquiring the gate.
	// An earlier holder's commit is now visible; a plain read before the gate
	// would not prove permission to insert.
	observed := registration.Observation{Used: uint64(used), Limit: uint64(limit)}
	existing, err := scanRecord(tx.QueryRow(ctx, selectRecord("request_id"), string(request.ID)))
	if err == nil {
		observed.HasRequest = true
		observed.Record = existing
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return classifyBeforeCommit(err)
	}
	if !observed.HasRequest {
		err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM machine_registrations WHERE name=$1)", request.Parameters.Name).Scan(&observed.NameTaken)
		if err != nil {
			return classifyBeforeCommit(err)
		}
	}
	admission := registration.DecideAdmission(request, observed)
	if admission.Kind != registration.AdmissionCreate {
		return admission.Outcome(), false
	}

	var createdAt time.Time
	err = tx.QueryRow(ctx, `INSERT INTO machine_registrations
		(request_id,machine_id,name,image,vcpus,memory_mib,disk_mib,fingerprint_version,fingerprint,state)
		VALUES($1,$2,$3,$4,$5,$6,$7,1,$8,'registered') RETURNING created_at`,
		string(request.ID), string(request.CandidateID), request.Parameters.Name, request.Parameters.Image,
		int32(request.Parameters.VCPUs), int64(request.Parameters.MemoryMiB), int64(request.Parameters.DiskMiB),
		request.Fingerprint[:]).Scan(&createdAt)
	if err != nil {
		// The constraint name needs the full server error, not just its SQLSTATE.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == sqlStateUniqueViolation {
			if pgErr.ConstraintName == "machine_registrations_machine_id_key" {
				// A randomly generated candidate may collide. The service reports
				// unavailability and the caller retries its original request ID;
				// this attempt must not invent a replacement identity.
				return registration.Outcome{Kind: registration.OutcomeUnavailable}, false
			}
			invariant.Fail("uniqueness conflict despite registration gate")
		}
		return classifyBeforeCommit(err)
	}
	var nextUsed int64
	err = tx.QueryRow(ctx, "UPDATE registration_gate SET used=used+1 WHERE singleton_id=1 AND used<max_registrations RETURNING used").Scan(&nextUsed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			invariant.Fail("registration gate increment failed")
		}
		return classifyBeforeCommit(err)
	}
	invariant.Check(nextUsed == used+1, "registration gate increment mismatch")
	if err := tx.Commit(ctx); err != nil {
		// COMMIT may have reached PostgreSQL. Only a server SQLSTATE that
		// confirms abort permits a database-only retry.
		return classifyCommitError(err)
	}
	created := registration.Record{
		RequestID: request.ID, MachineID: request.CandidateID, Parameters: request.Parameters,
		Fingerprint: request.Fingerprint, CreatedAt: createdAt.UTC(),
	}
	return registration.Outcome{Kind: registration.OutcomeCreated, Record: created}, false
}

func classifyCommitError(err error) (registration.Outcome, bool) {
	if errors.Is(err, pgx.ErrTxCommitRollback) {
		return registration.Outcome{Kind: registration.OutcomeUnavailable}, false
	}
	if retryableAbort(err) {
		return registration.Outcome{Kind: registration.OutcomeUnavailable}, true
	}
	return registration.Outcome{Kind: registration.OutcomeUnknown}, false
}

func classifyBeforeCommit(err error) (registration.Outcome, bool) {
	if retryableAbort(err) {
		return registration.Outcome{Kind: registration.OutcomeUnavailable}, true
	}
	if sqlState(err) == sqlStateLockNotAvailable {
		return registration.Outcome{Kind: registration.OutcomeBusy}, false
	}
	return registration.Outcome{Kind: registration.OutcomeUnavailable}, false
}

func retryableAbort(err error) bool {
	state := sqlState(err)
	return state == sqlStateSerializationFailure || state == sqlStateDeadlockDetected
}

// sqlState returns the server SQLSTATE, or "" when err carries no server error.
func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
