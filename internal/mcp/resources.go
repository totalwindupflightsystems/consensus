// axiom:trace work_item=interfaces-api-cli-01,full-platform-audit spec=specs/015-api-and-mcp.md plan=phase-3 impl=internal/mcp/resources.go
// axiom:trace work_item=make-consensus-fully-operational-end-to spec=specs/015-api-and-mcp.md plan=phase-3/task-3-2/step-3-2-1 impl=internal/mcp/resources.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// Resources — resources/list and resources/read (SPEC-015 §5.2, §5.4)
// ============================================================================

// handleResourcesList returns a static list of resources.
func (s *Server) handleResourcesList(sess *mcpSession) (any, *JSONRPCErrObj) {
	resources := []MCPResourceDefinition{
		{
			URI:         "consensus://sessions",
			Name:        "sessions",
			Description: "Active agent sessions",
			MimeType:    "application/json",
		},
		{
			URI:         "consensus://tools",
			Name:        "tools_registry",
			Description: "Available tools and skills",
			MimeType:    "application/json",
		},
	}

	return map[string]any{"resources": resources}, nil
}

// handleResourceTemplates returns URI templates for parameterized resources.
func (s *Server) handleResourceTemplates(sess *mcpSession) (any, *JSONRPCErrObj) {
	templates := []MCPResourceTemplate{
		{
			URITemplate: "consensus://sessions/{session_id}/context",
			Name:        "session_context",
			Description: "Active context view for a session",
			MimeType:    "application/json",
		},
	}

	return map[string]any{"resourceTemplates": templates}, nil
}

// handleResourcesRead returns the content of a specific resource.
func (s *Server) handleResourcesRead(req *JSONRPCRequest, sess *mcpSession) (any, *JSONRPCErrObj) {
	tID := traceID()
	sID := spanID()
	endRes := spanStart("mcp.resources/read", tID, sID)

	var read MCPReadResourceRequest
	if err := json.Unmarshal(req.Params, &read); err != nil {
		endRes(fmt.Errorf("invalid params: %s", err.Error()))
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Invalid params", Data: err.Error()}
	}

	ctx := context.Background()

	var result any
	var rpcErr *JSONRPCErrObj

	switch {
	case read.URI == "consensus://sessions":
		result, rpcErr = s.readSessionsResource(ctx, sess)
	case read.URI == "consensus://tools":
		result, rpcErr = s.readToolsResource(ctx, sess)
	case matchSessionContext(read.URI):
		sessionID := extractSessionIDFromURI(read.URI)
		result, rpcErr = s.readSessionContextResource(ctx, sess, sessionID)
	default:
		rpcErr = &JSONRPCErrObj{Code: -32602, Message: "Resource not found", Data: read.URI}
	}

	if rpcErr != nil {
		endRes(fmt.Errorf("resource error: %s", rpcErr.Message))
	} else {
		endRes(nil)
	}
	return result, rpcErr
}

// ============================================================================
// Resource Readers
// ============================================================================

func (s *Server) readSessionsResource(ctx context.Context, sess *mcpSession) (any, *JSONRPCErrObj) {
	_ = ctx // unused — uses context.Background() for now
	var rows []db.Row
	var err error

	if sess.authScope == "session" && sess.agentSessionID != "" {
		rows, err = s.db.Query(context.Background(),
			`SELECT id, agent_name, status, goal, iteration FROM sessions WHERE id = $1`,
			sess.agentSessionID,
		)
	} else {
		rows, err = s.db.Query(context.Background(),
			`SELECT id, agent_name, status, goal, iteration FROM sessions WHERE status IN ('idle','thinking','tool_exec','waiting_sub','paused','planning','executing') ORDER BY created_at DESC LIMIT 50`,
		)
	}

	if err != nil {
		return nil, &JSONRPCErrObj{Code: -32603, Message: "Internal error", Data: err.Error()}
	}

	sessions := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		sessions = append(sessions, map[string]any{
			"id":         toString(r["id"]),
			"agent_name": toString(r["agent_name"]),
			"status":     toString(r["status"]),
			"goal":       toString(r["goal"]),
			"iteration":  toInt64(r["iteration"]),
		})
	}

	content, _ := json.MarshalIndent(sessions, "", "  ")
	return MCPReadResourceResult{
		Contents: []MCPResourceContent{{
			URI:      "consensus://sessions",
			MimeType: "application/json",
			Text:     string(content),
		}},
	}, nil
}

func (s *Server) readToolsResource(ctx context.Context, sess *mcpSession) (any, *JSONRPCErrObj) {
	_ = ctx // unused — uses context.Background() for now
	rows, err := s.db.Query(context.Background(),
		`SELECT name, description, hemisphere, handler_type, status FROM tools_registry WHERE enabled = true ORDER BY name`,
	)
	if err != nil {
		return nil, &JSONRPCErrObj{Code: -32603, Message: "Internal error", Data: err.Error()}
	}

	tools := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		tools = append(tools, map[string]any{
			"name":         toString(r["name"]),
			"description":  toString(r["description"]),
			"hemisphere":   toString(r["hemisphere"]),
			"handler_type": toString(r["handler_type"]),
			"status":       toString(r["status"]),
		})
	}

	content, _ := json.MarshalIndent(tools, "", "  ")
	return MCPReadResourceResult{
		Contents: []MCPResourceContent{{
			URI:      "consensus://tools",
			MimeType: "application/json",
			Text:     string(content),
		}},
	}, nil
}

func (s *Server) readSessionContextResource(ctx context.Context, sess *mcpSession, sessionID string) (any, *JSONRPCErrObj) {
	_ = ctx // unused — uses context.Background() for now
	if err := s.checkSessionAccess(sess, sessionID); err != nil {
		return nil, err
	}

	// Read active context view
	rows, err := s.db.Query(context.Background(),
		`SELECT id, iteration_created, type, display_mode, rendered_text FROM active_context_view WHERE session_id = $1 ORDER BY iteration_created, id LIMIT 100`,
		sessionID,
	)
	if err != nil {
		return nil, &JSONRPCErrObj{Code: -32603, Message: "Internal error", Data: err.Error()}
	}

	events := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		events = append(events, map[string]any{
			"id":                toInt64(r["id"]),
			"iteration_created": toInt64(r["iteration_created"]),
			"type":              toString(r["type"]),
			"display_mode":      toString(r["display_mode"]),
			"rendered_text":     toString(r["rendered_text"]),
		})
	}

	content, _ := json.MarshalIndent(events, "", "  ")
	return MCPReadResourceResult{
		Contents: []MCPResourceContent{{
			URI:      "consensus://sessions/" + sessionID + "/context",
			MimeType: "application/json",
			Text:     string(content),
		}},
	}, nil
}

// ============================================================================
// Prompts — prompts/list and prompts/get (SPEC-015 §5.2)
// ============================================================================

func (s *Server) handlePromptsList(sess *mcpSession) (any, *JSONRPCErrObj) {
	prompts := []MCPPromptDefinition{
		{
			Name:        "agent_status",
			Description: "Get a summary of what an agent is currently doing",
			Arguments: []MCPPromptArgument{
				{Name: "session_id", Description: "The session to query", Required: true},
			},
		},
	}

	return map[string]any{"prompts": prompts}, nil
}

func (s *Server) handlePromptsGet(req *JSONRPCRequest, sess *mcpSession) (any, *JSONRPCErrObj) {
	tID := traceID()
	sID := spanID()
	endPrompt := spanStart("mcp.prompts/get", tID, sID)

	var input struct {
		Name      string         `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &input); err != nil {
		endPrompt(fmt.Errorf("invalid params: %s", err.Error()))
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Invalid params", Data: err.Error()}
	}

	if input.Name != "agent_status" {
		endPrompt(fmt.Errorf("prompt not found: %s", input.Name))
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Prompt not found", Data: input.Name}
	}

	var args struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(input.Arguments, &args); err != nil {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Invalid arguments", Data: err.Error()}
	}

	if args.SessionID == "" {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Missing required argument", Data: "session_id is required"}
	}

	if err := s.checkSessionAccess(sess, args.SessionID); err != nil {
		endPrompt(fmt.Errorf("access denied: %s", err.Message))
		return nil, err
	}

	rows, err := s.db.Query(context.Background(),
		`SELECT id, agent_name, status, goal, iteration, tokens_used_in, tokens_used_out FROM sessions WHERE id = $1`,
		args.SessionID,
	)
	if err != nil || len(rows) == 0 {
		endPrompt(fmt.Errorf("session not found: %s", args.SessionID))
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Session not found", Data: args.SessionID}
	}

	r := rows[0]
	// Build a human-readable status summary
	summary := formatJSON(map[string]any{
		"session_id":      toString(r["id"]),
		"agent_name":      toString(r["agent_name"]),
		"status":          toString(r["status"]),
		"goal":            toString(r["goal"]),
		"iteration":       toInt64(r["iteration"]),
		"tokens_used_in":  toInt64(r["tokens_used_in"]),
		"tokens_used_out": toInt64(r["tokens_used_out"]),
	})

	endPrompt(nil)
	return map[string]any{
		"description": "Agent status summary",
		"messages": []map[string]any{
			{
				"role": "user",
				"content": map[string]string{
					"type": "text",
					"text": summary,
				},
			},
		},
	}, nil
}

// ============================================================================
// URI Parsing Helpers
// ============================================================================

// matchSessionContext checks if a URI targets a session-specific context resource.
func matchSessionContext(uri string) bool {
	const prefix = "consensus://sessions/"
	const suffix = "/context"
	return len(uri) > len(prefix)+len(suffix) &&
		uri[:len(prefix)] == prefix &&
		hasSuffix(uri, suffix)
}

func extractSessionIDFromURI(uri string) string {
	// consensus://sessions/{session_id}/context
	const prefix = "consensus://sessions/"
	const suffix = "/context"
	return uri[len(prefix) : len(uri)-len(suffix)]
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

var _ = (*db.DB)(nil) // ensure db import used
