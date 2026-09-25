// Package llm: startup LLM API-key probe (ENV-CONSENSUS-1).
//
// A dead API key mimics a product defect: sessions get no assistant reply and
// the ledger stays empty, which has already been misdiagnosed twice as a bug.
// Every serve startup with a configured key therefore probes the provider
// CHEAPLY (a models-list call; a 1-token chat completion only as fallback)
// and reports the verdict loudly, naming the key's LAST 4 CHARACTERS ONLY.
//
// Security invariant: no function in this file ever formats the full key.
// Callers render the key exclusively through KeyLast4.
//
// axiom:trace work_item=ENV-CONSENSUS-1 impl=internal/llm/keyprobe.go test=internal/llm/keyprobe_test.go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// keyProbeTimeout bounds the whole probe. Startup must not hang on a black-holed
// endpoint; the timeout path classifies as network-unreachable and continues.
const keyProbeTimeout = 8 * time.Second

// KeyProbeVerdict is the result of one startup key probe.
type KeyProbeVerdict struct {
	// OK is true when the provider accepted the key (HTTP 200 on the probe).
	OK bool
	// AuthFailed is true on 401/403 — the key is invalid or revoked. This is
	// the loud, actionable case: it must surface BEFORE the server reports
	// healthy, not as a product defect on the first LLM call.
	AuthFailed bool
	// Status is the HTTP status of the decisive probe call (0 on network error).
	Status int
	// Endpoint is the URL the decisive probe call hit.
	Endpoint string
	// KeyLast4 is the last 4 characters of the probed key — the only key
	// material any log line may carry.
	KeyLast4 string
	// Err is the network/transport error when the probe endpoint itself was
	// unreachable. Non-nil implies startup continues (auth failures will
	// surface on the first real LLM call instead).
	Err error
}

// KeyLast4 returns the last 4 characters of a key, or fewer when the key is
// shorter. It is the ONLY sanctioned way to render key material anywhere.
func KeyLast4(key string) string {
	if len(key) <= 4 {
		return key
	}
	return key[len(key)-4:]
}

// IsKeyConfigured reports whether an LLM API key is actually configured:
// non-empty and not a ${VAR} placeholder (placeholders keep their own
// config-level startup WARN and must not trigger a probe against a literal
// "${...}" string).
func IsKeyConfigured(key string) bool {
	return key != "" && !strings.HasPrefix(key, "${")
}

// ProviderBaseURL resolves the effective API base URL: an explicit
// configuration value wins (trailing slash trimmed), otherwise the provider
// default. Both client constructors use this so the probe and the real client
// can never disagree about which host holds the key.
func ProviderBaseURL(provider Provider, baseURL string) string {
	if baseURL = strings.TrimRight(baseURL, "/"); baseURL != "" {
		return baseURL
	}
	switch provider {
	case ProviderOpenRouter:
		return "https://openrouter.ai/api/v1"
	case ProviderAnthropic:
		return "https://api.anthropic.com/v1"
	default:
		return "https://api.openai.com/v1"
	}
}

// ProbeKey checks an LLM API key against its provider with the cheapest
// possible call and classifies the outcome. It never blocks on network
// problems (that is the caller's policy) and never exposes the full key.
//
// Endpoint choice:
//   - openai/openrouter/openai-compatible (incl. unknown providers): GET
//     {base}/models; on 404/405 fall back to a 1-token chat completion.
//   - anthropic: a 1-token POST {base}/messages (Anthropic's models list is
//     not on the same /v1 surface as the Messages API, and a 1-token probe
//     is the provider-appropriate cheap check).
//
// httpClient may be nil (a short-timeout client is created); tests inject
// httptest servers via baseURL.
func ProbeKey(ctx context.Context, httpClient *http.Client, provider Provider, baseURL, apiKey, model string) *KeyProbeVerdict {
	v := &KeyProbeVerdict{KeyLast4: KeyLast4(apiKey)}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: keyProbeTimeout}
	}

	base := ProviderBaseURL(provider, baseURL)

	switch provider {
	case ProviderAnthropic:
		// 1-token probe of the Messages API (x-api-key auth).
		v.Status, v.Endpoint, v.Err = probeAnthropicMessages(ctx, httpClient, base, apiKey, model)
	default:
		// OpenAI-compatible: models list first (zero tokens), chat fallback.
		v.Status, v.Endpoint, v.Err = probeOpenAIModels(ctx, httpClient, base, apiKey)
		if v.Err == nil && (v.Status == http.StatusNotFound || v.Status == http.StatusMethodNotAllowed) {
			v.Status, v.Endpoint, v.Err = probeOpenAIChat(ctx, httpClient, base, apiKey, model)
		}
	}

	v.OK = v.Err == nil && v.Status >= 200 && v.Status < 300
	v.AuthFailed = v.Err == nil && (v.Status == http.StatusUnauthorized || v.Status == http.StatusForbidden)
	return v
}

// probeOpenAIModels issues GET {base}/models with Bearer auth.
func probeOpenAIModels(ctx context.Context, httpClient *http.Client, base, apiKey string) (int, string, error) {
	endpoint := base + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, endpoint, fmt.Errorf("llm probe: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	status, err := doProbe(httpClient, req)
	return status, endpoint, err
}

// probeOpenAIChat issues a 1-token chat completion — the fallback probe when
// the gateway does not expose /models.
func probeOpenAIChat(ctx context.Context, httpClient *http.Client, base, apiKey, model string) (int, string, error) {
	endpoint := base + "/chat/completions"
	if model == "" {
		model = "gpt-4o"
	}
	body, err := json.Marshal(map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
	})
	if err != nil {
		return 0, endpoint, fmt.Errorf("llm probe: marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, endpoint, fmt.Errorf("llm probe: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	status, err := doProbe(httpClient, req)
	return status, endpoint, err
}

// probeAnthropicMessages issues a 1-token Messages-API probe with x-api-key
// auth, mirroring anthropic_client.go's header contract.
func probeAnthropicMessages(ctx context.Context, httpClient *http.Client, base, apiKey, model string) (int, string, error) {
	endpoint := base + "/messages"
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	body, err := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 1,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
	})
	if err != nil {
		return 0, endpoint, fmt.Errorf("llm probe: marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, endpoint, fmt.Errorf("llm probe: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	status, err := doProbe(httpClient, req)
	return status, endpoint, err
}

// doProbe executes the request and returns (status, transportErr). The
// response body is drained and closed; its content is deliberately NOT parsed
// or logged — the status code classifies the verdict, and provider error
// bodies could echo the submitted key.
func doProbe(httpClient *http.Client, req *http.Request) (int, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
