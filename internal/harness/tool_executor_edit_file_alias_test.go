// Package harness: SCN-CONSENSUS-025 — a drifted `edit_file` request must
// reach the native edit handler instead of failing closed.
//
// Measured defect (real default-ALL scenario run, 2026-11-28, run 3 of the
// s2_read_file flake): the model asked for edit_file — the conventional agent
// vocabulary — while only `edit` (handler_ref 'edit') was registered. The
// request failed with `tool "edit_file" is not registered or enabled`, the
// edit never ran, and the scenario's edit leg proof counted zero successful
// edit results. read_file/write_file drift was already absorbed by the
// handler_ref arm (migrations 033/034); migration 037 seeds the edit_file
// alias row with the same seed-row pattern as 035 (shell aliases).
//
// Every assertion below is on the persisted request outcome or on the file
// system, never on the executor's own claim.
package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/tools"
)

// seedEditFamily registers the registry population migrations 033+037 leave
// behind: the public `edit` row and the `edit_file` alias row, both enabled,
// both pointing at the native edit handler.
func seedEditFamily(t *testing.T, database db.DB, requiresApproval int) {
	t.Helper()
	ctx := context.Background()
	for _, row := range []struct{ name, ref string }{{"edit", "edit"}, {"edit_file", "edit"}} {
		if err := database.Exec(ctx, `INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref, enabled, requires_approval)
			VALUES ($1, $2, 'SCN-CONSENSUS-025 edit alias probe', 'external', 'go_native', $3, 1, $4)`,
			"tool-"+row.name, row.name, row.ref, requiresApproval); err != nil {
			t.Fatalf("register tool %q: %v", row.name, err)
		}
	}
}

// The live defect's falsifier: a request under the drifted edit_file vocabulary
// dispatches to the edit handler, completes successfully, and the replacement
// lands in the workspace file.
func TestToolExecutor_EditFileAliasCompletesEdit(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)
	const (
		stale = "STALE-DRAFT v1\n"
		fresh = "SEED-EDIT-77\n"
	)
	if err := os.WriteFile(filepath.Join(workspace, "edit-target.txt"), []byte(stale), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	seedEditFamily(t, database, 0)

	ctx := context.Background()
	executor := NewToolExecutor(database, DefaultToolExecutorConfig())

	reqID := queueToolRequest(t, database, "sess-te-01", "edit_file",
		`{"path":"edit-target.txt","old_string":"STALE-DRAFT v1","new_string":"SEED-EDIT-77"}`)

	count, err := executor.PollOnce(ctx)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if count != 1 {
		t.Fatalf("processed = %d, want 1", count)
	}

	status, output, isError := requestOutcome(t, database, reqID)
	if status != "completed" || isError != 0 {
		t.Fatalf("edit_file via alias: status=%q is_error=%d output=%q; the alias row must dispatch to the edit handler",
			status, isError, output)
	}
	body, readErr := os.ReadFile(filepath.Join(workspace, "edit-target.txt"))
	if readErr != nil {
		t.Fatalf("read back edited file: %v", readErr)
	}
	if string(body) != fresh {
		t.Errorf("file content = %q, want the edit to have replaced the stale draft with %q", string(body), fresh)
	}
}

// A drifted name that matches no registry row NAME and no handler_ref must
// still fail closed — the alias row opens a path for edit_file only, not for
// arbitrary vocabulary.
func TestToolExecutor_UnknownEditVocabularyStillFailsClosed(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)
	if err := os.WriteFile(filepath.Join(workspace, "edit-target.txt"), []byte("STALE-DRAFT v1\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	seedEditFamily(t, database, 0)

	ctx := context.Background()
	executor := NewToolExecutor(database, DefaultToolExecutorConfig())

	// Matches no row's name ('edit', 'edit_file') and no row's handler_ref
	// ('edit'): the request must be refused, nothing edited.
	reqID := queueToolRequest(t, database, "sess-te-01", "edit_handler",
		`{"path":"edit-target.txt","old_string":"STALE-DRAFT v1","new_string":"SEED-EDIT-77"}`)

	count, err := executor.PollOnce(ctx)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if count != 1 {
		t.Fatalf("processed = %d, want 1", count)
	}

	status, output, isError := requestOutcome(t, database, reqID)
	if status != "failed" || isError != 1 {
		t.Fatalf("unknown vocabulary: status=%q is_error=%d output=%q; want a failed request", status, isError, output)
	}
	if !strings.Contains(output, "not registered or enabled") {
		t.Errorf("failure = %q, want it to name the fail-closed refusal", output)
	}
	body, readErr := os.ReadFile(filepath.Join(workspace, "edit-target.txt"))
	if readErr != nil {
		t.Fatalf("read back fixture: %v", readErr)
	}
	if string(body) != "STALE-DRAFT v1\n" {
		t.Errorf("file content = %q; an unknown tool name must not edit anything", string(body))
	}
}

// The alias row carries its OWN approval flag (migration 035 semantics: alias
// rows are independent registry entries, individually enable/disable-able).
// Gating the edit_file row itself must park a drifted edit_file request, and
// nothing is written. (Cross-row inheritance is deliberately NOT asserted:
// gating `edit` does not gate `edit_file`, exactly as gating `bash` does not
// gate `shell` under 035.)
func TestToolExecutor_EditFileAliasGatedFailsClosed(t *testing.T) {
	database, cleanup := setupToolExecutorTestDB(t)
	defer cleanup()

	workspace := t.TempDir()
	t.Setenv(tools.WorkspaceEnvVar, workspace)
	if err := os.WriteFile(filepath.Join(workspace, "edit-target.txt"), []byte("STALE-DRAFT v1\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	seedEditFamily(t, database, 0)
	ctx := context.Background()
	if err := database.Exec(ctx, `UPDATE tools_registry SET requires_approval = 1 WHERE name = 'edit_file'`); err != nil {
		t.Fatalf("gate the edit_file alias row: %v", err)
	}

	executor := NewToolExecutor(database, DefaultToolExecutorConfig())
	reqID := queueToolRequest(t, database, "sess-te-01", "edit_file",
		`{"path":"edit-target.txt","old_string":"STALE-DRAFT v1","new_string":"SEED-EDIT-77"}`)

	count, err := executor.PollOnce(ctx)
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	// A gated request resolved by NAME is PARKED, not failed: PollOnce processes
	// nothing and an approval_requests row is created for it.
	if count != 0 {
		t.Fatalf("processed = %d, want 0 (a gated request must park, not run)", count)
	}

	rows, qerr := database.Query(ctx, `SELECT q.status AS req_status, a.status AS appr_status, a.target_tool AS target
		FROM tool_requests q JOIN approval_requests a ON a.id = q.approval_request_id
		WHERE q.id = $1`, reqID)
	if qerr != nil {
		t.Fatalf("read back parked request: %v", qerr)
	}
	if len(rows) != 1 {
		t.Fatalf("gated edit_file request parked %d approval rows, want exactly 1", len(rows))
	}
	if toString(rows[0]["req_status"]) != "awaiting_approval" || toString(rows[0]["appr_status"]) != "pending" {
		t.Errorf("parked request = req_status %q / approval %q, want req awaiting_approval + approval pending",
			toString(rows[0]["req_status"]), toString(rows[0]["appr_status"]))
	}
	if toString(rows[0]["target"]) != "edit_file" {
		t.Errorf("approval target_tool = %q, want edit_file", toString(rows[0]["target"]))
	}
	body, readErr := os.ReadFile(filepath.Join(workspace, "edit-target.txt"))
	if readErr != nil {
		t.Fatalf("read back fixture: %v", readErr)
	}
	if string(body) != "STALE-DRAFT v1\n" {
		t.Errorf("file content = %q; a gated edit_file request must not edit anything", string(body))
	}
}
