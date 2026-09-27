// Package opencode: service adapter that bridges api.Service → opencode.Service
//
// This adapter wraps the api.Service to match the opencode.Service interface,
// avoiding circular imports (shim/opencode ↔ api).
//
// File operations (FindFiles, ReadFile, GetGitStatus) are implemented via the
// Go standard library since the shim runs on the same host filesystem.
//
// axiom:trace work_item=spec-017-hardening-01 spec=specs/017-ui-adapter-layer.md plan=phase-1/task-1/step-2 impl=internal/shim/opencode/service_adapter.go
package opencode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/wojons/consensus/internal/api"
)

// ServiceAdapter wraps api.Service to satisfy opencode.Service.
type ServiceAdapter struct {
	svc *api.Service
}

// NewServiceAdapter creates a shim-compatible service wrapper around the API service.
func NewServiceAdapter(svc *api.Service) *ServiceAdapter {
	return &ServiceAdapter{svc: svc}
}

func (a *ServiceAdapter) CreateSession(ctx context.Context, input SessionCreateInput) (*SessionCreateResult, error) {
	result, err := a.svc.Sessions.CreateSession(ctx, api.CreateSessionInput{
		AgentName:     input.AgentName,
		Goal:          input.Goal,
		ModelID:       input.ModelID,
		ContextBudget: input.ContextBudget,
	})
	if err != nil {
		return nil, err
	}
	return &SessionCreateResult{
		SessionID: result.SessionID,
		Status:    result.Status,
		APIKey:    result.APIKey,
		CreatedAt: result.CreatedAt,
	}, nil
}

func (a *ServiceAdapter) GetSession(ctx context.Context, id string) (*SessionResult, error) {
	resp, err := a.svc.Sessions.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	return &SessionResult{
		ID:            resp.ID,
		ParentID:      resp.ParentID,
		AgentName:     resp.AgentName,
		ModelID:       resp.ModelID,
		Status:        resp.Status,
		Goal:          resp.Goal,
		ContextBudget: resp.ContextBudget,
		TokensUsedIn:  resp.TokensUsedIn,
		TokensUsedOut: resp.TokensUsedOut,
		Iteration:     resp.Iteration,
		HeartbeatAt:   resp.HeartbeatAt,
		CreatedAt:     resp.CreatedAt,
		CompletedAt:   resp.CompletedAt,
	}, nil
}

func (a *ServiceAdapter) UpdateSession(ctx context.Context, id string, action string) error {
	return a.svc.Sessions.UpdateSession(ctx, id, action)
}

func (a *ServiceAdapter) DeleteSession(ctx context.Context, id string) error {
	return a.svc.Sessions.DeleteSession(ctx, id)
}

const messageResponsePollInterval = 100 * time.Millisecond

func (a *ServiceAdapter) SendMessage(ctx context.Context, input MessageSendInput) (*MessageSendResult, error) {
	_, iteration, err := a.svc.Sessions.GetSessionStatus(ctx, input.SessionID)
	if err != nil {
		return nil, err
	}
	targetIteration := iteration + 1

	if err := a.svc.Messages.SendMessage(ctx, api.SendMessageInput{
		SessionID: input.SessionID,
		Content:   input.Content,
		MsgType:   input.MsgType,
	}); err != nil {
		return nil, err
	}

	return a.waitForMessageResponse(ctx, input.SessionID, targetIteration)
}

// waitForMessageResponse returns only output from the iteration assigned to
// the submitted message. This prevents a late response from an earlier turn
// from satisfying a newer synchronous request.
func (a *ServiceAdapter) waitForMessageResponse(ctx context.Context, sessionID string, targetIteration int64) (*MessageSendResult, error) {
	ticker := time.NewTicker(messageResponsePollInterval)
	defer ticker.Stop()

	for {
		events, err := a.svc.Messages.ListMessages(ctx, sessionID, 50)
		if err != nil {
			return nil, fmt.Errorf("list responses for session %s: %w", sessionID, err)
		}
		status, _, err := a.svc.Sessions.GetSessionStatus(ctx, sessionID)
		if err != nil {
			return nil, err
		}

		for _, event := range events {
			if event.IterationCreated != targetIteration || strings.TrimSpace(event.Content) == "" {
				continue
			}
			switch event.Type {
			case "agent_response":
				return &MessageSendResult{Content: event.Content}, nil
			case "text_block":
				// The harness persists message_to_user as text_block in the same
				// transaction that settles the session to idle. Waiting for idle
				// avoids returning an intermediate text block from the turn.
				if status == "idle" {
					return &MessageSendResult{Content: event.Content}, nil
				}
			}
		}

		if status == "failed" {
			return nil, fmt.Errorf("session %s failed before producing an agent response for iteration %d", sessionID, targetIteration)
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for agent response for session %s iteration %d: %w", sessionID, targetIteration, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (a *ServiceAdapter) GetConfig(ctx context.Context) (map[string]string, error) {
	return a.svc.Config.GetConfig(ctx)
}

func (a *ServiceAdapter) UpdateConfig(ctx context.Context, settings map[string]string) error {
	return a.svc.Config.UpdateConfig(ctx, settings)
}

// FindFiles returns files matching a glob pattern via path/filepath.Glob.
func (a *ServiceAdapter) FindFiles(ctx context.Context, pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	// Resolve to absolute paths for consistency
	abs := make([]string, len(matches))
	for i, m := range matches {
		abs[i], _ = filepath.Abs(m)
	}
	return abs, nil
}

// ReadFile reads the contents of a file at the given path.
func (a *ServiceAdapter) ReadFile(ctx context.Context, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetGitStatus runs "git status --porcelain" and returns the results.
func (a *ServiceAdapter) GetGitStatus(ctx context.Context) (map[string]any, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		// If not a git repo or git unavailable, return empty status
		return map[string]any{
			"status":  "unavailable",
			"message": err.Error(),
			"changes": []string{},
		}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = []string{}
	}
	return map[string]any{
		"status":  "ok",
		"changes": lines,
	}, nil
}
