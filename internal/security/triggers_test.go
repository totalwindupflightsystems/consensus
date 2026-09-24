// Package security: tests for Go-level trigger enforcement (SPEC-003 §5).
//
// Includes the prerequisite enforcement test: create task with prerequisites →
// attempt to claim before prerequisite done → must fail.
//
// axiom:trace work_item=WI-006 spec=specs/003-database.md plan=phase-2/task-2,phase-5/task-1 impl=internal/security/triggers_test.go
package security

import (
	"context"
	"errors"
	"testing"
)

// ============================================================================
// Task Transition Tests (§5.1)
// ============================================================================

func TestValidTaskTransition(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want bool
	}{
		// Valid transitions
		{TaskStatusPending, TaskStatusClaimed, true},
		{TaskStatusPending, TaskStatusCancelled, true},
		{TaskStatusPending, TaskStatusFailed, true},
		{TaskStatusClaimed, TaskStatusInProgress, true},
		{TaskStatusClaimed, TaskStatusCancelled, true},
		{TaskStatusClaimed, TaskStatusFailed, true},
		{TaskStatusInProgress, TaskStatusReviewed, true},
		{TaskStatusInProgress, TaskStatusFailed, true},
		{TaskStatusInProgress, TaskStatusCancelled, true},
		{TaskStatusInProgress, TaskStatusClaimed, true},
		{TaskStatusReviewed, TaskStatusPublished, true},
		{TaskStatusReviewed, TaskStatusFailed, true},
		{TaskStatusReviewed, TaskStatusCancelled, true},

		// Same status
		{TaskStatusPending, TaskStatusPending, true},
		{TaskStatusPublished, TaskStatusPublished, true},

		// Invalid transitions
		{TaskStatusPending, TaskStatusInProgress, false}, // must claim first
		{TaskStatusPending, TaskStatusPublished, false},  // skip reviewed
		{TaskStatusPending, TaskStatusReviewed, false},
		{TaskStatusClaimed, TaskStatusPublished, false},    // skip in_progress + reviewed
		{TaskStatusInProgress, TaskStatusPublished, false}, // skip reviewed
		{TaskStatusPublished, TaskStatusPending, false},    // terminal — no revert
		{TaskStatusPublished, TaskStatusFailed, false},     // terminal
		{TaskStatusFailed, TaskStatusPending, false},       // terminal
		{TaskStatusCancelled, TaskStatusPending, false},    // terminal
		{"unknown", TaskStatusPending, false},
	}

	for _, tt := range tests {
		got := ValidTaskTransition(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("ValidTaskTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestEnforceTaskTransition(t *testing.T) {
	agentA := "agent-a-uuid"
	agentB := "agent-b-uuid"
	emptyStr := ""

	tests := []struct {
		name             string
		oldStatus        string
		newStatus        string
		oldLockedBy      *string
		newLockedBy      *string
		expectError      bool
		expectErrorMatch string // substring match
	}{
		{
			name:        "pending to claimed is valid",
			oldStatus:   TaskStatusPending,
			newStatus:   TaskStatusClaimed,
			expectError: false,
		},
		{
			name:        "pending to in_progress without claiming is blocked",
			oldStatus:   TaskStatusPending,
			newStatus:   TaskStatusInProgress,
			expectError: true,
		},
		{
			name:        "pending to published directly is blocked",
			oldStatus:   TaskStatusPending,
			newStatus:   TaskStatusPublished,
			expectError: true,
		},
		{
			name:             "in_progress to published without review is blocked",
			oldStatus:        TaskStatusInProgress,
			newStatus:        TaskStatusPublished,
			expectError:      true,
			expectErrorMatch: "must be reviewed",
		},
		{
			name:        "reviewed to published is valid",
			oldStatus:   TaskStatusReviewed,
			newStatus:   TaskStatusPublished,
			expectError: false,
		},
		{
			name:             "published to pending is blocked (terminal)",
			oldStatus:        TaskStatusPublished,
			newStatus:        TaskStatusPending,
			expectError:      true,
			expectErrorMatch: "terminal",
		},
		{
			name:             "claiming already-locked task is blocked",
			oldStatus:        TaskStatusPending,
			newStatus:        TaskStatusClaimed,
			oldLockedBy:      &agentA,
			newLockedBy:      &agentB,
			expectError:      true,
			expectErrorMatch: "already locked",
		},
		{
			name:        "same agent re-locking is valid",
			oldStatus:   TaskStatusPending,
			newStatus:   TaskStatusClaimed,
			oldLockedBy: &agentA,
			newLockedBy: &agentA,
			expectError: false,
		},
		{
			name:        "no previous lock is valid",
			oldStatus:   TaskStatusPending,
			newStatus:   TaskStatusClaimed,
			oldLockedBy: nil,
			newLockedBy: &agentA,
			expectError: false,
		},
		{
			name:        "empty locked_by is valid (null to null)",
			oldStatus:   TaskStatusPending,
			newStatus:   TaskStatusClaimed,
			oldLockedBy: &emptyStr,
			newLockedBy: &agentA,
			expectError: false,
		},
		{
			name:        "same status is valid (no-op)",
			oldStatus:   TaskStatusPublished,
			newStatus:   TaskStatusPublished,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EnforceTaskTransition(tt.oldStatus, tt.newStatus, tt.oldLockedBy, tt.newLockedBy)
			if tt.expectError {
				if err == nil {
					t.Errorf("EnforceTaskTransition expected error, got nil")
					return
				}
				// Verify it's a TaskTransitionError
				var tte *TaskTransitionError
				if !errors.As(err, &tte) {
					t.Errorf("EnforceTaskTransition returned wrong error type: %T", err)
				}
				if tt.expectErrorMatch != "" {
					if !contains(err.Error(), tt.expectErrorMatch) {
						t.Errorf("EnforceTaskTransition error %q should contain %q", err.Error(), tt.expectErrorMatch)
					}
				}
			} else {
				if err != nil {
					t.Errorf("EnforceTaskTransition unexpected error: %v", err)
				}
			}
		})
	}
}

// ============================================================================
// Prerequisite Tests (§5.2) — Core Acceptance Criterion
// ============================================================================

// mockPrerequisiteChecker implements PrerequisiteChecker for testing.
type mockPrerequisiteChecker struct {
	unpublished map[string]bool // set of prerequisite IDs that are not published
}

func (m *mockPrerequisiteChecker) CheckPrerequisites(ctx context.Context, ids []string) (int, error) {
	count := 0
	for _, id := range ids {
		if m.unpublished[id] {
			count++
		}
	}
	return count, nil
}

func TestEnforceTaskPrerequisites_NoPrereqs(t *testing.T) {
	ctx := context.Background()
	checker := &mockPrerequisiteChecker{unpublished: make(map[string]bool)}

	// Tasks with no prerequisites should always pass
	err := EnforceTaskPrerequisites(ctx, checker, "task-1", nil, TaskStatusPending, TaskStatusClaimed)
	if err != nil {
		t.Errorf("EnforceTaskPrerequisites with no prereqs: unexpected error: %v", err)
	}
}

func TestEnforceTaskPrerequisites_AllMet(t *testing.T) {
	ctx := context.Background()
	checker := &mockPrerequisiteChecker{unpublished: map[string]bool{}}

	// All prerequisites are published
	err := EnforceTaskPrerequisites(ctx, checker, "task-2", []string{"prereq-1", "prereq-2"}, TaskStatusPending, TaskStatusClaimed)
	if err != nil {
		t.Errorf("EnforceTaskPrerequisites with all prereqs met: unexpected error: %v", err)
	}
}

func TestEnforceTaskPrerequisites_UnmetBlocked(t *testing.T) {
	ctx := context.Background()
	checker := &mockPrerequisiteChecker{
		unpublished: map[string]bool{
			"prereq-1": true,
			"prereq-2": true,
		},
	}

	// Two prerequisites are not published — should be blocked
	err := EnforceTaskPrerequisites(ctx, checker, "task-3", []string{"prereq-1", "prereq-2"}, TaskStatusPending, TaskStatusClaimed)
	if err == nil {
		t.Errorf("EnforceTaskPrerequisites with unmet prereqs: expected error, got nil")
		return
	}

	var pe *PrerequisiteError
	if !errors.As(err, &pe) {
		t.Errorf("EnforceTaskPrerequisites returned wrong error type: %T", err)
	}
	if pe.UnpublishedCount != 2 {
		t.Errorf("EnforceTaskPrerequisites UnpublishedCount = %d, want 2", pe.UnpublishedCount)
	}
	if pe.TaskID != "task-3" {
		t.Errorf("EnforceTaskPrerequisites TaskID = %s, want task-3", pe.TaskID)
	}
}

func TestEnforceTaskPrerequisites_PartialBlock(t *testing.T) {
	ctx := context.Background()
	checker := &mockPrerequisiteChecker{
		unpublished: map[string]bool{
			"prereq-2": true, // only one is unmet
		},
	}

	err := EnforceTaskPrerequisites(ctx, checker, "task-4", []string{"prereq-1", "prereq-2"}, TaskStatusPending, TaskStatusClaimed)
	if err == nil {
		t.Errorf("EnforceTaskPrerequisites with partial unmet: expected error, got nil")
		return
	}

	var pe *PrerequisiteError
	if !errors.As(err, &pe) {
		t.Errorf("EnforceTaskPrerequisites returned wrong error type: %T", err)
	}
	if pe.UnpublishedCount != 1 {
		t.Errorf("EnforceTaskPrerequisites UnpublishedCount = %d, want 1", pe.UnpublishedCount)
	}
}

func TestEnforceTaskPrerequisites_OnlyBlocksOnClaiming(t *testing.T) {
	ctx := context.Background()
	checker := &mockPrerequisiteChecker{
		unpublished: map[string]bool{"prereq-1": true},
	}

	// Should NOT block when transitioning from claimed to in_progress
	err := EnforceTaskPrerequisites(ctx, checker, "task-5", []string{"prereq-1"}, TaskStatusClaimed, TaskStatusInProgress)
	if err != nil {
		t.Errorf("EnforceTaskPrerequisites should only block on pending→claimed, not during progress: %v", err)
	}

	// Should NOT block when transitioning from reviewed to published
	err = EnforceTaskPrerequisites(ctx, checker, "task-6", []string{"prereq-1"}, TaskStatusReviewed, TaskStatusPublished)
	if err != nil {
		t.Errorf("EnforceTaskPrerequisites should not block published transition: %v", err)
	}
}

func TestEnforceTaskPrerequisites_EmptyIDsList(t *testing.T) {
	ctx := context.Background()
	checker := &mockPrerequisiteChecker{unpublished: map[string]bool{}}

	// Empty prerequisite list should pass
	err := EnforceTaskPrerequisites(ctx, checker, "task-7", []string{}, TaskStatusPending, TaskStatusClaimed)
	if err != nil {
		t.Errorf("EnforceTaskPrerequisites with empty prereqs list: unexpected error: %v", err)
	}
}

// ============================================================================
// Prerequisite Enforcement Integration Test (§5.2) — Full Lifecycle
// ============================================================================

// TestPrerequisiteEnforcement_FullFlow simulates the real-world scenario:
//  1. Create a task with prerequisite_ids referencing another task
//  2. Attempt to claim the dependent task before prerequisite is published
//  3. Verify the claim is blocked with appropriate error
//  4. Publish the prerequisite
//  5. Verify the dependent task can now be claimed
func TestPrerequisiteEnforcement_FullFlow(t *testing.T) {
	ctx := context.Background()

	// Step 1: Set up a checker where "prereq-A" is NOT yet published
	unpublished := map[string]bool{"prereq-A": true}
	checker := &mockPrerequisiteChecker{unpublished: unpublished}

	// Step 2: Task "dependent-B" requires "prereq-A" — try to claim it
	err := EnforceTaskPrerequisites(ctx, checker, "dependent-B", []string{"prereq-A"}, TaskStatusPending, TaskStatusClaimed)

	// Step 3: Must be blocked with a PrerequisiteError
	if err == nil {
		t.Fatal("PrerequisiteEnforcement: expected error when claiming task with unpublished prereq, got nil")
	}

	var pe *PrerequisiteError
	if !errors.As(err, &pe) {
		t.Fatalf("PrerequisiteEnforcement: returned wrong error type: %T (want *PrerequisiteError)", err)
	}

	// Verify the error fields match the scenario
	if pe.TaskID != "dependent-B" {
		t.Errorf("PrerequisiteEnforcement: TaskID = %q, want %q", pe.TaskID, "dependent-B")
	}
	if pe.UnpublishedCount != 1 {
		t.Errorf("PrerequisiteEnforcement: UnpublishedCount = %d, want 1", pe.UnpublishedCount)
	}
	if len(pe.PrerequisiteIDs) != 1 || pe.PrerequisiteIDs[0] != "prereq-A" {
		t.Errorf("PrerequisiteEnforcement: PrerequisiteIDs = %v, want [prereq-A]", pe.PrerequisiteIDs)
	}

	// Verify error message mentions the task ID and count
	msg := err.Error()
	if !contains(msg, "dependent-B") {
		t.Errorf("PrerequisiteEnforcement: error message %q should contain task ID", msg)
	}
	if !contains(msg, "1 prerequisite") {
		t.Errorf("PrerequisiteEnforcement: error message %q should mention count", msg)
	}

	// Step 4: Publish the prerequisite (remove from unpublished set)
	delete(unpublished, "prereq-A")

	// Step 5: Now claiming should succeed
	err = EnforceTaskPrerequisites(ctx, checker, "dependent-B", []string{"prereq-A"}, TaskStatusPending, TaskStatusClaimed)
	if err != nil {
		t.Errorf("PrerequisiteEnforcement: claiming after prerequisite published should succeed, got: %v", err)
	}
}

// ============================================================================
// Rate Limit Tests (§5.5)
// ============================================================================

// mockRateLimitChecker implements RateLimitChecker for testing.
type mockRateLimitChecker struct {
	limits map[string]int // tool name → rate_limit_per_min
	recent map[string]int // key: "sessionID/toolName" → count
}

func (m *mockRateLimitChecker) GetToolRateLimit(ctx context.Context, toolName string) (int, error) {
	if limit, ok := m.limits[toolName]; ok {
		return limit, nil
	}
	return 0, nil
}

func (m *mockRateLimitChecker) CountRecentToolRequests(ctx context.Context, sessionID, toolName string) (int, error) {
	key := sessionID + "/" + toolName
	return m.recent[key], nil
}

func TestEnforceToolRateLimit_NoLimit(t *testing.T) {
	ctx := context.Background()
	checker := &mockRateLimitChecker{
		limits: map[string]int{},
		recent: map[string]int{},
	}

	err := EnforceToolRateLimit(ctx, checker, "session-1", "read_file")
	if err != nil {
		t.Errorf("EnforceToolRateLimit with no limit: unexpected error: %v", err)
	}
}

func TestEnforceToolRateLimit_WithinLimit(t *testing.T) {
	ctx := context.Background()
	checker := &mockRateLimitChecker{
		limits: map[string]int{"read_file": 10},
		recent: map[string]int{"session-1/read_file": 5},
	}

	err := EnforceToolRateLimit(ctx, checker, "session-1", "read_file")
	if err != nil {
		t.Errorf("EnforceToolRateLimit within limit: unexpected error: %v", err)
	}
}

func TestEnforceToolRateLimit_AtLimit(t *testing.T) {
	ctx := context.Background()
	checker := &mockRateLimitChecker{
		limits: map[string]int{"read_file": 10},
		recent: map[string]int{"session-1/read_file": 9},
	}

	// 9 is still within the limit of 10 (at limit means >= 10 is blocked)
	err := EnforceToolRateLimit(ctx, checker, "session-1", "read_file")
	if err != nil {
		t.Errorf("EnforceToolRateLimit at limit (9/10): unexpected error: %v", err)
	}
}

func TestEnforceToolRateLimit_Exceeded(t *testing.T) {
	ctx := context.Background()
	checker := &mockRateLimitChecker{
		limits: map[string]int{"api_call": 10},
		recent: map[string]int{"session-1/api_call": 10},
	}

	err := EnforceToolRateLimit(ctx, checker, "session-1", "api_call")
	if err == nil {
		t.Errorf("EnforceToolRateLimit exceeded: expected error, got nil")
		return
	}

	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Errorf("EnforceToolRateLimit returned wrong error type: %T", err)
	}
	if rle.ToolName != "api_call" {
		t.Errorf("ToolName = %s, want api_call", rle.ToolName)
	}
	if rle.MaxPerMinute != 10 {
		t.Errorf("MaxPerMinute = %d, want 10", rle.MaxPerMinute)
	}
	if rle.CurrentCount != 10 {
		t.Errorf("CurrentCount = %d, want 10", rle.CurrentCount)
	}
}

func TestEnforceToolRateLimit_DifferentTools(t *testing.T) {
	ctx := context.Background()
	checker := &mockRateLimitChecker{
		limits: map[string]int{
			"api_call":  10,
			"read_file": 5,
		},
		recent: map[string]int{
			"session-1/api_call":  10, // exceeded for api_call
			"session-1/read_file": 3,  // within limit for read_file
		},
	}

	// api_call should be blocked
	err := EnforceToolRateLimit(ctx, checker, "session-1", "api_call")
	if err == nil {
		t.Errorf("api_call should be rate limited")
	}

	// read_file should be allowed
	err = EnforceToolRateLimit(ctx, checker, "session-1", "read_file")
	if err != nil {
		t.Errorf("read_file should not be rate limited: %v", err)
	}
}

// ============================================================================
// Helper
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
