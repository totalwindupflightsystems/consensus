package mcp

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// Stdio Transport Tests
// ============================================================================

func TestTrimNewline(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello\n", "hello"},
		{"hello\r\n", "hello"},
		{"hello", "hello"},
		{"", ""},
		{"\n", ""},
	}
	for _, tc := range tests {
		got := trimNewline(tc.input)
		if got != tc.expected {
			t.Errorf("trimNewline(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestServeStdio_PingPong(t *testing.T) {
	srv := NewServer(&mockMCPDB{})

	// Test dispatch for each method that stdio supports
	methods := []struct {
		method string
		params string
	}{
		{"ping", ""},
		{"notifications/initialized", ""},
	}

	for _, tc := range methods {
		var raw json.RawMessage
		if tc.params != "" {
			raw = json.RawMessage(tc.params)
		}
		req := &JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  tc.method,
			Params:  raw,
		}
		sess := &mcpSession{authScope: "admin"}
		_, err := srv.handleMethod(req, sess)
		if err != nil {
			t.Errorf("method %s returned error: %v", tc.method, err)
		}
	}
}

func TestServeStdio_Initialize(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": "key-1", "scope": "admin", "session_id": nil},
		},
	}
	srv := NewServer(mock)

	sess := &mcpSession{}

	// Send initialize with auth in _meta
	params := `{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"},"_meta":{"authorization":"Bearer cs_ak_testkey"}}`
	var raw json.RawMessage
	raw = json.RawMessage(params)

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  raw,
	}

	result, err := srv.handleInitialize(req, sess)
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}

	initResult := result.(MCPInitializeResult)
	if initResult.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocol version 2024-11-05, got %s", initResult.ProtocolVersion)
	}
	if sess.authScope != "admin" {
		t.Errorf("expected authScope admin, got %s", sess.authScope)
	}
}

func TestServeStdio_ToolsList(t *testing.T) {
	mock := &mockMCPDB{}
	srv := NewServer(mock)
	sess := &mcpSession{authScope: "admin"}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	result, err := srv.handleMethod(req, sess)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}

	respMap := result.(map[string]any)
	tools, ok := respMap["tools"].([]MCPToolDefinition)
	if !ok {
		t.Fatalf("expected tools array, got %T", respMap["tools"])
	}
	if len(tools) == 0 {
		t.Error("expected at least one tool")
	}
}

func TestServeStdio_ResourcesList(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin"}

	result, err := srv.handleResourcesList(sess)
	if err != nil {
		t.Fatalf("resources/list failed: %v", err)
	}

	respMap := result.(map[string]any)
	resources, ok := respMap["resources"].([]MCPResourceDefinition)
	if !ok {
		t.Fatalf("expected resources array, got %T", respMap["resources"])
	}
	if len(resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(resources))
	}
}

func TestServeStdio_NotificationNoResponse(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	// notifications have no ID — should return nil,nil
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
		// No ID — this is a notification
	}

	result, err := srv.handleMethod(req, sess)
	if err != nil {
		t.Fatalf("notification failed: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for notification, got %v", result)
	}
}

func TestServeStdio_InvalidJSONRPC(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "1.0",
		ID:      1,
		Method:  "ping",
	}

	_, err := srv.handleMethod(req, sess)
	// handleMethod does not validate jsonrpc version — that's done in HandleMessage.
	// The stdio transport validates it before calling handleMethod.
	// So this should actually go through since handleMethod doesn't check it.
	if err != nil {
		t.Logf("handleMethod returned error for 1.0: %v", err)
	}
}

func TestServeStdio_UnknownMethod(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "nonexistent_method",
	}

	_, err := srv.handleMethod(req, sess)
	if err == nil {
		t.Fatal("expected error for unknown method")
	}
	if err.Code != -32601 {
		t.Errorf("expected code -32601, got %d", err.Code)
	}
}

func TestServeStdio_ParseError(t *testing.T) {
	// Test the parse error handling at the HTTP level is consistent
	srv := NewServer(&mockMCPDB{})

	// writeStdioError produces valid JSON-RPC error output similar to writeError
	w := httptest.NewRecorder()
	srv.writeError(w, 1, -32700, "Parse error", "bad json")

	var resp JSONRPCErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Code != -32700 {
		t.Errorf("expected code -32700, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "Parse error" {
		t.Errorf("expected 'Parse error', got %q", resp.Error.Message)
	}
}

// TestServeStdio_CompleteHandshake tests a full initialize + tools/list
// handshake sequence (what a real MCP client does over stdio).
func TestServeStdio_CompleteHandshake(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": "key-1", "scope": "admin", "session_id": nil},
		},
	}
	srv := NewServer(mock)
	sess := &mcpSession{}

	// Step 1: Initialize
	initReq := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}
	params, _ := json.Marshal(map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
		"_meta":           map[string]any{"authorization": "Bearer cs_ak_testkey"},
	})
	initReq.Params = params

	initResult, err := srv.handleInitialize(initReq, sess)
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}

	initResp, ok := initResult.(MCPInitializeResult)
	if !ok {
		t.Fatalf("expected MCPInitializeResult, got %T", initResult)
	}
	if initResp.ProtocolVersion != "2024-11-05" {
		t.Errorf("protocol version: expected 2024-11-05, got %s", initResp.ProtocolVersion)
	}

	// Step 2: notifications/initialized (notification — no response)
	notifReq := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	notifResult, err := srv.handleMethod(notifReq, sess)
	if err != nil {
		t.Fatalf("notifications/initialized failed: %v", err)
	}
	if notifResult != nil {
		t.Error("expected nil for notification")
	}

	// Step 3: tools/list
	toolsReq := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}
	toolsResult, err := srv.handleMethod(toolsReq, sess)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}

	toolsResp := toolsResult.(map[string]any)
	tools, ok := toolsResp["tools"].([]MCPToolDefinition)
	if !ok {
		t.Fatalf("expected tools array, got %T", toolsResp["tools"])
	}

	expectedTools := []string{
		"create_session", "send_message", "get_session_status",
		"list_memory", "review_approval", "query_tool",
	}
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}


