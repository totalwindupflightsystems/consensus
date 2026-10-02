package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

// p1SessionRouteTestServer is the command-store harness plus one in-flight
// session (s2, status 'thinking' — the Consensus analog of an upstream turn
// in flight) and one held message (memory_events id 42 in s1), so the 404 and
// 409 arms have real rows to answer from.
func p1SessionRouteTestServer(t *testing.T) (*Server, *httptest.Server, db.DB) {
	t.Helper()
	s, srv, conn := newCommandStoreTestServer(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`INSERT INTO sessions (id, agent_name, model_id, status, goal, iteration, created_at)
		 VALUES ('s2', 'worker', 'm', 'thinking', 'g2', 3, '2026-09-30T00:00:00Z')`,
		`INSERT INTO memory_events (id, type, content, session_id, iteration_created)
		 VALUES (42, 'user_instruction', 'hi', 's1', 0)`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare p1 route test database: %v", err)
		}
	}
	return s, srv, conn
}

// p1Post issues a POST with a verbatim body ("" sends no body at all) and
// returns status + decoded object body.
func p1Post(t *testing.T, base, path, body string) (int, map[string]any) {
	t.Helper()
	status, _, raw := doShimRequestBody(t, base, http.MethodPost, path, body)
	decoded := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			decoded = map[string]any{"_raw": string(raw)}
		}
	}
	return status, decoded
}

// assertP1Error checks the sibling writeOpencodeError envelope shape and code.
func assertP1Error(t *testing.T, path, body string, status int, got map[string]any, code string) {
	t.Helper()
	if errorCode(got) != code {
		t.Errorf("%s: error.code = %v, want %q (status %d, body %v)", path, errorCode(got), code, status, got)
	}
}

// ============================================================================
// ROUTE-FIX-014 — POST /session/{sessionID}/revert (session.revert)
// declared responses 200 Session, 400 BadRequest | InvalidRequestError,
// 404 NotFoundError, 409 SessionBusyError.
// ============================================================================

// TestSessionRevertTruthfulArms covers the declared contract of
// session.revert. The Consensus runtime keeps no revert engine — nothing is
// reverted and no turn is dispatched — so the happy path answers the declared
// 200 with the session AS IT IS (sessionInit translates a turn, this one has
// no analog to run), never a claimed reverted state. A session mid-turn
// (status 'thinking', the runtime's real in-flight state) answers the
// declared 409 SessionBusyError because the analogous upstream condition —
// revert while a prompt is in flight — genuinely holds in the sessions
// table; idle sessions answer 200.
func TestSessionRevertTruthfulArms(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	// Unknown session first: the declared 404 (NotFoundError) before any
	// body validation outcome is surfaced.
	status, body := p1Post(t, srv.URL, "/session/smissing/revert", `{"messageID":"msg-42"}`)
	if status != http.StatusNotFound {
		t.Fatalf("POST /session/smissing/revert: got %d, want 404 (declared NotFoundError). Body: %v", status, body)
	}

	// Present-but-malformed body -> declared 400.
	status, body = p1Post(t, srv.URL, "/session/s1/revert", `{"messageID":`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /session/s1/revert (malformed): got %d, want 400. Body: %v", status, body)
	}
	assertP1Error(t, "/session/s1/revert (malformed)", "", status, body, "INVALID_REQUEST")

	// messageID is required by the declared schema (required: [messageID]).
	status, body = p1Post(t, srv.URL, "/session/s1/revert", `{}`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /session/s1/revert (missing messageID): got %d, want 400. Body: %v", status, body)
	}
	assertP1Error(t, "/session/s1/revert (missing messageID)", "", status, body, "INVALID_REQUEST")

	// A value not matching the declared pattern ^msg -> declared 400.
	status, body = p1Post(t, srv.URL, "/session/s1/revert", `{"messageID":"not-a-msg"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /session/s1/revert (bad shape): got %d, want 400. Body: %v", status, body)
	}
	assertP1Error(t, "/session/s1/revert (bad shape)", "", status, body, "INVALID_REQUEST")

	// Well-formed messageID the session does not hold -> declared 404.
	status, body = p1Post(t, srv.URL, "/session/s1/revert", `{"messageID":"msg-999"}`)
	if status != http.StatusNotFound {
		t.Fatalf("POST /session/s1/revert (unknown message): got %d, want 404. Body: %v", status, body)
	}

	// In-flight session (status 'thinking' — the runtime's real mid-turn
	// state) -> declared 409 SessionBusyError, the analog of upstream's
	// revert-while-prompt-in-flight condition.
	status, body = p1Post(t, srv.URL, "/session/s2/revert", `{"messageID":"msg-42"}`)
	if status != http.StatusConflict {
		t.Fatalf("POST /session/s2/revert (busy): got %d, want 409 (SessionBusyError). Body: %v", status, body)
	}
	if got, _ := body["_tag"].(string); got != "SessionBusyError" {
		t.Errorf("POST /session/s2/revert (busy): _tag = %v, want SessionBusyError (declared 409 body)", got)
	}
	if got, _ := body["sessionID"].(string); got != "s2" {
		t.Errorf("POST /session/s2/revert (busy): sessionID = %v, want s2", got)
	}

	// Idle session holding the message -> declared 200 with the session AS
	// IT IS. No revert engine exists, so nothing was reverted and the body
	// must not claim otherwise (truthfulness convention, handleSyncStart).
	status, body = p1Post(t, srv.URL, "/session/s1/revert", `{"messageID":"msg-42"}`)
	if status != http.StatusOK {
		t.Fatalf("POST /session/s1/revert (happy): got %d, want 200. Body: %v", status, body)
	}
	if got, _ := body["id"].(string); got != "s1" {
		t.Errorf("POST /session/s1/revert (happy): body id = %v, want s1 (the declared 200 body is a Session)", got)
	}
}

// TestSessionRevertMethodGuard pins the non-POST behaviour: only POST is a
// declared opencode operation for /session/{sessionID}/revert, and 405 is not
// part of the declared response set, so every other method keeps the
// pre-existing stub-list 501.
func TestSessionRevertMethodGuard(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/session/s1/revert")
		if status != http.StatusNotImplemented {
			t.Errorf("%s /session/s1/revert: got %d, want 501 (stub-list residual). Body: %s",
				method, status, body)
		}
	}
}

// ============================================================================
// ROUTE-FIX-015 — DELETE /session/{sessionID}/share (session.unshare)
// declared responses 200 Session, 400 Bad request, 404 NotFoundError,
// 500 InternalServerError.
// ============================================================================

// TestSessionUnshareTruthfulArms covers session.unshare. The Consensus runtime
// keeps no share concept at all — no shares table, no published sessions
// (export is native, SPEC-015) — so every known session truthfully has "no
// active share" and the declared 404 NotFoundError IS the answer; the declared
// 200 Session would assert an unshare that never happened. A body is neither
// declared (requestBody: null) nor read.
func TestSessionUnshareTruthfulArms(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/session/smissing/share", nil)
	if err != nil {
		t.Fatalf("build DELETE /session/smissing/share: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /session/smissing/share: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("DELETE /session/smissing/share: got %d, want 404 (declared NotFoundError)", resp.StatusCode)
	}

	// Known session: no share concept exists, so "no active share" -> 404.
	status, _, raw := doShimRequest(t, srv.URL, http.MethodDelete, "/session/s1/share")
	if status != http.StatusNotFound {
		t.Fatalf("DELETE /session/s1/share: got %d, want 404 (nothing to unshare — the runtime keeps no share concept). Body: %s", status, raw)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("DELETE /session/s1/share body not JSON: %v (%s)", err, raw)
	}
	if env.Error.Code != "NOT_FOUND" {
		t.Errorf("DELETE /session/s1/share: error.code = %q, want NOT_FOUND", env.Error.Code)
	}
}

// ============================================================================
// ROUTE-FIX-016 — POST /session/{sessionID}/share (session.share)
// declared responses 200 Session, 400 Bad request, 404 NotFoundError,
// 500 InternalServerError.
// ============================================================================

// TestSessionShareTruthfulArms covers session.share. The declared 200 body is
// a Session carrying a share {url} — upstream publishes the session to a
// share URL. Consensus publishes nothing (export is native, SPEC-015), and a
// fabricated URL is forbidden (truthfulness convention), so the known-session
// arm answers the declared 400 Bad request naming the limitation — the
// sibling sessionCommand/handleGlobalUpgrade convention of answering inside
// the declared error vocabulary instead of a fabricated success or an
// undeclared code. The operation declares no requestBody (null), so a body is
// neither read nor required.
func TestSessionShareTruthfulArms(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	// Unknown session -> declared 404 first.
	status, body := p1Post(t, srv.URL, "/session/smissing/share", "")
	if status != http.StatusNotFound {
		t.Fatalf("POST /session/smissing/share: got %d, want 404 (declared NotFoundError). Body: %v", status, body)
	}

	// Known session -> declared 400 naming the limitation (nothing is
	// published; no share URL exists to return).
	status, body = p1Post(t, srv.URL, "/session/s1/share", "")
	if status != http.StatusBadRequest {
		t.Fatalf("POST /session/s1/share: got %d, want 400 (declared Bad request — no share can be created). Body: %v", status, body)
	}
	assertP1Error(t, "/session/s1/share", "", status, body, "INVALID_REQUEST")

	// The refusal must not carry a fabricated share url.
	if raw, ok := body["share"]; ok {
		t.Errorf("POST /session/s1/share: body carries share %v — a share must never be fabricated", raw)
	}
}

// ============================================================================
// ROUTE-FIX-017 — POST /session/{sessionID}/shell (session.shell)
// declared responses 200 {info, parts}, 400, 404, 409 SessionBusyError.
// ============================================================================

// TestSessionShellTruthfulArms covers session.shell. The declared requestBody
// requires agent and command; the declared 200 body is the created assistant
// message {info, parts}. The Consensus runtime keeps no shell-execution
// engine, so a valid request against an idle session answers the declared 400
// naming the limitation (sibling sessionCommand convention — never a
// fabricated message), and the same request against a session mid-turn
// (status 'thinking') answers the declared 409 SessionBusyError, the analog
// of upstream's shell-while-prompt-in-flight.
func TestSessionShellTruthfulArms(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	// Unknown session -> declared 404 before body outcomes.
	status, body := p1Post(t, srv.URL, "/session/smissing/shell", `{"agent":"build","command":"go test ./..."}`)
	if status != http.StatusNotFound {
		t.Fatalf("POST /session/smissing/shell: got %d, want 404 (declared NotFoundError). Body: %v", status, body)
	}

	// Malformed body -> declared 400.
	status, body = p1Post(t, srv.URL, "/session/s1/shell", `{"agent":`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /session/s1/shell (malformed): got %d, want 400. Body: %v", status, body)
	}
	assertP1Error(t, "/session/s1/shell (malformed)", "", status, body, "INVALID_REQUEST")

	// Required fields agent + command missing -> declared 400.
	status, body = p1Post(t, srv.URL, "/session/s1/shell", `{}`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /session/s1/shell (missing fields): got %d, want 400. Body: %v", status, body)
	}
	assertP1Error(t, "/session/s1/shell (missing fields)", "", status, body, "INVALID_REQUEST")

	// In-flight session -> declared 409 SessionBusyError.
	status, body = p1Post(t, srv.URL, "/session/s2/shell", `{"agent":"build","command":"go test ./..."}`)
	if status != http.StatusConflict {
		t.Fatalf("POST /session/s2/shell (busy): got %d, want 409 (SessionBusyError). Body: %v", status, body)
	}
	if got, _ := body["_tag"].(string); got != "SessionBusyError" {
		t.Errorf("POST /session/s2/shell (busy): _tag = %v, want SessionBusyError", got)
	}

	// Idle session, valid request, no shell engine -> declared 400 naming
	// the limitation (never a fabricated created message).
	status, body = p1Post(t, srv.URL, "/session/s1/shell", `{"agent":"build","command":"go test ./..."}`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /session/s1/shell (no engine): got %d, want 400 (declared Bad request). Body: %v", status, body)
	}
	assertP1Error(t, "/session/s1/shell (no engine)", "", status, body, "INVALID_REQUEST")
	if _, ok := body["info"]; ok {
		t.Errorf("POST /session/s1/shell (no engine): body carries info %v — a created message must never be fabricated", body["info"])
	}
}

// TestSessionShellMethodGuard pins the non-POST behaviour: every other method
// keeps the pre-existing stub-list 501 (405 is not in the declared set).
func TestSessionShellMethodGuard(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/session/s1/shell")
		if status != http.StatusNotImplemented {
			t.Errorf("%s /session/s1/shell: got %d, want 501 (stub-list residual). Body: %s",
				method, status, body)
		}
	}
}

// ============================================================================
// ROUTE-FIX-018 — POST /session/{sessionID}/summarize (session.summarize)
// declared responses 200 boolean, 400 BadRequest | InvalidRequestError,
// 404 NotFoundError.
// ============================================================================

// TestSessionSummarizeTruthfulArms covers session.summarize. The declared
// requestBody requires providerID and modelID; the declared 200 body is a
// plain boolean. The runtime keeps no summarize/compaction pipeline, so a
// valid request answers the declared 200 with the truthful false ("no
// summary produced") — the sibling handleSyncStart truthfulness convention —
// never a fabricated true.
func TestSessionSummarizeTruthfulArms(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	// Unknown session -> declared 404 first.
	status, body := p1Post(t, srv.URL, "/session/smissing/summarize", `{"providerID":"p","modelID":"m"}`)
	if status != http.StatusNotFound {
		t.Fatalf("POST /session/smissing/summarize: got %d, want 404 (declared NotFoundError). Body: %v", status, body)
	}

	// Malformed body -> declared 400.
	status, body = p1Post(t, srv.URL, "/session/s1/summarize", `{"providerID":`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /session/s1/summarize (malformed): got %d, want 400. Body: %v", status, body)
	}
	assertP1Error(t, "/session/s1/summarize (malformed)", "", status, body, "INVALID_REQUEST")

	// Required providerID/modelID missing -> declared 400.
	status, body = p1Post(t, srv.URL, "/session/s1/summarize", `{}`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /session/s1/summarize (missing fields): got %d, want 400. Body: %v", status, body)
	}
	assertP1Error(t, "/session/s1/summarize (missing fields)", "", status, body, "INVALID_REQUEST")

	// Known session, valid body, no summarize pipeline -> declared 200 with
	// the truthful boolean false.
	status, _, raw := doShimRequestBody(t, srv.URL, http.MethodPost, "/session/s1/summarize", `{"providerID":"p","modelID":"m"}`)
	if status != http.StatusOK {
		t.Fatalf("POST /session/s1/summarize (happy): got %d, want 200. Body: %s", status, raw)
	}
	var summarized bool
	if err := json.Unmarshal(raw, &summarized); err != nil {
		t.Fatalf("POST /session/s1/summarize: body not the declared JSON boolean: %v (%s)", err, raw)
	}
	if summarized {
		t.Errorf("POST /session/s1/summarize: body = true, want false (no summarize pipeline exists, no summary was produced)")
	}
}

// TestSessionSummarizeMethodGuard pins the non-POST behaviour: every other
// method keeps the pre-existing stub-list 501.
func TestSessionSummarizeMethodGuard(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, _, body := doShimRequest(t, srv.URL, method, "/session/s1/summarize")
		if status != http.StatusNotImplemented {
			t.Errorf("%s /session/s1/summarize: got %d, want 501 (stub-list residual). Body: %s",
				method, status, body)
		}
	}
}

// TestSessionShareMethodGuards pins the sub-path's undeclared methods: GET is
// NOT a declared opencode operation for /session/{sessionID}/share (the
// document declares only POST + DELETE), so it falls to the exclusion-list
// 501 like every other undeclared method — both implemented methods keep
// their own behaviour.
func TestSessionShareMethodGuards(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		status, _, body := doShimRequest(t, srv.URL, method, "/session/s1/share")
		if status != http.StatusNotImplemented {
			t.Errorf("%s /session/s1/share: got %d, want 501 (undeclared method, stub-list residual). Body: %s",
				method, status, body)
		}
	}
}

// TestP1SessionNeighboursUntouched is the non-vacuity control
// (TestSyncStartNeighboursUntouched pattern): serving the five routes must
// not disturb their neighbours — adjacent implemented routes keep answering
// per contract, the unknown sub-path still 404s, and the stub-list siblings
// that did not move (init, fork) keep their served behaviour. The message
// neighbour uses the mock harness (no native service -> its instant declared
// 503) so the control does not sit through the real turn timeout the store
// harness would pay; the DB-backed neighbours use the store harness.
func TestP1SessionNeighboursUntouched(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	// GET /session/{id} still works (translateSessionRow shape).
	status, _, raw := doShimRequest(t, srv.URL, http.MethodGet, "/session/s1")
	if status != http.StatusOK {
		t.Fatalf("GET /session/s1: got %d, want 200. Body: %s", status, raw)
	}
	var gotSession map[string]any
	if err := json.Unmarshal(raw, &gotSession); err != nil {
		t.Fatalf("GET /session/s1 body not JSON: %v (%s)", err, raw)
	}
	if gotSession["id"] != "s1" {
		t.Errorf("GET /session/s1: id = %v, want s1", gotSession["id"])
	}

	// GET /session/{id}/message/{id} still resolves the seeded message.
	status, _, raw = doShimRequest(t, srv.URL, http.MethodGet, "/session/s1/message/msg-42")
	if status != http.StatusOK {
		t.Errorf("GET /session/s1/message/msg-42: got %d, want 200. Body: %s", status, raw)
	}

	// GET /session/status still answers the map shape.
	status, _, raw = doShimRequest(t, srv.URL, http.MethodGet, "/session/status")
	if status != http.StatusOK {
		t.Errorf("GET /session/status: got %d, want 200. Body: %s", status, raw)
	}

	// Served siblings init and fork keep their live behaviour (missing
	// required fields -> 400, the init contract).
	status, _, raw = doShimRequestBody(t, srv.URL, http.MethodPost, "/session/s1/init", `{}`)
	if status != http.StatusBadRequest {
		t.Errorf("POST /session/s1/init: got %d, want 400 (served sibling unchanged). Body: %s", status, raw)
	}

	// POST /session/{id}/message keeps its own contract: 503 when no native
	// response service answers the turn (unchanged by this change; the mock
	// harness has none).
	_, mockSrv := newTestServer(&mockDB{})
	defer mockSrv.Close()
	status, _, raw = doShimRequestBody(t, mockSrv.URL, http.MethodPost, "/session/s1/message",
		`{"parts":[{"type":"text","text":"hi"}]}`)
	if status != http.StatusServiceUnavailable {
		t.Errorf("POST /session/s1/message: got %d, want 503 (contract unchanged). Body: %s", status, raw)
	}

	// Unknown sub-path still 404s (no blanket answer appeared).
	status, _, raw = doShimRequest(t, srv.URL, http.MethodGet, "/session/s1/unknown-sub")
	if status != http.StatusNotFound {
		t.Errorf("GET /session/s1/unknown-sub: got %d, want 404 (non-vacuity). Body: %s", status, raw)
	}
}

// ============================================================================
// ROUTE-FIX-035 — DELETE /session/{sessionID}/message/{messageID}
// (session.deleteMessage; declared responses 200 boolean "Successfully deleted
// message", 400 BadRequest | InvalidRequestError, 404 NotFoundError, 409
// SessionBusyError).
// ============================================================================

// TestSessionDeleteMessageServesDeclaredSuccess is the defect cell for
// ROUTE-FIX-035 / SHIM-NARROWED-005: before this change no case in
// handleSessionByID matched "message/<id>" + DELETE, so the request fell to
// the router's default arm and answered an untyped 404 for EVERY request — the
// declared success code was unreachable from outside the code.
//
// A known session holding the message must now answer the declared 200 with
// the declared boolean body. The value is the truthful false, not true: a
// message IS a memory_events row (opencode "msg-<id>" ids resolve to that
// ledger), and that ledger is append-only by construction — SPEC-002 §2.1,
// enforced by the triggers in migrations 017 (SQLite) and 018 (Postgres) which
// ABORT every UPDATE and DELETE. The shim keeps no delete engine, no message
// tombstone and no separate parts store (parts are synthesized from
// memory_events.content), and no genuine delete analog exists to perform, so
// no message was deleted and the boolean must not claim otherwise (the
// sessionSummarize precedent from d8f1d59 — the sibling operation whose
// declared 200 body is likewise a plain boolean: when the engine is absent the
// declared boolean reports the truth). The final assertion pins the no-op: the
// message is still readable through the sibling GET route, proving the 200 did
// not imply a removal that never happened.
func TestSessionDeleteMessageServesDeclaredSuccess(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	status, _, raw := doShimRequest(t, srv.URL, http.MethodDelete, "/session/s1/message/msg-42")
	if status != http.StatusOK {
		t.Fatalf("DELETE /session/s1/message/msg-42: got %d, want 200 (declared boolean success). Body: %s", status, raw)
	}
	var deleted bool
	if err := json.Unmarshal(raw, &deleted); err != nil {
		t.Fatalf("DELETE /session/s1/message/msg-42: body is not the declared JSON boolean: %v (%s)", err, raw)
	}
	if deleted {
		t.Errorf("DELETE /session/s1/message/msg-42: body = true, want false — memory_events is append-only (SPEC-002 §2.1) and no delete engine exists, so no message was deleted")
	}

	// Truthful no-op: nothing was deleted, so the message is still readable.
	status, _, raw = doShimRequest(t, srv.URL, http.MethodGet, "/session/s1/message/msg-42")
	if status != http.StatusOK {
		t.Errorf("GET /session/s1/message/msg-42 after DELETE: got %d, want 200 (message must remain — no delete engine). Body: %s", status, raw)
	}
}

// TestSessionDeleteMessageTruthfulArms covers the declared error vocabulary of
// session.deleteMessage: unknown session 404 NotFoundError; a session mid-turn
// (status 'thinking') 409 SessionBusyError — the genuine analog of upstream's
// "prompt in flight", checked before message resolution exactly as
// sessionRevert/sessionShell do (msg-42 belongs to s1, so only the busy check
// can answer the s2 probe); an absent/blank or ill-shaped messageID 400 (the
// declared pattern is ^msg, the sessionRevert validation precedent — and it
// makes the declared 400 arm reachable, since the operation declares no
// requestBody); and a well-formed messageID the session does not hold 404.
func TestSessionDeleteMessageTruthfulArms(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	// Unknown session -> declared 404 NotFoundError.
	status, _, raw := doShimRequest(t, srv.URL, http.MethodDelete, "/session/smissing/message/msg-42")
	if status != http.StatusNotFound {
		t.Fatalf("DELETE /session/smissing/message/msg-42: got %d, want 404 (declared NotFoundError). Body: %s", status, raw)
	}
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("DELETE /session/smissing/message/msg-42 body not JSON: %v (%s)", err, raw)
	}
	assertP1Error(t, "DELETE /session/smissing/message/msg-42", "", status, env, "NOT_FOUND")

	// Mid-turn session -> declared 409 SessionBusyError.
	status, _, raw = doShimRequest(t, srv.URL, http.MethodDelete, "/session/s2/message/msg-42")
	if status != http.StatusConflict {
		t.Fatalf("DELETE /session/s2/message/msg-42: got %d, want 409 (declared SessionBusyError). Body: %s", status, raw)
	}
	var busy map[string]any
	if err := json.Unmarshal(raw, &busy); err != nil {
		t.Fatalf("DELETE /session/s2/message/msg-42 body not JSON: %v (%s)", err, raw)
	}
	if got, _ := busy["_tag"].(string); got != "SessionBusyError" {
		t.Errorf("DELETE /session/s2/message/msg-42: _tag = %v, want SessionBusyError (declared 409 body)", got)
	}
	if got, _ := busy["sessionID"].(string); got != "s2" {
		t.Errorf("DELETE /session/s2/message/msg-42: sessionID = %v, want s2", got)
	}

	// Blank messageID (the path ends at "message/") -> declared 400.
	status, _, raw = doShimRequest(t, srv.URL, http.MethodDelete, "/session/s1/message/")
	if status != http.StatusBadRequest {
		t.Fatalf("DELETE /session/s1/message/: got %d, want 400 (declared BadRequest — messageID is required). Body: %s", status, raw)
	}
	var blank map[string]any
	if err := json.Unmarshal(raw, &blank); err == nil {
		assertP1Error(t, "DELETE /session/s1/message/", "", status, blank, "INVALID_REQUEST")
	}

	// Ill-shaped messageID (not the declared pattern ^msg) -> declared 400.
	status, _, raw = doShimRequest(t, srv.URL, http.MethodDelete, "/session/s1/message/not-a-msg")
	if status != http.StatusBadRequest {
		t.Fatalf("DELETE /session/s1/message/not-a-msg: got %d, want 400 (declared pattern ^msg). Body: %s", status, raw)
	}

	// Well-formed messageID the session does not hold -> declared 404.
	status, _, raw = doShimRequest(t, srv.URL, http.MethodDelete, "/session/s1/message/msg-999")
	if status != http.StatusNotFound {
		t.Fatalf("DELETE /session/s1/message/msg-999: got %d, want 404 (declared NotFoundError). Body: %s", status, raw)
	}
}

// TestSessionDeleteMessageMethodGuards pins the path's neighbours: GET
// /session/{id}/message/{messageID} keeps its own contract (the message is
// still resolvable), while the methods the document does NOT declare for this
// path (POST/PUT) keep the pre-existing router default 404 — this sub-path is
// not in the stub-list switch, so they never answered 501 and this change must
// not move them. DELETE is the newly served method and is asserted in
// TestSessionDeleteMessageServesDeclaredSuccess.
func TestSessionDeleteMessageMethodGuards(t *testing.T) {
	_, srv, _ := p1SessionRouteTestServer(t)

	status, _, raw := doShimRequest(t, srv.URL, http.MethodGet, "/session/s1/message/msg-42")
	if status != http.StatusOK {
		t.Errorf("GET /session/s1/message/msg-42: got %d, want 200 (served neighbour unchanged). Body: %s", status, raw)
	}

	for _, method := range []string{http.MethodPost, http.MethodPut} {
		status, _, raw = doShimRequest(t, srv.URL, method, "/session/s1/message/msg-42")
		if status != http.StatusNotFound {
			t.Errorf("%s /session/s1/message/msg-42: got %d, want 404 (undeclared method, pre-existing router default). Body: %s",
				method, status, raw)
		}
	}
}
