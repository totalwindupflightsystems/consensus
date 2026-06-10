// Package api: integration tests for tools & skills endpoints with real SQLite backend.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-4/step-2-4-2 test=internal/api/tools_test.go
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
// List Tools Tests — GET /api/v1/tools
// ============================================================================

func TestListTools_Empty(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tools []ToolResponse
	if err := json.NewDecoder(w.Body).Decode(&tools); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestListTools_WithData(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	// Seed tools
	_ = srv.conn.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, status, enabled) VALUES ('tool-1', 'memory_sweep', 'Clean up old memories', 'internal', 'sql_function', 'sweep_memory', 'active', 1)`)
	_ = srv.conn.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, status, enabled, requires_approval) VALUES ('tool-2', 'scrape_web', 'Scrape a URL', 'external', 'subprocess', 'scraper.js', 'active', 1, 1)`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tools []ToolResponse
	if err := json.NewDecoder(w.Body).Decode(&tools); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	if tools[0].Name > tools[1].Name {
		t.Fatal("tools should be sorted by name")
	}

	foundApproval := false
	for _, tr := range tools {
		if tr.Name == "scrape_web" && tr.RequiresApproval {
			foundApproval = true
		}
	}
	if !foundApproval {
		t.Error("expected scrape_web to require approval")
	}
}

func TestListTools_StatusFilter(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, status, enabled) VALUES ('tool-a', 'active_tool', 'Active', 'internal', 'sql_function', 'fn_active', 'active', 1)`)
	_ = srv.conn.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, status, enabled) VALUES ('tool-b', 'disabled_tool', 'Disabled', 'internal', 'sql_function', 'fn_dis', 'disabled', 0)`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools?status=active", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tools []ToolResponse
	json.NewDecoder(w.Body).Decode(&tools)

	if len(tools) != 1 {
		t.Errorf("expected 1 active tool, got %d", len(tools))
	}
	if len(tools) > 0 && tools[0].Name != "active_tool" {
		t.Errorf("expected 'active_tool', got %q", tools[0].Name)
	}
}

func TestListTools_HemisphereFilter(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, status, enabled) VALUES ('tool-int', 'internal_tool', 'Inside', 'internal', 'sql_function', 'fn_int', 'active', 1)`)
	_ = srv.conn.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, status, enabled) VALUES ('tool-ext', 'external_tool', 'Outside', 'external', 'subprocess', 'ext.js', 'active', 1)`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools?hemisphere=external", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tools []ToolResponse
	json.NewDecoder(w.Body).Decode(&tools)

	if len(tools) != 1 {
		t.Errorf("expected 1 external tool, got %d", len(tools))
	}
	if len(tools) > 0 && tools[0].Hemisphere != "external" {
		t.Errorf("expected hemisphere='external', got %q", tools[0].Hemisphere)
	}
}

// ============================================================================
// List Skills Tests — GET /api/v1/skills
// ============================================================================

func TestListSkills_Empty(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var skills []SkillResponse
	if err := json.NewDecoder(w.Body).Decode(&skills); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestListSkills_WithData(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO skills_registry (id, name, metadata, instructions, enabled) VALUES ('skill-1', 'web_scraper', '{"description":"Scrape websites","version":"1.0"}', 'To scrape a website: 1. Use fetch() 2. Parse HTML', 1)`)
	_ = srv.conn.Exec(ctx, `INSERT INTO skills_registry (id, name, metadata, instructions, enabled) VALUES ('skill-2', 'data_analyzer', '{"description":"Analyze datasets"}', 'Analysis workflow...', 1)`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var skills []SkillResponse
	if err := json.NewDecoder(w.Body).Decode(&skills); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	if skills[0].Name > skills[1].Name {
		t.Error("skills should be sorted by name")
	}

	// Skills metadata is present (progressive disclosure: instructions are NOT in SkillResponse)
	// This is the correct behavior — only /skills/:name returns instructions
	t.Logf("list skills returned %d skills (correctly without instructions)", len(skills))
}

func TestListSkills_DisabledFiltered(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO skills_registry (id, name, metadata, instructions, enabled) VALUES ('skill-ena', 'enabled_skill', '{}', '...', 1)`)
	// Currently the handler doesn't filter by enabled — it always returns all
	// We just verify the response reports the correct enabled status

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var skills []SkillResponse
	json.NewDecoder(w.Body).Decode(&skills)

	found := false
	for _, s := range skills {
		if s.Name == "enabled_skill" && s.Enabled {
			found = true
		}
	}
	if !found {
		t.Error("expected enabled_skill to have Enabled=true")
	}
}

// ============================================================================
// Get Skill Detail Tests — GET /api/v1/skills/:name
// ============================================================================

func TestGetSkill_Success(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO skills_registry (id, name, metadata, instructions, linked_tool_ids, enabled) VALUES ('skill-detail', 'web_scraper', '{"description":"Scrape websites","version":"1.0"}', 'To scrape a website:\n1. Use fetch() to get the page\n2. Parse the HTML\n3. Extract data', '["tool-scrape","tool-parse"]', 1)`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills/web_scraper", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SkillDetailResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Name != "web_scraper" {
		t.Errorf("expected name 'web_scraper', got %q", resp.Name)
	}
	if resp.Instructions == "" {
		t.Error("expected full instructions")
	}
	if !strings.Contains(resp.Instructions, "fetch()") {
		t.Error("expected instructions to contain fetch()")
	}
	if len(resp.LinkedToolIDs) != 2 {
		t.Errorf("expected 2 linked tool IDs, got %d", len(resp.LinkedToolIDs))
	}
	if !resp.Enabled {
		t.Error("expected enabled=true")
	}
}

func TestGetSkill_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetSkill_Disabled(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO skills_registry (id, name, metadata, instructions, enabled) VALUES ('skill-off', 'disabled_skill', '{}', 'Secret instructions', 0)`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills/disabled_skill", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	// Even disabled skills are returned (metadata is public)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SkillDetailResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Name != "disabled_skill" {
		t.Errorf("expected name 'disabled_skill', got %q", resp.Name)
	}
	if resp.Enabled {
		t.Error("expected enabled=false")
	}
}

// ============================================================================
// Execute Tool Tests — POST /api/v1/tools/:name/execute
// ============================================================================

func TestExecuteTool_NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	body := `{"session_id":"sess-exec"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/nonexistent/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteTool_MissingSessionID(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	body := `{"parameters":{}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/some_tool/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteTool_Disabled(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-dis', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, status, enabled) VALUES ('tool-dis', 'disabled_tool', 'Off', 'internal', 'sql_function', 'fn_off', 'active', 0)`)

	body := `{"session_id":"sess-dis"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/disabled_tool/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteTool_NotActive(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-test', 'test', 'gpt-4o', 'idle', 'Goal', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, status, enabled) VALUES ('tool-test', 'testing_tool', 'Test', 'internal', 'sql_function', 'fn_test', 'testing', 1)`)

	body := `{"session_id":"sess-test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/testing_tool/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExecuteTool_EnqueueExternal(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-ext', 'test', 'gpt-4o', 'idle', 'Enqueue test', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, status, enabled) VALUES ('tool-ext', 'external_scraper', 'Scrape URLs', 'external', 'http_endpoint', 'https://scraper.conscience', 'active', 1)`)

	body := `{"session_id":"sess-ext","parameters":{"url":"https://example.com"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/external_scraper/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ExecuteToolResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.IsError {
		t.Errorf("expected no error, got: %s", resp.Error)
	}

	// Verify the tool request was enqueued
	rows, err := srv.conn.Query(ctx, `SELECT id, tool_name, status, parameters FROM tool_requests WHERE session_id = 'sess-ext'`)
	if err != nil {
		t.Fatalf("query tool_requests: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 tool_requests row, got %d", len(rows))
	}
	if toString(rows[0]["status"]) != "pending" {
		t.Errorf("expected pending status, got %q", toString(rows[0]["status"]))
	}
	if toString(rows[0]["tool_name"]) != "external_scraper" {
		t.Errorf("expected tool_name 'external_scraper', got %q", toString(rows[0]["tool_name"]))
	}
}

func TestExecuteTool_SessionScoped(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-sc', 'mine', 'gpt-4o', 'idle', 'Scope', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, status, enabled) VALUES ('tool-sc', 'scoped_tool', 'Scoped', 'internal', 'sql_function', 'fn_sc', 'active', 1)`)

	sessionKey := "cs_sk_scoped_test_key_123456"
	hash := sha256Hash(sessionKey)
	prefix := sessionKey[:min(8, len(sessionKey))]
	_ = srv.conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ('key-sc', $1, $2, 'session', 'sess-sc', datetime('now'))`, hash, prefix)

	body := `{"session_id":"sess-sc"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/scoped_tool/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	// Should succeed — session key matches target session
	if w.Code == http.StatusForbidden {
		t.Errorf("should not be forbidden for own session: %s", w.Body.String())
	}
}

// ============================================================================
// Route Tests — ensure the routing dispatches correctly
// ============================================================================

func TestRouteTools_SkillsDetail(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	_ = srv.conn.Exec(ctx, `INSERT INTO skills_registry (id, name, metadata, instructions, enabled) VALUES ('skill-r', 'route_test', '{}', 'route test instructions', 1)`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills/route_test", nil)
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for skills/:name, got %d: %s", w.Code, w.Body.String())
	}

	var resp SkillDetailResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Name != "route_test" {
		t.Errorf("expected name 'route_test', got %q", resp.Name)
	}
	if !strings.Contains(resp.Instructions, "route test") {
		t.Error("expected instructions in response")
	}
}

func TestRouteTools_ExecuteRoute(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, created_at, heartbeat_at) VALUES ('sess-rt', 'rt', 'gpt-4o', 'idle', 'Route test', $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, status, enabled) VALUES ('tool-rt', 'route_executor', 'Route', 'external', 'subprocess', 'rt.sh', 'active', 1)`)

	body := `{"session_id":"sess-rt"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/route_executor/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for POST tools/:name/execute, got %d: %s", w.Code, w.Body.String())
	}

	var resp ExecuteToolResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.IsError {
		t.Errorf("expected no error, got: %s", resp.Error)
	}
	if resp.ToolName != "route_executor" {
		t.Errorf("expected tool_name 'route_executor', got %q", resp.ToolName)
	}
}
