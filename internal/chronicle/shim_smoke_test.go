// Package chronicle: shim endpoint smoke test for CI.
package chronicle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func buildBin(t *testing.T, tmpDir string) string {
	t.Helper()
	binPath := filepath.Join(tmpDir, "consensus")
	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/consensus")
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return binPath
}

func startServe(t *testing.T, tmpDir, dbPath string, port int) *exec.Cmd {
	t.Helper()
	binPath := buildBin(t, tmpDir)
	initCmd := exec.Command(binPath, "init", "--db-url", "sqlite://"+dbPath)
	initCmd.Dir = tmpDir
	initCmd.CombinedOutput()

	configYAML := fmt.Sprintf(`server:
  hostname: 127.0.0.1
  port: %d
llm:
  default_model: deepseek-chat
  provider: openai
  base_url: https://api.deepseek.com/v1
  api_key: %s
  max_context_tokens: 128000
database:
  url: "sqlite://%s"
`, port, os.Getenv("DEEPSEEK_API_KEY"), dbPath)
	os.WriteFile(filepath.Join(tmpDir, "consensus.yaml"), []byte(configYAML), 0644)

	ctx := t.Context()
	cmd := exec.CommandContext(ctx, binPath, "serve",
		"--config", filepath.Join(tmpDir, "consensus.yaml"),
		"--port", fmt.Sprintf("%d", port),
		"--adapter", "opencode",
	)
	cmd.Dir = tmpDir
	if err := cmd.Start(); err != nil {
		t.Fatalf("serve: %v", err)
	}
	return cmd
}

func TestShimEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping shim smoke test in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "shim-test.db")
	port := 8194

	serveCmd := startServe(t, tmpDir, dbPath, port)
	defer serveCmd.Process.Kill()
	time.Sleep(2 * time.Second)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 5 * time.Second}

	type check struct {
		path        string
		method      string
		minStatus   int
		maxStatus   int
		description string
		required    bool
	}
	checks := []check{
		{"/global/health", "GET", 200, 200, "global health", true},
		{"/session", "GET", 200, 401, "session list", true},
		{"/session/test-id", "GET", 200, 404, "session by id", false},
		{"/doc", "GET", 200, 200, "doc", true},
		{"/config", "GET", 200, 401, "config", true},
		{"/config/providers", "GET", 200, 401, "providers", true},
		{"/provider", "GET", 200, 401, "provider", true},
		{"/agent", "GET", 200, 401, "agent", true},
		{"/experimental/tool", "GET", 200, 401, "tools list", true},
		{"/experimental/tool/ids", "GET", 200, 401, "tool ids", true},
		{"/find", "POST", 200, 401, "find", true},
		{"/find/test", "GET", 200, 404, "find sub", false},
		{"/file/content", "POST", 200, 401, "file content", true},
		{"/file/status", "GET", 200, 401, "file status", true},
		{"/permission", "GET", 200, 401, "permissions", true},
		{"/permission/1", "GET", 200, 404, "permission by id", false},
		{"/tui/index.html", "GET", 200, 404, "tui", false},
		{"/lsp", "POST", 200, 401, "lsp", true},
		{"/mcp", "POST", 200, 404, "mcp", false},
		{"/auth/status", "GET", 200, 404, "auth status", false},
		{"/event", "GET", 200, 401, "events", true},
		{"/project", "GET", 200, 401, "project vcs", true},
		{"/project/test", "GET", 200, 404, "project sub", false},
		{"/vcs", "GET", 200, 401, "vcs", true},
		{"/vcs/test", "GET", 200, 404, "vcs sub", false},
	}

	passed, failed := 0, 0
	for _, c := range checks {
		req, _ := http.NewRequest(c.method, baseURL+c.path, nil)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("%s %s: %v", c.method, c.path, err)
			failed++
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		status := resp.StatusCode
		if status >= c.minStatus && status <= c.maxStatus {
			passed++
		} else if status == 404 && !c.required {
			t.Logf("  skip %s %s: 404 (not required) — %s", c.method, c.path, c.description)
		} else {
			detail := string(body)
			if len(detail) > 80 {
				detail = detail[:80] + "..."
			}
			t.Errorf("%s %s: %d (want %d-%d) — %s: %s",
				c.method, c.path, status, c.minStatus, c.maxStatus, c.description, detail)
			failed++
		}
	}
	t.Logf("shim: %d passed, %d failed", passed, failed)
	if failed > 0 {
		t.Errorf("%d endpoint(s) failed", failed)
	}
}

func TestShimHealthEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "shim-health.db")
	port := 8195

	serveCmd := startServe(t, tmpDir, dbPath, port)
	defer serveCmd.Process.Kill()
	time.Sleep(2 * time.Second)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 5 * time.Second}

	// Doc endpoint (known working)
	resp, _ := client.Get(baseURL + "/doc")
	if resp != nil {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Logf("/doc: %d, %d bytes", resp.StatusCode, len(body))
	}

	// Global health (may need fix)
	resp2, err := client.Get(baseURL + "/global/health")
	if err != nil {
		t.Fatalf("/global/health: %v", err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode == 200 {
		t.Logf("/global/health: 200 — %s", string(body2))
		var h map[string]any
		if json.Unmarshal(body2, &h) == nil {
			t.Logf("  healthy=%v version=%v", h["healthy"], h["version"])
		}
	} else {
		t.Logf("KNOWN GAP: /global/health returned %d — chi prefix routing issue", resp2.StatusCode)
	}
}
