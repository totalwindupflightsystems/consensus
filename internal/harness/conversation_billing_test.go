// Package harness: DF-CONSENSUS-27 regression — LLM usage must reach
// agent_billing and sessions.tokens_used_in/out on EVERY conversational
// terminal, not only the respond-commit path.
//
// The live repro (2026-09-25): a plain conversational turn — message_to_user
// with no system_actions — parses as a no-op turn, the iteration exits
// through the max-turns rollback terminal, and recordLLMUsageTx's in-tx
// usage write was silently rolled back with it: agent_billing stayed at 0
// rows, sessions token totals stayed 0/0, and GET /billing reported $0.00
// while real API spend was happening.
package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/api"
)

// dropPathConversationLLM answers with a plain conversational reply: a
// message_to_user with NO system actions. outputToTurnPlanV2 classifies this
// as ActionNoOp, so the turn neither commits nor responds and the iteration
// exits through a transaction-rollback terminal. This is the shape a real
// chat LLM produces for "just talk to me" turns — exactly the path where the
// usage ledger used to die (DF-CONSENSUS-27).
type dropPathConversationLLM struct {
	calls int
}

func (m *dropPathConversationLLM) Call(_ context.Context, _ []Message) (*LLMResponse, error) {
	m.calls++
	return &LLMResponse{
		Output: &AgentOutput{
			InternalMonologue:  "Answer conversationally without system actions.",
			MemoryStateChanges: []string{},
			SystemActions:      []string{},
			MessageToUser:      "Hello! Anything else you'd like to do?",
			ToolRequests:       []ToolRequest{},
			SubAgentSpawns:     []SubAgentSpawn{},
		},
		ModelID: "test-model",
		Usage: LLMUsage{
			PromptTokens:     1638,
			CompletionTokens: 158,
		},
	}, nil
}

// TestConversationalTurnWithRollbackTerminalStillBillsUsage drives a mocked
// LLM turn through the REST message path on a fresh DB and proves the usage
// ledger survives a rollback terminal (DF-CONSENSUS-27). Pre-fix, the usage
// row was written inside the planning transaction and the max-turns no-op
// terminal rolled it back: agent_billing empty, session token totals 0/0.
func TestConversationalTurnWithRollbackTerminalStillBillsUsage(t *testing.T) {
	llm := &dropPathConversationLLM{}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	adminKey := "cs_sk_billing_test_admin_key"
	hash := sha256.Sum256([]byte(adminKey))
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ('billing-admin', $1, $2, 'admin', CURRENT_TIMESTAMP)`,
		hex.EncodeToString(hash[:]), adminKey[:8]); err != nil {
		t.Fatalf("insert API key: %v", err)
	}

	server := api.NewServer(api.ServerConfig{DB: th.conn, Addr: ":0"})
	body := `{"role":"user","content":"Say hello and stop there — no system actions."}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/message", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST message returned %d: %s", w.Code, w.Body.String())
	}

	cfg := DefaultPlanningConfig()
	cfg.MaxTurns = 1
	result, err := th.RunInteractivePlanning(th.ctx, sessionID, cfg)
	if err != nil {
		t.Fatalf("run planning: %v", err)
	}
	if result == nil || (result.Status != "success" && result.Status != "warning") {
		t.Fatalf("planning result = %+v, want success or warning", result)
	}
	if llm.calls != 1 {
		t.Fatalf("LLM calls = %d, want 1", llm.calls)
	}

	// 1. Exactly one agent_billing row carrying the mocked usage.
	rows, err := th.conn.Query(th.ctx, `
		SELECT prompt_tokens, completion_tokens, cache_read_tokens, cache_write_tokens
		FROM agent_billing WHERE session_id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query agent_billing: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("agent_billing rows = %d, want exactly 1 (usage dropped on the rollback terminal?)", len(rows))
	}
	if got := toInt64(rows[0]["prompt_tokens"]); got != 1638 {
		t.Errorf("agent_billing.prompt_tokens = %d, want 1638", got)
	}
	if got := toInt64(rows[0]["completion_tokens"]); got != 158 {
		t.Errorf("agent_billing.completion_tokens = %d, want 158", got)
	}

	// 2. Session token totals include the turn's usage.
	srows, err := th.conn.Query(th.ctx,
		`SELECT tokens_used_in, tokens_used_out FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	if len(srows) != 1 {
		t.Fatalf("sessions rows = %d, want 1", len(srows))
	}
	if got := toInt64(srows[0]["tokens_used_in"]); got != 1638 {
		t.Errorf("sessions.tokens_used_in = %d, want 1638", got)
	}
	if got := toInt64(srows[0]["tokens_used_out"]); got != 158 {
		t.Errorf("sessions.tokens_used_out = %d, want 158", got)
	}

	// 3. GET /api/v1/sessions/{id}/billing totals equal the ledger row.
	breq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/billing", nil)
	breq.Header.Set("Authorization", "Bearer "+adminKey)
	bw := httptest.NewRecorder()
	server.Handler().ServeHTTP(bw, breq)
	if bw.Code != http.StatusOK {
		t.Fatalf("GET billing returned %d: %s", bw.Code, bw.Body.String())
	}
	var billing struct {
		TotalPromptTokens     int64            `json:"total_prompt_tokens"`
		TotalCompletionTokens int64            `json:"total_completion_tokens"`
		Entries               []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(bw.Body.Bytes(), &billing); err != nil {
		t.Fatalf("decode billing response: %v", err)
	}
	if billing.TotalPromptTokens != 1638 {
		t.Errorf("billing total_prompt_tokens = %d, want 1638", billing.TotalPromptTokens)
	}
	if billing.TotalCompletionTokens != 158 {
		t.Errorf("billing total_completion_tokens = %d, want 158", billing.TotalCompletionTokens)
	}
	if len(billing.Entries) != 1 {
		t.Errorf("billing entries = %d, want 1", len(billing.Entries))
	}
}
