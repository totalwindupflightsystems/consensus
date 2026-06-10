# SPEC-017 UI Adapter Layer Gaps

**Date:** 2026-05-05
**Severity:** medium
**Source:** Idle spec conformance sweep-020 against `specs/017-ui-adapter-layer.md`
**Remediation:** `spec-017-hardening-01`
**Status:** queued

---

## Summary

Sweep-020 audited the opencode shim (`internal/shim/opencode/server.go`, 1253 lines) against SPEC-017 and found 16 gaps: 1 CRITICAL, 5 HIGH, 4 MEDIUM, 6 LOW.

## Critical Findings

**HARDEN-SHIM-01** — 4 of 6 SSE event types not emitted. The event stream only emits `session.updated`; missing: `message.created`, `tool.started`, `tool.completed`, `permission.requested`, `permission.resolved`. The opencode TUI will appear frozen during agent execution.

## High-Severity Findings

**HARDEN-SHIM-02** — `PATCH /session/:id` endpoint missing. No path for session updates (pause, resume, rename).

**HARDEN-SHIM-03** — `GET /session/:id/message/:messageID` missing. Cannot retrieve individual messages.

**HARDEN-SHIM-04** — `shim_session_map` table unused. Created in migration but never read/written by shim code. Reconnection flow broken.

**HARDEN-SHIM-05** — `api_key` missing from session create response. Required by SPEC-015 §3.1.

**HARDEN-SHIM-06** — Architectural violation: shim speaks directly to database, bypassing native REST API. SPEC-017 §3.2 requires every shim endpoint to call through to native API.

## Resolution

Queued as `spec-017-hardening-01`. Priority: address architectural violation first, then missing endpoints and event types.

axiom:trace work_item=idle-spec-conformance-sweep-01 spec=specs/017-ui-adapter-layer.md sweep=sweep-020 finding=shim-gaps
