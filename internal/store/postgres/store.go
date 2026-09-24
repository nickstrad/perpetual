package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"perpetual/internal/invariant"
	"perpetual/internal/registration"
)

type Options struct {
	MaxRegistrations                                uint64
	LockTimeout, StatementTimeout, OperationTimeout time.Duration
}

// The caller owns Pool and closes it only after the service has joined workers.
type Store struct {
	pool    *pgxpool.Pool
	options Options
}

func New(pool *pgxpool.Pool, options Options) (*Store, error) {
	if pool == nil || options.MaxRegistrations == 0 || options.MaxRegistrations > math.MaxInt64 {
		return nil, errors.New("invalid registration store pool or limit")
	}
	if options.LockTimeout <= 0 || options.StatementTimeout <= options.LockTimeout || options.OperationTimeout <= options.StatementTimeout || options.OperationTimeout > 24*time.Hour {
		return nil, errors.New("store timeouts must satisfy 0 < lock < statement < operation")
	}
	return &Store{pool: pool, options: options}, nil
}

func (s *Store) Health(ctx context.Context) error {
	bounded, cancel := context.WithTimeout(ctx, s.options.OperationTimeout)
	defer cancel()
	return s.pool.Ping(bounded)
}

const recordColumns = "request_id,machine_id,name,image,vcpus,memory_mib,disk_mib,fingerprint,created_at"

// selectRecord reads the registration whose unique column equals $1. Callers
// pass a fixed column name from this package, never external text.
func selectRecord(column string) string {
	return "SELECT " + recordColumns + " FROM machine_registrations WHERE " + column + "=$1"
}

func scanRecord(row pgx.Row) (registration.Record, error) {
	var requestID, machineID, name, image string
	var vcpus int32
	var memoryMiB, diskMiB int64
	var fingerprint []byte
	var created time.Time
	err := row.Scan(&requestID, &machineID, &name, &image, &vcpus, &memoryMiB, &diskMiB, &fingerprint, &created)
	if err != nil {
		return registration.Record{}, err
	}
	invariant.Check(len(fingerprint) == 32, "stored fingerprint length invalid")
	var digest [32]byte
	copy(digest[:], fingerprint)
	record := registration.Record{
		RequestID: registration.RequestID(requestID), MachineID: registration.MachineID(machineID),
		Parameters:  registration.Parameters{Name: name, Image: image, VCPUs: uint32(vcpus), MemoryMiB: uint64(memoryMiB), DiskMiB: uint64(diskMiB)},
		Fingerprint: digest, CreatedAt: created.UTC(),
	}
	invariant.Check(registration.ValidateRequestID(requestID) == nil, "stored request ID invalid")
	invariant.Check(registration.ValidateMachineID(machineID) == nil, "stored machine ID invalid")
	invariant.Check(registration.ValidateParameters(record.Parameters) == nil, "stored parameters invalid")
	expected, err := registration.Fingerprint(record.Parameters)
	invariant.Check(err == nil && expected == record.Fingerprint, "stored fingerprint mismatch")
	invariant.Check(!record.CreatedAt.IsZero(), "stored timestamp invalid")
	return record, nil
}

func (s *Store) LookupRequest(ctx context.Context, id registration.RequestID) (registration.Record, bool, error) {
	return s.lookup(ctx, "request_id", string(id), "lookup request")
}

func (s *Store) LookupMachine(ctx context.Context, id registration.MachineID) (registration.Record, bool, error) {
	return s.lookup(ctx, "machine_id", string(id), "lookup machine")
}

// lookup reads one committed registration without taking the gate. An absent
// row is only an observation, never permission to create one.
func (s *Store) lookup(ctx context.Context, column, value, label string) (registration.Record, bool, error) {
	// The coordinator owns a registration's budget on its incoming context;
	// this store bound protects direct callers: lookups, health, migration, tests.
	bounded, cancel := context.WithTimeout(ctx, s.options.OperationTimeout)
	defer cancel()
	record, err := scanRecord(s.pool.QueryRow(bounded, selectRecord(column), value))
	if errors.Is(err, pgx.ErrNoRows) {
		return registration.Record{}, false, nil
	}
	if err != nil {
		return registration.Record{}, false, fmt.Errorf("%s: %w", label, err)
	}
	return record, true, nil
}

// lockGate takes the registration gate row lock that serializes every
// registration insert, then checks the gate's own range rules. Callers compare
// the returned limit with configuration themselves: startup reports a mismatch
// as an operator error, while a running reservation treats it as a defect.
//
// A missing gate row is an invariant failure for both callers. Migrate reaches
// this only after recording or verifying the migration that inserted the row
// in the same transaction, so its absence is database corruption of the same
// kind as a counter that differs from the registration rows.
func lockGate(ctx context.Context, tx pgx.Tx) (used, limit int64, err error) {
	err = tx.QueryRow(ctx, "SELECT used,max_registrations FROM registration_gate WHERE singleton_id=1 FOR UPDATE").Scan(&used, &limit)
	if errors.Is(err, pgx.ErrNoRows) {
		invariant.Fail("registration gate missing")
	}
	if err != nil {
		return 0, 0, err
	}
	invariant.Check(limit > 0, "registration gate limit invalid")
	invariant.Check(used >= 0 && used <= limit, "registration gate count invalid")
	return used, limit, nil
}
