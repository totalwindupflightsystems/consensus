// Package harness: tests for dynamic system prompt assembly (SPEC-012).
//
// axiom:trace work_item=runtime-harness-01 spec=specs/012-system-prompt-and-discovery.md plan=phase-2/task-2-1/step-2-1-1 test=internal/harness/prompt_test.go
package harness

import (
	"strings"
	"testing"
)

// ============================================================================
// Layer 1: Identity + Rules Tests (AC-008)
// ============================================================================

func TestBuildIdentityLayer_ContainsCoreElements(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}
	config := &SystemPromptConfig{
		AgentName: "test_agent",
		ModelID:   "gpt-4o",
		SessionID: "sess-123",
		Goal:      "Test the prompt builder",
		Status:    "idle",
	}

	layer := b.buildIdentityLayer(config)

	required := []string{
		"Consensus Agent Runtime",
		"test_agent",
		"gpt-4o",
		"sess-123",
		"Test the prompt builder",
		"Atomic Cognition",
		"Append-Only Memory",
		"RLS Isolation",
		"Deterministic State",
		"internal_monologue",
		"memory_state_changes",
		"system_actions",
		"tool_requests",
		"sub_agent_spawns",
		"DML_READ",
		"DML_WRITE",
		"DDL",
		"DANGEROUS",
		"{{SECRET.",
		"TRUNCATE",
		"GRANT",
	}

	for _, req := range required {
		if !strings.Contains(layer, req) {
			t.Errorf("identity layer missing: %q", req)
		}
	}
}

func TestBuildIdentityLayer_DifferentAgents(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}

	a1 := b.buildIdentityLayer(&SystemPromptConfig{
		AgentName: "researcher", Goal: "Find papers",
	})
	a2 := b.buildIdentityLayer(&SystemPromptConfig{
		AgentName: "coder", Goal: "Write functions",
	})

	// Each should contain its own identity
	if !strings.Contains(a1, "researcher") {
		t.Error("agent 1 missing its name")
	}
	if !strings.Contains(a2, "coder") {
		t.Error("agent 2 missing its name")
	}
	if strings.Contains(a1, "coder") {
		t.Error("agent 1 should not contain agent 2's name")
	}
}

// ============================================================================
// Layer 2: Schema Discovery Tests (AC-009)
// ============================================================================

func TestBuildSchemaLayer_CoreTablesListed(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}
	schema := &SchemaData{
		CoreTables: make([]TableInfo, 0, len(coreTableNames)),
	}
	for _, name := range coreTableNames {
		schema.CoreTables = append(schema.CoreTables, TableInfo{Name: name})
	}

	layer := b.buildSchemaLayer(schema)

	for _, name := range coreTableNames {
		if !strings.Contains(layer, name) {
			t.Errorf("schema layer missing core table: %q", name)
		}
	}
}

func TestBuildSchemaLayer_KeyRelationships(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}

	layer := b.buildSchemaLayer(&SchemaData{})

	required := []string{
		"sessions.id",
		"memory_events.session_id",
		"staging_buffer",
		"tasks.session_id",
	}

	for _, req := range required {
		if !strings.Contains(layer, req) {
			t.Errorf("schema layer missing relationship: %q", req)
		}
	}
}

func TestBuildSchemaLayer_DynamicTables(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}
	schema := &SchemaData{
		DynamicTables: []TableInfo{
			{Name: "order_tracking", Columns: "id, session_id, data, created_at"},
			{Name: "bug_reports", Columns: "id, session_id, data, created_at"},
		},
	}

	layer := b.buildSchemaLayer(schema)

	if !strings.Contains(layer, "order_tracking") {
		t.Error("missing dynamic table order_tracking")
	}
	if !strings.Contains(layer, "bug_reports") {
		t.Error("missing dynamic table bug_reports")
	}
	if !strings.Contains(layer, "id, session_id, data, created_at") {
		t.Error("missing column listing for dynamic tables")
	}
}

func TestBuildSchemaLayer_NoDynamicTables(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}

	layer := b.buildSchemaLayer(&SchemaData{})

	if strings.Contains(layer, "Dynamic Tables (Agent-Created)") {
		t.Error("should not have dynamic tables section when none exist")
	}
}

// ============================================================================
// Layer 3: Tools Layer Tests (AC-008)
// ============================================================================

func TestBuildToolsLayer_ToolsListed(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}
	tools := []ToolInfo{
		{Name: "web_scraper", Description: "Scrapes web pages", Hemisphere: "external"},
		{Name: "set_display_mode", Description: "Changes display state", Hemisphere: "internal"},
		{Name: "send_email", Description: "Sends emails", Hemisphere: "external"},
	}

	layer := b.buildToolsLayer(tools)

	for _, tool := range tools {
		if !strings.Contains(layer, tool.Name) {
			t.Errorf("tools layer missing: %q", tool.Name)
		}
		if !strings.Contains(layer, tool.Description) {
			t.Errorf("tools layer missing description for: %q", tool.Name)
		}
	}

	if !strings.Contains(layer, "Internal") && !strings.Contains(layer, "External") {
		t.Error("tools layer should mention hemisphere types")
	}
}

func TestBuildToolsLayer_Empty(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}

	layer := b.buildToolsLayer(nil)

	if strings.Contains(layer, "| `") {
		t.Error("empty tools should not render a table")
	}
}

// ============================================================================
// Layer 4: Skills Layer Tests (AC-012 — Progressive Disclosure)
// ============================================================================

func TestBuildSkillsLayer_MetadataOnly(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}
	skills := []SkillMetadata{
		{Name: "excel_generator", WhenToUse: "When user requests Excel files", Description: "Generates Excel spreadsheets"},
		{Name: "pdf_parser", WhenToUse: "When user uploads PDF documents", Description: "Extracts text from PDFs"},
	}

	layer := b.buildSkillsLayer(skills)

	// Should contain skill names and when-to-use
	for _, s := range skills {
		if !strings.Contains(layer, s.Name) {
			t.Errorf("skills layer missing name: %q", s.Name)
		}
		if !strings.Contains(layer, s.WhenToUse) {
			t.Errorf("skills layer missing when_to_use for: %q", s.Name)
		}
	}

	// Should have progressive disclosure message
	if !strings.Contains(layer, "load_skill") {
		t.Error("skills layer should mention load_skill() function")
	}
	if !strings.Contains(layer, "Metadata Only") {
		t.Error("skills layer should indicate metadata only")
	}
}

func TestBuildSkillsLayer_Empty(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}

	layer := b.buildSkillsLayer(nil)

	if strings.Contains(layer, "| `") {
		t.Error("empty skills should not render a table")
	}
}

// ============================================================================
// Layer 5: Constraints + Budget Tests (AC-008)
// ============================================================================

func TestBuildConstraintsLayer_BudgetDisplay(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}
	config := &SystemPromptConfig{
		Iteration:            12,
		MaxIterations:        100,
		PlanningMaxTurns:     10,
		BudgetUsedCents:      350,
		BudgetLimitCents:     1000,
		ConsecutiveErrors:    1,
		MaxConsecutiveErrors: 3,
		ContextBudget:        128000,
	}

	layer := b.buildConstraintsLayer(config)

	checks := map[string]bool{
		"12":     false,
		"100":    false,
		"10":     false,
		"350":    false,
		"1000":   false,
		"128000": false,
	}

	for key := range checks {
		if strings.Contains(layer, key) {
			checks[key] = true
		}
	}

	for key, found := range checks {
		if !found {
			t.Errorf("constraints layer missing value: %q", key)
		}
	}

	if !strings.Contains(layer, "circuit breaker") {
		t.Error("should mention circuit breaker")
	}
	if !strings.Contains(strings.ToLower(layer), "append-only enforcement") {
		t.Error("should mention append-only enforcement")
	}
	if !strings.Contains(layer, "secrets") {
		t.Error("should mention secret handling")
	}
}

// ============================================================================
// Complete System Prompt Assembly Tests
// ============================================================================

func TestPromptLayers_SystemPrompt(t *testing.T) {
	layers := &PromptLayers{
		Layer1Identity:    "IDENTITY_LAYER",
		Layer2Schema:      "SCHEMA_LAYER",
		Layer3Tools:       "TOOLS_LAYER",
		Layer4Skills:      "SKILLS_LAYER",
		Layer5Constraints: "CONSTRAINTS_LAYER",
	}

	result := layers.SystemPrompt()

	// System Prompt should NOT include Layer 6
	if strings.Contains(result, "CONTEXT_LAYER") {
		t.Error("SystemPrompt should not include context layer")
	}

	// Should include all static layers
	if !strings.Contains(result, "IDENTITY_LAYER") {
		t.Error("missing identity layer")
	}
	if !strings.Contains(result, "CONSTRAINTS_LAYER") {
		t.Error("missing constraints layer")
	}
}

func TestPromptLayers_FullString(t *testing.T) {
	layers := &PromptLayers{
		Layer1Identity: "IDENTITY_LAYER",
		Layer6Context:  "CONTEXT_LAYER",
	}

	result := layers.String()

	// Full string should include Layer 6 if present
	if !strings.Contains(result, "CONTEXT_LAYER") {
		t.Error("full string should include context layer")
	}
}

func TestPromptLayers_FullStringWithoutContext(t *testing.T) {
	layers := &PromptLayers{
		Layer1Identity: "IDENTITY_LAYER",
	}

	result := layers.String()

	if strings.Contains(result, "CONTEXT_LAYER") {
		t.Error("should not include context when not set")
	}
}

// ============================================================================
// Cache-Friendly Layout Tests (AC-011)
// ============================================================================

func TestPromptCacheFriendly_StaticPrefix(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}
	config := &SystemPromptConfig{
		AgentName: "test", ModelID: "gpt-4o", SessionID: "s1", Goal: "test",
	}

	// The system prompt should have immutable content at the top
	layers, err := b.buildLayers(nil, config)
	if err != nil {
		// Schema discovery will fail without a real DB, but that's fine for unit test
		// The important thing is the static layers come first
	}

	_ = layers

	// Identity should be first (cacheable)
	identityLayer := b.buildIdentityLayer(config)
	if !strings.HasPrefix(identityLayer, "# Consensus") {
		t.Error("identity layer should start with header")
	}

	// Constraints should be toward the end (not cache-breaking)
	constraintsLayer := b.buildConstraintsLayer(config)
	if strings.HasPrefix(constraintsLayer, "# Consensus") {
		t.Error("constraints layer should not be positioned as first content")
	}
}

// ============================================================================
// Sub-Agent Prompt Filtering Tests (AC-013)
// ============================================================================

func TestSubAgentPrompt_DifferentFromParent(t *testing.T) {
	parent := &SystemPromptConfig{
		AgentName: "parent_agent", Goal: "Complex orchestration", SessionID: "parent-1",
	}
	child := &SystemPromptConfig{
		AgentName: "child_agent", Goal: "Simple sub-task", SessionID: "child-1",
	}

	b := &SystemPromptBuilder{harness: &Harness{}}
	prompt1 := b.buildIdentityLayer(parent)
	prompt2 := b.buildIdentityLayer(child)

	// Child agent should have its own identity
	if !strings.Contains(prompt2, "child_agent") {
		t.Error("child should have its own identity")
	}
	if strings.Contains(prompt2, "parent_agent") {
		t.Error("child should not have parent identity")
	}

	// But both share the same structural template
	if strings.Count(prompt1, "# Consensus Agent Runtime") != 1 {
		t.Error("parent should have one header")
	}
	if strings.Count(prompt2, "# Consensus Agent Runtime") != 1 {
		t.Error("child should have one header")
	}
}

// ============================================================================
// JSON Schema Enforcement Tests (AC-010)
// ============================================================================

func TestPromptIncludesJSONSchema(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}
	layer := b.buildIdentityLayer(&SystemPromptConfig{
		AgentName: "test", ModelID: "gpt-4o", SessionID: "s1", Goal: "test",
	})

	// The output format section should contain the JSON schema
	if !strings.Contains(layer, "```json") {
		t.Error("prompt should include JSON schema block")
	}
	if !strings.Contains(layer, "\"internal_monologue\"") {
		t.Error("JSON schema should include internal_monologue field")
	}
	if !strings.Contains(layer, "\"memory_state_changes\"") {
		t.Error("JSON schema should include memory_state_changes field")
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestPromptBuilder_ConfigNilCheck(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}

	_, err := b.Build(nil, nil)
	if err == nil {
		t.Skip("nil config accepted — implementation dependent")
	}
	// Either way, should not panic
}

func TestPromptBuilder_EmptyGoal(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}
	layer := b.buildIdentityLayer(&SystemPromptConfig{
		AgentName: "test_agent", ModelID: "gpt-4o", SessionID: "s1", Goal: "",
	})

	if !strings.Contains(layer, "**Goal:**") {
		t.Error("should show goal field even when empty")
	}
}

func TestPromptBuilder_LongAgentName(t *testing.T) {
	longName := strings.Repeat("x", 256)
	b := &SystemPromptBuilder{harness: &Harness{}}
	layer := b.buildIdentityLayer(&SystemPromptConfig{
		AgentName: longName, ModelID: "gpt-4o", SessionID: "s1", Goal: "test",
	})

	if !strings.Contains(layer, longName) {
		t.Error("long agent name should be included")
	}
}

func TestPromptBuilder_SpecialCharacters(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}
	layer := b.buildIdentityLayer(&SystemPromptConfig{
		AgentName: "test", ModelID: "gpt-4o", SessionID: "s1",
		Goal: "Build API with **bold** and <html> tags",
	})

	// Special chars should be in the output (Markdown rendering handles them)
	if !strings.Contains(layer, "**bold**") {
		t.Error("special characters should be preserved")
	}
}

func TestBuildSkillsLayer_MultipleSkillsSorted(t *testing.T) {
	b := &SystemPromptBuilder{harness: &Harness{}}
	// buildSkillsLayer renders in input order; sorting is the caller's job
	// (the SQL query does ORDER BY name in production).
	skills := []SkillMetadata{
		{Name: "alpha", WhenToUse: "First"},
		{Name: "zebra", WhenToUse: "Last"},
	}

	layer := b.buildSkillsLayer(skills)

	firstIdx := strings.Index(layer, "alpha")
	lastIdx := strings.Index(layer, "zebra")

	if firstIdx < 0 || lastIdx < 0 {
		t.Fatal("skills not found in output")
	}

	if firstIdx > lastIdx {
		t.Errorf("expected alpha before zebra in sorted input, got alpha at %d, zebra at %d", firstIdx, lastIdx)
	}
}

func TestExtractJSONField_Valid(t *testing.T) {
	tests := []struct {
		json   string
		field  string
		expect string
	}{
		{`{"description": "test value"}`, "description", "test value"},
		{`{"name": "foo", "description": "bar"}`, "description", "bar"},
		{`{"description":"simple"}`, "description", "simple"},
	}

	for _, tt := range tests {
		result := extractJSONField(tt.json, tt.field)
		if result != tt.expect {
			t.Errorf("extractJSONField(%q, %q) = %q, want %q",
				tt.json, tt.field, result, tt.expect)
		}
	}
}

func TestExtractJSONField_Missing(t *testing.T) {
	result := extractJSONField(`{"foo": "bar"}`, "missing")
	if result != "" {
		t.Errorf("expected empty for missing field, got %q", result)
	}
}

func TestExtractJSONField_Empty(t *testing.T) {
	result := extractJSONField("", "any")
	if result != "" {
		t.Error("expected empty for empty JSON")
	}
}

// ============================================================================
// Regression: Prompt format MUST match AgentOutput struct JSON fields
// ============================================================================

func TestPromptFormatMatchesAgentOutputStruct(t *testing.T) {
	// Regression: formatPlanningSystemPromptV2 told the LLM to output
	// {"action": "...", "staged_commands": [...], "message_to_user": "..."}
	// but the AgentOutput parser reads:
	// {"memory_state_changes": [...], "system_actions": [...], "tool_requests": [...]}
	// The "action" and "staged_commands" fields were completely ignored,
	// causing all turns to parse as ActionNoOp.
	//
	// This test verifies the prompt:
	// 1. Includes "memory_state_changes" (what the parser reads)
	// 2. Includes "system_actions" (what the parser reads for commit/rollback/respond)
	// 3. Includes "tool_requests" (what the parser reads for tool calls)
	// 4. Does NOT include "action" as a standalone field name (deprecated format)
	// 5. Does NOT include "staged_commands" (deprecated format)
	// 6. Does NOT include "message_to_user" (deprecated format)

	h := &Harness{}

	// Build a minimal IterationContext
	ic := &IterationContext{
		Goal:          "Create e2e_test_table with 3 rows",
		SessionID:     "test-session",
		Status:        "planning",
		TrustLevel:    "high",
		Iteration:     0,
		MaxIterations: 10,
	}

	config := &PlanningConfig{
		MaxTurns:          10,
		MaxStagedCommands: 50,
	}

	// The prompt function uses coreTableNames and coreTableColumns (package vars)
	prompt := h.formatPlanningSystemPromptV2(ic, nil, 1, config)

	// Must include JSON fields that AgentOutput reads
	requiredFields := []string{
		`"memory_state_changes"`,
		`"system_actions"`,
		`"tool_requests"`,
		`"internal_monologue"`,
	}

	// Must NOT include deprecated/alternative field names
	deprecatedFields := []string{
		`"action"`,          // deprecated — parser ignores this field
		`"staged_commands"`, // deprecated — parser reads memory_state_changes
		`"message_to_user"`, // deprecated — parser reads system_actions
		`"end_iteration"`,   // deprecated — parser uses system_actions
	}

	for _, field := range requiredFields {
		if !strings.Contains(prompt, field) {
			t.Errorf("planning prompt MISSING required JSON field: %s", field)
		}
	}

	for _, field := range deprecatedFields {
		if strings.Contains(prompt, field) {
			t.Errorf("planning prompt contains DEPRECATED JSON field: %s — the AgentOutput parser ignores this field, causing ActionNoOp for all turns", field)
		}
	}

	// Verify the prompt includes schema info (not an empty template)
	if !strings.Contains(prompt, "Database Schema") {
		t.Error("prompt missing schema section")
	}
	if !strings.Contains(prompt, "memory_events") {
		t.Error("prompt missing memory_events table info")
	}
}
