// Package harness: end-to-end integration test (schema → harness → API → evidence).
//
// axiom:trace work_item=end-to-end-integration-test spec=specs/000-north-star.md,specs/008-harness.md,specs/015-api-and-mcp.md plan=phase-1/task-1/step-1
package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/api"
	"github.com/wojons/consensus/internal/hitl"
	"github.com/wojons/consensus/internal/session"
)

func TestEndToEnd_FullStack(t *testing.T) {
	th, err := newTestHarness(nil)
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	// Seed admin API key
	adminKey := "cs_sk_admin_e2e_test_key_42"
	adminHash := sha256Hash(adminKey)
	th.conn.Exec(th.ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('key-admin', $1, 'cs_sk_ad', 'admin', datetime('now'))`, adminHash)

	// Wire API server on same DB
	apiServer := api.NewServer(api.ServerConfig{DB: th.conn, HITL: hitl.New(th.conn)})
	ts := httptest.NewServer(apiServer.Handler())
	defer ts.Close()

	// Create session via REST API
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/sessions", strings.NewReader(`{"agent_name":"e2e-agent","goal":"Prove full stack works"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminKey)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("create session request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create session: %d %s", resp.StatusCode, string(body))
	}
	var created api.CreateSessionResponse
	json.Unmarshal(body, &created)
	sessionID := created.ID
	sessionKey := created.APIKey
	t.Logf("session created: %s", sessionID)

	// Send message
	req2, _ := http.NewRequest("POST", ts.URL+"/api/v1/sessions/"+sessionID+"/message", strings.NewReader(`{"content":"Hello agent"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+sessionKey)
	resp2, _ := ts.Client().Do(req2)
	resp2.Body.Close()

	// Run harness iteration
	iter := getSessionIteration(t, th, sessionID) + 1
	th.LLMClient = newMockLLM(&AgentOutput{
		InternalMonologue: "E2E proof.",
		MemoryStateChanges: []string{
			"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'E2E proof: harness works', '" + sessionID + "', " + itoa64(iter) + ")",
		},
	})
	result, err := th.RunAgentIteration(th.ctx, sessionID)
	if err != nil {
		t.Fatalf("iteration: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("iteration failed: %s", result.Status)
	}

	// Verify evidence
	auditCount, _ := th.assertAuditLogCount(sessionID)
	snapCount, _ := th.assertIterationSnapshotCount(sessionID)
	memRows, _ := th.conn.Query(th.ctx, `SELECT COUNT(*) as cnt FROM memory_events WHERE session_id = $1`, sessionID)
	memCount := toInt(memRows[0]["cnt"])

	if auditCount < 1 {
		t.Error("no audit log")
	}
	if snapCount < 1 {
		t.Error("no iteration snapshot")
	}
	if memCount < 2 {
		t.Errorf("expected >=2 memory events, got %d", memCount)
	}

	// Session via API
	req3, _ := http.NewRequest("GET", ts.URL+"/api/v1/sessions/"+sessionID, nil)
	req3.Header.Set("Authorization", "Bearer "+adminKey)
	resp3, err := ts.Client().Do(req3)
	if err != nil {
		t.Fatalf("get session request: %v", err)
	}
	defer resp3.Body.Close()
	body3, _ := io.ReadAll(resp3.Body)
	var sr api.SessionResponse
	json.Unmarshal(body3, &sr)
	if sr.AgentName != "e2e-agent" {
		t.Errorf("expected agent 'e2e-agent', got %q", sr.AgentName)
	}

	// Session RLS
	req4, _ := http.NewRequest("GET", ts.URL+"/api/v1/sessions/nonexistent", nil)
	req4.Header.Set("Authorization", "Bearer "+sessionKey)
	resp4, _ := ts.Client().Do(req4)
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusForbidden {
		t.Errorf("RLS: expected 403, got %d", resp4.StatusCode)
	}

	t.Logf("E2E FULL STACK PASS | audit=%d snap=%d mem=%d", auditCount, snapCount, memCount)
}

func TestEndToEnd_MultipleIterations(t *testing.T) {
	th, err := newTestHarness(nil)
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	sessionID, _ := th.createTestSession()
	for i := int64(1); i <= 3; i++ {
		th.LLMClient = newMockLLM(&AgentOutput{
			InternalMonologue: "Iter " + strconv.FormatInt(i, 10),
			MemoryStateChanges: []string{
				"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'iter-" + itoa64(i) + "', '" + sessionID + "', " + itoa64(i) + ")",
			},
		})
		r, _ := th.RunAgentIteration(th.ctx, sessionID)
		if r.Status != "success" {
			t.Fatalf("iter %d: %s", i, r.Status)
		}
	}
	ac, _ := th.assertAuditLogCount(sessionID)
	sc, _ := th.assertIterationSnapshotCount(sessionID)
	mr, _ := th.conn.Query(th.ctx, `SELECT COUNT(*) as cnt FROM memory_events WHERE session_id = $1`, sessionID)
	mc := toInt(mr[0]["cnt"])
	if ac < 3 || sc < 3 || mc < 3 {
		t.Errorf("expected >=3 each, got audit=%d snap=%d mem=%d", ac, sc, mc)
	}
}

func TestEndToEnd_ToolExec(t *testing.T) {
	th, _ := newTestHarness(nil)
	defer th.close()
	sid, _ := th.createTestSession()
	th.LLMClient = newMockLLM(&AgentOutput{
		InternalMonologue: "need tool",
		MemoryStateChanges: []string{
			"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'tool-req', '" + sid + "', 1)",
		},
		ToolRequests: []ToolRequest{{ToolName: "http_fetcher", Parameters: map[string]any{"url": "https://e.com"}}},
	})
	r, _ := th.RunAgentIteration(th.ctx, sid)
	if r.NextStatus != string(session.StatusToolExec) {
		t.Errorf("expected tool_exec, got %q", r.NextStatus)
	}
}

// helpers
func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func getSessionIteration(t *testing.T, th *testHarness, sid string) int64 {
	t.Helper()
	rows, _ := th.conn.Query(th.ctx, `SELECT iteration FROM sessions WHERE id = $1`, sid)
	return toInt64(rows[0]["iteration"])
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
