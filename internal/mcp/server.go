// axiom:trace work_item=interfaces-api-cli-01,full-platform-audit spec=specs/015-api-and-mcp.md plan=phase-3 impl=internal/mcp/server.go
// axiom:trace work_item=make-conscience-fully-operational-end-to spec=specs/015-api-and-mcp.md plan=phase-3/task-3-2/step-3-2-1 impl=internal/mcp/server.go
package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/wojons/conscientiousness/internal/db"
)

// ============================================================================
// JSON-RPC 2.0 Types
// ============================================================================

// JSONRPCRequest is a standard JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"` // nil = notification
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is a standard JSON-RPC 2.0 success response.
type JSONRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result"`
}

// JSONRPCError is a standard JSON-RPC 2.0 error response.
type JSONRPCErrorResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id"`
	Error   JSONRPCErrObj `json:"error"`
}

// JSONRPCErrObj is the error object in a JSON-RPC error response.
type JSONRPCErrObj struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ============================================================================
// MCP Protocol Types
// ============================================================================

// MCPInitializeRequest is the params for the initialize method.
type MCPInitializeRequest struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    MCPClientCapabilities  `json:"capabilities"`
	ClientInfo      MCPClientInfo          `json:"clientInfo"`
	Meta            map[string]interface{} `json:"_meta,omitempty"`
}

// MCPClientCapabilities describes what the client supports.
type MCPClientCapabilities struct {
	Roots    *MCPCapRoots    `json:"roots,omitempty"`
	Sampling *struct{}       `json:"sampling,omitempty"`
}

// MCPCapRoots describes client roots capability.
type MCPCapRoots struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// MCPClientInfo identifies the connecting client.
type MCPClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPInitializeResult is the result of the initialize method.
type MCPInitializeResult struct {
	ProtocolVersion string                `json:"protocolVersion"`
	Capabilities    MCPServerCapabilities `json:"capabilities"`
	ServerInfo      MCPServerInfo         `json:"serverInfo"`
}

// MCPServerCapabilities describes what the server supports.
type MCPServerCapabilities struct {
	Tools     *MCPServerToolsCap     `json:"tools,omitempty"`
	Resources *MCPServerResourcesCap `json:"resources,omitempty"`
	Prompts   *MCPServerPromptsCap   `json:"prompts,omitempty"`
}

// MCPServerToolsCap is capabilities for tools.
type MCPServerToolsCap struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// MCPServerResourcesCap is capabilities for resources.
type MCPServerResourcesCap struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// MCPServerPromptsCap is capabilities for prompts.
type MCPServerPromptsCap struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// MCPServerInfo identifies this server.
type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPToolDefinition is a tool as returned by tools/list.
type MCPToolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema is the JSON Schema for a tool's parameters.
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// Property is a single parameter property in a tool's input schema.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
}

// MCPCallToolRequest is the params for tools/call.
type MCPCallToolRequest struct {
	Name      string         `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// MCPCallToolResult is the result of tools/call.
type MCPCallToolResult struct {
	Content []MCPTextContent `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

// MCPTextContent is a text content block in MCP messages.
type MCPTextContent struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

// MCPResourceDefinition is a resource as returned by resources/list.
type MCPResourceDefinition struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPResourceTemplate is a resource template with URI parameters.
type MCPResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPReadResourceRequest is the params for resources/read.
type MCPReadResourceRequest struct {
	URI string `json:"uri"`
}

// MCPReadResourceResult is the result of resources/read.
type MCPReadResourceResult struct {
	Contents []MCPResourceContent `json:"contents"`
}

// MCPResourceContent is a single content item in a resource response.
type MCPResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// MCPPromptDefinition is a prompt as returned by prompts/list.
type MCPPromptDefinition struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Arguments   []MCPPromptArgument `json:"arguments,omitempty"`
}

// MCPPromptArgument is a single argument definition for a prompt.
type MCPPromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ============================================================================
// Server
// ============================================================================

// Server is an MCP protocol server implementing SPEC-015 §5.
type Server struct {
	db   db.DB
	name string
	ver  string

	mu       sync.RWMutex
	sessions map[string]*mcpSession // sessionID → session
}

// mcpSession tracks per-connection MCP state.
type mcpSession struct {
	id       string
	eventCh  chan string   // SSE events to the client
	done     chan struct{} // closed when client disconnects
	stop     func()        // cancel function for cleanup
	sessionKey string      // API key for this MCP session (for scoping)
	authScope  string      // admin, session, readonly — from API key validation
	agentSessionID string  // if session-scoped, the session ID
}

// NewServer creates a new MCP server sharing the given database connection.
func NewServer(database db.DB) *Server {
	return &Server{
		db:       database,
		name:     "conscience",
		ver:      "0.1.0",
		sessions: make(map[string]*mcpSession),
	}
}

// ServerInfo returns identifying information about this MCP server.
func (s *Server) ServerInfo() MCPServerInfo {
	return MCPServerInfo{Name: s.name, Version: s.ver}
}

// ============================================================================
// Trace Markers (WI-020 — OpenTelemetry-compatible structured logging)
// ============================================================================

// traceID generates a unique trace identifier for request tracing.
func traceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// spanID generates a unique span identifier within a trace.
func spanID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// spanStart logs the beginning of a traced operation.
// Returns a closure that should be deferred to log the span end with duration.
//
// Usage:
//
//	end := spanStart("tools/list", "trace_abc", "span_xyz")
//	defer end(nil)
//
//	end := spanStart("tools/call", "trace_abc", "span_xyz")
//	defer end(err) // pass error to record failure
func spanStart(operation, traceID, spanID string) func(error) {
	attrs := []any{
		"trace_id", traceID,
		"span_id", spanID,
		"operation", operation,
	}
	slog.Info("mcp: span start", attrs...)
	start := time.Now()

	return func(err error) {
		duration := time.Since(start)
		endAttrs := append(attrs,
			"duration_ms", duration.Milliseconds(),
		)
		if err != nil {
			endAttrs = append(endAttrs, "error", err.Error())
			slog.Warn("mcp: span end (error)", endAttrs...)
		} else {
			slog.Info("mcp: span end", endAttrs...)
		}
	}
}

// ============================================================================
// SSE Transport (SPEC-015 §5.4)
// ============================================================================

// HandleSSE is the main SSE endpoint. MCP clients connect here first to
// establish a stream, then POST JSON-RPC messages to the returned session URL.
func (s *Server) HandleSSE(w http.ResponseWriter, r *http.Request) {
	tID := traceID()
	sID := spanID()
	endSSE := spanStart("mcp.HandleSSE", tID, sID)

	flusher, ok := w.(http.Flusher)
	if !ok {
		endSSE(fmt.Errorf("streaming not supported"))
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sessionID := generateSessionID()
	sess := &mcpSession{
		id:      sessionID,
		eventCh: make(chan string, 64),
		done:    make(chan struct{}),
	}

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	slog.Info("mcp: client connected", "session", sessionID, "trace_id", tID)

	// Send endpoint event so the client knows where to POST
	endpointURL := fmt.Sprintf("/mcp/message?sessionId=%s", sessionID)
	sseWrite(w, flusher, "endpoint", endpointURL)

	defer func() {
		endSSE(nil) // trace span end — SSE session lifecycle complete
		close(sess.done)
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		slog.Info("mcp: client disconnected", "session", sessionID)
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-sess.eventCh:
			if !sseWrite(w, flusher, "message", msg) {
				return
			}
		}
	}
}

// HandleMessage handles JSON-RPC requests POSTed to the message endpoint.
func (s *Server) HandleMessage(w http.ResponseWriter, r *http.Request) {
	tID := traceID()
	sID := spanID()
	endHTTP := spanStart("mcp.HandleMessage", tID, sID)
	defer func() {
		endHTTP(nil)
	}()

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "missing sessionId", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeError(w, nil, -32700, "Parse error", err.Error())
		return
	}

	if req.JSONRPC != "2.0" {
		s.writeError(w, req.ID, -32600, "Invalid Request", "jsonrpc must be 2.0")
		return
	}

	// Route to handler
	result, rpcErr := s.handleMethod(&req, sess)

	// Notifications have no ID — no response needed
	if req.ID == nil || rpcErr != nil && req.ID == nil {
		return
	}

	if rpcErr != nil {
		s.writeError(w, req.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
		return
	}

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ============================================================================
// Method Dispatch
// ============================================================================

// methodHandler returns a result and optional error for an MCP method.
// Emits trace markers (WI-020) for each method call.
func (s *Server) handleMethod(req *JSONRPCRequest, sess *mcpSession) (any, *JSONRPCErrObj) {
	tID := traceID()
	sID := spanID()
	endSpan := spanStart("mcp."+req.Method, tID, sID)
	defer func() {
		// Capture panic if any
		if r := recover(); r != nil {
			endSpan(fmt.Errorf("panic: %v", r))
			panic(r) // re-panic after logging
		}
	}()

	switch req.Method {
	// Lifecycle
	case "initialize":
		return s.handleInitialize(req, sess)
	case "notifications/initialized":
		return nil, nil // no-op, acknowledged

	// Tools
	case "tools/list":
		return s.handleToolsList(sess)
	case "tools/call":
		return s.handleToolsCall(req, sess)

	// Resources
	case "resources/list":
		return s.handleResourcesList(sess)
	case "resources/templates/list":
		return s.handleResourceTemplates(sess)
	case "resources/read":
		return s.handleResourcesRead(req, sess)

	// Prompts
	case "prompts/list":
		return s.handlePromptsList(sess)
	case "prompts/get":
		return s.handlePromptsGet(req, sess)

	// Ping
	case "ping":
		return map[string]any{}, nil

	default:
		err := &JSONRPCErrObj{Code: -32601, Message: "Method not found", Data: req.Method}
		endSpan(fmt.Errorf("method not found: %s", req.Method))
		return nil, err
	}
}

// handleInitialize handles the MCP initialize handshake.
func (s *Server) handleInitialize(req *JSONRPCRequest, sess *mcpSession) (any, *JSONRPCErrObj) {
	var init MCPInitializeRequest
	if err := json.Unmarshal(req.Params, &init); err != nil {
		return nil, &JSONRPCErrObj{Code: -32602, Message: "Invalid params", Data: err.Error()}
	}

	// Extract and validate auth from _meta.authorization
	if err := s.validateAuth(req, sess); err != nil {
		return nil, err
	}

	slog.Info("mcp: initialize", "client", init.ClientInfo.Name, "client_version", init.ClientInfo.Version, "protocol", init.ProtocolVersion)

	return MCPInitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: MCPServerCapabilities{
			Tools:     &MCPServerToolsCap{ListChanged: false},
			Resources: &MCPServerResourcesCap{Subscribe: false, ListChanged: false},
			Prompts:   &MCPServerPromptsCap{ListChanged: false},
		},
		ServerInfo: s.ServerInfo(),
	}, nil
}

// ============================================================================
// Helpers
// ============================================================================

// generateSessionID creates a random session identifier.
func generateSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// sseWrite sends an SSE event. Returns false if the write failed.
func sseWrite(w http.ResponseWriter, flusher http.Flusher, event, data string) bool {
	_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	if err != nil {
		return false
	}
	flusher.Flush()
	return true
}

// writeError sends a JSON-RPC error response.
func (s *Server) writeError(w http.ResponseWriter, id any, code int, message string, data any) {
	resp := JSONRPCErrorResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: JSONRPCErrObj{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // JSON-RPC errors are HTTP 200
	json.NewEncoder(w).Encode(resp)
}

// Handler returns an http.Handler that serves SSE at /mcp/sse and message
// posts at /mcp/message. Use this with http.ListenAndServe or chi.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp/sse", s.HandleSSE)
	mux.HandleFunc("/mcp/message", s.HandleMessage)
	return mux
}
