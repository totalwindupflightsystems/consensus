// Package recoverydiag classifies durable crash-recovery evidence without
// changing Consensus's transaction or recovery semantics.
package recoverydiag

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/wojons/consensus/internal/db"
)

// Snapshot summarizes committed agent progress and staging residue for one
// session. AgentArtifacts contains stable, table-qualified row identifiers so
// a proof can verify the same committed rows before and after restart.
type Snapshot struct {
	MemoryEventCount int
	UserMessages     int
	AgentArtifacts   map[string]struct{}
	StagingByStatus  map[string]int
	StrandedStaging  []string
}

// Read loads the durable rows needed by the crash-recovery demos. It reports
// staged/executed staging_buffer rows as stranded evidence; those rows are
// diagnostics, not committed progress.
func Read(ctx context.Context, database db.DB, sessionID string) (Snapshot, error) {
	memoryRows, err := database.Query(ctx,
		`SELECT id, type FROM memory_events WHERE session_id = $1 ORDER BY id`, sessionID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("recovery diagnostics: query memory_events: %w", err)
	}
	iterationRows, err := database.Query(ctx,
		`SELECT iteration_id FROM iteration_commits WHERE session_id = $1 ORDER BY iteration_id`, sessionID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("recovery diagnostics: query iteration_commits: %w", err)
	}
	auditRows, err := database.Query(ctx,
		`SELECT id, result FROM audit_logs WHERE session_id = $1 ORDER BY id`, sessionID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("recovery diagnostics: query audit_logs: %w", err)
	}
	stagingRows, err := database.Query(ctx,
		`SELECT id, status FROM staging_buffer WHERE session_id = $1 ORDER BY id`, sessionID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("recovery diagnostics: query staging_buffer: %w", err)
	}

	return classifyRows(memoryRows, iterationRows, auditRows, stagingRows), nil
}

func classifyRows(memoryRows, iterationRows, auditRows, stagingRows []db.Row) Snapshot {
	snapshot := Snapshot{
		MemoryEventCount: len(memoryRows),
		AgentArtifacts:   make(map[string]struct{}),
		StagingByStatus:  make(map[string]int),
	}

	for _, row := range memoryRows {
		eventType := rowString(row["type"])
		switch eventType {
		case "user_message":
			snapshot.UserMessages++
		case "text_block", "tool_call", "tool_result", "thinking":
			snapshot.AgentArtifacts[artifactKey("memory", row["id"])] = struct{}{}
		}
	}
	for _, row := range iterationRows {
		snapshot.AgentArtifacts[artifactKey("iteration", row["iteration_id"])] = struct{}{}
	}
	for _, row := range auditRows {
		if rowString(row["result"]) == "committed" {
			snapshot.AgentArtifacts[artifactKey("audit", row["id"])] = struct{}{}
		}
	}
	for _, row := range stagingRows {
		status := rowString(row["status"])
		snapshot.StagingByStatus[status]++
		if status == "staged" || status == "executed" {
			snapshot.StrandedStaging = append(snapshot.StrandedStaging, artifactKey("staging", row["id"]))
		}
	}
	sort.Strings(snapshot.StrandedStaging)
	return snapshot
}

// VerifyDurableAgentProgress proves that at least one committed, non-user agent
// artifact existed before the crash and that every such artifact remains after
// restart. User-message persistence alone is explicitly insufficient.
func VerifyDurableAgentProgress(pre, post Snapshot) error {
	if len(pre.AgentArtifacts) == 0 {
		return fmt.Errorf("no durable agent artifact existed before crash; input-only evidence is insufficient")
	}

	missing := make([]string, 0)
	for artifact := range pre.AgentArtifacts {
		if _, ok := post.AgentArtifacts[artifact]; !ok {
			missing = append(missing, artifact)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("durable agent artifacts missing after restart: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Diagnostic returns a deterministic summary suitable for demo and E2E logs.
func (s Snapshot) Diagnostic() string {
	statuses := make([]string, 0, len(s.StagingByStatus))
	for status, count := range s.StagingByStatus {
		statuses = append(statuses, fmt.Sprintf("%s=%d", status, count))
	}
	sort.Strings(statuses)
	if len(statuses) == 0 {
		statuses = append(statuses, "none")
	}
	return fmt.Sprintf(
		"durable_agent_artifacts=%d user_messages=%d memory_events=%d staging_buffer={%s} stranded=%d stranded_entries=[%s]",
		len(s.AgentArtifacts), s.UserMessages, s.MemoryEventCount,
		strings.Join(statuses, ","), len(s.StrandedStaging), strings.Join(s.StrandedStaging, ","),
	)
}

func artifactKey(table string, id any) string {
	return table + ":" + rowString(id)
}

func rowString(value any) string {
	if value == nil {
		return "<nil>"
	}
	return fmt.Sprint(value)
}
