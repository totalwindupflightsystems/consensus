# cli-flows — Layer 9: CLI commands + developer onboarding (SPEC-016, SPEC-019)

## Goal
Verify CLI commands (init, serve, status, session, approve, memory, tool) and end-to-end user flows (AC-048 through AC-055).

## Affected ACs
- AC-048: `conscience init` — bootstrap creates DB, key, config
- AC-049: `conscience serve` — starts all subsystems
- AC-050: `conscience status` — shows active state
- AC-051: `conscience session` — create, list, show, cost
- AC-052: `conscience approve` — list, show, approve, reject
- AC-053: Developer onboarding — init + serve + session in <5min
- AC-054: Multi-session memory — Day 1 context on Day 2
- AC-055: Error recovery UX — operator sees error context

## Specs
- specs/016-cli-interface.md
- specs/019-user-interaction-flows.md

## Steps
1. Test `conscience init` → verify DB created, key printed, config written
2. Test `conscience serve` → verify all subsystems start (heartbeat, compression, HITL, events, opencode shim)
3. Test `conscience status` → verify active sessions, pending approvals, schema version displayed
4. Test `conscience session create/list/show/cost` → verify full CRUD
5. Test `conscience approve list/show/approve/reject` → verify HITL flow
6. Timebox onboarding: init + serve + session create → assert < 300s
7. Test multi-session memory: Session A analysis persists to Session B
8. Test error recovery: trigger errors → verify operator sees context via `approve show`
