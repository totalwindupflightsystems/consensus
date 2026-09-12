// Package h3: service adapter that bridges api.Service → h3.SessionService.
//
// Same pattern as shim/opencode/service_adapter.go: wraps the api.Service to
// satisfy the shim's service interface without creating an import cycle
// (shim/h3 ← api).
//
// Consensus runs its agent loop ASYNCHRONOUSLY (heartbeat claims the session,
// runs the iteration, returns it to idle), while the H3 protocol is
// SYNCHRONOUS (each /v1/process call must return a decision). This adapter
// bridges the two:
//
//	ProcessMessage:  SendMessage → poll GetSessionStatus until idle →
//	                 read the agent's final monologue (memory_events
//	                 type='text_block', persisted by the harness "so H3
//	                 clients can retrieve it") → return it.
//	FeedToolResult:  same, with the result stored as a user_message first.
//
// If no LLM key is configured the heartbeat loop has nothing to run; the
// poll times out and the caller sees the canned-session behavior of the
// shim's fallback path.
package h3

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/wojons/consensus/internal/api"
)

// poll settings for the async agent loop.
const (
	pollInterval   = 300 * time.Millisecond
	pollTimeout    = 90 * time.Second
	settleChecks   = 3 // consecutive idle checks before treating the turn as done
	outputLookback = 20
)

// ServiceAdapter wraps api.Service to satisfy h3.SessionService.
type ServiceAdapter struct {
	svc *api.Service
}

// NewServiceAdapter creates an H3-shim-compatible service wrapper around the API service.
func NewServiceAdapter(svc *api.Service) *ServiceAdapter {
	return &ServiceAdapter{svc: svc}
}

func (a *ServiceAdapter) CreateSession(ctx context.Context, agentName, goal, modelID, projectID string, contextBudget int) (string, string, error) {
	result, err := a.svc.Sessions.CreateSession(ctx, api.CreateSessionInput{
		AgentName:     agentName,
		Goal:          goal,
		ModelID:       modelID,
		ContextBudget: contextBudget,
	})
	if err != nil {
		return "", "", err
	}
	return result.SessionID, result.Status, nil
}

func (a *ServiceAdapter) GetSession(ctx context.Context, id string) (string, error) {
	resp, err := a.svc.Sessions.GetSession(ctx, id)
	if err != nil {
		return "", err
	}
	return resp.Status, nil
}

// ProcessMessage sends the user message and waits for the agent loop to
// finish, returning the agent's final monologue.
func (a *ServiceAdapter) ProcessMessage(ctx context.Context, sessionID, message string) (string, error) {
	if err := a.svc.Messages.SendMessage(ctx, api.SendMessageInput{
		SessionID: sessionID,
		Content:   message,
		MsgType:   "user_instruction",
	}); err != nil {
		return "", err
	}
	return a.waitForTurn(ctx, sessionID)
}

// FeedToolResult feeds a tool result back and waits for the next agent turn.
// Results are stored as a user_message memory event whose content is a JSON
// envelope (the shim's parseToolCall reads tool_requests JSON from the
// returned text).
//
// Planning suspends to 'tool_exec' when the LLM requests tools
// (handleToolCallDuringPlanning) and waits for the external executor to hand
// the session back — a bare SendMessage is NOT enough because the wake path
// only fires for idle/booting sessions (service.go SendMessage). So after
// storing the result we re-wake the session via UpdateSessionFields
// (status='thinking' + heartbeat bump), the same convention every other
// entry point uses.
func (a *ServiceAdapter) FeedToolResult(ctx context.Context, sessionID, toolName string, success bool, data any) (string, error) {
	payload := fmt.Sprintf(`{"tool_result":{"tool_name":%q,"success":%t,"data":%s}}`,
		toolName, success, marshalData(data))
	if err := a.svc.Messages.SendMessage(ctx, api.SendMessageInput{
		SessionID: sessionID,
		Content:   payload,
		MsgType:   "user_instruction",
	}); err != nil {
		return "", err
	}
	// Re-wake suspended sessions: tool_exec (planning suspended for this
	// result), executing/waiting_sub (executor flow). Never touch
	// paused/failed — those are deliberate holds.
	if status, err := a.GetSession(ctx, sessionID); err == nil {
		switch status {
		case "tool_exec", "executing", "waiting_sub":
			if err := a.svc.Sessions.UpdateSessionFields(ctx, sessionID, map[string]string{
				"status": "thinking",
			}); err != nil {
				return "", fmt.Errorf("h3: re-wake %s session: %w", status, err)
			}
		}
	}
	return a.waitForTurn(ctx, sessionID)
}

// waitForTurn polls the session status until the agent loop has settled back
// to idle, then returns the newest text_block memory event (the final
// monologue of the turn).
func (a *ServiceAdapter) waitForTurn(ctx context.Context, sessionID string) (string, error) {
	deadline := time.Now().Add(pollTimeout)

	// Wait for the loop to wake (status leaves booting/idle) and settle back.
	consecutiveIdle := 0
	woke := false
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		status, err := a.GetSession(ctx, sessionID)
		if err != nil {
			return "", err
		}
		switch status {
		case "booting", "idle":
			if woke || status == "idle" {
				consecutiveIdle++
			}
			if status == "idle" && consecutiveIdle >= settleChecks {
				return a.latestOutput(ctx, sessionID)
			}
		default:
			// thinking / planning / tool_exec / executing / waiting_sub — loop is live
			woke = true
			consecutiveIdle = 0
		}
		time.Sleep(pollInterval)
	}
	return "", fmt.Errorf("consensus agent turn did not settle within %s (session %s)", pollTimeout, sessionID)
}

// latestOutput returns the newest text_block memory event for the session —
// the harness persists the agent's final monologue there when a turn ends.
func (a *ServiceAdapter) latestOutput(ctx context.Context, sessionID string) (string, error) {
	events, err := a.svc.Messages.ListMessages(ctx, sessionID, outputLookback)
	if err != nil {
		return "", err
	}
	// ListMessages returns newest-first; find the most recent agent text.
	sort.SliceStable(events, func(i, j int) bool { return events[i].ID > events[j].ID })
	for _, ev := range events {
		if ev.Type == "text_block" {
			return ev.Content, nil
		}
	}
	// No text_block yet — the turn may have ended in a tool request (stored
	// as tool_requests JSON in the audit) or produced nothing printable.
	// Return empty: the shim handles empty as turn-end/parse-failure.
	slog.Debug("h3: no text_block output for session", "session_id", sessionID)
	return "", nil
}

// marshalData serializes tool-result data compactly for the feedback payload.
func marshalData(data any) string {
	if data == nil {
		return "null"
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "null"
	}
	return string(b)
}
