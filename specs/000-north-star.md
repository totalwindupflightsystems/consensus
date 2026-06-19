# 000 - North Star: Consensus

**Project:** Consensus  
**Status:** Design Complete  
**Created:** 2026-04-08  
**Source:** Gemini conversation (38 turns)

---

## What We Are Building

**Consensus** is a database-native cognitive architecture for AI agents.

The LLM is a massive, non-deterministic data beast. It must be paired with the most deterministic system in computing: a relational database. Consensus replaces the Bash/file-system execution layer that current AI agents use with PostgreSQL (or SQLite via PocketBase) as the runtime, memory system, security sandbox, and event bus—all in one.

The agent doesn't run commands in a terminal. It writes SQL to manage its own mind.

---

## Why It Exists

| Problem | Bash/CLI Agents | Consensus |
|---|---|---|
| State management | External stores, lost between sessions | Append-only ledger with time-travel rollback |
| Context window | Monolithic prompt strings that bloat | Dynamic SQL VIEW with pointer-based paging |
| Security | Python sandbox (escapable) | RLS at database kernel level |
| Error recovery | Manual state repair | Atomic transaction rollback |
| Memory cost | Full re-send every loop | Compressed pointers, vector-validated summaries |
| Hallucination damage | Corrupted files, deleted data | Rejected transaction, pristine state preserved |
| Self-improvement | Prompt engineering (fragile) | ALTER TABLE + JSON Schema constraints (unbreakable) |

---

## Core Principles

1. **Database-as-Runtime** — The database IS the application. No external state machines, no Redis, no message brokers.

2. **Atomic Cognition** — Every thought is a SQL transaction. It either fully succeeds or fully rolls back. No half-states.

3. **Write Once, Deploy Anywhere** — One Go binary, two database backends (Postgres or SQLite). The LLM doesn't know which it's running on.

4. **Agent as Microservice** — Agents manage state, execute CRUD, and control data pipelines. They don't emulate humans at keyboards.

5. **Unbypassable Constraints** — When the agent learns a hard lesson, it doesn't update its prompt. It alters the structural boundaries of its own mind (`ALTER TABLE ADD CONSTRAINT`). The database kernel enforces the rule forever.

---

## The Name

The framework is called **Consensus** because:

- The agent doesn't just "remember" rules — it **physically cannot break them** (database constraints)
- When told "never let this happen again," it **alters its own schema** — not its prompt
- The "conscious" layer is what the agent sees (dynamic view); the "subconscious" is the immutable ledger it can search but not see (Alt-Mode)
- The database kernel is the consensus — rejecting harmful actions before they manifest

---

## Architecture at a Glance

```
┌──────────────┐     ┌──────────────┐
│  LLM API     │◄────┤   Harness    │
│ (multi-model)│     │ (TypeScript) │
└──────────────┘     └──────┬───────┘
                            │
                     JSON ◄──► SQL
                            │
              ┌─────────────▼─────────────┐
              │    DATABASE (Postgres/     │
              │           SQLite)          │
              │                            │
              │  memory_events ─────────── │ ← Append-only ledger
              │  iteration_commits ─────── │ ← Time-travel snapshots
              │  active_context_view ───── │ ← Dynamic context window
              │  sessions ─────────────── │ ← Agent process control
              │  tasks ────────────────── │ ← Async event queue
              │  tool_requests ────────── │ ← External tool dispatch
              │  skills_registry ──────── │ ← Progressive disclosure
              │  agent_billing ────────── │ ← Cost circuit breakers
              └────────────────────────────┘
                            │
              ┌─────────────▼─────────────┐
               │  EXTERNAL TOOL RUNNER     │
               │  (Sandboxed subprocess)   │
               │  Sandbox-isolated         │
              └───────────────────────────┘
```

---

## Specs Index

| # | Spec | Description |
|---|---|---|
| 001 | Architecture | Core philosophy, design principles, data flow |
| 002 | Memory | Cognitive engine, context views, paging, compression |
| 003 | Database | Full schema design, all CREATE TABLE statements |
| 004 | Subagents | Spawning, forking, process control, RBAC |
| 005 | Security | RLS, Alt-Mode, cognitive firewall, secrets |
| 006 | Transactions | Atomic cognition, rollback, circuit breakers |
| 007 | JSON Schema | LLM output format, harness translation |
| 008 | Harness | TypeScript/Go execution loop, heartbeat |
| 009 | Deployment | Supabase & PocketBase paths, install experience |
| 010 | Tools | Internal/external hemispheres, JIT registry, CI/CD |
| 011 | Canonical Definitions | Cross-spec reconciliation: state machines, unified schemas, display_mode fix, tool execution, SQL injection mitigation |
| 012 | System Prompt & Discovery | LLM prompt assembly, schema discovery, progressive disclosure, caching strategy |
| 013 | Webhooks & Events | External event ingestion, webhook registration, event routing, triggers |
| 014 | HITL Interrupt State | Human-in-the-loop: approval requests, pause/resume, circuit breaker integration |
| 015 | API & MCP | REST API, real-time streams, MCP server, authentication, rate limiting |

---

## Next Steps

1. Draft baseline SQL schema (consolidating all CREATE TABLE statements)
2. Build harness TypeScript prototype
3. First end-to-end test: agent reads context view, outputs JSON, harness executes SQL
4. Resolve remaining open questions across all specs

---

## Origin Conversation

This project was designed in a conversation on April 8, 2026 between the founder and Gemini (Google AI). The full conversation is preserved at `gemini_chat.md` in the project root. The conversation covered 38 turns of architectural design, Devil's Advocate analysis, and implementation planning.