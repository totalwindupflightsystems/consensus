package harness

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/api"
)

// DF-CONSENSUS-67: create a memory page through the production API and prove
// the existing harness context reader resolves it into the agent prompt. The
// memory_pages row itself is never seeded by SQL in this test.
func TestMemoryPageRegistrationPromptReadPathEndToEnd(t *testing.T) {
	th, err := newTestHarness(nil)
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	const adminKey = "cs_sk_admin_df67_memory_page_test"
	if err := th.conn.Exec(th.ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at)
		VALUES ('key-df67-memory-page', $1, 'cs_sk_ad', 'admin', datetime('now'))`, sha256Hash(adminKey)); err != nil {
		t.Fatalf("create test admin credential: %v", err)
	}
	ts := httptest.NewServer(api.NewServer(api.ServerConfig{DB: th.conn, HITL: th.hitl}).Handler())
	defer ts.Close()

	status, body := doAPIJSON(t, ts.URL, http.MethodPost, "/api/v1/sessions", adminKey,
		`{"agent_name":"df67-pages","goal":"resolve a registered memory page"}`)
	if status != http.StatusCreated {
		t.Fatalf("create session = %d %s", status, body)
	}
	var session api.CreateSessionResponse
	if err := json.Unmarshal([]byte(body), &session); err != nil {
		t.Fatalf("decode session: %v", err)
	}

	// Produce the target event through the normal iteration writer, not a
	// memory_pages fixture. The page under test is created only through REST.
	th.LLMClient = newMockLLM(&AgentOutput{
		InternalMonologue: "create a page target",
		MemoryStateChanges: []string{fmt.Sprintf(
			`INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'df67 page target', '%s', 1)`, session.ID)},
	})
	result, err := th.RunAgentIteration(th.ctx, session.ID)
	if err != nil || result.Status != "success" {
		t.Fatalf("run target-producing iteration: status=%q err=%v", result.Status, err)
	}
	rows, err := th.conn.Query(th.ctx, `SELECT id FROM memory_events WHERE session_id = $1 AND type = 'text_block' AND content = 'df67 page target'`, session.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("find production-written target event: rows=%d err=%v", len(rows), err)
	}
	targetID := toInt64(rows[0]["id"])

	status, body = doAPIJSON(t, ts.URL, http.MethodPost, "/api/v1/memory-pages", adminKey,
		fmt.Sprintf(`{"session_id":%q,"name":"df67-page","target_ids":[%d]}`, session.ID, targetID))
	if status != http.StatusCreated {
		t.Fatalf("POST /api/v1/memory-pages = %d %s, want 201", status, body)
	}
	if !strings.Contains(body, `"name":"df67-page"`) {
		t.Fatalf("memory page create response = %s", body)
	}

	pageIDs, err := th.resolvePageMemoryIDs(th.ctx, session.ID)
	if err != nil {
		t.Fatalf("resolve registered memory page: %v", err)
	}
	if !pageIDs[targetID] {
		t.Fatalf("resolved page ids = %v, want target event %d", pageIDs, targetID)
	}

	active, err := th.ReadActiveContext(th.ctx, session.ID)
	if err != nil {
		t.Fatalf("read active context: %v", err)
	}
	if len(active.Messages) < 2 {
		t.Fatalf("active context has %d messages, want system and context", len(active.Messages))
	}
	contextPrompt := active.Messages[1].Content
	if !strings.Contains(contextPrompt, "df67 page target") {
		t.Fatalf("page target was not returned in prompt context:\n%s", contextPrompt)
	}
}
