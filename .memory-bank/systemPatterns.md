# System Patterns

## Architecture
- **Database-as-Runtime**: All state lives in SQL tables. No Redis, no message brokers, no filesystem state.
- **Two-Phase Execution**: Cognition transaction (fast) → async tool execution (no transaction). Never block a transaction on external I/O.
- **Append-Only Ledger**: memory_events is INSERT/SELECT only. Display state lives in separate display_modes table. Compression workers can update summary_text only.
- **Event Sourcing**: iteration_commits store active pointer arrays. Rollback = querying an older commit.
- **Progressive Disclosure**: Skill metadata is cheap (~100 tokens); full instructions are loaded on demand.

## Canonical State Machines
- **Session**: booting → idle → thinking ↔ tool_exec → waiting_sub → idle → completed/failed. Paused state for HITL.
- **Task**: pending → claimed → in_progress → reviewed → published. Cannot skip states.

## Key Patterns
- **RLS for security**: All tables scoped by `consensus.session_id` via `SET LOCAL`
- **Stored procedures for destructive ops**: complete_session(), set_display_mode(), soft_delete()
- **SQL injection mitigation**: Statement classifier (DML_READ/WRITE/DDL_CREATE/ALTER/DANGEROUS) + table whitelist + stored proc preference
- **Token caching**: System instructions (Layer 1) → Schema/Tools (Layer 2) → Active context (Layer 3)
- **Vector-validated compression**: Same embedding model for all tiers. LLM tier affects summarization, not embeddings.
- **Dynamic entity generator**: SECURITY DEFINER function for table creation. No raw DDL for agents.

## Data Flow
```
Harness reads active_context_view → formats Markdown → sends to LLM → LLM returns JSON
→ Harness parses JSON → Phase 1: cognition transaction (memory changes, system actions, pending tool requests)
→ Phase 2: async tool execution (each tool result in own mini-transaction)
→ Loop back to harness reads active_context_view
```