package llmcompat

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// upstream is a stand-in for the provider endpoint: it records what it was
// called with and answers a fixed chat-completion body.
func upstream(t *testing.T, hit *int, sawPath *string, sawBody *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hit++
		*sawPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		*sawBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
}

// The core contract: POST /v1/chat/completions reaches the provider with
// the incoming bytes and the provider's response bytes come back untouched.
// DF-CONSENSUS-39: the upstream suite's fixture needs its fake LLM's exact
// wire shape preserved; any transformation here would corrupt the fixture's
// captured inputs.
func TestPassThroughForwardsChatCompletions(t *testing.T) {
	var hit int
	var sawPath, sawBody string
	prov := upstream(t, &hit, &sawPath, &sawBody)
	defer prov.Close()

	h := NewPassThrough(prov.URL)
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := `{"model":"test-model","messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if hit != 1 {
		t.Fatalf("provider hits = %d, want 1", hit)
	}
	if sawPath != "/v1/chat/completions" {
		t.Errorf("provider path = %q, want /v1/chat/completions", sawPath)
	}
	if sawBody != body {
		t.Errorf("provider body = %q, want %q (bytes must pass through unchanged)", sawBody, body)
	}
	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), `"ok"`) {
		t.Errorf("response body = %q, want the provider's answer", got)
	}
}

// Base URLs that already end in /v1 (the documented DeepSeek/OpenAI shape)
// must not double the prefix: /v1/chat/completions in → /v1/chat/completions
// out, not /v1/v1/chat/completions.
func TestPassThroughBaseURLWithV1SuffixNoDoublePrefix(t *testing.T) {
	var sawPath string
	var hit int
	prov := upstream(t, &hit, &sawPath, new(string))
	defer prov.Close()

	// Append a fake /v1 to the test server's URL so the base carries the
	// documented suffix.
	base := prov.URL + "/v1"
	h := NewPassThrough(base)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	resp.Body.Close()

	if hit != 1 {
		t.Fatalf("provider hits = %d, want 1", hit)
	}
	if sawPath != "/v1/chat/completions" {
		t.Errorf("provider path = %q, want /v1/chat/completions (no /v1/v1 doubling)", sawPath)
	}
}

// An unparsable base URL must degrade loudly (503), not panic at mount time
// or silently black-hole requests.
func TestPassThroughUnparsableBaseURLAnswers503(t *testing.T) {
	h := NewPassThrough("ht tp://not a url")
	// url.Parse is lenient; force the failure mode by hand-verifying only
	// when Parse actually errors, otherwise skip (the handler already
	// checked). This guards the contract, not Go's parser.
	if _, err := url.Parse("ht tp://not a url"); err == nil {
		t.Skip("net/url accepts this input; unparsable-base branch not exercised")
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

// An empty base URL must answer 503 naming the missing config, not 404 —
// the caller has to be able to tell "endpoint exists, config missing" from
// "no such route" (same contract as the shim's typed not-implemented bodies).
func TestPassThroughEmptyBaseURLAnswers503WithReason(t *testing.T) {
	h := NewPassThrough("")
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "LLM_ENDPOINT_UNCONFIGURED") {
		t.Errorf("body = %q, want the LLM_ENDPOINT_UNCONFIGURED reason", b)
	}
}

// The H3 shim owns the rest of /v1/*: the pass-through must claim only the
// chat-completions path, never the whole subtree.
func TestMountPatternsClaimOnlyChatCompletions(t *testing.T) {
	if len(MountPatterns) != 1 || MountPatterns[0] != "/v1/chat/completions" {
		t.Errorf("MountPatterns = %v, want exactly [\"/v1/chat/completions\"]", MountPatterns)
	}
	if ServedPath != "/v1/chat/completions" {
		t.Errorf("ServedPath = %q, want /v1/chat/completions", ServedPath)
	}
}

// A dead provider must surface as 502 with a JSON body (loud, parseable),
// not hang or return the proxy's bare text error.
func TestPassThroughDeadProviderAnswers502(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close() // port now free — nothing listens

	h := NewPassThrough(deadURL)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "LLM_PROVIDER_UNREACHABLE") {
		t.Errorf("body = %q, want the LLM_PROVIDER_UNREACHABLE reason", b)
	}
}
