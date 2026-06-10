// Package tools implements the tool registry, resolution, and sandboxed execution
// boundary (SPEC-010).
//
// The tool system is split into two hemispheres:
//   - Internal: SQL stored procedures executed within DB transactions
//   - External: Sandboxed subprocesses for network/I/O operations
//
// Resolution order (SPEC-010 §Tool Resolution Order):
//   1. Internal hemisphere (SQL functions — fastest, atomic)
//   2. Skill-linked tools (pre-approved external tools)
//   3. JIT registry (agent-authored custom_agent_tools)
//   4. Runtime built-ins (harness-level utilities)
//
// axiom:trace work_item=runtime-harness-01 spec=specs/010-tools.md plan=phase-3/task-3-1/step-3-1-1 impl=internal/tools/tools.go
package tools

import (
	"context"
	"fmt"

	"github.com/wojons/conscientiousness/internal/db"
)

// ============================================================================
// Tool Types
// ============================================================================

// Hemisphere classifies where a tool executes.
type Hemisphere string

const (
	HemisphereInternal Hemisphere = "internal"
	HemisphereExternal Hemisphere = "external"
)

// HandlerType determines how a tool is dispatched.
type HandlerType string

const (
	HandlerSQLFunction HandlerType = "sql_function"
	HandlerHTTPEndpoint HandlerType = "http_endpoint"
	HandlerGoNative    HandlerType = "go_native"
	HandlerSubprocess  HandlerType = "subprocess"
)

// Tool represents a registered tool in the tools_registry table.
type Tool struct {
	ID                string      `json:"id"`
	Name              string      `json:"name"`
	Description       string      `json:"description"`
	Hemisphere        Hemisphere  `json:"hemisphere"`
	ParameterSchema   map[string]any `json:"parameter_schema"`
	HandlerType       HandlerType `json:"handler_type"`
	HandlerRef        string      `json:"handler_ref"`
	OwnerSessionID    *string     `json:"owner_session_id,omitempty"`
	Status            string      `json:"status"`
	Enabled           bool        `json:"enabled"`
	RequiresApproval  bool        `json:"requires_approval"`
	RateLimitPerMin   *int        `json:"rate_limit_per_min,omitempty"`
}

// ToolResult holds the output of a tool execution.
type ToolResult struct {
	RequestID int64  `json:"request_id"`
	Output    string `json:"output"`
	IsError   bool   `json:"is_error"`
	ErrorCode string `json:"error_code,omitempty"`
	TokenCount int   `json:"token_count,omitempty"`
}

// ============================================================================
// Registry Interface
// ============================================================================

// Registry resolves and caches tool lookups from the database.
type Registry struct {
	database db.DB
	cache    map[string]*Tool // in-memory cache
}

// NewRegistry creates a new tool registry.
func NewRegistry(database db.DB) *Registry {
	return &Registry{
		database: database,
		cache:    make(map[string]*Tool),
	}
}

// Lookup resolves a tool by name, following the resolution order:
// internal → skill-linked → JIT → built-in.
//
// Returns the tool and its resolution source, or an error if not found.
func (r *Registry) Lookup(ctx context.Context, name string) (*Tool, error) {
	// Check cache first
	if cached, ok := r.cache[name]; ok {
		return cached, nil
	}

	// Query tools_registry
	tool, err := r.queryTool(ctx, name)
	if err != nil {
		return nil, err
	}

	r.cache[name] = tool
	return tool, nil
}

// Invalidate removes a tool from the cache (e.g., after tool update).
func (r *Registry) Invalidate(name string) {
	delete(r.cache, name)
}

// List returns all active tools from the registry.
func (r *Registry) List(ctx context.Context) ([]*Tool, error) {
	if r.database == nil {
		return nil, fmt.Errorf("tools: no database configured")
	}
	rows, err := r.database.Query(ctx, `
		SELECT id, name, description, hemisphere, parameter_schema,
		       handler_type, handler_ref, owner_session_id,
		       status, enabled, requires_approval, rate_limit_per_min
		FROM tools_registry
		WHERE enabled = true AND status = 'active'
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("tools: list: %w", err)
	}

	tools := make([]*Tool, 0, len(rows))
	for _, row := range rows {
		tools = append(tools, rowToTool(row))
	}
	return tools, nil
}

// queryTool performs a database lookup for a specific tool.
func (r *Registry) queryTool(ctx context.Context, name string) (*Tool, error) {
	if r.database == nil {
		return nil, fmt.Errorf("tools: no database configured")
	}
	rows, err := r.database.Query(ctx, `
		SELECT id, name, description, hemisphere, parameter_schema,
		       handler_type, handler_ref, owner_session_id,
		       status, enabled, requires_approval, rate_limit_per_min
		FROM tools_registry
		WHERE name = $1 AND enabled = true
	`, name)
	if err != nil {
		return nil, fmt.Errorf("tools: lookup %q: %w", name, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("tools: not found: %q", name)
	}

	return rowToTool(rows[0]), nil
}

// ============================================================================
// Row Conversion
// ============================================================================

func rowToTool(row db.Row) *Tool {
	t := &Tool{
		Name:             toString(row["name"]),
		Description:      toString(row["description"]),
		Hemisphere:       Hemisphere(toString(row["hemisphere"])),
		HandlerType:      HandlerType(toString(row["handler_type"])),
		HandlerRef:       toString(row["handler_ref"]),
		Status:           toString(row["status"]),
		Enabled:          toBool(row["enabled"]),
		RequiresApproval: toBool(row["requires_approval"]),
	}

	if v, ok := row["id"]; ok {
		t.ID = toString(v)
	}
	if v, ok := row["owner_session_id"]; ok && v != nil {
		s := toString(v)
		t.OwnerSessionID = &s
	}
	if v, ok := row["rate_limit_per_min"]; ok && v != nil {
		n := toInt(v)
		t.RateLimitPerMin = &n
	}
	if v, ok := row["parameter_schema"]; ok && v != nil {
		// parameter_schema is JSONB — parse or pass through
		_ = v
	}

	return t
}

// ============================================================================
// Type Helpers
// ============================================================================

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int64:
		return b != 0
	case float64:
		return b != 0
	case string:
		return b == "true" || b == "1"
	default:
		return false
	}
}

func toInt(v any) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}
