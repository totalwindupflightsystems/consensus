# WI-006: Three-Tier SQL Execution Model — Meta-Planning

**axiom:trace work_item=WI-006 spec=specs/003-database.md,specs/005-security.md,specs/008-harness.md plan=meta-planning.md**

## Gap Reference
CS-GAP-005 (HIGH) + CS-GAP-008 (HIGH): Three-tier SQL execution model missing; only Tier 3 (raw SQL with classifier) exists. No trigger-based constraint enforcement for state transitions, prerequisites, or rate limits.

## Spec Coverage
| Spec | Section | Relevance |
|------|---------|-----------|
| SPEC-003 §5.1 | State transition locks | `enforce_task_transitions()` trigger |
| SPEC-003 §5.2 | Prerequisite dependencies | `enforce_prerequisites()` trigger |
| SPEC-003 §5.5 | Rate limiting | `enforce_tool_rate_limit()` trigger |
| SPEC-005 | Security/RLS | Trust-derived execution tiers |
| SPEC-008 §5.4 | Three-tier SQL execution model | Tier 1 (stored-proc), Tier 2 (parameterized), Tier 3 (raw+classifier) |

## Scope

### In Scope
1. **Tier 1 executor**: `executeTier1()` — only allows `SELECT function_name(...)` calls. Whitelist of allowed stored functions.
2. **Tier 2 executor**: `executeTier2()` — parameterized SQL with `$1, $2` placeholders, type-safe parameter binding via pgx.
3. **Wire `EnforceExecutionPolicy()`**: route to Tier 1, 2, or 3 based on session trust level.
4. **DB triggers** (with Go fallback for SQLite):
   - `enforce_task_transitions()` — valid status transitions: pending→claimed→in_progress→reviewed→published
   - `enforce_prerequisites()` — task can't be claimed until all prerequisites are `published`
   - `enforce_tool_rate_limit()` — per-tool rate limiting from `tools_registry.rate_limit_per_min`
5. **`agent_trust_level`**: add to session config (low/medium/high → Tier 1/2/3)
6. **Test**: create task with prerequisites → attempt to claim before prerequisite done → must fail

### Out of Scope
- `pg_jsonschema` extension setup (CS-GAP-002, WI-003)
- RLS 4-role model runtime wiring (CS-GAP-010)
- `pg_cron` job setup (covered by Go goroutines)
- Full Postgres trigger deployment via migration (Go fallback is primary for SQLite parity)

## Architecture

```
Session Config (agent_trust_level)
        │
        ▼
EnforceExecutionPolicy(class, stmt, trustLevel)
        │
        ├── Tier 1 (low trust) ──► executeTier1(stmt, allowedFunctions)
        │     Only SELECT fn_name(...) allowed
        │
        ├── Tier 2 (medium trust) ──► executeTier2(stmt, params, bindings)
        │     Parameterized SQL with $1, $2 placeholders
        │
        └── Tier 3 (high trust) ──► executeTier3(stmt, whitelist)
              Raw SQL with classifier + table whitelist (existing)
```

Trigger enforcement (Go-level for SQLite parity):
```
UPDATE tasks SET status = 'claimed'
        │
        ▼
enforceTaskTransitions(oldStatus, newStatus, lockedByAgent)
        │
        ├── pending→in_progress without claimed? → BLOCK
        ├── skip reviewed→published? → BLOCK
        ├── published→reverted? → BLOCK
        └── claim already claimed task? → BLOCK

enforcePrerequisites(task)
        │
        ├── Any prerequisite not published? → BLOCK

enforceToolRateLimit(sessionID, toolName)
        │
        ├── Count requests in last minute ≥ rate_limit_per_min? → BLOCK
```

## Dependencies
- `internal/security/classifier.go` — Existing classification + policy enforcement
- `internal/security/executor.go` — **NEW**: Three-tier execution functions
- `internal/security/triggers.go` — **NEW**: Go-level trigger enforcement
- `internal/session/session.go` — Session types, trust level addition
- `internal/harness/executor.go` — Wire `EnforceExecutionPolicy()` with tier routing
- `internal/harness/context.go` — Read trust level from session context
- `internal/db/db.go` — DB interface for trigger queries
- `internal/harness/testdata/migration_test.sql` — Update if needed

## Risk Assessment
| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Tier 2 breaks existing Tier 3 flow | Low | High | Backward-compatible: Tier 3 sets trust_level=high, no change |
| Trigger enforcement breaks existing task workflow | Medium | Medium | Fails closed with clear error; rollback on constraint violation |
| Go-level triggers don't match Postgres trigger behavior | Low | Medium | Same validation logic; dual path tested |

## Decision Points
| Decision | Options | Chosen | Rationale |
|----------|---------|--------|-----------|
| Trust level source | a) sessions table, b) model_registry tier | **sessions table** | Direct per-session config; model tier is a hint |
| Trigger enforcement | a) DB triggers only, b) Go fallback only, c) Both | **Go fallback primary** | SQLite parity; DB triggers as defense-in-depth |
| Allowed functions for Tier 1 | a) Fixed list in security package, b) tools_registry query | **Fixed list + tools_registry SQL functions** | Minimal bootstrap; tools_registry for extensibility |

## Assumptions

| # | Statement | How to Verify | Impact if Wrong |
|---|-----------|---------------|-----------------|
| A1 | Existing tests use `agent_trust_level` implicitly as "high" | All existing tests pass unchanged | Need to update test sessions |
| A2 | SQLite triggers are not supported; Go fallback is sufficient | Go-level validation matches SQL spec | May need to add SQLite trigger migration |
| A3 | Tier 2 parameter binding follows the same `$1, $2` convention for both SQLite and Postgres | pgx and modernc.org/sqlite both support `$N` | May need adapter for `?` placeholders |

## Open Questions
None — well-specified with clear implementation paths.
