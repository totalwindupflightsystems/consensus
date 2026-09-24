// Package mcp: regression tests for DOGFOOD-106 — `consensus mcp-stdio
// --api-key cs_ak_...` (or CONSENSUS_API_KEY) must authenticate. The
// documented invocation launches the server as a subprocess; a bare
// initialize request carries no _meta.authorization, so the stdio transport
// injects the process-configured key into the initialize handshake.
//
// axiom:trace work_item=dogfood-106 spec=specs/015-api-and-mcp.md test=internal/mcp/dogfood106_test.go
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// DOGFOOD-106: injectInitializeAuth
// ============================================================================

func TestInjectInitializeAuth(t *testing.T) {
	t.Run("empty params unchanged", func(t *testing.T) {
		if got := injectInitializeAuth(nil, "cs_ak_testkey"); got != nil {
			t.Errorf("expected nil params, got %q", got)
		}
	})

	t.Run("empty key unchanged", func(t *testing.T) {
		params := json.RawMessage(`{"protocolVersion":"2024-11-05"}`)
		if got := injectInitializeAuth(params, ""); string(got) != string(params) {
			t.Errorf("expected params unchanged, got %q", got)
		}
	})

	t.Run("injects bearer key into bare params", func(t *testing.T) {
		got := injectInitializeAuth(json.RawMessage(`{}`), "cs_ak_testkey")
		var p map[string]any
		if err := json.Unmarshal(got, &p); err != nil {
			t.Fatalf("invalid JSON after injection: %v (got %q)", err, got)
		}
		meta, ok := p["_meta"].(map[string]any)
		if !ok {
			t.Fatalf("expected _meta object, got %v", p["_meta"])
		}
		if auth := meta["authorization"]; auth != "Bearer cs_ak_testkey" {
			t.Errorf("expected 'Bearer cs_ak_testkey', got %v", auth)
		}
	})

	t.Run("preserves client-supplied authorization", func(t *testing.T) {
		params := json.RawMessage(`{"_meta":{"authorization":"Bearer cs_ak_clientkey"}}`)
		if got := injectInitializeAuth(params, "cs_ak_testkey"); string(got) != string(params) {
			t.Errorf("expected params unchanged, got %q", got)
		}
	})

	t.Run("invalid JSON unchanged", func(t *testing.T) {
		params := json.RawMessage(`{not json`)
		if got := injectInitializeAuth(params, "cs_ak_testkey"); string(got) != string(params) {
			t.Errorf("expected params unchanged, got %q", got)
		}
	})
}

// ============================================================================
// DOGFOOD-106: stdio transport honors the configured key
// ============================================================================

// TestServeStdio_ConfiguredKey_InitializeAuthenticates is the DOGFOOD-106
// acceptance test: a process started with a configured key (--api-key /
// CONSENSUS_API_KEY → SetAPIKey) authenticates a bare initialize that carries
// no _meta.authorization — the transport injects the key and the handshake
// returns serverInfo instead of 'Authentication required'.
func TestServeStdio_ConfiguredKey_InitializeAuthenticates(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": "key-1", "scope": "admin", "session_id": nil},
		},
	}
	srv := NewServer(mock)
	srv.SetAPIKey("cs_ak_testkey")

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	var out bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.serveStdioIO(ctx, in, &out) }()

	if err := <-done; err != nil {
		t.Fatalf("serveStdioIO returned error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON-RPC response: %v (body=%q)", err, out.String())
	}
	if errObj, ok := resp["error"]; ok {
		t.Fatalf("initialize with configured key failed: %v", errObj)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %v", resp["result"])
	}
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("expected serverInfo in result, got %v", result)
	}
	if serverInfo["name"] != "consensus" {
		t.Errorf("expected serverInfo.name 'consensus', got %v", serverInfo["name"])
	}
}

// TestServeStdio_NoKey_InitializeAuthenticationRequired verifies the
// DOGFOOD-101 gate stays intact: without a configured key, a bare initialize
// still returns Authentication required — the fix opens no unauthenticated
// hole.
func TestServeStdio_NoKey_InitializeAuthenticationRequired(t *testing.T) {
	srv := NewServer(&mockMCPDB{})

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	var out bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.serveStdioIO(ctx, in, &out) }()

	if err := <-done; err != nil {
		t.Fatalf("serveStdioIO returned error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON-RPC response: %v (body=%q)", err, out.String())
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected JSON-RPC error, got %v", resp)
	}
	if errObj["message"] != "Authentication required" {
		t.Errorf("expected 'Authentication required', got %v", errObj["message"])
	}
}
