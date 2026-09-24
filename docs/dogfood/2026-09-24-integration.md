# Dogfood Integration Report — consensus @ 576dc07 (2026-09-24)

Regression re-test of the three fixes the foreman landed after the 2026-09-09
run (ad8afd8 pool default 1→8, a8a787a conversational wiring + token
accounting, f5b3875 quickstart docs cluster), plus the bunker install leg that
was SKIPPED on 09-09.

## What was verified working (fixes hold)

1. **Documented env-var path no longer wedges the server (DF-CONSENSUS-10).**
   Fresh clone (2.4s) + `go build` (9.5s cold) + `init --db-url
   sqlite:////tmp/cf924.db` + `DEEPSEEK_API_KEY=... CONSENSUS_DB_URL=...
   CONSENSUS_PORT=8124 ./bin/consensus serve` → `/api/v1/health` HTTP 200 in
   under 5s, 37 tables, schema 23. The begin-tx deadline wedge is gone.
2. **Turn-1 conversational contract now delivers (DF-CONSENSUS-11, turn 1).**
   Session 91a4cf05: message → status idle in <5s, tokens_used_in=1695 /
   tokens_used_out=139 (real accounting), agent reply persisted and observable
   via GET /context and GET /memory: exactly "DOGFOOD-OK-0924". The 09-03/09-09
   "deaf harness" defect is fixed for the first turn.
3. **Quickstart drift cluster (DF-CONSENSUS-12):** README quickstart commands
   now match reality (init/serve flags, health shape, version probe all behaved
   as documented on a fresh checkout).
4. **Installability on a clean machine — PASSED (first time this leg ran).**
   Ephemeral bunker agent 89e7da8c on bunker-las-03 (destroyed after use):
   anonymous git clone 8.1s, `go build -o bin/consensus ./cmd/consensus/` 62s
   cold (module downloads included), init → serve → health 200. Go itself is
   NOT on the box (expected for a bare Debian user); the README does not say
   which Go to install — minor docs friction, noted below.

## New defects found this run

### DF-CONSENSUS-20 (P1): user message content never reaches the prompt on turns 2+

Multi-turn conversation is still broken — one level deeper than DF-15. Two
fresh sessions both produced **byte-identical prompt_tokens across turns with
different user text** (session 223e1096: 1668 in on both "TURN1" and "TURN2";
91a4cf05: 1695 on both), while memory_events correctly stores every
user_message row. The model visibly answers "Ready. No question or task has
been provided yet" to a turn whose content was "Reply with only: TURN2".
a8a787a wired the wake-up; the content filter still drops delivered
user_message rows from every prompt after the first call. Turn 1 works because
the first call happens before any hide; every later turn is blind. The
DF-CONSENSUS-15 fix direction (hide only on iteration rollover) does NOT fix
this — the hide survives rollover. Only escape hatch today: put the actual
request in the session GOAL (goal text is always in the prompt), as every
session in this run that produced a correct reply did.

### DF-CONSENSUS-21 (P0): 517 lock race permanently bricks a session, silently

`consume user messages: sqlite tx: exec: database is locked (517)` — planning
dies AFTER a successful, billed LLM call (2971ms, 1695 in / 515 out for
nothing). Frequency escalated through the run: 1 in the first 40 minutes, then
4/4 turns under a normal 0.5s-polling client (06:37–06:48), including a
brand-new session's FIRST turn. The session goes status=failed forever,
`last_error` stays null (the user sees no diagnostic), and a retry message to
the same session re-runs into the same locked tx. The fix class is retry-with-
backoff on SQLITE_BUSY for the consume tx + surfacing last_error + letting
failed sessions resume.

## Perf measurements (Step 2b)

- Headline op = send message → LLM reply observable. **Round trip never
  completed for measurement this run**: the 517 race killed every timed turn
  (first observed failure at ~6.3s: 1.3s LLM + planning crash). A perf row is
  NOT filed — you cannot benchmark a path that fails; DF-CONSENSUS-21 gates
  it.
- Reference points that did complete (turn-1 class, from serve logs):
  LLM provider call 1.3–3.3s wall (DeepSeek API, ~1.7k prompt tokens);
  local API accept → status update < 50ms; cold `go build` 9.5s control host /
  62s bunker; clone 2.4s / 8.1s.
- Install leg timing: 62s build + ~5s init/serve smoke on the bunker
  (Go install excluded — not part of the project's own docs).

## Right way to use consensus today (as of 576dc07)

1. Use the config-file or env-var path (both safe now; pin nothing).
2. Put the actual task in the session GOAL, not the first message — the goal
   is the only user input guaranteed to reach every LLM call.
3. One conversational turn per session is the reliable surface. Multi-turn
   breaks (DF-CONSENSUS-20) and can brick the session (DF-CONSENSUS-21).
4. Probe the API key first (ENV-CONSENSUS-1: dead keys mimic every symptom
   here); DEEPSEEK_API_KEY from ~/.hermes/.env verified live this run.
5. Docs gap: README does not name the Go toolchain requirement (observed on a
   bare Debian bunker: no go, no pointer in the quickstart).

## Errors encountered (chronological)

- `agent_name is required` on my first session create — my field guess, docs
  specify agent_name+goal (docs were right; friction was mine, not theirs).
- POST /api/v1/messages → 404 (route is /api/v1/sessions/{id}/message).
- 517 lock errors as above (3 sessions bricked: 91a4cf05, d3a45897, 47cb6a1f;
  846868a8 also died).
- Scheduler API :9090 timed out at report time (pre-existing scheduler-side
  slowness, not a consensus defect).

## Verdict

PROMISING-BUT-ROUGH — installability proven for the first time, turn-1
conversation fixed and token accounting real, but the headline promise ("send
a message, get the response") still fails on turns 2+, and the 517 race makes
even turn 1 unreliable under a normal client.
