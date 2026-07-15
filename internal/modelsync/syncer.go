// Package modelsync syncs the model_registry from models.dev.
//
// Keeps static entries intact (sync_source='static') and only touches
// models.dev-sourced entries. Provides a periodic sync loop for
// --auto-sync mode.
package modelsync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wojons/consensus/internal/db"
)

const modelsDevURL = "https://models.dev/api/models"

// ModelEntry mirrors a models.dev API response entry.
type ModelEntry struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Provider      string   `json:"provider"`
	ContextWindow int      `json:"context_window"`
	MaxOutput     int      `json:"max_output"`
	Pricing       struct {
		Input  float64 `json:"input"`
		Output float64 `json:"output"`
	} `json:"pricing"`
	Capabilities []string `json:"capabilities"`
}

// SyncResult reports what a sync cycle did.
type SyncResult struct {
	Added   int      `json:"added"`
	Updated int      `json:"updated"`
	Removed int      `json:"removed"`
	Errors  []string `json:"errors,omitempty"`
}

// Syncer fetches from models.dev and writes to model_registry.
type Syncer struct {
	database db.DB
	client   *http.Client
}

// New creates a Syncer with the given database connection.
func New(database db.DB) *Syncer {
	return &Syncer{
		database: database,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Sync fetches models from models.dev and updates the registry.
// Entries with sync_source='static' are never overwritten.
func (s *Syncer) Sync(ctx context.Context) (*SyncResult, error) {
	result := &SyncResult{}

	entries, err := s.fetchModels(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("fetch: %v", err))
		return result, err
	}

	for _, e := range entries {
		tier := classifyTier(e)
		existing, err := s.lookup(ctx, e.ID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("lookup %s: %v", e.ID, err))
			continue
		}

		if existing != nil && existing["sync_source"] == "static" {
			continue // never overwrite static entries
		}

		if existing != nil {
			err = s.update(ctx, e, tier)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("update %s: %v", e.ID, err))
				continue
			}
			result.Updated++
		} else {
			err = s.insert(ctx, e, tier)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("insert %s: %v", e.ID, err))
				continue
			}
			result.Added++
		}
	}

	return result, nil
}

func (s *Syncer) fetchModels(_ context.Context) ([]ModelEntry, error) {
	resp, err := s.client.Get(modelsDevURL)
	if err != nil {
		return nil, fmt.Errorf("models.dev request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev returned %d", resp.StatusCode)
	}

	var entries []ModelEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode models.dev response: %w", err)
	}
	return entries, nil
}

func (s *Syncer) lookup(ctx context.Context, modelID string) (map[string]string, error) {
	results, err := s.database.Query(ctx,
		"SELECT sync_source FROM model_registry WHERE model_id = $1", modelID)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	src, _ := results[0]["sync_source"].(string)
	return map[string]string{"sync_source": src}, nil
}

func mapProvider(modelsDevProvider string) string {
	switch strings.ToLower(modelsDevProvider) {
	case "openai":
		return "openai"
	case "anthropic":
		return "anthropic"
	default:
		return "openrouter"
	}
}

func (s *Syncer) insert(ctx context.Context, e ModelEntry, tier int) error {
	tags := capabilitiesToTags(e.Capabilities)
	now := time.Now().UTC().Format(time.RFC3339)
	tagsJSON, _ := json.Marshal(tags)

	return s.database.Exec(ctx,
		`INSERT INTO model_registry
			(model_id, tier, max_context, cost_per_m_in, cost_per_m_out,
			 classifier_tags, enabled, sync_source, synced_at, provider_id)
		 VALUES ($1, $2, $3, $4, $5, $6, TRUE, 'models.dev', $7, $8)`,
		e.ID, tier, e.ContextWindow, e.Pricing.Input, e.Pricing.Output,
		string(tagsJSON), now, mapProvider(e.Provider),
	)
}

func (s *Syncer) update(ctx context.Context, e ModelEntry, tier int) error {
	tags := capabilitiesToTags(e.Capabilities)
	now := time.Now().UTC().Format(time.RFC3339)
	tagsJSON, _ := json.Marshal(tags)

	return s.database.Exec(ctx,
		`UPDATE model_registry SET
			tier = $2, max_context = $3, cost_per_m_in = $4, cost_per_m_out = $5,
			classifier_tags = $6, synced_at = $7, provider_id = $8
		 WHERE model_id = $1 AND sync_source = 'models.dev'`,
		e.ID, tier, e.ContextWindow, e.Pricing.Input, e.Pricing.Output,
		string(tagsJSON), now, mapProvider(e.Provider),
	)
}

// AutoSyncLoop runs Sync on interval. For --auto-sync mode.
func (s *Syncer) AutoSyncLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run once at startup.
	if result, err := s.Sync(ctx); err != nil {
		slog.Warn("modelsync: initial sync failed", "error", err)
	} else {
		slog.Info("modelsync: initial sync complete",
			"added", result.Added, "updated", result.Updated)
	}

	for {
		select {
		case <-ticker.C:
			result, err := s.Sync(ctx)
			if err != nil {
				slog.Warn("modelsync: sync failed", "error", err)
				continue
			}
			if result.Added > 0 || result.Updated > 0 {
				slog.Info("modelsync: sync complete",
					"added", result.Added, "updated", result.Updated)
			}
		case <-ctx.Done():
			slog.Info("modelsync: stopping auto-sync")
			return
		}
	}
}

// RegisterIfMissing auto-registers a model from models.dev if not in registry.
func (s *Syncer) RegisterIfMissing(ctx context.Context, modelID string) error {
	existing, err := s.lookup(ctx, modelID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	entries, err := s.fetchModels(ctx)
	if err != nil {
		// Can't fetch — register as unknown.
		return s.insert(ctx, ModelEntry{
			ID:            modelID,
			Provider:      "unknown",
			ContextWindow: 128000,
		}, 1)
	}

	for _, e := range entries {
		if e.ID == modelID {
			return s.insert(ctx, e, classifyTier(e))
		}
	}

	return s.insert(ctx, ModelEntry{
		ID:            modelID,
		Provider:      "unknown",
		ContextWindow: 128000,
	}, 1)
}

func classifyTier(e ModelEntry) int {
	premium := []string{"gpt-", "claude-", "gemini-2.5", "grok-3"}
	for _, p := range premium {
		if strings.Contains(strings.ToLower(e.ID), strings.ToLower(p)) {
			return 3
		}
	}
	if e.ContextWindow < 32000 || e.Pricing.Input < 0.15 {
		return 1
	}
	return 2
}

func capabilitiesToTags(caps []string) []string {
	tags := []string{"chat"}
	for _, c := range caps {
		switch strings.ToLower(c) {
		case "embedding", "embeddings":
			tags = append(tags, "embedding")
		case "vision", "image":
			tags = append(tags, "vision")
		case "function_calling", "tools":
			tags = append(tags, "function_calling")
		case "code":
			tags = append(tags, "code")
		}
	}
	return tags
}
