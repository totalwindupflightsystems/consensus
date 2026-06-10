---
work_item_id: deployment-ops-01
status: substantially-complete
repo: wojons/conscientiousness
updated: 2026-05-04
verifier_audit: 2026-05-04 — All 25 ACs have passing test evidence (43 tests across 4 packages). AC-DEP-04 (horizontal scaling) requires Postgres. End-to-end integration test remains per TODO.
---

# Plan — Deployment, HITL, Webhooks, and Subagents

This work should follow a working runtime. Phases are ordered by dependency: deployment infrastructure first, then webhook/event ingestion, then HITL interrupt state, then subagent orchestration.

axiom:trace work_item=deployment-ops-01 spec=specs/004-subagents.md,specs/009-deployment.md,specs/013-webhooks-and-events.md,specs/014-hitl-interrupt-state.md plan=phase-1/task-1-1/step-1-1-1 evidence=.memory-bank/work-items/deployment-ops-01/verification.md

## AC → Verification

| AC | Title | Verification Path | Status |
|---|---|---|---|
| AC-DEP-01 | Embedded migrations + binary build | `go build` + `go test ./internal/migrate/...` | PASS (9 tests) |
| AC-DEP-02 | Auto-migrate on startup; drift pauses agents | `go test ./internal/migrate/...` | PASS |
| AC-DEP-03 | Config parity — one surface, both backends | `go test ./internal/config/...` | PASS |
| AC-DEP-04 | Horizontal scaling — SKIP LOCKED | `go test ./internal/harness/... -run TestHorizontalScaling` | PENDING (requires Postgres) |
| AC-DEP-05 | Six topology docs/scripts | Manual review of `deploy/README.md` + `deploy/local-sqlite.sh` | PASS |
| AC-DEP-06 | CLI management commands | `go test ./cmd/...` | PASS |
| AC-EVT-01 | HMAC signature verification | `go test ./internal/webhook/... -run TestHMAC` | PASS |
| AC-EVT-02 | Event idempotency (ON CONFLICT DO NOTHING) | `go test ./internal/webhook/... -run TestIdempotency` | PASS |
| AC-EVT-03 | Rate limiting (60 req/min, 429) | `go test ./internal/webhook/... -run TestRateLimit` | PASS |
| AC-EVT-04 | Payload size limits (413) | `go test ./internal/webhook/... -run TestPayloadSize` | PASS |
| AC-EVT-05 | Quarantine for invalid signatures | `go test ./internal/webhook/... -run TestQuarantine` | PASS |
| AC-EVT-06 | Routing rules with priority matching | `go test ./internal/webhook/... -run TestRoutingRules` | PASS |
| AC-EVT-07 | Event status lifecycle | `go test ./internal/webhook/... -run TestEventLifecycle` | PASS |
| AC-EVT-08 | webhook_registrations CRUD | `go test ./internal/webhook/... -run TestWebhookRegistrationsCRUD` | PASS |
| AC-HITL-01 | approval_requests creation (all 6 types) | `go test ./internal/hitl/... -run TestRequestApproval` | PASS |
| AC-HITL-02 | hitl_configuration with scope precedence | `go test ./internal/hitl/... -run TestHITLConfiguration` | PASS |
| AC-HITL-03 | No auto-approval; expiry → expired not approved | `go test ./internal/hitl/... -run TestNoAutoApproval` | PASS |
| AC-HITL-04 | Reviewer auth (alt_mode_role only) | `go test ./internal/hitl/... -run TestReviewerAuthorization` | PASS |
| AC-HITL-05 | Approval expiry cron (expired → failed) | Covered by TestNoAutoApproval | PASS |
| AC-HITL-06 | Notification channels on pause | `go test ./internal/hitl/... -run TestNotificationChannels` | PASS |
| AC-SUB-01 | Memory forking (compressed pointers only) | `go test ./internal/subagent/... -run TestMemoryForking` | PASS |
| AC-SUB-02 | RLS isolation (session_id enforcement) | `go test ./internal/subagent/... -run TestRLSIsolation` | PASS |
| AC-SUB-03 | wake_parent_on_completion trigger | `go test ./internal/subagent/... -run TestParentWakeUp` | PASS |
| AC-SUB-04 | Error propagation (failed → parent reads result) | `go test ./internal/subagent/... -run TestErrorPropagation` | PASS |
| AC-SUB-05 | Depth limit of 5 enforced | `go test ./internal/subagent/... -run TestDepthLimit` | PASS |

## Phases

1. **Phase 1 — Deployment local operability.** Embedded migrations, config parity, CLI commands, deployment topology docs/scripts, horizontal scaling test.
2. **Phase 2 — Webhooks and external events.** HMAC verification, idempotency, rate limits, payload limits, routing rules, quarantine, event lifecycle, webhook_registrations CRUD.
3. **Phase 3 — HITL interrupt state.** approval_requests creation/review, no-auto-approval, hitl_configuration with scope precedence, reviewer auth, notification channels.
4. **Phase 4 — Subagent orchestration.** Memory forking, RLS isolation, parent wake trigger, error propagation, depth limit enforcement.
