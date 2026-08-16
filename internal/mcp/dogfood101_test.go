// Package mcp: regression tests for DOGFOOD-101 — the MCP surface is
// authenticated end-to-end. A session must complete a valid-key initialize
// before any tool/list/read method may execute; both SSE and stdio dispatch
// through handleMethod, which enforces the gate.
//
// axiom:trace work_item=dogfood-101 spec=specs/015-api-and-mcp.md test=internal/mcp/dogfood101_test.go
package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// DOGFOOD-101: unauthenticated sessions are rejected at dispatch
// ============================================================================

// TestUnauthenticatedSession_ToolsCall_Forbidden verifies a session that never
// completed authenticated initialize cannot execute tools (both SSE and stdio
// route through handleMethod).
func TestUnauthenticatedSession_ToolsCall_Forbidden(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{} // fresh session — no key, no initialize

	params, _ := json.Marshal(map[string]any{
		"name": "list_memory",
		"arguments": map[string]any{
			"session_id": "victim-session",
		},
	})
	req := &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: params}

	_, err := srv.handleMethod(req, sess)
	if err == nil {
		t.Fatal("unauthenticated tools/call must fail")
	}
	if err.Code != -32002 {
		t.Errorf("expected code -32002, got %d: %v", err.Code, err)
	}
	if err.Message != "Forbidden" {
		t.Errorf("expected 'Forbidden', got %q", err.Message)
	}
}

// TestUnauthenticatedSession_ToolsList_Forbidden verifies tools/list (surface
// enumeration) is also gated.
func TestUnauthenticatedSession_ToolsList_Forbidden(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	req := &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"}

	_, err := srv.handleMethod(req, sess)
	if err == nil {
		t.Fatal("unauthenticated tools/list must fail")
	}
	if err.Code != -32002 {
		t.Errorf("expected code -32002, got %d: %v", err.Code, err)
	}
	if err.Message != "Forbidden" {
		t.Errorf("expected 'Forbidden', got %q", err.Message)
	}
}

// TestUnauthenticatedSession_ReadResources_Forbidden verifies data-reading
// resources are gated too, not just tools.
func TestUnauthenticatedSession_ReadResources_Forbidden(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	params, _ := json.Marshal(map[string]any{
		"uri": "consensus://sessions/victim-session/context",
	})
	req := &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "resources/read", Params: params}

	_, err := srv.handleMethod(req, sess)
	if err == nil {
		t.Fatal("unauthenticated resources/read must fail")
	}
	if err.Code != -32002 || err.Message != "Forbidden" {
		t.Errorf("expected -32002 Forbidden, got %d %q", err.Code, err.Message)
	}
}

// TestInitialize_WithoutKey_AuthenticationRequired is acceptance criterion 2:
// initialize with no key returns Authentication required.
func TestInitialize_WithoutKey_AuthenticationRequired(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	params, _ := json.Marshal(map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
		// No _meta — intentionally missing auth
	})
	req := &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: params}

	_, err := srv.handleMethod(req, sess)
	if err == nil {
		t.Fatal("initialize without key must fail")
	}
	if err.Code != -32000 {
		t.Errorf("expected code -32000, got %d: %v", err.Code, err)
	}
	if err.Message != "Authentication required" {
		t.Errorf("expected 'Authentication required', got %q", err.Message)
	}
	if sess.authenticated {
		t.Error("failed initialize must not mark the session authenticated")
	}
}

// ============================================================================
// DOGFOOD-101: SSE transport (acceptance criterion 1)
// ============================================================================

// TestSSE_UnauthenticatedSession_ToolsCall_Forbidden reproduces the live
// finding: open an MCP session via the SSE endpoint (sessionId issued with no
// credentials), then POST tools/call list_memory with no key — must return
// Forbidden, not another session's memory.
func TestSSE_UnauthenticatedSession_ToolsCall_Forbidden(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	srv.sessions["sse-unauth"] = &mcpSession{id: "sse-unauth"} // as HandleSSE would register it

	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "list_memory",
			"arguments": map[string]any{"session_id": "victim-session"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp/message?sessionId=sse-unauth", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.HandleMessage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 with JSON-RPC error, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON-RPC response: %v (body=%q)", err, w.Body.String())
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected JSON-RPC error, got %v", resp)
	}
	if int(errObj["code"].(float64)) != -32002 {
		t.Errorf("expected code -32002, got %v", errObj["code"])
	}
	if errObj["message"] != "Forbidden" {
		t.Errorf("expected message 'Forbidden', got %v", errObj["message"])
	}
}

// TestSSE_AuthenticatedSession_ToolsCall_Succeeds verifies the happy path over
// the real HTTP transport: initialize with a valid key, then tools/call works.
func TestSSE_AuthenticatedSession_ToolsCall_Succeeds(t *testing.T) {
	mock := &sequentialMockDB{
		results: [][]db.Row{
			// Query 1: initialize — api_keys lookup
			{{"id": "key-1", "scope": "admin", "session_id": nil}},
			// Query 2: list_memory
			{{"id": int64(1), "type": "text_block", "content": "hello", "iteration_created": int64(1), "created_at": "2024-01-01"}},
		},
	}
	srv := NewServer(mock)
	srv.sessions["sse-auth"] = &mcpSession{id: "sse-auth"}

	initBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
			"_meta":           map[string]any{"authorization": "Bearer cs_ak_testkey"},
		},
	})
	w := httptest.NewRecorder()
	srv.HandleMessage(w, httptest.NewRequest(http.MethodPost, "/mcp/message?sessionId=sse-auth", bytes.NewReader(initBody)))
	if w.Code != http.StatusOK {
		t.Fatalf("initialize: expected 200, got %d", w.Code)
	}
	var initResp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("initialize: invalid response: %v", err)
	}
	if _, ok := initResp["error"]; ok {
		t.Fatalf("initialize with valid key failed: %v", initResp["error"])
	}

	callBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "list_memory",
			"arguments": map[string]any{"session_id": "sess-1", "limit": 10},
		},
	})
	w2 := httptest.NewRecorder()
	srv.HandleMessage(w2, httptest.NewRequest(http.MethodPost, "/mcp/message?sessionId=sse-auth", bytes.NewReader(callBody)))
	if w2.Code != http.StatusOK {
		t.Fatalf("tools/call: expected 200, got %d", w2.Code)
	}
	var callResp map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &callResp); err != nil {
		t.Fatalf("tools/call: invalid response: %v (body=%q)", err, w2.Body.String())
	}
	if _, ok := callResp["error"]; ok {
		t.Fatalf("authenticated tools/call failed: %v", callResp["error"])
	}
	text := callResp["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, `"count": 1`) {
		t.Errorf("expected count 1 in result, got: %s", text)
	}
}

// ============================================================================
// DOGFOOD-101: stdio transport
// ============================================================================

// TestStdio_UnauthenticatedSession_CannotCallTools verifies the stdio session
// (which lasts the process lifetime and is initialized during the handshake)
// also rejects tools before a valid-key initialize.
func TestStdio_UnauthenticatedSession_CannotCallTools(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{id: "stdio"} // as ServeStdio creates it

	params, _ := json.Marshal(map[string]any{
		"name":      "create_session",
		"arguments": map[string]any{"agent_name": "a", "goal": "g"},
	})
	req := &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: params}

	_, err := srv.handleMethod(req, sess)
	if err == nil {
		t.Fatal("unauthenticated stdio tools/call must fail")
	}
	if err.Code != -32002 || err.Message != "Forbidden" {
		t.Errorf("expected -32002 Forbidden, got %d %q", err.Code, err.Message)
	}
}

// TestStdio_AuthenticatedHandshake_ToolsWork is acceptance criterion 3: a
// valid-key session (initialize → tools/list → tools/call) still works.
func TestStdio_AuthenticatedHandshake_ToolsWork(t *testing.T) {
	mock := &sequentialMockDB{
		results: [][]db.Row{
			// Query 1: initialize — api_keys lookup
			{{"id": "key-1", "scope": "admin", "session_id": nil}},
			// Query 2: list_memory
			{{"id": int64(1), "type": "text_block", "content": "hello", "iteration_created": int64(1), "created_at": "2024-01-01"}},
		},
	}
	srv := NewServer(mock)
	sess := &mcpSession{id: "stdio"}

	// Step 1: initialize with a valid key
	initParams, _ := json.Marshal(map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
		"_meta":           map[string]any{"authorization": "Bearer cs_ak_testkey"},
	})
	if _, err := srv.handleMethod(&JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: initParams}, sess); err != nil {
		t.Fatalf("initialize with valid key failed: %v", err)
	}
	if !sess.authenticated {
		t.Error("expected session marked authenticated after valid-key initialize")
	}

	// Step 2: tools/list works
	if _, err := srv.handleMethod(&JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "tools/list"}, sess); err != nil {
		t.Fatalf("tools/list failed for authenticated session: %v", err)
	}

	// Step 3: tools/call list_memory works
	callParams, _ := json.Marshal(map[string]any{
		"name":      "list_memory",
		"arguments": map[string]any{"session_id": "sess-1", "limit": 10},
	})
	result, err := srv.handleMethod(&JSONRPCRequest{JSONRPC: "2.0", ID: 3, Method: "tools/call", Params: callParams}, sess)
	if err != nil {
		t.Fatalf("tools/call failed for authenticated session: %v", err)
	}
	text := result.(MCPCallToolResult).Content[0].Text
	if !strings.Contains(text, `"count": 1`) {
		t.Errorf("expected count 1 in result, got: %s", text)
	}
}

// TestInitialize_InvalidKey_SessionStaysUnauthenticated verifies a failed
// initialize cannot be used to satisfy the dispatch gate.
func TestInitialize_InvalidKey_SessionStaysUnauthenticated(t *testing.T) {
	srv := NewServer(&mockMCPDB{queryResults: []db.Row{}}) // empty = key not found
	sess := &mcpSession{}

	params, _ := json.Marshal(map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
		"_meta":           map[string]any{"authorization": "Bearer cs_ak_badkey"},
	})
	if _, err := srv.handleMethod(&JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: params}, sess); err == nil {
		t.Fatal("expected error for invalid key")
	}
	if sess.authenticated {
		t.Fatal("invalid-key initialize must not mark the session authenticated")
	}

	// And tools stay blocked.
	_, err := srv.handleMethod(&JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "tools/list"}, sess)
	if err == nil || err.Code != -32002 {
		t.Fatalf("expected -32002 Forbidden after failed initialize, got %v", err)
	}
}
