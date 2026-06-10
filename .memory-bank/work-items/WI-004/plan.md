# WI-004: Cognitive Firewall — Implementation Plan

**axiom:trace work_item=WI-004 spec=specs/005-security.md,specs/013-webhooks-and-events.md plan=plan.md**

## Steps

### Step 1: Create scanner package (`internal/quarantine/scanner.go`)
- Define `ScanResult` struct
- Implement `ScanQuarantinedEvent(content string) ScanResult`
- Pattern categories with weighted scoring
- `scanner_version` constant

### Step 2: Create quarantine service (`internal/quarantine/service.go`)
- `QuarantineService` with DB operations
- `InsertQuarantine(ctx, eventID, source, payload)` — insert into `external_quarantine`
- `ListQuarantine(ctx, status)` — list with optional status filter
- `ApproveQuarantine(ctx, id)` — approve → copy to `tool_results` or `memory_events`
- `RejectQuarantine(ctx, id, reason)` — mark rejected with reason
- `ScanPendingQuarantine(ctx)` — scan all pending items

### Step 3: Wire webhook ingestion (`internal/webhook/webhook.go`)
- After `IngestEvent()` successfully inserts an event:
  - If signature invalid OR content scan detects threats → insert into `external_quarantine`
- Modify event status to `quarantined` when suspicious content found

### Step 4: Add API routes (`internal/api/server.go` + `internal/api/quarantine.go`)
- Register `/api/v1/quarantine` routes
- Implement handlers: list, approve, reject

### Step 5: Add SSE events (`internal/api/events.go`)
- Add `QuarantineEvent` type
- Wire into QuarantineService to emit events on approve/reject

### Step 6: Migration (`migrations/010_quarantine_scanner.sql`)
- Add 'webhook' to `external_quarantine.source_type` CHECK constraint

### Step 7: Tests
- `internal/quarantine/scanner_test.go` — unit tests for scanner patterns
- `internal/webhook/webhook_test.go` — add quarantine integration test
- `internal/api/quarantine_test.go` — API endpoint tests

### Step 8: Commit
- Conventional commit with Co-authored-by trailer
- Trace markers in all new/modified files
