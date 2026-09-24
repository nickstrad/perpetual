package registration

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type stalledBudgetStore struct {
	blockingStore
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (s *stalledBudgetStore) Reserve(ctx context.Context, r Request) Outcome {
	s.calls.Add(1)
	select {
	case s.entered <- struct{}{}:
	default:
	}
	<-s.release
	return Outcome{Kind: OutcomeUnavailable}
}
func TestT05BudgetIncludesQueuedTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &stalledBudgetStore{entered: make(chan struct{}, 1), release: make(chan struct{})}
		s, err := NewService(store, ServiceOptions{Epoch: "budget", MaxJobs: 2, QueueSize: 2, Workers: 1, OperationTimeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		first := make(chan Outcome, 1)
		second := make(chan Outcome, 1)
		go func() { first <- s.Register(context.Background(), baseRequest()) }()
		<-store.entered
		go func() { second <- s.Register(context.Background(), baseRequest()) }()
		synctest.Wait() // Both accepted jobs have reached a durable block.
		time.Sleep(2 * time.Second)
		close(store.release)
		synctest.Wait()
		select {
		case got := <-second:
			if got.Kind != OutcomeUnavailable {
				t.Errorf("expired queued job=%v", got.Kind)
			}
		default:
			t.Error("expired queued job did not resolve")
		}
		if calls := store.calls.Load(); calls != 1 {
			t.Errorf("expired queued job invoked store; calls=%d", calls)
		}
		select {
		case <-first:
		default:
			t.Error("first job did not finish")
		}
		if err := s.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestT05ShutdownDeadlineCancelsAndJoins(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newBlocking()
		s, err := NewService(store, ServiceOptions{Epoch: "shutdown", MaxJobs: 1, QueueSize: 1, Workers: 1, OperationTimeout: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan Outcome, 1)
		go func() { done <- s.Register(context.Background(), baseRequest()) }()
		<-store.entered
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		if err := s.Shutdown(ctx); err == nil {
			t.Fatal("unfinished shutdown did not report deadline")
		}
		synctest.Wait()
		if got := <-done; got.Kind != OutcomeUnavailable {
			t.Fatalf("cancelled worker outcome=%v", got.Kind)
		}
		if err := s.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}
