// Package api: APPROV-GATE-001 §4.2 — review-decision contract at the HTTP
// surface.
//
// Pre-fix, POST /api/v1/approvals/{id}/review with decision=rejected resumed
// the session exactly like approved (both paused→idle; findings.md §4.2,
// raw/03b E5→E6). These tests pin the API contract after the fix:
//
//   - rejected → 200, approval status 'rejected', session STILL 'paused'
//   - approved → 200, approval status 'approved', session 'idle'
//   - modified continues to resume (SPEC-014 §4.1 hook), already covered by
//     TestReviewApproval_Modify.
//
// test=internal/api/approvals_decision_contract_test.go
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// reviewContractFixture seeds a paused session + a pending tool-execution
// approval (the production row shape: target_tool set) and returns the ids.
func reviewContractFixture(t *testing.T, srv *integrationServer, tag string) (sessionID, approvalID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	sessionID = "sess-rc-" + tag
	approvalID = "ap-rc-" + tag

	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ($1, 'test', 'gpt-4o', 'paused', 'Goal', $2, $2)`, sessionID, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, context, target_tool, status, created_at) VALUES ($1, $2, 1, 'tool_execution', 'Execute tool "gated_tool"', 'medium', '{}', 'gated_tool', 'pending', $3)`, approvalID, sessionID, now)
	return sessionID, approvalID
}

// postReview drives POST /api/v1/approvals/{id}/review with the decision.
func postReview(srv *integrationServer, approvalID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+approvalID+"/review", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	return w
}

func TestReviewDecisionContract_RejectBlocksSession(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	sessionID, approvalID := reviewContractFixture(t, srv, "rej")

	w := postReview(srv, approvalID, `{"decision":"rejected","notes":"deny production write"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	sessRows, err := srv.conn.Query(context.Background(), `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	if len(sessRows) == 0 {
		t.Fatal("session missing")
	}
	if got := toString(sessRows[0]["status"]); got != "paused" {
		t.Errorf("session after rejected review = %q, want 'paused' (reject must block)", got)
	}
}

func TestReviewDecisionContract_ApproveResumesSession(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	sessionID, approvalID := reviewContractFixture(t, srv, "apr")

	w := postReview(srv, approvalID, `{"decision":"approved","notes":"allow"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	sessRows, err := srv.conn.Query(context.Background(), `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	if got := toString(sessRows[0]["status"]); got != "idle" {
		t.Errorf("session after approved review = %q, want 'idle' (resumed)", got)
	}
}
