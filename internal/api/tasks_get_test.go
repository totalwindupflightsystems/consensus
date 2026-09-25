// Package api: tests for GET /api/v1/tasks/{taskId} (DF-CONSENSUS-18).
//
// The served OpenAPI spec declares GET on /api/v1/tasks/{taskId} but only
// PATCH /claim handlers existed — GET answered 405. These tests pin the new
// handler's contract: 200 with the task JSON, 404 with the standard error
// shape for an unknown ID, and session-scoped access enforcement.
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

// insertTaskRow seeds one task row directly, bypassing the create endpoint,
// so the GET test does not depend on POST behavior.
func insertTaskRow(t *testing.T, srv *integrationServer, id, sessionID, title string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at)
		 VALUES ($1, 'test', 'gpt-4o', 'idle', 'Goal', $2, $2)`,
		sessionID, now)
	_ = srv.conn.Exec(ctx,
		`INSERT INTO tasks (id, session_id, title, description, status, priority, prerequisite_ids, created_at)
		 VALUES ($1, $2, $3, 'desc', 'pending', 4, '[]', $4)`,
		id, sessionID, title, now)
}

func TestGetTask_ReturnsTask(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	insertTaskRow(t, srv, "task-get-1", "sess-get-task", "Write the report")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-get-1", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TaskResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != "task-get-1" {
		t.Errorf("expected id task-get-1, got %q", resp.ID)
	}
	if resp.SessionID != "sess-get-task" {
		t.Errorf("expected session sess-get-task, got %q", resp.SessionID)
	}
	if resp.Title != "Write the report" {
		t.Errorf("expected title 'Write the report', got %q", resp.Title)
	}
	if resp.Status != "pending" {
		t.Errorf("expected status pending, got %q", resp.Status)
	}
	if resp.Priority != 4 {
		t.Errorf("expected priority 4, got %d", resp.Priority)
	}
}

// TestGetTask_NotFound pins the acceptance criterion "404 with the standard
// error shape when the task id does not exist" — and never 405 (the pre-fix
// symptom the spec-vs-runtime sweep measured).
func TestGetTask_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-missing", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected error code NOT_FOUND, got %q", errResp.Error.Code)
	}
	if errResp.Error.Message != "task not found" {
		t.Errorf("expected message 'task not found', got %q", errResp.Error.Message)
	}
}

// TestGetTask_MethodRegistered proves the route is mounted: before
// DF-CONSENSUS-18 the method was unregistered and chi answered 405.
func TestGetTask_MethodRegistered(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	insertTaskRow(t, srv, "task-get-2", "sess-get-task-2", "Second task")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-get-2", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code == http.StatusMethodNotAllowed {
		t.Fatal("GET /api/v1/tasks/{taskID} answered 405 — route not registered")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetTask_SessionScoped_CannotAccessOther(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	insertTaskRow(t, srv, "task-get-3", "sess-get-other", "Other session's task")

	// The session-scoped key references its own session (FK), so the owning
	// session row must exist even though this test never touches it.
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at)
		 VALUES ('sess-get-mine', 'mine', 'gpt-4o', 'idle', 'Mine', $1, $1)`, now)

	sessionKey := "cs_sk_get_task_other_xyz"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at)
		 VALUES ('key-get-other', $1, $2, 'session', 'sess-get-mine', datetime('now'))`,
		hash, prefix)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-get-3", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "FORBIDDEN") {
		t.Errorf("expected FORBIDDEN error code in body, got %q", w.Body.String())
	}
}
