# Project Brief: Conscience

## What
Conscience is a database-native cognitive architecture for AI agents where PostgreSQL (or SQLite via PocketBase) serves as the runtime, memory system, security sandbox, and event bus. The LLM doesn't run commands in a terminal — it writes SQL to manage its own mind.

## Why
Current Bash/CLI-based AI agents are non-deterministic, insecure, and lack durable state management. By pairing an LLM with the most deterministic system in computing (a relational database), Conscience enforces correct behavior through database constraints, not prompt engineering.

## Core Principles
1. **Database-as-Runtime** — The database IS the application
2. **Atomic Cognition** — Every thought is a SQL transaction
3. **Write Once, Deploy Anywhere** — Same architecture on Supabase or PocketBase
4. **Agent as Microservice** — Agents manage state, not emulate humans
5. **Unbypassable Constraints** — ALTER TABLE ADD CONSTRAINT, not prompt updates

## Tech Stack
- **Database**: PostgreSQL (Supabase) or SQLite (PocketBase)
- **Harness**: TypeScript (Deno/Edge Functions) or Go (PocketBase)
- **LLM Integration**: OpenAI, Anthropic (Structured Outputs)
- **Extensions**: pgvector, pg_jsonschema, pg_cron, pg_net, sqlite-vec, sqlite-jsonschema

## Spec Suite
21 specs in `/specs/` directory (000-020), covering architecture, memory, database schema, subagents, security, transactions, JSON format, harness, deployment, tools, canonical definitions, system prompts, webhooks, HITL, API/MCP, CLI interface, TUI protocol shims, OpenAPI contract, user interaction flows, and multi-turn planning.

## Status
Design complete. All specs written and cross-reconciled via SPEC-011. Specs 016-019 added for user-facing interaction: CLI management, protocol shims for existing AI tools, OpenAPI contract, and user flows. Ready for implementation prototyping.