// Package harness: the per-turn snapshot must carry the turn's chain-of-thought
// and its signature (spec 024 §B2).
//
// The CoT rule is only satisfied if the reasoning a turn produced survives the
// turn: a client that hands it back and a store that drops it is the same
// failure. This test pins the persistence half — including the Anthropic-format
// signature, which the provider requires echoed back verbatim — so a field
// rename or a forgotten assignment fails here rather than showing up later as a
// provider 400 with no visible cause.
//
// axiom:trace work_item=SHIM-SUPPORT-001 spec=specs/024-shim-support-matrix.md impl=internal/harness/executor.go
package harness

import (
	"encoding/json"
	"testing"
)

func TestTurnSnapshotCarriesReasoningAndSignature(t *testing.T) {
	out := &AgentOutput{
		InternalMonologue:  "thinking",
		MemoryStateChanges: []string{},
		SystemActions:      []string{"respond"},
		ToolRequests:       []ToolRequest{},
		SubAgentSpawns:     []SubAgentSpawn{},
	}
	resp := &LLMResponse{
		Content:            "the answer",
		ReasoningContent:   "the chain of thought",
		ReasoningSignature: "provider-signature",
	}

	raw := marshalTurnSnapshot(out, resp)

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("turn snapshot is not JSON: %v\n%s", err, raw)
	}
	if got := decoded["reasoning_content"]; got != "the chain of thought" {
		t.Fatalf("snapshot reasoning_content = %v, want the turn's chain of thought", got)
	}
	if got := decoded["reasoning_signature"]; got != "provider-signature" {
		t.Fatalf("snapshot reasoning_signature = %v, want the provider's signature", got)
	}
	// The embedded AgentOutput keys keep their names and shape for existing
	// readers: the snapshot adds fields, it does not restructure the document.
	if got := decoded["internal_monologue"]; got != "thinking" {
		t.Fatalf("snapshot internal_monologue = %v, want the flat AgentOutput key", got)
	}
	if _, ok := decoded["output"]; ok {
		t.Fatalf("AgentOutput is no longer embedded flat: %s", raw)
	}

	// A nil response must keep the previous behaviour (no reasoning keys at all).
	raw = marshalTurnSnapshot(out, nil)
	decoded = nil
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("snapshot without a response is not JSON: %v\n%s", err, raw)
	}
	if _, ok := decoded["reasoning_content"]; ok {
		t.Fatalf("snapshot invented reasoning_content without a response: %s", raw)
	}
}
