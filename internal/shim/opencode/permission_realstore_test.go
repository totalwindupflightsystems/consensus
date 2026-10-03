// Real-store round-trip tests for the permission routes (SPEC-017 §3.7).
//
// ch:trace row=ROUTE-FIX-044 row=ROUTE-FIX-045
//   spec=migrations/008_hitl_tables.sql
//   wave=consensus-foreman-2026-10-03-00-46-49.json#task-2
//   test=internal/shim/opencode/permission_realstore_test.go
//
// The mock-based tests in server_test.go prove the handler logic; these tests
// prove the SQL itself against a real SQLite database carrying the REAL
// approval_requests columns from migrations/008_hitl_tables.sql (target_sql,
// review_notes, reviewed_at, reviewer_id). T1-D4 and T1-D5 were schema
// mismatches that only a real store reproduces.

package opencode

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// newPermissionRealStoreServer opens a scratch SQLite database with the real
// approval_requests column set, seeds one pending row ('t1-perm-0002'), and
// returns the test HTTP server plus the raw connection for assertions.
func newPermissionRealStoreServer(t *testing.T) (*httptest.Server, db.DB) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "permission.db")
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + path,
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open permission test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, stmt := range []string{
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
		`INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, target_sql, status)
		 VALUES ('t1-perm-0002', '68ed0699-367d-4206-9268-25ebab753dcb', 2, 'destructive_action',
		         't1 sweep pending approval', 'high', 'DROP TABLE temp_cache', 'pending')`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare permission test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, conn
}

func postResolve(t *testing.T, base, id, body string) (int, []byte) {
	t.Helper()
	resp, err := http.Post(base+"/permission/"+id+"/resolve", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /permission/%s/resolve: %v", id, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read resolve body: %v", err)
	}
	return resp.StatusCode, raw
}

// TestGetPermissionRealStore is the T1-D4 reprobe: the seeded row EXISTS, so
// GET must answer the documented 200 with the documented JSON keys.
func TestGetPermissionRealStore(t *testing.T) {
	// ch:trace row=ROUTE-FIX-044 spec=migrations/008_hitl_tables.sql wave=consensus-foreman-2026-10-03-00-46-49.json#task-2
	srv, _ := newPermissionRealStoreServer(t)

	status, _, body := doShimRequest(t, srv.URL, http.MethodGet, "/permission/t1-perm-0002")
	if status != http.StatusOK {
		t.Fatalf("GET /permission/t1-perm-0002: got %d, want 200 (T1-D4 reprobe). Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("200 body is not JSON: %v (%s)", err, body)
	}
	if got["id"] != "t1-perm-0002" {
		t.Errorf("id = %v, want t1-perm-0002", got["id"])
	}
	if got["status"] != "pending" {
		t.Errorf("status = %v, want pending", got["status"])
	}
	if got["sql_preview"] != "DROP TABLE temp_cache" {
		t.Errorf("sql_preview = %v, want the target_sql value", got["sql_preview"])
	}
	if got["decision_reason"] != "" {
		t.Errorf("decision_reason = %v, want empty for an unresolved row", got["decision_reason"])
	}
	if got["resolved_at"] != nil {
		t.Errorf("resolved_at = %v, want null for an unresolved row", got["resolved_at"])
	}

	// Unknown id keeps the 404 arm.
	status, _, body = doShimRequest(t, srv.URL, http.MethodGet, "/permission/t1-perm-9999")
	if status != http.StatusNotFound {
		t.Fatalf("GET /permission/t1-perm-9999: got %d, want 404. Body: %s", status, body)
	}
}

// TestResolvePermissionRealStore is the T1-D5 reprobe: resolving the pending
// row must answer the documented 200, persist into the REAL columns, and a
// second resolve must hit the pending guard.
func TestResolvePermissionRealStore(t *testing.T) {
	// ch:trace row=ROUTE-FIX-045 spec=migrations/008_hitl_tables.sql wave=consensus-foreman-2026-10-03-00-46-49.json#task-2
	srv, conn := newPermissionRealStoreServer(t)

	// Happy path: 200 and the resolution lands in review_notes/reviewed_at.
	status, body := postResolve(t, srv.URL, "t1-perm-0002", `{"decision":"approved","reason":"safe to proceed"}`)
	if status != http.StatusOK {
		t.Fatalf("POST resolve pending row: got %d, want 200 (T1-D5 reprobe). Body: %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("resolve body is not JSON: %v (%s)", err, body)
	}
	if got["status"] != "approved" || got["resolved"] != true {
		t.Errorf("resolve body = %v, want status=approved resolved=true", got)
	}

	row, err := conn.QueryRow(context.Background(),
		`SELECT status, reviewer_id, review_notes, reviewed_at FROM approval_requests WHERE id = 't1-perm-0002'`)
	if err != nil {
		t.Fatalf("read back resolved row: %v", err)
	}
	if toString(row["status"]) != "approved" {
		t.Errorf("stored status = %v, want approved", row["status"])
	}
	if toString(row["reviewer_id"]) != "opencode-shim" {
		t.Errorf("stored reviewer_id = %v, want opencode-shim", row["reviewer_id"])
	}
	if toString(row["review_notes"]) != "safe to proceed" {
		t.Errorf("stored review_notes = %v, want the reason", row["review_notes"])
	}
	if row["reviewed_at"] == nil || toString(row["reviewed_at"]) == "" {
		t.Errorf("stored reviewed_at must be set, got %v", row["reviewed_at"])
	}

	// Pending guard: a second resolve answers the declared 404 arm.
	status, body = postResolve(t, srv.URL, "t1-perm-0002", `{"decision":"rejected","reason":"too late"}`)
	if status != http.StatusNotFound {
		t.Fatalf("POST resolve already-resolved row: got %d, want 404. Body: %s", status, body)
	}

	// Unknown id: the documented 404 arm, no UPDATE attempted.
	status, body = postResolve(t, srv.URL, "t1-perm-9999", `{"decision":"approved"}`)
	if status != http.StatusNotFound {
		t.Fatalf("POST resolve unknown id: got %d, want 404. Body: %s", status, body)
	}

	// Malformed body and invalid decision keep the 400 arm.
	status, body = postResolve(t, srv.URL, "t1-perm-9999", `{not json`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST resolve malformed body: got %d, want 400. Body: %s", status, body)
	}
	status, body = postResolve(t, srv.URL, "t1-perm-9999", `{"decision":"maybe_later"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST resolve invalid decision: got %d, want 400. Body: %s", status, body)
	}
}
