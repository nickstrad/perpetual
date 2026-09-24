package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"perpetual/internal/invariant"
	"strings"
	"testing"
	"time"
)

func TestT08HTTPInvariantTerminates(t *testing.T) {
	if os.Getenv("PERPETUAL_FATAL_HTTP") == "1" {
		wrapped := FatalHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic(invariant.Violation{Message: "test HTTP ownership invariant"})
		}), func(v any) { _, _ = io.WriteString(os.Stderr, "fatal-http-boundary\n"); os.Exit(29) })
		server := httptest.NewServer(wrapped)
		defer server.Close()
		_, _ = server.Client().Get(server.URL)
		os.Exit(0)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestT08HTTPInvariantTerminates$")
	cmd.Env = append(os.Environ(), "PERPETUAL_FATAL_HTTP=1")
	output, err := cmd.CombinedOutput()
	if err == nil || ctx.Err() != nil || !strings.Contains(string(output), "fatal-http-boundary") {
		t.Fatalf("handler invariant did not terminate through fatal boundary: err=%v output=%s", err, output)
	}
}
