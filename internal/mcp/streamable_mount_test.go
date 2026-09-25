// Package mcp_test: full-deployment mount checks for the streamable-HTTP MCP
// surface (MCP-DIRECT-001).
//
// This is an EXTERNAL test package on purpose: it wires the API server, the
// MCP handler and the opencode shim together exactly the way
// cmd/consensus/main.go does, and the shim imports the api package — an
// in-package (package mcp) test doing the same would be an import cycle.
//
// The point of the exercise: a client with nothing but the server URL and an
// API key must be able to attach over MCP. POST /mcp answers initialize and
// tools/list with no out-of-band route knowledge, GET /mcp serves the SSE
// stream, and the legacy POST /mcp/message path keeps working.
package mcp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/wojons/consensus/internal/api"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/mcp"
	"github.com/wojons/consensus/internal/shim/opencode"
	"github.com/wojons/consensus/specs"
)

// keyDB answers the api_keys lookup with one admin key and nothing else —
// enough for a real initialize handshake over the full-deployment tree.
type keyDB struct{}

func (keyDB) BeginTx(ctx context.Context) (db.Tx, error)            { return nil, nil }
func (keyDB) Exec(ctx context.Context, q string, args ...any) error { return nil }
func (keyDB) Query(ctx context.Context, q string, args ...any) ([]db.Row, error) {
	return []db.Row{{"id": "key-1", "scope": "admin", "session_id": nil}}, nil
}
func (keyDB) QueryRow(ctx context.Context, q string, args ...any) (db.Row, error) {
	rows, _ := keyDB{}.Query(ctx, q, args...)
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}
func (keyDB) Backend() db.Backend { return db.BackendSQLite }
func (keyDB) Close() error        { return nil }

// emptyDB answers every query with no rows: enough for the mount itself to be
// exercised (auth failures and 404-by-id are fine — registration is what this
// check verifies).
type emptyDB struct{}

func (emptyDB) BeginTx(ctx context.Context) (db.Tx, error)            { return nil, nil }
func (emptyDB) Exec(ctx context.Context, q string, args ...any) error { return nil }
func (emptyDB) Query(ctx context.Context, q string, args ...any) ([]db.Row, error) {
	return nil, nil
}
func (emptyDB) QueryRow(ctx context.Context, q string, args ...any) (db.Row, error) {
	return nil, nil
}
func (emptyDB) Backend() db.Backend { return db.BackendSQLite }
func (emptyDB) Close() error        { return nil }

// newFullDeployServer mirrors cmd/consensus/main.go: MCP handler under /mcp/*
// (plus the bare /mcp mount), opencode shim on its MountPatterns.
func newFullDeployServer(dbase db.DB) http.Handler {
	apiSrv := api.NewServer(api.ServerConfig{DB: dbase, Addr: ":0"})
	mux := apiSrv.Handler().(chi.Router)

	mcpHandler := mcp.NewServer(dbase).Handler()
	mux.Handle("/mcp/*", mcpHandler)
	mux.Handle("/mcp", mcpHandler) // MCP-DIRECT-001: bare /mcp answers too

	shimSrv := opencode.NewServer(dbase, "", nil, opencode.NewServiceAdapter(apiSrv.Service()))
	for _, pattern := range opencode.MountPatterns {
		mux.Handle(pattern, shimSrv.Handler())
	}
	return apiSrv.Handler()
}

// TestStreamableMount_InitializeAndToolsListOnBareMCP proves acceptance
// criteria 1 and 4 together: POST /mcp on a bare full-deployment server
// answers initialize (session issued via the Mcp-Session-Id header) and then
// tools/list — with no out-of-band route knowledge — returns at least 8 tools
// including list_tasks and claim_task, each with a typed input schema and a
// real description.
func TestStreamableMount_InitializeAndToolsListOnBareMCP(t *testing.T) {
	srv := httptest.NewServer(newFullDeployServer(keyDB{}))
	defer srv.Close()

	initBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "probe-client", "version": "1.0"},
			"_meta":           map[string]any{"authorization": "Bearer cs_ak_mounttest"},
		},
	})
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(string(initBody)))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	resp.Body.Close()
	var initResp map[string]any
	if err := json.Unmarshal(data, &initResp); err != nil {
		t.Fatalf("initialize: invalid JSON (status %d): %q", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initialize: expected 200, got %d (%v)", resp.StatusCode, initResp)
	}
	if _, ok := initResp["error"]; ok {
		t.Fatalf("initialize failed: %v", initResp["error"])
	}
	sessID := resp.Header.Get("Mcp-Session-Id")
	if sessID == "" {
		t.Fatal("initialize response carries no Mcp-Session-Id header — a streamable client cannot address follow-up calls")
	}

	// tools/list on the same URL with the session header.
	listBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	})
	req2, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(string(listBody)))
	if err != nil {
		t.Fatalf("build tools/list request: %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Mcp-Session-Id", sessID)
	resp2, err := (&http.Client{Timeout: 10 * time.Second}).Do(req2)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	data2, _ := io.ReadAll(io.LimitReader(resp2.Body, 65536))
	resp2.Body.Close()
	var listResp map[string]any
	if err := json.Unmarshal(data2, &listResp); err != nil {
		t.Fatalf("tools/list: invalid JSON (status %d): %q", resp2.StatusCode, strings.TrimSpace(string(data2)))
	}
	if _, ok := listResp["error"]; ok {
		t.Fatalf("tools/list failed with session header: %v", listResp["error"])
	}
	result, ok := listResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list: no result object: %v", listResp)
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list: result.tools missing: %v", result)
	}
	if len(tools) < 8 {
		t.Errorf("expected >= 8 tools (MCP-DIRECT-001 adds list_tasks + claim_task), got %d", len(tools))
	}

	// The two new tools must appear with a typed schema and a real
	// description — a bare name is a failed attach for a discoverable client.
	byName := map[string]map[string]any{}
	for _, toolAny := range tools {
		tool, ok := toolAny.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tool["name"].(string)
		byName[name] = tool
	}
	for _, want := range []string{"list_tasks", "claim_task"} {
		tool, ok := byName[want]
		if !ok {
			t.Errorf("tools/list is missing %q (MCP-DIRECT-001)", want)
			continue
		}
		desc, _ := tool["description"].(string)
		if len(desc) < 15 {
			t.Errorf("%q: description too thin (%q) — must be a real description", want, desc)
		}
		schema, ok := tool["inputSchema"].(map[string]any)
		if !ok {
			t.Errorf("%q: inputSchema missing or not an object: %v", want, tool["inputSchema"])
			continue
		}
		if schema["type"] != "object" {
			t.Errorf("%q: inputSchema.type = %v, want \"object\"", want, schema["type"])
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok || len(props) == 0 {
			t.Errorf("%q: inputSchema.properties missing or empty: %v", want, schema["properties"])
		}
	}
	if claim, ok := byName["claim_task"]; ok {
		if schema, ok := claim["inputSchema"].(map[string]any); ok {
			if props, _ := schema["properties"].(map[string]any); props != nil {
				if _, ok := props["task_id"]; !ok {
					t.Errorf("claim_task schema has no task_id property: %v", props)
				}
			}
		}
	}
}

// TestStreamableMount_UnauthenticatedToolsListIs401Class proves acceptance
// criterion 3 on the full-deployment mount: tools/list with no session and
// with a garbage session must surface a 401-class response.
func TestStreamableMount_UnauthenticatedToolsListIs401Class(t *testing.T) {
	srv := httptest.NewServer(newFullDeployServer(emptyDB{}))
	defer srv.Close()

	for _, tc := range []struct {
		name   string
		header string
	}{
		{"no-session-header", ""},
		{"unknown-session-header", "not-a-real-session-id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "tools/list",
			})
			req, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(string(body)))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			if tc.header != "" {
				req.Header.Set("Mcp-Session-Id", tc.header)
			}
			resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
			if err != nil {
				t.Fatalf("tools/list: %v", err)
			}
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
			resp.Body.Close()
			var out map[string]any
			_ = json.Unmarshal(data, &out)
			errObj, ok := out["error"].(map[string]any)
			if !ok {
				t.Fatalf("expected JSON-RPC error for %s, got status %d body %v", tc.name, resp.StatusCode, out)
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected HTTP 401 class for unauthenticated tools/list, got %d (error=%v)", resp.StatusCode, errObj)
			}
		})
	}
}

// TestStreamableMount_GETReturnsSSEStream proves acceptance criterion 2's GET
// half: GET /mcp answers an SSE stream (streamable-HTTP shape).
func TestStreamableMount_GETReturnsSSEStream(t *testing.T) {
	srv := httptest.NewServer(newFullDeployServer(emptyDB{}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/mcp", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET /mcp: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /mcp: expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("GET /mcp: expected text/event-stream, got %q", ct)
	}
	// Read the first event (the endpoint event) then cancel.
	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "event:") {
		t.Errorf("GET /mcp stream: expected an SSE event, got %q", strings.TrimSpace(string(buf[:n])))
	}
	cancel()
}

// TestStreamableMount_LegacyMessageStillRegistered proves the legacy
// POST /mcp/message path is still routed after the /mcp mount work (it must
// answer its own 404 "session not found", never chi's "404 page not found").
func TestStreamableMount_LegacyMessageStillRegistered(t *testing.T) {
	srv := httptest.NewServer(newFullDeployServer(emptyDB{}))
	defer srv.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp/message?sessionId=does-not-exist", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST /mcp/message: %v", err)
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if strings.Contains(string(data), "page not found") {
		t.Fatalf("POST /mcp/message fell through to the router (route lost): %q", strings.TrimSpace(string(data)))
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST /mcp/message with unknown session: expected handler-level 404, got %d (%q)", resp.StatusCode, strings.TrimSpace(string(data)))
	}
}

// TestStreamableMount_ServedSpecMarksMCPImplemented pins the served OpenAPI
// contract to the runtime behavior: once /mcp answers MCP traffic, the served
// spec must no longer declare it x-not-implemented (internal/api/
// openapi_paths_test.go exempts those path items — this test closes the door
// behind the exemption).
func TestStreamableMount_ServedSpecMarksMCPImplemented(t *testing.T) {
	srv := httptest.NewServer(newFullDeployServer(emptyDB{}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("GET /openapi.json: %v", err)
	}
	defer resp.Body.Close()
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode /openapi.json: %v", err)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("served spec has no paths object")
	}
	item, ok := paths["/mcp"].(map[string]any)
	if !ok {
		t.Fatal("served spec no longer declares /mcp — re-add it (MCP-DIRECT-001)")
	}
	if v, ok := item["x-not-implemented"].(bool); ok && v {
		t.Error("served spec still marks /mcp x-not-implemented while the runtime serves MCP traffic on it")
	}
}

// TestStreamableMount_RawSpecYAMLMatchesServedBehavior double-checks the
// embedded bundle (the same bytes /openapi.json serves, compiled in via
// specs.BundledYAML) declares /mcp with GET+POST and no x-not-implemented
// marker. The served-spec test above proves the runtime matches; this pins
// the source contract directly.
func TestStreamableMount_RawSpecYAMLMatchesServedBehavior(t *testing.T) {
	var doc map[string]any
	if err := yaml.Unmarshal(specs.BundledYAML, &doc); err != nil {
		t.Fatalf("embedded bundled.yaml does not parse: %v", err)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("bundled.yaml has no paths object")
	}
	item, ok := paths["/mcp"].(map[string]any)
	if !ok {
		t.Fatal("bundled.yaml does not declare /mcp")
	}
	if v, ok := item["x-not-implemented"].(bool); ok && v {
		t.Error("bundled.yaml still marks /mcp x-not-implemented — regenerate the bundle after the runtime change")
	}
	for _, m := range []string{"get", "post"} {
		if _, ok := item[m].(map[string]any); !ok {
			t.Errorf("bundled.yaml /mcp is missing the %q operation", m)
		}
	}
}
