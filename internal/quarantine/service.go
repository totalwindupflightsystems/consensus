// Package quarantine — service layer for the Cognitive Firewall.
//
// QuarantineService manages the lifecycle of quarantined external data:
//   - Inserting suspicious events into external_quarantine
//   - Scanning pending quarantined items
//   - Approving (promoting to agent memory) or rejecting (with reason)
//   - Emitting SSE events on state changes
//
// axiom:trace work_item=WI-004 spec=specs/005-security.md,specs/013-webhooks-and-events.md plan=phase-2/task-1/step-1
package quarantine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// Quarantine Status Constants
// ============================================================================

const (
	// Quarantine item is pending scan/approval.
	StatusPending = "pending"

	// Quarantine item was scanned and approved (clean).
	StatusValidated = "validated"

	// Quarantine item was rejected (malicious content).
	StatusRejected = "rejected"

	// Quarantine item expired without review.
	StatusExpired = "expired"
)

// ============================================================================
// Quarantine Item
// ============================================================================

// QuarantineItem represents a row in the external_quarantine table.
type QuarantineItem struct {
	ID               int64  `json:"id"`
	SessionID        string `json:"session_id"`
	SourceType       string `json:"source_type"`
	SourceURL        string `json:"source_url,omitempty"`
	RawContent       string `json:"raw_content"`
	ContentHash      string `json:"content_hash"`
	ValidationStatus string `json:"validation_status"`
	ValidationNotes  string `json:"validation_notes,omitempty"`
	PromotedMemoryID int64  `json:"promoted_memory_id,omitempty"`
	ExpiresAt        string `json:"expires_at"`
	CreatedAt        string `json:"created_at"`
}

// ============================================================================
// Event Bus Interface (avoid circular dep with api package)
// ============================================================================

// PublishFunc is a function type for emitting SSE events.
// Matches EventBus.PublishQuarantineEvent signature.
type PublishFunc func(sessionID string, eventType string, eventData any)

// QuarantineService manages the cognitive firewall quarantine lifecycle.
type QuarantineService struct {
	db      db.DB
	publish PublishFunc
}

// NewQuarantineService creates a new QuarantineService.
// publish is optional — if nil, no SSE events are emitted.
func NewQuarantineService(database db.DB, publish PublishFunc) *QuarantineService {
	return &QuarantineService{
		db:      database,
		publish: publish,
	}
}

// ============================================================================
// Insert — add an item to the quarantine
// ============================================================================

// InsertQuarantineInput is the data needed to quarantine an event.
type InsertQuarantineInput struct {
	SessionID   string
	SourceType  string
	RawContent  string
	ContentHash string
	SourceURL   string
}

// InsertQuarantine inserts a new row into external_quarantine and returns the item.
func (qs *QuarantineService) InsertQuarantine(ctx context.Context, input InsertQuarantineInput) (*QuarantineItem, error) {
	if input.SessionID == "" {
		input.SessionID = "00000000-0000-0000-0000-000000000000"
	}
	if input.SourceType == "" {
		input.SourceType = "api_response"
	}
	if input.ContentHash == "" {
		input.ContentHash = ContentHash(input.RawContent)
	}

	expiresAt := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	createdAt := time.Now().UTC().Format(time.RFC3339)

	err := qs.db.Exec(ctx, `
		INSERT INTO external_quarantine (session_id, source_type, source_url, raw_content, content_hash, validation_status, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, input.SessionID, input.SourceType, input.SourceURL, input.RawContent, input.ContentHash, StatusPending, expiresAt, createdAt)
	if err != nil {
		return nil, fmt.Errorf("quarantine: insert: %w", err)
	}

	// Fetch the inserted row to get the auto-generated ID
	rows, err := qs.db.Query(ctx, `
		SELECT id, session_id, source_type, source_url, raw_content, content_hash, validation_status, validation_notes, COALESCE(promoted_memory_id, 0), expires_at, created_at
		FROM external_quarantine
		WHERE content_hash = $1 AND session_id = $2
		ORDER BY id DESC LIMIT 1
	`, input.ContentHash, input.SessionID)
	if err != nil {
		return nil, fmt.Errorf("quarantine: fetch inserted: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("quarantine: inserted row not found")
	}

	item := rowToQuarantineItem(rows[0])
	slog.Debug("quarantine: item inserted", "id", item.ID, "source_type", item.SourceType, "hash", item.ContentHash)

	// Emit quarantine event
	if qs.publish != nil {
		qs.emitEvent(item, "quarantine_pending")
	}

	return item, nil
}

// ============================================================================
// List — list quarantined items with optional status filter
// ============================================================================

// ListQuarantine returns quarantined items, optionally filtered by status.
// If status is empty, all items are returned (limited to 100).
func (qs *QuarantineService) ListQuarantine(ctx context.Context, status string) ([]QuarantineItem, error) {
	var rows []db.Row
	var err error

	if status != "" {
		rows, err = qs.db.Query(ctx, `
			SELECT id, session_id, source_type, source_url, raw_content, content_hash, validation_status, validation_notes, COALESCE(promoted_memory_id, 0), expires_at, created_at
			FROM external_quarantine
			WHERE validation_status = $1
			ORDER BY created_at DESC
			LIMIT 100
		`, status)
	} else {
		rows, err = qs.db.Query(ctx, `
			SELECT id, session_id, source_type, source_url, raw_content, content_hash, validation_status, validation_notes, COALESCE(promoted_memory_id, 0), expires_at, created_at
			FROM external_quarantine
			ORDER BY created_at DESC
			LIMIT 100
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("quarantine: list: %w", err)
	}

	items := make([]QuarantineItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, *rowToQuarantineItem(row))
	}
	return items, nil
}

// ============================================================================
// Approve — promote quarantined content to agent-visible memory
// ============================================================================

// ApproveQuarantine approves a quarantined item.
// It marks the item as 'validated' and copies the content to memory_events.
func (qs *QuarantineService) ApproveQuarantine(ctx context.Context, id int64, sessionID string) (*QuarantineItem, error) {
	// Fetch the item first
	rows, err := qs.db.Query(ctx, `
		SELECT id, session_id, source_type, source_url, raw_content, content_hash, validation_status, validation_notes, COALESCE(promoted_memory_id, 0), expires_at, created_at
		FROM external_quarantine
		WHERE id = $1
	`, id)
	if err != nil {
		return nil, fmt.Errorf("quarantine: fetch for approve: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("quarantine: item %d not found", id)
	}

	item := rowToQuarantineItem(rows[0])

	if item.ValidationStatus != StatusPending {
		return nil, fmt.Errorf("quarantine: item %d is not pending (status: %s)", id, item.ValidationStatus)
	}

	// Copy the content to memory_events for the agent to see
	var promotedID int64
	if item.RawContent != "" {
		now := time.Now().UTC().Format(time.RFC3339)
		err = qs.db.Exec(ctx, `
			INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
			VALUES ('tool_result', $1, $2, 0, $3)
		`, item.RawContent, sessionID, now)
		if err != nil {
			return nil, fmt.Errorf("quarantine: promote to memory_events: %w", err)
		}

		// Get the inserted memory event ID
		memRows, err := qs.db.Query(ctx, `
			SELECT id FROM memory_events WHERE session_id = $1 ORDER BY id DESC LIMIT 1
		`, sessionID)
		if err == nil && len(memRows) > 0 {
			promotedID = toInt64(memRows[0]["id"])
		}
	}

	// Update quarantine status to validated
	err = qs.db.Exec(ctx, `
		UPDATE external_quarantine
		SET validation_status = 'validated', validation_notes = 'Approved by operator', promoted_memory_id = $1
		WHERE id = $2
	`, promotedID, id)
	if err != nil {
		return nil, fmt.Errorf("quarantine: update to validated: %w", err)
	}

	item.ValidationStatus = StatusValidated
	item.PromotedMemoryID = promotedID
	item.ValidationNotes = "Approved by operator"

	slog.Info("quarantine: item approved", "id", id, "promoted_memory_id", promotedID)

	if qs.publish != nil {
		qs.emitEvent(item, "quarantine_approved")
	}

	return item, nil
}

// ============================================================================
// Reject — mark quarantined item as rejected with reason
// ============================================================================

// RejectQuarantine rejects a quarantined item with a reason.
func (qs *QuarantineService) RejectQuarantine(ctx context.Context, id int64, reason string) (*QuarantineItem, error) {
	if reason == "" {
		reason = "Rejected by operator"
	}

	// Fetch the item first
	rows, err := qs.db.Query(ctx, `
		SELECT id, session_id, source_type, source_url, raw_content, content_hash, validation_status, validation_notes, COALESCE(promoted_memory_id, 0), expires_at, created_at
		FROM external_quarantine
		WHERE id = $1
	`, id)
	if err != nil {
		return nil, fmt.Errorf("quarantine: fetch for reject: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("quarantine: item %d not found", id)
	}

	item := rowToQuarantineItem(rows[0])

	err = qs.db.Exec(ctx, `
		UPDATE external_quarantine
		SET validation_status = 'rejected', validation_notes = $1
		WHERE id = $2
	`, reason, id)
	if err != nil {
		return nil, fmt.Errorf("quarantine: update to rejected: %w", err)
	}

	item.ValidationStatus = StatusRejected
	item.ValidationNotes = reason

	slog.Info("quarantine: item rejected", "id", id, "reason", reason)

	if qs.publish != nil {
		qs.emitEvent(item, "quarantine_rejected")
	}

	return item, nil
}

// ============================================================================
// Scan Pending — process all pending quarantine items
// ============================================================================

// ScanPendingQuarantine scans all pending quarantine items using the heuristic
// scanner. Approved items stay as pending (awaiting human or automated promotion),
// while rejected items are updated inline.
//
// Returns the count of items that were processed.
func (qs *QuarantineService) ScanPendingQuarantine(ctx context.Context) (int, error) {
	items, err := qs.ListQuarantine(ctx, StatusPending)
	if err != nil {
		return 0, fmt.Errorf("quarantine: list pending for scan: %w", err)
	}

	processed := 0
	config := DefaultScannerConfig()

	for _, item := range items {
		result := ScanQuarantinedEvent(item.RawContent, config)
		if result.Status == ScanRejected {
			err := qs.db.Exec(ctx, `
				UPDATE external_quarantine
				SET validation_status = 'rejected', validation_notes = $1
				WHERE id = $2
			`, result.Reason, item.ID)
			if err != nil {
				slog.Warn("quarantine: update rejected after scan", "id", item.ID, "error", err)
				continue
			}
			item.ValidationStatus = StatusRejected
			item.ValidationNotes = result.Reason

			slog.Info("quarantine: scan rejected", "id", item.ID, "score", result.ConfidenceScore, "rules", result.MatchedRules)

			if qs.publish != nil {
				qs.emitEvent(&item, "quarantine_rejected")
			}
		}
		// Approved items stay pending — they need human/API approval to be promoted
		processed++
	}

	return processed, nil
}

// ============================================================================
// SSE Event Emission
// ============================================================================

func (qs *QuarantineService) emitEvent(item *QuarantineItem, eventType string) {
	if qs.publish == nil {
		return
	}

	eventData := map[string]any{
		"quarantine_id":     item.ID,
		"session_id":        item.SessionID,
		"source_type":       item.SourceType,
		"validation_status": item.ValidationStatus,
		"validation_notes":  item.ValidationNotes,
	}
	qs.publish(item.SessionID, eventType, eventData)
}

// ============================================================================
// Row Helpers
// ============================================================================

func rowToQuarantineItem(row db.Row) *QuarantineItem {
	return &QuarantineItem{
		ID:               toInt64(row["id"]),
		SessionID:        toString(row["session_id"]),
		SourceType:       toString(row["source_type"]),
		SourceURL:        toString(row["source_url"]),
		RawContent:       toString(row["raw_content"]),
		ContentHash:      toString(row["content_hash"]),
		ValidationStatus: toString(row["validation_status"]),
		ValidationNotes:  toString(row["validation_notes"]),
		PromotedMemoryID: toInt64(row["promoted_memory_id"]),
		ExpiresAt:        toString(row["expires_at"]),
		CreatedAt:        toString(row["created_at"]),
	}
}

func toInt64(v any) int64 {
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

func toString(v any) string {
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
