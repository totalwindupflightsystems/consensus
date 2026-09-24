// Package api: tests for DOGFOOD-004 (session GET surfaces last error) and
// DOGFOOD-007 (heartbeat_at serialization consistency).
//
// axiom:trace work_item=dogfood-004,dogfood-007 spec=specs/015-api-and-mcp.md test=internal/api/dogfood_test.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGetSession_Failed_SurfacesLastError verifies that GET /api/v1/sessions/{id}
// includes the most recent audit_logs error_message for failed sessions (DOGFOOD-004).
func TestGetSession_Failed_SurfacesLastError(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := "2026-05-04T00:00:00Z"
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-fail', 'test', 'gpt-4o', 'failed', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO audit_logs (session_id, iteration, result, error_message, created_at) VALUES ('sess-fail', 1, 'rolled_back', 'llm: HTTP 401 — LLM auth failed: check your API key', $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-fail", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SessionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.LastError == nil {
		t.Fatal("expected last_error to be surfaced for failed session, got nil")
	}
	if *resp.LastError != "llm: HTTP 401 — LLM auth failed: check your API key" {
		t.Errorf("last_error = %q, want the audit_logs error message", *resp.LastError)
	}
}

// TestGetSession_Idle_NoLastError verifies last_error stays empty for non-failed sessions.
func TestGetSession_Idle_NoLastError(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := "2026-05-04T00:00:00Z"
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-ok', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-ok", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var resp SessionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.LastError != nil {
		t.Errorf("expected no last_error for idle session, got %q", *resp.LastError)
	}
}

// TestHeartbeatAt_ListAndDetail_Consistent verifies GET /api/v1/sessions and
// GET /api/v1/sessions/{id} return the same heartbeat_at for the same session
// (DOGFOOD-007).
func TestHeartbeatAt_ListAndDetail_Consistent(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	hb := "2026-05-04T12:30:00Z"
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-hb', 'test', 'gpt-4o', 'thinking', 'Goal', $1, $1)`, hb)

	// Detail response
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-hb", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("detail: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var detail SessionResponse
	if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
		t.Fatalf("detail decode: %v", err)
	}

	// List response
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req2.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w2 := httptest.NewRecorder()
	srv.router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var list []SessionResponse
	if err := json.NewDecoder(w2.Body).Decode(&list); err != nil {
		t.Fatalf("list decode: %v", err)
	}

	var listHB string
	for _, s := range list {
		if s.ID == "sess-hb" {
			listHB = s.HeartbeatAt
			break
		}
	}
	if listHB == "" {
		t.Fatal("session sess-hb not found in list response")
	}
	if listHB != detail.HeartbeatAt {
		t.Errorf("heartbeat_at inconsistent: list=%q detail=%q (DOGFOOD-007)", listHB, detail.HeartbeatAt)
	}
}
