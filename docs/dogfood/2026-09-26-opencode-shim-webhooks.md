# Dogfood 2026-09-26 (evening, run #9) — OpenCode-shim attach + webhook ingestion, real client

**Verdict:** 🟡 PROMISING-BUT-ROUGH. The server core keeps getting better (breaker
liveness, webhook ingestion honest + working, MCP bare-mount fixed, install leg
passed on a 4th distinct host), but the day's headline surface — attaching a real
opencode client — fails at the contract level: the TUI crashes on boot and run
mode is fire-and-forget-invisible.

Workdir: clean git worktree @ `fa24a3e` (main tree had 16 uncommitted foreman
files — isolation doctrine). Scratch instance: port 8130, sqlite
`/tmp/dogfood-run/dogfood.db`, real DeepSeek key (probed `HTTP 200` on
`/models` first). Client: `opencode 1.18.29` (installed at `~/.opencode/bin/opencode`).

## Promise under test

SPEC-017: *"the opencode shim lets `opencode attach http://localhost:8090`
work"* — a real opencode client uses Consensus as its backend. Plus the newly
landed webhook ingestion (SPEC-013 + WEBHOOK-1, merged the same day, never
user-tested).

## What was done, in order

1. **Bootstrap.** Build 8s. `init` friction: the documented
   `init --config <path>` silently ignores the flag (see DF-CONSENSUS-42);
   `CONSENSUS_CONFIG` env var is the working path. Server up ~4s, schema 24,
   both shims enabled per log.
2. **Shim surface sweep (curl).** `/global/health` 200 keyless;
   `/session` 401 anonymous → `[]` with basic auth (admin key accepted);
   `/config`, `/provider`, `/agent` sane; `/doc` 200 JSON; `/instance`
   keyless per spec §3.10. **DF-CONSENSUS-37 fix verified**: anonymous
   `GET /path` with `x-opencode-directory: /tmp` → 200, echoes
   `"worktree":"/tmp"`; `POST /log` validates and 400s with a clean
   opencode-format error.
3. **Real client, run mode.** `opencode run "<msg>" --attach
   http://127.0.0.1:8130 --dir <dir> -p <adminkey>`. First attempt: exit 0,
   **zero bytes of output** — but the server log showed the session was
   created through the shim, the harness picked it up, and DeepSeek 401'd
   (my env mistake: the config interpolates `${DEEPSEEK_API_KEY}` and I
   hadn't exported it — the literal unexpanded string went to DeepSeek).
   Side product: the **circuit breaker tripped live after 3 consecutive
   401s, persisted 3 rows in `agent_circuit_breakers`, and paused the
   session** — the README's breaker promise holds (first live observation
   in 9 runs; runs 1-2 claimed it broken, it demonstrably works now).
4. **Fix env, restart, re-drive.** Resumed the paused session via native
   `PATCH .../sessions {status:resume}` (action verbs are pause/resume/cancel —
   `idle` is a state, not an action). Then shim-path message → canned ack in
   66ms, real `PONG` in `memory_events` ~3s later. Full loop works
   server-side.
5. **Attach contract.** `opencode run --attach` after the key fix: exit 0 in
   ~1s, 0 bytes — while the server created the session, ran the turn, and
   filed `BANANA` into the ledger 5s later. → DF-CONSENSUS-44.
6. **TUI attach.** `opencode attach` in a PTY: black screen; typed text is
   echoed locally; Enter submits; **zero HTTP requests reach the server in 4
   minutes** (no new sessions, no log lines, `/event` never opened). Killing
   the PTY and re-running output-captured reproduces a clean crash:
   `opencode crashed — TypeError: undefined is not an object (evaluating
   'U.data.provider_default[I.id]')`. Shim `GET /config` returns only
   `{"settings":...}`; the pinned upstream client reads `provider_default`
   off that payload. → DF-CONSENSUS-43.
7. **Webhook ingestion (full doc recipe).** Registered `docs-demo` by direct
   SQL (docs are honest: no registration API exists). Valid signed delivery
   → `202 {"status":"accepted"}`; bad signature → `202` + row
   `signature_valid=0, status=quarantined`; repeated delivery id → `200
   duplicate`. Routing rule (`source_pattern='webhook'`,
   `event_type_pattern='ping'`, target = the breaker-paused session):
   event `pending→routed` within one 5s loop tick; log line `webhook: woke
   session via event routing`; session `paused→idle`. **But** the wake
   dispatched no iteration — the session's stranded unanswered message stayed
   stranded and the webhook payload never reached the agent. →
   DF-CONSENSUS-46. Also noticed: shim-path user messages are stored
   JSON-quoted vs raw on the native path → DF-CONSENSUS-45.
8. **Bunker install leg.** las-03 timeout (4th run), las-01 timeout,
   las-02 bunkerd crash-looping (`refusing to bind non-loopback plaintext
   listener`; restart counter 46,627 — fleet-infra finding, not consensus's;
   left untouched, out of scope). Substituted **dedi-2** (alive, registered):
   agent `6b3c5c53`, anonymous clone of
   `totalwindupflightsystems/consensus` @ `fa24a3e` (same HEAD), Go 1.26.5
   provisioned, `BUILD_SECONDS=109`, init→serve→**health 200, schema 24**.
   Bonus smoke: bare `GET /mcp` on the fresh clone returns `200` + SSE
   `event: endpoint` — **DF-CONSENSUS-23 (bare /mcp 404 P0) live-verified
   FIXED** at HEAD (board row already `complete`; first independent
   user-side confirmation). Destroy failed server-side
   (`deadline_exceeded` ×3; daemon audit shows `outcome:"internal"` ~30s);
   agent processes were stopped first and the 2h TTL backstop is armed;
   manual cleanup on dedi-2 is the follow-up (same class as 09-26).
9. **Perf (Step 2b).** Healthy turn: POST accept 66ms; LLM 1.9–3.9s;
   post→reply-in-ledger 2.8–3.2s; cold-session first turn ~5s; build 8s warm
   / 109s cold-with-toolchain-provision. The PERF-CONSENSUS-11 stall class
   did not reproduce (small sample, 6 turns). Nothing slow enough to
   profile — **no PERF row filed** (a measurement, not an omission).

## Numbers

| thing | value |
|---|---|
| time-to-first-success (shim message → ledger reply) | ~7 min from clean worktree (build+init+serve+first turn) |
| turn latency (accept → reply observable) | 2.8–3.2s (LLM 1.9–3.9s, accept 66ms) |
| cold build (fresh host incl. Go provision) | 109s |
| friction count | 3 (`--config` dead, TUI crash, run-mode invisibility) + 2 minor (resume verb set, SSE-holding curl) |
| rows filed | DF-CONSENSUS-42 (P1), -43 (P1), -44 (P1), -45 (P2), -46 (P2) |

## What a new user should know (the two-minute version)

- Configure via `CONSENSUS_CONFIG` until DF-CONSENSUS-42 lands; the `--config`
  flag lies.
- The opencode shim speaks the protocol (auth, fixed-workspace, message
  delivery all verified at HTTP level) but the TUI attach is broken until
  DF-CONSENSUS-43; use the REST API or MCP for real work today.
- Webhook ingestion works exactly as docs/API.md says — including the honest
  parts (quarantine-instead-of-reject, dead `event_types` field). Don't
  expect a routed event to resume an agent's pending work (DF-CONSENSUS-46).
- Breaker-on-LLM-auth-failure is real and durable; resume with native
  `PATCH /api/v1/sessions/:id {"status":"resume"}`.

## Honest limitations of this run

- TUI tested via PTY harness (crash reproduced cleanly without TTY); a
  desktop terminal may render the same crash visibly — the root cause and
  the zero-request observation stand either way.
- Install leg ran on dedi-2 (substitution, recorded) because all three Vegas
  bunkers were unreachable/down; destroy needed manual follow-up.
- `opencode` client version pinned at 1.18.29 (the repo's own evidence
  target); newer clients may behave differently — the shim must track the
  pinned client, which is exactly what SPEC-017 pins.
