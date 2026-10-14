package opencode

// Tests for the declared /pty/{ptyID} operation family (ROUTE-ADD-105 /
// ROUTE-ADD-106 / ROUTE-ADD-107 / ROUTE-ADD-108 / ROUTE-ADD-109):
//
//	GET    /pty/{ptyID}               → pty.get          (200 Pty, 400, 404)
//	PUT    /pty/{ptyID}               → pty.update       (200 Pty, 400, 404)
//	DELETE /pty/{ptyID}               → pty.remove       (200 bool, 400, 404)
//	GET    /pty/{ptyID}/connect       → pty.connect      (200 bool, 403, 404)
//	POST   /pty/{ptyID}/connect-token → pty.connectToken (200, 400, 403, 404)
//
// Every arm the contract declares is pinned here:
//
//   - the declared 400 arm where the operation declares one (ill-shaped ptyID
//     against the declared pattern ^pty; a present-but-blank or
//     non-directory directory/workspace selector; pty.update's requestBody
//     schema), with the DECLARED BadRequestError wire shape {name, data{kind,
//     message}};
//   - the declared 403 scope arm on the two connect operations, with their
//     exact declared schemas ({_tag:"Forbidden"} — additionalProperties false,
//     so the body carries that one field — and {_tag:"PtyForbiddenError",
//     message});
//   - the declared 404 for every well-formed ptyID, with the typed envelope
//     each operation declares (PtyNotFoundError {_tag, ptyID, message} for
//     get/update/remove/connectToken; NotFoundError {name, data{message}} for
//     connect);
//   - the no-fabrication guard: the declared 200 Pty / boolean / connect-ticket
//     payload is NEVER served, because the runtime keeps no pty registry and
//     mints no connect ticket (pty.list is truthfully empty, ROUTE-ADD-102);
//   - the pre-change answer for every undeclared method (net/http's default
//     404 — 405 is in none of the five declared response sets);
//   - the neighbour guard: bare /pty and /pty/shells are untouched, and any
//     other /pty/* shape keeps the default 404.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ptyNamedError decodes the declared BadRequestError shape
// {name:"BadRequest", data:{kind, message}}.
type ptyNamedError struct {
	Name string `json:"name"`
	Data struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	} `json:"data"`
}

// newPtyRouteServer starts a shim whose served workspace is a fresh temp dir
// and returns it plus a second existing directory the instance does NOT serve,
// so both the accepted-selector and the foreign-workspace arm are reachable
// without touching the process CWD.
func newPtyRouteServer(t *testing.T) (*Server, *httptest.Server, string, string) {
	t.Helper()
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	s.skipAuth = true
	served := t.TempDir()
	s.workdir = served
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv, served, t.TempDir()
}

// assertBadRequestKind pins the declared 400 arm: the NamedError BadRequest
// envelope naming where the input was wrong.
func assertBadRequestKind(t *testing.T, label string, body []byte, wantKind string) {
	t.Helper()
	var got ptyNamedError
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%s: 400 body is not JSON: %v (%s)", label, err, body)
	}
	if got.Name != "BadRequest" {
		t.Errorf("%s: 400 body name = %q, want the declared BadRequestError name BadRequest", label, got.Name)
	}
	if got.Data.Kind != wantKind {
		t.Errorf("%s: 400 body data.kind = %q, want %q", label, got.Data.Kind, wantKind)
	}
	if strings.TrimSpace(got.Data.Message) == "" {
		t.Errorf("%s: 400 body data.message empty, want the offending input named", label)
	}
}

// assertPtyNotFound pins the declared 404 arm of the operations whose schema
// is the typed PtyNotFoundError {_tag, ptyID, message}.
func assertPtyNotFound(t *testing.T, label string, body []byte, wantID string) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%s: 404 body is not JSON: %v (%s)", label, err, body)
	}
	if got["_tag"] != "PtyNotFoundError" {
		t.Errorf("%s: 404 body _tag = %v, want PtyNotFoundError", label, got["_tag"])
	}
	if got["ptyID"] != wantID {
		t.Errorf("%s: 404 body ptyID = %v, want %q", label, got["ptyID"], wantID)
	}
	if msg, _ := got["message"].(string); strings.TrimSpace(msg) == "" {
		t.Errorf("%s: 404 body message empty, want a reason", label)
	}
	if len(got) != 3 {
		t.Errorf("%s: 404 body must carry exactly the declared fields (_tag, ptyID, message), got %v", label, got)
	}
}

// assertPtyConnectNotFound pins pty.connect's declared 404 arm, whose schema is
// the NotFoundError NamedError shape {name, data:{message}} (the shape the
// pinned upstream SDK's throwOnError path consumes). Its own helper (rather
// than the session-site assertNotFoundNamedError) because pty.connect's message
// names a PTY, not a session.
func assertPtyConnectNotFound(t *testing.T, label string, body []byte) {
	t.Helper()
	var got ptyNamedError
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%s: 404 body is not JSON: %v (%s)", label, err, body)
	}
	if got.Name != "NotFoundError" {
		t.Errorf("%s: 404 body name = %q, want NotFoundError", label, got.Name)
	}
	if strings.TrimSpace(got.Data.Message) == "" {
		t.Errorf("%s: 404 body data.message empty, want a reason", label)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("%s: 404 body is not a JSON object: %v (%s)", label, err, body)
	}
	if _, ok := raw["error"]; ok {
		t.Errorf("%s: 404 body carries the retired shim \"error\" envelope key: %s", label, body)
	}
}

// assertJSONContentType pins the application/json content type on every
// declared arm.
func assertJSONContentType(t *testing.T, label string, header http.Header) {
	t.Helper()
	if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("%s: Content-Type = %q, want application/json", label, ct)
	}
}

// TestPtyGetArms pins GET /pty/{ptyID} (pty.get): the declared 404 for a
// well-formed unknown ptyID, the declared 400 arms for an ill-shaped ptyID and
// for a malformed declared selector, and the no-403 rule for the operations
// whose declared set has none (an existing foreign directory selector is
// accepted as a selector and still answers 404 — there is no PTY anywhere).
func TestPtyGetArms(t *testing.T) {
	_, srv, served, foreign := newPtyRouteServer(t)

	for _, tc := range []struct {
		name, query string
		wantStatus  int
		wantKind    string
	}{
		{"unknown ptyID", "", http.StatusNotFound, ""},
		{"ill-shaped ptyID", "?directory=" + url.QueryEscape(served), http.StatusNotFound, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/pty/pty_1"+tc.query)
			if status != tc.wantStatus {
				t.Fatalf("GET /pty/pty_1%s: got %d, want %d. Body: %s", tc.query, status, tc.wantStatus, body)
			}
			assertPtyNotFound(t, tc.name, body, "pty_1")
		})
	}

	t.Run("ill-shaped ptyID names a params violation", func(t *testing.T) {
		status, header, body := doShimRequest(t, srv.URL, http.MethodGet, "/pty/terminal-1")
		if status != http.StatusBadRequest {
			t.Fatalf("GET /pty/terminal-1: got %d, want 400. Body: %s", status, body)
		}
		assertJSONContentType(t, "GET /pty/terminal-1", header)
		assertBadRequestKind(t, "GET /pty/terminal-1", body, "Params")
	})

	t.Run("selector arms", func(t *testing.T) {
		missing := filepath.Join(served, "missing")
		for _, tc := range []struct {
			name, query string
			wantStatus  int
			wantKind    string
		}{
			{"served workspace", "directory=" + url.QueryEscape(served), http.StatusNotFound, ""},
			{"served workspace via workspace param", "workspace=" + url.QueryEscape(served), http.StatusNotFound, ""},
			{"missing directory", "directory=" + url.QueryEscape(missing), http.StatusBadRequest, "Query"},
			{"blank workspace", "workspace=", http.StatusBadRequest, "Query"},
			{"selector names a file", "directory=" + url.QueryEscape(filepath.Join(served, "file")), http.StatusBadRequest, "Query"},
			{"foreign existing directory (no 403 declared)", "directory=" + url.QueryEscape(foreign), http.StatusNotFound, ""},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if strings.Contains(tc.query, "/file") {
					if err := os.WriteFile(filepath.Join(served, "file"), []byte("x"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/pty/pty_1?"+tc.query)
				if status != tc.wantStatus {
					t.Fatalf("GET /pty/pty_1?%s: got %d, want %d. Body: %s", tc.query, status, tc.wantStatus, body)
				}
				if tc.wantStatus == http.StatusBadRequest {
					assertBadRequestKind(t, tc.name, body, tc.wantKind)
					return
				}
				assertPtyNotFound(t, tc.name, body, "pty_1")
			})
		}
	})
}

// TestPtyRemoveArms pins DELETE /pty/{ptyID} (pty.remove) — declared responses
// 200 boolean, 400 BadRequestError, 404 PtyNotFoundError: the declared 404
// (never the boolean 200, which would report a removal of a PTY that never
// existed), the declared 400 arms, and the tolerated body the operation does
// not declare.
func TestPtyRemoveArms(t *testing.T) {
	_, srv, served, _ := newPtyRouteServer(t)

	t.Run("unknown ptyID answers the declared 404", func(t *testing.T) {
		status, header, body := doShimRequest(t, srv.URL, http.MethodDelete, "/pty/pty_42")
		if status != http.StatusNotFound {
			t.Fatalf("DELETE /pty/pty_42: got %d, want 404. Body: %s", status, body)
		}
		assertJSONContentType(t, "DELETE /pty/pty_42", header)
		assertPtyNotFound(t, "DELETE /pty/pty_42", body, "pty_42")
	})

	t.Run("a body is neither read nor required", func(t *testing.T) {
		status, _, body := doShimRequestBody(t, srv.URL, http.MethodDelete, "/pty/pty_42", `{"any":"thing"}`)
		if status != http.StatusNotFound {
			t.Fatalf("DELETE /pty/pty_42 with a body: got %d, want 404. Body: %s", status, body)
		}
		assertPtyNotFound(t, "DELETE body arm", body, "pty_42")
	})

	for _, tc := range []struct{ name, path, wantKind string }{
		{"ill-shaped ptyID", "/pty/nope_1", "Params"},
		{"missing directory selector", "/pty/pty_42?directory=" + url.QueryEscape(filepath.Join(served, "gone")), "Query"},
		{"blank directory selector", "/pty/pty_42?directory=", "Query"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, http.MethodDelete, tc.path)
			if status != http.StatusBadRequest {
				t.Fatalf("DELETE %s: got %d, want 400. Body: %s", tc.path, status, body)
			}
			assertBadRequestKind(t, tc.name, body, tc.wantKind)
		})
	}
}

// TestPtyUpdateArms pins PUT /pty/{ptyID} (pty.update) — declared responses
// 200 Pty, 400 BadRequest | InvalidRequestError, 404 PtyNotFoundError: the
// declared requestBody schema is validated BEFORE the lookup (non-object,
// undeclared key, trailing value, wrong title type, malformed size, missing
// rows/cols, non-positive rows/cols all answer the declared 400), an absent or
// empty body is well-formed (the schema marks no required member), and no
// ptyID can name a PTY, so every accepted body answers the declared 404.
func TestPtyUpdateArms(t *testing.T) {
	_, srv, served, _ := newPtyRouteServer(t)

	t.Run("accepted bodies answer the declared 404", func(t *testing.T) {
		for _, tc := range []struct{ name, body string }{
			{"title only", `{"title":"work"}`},
			{"size only", `{"size":{"rows":24,"cols":80}}`},
			{"title and size", `{"title":"work","size":{"rows":24,"cols":80}}`},
			{"empty object", `{}`},
			{"absent body", ""},
		} {
			t.Run(tc.name, func(t *testing.T) {
				status, _, body := doShimRequestBody(t, srv.URL, http.MethodPut, "/pty/pty_7", tc.body)
				if status != http.StatusNotFound {
					t.Fatalf("PUT /pty/pty_7 (%s): got %d, want 404. Body: %s", tc.name, status, body)
				}
				assertPtyNotFound(t, tc.name, body, "pty_7")
			})
		}
	})

	t.Run("declared 400 arms", func(t *testing.T) {
		for _, tc := range []struct{ name, body, path, wantKind string }{
			{"non-object body", `["x"]`, "/pty/pty_7", "Body"},
			{"unparseable body", `not-json`, "/pty/pty_7", "Body"},
			{"trailing second value", `{"title":"a"}{"title":"b"}`, "/pty/pty_7", "Body"},
			{"undeclared top-level key", `{"unknown":true}`, "/pty/pty_7", "Body"},
			{"title is not a string", `{"title":123}`, "/pty/pty_7", "Payload"},
			{"size is not an object", `{"size":"big"}`, "/pty/pty_7", "Payload"},
			{"size is null", `{"size":null}`, "/pty/pty_7", "Payload"},
			{"size carries an undeclared key", `{"size":{"rows":24,"cols":80,"depth":1}}`, "/pty/pty_7", "Payload"},
			{"size missing cols", `{"size":{"rows":24}}`, "/pty/pty_7", "Payload"},
			{"size rows not positive", `{"size":{"rows":0,"cols":80}}`, "/pty/pty_7", "Payload"},
			{"size cols negative", `{"size":{"rows":24,"cols":-1}}`, "/pty/pty_7", "Payload"},
			{"ill-shaped ptyID", `{"title":"work"}`, "/pty/term_1", "Params"},
			{"missing selector", `{"title":"work"}`, "/pty/pty_7?workspace=" + url.QueryEscape(filepath.Join(served, "gone")), "Query"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				status, _, body := doShimRequestBody(t, srv.URL, http.MethodPut, tc.path, tc.body)
				if status != http.StatusBadRequest {
					t.Fatalf("PUT %s (%s): got %d, want 400. Body: %s", tc.path, tc.name, status, body)
				}
				assertBadRequestKind(t, tc.name, body, tc.wantKind)
			})
		}
	})
}

// TestPtyConnectArms pins GET /pty/{ptyID}/connect (pty.connect) — declared
// responses 200 boolean, 403 Forbidden, 404 NotFoundError. The operation
// declares NO 400 arm, so an ill-shaped ptyID is answered 404 (never a 400 the
// contract does not declare); the declared 403 is the scope arm: a selector
// that is blank, names no existing directory, or names an existing directory
// this instance does not serve. The 403 schema is {_tag:"Forbidden"} with
// additionalProperties: false, so the body carries exactly that one field.
func TestPtyConnectArms(t *testing.T) {
	_, srv, served, foreign := newPtyRouteServer(t)

	t.Run("declared 404 arms", func(t *testing.T) {
		for _, tc := range []struct{ name, path string }{
			{"unknown ptyID", "/pty/pty_9/connect"},
			{"ill-shaped ptyID (no 400 is declared)", "/pty/term_9/connect"},
			{"served workspace selector", "/pty/pty_9/connect?directory=" + url.QueryEscape(served)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				status, header, body := doShimRequest(t, srv.URL, http.MethodGet, tc.path)
				if status != http.StatusNotFound {
					t.Fatalf("GET %s: got %d, want 404. Body: %s", tc.path, status, body)
				}
				assertJSONContentType(t, tc.name, header)
				assertPtyConnectNotFound(t, tc.name, body)
			})
		}
	})

	t.Run("declared 403 scope arms", func(t *testing.T) {
		for _, tc := range []struct{ name, query string }{
			{"foreign existing directory", "directory=" + url.QueryEscape(foreign)},
			{"foreign workspace param", "workspace=" + url.QueryEscape(foreign)},
			{"nonexistent directory", "directory=" + url.QueryEscape(filepath.Join(served, "gone"))},
			{"blank selector", "directory="},
		} {
			t.Run(tc.name, func(t *testing.T) {
				path := "/pty/pty_9/connect?" + tc.query
				status, header, body := doShimRequest(t, srv.URL, http.MethodGet, path)
				if status != http.StatusForbidden {
					t.Fatalf("GET %s: got %d, want 403. Body: %s", path, status, body)
				}
				assertJSONContentType(t, tc.name, header)
				var got map[string]any
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("%s: 403 body is not JSON: %v (%s)", tc.name, err, body)
				}
				if got["_tag"] != "Forbidden" {
					t.Errorf("%s: 403 body _tag = %v, want Forbidden", tc.name, got["_tag"])
				}
				if len(got) != 1 {
					t.Errorf("%s: 403 body = %v, want exactly {_tag: Forbidden} (additionalProperties false)", tc.name, got)
				}
			})
		}
	})
}

// TestPtyConnectTokenArms pins POST /pty/{ptyID}/connect-token
// (pty.connectToken) — declared responses 200 PtyTicketConnectToken, 400
// BadRequestError, 403 PtyForbiddenError, 404 PtyNotFoundError. The declared
// 200 is a one-time credential the shim never mints, so it is never served;
// the 400 arms come first for malformed input, the declared 403 answers an
// existing directory this instance does not serve, and every well-formed
// ptyID answers the typed PtyNotFoundError.
func TestPtyConnectTokenArms(t *testing.T) {
	_, srv, served, foreign := newPtyRouteServer(t)

	t.Run("unknown ptyID answers the declared 404", func(t *testing.T) {
		status, header, body := doShimRequest(t, srv.URL, http.MethodPost, "/pty/pty_3/connect-token")
		if status != http.StatusNotFound {
			t.Fatalf("POST /pty/pty_3/connect-token: got %d, want 404. Body: %s", status, body)
		}
		assertJSONContentType(t, "connect-token 404", header)
		assertPtyNotFound(t, "connect-token 404", body, "pty_3")
	})

	t.Run("served workspace selector still answers the declared 404", func(t *testing.T) {
		path := "/pty/pty_3/connect-token?directory=" + url.QueryEscape(served)
		status, _, body := doShimRequest(t, srv.URL, http.MethodPost, path)
		if status != http.StatusNotFound {
			t.Fatalf("POST %s: got %d, want 404. Body: %s", path, status, body)
		}
		assertPtyNotFound(t, "connect-token served selector", body, "pty_3")
	})

	t.Run("declared 400 arms", func(t *testing.T) {
		for _, tc := range []struct{ name, path, wantKind string }{
			{"ill-shaped ptyID", "/pty/term_3/connect-token", "Params"},
			{"blank selector", "/pty/pty_3/connect-token?workspace=", "Query"},
			{"nonexistent directory", "/pty/pty_3/connect-token?directory=" + url.QueryEscape(filepath.Join(served, "gone")), "Query"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				status, _, body := doShimRequest(t, srv.URL, http.MethodPost, tc.path)
				if status != http.StatusBadRequest {
					t.Fatalf("POST %s: got %d, want 400. Body: %s", tc.path, status, body)
				}
				assertBadRequestKind(t, tc.name, body, tc.wantKind)
			})
		}
	})

	t.Run("declared 403 scope arm", func(t *testing.T) {
		path := "/pty/pty_3/connect-token?directory=" + url.QueryEscape(foreign)
		status, _, body := doShimRequest(t, srv.URL, http.MethodPost, path)
		if status != http.StatusForbidden {
			t.Fatalf("POST %s: got %d, want 403. Body: %s", path, status, body)
		}
		var got struct {
			Tag     string `json:"_tag"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("403 body is not JSON: %v (%s)", err, body)
		}
		if got.Tag != "PtyForbiddenError" {
			t.Errorf("403 body _tag = %q, want PtyForbiddenError", got.Tag)
		}
		if strings.TrimSpace(got.Message) == "" {
			t.Error("403 body message empty, want the refused workspace named")
		}
	})
}

// TestPtyFamilyNeverFabricatesDeclaredSuccess is the truthfulness guard: for
// each of the five operations a well-formed, well-formed-selector request must
// answer a DECLARED error code — never 200 — and must carry none of the
// declared success payload's fields. The runtime keeps no pty registry and
// mints no connect ticket, so a 200 Pty / true / ticket would fabricate a
// terminal, a connection, or a credential.
func TestPtyFamilyNeverFabricatesDeclaredSuccess(t *testing.T) {
	_, srv, served, _ := newPtyRouteServer(t)

	requests := []struct{ name, method, path string }{
		{"pty.get", http.MethodGet, "/pty/pty_1"},
		{"pty.update", http.MethodPut, "/pty/pty_1"},
		{"pty.remove", http.MethodDelete, "/pty/pty_1"},
		{"pty.connect", http.MethodGet, "/pty/pty_1/connect"},
		{"pty.connectToken", http.MethodPost, "/pty/pty_1/connect-token"},
	}
	fabricated := []string{"id", "pid", "status", "ticket", "expires_in", "exitCode"}
	for _, req := range requests {
		t.Run(req.name, func(t *testing.T) {
			path := req.path + "?directory=" + url.QueryEscape(served)
			status, _, body := doShimRequestBody(t, srv.URL, req.method, path, `{"title":"work"}`)
			if status == http.StatusOK {
				t.Fatalf("%s answered 200 — the declared success payload must never be fabricated. Body: %s", req.name, body)
			}
			if status != http.StatusNotFound && status != http.StatusBadRequest && status != http.StatusForbidden {
				t.Fatalf("%s answered %d, which is not in the declared response set. Body: %s", req.name, status, body)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("%s: body is not JSON: %v (%s)", req.name, err, body)
			}
			for _, field := range fabricated {
				if _, ok := got[field]; ok {
					t.Errorf("%s: response carries fabricating field %q: %s", req.name, field, body)
				}
			}
		})
	}
}

// TestPtyUndeclaredMethodsKeepDefault404 pins the pre-change answer rule: none
// of the five declared response sets contains 405, so a method the operation
// does not declare keeps exactly what it answered before these routes existed —
// net/http's default 404 (never a 405 the contract does not declare, and never
// a silent fall-through to another operation's arm).
func TestPtyUndeclaredMethodsKeepDefault404(t *testing.T) {
	_, srv, _, _ := newPtyRouteServer(t)

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/pty/pty_1"},
		{http.MethodPatch, "/pty/pty_1"},
		{http.MethodPut, "/pty/pty_1/connect"},
		{http.MethodPost, "/pty/pty_1/connect"},
		{http.MethodDelete, "/pty/pty_1/connect-token"},
		{http.MethodGet, "/pty/pty_1/connect-token"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			status, header, body := doShimRequest(t, srv.URL, tc.method, tc.path)
			if status != http.StatusNotFound {
				t.Fatalf("%s %s: got %d, want the pre-change 404. Body: %s", tc.method, tc.path, status, body)
			}
			if strings.TrimSpace(string(body)) != "404 page not found" {
				t.Errorf("%s %s: body = %q, want net/http's default 404 body", tc.method, tc.path, body)
			}
			if ct := header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
				t.Errorf("%s %s: Content-Type = %q, want the net/http default text/plain", tc.method, tc.path, ct)
			}
		})
	}
}

// TestPtyFamilyNeighboursUntouched pins that registering the /pty/{ptyID}
// family left every neighbouring registered shape exactly as it was: bare /pty
// (200 empty list, 501-typed create), /pty/shells, and the default 404 for any
// other /pty/* shape (no catch-all was introduced).
func TestPtyFamilyNeighboursUntouched(t *testing.T) {
	_, srv, _, _ := newPtyRouteServer(t)

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/pty")
	if status != http.StatusOK {
		t.Fatalf("GET /pty: got %d, want 200. Body: %s", status, body)
	}
	var list []map[string]any
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("GET /pty body is not a Pty array: %v (%s)", err, body)
	}
	if len(list) != 0 {
		t.Errorf("GET /pty = %v, want the truthful empty list", list)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodPost, "/pty")
	if status != http.StatusNotImplemented {
		t.Fatalf("POST /pty: got %d, want the typed 501 pty.create narrowing. Body: %s", status, body)
	}

	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/pty/shells")
	if status != http.StatusOK {
		t.Fatalf("GET /pty/shells: got %d, want 200. Body: %s", status, body)
	}

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/pty/pty_1/unknown"},
		{http.MethodGet, "/pty/pty_1/connect/extra"},
		{http.MethodGet, "/pty/"},
		{http.MethodGet, "/pty/pty_1/connect-token/extra"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			status, _, body := doShimRequest(t, srv.URL, tc.method, tc.path)
			if status != http.StatusNotFound {
				t.Fatalf("%s %s: got %d, want the net/http default 404 (no catch-all). Body: %s", tc.method, tc.path, status, body)
			}
			if strings.TrimSpace(string(body)) != "404 page not found" {
				t.Errorf("%s %s: body = %q, want net/http's default 404 body", tc.method, tc.path, body)
			}
		})
	}
}

// TestPtyFamilyServedSurfaceArtifact ties the runtime contract to the
// regenerated declared-vs-served artifact: the five /pty/{ptyID} operations
// must be ACCOUNTED — no longer drift (that was ROUTE-ADD-105..109's point: the
// declared error vocabulary is served, not an untyped stub) — and, since
// SHIM-GAP-004, recorded as ERROR-CONTRACT NARROWINGS rather than `covered`.
//
// The earlier version of this test asserted `covered`, which only asked whether
// the served status was IN the declared set. It was: the handlers answer the
// declared 404 (and 400/403). But the declared 200 is unreachable — the runtime
// keeps no PTY registry and spawns no pseudo-terminal — so scoring the row
// `covered` hid the fact that a client can never receive the declared success.
// The assertion strength is unchanged: the family must not be drift, and each
// row must carry a recorded rationale. The comparison keys on
// path+method+operationId because the generated ids are positional and a
// regeneration renumbers them.
func TestPtyFamilyServedSurfaceArtifact(t *testing.T) {
	artifact := filepath.Join("..", "..", "..", "specs", "openapi", "upstream",
		"opencode-declared-vs-served-1.18.33.json")
	raw, err := os.ReadFile(artifact)
	if os.IsNotExist(err) {
		t.Skipf("declared-vs-served artifact not present at %s", artifact)
	}
	if err != nil {
		t.Fatalf("read %s: %v", artifact, err)
	}

	type row struct {
		Path          string `json:"path"`
		Method        string `json:"method"`
		OperationID   string `json:"operationId"`
		ServedOutcome string `json:"served_outcome"`
		Rationale     string `json:"rationale"`
	}
	var doc struct {
		Covered  []row `json:"covered"`
		Drift    []row `json:"drift_declared_not_served"`
		Narrowed []row `json:"narrowed_error_contract_only"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", artifact, err)
	}
	if len(doc.Covered) == 0 || len(doc.Drift) == 0 {
		t.Fatalf("artifact looks empty (covered=%d drift=%d) — refusing to pass vacuously",
			len(doc.Covered), len(doc.Drift))
	}

	key := func(r row) string { return r.Method + " " + r.Path + " " + r.OperationID }
	want := map[string]bool{
		key(row{Path: "/pty/{ptyID}", Method: "GET", OperationID: "pty.get"}):                         false,
		key(row{Path: "/pty/{ptyID}", Method: "PUT", OperationID: "pty.update"}):                      false,
		key(row{Path: "/pty/{ptyID}", Method: "DELETE", OperationID: "pty.remove"}):                   false,
		key(row{Path: "/pty/{ptyID}/connect", Method: "GET", OperationID: "pty.connect"}):             false,
		key(row{Path: "/pty/{ptyID}/connect-token", Method: "POST", OperationID: "pty.connectToken"}): false,
	}
	for _, r := range doc.Narrowed {
		k := key(r)
		if _, ok := want[k]; !ok {
			continue
		}
		if !strings.HasPrefix(r.ServedOutcome, "404") {
			t.Errorf("narrowed %s: served_outcome = %q, want the declared 404 the handlers answer", k, r.ServedOutcome)
		}
		if strings.TrimSpace(r.Rationale) == "" {
			t.Errorf("narrowed %s carries no rationale — an unexplained narrowing", k)
		}
		want[k] = true
	}
	for k, found := range want {
		if !found {
			t.Errorf("%s is not a recorded error-contract narrowing in %s — the /pty/{ptyID} family must be accounted for, not drift",
				k, artifact)
		}
	}
	for _, r := range doc.Drift {
		if _, ok := want[key(r)]; ok {
			t.Errorf("%s is still classified as drift in %s", key(r), artifact)
		}
	}
	for _, r := range doc.Covered {
		if _, ok := want[key(r)]; ok {
			t.Errorf("covered %s in %s claims the declared contract is served; its declared 200 is unreachable, so it must be recorded as a narrowing",
				key(r), artifact)
		}
	}
}
