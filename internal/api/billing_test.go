// Package api: integration tests for billing, config, metrics, and auth endpoints with real SQLite backend.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-6/step-2-6-2 test=internal/api/billing_test.go
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
// GET /api/v1/sessions/{id}/billing — Per-session billing
// ============================================================================

func TestGetSessionBilling_Empty(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-bill1', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-bill1/billing", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp["session_id"] != "sess-bill1" {
		t.Errorf("expected session_id 'sess-bill1', got %v", resp["session_id"])
	}
	if cost, ok := resp["total_cost_usd"].(float64); !ok || cost != 0.0 {
		t.Errorf("expected total_cost_usd 0, got %v", resp["total_cost_usd"])
	}

	entries, ok := resp["entries"].([]any)
	if !ok || len(entries) != 0 {
		t.Errorf("expected 0 billing entries, got %d", len(entries))
	}
}

func TestGetSessionBilling_WithData(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-bill2', 'test', 'gpt-4o', 'completed', 'Goal', $1, $1)`, now)

	// Insert billing entries
	_ = srv.conn.Exec(ctx, `INSERT INTO agent_billing (session_id, iteration, model_id, category, prompt_tokens, completion_tokens, cost_usd, recorded_at) VALUES ('sess-bill2', 1, 'gpt-4o', 'cognition', 1000, 500, 0.0125, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO agent_billing (session_id, iteration, model_id, category, prompt_tokens, completion_tokens, cost_usd, recorded_at) VALUES ('sess-bill2', 1, 'gpt-4o', 'compression', 200, 100, 0.0020, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO agent_billing (session_id, iteration, model_id, category, prompt_tokens, completion_tokens, cost_usd, recorded_at) VALUES ('sess-bill2', 2, 'gpt-4o', 'cognition', 800, 300, 0.0075, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-bill2/billing", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Total cost: 0.0125 + 0.0020 + 0.0075 = 0.022
	if cost, ok := resp["total_cost_usd"].(float64); !ok || cost != 0.022 {
		t.Errorf("expected total_cost_usd 0.022, got %v", resp["total_cost_usd"])
	}

	// Total prompt tokens: 1000 + 200 + 800 = 2000
	if totalPrompt, ok := resp["total_prompt_tokens"].(float64); !ok || totalPrompt != 2000 {
		t.Errorf("expected total_prompt_tokens 2000, got %v", resp["total_prompt_tokens"])
	}

	entries, ok := resp["entries"].([]any)
	if !ok || len(entries) != 3 {
		t.Fatalf("expected 3 billing entries, got %d", len(entries))
	}

	// First entry should have iteration 1
	entry0 := entries[0].(map[string]any)
	if entry0["iteration"] != float64(1) {
		t.Errorf("first entry expected iteration 1, got %v", entry0["iteration"])
	}
	if entry0["category"] != "cognition" {
		t.Errorf("first entry expected category 'cognition', got %v", entry0["category"])
	}
}

func TestGetSessionBilling_Unauthorized(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-bill3', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	// Create a session-scoped key for another session
	otherKey := "cs_sk_other_1234567890_abcdef"
	otherHash := sha256Hash(otherKey)
	otherPrefix := "cs_sk_ot"
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-other', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-other', $1, $2, 'session', 'sess-other', $3)`, otherHash, otherPrefix, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-bill3/billing", nil)
	req.Header.Set("Authorization", "Bearer "+otherKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 FORBIDDEN, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetSessionBilling_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/nonexistent/billing", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetSessionBilling_ReadonlyAllowed(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-bill-r', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)

	// Create a readonly key
	roKey := "cs_rk_readonly_test_1234567890xyz"
	roHash := sha256Hash(roKey)
	roPrefix := "cs_rk_re"
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('key-ro', $1, $2, 'readonly', $3)`, roHash, roPrefix, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-bill-r/billing", nil)
	req.Header.Set("Authorization", "Bearer "+roKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Readonly can access billing
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (readonly allowed), got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// GET /api/v1/config — System Configuration (admin only)
// ============================================================================

func TestGetConfig_Empty(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var cfg ConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Empty config — system_settings may be nil if empty (omitempty JSON tag)
	// This is acceptable behavior for an empty system
	if cfg.SystemSettings == nil {
		// OK — omitempty means empty map not serialized
	} else if len(cfg.SystemSettings) != 0 {
		t.Errorf("expected empty system_settings, got %v", cfg.SystemSettings)
	}
}

func TestGetConfig_WithSettings(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO system_settings (key, value) VALUES ('llm.default_model', 'gpt-4o')`)
	_ = srv.conn.Exec(ctx, `INSERT INTO system_settings (key, value) VALUES ('llm.provider', 'openai')`)
	_ = srv.conn.Exec(ctx, `INSERT INTO system_settings (key, value) VALUES ('harness.heartbeat_interval_seconds', '5')`)
	_ = srv.conn.Exec(ctx, `INSERT INTO system_settings (key, value) VALUES ('app.title', 'Consensus')`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var cfg ConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Check structured fields
	if llm, ok := cfg.LLM.(map[string]any); ok {
		if llm["default_model"] != "gpt-4o" {
			t.Errorf("expected llm.default_model 'gpt-4o', got %v", llm["default_model"])
		}
		if llm["provider"] != "openai" {
			t.Errorf("expected llm.provider 'openai', got %v", llm["provider"])
		}
	} else {
		t.Error("LLM config should be a map")
	}

	if harness, ok := cfg.Harness.(map[string]any); ok {
		if harness["heartbeat_interval_seconds"] != "5" {
			t.Errorf("expected harness.heartbeat_interval_seconds '5', got %v", harness["heartbeat_interval_seconds"])
		}
	}

	// Uncategorized should be in system_settings
	if cfg.SystemSettings != nil {
		if cfg.SystemSettings["app.title"] != "Consensus" {
			t.Errorf("expected app.title in system_settings, got %v", cfg.SystemSettings)
		}
	}
}

func TestGetConfig_NotAdmin(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	// Create a session-scoped key
	sessKey := "cs_sk_cfg_test_1234567890_abcd"
	sessHash := sha256Hash(sessKey)
	sessPrefix := "cs_sk_cf"
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-cfg', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-cfg-s', $1, $2, 'session', 'sess-cfg', $3)`, sessHash, sessPrefix, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req.Header.Set("Authorization", "Bearer "+sessKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// GET /api/v1/metrics — System Metrics (admin/readonly)
// ============================================================================

func TestGetMetrics_Empty(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var metrics MetricsResponse
	if err := json.NewDecoder(w.Body).Decode(&metrics); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// No sessions created by newIntegrationServer (only the admin key exists)
	// All counts should be 0
	if metrics.TotalSessions != 0 {
		t.Errorf("expected 0 total sessions, got %d", metrics.TotalSessions)
	}
	if metrics.ActiveSessions != 0 {
		t.Errorf("expected 0 active sessions, got %d", metrics.ActiveSessions)
	}
}

func TestGetMetrics_WithData(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	// Create sessions with various states
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-m1', 'agent1', 'gpt-4o', 'thinking', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-m2', 'agent2', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-m3', 'agent3', 'gpt-4o', 'completed', 'Goal', $1, $1)`, now)

	// Create tasks
	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, status, created_at) VALUES ('task-m1', 'sess-m1', 'Task 1', 'pending', $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tasks (id, session_id, title, status, created_at) VALUES ('task-m2', 'sess-m1', 'Task 2', 'in_progress', $1)`, now)

	// Create approval
	_ = srv.conn.Exec(ctx, `INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, status, created_at) VALUES ('ap-m1', 'sess-m1', 1, 'tool_execution', 'Review', 'medium', 'pending', $1)`, now)

	// Insert billing
	_ = srv.conn.Exec(ctx, `INSERT INTO agent_billing (session_id, iteration, model_id, category, cost_usd, recorded_at) VALUES ('sess-m1', 1, 'gpt-4o', 'cognition', 0.50, $1)`, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var metrics MetricsResponse
	if err := json.NewDecoder(w.Body).Decode(&metrics); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if metrics.ActiveSessions != 2 {
		t.Errorf("expected 2 active sessions, got %d", metrics.ActiveSessions)
	}
	if metrics.PendingTasks != 1 {
		t.Errorf("expected 1 pending task, got %d", metrics.PendingTasks)
	}
	if metrics.PendingApprovals != 1 {
		t.Errorf("expected 1 pending approval, got %d", metrics.PendingApprovals)
	}
	if metrics.TotalSessions != 3 {
		t.Errorf("expected 3 total sessions, got %d", metrics.TotalSessions)
	}
	if metrics.TotalCostUSD != 0.5 {
		t.Errorf("expected total_cost_usd 0.5, got %v", metrics.TotalCostUSD)
	}
}

func TestGetMetrics_Readonly(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	// Create a readonly key
	roKey := "cs_rk_metrics_test_1234567890_mmm"
	roHash := sha256Hash(roKey)
	roPrefix := "cs_rk_me"
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('key-ro-m', $1, $2, 'readonly', $3)`, roHash, roPrefix, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+roKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	// Readonly can access metrics
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (readonly allowed), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetMetrics_SessionDenied(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	// Create a session-scoped key
	sessKey := "cs_sk_metr_test_1234567890_nope"
	sessHash := sha256Hash(sessKey)
	sessPrefix := "cs_sk_me"
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-met', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-smt', $1, $2, 'session', 'sess-met', $3)`, sessHash, sessPrefix, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+sessKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// POST /api/v1/auth/keys — Create API key (admin only)
// ============================================================================

func TestCreateAPIKey_Admin(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	body := strings.NewReader(`{"scope": "session"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/keys", body)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp["scope"] != "session" {
		t.Errorf("expected scope 'session', got %v", resp["scope"])
	}
	if apiKey, ok := resp["api_key"].(string); !ok || apiKey == "" {
		t.Error("expected non-empty api_key")
	}
	if prefix, ok := resp["key_prefix"].(string); !ok || prefix == "" {
		t.Error("expected non-empty key_prefix")
	}
	if id, ok := resp["id"].(string); !ok || id == "" {
		t.Error("expected non-empty id")
	}
}

func TestCreateAPIKey_BadScope(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	body := strings.NewReader(`{"scope": "invalid_scope"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/keys", body)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAPIKey_Unauthorized(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	// Session-scoped key
	sessKey := "cs_sk_auth_create_1234567890_n"
	sessHash := sha256Hash(sessKey)
	sessPrefix := "cs_sk_au"
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-ak', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-aku', $1, $2, 'session', 'sess-ak', $3)`, sessHash, sessPrefix, now)

	body := strings.NewReader(`{"scope": "session"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/keys", body)
	req.Header.Set("Authorization", "Bearer "+sessKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAPIKey_WithExpiration(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	body := strings.NewReader(`{"scope": "readonly", "expires_in": 3600}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/keys", body)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if scope, ok := resp["scope"].(string); !ok || scope != "readonly" {
		t.Errorf("expected scope 'readonly', got %v", resp["scope"])
	}
}

// ============================================================================
// GET /api/v1/auth/keys — List API keys (admin only)
// ============================================================================

func TestListAPIKeys_Admin(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/keys", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var keys []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&keys); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// At least the admin key itself
	if len(keys) < 1 {
		t.Errorf("expected at least 1 key, got %d", len(keys))
	}

	// Check that no key_hash is exposed
	for _, k := range keys {
		if _, hasHash := k["key_hash"]; hasHash {
			t.Error("key_hash should not be exposed in API key listing")
		}
		if _, hasFullKey := k["api_key"]; hasFullKey {
			t.Error("api_key should not be exposed in API key listing")
		}
	}
}

func TestListAPIKeys_Unauthorized(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	roKey := "cs_rk_list_keys_1234567890_xyz"
	roHash := sha256Hash(roKey)
	roPrefix := "cs_rk_li"
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('key-rol', $1, $2, 'readonly', $3)`, roHash, roPrefix, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/keys", nil)
	req.Header.Set("Authorization", "Bearer "+roKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// DELETE /api/v1/auth/keys/{id} — Revoke an API key (admin only)
// ============================================================================

func TestDeleteAPIKey_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	// Create a key to revoke
	tmpKey := "cs_sk_todelete_1234567890_del"
	tmpHash := sha256Hash(tmpKey)
	tmpPrefix := "cs_sk_to"
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('key-to-del', $1, $2, 'session', $3)`, tmpHash, tmpPrefix, now)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/keys/key-to-del", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp["status"] != "revoked" {
		t.Errorf("expected status 'revoked', got %v", resp["status"])
	}

	// Verify the key is now expired
	rows, err := srv.conn.Query(ctx, `SELECT expires_at FROM api_keys WHERE id = 'key-to-del' AND expires_at IS NOT NULL`)
	if err != nil || len(rows) == 0 {
		t.Error("expected key to have expires_at set")
	}
}

func TestDeleteAPIKey_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/keys/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteAPIKey_Unauthorized(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	sessKey := "cs_sk_del_unauth_1234567890_n"
	sessHash := sha256Hash(sessKey)
	sessPrefix := "cs_sk_de"
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-du', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-du', $1, $2, 'session', 'sess-du', $3)`, sessHash, sessPrefix, now)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/keys/key-du", nil)
	req.Header.Set("Authorization", "Bearer "+sessKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ============================================================================
// GET /api/v1/health — Health check (no auth required)
// ============================================================================

func TestHealthCheck(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	// No auth header — should still work
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %v", resp["status"])
	}
}
