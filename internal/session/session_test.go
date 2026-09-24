// Package session: tests for the session lifecycle state machine (SPEC-011 §1).
//
// axiom:trace work_item=runtime-harness-01 spec=specs/011-canonical-definitions.md plan=phase-4/task-4-1/step-4-1-1 test=internal/session/session_test.go
package session

import (
	"testing"
)

// ============================================================================
// Status Constants Tests
// ============================================================================

func TestStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   Status
		terminal bool
	}{
		{StatusBooting, false},
		{StatusIdle, false},
		{StatusThinking, false},
		{StatusPlanning, false},
		{StatusToolExec, false},
		{StatusExecuting, false},
		{StatusWaitingSub, false},
		{StatusCompleted, true},
		{StatusFailed, true},
		{StatusPaused, false},
	}

	for _, tt := range tests {
		if got := tt.status.IsTerminal(); got != tt.terminal {
			t.Errorf("%s.IsTerminal() = %v, want %v", tt.status, got, tt.terminal)
		}
	}
}

func TestStatus_IsActive(t *testing.T) {
	activeStatuses := []Status{StatusIdle, StatusThinking, StatusPlanning, StatusToolExec, StatusExecuting, StatusWaitingSub}
	inactiveStatuses := []Status{StatusBooting, StatusCompleted, StatusFailed, StatusPaused}

	for _, s := range activeStatuses {
		if !s.IsActive() {
			t.Errorf("%s should be active", s)
		}
	}
	for _, s := range inactiveStatuses {
		if s.IsActive() {
			t.Errorf("%s should not be active", s)
		}
	}
}

// ============================================================================
// Transition Validation Tests
// ============================================================================

func TestValidTransition_SuccessPath(t *testing.T) {
	// Complete happy path: booting -> idle -> thinking -> idle -> completed
	path := []Status{
		StatusBooting, StatusIdle, StatusThinking, StatusIdle, StatusCompleted,
	}
	for i := 0; i < len(path)-1; i++ {
		if !ValidTransition(path[i], path[i+1]) {
			t.Errorf("transition %s -> %s should be valid", path[i], path[i+1])
		}
	}
}

func TestValidTransition_PlanningPath(t *testing.T) {
	// Multi-turn planning path
	path := []Status{
		StatusIdle, StatusPlanning, StatusExecuting, StatusIdle, StatusCompleted,
	}
	for i := 0; i < len(path)-1; i++ {
		if !ValidTransition(path[i], path[i+1]) {
			t.Errorf("transition %s -> %s should be valid", path[i], path[i+1])
		}
	}
}

func TestValidTransition_ToolExecPath(t *testing.T) {
	// Agent with tool calls
	path := []Status{
		StatusThinking, StatusToolExec, StatusThinking, StatusIdle,
	}
	for i := 0; i < len(path)-1; i++ {
		if !ValidTransition(path[i], path[i+1]) {
			t.Errorf("transition %s -> %s should be valid", path[i], path[i+1])
		}
	}
}

func TestValidTransition_WaitingSubPath(t *testing.T) {
	// Agent spawning sub-agents
	path := []Status{
		StatusThinking, StatusWaitingSub, StatusIdle,
	}
	for i := 0; i < len(path)-1; i++ {
		if !ValidTransition(path[i], path[i+1]) {
			t.Errorf("transition %s -> %s should be valid", path[i], path[i+1])
		}
	}
}

func TestValidTransition_PauseResumePath(t *testing.T) {
	// Pause during thinking, resume back to thinking
	if !ValidTransition(StatusThinking, StatusPaused) {
		t.Error("thinking -> paused should be valid")
	}
	if !ValidTransition(StatusPaused, StatusThinking) {
		t.Error("paused -> thinking should be valid")
	}
	if !ValidTransition(StatusPaused, StatusIdle) {
		t.Error("paused -> idle should be valid")
	}
}

func TestValidTransition_ToFailed(t *testing.T) {
	// Most active states can transition to failed
	for _, s := range []Status{StatusBooting, StatusIdle, StatusThinking, StatusPlanning, StatusToolExec, StatusExecuting, StatusWaitingSub, StatusPaused} {
		if !ValidTransition(s, StatusFailed) {
			t.Errorf("%s -> failed should be valid", s)
		}
	}
}

func TestValidTransition_InvalidTransitions(t *testing.T) {
	invalid := []struct{ from, to Status }{
		// Cannot skip states
		{StatusBooting, StatusCompleted}, // must go through idle first
		{StatusIdle, StatusExecuting},    // must plan first
		{StatusBooting, StatusThinking},  // must go through idle
		// Cannot revert from terminal
		{StatusCompleted, StatusIdle},
		{StatusCompleted, StatusThinking},
		{StatusFailed, StatusIdle},
		{StatusFailed, StatusBooting},
		// Cannot go to booting after initial
		{StatusIdle, StatusBooting},
		{StatusThinking, StatusBooting},
	}

	for _, tt := range invalid {
		if ValidTransition(tt.from, tt.to) {
			t.Errorf("transition %s -> %s should be invalid", tt.from, tt.to)
		}
	}
}

func TestValidTransition_SameState(t *testing.T) {
	// Same-state transitions are always allowed (no-op)
	for _, s := range []Status{StatusIdle, StatusThinking, StatusBooting} {
		if !ValidTransition(s, s) {
			t.Errorf("same-state transition %s -> %s should be valid", s, s)
		}
	}
}

func TestValidTransition_UnknownStatus(t *testing.T) {
	if ValidTransition("nonexistent", StatusIdle) {
		t.Error("unknown status should not have valid transitions")
	}
	if ValidTransition(StatusIdle, "nonexistent") {
		t.Error("transition to unknown status should not be valid")
	}
}

// ============================================================================
// MustTransition Tests
// ============================================================================

func TestMustTransition_Valid(t *testing.T) {
	if err := MustTransition(StatusIdle, StatusThinking); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMustTransition_SameState(t *testing.T) {
	if err := MustTransition(StatusIdle, StatusIdle); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMustTransition_Invalid(t *testing.T) {
	err := MustTransition(StatusIdle, StatusExecuting)
	if err == nil {
		t.Fatal("expected error for invalid transition")
	}

	transErr, ok := err.(*TransitionError)
	if !ok {
		t.Fatalf("expected *TransitionError, got %T", err)
	}
	if transErr.From != StatusIdle {
		t.Errorf("from = %s, want idle", transErr.From)
	}
	if transErr.To != StatusExecuting {
		t.Errorf("to = %s, want executing", transErr.To)
	}
}

func TestMustTransition_TerminalFrom(t *testing.T) {
	err := MustTransition(StatusCompleted, StatusIdle)
	if err == nil {
		t.Fatal("expected error for transition from terminal")
	}
	err2 := MustTransition(StatusFailed, StatusIdle)
	if err2 == nil {
		t.Fatal("expected error for transition from terminal")
	}
}

// ============================================================================
// Session Transition Tests
// ============================================================================

func TestSession_Transition_Success(t *testing.T) {
	s := &Session{
		ID:        "sess-1",
		AgentName: "test",
		Status:    StatusBooting,
	}

	// Booting -> Idle
	if err := s.Transition(StatusIdle); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != StatusIdle {
		t.Errorf("status = %s, want idle", s.Status)
	}

	// Idle -> Thinking
	if err := s.TransitionToThinking(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != StatusThinking {
		t.Errorf("status = %s, want thinking", s.Status)
	}
}

func TestSession_Transition_Failure(t *testing.T) {
	s := &Session{
		ID:        "sess-2",
		AgentName: "test",
		Status:    StatusIdle,
	}

	// Cannot go direct to executing from idle
	if err := s.TransitionToExecuting(); err == nil {
		t.Fatal("expected error")
	}
	// Status should be unchanged
	if s.Status != StatusIdle {
		t.Errorf("status = %s, want idle", s.Status)
	}
}

func TestSession_Transition_SetsCompletedAt(t *testing.T) {
	s := &Session{
		ID:        "sess-3",
		AgentName: "test",
		Status:    StatusIdle,
	}

	if s.CompletedAt != nil {
		t.Error("completed_at should be nil initially")
	}

	if err := s.Complete(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.CompletedAt == nil {
		t.Error("completed_at should be set on terminal transition")
	}
	if s.Status != StatusCompleted {
		t.Errorf("status = %s, want completed", s.Status)
	}
}

func TestSession_Transition_NoDoubleComplete(t *testing.T) {
	s := &Session{
		ID:        "sess-4",
		AgentName: "test",
		Status:    StatusCompleted,
	}

	if err := s.Transition(StatusIdle); err == nil {
		t.Fatal("should not allow transition from completed")
	}
}

// ============================================================================
// Convenience Methods
// ============================================================================

func TestSession_Pause(t *testing.T) {
	s := &Session{ID: "s-1", Status: StatusThinking}
	if err := s.Pause(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != StatusPaused {
		t.Errorf("status = %s, want paused", s.Status)
	}
}

func TestSession_Pause_NotAllowed(t *testing.T) {
	s := &Session{ID: "s-2", Status: StatusBooting}
	if err := s.Pause(); err == nil {
		t.Fatal("should not allow pause from booting")
	}
}

func TestSession_Fail(t *testing.T) {
	s := &Session{ID: "s-3", Status: StatusThinking}
	if err := s.Fail(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != StatusFailed {
		t.Errorf("status = %s, want failed", s.Status)
	}
	if s.CompletedAt == nil {
		t.Error("completed_at should be set on fail")
	}
}

// ============================================================================
// Iteration & Heartbeat
// ============================================================================

func TestSession_BumpIteration(t *testing.T) {
	s := &Session{ID: "s-1", Iteration: 5}
	oldHeartbeat := s.HeartbeatAt

	s.BumpIteration()

	if s.Iteration != 6 {
		t.Errorf("iteration = %d, want 6", s.Iteration)
	}
	if s.HeartbeatAt.Equal(oldHeartbeat) || s.HeartbeatAt.Before(oldHeartbeat) {
		t.Error("heartbeat should be updated")
	}
}

func TestSession_Heartbeat(t *testing.T) {
	s := &Session{ID: "s-1", Iteration: 5}
	oldIter := s.Iteration
	oldHeartbeat := s.HeartbeatAt

	s.Heartbeat()

	if s.Iteration != oldIter {
		t.Errorf("heartbeat should not change iteration")
	}
	if s.HeartbeatAt.Equal(oldHeartbeat) || s.HeartbeatAt.Before(oldHeartbeat) {
		t.Error("heartbeat should be updated")
	}
}

// ============================================================================
// Terminal and Active States
// ============================================================================

func TestTerminalStatuses(t *testing.T) {
	terms := TerminalStatuses()
	if len(terms) != 2 {
		t.Errorf("expected 2 terminal statuses, got %d", len(terms))
	}
	foundCompleted := false
	foundFailed := false
	for _, ts := range terms {
		if ts == StatusCompleted {
			foundCompleted = true
		}
		if ts == StatusFailed {
			foundFailed = true
		}
	}
	if !foundCompleted || !foundFailed {
		t.Errorf("terminal statuses missing expected values")
	}
}

// ============================================================================
// TransitionError
// ============================================================================

func TestTransitionError_Message(t *testing.T) {
	err := &TransitionError{
		From:      StatusIdle,
		To:        StatusExecuting,
		AllowedTo: []Status{StatusThinking, StatusPlanning},
	}

	msg := err.Error()
	if msg == "" {
		t.Error("error message should not be empty")
	}
}

func TestTransitionError_TerminalMessage(t *testing.T) {
	err := &TransitionError{
		From: StatusCompleted,
		To:   StatusIdle,
	}

	msg := err.Error()
	if msg == "" {
		t.Error("terminal error message should not be empty")
	}
}
