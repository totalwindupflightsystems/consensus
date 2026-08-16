// Package mcp: regression tests for DOGFOOD-102 (P0 security) — the MCP
// session id and session api_key generators were deterministic:
// generateUUID() produced the same id for every session (so a second
// create_session failed with a UNIQUE constraint on sessions.id) and
// generateShortID(64) produced the same api_key for every session (one known
// constant opened every MCP session's memory). Both must now come from
// crypto/rand.
//
// axiom:trace work_item=dogfood-102 spec=specs/015-api-and-mcp.md test=internal/mcp/dogfood102_test.go
package mcp

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// ============================================================================
// DOGFOOD-102: unit — generators produce distinct, well-formed values
// ============================================================================

// TestGenerateUUID_Random verifies generateUUID returns distinct UUIDv4
// strings across calls and never the old deterministic constant.
func TestGenerateUUID_Random(t *testing.T) {
	ids := make([]string, 5)
	for i := range ids {
		ids[i] = generateUUID()
	}

	// All must be distinct.
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("generateUUID returned duplicate id %q across %d calls", id, len(ids))
		}
		seen[id] = true
	}

	// Must be a well-formed UUIDv4: 8-4-4-4-12 hex, version 4, variant RFC 4122.
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for _, id := range ids {
		if !re.MatchString(id) {
			t.Errorf("generateUUID returned %q, want UUIDv4 format (8-4-4-4-12, version 4, variant 8/9/a/b)", id)
		}
	}

	// Sanity: the old deterministic implementation returned the same constant
	// for every call — it must never appear again.
	if ids[0] == "00070e15-1c23-2a31-383f-464d545b6269" {
		t.Fatal("generateUUID returned the legacy deterministic value")
	}
}

// TestGenerateShortID_Random verifies generateShortID returns distinct values
// across calls while preserving length semantics (n/2 bytes → n hex chars).
func TestGenerateShortID_Random(t *testing.T) {
	ids := make([]string, 5)
	for i := range ids {
		ids[i] = generateShortID(64)
	}

	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if len(id) != 64 {
			t.Errorf("generateShortID(64) length = %d, want 64", len(id))
		}
		if seen[id] {
			t.Fatalf("generateShortID(64) returned duplicate %q across %d calls", id, len(ids))
		}
		seen[id] = true
	}

	// Length semantics must hold for other even lengths too.
	for _, l := range []int{8, 16, 32} {
		if id := generateShortID(l); len(id) != l {
			t.Errorf("generateShortID(%d) length = %d, want %d", l, len(id), l)
		}
	}
}

// ============================================================================
// DOGFOOD-102: integration — two sequential create_session calls both
// succeed with different session_id and different api_key
// ============================================================================

// TestCreateSession_Twice_DistinctIDsAndKeys reproduces the P0: before the
// fix the second create_session collided on sessions.id (same generated id)
// and handed out the same api_key as the first session.
func TestCreateSession_Twice_DistinctIDsAndKeys(t *testing.T) {
	srv := NewServer(&mockMCPDB{})
	sess := &mcpSession{authScope: "admin", sessionKey: "cs_ak_test"}

	args := json.RawMessage(`{"agent_name":"test_agent","goal":"test goal","model_id":"gpt-4o"}`)

	call := func() (id, apiKey string) {
		t.Helper()
		result, err := srv.toolCreateSession(args, sess)
		if err != nil {
			t.Fatalf("create_session failed: %v", err)
		}
		callResult := result.(MCPCallToolResult)
		if len(callResult.Content) == 0 {
			t.Fatal("expected content in result")
		}
		var out struct {
			ID     string `json:"id"`
			APIKey string `json:"api_key"`
		}
		if err := json.Unmarshal([]byte(callResult.Content[0].Text), &out); err != nil {
			t.Fatalf("failed to parse result %q: %v", callResult.Content[0].Text, err)
		}
		if out.ID == "" || out.APIKey == "" {
			t.Fatalf("expected id and api_key in result, got: %s", callResult.Content[0].Text)
		}
		if !strings.HasPrefix(out.APIKey, "cs_sk_") {
			t.Errorf("expected api_key to start with cs_sk_, got %q", out.APIKey)
		}
		return out.ID, out.APIKey
	}

	id1, key1 := call()
	id2, key2 := call()

	if id1 == id2 {
		t.Errorf("second create_session returned the SAME session id %q — would hit UNIQUE constraint on sessions.id", id1)
	}
	if key1 == key2 {
		t.Errorf("second create_session returned the SAME api_key %q — one constant opens every session", key1)
	}
}
