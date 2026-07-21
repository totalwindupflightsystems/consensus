// Package api: server endpoint and middleware tests (SPEC-015).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-1/task-1-5/step-1-5-1 test=internal/api/server_test.go
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// Test Harness: mock DB for API tests
// ============================================================================

type mockAPIDB struct {
	queryResults []db.Row
	queryErr     error
	execErr      error
	queries      []string // recorded queries
}

func (m *mockAPIDB) BeginTx(ctx context.Context) (db.Tx, error) { return nil, nil }
func (m *mockAPIDB) Exec(ctx context.Context, query string, args ...any) error {
	m.queries = append(m.queries, query)
	return m.execErr
}
func (m *mockAPIDB) Query(ctx context.Context, query string, args ...any) ([]db.Row, error) {
	m.queries = append(m.queries, query)
	return m.queryResults, m.queryErr
}
func (m *mockAPIDB) QueryRow(ctx context.Context, query string, args ...any) (db.Row, error) {
	rows, err := m.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}
func (m *mockAPIDB) Backend() db.Backend { return db.BackendSQLite }
func (m *mockAPIDB) Close() error { return nil }

// ============================================================================
// Health Endpoint Tests
// ============================================================================

func TestHealthEndpoint_ReturnsOK(t *testing.T) {
	srv := NewServer(ServerConfig{Addr: ":0", DB: &mockAPIDB{}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "ok") {
		t.Errorf("expected 'healthy' in body, got %q", body)
	}
}

// TestHealthEndpoint_UsesAdminPool verifies the health endpoint runs its
// DB queries through the AdminDB handle rather than the main DB. This is
// the INFRA-7 regression test: in Docker the main pool (SET ROLE
// agent_role) can hang, so health checks must use the admin pool that
// bypasses RLS. We assert this by giving the two pools distinct query
// recorders and checking that only the admin recorder was touched.
func TestHealthEndpoint_UsesAdminPool(t *testing.T) {
	mainDB := &mockAPIDB{}
	adminDB := &mockAPIDB{}
	srv := NewServer(ServerConfig{Addr: ":0", DB: mainDB, AdminDB: adminDB})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if len(mainDB.queries) != 0 {
		t.Errorf("health endpoint should NOT use the main pool, but it ran %d query/queries: %v",
			len(mainDB.queries), mainDB.queries)
	}
	if len(adminDB.queries) == 0 {
		t.Error("health endpoint should use the admin pool, but admin pool recorded no queries")
	}
}

// TestHealthEndpoint_NilAdminDB_FallsBack verifies backward compatibility:
// when AdminDB is not configured, health checks fall back to the main DB
// instead of crashing on a nil dereference.
func TestHealthEndpoint_NilAdminDB_FallsBack(t *testing.T) {
	mainDB := &mockAPIDB{}
	srv := NewServer(ServerConfig{Addr: ":0", DB: mainDB}) // AdminDB intentionally nil

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(mainDB.queries) == 0 {
		t.Error("with nil AdminDB, health endpoint should fall back to main DB, but no queries were recorded")
	}
}

// ============================================================================
// Auth Middleware Tests
// ============================================================================

func TestAuthMiddleware_MissingKey_Returns401(t *testing.T) {
	mock := &mockAPIDB{
		queryResults: []db.Row{},
	}
	srv := NewServer(ServerConfig{Addr: ":0", DB: mock})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "UNAUTHENTICATED") {
		t.Errorf("expected UNAUTHENTICATED in body")
	}
}

func TestAuthMiddleware_InvalidKey_Returns401(t *testing.T) {
	mock := &mockAPIDB{
		queryResults: []db.Row{},
	}
	srv := NewServer(ServerConfig{Addr: ":0", DB: mock})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer invalid-key")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidAdminKey_Passes(t *testing.T) {
	mock := &mockAPIDB{
		queryResults: []db.Row{
			{
				"id":         "key-1",
				"scope":      "admin",
				"session_id": nil,
			},
		},
	}
	srv := NewServer(ServerConfig{Addr: ":0", DB: mock})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	// Use a known key hash pairing: prefix "test-key" → sha256 = specific hash
	req.Header.Set("Authorization", "Bearer test-key-12345")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	// The key prefix is first 8 chars: "test-key" → "test-key"
	// hash lookup will match mock queryResults, scope = admin
	// Should pass auth and return 501 (endpoint not implemented, not 401)
	if w.Code == http.StatusUnauthorized {
		t.Errorf("should not be unauthorized, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthMiddleware_ExpiredKey_Returns401(t *testing.T) {
	mock := &mockAPIDB{
		queryResults: []db.Row{}, // empty = no valid key found (expired)
	}
	srv := NewServer(ServerConfig{Addr: ":0", DB: mock})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer expired-key-xx")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ============================================================================
// Rate Limiting Tests (SPEC-015 §7)
// ============================================================================

// rateLimitKey returns a callbackMock that returns the given rate limit data
// for api_rate_limits queries and auth data for api_keys queries.
func rateLimitKey(scope string, count int64, windowStart string) *callbackMock {
	return &callbackMock{
		queryFn: func(ctx context.Context, query string, args ...any) ([]db.Row, error) {
			if strings.Contains(query, "api_keys") {
				return []db.Row{{"id": "key-1", "scope": scope, "session_id": nil}}, nil
			}
			if strings.Contains(query, "api_rate_limits") {
				return []db.Row{{"requests_count": count, "window_start": windowStart}}, nil
			}
			return nil, nil
		},
	}
}

// rateLimitFreshKey returns a callbackMock that returns auth data but no rate limit data
// (simulates a first request with no existing rate limit counter).
func rateLimitFreshKey(scope string) *callbackMock {
	return &callbackMock{
		queryFn: func(ctx context.Context, query string, args ...any) ([]db.Row, error) {
			if strings.Contains(query, "api_keys") {
				return []db.Row{{"id": "key-1", "scope": scope, "session_id": nil}}, nil
			}
			if strings.Contains(query, "api_rate_limits") {
				return []db.Row{}, nil // no existing counter
			}
			return nil, nil
		},
	}
}

func TestRateLimit_UnderLimit_Passes(t *testing.T) {
	srv := NewServer(ServerConfig{Addr: ":0", DB: &mockAPIDB{
		queryResults: []db.Row{
			{"id": "key-1", "scope": "admin", "session_id": nil},
		},
	}})

	// First request — rate limit: no existing counter, inserts new one
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer rate-limit-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code == http.StatusTooManyRequests {
		t.Errorf("should not be rate limited on first request")
	}
}

func TestRateLimit_OverLimit_Returns429(t *testing.T) {
	// Pre-populate rate limit data showing max count (admin = 1000)
	srv := NewServer(ServerConfig{Addr: ":0", DB: &mockAPIDB{
		queryResults: []db.Row{
			{"id": "key-1", "scope": "admin", "session_id": nil},
		},
	}})

	nowish := time.Now().Add(-30 * time.Second).Format(time.RFC3339)
	customMock := &callbackMock{
		queryFn: func(ctx context.Context, query string, args ...any) ([]db.Row, error) {
			if strings.Contains(query, "api_keys") {
				return []db.Row{{"id": "key-1", "scope": "admin", "session_id": nil}}, nil
			}
			if strings.Contains(query, "api_rate_limits") {
				return []db.Row{{"requests_count": int64(1000), "window_start": nowish}}, nil
			}
			return nil, nil
		},
	}
	origDB := srv.db
	srv.db = customMock
	defer func() { srv.db = origDB }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer over-limit-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// TestRateLimit_AdminScope_AtBoundary: admin at 999/1000 passes, 1000/1000 blocks
func TestRateLimit_AdminScope_AtBoundary(t *testing.T) {
	nowish := time.Now().Add(-30 * time.Second).Format(time.RFC3339)

	// 999 requests — should pass (under admin limit of 1000)
	srv := NewServer(ServerConfig{Addr: ":0", DB: rateLimitKey("admin", 999, nowish)})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer admin-key-999")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Errorf("admin at 999/1000 should pass, got 429")
	}

	// 1000 requests — should be blocked (at admin limit of 1000)
	srv2 := NewServer(ServerConfig{Addr: ":0", DB: rateLimitKey("admin", 1000, nowish)})
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req2.Header.Set("Authorization", "Bearer admin-key-1000")
	w2 := httptest.NewRecorder()
	srv2.router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("admin at 1000/1000 should be blocked, got %d", w2.Code)
	}
}

// TestRateLimit_SessionScope_AtBoundary: session at 99/100 passes, 100/100 blocks
func TestRateLimit_SessionScope_AtBoundary(t *testing.T) {
	nowish := time.Now().Add(-30 * time.Second).Format(time.RFC3339)

	// 99 requests — should pass (under session limit of 100)
	srv := NewServer(ServerConfig{Addr: ":0", DB: rateLimitKey("session", 99, nowish)})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer session-key-99")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Errorf("session at 99/100 should pass, got 429")
	}

	// 100 requests — should be blocked
	srv2 := NewServer(ServerConfig{Addr: ":0", DB: rateLimitKey("session", 100, nowish)})
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req2.Header.Set("Authorization", "Bearer session-key-100")
	w2 := httptest.NewRecorder()
	srv2.router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("session at 100/100 should be blocked, got %d", w2.Code)
	}
}

// TestRateLimit_ReadonlyScope_AtBoundary: readonly at 199/200 passes, 200/200 blocks
func TestRateLimit_ReadonlyScope_AtBoundary(t *testing.T) {
	nowish := time.Now().Add(-30 * time.Second).Format(time.RFC3339)

	// 199 requests — should pass (under readonly limit of 200)
	srv := NewServer(ServerConfig{Addr: ":0", DB: rateLimitKey("readonly", 199, nowish)})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer readonly-key-199")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Errorf("readonly at 199/200 should pass, got 429")
	}

	// 200 requests — should be blocked
	srv2 := NewServer(ServerConfig{Addr: ":0", DB: rateLimitKey("readonly", 200, nowish)})
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req2.Header.Set("Authorization", "Bearer readonly-key-200")
	w2 := httptest.NewRecorder()
	srv2.router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("readonly at 200/200 should be blocked, got %d", w2.Code)
	}
}

// TestRateLimit_WebhookScope_AtBoundary: webhook at 499/500 passes, 500/500 blocks
func TestRateLimit_WebhookScope_AtBoundary(t *testing.T) {
	nowish := time.Now().Add(-30 * time.Second).Format(time.RFC3339)

	// 499 requests — should pass (under webhook limit of 500)
	srv := NewServer(ServerConfig{Addr: ":0", DB: rateLimitKey("webhook", 499, nowish)})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer webhook-key-499")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Errorf("webhook at 499/500 should pass, got 429")
	}

	// 500 requests — should be blocked
	srv2 := NewServer(ServerConfig{Addr: ":0", DB: rateLimitKey("webhook", 500, nowish)})
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req2.Header.Set("Authorization", "Bearer webhook-key-500")
	w2 := httptest.NewRecorder()
	srv2.router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("webhook at 500/500 should be blocked, got %d", w2.Code)
	}
}

// TestRateLimit_UnknownScope_FallsBack: unknown scope falls back to session default (100)
func TestRateLimit_UnknownScope_FallsBack(t *testing.T) {
	nowish := time.Now().Add(-30 * time.Second).Format(time.RFC3339)

	// 99 requests — should pass (session default of 100)
	srv := NewServer(ServerConfig{Addr: ":0", DB: rateLimitKey("custom", 99, nowish)})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer custom-key-99")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Errorf("unknown scope at 99 should pass (defaults to 100), got 429")
	}

	// 100 requests — should be blocked
	srv2 := NewServer(ServerConfig{Addr: ":0", DB: rateLimitKey("custom", 100, nowish)})
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req2.Header.Set("Authorization", "Bearer custom-key-100")
	w2 := httptest.NewRecorder()
	srv2.router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("unknown scope at 100 should be blocked (defaults to 100), got %d", w2.Code)
	}
}

// TestRateLimit_ConfigOverride: config overrides take effect
func TestRateLimit_ConfigOverride(t *testing.T) {
	nowish := time.Now().Add(-30 * time.Second).Format(time.RFC3339)

	// Override session to 50 — 49/50 passes
	srv := NewServer(ServerConfig{
		Addr:        ":0",
		DB:          rateLimitKey("session", 49, nowish),
		SessionRate: 50,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer session-conf-49")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Errorf("session with config 50 at 49/50 should pass, got 429")
	}

	// 50/50 should block
	srv2 := NewServer(ServerConfig{
		Addr:        ":0",
		DB:          rateLimitKey("session", 50, nowish),
		SessionRate: 50,
	})
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req2.Header.Set("Authorization", "Bearer session-conf-50")
	w2 := httptest.NewRecorder()
	srv2.router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("session with config 50 at 50/50 should be blocked, got %d", w2.Code)
	}
}

// TestRateLimit_ZeroConfig_UsesDefault: zero values in config use package defaults
func TestRateLimit_ZeroConfig_UsesDefault(t *testing.T) {
	nowish := time.Now().Add(-30 * time.Second).Format(time.RFC3339)

	// All zero config — admin should still default to 1000
	srv := NewServer(ServerConfig{
		Addr: ":0",
		DB:   rateLimitKey("admin", 999, nowish),
		// All rate fields = 0, defaults apply
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer zero-conf-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Errorf("zero config admin at 999 should pass (defaults to 1000), got 429")
	}
}

// TestRateLimit_WindowReset: after window expiry, counter resets
func TestRateLimit_WindowReset(t *testing.T) {
	// Window started 61 seconds ago — should be reset
	oldWindow := time.Now().Add(-61 * time.Second).Format(time.RFC3339)

	// Even though count is at limit (1000), old window means reset
	mock := &callbackMock{
		queryFn: func(ctx context.Context, query string, args ...any) ([]db.Row, error) {
			if strings.Contains(query, "api_keys") {
				return []db.Row{{"id": "key-1", "scope": "admin", "session_id": nil}}, nil
			}
			if strings.Contains(query, "api_rate_limits") {
				return []db.Row{{"requests_count": int64(1000), "window_start": oldWindow}}, nil
			}
			return nil, nil
		},
	}
	srv := NewServer(ServerConfig{Addr: ":0", DB: mock})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer reset-key")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Errorf("window expired — should reset counter and pass, got 429")
	}
}

// ============================================================================
// EventBus Tests
// ============================================================================

func TestEventBus_SubscribeAndPublish(t *testing.T) {
	eb := NewEventBus()
	id, ch := eb.Subscribe("session-1")
	defer eb.Unsubscribe("session-1", id)

	eb.Publish("session-1", Event{
		SessionID: "session-1",
		Type:      "session_update",
		Data:      SessionUpdateEvent{Status: "thinking", Iteration: 1},
	})

	select {
	case event := <-ch:
		if event.Type != "session_update" {
			t.Errorf("expected session_update, got %q", event.Type)
		}
		if event.SessionID != "session-1" {
			t.Errorf("expected session-1, got %q", event.SessionID)
		}
		// Don't try to extract Data; the JSON marshal makes it a map
		t.Logf("event received: type=%s session=%s", event.Type, event.SessionID)
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for event")
	}
}

func TestEventBus_GlobalSubscriber(t *testing.T) {
	eb := NewEventBus()
	id, ch := eb.Subscribe("") // global
	defer eb.Unsubscribe("", id)

	eb.Publish("session-2", Event{
		SessionID: "session-2",
		Type:      "approval_pending",
	})

	select {
	case event := <-ch:
		if event.Type != "approval_pending" {
			t.Errorf("expected approval_pending, got %q", event.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for global event")
	}
}

func TestEventBus_Unsubscribe_NoMoreEvents(t *testing.T) {
	eb := NewEventBus()
	id, ch := eb.Subscribe("session-3")

	eb.Publish("session-3", Event{Type: "test"})
	<-ch // consume first

	eb.Unsubscribe("session-3", id)
	eb.Publish("session-3", Event{Type: "test"})

	select {
	case <-ch:
		t.Error("should not receive event after unsubscribe")
	default:
		// expected
	}
}

// ============================================================================
// SSE Integration Test
// ============================================================================

func TestSSEHandler_StreamsEvents(t *testing.T) {
	srv := NewServer(ServerConfig{Addr: ":0", DB: &mockAPIDB{}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream?sessions_id=test-session", nil)
	w := httptest.NewRecorder()

	// Run SSE in a goroutine and publish events
	done := make(chan struct{})
	go func() {
		srv.HandleSSE(w, req)
		close(done)
	}()

	// Wait a moment for the initial connection event
	time.Sleep(50 * time.Millisecond)

	srv.events.Publish("test-session", Event{
		SessionID: "test-session",
		Type:      "session_update",
		Data:      SessionUpdateEvent{Status: "thinking", Iteration: 1},
	})

	// Wait for the event to be delivered
	time.Sleep(50 * time.Millisecond)

	// Cancel context to stop the SSE goroutine
	// In httptest, we can't easily cancel — just check we got data
	if w.Body.Len() == 0 {
		t.Error("SSE handler wrote no data")
	}

	t.Logf("SSE output: %s", w.Body.String())
}

// ============================================================================
// CORS Middleware Tests
// ============================================================================

func TestCORS_OptionsAllowed(t *testing.T) {
	mock := &mockAPIDB{
		queryResults: []db.Row{
			{"id": "key-1", "scope": "admin", "session_id": nil},
		},
	}
	srv := NewServer(ServerConfig{Addr: ":0", DB: mock})

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	srv.allowCORSMiddleware(srv.router).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
}

// ============================================================================
// Context Key Tests
// ============================================================================

func TestGetAuthScope(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), ctxKeyScope, "admin")
	req = req.WithContext(ctx)

	scope := GetAuthScope(req)
	if scope != "admin" {
		t.Errorf("expected admin, got %q", scope)
	}
}

func TestGetAuthScope_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	scope := GetAuthScope(req)
	if scope != "" {
		t.Errorf("expected empty, got %q", scope)
	}
}

// ============================================================================
// Mock DB with callback support
// ============================================================================

type callbackMock struct {
	queryFn func(context.Context, string, ...any) ([]db.Row, error)
}

func (c *callbackMock) BeginTx(ctx context.Context) (db.Tx, error) { return nil, nil }
func (c *callbackMock) Exec(ctx context.Context, query string, args ...any) error {
	return nil
}
func (c *callbackMock) Query(ctx context.Context, query string, args ...any) ([]db.Row, error) {
	if c.queryFn != nil {
		return c.queryFn(ctx, query, args...)
	}
	return nil, nil
}
func (c *callbackMock) QueryRow(ctx context.Context, query string, args ...any) (db.Row, error) {
	rows, _ := c.Query(ctx, query, args...)
	if len(rows) > 0 {
		return rows[0], nil
	}
	return nil, nil
}
func (c *callbackMock) Backend() db.Backend { return db.BackendSQLite }
func (c *callbackMock) Close() error { return nil }
