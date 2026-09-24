// Package llm: additional tests for DOGFOOD-004 (actionable LLM error UX).
//
// axiom:trace work_item=dogfood-004 spec=specs/008-harness.md test=internal/llm/dogfood004_test.go
package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/harness"
)

// TestOpenAIClient_Non2xxAuthError_IsActionable verifies that a 401 response
// with a non-JSON body (e.g. plaintext from a proxy or gateway) surfaces an
// actionable auth error instead of a cryptic JSON parse error (DOGFOOD-004).
func TestOpenAIClient_Non2xxAuthError_IsActionable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized — invalid API key provided"))
	}))
	defer server.Close()

	client := NewOpenAIClient(&Config{
		Provider: ProviderOpenAI,
		BaseURL:  server.URL,
		APIKey:   "sk-bad-key",
	})

	_, err := client.Call(context.Background(), []harness.Message{
		{Role: "user", Content: "hello"},
	})
	if err == nil {
		t.Fatal("expected error from 401 response")
	}

	msg := err.Error()
	if !strings.Contains(msg, "HTTP 401") {
		t.Errorf("error should mention HTTP status 401, got: %q", msg)
	}
	if !strings.Contains(msg, "check your API key") {
		t.Errorf("error should be actionable (hint to check API key), got: %q", msg)
	}
	if strings.Contains(msg, "invalid character") {
		t.Errorf("error must NOT be a JSON parse error, got: %q", msg)
	}
}

// TestOpenAIClient_Non2xxJSONError_SurfacesProviderMessage verifies that a
// structured OpenAI-style error body is surfaced with the provider's message.
func TestOpenAIClient_Non2xxJSONError_SurfacesProviderMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Incorrect API key provided","type":"invalid_request_error","code":"invalid_api_key"}}`))
	}))
	defer server.Close()

	client := NewOpenAIClient(&Config{
		Provider: ProviderOpenAI,
		BaseURL:  server.URL,
		APIKey:   "sk-bad-key",
	})

	_, err := client.Call(context.Background(), []harness.Message{
		{Role: "user", Content: "hello"},
	})
	if err == nil {
		t.Fatal("expected error from 401 response")
	}

	msg := err.Error()
	if !strings.Contains(msg, "Incorrect API key provided") {
		t.Errorf("error should surface the provider's message, got: %q", msg)
	}
}

// TestAnthropicClient_Non2xxAuthError_IsActionable verifies the Anthropic
// client also checks HTTP status before parsing (DOGFOOD-004).
func TestAnthropicClient_Non2xxAuthError_IsActionable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(&Config{
		Provider: ProviderAnthropic,
		BaseURL:  server.URL,
		APIKey:   "sk-ant-bad",
	})

	_, err := client.Call(context.Background(), []harness.Message{
		{Role: "user", Content: "hello"},
	})
	if err == nil {
		t.Fatal("expected error from 401 response")
	}

	msg := err.Error()
	if !strings.Contains(msg, "HTTP 401") {
		t.Errorf("error should mention HTTP status 401, got: %q", msg)
	}
	if !strings.Contains(msg, "invalid x-api-key") {
		t.Errorf("error should surface the provider message, got: %q", msg)
	}
}
