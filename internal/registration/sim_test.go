package registration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// This fixed scheduler models only atomic transactions, the mutation gate and
// separate reply delivery. It does not emulate PostgreSQL internals or Go scheduling.
const traceVersion = 1
const traceMaxSteps = 256
const traceMaxBytes = 1 << 20

type simAction struct {
	Op      string
	Job     string
	Request Request
	Want    OutcomeKind
	Count   uint64
}
type simHeader struct {
	Version   int
	Harness   string
	Revision  string
	Toolchain string
	Scenario  string
	Limit     uint64
	Initial   []Record
}
type simLine struct {
	Number      int
	Action      simAction
	Observation string
	Effects     []string
	Records     []Record
	Replies     []simReply
}
type simReply struct {
	Job     string
	Outcome Outcome
}
type simTxn struct {
	Request   Request
	EffectID  EffectID
	Record    Record
	Outcome   Outcome
	Pending   bool
	Delivered bool
}
type simHarness struct {
	header     simHeader
	epoch      string
	generation int
	sequence   uint64
	states     map[string]State
	tx         map[string]*simTxn
	records    map[RequestID]Record
	count      uint64
	gate       string
	waiting    []string
	replies    []simReply
	lines      []simLine
	trace      bytes.Buffer
}

func newSim(header simHeader) (*simHarness, error) {
	if header.Version != traceVersion || header.Harness != "registration-fixed-v1" {
		return nil, fmt.Errorf("incompatible trace version/harness")
	}
	if header.Limit == 0 {
		return nil, fmt.Errorf("invalid limit")
	}
	h := &simHarness{header: header, epoch: "epoch-1", generation: 1, states: map[string]State{}, tx: map[string]*simTxn{}, records: map[RequestID]Record{}}
	for _, r := range header.Initial {
		h.records[r.RequestID] = r
		h.count++
	}
	if err := h.appendJSON(header); err != nil {
		return nil, err
	}
	return h, nil
}
func (h *simHarness) appendJSON(value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if h.trace.Len()+len(b)+1 > traceMaxBytes {
		return fmt.Errorf("trace byte bound exceeded")
	}
	h.trace.Write(b)
	h.trace.WriteByte('\n')
	return nil
}
func (h *simHarness) dispatch(job string) error {
	tx, ok := h.tx[job]
	if !ok {
		return fmt.Errorf("nonexistent effect %q", job)
	}
	if h.gate != "" {
		h.waiting = append(h.waiting, job)
		return nil
	}
	h.gate = job
	observed := Observation{Used: h.count, Limit: h.header.Limit}
	if r, ok := h.records[tx.Request.ID]; ok {
		observed.HasRequest = true
		observed.Record = r
	}
	for _, r := range h.records {
		if r.Parameters.Name == tx.Request.Parameters.Name {
			observed.NameTaken = true
		}
	}
	admission := DecideAdmission(tx.Request, observed)
	switch admission.Kind {
	case AdmissionCreate:
		tx.Pending = true
		tx.Record = Record{RequestID: tx.Request.ID, MachineID: tx.Request.CandidateID, Parameters: tx.Request.Parameters, Fingerprint: tx.Request.Fingerprint, CreatedAt: baseRecord().CreatedAt}
		return nil
	case AdmissionExisting:
		tx.Outcome = Outcome{Kind: OutcomeExisting, Record: admission.Record}
	case AdmissionRequestConflict:
		tx.Outcome = Outcome{Kind: OutcomeRequestConflict}
	case AdmissionNameConflict:
		tx.Outcome = Outcome{Kind: OutcomeNameConflict}
	case AdmissionCapacity:
		tx.Outcome = Outcome{Kind: OutcomeCapacity}
	default:
		return fmt.Errorf("unsupported admission %v", admission.Kind)
	}
	h.gate = ""
	return h.dispatchNext()
}
func (h *simHarness) dispatchNext() error {
	if h.gate != "" || len(h.waiting) == 0 {
		return nil
	}
	job := h.waiting[0]
	h.waiting = h.waiting[1:]
	return h.dispatch(job)
}
func (h *simHarness) invariants() error {
	if h.count != uint64(len(h.records)) || h.count > h.header.Limit {
		return fmt.Errorf("durable count invariant violated")
	}
	names := map[string]bool{}
	machines := map[MachineID]bool{}
	for id, r := range h.records {
		if id != r.RequestID {
			return fmt.Errorf("request key mismatch")
		}
		if names[r.Parameters.Name] || machines[r.MachineID] {
			return fmt.Errorf("duplicate durable identity/name")
		}
		names[r.Parameters.Name] = true
		machines[r.MachineID] = true
		found := false
		for _, tx := range h.tx {
			if tx.Request.ID == id && tx.Request.CandidateID == r.MachineID && tx.Request.Parameters == r.Parameters && tx.Request.Fingerprint == r.Fingerprint {
				found = true
			}
		}
		for _, initial := range h.header.Initial {
			if initial == r {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("durable record lacks originating intent")
		}
	}
	for _, reply := range h.replies {
		if reply.Outcome.Kind == OutcomeCreated || reply.Outcome.Kind == OutcomeExisting {
			r, ok := h.records[reply.Outcome.Record.RequestID]
			if !ok || r != reply.Outcome.Record {
				return fmt.Errorf("success published before matching durability")
			}
		} else if reply.Outcome.Record != (Record{}) {
			return fmt.Errorf("uncertain/rejected reply published identity")
		}
	}
	pending := 0
	for job, tx := range h.tx {
		if tx.Pending {
			pending++
			if job != h.gate {
				return fmt.Errorf("transaction without exclusive gate")
			}
		}
	}
	if pending > 1 {
		return fmt.Errorf("multiple gate owners")
	}
	return nil
}
func (h *simHarness) apply(a simAction) error {
	if len(h.lines) >= traceMaxSteps {
		return fmt.Errorf("trace event bound exceeded")
	}
	line := simLine{Number: len(h.lines) + 1, Action: a}
	switch a.Op {
	case "submit":
		if _, exists := h.states[a.Job]; exists {
			return fmt.Errorf("duplicate job")
		}
		h.sequence++
		id := EffectID{Epoch: h.epoch, Sequence: h.sequence}
		next, effects := Step(NewState(), Submit{Request: a.Request, EffectID: id})
		if len(effects) != 1 {
			return fmt.Errorf("submit did not request one effect")
		}
		reserve, ok := effects[0].(Reserve)
		if !ok {
			return fmt.Errorf("submit emitted non-reserve")
		}
		h.states[a.Job] = next
		h.tx[a.Job] = &simTxn{Request: reserve.Request, EffectID: reserve.EffectID}
		line.Effects = []string{"reserve:" + a.Job}
	case "dispatch":
		if err := h.dispatch(a.Job); err != nil {
			return err
		}
	case "unknown":
		tx, ok := h.tx[a.Job]
		if !ok || !tx.Pending {
			return fmt.Errorf("unknown without dispatched pending transaction")
		}
		tx.Outcome = Outcome{Kind: OutcomeUnknown}
	case "commit", "abort":
		tx, ok := h.tx[a.Job]
		if !ok || !tx.Pending || h.gate != a.Job {
			return fmt.Errorf("resolve nonexistent pending effect")
		}
		if a.Op == "commit" {
			h.records[tx.Record.RequestID] = tx.Record
			h.count++
			if !tx.Delivered {
				tx.Outcome = Outcome{Kind: OutcomeCreated, Record: tx.Record}
			}
		} else if !tx.Delivered {
			tx.Outcome = Outcome{Kind: OutcomeUnavailable}
		}
		tx.Pending = false
		h.gate = ""
		if err := h.dispatchNext(); err != nil {
			return err
		}
	case "complete":
		tx, ok := h.tx[a.Job]
		if !ok {
			return fmt.Errorf("nonexistent effect %q", a.Job)
		}
		if !IsCurrentEpoch(h.epoch, tx.EffectID) {
			line.Observation = "stale completion discarded"
			break
		}
		if tx.Delivered || tx.Outcome.Kind == 0 {
			return fmt.Errorf("completion not eligible")
		}
		state, ok := h.states[a.Job]
		if !ok {
			return fmt.Errorf("completion without live job")
		}
		next, effects := Step(state, Completed{EffectID: tx.EffectID, Outcome: tx.Outcome})
		if len(effects) != 1 {
			return fmt.Errorf("completion must request one reply")
		}
		reply, ok := effects[0].(Reply)
		if !ok {
			return fmt.Errorf("completion emitted nonreply")
		}
		h.states[a.Job] = next
		tx.Delivered = true
		h.replies = append(h.replies, simReply{a.Job, reply.Outcome})
		line.Effects = []string{"reply:" + a.Job}
	case "drop-reply":
		line.Observation = "HTTP reply lost after coordinator completion"
	case "restart":
		h.states = map[string]State{}
		h.generation++
		h.epoch = fmt.Sprintf("epoch-%d", h.generation)
		h.sequence = 0
		line.Observation = "fresh production state constructor; durable transactions retained"
	case "database-restart":
		for _, tx := range h.tx {
			if tx.Pending {
				tx.Pending = false
				if !tx.Delivered {
					tx.Outcome = Outcome{Kind: OutcomeUnavailable}
				}
			}
		}
		h.gate = ""
		if err := h.dispatchNext(); err != nil {
			return err
		}
		line.Observation = "uncommitted transactions aborted; committed records retained"
	case "absent":
		if _, found := h.records[a.Request.ID]; found {
			return fmt.Errorf("expected absent committed observation")
		}
		line.Observation = "not found; pending ownership unchanged"
	case "inspect":
		r, found := h.records[a.Request.ID]
		if !found || r.MachineID != a.Request.CandidateID || r.Parameters != a.Request.Parameters {
			return fmt.Errorf("inspection lost original identity/intent")
		}
		line.Observation = "original committed identity"
	case "waiting":
		found := false
		for _, job := range h.waiting {
			if job == a.Job {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("expected gate wait")
		}
		line.Observation = "waiting for unresolved gate"
	case "expect":
		found := false
		for _, reply := range h.replies {
			if reply.Job == a.Job {
				found = true
				if reply.Outcome.Kind != a.Want {
					return fmt.Errorf("job %s outcome=%v want=%v", a.Job, reply.Outcome.Kind, a.Want)
				}
			}
		}
		if !found {
			return fmt.Errorf("expected reply missing")
		}
	case "count":
		if h.count != a.Count {
			return fmt.Errorf("count=%d want=%d", h.count, a.Count)
		}
	case "progress":
		for job, state := range h.states {
			if state.Phase == PhaseReserving {
				return fmt.Errorf("healthy progress stalled at job %s", job)
			}
		}
		if h.gate != "" || len(h.waiting) > 0 {
			return fmt.Errorf("healthy storage progress stalled")
		}
	default:
		return fmt.Errorf("unsupported trace action %q", a.Op)
	}
	if err := h.invariants(); err != nil {
		return err
	}
	ids := make([]string, 0, len(h.records))
	for id := range h.records {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, id := range ids {
		line.Records = append(line.Records, h.records[RequestID(id)])
	}
	line.Replies = append([]simReply(nil), h.replies...)
	if err := h.appendJSON(line); err != nil {
		return err
	}
	h.lines = append(h.lines, line)
	return nil
}
func replay(trace []byte) ([]byte, error) {
	if len(trace) > traceMaxBytes {
		return nil, fmt.Errorf("trace byte bound exceeded")
	}
	scanner := bufio.NewScanner(bytes.NewReader(trace))
	scanner.Buffer(make([]byte, 4096), traceMaxBytes)
	if !scanner.Scan() {
		return nil, fmt.Errorf("missing header")
	}
	var header simHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, err
	}
	h, err := newSim(header)
	if err != nil {
		return nil, err
	}
	for scanner.Scan() {
		var recorded simLine
		if err := json.Unmarshal(scanner.Bytes(), &recorded); err != nil {
			return nil, err
		}
		if recorded.Number != len(h.lines)+1 {
			return nil, fmt.Errorf("nonmonotonic event number")
		}
		if err := h.apply(recorded.Action); err != nil {
			return nil, err
		}
		actual, _ := json.Marshal(h.lines[len(h.lines)-1])
		expected, _ := json.Marshal(recorded)
		if !bytes.Equal(actual, expected) {
			return nil, fmt.Errorf("replay observation/effect mismatch at event %d", recorded.Number)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return h.trace.Bytes(), nil
}

var revisionOnce sync.Once
var sourceRevision string

// Metadata is collected outside the deterministic scheduler. Source exports may
// legitimately omit .git; record that uncertainty rather than invent a revision.
func revisionMetadata(directory string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	revisionCommand := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	revisionCommand.Dir = directory
	revision, err := revisionCommand.Output()
	if err != nil || len(revision) != 41 {
		return "unavailable (source export); dirty=unknown"
	}
	statusCommand := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCommand.Dir = directory
	status, err := statusCommand.Output()
	if err != nil || len(status) > 1<<20 {
		return strings.TrimSpace(string(revision)) + "; dirty=unknown"
	}
	return fmt.Sprintf("%s; dirty=%t", strings.TrimSpace(string(revision)), len(status) > 0)
}
func captureRevision() string {
	revisionOnce.Do(func() { sourceRevision = revisionMetadata("") })
	return sourceRevision
}
func TestTraceSourceExportMetadata(t *testing.T) {
	if got := revisionMetadata(t.TempDir()); got != "unavailable (source export); dirty=unknown" {
		t.Fatalf("source export metadata=%q", got)
	}
}
func scenarioHeader(name string, limit uint64) simHeader {
	return simHeader{Version: traceVersion, Harness: "registration-fixed-v1", Revision: captureRevision(), Toolchain: runtime.Version(), Scenario: name, Limit: limit}
}
func runScenario(t *testing.T, name string, limit uint64, actions []simAction) *simHarness {
	t.Helper()
	h, err := newSim(scenarioHeader(name, limit))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range actions {
		if err := h.apply(a); err != nil {
			t.Fatalf("%s action=%+v: %v\ntrace:\n%s", name, a, err, h.trace.String())
		}
	}
	first, err := replay(h.trace.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	second, err := replay(h.trace.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || !bytes.Equal(first, h.trace.Bytes()) {
		t.Fatal("twice replay changed normalized choices/outcomes")
	}
	return h
}
func submit(job string, r Request) simAction { return simAction{Op: "submit", Job: job, Request: r} }
func action(op, job string) simAction        { return simAction{Op: op, Job: job} }
func expected(job string, kind OutcomeKind) simAction {
	return simAction{Op: "expect", Job: job, Want: kind}
}
func finish(job string) []simAction {
	return []simAction{action("dispatch", job), action("commit", job), action("complete", job)}
}
func TestSimulationFixedScenarios(t *testing.T) {
	r := baseRequest()
	other := r
	other.ID = "33333333-3333-4333-8333-333333333333"
	other.CandidateID = "44444444-4444-4444-8444-444444444444"
	changed := r
	changed.Parameters.MemoryMiB = 1024
	changed.Fingerprint, _ = Fingerprint(changed.Parameters)
	t.Run("S01-lost-reply-restart", func(t *testing.T) {
		a := []simAction{submit("first", r)}
		a = append(a, finish("first")...)
		retry := r
		retry.CandidateID = other.CandidateID
		a = append(a, action("drop-reply", "first"), submit("retry", retry), action("dispatch", "retry"), action("complete", "retry"), expected("retry", OutcomeExisting), action("restart", ""), simAction{Op: "inspect", Request: r}, simAction{Op: "count", Count: 1})
		runScenario(t, "S01", 2, a)
	})
	for _, c := range []struct {
		name       string
		retry      Request
		resolution string
		want       OutcomeKind
	}{{"S02-pending-match", r, "commit", OutcomeExisting}, {"S03-pending-conflict", changed, "commit", OutcomeRequestConflict}, {"S04-pending-abort", r, "abort", OutcomeCreated}} {
		t.Run(c.name, func(t *testing.T) {
			a := []simAction{submit("first", r), action("dispatch", "first"), action("unknown", "first"), action("complete", "first"), expected("first", OutcomeUnknown), {Op: "absent", Request: r}, submit("retry", c.retry), action("dispatch", "retry"), action("waiting", "retry"), action(c.resolution, "first")}
			if c.resolution == "abort" {
				a = append(a, action("commit", "retry"))
			}
			a = append(a, action("complete", "retry"), expected("retry", c.want), simAction{Op: "count", Count: 1}, simAction{Op: "inspect", Request: r}, action("progress", ""))
			runScenario(t, c.name, 2, a)
		})
	}
	t.Run("S05-name-owner", func(t *testing.T) {
		runScenario(t, "S05", 2, []simAction{submit("a", r), submit("b", other), action("dispatch", "a"), action("dispatch", "b"), action("waiting", "b"), action("commit", "a"), action("complete", "a"), action("complete", "b"), expected("b", OutcomeNameConflict), {Op: "count", Count: 1}})
	})
	t.Run("S06-final-capacity", func(t *testing.T) {
		b := other
		b.Parameters.Name = "demo-b"
		b.Fingerprint, _ = Fingerprint(b.Parameters)
		runScenario(t, "S06", 1, []simAction{submit("a", r), submit("b", b), action("dispatch", "a"), action("dispatch", "b"), action("commit", "a"), action("complete", "a"), action("complete", "b"), expected("b", OutcomeCapacity), submit("retry", r), action("dispatch", "retry"), action("complete", "retry"), expected("retry", OutcomeExisting), {Op: "count", Count: 1}})
	})
	for _, cut := range []string{"before-dispatch", "after-dispatch", "after-commit", "before-reply"} {
		t.Run("S07-"+cut, func(t *testing.T) {
			a := []simAction{submit("old", r)}
			if cut != "before-dispatch" {
				a = append(a, action("dispatch", "old"))
			}
			if cut == "after-commit" || cut == "before-reply" {
				a = append(a, action("commit", "old"))
			}
			if cut == "before-reply" {
				a = append(a, action("complete", "old"))
			}
			a = append(a, action("restart", ""))
			if cut == "after-dispatch" {
				a = append(a, action("commit", "old"))
			}
			a = append(a, submit("fresh", r), action("dispatch", "fresh"))
			want := OutcomeExisting
			if cut == "before-dispatch" {
				a = append(a, action("commit", "fresh"))
				want = OutcomeCreated
			}
			a = append(a, action("complete", "fresh"), expected("fresh", want), simAction{Op: "count", Count: 1}, simAction{Op: "inspect", Request: r})
			runScenario(t, "S07-"+cut, 2, a)
		})
	}
	t.Run("S08-old-epoch", func(t *testing.T) {
		runScenario(t, "S08", 2, []simAction{submit("old", r), action("dispatch", "old"), action("restart", ""), action("commit", "old"), submit("new", r), action("complete", "old"), action("dispatch", "new"), action("complete", "new"), expected("new", OutcomeExisting), action("progress", "")})
	})
	t.Run("S09-fair-recovery", func(t *testing.T) {
		runScenario(t, "S09", 2, []simAction{submit("faulted", r), action("dispatch", "faulted"), action("unknown", "faulted"), action("complete", "faulted"), action("database-restart", ""), submit("healthy", r), action("dispatch", "healthy"), action("commit", "healthy"), action("complete", "healthy"), expected("healthy", OutcomeCreated), action("progress", ""), {Op: "count", Count: 1}})
	})
}
func TestSimulationS10InvalidReplayAndBounds(t *testing.T) {
	header := scenarioHeader("S10", 2)
	header.Version = 99
	if _, err := newSim(header); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatal("accepted unsupported trace version")
	}
	h, _ := newSim(scenarioHeader("S10", 2))
	if err := h.apply(action("complete", "nonexistent")); err == nil || !strings.Contains(err.Error(), "nonexistent") {
		t.Fatal("invented nonexistent effect")
	}
	for i := 0; i < traceMaxSteps; i++ {
		if err := h.apply(simAction{Op: "count"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.apply(simAction{Op: "count"}); err == nil {
		t.Fatal("event bound not enforced")
	}
	if _, err := replay(bytes.Repeat([]byte("x"), traceMaxBytes+1)); err == nil {
		t.Fatal("byte bound not enforced")
	}
	large, _ := newSim(scenarioHeader("bytes", 2))
	if err := large.appendJSON(strings.Repeat("x", traceMaxBytes)); err == nil {
		t.Fatal("append byte bound not enforced")
	}
}
func TestSimulationOracleDetectsPrematureSuccess(t *testing.T) {
	h, _ := newSim(scenarioHeader("detector", 2))
	h.replies = append(h.replies, simReply{Job: "injected", Outcome: Outcome{Kind: OutcomeCreated, Record: baseRecord()}})
	if err := h.invariants(); err == nil || !strings.Contains(err.Error(), "before matching durability") {
		t.Fatal("independent oracle failed to detect premature publication")
	}
}
