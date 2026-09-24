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

	// Count recent requests for this tool in this session
	// Use a 1-minute window from "now" for the check (the SQL trigger
	// uses now() at insert time, so we use the same window)
	since := time.Now().Add(-1 * time.Minute)
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
