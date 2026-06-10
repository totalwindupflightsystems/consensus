// Package harness implements the agent iteration loop (SPEC-008, SPEC-020).
//
// The harness is a long-running Go process that:
//  1. Polls for ready tasks via heartbeat
//  2. Reads active context from the database
//  3. Formats Markdown for the LLM
//  4. Parses JSON responses
//  5. Executes SQL in transactions
//  6. Manages interactive multi-turn planning (SPEC-020)
//
// axiom:trace work_item=repo-bootstrap-01 spec=specs/008-harness.md plan=phase-1/task-1/step-3
package harness

// StartHeartbeat begins the task polling loop.
// Declared in harness.go.
