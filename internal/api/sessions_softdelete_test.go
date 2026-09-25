// Package api: integration tests for session soft-delete tombstone semantics
// (DF-CONSENSUS-28, SPEC-015 §3.1, SPEC-003 §2.1).
//
// A DELETE /api/v1/sessions/{id} must actually delete — API-visibly. The row
// survives in the DB (soft delete), but every API surface treats the session
// as gone: list excludes it, get returns 404, message returns 410 and writes
// no ledger row, PATCH returns 404. DELETE itself is idempotent.
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

// seedSoftDeleteSession inserts a live session row and returns the id.
func seedSoftDeleteSession(t *testing.T, srv *integrationServer, id, status string) {
	t.Helper()
	ctx := context.Background()
	err := srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at)
		VALUES ($1, 'test', 'gpt-4o', $2, 'Goal', datetime('now'), datetime('now'))`, id, status)
	if err != nil {
		t.Fatalf("seed session %s: %v", id, err)
	}
}

// deleteSessionViaAPI issues DELETE /api/v1/sessions/{id} with the admin key.
func deleteSessionViaAPI(srv *integrationServer, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	return w
}

// countMemoryEventsFor counts memory_events rows for a session (direct DB read).
func countMemoryEventsFor(srv *integrationServer, id string) int {
	rows, _ := srv.conn.Query(context.Background(),
		`SELECT id FROM memory_events WHERE session_id = $1`, id)
	return len(rows)
}

// TestSoftDelete_ListAndGetHonorTombstone proves the tombstone flips the
// session from listed to absent and from gettable to 404.
func TestSoftDelete_ListAndGetHonorTombstone(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	seedSoftDeleteSession(t, srv, "sess-tomb", "idle")

	// Precondition: listed and gettable.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-tomb", nil)
	getReq.Header.Set("Authorization", "Bearer "+srv.adminKey)
	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, getReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("precondition GET by id: expected 200, got %d", rec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	listReq.Header.Set("Authorization", "Bearer "+srv.adminKey)
	listRec := httptest.NewRecorder()
	srv.router.ServeHTTP(listRec, listReq)
	var preList []SessionResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &preList); err != nil {
		t.Fatalf("precondition list: %v", err)
	}
	found := false
	for _, s := range preList {
		if s.ID == "sess-tomb" {
			found = true
		}
	}
	if !found {
		t.Fatal("precondition list: session should be listed before delete")
	}

	if rec := deleteSessionViaAPI(srv, "sess-tomb"); rec.Code != http.StatusOK {
		t.Fatalf("DELETE: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// GET by id → 404 NOT_FOUND (same shape as a missing id).
	rec = httptest.NewRecorder()
	srv.router.ServeHTTP(rec, getReq)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET by id after delete: expected 404, got %d: %s", rec.Code, rec.Body.String())
	} else if !strings.Contains(rec.Body.String(), "NOT_FOUND") {
		t.Errorf("GET by id after delete: expected NOT_FOUND error code, got %s", rec.Body.String())
	}

	// List excludes the tombstoned session.
	listRec = httptest.NewRecorder()
	srv.router.ServeHTTP(listRec, listReq)
	var postList []SessionResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &postList); err != nil {
		t.Fatalf("post list: %v", err)
	}
	for _, s := range postList {
		if s.ID == "sess-tomb" {
			t.Errorf("list after delete: deleted session still listed")
		}
	}

	// The row survives (soft delete) with a non-NULL deleted_at.
	rows, err := srv.conn.Query(context.Background(),
		`SELECT id, deleted_at FROM sessions WHERE id = 'sess-tomb'`)
	if err != nil {
		t.Fatalf("direct DB read: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("direct DB read: expected row to survive, got %d rows", len(rows))
	}
	if rows[0]["deleted_at"] == nil {
		t.Error("direct DB read: expected deleted_at to be set")
	}
}

// TestSoftDelete_SecondDeleteIsIdempotent200 proves DELETE is idempotent:
// the second DELETE on an already-deleted id returns 200 again, not 5xx.
func TestSoftDelete_SecondDeleteIsIdempotent200(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	seedSoftDeleteSession(t, srv, "sess-again", "thinking")

	if rec := deleteSessionViaAPI(srv, "sess-again"); rec.Code != http.StatusOK {
		t.Fatalf("first DELETE: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	rec := deleteSessionViaAPI(srv, "sess-again")
	if rec.Code != http.StatusOK {
		t.Fatalf("second DELETE: expected 200 (idempotent), got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"deleted"`) {
		t.Errorf("second DELETE body: expected status deleted, got %s", rec.Body.String())
	}

	// Idempotence does not resurrect the tombstone time — it stays set.
	rows, _ := srv.conn.Query(context.Background(),
		`SELECT deleted_at FROM sessions WHERE id = 'sess-again'`)
	if len(rows) != 1 || rows[0]["deleted_at"] == nil {
		t.Error("after second DELETE: expected deleted_at still set")
	}
}

// TestSoftDelete_MessageToDeletedSessionGone410 proves POST message on a
// deleted session returns 410 GONE and writes nothing to memory_events.
func TestSoftDelete_MessageToDeletedSessionGone410(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	seedSoftDeleteSession(t, srv, "sess-gone", "idle")

	if rec := deleteSessionViaAPI(srv, "sess-gone"); rec.Code != http.StatusOK {
		t.Fatalf("DELETE: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	before := countMemoryEventsFor(srv, "sess-gone")

	body := `{"content":"hello, are you there?"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/sess-gone/message", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusGone {
		t.Fatalf("POST message to deleted session: expected 410, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := countMemoryEventsFor(srv, "sess-gone"); got != before {
		t.Errorf("POST message to deleted session: memory_events count changed %d -> %d (no rows must be written)", before, got)
	}

	// A message to a never-existed id keeps the pre-existing 404 semantics.
	req404 := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/sess-never-existed/message", strings.NewReader(body))
	req404.Header.Set("Content-Type", "application/json")
	req404.Header.Set("Authorization", "Bearer "+srv.adminKey)
	rec404 := httptest.NewRecorder()
	srv.router.ServeHTTP(rec404, req404)
	if rec404.Code != http.StatusNotFound {
		t.Errorf("POST message to nonexistent session: expected 404 (unchanged behavior), got %d", rec404.Code)
	}
}

// TestSoftDelete_PatchOnDeletedSessionNotFound proves PATCH cannot resurrect
// or transition a tombstoned session (it is API-gone, 404 like get).
func TestSoftDelete_PatchOnDeletedSessionNotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	seedSoftDeleteSession(t, srv, "sess-patch-del", "idle")

	if rec := deleteSessionViaAPI(srv, "sess-patch-del"); rec.Code != http.StatusOK {
		t.Fatalf("DELETE: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/sessions/sess-patch-del", strings.NewReader(`{"status":"resume"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("PATCH on deleted session: expected 404, got %d: %s", rec.Code, rec.Body.String())
	}

	// And the tombstone was not disturbed by the attempt.
	rows, _ := srv.conn.Query(context.Background(),
		`SELECT deleted_at FROM sessions WHERE id = 'sess-patch-del'`)
	if len(rows) != 1 || rows[0]["deleted_at"] == nil {
		t.Error("after PATCH attempt: deleted_at must remain set")
	}
}

// TestSoftDelete_DeleteMissingIDMatchesExistingSemantics pins the existing
// no-op DELETE behavior for a nonexistent id (200 {"status":"deleted"}) —
// the route's historical semantics for missing ids, unchanged.
func TestSoftDelete_DeleteMissingIDMatchesExistingSemantics(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	rec := deleteSessionViaAPI(srv, "sess-never-was")
	if rec.Code != http.StatusOK {
		t.Errorf("DELETE missing id: expected 200 (existing no-op semantics), got %d: %s", rec.Code, rec.Body.String())
	}

	// Sanity: the smoke helper path — no row was created by the request.
	rows, _ := srv.conn.Query(context.Background(),
		`SELECT id FROM sessions WHERE id = 'sess-never-was'`)
	if len(rows) != 0 {
		t.Error("DELETE missing id must not create a row")
	}
}

// TestSoftDelete_ServiceLayerTombstone exercises SessionService directly:
// DeleteSession tombstones, GetSession reports not found, ListSessions omits.
func TestSoftDelete_ServiceLayerTombstone(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	seedSoftDeleteSession(t, srv, "sess-svc", "idle")

	if err := srv.svc.Sessions.DeleteSession(ctx, "sess-svc"); err != nil {
		t.Fatalf("service DeleteSession: %v", err)
	}
	if err := srv.svc.Sessions.DeleteSession(ctx, "sess-svc"); err != nil {
		t.Errorf("service DeleteSession idempotent: %v", err)
	}
	if _, err := srv.svc.Sessions.GetSession(ctx, "sess-svc"); err == nil {
		t.Error("service GetSession on deleted session: expected error")
	}
	list, err := srv.svc.Sessions.ListSessions(ctx, "", "", "admin")
	if err != nil {
		t.Fatalf("service ListSessions: %v", err)
	}
	for _, s := range list {
		if s.ID == "sess-svc" {
			t.Error("service ListSessions on deleted session: expected it to be omitted")
		}
	}
	st, _, err := srv.svc.Sessions.GetSessionStatus(ctx, "sess-svc")
	if err == nil {
		t.Errorf("service GetSessionStatus on deleted session: expected error, got status %q", st)
	}

	// time import guard: tombstone timestamp must be RFC3339-parseable.
	rows, _ := srv.conn.Query(ctx, `SELECT deleted_at FROM sessions WHERE id = 'sess-svc'`)
	if len(rows) == 1 {
		if ts, ok := rows[0]["deleted_at"].(string); ok && ts != "" {
			if _, err := time.Parse(time.RFC3339, ts); err != nil {
				t.Errorf("deleted_at not RFC3339: %q (%v)", ts, err)
			}
		}
	}
}
