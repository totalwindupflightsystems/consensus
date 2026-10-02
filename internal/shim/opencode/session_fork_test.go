package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wojons/consensus/internal/api"
)

// postFork issues POST /session/{id}/fork and decodes the JSON answer. An
// empty body is meaningful here (the upstream requestBody is optional), so it
// is sent verbatim rather than substituted.
func postFork(t *testing.T, base, path, body string) (int, map[string]any) {
	t.Helper()
	status, _, raw := postCommand(t, base, path, body)
	decoded := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			decoded = map[string]any{"_raw": string(raw)}
		}
	}
	return status, decoded
}

// errorCode digs the shim's typed error code ("INVALID_REQUEST", "NOT_FOUND")
// out of an error envelope, so the assertion names the contract arm rather than
// only the status line.
func errorCode(body map[string]any) string {
	env, ok := body["error"].(map[string]any)
	if !ok {
		return ""
	}
	code, _ := env["code"].(string)
	return code
}

// TestSessionForkCreatesChildSession answers upstream session.fork
// (ROUTE-FIX-011, SHIM-DRIFT-117, declared responses: 200 Session, 400, 404):
// POST /session/{id}/fork must create a new session forked from the named one
// instead of the pre-fix untyped 501 stub. The child row carries
// parent_id = the source session (the native fork link GET
// /session/{id}/children reads back) and inherits the source session's agent,
// model and goal while its own counters start fresh.
func TestSessionForkCreatesChildSession(t *testing.T) {
	_, srv, conn := newCommandStoreTestServer(t)
	ctx := context.Background()

	status, body := postFork(t, srv.URL, "/session/s1/fork", "")
	if status != http.StatusOK {
		t.Fatalf("POST /session/s1/fork: got %d, want 200 (declared). Body: %v", status, body)
	}
	childID, _ := body["id"].(string)
	if childID == "" {
		t.Fatalf("fork response carries no session id: %v", body)
	}
	if childID == "s1" {
		t.Fatalf("fork reused the source session id %q instead of allocating a new session", childID)
	}
	if got := body["parentID"]; got != "s1" {
		t.Errorf("fork parentID = %v, want s1 (the fork link)", got)
	}
	if got := body["title"]; got != "worker" {
		t.Errorf("fork title = %v, want the source agent_name \"worker\"", got)
	}
	if got := body["model"]; got != "m" {
		t.Errorf("fork model = %v, want the source model_id \"m\"", got)
	}
	if got := body["status"]; got != "booting" {
		t.Errorf("fork status = %v, want booting (a fresh session)", got)
	}

	// The child is a real row in the sessions store, linked to its parent.
	rows, err := conn.Query(ctx,
		`SELECT id, parent_id, status, agent_name, model_id, goal, iteration, tokens_used_in, tokens_used_out
		 FROM sessions WHERE parent_id = $1`, "s1")
	if err != nil {
		t.Fatalf("query forked sessions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("sessions with parent_id='s1' = %d, want the one forked child: %v", len(rows), rows)
	}
	child := rows[0]
	if toString(child["id"]) != childID {
		t.Errorf("stored child id = %q, response said %q", toString(child["id"]), childID)
	}
	if toString(child["model_id"]) != "m" || toString(child["agent_name"]) != "worker" {
		t.Errorf("stored child shape = agent %q model %q, want the inherited worker/m",
			toString(child["agent_name"]), toString(child["model_id"]))
	}
	if toString(child["goal"]) != "g" {
		t.Errorf("stored child goal = %q, want the inherited \"g\"", toString(child["goal"]))
	}
	for _, col := range []string{"iteration", "tokens_used_in", "tokens_used_out"} {
		if toInt64(child[col]) != 0 {
			t.Errorf("forked child %s = %d, want 0 (a fork starts its own accounting)", col, toInt64(child[col]))
		}
	}

	// The shim registers the mapping the way createSession does.
	mapped, err := conn.Query(ctx,
		`SELECT external_id FROM shim_session_map WHERE shim_type = 'opencode' AND session_id = $1`, childID)
	if err != nil {
		t.Fatalf("query shim_session_map: %v", err)
	}
	if len(mapped) != 1 {
		t.Errorf("shim_session_map rows for the forked child = %d, want 1", len(mapped))
	}

	// Wiring: the child is reachable through the ordinary children route and
	// readable back through the native service layer the running shim wraps.
	status, _, raw := doShimRequest(t, srv.URL, http.MethodGet, "/session/s1/children")
	if status != http.StatusOK {
		t.Fatalf("GET /session/s1/children: got %d, want 200. Body: %s", status, raw)
	}
	var children []map[string]any
	if err := json.Unmarshal(raw, &children); err != nil {
		t.Fatalf("children body is not a JSON array: %v (%s)", err, raw)
	}
	found := false
	for _, c := range children {
		if toString(c["id"]) == childID {
			found = true
			// DF-CONSENSUS-47: the children listing must carry the same model
			// field GET /session/{id} returns for the same row.
			if got := toString(c["model"]); got != "m" {
				t.Errorf("children listing model = %q, want the child's model_id \"m\" (listChildren must project model_id)", got)
			}
		}
	}
	if !found {
		t.Errorf("GET /session/s1/children does not list the forked child %s: %v", childID, children)
	}

	status, _, raw = doShimRequest(t, srv.URL, http.MethodGet, "/session/"+childID)
	if status != http.StatusOK {
		t.Errorf("GET /session/%s after fork: got %d, want 200. Body: %s", childID, status, raw)
	}

	got, err := api.NewService(conn, nil).Sessions.GetSession(ctx, childID)
	if err != nil {
		t.Fatalf("native GetSession(%s) after fork: %v", childID, err)
	}
	if got.ParentID == nil || *got.ParentID != "s1" {
		t.Errorf("native ParentID for the forked child = %v, want s1", got.ParentID)
	}
}

// TestSessionForkForkPointArms exercises the declared 400/404 arms of a named
// fork point against a real store, and proves a rejected fork point never
// creates a session.
func TestSessionForkForkPointArms(t *testing.T) {
	_, srv, conn := newCommandStoreTestServer(t)
	ctx := context.Background()

	if err := conn.Exec(ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created)
		 VALUES ('user_message', 'fork me here', 's1', 1)`); err != nil {
		t.Fatalf("seed a fork point: %v", err)
	}

	// 200: a fork point that exists in the source session.
	status, body := postFork(t, srv.URL, "/session/s1/fork", `{"messageID":"msg-1"}`)
	if status != http.StatusOK {
		t.Fatalf("fork at an existing message: got %d, want 200 (declared). Body: %v", status, body)
	}
	if got := body["parentID"]; got != "s1" {
		t.Errorf("fork at message parentID = %v, want s1", got)
	}

	// 400: a messageID that violates the declared ^msg shape.
	status, body = postFork(t, srv.URL, "/session/s1/fork", `{"messageID":"not-a-message-id"}`)
	if status != http.StatusBadRequest {
		t.Errorf("fork with a non-^msg messageID: got %d, want 400 (declared). Body: %v", status, body)
	}
	if code := errorCode(body); code != "INVALID_REQUEST" {
		t.Errorf("non-^msg fork point error code = %q, want INVALID_REQUEST", code)
	}

	// 404: a well-formed fork point the session does not hold.
	status, body = postFork(t, srv.URL, "/session/s1/fork", `{"messageID":"msg-999"}`)
	if status != http.StatusNotFound {
		t.Errorf("fork at an unknown message: got %d, want 404 (declared). Body: %v", status, body)
	}
	if code := errorCode(body); code != "NOT_FOUND" {
		t.Errorf("unknown fork point error code = %q, want NOT_FOUND", code)
	}

	// Only the successful arm may have created a child.
	rows, err := conn.Query(ctx, `SELECT id FROM sessions WHERE parent_id = $1`, "s1")
	if err != nil {
		t.Fatalf("query forked sessions: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("rejected fork points created %d children, want exactly the 1 from the 200 arm", len(rows))
	}
}

// TestSessionForkContractArms covers the remaining declared arms: a malformed
// body is 400 and an unknown source session is 404 (never the pre-fix 501).
func TestSessionForkContractArms(t *testing.T) {
	// Malformed JSON body: rejected before any session lookup.
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
	status, body := postFork(t, srv.URL, "/session/s1/fork", `{"messageID":`)
	srv.Close()
	if status != http.StatusBadRequest {
		t.Errorf("malformed fork body: got %d, want 400 (declared). Body: %v", status, body)
	}
	if code := errorCode(body); code != "INVALID_REQUEST" {
		t.Errorf("malformed fork body error code = %q, want INVALID_REQUEST", code)
	}

	// Unknown source session: the declared NotFoundError.
	_, srv2 := newTestServer(&mockDB{})
	status, body = postFork(t, srv2.URL, "/session/ses_missing/fork", "")
	srv2.Close()
	if status != http.StatusNotFound {
		t.Errorf("fork from an unknown session: got %d, want 404 (declared). Body: %v", status, body)
	}
	if code := errorCode(body); code != "NOT_FOUND" {
		t.Errorf("unknown session error code = %q, want NOT_FOUND", code)
	}
}

// TestSessionForkNeighboursUntouched guards the neighbours: non-POST on the
// fork sub-path keeps the pre-existing stub-list 501, the remaining stub subs
// are untouched, and POST /session/{id}/message keeps its own contract.
func TestSessionForkNeighboursUntouched(t *testing.T) {
	_, srv := newTestServer(&mockDB{queryRow: rowOf(map[string]any{"id": "s1"})})
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/session/s1/fork")
		if status != http.StatusNotImplemented {
			t.Errorf("%s /session/s1/fork: got %d, want 501 (stub-list residual). Body: %s",
				method, status, body)
		}
	}

	for _, sub := range []string{"/session/s1/shell", "/session/s1/summarize"} {
		status, _, body := postCommand(t, srv.URL, sub, `{}`)
		if status != http.StatusNotImplemented {
			t.Errorf("POST %s: got %d, want 501 (sibling stub). Body: %s", sub, status, body)
		}
	}

	// ROUTE-FIX-012 / SHIM-DRIFT-118: /session/{id}/init is now served for
	// real (no longer a 501 stub). With an empty body it returns 400
	// (missing required fields), which proves the route reaches the shim.
	status, _, body := postCommand(t, srv.URL, "/session/s1/init", `{}`)
	if status != http.StatusBadRequest {
		t.Errorf("POST /session/s1/init: got %d, want 400 (served for real, missing fields). Body: %s", status, body)
	}

	status, _, body = postCommand(t, srv.URL, "/session/s1/message",
		`{"parts":[{"type":"text","text":"hi"}]}`)
	if status != http.StatusServiceUnavailable {
		t.Errorf("POST /session/s1/message: got %d, want 503. Body: %s", status, body)
	}
}

// TestSessionForkChiMount is the BUG-009 regression for the newly served
// sub-path: /session/{id}/fork must reach the shim's own handler through a
// parent chi router mounted with MountPatterns (a shim-produced status proves
// the route is wired; chi's plain-text 404 would mean the mount lost it).
func TestSessionForkChiMount(t *testing.T) {
	s := NewServer(&mockDB{queryRow: rowOf(map[string]any{
		"id": "s1", "agent_name": "worker", "model_id": "m", "goal": "g",
	})}, "test-key", nil, nil)
	s.skipAuth = true
	r := chi.NewRouter()
	for _, p := range MountPatterns {
		r.Handle(p, s.Handler())
	}
	srv := httptest.NewServer(r)
	defer srv.Close()

	status, body := postFork(t, srv.URL, "/session/s1/fork", `{}`)
	if status != http.StatusOK {
		t.Fatalf("POST /session/s1/fork via chi mount: got %d, want 200 (shim reached). Body: %v", status, body)
	}
	if !strings.Contains(body["id"].(string), "-") {
		t.Errorf("chi-mounted fork returned id %v, want an allocated session id", body["id"])
	}
}
