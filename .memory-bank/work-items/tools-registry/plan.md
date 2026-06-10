# tools-registry — Layer 6: JIT registry, RLS ownership, skills, event plugins (SPEC-010)

## Goal
Implement tool system: JIT agent-authored tools, RLS ownership enforcement, skills registry with progressive disclosure, event-driven plugins (AC-030 through AC-034).

## Affected ACs
- AC-030: JIT tool registration — agent INSERTs, other discovers
- AC-031: Tool ownership RLS — agent A cannot modify agent B's tool
- AC-032: Skills registry — progressive disclosure (metadata vs full instructions)
- AC-033: Event-driven plugin — INSERT fires trigger → task_queue
- AC-034: Tool execution — agent requests tool, result feeds back

## Specs
- specs/010-tools.md (full spec)
- specs/003-database.md §2.12-2.14
- specs/011-canonical-definitions.md §4-5

## Steps
1. Verify custom_agent_tools table exists (migration 011)
2. Test: Agent A INSERTs tool → Agent B SELECTs tools_registry → asserts visible
3. Test: Agent B creates tool → Agent A attempts UPDATE → assert permission denied
4. Verify skills_registry table exists → test INSERT skill, query metadata, load_skill()
5. Test: INSERT into domain table → verify task_queue trigger fires
6. Test: Agent requests tool execution → verify tool_requests row + result in context
