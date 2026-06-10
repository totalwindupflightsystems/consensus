# harness-lifecycle — Layer 3: Session lifecycle, error handling, budget, restart

## Goal
Complete all Layer 3 ACs (AC-016 through AC-019) — session transitions, error handling, budget enforcement, restart persistence.

## Affected ACs
- AC-016: Session transitions thinking → planning → idle (in_progress)
- AC-017: Planning error → session failed (pending)
- AC-018: Budget enforcement pauses runaway sessions (pending)
- AC-019: Server restart — DB state persists (pending)

## Specs
- specs/008-harness.md
- specs/006-transactions.md
- specs/011-canonical-definitions.md §1 (state machine)
- specs/020-multi-turn-planning.md §9.1

## Steps
1. AC-017: Add session status update in handlePlanningError — currently rolls back tx but doesn't set status='failed'
2. AC-018: Verify budget tracking in agent_billing table — check cost accumulation per iteration, add pause logic when budget_limit_cents exceeded
3. AC-019: Kill server, restart, verify sessions and memory_events survive (prove WAL mode works)
4. Run TestRealLLMIntegration to verify no regressions
