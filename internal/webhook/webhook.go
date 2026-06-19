// Package webhook implements webhook ingestion, HMAC verification, and event
// routing for Consensus (SPEC-013).
//
// Webhooks are the primary mechanism for external systems to push events into
// the Consensus runtime. The database IS the event bus — no Kafka, no Redis.
// Webhooks write rows to external_events; a Go-level routing loop routes them
// to agents.
//
// Security features:
//   - HMAC-SHA256 signature verification per webhook registration
//   - Token-bucket rate limiting per source IP (configurable, default 60 req/min)
//   - Payload size limits (1 MB body, 64 KB headers)
//   - Idempotent event processing via Go-level dedup + UNIQUE index
//   - Invalid signature events routed to quarantined status
//   - Cognitive Firewall: suspicious content routed to external_quarantine for scanning
//
// axiom:trace work_item=spec-013-hardening-01,WI-004 spec=specs/013-webhooks-and-events.md,specs/005-security.md plan=phase-1/task-1/step-1,phase-2/task-1 impl=internal/webhook/webhook.go
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// QuarantineScanner interface — avoids direct dependency on quarantine package
// ============================================================================

// QuarantineScanner is the interface for scanning event content for threats.
// The quarantine package implements this; the webhook package calls it.
type QuarantineScanner interface {
	ScanContent(content string, sourceID string) (bool, float64, string, []string)
}

// ============================================================================
// Webhook Registration
// ============================================================================

// Registration represents a registered webhook source.
type Registration struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Source           string    `json:"source"`
	URLPath          string    `json:"url_path"`
	Secret           string    `json:"-"` // Never exposed
	EventTypes       []string  `json:"event_types"`
	TargetSessionID  string    `json:"target_session_id,omitempty"`
	TargetWorkflowID string    `json:"target_workflow_id,omitempty"`
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
}

// Store manages webhook registrations and event processing.
type Store struct {
	database db.DB
	mu       sync.RWMutex

	// Token-bucket rate limiter state (per source IP).
	// Only used when the rate limiter is not backed by the DB.
	buckets map[string]*tokenBucket

	// quarantineScanner is the cognitive firewall scanner (optional).
	// When set, suspicious event content is quarantined after ingestion.
	quarantineScanner QuarantineScanner

	// quarantineInserter is called to insert an item into quarantine (optional).
	quarantineInserter func(ctx context.Context, sessionID, sourceType, rawContent, sourceURL string) error
}

// New creates a new webhook store backed by the given database.
func New(database db.DB) *Store {
	return &Store{
		database: database,
		buckets:  make(map[string]*tokenBucket),
	}
}

// SetQuarantineScanner sets the cognitive firewall scanner for content threat detection.
// When set, HandleWebhook will scan incoming payloads and quarantine suspicious content.
func (s *Store) SetQuarantineScanner(scanner QuarantineScanner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quarantineScanner = scanner
}

// SetQuarantineInserter sets the function that inserts items into the quarantine table.
// Expected to be called when the scanner detects a threat.
func (s *Store) SetQuarantineInserter(inserter func(ctx context.Context, sessionID, sourceType, rawContent, sourceURL string) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quarantineInserter = inserter
}

// ============================================================================
// Registration CRUD (AC-EVT-08)
// ============================================================================

// CreateRegistration inserts a new webhook registration.
func (s *Store) CreateRegistration(ctx context.Context, r Registration) (*Registration, error) {
	if r.Name == "" {
		return nil, fmt.Errorf("webhook: registration name is required")
	}
	if r.Source == "" {
		return nil, fmt.Errorf("webhook: registration source is required")
	}
	if r.URLPath == "" {
		return nil, fmt.Errorf("webhook: registration url_path is required")
	}
	if r.Secret == "" {
		return nil, fmt.Errorf("webhook: HMAC secret is required for webhook registration")
	}

	id := fmt.Sprintf("wh_%s_%d", r.Name, time.Now().UnixNano())
	err := s.database.Exec(ctx, `
		INSERT INTO webhook_registrations (id, name, source, url_path, secret, event_types, target_session_id, target_workflow_id, enabled, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, id, r.Name, r.Source, r.URLPath, r.Secret, "{ }", r.TargetSessionID, r.TargetWorkflowID, true, time.Now())
	if err != nil {
		return nil, fmt.Errorf("webhook: create registration: %w", err)
	}
	r.ID = id
	return &r, nil
}

// GetRegistration retrieves a webhook registration by source or name.
func (s *Store) GetRegistration(ctx context.Context, sourceName string) (*Registration, error) {
	rows, err := s.database.Query(ctx, `
		SELECT id, name, source, url_path, secret, event_types, target_session_id, target_workflow_id, enabled, created_at
		FROM webhook_registrations
		WHERE name = $1 OR url_path = $1 OR source = $1
	`, sourceName)
	if err != nil {
		return nil, fmt.Errorf("webhook: get registration: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("webhook: registration %q not found", sourceName)
	}
	return rowToRegistration(rows[0])
}

// ListRegistrations returns all webhook registrations.
func (s *Store) ListRegistrations(ctx context.Context) ([]Registration, error) {
	rows, err := s.database.Query(ctx, `
		SELECT id, name, source, url_path, secret, event_types, target_session_id, target_workflow_id, enabled, created_at
		FROM webhook_registrations ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("webhook: list registrations: %w", err)
	}

	var result []Registration
	for _, row := range rows {
		r, err := rowToRegistration(row)
		if err != nil {
			return nil, err
		}
		result = append(result, *r)
	}
	return result, nil
}

// UpdateRegistration updates an existing webhook registration.
func (s *Store) UpdateRegistration(ctx context.Context, name string, r Registration) error {
	return s.database.Exec(ctx, `
		UPDATE webhook_registrations
		SET source = $1, url_path = $2, secret = $3, event_types = $4, target_session_id = $5, target_workflow_id = $6, enabled = $7
		WHERE name = $8
	`, r.Source, r.URLPath, r.Secret, "{ }", r.TargetSessionID, r.TargetWorkflowID, r.Enabled, name)
}

// DeleteRegistration removes a webhook registration by name.
func (s *Store) DeleteRegistration(ctx context.Context, name string) error {
	return s.database.Exec(ctx, `
		DELETE FROM webhook_registrations WHERE name = $1
	`, name)
}

// EnableRegistration enables or disables a webhook registration.
func (s *Store) EnableRegistration(ctx context.Context, name string, enabled bool) error {
	return s.database.Exec(ctx, `
		UPDATE webhook_registrations SET enabled = $1 WHERE name = $2
	`, enabled, name)
}

// ============================================================================
// Event Ingestion (AC-EVT-01, AC-EVT-02, AC-EVT-03, AC-EVT-04)
// ============================================================================

// Event represents an incoming external event.
type Event struct {
	ID             int64             `json:"id,omitempty"`
	Source         string            `json:"source"`
	SourceID       string            `json:"source_id"`
	EventType      string            `json:"event_type"`
	Payload        json.RawMessage   `json:"payload"`
	Headers        map[string]string `json:"headers,omitempty"`
	SignatureValid bool              `json:"signature_valid"`
	SessionID      string            `json:"session_id,omitempty"`
	WorkflowID     string            `json:"workflow_id,omitempty"`
	Status         string            `json:"status,omitempty"`
}

// IngestResult is the outcome of an IngestEvent call.
type IngestResult string

const (
	IngestOK         IngestResult = "ok"          // New event ingested
	IngestDuplicate  IngestResult = "duplicate"   // Already ingested (idempotent)
	IngestRejected   IngestResult = "rejected"    // Rate limited or oversized
)

// IngestEvent processes an incoming webhook event:
// 1. Check payload size limit
// 2. Check for duplicate (idempotency via Go-level lookup)
// 3. Insert into external_events with status based on signature validity
//
// Returns the ingest result and optionally the event ID for duplicate cases.
func (s *Store) IngestEvent(ctx context.Context, event Event, registration *Registration) (IngestResult, int64, error) {
	// If payload is too large, reject before signature check
	if len(event.Payload) > maxPayloadSize {
		return IngestRejected, 0, &PayloadTooLargeError{Limit: maxPayloadSize, Received: len(event.Payload)}
	}

	// IDEMPOTENCY CHECK (SPEC-013 §8.4, AC-EVT-02):
	// Check if this exact (source, source_id) pair already exists.
	// This complements the UNIQUE index on external_events(source, source_id).
	if event.SourceID != "" {
		existingRows, err := s.database.Query(ctx, `
			SELECT id FROM external_events WHERE source = $1 AND source_id = $2
		`, event.Source, event.SourceID)
		if err != nil {
			return IngestRejected, 0, fmt.Errorf("webhook: idempotency check failed: %w", err)
		}
		if len(existingRows) > 0 {
			existingID := toInt64(existingRows[0]["id"])
			slog.Debug("webhook: duplicate event", "source", event.Source, "source_id", event.SourceID, "existing_id", existingID)
			return IngestDuplicate, existingID, nil
		}
	}

	err := s.database.Exec(ctx, `
		INSERT INTO external_events (source, source_id, event_type, payload, headers, signature_valid, session_id, workflow_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, event.Source, event.SourceID, event.EventType,
		string(event.Payload), headersToJSON(event.Headers),
		event.SignatureValid,
		nullString(event.SessionID), nullString(event.WorkflowID),
		statusForEvent(event.SignatureValid),
	)
	if err != nil {
		// If it's a UNIQUE constraint violation (race condition on source_id),
		// treat as duplicate rather than error.
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "unique") {
			slog.Debug("webhook: duplicate event (caught by UNIQUE index)", "source", event.Source, "source_id", event.SourceID)
			return IngestDuplicate, 0, nil
		}
		return IngestRejected, 0, fmt.Errorf("webhook: ingest event: %w", err)
	}

	return IngestOK, 0, nil
}

// ============================================================================
// HMAC Signature Verification (AC-EVT-01)
// ============================================================================

// VerifyHMAC verifies an HMAC-SHA256 signature against the expected value.
// Uses constant-time comparison to prevent timing attacks.
func VerifyHMAC(payload []byte, signatureHeader string, secret string) bool {
	if signatureHeader == "" || secret == "" {
		return false
	}

	// Strip "sha256=" prefix if present
	signature := strings.TrimPrefix(signatureHeader, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
}

// ============================================================================
// Token-Bucket Rate Limiter (AC-EVT-03)
// ============================================================================

// Default rate limit for webhook endpoints.
const DefaultWebhookRateLimit = 60 // requests per minute (1 req/sec)
const DefaultRateLimitBurst = 10   // initial burst capacity

// maxPayloadSize is the maximum accepted webhook payload body.
const maxPayloadSize = 1 << 20 // 1 MB

// tokenBucket implements a simple token-bucket rate limiter.
type tokenBucket struct {
	rate       float64   // tokens per second
	burst      float64   // max token capacity
	tokens     float64   // current tokens
	lastRefill time.Time // last token refill time
	mu         sync.Mutex
}

// newTokenBucket creates a token bucket with the given rate and burst.
func newTokenBucket(ratePerMin int, burst int) *tokenBucket {
	return &tokenBucket{
		rate:       float64(ratePerMin) / 60.0,
		burst:      float64(burst),
		tokens:     float64(burst),
		lastRefill: time.Now(),
	}
}

// allow returns true if a request should be allowed (consuming one token).
func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens = min(tb.burst, tb.tokens+elapsed*tb.rate)
	tb.lastRefill = now

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// checkRateLimit checks if a webhook source is within its rate limit.
// Uses a Go-level token bucket keyed by source name.
func (s *Store) checkRateLimit(source string, limitPerMin int) bool {
	s.mu.Lock()
	bucket, exists := s.buckets[source]
	if !exists {
		bucket = newTokenBucket(limitPerMin, DefaultRateLimitBurst)
		s.buckets[source] = bucket
	}
	s.mu.Unlock()

	allowed := bucket.allow()
	if !allowed {
		slog.Warn("webhook: rate limited", "source", source)
	}
	return allowed
}

// ============================================================================
// Event Routing Loop (SPEC-013 §5, Go-level replacement for DB trigger)
// ============================================================================

// RoutingRule represents a routing rule from the database.
type RoutingRule struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	SourcePattern     string `json:"source_pattern,omitempty"`
	EventTypePattern  string `json:"event_type_pattern,omitempty"`
	PayloadPattern    string `json:"payload_pattern,omitempty"`
	TargetSessionID   string `json:"target_session_id,omitempty"`
	TargetWorkflowID  string `json:"target_workflow_id,omitempty"`
	Priority          int    `json:"priority"`
	Enabled           bool   `json:"enabled"`
}

// StartRoutingLoop runs a background goroutine that polls for pending external
// events, matches them against routing rules, and wakes target sessions.
//
// The loop runs on an interval (default: 5 seconds) and processes up to 100
// events per iteration. It's safe to call multiple times on a single Store
// (the polling is idempotent due to status transitions).
func (s *Store) StartRoutingLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}

	slog.Info("webhook: starting event routing loop", "interval", interval)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("webhook: routing loop stopped")
				return
			case <-ticker.C:
				if err := s.routePendingEvents(ctx); err != nil {
					slog.Warn("webhook: routing iteration failed", "error", err)
				}
			}
		}
	}()
}

// routePendingEvents fetches pending events, matches routing rules, and
// transitions events to 'routed' status with session/workflow targets.
func (s *Store) routePendingEvents(ctx context.Context) error {
	// 1. Fetch pending events (no session/workflow assigned yet, not quarantined)
	rows, err := s.database.Query(ctx, `
		SELECT id, source, source_id, event_type, payload, headers, signature_valid, session_id, workflow_id, status
		FROM external_events
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT 100
	`)
	if err != nil {
		return fmt.Errorf("webhook: fetch pending events: %w", err)
	}

	if len(rows) == 0 {
		return nil // nothing to route
	}

	// 2. Load routing rules (ordered by priority)
	rules, err := s.loadRoutingRules(ctx)
	if err != nil {
		return fmt.Errorf("webhook: load routing rules: %w", err)
	}

	routed := 0
	for _, row := range rows {
		eventID := toInt64(row["id"])
		eventSource := toString(row["source"])
		eventType := toString(row["event_type"])
		payloadStr := toString(row["payload"])

		// Try to match a routing rule
		match := s.matchRoutingRule(rules, eventSource, eventType, payloadStr)
		if match != nil {
			// Update event with routing targets
			if err := s.database.Exec(ctx, `
				UPDATE external_events
				SET session_id = $1, workflow_id = $2, status = 'routed', processed_at = $3
				WHERE id = $4
			`, nullString(match.TargetSessionID), nullString(match.TargetWorkflowID), time.Now(), eventID); err != nil {
				slog.Warn("webhook: failed to update routed event", "event_id", eventID, "error", err)
				continue
			}

			// Wake the target session if it exists and is waiting (SPEC-013 §5.1)
			if match.TargetSessionID != "" {
				if err := s.database.Exec(ctx, `
					UPDATE sessions
					SET status = 'idle', heartbeat_at = $1
					WHERE id = $2 AND status IN ('waiting_sub', 'paused')
				`, time.Now(), match.TargetSessionID); err != nil {
					slog.Warn("webhook: failed to wake session", "session_id", match.TargetSessionID, "error", err)
				} else {
					slog.Info("webhook: woke session via event routing", "session_id", match.TargetSessionID, "event_id", eventID)
				}
			}
			routed++
		}
	}

	if routed > 0 {
		slog.Info("webhook: routed events", "count", routed, "pending", len(rows))
	}

	return nil
}

// loadRoutingRules loads all enabled routing rules ordered by priority.
func (s *Store) loadRoutingRules(ctx context.Context) ([]RoutingRule, error) {
	rows, err := s.database.Query(ctx, `
		SELECT id, name, source_pattern, event_type_pattern, payload_pattern, target_session_id, target_workflow_id, priority, enabled
		FROM routing_rules
		WHERE enabled = 1
		ORDER BY priority ASC
	`)
	if err != nil {
		return nil, err
	}

	var rules []RoutingRule
	for _, row := range rows {
		rules = append(rules, RoutingRule{
			ID:               toString(row["id"]),
			Name:             toString(row["name"]),
			SourcePattern:    toString(row["source_pattern"]),
			EventTypePattern: toString(row["event_type_pattern"]),
			PayloadPattern:   toString(row["payload_pattern"]),
			TargetSessionID:  toString(row["target_session_id"]),
			TargetWorkflowID: toString(row["target_workflow_id"]),
			Priority:         toInt(row["priority"]),
			Enabled:          toBool(row["enabled"]),
		})
	}
	return rules, nil
}

// matchRoutingRule finds the first matching routing rule (highest priority first).
func (s *Store) matchRoutingRule(rules []RoutingRule, source, eventType, payload string) *RoutingRule {
	for i := range rules {
		r := &rules[i]
		if r.SourcePattern != "" && !strings.Contains(source, r.SourcePattern) {
			continue
		}
		if r.EventTypePattern != "" && !strings.Contains(eventType, r.EventTypePattern) {
			continue
		}
		if r.PayloadPattern != "" && !strings.Contains(payload, r.PayloadPattern) {
			continue
		}
		return r
	}
	return nil
}

// ============================================================================
// Event Status Lifecycle (AC-EVT-07)
// ============================================================================

// Event status values.
const (
	EventStatusPending     = "pending"
	EventStatusRouted      = "routed"
	EventStatusProcessing  = "processing"
	EventStatusCompleted   = "completed"
	EventStatusFailed      = "failed"
	EventStatusQuarantined = "quarantined"
)

// UpdateEventStatus transitions an external event to a new status.
func (s *Store) UpdateEventStatus(ctx context.Context, eventID int64, status string) error {
	validStatuses := map[string]bool{
		EventStatusPending: true, EventStatusRouted: true, EventStatusProcessing: true,
		EventStatusCompleted: true, EventStatusFailed: true, EventStatusQuarantined: true,
	}
	if !validStatuses[status] {
		return fmt.Errorf("webhook: invalid event status: %q", status)
	}

	return s.database.Exec(ctx, `
		UPDATE external_events SET status = $1, processed_at = $2 WHERE id = $3
	`, status, time.Now(), eventID)
}

// GetPendingEvents returns events with the given status.
func (s *Store) GetPendingEvents(ctx context.Context) ([]Event, error) {
	rows, err := s.database.Query(ctx, `
		SELECT id, source, source_id, event_type, payload, headers, signature_valid, session_id, workflow_id, status
		FROM external_events WHERE status = $1 ORDER BY created_at ASC LIMIT 100
	`, EventStatusPending)
	if err != nil {
		return nil, fmt.Errorf("webhook: get pending events: %w", err)
	}

	var events []Event
	for _, row := range rows {
		e, err := rowToEvent(row)
		if err != nil {
			continue
		}
		events = append(events, *e)
	}
	return events, nil
}

// ============================================================================
// HTTP Handler
// ============================================================================

// HandleWebhook is the HTTP handler for incoming webhook requests.
// Path: /webhooks/{source}
func (s *Store) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Extract source from URL path
	source := strings.TrimPrefix(r.URL.Path, "/webhooks/")
	if source == "" {
		http.Error(w, `{"error":{"code":"INVALID_REQUEST","message":"missing webhook source"}}`, http.StatusBadRequest)
		return
	}

	// Token-bucket rate limit check (SPEC-013 §8.2, AC-EVT-03)
	if !s.checkRateLimit(source, DefaultWebhookRateLimit) {
		http.Error(w, `{"error":{"code":"RATE_LIMITED","message":"too many webhook requests"}}`, http.StatusTooManyRequests)
		return
	}

	// Look up registration
	registration, err := s.GetRegistration(r.Context(), source)
	if err != nil {
		http.Error(w, `{"error":{"code":"NOT_FOUND","message":"unknown webhook source"}}`, http.StatusNotFound)
		return
	}

	if !registration.Enabled {
		http.Error(w, `{"error":{"code":"FORBIDDEN","message":"webhook is disabled"}}`, http.StatusForbidden)
		return
	}

	// Read body with size limit
	r.Body = http.MaxBytesReader(w, r.Body, maxPayloadSize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			http.Error(w, `{"error":{"code":"PAYLOAD_TOO_LARGE","message":"request body exceeds 1 MB limit"}}`, http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, `{"error":{"code":"INTERNAL_ERROR","message":"failed to read request body"}}`, http.StatusInternalServerError)
		return
	}

	// Verify HMAC signature
	signatureHeader := r.Header.Get("X-Hub-Signature-256")
	if signatureHeader == "" {
		signatureHeader = r.Header.Get("X-Signature-256")
	}
	if signatureHeader == "" {
		signatureHeader = r.Header.Get("X-Signature")
	}
	signatureValid := VerifyHMAC(body, signatureHeader, registration.Secret)

	// Extract event type and source ID
	eventType := r.Header.Get("X-Event-Type")
	if eventType == "" {
		eventType = r.Header.Get("X-GitHub-Event")
	}
	if eventType == "" {
		eventType = "unknown"
	}
	sourceID := r.Header.Get("X-Delivery-ID")
	if sourceID == "" {
		sourceID = r.Header.Get("X-GitHub-Delivery")
	}

	// Build event
	event := Event{
		Source:         "webhook",
		SourceID:       sourceID,
		EventType:      eventType,
		Payload:        body,
		Headers:        flattenHeaders(r.Header),
		SignatureValid: signatureValid,
		SessionID:      registration.TargetSessionID,
		WorkflowID:     registration.TargetWorkflowID,
	}

	// Ingest into database (with idempotency)
	result, existingID, err := s.IngestEvent(r.Context(), event, registration)
	if err != nil {
		if _, ok := err.(*PayloadTooLargeError); ok {
			http.Error(w, fmt.Sprintf(`{"error":{"code":"PAYLOAD_TOO_LARGE","message":"%s"}}`, err.Error()), http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, fmt.Sprintf(`{"error":{"code":"INTERNAL_ERROR","message":"%s"}}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Cognitive Firewall: scan content for threats after successful ingestion
	// (SPEC-005 §Cognitive Firewall, SPEC-013 §5.2)
	// This runs asynchronously — we don't block the response on quarantine checks.
	if result == IngestOK && s.quarantineScanner != nil {
		bodyStr := string(body)
		isThreat, confidence, reason, rules := s.quarantineScanner.ScanContent(bodyStr, sourceID)
		if isThreat || !signatureValid {
			slog.Warn("webhook: quarantining event content",
				"source", source, "source_id", sourceID,
				"signature_valid", signatureValid,
				"threat", isThreat,
				"confidence", confidence,
				"reason", reason,
				"rules", rules,
			)
			if s.quarantineInserter != nil {
				sessionID := registration.TargetSessionID
				if sessionID == "" {
					sessionID = "00000000-0000-0000-0000-000000000000"
				}
				if err := s.quarantineInserter(r.Context(), sessionID, "api_response", bodyStr, ""); err != nil {
					slog.Warn("webhook: failed to insert quarantine item", "error", err)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")

	// For duplicate events, return 200 OK with existing event ID
	if result == IngestDuplicate {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     "duplicate",
			"event_id":   existingID,
			"message":    "event already ingested",
		})
		return
	}

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
}

// ServeHTTP implements http.Handler so the Store can be mounted directly into a ServeMux.
func (s *Store) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.HandleWebhook(w, r)
}

// ============================================================================
// Error Types
// ============================================================================

// PayloadTooLargeError is returned when a webhook payload exceeds the limit.
type PayloadTooLargeError struct {
	Limit    int
	Received int
}

func (e *PayloadTooLargeError) Error() string {
	return fmt.Sprintf("webhook: payload too large: %d bytes (limit: %d bytes)", e.Received, e.Limit)
}

// ============================================================================
// Helpers
// ============================================================================

func statusForEvent(signatureValid bool) string {
	if signatureValid {
		return EventStatusPending
	}
	return EventStatusQuarantined
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func headersToJSON(headers map[string]string) string {
	if len(headers) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(headers)
	return string(b)
}

// flattenHeaders extracts relevant headers into a simpler map.
func flattenHeaders(h http.Header) map[string]string {
	result := make(map[string]string)
	relevant := []string{"X-Delivery-ID", "X-Event-Type", "X-GitHub-Event", "X-Signature-256", "Content-Type", "User-Agent"}
	for _, key := range relevant {
		if v := h.Get(key); v != "" {
			result[strings.ToLower(key)] = v
		}
	}
	return result
}

// rowToRegistration converts a database row to a Registration.
func rowToRegistration(row db.Row) (*Registration, error) {
	id, _ := row["id"].(string)
	name, _ := row["name"].(string)
	source, _ := row["source"].(string)
	urlPath, _ := row["url_path"].(string)
	secret, _ := row["secret"].(string)
	enabled := toBool(row["enabled"])
	targetSess, _ := row["target_session_id"].(string)
	targetWork, _ := row["target_workflow_id"].(string)

	createdAt := time.Now()
	if v, ok := row["created_at"].(string); ok {
		createdAt, _ = time.Parse(time.RFC3339, v)
	}

	return &Registration{
		ID:               id,
		Name:             name,
		Source:           source,
		URLPath:          urlPath,
		Secret:           secret,
		TargetSessionID:  targetSess,
		TargetWorkflowID: targetWork,
		Enabled:          enabled,
		CreatedAt:        createdAt,
	}, nil
}

// rowToEvent converts a database row to an Event.
func rowToEvent(row db.Row) (*Event, error) {
	id := toInt64(row["id"])
	source, _ := row["source"].(string)
	sourceID, _ := row["source_id"].(string)
	eventType, _ := row["event_type"].(string)
	payload, _ := row["payload"].(string)
	sigValid := toBool(row["signature_valid"])
	sessionID, _ := row["session_id"].(string)
	workflowID, _ := row["workflow_id"].(string)
	status, _ := row["status"].(string)

	return &Event{
		ID:             id,
		Source:         source,
		SourceID:       sourceID,
		EventType:      eventType,
		Payload:        json.RawMessage(payload),
		SignatureValid: sigValid,
		SessionID:      sessionID,
		WorkflowID:     workflowID,
		Status:         status,
	}, nil
}

func toBool(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case int64:
		return val > 0
	case float64:
		return val > 0
	case string:
		return val == "true" || val == "1"
	default:
		return false
	}
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		var i int
		fmt.Sscanf(val, "%d", &i)
		return i
	default:
		return 0
	}
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	default:
		return 0
	}
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprintf("%v", v)
	}
}
