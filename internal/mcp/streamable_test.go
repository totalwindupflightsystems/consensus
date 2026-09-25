// Package mcp: unit tests for the streamable-HTTP transport and the task
// tools added by MCP-DIRECT-001. Transport-level mount coverage (what a
// client actually hits on a full deployment) lives in
// streamable_mount_test.go; these tests pin the in-package behavior.
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
// Streamable-HTTP transport (HandleStreamable)
// ============================================================================

// streamablePOST posts one JSON-RPC request to the streamable endpoint of the
// given server with an optional Mcp-Session-Id header.
func streamablePOST(srv *Server, sessionID string, payload any) *httptest.ResponseRecorder {
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set(streamableSessionHeader, sessionID)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func streamableInitializePayload(auth string) map[string]any {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "streamable-test", "version": "1.0"},
	}
	if auth != "" {
		params["_meta"] = map[string]any{"authorization": auth}
	}
	return map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": params}
}

// TestStreamable_InitializeThenToolsList round-trips the full streamable
// handshake: header-less initialize (session bootstrap), Mcp-Session-Id
// echoed back, then authenticated tools/list with the header.
func TestStreamable_InitializeThenToolsList(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{{"id": "key-1", "scope": "admin", "session_id": nil}},
	}
	srv := NewServer(mock)

	w := streamablePOST(srv, "", streamableInitializePayload("Bearer cs_ak_streamtest"))
	if w.Code != http.StatusOK {
		t.Fatalf("initialize: expected 200, got %d (body=%q)", w.Code, w.Body.String())
	}
	var initResp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("initialize: invalid JSON: %q", w.Body.String())
	}
	assertNoRPCError(t, initResp)
	sessID := w.Header().Get(streamableSessionHeader)
	if sessID == "" {
		t.Fatal("initialize response carries no Mcp-Session-Id header")
	}

	w2 := streamablePOST(srv, sessID, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	if w2.Code != http.StatusOK {
		t.Fatalf("tools/list: expected 200, got %d (body=%q)", w2.Code, w2.Body.String())
	}
	var listResp map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("tools/list: invalid JSON: %q", w2.Body.String())
	}
	assertNoRPCError(t, listResp)
	result := listResp["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) < 8 {
		t.Errorf("expected >= 8 tools, got %d", len(tools))
	}
}

// TestStreamable_UnauthenticatedToolsList401 pins the 401 negative: tools/list
// with an unknown Mcp-Session-Id answers HTTP 401 with a JSON-RPC error body.
func TestStreamable_UnauthenticatedToolsList401(t *testing.T) {
	srv := NewServer(&mockMCPDB{})

	w := streamablePOST(srv, "forged-session-id", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected HTTP 401 for unknown session, got %d (body=%q)", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("401 body is not JSON: %q", w.Body.String())
	}
	errObj := resp["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32002 {
		t.Errorf("expected error code -32002, got %v", errObj["code"])
	}
}

// TestStreamable_UnauthenticatedNoSessionCannotList pins the other 401 shape:
// a header-less request is bootstrapped as a fresh session, and tools/list on
// it is rejected (never leaks data pre-auth, DOGFOOD-101 semantics).
func TestStreamable_UnauthenticatedNoSessionCannotList(t *testing.T) {
	srv := NewServer(&mockMCPDB{})

	w := streamablePOST(srv, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected HTTP 401 for header-less tools/list, got %d (body=%q)", w.Code, w.Body.String())
	}
	// The bootstrap session must have been torn down again.
	srv.mu.RLock()
	remaining := len(srv.sessions)
	srv.mu.RUnlock()
	if remaining != 0 {
		t.Errorf("expected no leftover unauthenticated sessions, got %d", remaining)
	}
}

// TestStreamable_NotificationAccepted pins the notification half: a request
// without an id gets HTTP 202 and no response body.
func TestStreamable_NotificationAccepted(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	srv.sessions["s1"] = &mcpSession{id: "s1", authenticated: true}

	w := streamablePOST(srv, "s1", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for notification, got %d", w.Code)
	}
	if strings.TrimSpace(w.Body.String()) != "" {
		t.Errorf("notification response should carry no body, got %q", w.Body.String())
	}
}

// TestStreamable_ParseError400 pins the parse-error mapping (HTTP 400 with a
// JSON-RPC -32700 body, unlike the legacy path which answers 200).
func TestStreamable_ParseError400(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	srv.sessions["s1"] = &mcpSession{id: "s1", authenticated: true}

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(streamableSessionHeader, "s1")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("400 body is not JSON: %q", w.Body.String())
	}
	errObj := resp["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32700 {
		t.Errorf("expected -32700, got %v", errObj["code"])
	}
}

// TestStreamable_InvalidMethod405 pins the method gate.
func TestStreamable_InvalidMethod405(t *testing.T) {
	srv := NewServer(&mockMCPDB{})

	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for DELETE /mcp, got %d", w.Code)
	}
}

// TestStreamable_GETMCPReturnsSSEStream pins the GET half of the bare mount
// on the mcp package's own handler (full-deployment mount covered in
// streamable_mount_test.go): text/event-stream with the endpoint event.
func TestStreamable_GETMCPReturnsSSEStream(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/mcp", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /mcp: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /mcp: expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("GET /mcp: expected text/event-stream, got %q", ct)
	}
	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "event:") {
		t.Errorf("GET /mcp stream: expected an SSE event, got %q", strings.TrimSpace(string(buf[:n])))
	}
}

// TestStreamable_LegacyMessageUnchanged guards the legacy path: POST
// /mcp/message still answers its own handler semantics (404 unknown session),
// never the streamable endpoint's behavior.
func TestStreamable_LegacyMessageUnchanged(t *testing.T) {
	srv := NewServer(&mockMCPDB{})

	req := httptest.NewRequest(http.MethodPost, "/mcp/message?sessionId=nope", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("legacy /mcp/message unknown session: expected 404, got %d", w.Code)
	}
}

// ============================================================================
// Task tools (list_tasks / claim_task)
// ============================================================================

func TestToolListTasks_ReturnsTasks(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": "t1", "session_id": "s1", "title": "write report", "status": "pending", "priority": int64(5), "created_at": "2026-09-25T00:00:00Z"},
			{"id": "t2", "session_id": "s1", "title": "review code", "status": "claimed", "priority": int64(3), "created_at": "2026-09-25T00:00:00Z"},
		},
	}
	srv := NewServer(mock)
	sess := &mcpSession{authScope: "admin", authenticated: true}

	result, err := srv.toolListTasks(json.RawMessage(`{"limit":10,"offset":0}`), sess)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}
	text := result.(MCPCallToolResult).Content[0].Text
	if !strings.Contains(text, `"count": 2`) {
		t.Errorf("expected count 2, got: %s", text)
	}
	if !strings.Contains(text, "write report") {
		t.Errorf("expected task title in result, got: %s", text)
	}
	// Pagination echo
	if !strings.Contains(text, `"limit": 10`) || !strings.Contains(text, `"offset": 0`) {
		t.Errorf("expected pagination echo, got: %s", text)
	}
}

func TestToolListTasks_StatusFilterBuildsFilteredQuery(t *testing.T) {
	mock := &mockMCPDB{}
	srv := NewServer(mock)
	sess := &mcpSession{authScope: "admin", authenticated: true}

	if _, err := srv.toolListTasks(json.RawMessage(`{"status":"pending"}`), sess); err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}
	found := false
	for _, q := range mock.queries {
		if strings.Contains(q, "FROM tasks") && strings.Contains(q, "status = $1") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a status-filtered tasks query, queries: %v", mock.queries)
	}
}

func TestToolListTasks_SessionScopeDefaultsToOwnSession(t *testing.T) {
	mock := &mockMCPDB{}
	srv := NewServer(mock)
	sess := &mcpSession{authScope: "session", agentSessionID: "own-sess", authenticated: true}

	if _, err := srv.toolListTasks(json.RawMessage(`{}`), sess); err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}
	found := false
	for _, q := range mock.queries {
		if strings.Contains(q, "FROM tasks") && strings.Contains(q, "session_id = $1") {
			found = true
		}
	}
	if !found {
		t.Errorf("session-scoped key without filter must query its own session's tasks, queries: %v", mock.queries)
	}
}

func TestToolListTasks_SessionScopeOtherSessionRejected(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "session", agentSessionID: "own-sess", authenticated: true}

	_, err := srv.toolListTasks(json.RawMessage(`{"session_id":"other-sess"}`), sess)
	if err == nil {
		t.Fatal("session-scoped key must not list another session's tasks")
	}
	if err.Code != -32002 {
		t.Errorf("expected -32002, got %d", err.Code)
	}
}

func TestToolClaimTask_Success(t *testing.T) {
	mock := &sequentialMockDB{
		results: [][]db.Row{
			// Query 1: task lookup
			{{"id": "t1", "session_id": "s1", "status": "pending", "locked_by_agent": nil}},
			// Query 2: post-claim re-read
			{{"id": "t1", "session_id": "s1", "title": "write report", "status": "claimed", "priority": int64(5),
				"locked_by_agent": "mcp-owner", "claimed_at": "2026-09-25T00:00:00Z", "created_at": "2026-09-25T00:00:00Z"}},
		},
	}
	srv := NewServer(mock)
	sess := &mcpSession{authScope: "admin", authenticated: true, id: "mcp-owner"}

	result, err := srv.toolClaimTask(json.RawMessage(`{"task_id":"t1"}`), sess)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}
	text := result.(MCPCallToolResult).Content[0].Text
	if !strings.Contains(text, `"status": "claimed"`) {
		t.Errorf("expected claimed status, got: %s", text)
	}
	// The claim update must set status, lock and timestamp.
	updateOK := false
	for _, q := range mock.queries {
		if strings.Contains(q, "UPDATE tasks") && strings.Contains(q, "locked_by_agent") {
			updateOK = true
		}
	}
	if !updateOK {
		t.Errorf("expected a claim UPDATE, queries: %v", mock.queries)
	}
}

func TestToolClaimTask_NotPendingRejected(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": "t1", "session_id": "s1", "status": "in_progress", "locked_by_agent": "someone"},
		},
	}
	srv := NewServer(mock)
	sess := &mcpSession{authScope: "admin", authenticated: true, id: "mcp-owner"}

	_, err := srv.toolClaimTask(json.RawMessage(`{"task_id":"t1"}`), sess)
	if err == nil {
		t.Fatal("expected error claiming a non-pending task")
	}
}

func TestToolClaimTask_LockedByOtherRejected(t *testing.T) {
	mock := &mockMCPDB{
		queryResults: []db.Row{
			{"id": "t1", "session_id": "s1", "status": "claimed", "locked_by_agent": "other-agent"},
		},
	}
	srv := NewServer(mock)
	sess := &mcpSession{authScope: "admin", authenticated: true, id: "mcp-owner"}

	_, err := srv.toolClaimTask(json.RawMessage(`{"task_id":"t1"}`), sess)
	if err == nil {
		t.Fatal("expected error claiming a task locked by another agent")
	}
}

func TestToolClaimTask_MissingTaskID(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin", authenticated: true}

	_, err := srv.toolClaimTask(json.RawMessage(`{}`), sess)
	if err == nil {
		t.Fatal("expected error for missing task_id")
	}
}

func TestToolClaimTask_MalformedJSON(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin", authenticated: true}

	_, err := srv.toolClaimTask(json.RawMessage(`bad`), sess)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestToolClaimTask_ReadonlyRejected(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "readonly", authenticated: true}

	_, err := srv.toolClaimTask(json.RawMessage(`{"task_id":"t1"}`), sess)
	if err == nil {
		t.Fatal("readonly scope must not claim tasks")
	}
	if err.Code != -32002 {
		t.Errorf("expected -32002, got %d", err.Code)
	}
}

func TestToolClaimTask_NotFound(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin", authenticated: true, id: "mcp-owner"}

	_, err := srv.toolClaimTask(json.RawMessage(`{"task_id":"ghost"}`), sess)
	if err == nil {
		t.Fatal("expected error for unknown task id")
	}
}

// TestToolsList_IncludesTaskTools pins acceptance criterion 4 at the unit
// level: the new tools appear with typed schemas and real descriptions.
func TestToolsList_IncludesTaskTools(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	result, err := srv.handleToolsList(nil)
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}
	tools := result.(map[string]any)["tools"].([]MCPToolDefinition)
	if len(tools) < 8 {
		t.Fatalf("expected >= 8 tools, got %d", len(tools))
	}
	byName := map[string]MCPToolDefinition{}
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	for _, name := range []string{"list_tasks", "claim_task"} {
		tool, ok := byName[name]
		if !ok {
			t.Errorf("missing tool %q", name)
			continue
		}
		if tool.Description == "" || len(tool.Description) < 15 {
			t.Errorf("%q: description too thin", name)
		}
		if tool.InputSchema.Type != "object" || len(tool.InputSchema.Properties) == 0 {
			t.Errorf("%q: inputSchema not typed: %+v", name, tool.InputSchema)
		}
	}
	if len(byName["claim_task"].InputSchema.Required) == 0 {
		t.Error("claim_task schema must mark required properties")
	}
}
