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

// Reserve repeats only confirmed PostgreSQL transaction aborts. The original
// request and candidate survive every attempt; no external effect is retried.
func (s *Store) Reserve(ctx context.Context, request registration.Request) registration.Outcome {
	if registration.ValidateRequestID(string(request.ID)) != nil || registration.ValidateMachineID(string(request.CandidateID)) != nil {
		return registration.Outcome{Kind: registration.OutcomeUnavailable}
	}
	fingerprint, err := registration.Fingerprint(request.Parameters)
	if err != nil || fingerprint != request.Fingerprint {
		return registration.Outcome{Kind: registration.OutcomeUnavailable}
	}
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
	var used, limit int64
	err = tx.QueryRow(ctx, "SELECT used,max_registrations FROM registration_gate WHERE singleton_id=1 FOR UPDATE").Scan(&used, &limit)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			invariant.Check(false, "registration gate missing")
		}
		return classifyBeforeCommit(err)
	}
	invariant.Check(limit > 0, "registration gate limit invalid")
	invariant.Check(used >= 0 && used <= limit, "registration gate count invalid")
	invariant.Check(uint64(limit) == s.options.MaxRegistrations, "registration gate limit changed")

	// This is a distinct Read Committed statement after acquiring the gate.
	// An earlier holder's commit is now visible; a plain read before the gate
	// would not prove permission to insert.
	observed := registration.Observation{Used: uint64(used), Limit: uint64(limit)}
	record, err := scanRecord(tx.QueryRow(ctx, "SELECT "+recordColumns+" FROM machine_registrations WHERE request_id=$1", string(request.ID)))
	if err == nil {
		observed.HasRequest = true
		observed.Record = record
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
	switch admission.Kind {
	case registration.AdmissionExisting:
		return registration.Outcome{Kind: registration.OutcomeExisting, Record: admission.Record}, false
	case registration.AdmissionRequestConflict:
		return registration.Outcome{Kind: registration.OutcomeRequestConflict}, false
	case registration.AdmissionNameConflict:
		return registration.Outcome{Kind: registration.OutcomeNameConflict}, false
	case registration.AdmissionCapacity:
		return registration.Outcome{Kind: registration.OutcomeCapacity}, false
	case registration.AdmissionCreate:
	default:
		invariant.Check(false, "invalid registration admission kind")
	}

	var created time.Time
	err = tx.QueryRow(ctx, `INSERT INTO machine_registrations
		(request_id,machine_id,name,image,vcpus,memory_mib,disk_mib,fingerprint_version,fingerprint,state)
		VALUES($1,$2,$3,$4,$5,$6,$7,1,$8,'registered') RETURNING created_at`,
		string(request.ID), string(request.CandidateID), request.Parameters.Name, request.Parameters.Image,
		int32(request.Parameters.VCPUs), int64(request.Parameters.MemoryMiB), int64(request.Parameters.DiskMiB),
		request.Fingerprint[:]).Scan(&created)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "machine_registrations_machine_id_key" {
				// A randomly generated candidate may collide. The service reports
				// unavailability and the caller retries its original request ID;
				// this attempt must not invent a replacement identity.
				return registration.Outcome{Kind: registration.OutcomeUnavailable}, false
			}
			invariant.Check(false, "uniqueness conflict despite registration gate")
		}
		return classifyBeforeCommit(err)
	}
	var nextUsed int64
	err = tx.QueryRow(ctx, "UPDATE registration_gate SET used=used+1 WHERE singleton_id=1 AND used<max_registrations RETURNING used").Scan(&nextUsed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			invariant.Check(false, "registration gate increment failed")
		}
		return classifyBeforeCommit(err)
	}
	invariant.Check(nextUsed == used+1, "registration gate increment mismatch")
	record = registration.Record{
		RequestID: request.ID, MachineID: request.CandidateID, Parameters: request.Parameters,
		Fingerprint: request.Fingerprint, CreatedAt: created.UTC(),
	}
	if err := tx.Commit(ctx); err != nil {
		// COMMIT may have reached PostgreSQL. Only a server SQLSTATE that
		// confirms abort permits a database-only retry.
		return classifyCommitError(err)
	}
	return registration.Outcome{Kind: registration.OutcomeCreated, Record: record}, false
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
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
		return registration.Outcome{Kind: registration.OutcomeBusy}, false
	}
	return registration.Outcome{Kind: registration.OutcomeUnavailable}, false
}

func retryableAbort(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01")
}
