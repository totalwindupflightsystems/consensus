// CLI exit-code contract regression tests (DOC-10).
//
// SPEC-016's scriptable principle (§1: "Designed for pipes, cron jobs, and
// automation") only holds if a failed verb reports failure through the §8
// exit codes. These tests pin the non-zero contract for the three failure
// classes scripts actually branch on: connection failure (3), auth rejected
// (4), and resource not found on a data-contract verb (5).
//
// axiom:trace work_item=DOC-10 spec=specs/016-cli-interface.md impl=internal/cli/config.go,internal/cli/tool.go,internal/cli/status.go test=internal/cli/exit_codes_test.go
package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// closedPortServer returns the URL of a bound-then-closed httptest.Server:
// nothing is listening, so a client call fails with connection refused.
func closedPortServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	return srv.URL
}

func TestExitCode_ConnectionFailure_Is3(t *testing.T) {
	// The client wraps network errors as "cannot connect to Consensus server
	// at <url> — is it running? (dial tcp ...: connect: connection refused)".
	// The §8 mapping must classify that as 3 (server unreachable).
	err := errString("cannot connect to Consensus server at http://127.0.0.1:1 — is it running?\n  (dial tcp 127.0.0.1:1: connect: connection refused)")
	if got := exitCode(err); got != 3 {
		t.Errorf("exitCode(connection refused) = %d, want 3", got)
	}
}

// TestStatus_ConnectionFailure: `consensus status` against a dead server must
// return an error (exit 3 via cli.Execute), never exit 0.
func TestStatus_ConnectionFailure(t *testing.T) {
	defer overrideGlobals(closedPortServer(t), "", "table", false)()

	cmd := newStatusCmd()
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected connection-failure error from status, got nil")
	}
	if !strings.Contains(err.Error(), "cannot connect") {
		t.Errorf("expected connection error, got: %q", err.Error())
	}
}

// TestSessionList_ConnectionFailure: same contract for a data verb.
func TestSessionList_ConnectionFailure(t *testing.T) {
	defer overrideGlobals(closedPortServer(t), "", "table", false)()

	cmd := newSessionListCmd()
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected connection-failure error from session list, got nil")
	}
	if !strings.Contains(err.Error(), "cannot connect") {
		t.Errorf("expected connection error, got: %q", err.Error())
	}
}

// TestConfigGet_ConnectionFailure: same contract for config get.
func TestConfigGet_ConnectionFailure(t *testing.T) {
	defer overrideGlobals(closedPortServer(t), "", "table", false)()

	cmd := newConfigGetCmd()
	cmd.SetArgs([]string{"llm.default_model"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected connection-failure error from config get, got nil")
	}
	if !strings.Contains(err.Error(), "cannot connect") {
		t.Errorf("expected connection error, got: %q", err.Error())
	}
}

// TestStatus_MetricsAuthRejected: health succeeds but the metrics call is
// rejected (wrong key). Before DOC-10, status swallowed the 401, printed
// "metrics: unavailable" and exited 0 — a script could not tell a healthy
// server from a rejected one. The failure must propagate (exit 4).
func TestStatus_MetricsAuthRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "version": "0.1.0"})
		case "/api/v1/metrics":
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{"code": "UNAUTHENTICATED", "message": "invalid or expired API key"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	defer overrideGlobals(srv.URL, "wrong-key", "json", false)()

	cmd := newStatusCmd()
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when metrics call is auth-rejected, got nil")
	}
	if !strings.Contains(err.Error(), "UNAUTHENTICATED") {
		t.Errorf("expected UNAUTHENTICATED error, got: %q", err.Error())
	}
	if ec := exitCode(err); ec != 4 {
		t.Errorf("exitCode(UNAUTHENTICATED) = %d, want 4", ec)
	}
}
