// cmd/consensus: startup LLM API-key probe wiring (ENV-CONSENSUS-1).
//
// A dead API key mimics a product defect — sessions get no assistant reply
// and the ledger stays empty — and has already been misdiagnosed twice. On
// serve startup with a configured key, probe the provider BEFORE the harness
// takes traffic and report the verdict loudly, naming the key's LAST 4
// CHARACTERS ONLY.
//
// Testability contract: the production entry point runLLMKeyProbe is a no-op
// under `go test` (flag.Lookup("test.v") detects the test binary), and both
// the probe function (llmProbeFn) and its HTTP client (llmProbeHTTPClient)
// are package vars that tests substitute — the probe is injectable, never a
// live call inside a unit test.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"time"

	"github.com/wojons/consensus/internal/llm"
)

// llmProbeFn is the seam for the LLM key probe; tests substitute it.
var llmProbeFn = llm.ProbeKey

// llmProbeHTTPClient is the client the startup probe uses; tests may point it
// at an httptest server. nil lets the probe create its own short-timeout
// client.
var llmProbeHTTPClient *http.Client

// runningUnderGoTest reports whether the current binary is a `go test`
// binary: the testing package registers test.* flags on flag.CommandLine,
// and production binaries never have them.
func runningUnderGoTest() bool {
	return flag.Lookup("test.v") != nil
}

// runLLMKeyProbe is the production entry point, called once from the serve
// path after the LLM client is constructed. Under `go test` it is a no-op so
// unit tests never make live calls; otherwise it runs the probe under a
// bounded deadline so a black-holed endpoint can never stall startup.
func runLLMKeyProbe(provider llm.Provider, baseURL, apiKey, model string) {
	if runningUnderGoTest() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	probeLLMKeyStartup(ctx, provider, baseURL, apiKey, model)
}

// probeLLMKeyStartup probes the configured LLM key and logs the verdict at
// the level the verdict deserves:
//
//	ok      → single INFO line naming last-4, provider, base URL
//	401/403 → LOUD ERROR naming last-4 + provider + base URL, stating that
//	          auth failed and the key is invalid or revoked — the dead-key
//	          symptom (no assistant reply, empty ledger) is a configuration
//	          failure, not a product defect
//	network → WARN that the probe endpoint was unreachable; startup
//	          continues because auth will surface on the first LLM call
//	other   → WARN with the status (429, 5xx, unexpected): inconclusive
//
// No key configured (empty or ${VAR} placeholder) skips the probe entirely —
// that case keeps its existing config-level WARN ("No LLM API key
// configured — ..." from ApplyStartupValidations).
func probeLLMKeyStartup(ctx context.Context, provider llm.Provider, baseURL, apiKey, model string) {
	if !llm.IsKeyConfigured(apiKey) {
		return
	}

	v := llmProbeFn(ctx, llmProbeHTTPClient, provider, baseURL, apiKey, model)

	switch {
	case v.OK:
		slog.Info("llm key probe ok (..."+v.KeyLast4+")",
			"provider", string(provider),
			"base_url", llm.ProviderBaseURL(provider, baseURL),
		)
	case v.AuthFailed:
		slog.Error("LLM AUTH FAILED for key ..."+v.KeyLast4+
			" — key invalid or revoked; dead keys mimic product defects (no assistant reply, empty ledger). "+
			"Fix the key before diagnosing product failures.",
			"provider", string(provider),
			"base_url", llm.ProviderBaseURL(provider, baseURL),
			"status", v.Status,
		)
	case v.Err != nil:
		slog.Warn("llm key probe unreachable (network) — continuing; auth failures will surface on first LLM call",
			"provider", string(provider),
			"base_url", llm.ProviderBaseURL(provider, baseURL),
			"error", v.Err,
		)
	default:
		slog.Warn("llm key probe inconclusive (unexpected status) — continuing; auth failures will surface on first LLM call",
			"provider", string(provider),
			"base_url", llm.ProviderBaseURL(provider, baseURL),
			"status", v.Status,
		)
	}
}
