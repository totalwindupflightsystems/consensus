// Package harness: DF-CONSENSUS-62 — the approval flow is reachable from a
// fresh install through user-facing surfaces only.
//
// The dogfood finding was that the HITL approval gate (SPEC-014 §2.2,
// handleApprovalGated) fires on tools_registry rows with requires_approval=true
// — but no REST route, CLI verb, or migration ever created such a row, so the
// gate could only be exercised by writing SQL by hand.
//
// This test walks the whole user path, with no direct tools_registry INSERT:
//
//	POST /api/v1/tools                    → register an approval-gated tool
//	POST /api/v1/sessions                 → a session to execute it for
//	POST /api/v1/tools/{name}/execute     → enqueue a tool_request
//	executor.PollOnce()                   → request parks at 'awaiting_approval'
//	POST /api/v1/approvals/{id}/review    → human approves
//	executor.PollOnce()                   → request executes, result recorded
//
// The HITL callbacks are wired exactly as cmd/consensus/main.go wires them, so
// the approve → re-queue → execute hop is the production one.
//
// test=internal/harness/tool_register_approval_flow_test.go
package harness

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/api"
	"github.com/wojons/consensus/internal/hitl"
)

// doAPIJSON performs a JSON request against the test server with the admin key
// and returns the status plus the raw body.
func doAPIJSON(t *testing.T, tsURL, method, path, key, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, tsURL+path, reader)
	if err != nil {
		t.Fatalf("%s %s: build request: %v", method, path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestToolRegistration_ApprovalFlowEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the flow test executes /bin/echo through the subprocess sandbox")
	}

	th, err := newTestHarness(nil)
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	// Wire the approve/reject follow-through exactly as cmd/consensus/main.go
	// does (hitl cannot import harness, so the callbacks are injected).
	th.hitl.SetApprovalCallback(func(ctx context.Context, approvalID, sessionID string) error {
		_, err := ResumeApprovedToolRequestsByApproval(ctx, th.conn, approvalID)
		return err
	})
	th.hitl.SetRejectionCallback(func(ctx context.Context, approvalID, sessionID, reviewerID, reviewNotes string) error {
		return RejectToolRequestsForApproval(ctx, th.conn, approvalID, sessionID, reviewNotes)
	})
	if err := th.hitl.SetConfiguration(th.ctx, hitl.DefaultConfiguration()); err != nil {
		t.Fatalf("set HITL config: %v", err)
	}

	adminKey := "cs_sk_admin_tool_flow_test_key_0001"
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at)
		 VALUES ('key-tool-flow', $1, 'cs_sk_ad', 'admin', datetime('now'))`,
		sha256Hash(adminKey)); err != nil {
		t.Fatalf("seed admin key: %v", err)
	}

	apiServer := api.NewServer(api.ServerConfig{DB: th.conn, HITL: th.hitl})
	ts := httptest.NewServer(apiServer.Handler())
	defer ts.Close()

	// ---- 1. Register the gated tool through the REST surface. ----
	registerBody := `{"name":"gated_echo","description":"Echo an approved marker",` +
		`"hemisphere":"external","handler_type":"subprocess",` +
		`"handler_ref":"/bin/echo df62-approved","requires_approval":true}`
	status, body := doAPIJSON(t, ts.URL, http.MethodPost, "/api/v1/tools", adminKey, registerBody)
	if status != http.StatusCreated {
		t.Fatalf("register tool: %d %s", status, body)
	}
	var created api.ToolResponse
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode created tool: %v", err)
	}
	if !created.RequiresApproval {
		t.Fatal("registered tool lost requires_approval — the gate would never fire")
	}
	if created.Status != "active" || !created.Enabled {
		t.Fatalf("registered tool is not dispatchable: status=%q enabled=%v", created.Status, created.Enabled)
	}

	// ---- 2. Create a session to execute it for. ----
	status, body = doAPIJSON(t, ts.URL, http.MethodPost, "/api/v1/sessions", adminKey,
		`{"agent_name":"df62-flow","goal":"exercise the approval gate","model_id":"test-model"}`)
	if status != http.StatusCreated {
		t.Fatalf("create session: %d %s", status, body)
	}
	var session api.CreateSessionResponse
	if err := json.Unmarshal([]byte(body), &session); err != nil {
		t.Fatalf("decode session: %v", err)
	}

	// ---- 3. Ask for execution: this enqueues a pending tool_request. ----
	status, body = doAPIJSON(t, ts.URL, http.MethodPost,
		"/api/v1/tools/gated_echo/execute", adminKey,
		`{"session_id":"`+session.ID+`","parameters":{}}`)
	if status != http.StatusOK {
		t.Fatalf("execute tool: %d %s", status, body)
	}

	reqRows, err := th.conn.Query(th.ctx,
		`SELECT id, status FROM tool_requests WHERE tool_name = 'gated_echo'`)
	if err != nil {
		t.Fatalf("query tool_requests: %v", err)
	}
	if len(reqRows) != 1 {
		t.Fatalf("tool_requests rows = %d, want 1", len(reqRows))
	}
	reqID := toInt(reqRows[0]["id"])
	if got := toString(reqRows[0]["status"]); got != "pending" {
		t.Fatalf("enqueued request status = %q, want pending", got)
	}

	// ---- 4. Executor poll #1: the gate must PARK, not execute. ----
	executor := NewToolExecutor(th.conn, DefaultToolExecutorConfig())
	if n, err := executor.PollOnce(th.ctx); err != nil {
		t.Fatalf("poll #1: %v", err)
	} else if n != 0 {
		t.Fatalf("poll #1 processed %d requests, want 0 (gated tool must not run unapproved)", n)
	}

	parkRows, err := th.conn.Query(th.ctx,
		`SELECT status, approval_request_id FROM tool_requests WHERE id = $1`, reqID)
	if err != nil {
		t.Fatalf("query parked request: %v", err)
	}
	if got := toString(parkRows[0]["status"]); got != "awaiting_approval" {
		t.Fatalf("tool_request status = %q, want awaiting_approval", got)
	}
	approvalID := toString(parkRows[0]["approval_request_id"])
	if approvalID == "" {
		t.Fatal("approval_request_id not linked by the park")
	}

	// Nothing may have executed while the approval was pending.
	resRows, err := th.conn.Query(th.ctx,
		`SELECT COUNT(*) AS cnt FROM tool_results WHERE request_id = $1`, reqID)
	if err != nil {
		t.Fatalf("count tool_results: %v", err)
	}
	if toInt(resRows[0]["cnt"]) != 0 {
		t.Fatalf("tool_results rows while pending = %d, want 0", toInt(resRows[0]["cnt"]))
	}

	// The approval must be visible on the documented review surface.
	status, body = doAPIJSON(t, ts.URL, http.MethodGet, "/api/v1/approvals/"+approvalID, adminKey, "")
	if status != http.StatusOK {
		t.Fatalf("get approval: %d %s", status, body)
	}
	if !strings.Contains(body, "gated_echo") {
		t.Errorf("approval detail does not name the gated tool: %s", body)
	}

	// ---- 5. Human approves through REST. ----
	status, body = doAPIJSON(t, ts.URL, http.MethodPost,
		"/api/v1/approvals/"+approvalID+"/review", adminKey,
		`{"decision":"approved","notes":"df62 flow test"}`)
	if status != http.StatusOK {
		t.Fatalf("approve: %d %s", status, body)
	}

	// ---- 6. Executor poll #2: the approved request executes. ----
	if n, err := executor.PollOnce(th.ctx); err != nil {
		t.Fatalf("poll #2: %v", err)
	} else if n != 1 {
		t.Fatalf("poll #2 processed %d requests, want 1 (approved tool must execute)", n)
	}

	doneRows, err := th.conn.Query(th.ctx, `SELECT status FROM tool_requests WHERE id = $1`, reqID)
	if err != nil {
		t.Fatalf("query after execute: %v", err)
	}
	if got := toString(doneRows[0]["status"]); got != "completed" {
		t.Fatalf("tool_request after approve+poll = %q, want completed", got)
	}

	resultRows, err := th.conn.Query(th.ctx,
		`SELECT output, is_error FROM tool_results WHERE request_id = $1`, reqID)
	if err != nil {
		t.Fatalf("query tool_results: %v", err)
	}
	if len(resultRows) != 1 {
		t.Fatalf("tool_results rows = %d, want 1", len(resultRows))
	}
	if toInt(resultRows[0]["is_error"]) != 0 {
		t.Fatalf("execution is_error = %d, want 0", toInt(resultRows[0]["is_error"]))
	}
	if out := toString(resultRows[0]["output"]); !strings.Contains(out, "df62-approved") {
		t.Errorf("tool output = %q, want it to contain the echo marker", out)
	}

	t.Log("registered-via-REST → enqueued → parked → approved-via-REST → executed")
}
