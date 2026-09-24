// Package quarantine — scanner adapter for webhook integration.
//
// This file provides an adapter that wraps ScanQuarantinedEvent in the
// webhook.QuarantineScanner interface, allowing the quarantine package to
// be used by the webhook handler without circular dependencies.
//
// axiom:trace work_item=WI-004 spec=specs/005-security.md plan=phase-2/task-1/step-2
package quarantine

import (
	"context"
	"fmt"
)

// ============================================================================
// WebhookScannerAdapter — implements webhook.QuarantineScanner
// ============================================================================

// WebhookScannerAdapter wraps the quarantine scanner for use by the webhook handler.
// It provides the ScanContent method that the webhook.QuarantineScanner interface needs.
type WebhookScannerAdapter struct {
	Config ScannerConfig
}

// NewWebhookScannerAdapter creates a new WebhookScannerAdapter with default config.
func NewWebhookScannerAdapter() *WebhookScannerAdapter {
	return &WebhookScannerAdapter{
		Config: DefaultScannerConfig(),
	}
}

// ScanContent scans event content for threats. Returns:
//   - isThreat: true if the content should be quarantined
//   - confidence: the confidence score (0.0-1.0)
//   - reason: human-readable explanation (empty if clean)
//   - matchedRules: list of rule names that triggered (nil if clean)
func (a *WebhookScannerAdapter) ScanContent(content string, sourceID string) (bool, float64, string, []string) {
	result := ScanQuarantinedEvent(content, a.Config)
	if result.Status == ScanRejected {
		return true, result.ConfidenceScore, result.Reason, result.MatchedRules
	}
	return false, result.ConfidenceScore, "", nil
}

// ============================================================================
// QuarantineInserter — creates a quarantine inserter function for the webhook store
// ============================================================================

// NewQuarantineInserter creates a function suitable for webhook.Store.SetQuarantineInserter.
func NewQuarantineInserter(qs *QuarantineService) func(ctx context.Context, sessionID, sourceType, rawContent, sourceURL string) error {
	return func(ctx context.Context, sessionID, sourceType, rawContent, sourceURL string) error {
		if rawContent == "" {
			return fmt.Errorf("quarantine: cannot insert empty content")
		}
		_, err := qs.InsertQuarantine(ctx, InsertQuarantineInput{
			SessionID:   sessionID,
			SourceType:  sourceType,
			RawContent:  rawContent,
			ContentHash: ContentHash(rawContent),
			SourceURL:   sourceURL,
		})
		return err
	}
}
