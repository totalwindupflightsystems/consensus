// Package harness: tests for context reader and Markdown formatting.
//
// axiom:trace work_item=runtime-harness-01 spec=specs/008-harness.md,specs/012-system-prompt-and-discovery.md plan=phase-1/task-1-1/step-1-1-3 test=internal/harness/context_test.go
package harness

import (
	"context"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// ============================================================================
// Test fixture: set up in-memory SQLite with minimal schema
// ============================================================================

func setupTestDB(t *testing.T) db.DB {
	t.Helper()
	ctx := context.Background()
	database, err := driver.Open(ctx, db.Config{URL: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	// Create minimal schema for context tests
	mustExec(t, database, `CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		agent_name TEXT NOT NULL,
		model_id TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'booting',
		trust_level TEXT NOT NULL DEFAULT 'high',
		goal TEXT,
		context_budget INT NOT NULL DEFAULT 128000,
		tokens_used_in BIGINT NOT NULL DEFAULT 0,
		tokens_used_out BIGINT NOT NULL DEFAULT 0,
		iteration BIGINT NOT NULL DEFAULT 0,
		project_id TEXT,
		parent_id TEXT,
		planning_max_turns INT NOT NULL DEFAULT 10
	)`)

	mustExec(t, database, `CREATE TABLE IF NOT EXISTS memory_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		content TEXT NOT NULL,
		summary_text TEXT,
		session_id TEXT NOT NULL,
		iteration_created BIGINT NOT NULL
	)`)

	mustExec(t, database, `CREATE TABLE IF NOT EXISTS display_modes (
		memory_id INTEGER PRIMARY KEY,
		mode TEXT NOT NULL DEFAULT 'full',
		session_id TEXT NOT NULL
	)`)

	mustExec(t, database, `CREATE TABLE IF NOT EXISTS tools_registry (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		hemisphere TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		status TEXT NOT NULL DEFAULT 'active'
	)`)

	return database
}

func mustExec(t *testing.T, database db.DB, query string) {
	t.Helper()
	if err := database.Exec(context.Background(), query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// ============================================================================
// Context Reading Tests
// ============================================================================

func TestReadActiveContext_SessionNotFound(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	h := &Harness{db: database}
	_, err := h.ReadActiveContext(context.Background(), "non-existent-id")
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("expected 'session not found' error, got: %v", err)
	}
}

func TestReadActiveContext_SessionFound(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	// Insert a test session
	mustExec(t, database, `INSERT INTO sessions (id, agent_name, model_id, status, goal)
		VALUES ('s1', 'test-agent', 'gpt-4o', 'idle', 'Analyze data')`)

	h := &Harness{db: database}
	ic, err := h.ReadActiveContext(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ReadActiveContext: %v", err)
	}

	if ic.SessionID != "s1" {
		t.Errorf("expected session 's1', got %q", ic.SessionID)
	}
	if ic.AgentName != "test-agent" {
		t.Errorf("expected agent 'test-agent', got %q", ic.AgentName)
	}
	if ic.Goal != "Analyze data" {
		t.Errorf("expected goal 'Analyze data', got %q", ic.Goal)
	}
	if ic.Status != "idle" {
		t.Errorf("expected status 'idle', got %q", ic.Status)
	}
	if ic.ModelID != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", ic.ModelID)
	}
}

func TestReadActiveContext_WithMemoryEvents(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	mustExec(t, database, `INSERT INTO sessions (id, agent_name, model_id, status, goal)
		VALUES ('s1', 'agent', 'gpt-4o', 'thinking', 'Test memory')`)

	mustExec(t, database, `INSERT INTO memory_events (type, content, session_id, iteration_created)
		VALUES ('text_block', 'Hello world', 's1', 1)`)
	mustExec(t, database, `INSERT INTO memory_events (type, content, session_id, iteration_created)
		VALUES ('thinking', 'I should analyze...', 's1', 1)`)

	h := &Harness{db: database}
	ic, err := h.ReadActiveContext(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ReadActiveContext: %v", err)
	}

	// Memory events should be included in the user message (the Markdown context)
	if len(ic.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(ic.Messages))
	}

	userMsg := ic.Messages[1]
	if userMsg.Role != "user" {
		t.Errorf("expected role 'user', got %q", userMsg.Role)
	}
	if !strings.Contains(userMsg.Content, "Hello world") {
		t.Error("user message should contain 'Hello world'")
	}
	if !strings.Contains(userMsg.Content, "I should analyze") {
		t.Error("user message should contain the thinking event")
	}
}

func TestReadActiveContext_WithDisplayModes(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	mustExec(t, database, `INSERT INTO sessions (id, agent_name, model_id, status, goal)
		VALUES ('s1', 'agent', 'gpt-4o', 'idle', 'Test display modes')`)

	// Insert two memory events
	mustExec(t, database, `INSERT INTO memory_events (id, type, content, session_id, iteration_created)
		VALUES (1, 'text_block', 'Visible content', 's1', 1)`)
	mustExec(t, database, `INSERT INTO memory_events (id, type, content, summary_text, session_id, iteration_created)
		VALUES (2, 'text_block', 'Long content to compress', 'Summary of long content', 's1', 2)`)

	// Set display mode: event 2 is compressed
	mustExec(t, database, `INSERT INTO display_modes (memory_id, mode, session_id)
		VALUES (2, 'compressed', 's1')`)

	h := &Harness{db: database}
	ic, err := h.ReadActiveContext(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ReadActiveContext: %v", err)
	}

	userMsg := ic.Messages[1].Content

	// Visible content should appear
	if !strings.Contains(userMsg, "Visible content") {
		t.Error("expected 'Visible content' in user message")
	}
	// Compressed event should show summary text
	if !strings.Contains(userMsg, "Summary of long content") {
		t.Error("expected compressed summary text in user message")
	}
	// Compressed tag should appear
	if !strings.Contains(userMsg, "compressed") {
		t.Error("expected 'compressed' marker in user message")
	}
}

func TestReadActiveContext_HiddenEventsExcluded(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	mustExec(t, database, `INSERT INTO sessions (id, agent_name, model_id, status, goal)
		VALUES ('s1', 'agent', 'gpt-4o', 'idle', 'Test hidden')`)

	mustExec(t, database, `INSERT INTO memory_events (id, type, content, session_id, iteration_created)
		VALUES (1, 'text_block', 'Visible', 's1', 1)`)
	mustExec(t, database, `INSERT INTO memory_events (id, type, content, session_id, iteration_created)
		VALUES (2, 'system', 'Secret system note', 's1', 2)`)

	mustExec(t, database, `INSERT INTO display_modes (memory_id, mode, session_id)
		VALUES (2, 'hidden', 's1')`)

	h := &Harness{db: database}
	ic, err := h.ReadActiveContext(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ReadActiveContext: %v", err)
	}

	userMsg := ic.Messages[1].Content

	if !strings.Contains(userMsg, "Visible") {
		t.Error("expected 'Visible' in context")
	}
	if strings.Contains(userMsg, "Secret system note") {
		t.Error("hidden event should NOT appear in context")
	}
}

func TestReadActiveContext_WithTools(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	mustExec(t, database, `INSERT INTO sessions (id, agent_name, model_id, status, goal)
		VALUES ('s1', 'agent', 'gpt-4o', 'idle', 'Test tools')`)

	mustExec(t, database, `INSERT INTO tools_registry (id, name, description, hemisphere, enabled, status)
		VALUES ('t1', 'fetch_url', 'Fetch a URL', 'external', 1, 'active')`)
	mustExec(t, database, `INSERT INTO tools_registry (id, name, description, hemisphere, enabled, status)
		VALUES ('t2', 'write_memory', 'Write to memory ledger', 'internal', 1, 'active')`)
	mustExec(t, database, `INSERT INTO tools_registry (id, name, description, hemisphere, enabled, status)
		VALUES ('t3', 'disabled_tool', 'Should not appear', 'external', 0, 'active')`)
	mustExec(t, database, `INSERT INTO tools_registry (id, name, description, hemisphere, enabled, status)
		VALUES ('t4', 'deprecated_tool', 'Should not appear', 'external', 1, 'deprecated')`)

	h := &Harness{db: database}
	ic, err := h.ReadActiveContext(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ReadActiveContext: %v", err)
	}

	sysMsg := ic.Messages[0].Content
	userMsg := ic.Messages[1].Content

	// System prompt should list tools
	if !strings.Contains(sysMsg, "fetch_url") {
		t.Error("system prompt should mention fetch_url")
	}
	if !strings.Contains(sysMsg, "write_memory") {
		t.Error("system prompt should mention write_memory")
	}

	// User message should list tools
	if !strings.Contains(userMsg, "fetch_url") {
		t.Error("user message should list fetch_url")
	}
	if !strings.Contains(userMsg, "write_memory") {
		t.Error("user message should list write_memory")
	}

	// Disabled and deprecated should NOT appear
	if strings.Contains(sysMsg, "disabled_tool") {
		t.Error("system prompt should NOT mention disabled_tool")
	}
	if strings.Contains(userMsg, "deprecated_tool") {
		t.Error("user message should NOT mention deprecated_tool")
	}
}

func TestReadActiveContext_SystemPromptContainsRules(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	mustExec(t, database, `INSERT INTO sessions (id, agent_name, model_id, status, goal)
		VALUES ('s1', 'agent', 'gpt-4o', 'idle', 'Test rules')`)

	h := &Harness{db: database}
	ic, err := h.ReadActiveContext(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ReadActiveContext: %v", err)
	}

	sysMsg := ic.Messages[0].Content

	// System prompt must contain key rules
	rules := []string{
		"Consensus agent",
		"structured JSON",
		"memory_state_changes",
		"atomic transaction",
		"append-only",
	}
	for _, rule := range rules {
		if !strings.Contains(sysMsg, rule) {
			t.Errorf("system prompt missing rule: %q", rule)
		}
	}
}

func TestReadActiveContext_ConstraintsInMarkdown(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	mustExec(t, database, `INSERT INTO sessions (id, agent_name, model_id, status, goal)
		VALUES ('s1', 'agent', 'gpt-4o', 'idle', 'Test constraints')`)

	h := &Harness{db: database}
	ic, err := h.ReadActiveContext(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ReadActiveContext: %v", err)
	}

	userMsg := ic.Messages[1].Content

	// Constraints header should be present
	if !strings.Contains(userMsg, "## Constraints") {
		t.Error("missing Constraints section in context markdown")
	}
	if !strings.Contains(userMsg, "Iteration:") {
		t.Error("missing iteration constraint")
	}
	if !strings.Contains(userMsg, "Consecutive errors:") {
		t.Error("missing consecutive errors constraint")
	}
	if !strings.Contains(userMsg, "Budget:") {
		t.Error("missing budget constraint")
	}
}

func TestFormatMemoryEvent_Full(t *testing.T) {
	m := MemoryEventInfo{
		Type:    "text_block",
		Content: "The user asked about data",
		DisplayMode: "full",
		IterationCreated: 5,
	}

	result := formatMemoryEvent(m)
	if !strings.Contains(result, "The user asked about data") {
		t.Error("missing content in formatted output")
	}
	if !strings.Contains(result, "user asked") {
		t.Error("missing readable text in formatted output")
	}
}

func TestFormatMemoryEvent_Compressed(t *testing.T) {
	m := MemoryEventInfo{
		Type:        "text_block",
		Content:     "Very long content...",
		SummaryText: "Short summary",
		DisplayMode: "compressed",
		IterationCreated: 3,
	}

	result := formatMemoryEvent(m)
	if strings.Contains(result, "Very long content") {
		t.Error("compressed event should NOT show full content")
	}
	if !strings.Contains(result, "Short summary") {
		t.Error("compressed event should show summary text")
	}
}

func TestFormatMemoryEvent_Hidden(t *testing.T) {
	m := MemoryEventInfo{
		Type:        "system",
		Content:     "secret",
		DisplayMode: "hidden",
	}

	result := formatMemoryEvent(m)
	if result != "" {
		t.Errorf("hidden event should return empty string, got %q", result)
	}
}
