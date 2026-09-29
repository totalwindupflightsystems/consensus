// Package harness: per-turn snapshot persistence (spec 024 §B2).
package harness

import (
	"encoding/json"
	"testing"
)

// TestMarshalTurnSnapshot_PersistsCoTWithTheTurn pins that a thinking turn's
// chain-of-thought is stored WITH the turn instead of being discarded, and that
// the existing top-level AgentOutput keys keep their exact place and shape so
// current readers (API surface, audit tests) are unaffected.
func TestMarshalTurnSnapshot_PersistsCoTWithTheTurn(t *testing.T) {
	output := &AgentOutput{
		InternalMonologue:  "looked at the ledger",
		MemoryStateChanges: []string{"INSERT INTO memory_events VALUES (1)"},
		SystemActions:      []string{"respond"},
		MessageToUser:      "done",
	}
	resp := &LLMResponse{
		Output:           output,
		ReasoningContent: "the user asked X, so I check Y first",
		ToolCalls: []ToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: ToolCallFunction{Name: "read_file", Arguments: `{"path":"/tmp/x"}`},
		}},
	}

	raw := marshalTurnSnapshot(output, resp)
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("snapshot is not valid JSON: %v\n%s", err, raw)
	}

	// CoT and tool calls ride with the turn.
	if got, _ := decoded["reasoning_content"].(string); got != resp.ReasoningContent {
		t.Errorf("snapshot reasoning_content = %q, want %q", got, resp.ReasoningContent)
	}
	calls, ok := decoded["tool_calls"].([]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("snapshot tool_calls = %v, want one entry", decoded["tool_calls"])
	}
	call, _ := calls[0].(map[string]any)
	if call["id"] != "call_1" {
		t.Errorf("snapshot tool_calls[0].id = %v, want call_1", call["id"])
	}

	// Backward compatibility: every AgentOutput key is still top-level.
	for _, key := range []string{"internal_monologue", "memory_state_changes", "system_actions", "message_to_user", "tool_requests", "sub_agent_spawns"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("snapshot lost the pre-existing top-level key %q: %s", key, raw)
		}
	}
	if decoded["internal_monologue"] != "looked at the ledger" {
		t.Errorf("internal_monologue = %v, want the original value", decoded["internal_monologue"])
	}
}

// TestMarshalTurnSnapshot_NoTurnKeepsNull pins the pre-existing behaviour for a
// nil output: the snapshot column gets the JSON literal null.
func TestMarshalTurnSnapshot_NoTurnKeepsNull(t *testing.T) {
	if got := string(marshalTurnSnapshot(nil, &LLMResponse{ReasoningContent: "ignored"})); got != "null" {
		t.Errorf("marshalTurnSnapshot(nil, ...) = %q, want null", got)
	}
}

// TestMarshalTurnSnapshot_NilResponseStillMarshals pins that a turn snapshot is
// always writable even when no response detail is available.
func TestMarshalTurnSnapshot_NilResponseStillMarshals(t *testing.T) {
	raw := marshalTurnSnapshot(&AgentOutput{InternalMonologue: "m"}, nil)
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("snapshot is not valid JSON: %v\n%s", err, raw)
	}
	if decoded["internal_monologue"] != "m" {
		t.Errorf("internal_monologue = %v, want m", decoded["internal_monologue"])
	}
	if _, ok := decoded["reasoning_content"]; ok {
		t.Errorf("reasoning_content present although no response was supplied: %s", raw)
	}
}
