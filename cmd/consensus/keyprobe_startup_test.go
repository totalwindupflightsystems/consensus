// cmd/consensus: wiring tests for the startup LLM key probe (ENV-CONSENSUS-1).
//
// These pin the startup contract: probe only when a real key is configured,
// never under `go test`, and never log more than the key's last 4 characters
// on any verdict path. The HTTP layer is exercised through httptest servers
// via the llmProbeHTTPClient seam — the wiring tests make no live calls.
//
// Test keys here keep the post-"sk-" suffix under 20 characters so the repo
// secrets guard (sk-[a-zA-Z0-9_-]{20,}) can never match them.
//
// axiom:trace work_item=ENV-CONSENSUS-1 impl=cmd/consensus/keyprobe_startup.go test=cmd/consensus/keyprobe_startup_test.go
package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wojons/consensus/internal/llm"
)

// wiringKeyDead / wiringKeyOk: distinctive, leak-checkable, guard-safe.
const (
	wiringKeyDead = "sk-wiring-dead-778811"
	wiringKeyOk   = "sk-wiring-ok-9933"
)

// captureSlog redirects the default logger to a buffer for the test's
// duration and returns it.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// swapProbeSeam replaces the probe function/client seams and restores them
// on cleanup.
func swapProbeSeam(t *testing.T, fn func(ctx context.Context, httpClient *http.Client, provider llm.Provider, baseURL, apiKey, model string) *llm.KeyProbeVerdict, client *http.Client) {
	t.Helper()
	prevFn, prevClient := llmProbeFn, llmProbeHTTPClient
	llmProbeFn, llmProbeHTTPClient = fn, client
	t.Cleanup(func() { llmProbeFn, llmProbeHTTPClient = prevFn, prevClient })
}

func TestProbeLLMKeyStartup_AuthFailureLogsLoudErrorWithLast4Only(t *testing.T) {
	restore := captureSlog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	swapProbeSeam(t, llm.ProbeKey, srv.Client())

	probeLLMKeyStartup(context.Background(), llm.ProviderOpenAI, srv.URL, wiringKeyDead, "test-model")

	logs := restore.String()
	if !strings.Contains(logs, "LLM AUTH FAILED") {
		t.Errorf("expected a LOUD auth-failure error line, got:\n%s", logs)
	}
	if !strings.Contains(logs, "...8811") {
		t.Errorf("expected the key last-4 (...8811) in the log, got:\n%s", logs)
	}
	if strings.Contains(logs, wiringKeyDead) || strings.Contains(logs, wiringKeyDead[:len(wiringKeyDead)-4]) {
		t.Errorf("log exposes more than the last 4 characters:\n%s", logs)
	}
	if !strings.Contains(logs, "invalid or revoked") {
		t.Errorf("expected the dead-key diagnostic in the log, got:\n%s", logs)
	}
}

func TestProbeLLMKeyStartup_SuccessLogsSingleInfoLine(t *testing.T) {
	restore := captureSlog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	swapProbeSeam(t, llm.ProbeKey, srv.Client())

	probeLLMKeyStartup(context.Background(), llm.ProviderOpenAI, srv.URL, wiringKeyOk, "test-model")

	logs := restore.String()
	if strings.Count(logs, "llm key probe ok") != 1 {
		t.Errorf("expected exactly one ok probe line, got:\n%s", logs)
	}
	if !strings.Contains(logs, "...9933") || !strings.Contains(logs, "provider=openai") {
		t.Errorf("ok line must name last-4 and provider, got:\n%s", logs)
	}
	if strings.Contains(logs, wiringKeyOk) || strings.Contains(logs, wiringKeyOk[:len(wiringKeyOk)-4]) {
		t.Errorf("log exposes more than the last 4 characters:\n%s", logs)
	}
}

func TestProbeLLMKeyStartup_NetworkFailureWarnsAndContinues(t *testing.T) {
	restore := captureSlog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // refuses connections now
	swapProbeSeam(t, llm.ProbeKey, http.DefaultClient)

	probeLLMKeyStartup(context.Background(), llm.ProviderOpenAI, url, wiringKeyOk, "test-model")

	logs := restore.String()
	if !strings.Contains(logs, "llm key probe unreachable (network)") {
		t.Errorf("expected the network WARN (startup must continue), got:\n%s", logs)
	}
	if !strings.Contains(logs, "auth failures will surface on first LLM call") {
		t.Errorf("expected the continuation notice, got:\n%s", logs)
	}
}

func TestProbeLLMKeyStartup_SkipsUnconfiguredAndPlaceholderKeys(t *testing.T) {
	restore := captureSlog(t)
	var calls atomic.Int32
	swapProbeSeam(t,
		func(ctx context.Context, httpClient *http.Client, provider llm.Provider, baseURL, apiKey, model string) *llm.KeyProbeVerdict {
			calls.Add(1)
			return &llm.KeyProbeVerdict{}
		}, nil)

	for _, key := range []string{"", "${DEEPSEEK_API_KEY}", "${CONSENSUS_API_KEY}"} {
		probeLLMKeyStartup(context.Background(), llm.ProviderOpenAI, "https://unused.invalid", key, "m")
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("probe ran %d times for unconfigured/placeholder keys, want 0 (config WARN covers that case)", got)
	}
	if restore.String() != "" {
		t.Errorf("expected no probe log lines, got:\n%s", restore.String())
	}
}

func TestRunLLMKeyProbe_IsNoOpUnderGoTest(t *testing.T) {
	var called atomic.Bool
	swapProbeSeam(t,
		func(ctx context.Context, httpClient *http.Client, provider llm.Provider, baseURL, apiKey, model string) *llm.KeyProbeVerdict {
			called.Store(true)
			return &llm.KeyProbeVerdict{}
		}, nil)

	// A configured key: in a production binary this would probe. Inside a
	// `go test` binary the entry point must skip.
	runLLMKeyProbe(llm.ProviderOpenAI, "https://unused.invalid", wiringKeyOk, "m")

	if called.Load() {
		t.Error("runLLMKeyProbe probed inside a go test binary — the go-test skip is broken")
	}
}
