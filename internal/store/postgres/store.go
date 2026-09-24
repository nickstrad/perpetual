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
	bounded, cancel := context.WithTimeout(ctx, s.options.OperationTimeout)
	defer cancel()
	record, err := scanRecord(s.pool.QueryRow(bounded, "SELECT "+recordColumns+" FROM machine_registrations WHERE request_id=$1", string(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return registration.Record{}, false, nil
	}
	if err != nil {
		return registration.Record{}, false, fmt.Errorf("lookup request: %w", err)
	}
	return record, true, nil
}

func (s *Store) LookupMachine(ctx context.Context, id registration.MachineID) (registration.Record, bool, error) {
	bounded, cancel := context.WithTimeout(ctx, s.options.OperationTimeout)
	defer cancel()
	record, err := scanRecord(s.pool.QueryRow(bounded, "SELECT "+recordColumns+" FROM machine_registrations WHERE machine_id=$1", string(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return registration.Record{}, false, nil
	}
	if err != nil {
		return registration.Record{}, false, fmt.Errorf("lookup machine: %w", err)
	}
	return record, true, nil
}
