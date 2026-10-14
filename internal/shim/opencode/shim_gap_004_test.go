// SHIM-GAP-004 — the narrowed / error-contract-only operations.
//
// ch:trace row=SHIM-GAP-004 spec=specs/018-openapi-contract.md#12
//
// Three behaviours are pinned here:
//
//  1. the two operations SHIM-GAP-004 WIDENED to their full declared contract —
//     permission.reply (POST /permission/{requestID}/reply) and tui.selectSession
//     (POST /tui/select-session) — serve every declared response code, where
//     before this change the 404 was the only reachable answer;
//  2. the 400 arm question.reply declares is now served (its declared requestBody
//     is validated), while its declared 200 stays unreachable and RECORDED;
//  3. the comparison artifact accounts for every error-contract narrowing: each
//     narrowed row carries a rationale and counts.narrowed_unexplained is 0, which
//     is what makes "the narrowing is invisible from outside" impossible.
package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// shimGap004Server builds a scratch SQLite database carrying the REAL
// approval_requests column set (migrations/008_hitl_tables.sql) plus a sessions
// row, seeds one pending approval ("per_gap004") and one idle session ("ses_gap004"),
// and returns the test server plus the raw connection.
func shimGap004Server(t *testing.T) (*httptest.Server, db.DB) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shim-gap-004.db")
	conn, err := driver.Open(ctx, db.Config{URL: "sqlite://" + path, MaxOpenConns: 4})
	if err != nil {
		t.Fatalf("open shim-gap-004 test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, stmt := range []string{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			iteration INTEGER NOT NULL DEFAULT 0,
			heartbeat_at TEXT,
			deleted_at TEXT
		)`,
		`CREATE TABLE approval_requests (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			iteration INTEGER NOT NULL DEFAULT 0,
			request_type TEXT NOT NULL,
			description TEXT NOT NULL,
			risk_level TEXT NOT NULL DEFAULT 'medium',
			context TEXT NOT NULL DEFAULT '{}',
			target_tool TEXT,
			target_sql TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			reviewer_id TEXT,
			review_notes TEXT,
			modified_sql TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			reviewed_at TEXT,
			expires_at TEXT
		)`,
		`INSERT INTO sessions (id, status, iteration) VALUES ('ses_gap004', 'idle', 0)`,
		`INSERT INTO approval_requests (id, session_id, request_type, description, risk_level, status)
		 VALUES ('per_gap004', 'ses_gap004', 'destructive_action', 'shim-gap-004 pending approval', 'high', 'pending')`,
		`INSERT INTO approval_requests (id, session_id, request_type, description, risk_level, status)
		 VALUES ('per_gap004_done', 'ses_gap004', 'destructive_action', 'already reviewed', 'low', 'approved')`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare shim-gap-004 test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, conn
}

func shimGap004Post(t *testing.T, base, path, body string) (int, any) {
	t.Helper()
	status, _, raw := doShimRequestBody(t, base, http.MethodPost, path, body)
	var decoded any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("POST %s: body is not JSON: %v (%s)", path, err, raw)
		}
	}
	return status, decoded
}

func asObject(t *testing.T, path string, v any) map[string]any {
	t.Helper()
	obj, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: body is %T, want a JSON object (%v)", path, v, v)
	}
	return obj
}

// ============================================================================
// permission.reply — WIDENED to the declared 200/400/404 contract
// ============================================================================

func TestPermissionReplyServesDeclaredSuccess(t *testing.T) {
	srv, conn := shimGap004Server(t)
	path := "/permission/per_gap004/reply"

	status, body := shimGap004Post(t, srv.URL, path, `{"reply":"once"}`)
	if status != http.StatusOK {
		t.Fatalf("%s: got %d, want the declared 200. Body: %v", path, status, body)
	}
	if body != true {
		t.Fatalf("%s: declared 200 body = %v, want the boolean true", path, body)
	}

	// The row must actually be resolved — the declared boolean never reports a
	// no-op success.
	row, err := conn.QueryRow(context.Background(),
		`SELECT status, review_notes, reviewer_id, reviewed_at FROM approval_requests WHERE id = 'per_gap004'`)
	if err != nil || row == nil {
		t.Fatalf("read resolved approval row: err=%v row=%v", err, row)
	}
	if got := toString(row["status"]); got != "approved" {
		t.Errorf("status = %q, want %q after reply=once", got, "approved")
	}
	if got := toString(row["review_notes"]); got != "granted once" {
		t.Errorf("review_notes = %q, want %q (the sessionPermissionRespond mapping)", got, "granted once")
	}
	if got := toString(row["reviewer_id"]); got != "opencode-shim" {
		t.Errorf("reviewer_id = %q, want opencode-shim", got)
	}
	if toString(row["reviewed_at"]) == "" {
		t.Error("reviewed_at is empty; the review timestamp was not written")
	}

	// The declared 404 for a row that is no longer pending (only 200/400/404 are
	// declared, so a resolved row can never be a fabricated second success).
	status, body = shimGap004Post(t, srv.URL, path, `{"reply":"once"}`)
	if status != http.StatusNotFound {
		t.Fatalf("%s (resolved row): got %d, want the declared 404. Body: %v", path, status, body)
	}
	if got := asObject(t, path, body)["_tag"]; got != "PermissionNotFoundError" {
		t.Errorf("%s (resolved row): _tag = %v, want PermissionNotFoundError", path, got)
	}
}

func TestPermissionReplyRejectMapsToRejected(t *testing.T) {
	srv, conn := shimGap004Server(t)
	status, body := shimGap004Post(t, srv.URL, "/permission/per_gap004/reply",
		`{"reply":"reject","message":"not this time"}`)
	if status != http.StatusOK || body != true {
		t.Fatalf("permission.reply reject: got %d %v, want the declared 200 true", status, body)
	}
	row, err := conn.QueryRow(context.Background(),
		`SELECT status, review_notes FROM approval_requests WHERE id = 'per_gap004'`)
	if err != nil || row == nil {
		t.Fatalf("read rejected approval row: err=%v row=%v", err, row)
	}
	if got := toString(row["status"]); got != "rejected" {
		t.Errorf("status = %q, want %q after reply=reject", got, "rejected")
	}
	if got := toString(row["review_notes"]); !strings.Contains(got, "denied by permission.reply") ||
		!strings.Contains(got, "not this time") {
		t.Errorf("review_notes = %q, want the rejection marker plus the declared message", got)
	}
}

func TestPermissionReplyDeclaredErrorArms(t *testing.T) {
	srv, _ := shimGap004Server(t)
	path := "/permission/per_missing/reply"

	// Unknown id -> the declared 404 in the declared PermissionNotFoundError
	// shape ({_tag, requestID, message}, additionalProperties false).
	status, body := shimGap004Post(t, srv.URL, path, `{"reply":"always"}`)
	if status != http.StatusNotFound {
		t.Fatalf("%s: got %d, want the declared 404. Body: %v", path, status, body)
	}
	obj := asObject(t, path, body)
	if obj["_tag"] != "PermissionNotFoundError" {
		t.Errorf("%s: _tag = %v, want PermissionNotFoundError", path, obj["_tag"])
	}
	if obj["requestID"] != "per_missing" {
		t.Errorf("%s: requestID = %v, want per_missing", path, obj["requestID"])
	}
	if _, ok := obj["message"].(string); !ok {
		t.Errorf("%s: message = %v, want a string (required by the declared schema)", path, obj["message"])
	}

	for _, tc := range []struct {
		name string
		body string
	}{
		{"malformed JSON", `{"reply":`},
		{"absent body", ""},
		{"non-object body", `["once"]`},
		{"missing reply", `{"message":"hi"}`},
		{"empty reply", `{"reply":""}`},
		{"bad enum", `{"reply":"sometimes"}`},
		{"undeclared field", `{"reply":"once","reason":"extra"}`},
		{"trailing second value", `{"reply":"once"}{"reply":"once"}`},
	} {
		status, body := shimGap004Post(t, srv.URL, path, tc.body)
		if status != http.StatusBadRequest {
			t.Errorf("permission.reply %s: got %d, want the declared 400. Body: %v", tc.name, status, body)
			continue
		}
	}

	// An ill-shaped requestID (declared pattern ^per) is a declared 400 too.
	status, body = shimGap004Post(t, srv.URL, "/permission/nope/reply", `{"reply":"once"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("permission.reply ill-shaped id: got %d, want the declared 400. Body: %v", status, body)
	}
	if got := errorCode(asObject(t, "/permission/nope/reply", body)); got != "INVALID_REQUEST" {
		t.Errorf("permission.reply ill-shaped id: error.code = %q, want INVALID_REQUEST", got)
	}
}

// ============================================================================
// tui.selectSession — WIDENED to the declared 200/400/404 contract
// ============================================================================

func TestTUISelectSessionServesDeclaredContract(t *testing.T) {
	srv, _ := shimGap004Server(t)
	path := "/tui/select-session"

	// A session that exists answers the declared boolean false: no TUI process
	// is attached, so nothing was navigated — never a claim that a TUI moved.
	status, body := shimGap004Post(t, srv.URL, path, `{"sessionID":"ses_gap004"}`)
	if status != http.StatusOK {
		t.Fatalf("%s: got %d, want the declared 200. Body: %v", path, status, body)
	}
	if body != false {
		t.Fatalf("%s: declared 200 body = %v, want the boolean false (no TUI attached)", path, body)
	}

	// Unknown session -> the declared 404 NotFoundError NamedError shape.
	status, body = shimGap004Post(t, srv.URL, path, `{"sessionID":"ses_does_not_exist"}`)
	if status != http.StatusNotFound {
		t.Fatalf("%s (unknown session): got %d, want the declared 404. Body: %v", path, status, body)
	}
	obj := asObject(t, path, body)
	if obj["name"] != "NotFoundError" {
		t.Errorf("%s (unknown session): name = %v, want NotFoundError", path, obj["name"])
	}
	if data, ok := obj["data"].(map[string]any); !ok || strings.TrimSpace(toString(data["message"])) == "" {
		t.Errorf("%s (unknown session): data.message missing from the NamedError body (%v)", path, obj)
	}

	// The declared 400 arm: the requestBody is required and carries only sessionID.
	for _, tc := range []struct {
		name string
		body string
	}{
		{"malformed JSON", `{"sessionID":`},
		{"absent body", ""},
		{"non-object body", `"ses_gap004"`},
		{"missing sessionID", `{}`},
		{"empty sessionID", `{"sessionID":"   "}`},
		{"undeclared field", `{"sessionID":"ses_gap004","force":true}`},
	} {
		status, body := shimGap004Post(t, srv.URL, path, tc.body)
		if status != http.StatusBadRequest {
			t.Errorf("tui.selectSession %s: got %d, want the declared 400. Body: %v", tc.name, status, body)
		}
	}

	// The operation is POST-only; 405 is not in its declared set but the sibling
	// TUI handlers answer METHOD_NOT_ALLOWED rather than reading the store.
	if status, _, _ := doShimRequest(t, srv.URL, http.MethodGet, path); status != http.StatusMethodNotAllowed {
		t.Errorf("GET %s: got %d, want 405 METHOD_NOT_ALLOWED", path, status)
	}
}

// ============================================================================
// question.reply / question.reject — the declared 400 arm is served; the
// declared 200 stays unreachable and is RECORDED (specs/018 §12)
// ============================================================================

func TestQuestionFamilyServesDeclaredErrorArms(t *testing.T) {
	srv, _ := shimGap004Server(t)

	// question.reply with a schema-valid body reaches the resource lookup and
	// answers the declared 404 QuestionNotFoundError (no question producer
	// exists — the recorded narrowing).
	status, body := shimGap004Post(t, srv.URL, "/question/que_1/reply", `{"answers":[["yes"]]}`)
	if status != http.StatusNotFound {
		t.Fatalf("question.reply: got %d, want the declared 404. Body: %v", status, body)
	}
	if got := asObject(t, "/question/que_1/reply", body)["_tag"]; got != "QuestionNotFoundError" {
		t.Errorf("question.reply: _tag = %v, want QuestionNotFoundError", got)
	}

	// The declared requestBody is now validated: malformed, absent or
	// schema-violating bodies answer the declared 400 instead of falling
	// through to the 404 (the DF-CONSENSUS-40 class of defect).
	for _, tc := range []struct {
		name string
		body string
	}{
		{"malformed JSON", `{"answers":`},
		{"absent body", ""},
		{"non-object body", `[["yes"]]`},
		{"missing answers", `{}`},
		{"answers not an array", `{"answers":"yes"}`},
		{"answers not arrays of strings", `{"answers":["yes"]}`},
		{"answers carries a non-string", `{"answers":[["yes",7]]}`},
		{"undeclared field", `{"answers":[["yes"]],"force":true}`},
	} {
		status, body := shimGap004Post(t, srv.URL, "/question/que_1/reply", tc.body)
		if status != http.StatusBadRequest {
			t.Errorf("question.reply %s: got %d, want the declared 400. Body: %v", tc.name, status, body)
		}
	}

	// An empty answers array is schema-valid (no minItems declared), so it must
	// reach the lookup and answer the declared 404 — not a 400.
	if status, body := shimGap004Post(t, srv.URL, "/question/que_1/reply", `{"answers":[]}`); status != http.StatusNotFound {
		t.Errorf("question.reply empty answers: got %d, want the declared 404. Body: %v", status, body)
	}

	// question.reject declares NO requestBody: a request with no body at all is
	// well-formed and answers the declared 404.
	status, body = shimGap004Post(t, srv.URL, "/question/que_2/reject", "")
	if status != http.StatusNotFound {
		t.Fatalf("question.reject: got %d, want the declared 404. Body: %v", status, body)
	}
	if got := asObject(t, "/question/que_2/reject", body)["_tag"]; got != "QuestionNotFoundError" {
		t.Errorf("question.reject: _tag = %v, want QuestionNotFoundError", got)
	}

	// Both operations answer the declared 400 for an ill-shaped requestID
	// (declared pattern ^que).
	for _, path := range []string{"/question/nope/reply", "/question/nope/reject"} {
		body := `{"answers":[["yes"]]}`
		if path == "/question/nope/reject" {
			body = ""
		}
		status, got := shimGap004Post(t, srv.URL, path, body)
		if status != http.StatusBadRequest {
			t.Errorf("%s: got %d, want the declared 400. Body: %v", path, status, got)
		}
	}
}

// ============================================================================
// The comparison artifact must account for EVERY error-contract narrowing:
// a narrowed row without a rationale is the invisibility SHIM-GAP-004 removes.
// ============================================================================

func TestDeclaredVsServedAccountsForEveryNarrowing(t *testing.T) {
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
		Detail        string `json:"detail"`
	}
	var doc struct {
		Counts struct {
			NarrowedErrorContractOnly int `json:"narrowed_error_contract_only"`
			NarrowedUnexplained       int `json:"narrowed_unexplained"`
		} `json:"counts"`
		Covered     []row `json:"covered"`
		Narrowed    []row `json:"narrowed_error_contract_only"`
		Intentional []row `json:"intentional_narrowing"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", artifact, err)
	}
	if len(doc.Covered) == 0 || len(doc.Narrowed) == 0 {
		t.Fatalf("artifact looks empty (covered=%d narrowed=%d) — refusing to pass vacuously",
			len(doc.Covered), len(doc.Narrowed))
	}

	// 1. No narrowing is unexplained: every narrowed row records WHY.
	for _, r := range doc.Narrowed {
		if strings.TrimSpace(r.Rationale) == "" {
			t.Errorf("artifact records %s %s (%s) as narrowed with no rationale — an unexplained narrowing",
				r.Method, r.Path, r.OperationID)
		}
	}

	// 2. The artifact's own count of unexplained narrowings is zero, and it
	//    agrees with the rows it carries (a declared count that disagrees with
	//    the enumerated rows is the failure class this test exists for).
	if doc.Counts.NarrowedUnexplained != 0 {
		t.Errorf("counts.narrowed_unexplained = %d, want 0", doc.Counts.NarrowedUnexplained)
	}
	if doc.Counts.NarrowedErrorContractOnly != len(doc.Narrowed) {
		t.Errorf("counts.narrowed_error_contract_only = %d but the artifact enumerates %d narrowed rows",
			doc.Counts.NarrowedErrorContractOnly, len(doc.Narrowed))
	}

	// 3. The 11 findings SHIM-GAP-004 was dispatched for must each appear with a
	//    recorded disposition: full declared coverage, or a recorded narrowing.
	//    (Four were already widened by the ROUTE-FIX wave this row inherits.)
	findings := []struct{ operation, path, method string }{
		{"permission.reply", "/permission/{requestID}/reply", "POST"},
		{"project.update", "/project/{projectID}", "PATCH"},
		{"question.reject", "/question/{requestID}/reject", "POST"},
		{"question.reply", "/question/{requestID}/reply", "POST"},
		{"session.deleteMessage", "/session/{sessionID}/message/{messageID}", "DELETE"},
		{"part.delete", "/session/{sessionID}/message/{messageID}/part/{partID}", "DELETE"},
		{"part.update", "/session/{sessionID}/message/{messageID}/part/{partID}", "PATCH"},
		{"permission.respond", "/session/{sessionID}/permissions/{permissionID}", "POST"},
		{"session.todo", "/session/{sessionID}/todo", "GET"},
		{"session.unrevert", "/session/{sessionID}/unrevert", "POST"},
		{"tui.selectSession", "/tui/select-session", "POST"},
	}
	covered := map[string]bool{}
	for _, r := range doc.Covered {
		covered[r.OperationID+" "+r.Path+" "+r.Method] = true
	}
	narrowed := map[string]row{}
	for _, r := range doc.Narrowed {
		narrowed[r.OperationID+" "+r.Path+" "+r.Method] = r
	}
	for _, f := range findings {
		key := f.operation + " " + f.path + " " + f.method
		if covered[key] {
			continue
		}
		r, ok := narrowed[key]
		if !ok {
			t.Errorf("SHIM-GAP-004 finding %s (%s %s) is neither served as declared nor recorded as a narrowing",
				f.operation, f.method, f.path)
			continue
		}
		if strings.TrimSpace(r.Rationale) == "" {
			t.Errorf("SHIM-GAP-004 finding %s is recorded as narrowed with an empty rationale", f.operation)
		}
	}
}
