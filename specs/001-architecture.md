# SPEC-001: System Architecture & Core Philosophy

**Status:** Draft  
**Source:** Gemini Chat Turns 1-38  
**Last Updated:** 2026-04-08

---

## 1. Core Thesis

The LLM is a massive, multi-vector data beast. It is inherently non-deterministic. Attempting to manage its state, memory, and permissions inside a file system or a raw terminal is a structural failure. A non-deterministic engine must be paired with the most deterministic system we have in computing: a relational database.

**Conscience** is an architecture where the database (PostgreSQL or SQLite) is the runtime environment, the memory ledger, the security sandbox, and the cognitive loop.

### Evolution of Agent Runtimes

| Generation | Runtime | Limitation |
|---|---|---|
| 1st | Browser chatbots | No persistence, no tools |
| 2nd | Code editor sidebars | Read-only, autocomplete-only |
| 3rd | CLI/TUI agents (Bash) | Non-deterministic, insecure, no state |
| **4th** | **Database-native (Conscience)** | **Deterministic state machine with memory** |

## 2. Design Principles

### 2.1 Database-as-Runtime (DBaaR)

The database is the entire application. No external state machines, no Redis caches, no message brokers. The database IS the state machine.

- **Memory** = SQL tables with time-travel
- **Event bus** = Postgres triggers / PocketBase hooks
- **Security sandbox** = Row-Level Security (RLS) / API Rules
- **Execution state** = Session rows with status columns
- **Tool registry** = Database tables, not filesystem directories

### 2.2 Atomic Cognition

Every agent thought or action executes within a strict SQL transaction. If any part fails, the entire transaction rolls back. The agent's cognitive state remains pristine—no half-written files, no corrupted states.

```sql
BEGIN;
  UPDATE active_context SET display_mode = 'compressed' WHERE event_id = 104;
  INSERT INTO tool_requests (action, target) VALUES ('scrape', '...');
  -- Command 3 has a hallucinated column → ERROR
ROLLBACK; -- Everything reverts cleanly
```

### 2.3 Write Once, Deploy Anywhere

One architecture, two database backends, one Go binary:

| Backend | Database | Best For |
|---|---|---|
| **Postgres** (Supabase Cloud, self-hosted, any provider) | PostgreSQL | Enterprise, multi-tenant, horizontal scaling |
| **SQLite** (embedded) | SQLite (in-process) | Single developer, local-first, air-gapped |

The LLM doesn't know which database it's running on. It sees the same schema, reads the same views, outputs the same JSON. The Go binary connects to either backend via `--db` flag.

### 2.4 Agent as Microservice, Not User

Bash agents emulate a human at a keyboard. Conscience agents operate as backend microservices—managing state, executing precise CRUD operations, and managing data pipelines within a constrained, reliable surface area.

## 3. System Components Overview

```
┌─────────────────────────────────────────────────────────┐
│                    CONSCIENCE ARCHITECTURE               │
├─────────────────────────────────────────────────────────┤
│                                                          │
│              ┌──────────────────────────┐                │
│              │    Go Binary (conscience) │                │
│              │                          │                │
│              │  ┌─────────┐  ┌───────┐  │                │
│  ┌────────┐  │  │ Harness │  │ REST  │  │                │
│  │ LLM    │◄─┤  │ Loop    │  │ API   │  │                │
│  │ API    │  │  └─────────┘  └───────┘  │                │
│  └────────┘  │  ┌─────────┐  ┌───────┐  │                │
│              │  │ MCP     │  │ CLI   │  │                │
│              │  │ Server  │  │       │  │                │
│              │  └─────────┘  └───────┘  │                │
│              │         DB Driver I/F    │                │
│              └────────────┬─────────────┘                │
│                           │                              │
│              ┌────────────┴─────────────┐                │
│              │                          │                │
│  ┌───────────┴──────────┐  ┌────────────┴──────────┐    │
│  │     PostgreSQL       │  │       SQLite          │    │
│  │  (any provider)      │  │    (embedded)         │    │
│  │                      │  │                       │    │
│  │  ┌─────────────┐     │  │  ┌─────────────┐     │    │
│  │  │ memory_     │     │  │  │ memory_     │     │    │
│  │  │ events      │     │  │  │ events      │     │    │
│  │  └─────────────┘     │  │  └─────────────┘     │    │
│  │  ┌─────────────┐     │  │  ┌─────────────┐     │    │
│  │  │ active_     │     │  │  │ active_     │     │    │
│  │  │ context_view│     │  │  │ context_view│     │    │
│  │  └─────────────┘     │  │  └─────────────┘     │    │
│  │  ┌─────────────┐     │  │  ┌─────────────┐     │    │
│  │  │ sessions    │     │  │  │ sessions    │     │    │
│  │  │ + RLS       │     │  │  │ + Go hooks  │     │    │
│  │  └─────────────┘     │  │  └─────────────┘     │    │
│  └──────────────────────┘  └───────────────────────┘    │
│                                                          │
│  ┌──────────────────────────────────────────────────┐   │
│  │         EXTERNAL TOOL RUNNER (sandboxed)          │   │
│  │  ┌──────┐  ┌──────┐  ┌──────┐  ┌──────┐        │   │
│  │  │scrape│  │ API  │  │exec  │  │test  │        │   │
│  │  └──────┘  └──────┘  └──────┘  └──────┘        │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

## 4. Core Data Flow (The Loop)

```
1. Harness reads active_context_view for current session
2. Harness formats Markdown from view rows
3. Harness sends Markdown + system prompt to LLM
4. LLM returns JSON payload
5. Harness parses JSON, begins SQL transaction (BEGIN)
6. Harness executes SQL commands from payload
7. If error → ROLLBACK, inject error into next context
8. If success → COMMIT, save iteration snapshot
9. pg_cron / triggers wake sub-agents or parent
10. Loop back to step 1
```

## 5. The Two Hemispheres of Execution

### Internal Hemisphere (SQL/State Tools)
- Deterministic stored procedures
- Memory manipulation, view updates, context paging
- Runs instantly within DB kernel, zero network latency
- Full transactional safety

### External Hemisphere (Go/Network Tools)
- Network-bound operations (scraping, API calls)
- Agent-written code stored in DB, executed in sandbox
- Isolated subprocess, can't crash the database
- Results written back to `tool_results` table

## 6. Key Differentiators from Existing Frameworks

| Problem | LangChain/AutoGen | Conscience |
|---|---|---|
| State management | External memory stores | Append-only ledger with time travel |
| Context window | Monolithic prompt strings | Dynamic SQL VIEW with paging |
| Security | Python sandbox (escapable) | RLS at DB kernel level |
| Error recovery | Manual state repair | Atomic transaction rollback |
| Memory cost | Full re-send every loop | Pointer-based, compressed |
| Tool execution | Sequential, blocking | Async, event-driven |
| Sub-agent scaling | Linear context growth | Forked memory via SQL INSERT |
| Hallucination damage | Corrupted files | Rejected transaction |
| Self-improvement | Prompt engineering | ALTER TABLE + JSON Schema constraints |

## 7. Why "Conscience"

The framework is called **Conscience** because:

1. The agent doesn't just "remember" rules—it **physically cannot break them** because they're enforced by database constraints
2. When a user says "modify the system so we can never have that data missing again," the agent doesn't just update its prompt—it **alters the structural boundaries of its own mind** (`ALTER TABLE ADD CONSTRAINT`)
3. The "conscious" layer is what the agent sees (the dynamic view); the "subconscious" layer is the immutable full ledger it can't see but can search (Alt-Mode)
4. The database kernel acts as the agent's conscience—rejecting harmful actions before they manifest

## 8. References

- Turns 1-3: Core paradigm shift (Bash/TS → SQL/DB)
- Turns 4-5: Virtual memory, context paging, sub-agent memory
- Turns 12-14: JSON vs SQL debate, transaction safety, circuit breakers
- Turns 19-20: Unified deployment, write-once philosophy
- Turns 26-28: Master architecture document formulation
- Turns 35-36: "Conscience" naming rationale, unbypassable constraints