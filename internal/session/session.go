// Package session implements the session lifecycle and status state machine
// (SPEC-011 §1, SPEC-006 §Circuit Breakers).
//
// Sessions track the lifecycle of every agent instance. The status field
// is governed by a finite state machine with validated transitions. No
// agent can skip states or move backwards without permission.
//
// axiom:trace work_item=runtime-harness-01 spec=specs/011-canonical-definitions.md plan=phase-4/task-4-1/step-4-1-1 impl=internal/session/session.go
package session

import (
	"fmt"
	"time"
)

// ============================================================================
// Status Constants (SPEC-011 §1.1)
// ============================================================================

// Status represents an agent session status in the finite state machine.
type Status string

const (
	StatusBooting     Status = "booting"
	StatusIdle        Status = "idle"
	StatusPlanning    Status = "planning"
	StatusThinking    Status = "thinking"
	StatusToolExec    Status = "tool_exec"
	StatusExecuting   Status = "executing"
	StatusWaitingSub  Status = "waiting_sub"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusPaused      Status = "paused"
)

// TerminalStatuses returns the set of statuses from which no further transitions are allowed.
func TerminalStatuses() []Status {
	return []Status{StatusCompleted, StatusFailed}
}

// IsTerminal returns true if the status is a terminal (non-recoverable) state.
func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed
}

// IsActive returns true if the session is in a working (non-terminal, non-paused) state.
func (s Status) IsActive() bool {
	switch s {
	case StatusIdle, StatusThinking, StatusPlanning, StatusToolExec, StatusExecuting, StatusWaitingSub:
		return true
	default:
		return false
	}
}

// IsPausable returns true if the session can be paused from this state.
func (s Status) IsPausable() bool {
	switch s {
	case StatusIdle, StatusThinking, StatusPlanning, StatusToolExec, StatusExecuting, StatusWaitingSub:
		return true
	default:
		return false
	}
}

// ============================================================================
// State Machine (SPEC-011 §1.2)
// ============================================================================

// validTransitions maps each status to the set of statuses it can transition to.
var validTransitions = map[Status][]Status{
	StatusBooting:    {StatusIdle, StatusFailed},
	StatusIdle:       {StatusThinking, StatusPlanning, StatusPaused, StatusCompleted, StatusFailed},
	StatusPlanning:   {StatusExecuting, StatusThinking, StatusIdle, StatusPaused, StatusFailed},
	StatusThinking:   {StatusIdle, StatusToolExec, StatusWaitingSub, StatusPlanning, StatusExecuting, StatusPaused, StatusCompleted, StatusFailed},
	StatusToolExec:   {StatusThinking, StatusIdle, StatusPaused, StatusFailed},
	StatusExecuting:  {StatusIdle, StatusThinking, StatusPaused, StatusCompleted, StatusFailed},
	StatusWaitingSub: {StatusIdle, StatusThinking, StatusPaused, StatusFailed},
	StatusPaused:     {StatusIdle, StatusThinking, StatusPlanning, StatusFailed},
	StatusCompleted:  {}, // terminal
	StatusFailed:     {}, // terminal
}

// ValidTransition checks whether a transition from one status to another is allowed.
func ValidTransition(from, to Status) bool {
	if from == to {
		return true
	}
	allowed, ok := validTransitions[from]
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

// MustTransition performs a validated status transition.
// Returns an error if the transition is not allowed.
func MustTransition(from, to Status) error {
	if from == to {
		return nil
	}
	if from.IsTerminal() {
		return fmt.Errorf("session: cannot transition from terminal status %q", from)
	}
	if !ValidTransition(from, to) {
		return &TransitionError{
			From:       from,
			To:         to,
			AllowedTo:  validTransitions[from],
		}
	}
	return nil
}

// ============================================================================
// Session
// ============================================================================

// Trust level constants for agent trust classification.
const (
	TrustLevelLow    = "low"
	TrustLevelMedium = "medium"
	TrustLevelHigh   = "high"
)

// Session represents an agent instance with its lifecycle state.
type Session struct {
	ID                 string     `json:"id"`
	ParentID           *string    `json:"parent_id,omitempty"`
	AgentName          string     `json:"agent_name"`
	ModelID            string     `json:"model_id"`
	Status             Status     `json:"status"`
	TrustLevel         string     `json:"trust_level"` // low, medium, high (SPEC-008 §5.4)
	Goal               string     `json:"goal"`
	ContextBudget      int        `json:"context_budget"`
	TokensUsedIn       int64      `json:"tokens_used_in"`
	TokensUsedOut      int64      `json:"tokens_used_out"`
	Iteration          int64      `json:"iteration"`
	ProjectID          *string    `json:"project_id,omitempty"` // NULL = Global scope
	HeartbeatAt        time.Time  `json:"heartbeat_at"`
	PlanningMaxTurns   int        `json:"planning_max_turns"`
	CreatedAt          time.Time  `json:"created_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
}

// Transition applies a validated status transition to the session.
func (s *Session) Transition(newStatus Status) error {
	if err := MustTransition(s.Status, newStatus); err != nil {
		return err
	}
	s.Status = newStatus

	// Auto-complete timestamp on terminal states
	if newStatus.IsTerminal() {
		now := time.Now()
		s.CompletedAt = &now
	}
	return nil
}

// TransitionToIdle transitions the session to idle, the default rest state.
func (s *Session) TransitionToIdle() error {
	return s.Transition(StatusIdle)
}

// TransitionToThinking transitions the session to thinking to process new context.
func (s *Session) TransitionToThinking() error {
	return s.Transition(StatusThinking)
}

// TransitionToPlanning transitions to the interactive multi-turn planning state.
func (s *Session) TransitionToPlanning() error {
	return s.Transition(StatusPlanning)
}

// TransitionToToolExec transitions to the tool execution phase.
func (s *Session) TransitionToToolExec() error {
	return s.Transition(StatusToolExec)
}

// TransitionToExecuting transitions to the SQL execution phase.
func (s *Session) TransitionToExecuting() error {
	return s.Transition(StatusExecuting)
}

// TransitionToWaitingSub transitions to waiting for sub-agents.
func (s *Session) TransitionToWaitingSub() error {
	return s.Transition(StatusWaitingSub)
}

// Pause transitions to paused (requires idle/thinking/planning/tool_exec/executing/waiting_sub).
func (s *Session) Pause() error {
	return s.Transition(StatusPaused)
}

// Complete transitions to completed.
func (s *Session) Complete() error {
	return s.Transition(StatusCompleted)
}

// Fail transitions to failed.
func (s *Session) Fail() error {
	return s.Transition(StatusFailed)
}

// BumpIteration increments the iteration counter and updates the heartbeat timestamp.
func (s *Session) BumpIteration() {
	s.Iteration++
	s.HeartbeatAt = time.Now()
}

// Heartbeat updates the heartbeat timestamp without incrementing iteration.
func (s *Session) Heartbeat() {
	s.HeartbeatAt = time.Now()
}

// ============================================================================
// TransitionError
// ============================================================================

// TransitionError represents an invalid status transition attempt.
type TransitionError struct {
	From      Status
	To        Status
	AllowedTo []Status
}

func (e *TransitionError) Error() string {
	if len(e.AllowedTo) == 0 {
		return fmt.Sprintf("session: cannot transition from terminal status %q to %q", e.From, e.To)
	}
	return fmt.Sprintf("session: invalid transition %q -> %q (allowed: %v)", e.From, e.To, e.AllowedTo)
}
