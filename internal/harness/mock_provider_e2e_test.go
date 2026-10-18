// Package harness_test: DF-CONSENSUS-109 end-to-end proof.
//
// The two dogfood defects were reported by a keyless fresh user in mock mode:
//  1. POST /api/v1/sessions answered 400 MISSING_LLM_CONFIG, and
//  2. once past that, the harness burned MaxTurns on "planning: no-op turn"
//     and ended with "max turns reached with no productive work".
//
// This test binds the pieces the user actually hits: the real migration set,
// the factory-built mock client (llm.NewClient with provider=mock and
// CONSENSUS_MOCK_LLM=1), and the real planning loop. It asserts that one
// iteration terminalizes with the mock's assistant message persisted.
//
// axiom:trace spec=specs/020-multi-turn-planning.md impl=internal/llm/client.go
package harness_test

import (
	"context"
	"os"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	"github.com/wojons/consensus/internal/harness"
	"github.com/wojons/consensus/internal/llm"
	"github.com/wojons/consensus/internal/migrate"
)

func TestMockProviderEndToEnd_IterationTerminatesWithAssistantMessage(t *testing.T) {
	t.Setenv("CONSENSUS_MOCK_LLM", "1")

	ctx := context.Background()

	tmp, err := os.CreateTemp("", "consensus-df109-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	dbPath := tmp.Name()
	_ = tmp.Close()
	defer func() {
		_ = os.Remove(dbPath)
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	}()

	conn, err := driver.Open(ctx, db.Config{URL: "sqlite://" + dbPath})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Real schema: the same migration set `consensus serve` applies.
	if _, err := migrate.New(conn).AutoMigrate(ctx); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	// The mock provider as a keyless user configures it: provider=mock, no key.
	client, err := llm.NewClient(&llm.Config{Provider: llm.ProviderMock, Model: "mock-model"})
	if err != nil {
		t.Fatalf("llm.NewClient(mock): %v", err)
	}

	const sessionID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeee00d9"
	if err := conn.Exec(ctx, `INSERT OR IGNORE INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out) VALUES ('mock-model', 1, 128000, 0, 0)`); err != nil {
		t.Fatalf("seed model_registry: %v", err)
	}
	if err := conn.Exec(ctx, `INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal) VALUES ($1, 'keyless-fresh-user', 'mock-model', 'thinking', 'high', 'Say hello')`, sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	h := harness.New(conn, client)
	cfg := harness.DefaultPlanningConfig()
	cfg.MaxTurns = 10 // the budget the dogfood run burned without output

	result, err := h.RunInteractivePlanning(ctx, sessionID, cfg)
	if err != nil {
		t.Fatalf("run planning: %v", err)
	}
	if result == nil {
		t.Fatal("planning returned nil result")
	}
	if result.Status != "success" {
		t.Fatalf("planning status = %q (error_injected=%q), want success — "+
			"a mock session must not end on the max-turns warning",
			result.Status, result.ErrorInjected)
	}

	rows, err := conn.Query(ctx, `SELECT content FROM memory_events WHERE session_id = $1 AND type = 'text_block'`, sessionID)
	if err != nil {
		t.Fatalf("query assistant messages: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("assistant text_blocks = %d, want 1", len(rows))
	}
	got, _ := rows[0]["content"].(string)
	if got != llm.MockAssistantMessage {
		t.Errorf("assistant message = %q, want the mock's deterministic reply %q", got, llm.MockAssistantMessage)
	}

	statusRows, err := conn.Query(ctx, `SELECT status FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query session status: %v", err)
	}
	if len(statusRows) != 1 {
		t.Fatalf("session rows = %d, want 1", len(statusRows))
	}
	if status, _ := statusRows[0]["status"].(string); status != "idle" {
		t.Errorf("session status = %q, want idle", status)
	}
}
