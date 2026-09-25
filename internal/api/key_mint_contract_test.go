// Package api: integration tests for the API key mint contract
// (DF-CONSENSUS-29, SPEC-015 §3.8, docs/API.md "Key management endpoints").
//
// The mint contract, as documented: POST /api/v1/auth/keys returns the new
// key's full secret in `api_key` exactly once (the row stores only the
// sha256 hash, so this is the sole chance to capture it); the list endpoint
// stays prefix-only forever; and a mint with a nonexistent session_id is
// rejected 400 NOT_FOUND (the session-exists check must actually execute).
//
// Provenance note (2026-09-25): board row DF-CONSENSUS-29 reported the mint
// response as a redacted placeholder ("cs_sk_...cdb8") and a dead
// session-exists check. Neither reproduces anywhere in the tree: the handler
// writes the raw key into the response map, writeJSON is a plain marshal,
// no body-rewriting middleware exists, and db.DB.QueryRow is eager (executes
// without Scan and errors on zero rows — verified live on a scratch server:
// bogus session_id → 400 NOT_FOUND, valid mint → full secret). These tests
// pin the WORKING contract so any future sanitizer/masker introduction or
// QueryRow-semantics regression fails loudly here, on the exact surfaces the
// board row describes.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mintKeyViaAPI issues POST /api/v1/auth/keys with the admin key and returns
// the recorder plus the decoded response body as a generic map.
func mintKeyViaAPI(srv *integrationServer, body string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/keys", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

// createSessionForMint creates a live session via the REST API and returns
// its id, so the minted session-scoped key has a real session to bind to.
func createSessionForMint(t *testing.T, srv *integrationServer) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions",
		strings.NewReader(`{"agent_name":"mint-contract","goal":"mint contract test"}`))
	req.Header.Set("Authorization", "Bearer "+srv.adminKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("create session: expected 2xx, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("create session: decode: %v", err)
	}
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("create session: no id in response: %s", w.Body.String())
	}
	return id
}

// TestKeyMint_ReturnsFullUsableSecretOnce pins the show-once contract:
// the mint response's api_key is the FULL secret (no literal "..." placeholder,
// not the 8-char prefix), it authenticates as a Bearer token, and the list
// endpoint still exposes prefixes only.
func TestKeyMint_ReturnsFullUsableSecretOnce(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	sessionID := createSessionForMint(t, srv)

	rec, resp := mintKeyViaAPI(srv, `{"scope":"session","session_id":"`+sessionID+`"}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("mint: expected 2xx, got %d: %s", rec.Code, rec.Body.String())
	}

	apiKey, _ := resp["api_key"].(string)
	prefix, _ := resp["key_prefix"].(string)
	if apiKey == "" {
		t.Fatalf("mint: api_key missing from response: %s", rec.Body.String())
	}
	if strings.Contains(apiKey, "...") {
		t.Errorf("mint: api_key contains a literal \"...\" placeholder: %q", apiKey)
	}
	if prefix != "" && apiKey == prefix {
		t.Errorf("mint: api_key equals key_prefix %q — secret was redacted", apiKey)
	}
	if len(apiKey) < 32 {
		t.Errorf("mint: api_key suspiciously short (%d chars) — not a full secret: %q", len(apiKey), apiKey)
	}

	// The minted secret must authenticate (board row ACCEPTANCE: use the
	// exact response value as Bearer on the bound session).
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID, nil)
	getReq.Header.Set("Authorization", "Bearer "+apiKey)
	getRec := httptest.NewRecorder()
	srv.router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Errorf("minted key does not authenticate: GET own session got %d: %s", getRec.Code, getRec.Body.String())
	}

	// ...and the session-scoped key must NOT read foreign sessions.
	foreignID := createSessionForMint(t, srv)
	foreignReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+foreignID, nil)
	foreignReq.Header.Set("Authorization", "Bearer "+apiKey)
	foreignRec := httptest.NewRecorder()
	srv.router.ServeHTTP(foreignRec, foreignReq)
	if foreignRec.Code != http.StatusForbidden {
		t.Errorf("minted session key read a foreign session: got %d, want 403", foreignRec.Code)
	}

	// List stays prefix-only: no api_key field, and no leaked full secret.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/keys", nil)
	listReq.Header.Set("Authorization", "Bearer "+srv.adminKey)
	listRec := httptest.NewRecorder()
	srv.router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list keys: expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var listed []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("list keys: decode: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("list keys: expected at least the minted key")
	}
	for _, entry := range listed {
		if _, hasSecret := entry["api_key"]; hasSecret {
			t.Errorf("list keys: entry leaks an api_key field: %v", entry)
		}
		if p, _ := entry["key_prefix"].(string); p == apiKey {
			t.Errorf("list keys: entry exposes the full secret as key_prefix: %v", entry)
		}
	}
}

// TestKeyMint_BogusSessionID_Returns400NotFound pins the session-exists
// enforcement: the driver's QueryRow is eager (it executes the query and
// errors on zero rows), so a nonexistent session_id must yield 400 NOT_FOUND
// — never 2xx. Guards against both a semantics regression in db.DB.QueryRow
// and a "harmless" rewrite of the check that stops it from executing.
func TestKeyMint_BogusSessionID_Returns400NotFound(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	rec, _ := mintKeyViaAPI(srv, `{"scope":"session","session_id":"no-such-session-0000"}`)
	if rec.Code == http.StatusOK || rec.Code == http.StatusCreated {
		t.Fatalf("mint with nonexistent session_id: expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("mint with nonexistent session_id: expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NOT_FOUND") {
		t.Errorf("mint with nonexistent session_id: expected NOT_FOUND error code, got %s", rec.Body.String())
	}
}

// TestKeyMint_SecretSurvivesRoundTrip proves the minted secret authenticates
// across ALL mint scope variants the board row exercised (session,
// session+expires_in, readonly) — every returned api_key is a usable secret,
// each distinct from its key_prefix and from the other mints.
func TestKeyMint_SecretSurvivesRoundTrip(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	sessionID := createSessionForMint(t, srv)
	bodies := []string{
		`{"scope":"session","session_id":"` + sessionID + `"}`,
		`{"scope":"session","session_id":"` + sessionID + `","expires_in":3600}`,
		`{"scope":"readonly"}`,
	}
	seen := map[string]bool{}
	for i, body := range bodies {
		rec, resp := mintKeyViaAPI(srv, body)
		if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
			t.Fatalf("mint variant %d: expected 2xx, got %d: %s", i, rec.Code, rec.Body.String())
		}
		apiKey, _ := resp["api_key"].(string)
		prefix, _ := resp["key_prefix"].(string)
		if apiKey == "" || strings.Contains(apiKey, "...") || (prefix != "" && apiKey == prefix) {
			t.Errorf("mint variant %d: api_key is not a full usable secret: %q (prefix %q)", i, apiKey, prefix)
			continue
		}
		if seen[apiKey] {
			t.Errorf("mint variant %d: duplicate api_key %q", i, apiKey)
		}
		seen[apiKey] = true

		// Round-trip: hash of the returned secret must equal the stored hash.
		rows, err := srv.conn.Query(context.Background(),
			`SELECT key_prefix FROM api_keys WHERE key_hash = $1`, sha256Hash(apiKey))
		if err != nil {
			t.Fatalf("mint variant %d: hash lookup: %v", i, err)
		}
		if len(rows) != 1 {
			t.Errorf("mint variant %d: returned secret's hash not found in api_keys (%d rows)", i, len(rows))
		}
	}
}

// TestKeyMint_NonAdminScope_Forbidden pins the admin gate on the mint route.
func TestKeyMint_NonAdminScope_Forbidden(t *testing.T) {
	srv := newIntegrationServer(t)
	defer srv.close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/keys", strings.NewReader(`{"scope":"readonly"}`))
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 40))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("mint with garbage bearer: expected 401, got %d", w.Code)
	}
}
