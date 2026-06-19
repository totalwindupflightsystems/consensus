# PRD — Consensus: Database-Native Cognitive Architecture

## Problem
- **AI agents today run on Bash/file-system execution layers** — they manage state in files, permissions in sandboxes, and memory in monolithic prompt strings. The non-deterministic nature of LLMs combined with non-deterministic execution environments produces corrupted state, lost context, and fragile error recovery.
- **Who has the problem?** AI developers building autonomous agents that need reliable state management, memory persistence, security isolation, and self-improvement capabilities. Anyone who has watched an agent hallucinate a command that deleted files, or lose context because the prompt window overflowed.
- **The pain:** State lost between sessions. Context windows that bloat and degrade. Security sandboxes that can be escaped. Manual state repair after errors. Full context re-sent every loop burning tokens. Prompt engineering as the only "improvement" mechanism.

## Goals
- **Database-as-Runtime**: Replace the Bash execution layer with PostgreSQL/SQLite as the agent's runtime environment. The database IS the application.
- **Atomic Cognition**: Every agent thought executes as a SQL transaction — either fully succeeds or fully rolls back. No half-states, no corrupted files.
- **Append-Only Memory Ledger**: Event sourcing with time-travel rollback — the agent's complete cognitive history is immutable and queryable.
- **Dynamic Context Windows**: Replace monolithic prompt strings with SQL VIEWs that provide pointer-based paging and vector-validated compression.
- **Kernel-Level Security**: Row-Level Security (RLS) at the database kernel level — even hallucinated SQL cannot cross session boundaries.
- **Unbypassable Constraints**: When the agent learns a hard lesson, it doesn't update its prompt — it `ALTER TABLE ADD CONSTRAINT`. The database kernel enforces the rule forever.
- **Write Once, Deploy Anywhere**: Single Go binary, two database backends (Postgres for enterprise, SQLite for local-first). The LLM never knows which it's running on.
- **Agent as Microservice**: Agents manage state, execute CRUD, and control data pipelines — they don't emulate humans at keyboards.

## Non-Goals
- Human-in-the-loop chat interface (v2)
- Multi-modal input (images, audio) (v2)
- Federation across multiple runtime instances (v2)
- Direct browser automation (delegated to external tool sandbox)

## Users and Flows
- **AI Developer**: Configures LLM provider, starts agent sessions, monitors harness loop, reviews HITL approval requests
- **Agent (the LLM)**: Reads active_context_view, outputs JSON payloads, executes SQL inside transactions, manages its own memory via the database
- **Operator**: Manages deployment (Supabase cloud or PocketBase local), monitors billing/cost circuit breakers, reviews Alt-Mode audit logs

## Success Metrics
- Agent completes a full harness loop: reads context → outputs JSON → harness executes SQL → commits transaction → loops
- Zero state corruption across 100+ consecutive iterations
- Database constraints reject hallucinated SQL without crashing the agent
- Context window stays within budget via SQL VIEW paging (not monolithic prompt strings)
- Single `go build` produces working binary that connects to either Postgres or SQLite
- External tool sandbox executes agent-requested commands with 30s timeout, 1MB output cap, semaphore-limited concurrency

## Open Questions
- PocketBase API Rules vs full Postgres RLS parity — how close can we get?
- Optimal compression tier thresholds (cosine similarity for tier-1 → tier-2 transitions)?
- Should the harness loop be pull-based (polling) or push-based (pg_notify/triggers)?
- Multi-tenant isolation model for cloud deployment?
