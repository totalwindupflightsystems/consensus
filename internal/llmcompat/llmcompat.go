// Package llmcompat serves the OpenAI-compatible transport surface
// (/v1/chat/completions) as a transparent pass-through to the configured LLM
// provider base URL.
//
// WHY: DF-CONSENSUS-39. The pinned upstream opencode SDK suite
// (packages/opencode/test/server/httpapi-sdk.test.ts, rev 16747470,
// v1.18.29) asserts that a REST API prompt carries project skills in the
// prompt context the LLM sees. Its fixture registers provider
// test/test-model pointed at an in-process fake LLM (TestLLMServer, a real
// HTTP server on a loopback port) and asserts the captured request body
// contains the skill name. Upstream's own server calls that endpoint
// itself; a shim (out-of-process) cannot — the only transport the shim can
// legitimately serve is the OpenAI-compatible POST /v1/chat/completions,
// which the suite's preload currently hijacks to this shim and which
// answered "404 page not found" (live-proven 2026-09-30: harness log
// `planning: LLM call failed … llm: HTTP 404`).
//
// SPEC-018 §3 reserves the /v1/chat/completions surface as PLANNED:
// "OpenAI/Anthropic compat endpoints (/v1/chat/completions, /v1/models, …)
// — PLANNED / NOT IMPLEMENTED. … When implemented, they belong in new
// paths files (e.g. shim-openai.yaml)". This package implements the
// chat-completions member of that reservation as a pure proxy: request and
// response bytes are forwarded unchanged (streaming included), so the
// provider's contract — whatever it is — is preserved end to end and no
// Consensus-specific transformation can leak into the wire shape.
package llmcompat

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// NewPassThrough returns a handler that forwards every request to the
// OpenAI-compatible endpoint at providerBaseURL (e.g.
// https://api.deepseek.com/v1). The URL path is appended to the base path,
// so POST /v1/chat/completions → <base>/chat/completions when base already
// ends in /v1, matching the upstream providers' documented contract.
//
// An empty providerBaseURL is a configuration error the caller must
// detect (ServeMux would otherwise panic on a nil handler); New returns
// http.HandlerFunc that answers 503 SERVICE_UNAVAILABLE instead, so a
// misconfigured deployment degrades loudly rather than at bind time.
func NewPassThrough(providerBaseURL string) http.Handler {
	if strings.TrimSpace(providerBaseURL) == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":{"code":"LLM_ENDPOINT_UNCONFIGURED","message":"no llm.base_url configured; cannot serve the OpenAI-compatible transport"}}`, http.StatusServiceUnavailable)
		})
	}
	target, err := url.Parse(providerBaseURL)
	if err != nil {
		slog.Error("llmcompat: provider base URL unparsable; pass-through disabled",
			"error", err)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":{"code":"LLM_ENDPOINT_INVALID","message":"llm.base_url is not a valid URL; cannot serve the OpenAI-compatible transport"}}`, http.StatusServiceUnavailable)
		})
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	// NewSingleHostReverseProxy joins the base's path with the incoming
	// path, so a base that already ends in /v1 (the documented
	// DeepSeek/OpenAI shape) would double the prefix: /v1/chat/completions
	// in → /v1/v1/chat/completions out. Rewrite the path relative to the
	// base's own path instead:
	//   in /v1/chat/completions + base .../v1  → out /v1/chat/completions
	//   in /chat/completions     + base .../v1  → out /v1/chat/completions
	origDirector := proxy.Director
	basePath := strings.TrimRight(target.Path, "/")
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		if basePath == "" {
			return
		}
		// origDirector already produced basePath + incoming; strip one
		// copy of the base prefix, then re-attach exactly once.
		joined := req.URL.Path
		rel := strings.TrimPrefix(joined, basePath)
		rel = strings.TrimPrefix(rel, basePath) // in case JoinPath doubled it
		req.URL.Path = basePath + rel
		req.URL.RawPath = ""
	}
	// The transport-level error is reported to the caller as a 502 with a
	// JSON body (upstream shape) instead of the default bare text.
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Warn("llmcompat: provider request failed",
			"path", r.URL.Path, "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"code":"LLM_PROVIDER_UNREACHABLE","message":"provider endpoint unreachable"}}` + "\n"))
	}
	return proxy
}

// MountPatterns lists the chi router patterns required to expose the
// pass-through under the production router. Mirrors shim.MountPatterns.
var MountPatterns = []string{
	"/v1/chat/completions",
}

// ServedPath is the single path the pass-through claims. The H3 shim keeps
// the rest of /v1/* (its own protocol surface).
const ServedPath = "/v1/chat/completions"
