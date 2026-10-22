package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// consoleSwitchPath is the pinned-upstream path this row serves:
// POST /experimental/console/switch — experimental.console.switchOrg
// (ROUTE-ADD-067, source item SHIM-DRIFT-035 in
// specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json;
// contract specs/024-shim-support-matrix.md §A).
const consoleSwitchPath = "/experimental/console/switch"

// TestConsoleSwitchAnswersDeclaredBoolean pins the declared contract of
// experimental.console.switchOrg (declared response: 200 boolean "Switch
// success"; optional directory/workspace query selectors; requestBody
// {accountID*: string, orgID*: string} with additionalProperties: false and
// no `required: true`, so an absent body is well-formed).
//
// RED proof: before this row the path had no route at all and every request
// answered the net/http default 404 (the artifact's NOT-SERVED class); the
// green arm below fails with "got 404, want 200" on the pre-fix tree.
//
// This runtime keeps no Console account/org store and no upstream Console
// backend, so the declared boolean is the truthful `false` — the switch was
// not performed — never a fabricated success (the tui.showToast /
// provider.oauth.authorize truthful-boolean-false precedent).
func TestConsoleSwitchAnswersDeclaredBoolean(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	body := `{"accountID":"acc-1","orgID":"org-1"}`
	status, header, data := doShimRequestBody(t, srv.URL, http.MethodPost, consoleSwitchPath, body)
	if status != http.StatusOK {
		t.Fatalf("POST %s: got %d, want 200 (body %s)", consoleSwitchPath, status, data)
	}
	if ct := header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var switched bool
	if err := json.Unmarshal(data, &switched); err != nil {
		t.Fatalf("POST %s: body is not the declared boolean: %v (%s)", consoleSwitchPath, err, data)
	}
	if switched {
		t.Errorf("POST %s: body = true, want the truthful false — this shim switches no Console org", consoleSwitchPath)
	}

	// An absent body is well-formed: the pinned document does not mark
	// requestBody required. The truthful answer is unchanged.
	status, _, data = doShimRequestBody(t, srv.URL, http.MethodPost, consoleSwitchPath, "")
	if status != http.StatusOK {
		t.Fatalf("POST %s with no body: got %d, want 200 (body %s)", consoleSwitchPath, status, data)
	}
	if err := json.Unmarshal(data, &switched); err != nil || switched {
		t.Errorf("POST %s with no body: got %s, want boolean false (%v)", consoleSwitchPath, data, err)
	}

	// Declared optional query selectors are accepted when non-blank.
	status, _, data = doShimRequestBody(t, srv.URL, http.MethodPost,
		consoleSwitchPath+"?directory=/tmp&workspace=wrk_demo", body)
	if status != http.StatusOK {
		t.Fatalf("POST %s with declared selectors: got %d, want 200 (body %s)", consoleSwitchPath, status, data)
	}
}

// TestConsoleSwitchRejectsMalformedInput pins the 400 arm: the declared
// requestBody sets additionalProperties: false and requires accountID + orgID,
// so a body that cannot be that object is refused instead of being accepted as
// a selection. The sibling Body/Query BadRequest envelope is the same one
// handleLog/handleVCSApply answer (writeOpencodeBadRequest).
func TestConsoleSwitchRejectsMalformedInput(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	cases := []struct {
		name     string
		path     string
		body     string
		wantKind string
	}{
		{"malformed json", consoleSwitchPath, `{"accountID":`, "Body"},
		{"non-object body", consoleSwitchPath, `["acc-1","org-1"]`, "Body"},
		{"json null body", consoleSwitchPath, `null`, "Body"},
		{"trailing json value", consoleSwitchPath, `{"accountID":"a","orgID":"o"}{"accountID":"b"}`, "Body"},
		{"undeclared field", consoleSwitchPath, `{"accountID":"a","orgID":"o","label":"x"}`, "Body"},
		{"missing orgID", consoleSwitchPath, `{"accountID":"a"}`, "Body"},
		{"missing accountID", consoleSwitchPath, `{"orgID":"o"}`, "Body"},
		{"blank accountID", consoleSwitchPath, `{"accountID":"  ","orgID":"o"}`, "Body"},
		{"non-string orgID", consoleSwitchPath, `{"accountID":"a","orgID":7}`, "Body"},
		{"blank directory selector", consoleSwitchPath + "?directory=", `{"accountID":"a","orgID":"o"}`, "Query"},
		{"blank workspace selector", consoleSwitchPath + "?workspace=%20", `{"accountID":"a","orgID":"o"}`, "Query"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _, data := doShimRequestBody(t, srv.URL, http.MethodPost, tc.path, tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("POST %s: got %d, want 400 (body %s)", tc.path, status, data)
			}
			var got struct {
				Name string `json:"name"`
				Data struct {
					Kind    string `json:"kind"`
					Message string `json:"message"`
				} `json:"data"`
			}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("POST %s: body not the BadRequest envelope: %v (%s)", tc.path, err, data)
			}
			if got.Name != "BadRequest" {
				t.Errorf("name = %q, want BadRequest (body %s)", got.Name, data)
			}
			if got.Data.Kind != tc.wantKind {
				t.Errorf("data.kind = %q, want %q (body %s)", got.Data.Kind, tc.wantKind, data)
			}
			if got.Data.Message == "" {
				t.Errorf("data.message empty, want the offending input named (body %s)", data)
			}
		})
	}
}

// TestConsoleSwitchMethodGuard pins the sibling method-guard envelope: the
// declared operation is POST-only, so another method on the exact path keeps
// the METHOD_NOT_ALLOWED 405 body used by every other shim route.
func TestConsoleSwitchMethodGuard(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, _, data := doShimRequest(t, srv.URL, method, consoleSwitchPath)
		if status != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: got %d, want 405 (body %s)", method, consoleSwitchPath, status, data)
			continue
		}
		var got struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &got); err != nil || got.Error.Code != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s %s: error.code = %q (err %v), want METHOD_NOT_ALLOWED", method, consoleSwitchPath, got.Error.Code, err)
		}
	}
}

// TestConsoleSwitchRequiresAuth pins the served-surface auth row (api-key):
// the route is a Consensus-authenticated shim route, so an unauthenticated
// POST is 401 UNAUTHENTICATED — never the pre-fix 404.
func TestConsoleSwitchRequiresAuth(t *testing.T) {
	s := NewServer(&mockDB{}, "test-key", nil, nil) // no skipAuth
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	status, _, data := doShimRequestBody(t, srv.URL, http.MethodPost, consoleSwitchPath, `{"accountID":"a","orgID":"o"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST %s: got %d, want 401. Body: %s", consoleSwitchPath, status, data)
	}
	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("body not the error envelope: %v (%s)", err, data)
	}
	if got.Error.Code != "UNAUTHENTICATED" {
		t.Errorf("error.code = %q, want UNAUTHENTICATED", got.Error.Code)
	}
}
