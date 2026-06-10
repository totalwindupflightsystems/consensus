# fix-llm-prompt — Fix LLM prompt: memory_events column names

## Goal
The LLM generates INSERT statements with wrong column names (`event_type`, `agent_type`) instead of the actual schema column `type`. The system prompt needs the real memory_events schema.

## Affected ACs
- AC-015 (integration test — produces warnings on every run)
- AC-016 (session lifecycle)
- AC-020 (append-only ledger)

## Specs
- specs/002-memory.md §2.2 (memory_events schema)
- specs/008-harness.md (harness core loop)
- specs/011-canonical-definitions.md §3.5 (canonical type CHECK)

## Steps
1. Read internal/harness/planning.go — find where the system prompt is assembled
2. Locate the JSON schema or prompt template that tells the LLM what columns to use
3. Replace hardcoded/guessed column names with actual memory_events schema:
   - type (CHECK: header, text_block, tool_call, tool_result, thinking, system, inherited_pointer, user_message)
   - content (TEXT NOT NULL)
   - session_id (TEXT NOT NULL)
   - iteration_created (BIGINT NOT NULL)
   - summary_text (TEXT, nullable)
   - embedding (BLOB, nullable — skip in prompt)
   - created_at (TEXT, auto — skip in prompt)
4. Verify: run TestRealLLMIntegration, assert no "has no column named" warnings
5. Verify: run TestRealLLMIntegration, assert memory_events > 1 (LLM successfully wrote an event)
