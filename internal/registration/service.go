package registration

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"perpetual/internal/invariant"
)

type Store interface {
	Reserve(context.Context, Request) Outcome
	LookupRequest(context.Context, RequestID) (Record, bool, error)
	LookupMachine(context.Context, MachineID) (Record, bool, error)
	Health(context.Context) error
}

type ServiceOptions struct {
	Epoch                       string
	MaxJobs, QueueSize, Workers int
	OperationTimeout            time.Duration
}

type registrationJob struct {
	request  Request
	reply    chan Outcome // one slot lets a detached caller leave without blocking the owner
	deadline time.Time
}

type reservationWork struct {
	effect   Reserve
	deadline time.Time
}

type completedJob struct {
	id      EffectID
	outcome Outcome
}

type activeJob struct {
	state State
	reply chan Outcome
}

type Coordinator struct {
	store   Store
	options ServiceOptions
	input   chan registrationJob
	work    chan reservationWork
	done    chan completedJob
	stop    chan struct{}
	stopped chan struct{}
	slots   chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex // orders admission with shutdown
	workers sync.WaitGroup
	closing bool
}

func NewService(store Store, options ServiceOptions) (*Coordinator, error) {
	if store == nil || options.Epoch == "" || options.MaxJobs <= 0 || options.QueueSize <= 0 || options.Workers <= 0 || options.Workers > options.MaxJobs || options.OperationTimeout <= 0 {
		return nil, errors.New("invalid service bounds or store")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Coordinator{
		store: store, options: options, input: make(chan registrationJob, options.QueueSize),
		work: make(chan reservationWork, options.MaxJobs), done: make(chan completedJob, options.Workers),
		stop: make(chan struct{}), stopped: make(chan struct{}), slots: make(chan struct{}, options.MaxJobs),
		ctx: ctx, cancel: cancel,
	}
	for i := 0; i < options.Workers; i++ {
		s.workers.Add(1)
		go s.worker()
	}
	go s.run()
	return s, nil
}

func (s *Coordinator) Register(ctx context.Context, request Request) Outcome {
	job := registrationJob{request: request, reply: make(chan Outcome, 1)}
	s.mu.Lock()
	if ctx.Err() != nil {
		s.mu.Unlock()
		return Outcome{Kind: OutcomeUnavailable}
	}
	if s.closing {
		s.mu.Unlock()
		return Outcome{Kind: OutcomeUnavailable}
	}
	select {
	case s.slots <- struct{}{}:
	default:
		s.mu.Unlock()
		return Outcome{Kind: OutcomeBusy}
	}
	// The lock prevents shutdown from passing this acceptance boundary. Neither
	// operation blocks: a saturated queue is a known local rejection.
	job.deadline = time.Now().Add(s.options.OperationTimeout)
	select {
	case s.input <- job:
	default:
		<-s.slots
		s.mu.Unlock()
		return Outcome{Kind: OutcomeBusy}
	}
	s.mu.Unlock()
	select {
	case result := <-job.reply:
		return result
	case <-ctx.Done():
		// The service owns the accepted effect. Caller departure says nothing
		// about its transaction, even if dispatch has not begun yet.
		return Outcome{Kind: OutcomeUnknown}
	}
}

func (s *Coordinator) worker() {
	defer s.workers.Done()
	for work := range s.work {
		ctx, cancel := context.WithDeadline(s.ctx, work.deadline)
		outcome := Outcome{Kind: OutcomeUnavailable}
		if ctx.Err() == nil {
			outcome = s.store.Reserve(ctx, work.effect.Request)
		}
		cancel()
		s.done <- completedJob{id: work.effect.EffectID, outcome: outcome}
	}
}

func (s *Coordinator) run() {
	defer close(s.stopped)
	active := make(map[EffectID]activeJob, s.options.MaxJobs)
	var sequence uint64
	stopping := false
	stop := s.stop
	for {
		if stopping && len(s.slots) == 0 {
			close(s.work)
			s.workers.Wait()
			return
		}
		select {
		case job := <-s.input:
			invariant.Check(sequence < math.MaxUint64, "effect sequence overflow")
			sequence++
			id := EffectID{Epoch: s.options.Epoch, Sequence: sequence}
			state, effects := Step(NewState(), Submit{Request: job.request, EffectID: id})
			invariant.Check(len(effects) == 1, "submit must request one effect")
			reserve, ok := effects[0].(Reserve)
			invariant.Check(ok, "submit requested unexpected effect")
			active[id] = activeJob{state: state, reply: job.reply}
			// Each queued item holds one of MaxJobs slots until its completion is
			// handled, so this buffer of MaxJobs can never be full here.
			select {
			case s.work <- reservationWork{effect: reserve, deadline: job.deadline}:
			default:
				invariant.Fail("work queue exceeded job slots")
			}
		case completed := <-s.done:
			// Only this coordinator's workers feed s.done, with this epoch's IDs.
			invariant.Check(IsCurrentEpoch(s.options.Epoch, completed.id), "completion from another epoch")
			job, ok := active[completed.id]
			invariant.Check(ok, "completion for unknown effect")
			_, effects := Step(job.state, Completed{EffectID: completed.id, Outcome: completed.outcome})
			invariant.Check(len(effects) == 1, "completion must request one reply")
			reply, ok := effects[0].(Reply)
			invariant.Check(ok, "completion requested unexpected effect")
			job.reply <- reply.Outcome
			delete(active, completed.id)
			<-s.slots
		case <-stop:
			stopping = true
			stop = nil // one shutdown event; continue draining accepted jobs
		}
	}
}

func (s *Coordinator) InspectRequest(ctx context.Context, id RequestID) (Record, bool, error) {
	return s.store.LookupRequest(ctx, id)
}
func (s *Coordinator) InspectMachine(ctx context.Context, id MachineID) (Record, bool, error) {
	return s.store.LookupMachine(ctx, id)
}
func (s *Coordinator) Health(ctx context.Context) error {
	s.mu.Lock()
	closing := s.closing
	s.mu.Unlock()
	if closing {
		return errors.New("service stopping")
	}
	return s.store.Health(ctx)
}

// StopAdmission is the shutdown linearization point. It makes readiness fail
// before the HTTP listener starts draining existing connections.
func (s *Coordinator) StopAdmission() {
	s.mu.Lock()
	if !s.closing {
		s.closing = true
		close(s.stop)
	}
	s.mu.Unlock()
}

func (s *Coordinator) Shutdown(ctx context.Context) error {
	s.StopAdmission()
	select {
	case <-s.stopped:
		s.cancel()
		return nil
	case <-ctx.Done():
		s.cancel()
		return fmt.Errorf("service shutdown before workers joined: %w", ctx.Err())
	}
}
