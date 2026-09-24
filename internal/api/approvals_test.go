// Package api: integration tests for HITL approval endpoints with real SQLite backend.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-5/step-2-5-2 test=internal/api/approvals_test.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// GET /api/v1/approvals — List pending approvals
// ============================================================================

func TestListApprovals_Empty(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var approvals []ApprovalResponse
	if err := json.NewDecoder(w.Body).Decode(&approvals); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(approvals) != 0 {
		t.Errorf("expected 0 approvals, got %d", len(approvals))
	}
}

func TestListApprovals_WithData(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-ap1', 'test', 'gpt-4o', 'paused', 'Goal', $1, $1)`, now)

	// low risk first, but ordering should put high risk first
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-1', 'sess-ap1', 1, 'tool_execution', 'Low risk tool', 'low', 'pending', $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-2', 'sess-ap1', 2, 'destructive_action', 'High risk action', 'high', 'pending', $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-3', 'sess-ap1', 3, 'schema_change', 'Medium risk', 'medium', 'pending', $1)`, now)
	// already approved — not listed
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-4', 'sess-ap1', 4, 'budget_override', 'Done', 'low', 'approved', $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var approvals []ApprovalResponse
	if err := json.NewDecoder(w.Body).Decode(&approvals); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(approvals) != 3 {
		t.Fatalf("expected 3 pending approvals, got %d", len(approvals))
	}

	// High risk should be first
	if approvals[0].RiskLevel != "high" || approvals[0].ID != "ap-2" {
		t.Errorf("expected high risk first, got %s (%s)", approvals[0].RiskLevel, approvals[0].ID)
	}
	// Then medium
	if approvals[1].RiskLevel != "medium" || approvals[1].ID != "ap-3" {
		t.Errorf("expected medium second, got %s (%s)", approvals[1].RiskLevel, approvals[1].ID)
	}
	// Then low
	if approvals[2].RiskLevel != "low" || approvals[2].ID != "ap-1" {
		t.Errorf("expected low third, got %s (%s)", approvals[2].RiskLevel, approvals[2].ID)
	}
}

func TestListApprovals_SessionFilter(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-a', 'test', 'gpt-4o', 'paused', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-b', 'test', 'gpt-4o', 'paused', 'Goal', $1, $1)`, now)

	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-a', 'sess-a', 1, 'tool_execution', 'Session A', 'medium', 'pending', $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-b', 'sess-b', 1, 'destructive_action', 'Session B', 'high', 'pending', $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals?session_id=sess-a", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var approvals []ApprovalResponse
	json.NewDecoder(w.Body).Decode(&approvals)

	if len(approvals) != 1 {
		t.Fatalf("expected 1 approval for sess-a, got %d", len(approvals))
	}
	if approvals[0].ID != "ap-a" {
		t.Errorf("expected ap-a, got %s", approvals[0].ID)
	}
}

func TestListApprovals_Unauthorized(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	// Create a session-scoped key
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-unauth', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	sessionKey := "cs_sk_session_unauth_test_123_xyz"
	hash := sha256Hash(sessionKey)
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-sess-ap', $1, $2, 'session', 'sess-unauth', datetime('now'))`, hash, sessionKey[:8])

	req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for session scope, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// GET /api/v1/approvals/:id — Get approval details
// ============================================================================

func TestGetApproval_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-get', 'test', 'gpt-4o', 'paused', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, target_tool, target_sql, context, status, created_at) VALUES ('ap-detail', 'sess-get', 5, 'tool_execution', 'Send email', 'high', 'send_email', 'SELECT * FROM test', '{"to":"test@test.com"}', 'pending', $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/ap-detail", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ApprovalResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.ID != "ap-detail" {
		t.Errorf("expected id ap-detail, got %s", resp.ID)
	}
	if resp.Description != "Send email" {
		t.Errorf("expected 'Send email', got %q", resp.Description)
	}
	if resp.RiskLevel != "high" {
		t.Errorf("expected 'high', got %q", resp.RiskLevel)
	}
	if resp.TargetTool == nil || *resp.TargetTool != "send_email" {
		t.Errorf("expected target_tool 'send_email', got %v", resp.TargetTool)
	}
	if resp.TargetSQL == nil || *resp.TargetSQL != "SELECT * FROM test" {
		t.Errorf("expected target_sql, got %v", resp.TargetSQL)
	}
}

func TestGetApproval_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetApproval_Unauthorized(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-get2', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	sessionKey := "cs_sk_session_get2_test_123_xyz"
	hash := sha256Hash(sessionKey)
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-get2', $1, $2, 'session', 'sess-get2', datetime('now'))`, hash, sessionKey[:8])

	req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/ap-detail", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// POST /api/v1/approvals/:id/review — Approve/reject/modify
// ============================================================================

func TestReviewApproval_Approve(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-rev1', 'test', 'gpt-4o', 'paused', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-rev1', 'sess-rev1', 10, 'tool_execution', 'Approve me', 'medium', 'pending', $1)`, now)

	body := `{"decision":"approved","notes":"Looks fine"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/ap-rev1/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ApprovalResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", resp.Status)
	}
	if resp.ReviewNotes == nil || *resp.ReviewNotes != "Looks fine" {
		t.Errorf("expected review_notes 'Looks fine', got %v", resp.ReviewNotes)
	}
	if resp.ReviewedAt == nil {
		t.Error("expected reviewed_at to be set")
	}

	// Session should be resumed from paused to idle
	sessions, _ := srv.conn.Query(ctx, `SELECT status FROM sessions WHERE id = 'sess-rev1'`)
	if len(sessions) > 0 && toString(sessions[0]["status"]) != "idle" {
		t.Errorf("expected session status 'idle' after approval, got %q", toString(sessions[0]["status"]))
	}
}

func TestReviewApproval_Reject(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-rej', 'test', 'gpt-4o', 'paused', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-rej', 'sess-rej', 1, 'destructive_action', 'Bad idea', 'critical', 'pending', $1)`, now)

	body := `{"decision":"rejected","notes":"This would delete production data"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/ap-rej/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ApprovalResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Status != "rejected" {
		t.Errorf("expected 'rejected', got %q", resp.Status)
	}
	if resp.ReviewNotes == nil || *resp.ReviewNotes != "This would delete production data" {
		t.Errorf("expected correct reject notes, got %v", resp.ReviewNotes)
	}
}

func TestReviewApproval_Modify(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-mod', 'test', 'gpt-4o', 'paused', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-mod', 'sess-mod', 3, 'tool_execution', 'Modify this', 'medium', 'pending', $1)`, now)

	body := `{"decision":"modified","notes":"Change the WHERE clause","modified_sql":"DELETE FROM temp WHERE created_at < now() - interval '7 days'"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/ap-mod/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ApprovalResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Status != "modified" {
		t.Errorf("expected 'modified', got %q", resp.Status)
	}
	if resp.ModifiedSQL == nil || !strings.Contains(*resp.ModifiedSQL, "interval '7 days'") {
		t.Errorf("expected modified_sql with interval, got %v", resp.ModifiedSQL)
	}
}

func TestReviewApproval_MissingModifiedSQL(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-miss', 'test', 'gpt-4o', 'paused', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-miss', 'sess-miss', 1, 'tool_execution', 'Test', 'medium', 'pending', $1)`, now)

	body := `{"decision":"modified","notes":"Forgot the SQL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/ap-miss/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing modified_sql, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReviewApproval_AlreadyReviewed(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-done', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-done', 'sess-done', 1, 'tool_execution', 'Done', 'low', 'approved', $1)`, now)

	body := `{"decision":"rejected"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/ap-done/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for already reviewed, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReviewApproval_InvalidDecision(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-inv', 'test', 'gpt-4o', 'paused', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-inv', 'sess-inv', 1, 'tool_execution', 'Test', 'low', 'pending', $1)`, now)

	body := `{"decision":"maybe_later"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/ap-inv/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid decision, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReviewApproval_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	body := `{"decision":"approved"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/nonexistent/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReviewApproval_Unauthorized(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-unau', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	sessionKey := "cs_sk_session_unau_test_123_xyz"
	hash := sha256Hash(sessionKey)
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-unau', $1, $2, 'session', 'sess-unau', datetime('now'))`, hash, sessionKey[:8])

	body := `{"decision":"approved"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/ap-inv/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for session scope, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// GET /api/v1/sessions/:id/approvals — Session-scoped approvals list
// ============================================================================

func TestSessionApprovals_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-scp', 'test', 'gpt-4o', 'paused', 'Goal', $1, $1)`, now)

	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-scp1', 'sess-scp', 1, 'tool_execution', 'First', 'high', 'pending', $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-scp2', 'sess-scp', 2, 'destructive_action', 'Second (approved)', 'medium', 'approved', $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-scp/approvals", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var approvals []ApprovalResponse
	if err := json.NewDecoder(w.Body).Decode(&approvals); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Default status filter is 'pending', so only ap-scp1
	if len(approvals) != 1 {
		t.Fatalf("expected 1 pending approval, got %d", len(approvals))
	}
	if approvals[0].ID != "ap-scp1" {
		t.Errorf("expected ap-scp1, got %s", approvals[0].ID)
	}
}

func TestSessionApprovals_StatusFilter(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-stf', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-stf1', 'sess-stf', 1, 'tool_execution', 'Pending', 'low', 'pending', $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-stf2', 'sess-stf', 2, 'tool_execution', 'Approved', 'low', 'approved', $1)`, now)

	// Filter to only approved
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-stf/approvals?status=approved", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var approvals []ApprovalResponse
	json.NewDecoder(w.Body).Decode(&approvals)

	if len(approvals) != 1 {
		t.Fatalf("expected 1 approved, got %d", len(approvals))
	}
	if approvals[0].ID != "ap-stf2" {
		t.Errorf("expected ap-stf2, got %s", approvals[0].ID)
	}
}

func TestSessionApprovals_All(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-all', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-all1', 'sess-all', 1, 'tool_execution', 'Pending', 'low', 'pending', $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-all2', 'sess-all', 2, 'tool_execution', 'Approved', 'low', 'approved', $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-all3', 'sess-all', 3, 'tool_execution', 'Rejected', 'low', 'rejected', $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-all/approvals?status=all", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var approvals []ApprovalResponse
	json.NewDecoder(w.Body).Decode(&approvals)

	if len(approvals) != 3 {
		t.Fatalf("expected 3 approvals total, got %d", len(approvals))
	}
}

func TestSessionApprovals_WrongSession(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-mine', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-other', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	sessionKey := "cs_sk_session_mine_test_123_xyz"
	hash := sha256Hash(sessionKey)
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-mine', $1, $2, 'session', 'sess-mine', datetime('now'))`, hash, sessionKey[:8])

	// Try to access OTHER session's approvals with MY session key
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-other/approvals", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-session access, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSessionApprovals_OwnSessionKey(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-own', 'test', 'gpt-4o', 'paused', 'Goal', $1, $1)`, now)

	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-own', 'sess-own', 1, 'tool_execution', 'Own approval', 'high', 'pending', $1)`, now)

	sessionKey := "cs_sk_session_own_test_123_xyz"
	hash := sha256Hash(sessionKey)
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-own', $1, $2, 'session', 'sess-own', datetime('now'))`, hash, sessionKey[:8])

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-own/approvals", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for own session, got %d: %s", w.Code, w.Body.String())
	}

	var approvals []ApprovalResponse
	json.NewDecoder(w.Body).Decode(&approvals)

	if len(approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(approvals))
	}
	if approvals[0].ID != "ap-own" {
		t.Errorf("expected ap-own, got %s", approvals[0].ID)
	}
}

func TestSessionApprovals_Unauthorized(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-ro', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	readonlyKey := "cs_ro_readonly_test_123456_xyz"
	hash := sha256Hash(readonlyKey)
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('key-ro-ap', $1, $2, 'readonly', datetime('now'))`, hash, readonlyKey[:8])

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-ro/approvals", nil)
	req.Header.Set("Authorization", "Bearer "+readonlyKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for readonly scope, got %d: %s", w.Code, w.Body.String())
	}
}
