# Consensus Demo

Run the live demo:

```bash
go test -v -run TestDemo_FullAgentHarness -timeout 300s ./demo/
```

Requires `DEEPSEEK_API_KEY` environment variable (or set in consensus.yaml).

## What It Demonstrates

> **Note:** The demo test skips entirely when `DEEPSEEK_API_KEY` is not set.
> Set the environment variable to run: `DEEPSEEK_API_KEY=sk-... go test -v -run TestDemo -timeout 300s ./demo/`

1. **LLM-Powered Agent Loop** — Creates a session, sends a task, and lets the heartbeat/planning loop
   process it with real DeepSeek API calls. Shows agent plans, SQL execution, and memory events.
2. **Multi-Topic Sessions** — Two concurrent sessions with different agent roles (security auditor,
   performance engineer) process independently, each storing topic-specific memory events.
3. **Crash Recovery** — Server killed mid-session (simulating crash) → database intact on disk →
   server restarted → session data and memory events survive.

## Expected Output

```
╔══════════════════════════════════════════════════════════════╗
║     CONSCIENCE — Real LLM-Powered Agent Harness Demo        ║
╚══════════════════════════════════════════════════════════════╝

✓ Server started — admin key: cs_ak_...
✓ Heartbeat loop active — will auto-process sessions

━━━ DEMO 1: Agent Plans & Executes via LLM ━━━
   Session xxx... created
   Waking session for heartbeat pickup...
   Waiting for heartbeat to process session...
   ┌─ Demo 1 ─────────────────────────────
   │ Status: completed | Iterations: 2 | Tokens: …
   │ Memory events: 3
  💬 [text_block] CREATE TABLE demo_tasks …
  🔧 [tool_call] sql_execute …
   └─────────────────────────────────────────

━━━ DEMO 2: Multi-Topic Sessions ━━━
   ┌─ Demo 2a (Security) ─────────────────────────────
   │ Status: completed | Iterations: 2
   │ Memory events: 4
  💬 Cross-site request forgery (CSRF): …
  💬 Cross-site scripting (XSS): …
   └─────────────────────────────────────────
   ┌─ Demo 2b (Performance) ─────────────────────────────
   │ Status: completed | Iterations: 2
   │ Memory events: 3
  💬 Query optimization: proper indexing …
   └─────────────────────────────────────────

━━━ DEMO 3: Crash Recovery ━━━
   💥 Server killed
   ✓ Database intact on disk
   ✓ Server restarted
   ✓ Session data intact — crash recovery works

╔══════════════════════════════════════════════════════════════╗
║                      DEMO COMPLETE                          ║
╠══════════════════════════════════════════════════════════════╣
║  Real LLM calls: DeepSeek V4 Flash (via HTTPS API)          ║
║  Agent plans, executes SQL, stores memory — autonomously    ║
║  Data survives server crash + restart                       ║
║  Sessions queryable via REST API                            ║
╚══════════════════════════════════════════════════════════════╝
```
