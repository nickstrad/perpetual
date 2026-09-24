package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"perpetual/internal/config"
	"perpetual/internal/hostlock"
	"perpetual/internal/httpapi"
	"perpetual/internal/registration"
	"perpetual/internal/store/postgres"
)

func main() {
	cfg, err := config.Parse(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config) error {
	lock, err := hostlock.Acquire(filepath.Join(cfg.RuntimeDir, "agent-plane.lock"))
	if err != nil {
		return err
	}
	// Close the pool, then the host lock, unless a failed worker join leaves
	// both to process exit (see the registration shutdown path below).
	joinSucceeded := true
	var pool *pgxpool.Pool
	defer func() {
		if !joinSucceeded {
			return
		}
		if pool != nil {
			pool.Close()
		}
		_ = lock.Close()
	}()
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse database configuration: %w", err)
	}
	poolCfg.MaxConns = int32(cfg.PoolConnections)
	pool, err = pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("open database pool: %w", err)
	}
	store, err := postgres.New(pool, postgres.Options{
		MaxRegistrations: cfg.MaxRegistrations, LockTimeout: cfg.LockTimeout,
		StatementTimeout: cfg.StatementTimeout, OperationTimeout: cfg.OperationTimeout,
	})
	if err != nil {
		return err
	}
	if err := store.Migrate(ctx); err != nil {
		return fmt.Errorf("prepare registration database: %w", err)
	}
	epoch, err := registration.NewRandomID() // an epoch is an opaque unique string
	if err != nil {
		return fmt.Errorf("generate service epoch: %w", err)
	}
	service, err := registration.NewService(store, registration.ServiceOptions{
		Epoch: epoch, MaxJobs: cfg.MaxJobs, QueueSize: cfg.QueueSize,
		Workers: cfg.Workers, OperationTimeout: cfg.OperationTimeout,
	})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		shutdownService(service, cfg.ShutdownGrace)
		return fmt.Errorf("listen: %w", err)
	}
	server := &http.Server{
		Handler:           httpapi.FatalHandler(httpapi.New(service, httpapi.Options{}), nil),
		ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second,
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(newLimitedListener(listener, cfg.MaxConnections)) }()
	select {
	case <-ctx.Done():
	case err = <-serveDone:
		if !errors.Is(err, http.ErrServerClosed) {
			err = fmt.Errorf("HTTP server: %w", err)
		} else {
			err = nil
		}
	}
	grace, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancel()
	service.StopAdmission()
	if stopErr := server.Shutdown(grace); stopErr != nil {
		if err == nil {
			err = fmt.Errorf("HTTP shutdown: %w", stopErr)
		}
		_ = server.Close() // force handler contexts to end before pool teardown
	}
	if stopErr := service.Shutdown(grace); stopErr != nil {
		// Worker exit is required before closing the pool. Process exit is the
		// safe bound if an adapter ignores its cancellation contract. Keep the
		// host lock until that immediate exit closes its descriptor.
		joinSucceeded = false
		return fmt.Errorf("registration shutdown: %w", stopErr)
	}
	return err
}

func shutdownService(service *registration.Coordinator, bound time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()
	_ = service.Shutdown(ctx)
}

// The listener owns connection tokens, including idle keep-alive connections.
// Closing each accepted connection releases exactly one slot.
type limitedListener struct {
	net.Listener
	slots  chan struct{}
	closed chan struct{}
	once   sync.Once
}

func newLimitedListener(listener net.Listener, maximum int) *limitedListener {
	return &limitedListener{Listener: listener, slots: make(chan struct{}, maximum), closed: make(chan struct{})}
}

func (l *limitedListener) Accept() (net.Conn, error) {
	select {
	case l.slots <- struct{}{}:
	case <-l.closed:
		return nil, net.ErrClosed
	}
	conn, err := l.Listener.Accept()
	if err != nil {
		<-l.slots
		return nil, err
	}
	return &limitedConn{Conn: conn, slots: l.slots}, nil
}

func (l *limitedListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return l.Listener.Close()
}

type limitedConn struct {
	net.Conn
	once  sync.Once
	slots chan struct{} // the listener's token channel
}

func (c *limitedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { <-c.slots })
	return err
}
