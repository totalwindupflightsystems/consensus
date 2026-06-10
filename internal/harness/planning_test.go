// Package harness: tests for interactive multi-turn planning (SPEC-020).
//
// V2 tests — aligned with spec-020-hardening-01 rewrite: single long-running
// transaction, DB-persisted staging buffer, command type system, respond action.
//
// axiom:trace work_item=spec-020-hardening-01 spec=specs/020-multi-turn-planning.md plan=phase-1~6 test=internal/harness/planning_test.go
package harness

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Staging Buffer Unit Tests
// ============================================================================

func TestStagingBuffer_Empty_V2(t *testing.T) {
	buf := &StagingBuffer{IsActive: true}
	if len(buf.Entries) != 0 {
		t.Error("new buffer should be empty")
	}
	if !buf.IsActive {
		t.Error("new buffer should be active")
	}
}

func TestStagingEntry_Fields(t *testing.T) {
	raw := json.RawMessage(`{"rows": 3}`)
	entry := &StagingEntry{
		ID:          1,
		SessionID:   "sess-1",
		Turn:        2,
		Seq:         1,
		CmdType:     CmdSQL,
		Description: "Insert row",
		Status:      BufferExecuted,
		Result:      &raw,
	}
	if entry.CmdType != CmdSQL {
		t.Error("cmd_type should be sql")
	}
	if entry.Description == "" {
		t.Error("description should be set")
	}
	if entry.Status != BufferExecuted {
		t.Error("status should be executed")
	}
}

func TestStagingEntry_Payload(t *testing.T) {
	entry := &StagingEntry{
		CmdType: CmdSQL,
		Payload: []byte("SELECT * FROM users"),
		Status:  BufferStaged,
	}
	if string(entry.Payload) != "SELECT * FROM users" {
		t.Errorf("payload = %s, expected SQL", string(entry.Payload))
	}
	if entry.Executed {
		t.Error("staged entry should not be executed")
	}
}

func TestStagingBuffer_StatusLifecycle_V2(t *testing.T) {
	statuses := []BufferStatus{
		BufferStaged, BufferExecuted, BufferCommitted,
		BufferRolledBack, BufferFailed,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("status should not be empty")
		}
	}
}

// ============================================================================
// Command Type Tests (HARDEN-PLAN-04)
// ============================================================================

func TestCmdType_AllTypes(t *testing.T) {
	types := []CmdType{CmdSQL, CmdFileWrite, CmdFileEdit, CmdFileDelete, CmdMemoryWrite, CmdToolCallRef}
	seen := map[CmdType]bool{}
	for _, ct := range types {
		if ct == "" {
			t.Error("cmd type should not be empty")
		}
		seen[ct] = true
	}
	if len(seen) != 6 {
		t.Errorf("expected 6 unique cmd types, got %d", len(seen))
	}
}

func TestCmdType_StringValues(t *testing.T) {
	if string(CmdSQL) != "sql" {
		t.Errorf("CmdSQL = %q, want sql", CmdSQL)
	}
	if string(CmdMemoryWrite) != "memory_write" {
		t.Errorf("CmdMemoryWrite = %q, want memory_write", CmdMemoryWrite)
	}
}

// ============================================================================
// Turn Formatting V2 Tests (AC-021)
// ============================================================================

func TestFormatTurnContextV2_Basic(t *testing.T) {
	var h Harness
	ic := &IterationContext{Goal: "Test goal", Status: "planning", Iteration: 5}
	buf := &StagingBuffer{IsActive: true}
	cfg := DefaultPlanningConfig()

	result := h.formatTurnContextV2(ic, buf, 3, cfg, nil)
	if !strings.Contains(result, "Transaction Window") {
		t.Error("turn context should include 'Transaction Window'")
	}
	if !strings.Contains(result, "stage_and_execute") {
		t.Error("turn context should list available actions")
	}
	if !strings.Contains(result, "Test goal") {
		t.Error("turn context should include the goal")
	}
}

func TestFormatTurnContextV2_WithMemoryChanges(t *testing.T) {
	var h Harness
	ic := &IterationContext{Goal: "Test", Status: "planning", Iteration: 5}
	buf := &StagingBuffer{IsActive: true}
	cfg := DefaultPlanningConfig()

	result := h.formatTurnContextV2(ic, buf, 2, cfg, []string{"INSERT INTO memory_events VALUES (1)"})
	if !strings.Contains(result, "Pending Memory Changes") {
		t.Error("should show pending memory changes")
	}
}

func TestFormatBufferStateV2_Empty(t *testing.T) {
	var h Harness
	buf := &StagingBuffer{}
	result := h.formatBufferStateV2(buf)
	if !strings.Contains(result, "empty") {
		t.Error("empty buffer should show '(empty)'")
	}
}

func TestFormatBufferStateV2_WithCommands(t *testing.T) {
	var h Harness
	buf := &StagingBuffer{
		Entries: []*StagingEntry{
			{CmdType: CmdSQL, Description: "Insert row", Status: BufferExecuted, Turn: 1},
			{CmdType: CmdFileWrite, Description: "Write config.json", Status: BufferStaged, Turn: 2},
			{CmdType: CmdSQL, Description: "Bad query", Status: BufferFailed, Turn: 3},
		},
	}
	result := h.formatBufferStateV2(buf)
	if !strings.Contains(result, "✓") {
		t.Error("executed entry should have checkmark")
	}
	if !strings.Contains(result, "✗") {
		t.Error("failed entry should have X mark")
	}
	if strings.Contains(result, "empty") {
		t.Error("non-empty buffer should not show '(empty)'")
	}
}

func TestFormatBufferStateV2_WithResult(t *testing.T) {
	var h Harness
	raw := json.RawMessage(`{"rows": 5}`)
	buf := &StagingBuffer{
		Entries: []*StagingEntry{
			{CmdType: CmdSQL, Description: "Query", Status: BufferExecuted, Turn: 1, Result: &raw},
		},
	}
	result := h.formatBufferStateV2(buf)
	if !strings.Contains(result, "Result:") {
		t.Error("should show result when present")
	}
}

func TestFormatBufferStateV2_RolledBack(t *testing.T) {
	var h Harness
	buf := &StagingBuffer{
		Entries: []*StagingEntry{
			{CmdType: CmdSQL, Description: "Reverted", Status: BufferRolledBack, Turn: 1},
		},
	}
	result := h.formatBufferStateV2(buf)
	if !strings.Contains(result, "↩") {
		t.Error("rolled back entry should have rollback symbol")
	}
}

// ============================================================================
// Plan Action Dispatch V2 Tests (HARDEN-PLAN-03)
// ============================================================================

func TestOutputToTurnPlanV2_StageExec(t *testing.T) {
	var h Harness
	output := &AgentOutput{
		InternalMonologue: "I will insert a memory event",
		MemoryStateChanges: []string{
			"INSERT INTO memory_events (type, content) VALUES ('text_block', 'done')",
		},
	}

	plan := h.outputToTurnPlanV2(output)

	if plan.Action != ActionStageExec {
		t.Errorf("action = %s, want stage_and_execute", plan.Action)
	}
	if len(plan.StagedCommands) != 1 {
		t.Errorf("expected 1 staged command, got %d", len(plan.StagedCommands))
	}
	if plan.StagedCommands[0].CmdType != CmdSQL {
		t.Errorf("cmd_type = %s, want sql", plan.StagedCommands[0].CmdType)
	}
	if plan.Monologue != "I will insert a memory event" {
		t.Errorf("monologue mismatch")
	}
}

func TestOutputToTurnPlanV2_ToolCall(t *testing.T) {
	var h Harness
	output := &AgentOutput{
		InternalMonologue: "Let me scrape that page",
		ToolRequests: []ToolRequest{
			{ToolName: "web_scraper", Parameters: map[string]any{"url": "https://example.com"}},
		},
	}

	plan := h.outputToTurnPlanV2(output)
	if plan.Action != ActionToolCall {
		t.Errorf("action = %s, want tool_call", plan.Action)
	}
	if len(plan.ToolRequests) != 1 {
		t.Errorf("expected 1 tool request, got %d", len(plan.ToolRequests))
	}
}

func TestOutputToTurnPlanV2_ToolCallPriority(t *testing.T) {
	var h Harness
	output := &AgentOutput{
		InternalMonologue:  "Need to fetch data first",
		ToolRequests:       []ToolRequest{{ToolName: "fetch", Parameters: map[string]any{"url": "x"}}},
		MemoryStateChanges: []string{"INSERT INTO mem VALUES (1)"},
		SystemActions:      []string{"commit"},
	}

	plan := h.outputToTurnPlanV2(output)
	if plan.Action != ActionToolCall {
		t.Errorf("action = %s, want tool_call (takes priority)", plan.Action)
	}
	if len(plan.MemoryStateChanges) == 0 {
		t.Error("memory_state_changes should be preserved with tool_call")
	}
}

func TestOutputToTurnPlanV2_CommitIntent(t *testing.T) {
	var h Harness
	output := &AgentOutput{
		InternalMonologue:  "All done, committing",
		SystemActions:      []string{"commit"},
		MemoryStateChanges: []string{"INSERT INTO audit VALUES (1)"},
	}

	plan := h.outputToTurnPlanV2(output)
	if plan.Action != ActionCommit {
		t.Errorf("action = %s, want commit", plan.Action)
	}
	if len(plan.MemoryStateChanges) != 1 {
		t.Errorf("expected 1 memory state change, got %d", len(plan.MemoryStateChanges))
	}
}

func TestOutputToTurnPlanV2_RollbackIntent(t *testing.T) {
	var h Harness
	output := &AgentOutput{
		InternalMonologue: "This approach is wrong, rolling back",
		SystemActions:     []string{"rollback"},
	}

	plan := h.outputToTurnPlanV2(output)
	if plan.Action != ActionRollback {
		t.Errorf("action = %s, want rollback", plan.Action)
	}
}

func TestOutputToTurnPlanV2_RollbackWithEnd(t *testing.T) {
	var h Harness
	output := &AgentOutput{
		InternalMonologue: "This isn't working, giving up",
		SystemActions:     []string{"rollback end_iteration"},
	}

	plan := h.outputToTurnPlanV2(output)
	if plan.Action != ActionRollback {
		t.Errorf("action = %s, want rollback", plan.Action)
	}
	if !plan.EndIteration {
		t.Error("should set end_iteration when 'end' is in the rollback action")
	}
}

func TestOutputToTurnPlanV2_Respond(t *testing.T) {
	var h Harness
	output := &AgentOutput{
		InternalMonologue: "I need to ask the user a question",
		SystemActions:     []string{"respond"},
	}

	plan := h.outputToTurnPlanV2(output)
	if plan.Action != ActionRespond {
		t.Errorf("action = %s, want respond", plan.Action)
	}
}

func TestOutputToTurnPlanV2_NoOp(t *testing.T) {
	var h Harness
	output := &AgentOutput{
		InternalMonologue: "Let me think about this...",
	}

	plan := h.outputToTurnPlanV2(output)
	if plan.Action != ActionNoOp {
		t.Errorf("action = %s, want no_op", plan.Action)
	}
}

// ============================================================================
// Default Planning Config Tests (SPEC-020 §11)
// ============================================================================

func TestDefaultPlanningConfig_V2(t *testing.T) {
	cfg := DefaultPlanningConfig()
	if cfg == nil {
		t.Fatal("DefaultPlanningConfig should not return nil")
	}
	if cfg.MaxTurns != 10 {
		t.Errorf("max_turns = %d, want 10", cfg.MaxTurns)
	}
	if cfg.MaxRollbacks != 3 {
		t.Errorf("max_rollbacks = %d, want 3", cfg.MaxRollbacks)
	}
	if cfg.Timeout != 180*time.Second {
		t.Errorf("timeout = %s, want 180s (increased from 60s for local LLMs like LM Studio)", cfg.Timeout)
	}
	if cfg.MaxStagedCommands != 50 {
		t.Errorf("max_staged_commands = %d, want 50", cfg.MaxStagedCommands)
	}
	if !cfg.AutoCommitOnMax {
		t.Error("auto_commit_on_max should be true (SPEC-020 §11)")
	}
}

// ============================================================================
// SQL Truncation Tests
// ============================================================================

func TestTruncateSQL_V2(t *testing.T) {
	tests := []struct {
		sql    string
		maxLen int
		expect string
	}{
		{"SELECT 1", 100, "SELECT 1"},
		{"SELECT * FROM very_long_table_name_that_exceeds_limit", 20, "SELECT * FROM ver..."},
		{"", 10, ""},
	}
	for _, tt := range tests {
		result := truncateSQL(tt.sql, tt.maxLen)
		if result != tt.expect {
			t.Errorf("truncateSQL(%q, %d) = %q, want %q", tt.sql, tt.maxLen, result, tt.expect)
		}
	}
}

func TestTruncateJSON(t *testing.T) {
	raw := json.RawMessage(`{"very_long_key": "and a long value that needs truncation beyond 30 chars"}`)
	result := truncateJSON(raw, 30)
	if len(result) > 30 {
		t.Errorf("truncated json = %d chars, want <= 30", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("truncated json should end with '...'")
	}

	short := json.RawMessage(`"short"`)
	result = truncateJSON(short, 100)
	if string(short) != result {
		t.Error("short json should not be truncated")
	}
}

// ============================================================================
// Context + Plan Action Timing
// ============================================================================

func TestPlanningTimeout_ContextExpiry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)

	select {
	case <-ctx.Done():
		// expected
	default:
		t.Error("context should have expired")
	}
}

// ============================================================================
// Action Type Enumeration Tests
// ============================================================================

func TestPlanAction_AllValues(t *testing.T) {
	actions := []PlanAction{
		ActionStageExec, ActionStageOnly, ActionToolCall,
		ActionCommit, ActionRollback, ActionRespond, ActionNoOp,
	}
	seen := map[PlanAction]bool{}
	for _, a := range actions {
		if a == "" {
			t.Error("action should not be empty")
		}
		seen[a] = true
	}
	// We now have 7 actions (added respond)
	if len(seen) != 7 {
		t.Errorf("expected 7 unique actions, got %d", len(seen))
	}
}

func TestBufferStatus_AllValues(t *testing.T) {
	statuses := []BufferStatus{
		BufferStaged, BufferExecuted, BufferCommitted,
		BufferRolledBack, BufferFailed,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("buffer status should not be empty")
		}
	}
}

func TestContainsWord(t *testing.T) {
	tests := []struct {
		s, word string
		expect  bool
	}{
		{"commit", "commit", true},
		{"COMMIT", "commit", true},
		{"rollback end", "end", true},
		{"stage", "commit", false},
		{"respond with msg", "respond", true},
	}
	for _, tt := range tests {
		result := containsWord(tt.s, tt.word)
		if result != tt.expect {
			t.Errorf("containsWord(%q, %q) = %v, want %v", tt.s, tt.word, result, tt.expect)
		}
	}
}

func TestToLower(t *testing.T) {
	tests := []struct{ in, expect string }{
		{"HELLO", "hello"},
		{"Commit", "commit"},
		{"already", "already"},
		{"", ""},
	}
	for _, tt := range tests {
		result := toLower(tt.in)
		if result != tt.expect {
			t.Errorf("toLower(%q) = %q, want %q", tt.in, result, tt.expect)
		}
	}
}

func TestIndexOf(t *testing.T) {
	if idx := indexOf("hello world", "world"); idx != 6 {
		t.Errorf("indexOf = %d, want 6", idx)
	}
	if idx := indexOf("hello", "x"); idx != -1 {
		t.Errorf("indexOf not-found = %d, want -1", idx)
	}
}

func TestFormatSQLList(t *testing.T) {
	result := formatSQLList([]string{"SELECT 1", "INSERT INTO x VALUES (1)"})
	if !strings.Contains(result, "[1]") || !strings.Contains(result, "[2]") {
		t.Error("should show numbered list")
	}
}
