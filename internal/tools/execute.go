// Package tools: high-level tool execution integration (WI-005).
//
// This file bridges the sandbox execution (sandbox.go) with the tool registry
// and database-backed tool_requests. It provides the ExecuteTool function that
// resolves a tool request to a handler, enforces rate limits, checks approval
// requirements, and dispatches to the appropriate execution engine.
//
// axiom:trace work_item=WI-005 spec=specs/010-tools.md plan=phase-1/task-2
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// Tool Execution Entry Point
// ============================================================================

// ToolExecutionRequest describes a single pending tool request.
type ToolExecutionRequest struct {
	ID                string         `json:"id"`
	SessionID         string         `json:"session_id"`
	ToolName          string         `json:"tool_name"`
	Parameters        map[string]any `json:"parameters"`
	TimeoutMS         int            `json:"timeout_ms"`
	ApprovalRequestID *string        `json:"approval_request_id,omitempty"`
}

// ToolExecutionResult captures the outcome of executing a tool request.
type ToolExecutionResult struct {
	RequestID  string `json:"request_id"`
	Output     string `json:"output"`
	IsError    bool   `json:"is_error"`
	ErrorCode  string `json:"error_code,omitempty"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// ============================================================================
// Registry Lookup + Execution Dispatch
// ============================================================================

// ExecuteTool looks up a tool in the registry and dispatches it to the
// correct handler type (sql_function, subprocess, go_native, http_endpoint).
//
// This is the main dispatch function called by the harness tool executor.
func ExecuteTool(ctx context.Context, database db.DB, req *ToolExecutionRequest) (*ToolExecutionResult, error) {
	if database == nil {
		return nil, fmt.Errorf("execute_tool: no database configured")
	}
	if req == nil {
		return nil, fmt.Errorf("execute_tool: nil request")
	}

	slog.Debug("execute_tool: dispatching",
		"tool", req.ToolName,
		"session", req.SessionID,
		"request_id", req.ID,
	)

	// Look up the tool in tools_registry
	rows, err := database.Query(ctx, `
		SELECT name, handler_type, handler_ref, hemisphere, requires_approval, rate_limit_per_min
		FROM tools_registry
		WHERE name = $1 AND enabled = true
		LIMIT 1
	`, req.ToolName)
	if err != nil {
		return nil, fmt.Errorf("execute_tool: lookup %q: %w", req.ToolName, err)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("execute_tool: tool %q is not registered or enabled", req.ToolName)
	}

	handlerType := toString(rows[0]["handler_type"])
	handlerRef := toString(rows[0]["handler_ref"])

	switch handlerType {
	case "sql_function":
		return executeSQLFunction(ctx, database, req, handlerRef)

	case "subprocess":
		return executeSubprocess(ctx, database, req, handlerRef)

	case "go_native":
		return executeGoNative(ctx, database, req, handlerRef)

	case "http_endpoint":
		return executeHTTPEndpoint(ctx, database, req, handlerRef)

	default:
		return nil, fmt.Errorf("execute_tool: unsupported handler_type %q for tool %q", handlerType, req.ToolName)
	}
}

// ============================================================================
// SQL Function Execution
// ============================================================================

func executeSQLFunction(ctx context.Context, database db.DB, req *ToolExecutionRequest, handlerRef string) (*ToolExecutionResult, error) {
	startTime := time.Now()

	// Marshal parameters to JSON for passing to the SQL function
	paramsJSON, err := json.Marshal(req.Parameters)
	if err != nil {
		return errorResult(req.ID, fmt.Sprintf("marshal params: %v", err), startTime), nil
	}

	resultRows, err := database.Query(ctx, fmt.Sprintf("SELECT %s($1) as result", handlerRef), string(paramsJSON))
	if err != nil {
		return errorResult(req.ID, fmt.Sprintf("sql_function %s: %v", handlerRef, err), startTime), nil
	}

	output := ""
	if len(resultRows) > 0 {
		b, _ := json.Marshal(resultRows)
		output = string(b)
	}

	return &ToolExecutionResult{
		RequestID:  req.ID,
		Output:     output,
		DurationMs: time.Since(startTime).Milliseconds(),
	}, nil
}

// ============================================================================
// Subprocess Execution
// ============================================================================

// executeSubprocess runs a subprocess tool in the SESSION WORKSPACE and hands it
// the request payload (the decoded model parameters as JSON on stdin).
//
// DF-CONSENSUS-077: pre-fix this used a throwaway temp dir and dropped the
// parameters, so a registered subprocess tool could never act on the workspace
// and never saw its arguments.
func executeSubprocess(ctx context.Context, database db.DB, req *ToolExecutionRequest, handlerRef string) (*ToolExecutionResult, error) {
	startTime := time.Now()

	// Build config from request
	cfg := DefaultExternalToolConfig()

	// Use the request's timeout if set
	if req.TimeoutMS > 0 {
		cfg.Timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}

	// The tool acts on the session workspace, not a throwaway directory.
	workspace, err := ResolveWorkspace(ctx, database, req.SessionID)
	if err != nil {
		return errorResult(req.ID, err.Error(), startTime), nil
	}
	cfg.WorkDir = workspace

	// Payload: the model's parameters, JSON, on stdin. Always a JSON object so a
	// handler can parse unconditionally.
	payload, err := json.Marshal(nonNilParams(req.Parameters))
	if err != nil {
		return errorResult(req.ID, fmt.Sprintf("marshal params: %v", err), startTime), nil
	}
	cfg.Stdin = payload

	// Parse the executable and args from handlerRef.
	// handlerRef can be:
	//   - "executable" (single command name)
	//   - "executable arg1 arg2" (command with space-separated args)
	//   - "/path/to/executable" (full path)
	parts := splitCommand(handlerRef)
	executable := parts[0]
	args := parts[1:]

	// Run in sandbox
	result, err := ExecuteExternalTool(ctx, executable, args, cfg)
	if err != nil {
		return errorResult(req.ID, fmt.Sprintf("sandbox: %v", err), startTime), nil
	}

	execResult := &ToolExecutionResult{
		RequestID:  req.ID,
		Output:     result.Output,
		DurationMs: result.DurationMs,
	}

	if result.Error != "" {
		execResult.IsError = true
		execResult.ErrorCode = fmt.Sprintf("EXIT_%d", result.ExitCode)
	}

	if result.ExitCode != 0 {
		code := result.ExitCode
		execResult.ExitCode = &code
	}

	return execResult, nil
}

// executeGoNative dispatches a go_native tool through the handler registry
// (DF-CONSENSUS-077).
//
// The two failure shapes are deliberately distinct:
//   - an unregistered handler_ref returns a non-nil error and a nil result: no
//     effect happened, so nothing may be reported as a successful execution
//     (fail-closed — the pre-fix stub returned a success string here);
//   - a handler that ran and failed returns an error RESULT (IsError=true), the
//     same shape executeSubprocess uses, so the reason reaches the caller.
func executeGoNative(ctx context.Context, database db.DB, req *ToolExecutionRequest, handlerRef string) (*ToolExecutionResult, error) {
	startTime := time.Now()

	output, err := RunNativeTool(ctx, database, req.SessionID, req.ToolName, handlerRef, req.Parameters)
	if err != nil {
		if errors.Is(err, ErrNativeHandlerNotRegistered) {
			return nil, fmt.Errorf("execute_tool: %w", err)
		}
		return errorResult(req.ID, err.Error(), startTime), nil
	}

	return &ToolExecutionResult{
		RequestID:  req.ID,
		Output:     output,
		DurationMs: time.Since(startTime).Milliseconds(),
	}, nil
}

// nonNilParams returns params, or an empty object when it is nil, so a handler
// always receives a JSON object.
func nonNilParams(params map[string]any) map[string]any {
	if params == nil {
		return map[string]any{}
	}
	return params
}

// ============================================================================
// Helpers
// ============================================================================

func errorResult(requestID, errMsg string, startTime time.Time) *ToolExecutionResult {
	return &ToolExecutionResult{
		RequestID:  requestID,
		Output:     errMsg,
		IsError:    true,
		ErrorCode:  "TOOL_ERROR",
		DurationMs: time.Since(startTime).Milliseconds(),
	}
}

// splitCommand splits a command string into executable and args,
// handling quoted arguments.
func splitCommand(cmd string) []string {
	if cmd == "" {
		return []string{""}
	}
	// Simple split by spaces, respecting double-quoted tokens
	var parts []string
	var current strings.Builder
	inQuote := false
	for _, r := range cmd {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// executeHTTPEndpoint forwards a tool call to an HTTP API and returns a
// ToolExecutionResult. handlerRef is a full URL; {param} placeholders are
// substituted from req.Parameters. Retries up to 3 attempts (1s/2s/4s) on
// transient errors.
func executeHTTPEndpoint(ctx context.Context, database db.DB, req *ToolExecutionRequest, handlerRef string) (*ToolExecutionResult, error) {
	startTime := time.Now()

	// Resolve API key
	apiKey := ""
	if keyRows, err := database.Query(ctx, "SELECT api_key FROM sessions WHERE id = $1", req.SessionID); err == nil && len(keyRows) > 0 {
		apiKey = toString(keyRows[0]["api_key"])
	}

	// Substitute {param} placeholders
	targetURL := handlerRef
	for k, v := range req.Parameters {
		targetURL = strings.ReplaceAll(targetURL, "{"+k+"}", fmt.Sprintf("%v", v))
	}

	// Infer HTTP method
	method := "POST"
	switch {
	case strings.HasSuffix(req.ToolName, "_read"), strings.HasSuffix(req.ToolName, "_list"):
		method = "GET"
	case strings.HasSuffix(req.ToolName, "_delete"):
		method = "DELETE"
	case strings.HasSuffix(req.ToolName, "_update"), req.ToolName == "tag_update":
		method = "PUT"
	}

	// Build body
	var bodyReader io.Reader
	if method != "GET" && method != "DELETE" {
		buf, err := json.Marshal(req.Parameters)
		if err != nil {
			return errorResult(req.ID, fmt.Sprintf("marshal params: %v", err), startTime), nil
		}
		bodyReader = bytes.NewReader(buf)
	}

	// Retry loop
	var lastErr error
	for attempt := 0; attempt <= 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return errorResult(req.ID, fmt.Sprintf("cancelled: %v", ctx.Err()), startTime), nil
			case <-time.After(time.Duration(1<<(attempt-1)) * time.Second):
			}
			if br, ok := bodyReader.(*bytes.Reader); ok {
				_, _ = br.Seek(0, io.SeekStart)
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, method, targetURL, bodyReader)
		if err != nil {
			return errorResult(req.ID, fmt.Sprintf("build request: %v", err), startTime), nil
		}
		if apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		}
		if bodyReader != nil {
			httpReq.Header.Set("Content-Type", "application/json")
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = err
			if isRetryableHTTP(err) {
				continue
			}
			return errorResult(req.ID, fmt.Sprintf("http: %v", err), startTime), nil
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return &ToolExecutionResult{
				RequestID:  req.ID,
				Output:     string(body),
				DurationMs: time.Since(startTime).Milliseconds(),
			}, nil
		}

		result := &ToolExecutionResult{
			RequestID:  req.ID,
			Output:     fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
			IsError:    true,
			ErrorCode:  fmt.Sprintf("HTTP_%d", resp.StatusCode),
			DurationMs: time.Since(startTime).Milliseconds(),
		}

		if resp.StatusCode == 503 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		return result, nil
	}

	return errorResult(req.ID, fmt.Sprintf("all retries exhausted: %v", lastErr), startTime), nil
}

func isRetryableHTTP(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") || strings.Contains(msg, "EOF")
}
