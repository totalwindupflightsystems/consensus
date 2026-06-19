// Package harness: integration test helpers for SQLite in-memory backend.
//
// TestHarness wires a real SQLite database with a minimal test migration
// and provides helper methods for seeding test data.
//
// axiom:trace work_item=runtime-harness-01 spec=specs/008-harness.md plan=phase-6/task-6-1/step-6-1-1
package harness

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	"github.com/wojons/consensus/internal/hitl"
)

// testHarness wraps a Harness with a real SQLite in-memory database for
// integration testing. It handles setup (migration, seeding) and teardown.
type testHarness struct {
	*Harness
	conn    db.DB
	ctx    context.Context
	cancel context.CancelFunc
	hitl   *hitl.Manager
	tmpPath string // temp database file, cleaned up on close
}

// newTestHarness creates a new harness connected to a fresh SQLite in-memory database.
// It runs the test migration, seeds the model_registry, and returns a ready-to-use
// harness with a mock LLM client.
func newTestHarness(llm LLMClient) (*testHarness, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Use a temp file instead of :memory: so that all sql.DB pool connections
	// share the same database. With :memory:, each connection gets its own
	// private in-memory database, causing "no such table" errors when the
	// harness opens a separate transaction connection.
	tmpFile, err := os.CreateTemp("", "consensus-test-*.db")
	if err != nil {
		cancel()
		return nil, fmt.Errorf("test harness: create temp db: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	conn, err := driver.Open(ctx, db.Config{URL: "sqlite://" + tmpPath})
	if err != nil {
		os.Remove(tmpPath)
		cancel()
		return nil, fmt.Errorf("test harness: open sqlite: %w", err)
	}

	// Run test migration
	if err := runTestMigration(ctx, conn); err != nil {
		conn.Close()
		cancel()
		return nil, fmt.Errorf("test harness: migration: %w", err)
	}

	// Seed model_registry (required FK for sessions)
	if err := seedModelRegistry(ctx, conn); err != nil {
		conn.Close()
		cancel()
		return nil, fmt.Errorf("test harness: seed models: %w", err)
	}

	h := New(conn, llm)

	return &testHarness{
		Harness: h,
		conn:    conn,
		ctx:     ctx,
		cancel:  cancel,
		hitl:    hitl.New(conn),
		tmpPath: tmpPath,
	}, nil
}

// close cleans up the test harness.
func (th *testHarness) close() {
	th.cancel()
	th.conn.Close()
	if th.tmpPath != "" {
		os.Remove(th.tmpPath)
	}
}

// runTestMigration loads and executes the SQLite test migration from testdata/.
func runTestMigration(ctx context.Context, conn db.DB) error {
	path := "testdata/migration_test.sql"
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("test migration: read %s: %w", path, err)
	}

	// Split on semicolons for individual execution (SQLite doesn't like multi-statement Exec)
	statements := splitMigrationStatements(string(data))
	for _, stmt := range statements {
		if err := conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("test migration: %w (SQL: %s)", err, truncateSQL(stmt, 80))
		}
	}
	return nil
}

// splitMigrationStatements splits the migration SQL on semicolons, preserving
// multi-line statements like CREATE TABLE.
func splitMigrationStatements(sql string) []string {
	var result []string
	var current strings.Builder
	lines := strings.Split(sql, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}

		current.WriteString(line)
		current.WriteString("\n")

		// A semicolon at or near the end of the line indicates a statement boundary
		if strings.HasSuffix(trimmed, ";") {
			stmt := strings.TrimSpace(current.String())
			stmt = strings.TrimSuffix(stmt, ";")
			if stmt != "" {
				result = append(result, stmt)
			}
			current.Reset()
		}
	}

	// Flush remaining (shouldn't happen with proper migrations, but be safe)
	if current.Len() > 0 {
		stmt := strings.TrimSpace(current.String())
		stmt = strings.TrimSuffix(stmt, ";")
		if stmt != "" {
			result = append(result, stmt)
		}
	}

	return result
}

// seedModelRegistry inserts the minimum model entry needed for FK constraints.
func seedModelRegistry(ctx context.Context, conn db.DB) error {
	return conn.Exec(ctx, `INSERT INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out) VALUES ('test-model', 1, 128000, 1.50, 15.00)`)
}

// createTestSession inserts a minimal session row and returns its ID.
// The session defaults to trust_level='high' for backward compatibility
// with existing tests (Tier 3 raw SQL access).
func (th *testHarness) createTestSession() (string, error) {
	// SQLite doesn't have gen_random_uuid(), use a fixed test UUID
	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeee1"
	err := th.conn.Exec(th.ctx, `INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal) VALUES ($1, 'test-agent', 'test-model', 'idle', 'high', 'Test goal: prove the harness works')`, sessionID)
	return sessionID, err
}

// createTestSessionWithTrustLevel creates a test session with a specific trust level.
func (th *testHarness) createTestSessionWithTrustLevel(trustLevel string) (string, error) {
	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeee" + trustLevel[:1]
	err := th.conn.Exec(th.ctx, `INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal) VALUES ($1, 'test-agent', 'test-model', 'idle', $2, 'Test goal')`, sessionID, trustLevel)
	return sessionID, err
}

// assertAuditLogCount returns the number of audit log rows for a session.
func (th *testHarness) assertAuditLogCount(sessionID string) (int, error) {
	rows, err := th.conn.Query(th.ctx, `SELECT COUNT(*) as cnt FROM audit_logs WHERE session_id = $1`, sessionID)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return toInt(rows[0]["cnt"]), nil
}

// assertIterationSnapshotCount returns the number of iteration_commits rows for a session.
func (th *testHarness) assertIterationSnapshotCount(sessionID string) (int, error) {
	rows, err := th.conn.Query(th.ctx, `SELECT COUNT(*) as cnt FROM iteration_commits WHERE session_id = $1`, sessionID)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return toInt(rows[0]["cnt"]), nil
}
