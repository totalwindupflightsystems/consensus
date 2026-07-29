// Package h3 implements the H3 brain-swap protocol shim for Consensus.
//
// The shim translates H3's /v1/process and /v1/result into Consensus-native
// session operations. It enables Hermes to use Consensus as an external agent brain
// via the H3 protocol — the same protocol used by any H3-compliant harness.
//
// Architecture:
//
//	Hermes → POST /v1/process → H3 Shim → Consensus API (create session + send message)
//	Hermes → POST /v1/result  → H3 Shim → Consensus API (feed back tool result)
//	Loop continues until Consensus returns decision=end
//
// The shim runs in-process alongside Consensus, using the same database connection.
// It does NOT bypass the native API — it calls api.Service, the same business
// logic the REST API and opencode shim use.
package h3

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// H3 Protocol Types (subset — matches get-h3/protocol v1.0)
// ============================================================================

type DecisionType string

const (
	DecisionToolCall DecisionType = "tool_call"
	DecisionLLMCall  DecisionType = "llm_call"
	DecisionText     DecisionType = "text"
	DecisionWait     DecisionType = "wait"
	DecisionDelegate DecisionType = "delegate"
	DecisionEnd      DecisionType = "end"
)

type Decision struct {
	Decision   DecisionType `json:"decision"`
	DecisionID string       `json:"decision_id"`
	ToolCall   *ToolCall    `json:"tool_call,omitempty"`
	LLMCall    *LLMCall     `json:"llm_call,omitempty"`
	Text       *TextResp    `json:"text,omitempty"`
	Wait       *Wait        `json:"wait,omitempty"`
	Delegate   *Delegate    `json:"delegate,omitempty"`
	End        *End         `json:"end,omitempty"`
	Error      *ErrorDetail `json:"error,omitempty"`
}

type ToolCall struct {
	Name      string `json:"name"`
	Params    any    `json:"params"`
	Reasoning string `json:"reasoning,omitempty"`
}

type LLMCall struct {
	Model        string       `json:"model"`
	SystemPrompt string       `json:"system_prompt,omitempty"`
	Messages     []LLMMessage `json:"messages"`
	Temperature  *float64     `json:"temperature,omitempty"`
	MaxTokens    *int         `json:"max_tokens,omitempty"`
}

type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type TextResp struct {
	Content  string `json:"content"`
	Finished bool   `json:"finished"`
}

type Wait struct {
	Reason          string `json:"reason"`
	DurationSeconds *int   `json:"duration_seconds,omitempty"`
	PollEndpoint    string `json:"poll_endpoint,omitempty"`
}

type Delegate struct {
	Agent    string `json:"agent,omitempty"`
	Task     string `json:"task"`
	Context  string `json:"context,omitempty"`
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`
}

type EndReason string

const (
	EndTaskComplete EndReason = "task_complete"
	EndUserRequest  EndReason = "user_requested"
	EndError        EndReason = "error"
	EndTimeout      EndReason = "timeout"
)

type End struct {
	Reason  EndReason `json:"reason"`
	Summary string    `json:"summary,omitempty"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ============================================================================
// H3 Request Types
// ============================================================================

type ProcessRequest struct {
	SessionID string   `json:"session_id"`
	Message   Message  `json:"message"`
	Identity  Identity `json:"identity"`
	Context   Context  `json:"context"`
}

type Message struct {
	Role        string       `json:"role"`
	Content     string       `json:"content"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Timestamp   string       `json:"timestamp"`
}

type Attachment struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	MimeType string `json:"mime_type"`
}

type Identity struct {
	Platform string `json:"platform"`
	ChatID   string `json:"chat_id"`
	ThreadID string `json:"thread_id,omitempty"`
	UserName string `json:"user_name"`
	UserID   string `json:"user_id"`
}

type Context struct {
	History      []HistoryEntry `json:"history"`
	Tools        []Tool         `json:"tools"`
	Models       []Model        `json:"models"`
	Memory       string         `json:"memory,omitempty"`
	Skills       []string       `json:"skills,omitempty"`
	Config       Config         `json:"config"`
	SessionState SessionState   `json:"session_state"`
}

type HistoryEntry struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type Model struct {
	Name            string  `json:"name"`
	Provider        string  `json:"provider"`
	CostPer1kInput  float64 `json:"cost_per_1k_input,omitempty"`
	CostPer1kOutput float64 `json:"cost_per_1k_output,omitempty"`
	ContextWindow   int     `json:"context_window"`
}

type Config struct {
	MaxIterations  int    `json:"max_iterations"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	ProjectDir     string `json:"project_dir,omitempty"`
}

type SessionState struct {
	TurnCount      int     `json:"turn_count"`
	TotalToolCalls int     `json:"total_tool_calls"`
	TotalLLMCalls  int     `json:"total_llm_calls"`
	CostSoFar      float64 `json:"cost_so_far"`
	StartedAt      string  `json:"started_at"`
}

type ResultRequest struct {
	SessionID  string `json:"session_id"`
	DecisionID string `json:"decision_id"`
	Result     Result `json:"result"`
}

type Result struct {
	Type       string  `json:"type"`
	ToolName   string  `json:"tool_name,omitempty"`
	Data       any     `json:"data,omitempty"`
	DurationMs float64 `json:"duration_ms,omitempty"`
	Success    bool    `json:"success"`
}

// ============================================================================
// Consensus API Interfaces (minimal — what the shim needs)
// ============================================================================

// SessionService is the subset of the Consensus API service the H3 shim needs.
type SessionService interface {
	CreateSession(ctx context.Context, agentName, goal, modelID, projectID string, contextBudget int) (sessionID string, status string, err error)
	GetSession(ctx context.Context, id string) (status string, err error)
	ProcessMessage(ctx context.Context, sessionID, message string) (response string, err error)
	FeedToolResult(ctx context.Context, sessionID, toolName string, success bool, data any) (response string, err error)
}

// ============================================================================
// Server — H3 Protocol Shim
// ============================================================================

// Server implements the H3 protocol endpoints that Hermes calls.
// It translates H3 requests into Consensus session operations.
type Server struct {
	db  db.DB
	svc SessionService
	mux *http.ServeMux

	// Session tracking — maps H3 session_id → Consensus session_id
	sessions map[string]string // h3_session_id → consensus_session_id
	mu       sync.RWMutex

	// Turn tracking
	turns map[string]int // h3_session_id → turn count
}

// NewServer creates an H3 protocol shim server.
func NewServer(database db.DB, svc SessionService) *Server {
	s := &Server{
		db:       database,
		svc:      svc,
		mux:      http.NewServeMux(),
		sessions: make(map[string]string),
		turns:    make(map[string]int),
	}

	s.mux.HandleFunc("/v1/health", s.handleHealth)
	s.mux.HandleFunc("/v1/process", s.handleProcess)
	s.mux.HandleFunc("/v1/result", s.handleResult)
	s.mux.HandleFunc("/v1/cancel", s.handleCancel)

	return s
}

// Handler returns the http.Handler for mounting on the consensus router.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// ============================================================================
// Endpoint Handlers
// ============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":           "ok",
		"version":          "1.0.0",
		"transport":        "rest",
		"protocol_version": "1.0",
		"capabilities":     []string{"text", "tool_call", "llm_call", "end"},
	})
}

func (s *Server) handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "INVALID_REQUEST", "failed to decode request body: "+err.Error())
		return
	}

	slog.Info("h3: process", "session_id", req.SessionID, "message", truncate(req.Message.Content, 80))

	// Map H3 session → Consensus session (create if new)
	consensusID, isNew := s.getOrCreateConsensusSession(r.Context(), &req)

	// If this is a new session or we need to send the message to Consensus
	response, err := s.svc.ProcessMessage(r.Context(), consensusID, req.Message.Content)
	if err != nil {
		slog.Error("h3: process message failed", "session_id", req.SessionID, "error", err)
		s.writeDecision(w, Decision{
			Decision:   DecisionEnd,
			DecisionID: uuid.NewString(),
			End:        &End{Reason: EndError, Summary: err.Error()},
		})
		return
	}

	// Increment turn count
	s.mu.Lock()
	s.turns[req.SessionID]++
	turns := s.turns[req.SessionID]
	s.mu.Unlock()

	decisionID := uuid.NewString()

	// Check if response looks like a tool call request
	// Consensus agents return structured JSON — parse it to determine decision type
	toolReq := s.parseToolCall(response)
	if toolReq != nil {
		s.writeDecision(w, Decision{
			Decision:   DecisionToolCall,
			DecisionID: decisionID,
			ToolCall:   toolReq,
		})
		return
	}

	// Default: text response
	finished := turns >= req.Context.Config.MaxIterations || stringsContains(response, "DONE")
	if isNew {
		_ = isNew // suppress unused warning
	}

	s.writeDecision(w, Decision{
		Decision:   DecisionText,
		DecisionID: decisionID,
		Text: &TextResp{
			Content:  response,
			Finished: finished,
		},
	})
}

func (s *Server) handleResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "INVALID_REQUEST", "failed to decode request body: "+err.Error())
		return
	}

	slog.Info("h3: result", "session_id", req.SessionID, "decision_id", req.DecisionID, "type", req.Result.Type)

	consensusID, ok := s.getConsensusSession(req.SessionID)
	if !ok {
		s.writeDecision(w, Decision{
			Decision:   DecisionEnd,
			DecisionID: uuid.NewString(),
			End:        &End{Reason: EndError, Summary: fmt.Sprintf("session not found: %s", req.SessionID)},
		})
		return
	}

	// Feed the tool result back to Consensus
	response, err := s.svc.FeedToolResult(r.Context(), consensusID, req.Result.ToolName, req.Result.Success, req.Result.Data)
	if err != nil {
		slog.Error("h3: feed tool result failed", "error", err)
		s.writeDecision(w, Decision{
			Decision:   DecisionEnd,
			DecisionID: uuid.NewString(),
			End:        &End{Reason: EndError, Summary: err.Error()},
		})
		return
	}

	// Check if we're done
	if response == "" || stringsContains(response, "DONE") || stringsContains(response, "COMPLETE") {
		s.writeDecision(w, Decision{
			Decision:   DecisionEnd,
			DecisionID: uuid.NewString(),
			End:        &End{Reason: EndTaskComplete, Summary: response},
		})
		return
	}

	// Check for tool calls in the response
	toolReq := s.parseToolCall(response)
	if toolReq != nil {
		s.writeDecision(w, Decision{
			Decision:   DecisionToolCall,
			DecisionID: uuid.NewString(),
			ToolCall:   toolReq,
		})
		return
	}

	// Continue with text
	s.writeDecision(w, Decision{
		Decision:   DecisionText,
		DecisionID: uuid.NewString(),
		Text:       &TextResp{Content: response, Finished: false},
	})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Minimal: acknowledge cancellation
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}

// ============================================================================
// Session Mapping
// ============================================================================

func (s *Server) getOrCreateConsensusSession(ctx context.Context, req *ProcessRequest) (string, bool) {
	s.mu.RLock()
	consensusID, exists := s.sessions[req.SessionID]
	s.mu.RUnlock()

	if exists {
		return consensusID, false
	}

	// Create new Consensus session
	agentName := fmt.Sprintf("h3-%s", req.Identity.UserName)
	goal := req.Message.Content
	modelID := "deepseek-v4-pro" // default, overridden by context if available
	if len(req.Context.Models) > 0 {
		modelID = req.Context.Models[0].Name
	}

	id, _, err := s.svc.CreateSession(ctx, agentName, goal, modelID, "", 200000)
	if err != nil {
		slog.Error("h3: failed to create consensus session", "error", err)
		// Fall back to a UUID — the message processing will handle it
		id = uuid.NewString()
	}

	s.mu.Lock()
	s.sessions[req.SessionID] = id
	s.mu.Unlock()

	return id, true
}

func (s *Server) getConsensusSession(h3SessionID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.sessions[h3SessionID]
	return id, ok
}

// ============================================================================
// Helpers
// ============================================================================

// parseToolCall attempts to extract a tool call request from a Consensus agent response.
// Consensus agents emit JSON with potential tool_requests or system_actions arrays.
// We look for a tool call pattern and convert it to H3 format.
func (s *Server) parseToolCall(response string) *ToolCall {
	// Try to parse as JSON — Consensus agents return structured output
	var parsed map[string]any
	if err := json.Unmarshal([]byte(response), &parsed); err != nil {
		return nil
	}

	// Check for tool_requests array (Consensus format)
	if toolReqs, ok := parsed["tool_requests"].([]any); ok && len(toolReqs) > 0 {
		if tr, ok := toolReqs[0].(map[string]any); ok {
			name, _ := tr["tool_name"].(string)
			params, _ := tr["parameters"]
			reasoning, _ := parsed["internal_monologue"].(string)
			return &ToolCall{
				Name:      name,
				Params:    params,
				Reasoning: reasoning,
			}
		}
	}

	return nil
}

func (s *Server) writeDecision(w http.ResponseWriter, d Decision) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

func (s *Server) writeError(w http.ResponseWriter, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]any{
		"error": ErrorDetail{Code: code, Message: message},
	})
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func stringsContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
