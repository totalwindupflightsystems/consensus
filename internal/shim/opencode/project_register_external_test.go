package opencode_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/api"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	"github.com/wojons/consensus/internal/migrate"
	"github.com/wojons/consensus/internal/shim/opencode"
)

// DF-CONSENSUS-67: register a project through the native production API, then
// read that same row through the pre-existing OpenCode GET /project surface.
// The projects table is never seeded directly by the test.
func TestProjectRegistrationShimReadPathEndToEnd(t *testing.T) {
	ctx := context.Background()
	conn, err := driver.Open(ctx, db.Config{URL: "sqlite://" + filepath.Join(t.TempDir(), "df67-project.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := migrate.New(conn).AutoMigrate(ctx); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	const adminKey = "cs_sk_admin_df67_project_test_key"
	hash := sha256.Sum256([]byte(adminKey))
	if err := conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at)
		VALUES ('key-df67-project', $1, 'cs_sk_ad', 'admin', datetime('now'))`, hex.EncodeToString(hash[:])); err != nil {
		t.Fatalf("create test admin credential: %v", err)
	}

	native := httptest.NewServer(api.NewServer(api.ServerConfig{DB: conn}).Handler())
	defer native.Close()
	shim := httptest.NewServer(opencode.NewServer(conn, "project-read-key", nil, nil).Handler())
	defer shim.Close()

	req, _ := http.NewRequest(http.MethodPost, native.URL+"/api/v1/projects", strings.NewReader(`{"name":"df67-project"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminKey)
	resp, err := native.Client().Do(req)
	if err != nil {
		t.Fatalf("register project: %v", err)
	}
	rawCreated, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read create response: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/projects = %d %s, want 201", resp.StatusCode, rawCreated)
	}
	var created map[string]any
	if err := json.Unmarshal(rawCreated, &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	projectID, _ := created["id"].(string)
	if projectID == "" {
		t.Fatalf("create response has no project id: %v", created)
	}

	req, _ = http.NewRequest(http.MethodGet, shim.URL+"/project", nil)
	req.SetBasicAuth("opencode", adminKey)
	resp, err = shim.Client().Do(req)
	if err != nil {
		t.Fatalf("read shim projects: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var projects []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		t.Fatalf("decode shim project list: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /project = %d %v, want 200", resp.StatusCode, projects)
	}
	if len(projects) != 1 || projects[0].ID != projectID || projects[0].Name != "df67-project" {
		t.Fatalf("shim projects = %+v, want registered project %s", projects, projectID)
	}
}
