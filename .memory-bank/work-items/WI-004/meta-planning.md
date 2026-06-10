# WI-004: Cognitive Firewall (Quarantine Scanner) — Meta-Plan

**axiom:trace work_item=WI-004 spec=specs/005-security.md,specs/013-webhooks-and-events.md plan=meta-planning**

## Overview

Gap CS-GAP-003 (CRITICAL): The `external_quarantine` table exists in the schema but has no scanning pipeline. External data flows into `external_events` and eventually `memory_events` without passing through quarantine validation. This meta-plan defines the phases to implement the Cognitive Firewall.

## Key Spec References

- **SPEC-005 §Cognitive Firewall**: External data lands in `external_quarantine` → fast local model scan → clean data promoted to agent memory / infected data rejected with reason
- **SPEC-013 §5.2**: Invalid-signature events reuses `external_quarantine` infrastructure
- **SPEC-013 §8**: HMAC, rate limiting, payload limits already implemented

## Architecture Decision

**MVP approach**: Regex + heuristic-based scanning (replaceable later with a fast local model). The scanner checks payloads for known attack patterns (SQL injection, XSS, prompt injection) and assigns a confidence score.

## Phases

### Phase 1: Scanner Engine
- Define `ScanResult` type (approved/rejected, confidence, reason, scanner_version)
- Implement `ScanQuarantinedEvent()` with regex/heuristic pattern matching
- Pattern categories: SQL injection, XSS, prompt injection, suspicious structures

### Phase 2: Quarantine Service + Webhook Integration
- Create QuarantineService with CRUD operations on `external_quarantine`
- Wire into webhook `IngestEvent()`: after insertion, scan payload → quarantine if suspicious
- Support both invalid-signature quarantine and content-based quarantine

### Phase 3: API Endpoints
- `GET /api/v1/quarantine` — list quarantined items
- `POST /api/v1/quarantine/:id/approve` — approve and promote to memory
- `POST /api/v1/quarantine/:id/reject` — mark rejected with reason

### Phase 4: SSE Events
- Add quarantine status change events to EventBus
- Emit events on approve/reject/scanner actions

### Phase 5: Migration
- Add 'webhook' to `external_quarantine.source_type` CHECK constraint

### Phase 6: Tests
- Scanner unit tests
- Webhook integration tests (malicious payload → quarantine)
- API endpoint tests
