---
work_item_id: idle-spec-conformance-sweep-01
status: active
repo: wojons/conscientiousness
created: 2026-05-05
sweep_iteration: sweep-017 (last complete)
updated: 2026-05-05 (verifier captain correction)
---

# Plan — Idle-Time Spec Conformance Sweep

Recurring sweep work item. When all explicit TODO items are complete or credential-gated, audit one spec per iteration against the live codebase and runtime. Record conformance or create remediation work items.

axiom:trace work_item=idle-spec-conformance-sweep-01 spec=<rotating> plan=sweep-001 evidence=.memory-bank/work-items/idle-spec-conformance-sweep-01/verification.md

## Swept Specs (Rotation Log)

| Sweep | Spec | Date | Outcome |
|---|---|---|---|
| sweep-001 | `specs/019-user-interaction-flows.md` | 2026-05-05 | CONFORMANT |
| sweep-002 | `specs/018-openapi-contract.md` | 2026-05-05 | CONFORMANT (4 tooling items deferred) |
| sweep-003 | `specs/006-transactions.md` | 2026-05-05 | GAPS FOUND → remediated via `spec-006-hardening-01` |
| sweep-004 | `specs/000-north-star.md` | 2026-05-05 | CONFORMANT |
| sweep-005 | `specs/001-architecture.md` | 2026-05-05 | CONFORMANT |
| sweep-006 | `specs/002-memory.md` | 2026-05-05 | GAPS FOUND → remediated via `spec-002-hardening-01` |
| sweep-007 | `specs/003-database.md` | 2026-05-05 | GAPS FOUND → `spec-003-hardening-01` (queued) |
| sweep-008 | `specs/004-subagents.md` | 2026-05-06 | CONFORMANT (4 minor gaps deferred) |
| sweep-009 | `specs/005-security.md` | 2026-05-05 | CONFORMANT (4 minor gaps deferred) |
| sweep-010 | `specs/007-json-schema.md` | 2026-05-06 | CONFORMANT (1 medium + 3 low deferred) |
| sweep-011 | `specs/008-harness.md` | 2026-05-06 | CONFORMANT (5 findings, all deferred) |
| sweep-012 | `specs/009-deployment.md` | 2026-05-06 | GAPS FOUND → remediated via `spec-009-hardening-01` |
| sweep-013 | `specs/010-tools.md` | 2026-05-05 | CONFORMANT (3 low gaps Postgres-only) |
| sweep-014 | `specs/011-canonical-definitions.md` | 2026-05-05 | CONFORMANT (1 low gap) |
| sweep-015 | `specs/012-system-prompt-and-discovery.md` | 2026-05-05 | CONFORMANT (3 deferred) |
| sweep-016 | `specs/013-webhooks-and-events.md` | 2026-05-06 | GAPS FOUND → `spec-013-hardening-01` (REMEDIATED) |
| sweep-017 | `specs/014-hitl-interrupt-state.md` | 2026-05-05 | GAPS FOUND → `spec-014-hardening-01` (queued) |

## Verified Conformant (10 specs)

- specs/019 — User interaction flows (12/12 ACs)
- specs/018 — OpenAPI contract (5/9 implemented, 4 tooling deferred)
- specs/000 — North star (all principles mapped)
- specs/001 — Architecture (design principles verified)
- specs/004 — Subagents (5/5 ACs, 4 minor gaps)
- specs/005 — Security (15/15 checks, 4 minor gaps)
- specs/007 — JSON Schema (14/14 checks)
- specs/008 — Harness (57/62 implemented, 5 deferred)
- specs/010 — Tools (28/28 core checks)
- specs/011 — Canonical Definitions (13/13 aligned)
- specs/012 — System Prompt (21/24 checks)

## Gaps Found → Hardening Created (4 specs)

| Spec | Hardening Item | Status |
|---|---|---|
| specs/006-transactions | `spec-006-hardening-01` | ✅ COMPLETE (7/7 ACs) |
| specs/002-memory | `spec-002-hardening-01` | ✅ COMPLETE (5/5 ACs) |
| specs/009-deployment | `spec-009-hardening-01` | ✅ COMPLETE (2/2 ACs) |
| specs/003-database | `spec-003-hardening-01` | ✅ COMPLETE |
| specs/013-webhooks | `spec-013-hardening-01` | ✅ COMPLETE |
| specs/014-hitl | `spec-014-hardening-01` | ✅ COMPLETE |
| specs/015-api | `spec-015-api-and-mcp.md` | ✅ CONFORMANT (sweep-018) |
| specs/016-cli | `spec-016-hardening-01` | ✅ COMPLETE |
| specs/017-ui | `spec-017-hardening-01` | ✅ COMPLETE |
| specs/020-planning | `spec-020-hardening-01` | ✅ COMPLETE |
| specs/021-layout | N/A | ✅ DEFERRED (sweep-022) |
| specs/022-library | N/A | ✅ DEFERRED (sweep-023) |

## Status: ALL 22 SPECS SWEPT. ALL 10 HARDENING ITEMS COMPLETE.

(Updated 2026-05-07 — plan.md was stale; _current.md had the canonical completion status.)

## Pointers

- Verification evidence: `.memory-bank/work-items/idle-spec-conformance-sweep-01/verification.md`
- TODO: `.memory-bank/TODO.md`
- Current state: `.memory-bank/work-items/_current.md`
