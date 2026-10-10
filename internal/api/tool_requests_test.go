// Package api: integration tests for DF-CONSENSUS-49 tool-call visibility.
//
// axiom:trace work_item=DF-CONSENSUS-49 spec=specs/015-api-and-mcp.md test=internal/api/tool_requests_test.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type toolResultPayload struct {
	ID         int64  `json:"id"`
	RequestID  int64  `json:"request_id"`
	Output     string `json:"output"`
	IsError    bool   `json:"is_error"`
	ErrorCode  string `json:"error_code"`
	ExitCode   *int   `json:"exit_code"`
	DurationMS *int64 `json:"duration_ms"`
	TokenCount *int   `json:"token_count"`
	CreatedAt  string `json:"created_at"`
}

type toolRequestPayload struct {
	ID                int64              `json:"id"`
	SessionID         string             `json:"session_id"`
	IterationID       int64              `json:"iteration_id"`
	ToolName          string             `json:"tool_name"`
	Parameters        map[string]any     `json:"parameters"`
	Status            string             `json:"status"`
	TimeoutMS         int                `json:"timeout_ms"`
	ApprovalRequestID *string            `json:"approval_request_id"`
	CreatedAt         string             `json:"created_at"`
	ExecutedAt        *string            `json:"executed_at"`
	CompletedAt       *string            `json:"completed_at"`
	Result            *toolResultPayload `json:"result"`
}

type sessionToolCallPayload struct {
	Source      string         `json:"source"`
	ID          int64          `json:"id"`
	SessionID   string         `json:"session_id"`
	Iteration   int64          `json:"iteration"`
	Turn        *int           `json:"turn"`
	Seq         *int           `json:"seq"`
	ToolName    string         `json:"tool_name"`
	Parameters  map[string]any `json:"parameters"`
	Status      string         `json:"status"`
	Description string         `json:"description"`
	Executed    bool           `json:"executed"`
	Result      any            `json:"result"`
	CreatedAt   string         `json:"created_at"`
	ExecutedAt  *string        `json:"executed_at"`
	CompletedAt *string        `json:"completed_at"`
}

func seedToolRequestSession(t *testing.T, srv *integrationServer, id string) {
	t.Helper()
	if err := srv.conn.Exec(context.Background(), `
		INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at)
		VALUES ($1, 'tool-visibility', 'gpt-4o', 'idle', 'Observe tools', '2026-10-10T10:00:00Z', '2026-10-10T10:00:00Z')`, id); err != nil {
		t.Fatalf("seed session %s: %v", id, err)
	}
}

func serveAPIRequest(t *testing.T, srv *integrationServer, method, target, keyHeader, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if key != "" {
		req.Header.Set(keyHeader, key)
	}
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	return w
}

func TestListToolRequests_PendingVisibleWithFilters(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()
	seedToolRequestSession(t, srv, "sess-tool-list")
	seedToolRequestSession(t, srv, "sess-tool-other")

	ctx := context.Background()
	if err := srv.conn.Exec(ctx, `
		INSERT INTO tool_requests (id, session_id, iteration_id, tool_name, parameters, status, timeout_ms, created_at)
		VALUES (101, 'sess-tool-list', 4, 'fetch_url', '{"url":"https://example.com"}', 'pending', 45000, '2026-10-10T10:00:01Z')`); err != nil {
		t.Fatalf("seed pending request: %v", err)
	}
	if err := srv.conn.Exec(ctx, `
		INSERT INTO tool_requests (id, session_id, iteration_id, tool_name, parameters, status, created_at)
		VALUES (102, 'sess-tool-list', 5, 'done_tool', '{}', 'completed', '2026-10-10T10:00:02Z')`); err != nil {
		t.Fatalf("seed completed request: %v", err)
	}
	if err := srv.conn.Exec(ctx, `
		INSERT INTO tool_requests (id, session_id, iteration_id, tool_name, parameters, status, created_at)
		VALUES (103, 'sess-tool-other', 1, 'other_tool', '{}', 'pending', '2026-10-10T10:00:03Z')`); err != nil {
		t.Fatalf("seed other request: %v", err)
	}

	w := serveAPIRequest(t, srv, http.MethodGet,
		"/api/v1/tool-requests?session_id=sess-tool-list&status=pending",
		"Authorization", "Bearer "+srv.adminKey)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got []toolRequestPayload
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one filtered request, got %d: %s", len(got), w.Body.String())
	}
	if got[0].ID != 101 || got[0].Status != "pending" || got[0].ToolName != "fetch_url" {
		t.Fatalf("unexpected pending request: %+v", got[0])
	}
	if got[0].Parameters["url"] != "https://example.com" {
		t.Fatalf("parameters not parsed: %#v", got[0].Parameters)
	}
	if got[0].TimeoutMS != 45000 {
		t.Fatalf("timeout_ms = %d, want 45000", got[0].TimeoutMS)
	}
}

func TestGetToolRequest_CompletedIncludesResult(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()
	seedToolRequestSession(t, srv, "sess-tool-result")

	ctx := context.Background()
	if err := srv.conn.Exec(ctx, `
		INSERT INTO tool_requests (id, session_id, iteration_id, tool_name, parameters, status, created_at, executed_at, completed_at)
		VALUES (201, 'sess-tool-result', 7, 'shell', '{"command":"printf ok"}', 'completed',
		        '2026-10-10T10:01:00Z', '2026-10-10T10:01:01Z', '2026-10-10T10:01:02Z')`); err != nil {
		t.Fatalf("seed completed request: %v", err)
	}
	if err := srv.conn.Exec(ctx, `
		INSERT INTO tool_results (id, request_id, session_id, output, is_error, exit_code, duration_ms, token_count, created_at)
		VALUES (301, 201, 'sess-tool-result', 'ok', 0, 0, 17, 3, '2026-10-10T10:01:02Z')`); err != nil {
		t.Fatalf("seed result: %v", err)
	}

	w := serveAPIRequest(t, srv, http.MethodGet, "/api/v1/tool-requests/201",
		"Authorization", "Bearer "+srv.adminKey)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got toolRequestPayload
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Status != "completed" || got.Result == nil {
		t.Fatalf("completed request missing result: %+v", got)
	}
	if got.Result.Output != "ok" || got.Result.IsError || got.Result.RequestID != 201 {
		t.Fatalf("unexpected result: %+v", got.Result)
	}
	if got.Result.ExitCode == nil || *got.Result.ExitCode != 0 {
		t.Fatalf("exit_code = %v, want 0", got.Result.ExitCode)
	}
	if got.Result.DurationMS == nil || *got.Result.DurationMS != 17 {
		t.Fatalf("duration_ms = %v, want 17", got.Result.DurationMS)
	}
}

func TestSessionToolCalls_MergesRequestsAndStagingRefs(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()
	seedToolRequestSession(t, srv, "sess-tool-merge")

	ctx := context.Background()
	if err := srv.conn.Exec(ctx, `
		INSERT INTO tool_requests (id, session_id, iteration_id, tool_name, parameters, status, created_at)
		VALUES (401, 'sess-tool-merge', 9, 'fetch_url', '{"url":"https://example.com"}', 'pending', '2026-10-10T10:02:01Z')`); err != nil {
		t.Fatalf("seed tool request: %v", err)
	}
	stagingPayload, err := json.Marshal(`{"tool_name":"search_web","parameters":{"query":"consensus"}}`)
	if err != nil {
		t.Fatalf("encode durable staging payload: %v", err)
	}
	if err := srv.conn.Exec(ctx, `
		INSERT INTO staging_buffer (id, session_id, iteration, turn, seq, cmd_type, payload, description, executed, result, status, created_at, executed_at)
		VALUES (501, 'sess-tool-merge', 9, 2, 1, 'tool_call_ref',
		        $1, 'tool_call: search_web', 0, '{"request_enqueued":true}', 'executed',
		        '2026-10-10T10:02:02Z', '2026-10-10T10:02:03Z')`, string(stagingPayload)); err != nil {
		t.Fatalf("seed staging ref: %v", err)
	}
	if err := srv.conn.Exec(ctx, `
		INSERT INTO staging_buffer (id, session_id, iteration, turn, seq, cmd_type, payload, description, status, created_at)
		VALUES (502, 'sess-tool-merge', 9, 2, 2, 'sql', '"SELECT 1"', 'not a tool', 'staged', '2026-10-10T10:02:04Z')`); err != nil {
		t.Fatalf("seed non-tool staging row: %v", err)
	}

	w := serveAPIRequest(t, srv, http.MethodGet, "/api/v1/sessions/sess-tool-merge/tool-calls",
		"Authorization", "Bearer "+srv.adminKey)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got []sessionToolCallPayload
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected request + tool_call_ref only, got %d: %s", len(got), w.Body.String())
	}
	if got[0].Source != "tool_request" || got[0].Status != "pending" || got[0].ToolName != "fetch_url" {
		t.Fatalf("unexpected request item: %+v", got[0])
	}
	if got[1].Source != "staging_buffer" || got[1].Status != "executed" || got[1].ToolName != "search_web" {
		t.Fatalf("unexpected staging item: %+v", got[1])
	}
	if got[1].Parameters["query"] != "consensus" || !got[1].Executed {
		t.Fatalf("staging payload not exposed: %+v", got[1])
	}
}

func TestToolRequestEndpoints_RequireAuthentication(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	for _, path := range []string{
		"/api/v1/tool-requests",
		"/api/v1/tool-requests/999",
		"/api/v1/sessions/sess-auth/tool-calls",
	} {
		t.Run(path, func(t *testing.T) {
			w := serveAPIRequest(t, srv, http.MethodGet, path, "", "")
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestGetToolRequest_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	w := serveAPIRequest(t, srv, http.MethodGet, "/api/v1/tool-requests/999999",
		"Authorization", "Bearer "+srv.adminKey)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListToolRequests_ReadonlyKeyAllowedViaXAPIKey(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()
	seedToolRequestSession(t, srv, "sess-tool-readonly")

	ctx := context.Background()
	readonlyKey := "cs_rk_tool_visibility_readonly"
	if err := srv.conn.Exec(ctx, `
		INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at)
		VALUES ('key-tool-readonly', $1, $2, 'readonly', datetime('now'))`,
		sha256Hash(readonlyKey), readonlyKey[:8]); err != nil {
		t.Fatalf("seed readonly key: %v", err)
	}
	if err := srv.conn.Exec(ctx, `
		INSERT INTO tool_requests (id, session_id, iteration_id, tool_name, parameters, status, created_at)
		VALUES (601, 'sess-tool-readonly', 1, 'observe', '{}', 'pending', '2026-10-10T10:03:00Z')`); err != nil {
		t.Fatalf("seed tool request: %v", err)
	}

	// Exercise the same network and X-API-Key path as the documented curl probe,
	// while keeping the server, database, and credential entirely test-local.
	httpServer := httptest.NewServer(srv.router)
	defer httpServer.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/api/v1/tool-requests", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("X-API-Key", readonlyKey)
	resp, err := httpServer.Client().Do(req)
	if err != nil {
		t.Fatalf("GET tool requests: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected readonly 200, got %d", resp.StatusCode)
	}
	var got []toolRequestPayload
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != 601 || got[0].Status != "pending" {
		t.Fatalf("unexpected HTTP response: %+v", got)
	}
}

func TestSessionToolCalls_EnforcesSessionAccess(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()
	seedToolRequestSession(t, srv, "sess-tool-owner")
	seedToolRequestSession(t, srv, "sess-tool-foreign")

	ctx := context.Background()
	sessionKey := "cs_sk_tool_visibility_owner"
	if err := srv.conn.Exec(ctx, `
		INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at)
		VALUES ('key-tool-owner', $1, $2, 'session', 'sess-tool-owner', datetime('now'))`,
		sha256Hash(sessionKey), sessionKey[:8]); err != nil {
		t.Fatalf("seed session key: %v", err)
	}

	w := serveAPIRequest(t, srv, http.MethodGet, "/api/v1/sessions/sess-tool-foreign/tool-calls",
		"Authorization", "Bearer "+sessionKey)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}
