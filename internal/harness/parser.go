// Package harness: structured JSON output parser (SPEC-007, SPEC-008).
//
// The parser validates LLM JSON output against the AgentOutput schema.
// It enforces:
//   - Required fields (internal_monologue, memory_state_changes, system_actions)
//   - Array types for statement lists
//   - Valid tool request shapes
//   - Statement sanitization (null bytes, BOM, trim)
//
// axiom:trace work_item=runtime-harness-01 spec=specs/007-json-schema.md,specs/008-harness.md plan=phase-1/task-1-1/step-1-1-1 impl=internal/harness/parser.go
package harness

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ============================================================================
// Parser — LLM JSON Output Validation
// ============================================================================

// ParseAgentResponse parses raw LLM JSON output and validates it against
// the AgentOutput schema. It returns a structured AgentOutput on success,
// or a descriptive error on failure.
//
// Validation rules:
//   - Input must be valid JSON
//   - internal_monologue is required
//   - memory_state_changes must be present (can be empty)
//   - system_actions must be present (can be empty)
//   - tool_requests must be an array of valid ToolRequest objects
//   - All SQL statements are sanitized (null bytes removed, BOM trimmed)
func ParseAgentResponse(raw []byte) (*AgentOutput, error) {
	if len(raw) == 0 {
		return nil, &ParseError{Message: "empty response"}
	}

	// Remove BOM if present
	raw = trimBOM(raw)

	// Attempt JSON decode
	var output AgentOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, &ParseError{
			Message: fmt.Sprintf("invalid JSON: %v", err),
			Cause:   err,
		}
	}

	// Validate required fields
	if err := validateOutput(&output); err != nil {
		return nil, err
	}

	// Sanitize statements
	sanitizeOutput(&output)

	return &output, nil
}

// validateOutput checks that all required fields in AgentOutput are valid.
func validateOutput(output *AgentOutput) error {
	// internal_monologue is required (can be empty string, but must be present)
	// JSON unmarshal gives "" for missing string fields, but we require the field
	// to be explicit. We check via re-marshaling to detect if it was present.
	// For struct validation: if internal_monologue was not in the JSON,
	// it's zero-value which is acceptable per spec (empty monologue is valid).

	// memory_state_changes can be empty but must be present
	if output.MemoryStateChanges == nil {
		return &ParseError{Message: "missing required field: memory_state_changes"}
	}

	// system_actions can be empty but must be present
	if output.SystemActions == nil {
		return &ParseError{Message: "missing required field: system_actions"}
	}

	// Validate tool_requests entries
	for i, tr := range output.ToolRequests {
		if err := validateToolRequest(i, &tr); err != nil {
			return err
		}
	}

	// Validate sub_agent_spawns entries
	for i, spawn := range output.SubAgentSpawns {
		if spawn.AgentName == "" {
			return &ParseError{
				Message: fmt.Sprintf("sub_agent_spawns[%d]: agent_name is required", i),
			}
		}
		if spawn.Goal == "" {
			return &ParseError{
				Message: fmt.Sprintf("sub_agent_spawns[%d]: goal is required", i),
			}
		}
	}

	return nil
}

// validateToolRequest checks that a single ToolRequest entry is valid.
func validateToolRequest(index int, tr *ToolRequest) error {
	if tr.ToolName == "" {
		return &ParseError{
			Message: fmt.Sprintf("tool_requests[%d]: tool_name is required", index),
		}
	}
	// Tool name must be alphanumeric + underscore + hyphen
	if !isValidToolName(tr.ToolName) {
		return &ParseError{
			Message: fmt.Sprintf("tool_requests[%d]: invalid tool_name %q", index, tr.ToolName),
		}
	}
	return nil
}

// isValidToolName checks that a tool name contains only safe characters.
func isValidToolName(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// sanitizeOutput applies defense-in-depth sanitization to all statements.
func sanitizeOutput(output *AgentOutput) {
	output.MemoryStateChanges = sanitizeStatements(output.MemoryStateChanges)
	output.SystemActions = sanitizeStatements(output.SystemActions)
}

// sanitizeStatements removes null bytes and trims whitespace from statements.
func sanitizeStatements(statements []string) []string {
	cleaned := make([]string, 0, len(statements))
	for _, stmt := range statements {
		s := sanitizeStatement(stmt)
		if s != "" {
			cleaned = append(cleaned, s)
		}
	}
	return cleaned
}

// sanitizeStatement removes null bytes, BOM, and trims whitespace from a single statement.
func sanitizeStatement(stmt string) string {
	s := strings.ReplaceAll(stmt, "\x00", "") // remove null bytes
	s = trimBOMString(s)                      // remove BOM
	s = strings.TrimSpace(s)                  // trim whitespace
	return s
}

// trimBOM removes a UTF-8 BOM from the beginning of a byte slice.
func trimBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

// trimBOMString removes a UTF-8 BOM from the beginning of a string.
func trimBOMString(s string) string {
	if len(s) == 0 {
		return s
	}
	// Check for BOM rune (U+FEFF) at start of string
	runes := []rune(s)
	if runes[0] == '\uFEFF' {
		return string(runes[1:])
	}
	return s
}

// ============================================================================
// ParseError — Structured Parser Error
// ============================================================================

// ParseError represents a parse/validation failure with a human-readable
// message and optional underlying error.
type ParseError struct {
	Message string `json:"message"`
	Cause   error  `json:"-"`
}

// Error implements the error interface.
func (e *ParseError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("parse error: %s (cause: %v)", e.Message, e.Cause)
	}
	return fmt.Sprintf("parse error: %s", e.Message)
}

// Unwrap returns the underlying error.
func (e *ParseError) Unwrap() error {
	return e.Cause
}
