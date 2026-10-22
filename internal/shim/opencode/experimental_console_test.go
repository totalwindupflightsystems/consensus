// Package opencode: ROUTE-ADD-065 contract tests for GET /experimental/console
// (upstream operationId experimental.console.get, declared responses
// 200 ConsoleState | 400 BadRequestError | 500 InternalServerError).
//
// ch:trace row=ROUTE-ADD-065 spec=specs/openapi/upstream/openapi-1.18.33.json#experimental.console.get test=internal/shim/opencode/experimental_console_test.go witness=none:unattended-worker-session
//
// Before this change the exact path had no shim route at all, so every method
// fell through to net/http's default 404 — the declared-vs-served comparison's
// NOT-SERVED class (source item SHIM-DRIFT-062 in the 2026-09-29 baseline
// report, positional id SHIM-DRIFT-033 in the regenerated artifact),
// indistinguishable from "no such opencode operation". These tests pin the
// post-change contract arm by arm.
package opencode

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

const experimentalConsolePath = "/experimental/console"

// consoleStateKeys are the declared ConsoleState members the shim can state
// truthfully. activeOrgName is optional in the pinned schema and this runtime
// joins no org directory, so it is omitted rather than emitted blank; the
// declared schema has additionalProperties:false, so the body may carry ONLY
// declared members.
var consoleStateDeclaredKeys = map[string]bool{
	"consoleManagedProviders": true,
	"activeOrgName":           true,
	"switchableOrgCount":      true,
}

// TestExperimentalConsoleGetServesDeclared200 pins the happy path: the shim
// answers the declared 200 ConsoleState with a truthful empty state — no
// Console-managed provider binding exists in the instance's settings store and
// the instance offers no switchable org, so the required members carry [] and
// 0 rather than an invented provider or a fabricated org count.
func TestExperimentalConsoleGetServesDeclared200(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, experimentalConsolePath)
	if status != http.StatusOK {
		t.Fatalf("GET %s: got %d, want 200. Body: %s", experimentalConsolePath, status, body)
	}
	if ct := header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var state map[string]any
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, body)
	}
	for key := range state {
		if !consoleStateDeclaredKeys[key] {
			t.Errorf("body carries undeclared member %q (ConsoleState is additionalProperties:false): %s", key, body)
		}
	}
	providers, ok := state["consoleManagedProviders"]
	if !ok {
		t.Fatalf("declared required member consoleManagedProviders missing: %s", body)
	}
	list, ok := providers.([]any)
	if !ok {
		t.Fatalf("consoleManagedProviders = %#v, want a JSON array (never null): %s", providers, body)
	}
	if len(list) != 0 {
		t.Errorf("consoleManagedProviders = %#v, want the truthful empty list (no Console binding is stored)", list)
	}
	count, ok := state["switchableOrgCount"]
	if !ok {
		t.Fatalf("declared required member switchableOrgCount missing: %s", body)
	}
	if n, ok := count.(float64); !ok || n != 0 {
		t.Errorf("switchableOrgCount = %#v, want 0 (this instance joins no org directory)", count)
	}
	if _, present := state["activeOrgName"]; present {
		t.Errorf("activeOrgName is present (%#v) but this runtime has no active org — the optional member must be omitted, not fabricated", state["activeOrgName"])
	}
}

// TestExperimentalConsoleGetReadsConsoleBindingsFromStore proves the served
// list is a real read of the instance's settings store (the same store PUT
// /auth/{providerID} writes credentials into), not a hardcoded empty answer: a
// stored console.<providerID> binding is reported, deduplicated across its
// field rows, and unrelated settings keys are ignored.
func TestExperimentalConsoleGetReadsConsoleBindingsFromStore(t *testing.T) {
	mdb := &mockDB{queryResults: []db.Row{
		rowOf(map[string]any{"key": "console.anthropic"}),
		rowOf(map[string]any{"key": "console.anthropic.api"}),
		rowOf(map[string]any{"key": "console.openai"}),
		rowOf(map[string]any{"key": "llm.provider"}),
	}}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, experimentalConsolePath)
	if status != http.StatusOK {
		t.Fatalf("GET %s: got %d, want 200. Body: %s", experimentalConsolePath, status, body)
	}
	var state struct {
		ConsoleManagedProviders []string `json:"consoleManagedProviders"`
		SwitchableOrgCount      int      `json:"switchableOrgCount"`
	}
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, body)
	}
	want := []string{"anthropic", "openai"}
	if len(state.ConsoleManagedProviders) != len(want) {
		t.Fatalf("consoleManagedProviders = %v, want %v (deduplicated, unrelated keys ignored). Body: %s",
			state.ConsoleManagedProviders, want, body)
	}
	for i := range want {
		if state.ConsoleManagedProviders[i] != want[i] {
			t.Errorf("consoleManagedProviders[%d] = %q, want %q", i, state.ConsoleManagedProviders[i], want[i])
		}
	}
	if state.SwitchableOrgCount != 0 {
		t.Errorf("switchableOrgCount = %d, want 0", state.SwitchableOrgCount)
	}
}

// TestExperimentalConsoleGetValuedSelectorKeeps200 pins the declared
// directory/workspace query selectors: both are optional plain strings, so a
// valued selector is well-formed input and keeps the declared 200. The console
// state this instance reports is workspace-independent (it has no Console
// integration in any workspace), so a selector filters nothing.
func TestExperimentalConsoleGetValuedSelectorKeeps200(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, path := range []string{
		experimentalConsolePath + "?directory=/tmp/ws",
		experimentalConsolePath + "?workspace=ws1",
		experimentalConsolePath + "?directory=/tmp/ws&workspace=ws1",
	} {
		status, _, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		if status != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200 (a valued selector is well-formed). Body: %s", path, status, body)
		}
	}
}

// TestExperimentalConsoleGetMalformedSelectorIsDeclared400 pins the declared
// 400 arm: a present-but-blank declared query selector is malformed input and
// answers the declared BadRequestError body {name:"BadRequest",
// data:{kind,message}} (sibling convention, handleFormatter/handleSyncHistory).
// Malformed input must not read the store at all.
func TestExperimentalConsoleGetMalformedSelectorIsDeclared400(t *testing.T) {
	for _, path := range []string{
		experimentalConsolePath + "?directory=",
		experimentalConsolePath + "?workspace=",
		experimentalConsolePath + "?directory=%20",
	} {
		mdb := &mockDB{}
		_, srv := newTestServer(mdb)
		status, header, body := doShimRequest(t, srv.URL, http.MethodGet, path)
		srv.Close()

		if status != http.StatusBadRequest {
			t.Fatalf("GET %s: got %d, want 400. Body: %s", path, status, body)
		}
		if ct := header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("GET %s: Content-Type = %q, want application/json", path, ct)
		}
		var bad struct {
			Name string `json:"name"`
			Data struct {
				Message string `json:"message"`
				Kind    string `json:"kind"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &bad); err != nil {
			t.Fatalf("GET %s: body is not the declared BadRequestError shape: %v (%s)", path, err, body)
		}
		if bad.Name != "BadRequest" {
			t.Errorf("GET %s: name = %q, want BadRequest (declared enum)", path, bad.Name)
		}
		if bad.Data.Kind != "Query" {
			t.Errorf("GET %s: data.kind = %q, want Query", path, bad.Data.Kind)
		}
		if bad.Data.Message == "" {
			t.Errorf("GET %s: empty data.message, want the malformed-input reason", path)
		}
		if len(mdb.queries) != 0 {
			t.Errorf("GET %s: malformed input must not read the settings store; queries: %v", path, mdb.queries)
		}
	}
}

// TestExperimentalConsoleGetUnreadableStoreIsDeclared500 pins the declared 500
// arm: the served list is derived from the instance's settings store, so an
// unreadable store must not be reported as "no Console-managed providers" —
// absence is never claimed from a store the shim could not read (the
// handleProviderGet rule). The answer is the declared InternalServerError body
// {_tag:"InternalServerError"} (additionalProperties:false — exactly one
// member).
func TestExperimentalConsoleGetUnreadableStoreIsDeclared500(t *testing.T) {
	mdb := &mockDB{queryErr: errors.New("settings store unavailable")}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	status, header, body := doShimRequest(t, srv.URL, http.MethodGet, experimentalConsolePath)
	if status != http.StatusInternalServerError {
		t.Fatalf("GET %s with an unreadable store: got %d, want 500. Body: %s", experimentalConsolePath, status, body)
	}
	if ct := header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var internal map[string]any
	if err := json.Unmarshal(body, &internal); err != nil {
		t.Fatalf("500 body is not JSON: %v (%s)", err, body)
	}
	if internal["_tag"] != "InternalServerError" {
		t.Errorf("_tag = %#v, want InternalServerError (declared schema)", internal["_tag"])
	}
	if len(internal) != 1 {
		t.Errorf("500 body = %s, want exactly {_tag} (additionalProperties:false)", body)
	}
	if len(mdb.queries) == 0 {
		t.Error("no settings-store read was issued — the 500 arm must come from a real read failure")
	}
}

// TestExperimentalConsoleGetMethodGuard pins the sibling method-guard
// convention: only GET is declared on this exact path, so every other method
// answers the 405 METHOD_NOT_ALLOWED envelope (never a catch-all, never a
// lying 404).
func TestExperimentalConsoleGetMethodGuard(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		_, srv := newTestServer(&mockDB{})
		status, header, body := doShimRequest(t, srv.URL, method, experimentalConsolePath)
		srv.Close()

		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: got %d, want 405. Body: %s", method, experimentalConsolePath, status, body)
			continue
		}
		if ct := header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s %s: Content-Type = %q, want application/json", method, experimentalConsolePath, ct)
		}
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("%s %s: body is not the error envelope: %v (%s)", method, experimentalConsolePath, err, body)
			continue
		}
		if envelope.Error.Code != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s %s: error.code = %q, want METHOD_NOT_ALLOWED", method, experimentalConsolePath, envelope.Error.Code)
		}
	}
}
