// Package harness: unit tests for JSON output parser.
//
// axiom:trace work_item=runtime-harness-01 spec=specs/007-json-schema.md plan=phase-1/task-1-1/step-1-1-1 test=internal/harness/parser_test.go
package harness

import (
	"encoding/json"
	"testing"
)

func TestParseAgentResponse_ValidFullOutput(t *testing.T) {
	input := []byte(`{
		"internal_monologue": "I should query the recent memory and look for patterns.",
		"memory_state_changes": [
			"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'Analyzing Q4 data...', 'abc-123', 5)",
			"INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('thinking', 'Patterns found: 3 anomalies', 'abc-123', 5)"
		],
		"system_actions": [
			"UPDATE sessions SET status = 'idle' WHERE id = 'abc-123'"
		],
		"tool_requests": [
			{
				"tool_name": "fetch_url",
				"parameters": {"url": "https://example.com/data.csv"}
			}
		],
		"sub_agent_spawns": []
	}`)

	output, err := ParseAgentResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.InternalMonologue == "" {
		t.Error("expected non-empty internal_monologue")
	}
	if len(output.MemoryStateChanges) != 2 {
		t.Errorf("expected 2 memory_state_changes, got %d", len(output.MemoryStateChanges))
	}
	if len(output.SystemActions) != 1 {
		t.Errorf("expected 1 system_action, got %d", len(output.SystemActions))
	}
	if len(output.ToolRequests) != 1 {
		t.Errorf("expected 1 tool_request, got %d", len(output.ToolRequests))
	}
	if output.ToolRequests[0].ToolName != "fetch_url" {
		t.Errorf("expected tool_name 'fetch_url', got %q", output.ToolRequests[0].ToolName)
	}
}

func TestParseAgentResponse_MinimalOutput(t *testing.T) {
	input := []byte(`{
		"internal_monologue": "",
		"memory_state_changes": [],
		"system_actions": [],
		"tool_requests": [],
		"sub_agent_spawns": []
	}`)

	output, err := ParseAgentResponse(input)
	if err != nil {
		t.Fatalf("unexpected error for minimal valid output: %v", err)
	}

	if output.InternalMonologue != "" {
		t.Error("expected empty internal_monologue")
	}
	if len(output.MemoryStateChanges) != 0 {
		t.Error("expected empty memory_state_changes")
	}
}

func TestParseAgentResponse_EmptyInput(t *testing.T) {
	_, err := ParseAgentResponse([]byte{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}

	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pe.Message != "empty response" {
		t.Errorf("expected 'empty response', got %q", pe.Message)
	}
}

func TestParseAgentResponse_InvalidJSON(t *testing.T) {
	inputs := []string{
		``,
		`not json`,
		`{"internal_monologue": "test", "memory_state_changes": [`,
		`{"internal_monologue": unquoted}`,
	}

	for i, input := range inputs {
		_, err := ParseAgentResponse([]byte(input))
		if err == nil {
			t.Errorf("case %d: expected error for invalid JSON: %s", i, input)
			continue
		}
		pe, ok := err.(*ParseError)
		if !ok {
			t.Errorf("case %d: expected *ParseError, got %T: %v", i, err, err)
			continue
		}
		if pe.Message == "" {
			t.Errorf("case %d: parse error message is empty", i)
		}
	}
}

func TestParseAgentResponse_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
		errContains string
	}{
		{
			name: "missing memory_state_changes",
			input: `{"internal_monologue": "test", "system_actions": []}`,
			errContains: "memory_state_changes",
		},
		{
			name: "missing system_actions",
			input: `{"internal_monologue": "test", "memory_state_changes": []}`,
			errContains: "system_actions",
		},
		{
			name: "both missing",
			input: `{"internal_monologue": "test"}`,
			errContains: "memory_state_changes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseAgentResponse([]byte(tt.input))
			if err == nil {
				t.Fatal("expected error")
			}
			pe, ok := err.(*ParseError)
			if !ok {
				t.Fatalf("expected *ParseError, got %T", err)
			}
			if !stringsContains(pe.Message, tt.errContains) {
				t.Errorf("expected error containing %q, got %q", tt.errContains, pe.Message)
			}
		})
	}
}

func TestParseAgentResponse_InvalidToolName(t *testing.T) {
	tests := []struct {
		name    string
		toolName string
		shouldPass bool
	}{
		{"valid_alpha", "fetch_url", true},
		{"valid_with_dots", "net.http.get", true},
		{"valid_with_hyphens", "fetch-url-v2", true},
		{"empty_name", "", false},
		{"name_with_spaces", "fetch url", false},
		{"name_with_special_chars", "fetch$url", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := jsonEncode(t, map[string]any{
				"internal_monologue": "test",
				"memory_state_changes": []string{},
				"system_actions":       []string{},
				"tool_requests": []map[string]any{
					{
						"tool_name":  tt.toolName,
						"parameters": map[string]any{},
					},
				},
			})

			_, err := ParseAgentResponse(input)
			if tt.shouldPass && err != nil {
				t.Errorf("expected pass, got error: %v", err)
			}
			if !tt.shouldPass && err == nil {
				t.Error("expected error for invalid tool name")
			}
		})
	}
}

func TestParseAgentResponse_SubAgentSpawnValidation(t *testing.T) {
	tests := []struct {
		name       string
		agentName  string
		goal       string
		shouldPass bool
	}{
		{"valid", "summarizer", "Summarize the article", true},
		{"missing agent_name", "", "summarize", false},
		{"missing goal", "summarizer", "", false},
		{"both empty", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := jsonEncode(t, map[string]any{
				"internal_monologue":  "test",
				"memory_state_changes": []string{},
				"system_actions":       []string{},
				"sub_agent_spawns": []map[string]any{
					{
						"agent_name": tt.agentName,
						"goal":       tt.goal,
					},
				},
			})

			_, err := ParseAgentResponse(input)
			if tt.shouldPass && err != nil {
				t.Errorf("expected pass, got: %v", err)
			}
			if !tt.shouldPass && err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParseAgentResponse_NullByteRemoval(t *testing.T) {
	input := jsonEncode(t, map[string]any{
		"internal_monologue": "test",
		"memory_state_changes": []string{
			"INSERT INTO\x00 memory_events\x00 (content) VALUES\x00 ('test')",
		},
		"system_actions": []string{},
	})

	output, err := ParseAgentResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cleaned := output.MemoryStateChanges[0]
	if stringsContains(cleaned, "\x00") {
		t.Error("null byte was not removed from statement")
	}
	if cleaned != "INSERT INTO memory_events (content) VALUES ('test')" {
		t.Errorf("unexpected cleaned statement: %q", cleaned)
	}
}

func TestParseAgentResponse_BOMRemoval(t *testing.T) {
	// Prepend BOM to the raw JSON
	bom := []byte{0xEF, 0xBB, 0xBF}
	validJSON := []byte(`{"internal_monologue":"test","memory_state_changes":[],"system_actions":[]}`)
	input := append(bom, validJSON...)

	output, err := ParseAgentResponse(input)
	if err != nil {
		t.Fatalf("unexpected error with BOM: %v", err)
	}
	if output.InternalMonologue != "test" {
		t.Errorf("expected 'test', got %q", output.InternalMonologue)
	}
}

func TestParseAgentResponse_BOMInStatement(t *testing.T) {
	input := jsonEncode(t, map[string]any{
		"internal_monologue": "test",
		"memory_state_changes": []string{
			"\uFEFFSELECT 1",
		},
		"system_actions": []string{},
	})

	output, err := ParseAgentResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.MemoryStateChanges[0] != "SELECT 1" {
		t.Errorf("BOM was not removed from statement: %q", output.MemoryStateChanges[0])
	}
}

func TestParseAgentResponse_EmptyStatementsFiltered(t *testing.T) {
	input := jsonEncode(t, map[string]any{
		"internal_monologue": "test",
		"memory_state_changes": []string{
			"INSERT INTO t VALUES (1)",
			"   ",
			"",
			"SELECT 1",
			"\t\n",
		},
		"system_actions": []string{},
	})

	output, err := ParseAgentResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.MemoryStateChanges) != 2 {
		t.Errorf("expected 2 valid statements after filtering, got %d: %v",
			len(output.MemoryStateChanges), output.MemoryStateChanges)
	}
}

func TestParseAgentResponse_LargeArray(t *testing.T) {
	// Build a large memory_state_changes array
	statements := make([]string, 1000)
	for i := range statements {
		statements[i] = "SELECT " + string(rune('0'+i%10))
	}

	input := jsonEncode(t, map[string]any{
		"internal_monologue":  "test",
		"memory_state_changes": statements,
		"system_actions":       []string{},
	})

	output, err := ParseAgentResponse(input)
	if err != nil {
		t.Fatalf("unexpected error for large array: %v", err)
	}
	if len(output.MemoryStateChanges) != 1000 {
		t.Errorf("expected 1000 statements, got %d", len(output.MemoryStateChanges))
	}
}

func TestParseAgentResponse_UnicodeContent(t *testing.T) {
	input := jsonEncode(t, map[string]any{
		"internal_monologue": "分析数据",
		"memory_state_changes": []string{
			"INSERT INTO memory_events (content) VALUES ('こんにちは')",
		},
		"system_actions": []string{},
	})

	output, err := ParseAgentResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.InternalMonologue != "分析数据" {
		t.Errorf("unicode monologue lost: %q", output.InternalMonologue)
	}
	if !stringsContains(output.MemoryStateChanges[0], "こんにちは") {
		t.Error("unicode content lost in statement")
	}
}

func TestParseAgentResponse_ExtraFieldsIgnored(t *testing.T) {
	// Extra fields in the JSON should be silently ignored
	input := []byte(`{
		"internal_monologue": "test",
		"memory_state_changes": [],
		"system_actions": [],
		"extra_field": "should be ignored",
		"nested": {"hello": "world"}
	}`)

	output, err := ParseAgentResponse(input)
	if err != nil {
		t.Fatalf("unexpected error with extra fields: %v", err)
	}
	if output.InternalMonologue != "test" {
		t.Error("unexpected monologue")
	}
}

func TestParseAgentResponse_EmptyMonologueOK(t *testing.T) {
	input := []byte(`{
		"internal_monologue": "",
		"memory_state_changes": ["SELECT 1"],
		"system_actions": []
	}`)

	output, err := ParseAgentResponse(input)
	if err != nil {
		t.Fatalf("unexpected error for empty monologue: %v", err)
	}
	if output.InternalMonologue != "" {
		t.Error("expected empty monologue")
	}
}

// ============================================================================
// Helpers
// ============================================================================

func jsonEncode(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal test input: %v", err)
	}
	return data
}

func stringsContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
