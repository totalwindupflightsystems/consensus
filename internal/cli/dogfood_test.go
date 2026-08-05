package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// ============================================================================
// DOGFOOD-005 — empty tool/skill registries print an actionable hint
// ============================================================================

func TestToolList_EmptyRegistry_PrintsHint(t *testing.T) {
	ms := newMockAPIServer()
	defer ms.Close()
	defer overrideGlobals(ms.URL, "test-key", "table", false)()

	// Override the mock to return an empty tools registry.
	ms.Server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/tools" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode([]map[string]any{})
			return
		}
		ms.handle(w, r)
	})

	cmd := newToolListCmd()
	err, out := captureStdout(func() error { return cmd.Execute() })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No tools registered yet") {
		t.Errorf("expected empty-registry hint in output, got: %q", out)
	}
	if strings.Contains(out, "(no results)") {
		t.Errorf("expected hint to replace '(no results)', got: %q", out)
	}
}

func TestSkillList_EmptyRegistry_PrintsHint(t *testing.T) {
	ms := newMockAPIServer()
	defer ms.Close()
	defer overrideGlobals(ms.URL, "test-key", "table", false)()

	ms.Server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/skills" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode([]map[string]any{})
			return
		}
		ms.handle(w, r)
	})

	cmd := newSkillListCmd()
	err, out := captureStdout(func() error { return cmd.Execute() })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No skills installed yet") {
		t.Errorf("expected empty-registry hint in output, got: %q", out)
	}
	if strings.Contains(out, "(no results)") {
		t.Errorf("expected hint to replace '(no results)', got: %q", out)
	}
}

// ============================================================================
// DOGFOOD-007 — empty session logs print an explanatory message
// ============================================================================

func TestSessionLogs_EmptyIterations_PrintsExplanation(t *testing.T) {
	ms := newMockAPIServer()
	defer ms.Close()
	defer overrideGlobals(ms.URL, "test-key", "table", false)()

	ms.Server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/iterations") && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode([]map[string]any{})
			return
		}
		ms.handle(w, r)
	})

	cmd := newSessionLogsCmd()
	cmd.SetArgs([]string{"sess-empty"})
	err, out := captureStdout(func() error { return cmd.Execute() })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No iteration commits yet") {
		t.Errorf("expected explanatory message in output, got: %q", out)
	}
	if !strings.Contains(out, "/api/v1/events?session_id=sess-empty") {
		t.Errorf("expected SSE endpoint pointer in output, got: %q", out)
	}
	if strings.Contains(out, "(no results)") {
		t.Errorf("expected explanation to replace '(no results)', got: %q", out)
	}
}

// ============================================================================
// Formatter empty-hint unit behavior
// ============================================================================

func TestFormatter_SetEmptyHint_ReplacesNoResults(t *testing.T) {
	var buf bytes.Buffer
	fm := NewFormatter(&buf, FormatTable, false)
	fm.SetEmptyHint("No tools registered yet — see docs/TOOLS.md to learn how to register tools")

	rows := []map[string]any{}
	if err := fm.PrintTable(rows, []string{"name", "description"}); err != nil {
		t.Fatalf("print: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "No tools registered yet") {
		t.Errorf("expected hint output, got: %q", got)
	}
}

func TestFormatter_NoEmptyHint_DefaultNoResults(t *testing.T) {
	var buf bytes.Buffer
	fm := NewFormatter(&buf, FormatTable, false)

	rows := []map[string]any{}
	if err := fm.PrintTable(rows, []string{"name", "description"}); err != nil {
		t.Fatalf("print: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "(no results)") {
		t.Errorf("expected default '(no results)' output, got: %q", got)
	}
}
