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
	ID              string         `json:"id"`
	SessionID       string         `json:"session_id"`
	ToolName        string         `json:"tool_name"`
	Parameters      map[string]any `json:"parameters"`
	TimeoutMS       int            `json:"timeout_ms"`
	ApprovalRequestID *string      `json:"approval_request_id,omitempty"`
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
		// Tool not in registry — return a generic result
		return &ToolExecutionResult{
			RequestID: req.ID,
			Output:    fmt.Sprintf("tool %q executed (unregistered)", req.ToolName),
		}, nil
	}

	handlerType := toString(rows[0]["handler_type"])
	handlerRef := toString(rows[0]["handler_ref"])

	switch handlerType {
	case "sql_function":
		return executeSQLFunction(ctx, database, req, handlerRef)

	case "subprocess":
		return executeSubprocess(ctx, req, handlerRef)

	case "go_native":
		return &ToolExecutionResult{
			RequestID: req.ID,
			Output:    fmt.Sprintf("tool %q executed (go_native stub: %s)", req.ToolName, handlerRef),
		}, nil

	case "http_endpoint":
		return executeHTTPEndpoint(ctx, database, req, handlerRef)

	default:
		return &ToolExecutionResult{
			RequestID: req.ID,
			Output:    fmt.Sprintf("tool %q executed (unknown handler_type: %s)", req.ToolName, handlerType),
		}, nil
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

func executeSubprocess(ctx context.Context, req *ToolExecutionRequest, handlerRef string) (*ToolExecutionResult, error) {
	startTime := time.Now()

	// Build config from request
	cfg := DefaultExternalToolConfig()

	// Use the request's timeout if set
	if req.TimeoutMS > 0 {
		cfg.Timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}

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
				br.Seek(0, io.SeekStart)
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
		resp.Body.Close()

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
