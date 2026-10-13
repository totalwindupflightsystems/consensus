package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

func newMCPConfigTestServer(t *testing.T) (*httptest.Server, db.DB) {
	t.Helper()

	ctx := context.Background()
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + filepath.Join(t.TempDir(), "mcp-config.db"),
		MaxOpenConns: 1,
	})
	if err != nil {
		t.Fatalf("open MCP config test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.Exec(ctx, `CREATE TABLE system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		description TEXT,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("create MCP config settings table: %v", err)
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, conn
}

func TestMCPConfigPatchFlattensServerBlocks(t *testing.T) {
	serverConfig := map[string]any{
		"type": "remote",
		"url":  "https://example.invalid/mcp",
	}

	tests := []struct {
		name      string
		patchPath string
		getPath   string
		patch     map[string]any
		readPatch func(t *testing.T, body []byte) map[string]any
	}{
		{
			name:      "config block form",
			patchPath: "/config",
			getPath:   "/config",
			patch: map[string]any{
				"mcp": map[string]any{"websearch": serverConfig},
			},
			readPatch: func(t *testing.T, body []byte) map[string]any {
				t.Helper()
				var got struct {
					Settings map[string]any `json:"settings"`
				}
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("decode GET /config: %v (%s)", err, body)
				}
				if _, exists := got.Settings["mcp"]; exists {
					t.Errorf("GET /config retained literal mcp setting: %v", got.Settings)
				}
				assertMCPConfigSetting(t, got.Settings["mcp.websearch"], serverConfig)
				return got.Settings
			},
		},
		{
			name:      "config dotted form",
			patchPath: "/config",
			getPath:   "/config",
			patch: map[string]any{
				"mcp.websearch": serverConfig,
			},
			readPatch: func(t *testing.T, body []byte) map[string]any {
				t.Helper()
				var got struct {
					Settings map[string]any `json:"settings"`
				}
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("decode GET /config: %v (%s)", err, body)
				}
				assertMCPConfigSetting(t, got.Settings["mcp.websearch"], serverConfig)
				return got.Settings
			},
		},
		{
			name:      "global config block form",
			patchPath: "/global/config",
			getPath:   "/global/config",
			patch: map[string]any{
				"mcp": map[string]any{"websearch": serverConfig},
			},
			readPatch: func(t *testing.T, body []byte) map[string]any {
				t.Helper()
				var got map[string]any
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("decode GET /global/config: %v (%s)", err, body)
				}
				mcp, ok := got["mcp"].(map[string]any)
				if !ok {
					t.Fatalf("GET /global/config mcp = %#v, want object", got["mcp"])
				}
				if !reflect.DeepEqual(mcp["websearch"], serverConfig) {
					t.Errorf("GET /global/config mcp.websearch = %#v, want %#v", mcp["websearch"], serverConfig)
				}
				return got
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, conn := newMCPConfigTestServer(t)

			patchConfig(t, srv.URL, tc.patchPath, tc.patch)
			assertMCPNameOperationsResolve(t, srv.URL, "websearch")

			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, tc.getPath)
			if status != http.StatusOK {
				t.Fatalf("GET %s: got %d, want 200. Body: %s", tc.getPath, status, body)
			}
			roundTripPatch := tc.readPatch(t, body)
			patchConfig(t, srv.URL, tc.patchPath, roundTripPatch)
			assertMCPNameOperationsResolve(t, srv.URL, "websearch")

			rows, err := conn.Query(context.Background(), `SELECT key, value FROM system_settings ORDER BY key`)
			if err != nil {
				t.Fatalf("read flattened MCP settings: %v", err)
			}
			if len(rows) != 1 {
				t.Fatalf("settings rows = %v, want exactly one flattened server row", rows)
			}
			if key := toString(rows[0]["key"]); key != "mcp.websearch" {
				t.Errorf("settings key = %q, want mcp.websearch", key)
			}
			assertMCPConfigSetting(t, toString(rows[0]["value"]), serverConfig)
		})
	}
}

func patchConfig(t *testing.T, baseURL, path string, patch map[string]any) {
	t.Helper()
	body, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("marshal PATCH %s body: %v", path, err)
	}
	status, _, response := doShimRequestBody(t, baseURL, http.MethodPatch, path, string(body))
	if status != http.StatusOK {
		t.Fatalf("PATCH %s: got %d, want 200. Body: %s", path, status, response)
	}
}

func assertMCPNameOperationsResolve(t *testing.T, baseURL, name string) {
	t.Helper()

	status, _, body := doShimRequestBody(t, baseURL, http.MethodPost, "/mcp/"+name+"/connect", "")
	if status == http.StatusNotFound {
		t.Fatalf("POST /mcp/%s/connect returned McpServerNotFoundError after config PATCH: %s", name, body)
	}
	if status != http.StatusOK {
		t.Fatalf("POST /mcp/%s/connect: got %d, want 200. Body: %s", name, status, body)
	}

	status, _, body = doShimRequest(t, baseURL, http.MethodDelete, "/mcp/"+name+"/auth")
	if status == http.StatusNotFound {
		t.Fatalf("DELETE /mcp/%s/auth returned McpServerNotFoundError after config PATCH: %s", name, body)
	}
	if status != http.StatusOK {
		t.Fatalf("DELETE /mcp/%s/auth: got %d, want 200. Body: %s", name, status, body)
	}
}

func assertMCPConfigSetting(t *testing.T, got any, want map[string]any) {
	t.Helper()

	encoded, ok := got.(string)
	if !ok {
		t.Fatalf("flattened MCP setting = %#v, want JSON string", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("flattened MCP setting is not readable JSON: %v (%q)", err, encoded)
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Errorf("flattened MCP setting = %#v, want %#v", decoded, want)
	}
}
