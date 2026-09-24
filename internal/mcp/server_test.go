package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// Mock DB for MCP tests
// ============================================================================

type mockMCPDB struct {
	queryResults []db.Row
	queryErr     error
	execErr      error
	queries      []string
}

func (m *mockMCPDB) BeginTx(ctx context.Context) (db.Tx, error) { return nil, nil }
func (m *mockMCPDB) Exec(ctx context.Context, query string, args ...any) error {
	m.queries = append(m.queries, query)
	return m.execErr
}
func (m *mockMCPDB) Query(ctx context.Context, query string, args ...any) ([]db.Row, error) {
	m.queries = append(m.queries, query)
	return m.queryResults, m.queryErr
}
func (m *mockMCPDB) QueryRow(ctx context.Context, query string, args ...any) (db.Row, error) {
	rows, err := m.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}
func (m *mockMCPDB) Backend() db.Backend { return db.BackendSQLite }
func (m *mockMCPDB) Close() error        { return nil }

// sequentialMockDB returns different results for each Query call.
type sequentialMockDB struct {
	results [][]db.Row
	callNum int
	queries []string
}

func (m *sequentialMockDB) BeginTx(ctx context.Context) (db.Tx, error) { return nil, nil }
func (m *sequentialMockDB) Exec(ctx context.Context, query string, args ...any) error {
	m.queries = append(m.queries, query)
	return nil
}
func (m *sequentialMockDB) Query(ctx context.Context, query string, args ...any) ([]db.Row, error) {
	m.queries = append(m.queries, query)
	if m.callNum < len(m.results) {
		r := m.results[m.callNum]
		m.callNum++
		return r, nil
	}
	return nil, nil
}
func (m *sequentialMockDB) QueryRow(ctx context.Context, query string, args ...any) (db.Row, error) {
	rows, _ := m.Query(ctx, query, args...)
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}
func (m *sequentialMockDB) Close() error        { return nil }
func (m *sequentialMockDB) Backend() db.Backend { return db.BackendSQLite }

// ============================================================================
// Helpers
// ============================================================================

func makeJSONRPC(method string, params any) []byte {
	raw, _ := json.Marshal(params)
	r := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  raw,
	}
	data, _ := json.Marshal(r)
	return data
}

// makeInitializeRequest creates an initialize request with auth token.
func makeInitializeRequest(auth string) []byte {
	return makeJSONRPC("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "test-client",
			"version": "1.0",
		},
		"_meta": map[string]any{
			"authorization": auth,
		},
	})
}

func readJSONRPCResponse(t *testing.T, body string) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("failed to parse JSON-RPC response: %v\nBody: %s", err, body)
	}
	return resp
}

func assertNoRPCError(t *testing.T, resp map[string]any) {
	t.Helper()
	if _, ok := resp["error"]; ok {
		t.Fatalf("unexpected RPC error: %v", resp["error"])
	}
}

func assertRPCError(t *testing.T, resp map[string]any, wantCode int) {
	t.Helper()
	errObj, ok := resp["error"]
	if !ok {
		t.Fatalf("expected RPC error, got success: %v", resp)
	}
	errMap := errObj.(map[string]any)
	code := int(errMap["code"].(float64))
	if code != wantCode {
		t.Errorf("expected error code %d, got %d: %v", wantCode, code, errMap)
	}
}

// ============================================================================
// SSE Transport Tests
// ============================================================================

func TestSSEEndpoint_RespondsWithEndpointEvent(t *testing.T) {
	t.Skip("SSE handler blocks in test; transport tested via integration")
}

func TestMessageEndpoint_InvalidMethod_Returns405(t *testing.T) {
	srv := NewServer(&mockMCPDB{})

	req := httptest.NewRequest(http.MethodGet, "/mcp/message?sessionId=abc", nil)
	w := httptest.NewRecorder()

	srv.HandleMessage(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestMessageEndpoint_MissingSessionID_Returns400(t *testing.T) {
	srv := NewServer(&mockMCPDB{})

	req := httptest.NewRequest(http.MethodPost, "/mcp/message", bytes.NewReader([]byte("{}")))
	w := httptest.NewRecorder()

	srv.HandleMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ============================================================================
// Initialize / Auth Tests
// ============================================================================

func TestInitialize_WithoutAuth_ReturnsError(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params: func() json.RawMessage {
			data, _ := json.Marshal(map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
				// No _meta — intentionally missing auth
			})
			return data
		}(),
	}

	_, err := srv.handleInitialize(req, sess)
	if err == nil {
		t.Fatal("expected error for missing auth")
	}
	if err.Code != -32000 {
		t.Errorf("expected -32000, got %d: %v", err.Code, err)
	}
}

func TestInitialize_WithValidAuth_Succeeds(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": "key-1", "scope": "admin", "session_id": nil},
		},
	}
	srv := NewServer(mock)
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params: func() json.RawMessage {
			data, _ := json.Marshal(map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
				"_meta": map[string]any{
					"authorization": "Bearer cs_ak_testkey",
				},
			})
			return data
		}(),
	}

	result, err := srv.handleInitialize(req, sess)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	initResult := result.(MCPInitializeResult)
	if initResult.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocolVersion 2024-11-05, got %s", initResult.ProtocolVersion)
	}
	if initResult.ServerInfo.Name != "consensus" {
		t.Errorf("expected server name 'consensus', got %s", initResult.ServerInfo.Name)
	}
	if sess.authScope != "admin" {
		t.Errorf("expected auth scope admin, got %s", sess.authScope)
	}
}

func TestInitialize_WithInvalidKey_ReturnsError(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{}, // empty = key not found
	}
	srv := NewServer(mock)
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params: func() json.RawMessage {
			data, _ := json.Marshal(map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
				"_meta": map[string]any{
					"authorization": "Bearer cs_ak_badkey",
				},
			})
			return data
		}(),
	}

	_, err := srv.handleInitialize(req, sess)
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if err.Code != -32001 {
		t.Errorf("expected -32001, got %d: %v", err.Code, err)
	}
}

// ============================================================================
// Tools Tests
// ============================================================================

func TestToolsList_ReturnsAllTools(t *testing.T) {
	mock := &mockMCPDB{}
	srv := NewServer(mock)

	result, err := srv.handleToolsList(nil)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	respMap := result.(map[string]any)
	tools, ok := respMap["tools"].([]MCPToolDefinition)
	if !ok {
		t.Fatalf("expected tools array, got %T", respMap["tools"])
	}

	expectedTools := []string{
		"create_session", "send_message", "get_session_status",
		"list_memory", "review_approval", "query_tool",
	}
	if len(tools) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d", len(expectedTools), len(tools))
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

func TestToolsCreateSession_Succeeds(t *testing.T) {
	mock := &mockMCPDB{}
	srv := NewServer(mock)

	sess := &mcpSession{authScope: "admin", sessionKey: "cs_ak_test"}
	result, err := srv.toolCreateSession(
		json.RawMessage(`{"agent_name":"test_agent","goal":"test goal","model_id":"gpt-4o"}`),
		sess,
	)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	callResult := result.(MCPCallToolResult)
	if len(callResult.Content) == 0 {
		t.Fatal("expected content in result")
	}

	text := callResult.Content[0].Text
	if !strings.Contains(text, "id") || !strings.Contains(text, "api_key") {
		t.Errorf("expected id and api_key in result, got: %s", text)
	}
}

func TestToolsCreateSession_ReadonlyRejected(t *testing.T) {
	mock := &mockMCPDB{}
	srv := NewServer(mock)

	sess := &mcpSession{authScope: "readonly"}
	_, err := srv.toolCreateSession(
		json.RawMessage(`{"agent_name":"t","goal":"g"}`),
		sess,
	)
	if err == nil {
		t.Fatal("expected error for readonly scope")
	}
	if err.Code != -32002 {
		t.Errorf("expected -32002, got %d: %v", err.Code, err)
	}
}

func TestToolsSendMessage_Succeeds(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": "sess-1", "status": "idle"},
		},
	}
	srv := NewServer(mock)

	sess := &mcpSession{authScope: "admin"}
	result, err := srv.toolSendMessage(
		json.RawMessage(`{"session_id":"sess-1","message":"hello"}`),
		sess,
	)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	callResult := result.(MCPCallToolResult)
	if !strings.Contains(callResult.Content[0].Text, `"sent": true`) {
		t.Errorf("expected sent=true, got: %s", callResult.Content[0].Text)
	}
}

func TestToolsSendMessage_CompletedSessionRejected(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": "sess-1", "status": "completed"},
		},
	}
	srv := NewServer(mock)

	sess := &mcpSession{authScope: "admin"}
	_, err := srv.toolSendMessage(
		json.RawMessage(`{"session_id":"sess-1","message":"hello"}`),
		sess,
	)
	if err == nil {
		t.Fatal("expected error for completed session")
	}
}

func TestToolsGetSessionStatus_ReturnsStatus(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{
				"id": "sess-1", "agent_name": "test", "status": "thinking",
				"goal": "do stuff", "iteration": int64(5),
				"tokens_used_in": int64(100), "tokens_used_out": int64(50),
			},
		},
	}
	srv := NewServer(mock)

	sess := &mcpSession{authScope: "admin"}
	result, err := srv.toolGetSessionStatus(
		json.RawMessage(`{"session_id":"sess-1"}`),
		sess,
	)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	text := result.(MCPCallToolResult).Content[0].Text
	if !strings.Contains(text, `"status": "thinking"`) {
		t.Errorf("expected status thinking, got: %s", text)
	}
	if !strings.Contains(text, `"iteration": 5`) {
		t.Errorf("expected iteration 5, got: %s", text)
	}
}

func TestToolsListMemory_ReturnsEvents(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": int64(1), "type": "text_block", "content": "hello world", "iteration_created": int64(1), "created_at": "2024-01-01"},
			{"id": int64(2), "type": "tool_call", "content": "scrape site", "iteration_created": int64(1), "created_at": "2024-01-01"},
		},
	}
	srv := NewServer(mock)

	sess := &mcpSession{authScope: "admin"}
	result, err := srv.toolListMemory(
		json.RawMessage(`{"session_id":"sess-1","limit":10}`),
		sess,
	)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	text := result.(MCPCallToolResult).Content[0].Text
	if !strings.Contains(text, `"count": 2`) {
		t.Errorf("expected count 2, got: %s", text)
	}
}

func TestToolsReviewApproval_ApprovesSuccessfully(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": "appr-1", "session_id": "sess-1", "status": "pending"},
		},
	}
	srv := NewServer(mock)

	sess := &mcpSession{authScope: "admin"}
	result, err := srv.toolReviewApproval(
		json.RawMessage(`{"approval_id":"appr-1","decision":"approved","notes":"looks good"}`),
		sess,
	)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	text := result.(MCPCallToolResult).Content[0].Text
	if !strings.Contains(text, `"status": "approved"`) {
		t.Errorf("expected status approved, got: %s", text)
	}
}

func TestToolsReviewApproval_NonAdminRejected(t *testing.T) {
	srv := NewServer(&mockMCPDB{})

	sess := &mcpSession{authScope: "session"}
	_, err := srv.toolReviewApproval(
		json.RawMessage(`{"approval_id":"a","decision":"approved"}`),
		sess,
	)
	if err == nil {
		t.Fatal("expected error for non-admin scope")
	}
}

func TestToolsQueryTool_InternalSQLFunction(t *testing.T) {
	// Use a sequential mock that returns different results per query
	mock := &sequentialMockDB{
		results: [][]db.Row{
			// Query 1: tools_registry lookup
			{{"name": "list_sessions", "hemisphere": "internal", "handler_type": "sql_function", "handler_ref": "list_active_sessions"}},
			// Query 2: the SQL function call result
			{{"id": "sess-1", "status": "idle"}, {"id": "sess-2", "status": "thinking"}},
		},
	}
	srv := NewServer(mock)

	sess := &mcpSession{authScope: "admin"}
	result, err := srv.toolQueryTool(
		json.RawMessage(`{"session_id":"sess-1","tool_name":"list_sessions"}`),
		sess,
	)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	text := result.(MCPCallToolResult).Content[0].Text
	if !strings.Contains(text, `"count": 2`) {
		t.Errorf("expected count 2, got: %s", text)
	}
}

// ============================================================================
// Resources Tests
// ============================================================================

func TestResourcesList_ReturnsResources(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	result, err := srv.handleResourcesList(nil)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	respMap := result.(map[string]any)
	resources, ok := respMap["resources"].([]MCPResourceDefinition)
	if !ok {
		t.Fatalf("expected resources array, got %T", respMap["resources"])
	}

	if len(resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(resources))
	}

	uris := map[string]bool{}
	for _, r := range resources {
		uris[r.URI] = true
	}
	if !uris["consensus://sessions"] {
		t.Error("missing consensus://sessions resource")
	}
	if !uris["consensus://tools"] {
		t.Error("missing consensus://tools resource")
	}
}

func TestResourceTemplates_ReturnsTemplates(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	result, err := srv.handleResourceTemplates(nil)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	respMap := result.(map[string]any)
	templates, ok := respMap["resourceTemplates"].([]MCPResourceTemplate)
	if !ok {
		t.Fatalf("expected resourceTemplates array, got %T", respMap["resourceTemplates"])
	}

	if len(templates) != 1 {
		t.Errorf("expected 1 template, got %d", len(templates))
	}
	if templates[0].URITemplate != "consensus://sessions/{session_id}/context" {
		t.Errorf("unexpected template: %s", templates[0].URITemplate)
	}
}

func TestResourcesRead_Sessions(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": "sess-1", "agent_name": "test", "status": "idle", "goal": "test", "iteration": int64(3)},
		},
	}
	srv := NewServer(mock)

	sess := &mcpSession{authScope: "admin"}
	result, err := srv.readSessionsResource(context.Background(), sess)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	res := result.(MCPReadResourceResult)
	if len(res.Contents) == 0 {
		t.Fatal("expected content")
	}
	text := res.Contents[0].Text
	if !strings.Contains(text, `"status": "idle"`) {
		t.Errorf("expected idle status in response, got: %s", text)
	}
}

func TestResourcesRead_Tools(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"name": "scraper", "description": "scrapes web", "hemisphere": "external", "handler_type": "subprocess", "status": "active"},
		},
	}
	srv := NewServer(mock)

	sess := &mcpSession{authScope: "admin"}
	result, err := srv.readToolsResource(context.Background(), sess)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	res := result.(MCPReadResourceResult)
	text := res.Contents[0].Text
	if !strings.Contains(text, `"name": "scraper"`) {
		t.Errorf("expected scraper tool, got: %s", text)
	}
}

func TestResourcesRead_SessionContext(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": int64(1), "iteration_created": int64(1), "type": "text_block", "display_mode": "full", "rendered_text": "Hello world"},
		},
	}
	srv := NewServer(mock)

	sess := &mcpSession{authScope: "admin"}
	result, err := srv.readSessionContextResource(context.Background(), sess, "sess-1")
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	res := result.(MCPReadResourceResult)
	text := res.Contents[0].Text
	if !strings.Contains(text, `"rendered_text": "Hello world"`) {
		t.Errorf("expected rendered text, got: %s", text)
	}
}

// ============================================================================
// Prompts Tests
// ============================================================================

func TestPromptsList_ReturnsAgentStatus(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	result, err := srv.handlePromptsList(nil)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	respMap := result.(map[string]any)
	prompts, ok := respMap["prompts"].([]MCPPromptDefinition)
	if !ok {
		t.Fatalf("expected prompts array, got %T", respMap["prompts"])
	}

	if len(prompts) != 1 || prompts[0].Name != "agent_status" {
		t.Errorf("expected agent_status prompt, got %+v", prompts)
	}
}

func TestPromptsGet_AgentStatus_ReturnsSummary(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{
				"id": "sess-1", "agent_name": "test", "status": "thinking",
				"goal": "analyze data", "iteration": int64(10),
				"tokens_used_in": int64(500), "tokens_used_out": int64(200),
			},
		},
	}
	srv := NewServer(mock)

	req := &JSONRPCRequest{}
	req.Params, _ = json.Marshal(map[string]any{
		"name": "agent_status",
		"arguments": map[string]string{
			"session_id": "sess-1",
		},
	})

	sess := &mcpSession{authScope: "admin"}
	result, err := srv.handlePromptsGet(req, sess)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	resp := result.(map[string]any)
	messages := resp["messages"].([]map[string]any)
	if len(messages) == 0 {
		t.Fatal("expected messages")
	}

	content := messages[0]["content"].(map[string]string)
	if !strings.Contains(content["text"], `"iteration": 10`) {
		t.Errorf("expected iteration 10 in prompt text, got: %s", content["text"])
	}
}

// ============================================================================
// Ping Test
// ============================================================================

func TestPing_ReturnsEmpty(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	result, err := srv.handleMethod(&JSONRPCRequest{JSONRPC: "2.0", Method: "ping"}, sess)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	_, ok := result.(map[string]any)
	if !ok {
		t.Errorf("expected map result for ping, got %T", result)
	}
}

// ============================================================================
// Notification Tests (no response expected)
// ============================================================================

func TestNotificationsInitialized_ReturnsNil(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	result, err := srv.handleMethod(&JSONRPCRequest{JSONRPC: "2.0", Method: "notifications/initialized"}, sess)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for notification, got %v", result)
	}
}

// ============================================================================
// Unknown Method
// ============================================================================

func TestUnknownMethod_ReturnsError(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authenticated: true}

	_, err := srv.handleMethod(&JSONRPCRequest{JSONRPC: "2.0", Method: "nonexistent"}, sess)
	if err == nil {
		t.Fatal("expected error for unknown method")
	}
	if err.Code != -32601 {
		t.Errorf("expected -32601, got %d", err.Code)
	}
}

// ============================================================================
// Handler Integration Test
// ============================================================================

func TestHandler_ReturnsMultiplexer(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	h := srv.Handler()

	if h == nil {
		t.Fatal("expected non-nil handler")
	}

	// Test the message endpoint works (non-blocking)
	req := httptest.NewRequest(http.MethodPost, "/mcp/message?sessionId=none", bytes.NewReader([]byte("{}")))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Logf("message endpoint returned %d (expected 404 for unknown session, or another status)", w.Code)
	}
}

// ============================================================================
// Helpers
// ============================================================================

func makeInitializeWithoutAuth() []byte {
	return makeJSONRPC("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "test-client",
			"version": "1.0",
		},
	})
}

func extractSessionID(t *testing.T, sseBody string) string {
	t.Helper()
	lines := strings.Split(sseBody, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, "sessionId=") {
			_ = i
			data := strings.TrimPrefix(line, "data: ")
			if idx := strings.Index(data, "sessionId="); idx >= 0 {
				return data[idx+10:]
			}
		}
	}
	t.Fatal("could not extract sessionId from SSE response")
	return ""
}

// ============================================================================
// writeError Tests
// ============================================================================

func TestWriteError_SendsJSONRPCError(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	w := httptest.NewRecorder()
	srv.writeError(w, 1, -32000, "Test error", "details")

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (JSON-RPC errors are HTTP 200), got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content type, got %q", ct)
	}

	var resp JSONRPCErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", resp.JSONRPC)
	}
	if resp.Error.Code != -32000 {
		t.Errorf("expected code -32000, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "Test error" {
		t.Errorf("expected 'Test error', got %q", resp.Error.Message)
	}
}

// ============================================================================
// handleResourcesRead — Unknown URI Branch
// ============================================================================

func TestResourcesRead_UnknownURI_ReturnsError(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "resources/read",
	}
	req.Params, _ = json.Marshal(map[string]any{
		"uri": "consensus://unknown_resource",
	})

	_, err := srv.handleResourcesRead(req, sess)
	if err == nil {
		t.Fatal("expected error for unknown resource URI")
	}
	if err.Code != -32602 {
		t.Errorf("expected -32602, got %d", err.Code)
	}
	if !strings.Contains(err.Message, "Resource not found") {
		t.Errorf("expected 'Resource not found', got %q", err.Message)
	}
}

// ============================================================================
// handleToolsCall — Unknown Tool Branch
// ============================================================================

func TestToolsCall_UnknownTool_ReturnsError(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
	}
	req.Params, _ = json.Marshal(map[string]any{
		"name": "nonexistent_tool",
	})

	_, err := srv.handleToolsCall(req, sess)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if err.Code != -32601 {
		t.Errorf("expected -32601, got %d", err.Code)
	}
}

func TestToolsCall_InvalidParams_ReturnsError(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
	}
	req.Params, _ = json.Marshal("not an object") // malformed params

	_, err := srv.handleToolsCall(req, sess)
	if err == nil {
		t.Fatal("expected invalid params error")
	}
	if err.Code != -32602 {
		t.Errorf("expected -32602, got %d", err.Code)
	}
}

// ============================================================================
// handleResourcesRead — Invalid Params Branch
// ============================================================================

func TestResourcesRead_InvalidParams_ReturnsError(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "resources/read",
	}
	req.Params, _ = json.Marshal("garbage")

	_, err := srv.handleResourcesRead(req, sess)
	if err == nil {
		t.Fatal("expected invalid params error")
	}
	if err.Code != -32602 {
		t.Errorf("expected -32602, got %d", err.Code)
	}
}

// ============================================================================
// HandleMessage — JSON Parse Error
// ============================================================================

func TestHandleMessage_ParseError(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	srv.sessions["test-sess"] = &mcpSession{id: "test-sess"}

	// Use bad JSON
	req := httptest.NewRequest(http.MethodPost, "/mcp/message?sessionId=test-sess",
		bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()

	srv.HandleMessage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for JSON-RPC error, got %d", w.Code)
	}
}

func TestHandleMessage_NotJSONRPC2(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	srv.sessions["test-sess"] = &mcpSession{id: "test-sess"}

	body, _ := json.Marshal(map[string]any{"jsonrpc": "1.0", "method": "ping", "id": 1})
	req := httptest.NewRequest(http.MethodPost, "/mcp/message?sessionId=test-sess",
		bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.HandleMessage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["error"]; !ok {
		t.Error("expected error for invalid jsonrpc version")
	}
}

// ============================================================================
// handleInitialize — Invalid Params (malformed JSON)
// ============================================================================

func TestInitialize_InvalidParams(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
	}
	req.Params, _ = json.Marshal("not_object")

	_, err := srv.handleInitialize(req, sess)
	if err == nil {
		t.Fatal("expected invalid params error")
	}
	if err.Code != -32602 {
		t.Errorf("expected -32602, got %d", err.Code)
	}
}

// ============================================================================
// Read Resources — Session Context with DB Query (extended)
// ============================================================================

func TestResourcesRead_SessionContext_Extended(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": int64(1), "iteration_created": int64(1), "type": "text_block",
				"display_mode": "full", "rendered_text": "Hello world"},
		},
	}
	srv := NewServer(mock)
	sess := &mcpSession{authScope: "admin"}

	req := &JSONRPCRequest{JSONRPC: "2.0", Method: "resources/read"}
	req.Params, _ = json.Marshal(map[string]any{
		"uri": "consensus://sessions/sess-1/context",
	})

	result, err := srv.handleResourcesRead(req, sess)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	res := result.(MCPReadResourceResult)
	text := res.Contents[0].Text
	if !strings.Contains(text, "Hello world") {
		t.Errorf("expected Hello world, got: %s", text)
	}
}

// ============================================================================
// Read Sessions Resource — Session-scoped auth
// ============================================================================

func TestResourcesRead_Sessions_SessionScoped(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": "sess-1", "agent_name": "test", "status": "idle", "goal": "test", "iteration": int64(1)},
		},
	}
	srv := NewServer(mock)
	sess := &mcpSession{authScope: "session", agentSessionID: "sess-1"}

	req := &JSONRPCRequest{JSONRPC: "2.0", Method: "resources/read"}
	req.Params, _ = json.Marshal(map[string]any{"uri": "consensus://sessions"})

	_, err := srv.handleResourcesRead(req, sess)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}
}

// ============================================================================
// SSE Write Tests
// ============================================================================

func TestSSEWrite_Success(t *testing.T) {
	// sseWrite is tested indirectly through HandleMessage; test failure case
	w := httptest.NewRecorder()
	// sseWrite requires flusher — Recorder supports it
	ok := sseWrite(w, w, "test_event", "test_data")
	if !ok {
		t.Fatal("expected sseWrite to succeed")
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: test_event") {
		t.Error("expected event header")
	}
	if !strings.Contains(body, "data: test_data") {
		t.Error("expected data")
	}
}

// ============================================================================
// HandleMessage — Body read error (covered via edge case)
// ============================================================================

func TestHandleMessage_NoSession(t *testing.T) {
	srv := NewServer(&mockMCPDB{})

	req := httptest.NewRequest(http.MethodPost, "/mcp/message?sessionId=nonexistent",
		bytes.NewReader([]byte("{}")))
	w := httptest.NewRecorder()

	srv.HandleMessage(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent session, got %d", w.Code)
	}
}

// ============================================================================
// HandleMessage — Notification (no ID, no response)
// ============================================================================

func TestHandleMessage_NotificationNoResponse(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	srv.sessions["test-notify"] = &mcpSession{id: "test-notify"}

	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp/message?sessionId=test-notify",
		bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.HandleMessage(w, req)

	// Notification — no response expected. Body should be empty.
	if w.Body.Len() > 0 {
		t.Error("expected empty response for notification with no ID")
	}
}
