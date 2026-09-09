# Dogfood Integration Report — 2026-09-09

**Target:** consensus @ `8e3e7e6` (master), Go 1.26.5, built from source in 2s.
**Mode:** regression + fix-verification pass (3rd re-run; 9 prior DF tasks still pending on the board).
**Verdict:** 🔴 **DOES-NOT-DELIVER (documented path)** / 🟡 PROMISING-BUT-ROUGH (undocumented config-file path) — see split verdict below.

## The Promise (under test)

> A fresh user follows the README: run the server (docker image, or env-var
> invocation), create a session, POST a message, get the agent response.
> Sessions survive `kill -9`. Circuit breakers cap LLM spend.

## What Actually Happened

### 1. The README env-var install bricks the server on first real use (NEW P0, DF-CONSENSUS-10)

Started exactly the documented way:

```bash
CONSENSUS_DB_URL="sqlite:///tmp/dogfood-consensus/scratch.db" \
CONSENSUS_PORT=8124 DEEPSEEK_API_KEY=<real key> \
./consensus serve
```

`init` → `serve` → health 200 → session 201 → message POST 200. Then, on the
first harness pass:

```
planning: starting interactive session  max_turns=10 timeout=3m0s
(3 minutes of nothing)
harness: planning failed  error="planning: initial context: read context:
  begin tx: sqlite: begin tx: context deadline exceeded"
harness: found active session            <- heartbeat retries the same session
```

During the wedged window the API returns `401 UNAUTHENTICATED` to the **valid
admin key**, then stops answering entirely — including the **no-auth**
`/api/v1/health` (curl HTTP 000, 10s timeouts). kill -9 → `PRAGMA
integrity_check: ok` → restart → heartbeat auto-resumes the poisoned session →
**re-wedges within seconds**. A session wedged this way bricks the instance on
every future boot; the only fix is deleting the session/DB.

**A/B proof it's the connection pool:** the identical binary and identical
workflow with a config file pinning `database: { max_open_conns: 4 }` runs the
whole loop cleanly (`llm: calling provider → response received → commands
staged → committing → idle`). The env-var path exposes no pool knob, and the
WAL grew to 1.5 MB on a near-idle DB — pool starvation. The 2026-09-03 run
"verified" the goal-driven pattern only because its scratch recipe happened to
pin `max_open_conns: 4` in a config file.

### 2. Conversational path still deaf to the user (DF-CONSENSUS-11, re-verified)

With the wedge eliminated (pinned pool, real key, fresh DB):

```
POST /api/v1/sessions/{id}/message  {"role":"user","content":"What is 2+2? ..."}
→ 200 {"status":"message_received"}   (9 ms; docs still say "returns the agent response")

llm: calling provider   messages=2          <- system + context, NO user turn
llm: response received  prompt_tokens=1580 completion_tokens=104
planning: responding to user  turn=1

memory_events row 2: "The session goal is 'standby' ... There is no user
request or pending task to act on. The appropriate action is to remain in
standby"
```

The user text IS durably stored as a `memory_events` `user_message` row — and
the prompt builder never projects it into the LLM turn list. The model
explicitly says it sees no user request. Same root cause as the 09-03 finding,
now cleanly isolated from the pool wedge.

**Second bug in the same verified run:** the server logs real token usage
(`prompt_tokens=1580 completion_tokens=104`) but `sessions.tokens_used_in/out`
stay `0/0` and `agent_billing` stays empty. The README's circuit-breaker and
budget-guard promises have no fuel: nothing is counting.

### 3. What DOES work (2026-09-09 verified)

- Build from source: `go build ./cmd/consensus` — 2 s, zero issues.
- Goal-driven pattern **with pinned pool**: real DeepSeek calls, SQL commands
  staged and committed, session ends `idle`. This is the only working
  conversational-adjacent path, and it requires a config file today.
- SSE stream: `connected` + `session_update` frames flow correctly.
- Append-only ledger: user message durable across kill -9; DB integrity ok.
- Heartbeat auto-resume: works exactly as documented (too well — see poison-pill).
- Anonymous `git clone` from GitHub now works (was broken 09-03). Docker/ghcr
  pull is STILL `DENIED` (403 anonymous, token endpoint returns DENIED).

## The Working Recipe (the only one that works today)

```yaml
# consensus.yaml — the README env-var path bricks the server (DF-CONSENSUS-10)
server: { port: 8124 }
llm: { default_model: deepseek-chat, provider: openai,
       base_url: https://api.deepseek.com/v1, api_key: "" }   # key via env
harness: { heartbeat_interval_seconds: 3, max_iterations: 3,
           max_consecutive_errors: 2, budget_limit_cents: 100 }
database: { url: "sqlite:///tmp/cs/scratch.db", max_open_conns: 4 }  # <- the fix
```

```bash
export DEEPSEEK_API_KEY=sk-...
consensus init --config consensus.yaml   # save the printed cs_ak_ bootstrap key
consensus serve --config consensus.yaml
# session create REQUIRES agent_name + goal (undocumented in docs/API.md):
curl -X POST :8124/api/v1/sessions -H "Authorization: Bearer $KEY" \
  -d '{"agent_name":"x","goal":"Do a thing"}'
# wake it (role:user messages never reach the LLM — DF-CONSENSUS-11):
curl -X POST :8124/api/v1/sessions/$SID/message -H "Authorization: Bearer $KEY" \
  -d '{"type":"user_instruction","content":"Start now."}'
```

## Friction Count: 9

1. Env-var install wedges the whole server (P0).
2. Wedge survives restart via heartbeat poison-pill (P0).
3. 401-to-valid-key + health-freeze during wedge, no error surfaced to clients.
4. Conversational message never reaches the LLM (P0, third run in a row).
5. Docs still promise a synchronous response contract.
6. `agent_name`/`goal` required but absent from documented curl (third run).
7. tokens_used/billing stay 0 during real calls — breaker/budget dead.
8. ghcr image still not anonymously pullable.
9. Session-create silently ignores `model` field.

Time-to-first-success: never on the documented path (~12 min to conclude it
cannot succeed); ~4 min on the config-file path (build 2s + init + serve +
session + wake + verified LLM call).
