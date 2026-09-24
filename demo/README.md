# Consensus Demo

Run the live demo:

```bash
go test -v -run TestDemo_FullAgentHarness -timeout 300s ./demo/
```

Requires `DEEPSEEK_API_KEY` environment variable (or set in consensus.yaml).

## Keyless Smoke Test (no API key, no cost)

Validate the install end-to-end without spending money or waiting minutes:

```bash
make smoke
```

Runs the full loop — real server binary, scratch SQLite database,
session → heartbeat → LLM call — against a mocked OpenAI-compatible
endpoint. Passes in under 60 seconds.

## What It Demonstrates

> **Note:** The demo test skips entirely when `DEEPSEEK_API_KEY` is not set.
> Set the environment variable to run: `DEEPSEEK_API_KEY=sk-... go test -v -run TestDemo -timeout 300s ./demo/`

1. **LLM-Powered Agent Loop** — Creates a session, sends a task, and lets the heartbeat/planning loop
   process it with real DeepSeek API calls. Shows agent plans, SQL execution, and memory events.
2. **Multi-Topic Sessions** — Two concurrent sessions with different agent roles (security auditor,
   performance engineer) process independently, each storing topic-specific memory events.
3. **Crash Recovery** — First commits a non-user agent artifact, starts a second iteration, observes
   that work is in flight, and kills the server. After restart, the demo verifies the same committed
   artifact remains. The open transaction is expected to roll back; staged/executed
   `staging_buffer` residue is printed as recovery diagnostics rather than counted as success.

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
   Pre-crash committed evidence: durable_agent_artifacts=1 ...
   ✓ Observed in-flight status "planning"
   💥 Server killed while work was in flight
   ✓ Database intact on disk
   Recovery diagnostics after abnormal exit: ... staging_buffer={...} stranded=N stranded_entries=[...]
   ✓ Server restarted
   ✓ Committed agent artifact survived restart
   ℹ In-flight transaction is not claimed as durable

╔══════════════════════════════════════════════════════════════╗
║                      DEMO COMPLETE                          ║
╠══════════════════════════════════════════════════════════════╣
║  Real LLM calls: DeepSeek V4 Flash (via HTTPS API)          ║
║  Agent plans, executes SQL, stores memory — autonomously    ║
║  Committed agent progress survives crash + restart          ║
║  Sessions queryable via REST API                            ║
╚══════════════════════════════════════════════════════════════╝
```
