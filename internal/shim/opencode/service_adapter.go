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
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wojons/conscientiousness/internal/api"
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

func (a *ServiceAdapter) SendMessage(ctx context.Context, input MessageSendInput) error {
	return a.svc.Messages.SendMessage(ctx, api.SendMessageInput{
		SessionID: input.SessionID,
		Content:   input.Content,
		MsgType:   input.MsgType,
	})
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
