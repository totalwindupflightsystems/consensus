# SPEC-020 Multi-Turn Planning Gaps

**Date:** 2026-05-05
**Severity:** high
**Source:** Idle spec conformance sweep-021 against `specs/020-multi-turn-planning.md`
**Remediation:** `spec-020-hardening-01`
**Status:** queued

---

## Summary

Sweep-021 audited the interactive planning implementation (`internal/harness/planning.go`, 587 lines) against SPEC-020 (702 lines) and found 4 HIGH, 5 MEDIUM, 2 LOW gaps. The core architectural issues are: no single long-running transaction, no persistent staging buffer, and the `respond` action is missing.

## Critical Findings

**HARDEN-PLAN-01** — Per-turn mini-transactions auto-commit, defeating the spec's "engineer in an open transaction window" metaphor. Cannot roll back earlier turns.

**HARDEN-PLAN-02** — Staging buffer is entirely in-memory, lost on process restart. No crash recovery possible.

**HARDEN-PLAN-03** — `respond` action missing. Agent cannot reply to user without committing or rolling back.

**HARDEN-PLAN-04** — Only raw SQL staging, no command type system (file_write, memory_write, tool_call_ref).

## Resolution

Queued as `spec-020-hardening-01`. High priority — this is the core interaction model for the agent.

axiom:trace work_item=idle-spec-conformance-sweep-01 spec=specs/020-multi-turn-planning.md sweep=sweep-021 finding=planning-gaps
