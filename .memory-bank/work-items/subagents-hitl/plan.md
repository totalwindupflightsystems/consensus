# subagents-hitl — Layer 7: Sub-agent spawning, isolation, approvals, circuit breakers

## Goal
Implement sub-agent system (SPEC-004) and HITL approval flow (SPEC-014) — AC-035 through AC-042.

## Affected ACs
- AC-035: Sub-agent spawn — parent creates task, child starts
- AC-036: Memory fork isolation — child inherits compressed pointers only
- AC-037: Parent wake on sub-agent completion
- AC-038: Approval request creation — session pauses
- AC-039: Approval review — approve/reject/modify resumes session
- AC-040: Circuit breaker — 3 consecutive errors → paused
- AC-041: Tool-required approval — requires_approval=true triggers HITL
- AC-042: Multi-session isolation — session-scoped key enforcement

## Specs
- specs/004-subagents.md
- specs/014-hitl-interrupt-state.md
- specs/005-security.md
- specs/015-api-and-mcp.md §2

## Steps
1. Verify tasks table + sessions.parent_id exist (migrations 001, 009)
2. Test: parent spawns task → verify child session created → assert child executes
3. Test: parent has 10 events, 3 compressed → spawn child → assert 3 inherited_pointer events
4. Verify wake_parent_on_completion trigger or Go equivalent exists
5. Test: agent calls request_approval() → assert approval_requests row + session=paused
6. Test: review_approval('approved') → assert session=idle + context includes outcome
7. Test: 3 consecutive errors → assert session=paused + approval created
8. Test: session A's key used to query session B's memory → assert 403
