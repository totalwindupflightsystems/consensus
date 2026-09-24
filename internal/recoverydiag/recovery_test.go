package recoverydiag

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/sqlite"
)

func TestVerifyDurableAgentProgressRejectsInputOnly(t *testing.T) {
	pre := Snapshot{
		MemoryEventCount: 1,
		UserMessages:     1,
		AgentArtifacts:   map[string]struct{}{},
	}
	post := Snapshot{
		MemoryEventCount: 1,
		UserMessages:     1,
		AgentArtifacts:   map[string]struct{}{},
	}

	err := VerifyDurableAgentProgress(pre, post)
	if err == nil {
		t.Fatal("input-only memory incorrectly proved durable agent progress")
	}
	if !strings.Contains(err.Error(), "no durable agent artifact") {
		t.Fatalf("error = %q, want missing durable-agent-artifact diagnostic", err)
	}
}

func TestVerifyDurableAgentProgressRequiresSameArtifactAfterRestart(t *testing.T) {
	pre := Snapshot{AgentArtifacts: map[string]struct{}{"memory:7": {}}}

	if err := VerifyDurableAgentProgress(pre, Snapshot{AgentArtifacts: map[string]struct{}{}}); err == nil {
		t.Fatal("lost committed artifact incorrectly passed recovery proof")
	}
	if err := VerifyDurableAgentProgress(pre, Snapshot{AgentArtifacts: map[string]struct{}{"memory:7": {}, "iteration:2": {}}}); err != nil {
		t.Fatalf("retained committed artifact rejected: %v", err)
	}
}

func TestReadQueriesDurableAndStrandedEvidence(t *testing.T) {
	ctx := context.Background()
	database, err := sqlite.Open(ctx, db.Config{
		URL:          "sqlite://" + filepath.Join(t.TempDir(), "recovery.db"),
		MaxOpenConns: 1,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer database.Close()

	statements := []string{
		`CREATE TABLE memory_events (id INTEGER PRIMARY KEY, session_id TEXT, type TEXT)`,
		`CREATE TABLE iteration_commits (iteration_id INTEGER PRIMARY KEY, session_id TEXT)`,
		`CREATE TABLE audit_logs (id INTEGER PRIMARY KEY, session_id TEXT, result TEXT)`,
		`CREATE TABLE staging_buffer (id INTEGER PRIMARY KEY, session_id TEXT, status TEXT)`,
		`INSERT INTO memory_events VALUES (1, 'session-1', 'user_message')`,
		`INSERT INTO memory_events VALUES (2, 'session-1', 'tool_result')`,
		`INSERT INTO memory_events VALUES (3, 'other-session', 'text_block')`,
		`INSERT INTO iteration_commits VALUES (4, 'session-1')`,
		`INSERT INTO audit_logs VALUES (5, 'session-1', 'committed')`,
		`INSERT INTO staging_buffer VALUES (6, 'session-1', 'executed')`,
	}
	for _, statement := range statements {
		if err := database.Exec(ctx, statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}

	snapshot, err := Read(ctx, database, "session-1")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, artifact := range []string{"memory:2", "iteration:4", "audit:5"} {
		if _, ok := snapshot.AgentArtifacts[artifact]; !ok {
			t.Errorf("missing queried artifact %s", artifact)
		}
	}
	if _, ok := snapshot.AgentArtifacts["memory:3"]; ok {
		t.Error("artifact from another session leaked into recovery snapshot")
	}
	if got := strings.Join(snapshot.StrandedStaging, ","); got != "staging:6" {
		t.Fatalf("stranded staging = %q, want staging:6", got)
	}
}

func TestClassifyRowsSurfacesStrandedStaging(t *testing.T) {
	snapshot := classifyRows(
		[]db.Row{
			{"id": int64(1), "type": "user_message"},
			{"id": int64(2), "type": "text_block"},
			{"id": int64(3), "type": "system"},
		},
		[]db.Row{{"iteration_id": int64(4)}},
		[]db.Row{
			{"id": int64(5), "result": "committed"},
			{"id": int64(6), "result": "rolled_back"},
		},
		[]db.Row{
			{"id": int64(7), "status": "staged"},
			{"id": int64(8), "status": "executed"},
			{"id": int64(9), "status": "committed"},
		},
	)

	if snapshot.UserMessages != 1 || snapshot.MemoryEventCount != 3 {
		t.Fatalf("memory classification = users:%d total:%d, want 1 and 3", snapshot.UserMessages, snapshot.MemoryEventCount)
	}
	for _, artifact := range []string{"memory:2", "iteration:4", "audit:5"} {
		if _, ok := snapshot.AgentArtifacts[artifact]; !ok {
			t.Errorf("missing durable artifact %s", artifact)
		}
	}
	if _, ok := snapshot.AgentArtifacts["audit:6"]; ok {
		t.Error("rolled-back audit row classified as durable progress")
	}
	if got := strings.Join(snapshot.StrandedStaging, ","); got != "staging:7,staging:8" {
		t.Fatalf("stranded staging = %q, want staging:7,staging:8", got)
	}
	wantDiagnostic := "durable_agent_artifacts=3 user_messages=1 memory_events=3 staging_buffer={committed=1,executed=1,staged=1} stranded=2 stranded_entries=[staging:7,staging:8]"
	if got := snapshot.Diagnostic(); got != wantDiagnostic {
		t.Fatalf("diagnostic = %q, want %q", got, wantDiagnostic)
	}
}
