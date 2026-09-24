package postgres

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"perpetual/internal/invariant"
)

//go:embed migrations/001_registrations.sql
var migrationSQL string

const migrationVersion int64 = 1

// Dedicated database startup coordination. Runtime reservations use the
// visible registration_gate row instead of this advisory lock.
const migrationAdvisoryKey int64 = 0x7065727065747561

func (s *Store) Migrate(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.options.OperationTimeout)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer rollback(tx)
	if err := setLocalTimeouts(ctx, tx, s.options); err != nil {
		return fmt.Errorf("migration timeouts: %w", err)
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationAdvisoryKey); err != nil {
		return fmt.Errorf("migration ownership lock: %w", err)
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version bigint PRIMARY KEY CHECK (version > 0),
		checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create migration history: %w", err)
	}
	// LIMIT 2 bounds the read: one row is the supported state, and any second
	// row is already an unsupported history.
	rows, err := tx.Query(ctx, "SELECT version,checksum FROM schema_migrations ORDER BY version LIMIT 2")
	if err != nil {
		return fmt.Errorf("read migration history: %w", err)
	}
	history, err := pgx.CollectRows(rows, scanAppliedMigration)
	if err != nil {
		return fmt.Errorf("read migration history: %w", err)
	}
	expected := sha256.Sum256([]byte(migrationSQL))
	expectedChecksum := hex.EncodeToString(expected[:])
	switch len(history) {
	case 0:
		if _, err := tx.Exec(ctx, migrationSQL); err != nil {
			return fmt.Errorf("apply registration schema: %w", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO registration_gate(singleton_id,used,max_registrations) VALUES(1,0,$1)", int64(s.options.MaxRegistrations)); err != nil {
			return fmt.Errorf("initialize registration gate: %w", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)", migrationVersion, expectedChecksum); err != nil {
			return fmt.Errorf("record registration schema: %w", err)
		}
	case 1:
		if history[0].version != migrationVersion || history[0].checksum != expectedChecksum {
			return fmt.Errorf("unsupported registration schema version or checksum")
		}
	default:
		return fmt.Errorf("unsupported additional registration schema version")
	}
	used, limit, err := lockGate(ctx, tx)
	if err != nil {
		return fmt.Errorf("verify registration gate: %w", err)
	}
	// Read Committed takes a new snapshot for this statement. A previous
	// service may have committed while we waited to lock the gate.
	var registrations int64
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM machine_registrations").Scan(&registrations); err != nil {
		return fmt.Errorf("count committed registrations: %w", err)
	}
	invariant.Check(used == registrations, "registration gate count differs from rows")
	if limit != int64(s.options.MaxRegistrations) {
		return fmt.Errorf("configured registration limit differs from database")
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration verification: %w", err)
	}
	return nil
}

type appliedMigration struct {
	version  int64
	checksum string
}

func scanAppliedMigration(row pgx.CollectableRow) (appliedMigration, error) {
	var applied appliedMigration
	err := row.Scan(&applied.version, &applied.checksum)
	return applied, err
}

func setLocalTimeouts(ctx context.Context, tx pgx.Tx, options Options) error {
	// PostgreSQL accepts integer milliseconds as text. Round up positive
	// sub-millisecond durations so the server never interprets zero as disabled.
	lockMS := ceilMilliseconds(options.LockTimeout)
	statementMS := ceilMilliseconds(options.StatementTimeout)
	_, err := tx.Exec(ctx, "SELECT set_config('lock_timeout',$1,true), set_config('statement_timeout',$2,true)", fmt.Sprintf("%dms", lockMS), fmt.Sprintf("%dms", statementMS))
	return err
}

func ceilMilliseconds(duration time.Duration) int64 {
	milliseconds := int64(duration / time.Millisecond)
	if duration%time.Millisecond != 0 {
		milliseconds++
	}
	return milliseconds
}

func rollback(tx pgx.Tx) {
	cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := tx.Rollback(cleanup); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		log.Printf("registration transaction cleanup failed: %v", err)
	}
}
