# Consensus

![CONSENSUS](docs/social-preview.png)

**Stop building agents on sand. Your database is the runtime.**

---

## Your Agent Framework Is Lying To You

88% of agentic AI projects never reach production. Not because of bad prompts.
Not because of model limitations. Because of **state**.

The orchestration frameworks — LangChain, CrewAI, AutoGPT, Bee Agent — share
the same architecture: a Python script holding your agent's thoughts in a
`dict`, a JSON blob, or an in-memory vector store. Then it crashes. Everything
your agent learned is gone. The API credits are burned. The context window
overflowed three iterations ago. And you can't trace *why* it made that
decision because the reasoning was evicted from the buffer.

**That's not an agent. That's a slot machine with a nice README.**

The coding agents — Claude Code, OpenCode, Cursor, Copilot, pi-agent, Hermes,
OpenClaw — are worse. Every session starts from zero. No memory across
invocations. No audit trail of what was tried and failed. The agent fixes a
bug on Tuesday, reintroduces it on Thursday because it has no idea it already
solved this problem. You paste in the same 400 lines of context every time.
You repeat yourself more than the model does.

**That's not autonomous. That's a fancy autocomplete with a CLI.**

---

## The Database *Is* The Agent

Consensus flips the architecture. The database is not a sidecar bolted on
after the fact to save artifacts. The database **is** the execution engine.

- Agent context is a live SQL VIEW, not a Python dict
- Agent memory is an append-only ledger enforced by database triggers — not a
  JSON file that gets overwritten on the next save
- Every state change is ACID-committed or fully rolled back — no "the agent
  thinks it saved but the filesystem disagrees"
- Session isolation is enforced at the DB layer — Agent A physically cannot
  read Agent B's data

This isn't theoretical. It's running. Right now. With real DeepSeek API calls.

### Proof — Live Demo Output

```
╔══════════════════════════════════════════════════════════════╗
║     CONSCIENCE — Real LLM-Powered Agent Harness Demo        ║
╚══════════════════════════════════════════════════════════════╝

━━━ DEMO 2: Multi-Topic Sessions ━━━
   ┌─ Demo 2a (Security) ─────────────────────────────
   │ Status: idle | Iterations: 2
   │ Memory events: 4
  💬 Cross-site request forgery (CSRF): an attacker tricks a user
      into performing unwanted actions on a trusted site
  💬 Cross-site scripting (XSS): an attacker injects malicious
      scripts into web pages viewed by other users
  💬 SQL injection: an attacker inserts malicious SQL code into
      input fields to manipulate database queries
   └─────────────────────────────────────────

━━━ DEMO 3: Crash Recovery ━━━
   💥 Server killed (simulating crash)
   ✓ Database intact on disk
   ✓ Server restarted
   ✓ Session data intact — crash recovery works
```

Real LLM calls. Real agent output. Survives kill -9. Try that with LangChain.

Run it yourself:
```bash
DEEPSEEK_API_KEY=$DEEPSEEK_API_KEY go test -v -run TestDemo -timeout 300s ./demo/
```

---

## What Your Current Framework Can't Do

| Your Framework | What Happens | Consensus |
|---------------|-------------|------------|
| Agent computes a value, server restarts | Gone. Start over. Burn more tokens. | SQLite WAL. Heartbeat auto-resumes. Proven. |
| Agent makes a bad decision | Good luck finding *why*. The reasoning was evicted 5 turns ago. | Append-only `memory_events` ledger. Full audit trail. Every thought, every tool call, permanently recorded. |
| Context window hits 128K tokens | Hope your manual truncation didn't cut anything important. | Vector-validated compression. Every summary must pass cosine similarity ≥0.85 against the original. Fail → escalate. No guesswork. |
| "I know the agent mentioned the API key issue somewhere" | `grep` through log files. Good luck. | Semantic retrieval. Embed your query, get ranked results by cosine similarity. Find what you need in 20ms. |
| Two agents running concurrently | Shared dicts. Race conditions. Agent B overwrites Agent A's state. | Session-scoped memory. DB-level isolation. Physically impossible to cross-contaminate. |
| API rate limit triggers a retry loop | Agent retries 400 times before you notice. $200 gone. | `agent_circuit_breakers`. 2 consecutive errors → session pauses. Configurable per session. |
| Agent "saves" its work | Did it actually save? Did the file write complete? Who knows? | ACID transactions. Commit fully or rollback entirely. No partial state. Ever. |

### Your Coding Agent Has Amnesia

| Tool | What It Can't Do | Consensus |
|------|-----------------|------------|
| Claude Code, OpenCode, Cursor, Copilot | Session starts from zero. Every. Single. Time. | Persistent sessions with append-only memory. Agent remembers what it did yesterday. |
| pi-agent, Hermes, OpenClaw | "I already fixed this bug. Why am I fixing it again?" | Full audit trail. Every decision, every tool call, every fix — permanently recorded and queryable. |
| All of them | You paste context. It responds. You paste more context. It responds again. | The agent owns its context. It queries its own memory. It decides what's relevant. |
| All of them | No shared state between parallel agents. Each lives in its own silo. | Multi-session with DB-level isolation. Agents can spawn sub-agents that share a memory ledger. |
| All of them | $50 debugging session, 40 turns deep, terminal crashes. Gone. | SQLite WAL. Heartbeat resumes active sessions on restart. Every token you paid for is preserved. |

---

## The Test Suite Doesn't Lie

```
28/28 packages green — zero failures
60/60 acceptance criteria passing
```

Not mocked. Not simulated. The E2E tests launch a real server, make real
DeepSeek API calls, kill the server mid-session, and verify the agent
resumes cleanly on restart. The semantic retrieval tests insert 20 events
across 4 topic clusters and verify the right events come back for every query.

---

## Quick Start

```bash
go build -o bin/consensus ./cmd/consensus/
./bin/consensus init
./bin/consensus serve
```

Three commands. You have a running agent harness with:
- Append-only memory ledger
- Semantic retrieval
- ACID transactions
- Crash recovery
- Circuit breakers
- Session isolation

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                       Consensus                              │
│                                                               │
│  ┌──────────┐   ┌──────────┐   ┌───────────────────────┐    │
│  │ REST API │   │ Harness  │   │ Compression Worker    │    │
│  │ (chi)    │   │ (core    │   │ (vector-validated     │    │
│  │          │   │  loop)   │   │  summarization)       │    │
│  └────┬─────┘   └────┬─────┘   └───────────┬───────────┘    │
│       │               │                     │                │
│       └───────────────┼─────────────────────┘                │
│                       │                                      │
│              ┌────────┴────────┐                             │
│              │  SQLite / PG    │                             │
│              │                 │                             │
│              │  sessions       │  ← agent identity           │
│              │  memory_events  │  ← append-only ledger       │
│              │  event_embeddings ← semantic retrieval        │
│              │  staging_buffer │  ← staged SQL execution     │
│              │  circuit_breakers ← safety limits             │
│              │  compression_queue ← summarization            │
│              │  audit_logs     │  ← full traceability        │
│              └─────────────────┘                             │
└──────────────────────────────────────────────────────────────┘
```

## License

MIT

---

**[Run the demo →](demo/)** &nbsp;|&nbsp; **[Specifications →](specs/)** &nbsp;|&nbsp; **[Deployment →](deploy/)**
test change
