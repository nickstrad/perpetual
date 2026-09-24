// Package testfixture owns disposable integration resources; production must not import it.
package testfixture

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var serial atomic.Uint64

// Database requires the explicitly owned container started by test-postgres.sh.
// Ordinary unit runs do not invoke this helper (integration files use a build tag).
func Database(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("PERPETUAL_TEST_DSN")
	if dsn == "" || !strings.HasPrefix(os.Getenv("PERPETUAL_TEST_CONTAINER"), "perpetual-test-") {
		t.Fatal("owned PostgreSQL fixture required: run make test-integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("case_%d_%d", os.Getpid(), serial.Add(1))
	var pool *pgxpool.Pool
	created := false
	// Register ownership before CREATE DATABASE: later setup errors must still
	// remove any database this call created.
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		if !created {
			return
		}
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		cleanupAdmin, err := pgx.Connect(cleanup, dsn)
		if err != nil {
			t.Errorf("reconnect fixture cleanup: %v", err)
			return
		}
		defer cleanupAdmin.Close(cleanup)
		if _, err := cleanupAdmin.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop owned database: %v", err)
		}
	})
	_, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	_ = admin.Close(ctx) // cleanup reconnects to drop the database
	if err != nil {
		t.Fatal(err)
	}
	created = true
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	cfg.MaxConns = 8
	pool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	fixtureURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	fixtureURL.Path = "/" + name
	return pool, fixtureURL.String()
}
func Restart(t *testing.T) {
	t.Helper()
	name := os.Getenv("PERPETUAL_TEST_CONTAINER")
	if !strings.HasPrefix(name, "perpetual-test-") {
		t.Fatal("refusing restart without owned fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "docker", "restart", "--time", "2", name).CombinedOutput(); err != nil {
		t.Fatalf("restart fixture: %v: %s", err, out)
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if exec.CommandContext(ctx, "docker", "exec", name, "pg_isready", "-h", "127.0.0.1", "-U", "postgres").Run() == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture readiness after restart timed out")
		case <-ticker.C:
		}
	}
}
