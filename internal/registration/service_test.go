package registration

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type blockingStore struct {
	entered   chan Request
	release   chan struct{}
	calls     atomic.Int32
	completed atomic.Int32
}

func (s *blockingStore) Reserve(ctx context.Context, r Request) Outcome {
	s.calls.Add(1)
	select {
	case s.entered <- r:
	case <-ctx.Done():
		return Outcome{Kind: OutcomeUnavailable}
	}
	select {
	case <-s.release:
		s.completed.Add(1)
		record := baseRecord()
		record.RequestID = r.ID
		record.MachineID = r.CandidateID
		record.Parameters = r.Parameters
		record.Fingerprint = r.Fingerprint
		return Outcome{Kind: OutcomeCreated, Record: record}
	case <-ctx.Done():
		return Outcome{Kind: OutcomeUnavailable}
	}
}
func (s *blockingStore) LookupRequest(context.Context, RequestID) (Record, bool, error) {
	return baseRecord(), true, nil
}
func (s *blockingStore) LookupMachine(context.Context, MachineID) (Record, bool, error) {
	return baseRecord(), true, nil
}
func (s *blockingStore) Health(context.Context) error { return nil }
func newBlocking() *blockingStore {
	return &blockingStore{entered: make(chan Request, 8), release: make(chan struct{})}
}
func newTestService(t *testing.T, store Store, jobs, queue, workers int) *Coordinator {
	t.Helper()
	s, err := NewService(store, ServiceOptions{Epoch: "test-epoch", MaxJobs: jobs, QueueSize: queue, Workers: workers, OperationTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return s
}
func boundedReceive[T any](t *testing.T, c <-chan T) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("expected handshake did not arrive")
		var zero T
		return zero
	}
}
func TestT05CancelBeforeEnqueue(t *testing.T) {
	store := newBlocking()
	s := newTestService(t, store, 2, 2, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := s.Register(ctx, baseRequest())
	if got.Kind != OutcomeUnavailable && got.Kind != OutcomeBusy {
		t.Fatalf("pre-enqueue cancellation=%v", got.Kind)
	}
	if store.calls.Load() != 0 {
		t.Fatal("pre-cancelled request dispatched")
	}
	close(store.release)
}
func TestT05DetachedCallerDoesNotCancelAcceptedWork(t *testing.T) {
	store := newBlocking()
	s := newTestService(t, store, 2, 2, 1)
	ctx, cancel := context.WithCancel(context.Background())
	reply := make(chan Outcome, 1)
	go func() { reply <- s.Register(ctx, baseRequest()) }()
	boundedReceive(t, store.entered)
	cancel()
	got := boundedReceive(t, reply)
	if got.Kind != OutcomeUnknown {
		t.Fatalf("accepted cancellation=%v want unknown", got.Kind)
	}
	close(store.release)
	shutdownCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	if err := s.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	if store.completed.Load() != 1 || store.calls.Load() != 1 {
		t.Fatalf("accepted effect cancelled or duplicated: calls=%d completed=%d", store.calls.Load(), store.completed.Load())
	}
}
func TestT05CompletionUnderSaturation(t *testing.T) {
	store := newBlocking()
	s := newTestService(t, store, 2, 1, 1)
	done := make(chan Outcome, 3)
	go func() { done <- s.Register(context.Background(), baseRequest()) }()
	boundedReceive(t, store.entered)
	// The first accepted operation occupies a worker; simultaneous callers fill
	// admission while completion is released. Every admitted waiter must drain.
	for i := 0; i < 2; i++ {
		go func() { done <- s.Register(context.Background(), baseRequest()) }()
	}
	close(store.release)
	created := 0
	for i := 0; i < 3; i++ {
		got := boundedReceive(t, done)
		switch got.Kind {
		case OutcomeCreated:
			created++
		case OutcomeBusy:
		default:
			t.Fatalf("saturation outcome=%v", got.Kind)
		}
	}
	if created == 0 {
		t.Fatal("no accepted job progressed")
	}
}
func TestT05ReadIndependentAndShutdownRejects(t *testing.T) {
	store := newBlocking()
	s := newTestService(t, store, 1, 1, 1)
	done := make(chan Outcome, 1)
	go func() { done <- s.Register(context.Background(), baseRequest()) }()
	boundedReceive(t, store.entered)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	record, found, err := s.InspectRequest(ctx, requestA)
	if err != nil || !found || record != baseRecord() {
		t.Fatalf("read blocked by mutation: %v", err)
	}
	if got := s.Register(ctx, baseRequest()); got.Kind != OutcomeBusy {
		t.Errorf("full job capacity=%v", got.Kind)
	}
	close(store.release)
	boundedReceive(t, done)
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if got := s.Register(ctx, baseRequest()); got.Kind != OutcomeUnavailable && got.Kind != OutcomeBusy {
		t.Fatalf("shutdown admitted mutation=%v", got.Kind)
	}
	if s.Health(ctx) == nil {
		t.Fatal("shutdown advertised healthy")
	}
}
func TestT05InvalidBounds(t *testing.T) {
	store := newBlocking()
	for _, options := range []ServiceOptions{{}, {Epoch: "x", MaxJobs: 1, QueueSize: 1, Workers: 0, OperationTimeout: time.Second}, {Epoch: "x", MaxJobs: 1, QueueSize: 1, Workers: 1}, {MaxJobs: 1, QueueSize: 1, Workers: 1, OperationTimeout: time.Second}} {
		s, err := NewService(store, options)
		if err == nil {
			if s != nil {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				_ = s.Shutdown(ctx)
				cancel()
			}
			t.Errorf("accepted invalid bounds %+v", options)
		}
	}
}

type failingStore struct{ blockingStore }

func (s *failingStore) LookupRequest(context.Context, RequestID) (Record, bool, error) {
	return Record{}, false, errors.New("database unavailable")
}
func TestT05ReadErrorPreserved(t *testing.T) {
	store := &failingStore{}
	s := newTestService(t, store, 1, 1, 1)
	if _, found, err := s.InspectRequest(context.Background(), requestA); err == nil || found {
		t.Fatal("read error became absence")
	}
}

func TestT05ConstructorConditionPairs(t *testing.T) {
	valid := ServiceOptions{Epoch: "constructor", MaxJobs: 2, QueueSize: 2, Workers: 1, OperationTimeout: time.Second}
	tests := []struct {
		name     string
		edit     func(*ServiceOptions)
		nilStore bool
	}{{"nil-store", func(*ServiceOptions) {}, true}, {"epoch", func(o *ServiceOptions) { o.Epoch = "" }, false}, {"jobs", func(o *ServiceOptions) { o.MaxJobs = 0 }, false}, {"queue", func(o *ServiceOptions) { o.QueueSize = 0 }, false}, {"workers", func(o *ServiceOptions) { o.Workers = 0 }, false}, {"workers-over-jobs", func(o *ServiceOptions) { o.Workers = 3 }, false}, {"budget", func(o *ServiceOptions) { o.OperationTimeout = 0 }, false}}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			options := valid
			c.edit(&options)
			var store Store = newBlocking()
			if c.nilStore {
				store = nil
			}
			s, err := NewService(store, options)
			if err == nil {
				if s != nil {
					_ = s.Shutdown(context.Background())
				}
				t.Fatal("invalid constructor accepted")
			}
		})
	}
}

func TestT05StopAdmissionDrainsAcceptedWork(t *testing.T) {
	store := newBlocking()
	s := newTestService(t, store, 2, 2, 1)
	done := make(chan Outcome, 1)
	go func() { done <- s.Register(context.Background(), baseRequest()) }()
	boundedReceive(t, store.entered)
	s.StopAdmission()
	if s.Health(context.Background()) == nil {
		t.Fatal("draining service remains ready")
	}
	if got := s.Register(context.Background(), baseRequest()); got.Kind != OutcomeUnavailable {
		t.Fatalf("draining admission=%v", got.Kind)
	}
	close(store.release)
	if got := boundedReceive(t, done); got.Kind != OutcomeCreated {
		t.Fatalf("accepted work lost during drain: %v", got.Kind)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}
