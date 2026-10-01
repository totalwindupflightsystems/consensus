// Package tools: rate limiting for tool execution (WI-005).
//
// The rate limiter provides Go-level enforcement of per-tool rate limits
// defined in tools_registry.rate_limit_per_min. This complements the SQL-level
// trigger (enforce_tool_rate_limit in migration 001) for defense in depth.
//
// axiom:trace work_item=WI-005 spec=specs/010-tools.md,specs/003-database.md plan=phase-2/task-1
package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// Rate Limit Check
// ============================================================================

// CheckToolRateLimit checks whether a tool request would exceed the per-minute
// rate limit for the given tool and session.
//
// Returns nil if under the limit, or an error if the limit would be exceeded.
// Returns nil if the tool has no rate_limit_per_min configured.
func CheckToolRateLimit(ctx context.Context, database db.DB, toolName, sessionID string) error {
	if database == nil {
		return fmt.Errorf("rate_limit: no database configured")
	}

	// Query the tool's rate limit
	rows, err := database.Query(ctx, `
		SELECT rate_limit_per_min
		FROM tools_registry
		WHERE name = $1 AND enabled = true
		LIMIT 1
	`, toolName)
	if err != nil {
		return fmt.Errorf("rate_limit: lookup %q: %w", toolName, err)
	}
	if len(rows) == 0 {
		// Tool not found — no rate limit enforcement
		return nil
	}

	rateLimitRaw := rows[0]["rate_limit_per_min"]
	if rateLimitRaw == nil {
		// No rate limit configured
		return nil
	}

	maxPerMin := toInt(rateLimitRaw)
	if maxPerMin <= 0 {
		// Zero or negative means no limit
		return nil
	}

	// Count recent requests for this tool in this session.
	//
	// The 1-minute window bound is normalized to UTC because
	// tool_requests.created_at holds UTC timestamps: SQLite's
	// CURRENT_TIMESTAMP / datetime('now') write UTC text and the column compares
	// lexicographically, while Postgres stores TIMESTAMPTZ. Rendering the bound
	// in the process-local zone shifts it by the host's UTC offset, so the window
	// silently stops matching recent rows (positive offsets) or starts matching
	// stale ones (negative offsets) — both clock directions are wrong
	// (QA-CONSENSUS-23).
	since := rateLimitWindowStart(time.Now())
	countRows, err := database.Query(ctx, `
		SELECT COUNT(*) as cnt
		FROM tool_requests
		WHERE session_id = $1
		  AND tool_name = $2
		  AND created_at >= $3
		  AND status NOT IN ('timeout', 'failed')
	`, sessionID, toolName, since)
	if err != nil {
		return fmt.Errorf("rate_limit: count: %w", err)
	}

	recentCount := 0
	if len(countRows) > 0 {
		recentCount = toInt(countRows[0]["cnt"])
	}

	if recentCount >= maxPerMin {
		return fmt.Errorf("rate limit exceeded for tool %q: %d requests in last minute (max %d)",
			toolName, recentCount, maxPerMin)
	}

	return nil
}

// rateLimitWindowStart returns the lower bound of the sliding one-minute
// window evaluated by CheckToolRateLimit, normalized to UTC.
//
// Normalizing matters on SQLite: created_at is TEXT written in UTC (by
// CURRENT_TIMESTAMP / datetime('now')) and SQLite compares TEXT
// lexicographically, so a bound rendered in the process-local zone is offset by
// the host's UTC offset — matching no recent row at positive offsets and stale
// rows at negative ones. Postgres compares TIMESTAMPTZ instants and is
// unaffected either way.
//
// The SQL-level trigger (enforce_tool_rate_limit, migration 001 §11.5) needs no
// equivalent handling: it compares now() against the TIMESTAMPTZ created_at
// column inside the database, where the session timezone cannot shift the
// comparison, and the SQLite schema has no such trigger (the Go path above is
// the only SQLite enforcement).
func rateLimitWindowStart(now time.Time) time.Time {
	return now.UTC().Add(-1 * time.Minute)
}
