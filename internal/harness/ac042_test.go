// Package harness: multi-session isolation test (AC-042).
//
// axiom:trace work_item=ac-042-multi-session-isolation spec=SPEC-015 impl=internal/harness
package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMultiSessionIsolation verifies AC-042:
// Session A cannot see session B's data using session-scoped keys.
func TestMultiSessionIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-session isolation test in short mode")
	}

	binPath := findConscienceBinary(t)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "consensus-test.db")
	port := randomPort(t)

	config := fmt.Sprintf(`server:
  port: %d
  bootstrap_api_key_ttl: 2160h
  admin_api_key_ttl: 2160h
llm:
  default_model: deepseek-chat
  provider: openai
  base_url: https://api.deepseek.com/v1
  api_key: sk-test-fallback
  max_context_tokens: 128000
  max_output_tokens: 16384
harness:
  heartbeat_interval_seconds: 5
  max_iterations: 3
  max_consecutive_errors: 3
  budget_limit_cents: 100
database:
  url: "sqlite://%s"
  max_open_conns: 4
logging:
  level: debug
`, port, dbPath)
	if err := os.WriteFile(filepath.Join(tmpDir, "consensus.yaml"), []byte(config), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Start server
	cmd := exec.Command(binPath, "--config", filepath.Join(tmpDir, "consensus.yaml"), "serve")
	cmd.Dir = tmpDir
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer cmd.Process.Kill()

	adminKey := parseBootstrapKey(t, stdout, 10*time.Second)
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if !waitForHealth(t, serverURL, 15*time.Second) {
		cmd.Process.Kill()
		t.Fatal("server not healthy")
	}

	doReq := func(method, path, body string, authKey string) (int, string) {
		var reqBody io.Reader
		if body != "" {
			reqBody = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, serverURL+path, reqBody)
		if authKey != "" {
			req.Header.Set("Authorization", "Bearer "+authKey)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request %s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	// Create session A
	codeA, bodyA := doReq("POST", "/api/v1/sessions",
		`{"agent_name":"session-a","goal":"isolation test A"}`, adminKey)
	if codeA != 200 && codeA != 201 {
		t.Fatalf("create session A: HTTP %d: %s", codeA, bodyA)
	}
	var sessionA struct {
		ID     string `json:"id"`
		APIKey string `json:"api_key"`
	}
	json.Unmarshal([]byte(bodyA), &sessionA)
	t.Logf("session A: id=%s key=%s...", sessionA.ID, sessionA.APIKey[:16])

	// Create session B
	codeB, bodyB := doReq("POST", "/api/v1/sessions",
		`{"agent_name":"session-b","goal":"isolation test B"}`, adminKey)
	if codeB != 200 && codeB != 201 {
		t.Fatalf("create session B: HTTP %d: %s", codeB, bodyB)
	}
	var sessionB struct {
		ID     string `json:"id"`
		APIKey string `json:"api_key"`
	}
	json.Unmarshal([]byte(bodyB), &sessionB)
	t.Logf("session B: id=%s key=%s...", sessionB.ID, sessionB.APIKey[:16])

	// Test 1: Session B's key tries to read A's session → expect 403
	codeCross, bodyCross := doReq("GET", "/api/v1/sessions/"+sessionA.ID, "", sessionB.APIKey)
	t.Logf("cross-session GET (B key → A session): HTTP %d", codeCross)
	_ = bodyCross // body may be useful for debugging

	if codeCross != 403 {
		t.Errorf("AC-042 FAIL: expected HTTP 403 for cross-session access, got %d", codeCross)
	} else {
		t.Log("AC-042: cross-session access correctly blocked (403)")
	}

	// Test 2: Admin key reads A's session → expect 200
	codeAdmin, bodyAdmin := doReq("GET", "/api/v1/sessions/"+sessionA.ID, "", adminKey)
	t.Logf("admin GET (session A): HTTP %d", codeAdmin)
	_ = bodyAdmin

	if codeAdmin != 200 {
		t.Errorf("AC-042 FAIL: expected HTTP 200 for admin access, got %d: %s", codeAdmin, bodyAdmin)
	} else {
		t.Log("AC-042: admin access allowed (200)")
	}

	// Test 3: Session B's key reads its own session → expect 200
	codeOwn, _ := doReq("GET", "/api/v1/sessions/"+sessionB.ID, "", sessionB.APIKey)
	t.Logf("own-session GET (B key → B session): HTTP %d", codeOwn)

	if codeOwn != 200 {
		t.Errorf("AC-042 FAIL: expected HTTP 200 for own-session access, got %d", codeOwn)
	} else {
		t.Log("AC-042: own-session access allowed (200)")
	}

	if codeCross == 403 && codeAdmin == 200 && codeOwn == 200 {
		t.Log("AC-042 PASS: session isolation enforced")
	}
}
