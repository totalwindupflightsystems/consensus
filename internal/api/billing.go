// Package api: billing, config, metrics, and auth endpoint handlers (SPEC-015 §3.6-3.8).
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/015-api-and-mcp.md plan=phase-2/task-2-6/step-2-6-1 impl=internal/api/billing.go
package api

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
)

// Note: sha256Hash is defined in server.go and generateAPIKey/newUUID are in sessions.go.
// These are package-level functions accessible from all files in the api package.

// ============================================================================
// GET /api/v1/sessions/{id}/billing — per-session cost breakdown (SPEC-015 §3.6)
// ============================================================================

func (s *Server) handleGetSessionBilling(w http.ResponseWriter, r *http.Request, id string) {
	scope := GetAuthScope(r)
	sessionID := GetAuthSessionID(r)

	if scope == "session" && sessionID != id {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "session key can only access its own billing")
		return
	}

	ctx := r.Context()

	// Verify session exists
	_, err := s.db.QueryRow(ctx, `SELECT id FROM sessions WHERE id = $1`, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "session not found")
		return
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, session_id, iteration, model_id, category,
		        prompt_tokens, completion_tokens, cache_read_tokens, cache_write_tokens,
		        cost_usd, recorded_at
		 FROM agent_billing
		 WHERE session_id = $1
		 ORDER BY iteration ASC, recorded_at ASC`, id)
	if err != nil {
		slog.Error("api: failed to query billing", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to query billing")
		return
	}

	type billingEntry struct {
		ID               int64   `json:"id"`
		SessionID        string  `json:"session_id"`
		Iteration        int64   `json:"iteration"`
		ModelID          string  `json:"model_id"`
		Category         string  `json:"category"`
		PromptTokens     int64   `json:"prompt_tokens"`
		CompletionTokens int64   `json:"completion_tokens"`
		CacheReadTokens  int64   `json:"cache_read_tokens"`
		CacheWriteTokens int64   `json:"cache_write_tokens"`
		CostUSD          float64 `json:"cost_usd"`
		RecordedAt       string  `json:"recorded_at"`
	}

	billings := make([]billingEntry, 0, len(rows))
	var totalCost float64
	var totalPrompt, totalCompletion int64

	for _, row := range rows {
		cost := toFloat64(row["cost_usd"])
		entry := billingEntry{
			ID:               toInt64(row["id"]),
			SessionID:        toString(row["session_id"]),
			Iteration:        toInt64(row["iteration"]),
			ModelID:          toString(row["model_id"]),
			Category:         toString(row["category"]),
			PromptTokens:     toInt64(row["prompt_tokens"]),
			CompletionTokens: toInt64(row["completion_tokens"]),
			CacheReadTokens:  toInt64(row["cache_read_tokens"]),
			CacheWriteTokens: toInt64(row["cache_write_tokens"]),
			CostUSD:          cost,
			RecordedAt:       toString(row["recorded_at"]),
		}
		billings = append(billings, entry)
		totalCost += cost
		totalPrompt += entry.PromptTokens
		totalCompletion += entry.CompletionTokens
	}

	// Round total cost to 6 decimal places
	totalCost = math.Round(totalCost*1e6) / 1e6

	writeJSON(w, map[string]any{
		"session_id":         id,
		"total_cost_usd":     totalCost,
		"total_prompt_tokens": totalPrompt,
		"total_completion_tokens": totalCompletion,
		"entries":            billings,
	})
}

// ============================================================================
// GET /api/v1/config — system configuration (SPEC-015 §3.7)
// ============================================================================

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	scope := GetAuthScope(r)
	if scope != "admin" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "admin scope required")
		return
	}

	ctx := r.Context()
	rows, err := s.db.Query(ctx, `SELECT key, value FROM system_settings ORDER BY key`)
	if err != nil {
		slog.Error("api: failed to query config", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to read configuration")
		return
	}

	settings := make(map[string]any, len(rows))
	for _, row := range rows {
		key := toString(row["key"])
		value := toString(row["value"])
		settings[key] = value
	}

	// Build structured config from system_settings
	llmConfig := make(map[string]any)
	hitlConfig := make(map[string]any)
	harnessConfig := make(map[string]any)
	dbConfig := make(map[string]any)
	loggingConfig := make(map[string]any)
	otherSettings := make(map[string]any)

	for key, value := range settings {
		switch {
		case strings.HasPrefix(key, "llm."):
			llmConfig[strings.TrimPrefix(key, "llm.")] = value
		case strings.HasPrefix(key, "hitl."):
			hitlConfig[strings.TrimPrefix(key, "hitl.")] = value
		case strings.HasPrefix(key, "harness."):
			harnessConfig[strings.TrimPrefix(key, "harness.")] = value
		case strings.HasPrefix(key, "db."):
			dbConfig[strings.TrimPrefix(key, "db.")] = value
		case strings.HasPrefix(key, "logging."):
			loggingConfig[strings.TrimPrefix(key, "logging.")] = value
		default:
			otherSettings[key] = value
		}
	}

	// If no per-section settings, return the raw system_settings map
	if len(llmConfig)+len(hitlConfig)+len(harnessConfig)+len(dbConfig)+len(loggingConfig) == 0 {
		writeJSON(w, ConfigResponse{
			SystemSettings: settings,
		})
		return
	}

	writeJSON(w, ConfigResponse{
		LLM:            llmConfig,
		HITL:           hitlConfig,
		Harness:        harnessConfig,
		Database:       dbConfig,
		Logging:        loggingConfig,
		SystemSettings: otherSettings,
	})
}

// ============================================================================
// GET /api/v1/metrics — system-wide metrics (SPEC-015 §3.7)
// ============================================================================

func (s *Server) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	scope := GetAuthScope(r)
	if scope != "admin" && scope != "readonly" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "admin or readonly scope required")
		return
	}

	ctx := r.Context()

	activeStatuses := []string{"booting", "idle", "thinking", "planning", "tool_exec", "executing", "waiting_sub"}
	var activeCount int

	// Count active sessions
	row, err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM sessions WHERE status IN ('booting','idle','thinking','planning','tool_exec','executing','waiting_sub')`)
	if err == nil {
		activeCount = toInt(row["COUNT(*)"])
	}

	// Count pending tasks
	pendingTasks := 0
	row, err = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE status = 'pending'`)
	if err == nil {
		pendingTasks = toInt(row["COUNT(*)"])
	}

	// Count pending approvals
	pendingApprovals := 0
	row, err = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM approval_requests WHERE status = 'pending'`)
	if err == nil {
		pendingApprovals = toInt(row["COUNT(*)"])
	}

	// Count total sessions
	totalSessions := 0
	row, err = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM sessions`)
	if err == nil {
		totalSessions = toInt(row["COUNT(*)"])
	}

	// Sum total cost
	var totalCost float64
	row, err = s.db.QueryRow(ctx, `SELECT COALESCE(SUM(cost_usd), 0) FROM agent_billing`)
	if err == nil {
		totalCost = toFloat64(row["COALESCE(SUM(cost_usd), 0)"])
	}

	_ = activeStatuses // suppress unused warning

	writeJSON(w, MetricsResponse{
		ActiveSessions:   activeCount,
		PendingTasks:     pendingTasks,
		PendingApprovals: pendingApprovals,
		TotalSessions:    totalSessions,
		TotalCostUSD:     math.Round(totalCost*1e6) / 1e6,
	})
}

// ============================================================================
// Auth Endpoints — API key management (SPEC-015 §3.8)
// ============================================================================

// POST /api/v1/auth/keys — create a new API key
func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	scope := GetAuthScope(r)
	if scope != "admin" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "admin scope required")
		return
	}

	var req struct {
		Scope     string  `json:"scope"`
		SessionID *string `json:"session_id,omitempty"`
		ExpiresIn *int    `json:"expires_in,omitempty"` // seconds from now
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed request body: "+err.Error())
		return
	}

	validScopes := map[string]bool{"admin": true, "session": true, "readonly": true, "webhook": true}
	if !validScopes[req.Scope] {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "scope must be one of: admin, session, readonly, webhook")
		return
	}

	// Generate key
	apiKey := generateAPIKey()
	keyHash := sha256Hash(apiKey)
	keyPrefix := apiKey[:8]
	keyID := newUUID()

	now := time.Now().UTC().Format(time.RFC3339)

	ctx := r.Context()

	// Determine scope prefix for the key
	scopeToPrefix := map[string]string{
		"admin":    "cs_ak_",
		"session":  "cs_sk_",
		"readonly": "cs_rk_",
		"webhook":  "cs_wk_",
	}
	_ = scopeToPrefix

	var sessionID string
	if req.SessionID != nil {
		sessionID = *req.SessionID
		// Verify session exists
		_, err := s.db.QueryRow(ctx, `SELECT id FROM sessions WHERE id = $1`, sessionID)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "NOT_FOUND", "session not found")
			return
		}
	}

	var expiresAt *string
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		expTime := time.Now().UTC().Add(time.Duration(*req.ExpiresIn) * time.Second)
		expStr := expTime.Format(time.RFC3339)
		expiresAt = &expStr
	}

	// Insert API key
	args := []any{keyID, keyHash, keyPrefix, req.Scope}
	var query string
	if sessionID != "" && expiresAt != nil {
		query = `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, expires_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`
		args = append(args, sessionID, *expiresAt, now)
	} else if sessionID != "" {
		query = `INSERT INTO api_keys (id, key_hash, key_prefix, scope, session_id, created_at) VALUES ($1, $2, $3, $4, $5, $6)`
		args = append(args, sessionID, now)
	} else if expiresAt != nil {
		query = `INSERT INTO api_keys (id, key_hash, key_prefix, scope, expires_at, created_at) VALUES ($1, $2, $3, $4, $5, $6)`
		args = append(args, *expiresAt, now)
	} else {
		query = `INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ($1, $2, $3, $4, $5)`
		args = append(args, now)
	}

	if err := s.db.Exec(ctx, query, args...); err != nil {
		slog.Error("api: failed to create api key", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create api key")
		return
	}

	writeJSON(w, map[string]any{
		"id":         keyID,
		"key_prefix": keyPrefix,
		"api_key":    apiKey,
		"scope":      req.Scope,
		"created_at": now,
	})
}

// GET /api/v1/auth/keys — list API keys (prefix + scope only, no hashes)
func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	scope := GetAuthScope(r)
	if scope != "admin" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "admin scope required")
		return
	}

	ctx := r.Context()
	rows, err := s.db.Query(ctx,
		`SELECT id, key_prefix, scope, session_id, expires_at, created_at
		 FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		slog.Error("api: failed to list api keys", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list api keys")
		return
	}

	type keyEntry struct {
		ID        string  `json:"id"`
		Prefix    string  `json:"prefix"`
		Scope     string  `json:"scope"`
		SessionID *string `json:"session_id,omitempty"`
		ExpiresAt *string `json:"expires_at,omitempty"`
		CreatedAt string  `json:"created_at"`
	}

	keys := make([]keyEntry, 0, len(rows))
	for _, row := range rows {
		k := keyEntry{
			ID:        toString(row["id"]),
			Prefix:    toString(row["key_prefix"]),
			Scope:     toString(row["scope"]),
			CreatedAt: toString(row["created_at"]),
		}
		if sid := row["session_id"]; sid != nil {
			s := toString(sid)
			k.SessionID = &s
		}
		if exp := row["expires_at"]; exp != nil {
			s := toString(exp)
			k.ExpiresAt = &s
		}
		keys = append(keys, k)
	}

	writeJSON(w, keys)
}

// DELETE /api/v1/auth/keys/{id} — revoke an API key
func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request, keyID string) {
	scope := GetAuthScope(r)
	if scope != "admin" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "admin scope required")
		return
	}

	ctx := r.Context()

	// Check that the key exists
	_, err := s.db.QueryRow(ctx, `SELECT id FROM api_keys WHERE id = $1`, keyID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "api key not found")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.db.Exec(ctx, `UPDATE api_keys SET expires_at = $1 WHERE id = $2`, now, keyID); err != nil {
		slog.Error("api: failed to revoke api key", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to revoke api key")
		return
	}

	writeJSON(w, map[string]any{
		"status":     "revoked",
		"key_id":     keyID,
		"revoked_at": now,
	})
}

// ============================================================================
// Helpers
// ============================================================================

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case string:
		// SQLite may return numeric values as strings
		var f float64
		json.Unmarshal([]byte(n), &f)
		return f
	default:
		return 0
	}
}
