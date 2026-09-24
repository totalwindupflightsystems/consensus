// axiom:trace work_item=interfaces-api-cli-01,full-platform-audit spec=specs/015-api-and-mcp.md plan=phase-3 impl=internal/mcp/tools.go
// axiom:trace work_item=make-consensus-fully-operational-end-to spec=specs/015-api-and-mcp.md plan=phase-3/task-3-2/step-3-2-1 impl=internal/mcp/tools.go
package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// ============================================================================
// Tools — tools/list and tools/call (SPEC-015 §5.2)
// ============================================================================

// handleToolsList returns all available MCP tools.
func (s *Server) handleToolsList(sess *mcpSession) (any, *JSONRPCErrObj) {
	tools := []MCPToolDefinition{
		{
			Name:        "create_session",
			Description: "Create a new agent session",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"agent_name": {Type: "string", Description: "Name for the agent"},
					"goal":       {Type: "string", Description: "The task for the agent to accomplish"},
					"model_id":   {Type: "string", Description: "LLM model to use"},
				},
				Required: []string{"agent_name", "goal"},
			},
		},
		{
			Name:        "send_message",
			Description: "Send a message to a running agent",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"session_id": {Type: "string", Description: "The session to message"},
					"message":    {Type: "string", Description: "The message content"},
				},
				Required: []string{"session_id", "message"},
			},
		},
		{
			Name:        "get_session_status",
			Description: "Get the current status of an agent session",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"session_id": {Type: "string", Description: "The session to query"},
				},
				Required: []string{"session_id"},
			},
		},
		{
			Name:        "list_memory",
			Description: "List memory events for a session",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"session_id": {Type: "string", Description: "The session to query"},
					"limit":      {Type: "integer", Description: "Max events to return (default 20)"},
				},
				Required: []string{"session_id"},
			},
		},
		{
			Name:        "review_approval",
			Description: "Approve or reject a pending HITL approval request",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"approval_id":  {Type: "string", Description: "The approval request ID"},
					"decision":     {Type: "string", Description: "approved, rejected, or modified", Enum: []any{"approved", "rejected", "modified"}},
					"notes":        {Type: "string", Description: "Optional review notes"},
					"modified_sql": {Type: "string", Description: "Modified SQL (required for modified decision)"},
				},
				Required: []string{"approval_id", "decision"},
			},
		},
		{
			Name:        "query_tool",
			Description: "Execute an internal (SQL) tool registered in tools_registry",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"session_id": {Type: "string", Description: "Session to execute within"},
					"tool_name":  {Type: "string", Description: "Name from tools_registry"},
					"parameters": {Type: "object", Description: "Tool-specific parameters"},
				},
				Required: []string{"session_id", "tool_name"},
			},
		},
	}

	return map[string]any{"tools": tools}, nil
}

// handleToolsCall dispatches a tool call to the correct implementation.
// Each tool call is traced with start/end span markers (WI-020).
func (s *Server) handleToolsCall(req *JSONRPCRequest, sess *mcpSession) (any, *JSONRPCErrObj) {
	var call MCPCallToolRequest
	if err := json.Unmarshal(req.Params, &call); err != nil {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Invalid params", Data: err.Error()}
	}

	// Trace the tool call
	tID := traceID()
	sID := spanID()
	endTool := spanStart("mcp.tools/call."+call.Name, tID, sID)

	var result any
	var rpcErr *JSONRPCErrObj

	switch call.Name {
	case "create_session":
		result, rpcErr = s.toolCreateSession(call.Arguments, sess)
	case "send_message":
		result, rpcErr = s.toolSendMessage(call.Arguments, sess)
	case "get_session_status":
		result, rpcErr = s.toolGetSessionStatus(call.Arguments, sess)
	case "list_memory":
		result, rpcErr = s.toolListMemory(call.Arguments, sess)
	case "review_approval":
		result, rpcErr = s.toolReviewApproval(call.Arguments, sess)
	case "query_tool":
		result, rpcErr = s.toolQueryTool(call.Arguments, sess)
	default:
		rpcErr = &JSONRPCErrObj{Code: -32601, Message: "Tool not found", Data: call.Name}
	}

	// End trace span with error if present
	if rpcErr != nil {
		endTool(fmt.Errorf("tool error: %s (code %d)", rpcErr.Message, rpcErr.Code))
	} else {
		endTool(nil)
	}

	return result, rpcErr
}

// ============================================================================
// Tool Implementations
// ============================================================================

// toolCreateSession creates a new agent session (SPEC-015 §5.2).
func (s *Server) toolCreateSession(args json.RawMessage, sess *mcpSession) (any, *JSONRPCErrObj) {
	if err := s.checkWriteAccess(sess); err != nil {
		return nil, err
	}

	var input struct {
		AgentName string `json:"agent_name"`
		Goal      string `json:"goal"`
		ModelID   string `json:"model_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Invalid arguments", Data: err.Error()}
	}

	if input.AgentName == "" || input.Goal == "" {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Missing required fields", Data: "agent_name and goal are required"}
	}
	if input.ModelID == "" {
		input.ModelID = "gpt-4o"
	}

	ctx := context.Background()

	// Create the session
	sessionID := generateUUID()
	err := s.db.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, goal) VALUES ($1, $2, $3, 'booting', $4)`,
		sessionID, input.AgentName, input.ModelID, input.Goal,
	)
	if err != nil {
		return nil, &JSONRPCErrObj{Code: -32603, Message: "Internal error", Data: err.Error()}
	}

	// Create session API key
	apiKeyVal := "cs_sk_" + generateShortID(64)
	err = s.db.Exec(ctx,
		`INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id) VALUES ($1, $2, $3, 'session', $4)`,
		generateUUID(), sha256Sum(apiKeyVal), apiKeyVal[:8], sessionID,
	)
	if err != nil {
		return nil, &JSONRPCErrObj{Code: -32603, Message: "Internal error", Data: err.Error()}
	}

	// Insert a starter memory event
	err = s.db.Exec(ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('header', $2, $1, 0)`,
		sessionID, fmt.Sprintf("Session created for: %s", input.Goal),
	)

	result := map[string]any{
		"id":      sessionID,
		"status":  "booting",
		"api_key": apiKeyVal,
	}

	return MCPCallToolResult{
		Content: []MCPTextContent{{Type: "text", Text: formatJSON(result)}},
	}, nil
}

// toolSendMessage sends a message to a running agent (SPEC-015 §5.2).
func (s *Server) toolSendMessage(args json.RawMessage, sess *mcpSession) (any, *JSONRPCErrObj) {
	if err := s.checkWriteAccess(sess); err != nil {
		return nil, err
	}

	var input struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Invalid arguments", Data: err.Error()}
	}
	if input.SessionID == "" || input.Message == "" {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Missing required fields", Data: "session_id and message are required"}
	}

	if err := s.checkSessionAccess(sess, input.SessionID); err != nil {
		return nil, err
	}

	ctx := context.Background()

	// Check session exists and is active
	rows, err := s.db.Query(ctx,
		`SELECT id, status FROM sessions WHERE id = $1`,
		input.SessionID,
	)
	if err != nil || len(rows) == 0 {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Session not found", Data: input.SessionID}
	}

	status := toString(rows[0]["status"])
	if status == "completed" || status == "failed" {
		return nil, &JSONRPCErrObj{Code: -32602, Message: fmt.Sprintf("Session is %s", status), Data: ""}
	}

	// Insert message into memory_events
	err = s.db.Exec(ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('user_message', $1 || ' ' || $2, $1, 0)`,
		input.SessionID, input.Message,
	)
	if err != nil {
		return nil, &JSONRPCErrObj{Code: -32603, Message: "Internal error", Data: err.Error()}
	}

	// If session is idle or paused, wake it to thinking
	if status == "idle" || status == "paused" {
		s.db.Exec(ctx,
			`UPDATE sessions SET status = 'thinking', heartbeat_at = datetime('now') WHERE id = $1`,
			input.SessionID,
		)
	}

	return MCPCallToolResult{
		Content: []MCPTextContent{{Type: "text", Text: fmt.Sprintf(`{"sent": true, "session_id": "%s"}`, input.SessionID)}},
	}, nil
}

// toolGetSessionStatus gets the current status of an agent session (SPEC-015 §5.2).
func (s *Server) toolGetSessionStatus(args json.RawMessage, sess *mcpSession) (any, *JSONRPCErrObj) {
	var input struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Invalid arguments", Data: err.Error()}
	}
	if input.SessionID == "" {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Missing required fields", Data: "session_id is required"}
	}

	if err := s.checkSessionAccess(sess, input.SessionID); err != nil {
		return nil, err
	}

	ctx := context.Background()
	rows, err := s.db.Query(ctx,
		`SELECT id, agent_name, status, goal, iteration, tokens_used_in, tokens_used_out, created_at, heartbeat_at FROM sessions WHERE id = $1`,
		input.SessionID,
	)
	if err != nil || len(rows) == 0 {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Session not found", Data: input.SessionID}
	}

	r := rows[0]
	result := map[string]any{
		"id":              toString(r["id"]),
		"agent_name":      toString(r["agent_name"]),
		"status":          toString(r["status"]),
		"goal":            toString(r["goal"]),
		"iteration":       toInt64(r["iteration"]),
		"tokens_used_in":  toInt64(r["tokens_used_in"]),
		"tokens_used_out": toInt64(r["tokens_used_out"]),
	}

	return MCPCallToolResult{
		Content: []MCPTextContent{{Type: "text", Text: formatJSON(result)}},
	}, nil
}

// toolListMemory lists memory events for a session (SPEC-015 §5.2).
func (s *Server) toolListMemory(args json.RawMessage, sess *mcpSession) (any, *JSONRPCErrObj) {
	var input struct {
		SessionID string `json:"session_id"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Invalid arguments", Data: err.Error()}
	}
	if input.SessionID == "" {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Missing required fields", Data: "session_id is required"}
	}
	if input.Limit <= 0 {
		input.Limit = 20
	}

	if err := s.checkSessionAccess(sess, input.SessionID); err != nil {
		return nil, err
	}

	ctx := context.Background()
	rows, err := s.db.Query(ctx,
		`SELECT id, type, content, iteration_created, created_at FROM memory_events WHERE session_id = $1 ORDER BY id DESC LIMIT $2`,
		input.SessionID, input.Limit,
	)
	if err != nil {
		return nil, &JSONRPCErrObj{Code: -32603, Message: "Internal error", Data: err.Error()}
	}

	events := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		events = append(events, map[string]any{
			"id":                toInt64(r["id"]),
			"type":              toString(r["type"]),
			"content":           truncateContent(toString(r["content"]), 500),
			"iteration_created": toInt64(r["iteration_created"]),
			"created_at":        toString(r["created_at"]),
		})
	}

	result := map[string]any{"events": events, "count": len(events)}
	return MCPCallToolResult{
		Content: []MCPTextContent{{Type: "text", Text: formatJSON(result)}},
	}, nil
}

// toolReviewApproval approves or rejects a pending HITL approval (SPEC-015 §5.2).
func (s *Server) toolReviewApproval(args json.RawMessage, sess *mcpSession) (any, *JSONRPCErrObj) {
	if err := s.checkAdminScope(sess); err != nil {
		return nil, err
	}

	var input struct {
		ApprovalID  string `json:"approval_id"`
		Decision    string `json:"decision"`
		Notes       string `json:"notes"`
		ModifiedSQL string `json:"modified_sql"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Invalid arguments", Data: err.Error()}
	}

	validDecisions := map[string]bool{"approved": true, "rejected": true, "modified": true}
	if !validDecisions[input.Decision] {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Invalid decision", Data: "must be approved, rejected, or modified"}
	}
	if input.Decision == "modified" && input.ModifiedSQL == "" {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Missing modified_sql", Data: "modified_sql is required for modified decision"}
	}

	ctx := context.Background()

	// Verify the approval exists and is pending
	rows, err := s.db.Query(ctx,
		`SELECT id, session_id, status FROM approval_requests WHERE id = $1`,
		input.ApprovalID,
	)
	if err != nil || len(rows) == 0 {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Approval not found", Data: input.ApprovalID}
	}

	status := toString(rows[0]["status"])
	if status != "pending" {
		return nil, &JSONRPCErrObj{Code: -32602, Message: fmt.Sprintf("Approval already %s", status), Data: ""}
	}

	sessionID := toString(rows[0]["session_id"])

	// Update approval
	err = s.db.Exec(ctx,
		`UPDATE approval_requests SET status = $1, review_notes = $2, reviewed_at = datetime('now') WHERE id = $3`,
		input.Decision, input.Notes, input.ApprovalID,
	)
	if err != nil {
		return nil, &JSONRPCErrObj{Code: -32603, Message: "Internal error", Data: err.Error()}
	}

	// If approved/resolved, resume the session
	if input.Decision == "approved" || input.Decision == "modified" {
		s.db.Exec(ctx,
			`UPDATE sessions SET status = 'thinking', heartbeat_at = datetime('now') WHERE id = $1 AND status = 'paused'`,
			sessionID,
		)
	}

	result := map[string]any{"status": input.Decision, "approval_id": input.ApprovalID}
	return MCPCallToolResult{
		Content: []MCPTextContent{{Type: "text", Text: formatJSON(result)}},
	}, nil
}

// toolQueryTool executes an internal tool registered in tools_registry (SPEC-015 §5.2).
func (s *Server) toolQueryTool(args json.RawMessage, sess *mcpSession) (any, *JSONRPCErrObj) {
	if err := s.checkWriteAccess(sess); err != nil {
		return nil, err
	}

	var input struct {
		SessionID  string         `json:"session_id"`
		ToolName   string         `json:"tool_name"`
		Parameters map[string]any `json:"parameters"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Invalid arguments", Data: err.Error()}
	}
	if input.SessionID == "" || input.ToolName == "" {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Missing required fields", Data: "session_id and tool_name are required"}
	}

	if err := s.checkSessionAccess(sess, input.SessionID); err != nil {
		return nil, err
	}

	ctx := context.Background()

	// Look up the tool in tools_registry
	rows, err := s.db.Query(ctx,
		`SELECT name, hemisphere, handler_type, handler_ref FROM tools_registry WHERE name = $1 AND enabled = true AND status = 'active'`,
		input.ToolName,
	)
	if err != nil || len(rows) == 0 {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Tool not found", Data: input.ToolName}
	}

	tool := rows[0]
	hemisphere := toString(tool["hemisphere"])
	handlerType := toString(tool["handler_type"])

	if hemisphere != "internal" {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Only internal tools supported via MCP", Data: hemisphere}
	}

	result := map[string]any{}
	switch handlerType {
	case "sql_function":
		// Call a SQL function — SELECT * FROM function_name()
		handlerRef := toString(tool["handler_ref"])
		rows, err := s.db.Query(ctx, fmt.Sprintf("SELECT * FROM %s()", handlerRef))
		if err != nil {
			return nil, &JSONRPCErrObj{Code: -32603, Message: "Tool execution error", Data: err.Error()}
		}
		result["rows"] = rows
		result["count"] = len(rows)
	default:
		result["message"] = fmt.Sprintf("Tool %s found but handler type %s not supported via MCP", input.ToolName, handlerType)
	}

	return MCPCallToolResult{
		Content: []MCPTextContent{{Type: "text", Text: formatJSON(result)}},
	}, nil
}

// ============================================================================
// Helpers
// ============================================================================

func generateUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand only fails on catastrophic system entropy loss; never
		// fall back to deterministic output (DOGFOOD-102: every MCP session
		// used to get the SAME id, so create_session worked exactly once).
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	// Set UUIDv4 version (4) and RFC 4122 variant bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// generateShortID creates a random hex string of given length.
func generateShortID(n int) string {
	b := make([]byte, n/2)
	if _, err := rand.Read(b); err != nil {
		// Same guarantee as generateUUID: never deterministic (DOGFOOD-102 —
		// every MCP session used to get the SAME api_key).
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

func truncateContent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func formatJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}

var _ = strings.TrimSpace // keep strings import used
