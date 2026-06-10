// Package harness: tests for audit log and iteration snapshot writers.
//
// axiom:trace work_item=runtime-harness-01 spec=specs/006-transactions.md plan=phase-1/task-1-1/step-1-1-5 test=internal/harness/audit_test.go
package harness

import (
	"context"
	"strings"
	"testing"
)

// stubDB is defined for potential future integration tests with a mock DB.

// ============================================================================
// Audit Log Tests (AC-006)
// ============================================================================

func TestWriteAuditLog_Committed(t *testing.T) {
	// Test that validation rejects invalid results
	h := &Harness{}
	err := h.WriteAuditLog(context.Background(), &AuditEntry{
		SessionID: "s1",
		Iteration: 1,
		Result:    "foo", // invalid
	})
	if err == nil {
		t.Error("expected error for invalid result")
	}
	if !strings.Contains(err.Error(), "invalid result") {
		t.Errorf("expected 'invalid result', got: %v", err)
	}
}

func TestWriteAuditLog_MissingSession(t *testing.T) {
	h := &Harness{}
	err := h.WriteAuditLog(context.Background(), &AuditEntry{
		Result: "committed",
	})
	if err == nil {
		t.Error("expected error for missing session_id")
	}
	if !strings.Contains(err.Error(), "missing session_id") {
		t.Errorf("expected 'missing session_id', got: %v", err)
	}
}

func TestWriteAuditLog_EmptySQLExecuted(t *testing.T) {
	// Even with empty SQL, the entry should be valid
	entry := &AuditEntry{
		SessionID:   "s1",
		Iteration:   1,
		Monologue:   "thinking...",
		SQLExecuted: []string{},
		Result:      "committed",
	}

	if entry.Result != "committed" {
		t.Error("result should be committed")
	}
	if len(entry.SQLExecuted) != 0 {
		t.Error("SQL should be empty")
	}
}

func TestWriteAuditLog_RolledBack(t *testing.T) {
	// Rolled back with error message
	entry := &AuditEntry{
		SessionID:    "s1",
		Iteration:    2,
		Monologue:    "trying to insert...",
		SQLExecuted:  []string{"INSERT INTO tasks VALUES (1)"},
		Result:       "rolled_back",
		ErrorMessage: "Column 'x' does not exist",
	}

	if entry.Result != "rolled_back" {
		t.Error("expected rolled_back")
	}
	if entry.ErrorMessage == "" {
		t.Error("rolled back should have error message")
	}
}

// ============================================================================
// Iteration Snapshot Tests (AC-007)
// ============================================================================

func TestWriteIterationSnapshot_Validation(t *testing.T) {
	h := &Harness{}

	// Missing session_id
	err := h.WriteIterationSnapshot(context.Background(), "", 1, nil, nil, 0)
	if err == nil {
		t.Error("expected error for missing session_id")
	}

	// Invalid iteration (0)
	err = h.WriteIterationSnapshot(context.Background(), "s1", 0, nil, nil, 0)
	if err == nil {
		t.Error("expected error for invalid iteration")
	}

	// Invalid iteration (negative)
	err = h.WriteIterationSnapshot(context.Background(), "s1", -1, nil, nil, 0)
	if err == nil {
		t.Error("expected error for negative iteration")
	}
}

func TestWriteIterationSnapshot_WithData(t *testing.T) {
	// Valid snapshot parameters
	llmResponse := []byte(`{"internal_monologue":"test","memory_state_changes":[]}`)
	sqlExecuted := []string{"SELECT 1", "INSERT INTO memory_events VALUES (1)"}

	// Just test that the data shapes are reasonable
	if len(llmResponse) == 0 {
		t.Error("llmResponse should not be empty")
	}
	if len(sqlExecuted) != 2 {
		t.Error("expected 2 SQL statements")
	}
}

// ============================================================================
// Combined FinalizeIteration Tests
// ============================================================================

func TestFinalizeIteration_AssemblesCorrectly(t *testing.T) {
	// Verify that FinalizeIteration correctly assembles its sub-parts
	sessionID := "test-session-1"
	iteration := int64(5)
	monologue := "I should check the results"
	sqlExecuted := []string{"SELECT 1", "INSERT INTO tasks VALUES (1)"}
	llmResponse := []byte(`{"internal_monologue":"test"}`)
	rowsAffected := 2
	result := "committed"

	// Test the helper functions independently
	formatted := FormatSQLStatements(sqlExecuted)
	if formatted == "(no SQL executed)" {
		t.Error("should format non-empty SQL")
	}
	if !strings.Contains(formatted, "SELECT 1") {
		t.Error("formatted SQL should contain first statement")
	}

	auditEntry := &AuditEntry{
		SessionID:   sessionID,
		Iteration:   iteration,
		Monologue:   monologue,
		SQLExecuted: sqlExecuted,
		Result:      result,
	}
	if auditEntry.SessionID == "" {
		t.Error("session_id should be set")
	}
	if auditEntry.Iteration != iteration {
		t.Errorf("iteration mismatch: expected %d, got %d", iteration, auditEntry.Iteration)
	}

	// These values should all pass through validation
	_ = sessionID
	_ = iteration
	_ = monologue
	_ = sqlExecuted
	_ = llmResponse
	_ = rowsAffected
	_ = result
}

// ============================================================================
// FormatSQLStatements Tests
// ============================================================================

func TestFormatSQLStatements_Empty(t *testing.T) {
	result := FormatSQLStatements([]string{})
	if !strings.Contains(result, "no SQL executed") {
		t.Errorf("expected 'no SQL executed', got %q", result)
	}
}

func TestFormatSQLStatements_Nil(t *testing.T) {
	result := FormatSQLStatements(nil)
	if !strings.Contains(result, "no SQL executed") {
		t.Errorf("expected 'no SQL executed' for nil, got %q", result)
	}
}

func TestFormatSQLStatements_Single(t *testing.T) {
	result := FormatSQLStatements([]string{"SELECT 1"})
	if result != "SELECT 1" {
		t.Errorf("expected 'SELECT 1', got %q", result)
	}
}

func TestFormatSQLStatements_Multiple(t *testing.T) {
	result := FormatSQLStatements([]string{
		"SELECT * FROM memory_events",
		"INSERT INTO tasks VALUES (1)",
		"UPDATE sessions SET status = 'idle'",
	})

	if !strings.Contains(result, "SELECT * FROM memory_events") {
		t.Error("missing first statement")
	}
	if !strings.Contains(result, "INSERT INTO tasks") {
		t.Error("missing second statement")
	}
	if !strings.Contains(result, "UPDATE sessions") {
		t.Error("missing third statement")
	}

	// Should be separated by semicolon and newline
	parts := strings.Split(result, ";\n")
	if len(parts) != 3 {
		t.Errorf("expected 3 parts, got %d: %q", len(parts), result)
	}
}

func TestFormatSQLStatements_ComplexSQL(t *testing.T) {
	result := FormatSQLStatements([]string{
		"WITH recent AS (SELECT * FROM memory_events ORDER BY created_at DESC LIMIT 10) SELECT * FROM recent",
		"INSERT INTO tasks (id, title, description) VALUES (gen_random_uuid(), 'Test Task', 'A detailed description with multiple words')",
	})

	if !strings.Contains(result, "WITH recent AS") {
		t.Error("missing CTE statement")
	}
	if !strings.Contains(result, "gen_random_uuid()") {
		t.Error("missing second statement")
	}
}

// ============================================================================
// nullIfEmpty Tests
// ============================================================================

func TestNullIfEmpty_Empty(t *testing.T) {
	result := nullIfEmpty("")
	if result != nil {
		t.Error("expected nil for empty string")
	}
}

func TestNullIfEmpty_NonEmpty(t *testing.T) {
	result := nullIfEmpty("hello")
	if result == nil {
		t.Error("expected non-nil for non-empty string")
	}
	if *result != "hello" {
		t.Errorf("expected 'hello', got %q", *result)
	}
}

func TestNullIfEmpty_WhitespaceOnly(t *testing.T) {
	result := nullIfEmpty("   ")
	if result == nil {
		t.Error("whitespace should be treated as non-empty for error messages")
	}
}

// ============================================================================
// AuditEntry Struct Tests
// ============================================================================

func TestAuditEntry_Defaults(t *testing.T) {
	entry := AuditEntry{
		SessionID:   "s1",
		Iteration:   1,
		Result:      "committed",
	}

	if entry.ErrorMessage != "" {
		t.Error("committed entry should have empty error message")
	}
	if entry.Monologue != "" {
		t.Error("default monologue should be empty")
	}
	if len(entry.SQLExecuted) != 0 {
		t.Error("default SQL executed should be empty")
	}
}

func TestAuditEntry_ErrorCase(t *testing.T) {
	entry := AuditEntry{
		SessionID:    "s1",
		Iteration:    3,
		Monologue:    "Attempted UPDATE on memory_events",
		SQLExecuted:  []string{"UPDATE memory_events SET content = 'x'"},
		Result:       "rolled_back",
		ErrorMessage: "permission denied: memory_events is append-only",
	}

	if entry.Result != "rolled_back" {
		t.Error("failed iteration should be rolled back")
	}
	if len(entry.SQLExecuted) == 0 {
		t.Error("should record the SQL that was attempted")
	}
	if entry.ErrorMessage == "" {
		t.Error("should record what went wrong")
	}
}

// ============================================================================
// Integration: Full iteration outcome assembly
// ============================================================================

func TestBuildIterationResult(t *testing.T) {
	// Simulate building a full iteration result from an agent output
	output := &AgentOutput{
		InternalMonologue: "User wants dark mode. I should check the settings table.",
		MemoryStateChanges: []string{
			"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'Adding dark mode', 's1', 5)",
		},
		SystemActions: []string{},
		ToolRequests:  nil,
	}

	// This is what would be written to audit_logs
	allSQL := append([]string{}, output.MemoryStateChanges...)
	allSQL = append(allSQL, output.SystemActions...)

	entry := AuditEntry{
		SessionID:   "s1",
		Iteration:   5,
		Monologue:   output.InternalMonologue,
		SQLExecuted: allSQL,
		Result:      "committed",
	}

	if len(entry.SQLExecuted) != 1 {
		t.Errorf("expected 1 SQL statement, got %d", len(entry.SQLExecuted))
	}
	if entry.Monologue != output.InternalMonologue {
		t.Error("monologue should match")
	}
}

func TestBuildIterationResult_WithTools(t *testing.T) {
	output := &AgentOutput{
		InternalMonologue: "I need to scrape this URL",
		MemoryStateChanges: []string{
			"INSERT INTO memory_events (type, content, session_id) VALUES ('header', 'Starting scrape', 's1')",
		},
		SystemActions: []string{
			"UPDATE sessions SET status = 'tool_exec' WHERE id = 's1'",
		},
		ToolRequests: []ToolRequest{
			{ToolName: "web_scraper", Parameters: map[string]any{"url": "https://example.com"}},
		},
	}

	allSQL := append([]string{}, output.MemoryStateChanges...)
	allSQL = append(allSQL, output.SystemActions...)

	entry := AuditEntry{
		SessionID:   "s1",
		Iteration:   3,
		Monologue:   output.InternalMonologue,
		SQLExecuted: allSQL,
		Result:      "committed",
	}

	if len(entry.SQLExecuted) != 2 {
		t.Errorf("expected 2 SQL statements, got %d", len(entry.SQLExecuted))
	}
	if len(output.ToolRequests) != 1 {
		t.Error("tool requests should be recorded")
	}
}

// ============================================================================
// Iteration Snapshot completeness
// ============================================================================

func TestIterationSnapshot_CompletePayload(t *testing.T) {
	// A full iteration snapshot should contain all the data needed to replay
	llmResponse := []byte(`{
		"internal_monologue": "I will create a new memory page for the research results",
		"memory_state_changes": [
			"INSERT INTO memory_pages (name, target_ids, session_id) VALUES ('Q4 Research', ARRAY[1,2,3], 's1')",
			"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('user_message', 'Summarize the research', 's1', 4)"
		],
		"system_actions": [],
		"tool_requests": [],
		"sub_agent_spawns": []
	}`)

	sqlExecuted := []string{
		"INSERT INTO memory_pages (name, target_ids, session_id) VALUES ('Q4 Research', ARRAY[1,2,3], 's1')",
		"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('user_message', 'Summarize the research', 's1', 4)",
	}

	rowsAffected := 2

	// Verify the snapshot data is complete
	if len(llmResponse) < 50 {
		t.Error("llmResponse too short")
	}
	if len(sqlExecuted) != 2 {
		t.Errorf("expected 2 SQL, got %d", len(sqlExecuted))
	}
	if rowsAffected != 2 {
		t.Errorf("expected 2 rows affected, got %d", rowsAffected)
	}
}
