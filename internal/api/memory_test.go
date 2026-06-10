// Package api: integration tests for memory & context endpoints with real SQLite backend.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-2/step-2-2-2 test=internal/api/memory_test.go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ============================================================================
// List Memory Tests — GET /api/v1/sessions/{id}/memory
// ============================================================================

func TestListMemory_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-mem', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	// Seed memory events
	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('text_block', 'First message', 'sess-mem', 1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('thinking', 'Agent thought', 'sess-mem', 1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('tool_result', 'Tool ran', 'sess-mem', 2, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-mem/memory", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var events []MemoryEventResponse
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	// Events should be ordered by id DESC (newest first)
	if len(events) >= 3 {
		if events[0].Type != "tool_result" {
			t.Errorf("expected first event type tool_result, got %q", events[0].Type)
		}
	}

	// Verify display_mode defaults to 'full'
	for _, evt := range events {
		if evt.DisplayMode != "full" {
			t.Errorf("expected default display_mode 'full' for event %d, got %q", evt.ID, evt.DisplayMode)
		}
	}
}

func TestListMemory_WithTypeFilter(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-mem2', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('text_block', 'Text', 'sess-mem2', 1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('thinking', 'Thought', 'sess-mem2', 1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('text_block', 'More text', 'sess-mem2', 2, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-mem2/memory?type=text_block", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var events []MemoryEventResponse
	json.NewDecoder(w.Body).Decode(&events)

	if len(events) != 2 {
		t.Errorf("expected 2 text_block events, got %d", len(events))
	}
	for _, evt := range events {
		if evt.Type != "text_block" {
			t.Errorf("expected only text_block events, got %q", evt.Type)
		}
	}
}

func TestListMemory_WithLimit(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-mem3', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	// Seed 5 events
	for i := 1; i <= 5; i++ {
		_ = srv.conn.Exec(ctx, fmt.Sprintf(`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('text_block', 'Event %d', 'sess-mem3', 1, $1)`, i), now)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-mem3/memory?limit=3", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var events []MemoryEventResponse
	json.NewDecoder(w.Body).Decode(&events)

	if len(events) != 3 {
		t.Errorf("expected 3 events (limit=3), got %d", len(events))
	}
}

func TestListMemory_SessionScoped_CanAccessOwn(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-mine', 'mine', 'gpt-4o', 'idle', 'Mine', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('text_block', 'My memory', 'sess-mine', 1, $1)`, now)

	sessionKey := "cs_sk_mem_mine_test_key_abc"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-mem-mine', $1, $2, 'session', 'sess-mine', datetime('now'))`, hash, prefix)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-mine/memory", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var events []MemoryEventResponse
	json.NewDecoder(w.Body).Decode(&events)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestListMemory_SessionScoped_CannotAccessOther(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-mine', 'mine', 'gpt-4o', 'idle', 'Mine', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-other', 'other', 'gpt-4o', 'idle', 'Other', $1, $1)`, now)

	sessionKey := "cs_sk_mem_mine_xyz_abcdefgh"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-mem-mine2', $1, $2, 'session', 'sess-mine', datetime('now'))`, hash, prefix)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-other/memory", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// Get Single Memory Event Tests — GET /api/v1/sessions/{id}/memory/{mid}
// ============================================================================

func TestGetMemoryEvent_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-single', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('text_block', 'Hello world', 'sess-single', 1, $1)`, now)

	// Get the ID of the inserted event
	rows, _ := srv.conn.Query(ctx, `SELECT id FROM memory_events WHERE session_id = 'sess-single'`)
	memID := fmt.Sprintf("%d", toInt64(rows[0]["id"]))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-single/memory/"+memID, nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var evt MemoryEventResponse
	if err := json.NewDecoder(w.Body).Decode(&evt); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if evt.Type != "text_block" {
		t.Errorf("expected type text_block, got %q", evt.Type)
	}
	if evt.Content != "Hello world" {
		t.Errorf("expected content 'Hello world', got %q", evt.Content)
	}
	if evt.SessionID != "sess-single" {
		t.Errorf("expected session 'sess-single', got %q", evt.SessionID)
	}
}

func TestGetMemoryEvent_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-nf', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-nf/memory/99999", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetMemoryEvent_DifferentSessionEvent_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-a', 'a', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-b', 'b', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	// Insert event in sess-a
	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('text_block', 'Event A', 'sess-a', 1, $1)`, now)
	rows, _ := srv.conn.Query(ctx, `SELECT id FROM memory_events WHERE session_id = 'sess-a'`)
	memID := fmt.Sprintf("%d", toInt64(rows[0]["id"]))

	// Try to access event A from session B — should 404 because query also filters by session_id
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-b/memory/"+memID, nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 (event belongs to other session), got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// Get Active Context Tests — GET /api/v1/sessions/{id}/context
// ============================================================================

func TestGetActiveContext_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-ctx', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('text_block', 'Visible content', 'sess-ctx', 1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('thinking', 'Agent reasoning', 'sess-ctx', 1, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-ctx/context", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var ctxRows []ActiveContextResponse
	if err := json.NewDecoder(w.Body).Decode(&ctxRows); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(ctxRows) != 2 {
		t.Errorf("expected 2 context rows, got %d", len(ctxRows))
	}

	for _, row := range ctxRows {
		if row.RenderedText == nil {
			t.Errorf("expected rendered_text for row %d (type=%s, mode=%s)", row.ID, row.Type, row.DisplayMode)
		}
		if row.DisplayMode != "full" {
			t.Errorf("expected full display_mode, got %q", row.DisplayMode)
		}
	}
}

func TestGetActiveContext_HiddenEventsExcluded(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-hide', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	// Insert events
	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (id, type, content, session_id, iteration_created, created_at) VALUES (1, 'text_block', 'Visible', 'sess-hide', 1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (id, type, content, session_id, iteration_created, created_at) VALUES (2, 'thinking', 'Hidden', 'sess-hide', 2, $1)`, now)

	// Set event 2 to hidden
	_ = srv.conn.Exec(ctx, `INSERT INTO display_modes (memory_id, mode, set_by_iteration, session_id) VALUES (2, 'hidden', 2, 'sess-hide')`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-hide/context", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var ctxRows []ActiveContextResponse
	json.NewDecoder(w.Body).Decode(&ctxRows)

	// Hidden event should be excluded — only 1 visible
	if len(ctxRows) != 1 {
		t.Errorf("expected 1 visible context row (hidden excluded), got %d", len(ctxRows))
	}
	if len(ctxRows) > 0 && ctxRows[0].ID != 1 {
		t.Errorf("expected event 1 (visible), got event %d", ctxRows[0].ID)
	}
}

func TestGetActiveContext_CompressedMode(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-compress', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (id, type, content, summary_text, session_id, iteration_created, created_at) VALUES (1, 'text_block', 'This is a very long piece of content that should be summarized', 'Summary: long content', 'sess-compress', 1, $1)`, now)

	// Set to compressed with summary available
	_ = srv.conn.Exec(ctx, `INSERT INTO display_modes (memory_id, mode, set_by_iteration, session_id) VALUES (1, 'compressed', 2, 'sess-compress')`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-compress/context", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var ctxRows []ActiveContextResponse
	json.NewDecoder(w.Body).Decode(&ctxRows)

	if len(ctxRows) != 1 {
		t.Fatalf("expected 1 context row, got %d", len(ctxRows))
	}

	row := ctxRows[0]
	if row.DisplayMode != "compressed" {
		t.Errorf("expected display_mode compressed, got %q", row.DisplayMode)
	}
	if row.RenderedText == nil || *row.RenderedText != "Summary: long content" {
		t.Errorf("expected rendered text to be summary 'Summary: long content', got %v", row.RenderedText)
	}
}

// ============================================================================
// List Iterations Tests — GET /api/v1/sessions/{id}/iterations
// ============================================================================

func TestListIterations_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-iter', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	// Seed iteration commits
	_ = srv.conn.Exec(ctx, `INSERT INTO iteration_commits (session_id, active_pointers, display_rules, llm_response, sql_executed, rows_affected, created_at) VALUES ('sess-iter', '[1,2,3]', '{}', '{"thought":"ok"}', '["INSERT INTO memory_events VALUES (1)"]', 1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO iteration_commits (session_id, active_pointers, display_rules, llm_response, sql_executed, rows_affected, created_at) VALUES ('sess-iter', '[4,5]', '{}', '{"thought":"done"}', '["INSERT INTO memory_events VALUES (2)"]', 1, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-iter/iterations", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var iterations []IterationCommitResponse
	if err := json.NewDecoder(w.Body).Decode(&iterations); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(iterations) != 2 {
		t.Errorf("expected 2 iterations, got %d", len(iterations))
	}

	// Iterations should be ordered by iteration_id DESC
	if len(iterations) >= 2 {
		if iterations[0].IterationID < iterations[1].IterationID {
			t.Error("expected iterations in DESC order")
		}
	}

	// Check fields
	iter := iterations[0]
	if len(iter.ActivePointers) == 0 {
		t.Error("expected active_pointers to be populated")
	}
	if iter.RowsAffected != 1 {
		t.Errorf("expected rows_affected 1, got %d", iter.RowsAffected)
	}
	if iter.LLMResponse == nil {
		t.Error("expected llm_response to be populated")
	}
	if len(iter.SQLExecuted) == 0 {
		t.Error("expected sql_executed to be populated")
	}
}

func TestListIterations_WithLimit(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-iter2', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	// Seed 4 iteration commits
	for i := 1; i <= 4; i++ {
		_ = srv.conn.Exec(ctx, fmt.Sprintf(`INSERT INTO iteration_commits (session_id, active_pointers, display_rules, rows_affected, created_at) VALUES ('sess-iter2', '[%d]', '{}', 1, $1)`, i), now)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-iter2/iterations?limit=2", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var iterations []IterationCommitResponse
	json.NewDecoder(w.Body).Decode(&iterations)

	if len(iterations) != 2 {
		t.Errorf("expected 2 iterations (limit=2), got %d", len(iterations))
	}
}

func TestListIterations_SessionScoped_CanAccessOwn(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-iter-mine', 'mine', 'gpt-4o', 'idle', 'Mine', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO iteration_commits (session_id, active_pointers, display_rules, rows_affected, created_at) VALUES ('sess-iter-mine', '[1]', '{}', 1, $1)`, now)

	sessionKey := "cs_sk_iter_mine_test_key_abc"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-iter-mine', $1, $2, 'session', 'sess-iter-mine', datetime('now'))`, hash, prefix)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-iter-mine/iterations", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var iterations []IterationCommitResponse
	json.NewDecoder(w.Body).Decode(&iterations)
	if len(iterations) != 1 {
		t.Errorf("expected 1 iteration, got %d", len(iterations))
	}
}

func TestListIterations_SessionScoped_CannotAccessOther(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-iter-mine2', 'mine', 'gpt-4o', 'idle', 'Mine', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-iter-other', 'other', 'gpt-4o', 'idle', 'Other', $1, $1)`, now)

	sessionKey := "cs_sk_iter_mine_xyz_abcdef"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-iter-mine2', $1, $2, 'session', 'sess-iter-mine2', datetime('now'))`, hash, prefix)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-iter-other/iterations", nil)
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// Empty Results Tests
// ============================================================================

func TestListMemory_NoEvents_ReturnsEmpty(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-empty', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-empty/memory", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var events []MemoryEventResponse
	json.NewDecoder(w.Body).Decode(&events)

	if len(events) != 0 {
		t.Errorf("expected empty array, got %d events", len(events))
	}
}

func TestListIterations_NoIterations_ReturnsEmpty(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-no-iter', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-no-iter/iterations", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var iterations []IterationCommitResponse
	json.NewDecoder(w.Body).Decode(&iterations)

	if len(iterations) != 0 {
		t.Errorf("expected empty array, got %d iterations", len(iterations))
	}
}

// ============================================================================
// Memory Event with Display Mode Tests
// ============================================================================

func TestListMemory_WithDisplayMode(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-dm', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	_ = srv.conn.Exec(ctx, `INSERT INTO memory_events (id, type, content, summary_text, session_id, iteration_created, created_at) VALUES (1, 'text_block', 'Full content', 'Summary', 'sess-dm', 1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO display_modes (memory_id, mode, set_by_iteration, session_id) VALUES (1, 'compressed', 2, 'sess-dm')`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-dm/memory", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var events []MemoryEventResponse
	json.NewDecoder(w.Body).Decode(&events)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].DisplayMode != "compressed" {
		t.Errorf("expected display_mode 'compressed', got %q", events[0].DisplayMode)
	}
}

// ============================================================================
// Admin vs Readonly Access Tests
// ============================================================================
