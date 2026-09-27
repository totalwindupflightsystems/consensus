// Package mcp: regression coverage for DF-CONSENSUS-48 — one credential at
// streamable-HTTP initialize authorises the whole session. An outside review
// (docs/reviews/consensus-carter-review-2026-09-27.html, commit ee44cbc)
// reported POST /mcp initialize honouring params._meta.authorization while
// tools/list with the same credential answered 401 "Authentication required".
// The live contract on the current tree is: the key is validated ONCE at
// initialize and stored on the mcpSession; every follow-up call addresses the
// session by the Mcp-Session-Id header alone and passes through the DOGFOOD-101
// dispatch gate on the strength of that stored auth. These tests pin that
// sequence end-to-end through the real streamable HTTP handler (never private
// methods), plus the negative half: a missing/invalid credential can never
// produce a usable session, and the bootstrap session of a failed initialize
// is torn down so its id cannot be replayed.
//
// axiom:trace work_item=DF-CONSENSUS-48 spec=specs/015-api-and-mcp.md impl=internal/mcp/streamable.go test=internal/mcp/dfconsensus48_test.go
package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

// df48SessionHeader returns the Mcp-Session-Id value carried by a response,
// failing the test when the server forgot to hand one back (a streamable
// client could not address any follow-up call without it).
func df48SessionHeader(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	sid := w.Header().Get(streamableSessionHeader)
	if sid == "" {
		t.Fatal("response carries no Mcp-Session-Id header — a streamable client cannot address follow-up calls")
	}
	return sid
}

// df48AssertOK decodes a JSON-RPC response and fails when it carries an error
// object (the HTTP status is checked by the caller).
func df48AssertOK(t *testing.T, step string, body []byte) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("%s: invalid JSON: %q", step, body)
	}
	if errObj, ok := resp["error"]; ok {
		t.Fatalf("%s: unexpected JSON-RPC error: %v", step, errObj)
	}
	if _, ok := resp["result"]; !ok {
		t.Fatalf("%s: no result object: %v", step, resp)
	}
	return resp
}

// df48IsForbiddenErr asserts the error envelope of a rejected call: HTTP 401
// with a -32002 Forbidden body (the streamable contract — never a bare-text
// error, so machine clients can tell auth failures from routing ones).
func df48IsForbiddenErr(t *testing.T, step string, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("%s: expected HTTP 401, got %d (body=%q)", step, w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s: 401 body is not JSON: %q", step, w.Body.String())
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("%s: expected JSON-RPC error envelope, got %v", step, resp)
	}
	if int(errObj["code"].(float64)) != -32002 {
		t.Errorf("%s: expected error code -32002, got %v", step, errObj["code"])
	}
	if errObj["message"] != "Forbidden" {
		t.Errorf("%s: expected message 'Forbidden', got %v", step, errObj["message"])
	}
}

// TestStreamable_OneCredential_AuthorizesListAndCall is the DF-CONSENSUS-48
// acceptance sequence, executed through the streamable HTTP handler exactly as
// a real client drives it:
//
//	initialize (credential in params._meta.authorization, header-less)
//	  -> capture Mcp-Session-Id from the response
//	tools/list  (session header ONLY — no credential re-supplied)
//	tools/call  (same session header ONLY)
//
// The follow-up requests carry _no_ _meta.authorization: if the session-bound
// auth regresses (the review's finding), both steps fail with -32002 and this
// test goes red.
func TestStreamable_OneCredential_AuthorizesListAndCall(t *testing.T) {
	mock := &sequentialMockDB{
		results: [][]db.Row{
			// Query 1: initialize — api_keys lookup (validateAuth)
			{{"id": "key-1", "scope": "admin", "session_id": nil}},
			// Query 2: tools/call list_memory
			{{"id": int64(1), "type": "text_block", "content": "hello from df48", "iteration_created": int64(1), "created_at": "2024-01-01"}},
		},
	}
	srv := NewServer(mock)

	// Step 1: header-less initialize carrying the credential in
	// params._meta.authorization (the streamable bootstrap path).
	w := streamablePOST(srv, "", streamableInitializePayload("Bearer cs_ak_df48"))
	if w.Code != http.StatusOK {
		t.Fatalf("initialize: expected 200, got %d (body=%q)", w.Code, w.Body.String())
	}
	initResp := df48AssertOK(t, "initialize", w.Body.Bytes())
	result := initResp["result"].(map[string]any)
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("initialize: unexpected protocolVersion %v", result["protocolVersion"])
	}
	sid := df48SessionHeader(t, w)

	// The bootstrapped session must now be authenticated and hold the
	// validated credential — this is what authorises every follow-up call.
	srv.mu.RLock()
	sess := srv.sessions[sid]
	authenticated := sess != nil && sess.authenticated
	key := ""
	if sess != nil {
		key = sess.sessionKey
	}
	srv.mu.RUnlock()
	if !authenticated {
		t.Fatal("successful initialize must leave the streamable session authenticated")
	}
	if key != "cs_ak_df48" {
		t.Errorf("session.sessionKey = %q, want the credential accepted at initialize", key)
	}

	// Step 2: tools/list addressing the session by header alone — params
	// carry no _meta.authorization at all.
	w2 := streamablePOST(srv, sid, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list",
		"params": map[string]any{},
	})
	if w2.Code != http.StatusOK {
		t.Fatalf("tools/list (session header only): expected 200, got %d (body=%q)", w2.Code, w2.Body.String())
	}
	listResp := df48AssertOK(t, "tools/list", w2.Body.Bytes())
	tools := listResp["result"].(map[string]any)["tools"].([]any)
	if len(tools) < 8 {
		t.Errorf("tools/list: expected >= 8 tools, got %d", len(tools))
	}

	// Step 3: tools/call on the SAME session header — still no credential.
	w3 := streamablePOST(srv, sid, map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{
			"name":      "list_memory",
			"arguments": map[string]any{"session_id": "sess-1", "limit": 10},
		},
	})
	if w3.Code != http.StatusOK {
		t.Fatalf("tools/call (session header only): expected 200, got %d (body=%q)", w3.Code, w3.Body.String())
	}
	callResp := df48AssertOK(t, "tools/call", w3.Body.Bytes())
	text := callResp["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, `"count": 1`) {
		t.Errorf("tools/call: expected count 1 in result, got: %s", text)
	}

	// Exactly two DB queries must have run: the initialize key lookup and
	// the list_memory read. A third api_keys lookup would mean follow-up
	// calls are re-validating credentials instead of trusting the session.
	if len(mock.queries) != 2 {
		t.Errorf("expected exactly 2 queries (initialize key lookup + list_memory), got %d: %v", len(mock.queries), mock.queries)
	}
}

// TestStreamable_NoCredentialFollowUp_Rejected pins the negative half of the
// same contract: a streamable request that never completed an authenticated
// initialize cannot ride the bootstrap path into tools — with and without a
// session header, the answer is 401 Forbidden, and no session is left behind.
func TestStreamable_NoCredentialFollowUp_Rejected(t *testing.T) {
	t.Run("headerless tools/list on a fresh bootstrap", func(t *testing.T) {
		srv := NewServer(&mockMCPDB{})
		w := streamablePOST(srv, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
		df48IsForbiddenErr(t, "headerless tools/list", w)

		// The failed bootstrap must be torn down — its id was never
		// handed out, and nothing unauthenticated may linger addressable.
		srv.mu.RLock()
		remaining := len(srv.sessions)
		srv.mu.RUnlock()
		if remaining != 0 {
			t.Errorf("expected no leftover unauthenticated sessions, got %d", remaining)
		}
	})

	t.Run("initialize without _meta then tools/list", func(t *testing.T) {
		srv := NewServer(&mockMCPDB{})
		// initialize with NO credential at all.
		w := streamablePOST(srv, "", streamableInitializePayload(""))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("initialize without credential: expected 401, got %d (body=%q)", w.Code, w.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("initialize error body is not JSON: %q", w.Body.String())
		}
		errObj, ok := resp["error"].(map[string]any)
		if !ok {
			t.Fatalf("expected JSON-RPC error envelope, got %v", resp)
		}
		// Auth failures at the handshake keep naming the actual problem:
		// -32000 Authentication required (the key was missing), not a
		// generic Forbidden.
		if int(errObj["code"].(float64)) != -32000 || errObj["message"] != "Authentication required" {
			t.Errorf("expected -32000 'Authentication required', got %v", errObj)
		}
		// No session header may be issued for a failed handshake.
		if sid := w.Header().Get(streamableSessionHeader); sid != "" {
			t.Errorf("failed initialize must not hand out a session id, got %q", sid)
		}
		// And the bootstrap session is gone: replaying its (never
		// published) id must 401, not resume an authenticated session.
		srv.mu.RLock()
		remaining := len(srv.sessions)
		srv.mu.RUnlock()
		if remaining != 0 {
			t.Errorf("failed initialize must tear down its bootstrap session, %d remain", remaining)
		}
	})

	t.Run("invalid credential initialize then forged session replay", func(t *testing.T) {
		srv := NewServer(&mockMCPDB{queryResults: []db.Row{}}) // key not found
		w := streamablePOST(srv, "", streamableInitializePayload("Bearer cs_ak_invalid0"))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("initialize with invalid key: expected 401, got %d (body=%q)", w.Code, w.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		errObj, ok := resp["error"].(map[string]any)
		if !ok {
			t.Fatalf("expected JSON-RPC error envelope, got %v", resp)
		}
		// -32001 Invalid API key — the lookup ran and found nothing.
		if int(errObj["code"].(float64)) != -32001 {
			t.Errorf("expected -32001 'Invalid API key', got %v", errObj)
		}
		if sid := w.Header().Get(streamableSessionHeader); sid != "" {
			t.Errorf("invalid-key initialize must not hand out a session id, got %q", sid)
		}
		// Attempting to continue with a made-up session id for the same
		// flow must stay rejected (no way to upgrade a failed handshake
		// into a usable session).
		w2 := streamablePOST(srv, "forged-df48", map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
		df48IsForbiddenErr(t, "forged session after invalid initialize", w2)
	})
}
