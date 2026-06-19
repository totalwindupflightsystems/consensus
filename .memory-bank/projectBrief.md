# Project Brief: Consensus

## Purpose
Consensus is a database-native cognitive architecture for AI agents where PostgreSQL (or SQLite) serves as the runtime, memory system, security sandbox, and event bus. The LLM doesn't run commands in a terminal — it writes SQL to manage its own mind.

## Goals
- Replace Bash/CLI-based agent execution with deterministic database-native operations
- Enable agents with unbypassable constraints enforced at the database kernel level
- Support two deployment modes: Supabase (PostgreSQL cloud) and local binary (embedded SQLite)
- Provide harness loop, REST API, MCP server, and CLI in a single Go binary
- Achieve append-only ledger with time-travel and full rollback capability

## Non-Goals
- Not a general-purpose agent framework (like LangChain)
- Not a replacement for human-in-the-loop approval systems
- Not a vector database (uses pgvector/sqlite-vec for embedding storage)
- No TypeScript/Deno/Edge Functions in the runtime

## Definitions
- **Done:** Spec written, implemented, end-to-end tested, with traceability evidence
- **Ready for review:** Implementation passes tier-3+ runtime verification, specs updated, tests pass
- **Blocked:** Missing dependency, unresolved cross-spec conflict, or unbypassable database constraint

## Key Integrations
- LLM APIs: OpenAI, Anthropic (Structured Outputs, prompt caching)
- Database: PostgreSQL (Supabase, Neon, RDS) or SQLite (embedded)
- Extensions: pgvector, pg_jsonschema, pg_cron, pg_net, sqlite-vec, sqlite-jsonschema

## Repo Conventions
- Specs: `specs/`
- Memory Bank: `.memory-bank/`
- Axiom config: `.axiom/`
- Agent config: `.opencode/`
