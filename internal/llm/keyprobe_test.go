// Package llm: tests for the startup API-key probe (ENV-CONSENSUS-1).
//
// Dead API keys mimic product defects (sessions with no assistant reply and
// an empty ledger), so the probe must classify auth failure loudly while
// never exposing more than the key's last 4 characters. These tests pin the
// verdict shape, the last-4 derivation, the provider endpoint choice, and
// the no-leak invariant on every field the probe can produce.
//
// Test keys here are deliberately short after the "sk-" prefix: the repo
// secrets guard scans for sk-[a-zA-Z0-9_-]{20,} and must never match them.
//
// axiom:trace work_item=ENV-CONSENSUS-1 impl=internal/llm/keyprobe.go test=internal/llm/keyprobe_test.go
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// probeTestKey is long enough to make leak assertions meaningful (a truncated
// copy of a real key must not survive anywhere in a verdict) but stays under
// the repo secrets-guard threshold sk-[a-zA-Z0-9_-]{20,}.
const probeTestKey = "sk-probe-live-424987"

// assertNoKeyLeak fails the test when the verdict (or its error) exposes more
// than the last 4 characters of key.
func assertNoKeyLeak(t *testing.T, key string, v *KeyProbeVerdict) {
	t.Helper()
	rendered := fmt.Sprintf("%+v", *v)
	if v.Err != nil {
		rendered += "\n" + v.Err.Error()
	}
	if strings.Contains(rendered, key) {
		t.Errorf("verdict leaks the full key: %s", rendered)
	}
	prefix := key[:len(key)-4]
	if strings.Contains(rendered, prefix) {
		t.Errorf("verdict exposes more than the last 4 characters (found prefix %q): %s", prefix, rendered)
	}
}

func TestProbeKey_ModelsListOK(t *testing.T) {
	var seenMethod, seenPath, seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod, seenPath = r.Method, r.URL.Path
		seenAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"m1"}]}`)
	}))
	defer srv.Close()

	v := ProbeKey(context.Background(), nil, ProviderOpenAI, srv.URL, probeTestKey, "test-model")

	if seenMethod != http.MethodGet || seenPath != "/models" {
		t.Errorf("probe hit %s %s, want GET /models (cheap, no tokens)", seenMethod, seenPath)
	}
	if seenAuth != "Bearer "+probeTestKey {
		t.Errorf("Authorization = %q, want the Bearer probe key", seenAuth)
	}
	if !v.OK || v.AuthFailed || v.Status != http.StatusOK {
		t.Fatalf("verdict = %+v, want OK status 200", v)
	}
	if v.KeyLast4 != "4987" {
		t.Errorf("KeyLast4 = %q, want 4987", v.KeyLast4)
	}
	if !strings.HasSuffix(v.Endpoint, "/models") {
		t.Errorf("Endpoint = %q, want the /models endpoint", v.Endpoint)
	}
	assertNoKeyLeak(t, probeTestKey, v)
}

func TestProbeKey_AuthFailedStatuses(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			fmt.Fprint(w, `{"error":{"message":"invalid api key"}}`)
		}))
		v := ProbeKey(context.Background(), nil, ProviderOpenAI, srv.URL, probeTestKey, "test-model")
		srv.Close()

		if !v.AuthFailed || v.OK {
			t.Fatalf("status %d: verdict = %+v, want AuthFailed", status, v)
		}
		if v.Status != status {
			t.Errorf("status = %d, want %d", v.Status, status)
		}
		assertNoKeyLeak(t, probeTestKey, v)
	}
}

func TestProbeKey_NetworkErrorIsPureNetworkVerdict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // endpoint now refuses connections

	v := ProbeKey(context.Background(), nil, ProviderOpenAI, url, probeTestKey, "test-model")

	if v.Err == nil {
		t.Fatal("expected a network-error verdict for a refused endpoint")
	}
	if v.OK || v.AuthFailed || v.Status != 0 {
		t.Errorf("verdict = %+v, want a pure network failure (no status, no auth verdict)", v)
	}
	assertNoKeyLeak(t, probeTestKey, v)
}

func TestProbeKey_InconclusiveStatusIsNeitherOKNorAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	v := ProbeKey(context.Background(), nil, ProviderOpenAI, srv.URL, probeTestKey, "test-model")

	if v.OK || v.AuthFailed || v.Err != nil {
		t.Errorf("verdict = %+v, want inconclusive (HTTP 503 is neither ok nor auth-failed)", v)
	}
	if v.Status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", v.Status)
	}
}

func TestProbeKey_FallsBackToChatWhenModelsMissing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r) // 404 — OpenAI-compatible gateway without /models
	})
	var chatAuth string
	var chatBody []byte
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		chatAuth = r.Header.Get("Authorization")
		chatBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	v := ProbeKey(context.Background(), nil, ProviderOpenAI, srv.URL, probeTestKey, "test-model")

	if !v.OK {
		t.Fatalf("verdict = %+v, want OK via the 1-token chat fallback", v)
	}
	if !strings.HasSuffix(v.Endpoint, "/chat/completions") {
		t.Errorf("Endpoint = %q, want the chat fallback endpoint", v.Endpoint)
	}
	if chatAuth != "Bearer "+probeTestKey {
		t.Errorf("chat Authorization = %q, want the Bearer probe key", chatAuth)
	}
	var body map[string]any
	if err := json.Unmarshal(chatBody, &body); err != nil {
		t.Fatalf("chat body is not JSON: %v", err)
	}
	if body["model"] != "test-model" {
		t.Errorf("chat model = %v, want test-model", body["model"])
	}
	if mt, ok := body["max_tokens"].(float64); !ok || mt != 1 {
		t.Errorf("chat max_tokens = %v, want 1 (the probe must stay cheap)", body["max_tokens"])
	}
}

func TestProbeKey_AnthropicUsesMessagesWithXAPIKey(t *testing.T) {
	var gotKey, gotVersion, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"content":[{"type":"text","text":"hi"}]}`)
	}))
	defer srv.Close()

	v := ProbeKey(context.Background(), nil, ProviderAnthropic, srv.URL, probeTestKey, "test-model")

	if gotPath != "/messages" {
		t.Errorf("anthropic probe hit %s, want /messages", gotPath)
	}
	if gotKey != probeTestKey {
		t.Errorf("x-api-key = %q, want the probe key", gotKey)
	}
	if gotVersion == "" {
		t.Error("anthropic-version header missing on the probe")
	}
	if !v.OK || v.AuthFailed {
		t.Fatalf("verdict = %+v, want OK", v)
	}
	assertNoKeyLeak(t, probeTestKey, v)
}

func TestKeyLast4(t *testing.T) {
	cases := []struct{ key, want string }{
		{"", ""},
		{"abc", "abc"},
		{"abcd", "abcd"},
		{probeTestKey, "4987"},
	}
	for _, tc := range cases {
		if got := KeyLast4(tc.key); got != tc.want {
			t.Errorf("KeyLast4(%q) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestIsKeyConfigured(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"", false},
		{"${DEEPSEEK_API_KEY}", false}, // placeholder keeps its own config WARN
		{"${CONSENSUS_API_KEY}", false},
		{probeTestKey, true},
	}
	for _, tc := range cases {
		if got := IsKeyConfigured(tc.key); got != tc.want {
			t.Errorf("IsKeyConfigured(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestProviderBaseURL(t *testing.T) {
	cases := []struct {
		name     string
		provider Provider
		baseURL  string
		want     string
	}{
		{"openai default", ProviderOpenAI, "", "https://api.openai.com/v1"},
		{"openrouter default", ProviderOpenRouter, "", "https://openrouter.ai/api/v1"},
		{"anthropic default", ProviderAnthropic, "", "https://api.anthropic.com/v1"},
		{"unknown provider defaults openai-compatible", Provider("some-proxy"), "", "https://api.openai.com/v1"},
		{"explicit base wins over provider default", ProviderOpenAI, "https://api.deepseek.com/v1/", "https://api.deepseek.com/v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProviderBaseURL(tc.provider, tc.baseURL); got != tc.want {
				t.Errorf("ProviderBaseURL(%q, %q) = %q, want %q", tc.provider, tc.baseURL, got, tc.want)
			}
		})
	}
}
