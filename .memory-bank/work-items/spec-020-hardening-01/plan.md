---
work_item_id: spec-020-hardening-01
status: queued
spec: specs/020-multi-turn-planning.md
source_sweep: sweep-021
created: 2026-05-05
---

# Plan — SPEC-020 Interactive Planning Hardening

Remediate gaps found during idle sweep-021. This is the highest-priority hardening item — it affects the core agent interaction model.

axiom:trace work_item=spec-020-hardening-01 spec=specs/020-multi-turn-planning.md sweep=sweep-021

## Findings

| ID | Severity | Description |
|----|----------|-------------|
| HARDEN-PLAN-01 | HIGH | No single long-running transaction (per-turn auto-commit) |
| HARDEN-PLAN-02 | HIGH | Staging buffer not persisted to database |
| HARDEN-PLAN-03 | HIGH | `respond` action missing |
| HARDEN-PLAN-04 | HIGH | No command type system (sql/file/memory/tool) |
| HARDEN-PLAN-05 | MEDIUM | Not wired into harness runtime |
| HARDEN-PLAN-06 | MEDIUM | Transaction timeout 10min vs 60s spec |
| HARDEN-PLAN-07 | MEDIUM | Crash recovery not implemented |
| HARDEN-PLAN-08 | MEDIUM | Memory state changes ignored at commit |
| HARDEN-PLAN-09 | MEDIUM | `end_iteration` concept missing |
| HARDEN-PLAN-10 | LOW | Buffer limits not enforced |
| HARDEN-PLAN-11 | LOW | `tool_exec → planning` state transition missing |

## Phases

### Phase 1: Single long-running transaction (HARDEN-PLAN-01, HARDEN-PLAN-02)
- Open one DB transaction per planning session that spans all turns
- Persist staging buffer entries to `staging_buffer` table
- Execute staged SQL within the session transaction, not per-turn mini-transactions

### Phase 2: Command type system (HARDEN-PLAN-04)
- Add `Type` field to StagingEntry: sql, file_write, file_edit, file_delete, memory_write, tool_call_ref
- Implement dispatcher routing by command type
- Wire file operations through tool_registry

### Phase 3: `respond` action (HARDEN-PLAN-03)
- Add `ActionRespond` constant and handler
- Roll back transaction and surface message to user

### Phase 4: Runtime integration (HARDEN-PLAN-05)
- Wire `RunInteractivePlanning` into heartbeat loop and Runner interface
- Add per-session mode toggle (one-shot vs interactive)

### Phase 5: Timeout & recovery (HARDEN-PLAN-06, HARDEN-PLAN-07)
- Set transaction timeout to 60s per spec
- Add stale session reaping (pg_cron for Postgres, goroutine ticker for SQLite)
- Add orphan staging buffer cleanup

### Phase 6: Polish (HARDEN-PLAN-08 through HARDEN-PLAN-11)
- Process memory_state_changes at commit
- Add end_iteration concept
- Enforce buffer limits
- Add tool_exec → planning state transition edge
