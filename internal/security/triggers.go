// Package security: Go-level trigger enforcement for SQL constraint types
// (SPEC-003 §5, SPEC-005). Provides portable Go fallbacks for the SQL
// triggers defined in the spec, enabling SQLite parity.
//
// When running against PostgreSQL, these same validations can be enforced
// as SQL triggers for defense-in-depth. The Go layer always runs first
// regardless of backend.
//
// Trigger types:
//   - enforce_task_transitions (§5.1): valid status transitions
//   - enforce_prerequisites (§5.2): task dependency chain
//   - enforce_tool_rate_limit (§5.5): per-tool rate limiting
//
// axiom:trace work_item=WI-006 spec=specs/003-database.md,specs/005-security.md plan=phase-2/task-1 impl=internal/security/triggers.go
package security

import (
	"context"
	"fmt"
	"time"
)

// ============================================================================
// Task Status Constants (matches SPEC-003 §2.5)
// ============================================================================

const (
	TaskStatusPending    = "pending"
	TaskStatusClaimed    = "claimed"
	TaskStatusInProgress = "in_progress"
	TaskStatusReviewed   = "reviewed"
	TaskStatusPublished  = "published"
	TaskStatusFailed     = "failed"
	TaskStatusCancelled  = "cancelled"
)

// validTaskTransitions maps each task status to allowed next statuses.
// This mirrors the SQL CHECK constraint in SPEC-003 §2.5 and the
// enforce_task_transitions() trigger in SPEC-003 §5.1.
var validTaskTransitions = map[string][]string{
	TaskStatusPending:    {TaskStatusClaimed, TaskStatusCancelled, TaskStatusFailed},
	TaskStatusClaimed:    {TaskStatusInProgress, TaskStatusCancelled, TaskStatusFailed},
	TaskStatusInProgress: {TaskStatusReviewed, TaskStatusClaimed, TaskStatusFailed, TaskStatusCancelled},
	TaskStatusReviewed:   {TaskStatusPublished, TaskStatusFailed, TaskStatusCancelled},
	TaskStatusPublished:  {}, // terminal — no transitions allowed
	TaskStatusFailed:     {}, // terminal
	TaskStatusCancelled:  {}, // terminal
}

// ValidTaskTransition checks whether a transition from one task status to
// another is allowed per the state machine (SPEC-003 §5.1).
func ValidTaskTransition(from, to string) bool {
	if from == to {
		return true // same status is always valid (no-op)
	}
	allowed, ok := validTaskTransitions[from]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}

// ============================================================================
// Task Transition Enforcement (§5.1)
// ============================================================================

// TaskTransitionError is returned when a task status transition is invalid.
type TaskTransitionError struct {
	From    string
	To      string
	Details string
}

func (e *TaskTransitionError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("task transition invalid: %s -> %s (%s)", e.From, e.To, e.Details)
	}
	return fmt.Sprintf("task transition invalid: %s -> %s", e.From, e.To)
}

// EnforceTaskTransition checks whether a task status transition is valid.
//
// Rules (SPEC-003 §5.1):
//  1. Same-status transitions are always allowed (no-op)
//  2. Cannot go from pending directly to in_progress without claiming (must go through 'claimed')
//  3. Cannot skip REVIEWED → go straight to PUBLISHED
//  4. Cannot go backwards from published (terminal)
//  5. Cannot claim a task already locked by another agent
//
// Returns nil if the transition is allowed, or a TaskTransitionError if blocked.
func EnforceTaskTransition(oldStatus, newStatus string, oldLockedByAgent, newLockedByAgent *string) error {
	// Rule 1: Same-status is always a valid no-op
	if oldStatus == newStatus {
		return nil
	}

	// Rule 3: cannot skip reviewed → published (check before general valid transitions)
	if newStatus == TaskStatusPublished && oldStatus != TaskStatusReviewed {
		return &TaskTransitionError{
			From:    oldStatus,
			To:      newStatus,
			Details: fmt.Sprintf("task must be reviewed before publishing (current: %s)", oldStatus),
		}
	}

	// Validate the raw transition
	if !ValidTaskTransition(oldStatus, newStatus) {
		// Provide more specific error messages
		if oldStatus == TaskStatusPublished {
			return &TaskTransitionError{
				From:    oldStatus,
				To:      newStatus,
				Details: "published tasks are terminal — no further transitions allowed",
			}
		}
		if oldStatus == TaskStatusPending && newStatus == TaskStatusInProgress {
			return &TaskTransitionError{
				From:    oldStatus,
				To:      newStatus,
				Details: "task must be claimed before moving to in_progress (use 'claimed' status)",
			}
		}
		return &TaskTransitionError{
			From: oldStatus,
			To:   newStatus,
		}
	}

	// Rule 5: cannot claim a task already locked by another agent
	if newLockedByAgent != nil && oldLockedByAgent != nil &&
		*newLockedByAgent != *oldLockedByAgent &&
		*oldLockedByAgent != "" {
		return &TaskTransitionError{
			From:    oldStatus,
			To:      newStatus,
			Details: fmt.Sprintf("task already locked by agent %s", *oldLockedByAgent),
		}
	}

	return nil
}

// ============================================================================
// Prerequisite Enforcement (§5.2)
// ============================================================================

// PrerequisiteError is returned when a task's prerequisites are not met.
type PrerequisiteError struct {
	TaskID      string
	PrerequisiteIDs []string
	UnpublishedCount int
}

func (e *PrerequisiteError) Error() string {
	return fmt.Sprintf("task %s: %d prerequisite(s) not yet published (requires %d completed)",
		e.TaskID, e.UnpublishedCount, len(e.PrerequisiteIDs))
}

// PrerequisiteChecker provides the database query interface needed for
// prerequisite validation.
type PrerequisiteChecker interface {
	// CheckPrerequisites queries the database for the status of prerequisite tasks.
	// Returns the number of prerequisites whose status is NOT 'published'.
	// If taskID is empty, the checker operates on the given IDs directly.
	CheckPrerequisites(ctx context.Context, prerequisiteIDs []string) (int, error)
}

// EnforceTaskPrerequisites checks that all prerequisites for a task are
// completed (published) before the task can be claimed or progressed.
//
// Rules (SPEC-003 §5.2):
//   - When transitioning to 'claimed' or 'in_progress' from 'pending',
//     all prerequisite_ids must have status = 'published'
//   - If no prerequisites, the check passes
//
// Returns nil if all prerequisites are met, or a PrerequisiteError if unmet.
func EnforceTaskPrerequisites(ctx context.Context, checker PrerequisiteChecker, taskID string, prerequisiteIDs []string, oldStatus, newStatus string) error {
	// Only enforce on specific transitions
	if oldStatus != TaskStatusPending {
		return nil // only check when moving from pending
	}
	if newStatus != TaskStatusClaimed && newStatus != TaskStatusInProgress {
		return nil // only check when claiming or starting work
	}

	if len(prerequisiteIDs) == 0 {
		return nil // no prerequisites
	}

	unmetCount, err := checker.CheckPrerequisites(ctx, prerequisiteIDs)
	if err != nil {
		return fmt.Errorf("prerequisite check failed: %w", err)
	}

	if unmetCount > 0 {
		return &PrerequisiteError{
			TaskID:           taskID,
			PrerequisiteIDs:  prerequisiteIDs,
			UnpublishedCount: unmetCount,
		}
	}

	return nil
}

// ============================================================================
// Rate Limiting (§5.5)
// ============================================================================

// RateLimitError is returned when a tool's rate limit is exceeded.
type RateLimitError struct {
	ToolName     string
	MaxPerMinute int
	CurrentCount int
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded for tool %q: %d per minute (current: %d)",
		e.ToolName, e.MaxPerMinute, e.CurrentCount)
}

// RateLimitChecker provides the database query interface for rate limit
// enforcement.
type RateLimitChecker interface {
	// GetToolRateLimit returns the rate_limit_per_min for a tool from tools_registry.
	// Returns 0 if no limit is set.
	GetToolRateLimit(ctx context.Context, toolName string) (int, error)

	// CountRecentToolRequests counts how many times a tool was called by a
	// session in the last minute.
	CountRecentToolRequests(ctx context.Context, sessionID, toolName string) (int, error)
}

// DefaultRateLimitWindow is the time window for rate limit counting.
const DefaultRateLimitWindow = 1 * time.Minute

// EnforceToolRateLimit checks whether a tool request would exceed the
// configured rate limit.
//
// Rules (SPEC-003 §5.5):
//   - Reads rate_limit_per_min from tools_registry for the given tool
//   - Counts recent tool_requests for the same session + tool in last minute
//   - If count >= limit, returns RateLimitError
//   - If no rate limit configured (rate_limit_per_min IS NULL), pass through
//
// Returns nil if the request is within limits, or a RateLimitError if exceeded.
func EnforceToolRateLimit(ctx context.Context, checker RateLimitChecker, sessionID, toolName string) error {
	maxPerMin, err := checker.GetToolRateLimit(ctx, toolName)
	if err != nil {
		return fmt.Errorf("rate limit check: read tool rate: %w", err)
	}

	if maxPerMin <= 0 {
		return nil // no rate limit configured
	}

	recentCount, err := checker.CountRecentToolRequests(ctx, sessionID, toolName)
	if err != nil {
		return fmt.Errorf("rate limit check: count requests: %w", err)
	}

	if recentCount >= maxPerMin {
		return &RateLimitError{
			ToolName:     toolName,
			MaxPerMinute: maxPerMin,
			CurrentCount: recentCount,
		}
	}

	return nil
}
