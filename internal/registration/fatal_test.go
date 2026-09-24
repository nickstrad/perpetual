package registration

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type invariantStore struct{ blockingStore }

func (s *invariantStore) Reserve(context.Context, Request) Outcome {
	return Outcome{Kind: OutcomeCreated}
}
func TestT08WorkerInvariantTerminates(t *testing.T) {
	if os.Getenv("PERPETUAL_FATAL_WORKER") == "1" {
		s, err := NewService(&invariantStore{}, ServiceOptions{Epoch: "fatal", MaxJobs: 1, QueueSize: 1, Workers: 1, OperationTimeout: time.Second})
		if err != nil {
			os.Exit(25)
		}
		_ = s.Register(context.Background(), baseRequest())
		os.Exit(0)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestT08WorkerInvariantTerminates$")
	cmd.Env = append(os.Environ(), "PERPETUAL_FATAL_WORKER=1")
	output, err := cmd.CombinedOutput()
	if err == nil || ctx.Err() != nil || !(strings.Contains(string(output), "request") || strings.Contains(string(output), "invariant")) {
		t.Fatalf("worker invariant did not terminate: err=%v output=%s", err, output)
	}
}
