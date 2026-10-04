package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Upstream find.symbols (specs/openapi/upstream/openapi-1.18.33.json
// paths."/find/symbol".get) declares exactly two responses: 200 Symbol[]
// "Symbols" (workspace symbol search via LSP) and 400 BadRequestError. Before
// ROUTE-FIX-002 the sub-path was answered by handleFindSub's stub arm —
// HTTP 501 NOT_IMPLEMENTED for EVERY request — so neither declared code was
// reachable from outside (the artifact's OUTCOME-MISMATCH row
// SHIM-DRIFT-085, "declared 200,400, served 501").
//
// These tests are the declared-contract battery: every GET to the operation
// must answer inside {200, 400} — never 501, never an undeclared code — with
// the 200 body shaped as the upstream Symbol[] (an array, never null; the
// truthful empty list, because the shim has no LSP integration and therefore
// no symbol producer) and the 400 arms carrying the shim's
// INVALID_REQUEST envelope.

// symbolEntry is the upstream Symbol schema used to prove the 200 body
// conforms field-for-field (name, kind, location{uri,range}; the empty list
// is the only truthful body while no LSP producer exists, but the decode
// target pins the shape a future producer must emit).
type symbolEntry struct {
	Name     string `json:"name"`
	Kind     int    `json:"kind"`
	Location struct {
		URI   string `json:"uri"`
		Range struct {
			Start struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"start"`
			End struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"end"`
		} `json:"range"`
	} `json:"location"`
}

// findSymbolsBody is the parsed 200 body plus its raw form (so the empty
// array can be distinguished from JSON null).
type findSymbolsBody struct {
	raw     string
	symbols []symbolEntry
}

func getFindSymbols(t *testing.T, base, path string) (int, findSymbolsBody) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", path, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", path, err)
	}
	body := findSymbolsBody{raw: string(data)}
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(data, &body.symbols); err != nil {
			t.Fatalf("GET %s 200 body is not a Symbol array: %v (%s)", path, err, data)
		}
	}
	return resp.StatusCode, body
}

// findSymbolsEnvelope is the shim's error envelope, the declared
// BadRequestError shape for this operation.
type findSymbolsEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeFindSymbolsEnvelope(t *testing.T, body string) findSymbolsEnvelope {
	t.Helper()
	var env findSymbolsEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("error body is not the shim envelope: %v (%s)", err, body)
	}
	return env
}

// TestFindSymbolsServesDeclared200 answers the happy path: GET /find/symbol
// returns 200 with the truthful empty Symbol list (the shim has no LSP
// integration — handleLSP reports enabled:false unconditionally — so there is
// no producer for the declared Symbol shape and the handler never fabricates
// entries), and the declared optional query parameters are accepted.
func TestFindSymbolsServesDeclared200(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	t.Run("symbol query answers the declared empty list", func(t *testing.T) {
		status, body := getFindSymbols(t, srv.URL, "/find/symbol?query=handleFind")
		if status != http.StatusOK {
			t.Fatalf("GET /find/symbol?query=handleFind: got %d, want 200 (declared). Body: %s", status, body.raw)
		}
		if strings.TrimSpace(body.raw) == "null" {
			t.Fatalf("GET /find/symbol answered JSON null — the document declares an array; body: %s", body.raw)
		}
		if len(body.symbols) != 0 {
			t.Fatalf("GET /find/symbol returned %d symbols — the shim has no LSP producer, entries would be fabricated: %s", len(body.symbols), body.raw)
		}
	})

	t.Run("declared optional query params accepted", func(t *testing.T) {
		for _, qs := range []string{
			"?query=handleFind&directory=/tmp",
			"?query=handleFind&workspace=default",
			"?query=handleFind&directory=/tmp&workspace=default",
		} {
			status, body := getFindSymbols(t, srv.URL, "/find/symbol"+qs)
			if status != http.StatusOK {
				t.Fatalf("GET /find/symbol%s: got %d, want 200 (optional selectors accepted). Body: %s", qs, status, body.raw)
			}
			if len(body.symbols) != 0 {
				t.Fatalf("GET /find/symbol%s: %d symbols, want the truthful empty list: %s", qs, len(body.symbols), body.raw)
			}
		}
	})
}

// TestFindSymbolsServesDeclared400 pins the declared error arm: every
// malformed request answers 400 with the shim's INVALID_REQUEST envelope.
func TestFindSymbolsServesDeclared400(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, tc := range []struct {
		name, path, wantMsgPart string
	}{
		{"missing query", "/find/symbol", "?query= is required"},
		{"blank query", "/find/symbol?query=", "?query= is required"},
		{"whitespace query", "/find/symbol?query=%20", "?query= is required"},
		{"blank directory", "/find/symbol?query=handleFind&directory=", `query parameter "directory" must not be blank`},
		{"blank workspace", "/find/symbol?query=handleFind&workspace=", `query parameter "workspace" must not be blank`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, body := getFindSymbols(t, srv.URL, tc.path)
			if status != http.StatusBadRequest {
				t.Fatalf("GET %s: got %d, want 400 (declared BadRequestError). Body: %s", tc.path, status, body.raw)
			}
			env := decodeFindSymbolsEnvelope(t, body.raw)
			if env.Error.Code != "INVALID_REQUEST" {
				t.Errorf("error code = %q, want INVALID_REQUEST (%s)", env.Error.Code, body.raw)
			}
			if !strings.Contains(env.Error.Message, tc.wantMsgPart) {
				t.Errorf("error message %q does not mention %q", env.Error.Message, tc.wantMsgPart)
			}
		})
	}
}

// TestFindSymbolsNeverAnswers501 is the SHIM-DRIFT-085 regression: before
// ROUTE-FIX-002 every GET on the sub-path — with or without a query —
// answered the 501 stub, so neither declared response was reachable. Every
// request here must answer a declared code (200 or 400) and never the
// not-implemented envelope.
func TestFindSymbolsNeverAnswers501(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, path := range []string{
		"/find/symbol",
		"/find/symbol?query=handleFind",
		"/find/symbol?query=",
		"/find/symbol?query=handleFind&directory=/tmp",
	} {
		req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("build GET %s: %v", path, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		status := resp.StatusCode
		if status == http.StatusNotImplemented {
			t.Errorf("GET %s answered 501 — SHIM-DRIFT-085 regression. Body: %s", path, data)
			continue
		}
		if strings.Contains(string(data), "not_implemented") {
			t.Errorf("GET %s answered the not-implemented envelope on a declared GET — SHIM-DRIFT-085 regression. Body: %s", path, data)
		}
		switch status {
		case http.StatusOK, http.StatusBadRequest:
		default:
			t.Errorf("GET %s: got %d, want a declared code (200 or 400). Body: %s", path, status, data)
		}
	}
}

// TestFindSymbolsMethodGuardKeepsTyped501: only GET is a declared opencode
// operation for /find/symbol and 405 is not part of the operation's declared
// response set, so undeclared methods keep the typed not-implemented envelope
// (the session.diff sibling convention, ROUTE-FIX-010) naming the operation
// and the real GET route.
func TestFindSymbolsMethodGuardKeepsTyped501(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req, err := http.NewRequest(method, srv.URL+"/find/symbol?query=handleFind", nil)
		if err != nil {
			t.Fatalf("build %s /find/symbol: %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s /find/symbol: %v", method, err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("%s /find/symbol: got %d, want 501 (undeclared method keeps the typed stub). Body: %s", method, resp.StatusCode, data)
			continue
		}
		var env notImplementedResponse
		if err := json.Unmarshal(data, &env); err != nil {
			t.Errorf("%s /find/symbol 501 body is not the typed envelope: %v (%s)", method, err, data)
			continue
		}
		if env.Error != "not_implemented" || env.Operation != "find.symbols" {
			t.Errorf("%s /find/symbol typed envelope = %+v, want error not_implemented / operation find.symbols", method, env)
		}
	}
}

// TestFindSymbolsNeighboursUnchanged: the fix must not disturb the sibling
// arms of handleFindSub — /find/file (declared 200), the plural spelling
// /find/symbols (same "symbol" prefix arm), the unknown-sub-path 404 and the
// /find root route.
func TestFindSymbolsNeighboursUnchanged(t *testing.T) {
	_, srv := newTestServer(&mockDB{})
	defer srv.Close()

	t.Run("GET /find/file still searches files", func(t *testing.T) {
		status, _, data := doShimRequest(t, srv.URL, http.MethodGet, "/find/file?query=*.go")
		if status != http.StatusOK {
			t.Fatalf("GET /find/file?query=*.go: got %d, want 200. Body: %s", status, data)
		}
		if !strings.Contains(string(data), "files") {
			t.Errorf("GET /find/file body lost the files array: %s", data)
		}
	})

	t.Run("plural /find/symbols rides the same prefix arm", func(t *testing.T) {
		status, _, data := doShimRequest(t, srv.URL, http.MethodGet, "/find/symbols?query=handleFind")
		if status != http.StatusOK {
			t.Fatalf("GET /find/symbols?query=handleFind: got %d, want 200 (same symbol prefix arm). Body: %s", status, data)
		}
		if strings.TrimSpace(string(data)) != "[]" {
			t.Errorf("GET /find/symbols body = %s, want []", data)
		}
	})

	t.Run("unknown find sub-path still 404", func(t *testing.T) {
		status, _, data := doShimRequest(t, srv.URL, http.MethodGet, "/find/nothing?query=x")
		if status != http.StatusNotFound {
			t.Fatalf("GET /find/nothing: got %d, want 404. Body: %s", status, data)
		}
		env := decodeFindSymbolsEnvelope(t, string(data))
		if env.Error.Code != "NOT_FOUND" {
			t.Errorf("unknown sub-path error code = %q, want NOT_FOUND", env.Error.Code)
		}
	})

	t.Run("GET /find root still searches files", func(t *testing.T) {
		status, _, data := doShimRequest(t, srv.URL, http.MethodGet, "/find?pattern=*.go")
		if status != http.StatusOK {
			t.Fatalf("GET /find?pattern=*.go: got %d, want 200. Body: %s", status, data)
		}
		if !strings.Contains(string(data), "files") {
			t.Errorf("GET /find body lost the files array: %s", data)
		}
	})
}

// TestFindSymbolsChiMountMatchesProductionWiring is the BUG-009-shaped
// regression for this route: through a parent chi router mounted with
// MountPatterns (the shape cmd/consensus/main.go uses), /find/symbol must
// reach the shim handler instead of chi's 404 — and must answer the declared
// contract there too.
func TestFindSymbolsChiMountMatchesProductionWiring(t *testing.T) {
	shim := NewServer(&mockDB{}, "test-key", nil, nil)
	shim.skipAuth = true

	router := chi.NewRouter()
	for _, pattern := range MountPatterns {
		router.Handle(pattern, shim.Handler())
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	status, _, data := doShimRequest(t, srv.URL, http.MethodGet, "/find/symbol?query=handleFind")
	if status == http.StatusNotFound {
		t.Fatalf("GET /find/symbol via chi mount returned 404 — MountPatterns did not pass the sub-path through. Body: %s", data)
	}
	if status != http.StatusOK {
		t.Fatalf("GET /find/symbol via chi mount: got %d, want 200. Body: %s", status, data)
	}
	if strings.TrimSpace(string(data)) != "[]" {
		t.Errorf("chi-mounted /find/symbol body = %q, want [] (the mock store holds no symbols)", strings.TrimSpace(string(data)))
	}
}
