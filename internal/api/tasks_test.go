// Package api: integration tests for task endpoints with real SQLite backend.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-3/step-2-3-2 test=internal/api/tasks_test.go
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
// Create Task Tests — POST /api/v1/sessions/{id}/tasks
// ============================================================================

func TestCreateTask_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-task', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	body := `{"title":"Analyze Q4 data","description":"Look at revenue trends","priority":3}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/sess-task/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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

	if resp.ID == "" {
		t.Error("expected non-empty task ID")
	}
	if resp.Title != "Analyze Q4 data" {
		t.Errorf("expected title 'Analyze Q4 data', got %q", resp.Title)
	}
	if resp.Description == nil || *resp.Description != "Look at revenue trends" {
		t.Errorf("expected description 'Look at revenue trends', got %v", resp.Description)
	}
	if resp.Status != "pending" {
		t.Errorf("expected status 'pending', got %q", resp.Status)
	}
	if resp.Priority != 3 {
		t.Errorf("expected priority 3, got %d", resp.Priority)
	}
	if resp.SessionID != "sess-task" {
		t.Errorf("expected session 'sess-task', got %q", resp.SessionID)
	}
}

func TestCreateTask_DefaultPriority(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-def', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	body := `{"title":"Do something"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/sess-def/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TaskResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Priority != 5 {
		t.Errorf("expected default priority 5, got %d", resp.Priority)
	}
}

func TestCreateTask_WithPrerequisites(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-pre', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	body := `{"title":"Dependent task","description":"Depends on other","prerequisite_ids":["task-a","task-b"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/sess-pre/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TaskResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.PrerequisiteIDs) != 2 {
		t.Errorf("expected 2 prerequisites, got %d", len(resp.PrerequisiteIDs))
	}
}

func TestCreateTask_MissingTitle(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-mt', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	body := `{"description":"No title here"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/sess-mt/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTask_InvalidPriority(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-ip', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	tests := []struct {
		name         string
		priority     string
		expect200    bool
	}{
		{"too low (defaulted)", `{"title":"Test","priority":0}`, true},   // 0 → defaults to 5
		{"too high", `{"title":"Test","priority":11}`, false},            // 11 rejected
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/sess-ip/tasks", strings.NewReader(tt.priority))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+srv.adminKey)
			w := httptest.NewRecorder()
			srv.router.ServeHTTP(w, req)

			expectedCode := http.StatusBadRequest
			if tt.expect200 {
				expectedCode = http.StatusOK
			}
			if w.Code != expectedCode {
				t.Errorf("expected %d, got %d: %s", expectedCode, w.Code, w.Body.String())
			}
		})
	}
}

func TestCreateTask_SessionScoped_CanAccessOwn(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-task-own', 'mine', 'gpt-4o', 'idle', 'Mine', $1, $1)`, now)

	sessionKey := "cs_sk_task_own_test_key_abc"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-task-own', $1, $2, 'session', 'sess-task-own', datetime('now'))`, hash, prefix)

	body := `{"title":"My task"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/sess-task-own/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTask_SessionScoped_CannotAccessOther(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-task-mine', 'mine', 'gpt-4o', 'idle', 'Mine', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-task-other', 'other', 'gpt-4o', 'idle', 'Other', $1, $1)`, now)

	sessionKey := "cs_sk_task_mine_xyz_abcdef"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-task-mine', $1, $2, 'session', 'sess-task-mine', datetime('now'))`, hash, prefix)

	body := `{"title":"Hack attempt"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/sess-task-other/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// List Tasks Tests — GET /api/v1/sessions/{id}/tasks
// ============================================================================

func TestListTasks_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-lt', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, description, status, priority, created_at) VALUES ('task-1', 'sess-lt', 'Task A', 'Desc A', 'pending', 8, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, description, status, priority, created_at) VALUES ('task-2', 'sess-lt', 'Task B', 'Desc B', 'pending', 3, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-lt/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tasks []TaskResponse
	if err := json.NewDecoder(w.Body).Decode(&tasks); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}

	// Tasks should be ordered by priority DESC, created_at ASC
	if len(tasks) >= 2 {
		if tasks[0].Priority < tasks[1].Priority {
			t.Error("expected tasks ordered by priority DESC")
		}
	}
}

func TestListTasks_WithStatusFilter(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-lt2', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, status, priority, created_at) VALUES ('task-a', 'sess-lt2', 'Pending task', 'pending', 5, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, status, priority, created_at) VALUES ('task-b', 'sess-lt2', 'Failed task', 'failed', 5, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-lt2/tasks?status=pending", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tasks []TaskResponse
	json.NewDecoder(w.Body).Decode(&tasks)

	if len(tasks) != 1 {
		t.Errorf("expected 1 pending task, got %d", len(tasks))
	}
	if len(tasks) > 0 && tasks[0].Status != "pending" {
		t.Errorf("expected only pending tasks, got %q", tasks[0].Status)
	}
}

func TestListTasks_SessionScoped(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-lt-mine', 'mine', 'gpt-4o', 'idle', 'Mine', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, status, priority, created_at) VALUES ('task-mine', 'sess-lt-mine', 'My task', 'pending', 5, $1)`, now)

	sessionKey := "cs_sk_lt_mine_test_key_abc"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-lt-mine', $1, $2, 'session', 'sess-lt-mine', datetime('now'))`, hash, prefix)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-lt-mine/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tasks []TaskResponse
	json.NewDecoder(w.Body).Decode(&tasks)

	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
}

// ============================================================================
// Update Task Tests — PATCH /api/v1/tasks/{tid}
// ============================================================================

func TestUpdateTask_ValidTransitions(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-up', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, status, priority, created_at) VALUES ('task-up', 'sess-up', 'Update me', 'pending', 5, $1)`, now)

	transitions := []string{"claimed", "in_progress"}

	for _, target := range transitions {
		t.Run("to_"+target, func(t *testing.T) {
			// Set task back to pending first
			_ = srv.conn.Exec(ctx, `UPDATE tasks SET status = 'pending' WHERE id = 'task-up'`)

			body := `{"status":"` + target + `"}`
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/task-up", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+srv.adminKey)
			w := httptest.NewRecorder()

			srv.router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected 200 for %s, got %d: %s", target, w.Code, w.Body.String())
			}
		})
	}
}

func TestUpdateTask_InvalidTransition(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-inv', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, status, priority, created_at) VALUES ('task-inv', 'sess-inv', 'Invalid', 'pending', 5, $1)`, now)

	// Trying to go directly to "published" from "pending" should fail (needs reviewed first)
	body := `{"status":"published"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/task-inv", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateTask_CompleteWorkflow(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-flow', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, status, priority, created_at) VALUES ('task-flow', 'sess-flow', 'Full lifecycle', 'pending', 5, $1)`, now)

	steps := []string{"claimed", "in_progress", "reviewed", "published"}
	expectedCodes := []int{200, 200, 200, 200}

	for i, target := range steps {
		body := `{"status":"` + target + `"}`
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/task-flow", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+srv.adminKey)
		w := httptest.NewRecorder()

		srv.router.ServeHTTP(w, req)

		if w.Code != expectedCodes[i] {
			t.Fatalf("step %d (%s): expected %d, got %d: %s", i+1, target, expectedCodes[i], w.Code, w.Body.String())
		}

		// Verify status in DB
		rows, _ := srv.conn.Query(ctx, `SELECT status FROM tasks WHERE id = 'task-flow'`)
		if toString(rows[0]["status"]) != target {
			t.Errorf("step %d: expected status %q in DB, got %q", i+1, target, toString(rows[0]["status"]))
		}
	}
}

func TestUpdateTask_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	body := `{"status":"claimed"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/nonexistent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// Claim Task Tests — POST /api/v1/tasks/{tid}/claim
// ============================================================================

func TestClaimTask_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-claim', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, status, priority, created_at) VALUES ('task-claim', 'sess-claim', 'Claimable', 'pending', 5, $1)`, now)

	// Create a session-scoped key for claiming
	sessionKey := "cs_sk_claim_test_key_abcxyz"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-claim', $1, $2, 'session', 'sess-claim', datetime('now'))`, hash, prefix)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-claim/claim", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp TaskResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Status != "claimed" {
		t.Errorf("expected status 'claimed', got %q", resp.Status)
	}
	if resp.LockedByAgent == nil || *resp.LockedByAgent != "sess-claim" {
		t.Errorf("expected locked_by 'sess-claim', got %v", resp.LockedByAgent)
	}
	if resp.ClaimedAt == nil {
		t.Error("expected claimed_at to be set")
	}
}

func TestClaimTask_AlreadyClaimed(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-cl2', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, status, priority, locked_by_agent, created_at) VALUES ('task-cl2', 'sess-cl2', 'Locked', 'claimed', 5, 'sess-cl2', $1)`, now)

	sessionKey := "cs_sk_cl2_test_key_abcdef"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-cl2', $1, $2, 'session', 'sess-cl2', datetime('now'))`, hash, prefix)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-cl2/claim", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestClaimTask_CrossSessionClaim(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-a', 'a', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-b', 'b', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, status, priority, created_at) VALUES ('task-cs', 'sess-b', 'Other task', 'pending', 5, $1)`, now)

	// Session A claims Session B's task — allowed: any session can claim a pending task
	sessionKey := "cs_sk_cs_test_key_abcdefg"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-cs', $1, $2, 'session', 'sess-a', datetime('now'))`, hash, prefix)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-cs/claim", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	// Cross-session claiming is allowed as per SPEC-015 §3.3
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the task is locked to sess-a
	var resp TaskResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.LockedByAgent == nil || *resp.LockedByAgent != "sess-a" {
		t.Errorf("expected locked_by 'sess-a', got %v", resp.LockedByAgent)
	}
}

func TestClaimTask_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/nonexistent/claim", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
