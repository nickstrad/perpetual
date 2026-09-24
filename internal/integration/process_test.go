//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"perpetual/internal/testfixture"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const requestID = "11111111-1111-4111-8111-111111111111"
const otherID = "33333333-3333-4333-8333-333333333333"
const lossID = "55555555-5555-4555-8555-555555555555"

type boundedLog struct {
	sync.Mutex
	data bytes.Buffer
}

func (b *boundedLog) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	n := len(p)
	remaining := (64 << 10) - b.data.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.data.Write(p)
	}
	return n, nil
}
func (b *boundedLog) String() string { b.Lock(); defer b.Unlock(); return b.data.String() }

type ownedProcess struct {
	command *exec.Cmd
	done    chan error
	log     *boundedLog
	stopped bool
}

func (p *ownedProcess) stop(t *testing.T, crash bool) {
	t.Helper()
	if p.stopped {
		return
	}
	p.stopped = true
	if crash {
		_ = p.command.Process.Kill()
	} else {
		_ = p.command.Process.Signal(syscall.SIGTERM)
	}
	select {
	case err := <-p.done:
		if !crash && err != nil {
			t.Errorf("graceful service shutdown failed: %v logs=%s", err, p.log.String())
		}
	case <-time.After(12 * time.Second):
		_ = p.command.Process.Kill()
		select {
		case <-p.done:
		case <-time.After(3 * time.Second):
			t.Errorf("service survived SIGKILL; pid=%d logs=%s", p.command.Process.Pid, p.log.String())
		}
		t.Error("service exceeded shutdown grace")
	}
}
func build(t *testing.T, root, target, output string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"build", "-buildvcs=false"}
	if raceEnabled {
		args = append(args, "-race")
	}
	args = append(args, "-o", output, target)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build process fixture: %v %s", err, output)
	}
}
func availableAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}
func launch(t *testing.T, binary, dsn, address, runDir string) *ownedProcess {
	t.Helper()
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(), "PERPETUAL_DATABASE_URL="+dsn, "PERPETUAL_LISTEN_ADDRESS="+address, "PERPETUAL_RUNTIME_DIR="+runDir, "PERPETUAL_MAX_REGISTRATIONS=4")
	logs := &boundedLog{}
	cmd.Stdout = logs
	cmd.Stderr = logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &ownedProcess{command: cmd, done: make(chan error, 1), log: logs}
	go func() { p.done <- cmd.Wait() }()
	t.Cleanup(func() { p.stop(t, false) })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 200 * time.Millisecond}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := client.Get("http://" + address + "/healthz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				return p
			}
		}
		select {
		case err := <-p.done:
			p.stopped = true
			t.Fatalf("service exited before readiness: %v logs=%s", err, logs.String())
		case <-ctx.Done():
			t.Fatalf("service readiness timed out: %s", logs.String())
		case <-ticker.C:
		}
	}
}
func cli(t *testing.T, binary, url string, args ...string) (int, []byte, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), "PERPETUAL_API_URL="+url)
	var out, errout bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errout
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("CLI exceeded bound: %v", ctx.Err())
	}
	exit := 0
	if err != nil {
		var ok bool
		var e *exec.ExitError
		e, ok = err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		exit = e.ExitCode()
	}
	return exit, out.Bytes(), errout.String()
}
func registerArgs(id, name string) []string {
	return []string{"machine", "register", "--request-id", id, "--name", name, "--image", "base"}
}
func machineID(t *testing.T, body []byte) string {
	t.Helper()
	var response struct {
		MachineID   string `json:"machine_id"`
		State       string `json:"state"`
		Provisioned *bool  `json:"provisioned"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response.MachineID == "" || response.State != "registered" || response.Provisioned == nil || *response.Provisioned {
		t.Fatalf("invalid intent response %s", body)
	}
	return response.MachineID
}
func TestProcessCLIRegistrationRestartAndReplyLoss(t *testing.T) {
	pool, dsn := testfixture.Database(t)
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	bins := t.TempDir()
	agent := filepath.Join(bins, "agent-plane")
	command := filepath.Join(bins, "perpetual")
	build(t, root, "./cmd/agent-plane", agent)
	build(t, root, "./cmd/perpetual", command)
	address := availableAddress(t)
	runDir := t.TempDir()
	process := launch(t, agent, dsn, address, runDir)
	url := "http://" + address
	exit, first, diagnostics := cli(t, command, url, registerArgs(requestID, "demo-a")...)
	if exit != 0 {
		t.Fatalf("register exit=%d diagnostics=%s", exit, diagnostics)
	}
	winner := machineID(t, first)
	exit, retry, _ := cli(t, command, url, registerArgs(requestID, "demo-a")...)
	if exit != 0 || !bytes.Equal(first, retry) {
		t.Fatalf("matching retry changed registration: %s versus %s", first, retry)
	}
	changed := append(registerArgs(requestID, "demo-a"), "--memory-mib", "1024")
	exit, _, _ = cli(t, command, url, changed...)
	if exit != 3 {
		t.Fatalf("conflicting parameters exit=%d", exit)
	}
	exit, _, _ = cli(t, command, url, registerArgs(otherID, "demo-a")...)
	if exit != 3 {
		t.Fatalf("conflicting name exit=%d", exit)
	}
	// Read the actual successful service response, then close the downstream
	// socket before sending its bytes. The handshake proves the fault is after
	// commit acknowledgment, independently of CLI's missing response.
	observed := make(chan []byte, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstream, err := http.NewRequestWithContext(r.Context(), r.Method, url+r.URL.RequestURI(), r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		upstream.Header = r.Header.Clone()
		response, err := (&http.Client{Timeout: 5 * time.Second}).Do(upstream)
		if err != nil {
			t.Error(err)
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, 16385))
		if err != nil || response.StatusCode != 201 {
			t.Errorf("fault boundary missing committed success: status=%d err=%v body=%s", response.StatusCode, err, body)
			return
		}
		observed <- body
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = connection.Close()
	}))
	defer proxy.Close()
	exit, _, diagnostics = cli(t, command, proxy.URL, registerArgs(lossID, "demo-b")...)
	if exit != 6 || !strings.Contains(diagnostics, lossID) {
		t.Fatalf("lost reply exit=%d diagnostics=%s", exit, diagnostics)
	}
	var committed []byte
	select {
	case committed = <-observed:
	case <-time.After(time.Second):
		t.Fatal("lost reply boundary was never observed")
	}
	lostWinner := machineID(t, committed)
	exit, found, _ := cli(t, command, url, registerArgs(lossID, "demo-b")...)
	if exit != 0 || machineID(t, found) != lostWinner {
		t.Fatal("lost reply retry changed identity")
	}
	// P09: kill the actual service after the observable commit boundary. The
	// owned database remains running; a fresh process must reconstruct from it.
	process.stop(t, true)
	process = launch(t, agent, dsn, address, runDir)
	for _, args := range [][]string{{"registration", "inspect", requestID}, {"machine", "inspect", winner}} {
		exit, body, diag := cli(t, command, url, args...)
		if exit != 0 || machineID(t, body) != winner {
			t.Fatalf("service restart inspection exit=%d body=%s diagnostics=%s", exit, body, diag)
		}
	}
	// A second actual process fails ownership before it can serve requests.
	secondCtx, stopSecond := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopSecond()
	second := exec.CommandContext(secondCtx, agent)
	second.Env = append(os.Environ(), "PERPETUAL_DATABASE_URL="+dsn, "PERPETUAL_LISTEN_ADDRESS="+availableAddress(t), "PERPETUAL_RUNTIME_DIR="+runDir, "PERPETUAL_MAX_REGISTRATIONS=4")
	if output, err := second.CombinedOutput(); err == nil {
		t.Fatalf("second process acquired same host lock: %s", output)
	}
	if secondCtx.Err() != nil {
		t.Fatal("second process blocked instead of rejecting host ownership")
	}
	// P10 service-backed demonstration: database restart is independent from
	// the service restart above. The still-running service must reconnect.
	testfixture.Restart(t)
	deadline := time.Now().Add(10 * time.Second)
	for {
		exit, body, _ := cli(t, command, url, "registration", "inspect", requestID)
		if exit == 0 {
			if machineID(t, body) != winner {
				t.Fatal("database restart changed identity")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("service failed to reconnect after database restart")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var rows, used int
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM machine_registrations), used FROM registration_gate WHERE singleton_id=1").Scan(&rows, &used); err != nil {
		t.Fatal(err)
	}
	if rows != 2 || used != 2 {
		t.Fatalf("durable rows=%d counter=%d want2", rows, used)
	}
	process.stop(t, false)
	t.Logf("real CLI/register/conflict/lost reply/service crash/database restart verified; records=%d, count=%d", rows, used)
}
