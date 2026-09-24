package registration

import (
	"encoding/hex"
	"perpetual/internal/invariant"
	"reflect"
	"strings"
	"testing"
	"time"
)

const requestA = RequestID("11111111-1111-4111-8111-111111111111")
const machineA = MachineID("22222222-2222-4222-8222-222222222222")

func baseParameters() Parameters {
	return Parameters{Name: "demo-a", Image: "base", VCPUs: 1, MemoryMiB: 512, DiskMiB: 1024}
}
func baseRequest() Request {
	var digest [32]byte
	b, _ := hex.DecodeString("ff70ad39bb4ef4857ee39460ababfbfc1baeca31b43a61cd676ccd854b258808")
	copy(digest[:], b)
	return Request{ID: requestA, CandidateID: machineA, Parameters: baseParameters(), Fingerprint: digest}
}
func baseRecord() Record {
	r := baseRequest()
	return Record{RequestID: r.ID, MachineID: r.CandidateID, Parameters: r.Parameters, Fingerprint: r.Fingerprint, CreatedAt: time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)}
}
func expectViolation(t *testing.T, contains string, fn func()) {
	t.Helper()
	defer func() {
		value := recover()
		violation, ok := value.(invariant.Violation)
		if !ok {
			t.Errorf("wanted typed invariant containing %q; recovered %#v", contains, value)
			return
		}
		if !strings.Contains(violation.Message, contains) {
			t.Errorf("violation %q does not identify %q", violation.Message, contains)
		}
	}()
	fn()
}
func TestT01Identifiers(t *testing.T) {
	valid := []string{string(requestA), "00000000-0000-0000-0000-000000000000", "abcdefab-cdef-abcd-efab-cdefabcdefab"}
	invalid := []string{"", string(requestA)[1:], string(requestA) + "0", strings.ToUpper(valid[2]), "111111111111-4111-8111-111111111111", "g1111111-1111-4111-8111-111111111111", "11111111_1111-4111-8111-111111111111"}
	for name, validate := range map[string]func(string) error{"request": ValidateRequestID, "machine": ValidateMachineID} {
		t.Run(name, func(t *testing.T) {
			for _, id := range valid {
				if err := validate(id); err != nil {
					t.Errorf("valid %q: %v", id, err)
				}
			}
			for _, id := range invalid {
				if err := validate(id); err == nil {
					t.Errorf("accepted invalid %q", id)
				}
			}
		})
	}
	// Every byte is checked, including the last byte and all four separators.
	for i := 0; i < 36; i++ {
		b := []byte(requestA)
		b[i] = '!'
		if ValidateRequestID(string(b)) == nil {
			t.Errorf("accepted malformed byte %d", i)
		}
	}
}
func TestT01ParameterBoundaries(t *testing.T) {
	type boundary struct {
		name  string
		edit  func(*Parameters)
		valid bool
	}
	cases := []boundary{
		{"minimum", func(p *Parameters) { p.Name = "a"; p.Image = "Z"; p.MemoryMiB = 128 }, true},
		{"maximum", func(p *Parameters) {
			p.Name = strings.Repeat("a", 63)
			p.Image = strings.Repeat("Z", 128)
			p.VCPUs = 64
			p.MemoryMiB = 1048576
			p.DiskMiB = 16777216
		}, true},
		{"internal-hyphen", func(p *Parameters) { p.Name = "a--0"; p.Image = "A_0.-" }, true},
		{"empty-name", func(p *Parameters) { p.Name = "" }, false}, {"long-name", func(p *Parameters) { p.Name = strings.Repeat("a", 64) }, false},
		{"initial-hyphen", func(p *Parameters) { p.Name = "-a" }, false}, {"final-hyphen", func(p *Parameters) { p.Name = "a-" }, false},
		{"name-uppercase", func(p *Parameters) { p.Name = "A" }, false}, {"name-nonascii", func(p *Parameters) { p.Name = "é" }, false},
		{"name-underscore", func(p *Parameters) { p.Name = "a_b" }, false}, {"empty-image", func(p *Parameters) { p.Image = "" }, false},
		{"long-image", func(p *Parameters) { p.Image = strings.Repeat("a", 129) }, false}, {"path-image", func(p *Parameters) { p.Image = "a/b" }, false}, {"image-nonascii", func(p *Parameters) { p.Image = "é" }, false},
		{"zero-vcpu", func(p *Parameters) { p.VCPUs = 0 }, false}, {"excess-vcpu", func(p *Parameters) { p.VCPUs = 65 }, false},
		{"small-memory", func(p *Parameters) { p.MemoryMiB = 127 }, false}, {"large-memory", func(p *Parameters) { p.MemoryMiB = 1048577 }, false},
		{"small-disk", func(p *Parameters) { p.DiskMiB = 1023 }, false}, {"large-disk", func(p *Parameters) { p.DiskMiB = 16777217 }, false},
		{"overflow-memory", func(p *Parameters) { p.MemoryMiB = ^uint64(0) }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := baseParameters()
			c.edit(&p)
			before := p
			err := ValidateParameters(p)
			if (err == nil) != c.valid {
				t.Errorf("valid=%v error=%v", c.valid, err)
			}
			if p != before {
				t.Fatal("mutated parameters")
			}
		})
	}
}
func TestT02CanonicalLiteral(t *testing.T) {
	p := baseParameters()
	b, err := Canonicalize(p)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	const want = `{"version":1,"name":"demo-a","image":"base","vcpus":1,"memory_mib":512,"disk_mib":1024}`
	if string(b) != want {
		t.Fatalf("canonical bytes=%q want %q", b, want)
	}
	digest, err := Fingerprint(p)
	if err != nil {
		t.Fatal(err)
	}
	if digest != baseRequest().Fingerprint {
		t.Fatalf("digest=%x", digest)
	}
	for _, edit := range []func(*Parameters){func(p *Parameters) { p.Name = "demo-b" }, func(p *Parameters) { p.Image = "other" }, func(p *Parameters) { p.VCPUs = 2 }, func(p *Parameters) { p.MemoryMiB = 1024 }, func(p *Parameters) { p.DiskMiB = 2048 }} {
		changed := p
		edit(&changed)
		d, err := Fingerprint(changed)
		if err != nil || d == digest {
			t.Fatalf("changed intent failed digest distinction: %v", err)
		}
	}
	invalid := p
	invalid.Name = ""
	if _, err := Fingerprint(invalid); err == nil {
		t.Error("invalid intent fingerprinted")
	}
	if _, err := Canonicalize(invalid); err == nil {
		t.Error("invalid intent canonicalized")
	}
}
func TestT01NewRequestValidation(t *testing.T) {
	got, err := NewRequest(requestA, machineA, baseParameters())
	if err != nil || got != baseRequest() {
		t.Fatalf("request=%+v err=%v", got, err)
	}
	cases := []struct {
		id      RequestID
		machine MachineID
		p       Parameters
	}{{"bad", machineA, baseParameters()}, {requestA, "bad", baseParameters()}, {requestA, machineA, Parameters{}}}
	for _, c := range cases {
		if _, err := NewRequest(c.id, c.machine, c.p); err == nil {
			t.Errorf("accepted invalid request %+v", c)
		}
	}
}
func TestT03AdmissionIndependence(t *testing.T) {
	request, record := baseRequest(), baseRecord()
	changed := request
	changed.Parameters.MemoryMiB = 1024
	differentHash := request
	differentHash.Fingerprint[0] ^= 1
	cases := []struct {
		name string
		r    Request
		o    Observation
		want AdmissionKind
	}{
		{"C01-create", request, Observation{Used: 1, Limit: 2}, AdmissionCreate},
		{"C02-existing", request, Observation{HasRequest: true, Record: record, Used: 1, Limit: 2}, AdmissionExisting},
		{"C03-parameters", changed, Observation{HasRequest: true, Record: record, Used: 1, Limit: 2}, AdmissionRequestConflict},
		{"C04-fingerprint", differentHash, Observation{HasRequest: true, Record: record, Used: 1, Limit: 2}, AdmissionRequestConflict},
		{"C05-name", request, Observation{NameTaken: true, Used: 1, Limit: 2}, AdmissionNameConflict},
		{"C06-capacity", request, Observation{Used: 2, Limit: 2}, AdmissionCapacity},
		{"C07-retry-precedence", request, Observation{HasRequest: true, Record: record, NameTaken: true, Used: 1, Limit: 1}, AdmissionExisting},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := c.o
			got := DecideAdmission(c.r, c.o)
			if got.Kind != c.want {
				t.Errorf("kind=%v want %v", got.Kind, c.want)
			}
			if got.Kind == AdmissionExisting {
				if got.Record != record {
					t.Error("lost recorded winner")
				}
			} else if got.Record != (Record{}) {
				t.Error("rejection published record")
			}
			if before != c.o {
				t.Error("mutated observation")
			}
		})
	}
}
func TestT03AdmissionInvariants(t *testing.T) {
	expectViolation(t, "limit", func() { DecideAdmission(baseRequest(), Observation{}) })
	expectViolation(t, "count", func() { DecideAdmission(baseRequest(), Observation{Used: 2, Limit: 1}) })
	wrong := baseRecord()
	wrong.RequestID = "33333333-3333-4333-8333-333333333333"
	expectViolation(t, "request", func() {
		DecideAdmission(baseRequest(), Observation{HasRequest: true, Record: wrong, Used: 1, Limit: 2})
	})
}
func TestT04Transitions(t *testing.T) {
	request := baseRequest()
	id := EffectID{Epoch: "epoch-a", Sequence: 1}
	initial := State{}
	reserving, effects := Step(initial, Submit{Request: request, EffectID: id})
	want := State{Phase: PhaseReserving, EffectID: id, Request: request}
	if reserving != want || !reflect.DeepEqual(effects, []Effect{Reserve{EffectID: id, Request: request}}) {
		t.Fatalf("submit state=%+v effects=%+v", reserving, effects)
	}
	if initial != (State{}) {
		t.Fatal("mutated initial state")
	}
	for kind := OutcomeCreated; kind <= OutcomeUnknown; kind++ {
		t.Run(string(rune('A'+kind)), func(t *testing.T) {
			outcome := Outcome{Kind: kind}
			if kind == OutcomeCreated || kind == OutcomeExisting {
				outcome.Record = baseRecord()
			}
			before := reserving
			next, eff := Step(reserving, Completed{EffectID: id, Outcome: outcome})
			phase := PhaseResolved
			if kind == OutcomeUnknown {
				phase = PhaseUncertain
			}
			want := before
			want.Phase = phase
			want.Outcome = outcome
			if next != want || !reflect.DeepEqual(eff, []Effect{Reply{Outcome: outcome}}) {
				t.Fatalf("completion state=%+v effects=%+v", next, eff)
			}
			if reserving != before {
				t.Fatal("mutated pending state")
			}
		})
	}
}
func TestT04TransitionInvariants(t *testing.T) {
	id := EffectID{Epoch: "e", Sequence: 1}
	s := State{Phase: PhaseReserving, EffectID: id, Request: baseRequest()}
	expectViolation(t, "submitted", func() { Step(s, Submit{Request: baseRequest(), EffectID: id}) })
	expectViolation(t, "pending", func() { Step(State{}, Completed{EffectID: id, Outcome: Outcome{Kind: OutcomeBusy}}) })
	expectViolation(t, "effect", func() {
		Step(s, Completed{EffectID: EffectID{Epoch: "e", Sequence: 2}, Outcome: Outcome{Kind: OutcomeBusy}})
	})
	expectViolation(t, "event", func() { Step(State{}, nil) })
	for _, kind := range []OutcomeKind{0, 255} {
		expectViolation(t, "outcome", func() { CheckOutcome(baseRequest(), Outcome{Kind: kind}) })
	}
	expectViolation(t, "record", func() { CheckOutcome(baseRequest(), Outcome{Kind: OutcomeUnknown, Record: baseRecord()}) })
	edits := []struct {
		name string
		edit func(*Record)
	}{
		{"request", func(r *Record) { r.RequestID = "33333333-3333-4333-8333-333333333333" }},
		{"parameters", func(r *Record) { r.Parameters.MemoryMiB = 1024 }},
		{"fingerprint", func(r *Record) { r.Fingerprint[0] ^= 1 }},
		{"machine", func(r *Record) { r.MachineID = "invalid" }},
		{"timestamp", func(r *Record) { r.CreatedAt = time.Time{} }},
		{"UTC", func(r *Record) { r.CreatedAt = r.CreatedAt.In(time.FixedZone("offset", 3600)) }},
	}
	for _, c := range edits {
		t.Run(c.name, func(t *testing.T) {
			r := baseRecord()
			c.edit(&r)
			expectViolation(t, c.name, func() { CheckOutcome(baseRequest(), Outcome{Kind: OutcomeCreated, Record: r}) })
		})
	}
	// The persisted winner may differ from this attempt's discarded candidate.
	retry := baseRequest()
	retry.CandidateID = "44444444-4444-4444-8444-444444444444"
	CheckOutcome(retry, Outcome{Kind: OutcomeExisting, Record: baseRecord()})
}
func FuzzIdentifiers(f *testing.F) {
	for _, s := range []string{"", string(requestA), "bad", strings.Repeat("0", 37)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 128 {
			s = s[:128]
		}
		err := ValidateRequestID(s)
		if err == nil {
			if len(s) != 36 || strings.ToLower(s) != s {
				t.Fatalf("accepted malformed %q", s)
			}
		}
		if (err == nil) != (ValidateMachineID(s) == nil) {
			t.Fatal("identifier domains disagree")
		}
	})
}
func FuzzCanonicalization(f *testing.F) {
	f.Add("demo-a", "base", uint32(1), uint64(512), uint64(1024))
	f.Add("", "", uint32(0), uint64(0), ^uint64(0))
	f.Fuzz(func(t *testing.T, name, image string, cpu uint32, memory, disk uint64) {
		if len(name) > 256 {
			name = name[:256]
		}
		if len(image) > 256 {
			image = image[:256]
		}
		p := Parameters{name, image, cpu, memory, disk}
		before := p
		d, e := Fingerprint(p)
		again, e2 := Fingerprint(p)
		if p != before || d != again || (e == nil) != (e2 == nil) {
			t.Fatal("nondeterminism or mutation")
		}
		if (e == nil) != (ValidateParameters(p) == nil) {
			t.Fatal("fingerprint bypassed validation")
		}
	})
}

func TestT04FreshStateAndEpochRouting(t *testing.T) {
	if NewState() != (State{Phase: PhaseNew}) {
		t.Fatal("reconstruction restored volatile state")
	}
	if !IsCurrentEpoch("epoch-a", EffectID{Epoch: "epoch-a", Sequence: 1}) {
		t.Fatal("current worker completion rejected")
	}
	if IsCurrentEpoch("epoch-b", EffectID{Epoch: "epoch-a", Sequence: 1}) {
		t.Fatal("stale worker completion accepted")
	}
}

func TestT04EffectIdentityConditions(t *testing.T) {
	// Submit requires both a nonempty epoch and nonzero sequence. Each differs
	// from the valid (epoch-a, 1) baseline by exactly one condition.
	for _, id := range []EffectID{{Epoch: "", Sequence: 1}, {Epoch: "epoch-a", Sequence: 0}} {
		expectViolation(t, "effect", func() { Step(NewState(), Submit{Request: baseRequest(), EffectID: id}) })
	}
	// Empty service epoch is defensive: production construction rejects it.
	if IsCurrentEpoch("", EffectID{Epoch: "", Sequence: 1}) {
		t.Fatal("empty epoch admitted")
	}
	if !IsCurrentEpoch("epoch-a", EffectID{Epoch: "epoch-a", Sequence: 0}) {
		t.Fatal("malformed current completion was hidden as stale")
	}
}

func FuzzPureCore(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3})
	f.Add([]byte{1, 1, 1, 1})
	f.Add([]byte{2, 64, 255, 255})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64 {
			data = data[:64]
		}
		var choices [4]byte
		copy(choices[:], data)
		request := baseRequest()
		request.Parameters.VCPUs = uint32(choices[1]%64) + 1
		request.Parameters.MemoryMiB = 128 + uint64(choices[2])*128
		request.Parameters.DiskMiB = 1024 + uint64(choices[3])*1024
		var err error
		request.Fingerprint, err = Fingerprint(request.Parameters)
		if err != nil {
			t.Fatal(err)
		}
		observed := Observation{Limit: 1}
		if choices[0]&1 != 0 {
			observed.HasRequest = true
			observed.Used = 1
			observed.Record = baseRecord()
			observed.Record.Parameters = request.Parameters
			observed.Record.Fingerprint = request.Fingerprint
		}
		if choices[0]&2 != 0 {
			observed.NameTaken = true
			observed.Used = 1 // A taken name implies one retained registration.
		}
		before := observed
		input := request
		a := DecideAdmission(request, observed)
		b := DecideAdmission(request, observed)
		if a != b || before != observed || input != request {
			t.Fatal("admission nondeterminism or mutation")
		}
		if observed.HasRequest && a.Kind != AdmissionExisting {
			t.Fatal("matching retry rejected at capacity")
		}
		if !observed.HasRequest && observed.NameTaken && a.Kind != AdmissionNameConflict {
			t.Fatal("name ownership ignored")
		}
		id := EffectID{Epoch: "fuzz", Sequence: 1}
		initial := NewState()
		next, effects := Step(initial, Submit{Request: request, EffectID: id})
		again, repeated := Step(initial, Submit{Request: request, EffectID: id})
		if next != again || !reflect.DeepEqual(effects, repeated) || initial != NewState() || len(effects) != 1 {
			t.Fatal("submit nondeterminism, mutation or unbounded effects")
		}
		outcome := Outcome{Kind: OutcomeUnknown}
		if choices[0]&4 != 0 {
			outcome = Outcome{Kind: OutcomeExisting, Record: baseRecord()}
			outcome.Record.Parameters = request.Parameters
			outcome.Record.Fingerprint = request.Fingerprint
		}
		saved := next
		resolved, reply := Step(next, Completed{EffectID: id, Outcome: outcome})
		if next != saved || len(reply) != 1 || resolved.Outcome != outcome {
			t.Fatal("completion mutated input or lost outcome")
		}
		if outcome.Kind == OutcomeUnknown && resolved.Phase != PhaseUncertain {
			t.Fatal("unknown completion lost uncertainty")
		}
	})
}
