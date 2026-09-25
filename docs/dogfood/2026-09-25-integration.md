# Dogfood Integration Report — consensus @ 235efdb (2026-09-25)

Angle change per dogfood doctrine: the 2026-09-24 run swept the session/CLI
surface at 576dc07 and found DF-CONSENSUS-20 (turns 2+ deaf) + DF-CONSENSUS-21
(P0 517 lock race). Since then the foreman landed 97d2ff1 (DF-15 fix) and
e9b34eb (DF-21 fix). Today a real user multi-turn conversation was driven
against a fresh scratch instance to (a) live-verify the closed P0, (b) retest
the open P1, (c) time the headline operation, (d) run the bunker install leg.

## Setup (fresh user path, control host)

Built from current HEAD `235efdb` (`go build -o bin/consensus ./cmd/consensus/`,
15.1s warm), `init --db-url sqlite:////tmp/cdog925.db`, serve on :8126 with the
documented env-var path + `consensus.yaml`, live DEEPSEEK_API_KEY. Health 200,
37 tables, schema 23, in ~4s.

## 1. DF-CONSENSUS-21 fix — LIVE-VERIFIED (was P0)

The exact 517 lock race fired naturally on this run and the new retry path
recovered it every time:

- Turn 2 of session 493115e3: successful LLM call (2717ms, 1683 in/432 out),
  then `planning: retrying user-message consumption after sqlite busy ...
  attempt=1 backoff=10ms error="sqlite tx: exec: database is locked (517)"`.
  Planning continued, reply delivered, session stayed healthy.
- Burst battery (the row's own pass criterion, executed literally): **20 rapid
  message-sends (5 parallel per session x 4 sessions, no polling between
  sends), all HTTP 200 in 0.03-0.44s; all 4 sessions ended idle with
  last_error=null; 5/5 busy-race retries succeeded; 0 failed sessions** in the
  serve log. Correct per-message echoes on all sessions (one session batched
  its 5 queued messages into one combined turn with all 5 tokens echoed —
  reasonable queue-drain behavior, not a defect).

## 2. DF-CONSENSUS-20 fix — LIVE-VERIFIED (turns 2+ now deliver)

Yesterday's P1 (byte-identical prompt_tokens across turns; model blind to
turn-2 content) is fixed at 235efdb:

- Session 493115e3 turn 1: prompt_tokens=1679, reply exactly "TURN1-FIX21".
- Turn 2 ("Reply with only: TURN2-CONTENT-CHECK-925"): prompt_tokens=**1683**
  (differs), the model's reasoning block quotes the turn-2 token verbatim, and
  the persisted reply is exactly "TURN2-CONTENT-CHECK-925".
- Cold timed session 6d5fcc9f: reply exactly "PERF-COLD-925" (~4s wall).
- Burst probes each echoed their distinct token correctly.

Multi-turn conversation is now a working surface. The "put everything in the
goal" workaround from the 09-24 report is no longer required.

## 3. New performance finding: harness pickup stall (see PERF-CONSENSUS-11)

Headline op = POST message → reply observable via GET /context.

- LLM provider call: 1.4-2.9s (deepseek-flash, ~1.7k prompt tokens) — fine.
- POST accept: 0.03-0.2s (HTTP 200) — fine.
- Warm session 6d5fcc9f, turn 3: **94.7s** POST→reply. Turn 4: **108.2s**
  (POST 0.04s, pickup→reply 108.1s). LLM call inside: 1.6-1.8s.
- Serve log: dispatch is heartbeat-driven (`internal/harness/executor.go:607`,
  5s ticker) and the loop stalled 6m25s (20:40:32→20:46:57) then ~4m
  (20:47→20:51) between "message_received" and "harness: found active session".
- A fresh session sent during the SAME window picked up in 6.3s, and the same
  stalled session's NEXT turn ran 2.8s — the stall is intermittent, hit
  specific turns on a previously-bursty session, and self-healed.
- Fresh-session turn 2: 4.3s; fresh-session turn 1: 6.3s (includes 5s ticker
  worst-case wait — healthy).

No profile was captured (the stall had cleared by the time we instrumented;
pprof is not imported into the binary). The row carries the numbers + log
evidence + hot path; profiling belongs to the foreman who can reproduce.

## 4. Minor findings

- **DF-CONSENSUS-22 (P2):** every serve start logs
  `WARN Compression worker DISABLED — provider DeepSeek has no embeddings
  endpoint (set compression.enabled=false or configure an OpenAI-compatible
  embeddings provider)` — but the shipped `consensus.yaml` still ships
  `compression.enabled=true` (C-GAP-002 closed the config default; the shipped
  file itself still disagrees with the provider). New users see a WARN on
  every boot of the documented path.
- **Install leg (see below):** Go version drift — README quickstart says
  1.26.5; go.dev now serves 1.26.6 (works fine). Cosmetic.
- Bunker `/tmp` quirk (not a consensus defect): a stale root-owned go.tgz
  from a previous agent blocked re-download (`rm: Operation not permitted`);
  the already-downloaded tarball was used.

## Right way to use consensus today (as of 235efdb)

1. Source-build quickstart (clone → go build → init → serve) works end to end;
   both env-var and config paths are safe. ghcr image still requires
   authenticated pull.
2. Multi-turn conversation now works — messages reach the LLM on every turn
   (DF-CONSENSUS-20/21 fixes verified live). Goal + message both flow.
3. Expect 0.03s POST accept, ~1.5-3s LLM, reply observable within one 5s
   heartbeat tick (~2-6s typical). If a turn sits in message_received for
   minutes, it is the heartbeat dispatch stall (PERF-CONSENSUS-11) — poll
   /context; the reply lands when the loop recovers.
4. `agent_name` (+ `goal`) are the required create fields; message route is
   POST /api/v1/sessions/{id}/message.
5. Docs gap: README still does not name the Go toolchain requirement for the
   bare-machine path (carried from 09-24; go1.26.6 confirmed working).

## Install leg — PASSED (ephemeral bunker)

bunker-las-03 (100.69.3.13, bunkerd active, Docker 26.1.5), agent fa023371
(TTL 2h, destroyed after use):

- anonymous git clone 4.8s → HEAD 235efdb
- Go toolchain: NOT on the box (bare Debian 13.7, expected); go1.26.6
  installed to ~ (README does not name it — carried docs finding)
- `go build -o bin/consensus ./cmd/consensus/` 58s cold (module downloads incl.)
- init → serve → /api/v1/health 200 (`status":"ok", db_backend sqlite,
  schema 23`) — smoke PASSED
- install_seconds ≈ 58 (build; Go install excluded — not part of the
  project's own docs)
- bunker destroy fa023371: confirmed removed, 0 agents remaining

## Verdict

SHIPPABLE on the conversational promise, with one intermittent dispatch-stall
watch item: the headline promise ("send a message, get the response") now
holds across many turns on a live LLM; both of yesterday's defects are
verified fixed under the exact conditions that triggered them; installability
proven again from zero. The PERF-CONSENSUS-11 stall (94-108s on 2 of ~30
turns) is the one user-visible wart and is filed with numbers.

Perf summary: healthy turn 2-6s end-to-end; 2 anomalous turns 95-108s
(heartbeat pickup stall, log-proven, intermittent). Nothing else was slow
enough to file — local API accept <0.05s, build 15s warm / 58s cold bunker,
clone 5-8s.
