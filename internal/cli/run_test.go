package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"perpetual/internal/registration"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const id = "11111111-1111-4111-8111-111111111111"
const success = `{"request_id":"11111111-1111-4111-8111-111111111111","machine_id":"22222222-2222-4222-8222-222222222222","state":"registered","provisioned":false,"parameters":{"name":"demo-a","image":"base","vcpus":1,"memory_mib":512,"disk_mib":1024},"created_at":"2026-09-23T00:00:00Z"}`

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }
func TestT06CLIGeneratedIdentityBeforeSingleSend(t *testing.T) {
	var out, diagnostics bytes.Buffer
	var sends int
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sends++
		if !strings.Contains(diagnostics.String(), id) {
			t.Error("ID not printed before mutation send")
		}
		if r.Method != "PUT" || r.URL.Path != "/v1/registrations/"+id {
			t.Errorf("wrong request %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"memory_mib":512`) || !strings.Contains(string(body), `"disk_mib":1024`) || !strings.Contains(string(body), `"vcpus":1`) {
			t.Errorf("defaults not sent: %s", body)
		}
		return nil, errors.New("network result lost")
	})}
	code := Run(context.Background(), []string{"machine", "register", "--name", "demo-a", "--image", "base"}, &out, &diagnostics, Options{Client: client, BaseURL: "http://example.invalid", NewRequestID: func() (registration.RequestID, error) { return id, nil }})
	if code != 6 || sends != 1 || out.Len() != 0 || !strings.Contains(diagnostics.String(), id) {
		t.Fatalf("code=%d sends=%d stdout=%q stderr=%q", code, sends, out.String(), diagnostics.String())
	}
}
func TestT06CLIStatusMapping(t *testing.T) {
	cases := []struct {
		status int
		body   string
		exit   int
	}{{201, success, 0}, {200, success, 0}, {400, `{"error":{"code":"invalid_request"}}`, 2}, {409, `{"error":{"code":"request_conflict"}}`, 3}, {429, `{"error":{"code":"registration_capacity"}}`, 5}, {503, `{"error":{"code":"busy"}}`, 5}, {503, `{"error":{"code":"unavailable"}}`, 5}, {503, `{"error":{"code":"outcome_unknown","request_id":"` + id + `"}}`, 6}}
	for _, c := range cases {
		t.Run(c.body, func(t *testing.T) {
			var sends atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sends.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(c.status)
				_, _ = io.WriteString(w, c.body)
			}))
			defer server.Close()
			var out, errout bytes.Buffer
			code := Run(context.Background(), []string{"machine", "register", "--request-id", id, "--name", "demo-a", "--image", "base"}, &out, &errout, Options{Client: server.Client(), BaseURL: server.URL})
			if code != c.exit || sends.Load() != 1 {
				t.Fatalf("exit=%d want=%d sends=%d diagnostics=%s", code, c.exit, sends.Load(), errout.String())
			}
			if c.exit == 0 {
				if out.String() != success+"\n" {
					t.Errorf("success stdout=%q", out.String())
				}
			} else if out.Len() != 0 {
				t.Error("diagnostic contaminated JSON stdout")
			}
		})
	}
}
func TestT06CLIInspectRoutes(t *testing.T) {
	for _, command := range []string{"registration", "machine"} {
		for _, status := range []int{200, 404, 503} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				plural := command + "s"
				target := id
				if command == "machine" {
					target = "22222222-2222-4222-8222-222222222222"
				}
				if r.Method != "GET" || r.URL.Path != "/v1/"+plural+"/"+target {
					t.Errorf("wrong inspect route %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(status)
				if status == 200 {
					_, _ = io.WriteString(w, success)
				} else {
					_, _ = io.WriteString(w, `{"error":{"code":"not_found"}}`)
				}
			}))
			var out, errout bytes.Buffer
			target := id
			if command == "machine" {
				target = "22222222-2222-4222-8222-222222222222"
			}
			code := Run(context.Background(), []string{command, "inspect", target}, &out, &errout, Options{Client: server.Client(), BaseURL: server.URL})
			server.Close()
			want := 0
			if status == 404 {
				want = 4
			}
			if status == 503 {
				want = 5
			}
			if code != want {
				t.Errorf("inspect status=%d exit=%d want=%d", status, code, want)
			}
		}
	}
}
func TestT06CLIRejectsInvalidBeforeSend(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("invalid command sent request")
		return nil, errors.New("unexpected")
	})}
	for _, args := range [][]string{{}, {"machine", "register"}, {"machine", "register", "--name", "A", "--image", "base", "--request-id", id}, {"machine", "register", "--name", "a", "--image", "base", "--vcpus", "4294967297", "--request-id", id}, {"registration", "inspect", "bad"}, {"widget", "inspect", id}, {"machine", "register", "--request-id", "bad", "--name", "a", "--image", "base"}, {"machine", "register", "--unknown", "x"}} {
		var out, errout bytes.Buffer
		if code := Run(context.Background(), args, &out, &errout, Options{Client: client, BaseURL: "http://example.invalid"}); code != 2 {
			t.Errorf("args=%v exit=%d", args, code)
		}
	}
}
func TestT06CLIReadFailureAndResponseBound(t *testing.T) {
	for _, body := range []string{strings.Repeat("x", 16385), "invalid JSON", ""} {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		var out, errout bytes.Buffer
		if code := Run(context.Background(), []string{"registration", "inspect", id}, &out, &errout, Options{Client: client, BaseURL: "http://example.invalid"}); code != 5 {
			t.Errorf("bad read body exit=%d", code)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	var out, errout bytes.Buffer
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })}
	if code := Run(ctx, []string{"registration", "inspect", id}, &out, &errout, Options{Client: client, BaseURL: "http://example.invalid"}); code != 5 {
		t.Errorf("read timeout exit=%d", code)
	}
}

func TestT06CLIRejectsMismatchedSuccess(t *testing.T) {
	cases := []struct {
		name string
		args []string
		body string
		want int
	}{
		{"registration-parameters", []string{"machine", "register", "--request-id", id, "--name", "demo-a", "--image", "base"}, strings.Replace(success, `"memory_mib":512`, `"memory_mib":1024`, 1), 6},
		{"inspect-request-identity", []string{"registration", "inspect", id}, strings.Replace(success, `"request_id":"`+id+`"`, `"request_id":"33333333-3333-4333-8333-333333333333"`, 1), 5},
		{"inspect-machine-identity", []string{"machine", "inspect", id}, success, 5},
		{"missing-provisioned", []string{"registration", "inspect", id}, strings.Replace(success, `"provisioned":false,`, "", 1), 5},
		{"null-provisioned", []string{"registration", "inspect", id}, strings.Replace(success, `"provisioned":false`, `"provisioned":null`, 1), 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(c.body))}, nil
			})}
			var out, diagnostics bytes.Buffer
			code := Run(context.Background(), c.args, &out, &diagnostics, Options{Client: client, BaseURL: "http://example.invalid"})
			if code != c.want || out.Len() != 0 {
				t.Fatalf("exit=%d stdout=%s want=%d", code, out.String(), c.want)
			}
		})
	}
}
func TestT06CLIMalformedMutationReplyPreservesUncertainty(t *testing.T) {
	for _, body := range []string{"not JSON", `{"error":{"code":"unrecognized"}}`, strings.Repeat("x", 16385)} {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		var out, diagnostics bytes.Buffer
		code := Run(context.Background(), []string{"machine", "register", "--request-id", id, "--name", "demo-a", "--image", "base"}, &out, &diagnostics, Options{Client: client, BaseURL: "http://example.invalid"})
		if code != 6 || !strings.Contains(diagnostics.String(), id) {
			t.Errorf("malformed mutation reply exit=%d stderr=%s", code, diagnostics.String())
		}
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, errors.New("fixture output failure") }
func TestT06CLIErrorDecisionPairs(t *testing.T) {
	for _, url := range []string{"://bad", "ftp://host", "http:///missing", "http://user@host", "http://host?query=1", "http://host#fragment"} {
		if _, err := endpointURL(url, "/v1/x"); err == nil {
			t.Errorf("invalid endpoint accepted %s", url)
		}
	}
	for _, url := range []string{"", "http://host", "https://host"} {
		if _, err := endpointURL(url, "/v1/x"); err != nil {
			t.Error(err)
		}
	}
	var out, diagnostics bytes.Buffer
	args := []string{"machine", "register", "--name", "demo-a", "--image", "base"}
	code := Run(context.Background(), args, &out, &diagnostics, Options{NewRequestID: func() (registration.RequestID, error) { return "", errors.New("entropy failure") }})
	if code != 5 {
		t.Errorf("ID generation error=%d", code)
	}
	code = Run(context.Background(), append(args, "--request-id", id), &out, failedWriter{}, Options{})
	if code != 5 {
		t.Errorf("ID emission failure=%d", code)
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(success))}, nil
	})}
	for _, command := range [][]string{append(args, "--request-id", id), {"registration", "inspect", id}} {
		if code := Run(context.Background(), command, failedWriter{}, &diagnostics, Options{Client: client}); code != 5 {
			t.Errorf("stdout failure=%d", code)
		}
	}
	if code := Run(nil, nil, &out, &diagnostics, Options{}); code != 5 {
		t.Error("nil context accepted")
	}
	if code := Run(context.Background(), nil, nil, &diagnostics, Options{}); code != 5 {
		t.Error("nil stdout accepted")
	}
	if code := Run(context.Background(), nil, &out, nil, Options{}); code != 5 {
		t.Error("nil stderr accepted")
	}
	for _, body := range []string{strings.Replace(success, `"request_id":"`+id+`"`, `"request_id":"bad"`, 1), strings.Replace(success, `"machine_id":"22222222-2222-4222-8222-222222222222"`, `"machine_id":"bad"`, 1), strings.Replace(success, `"state":"registered"`, `"state":"running"`, 1), strings.Replace(success, `"provisioned":false`, `"provisioned":true`, 1), strings.Replace(success, `"vcpus":1`, `"vcpus":0`, 1), strings.Replace(success, `"created_at":"2026-09-23T00:00:00Z"`, `"created_at":"0001-01-01T00:00:00Z"`, 1)} {
		if err := validateRegistrationResponse([]byte(body), "", "", nil); err == nil {
			t.Errorf("malformed registration success accepted %s", body)
		}
	}
}
