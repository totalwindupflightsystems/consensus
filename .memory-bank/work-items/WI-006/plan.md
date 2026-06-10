# WI-006: Three-Tier SQL Execution Model — Execution Plan

> **axiom:trace work_item=WI-006 spec=specs/003-database.md,specs/005-security.md,specs/008-harness.md plan=plan.md**

## Summary

Implement three SQL execution tiers based on session trust level, plus DB-level trigger enforcement for task transitions, prerequisites, and rate limits. Currently only Tier 3 (raw SQL with classifier) exists.

## Steps

### Step 1: Core Security Layer
**Files**: `internal/security/executor.go` (NEW), `internal/security/executor_test.go` (NEW)

- [x] Create directory structure
- [ ] Implement `ExecutionTier` type and routing
- [ ] Implement `executeTier1()` — stored-procedure-only validation
- [ ] Implement `executeTier2()` — parameterized SQL validation
- [ ] Update `EnforceExecutionPolicy()` to route by trust level
- [ ] Write unit tests

### Step 2: Trigger Enforcement
**Files**: `internal/security/triggers.go` (NEW), `internal/security/triggers_test.go` (NEW)

- [ ] Implement `enforceTaskTransitions()` — state machine validation
- [ ] Implement `enforcePrerequisites()` — dependency chain validation
- [ ] Implement `enforceToolRateLimit()` — per-tool rate limiting
- [ ] Write unit tests

### Step 3: Session Trust Level
**Files**: `internal/session/session.go` (modify)

- [ ] Add `TrustLevel` type and constants
- [ ] Add `TrustLevel` field to `Session` struct
- [ ] Update test harness

### Step 4: Harness Wiring
**Files**: `internal/harness/executor.go` (modify), `internal/harness/context.go` (modify)

- [ ] Wire trust level from session into `executeStatement()`
- [ ] Route through `executeTier1`/`executeTier2`/`executeTier3`
- [ ] Verify all existing tests pass

### Step 5: Prerequisite Enforcement Test
**Files**: `internal/security/triggers_test.go` (modify)

- [ ] Create prerequisite enforcement integration test
- [ ] Verify claim-before-published is blocked

### Step 6: Commit
- [ ] `git pull --rebase`
- [ ] `git add -A`
- [ ] `git commit -m "feat: implement three-tier SQL execution model (WI-006)"`

## Rollback
- Revert all changes: `git checkout -- internal/security/executor.go internal/security/triggers.go internal/session/session.go internal/harness/executor.go`
- Or: `git reset HEAD~1`

## Verification
```bash
go build ./...
go test ./...
go vet ./...
```
