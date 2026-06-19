// axiom:trace work_item=interfaces-api-cli-01,full-platform-audit spec=specs/015-api-and-mcp.md plan=phase-3 impl=internal/mcp/auth.go
// axiom:trace work_item=make-consensus-fully-operational-end-to spec=specs/015-api-and-mcp.md plan=phase-3/task-3-2/step-3-2-1 impl=internal/mcp/auth.go
package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// MCP Authentication (SPEC-015 §5.3)
// ============================================================================

// validateAuth extracts the Bearer token from an initialize request's
// _meta.authorization field and validates it against the api_keys table.
func (s *Server) validateAuth(req *JSONRPCRequest, sess *mcpSession) *JSONRPCErrObj {
	var init struct {
		Meta map[string]any `json:"_meta"`
	}
	if err := json.Unmarshal(req.Params, &init); err != nil {
		return &JSONRPCErrObj{Code: -32000, Message: "Authentication required", Data: "missing _meta.authorization"}
	}

	if init.Meta == nil {
		return &JSONRPCErrObj{Code: -32000, Message: "Authentication required", Data: "missing _meta.authorization"}
	}

	raw, ok := init.Meta["authorization"]
	if !ok {
		return &JSONRPCErrObj{Code: -32000, Message: "Authentication required", Data: "missing _meta.authorization"}
	}

	authStr, ok := raw.(string)
	if !ok {
		return &JSONRPCErrObj{Code: -32000, Message: "Invalid authorization", Data: "_meta.authorization must be a string"}
	}

	// Strip "Bearer " prefix if present
	const bearerPrefix = "Bearer "
	if len(authStr) > len(bearerPrefix) && authStr[:len(bearerPrefix)] == bearerPrefix {
		authStr = authStr[len(bearerPrefix):]
	}

	if len(authStr) < 8 {
		return &JSONRPCErrObj{Code: -32001, Message: "Invalid API key", Data: "API key too short"}
	}

	// Look up key by prefix + hash
	prefix := authStr[:min8(len(authStr))]
	hash := sha256Sum(authStr)

	ctx := context.Background()
	rows, err := s.db.Query(ctx,
		`SELECT id, scope, session_id FROM api_keys WHERE key_prefix = $1 AND key_hash = $2 AND (expires_at IS NULL OR expires_at > datetime('now'))`,
		prefix, hash,
	)
	if err != nil || len(rows) == 0 {
		return &JSONRPCErrObj{Code: -32001, Message: "Invalid API key", Data: "key not found or expired"}
	}

	scope := toString(rows[0]["scope"])
	sess.authScope = scope
	sess.sessionKey = authStr

	if sid := rows[0]["session_id"]; sid != nil {
		sess.agentSessionID = toString(sid)
	}

	return nil
}

// ============================================================================
// Auth Helpers for Tools
// ============================================================================

// checkAdminScope returns an error if the MCP session doesn't have admin scope.
func (s *Server) checkAdminScope(sess *mcpSession) *JSONRPCErrObj {
	if sess.authScope != "admin" {
		return &JSONRPCErrObj{Code: -32002, Message: "Forbidden", Data: "admin scope required"}
	}
	return nil
}

// checkSessionAccess verifies the session can access the given agent session ID.
func (s *Server) checkSessionAccess(sess *mcpSession, targetSessionID string) *JSONRPCErrObj {
	if sess.authScope == "session" && sess.agentSessionID != targetSessionID {
		return &JSONRPCErrObj{Code: -32002, Message: "Forbidden", Data: "session-scoped key cannot access other sessions"}
	}
	return nil
}

// checkWriteAccess verifies the session has write access (not readonly).
func (s *Server) checkWriteAccess(sess *mcpSession) *JSONRPCErrObj {
	if sess.authScope == "readonly" {
		return &JSONRPCErrObj{Code: -32002, Message: "Forbidden", Data: "readonly scope cannot mutate"}
	}
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

func sha256Sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func min8(n int) int {
	if n < 8 {
		return n
	}
	return 8
}

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
		return ""
	}
}

// toFloat64 converts a value to float64 if possible.
func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0
	}
}

// toInt64 converts a value to int64 if possible.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	default:
		return 0
	}
}

var _ db.DB // ensure import is used
