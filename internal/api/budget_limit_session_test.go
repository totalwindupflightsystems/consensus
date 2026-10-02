// Package api: integration tests for the budget_limit_cents create-session
// surface (REVIEW-CONSENSUS-6).
//
// The harness already enforces budget_limit_cents per LLM call (Step 1.5 in
// internal/harness/executor.go, BudgetCheck). These tests prove the knob is
// reachable from the public API: POST /api/v1/sessions accepts it, the row
// persists it, and GET /api/v1/sessions/{id} surfaces it.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// createSessionWithBudget POSTs a session with the given body and returns the
// decoded response plus the recorder.
func createSessionWithBudget(t *testing.T, srv *integrationServer, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

// TestCreateSession_BudgetLimitCents_PersistedAndSurfaced is the
// REVIEW-CONSENSUS-6 acceptance cell: budget_limit_cents flows from the
// request body into the sessions row and back out through GET.
func TestCreateSession_BudgetLimitCents_PersistedAndSurfaced(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	w, resp := createSessionWithBudget(t, srv,
		`{"agent_name":"budget-test","goal":"prove the budget knob is reachable","budget_limit_cents":1}`)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("create session: expected 2xx, got %d: %s", w.Code, w.Body.String())
	}

	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("create session: no id in response: %s", w.Body.String())
	}

	// (a) The create response echoes the limit.
	if got, _ := resp["budget_limit_cents"].(float64); got != 1 {
		t.Fatalf("create response budget_limit_cents=%v, want 1: %s", resp["budget_limit_cents"], w.Body.String())
	}

	// (b) The DB row carries it (source of record, end-to-end insert path).
	row, err := srv.conn.QueryRow(context.Background(),
		`SELECT budget_limit_cents FROM sessions WHERE id = $1`, id)
	if err != nil || row == nil {
		t.Fatalf("read back session row: err=%v row=%v", err, row)
	}
	if got := toInt64(row["budget_limit_cents"]); got != 1 {
		t.Fatalf("sessions.budget_limit_cents=%d, want 1 (row: %v)", got, row)
	}

	// (c) GET /api/v1/sessions/{id} surfaces it.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	gw := httptest.NewRecorder()
	srv.router.ServeHTTP(gw, req)
	if gw.Code != http.StatusOK {
		t.Fatalf("get session: expected 200, got %d: %s", gw.Code, gw.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(gw.Body.Bytes(), &got); err != nil {
		t.Fatalf("get session: decode: %v", err)
	}
	if v, _ := got["budget_limit_cents"].(float64); v != 1 {
		t.Fatalf("GET budget_limit_cents=%v, want 1: %s", got["budget_limit_cents"], gw.Body.String())
	}
}

// TestCreateSession_BudgetLimitCents_ZeroMeansNoLimit pins the existing
// behavior: an absent (or zero) limit stays zero on the row — the harness
// treats limit <= 0 as unlimited.
func TestCreateSession_BudgetLimitCents_ZeroMeansNoLimit(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	w, resp := createSessionWithBudget(t, srv,
		`{"agent_name":"budget-default","goal":"no limit requested"}`)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("create session: expected 2xx, got %d: %s", w.Code, w.Body.String())
	}
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("create session: no id in response: %s", w.Body.String())
	}
	if v, present := resp["budget_limit_cents"]; present && v != float64(0) {
		t.Fatalf("zero-limit create echoed budget_limit_cents=%v, want absent/0", v)
	}

	row, err := srv.conn.QueryRow(context.Background(),
		`SELECT budget_limit_cents FROM sessions WHERE id = $1`, id)
	if err != nil || row == nil {
		t.Fatalf("read back session row: err=%v row=%v", err, row)
	}
	if got := toInt64(row["budget_limit_cents"]); got != 0 {
		t.Fatalf("sessions.budget_limit_cents=%d, want 0", got)
	}
}
