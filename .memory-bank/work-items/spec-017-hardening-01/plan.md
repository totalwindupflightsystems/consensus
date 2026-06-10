---
work_item_id: spec-017-hardening-01
status: queued
spec: specs/017-ui-adapter-layer.md
source_sweep: sweep-020
created: 2026-05-05
---

# Plan — SPEC-017 opencode Shim Hardening

Remediate shim gaps found during idle sweep-020 of `specs/017-ui-adapter-layer.md`.

axiom:trace work_item=spec-017-hardening-01 spec=specs/017-ui-adapter-layer.md sweep=sweep-020

## Findings (16 gaps)

| ID | Severity | Description |
|----|----------|-------------|
| HARDEN-SHIM-01 | CRITICAL | 4 of 6 SSE event types not emitted (message.created, tool.started, tool.completed, permission.resolved) |
| HARDEN-SHIM-02 | HIGH | PATCH /session/:id missing |
| HARDEN-SHIM-03 | HIGH | GET /session/:id/message/:messageID missing |
| HARDEN-SHIM-04 | HIGH | shim_session_map table unused |
| HARDEN-SHIM-05 | HIGH | api_key missing from create session response |
| HARDEN-SHIM-06 | HIGH | Architectural: shim bypasses native API, speaks directly to DB |
| HARDEN-SHIM-07 | MEDIUM | Standalone GET /event missing |
| HARDEN-SHIM-08 | MEDIUM | PUT /auth/:id missing |
| HARDEN-SHIM-09 | MEDIUM | PATCH /config missing |
| HARDEN-SHIM-10 | MEDIUM | Permission resolution writes DB directly |
| HARDEN-SHIM-11 | LOW | Message ID bug (toInt64 on UUID) |
| HARDEN-SHIM-12 | LOW | Provider/agent hardcoded |
| HARDEN-SHIM-13 | LOW | /project and /vcs 404 instead of 501 |
| HARDEN-SHIM-14 | LOW | /doc serves Swagger UI not raw OpenAPI |
| HARDEN-SHIM-15 | LOW | Message parts only handle type: "text" |
| HARDEN-SHIM-16 | LOW | File endpoints correctly returning 501 |

## Phases

### Phase 1: Architectural refactor (HARDEN-SHIM-06)
Refactor shim to call native REST API instead of raw database

### Phase 2: Event stream (HARDEN-SHIM-01, HARDEN-SHIM-07)
Implement missing SSE event types and standalone /event endpoint

### Phase 3: Missing endpoints (HARDEN-SHIM-02, HARDEN-SHIM-03, HARDEN-SHIM-08, HARDEN-SHIM-09)
Add PATCH /session/:id, GET /session/:id/message/:messageID, PUT /auth/:id, PATCH /config

### Phase 4: Session map + API key (HARDEN-SHIM-04, HARDEN-SHIM-05)
Wire shim_session_map reads/writes; include api_key in create response

### Phase 5: Bug fixes & polish (HARDEN-SHIM-10 through HARDEN-SHIM-16)
Fix message ID, hardcoded providers, 404→501, /doc, message parts
