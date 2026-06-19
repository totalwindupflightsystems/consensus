# WI-004: Cognitive Firewall (Quarantine Scanner) — Progress

**axiom:trace work_item=WI-004 spec=specs/005-security.md,specs/013-webhooks-and-events.md plan=phases-1-6**

## Summary

Implemented the Cognitive Firewall quarantine scanner (CS-GAP-003). All 26 test packages pass, including new scanner unit tests, quarantine service tests, webhook integration tests, and API endpoint tests.

## Files Created

| File | Purpose |
|------|---------|
| `internal/quarantine/scanner.go` | `ScanResult` type, `ScanQuarantinedEvent()` with regex/heuristic patterns (SQL injection, XSS, prompt injection, suspicious structures) |
| `internal/quarantine/service.go` | `QuarantineService` with CRUD operations (Insert, List, Approve, Reject, ScanPending) |
| `internal/quarantine/adapter.go` | `WebhookScannerAdapter` bridging quarantine scanner with webhook |
| `internal/quarantine/doc.go` | Package documentation |
| `internal/quarantine/scanner_test.go` | Scanner unit tests (18 test functions) |
| `internal/quarantine/service_test.go` | Service integration tests (7 test functions) |
| `internal/api/quarantine.go` | API handlers: GET list, POST approve/reject |
| `internal/api/quarantine_test.go` | API endpoint tests (9 test functions) |
| `.memory-bank/work-items/WI-004/meta-planning.md` | Meta-planning document |
| `.memory-bank/work-items/WI-004/plan.md` | Implementation plan |
| `migrations/010_quarantine_scanner.sql` | Migration for 'webhook' source_type in CHECK constraint |

## Files Modified

| File | Change |
|------|--------|
| `internal/webhook/webhook.go` | Added `QuarantineScanner` interface, `SetQuarantineScanner`/`SetQuarantineInserter` methods, post-ingestion scan in `HandleWebhook` |
| `internal/webhook/doc.go` | Updated trace markers |
| `internal/api/server.go` | Added `quarantineSvc` field, `QuarantineService` config, quarantine route dispatch, `EventBus()` accessor |
| `internal/api/events.go` | Added `PublishQuarantineEvent` method |
| `cmd/consensus/main.go` | Wired quarantine service, scanner, inserter into webhook store and API server |
| `internal/webhook/webhook_test.go` | Added `TestQuarantineMaliciousPayload` integration test |

## Test Coverage

- **Scanner**: 18 tests covering clean content, SQL injection (5 variants), XSS (5 variants), prompt injection (5 variants), suspicious structures, empty content, high threshold, content hash, adapter, result types
- **Service**: 7 tests covering insert/list, approve, reject, double-approve rejection, scan pending, event emission
- **API**: 9 tests covering list all, list pending, approve, reject, reject without reason, nonexistent approve, invalid ID, unauthenticated
- **Webhook Integration**: 1 end-to-end test: clean payload bypasses quarantine, malicious payload (DROP TABLE) triggers quarantine

## Architecture

```
Webhook Payload → HandleWebhook → IngestEvent (external_events)
                                      │
                                      ▼
                              ScanQuarantinedEvent()
                                      │
                         ┌────────────┴────────────┐
                         ▼                         ▼
                   Clean (score < threshold)   Suspicious (score >= threshold)
                         │                         │
                         ▼                         ▼
                   Normal routing           InsertQuarantine (external_quarantine)
                                                    │
                                                    ▼
                                            API approve → memory_events
                                            API reject  → rejected status
```
