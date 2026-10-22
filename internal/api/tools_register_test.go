// Package api: POST /api/v1/tools — tool registration (DF-CONSENSUS-62).
//
// The dogfood finding was that no user-facing surface could create the
// tools_registry row the approval gate fires on. These tests pin the on-ramp:
//
//   - an admin key registers a tool and gets the created row back (201)
//   - the tool immediately appears in GET /api/v1/tools
//   - requires_approval round-trips as a real registry column (the gate input)
//   - defaults match the column defaults (internal / sql_function / active)
//   - duplicate names are 409, bad enums and missing fields are 400
//   - non-admin keys are 403 (registration defines dispatchable capability)
//
// test=internal/api/tools_register_test.go
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

// registerTool posts a registration body with the given API key and returns
// the recorder.
func registerTool(t *testing.T, srv *integrationServer, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	return w
}

func TestCreateTool_RegistersAndLists(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	w := registerTool(t, srv, srv.adminKey,
		`{"name":"gated_scrape","description":"Scrape a URL for the agent",`+
			`"hemisphere":"external","handler_type":"http_endpoint",`+
			`"handler_ref":"https://scraper.consensus/fetch",`+
			`"requires_approval":true,"parameter_schema":{"type":"object","properties":{"url":{"type":"string"}}}}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created ToolResponse
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == "" {
		t.Error("created tool has no id")
	}
	if created.Name != "gated_scrape" {
		t.Errorf("name = %q, want gated_scrape", created.Name)
	}
	if created.Hemisphere != "external" {
		t.Errorf("hemisphere = %q, want external", created.Hemisphere)
	}
	if created.HandlerType != "http_endpoint" {
		t.Errorf("handler_type = %q, want http_endpoint", created.HandlerType)
	}
	if created.Status != "active" {
		t.Errorf("status = %q, want active", created.Status)
	}
	if !created.Enabled {
		t.Error("enabled = false, want true (default)")
	}
	if !created.RequiresApproval {
		t.Error("requires_approval = false, want true — the approval gate reads this column")
	}

	// The registry row must carry what the executor needs: handler_type,
	// handler_ref and the approval flag are the gate + dispatch inputs.
	ctx := context.Background()
	rows, err := srv.conn.Query(ctx,
		`SELECT name, handler_type, handler_ref, parameter_schema, requires_approval
		 FROM tools_registry WHERE name = 'gated_scrape'`)
	if err != nil {
		t.Fatalf("query registry: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("registry rows = %d, want 1", len(rows))
	}
	if got := toString(rows[0]["handler_ref"]); got != "https://scraper.consensus/fetch" {
		t.Errorf("handler_ref = %q", got)
	}
	if toInt(rows[0]["requires_approval"]) != 1 {
		t.Errorf("requires_approval = %v, want 1", rows[0]["requires_approval"])
	}
	if got := toString(rows[0]["parameter_schema"]); !strings.Contains(got, `"url"`) {
		t.Errorf("parameter_schema = %q, want the posted schema", got)
	}

	// AC3: the registered tool shows up in the list surface.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/tools", nil)
	listReq.Header.Set("Authorization", "Bearer "+srv.adminKey)
	listW := httptest.NewRecorder()
	srv.router.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", listW.Code, listW.Body.String())
	}
	var listed []ToolResponse
	if err := json.NewDecoder(listW.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	found := false
	for _, tl := range listed {
		if tl.Name == "gated_scrape" {
			found = true
			if !tl.RequiresApproval {
				t.Error("listed tool lost requires_approval")
			}
		}
	}
	if !found {
		t.Fatalf("registered tool missing from GET /api/v1/tools (got %d tools)", len(listed))
	}
}

func TestCreateTool_DefaultsMatchColumnDefaults(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	w := registerTool(t, srv, srv.adminKey,
		`{"name":"minimal_tool","description":"Minimum viable registration","handler_ref":"fn_minimal"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created ToolResponse
	_ = json.NewDecoder(w.Body).Decode(&created)
	if created.Hemisphere != "internal" {
		t.Errorf("hemisphere = %q, want internal (default)", created.Hemisphere)
	}
	if created.HandlerType != "sql_function" {
		t.Errorf("handler_type = %q, want sql_function (default)", created.HandlerType)
	}
	if created.Status != "active" {
		t.Errorf("status = %q, want active (default)", created.Status)
	}
	if created.RequiresApproval {
		t.Error("requires_approval = true, want false (default)")
	}
}

func TestCreateTool_DuplicateIsConflict(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	body := `{"name":"dup_tool","description":"first","handler_ref":"fn_dup"}`
	if w := registerTool(t, srv, srv.adminKey, body); w.Code != http.StatusCreated {
		t.Fatalf("first registration: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w := registerTool(t, srv, srv.adminKey, body)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate: expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "CONFLICT") {
		t.Errorf("duplicate error body missing CONFLICT code: %s", w.Body.String())
	}
}

func TestCreateTool_ValidationRejects(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing name", `{"description":"d","handler_ref":"fn_x"}`, "name is required"},
		{"missing description", `{"name":"v1","handler_ref":"fn_x"}`, "description is required"},
		{"missing handler_ref", `{"name":"v2","description":"d"}`, "handler_ref is required"},
		{"bad hemisphere", `{"name":"v3","description":"d","handler_ref":"fn_x","hemisphere":"right"}`, "hemisphere must be one of"},
		{"bad handler_type", `{"name":"v4","description":"d","handler_ref":"fn_x","handler_type":"bash"}`, "handler_type must be one of"},
		{"bad status", `{"name":"v5","description":"d","handler_ref":"fn_x","status":"sleeping"}`, "status must be one of"},
		{"malformed schema", `{"name":"v6","description":"d","handler_ref":"fn_x","parameter_schema":[1,2]}`, "parameter_schema must be a JSON object"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := registerTool(t, srv, srv.adminKey, tc.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Errorf("body %q does not mention %q", w.Body.String(), tc.want)
			}
		})
	}

	// Nothing may have been written by the rejected requests.
	rows, err := srv.conn.Query(context.Background(), `SELECT COUNT(*) AS cnt FROM tools_registry`)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if toInt(rows[0]["cnt"]) != 0 {
		t.Errorf("registry rows = %d after rejected registrations, want 0", toInt(rows[0]["cnt"]))
	}
}

func TestCreateTool_RequiresAdminScope(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	readonlyKey := "readonly-key-not-a-secret"
	_ = srv.conn.Exec(ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('key-ro', $1, $2, 'readonly', datetime('now'))`,
		sha256Hash(readonlyKey), readonlyKey[:8])

	body := `{"name":"ro_tool","description":"should be refused","handler_ref":"fn_ro"}`
	w := registerTool(t, srv, readonlyKey, body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("readonly key: expected 403, got %d: %s", w.Code, w.Body.String())
	}

	rows, err := srv.conn.Query(ctx, `SELECT COUNT(*) AS cnt FROM tools_registry`)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if toInt(rows[0]["cnt"]) != 0 {
		t.Errorf("registry rows = %d after forbidden registration, want 0", toInt(rows[0]["cnt"]))
	}
}

func TestCreateTool_Unauthenticated(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools",
		strings.NewReader(`{"name":"anon","description":"d","handler_ref":"fn_a"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestExecuteEnqueuedToolRequest_CarriesIteration pins the enqueue write
// against the PRODUCTION shape of tool_requests: iteration_id is NOT NULL with
// no column default (001 §4.2 Postgres; 031 SQLite rebuild). Before
// DF-CONSENSUS-62 the enqueue INSERT omitted the column, so on a fresh install
// POST /api/v1/tools/{name}/execute answered
// `500 INTERNAL_ERROR: failed to enqueue tool`
// (`NOT NULL constraint failed: tool_requests.iteration_id`) and nothing ever
// reached the approval gate. The session's current iteration must be recorded.
func TestExecuteEnqueuedToolRequest_CarriesIteration(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_ = srv.conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at, heartbeat_at) VALUES ('sess-it', 'test', 'gpt-4o', 'idle', 'Iteration carry', 7, $1, $1)`, now)
	_ = srv.conn.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, status, enabled, requires_approval) VALUES ('tool-it', 'gated_ext', 'Gated external tool', 'external', 'subprocess', '/bin/echo hi', 'active', 1, 1)`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/gated_ext/execute",
		strings.NewReader(`{"session_id":"sess-it"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("enqueue: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	rows, err := srv.conn.Query(ctx,
		`SELECT session_id, iteration_id, status FROM tool_requests WHERE tool_name = 'gated_ext'`)
	if err != nil {
		t.Fatalf("query tool_requests: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("tool_requests rows = %d, want 1 (the enqueue must reach the executor)", len(rows))
	}
	if got := toInt64(rows[0]["iteration_id"]); got != 7 {
		t.Errorf("iteration_id = %d, want 7 (the session's current iteration)", got)
	}
	if got := toString(rows[0]["status"]); got != "pending" {
		t.Errorf("status = %q, want pending", got)
	}
}
