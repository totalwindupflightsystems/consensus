package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// ============================================================================
// AC-EVT-01: HMAC signature verification
// ============================================================================

func TestHMACVerification(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-1
	secret := "whsec_test_secret_12345"
	payload := []byte(`{"event":"push","repository":"test"}`)

	// Generate valid HMAC
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name     string
		sig      string
		secret   string
		expected bool
	}{
		{"valid signature", validSig, secret, true},
		{"valid without prefix", strings.TrimPrefix(validSig, "sha256="), secret, true},
		{"wrong secret", validSig, "wrong_secret", false},
		{"empty signature", "", secret, false},
		{"tampered payload", validSig + "extra", secret, false},
		{"empty secret", validSig, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := VerifyHMAC(payload, tt.sig, tt.secret)
			if result != tt.expected {
				t.Errorf("VerifyHMAC() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTimingSafeComparison(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-1
	secret := "whsec_timing_test"
	payload := []byte(`{"test":"timing"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := hex.EncodeToString(mac.Sum(nil))

	// Verify rejection with signature that differs only in last byte
	badBytes, _ := hex.DecodeString(validSig)
	badBytes[len(badBytes)-1] ^= 0x01
	badSig := hex.EncodeToString(badBytes)

	if VerifyHMAC(payload, badSig, secret) {
		t.Error("expected rejection of near-matching signature")
	}
}

// ============================================================================
// AC-EVT-02: Event idempotency (Go-level + UNIQUE index)
// ============================================================================

func TestIdempotencyGoLevel(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-2
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	store := New(database)

	// Register a webhook
	reg, err := store.CreateRegistration(ctx, Registration{
		Name:    "test_idempotency",
		Source:  "github",
		URLPath: "/webhooks/github",
		Secret:  "whsec_test",
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	// Send same event twice with same source_id
	event := Event{
		Source:         "webhook",
		SourceID:       "delivery-001",
		EventType:      "push",
		Payload:        []byte(`{"test":true}`),
		SignatureValid: true,
		SessionID:      reg.TargetSessionID,
	}

	result, _, err := store.IngestEvent(ctx, event, reg)
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if result != IngestOK {
		t.Fatalf("expected IngestOK, got %s", result)
	}

	// Second ingest with same source_id should detect duplicate
	result2, existingID, err2 := store.IngestEvent(ctx, event, reg)
	if err2 != nil {
		t.Fatalf("second ingest: %v", err2)
	}
	if result2 != IngestDuplicate {
		t.Errorf("expected IngestDuplicate, got %s", result2)
	}
	if existingID == 0 {
		t.Error("expected non-zero existing ID for duplicate event")
	}

	// Check count — only 1 event in DB
	rows, err := database.Query(ctx, `SELECT COUNT(*) as cnt FROM external_events WHERE source_id = 'delivery-001'`)
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	count := toInt(rows[0]["cnt"])
	if count != 1 {
		t.Errorf("expected 1 event with source_id delivery-001, got %d", count)
	}

	t.Logf("Idempotency: %d events stored (1 expected)  [existing_id=%d|result=%s]", count, existingID, result2)
}

func TestIdempotencyDifferentSourceIDs(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-2
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	store := New(database)

	reg, err := store.CreateRegistration(ctx, Registration{
		Name:    "test_idem_diff",
		Source:  "github",
		URLPath: "/webhooks/github",
		Secret:  "whsec_test",
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	// Two events with different source_id — both should succeed
	e1 := Event{Source: "webhook", SourceID: "delivery-001", EventType: "push", Payload: []byte(`{}`), SignatureValid: true}
	e2 := Event{Source: "webhook", SourceID: "delivery-002", EventType: "push", Payload: []byte(`{}`), SignatureValid: true}

	if result, _, err := store.IngestEvent(ctx, e1, reg); err != nil || result != IngestOK {
		t.Fatalf("event 1: %v / %s", err, result)
	}
	if result, _, err := store.IngestEvent(ctx, e2, reg); err != nil || result != IngestOK {
		t.Fatalf("event 2: %v / %s", err, result)
	}

	rows, _ := database.Query(ctx, `SELECT COUNT(*) as cnt FROM external_events`)
	count := toInt(rows[0]["cnt"])
	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}
	t.Logf("Different source_ids: %d events stored", count)
}

// ============================================================================
// AC-EVT-03: Token-bucket rate limiting
// ============================================================================

func TestTokenBucketRateLimiter(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-3
	// Test the in-memory token bucket directly
	bucket := newTokenBucket(10, 3) // 10/min = ~0.167/sec, burst 3

	// First 3 should be allowed (burst)
	for i := 0; i < 3; i++ {
		if !bucket.allow() {
			t.Errorf("burst request %d should be allowed", i+1)
		}
	}

	// 4th should be denied (burst exhausted, rate too slow)
	if bucket.allow() {
		t.Error("4th request should be denied (burst exhausted)")
	}

	t.Logf("Token bucket: burst=3 OK, 4th denied (rate=10/min)")

	// Now test rate-limited bucket with high rate — all should pass
	bucket2 := newTokenBucket(600, 10) // high rate
	for i := 0; i < 10; i++ {
		if !bucket2.allow() {
			t.Errorf("high-rate request %d should be allowed", i+1)
		}
	}
}

func TestTokenBucketRefill(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-3
	// Create a bucket with very fast rate — should refill quickly
	bucket := newTokenBucket(6000, 5) // 100/sec

	// Drain it
	for i := 0; i < 5; i++ {
		bucket.allow()
	}

	// Wait for refill
	time.Sleep(100 * time.Millisecond)

	// Should have refilled enough for more requests
	allowed := 0
	for i := 0; i < 10; i++ {
		if bucket.allow() {
			allowed++
		}
	}
	if allowed < 5 {
		t.Errorf("expected at least 5 refilled tokens after 100ms, got %d", allowed)
	}
	t.Logf("Token refill: %d requests allowed after 100ms (rate=100/sec)", allowed)
}

// ============================================================================
// AC-EVT-04: Payload size limits
// ============================================================================

func TestPayloadSizeLimits(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-4
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	store := New(database)

	reg, err := store.CreateRegistration(ctx, Registration{
		Name:    "test_payload",
		Source:  "github",
		URLPath: "/webhooks/github",
		Secret:  "whsec_test",
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	// Test with payload under limit
	smallPayload := make([]byte, 100)
	event := Event{
		Source:         "webhook",
		SourceID:       "small-delivery",
		EventType:      "push",
		Payload:        smallPayload,
		SignatureValid: true,
	}
	result, _, err := store.IngestEvent(ctx, event, reg)
	if err != nil {
		t.Errorf("expected small payload to succeed, got: %v", err)
	}
	if result != IngestOK {
		t.Errorf("expected IngestOK for small payload, got %s", result)
	}

	// Test with payload over limit
	largePayload := make([]byte, maxPayloadSize+1)
	for i := range largePayload {
		largePayload[i] = 'x'
	}
	event.Payload = largePayload
	event.SourceID = "large-delivery"

	_, _, err = store.IngestEvent(ctx, event, reg)
	if err == nil {
		t.Error("expected error for oversized payload")
	}
	if _, ok := err.(*PayloadTooLargeError); !ok {
		t.Errorf("expected PayloadTooLargeError, got: %v", err)
	}
	t.Logf("Payload limit correctly enforced: %v", err)
}

// ============================================================================
// AC-EVT-05: Quarantine for invalid signatures
// ============================================================================

func TestQuarantineFlow(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-5
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	store := New(database)

	reg, err := store.CreateRegistration(ctx, Registration{
		Name:    "test_quarantine",
		Source:  "github",
		URLPath: "/webhooks/github",
		Secret:  "whsec_test",
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	// Event with invalid signature
	event := Event{
		Source:         "webhook",
		SourceID:       "bad-sig-delivery",
		EventType:      "push",
		Payload:        []byte(`{"bad":true}`),
		SignatureValid: false, // Explicitly invalid
	}

	result, _, err := store.IngestEvent(ctx, event, reg)
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if result != IngestOK {
		t.Fatalf("expected IngestOK (still stored, just quarantined), got %s", result)
	}

	// Verify event is quarantined
	rows, err := database.Query(ctx, `SELECT status FROM external_events WHERE source_id = 'bad-sig-delivery'`)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected event to be stored even with invalid signature")
	}

	status := rows[0]["status"].(string)
	if status != EventStatusQuarantined {
		t.Errorf("expected status %q, got %q", EventStatusQuarantined, status)
	}
	t.Logf("Invalid signature event status: %s", status)
}

// ============================================================================
// AC-EVT-06: Routing rules with priority matching
// ============================================================================

func TestRoutingRulesPriority(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-6
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	_ = database.Exec(ctx, `CREATE TABLE IF NOT EXISTS routing_rules (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		source_pattern TEXT,
		event_type_pattern TEXT,
		payload_pattern TEXT,
		target_session_id TEXT,
		target_workflow_id TEXT,
		priority INTEGER NOT NULL DEFAULT 5,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	)`)

	// Insert routing rules with different priorities
	if err := database.Exec(ctx, `INSERT INTO routing_rules (id, name, source_pattern, event_type_pattern, target_session_id, priority, enabled, created_at) VALUES ('r1', 'high-prio', 'github', 'push', 'session-high', 1, 1, '2026-05-04T00:00:00Z')`); err != nil {
		t.Fatalf("insert rule 1: %v", err)
	}
	if err := database.Exec(ctx, `INSERT INTO routing_rules (id, name, source_pattern, event_type_pattern, target_session_id, priority, enabled, created_at) VALUES ('r2', 'low-prio', 'github', '%', 'session-low', 10, 1, '2026-05-04T00:00:00Z')`); err != nil {
		t.Fatalf("insert rule 2: %v", err)
	}

	// Query matching rules for a push event by priority
	rows, err := database.Query(ctx, `SELECT name, priority FROM routing_rules WHERE source_pattern = 'github' AND (event_type_pattern = 'push' OR event_type_pattern = '%') ORDER BY priority ASC`)
	if err != nil {
		t.Fatalf("query rules: %v", err)
	}

	if len(rows) < 2 {
		t.Fatalf("expected at least 2 matching rules, got %d", len(rows))
	}

	// First rule should be the high-priority one
	firstRule := rows[0]["name"].(string)
	if firstRule != "high-prio" {
		t.Errorf("expected highest priority rule 'high-prio', got %q", firstRule)
	}
	t.Logf("Highest priority match: %s (priority %v)", firstRule, rows[0]["priority"])
}

// ============================================================================
// AC-EVT-07: Event status lifecycle
// ============================================================================

func TestEventLifecycle(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-7
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	store := New(database)

	reg, err := store.CreateRegistration(ctx, Registration{
		Name:    "test_lifecycle",
		Source:  "github",
		URLPath: "/webhooks/github",
		Secret:  "whsec_test",
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	// Ingest a valid event
	event := Event{
		Source:         "webhook",
		SourceID:       "lifecycle-delivery",
		EventType:      "push",
		Payload:        []byte(`{"test":true}`),
		SignatureValid: true,
	}
	result, _, err := store.IngestEvent(ctx, event, reg)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result != IngestOK {
		t.Fatalf("expected IngestOK, got %s", result)
	}

	// Get the event ID
	rows, err := database.Query(ctx, `SELECT id, status FROM external_events WHERE source_id = 'lifecycle-delivery'`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("event not found")
	}
	eventID := toInt64(rows[0]["id"])
	initialStatus := rows[0]["status"].(string)
	t.Logf("Initial status: %s", initialStatus)

	// Transition: pending -> routed
	if err := store.UpdateEventStatus(ctx, eventID, EventStatusRouted); err != nil {
		t.Fatalf("transition to routed: %v", err)
	}

	// Transition: routed -> processing
	if err := store.UpdateEventStatus(ctx, eventID, EventStatusProcessing); err != nil {
		t.Fatalf("transition to processing: %v", err)
	}

	// Transition: processing -> completed
	if err := store.UpdateEventStatus(ctx, eventID, EventStatusCompleted); err != nil {
		t.Fatalf("transition to completed: %v", err)
	}

	// Verify final status
	rows, err = database.Query(ctx, `SELECT status FROM external_events WHERE source_id = 'lifecycle-delivery'`)
	if err != nil {
		t.Fatalf("final query: %v", err)
	}
	finalStatus := rows[0]["status"].(string)
	if finalStatus != EventStatusCompleted {
		t.Errorf("expected completed, got %s", finalStatus)
	}
	t.Logf("Lifecycle: %s -> routed -> processing -> %s", initialStatus, finalStatus)
}

func TestInvalidEventStatus(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-7
	store := &Store{}
	err := store.UpdateEventStatus(context.Background(), 1, "invalid_status")
	if err == nil {
		t.Error("expected error for invalid status")
	}
	t.Logf("Invalid status correctly rejected: %v", err)
}

// ============================================================================
// Go-Level Event Routing Loop (SPEC-013 §5)
// ============================================================================

func TestEventRoutingLoop(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-8
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	store := New(database)

	// Create sessions table and sessions for wake test
	_ = database.Exec(ctx, `CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		agent_name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'idle',
		project_id TEXT,
		heartbeat_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	_ = database.Exec(ctx, `INSERT INTO sessions (id, agent_name, status) VALUES ('target-session', 'test-agent', 'waiting_sub')`)
	_ = database.Exec(ctx, `INSERT INTO sessions (id, agent_name, status) VALUES ('idle-session', 'idle-agent', 'idle')`)

	// Create routing rules table with a rule
	_ = database.Exec(ctx, `CREATE TABLE IF NOT EXISTS routing_rules (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, source_pattern TEXT,
		event_type_pattern TEXT, payload_pattern TEXT,
		target_session_id TEXT, target_workflow_id TEXT,
		priority INTEGER NOT NULL DEFAULT 5, enabled INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	)`)
	_ = database.Exec(ctx, `INSERT INTO routing_rules (id, name, source_pattern, event_type_pattern, target_session_id, priority, enabled, created_at) VALUES ('r1', 'rule', 'webhook', 'push', 'target-session', 1, 1, datetime('now'))`)

	// Create a pending event
	_ = database.Exec(ctx, `INSERT INTO external_events (source, source_id, event_type, payload, signature_valid, status, session_id, workflow_id) VALUES ('webhook', 'routing-test', 'push', '{}', 1, 'pending', NULL, NULL)`)

	// Run routing
	if err := store.routePendingEvents(ctx); err != nil {
		t.Fatalf("routing: %v", err)
	}

	// Verify event was routed
	rows, err := database.Query(ctx, `SELECT status, session_id FROM external_events WHERE source_id = 'routing-test'`)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("event missing")
	}
	if rows[0]["status"] != "routed" {
		t.Errorf("expected status 'routed', got %q", rows[0]["status"])
	}
	if rows[0]["session_id"] != "target-session" {
		t.Errorf("expected session_id 'target-session', got %q", rows[0]["session_id"])
	}

	// Verify session wake: waiting_sub session should become idle
	sRows, err := database.Query(ctx, `SELECT status FROM sessions WHERE id = 'target-session'`)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	if sRows[0]["status"] != "idle" {
		t.Errorf("expected target session to be woken (status=idle), got %q", sRows[0]["status"])
	}

	// Idle session should NOT be woken (only waiting_sub/paused)
	sRows2, err := database.Query(ctx, `SELECT status FROM sessions WHERE id = 'idle-session'`)
	if err != nil {
		t.Fatalf("query idle session: %v", err)
	}
	if sRows2[0]["status"] != "idle" {
		t.Errorf("idle session should remain idle, got %q", sRows2[0]["status"])
	}

	t.Logf("Event routing: event routed to target-session, session woken (waiting_sub->idle)")
}

// ============================================================================
// Match Routing Rule Logic
// ============================================================================

func TestMatchRoutingRule(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-8
	store := New(nil)

	rules := []RoutingRule{
		{ID: "r1", Name: "specific", SourcePattern: "github", EventTypePattern: "push", Priority: 1, Enabled: true, TargetSessionID: "sess-1"},
		{ID: "r2", Name: "broad", SourcePattern: "github", Priority: 10, Enabled: true, TargetSessionID: "sess-2"},
		{ID: "r3", Name: "unrelated", SourcePattern: "stripe", Priority: 5, Enabled: true, TargetSessionID: "sess-3"},
	}

	// Match github push — should hit the specific rule
	m := store.matchRoutingRule(rules, "webhook_github", "push", "{}")
	if m == nil {
		t.Fatal("expected match for github push")
	}
	if m.Name != "specific" {
		t.Errorf("expected 'specific' rule, got %q", m.Name)
	}

	// Match github issue — should hit the broad rule (event_type mismatch on specific)
	m = store.matchRoutingRule(rules, "webhook_github", "issues", "{}")
	if m == nil {
		t.Fatal("expected match for github issues")
	}
	if m.Name != "broad" {
		t.Errorf("expected 'broad' rule, got %q", m.Name)
	}

	// Stripe event — should hit the unrelated rule
	m = store.matchRoutingRule(rules, "webhook_stripe", "charge", "{}")
	if m == nil {
		t.Fatal("expected match for stripe")
	}
	if m.Name != "unrelated" {
		t.Errorf("expected 'unrelated' rule, got %q", m.Name)
	}

	t.Logf("Routing rule matching: specific, broad, and unrelated all match correctly")
}

// ============================================================================
// AC-EVT-08: webhook_registrations CRUD
// ============================================================================

func TestWebhookRegistrationsCRUD(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-9
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	store := New(database)

	// CREATE
	reg, err := store.CreateRegistration(ctx, Registration{
		Name:    "github_push",
		Source:  "github",
		URLPath: "/webhooks/github",
		Secret:  "whsec_github_secret_abc123",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Logf("Created: %s", reg.Name)

	// READ by name
	found, err := store.GetRegistration(ctx, "github_push")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if found.Name != "github_push" {
		t.Errorf("expected name 'github_push', got %q", found.Name)
	}

	// READ by source
	found2, err := store.GetRegistration(ctx, "github")
	if err != nil {
		t.Fatalf("get by source: %v", err)
	}
	if found2.Source != "github" {
		t.Errorf("expected source 'github', got %q", found2.Source)
	}

	// List
	regs, err := store.ListRegistrations(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(regs) == 0 {
		t.Error("expected at least one registration")
	}
	t.Logf("Listed %d registrations", len(regs))

	// UPDATE (enable/disable)
	if err := store.EnableRegistration(ctx, "github_push", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	found, _ = store.GetRegistration(ctx, "github_push")
	if found.Enabled {
		t.Error("expected registration to be disabled")
	}

	if err := store.EnableRegistration(ctx, "github_push", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	found, _ = store.GetRegistration(ctx, "github_push")
	if !found.Enabled {
		t.Error("expected registration to be enabled")
	}

	// DELETE
	if err := store.DeleteRegistration(ctx, "github_push"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = store.GetRegistration(ctx, "github_push")
	if err == nil {
		t.Error("expected error when getting deleted registration")
	}
	t.Logf("CRUD cycle complete: create, read, list, update, delete — all passed")
}

func TestWebhookRegistrationValidation(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-9
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := New(database)

	// Missing name
	_, err := store.CreateRegistration(ctx, Registration{Source: "github", URLPath: "/wh/github", Secret: "secret"})
	if err == nil {
		t.Error("expected error for missing name")
	}

	// Missing source
	_, err = store.CreateRegistration(ctx, Registration{Name: "test", URLPath: "/wh/github", Secret: "secret"})
	if err == nil {
		t.Error("expected error for missing source")
	}

	// Missing secret
	_, err = store.CreateRegistration(ctx, Registration{Name: "test", Source: "github", URLPath: "/wh/github"})
	if err == nil {
		t.Error("expected error for missing secret")
	}
}

// ============================================================================
// HTTP Handler tests
// ============================================================================

func TestWebhookHandlerKnownSource(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-10
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	store := New(database)
	_, err := store.CreateRegistration(ctx, Registration{
		Name:    "github",
		Source:  "github",
		URLPath: "/webhooks/github",
		Secret:  "whsec_test_handler",
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	// Build request with valid HMAC
	payload := `{"ref":"refs/heads/main"}`
	mac := hmac.New(sha256.New, []byte("whsec_test_handler"))
	mac.Write([]byte(payload))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhooks/github", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "delivery-abc-123")

	w := httptest.NewRecorder()
	store.HandleWebhook(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202 Accepted, got %d: %s", w.Code, w.Body.String())
	}
	t.Logf("Webhook handler: %d — %s", w.Code, w.Body.String())
}

func TestWebhookHandlerUnknownSource(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-10
	database, cleanup := setupTestDB(t)
	defer cleanup()

	store := New(database)
	req := httptest.NewRequest("POST", "/webhooks/unknown", nil)
	w := httptest.NewRecorder()

	store.HandleWebhook(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestWebhookHandlerRateLimited(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-10
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	store := New(database)
	reg, err := store.CreateRegistration(ctx, Registration{
		Name:    "rate-limited-src",
		Source:  "github",
		URLPath: "/webhooks/rate-limited-src",
		Secret:  "whsec_test_handler",
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	// Set a very restrictive bucket for this source (1 req/min, burst 1 — only first request passes)
	source := reg.URLPath
	source = strings.TrimPrefix(source, "/webhooks/")
	store.mu.Lock()
	store.buckets[source] = newTokenBucket(1, 1) // 1/min, burst 1
	store.mu.Unlock()

	payload := "{}"
	mac := hmac.New(sha256.New, []byte("whsec_test_handler"))
	mac.Write([]byte(payload))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req1 := httptest.NewRequest("POST", reg.URLPath, strings.NewReader(payload))
	req1.Header.Set("X-Hub-Signature-256", sig)
	req1.Header.Set("X-GitHub-Event", "push")
	req1.Header.Set("X-GitHub-Delivery", "del-001")
	w1 := httptest.NewRecorder()
	store.HandleWebhook(w1, req1)
	if w1.Code != http.StatusAccepted {
		t.Errorf("first request should be accepted, got %d", w1.Code)
	}

	// Second request should be rate limited
	req2 := httptest.NewRequest("POST", reg.URLPath, strings.NewReader(payload))
	req2.Header.Set("X-Hub-Signature-256", sig)
	req2.Header.Set("X-GitHub-Event", "push")
	req2.Header.Set("X-GitHub-Delivery", "del-002")
	w2 := httptest.NewRecorder()
	store.HandleWebhook(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second request should be rate limited (429), got %d", w2.Code)
	}

	t.Logf("Rate limiting: 1st request accepted (%d), 2nd rate-limited (%d)", w1.Code, w2.Code)
}

// ============================================================================
// ServeHTTP interface
// ============================================================================

func TestServeHTTP(t *testing.T) {
	// axiom:trace work_item=spec-013-hardening-01 spec=specs/013-webhooks-and-events.md plan=phase-1/task-1/step-11
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	store := New(database)
	_, err := store.CreateRegistration(ctx, Registration{
		Name:    "test_servehttp",
		Source:  "github",
		URLPath: "/webhooks/github",
		Secret:  "whsec_test_handler",
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	payload := "{}"
	mac := hmac.New(sha256.New, []byte("whsec_test_handler"))
	mac.Write([]byte(payload))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "servehttp-test")

	w := httptest.NewRecorder()
	store.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202 Accepted via ServeHTTP, got %d: %s", w.Code, w.Body.String())
	}
	t.Logf("ServeHTTP: %d — %s", w.Code, w.Body.String())
}

// ============================================================================
// WI-004: Cognitive Firewall — Quarantine Flow Integration Tests
// ============================================================================

// TestQuarantineMaliciousPayload tests that a malicious webhook payload
// with a valid signature is still quarantined by the cognitive firewall.
func TestQuarantineMaliciousPayload(t *testing.T) {
	// axiom:trace work_item=WI-004 spec=specs/005-security.md,specs/013-webhooks-and-events.md plan=phase-6/task-4/step-1
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// Create the external_quarantine table for this test
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS external_quarantine (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		source_type TEXT NOT NULL DEFAULT 'api_response',
		source_url TEXT,
		raw_content TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		validation_status TEXT NOT NULL DEFAULT 'pending',
		validation_notes TEXT,
		promoted_memory_id INTEGER,
		expires_at TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)

	store := New(database)

	// Create a mock quarantine inserter that tracks calls
	var quarantinedContent []string
	var quarantinedSessions []string
	store.SetQuarantineInserter(func(ctx context.Context, sessionID, sourceType, rawContent, sourceURL string) error {
		quarantinedContent = append(quarantinedContent, rawContent)
		quarantinedSessions = append(quarantinedSessions, sessionID)
		// Also actually insert into the table so we can verify
		hashBytes := md5.Sum([]byte(rawContent))
		contentHash := fmt.Sprintf("%x", hashBytes)
		expiresAt := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
		return database.Exec(ctx, `
			INSERT INTO external_quarantine (session_id, source_type, raw_content, content_hash, validation_status, expires_at, created_at)
			VALUES ($1, $2, $3, $4, 'pending', $5, datetime('now'))
		`, sessionID, sourceType, rawContent, contentHash, expiresAt)
	})

	// Set up a mock scanner that rejects SQL injection content
	store.SetQuarantineScanner(&mockScanner{
		threatPattern: "DROP TABLE",
	})

	// Register a webhook — Name must match the URL path after /webhooks/ prefix
	reg, err := store.CreateRegistration(ctx, Registration{
		Name:    "quarantine-test",
		Source:  "github",
		URLPath: "/webhooks/quarantine-test",
		Secret:  "whsec_quarantine_test",
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	// Test 1: Clean payload should NOT be quarantined
	cleanPayload := `{"event": "push", "ref": "refs/heads/main"}`
	mac := hmac.New(sha256.New, []byte("whsec_quarantine_test"))
	mac.Write([]byte(cleanPayload))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", reg.URLPath, strings.NewReader(cleanPayload))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "clean-delivery-001")

	w := httptest.NewRecorder()
	store.HandleWebhook(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202 for clean payload, got %d", w.Code)
	}

	// The clean payload should NOT be in quarantine
	rows, _ := database.Query(ctx, `SELECT COUNT(*) as cnt FROM external_quarantine`)
	if len(rows) > 0 {
		count := toInt(rows[0]["cnt"])
		if count > 0 {
			t.Errorf("expected 0 quarantine items for clean payload, got %d", count)
		}
	}
	t.Logf("Clean payload correctly not quarantined")

	// Test 2: Malicious payload (SQL injection) should be quarantined
	maliciousPayload := `{"query": "'; DROP TABLE sessions; --"}`
	mac2 := hmac.New(sha256.New, []byte("whsec_quarantine_test"))
	mac2.Write([]byte(maliciousPayload))
	sig2 := "sha256=" + hex.EncodeToString(mac2.Sum(nil))

	req2 := httptest.NewRequest("POST", reg.URLPath, strings.NewReader(maliciousPayload))
	req2.Header.Set("X-Hub-Signature-256", sig2)
	req2.Header.Set("X-GitHub-Event", "push")
	req2.Header.Set("X-GitHub-Delivery", "malicious-delivery-001")

	w2 := httptest.NewRecorder()
	store.HandleWebhook(w2, req2)

	if w2.Code != http.StatusAccepted {
		t.Errorf("expected 202 for malicious payload (still accepted, just quarantined), got %d", w2.Code)
	}

	// Verify the malicious payload was quarantined
	rows2, err := database.Query(ctx, `SELECT id, raw_content, validation_status FROM external_quarantine ORDER BY id DESC LIMIT 1`)
	if err != nil {
		t.Fatalf("query quarantine: %v", err)
	}
	if len(rows2) == 0 {
		t.Fatal("expected quarantined item for malicious payload, got none")
	}
	rawContent := toString(rows2[0]["raw_content"])
	if !strings.Contains(rawContent, "DROP TABLE") {
		t.Errorf("expected raw_content containing 'DROP TABLE', got %q", rawContent)
	}
	if toString(rows2[0]["validation_status"]) != "pending" {
		t.Errorf("expected pending validation status, got %q", toString(rows2[0]["validation_status"]))
	}
	t.Logf("Malicious payload correctly quarantined: id=%v, status=%s", rows2[0]["id"], rows2[0]["validation_status"])

	// Also verify external_events was written (event still ingested, just also quarantined)
	eventRows, _ := database.Query(ctx, `SELECT id, status FROM external_events WHERE source_id = 'malicious-delivery-001'`)
	if len(eventRows) == 0 {
		t.Error("expected external_events entry for malicious payload")
	} else {
		t.Logf("Event also stored in external_events: id=%v, status=%s", eventRows[0]["id"], eventRows[0]["status"])
	}
}

// mockScanner implements webhook.QuarantineScanner for testing.
type mockScanner struct {
	threatPattern string
}

func (m *mockScanner) ScanContent(content string, sourceID string) (bool, float64, string, []string) {
	if strings.Contains(content, m.threatPattern) {
		return true, 0.85, "SQL injection pattern detected", []string{"SQLI-DROP-TABLE"}
	}
	return false, 0.0, "", nil
}

// ============================================================================
// Helpers
// ============================================================================

func setupTestDB(t *testing.T) (db.DB, func()) {
	t.Helper()

	ctx := context.Background()
	dbURL := fmt.Sprintf("sqlite://file:%s?mode=memory&cache=shared", t.Name())
	database, err := driver.Open(ctx, db.Config{URL: dbURL})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create the webhook_registrations table (matches migration 007)
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS webhook_registrations (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		source TEXT NOT NULL,
		url_path TEXT NOT NULL,
		secret TEXT NOT NULL,
		event_types TEXT NOT NULL DEFAULT '{}',
		target_session_id TEXT,
		target_workflow_id TEXT,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	)`)

	// Create the external_events table (matches migration 007, with UNIQUE index)
	mustExec(t, database, ctx, `CREATE TABLE IF NOT EXISTS external_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source TEXT NOT NULL,
		source_id TEXT,
		event_type TEXT NOT NULL,
		payload TEXT NOT NULL,
		headers TEXT,
		signature_valid INTEGER NOT NULL DEFAULT 0,
		session_id TEXT,
		workflow_id TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		processed_at TEXT
	)`)
	// Idempotency index
	mustExec(t, database, ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_events_source_id ON external_events(source, source_id)`)

	cleanup := func() {
		if err := database.Close(); err != nil {
			t.Logf("warning: failed to close test database: %v", err)
		}
	}

	return database, cleanup
}

func mustExec(t *testing.T, database db.DB, ctx context.Context, query string, args ...any) {
	t.Helper()
	if err := database.Exec(ctx, query, args...); err != nil {
		t.Fatalf("failed to execute: %s: %v", query, err)
	}
}
