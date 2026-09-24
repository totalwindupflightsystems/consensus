// Package tools: tests for rate limiter (WI-005).
//
// axiom:trace work_item=WI-005 spec=specs/010-tools.md plan=phase-4/task-2 test=internal/tools/rate_limiter_test.go
package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// setupRateLimitTestDB creates an in-memory database with tables needed for
// rate limiter testing.
func setupRateLimitTestDB(t *testing.T) (db.DB, func()) {
	t.Helper()
	ctx := context.Background()
	database, err := driver.Open(ctx, db.Config{URL: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			agent_name TEXT NOT NULL DEFAULT 'test',
			status TEXT NOT NULL DEFAULT 'idle',
			project_id TEXT,
			heartbeat_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tools_registry (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL DEFAULT 1,
			rate_limit_per_min INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS tool_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			tool_name TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
	} {
		if err := database.Exec(ctx, stmt); err != nil {
			database.Close()
			t.Fatalf("create table: %v", err)
		}
	}

	_ = database.Exec(ctx, `INSERT INTO sessions (id) VALUES ('sess-rl-01')`)

	cleanup := func() { database.Close() }
	return database, cleanup
}

// ============================================================================
// Rate Limit Tests
// ============================================================================

func TestCheckToolRateLimit_NoLimit(t *testing.T) {
	database, cleanup := setupRateLimitTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Register a tool with no rate limit
	_ = database.Exec(ctx, `INSERT INTO tools_registry (id, name, enabled) VALUES ('rl-nolimit', 'no_limit_tool', 1)`)

	err := CheckToolRateLimit(ctx, database, "no_limit_tool", "sess-rl-01")
	if err != nil {
		t.Errorf("expected no error for tool without rate limit, got: %v", err)
	}
}

func TestCheckToolRateLimit_UnderLimit(t *testing.T) {
	database, cleanup := setupRateLimitTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Register a tool with rate limit of 10 per minute
	_ = database.Exec(ctx, `INSERT INTO tools_registry (id, name, enabled, rate_limit_per_min) VALUES ('rl-10', 'limited_tool', 1, 10)`)

	// Insert 3 requests (under the limit)
	for i := 0; i < 3; i++ {
		_ = database.Exec(ctx, `INSERT INTO tool_requests (session_id, tool_name, status, created_at) VALUES ($1, 'limited_tool', 'completed', datetime('now'))`,
			"sess-rl-01")
	}

	err := CheckToolRateLimit(ctx, database, "limited_tool", "sess-rl-01")
	if err != nil {
		t.Errorf("expected no error (3 requests, limit 10), got: %v", err)
	}
}

func TestCheckToolRateLimit_Exceeded(t *testing.T) {
	database, cleanup := setupRateLimitTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Register a tool with rate limit of 3 per minute
	_ = database.Exec(ctx, `INSERT INTO tools_registry (id, name, enabled, rate_limit_per_min) VALUES ('rl-3', 'heavy_tool', 1, 3)`)

	// Insert 5 requests (exceeds the limit)
	for i := 0; i < 5; i++ {
		_ = database.Exec(ctx, `INSERT INTO tool_requests (session_id, tool_name, status, created_at) VALUES ($1, 'heavy_tool', 'completed', datetime('now'))`,
			"sess-rl-01")
	}

	err := CheckToolRateLimit(ctx, database, "heavy_tool", "sess-rl-01")
	if err == nil {
		t.Fatal("expected rate limit error, got nil")
	}
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Errorf("expected 'rate limit exceeded' error, got: %v", err)
	}
	t.Logf("Rate limit correctly enforced: %v", err)
}

func TestCheckToolRateLimit_DisabledTool(t *testing.T) {
	database, cleanup := setupRateLimitTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Register a disabled tool with a rate limit
	_ = database.Exec(ctx, `INSERT INTO tools_registry (id, name, enabled, rate_limit_per_min) VALUES ('rl-dis', 'disabled_tool', 0, 5)`)

	// Disabled tools should not enforce rate limits (they can't be executed anyway)
	err := CheckToolRateLimit(ctx, database, "disabled_tool", "sess-rl-01")
	if err != nil {
		t.Errorf("expected no error for disabled tool, got: %v", err)
	}
}

func TestCheckToolRateLimit_UnknownTool(t *testing.T) {
	database, cleanup := setupRateLimitTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Unknown tools should not trigger rate limit errors
	err := CheckToolRateLimit(ctx, database, "nonexistent_tool", "sess-rl-01")
	if err != nil {
		t.Errorf("expected no error for unknown tool, got: %v", err)
	}
}

func TestCheckToolRateLimit_SessionIsolation(t *testing.T) {
	database, cleanup := setupRateLimitTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Add a second session
	_ = database.Exec(ctx, `INSERT INTO sessions (id) VALUES ('sess-rl-02')`)

	// Register a tool with rate limit of 3 per minute
	_ = database.Exec(ctx, `INSERT INTO tools_registry (id, name, enabled, rate_limit_per_min) VALUES ('rl-sess', 'per_session_tool', 1, 3)`)

	// Session 1 uses the tool 5 times (exceeded)
	for i := 0; i < 5; i++ {
		_ = database.Exec(ctx, `INSERT INTO tool_requests (session_id, tool_name, status, created_at) VALUES ($1, 'per_session_tool', 'completed', datetime('now'))`,
			"sess-rl-01")
	}

	// Session 2 hasn't used the tool at all (should be under limit)
	err := CheckToolRateLimit(ctx, database, "per_session_tool", "sess-rl-02")
	if err != nil {
		t.Errorf("expected no error for session 2 (different session), got: %v", err)
	}

	// Session 1 should still be exceeded
	err = CheckToolRateLimit(ctx, database, "per_session_tool", "sess-rl-01")
	if err == nil {
		t.Error("expected rate limit error for session 1")
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestCheckToolRateLimit_NilDB(t *testing.T) {
	err := CheckToolRateLimit(context.Background(), nil, "test_tool", "sess-test")
	if err == nil || !strings.Contains(err.Error(), "no database") {
		t.Errorf("expected 'no database' error, got: %v", err)
	}
}

func TestCheckToolRateLimit_ZeroLimit(t *testing.T) {
	database, cleanup := setupRateLimitTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// A rate limit of 0 means no limit
	_ = database.Exec(ctx, `INSERT INTO tools_registry (id, name, enabled, rate_limit_per_min) VALUES ('rl-zero', 'zero_limit_tool', 1, 0)`)

	err := CheckToolRateLimit(ctx, database, "zero_limit_tool", "sess-rl-01")
	if err != nil {
		t.Errorf("expected no error for zero rate limit, got: %v", err)
	}
}
