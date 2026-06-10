# planning-interactive — Layer 5: Interactive transaction staging (SPEC-020)

## Goal
Implement interactive transaction model: stage, execute, inspect, commit, rollback within a single iteration (AC-026 through AC-029).

## Affected ACs
- AC-026: stage_and_execute in same iteration
- AC-027: Transaction rollback and retry
- AC-028: Staging buffer visibility per turn
- AC-029: Max turns auto-commit

## Specs
- specs/020-multi-turn-planning.md (full spec)
- specs/006-transactions.md

## Steps
1. Verify staging_buffer table exists (migration 004)
2. Test: create session, stage SELECT → verify result in next turn context → stage UPDATE based on result → commit
3. Test: stage INSERT with error → rollback → stage corrected INSERT → commit → verify only corrected data
4. Test: 3-turn session → verify turn 3 context shows turn 1+2 results with ✓/✗
5. Test: 10-turn budget → verify auto-commit at turn 10
