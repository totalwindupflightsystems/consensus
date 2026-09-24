// Package api: shared helpers used across endpoint handler files.
//
// axiom:trace work_item=spec-014-hardening-01 spec=specs/015-api-and-mcp.md plan=phase-1/task-1-3
package api

import (
	"encoding/json"
	"strings"
)

// parseJSONRaw attempts to parse a JSON string, returning the raw string if it fails.
func parseJSONRaw(s string) any {
	if s == "" || s == "{}" {
		return nil
	}
	var result any
	trimmed := strings.TrimSpace(s)
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return s
	}
	return result
}

// strPtr returns a pointer to s, or nil if s is empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
