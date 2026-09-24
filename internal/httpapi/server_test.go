package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"perpetual/internal/registration"
	"strings"
	"testing"
	"time"
)

const validBody = `{"name":"demo-a","image":"base","vcpus":1,"memory_mib":512,"disk_mib":1024}`
const rid = "11111111-1111-4111-8111-111111111111"
const mid = "22222222-2222-4222-8222-222222222222"

func TestT01WireStrictness(t *testing.T) {
	valid := []string{validBody, ` { "disk_mib":1024,"memory_mib":512,"vcpus":1,"image":"base","name":"demo-a" } `}
	for _, body := range valid {
		p, err := DecodeRegistration(strings.NewReader(body))
		if err != nil || p.Name != "demo-a" || p.MemoryMiB != 512 {
			t.Errorf("valid body=%q parameters=%+v err=%v", body, p, err)
		}
	}
	invalid := []string{"", "null", "[]", "{}", validBody + "{}", validBody + " false", strings.Replace(validBody, `"name":"demo-a"`, `"name":null`, 1), strings.Replace(validBody, `"name":"demo-a",`, "", 1), strings.Replace(validBody, `"vcpus":1`, `"vcpus":1.0`, 1), strings.Replace(validBody, `"vcpus":1`, `"vcpus":-1`, 1), strings.Replace(validBody, `"vcpus":1`, `"vcpus":4294967296`, 1), strings.Replace(validBody, `"memory_mib":512`, `"memory_mib":18446744073709551616`, 1), strings.Replace(validBody, `"name":"demo-a"`, `"name":"demo-a","name":"demo-b"`, 1), strings.Replace(validBody, `"name":"demo-a"`, `"name":"demo-a","\u006eame":"demo-a"`, 1), strings.Replace(validBody, `"name":"demo-a"`, `"unknown":0,"name":"demo-a"`, 1), validBody + strings.Repeat(" ", 4096)}
	for _, body := range invalid {
		if _, err := DecodeRegistration(strings.NewReader(body)); err == nil {
			t.Errorf("accepted invalid body %q", body)
		}
	}
	for _, field := range []string{`"image":"base"`, `"vcpus":1`, `"memory_mib":512`, `"disk_mib":1024`} {
		name := strings.Split(field, ":")[0]
		body := strings.Replace(validBody, field, name+":null", 1)
		if _, err := DecodeRegistration(strings.NewReader(body)); err == nil {
			t.Errorf("accepted null %s", name)
		}
	}
}

type fakeService struct {
	outcome registration.Outcome
	record  registration.Record
	found   bool
	err     error
	calls   int
	request registration.Request
}

func (s *fakeService) Register(_ context.Context, r registration.Request) registration.Outcome {
	s.calls++
	s.request = r
	return s.outcome
}
func (s *fakeService) InspectRequest(context.Context, registration.RequestID) (registration.Record, bool, error) {
	return s.record, s.found, s.err
}
func (s *fakeService) InspectMachine(context.Context, registration.MachineID) (registration.Record, bool, error) {
	return s.record, s.found, s.err
}
func (s *fakeService) Health(context.Context) error { return s.err }
func record() registration.Record {
	p := registration.Parameters{Name: "demo-a", Image: "base", VCPUs: 1, MemoryMiB: 512, DiskMiB: 1024}
	digest, _ := registration.Fingerprint(p)
	return registration.Record{RequestID: rid, MachineID: mid, Parameters: p, Fingerprint: digest, CreatedAt: time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)}
}
func handler(service *fakeService) http.Handler {
	return New(service, Options{NewMachineID: func() (registration.MachineID, error) { return mid, nil }})
}
func TestT06HTTPOutcomes(t *testing.T) {
	cases := []struct {
		kind   registration.OutcomeKind
		status int
		code   string
	}{{registration.OutcomeCreated, 201, ""}, {registration.OutcomeExisting, 200, ""}, {registration.OutcomeRequestConflict, 409, "request_conflict"}, {registration.OutcomeNameConflict, 409, "name_conflict"}, {registration.OutcomeCapacity, 429, "registration_capacity"}, {registration.OutcomeBusy, 503, "busy"}, {registration.OutcomeUnavailable, 503, "unavailable"}, {registration.OutcomeUnknown, 503, "outcome_unknown"}}
	for _, c := range cases {
		t.Run(c.code+http.StatusText(c.status), func(t *testing.T) {
			s := &fakeService{outcome: registration.Outcome{Kind: c.kind}}
			if c.status < 300 {
				s.outcome.Record = record()
			}
			req := httptest.NewRequest(http.MethodPut, "/v1/registrations/"+rid, strings.NewReader(validBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler(s).ServeHTTP(w, req)
			if w.Code != c.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if s.calls != 1 || s.request.ID != rid {
				t.Fatal("request identity lost")
			}
			if c.code != "" {
				var e struct {
					Error struct {
						Code, RequestID string `json:"-"`
					}
				}
				_ = e
				var body map[string]map[string]string
				if json.Unmarshal(w.Body.Bytes(), &body) != nil || body["error"]["code"] != c.code {
					t.Fatalf("error envelope=%s", w.Body)
				}
				if c.kind == registration.OutcomeUnknown && body["error"]["request_id"] != rid {
					t.Fatal("uncertainty lost original identity")
				}
			} else {
				var body struct {
					RequestID   string                  `json:"request_id"`
					MachineID   string                  `json:"machine_id"`
					State       string                  `json:"state"`
					Provisioned *bool                   `json:"provisioned"`
					Parameters  registration.Parameters `json:"parameters"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.RequestID != rid || body.MachineID != mid || body.State != "registered" || body.Provisioned == nil || *body.Provisioned || body.Parameters != record().Parameters {
					t.Fatalf("wrong registered intent response=%s", w.Body)
				}
			}
		})
	}
}

// The response schema is a shared type whose CreatedAt is a time.Time. These
// literals were written by hand for the RFC 3339 layout with trimmed
// nanoseconds ("2006-01-02T15:04:05.999999999Z07:00") that the server
// formatted explicitly before the type was shared.
func TestT06HTTPRecordBytes(t *testing.T) {
	const utc = `{"request_id":"` + rid + `","machine_id":"` + mid + `","state":"registered","provisioned":false,` +
		`"parameters":{"name":"demo-a","image":"base","vcpus":1,"memory_mib":512,"disk_mib":1024},` +
		`"created_at":"2026-09-23T00:00:00Z"}` + "\n"
	fractional := record()
	// 450,000,000 ns trims to ".45"; a -7h fixed zone prints as "-07:00".
	fractional.CreatedAt = time.Date(2026, 9, 23, 1, 2, 3, 450000000, time.FixedZone("", -7*60*60))
	for _, c := range []struct {
		record registration.Record
		want   string
	}{
		{record(), utc},
		{fractional, strings.Replace(utc, "2026-09-23T00:00:00Z", "2026-09-23T01:02:03.45-07:00", 1)},
	} {
		s := &fakeService{record: c.record, found: true}
		w := httptest.NewRecorder()
		handler(s).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/registrations/"+rid, nil))
		if w.Code != http.StatusOK || w.Body.String() != c.want {
			t.Errorf("status=%d body=%q want %q", w.Code, w.Body.String(), c.want)
		}
	}
}
func TestT06HTTPInvalidHasNoEffects(t *testing.T) {
	// The last case is well-formed JSON with an invalid name: the decoder is
	// syntactic only, so registration.NewRequest must reject it before Register.
	for _, c := range []struct{ path, body, content string }{{"/v1/registrations/bad", validBody, "application/json"}, {"/v1/registrations/" + rid, "{}", "application/json"}, {"/v1/registrations/" + rid, validBody, "text/plain"}, {"/v1/registrations/" + rid, strings.Repeat("x", 4097), "application/json"}, {"/v1/registrations/" + rid, strings.Replace(validBody, `"name":"demo-a"`, `"name":"A"`, 1), "application/json"}} {
		s := &fakeService{}
		req := httptest.NewRequest("PUT", c.path, strings.NewReader(c.body))
		req.Header.Set("Content-Type", c.content)
		w := httptest.NewRecorder()
		handler(s).ServeHTTP(w, req)
		if w.Code != 400 {
			t.Errorf("invalid status=%d", w.Code)
		}
		if s.calls != 0 {
			t.Error("invalid input dispatched a mutation")
		}
	}
}
func TestT06HTTPReadClassifications(t *testing.T) {
	for _, path := range []string{"/v1/registrations/" + rid, "/v1/machines/" + mid} {
		for _, c := range []struct {
			found  bool
			err    error
			status int
		}{{true, nil, 200}, {false, nil, 404}, {false, errors.New("private-dsn"), 503}} {
			s := &fakeService{record: record(), found: c.found, err: c.err}
			w := httptest.NewRecorder()
			handler(s).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			if w.Code != c.status {
				t.Errorf("read status=%d want %d", w.Code, c.status)
			}
			if strings.Contains(w.Body.String(), "private-dsn") {
				t.Error("leaked database diagnostics")
			}
		}
	}
	for _, err := range []error{nil, errors.New("unavailable")} {
		s := &fakeService{err: err}
		w := httptest.NewRecorder()
		handler(s).ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
		want := 200
		if err != nil {
			want = 503
		}
		if w.Code != want {
			t.Errorf("health=%d want %d", w.Code, want)
		}
	}
}
func TestT02WireCanonicalEquivalence(t *testing.T) {
	a, err := DecodeRegistration(strings.NewReader(validBody))
	if err != nil {
		t.Fatal(err)
	}
	b, err := DecodeRegistration(strings.NewReader(`{"disk_mib":1024,"image":"base","memory_mib":512,"name":"demo-a","vcpus":1}`))
	if err != nil || a != b {
		t.Fatalf("equivalent representations differ: %v", err)
	}
	x, _ := registration.Fingerprint(a)
	y, _ := registration.Fingerprint(b)
	if x != y {
		t.Fatal("wire order changed fingerprint")
	}
}

// FuzzDecodeRegistration drives arbitrary bodies through the PUT handler.
// DecodeRegistration is syntactic only, so the domain property lives at the
// HTTP boundary: a body either reaches Register once with valid parameters, or
// is rejected as invalid_request with no mutation.
func FuzzDecodeRegistration(f *testing.F) {
	for _, s := range []string{"", validBody, "null", validBody + validBody, `{"name":null}`, strings.Replace(validBody, `"name":"demo-a"`, `"name":"A"`, 1)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 8192 {
			s = s[:8192]
		}
		service := &fakeService{outcome: registration.Outcome{Kind: registration.OutcomeCreated, Record: record()}}
		req := httptest.NewRequest(http.MethodPut, "/v1/registrations/"+rid, strings.NewReader(s))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler(service).ServeHTTP(w, req)
		switch {
		case service.calls == 1 && w.Code == http.StatusCreated:
			if err := registration.ValidateParameters(service.request.Parameters); err != nil {
				t.Fatalf("handler dispatched invalid domain: %v", err)
			}
		case service.calls == 0 && w.Code == http.StatusBadRequest:
			var body ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error.Code != CodeInvalidRequest {
				t.Fatalf("rejection envelope=%s", w.Body)
			}
		default:
			t.Fatalf("status=%d calls=%d", w.Code, service.calls)
		}
	})
}
func TestT06HTTPRealRoundTrip(t *testing.T) {
	s := &fakeService{outcome: registration.Outcome{Kind: registration.OutcomeCreated, Record: record()}}
	server := httptest.NewServer(handler(s))
	defer server.Close()
	req, _ := http.NewRequest("PUT", server.URL+"/v1/registrations/"+rid, strings.NewReader(validBody))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 16385))
	if response.StatusCode != 201 || len(body) > 16384 {
		t.Fatalf("roundtrip status=%d body=%s", response.StatusCode, body)
	}
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, errors.New("fixture read failure") }
func TestT01WireErrorExits(t *testing.T) {
	if _, err := DecodeRegistration(failedReader{}); err == nil {
		t.Fatal("read failure ignored")
	}
	for _, field := range []string{`"name":"demo-a",`, `"image":"base",`, `"vcpus":1,`, `"memory_mib":512,`, `,"disk_mib":1024`} {
		if _, err := DecodeRegistration(strings.NewReader(strings.Replace(validBody, field, "", 1))); err == nil {
			t.Errorf("missing field accepted: %s", field)
		}
	}
	for _, body := range []string{`{"name":`, validBody[:len(validBody)-1], strings.Replace(validBody, `"name":"demo-a"`, `"name":42`, 1), strings.Replace(validBody, `"image":"base"`, `"image":[]`, 1)} {
		if _, err := DecodeRegistration(strings.NewReader(body)); err == nil {
			t.Errorf("invalid decode accepted %s", body)
		}
	}
}
func TestT06HTTPGuardDecisions(t *testing.T) {
	for _, c := range []struct {
		method, path, content string
		status                int
	}{{"POST", "/healthz", "", 405}, {"DELETE", "/v1/registrations/" + rid, "", 405}, {"PUT", "/v1/machines/" + mid, "", 405}, {"GET", "/v1/machines/bad", "", 400}, {"GET", "/missing", "", 404}, {"GET", "/v1/registrations/", "", 404}, {"GET", "/v1/registrations/" + rid + "/extra", "", 404}, {"PUT", "/v1/registrations/" + rid, "", 400}, {"PUT", "/v1/registrations/" + rid, "application/json; broken", 400}} {
		s := &fakeService{}
		r := httptest.NewRequest(c.method, c.path, strings.NewReader(validBody))
		r.Header.Set("Content-Type", c.content)
		w := httptest.NewRecorder()
		handler(s).ServeHTTP(w, r)
		if w.Code != c.status || s.calls != 0 {
			t.Errorf("guard %s %s => %d calls=%d", c.method, c.path, w.Code, s.calls)
		}
	}
	for _, c := range []struct {
		id     registration.MachineID
		err    error
		status int
	}{{"", errors.New("entropy unavailable"), 503}, {"bad", nil, 400}} {
		s := &fakeService{}
		h := New(s, Options{NewMachineID: func() (registration.MachineID, error) { return c.id, c.err }})
		r := httptest.NewRequest("PUT", "/v1/registrations/"+rid, strings.NewReader(validBody))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.status || s.calls != 0 {
			t.Errorf("ID creation failure status=%d calls=%d", w.Code, s.calls)
		}
	}
}
