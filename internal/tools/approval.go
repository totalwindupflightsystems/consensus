// Package tools: approval gating for tool execution (WI-005).
//
// When a tool in tools_registry has requires_approval=true, the executor must
// create a HITL approval request before executing the tool. This file provides
// the check and request creation functions.
//
// See SPEC-014 §2.2 (Tool-Required Interrupts) and §3.1 (approval_requests schema).
//
// axiom:trace work_item=WI-005 spec=specs/014-hitl-interrupt-state.md plan=phase-2/task-2
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/wojons/conscientiousness/internal/db"
)

// ============================================================================
// Approval Check
// ============================================================================

// ToolRequiresApproval checks whether a tool has requires_approval=true.
func ToolRequiresApproval(ctx context.Context, database db.DB, toolName string) (bool, error) {
	if database == nil {
		return false, fmt.Errorf("approval: no database configured")
	}

	rows, err := database.Query(ctx, `
		SELECT requires_approval
		FROM tools_registry
		WHERE name = $1 AND enabled = true
		LIMIT 1
	`, toolName)
	if err != nil {
		return false, fmt.Errorf("approval: lookup %q: %w", toolName, err)
	}
	if len(rows) == 0 {
		return false, nil // tool not found, can't require approval
	}

	return toBool(rows[0]["requires_approval"]), nil
}

// ============================================================================
// Approval Request Creation
// ============================================================================

// CreateToolApprovalRequest creates a HITL approval request for a tool execution.
// It inserts into approval_requests, updates the session status to 'paused',
// and returns the approval request ID.
//
// The caller is responsible for:
//   - Updating the tool_request status to 'awaiting_approval'
//   - Linking the approval_request_id to the tool_request
//   - Checking approval status on subsequent poll cycles
func CreateToolApprovalRequest(ctx context.Context, database db.DB, sessionID, toolName string, parameters map[string]any) (string, error) {
	if database == nil {
		return "", fmt.Errorf("approval: no database configured")
	}

	// Marshal parameters for context
	paramsJSON, err := json.Marshal(parameters)
	if err != nil {
		return "", fmt.Errorf("approval: marshal params: %w", err)
	}

	requestID := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(60 * time.Minute) // 1 hour default timeout

	// Insert into approval_requests
	err = database.Exec(ctx, `
		INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, context, target_tool, status, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, requestID, sessionID, 0, "tool_execution",
		fmt.Sprintf("Execute tool %q", toolName),
		"medium",
		string(paramsJSON),
		toolName,
		"pending",
		expiresAt, now)
	if err != nil {
		return "", fmt.Errorf("approval: insert request: %w", err)
	}

	// Pause the session so the harness stops iterating
	err = database.Exec(ctx, `
		UPDATE sessions SET status = 'paused', heartbeat_at = $1
		WHERE id = $2 AND status NOT IN ('completed', 'failed')
	`, now, sessionID)
	if err != nil {
		return "", fmt.Errorf("approval: pause session: %w", err)
	}

	return requestID, nil
}
