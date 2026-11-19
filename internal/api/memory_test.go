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
	_ = json.NewDecoder(w.Body).Decode(&events)

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
	_ = json.NewDecoder(w.Body).Decode(&events)

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
	_ = json.NewDecoder(w.Body).Decode(&events)
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
	_ = json.NewDecoder(w.Body).Decode(&ctxRows)

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
	_ = json.NewDecoder(w.Body).Decode(&ctxRows)

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
	_ = json.NewDecoder(w.Body).Decode(&iterations)

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
	_ = json.NewDecoder(w.Body).Decode(&iterations)
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
	_ = json.NewDecoder(w.Body).Decode(&events)

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
	_ = json.NewDecoder(w.Body).Decode(&iterations)

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
	_ = json.NewDecoder(w.Body).Decode(&events)

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

// ============================================================================
// DF-CONSENSUS-073-R2 regression — sessions driven by the conversational
// planning loop record per-iteration state in agent_billing, NOT in
// iteration_commits. The endpoint used to return [] for them.
// ============================================================================

// listIterationsFor drives GET /api/v1/sessions/{id}/iterations with the admin
// key and decodes the response.
func listIterationsFor(t *testing.T, srv *integrationServer, sessionID, query string) []IterationCommitResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/iterations"+query, nil)
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
	return iterations
}

// TestListIterations_PlanningLoopSessionReportsBillingIterations is the
// DF-CONSENSUS-073 defect: a session with iteration >= 4 whose only
// per-iteration records are agent_billing rows (the planning loop never writes
// iteration_commits) must not answer [].
func TestListIterations_PlanningLoopSessionReportsBillingIterations(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	const sessionID = "sess-planning-loop"

	// iteration 4 with ZERO iteration_commits rows — the live shape of the
	// sessions named in the defect report (a238a831, 933db6f9).
	if err := srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at, heartbeat_at) VALUES ($1, 'dogfood-agent', 'gpt-4o', 'completed', 'goal', 4, $2, $2)`, sessionID, now); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if err := srv.conn.Exec(ctx, `INSERT INTO agent_billing (session_id, iteration, model_id, category, prompt_tokens, completion_tokens, cache_read_tokens, cache_write_tokens, cost_usd, recorded_at) VALUES ($1, 1, 'gpt-4o', 'cognition', 1000, 100, 10, 5, 0.001500, $2)`, sessionID, now); err != nil {
		t.Fatalf("insert billing 1: %v", err)
	}
	if err := srv.conn.Exec(ctx, `INSERT INTO agent_billing (session_id, iteration, model_id, category, prompt_tokens, completion_tokens, cache_read_tokens, cache_write_tokens, cost_usd, recorded_at) VALUES ($1, 2, 'gpt-4o', 'cognition', 2000, 200, 20, 10, 0.003000, $2)`, sessionID, now); err != nil {
		t.Fatalf("insert billing 2: %v", err)
	}
	// Two LLM calls inside iteration 3 — they must aggregate into ONE entry.
	if err := srv.conn.Exec(ctx, `INSERT INTO agent_billing (session_id, iteration, model_id, category, prompt_tokens, completion_tokens, cache_read_tokens, cache_write_tokens, cost_usd, recorded_at) VALUES ($1, 3, 'gpt-4o', 'cognition', 3000, 300, 30, 15, 0.004500, $2)`, sessionID, now); err != nil {
		t.Fatalf("insert billing 3a: %v", err)
	}
	if err := srv.conn.Exec(ctx, `INSERT INTO agent_billing (session_id, iteration, model_id, category, prompt_tokens, completion_tokens, cache_read_tokens, cache_write_tokens, cost_usd, recorded_at) VALUES ($1, 3, 'gpt-4o', 'tool_call', 4000, 400, 40, 20, 0.006000, $2)`, sessionID, now); err != nil {
		t.Fatalf("insert billing 3b: %v", err)
	}

	iterations := listIterationsFor(t, srv, sessionID, "")

	if len(iterations) != 3 {
		t.Fatalf("expected 3 iterations (one per billed iteration), got %d: %s", len(iterations), fmt.Sprintf("%+v", iterations))
	}

	// Newest first, matching the pre-existing iteration_id DESC contract.
	for i, want := range []int64{3, 2, 1} {
		if iterations[i].Iteration != want {
			t.Fatalf("iterations[%d].iteration = %d, want %d", i, iterations[i].Iteration, want)
		}
		if iterations[i].IterationID != want {
			t.Errorf("iterations[%d].iteration_id = %d, want %d (the iteration number)", i, iterations[i].IterationID, want)
		}
		if len(iterations[i].Sources) != 1 || iterations[i].Sources[0] != "agent_billing" {
			t.Errorf("iterations[%d].sources = %v, want [agent_billing]", i, iterations[i].Sources)
		}
		if len(iterations[i].ActivePointers) != 0 || iterations[i].ActivePointers == nil {
			t.Errorf("iterations[%d].active_pointers = %v, want empty (non-nil) array", i, iterations[i].ActivePointers)
		}
		if iterations[i].CreatedAt == "" {
			t.Errorf("iterations[%d].created_at is empty", i)
		}
	}

	// Aggregation: iteration 3 folds both LLM calls.
	latest := iterations[0]
	if latest.LLMCalls != 2 {
		t.Errorf("iteration 3 llm_calls = %d, want 2", latest.LLMCalls)
	}
	if latest.PromptTokens != 7000 || latest.CompletionTokens != 700 {
		t.Errorf("iteration 3 tokens = %d/%d, want 7000/700", latest.PromptTokens, latest.CompletionTokens)
	}
	if latest.CacheReadTokens != 70 || latest.CacheWriteTokens != 35 {
		t.Errorf("iteration 3 cache tokens = %d/%d, want 70/35", latest.CacheReadTokens, latest.CacheWriteTokens)
	}
	if diff := latest.CostUSD - 0.0105; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("iteration 3 cost_usd = %v, want 0.0105", latest.CostUSD)
	}
	if latest.ModelID != "gpt-4o" {
		t.Errorf("iteration 3 model_id = %q, want gpt-4o", latest.ModelID)
	}

	// The limit must bound the merged view, not the underlying queries.
	limited := listIterationsFor(t, srv, sessionID, "?limit=2")
	if len(limited) != 2 {
		t.Fatalf("expected 2 iterations with limit=2, got %d", len(limited))
	}
	if limited[0].Iteration != 3 || limited[1].Iteration != 2 {
		t.Errorf("limit=2 returned iterations %d,%d, want 3,2", limited[0].Iteration, limited[1].Iteration)
	}
}

// TestListIterations_MergesSnapshotAndBilling covers the session shape that has
// BOTH stores populated (the SQL-executor loop): one entry per iteration, with
// the snapshot's structural fields and the billing aggregate on the same row.
func TestListIterations_MergesSnapshotAndBilling(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	const sessionID = "sess-merged"

	if err := srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at, heartbeat_at) VALUES ($1, 'test', 'gpt-4o', 'idle', 'goal', 2, $2, $2)`, sessionID, now); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	// A snapshot written by WriteIterationSnapshot keys its row by iteration.
	if err := srv.conn.Exec(ctx, `INSERT INTO iteration_commits (session_id, iteration_id, active_pointers, display_rules, llm_response, sql_executed, rows_affected, created_at) VALUES ($1, 2, '[7,8]', '{}', '{"thought":"ok"}', '["INSERT INTO memory_events VALUES (1)"]', 1, $2)`, sessionID, now); err != nil {
		t.Fatalf("insert iteration commit: %v", err)
	}
	if err := srv.conn.Exec(ctx, `INSERT INTO agent_billing (session_id, iteration, model_id, category, prompt_tokens, completion_tokens, cost_usd, recorded_at) VALUES ($1, 1, 'gpt-4o', 'cognition', 500, 50, 0.000500, $2)`, sessionID, now); err != nil {
		t.Fatalf("insert billing 1: %v", err)
	}
	if err := srv.conn.Exec(ctx, `INSERT INTO agent_billing (session_id, iteration, model_id, category, prompt_tokens, completion_tokens, cost_usd, recorded_at) VALUES ($1, 2, 'gpt-4o', 'cognition', 1500, 150, 0.002000, $2)`, sessionID, now); err != nil {
		t.Fatalf("insert billing 2: %v", err)
	}

	iterations := listIterationsFor(t, srv, sessionID, "")
	if len(iterations) != 2 {
		t.Fatalf("expected 2 iterations, got %d", len(iterations))
	}

	merged := iterations[0] // iteration 2: snapshot + billing
	if merged.Iteration != 2 {
		t.Fatalf("iterations[0].iteration = %d, want 2", merged.Iteration)
	}
	if len(merged.Sources) != 2 || merged.Sources[0] != "iteration_commit" || merged.Sources[1] != "agent_billing" {
		t.Errorf("iteration 2 sources = %v, want [iteration_commit agent_billing]", merged.Sources)
	}
	// Snapshot fields survive the merge.
	if merged.RowsAffected != 1 {
		t.Errorf("iteration 2 rows_affected = %d, want 1", merged.RowsAffected)
	}
	if merged.LLMResponse == nil {
		t.Error("iteration 2 llm_response is nil, want the snapshot value")
	}
	if len(merged.ActivePointers) != 2 {
		t.Errorf("iteration 2 active_pointers = %v, want 2 elements", merged.ActivePointers)
	}
	if len(merged.SQLExecuted) != 1 {
		t.Errorf("iteration 2 sql_executed = %v, want 1 statement", merged.SQLExecuted)
	}
	// Billing aggregate lands on the same entry.
	if merged.PromptTokens != 1500 || merged.CompletionTokens != 150 {
		t.Errorf("iteration 2 tokens = %d/%d, want 1500/150", merged.PromptTokens, merged.CompletionTokens)
	}
	if diff := merged.CostUSD - 0.002; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("iteration 2 cost_usd = %v, want 0.002", merged.CostUSD)
	}

	billingOnly := iterations[1] // iteration 1: billing only
	if len(billingOnly.Sources) != 1 || billingOnly.Sources[0] != "agent_billing" {
		t.Errorf("iteration 1 sources = %v, want [agent_billing]", billingOnly.Sources)
	}
	if billingOnly.PromptTokens != 500 {
		t.Errorf("iteration 1 prompt_tokens = %d, want 500", billingOnly.PromptTokens)
	}
}
