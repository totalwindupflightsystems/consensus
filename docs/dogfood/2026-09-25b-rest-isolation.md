# Dogfood — REST surface, crash recovery, session isolation (2026-09-25, second run of the day)

ANGLE NOTE: this is the SECOND dogfood run on 2026-09-25. The first (this morning,
`2026-09-25-mcp-surface.md`) swept the MCP surface; per the "change the ANGLE" rule this
run took the surfaces it did NOT touch: the documented REST API (docs/API.md), the
README's two headline claims (crash recovery, session isolation), the key-scoping model,
and session lifecycle.

## Setup (real-user path, no shortcuts)

- Scratch DB via the documented env-only form: `CONSENSUS_DB_URL=sqlite:///tmp/... CONSENSUS_PORT=8140 ./bin/consensus init` then `serve`. Both worked first try.
- Port 8090 was occupied by a foreign process; the CLI's stale-sidecar diagnostic fired with
  an actionable message (ss command + two free-port forms). Good UX, works as documented.
- Bootstrap admin key printed once at init, `cs_ak_...`, stored hashed. Documented behavior confirmed.
- LLM: `DEEPSEEK_API_KEY` env (a live key, pre-probed per ENV-CONSENSUS-1 practice).

## What was exercised, and what happened

### The happy path works
- `POST /api/v1/sessions` with agent_name+goal → 201; `POST .../message` → async; poll to
  idle in ~3s (warm); the assistant reply lands in `last_message` and as a `text_block`
  memory event. Server log shows real LLM calls (`llm: response received ... prompt_tokens=1638 completion_tokens=158`).
- Errors are JSON-enveloped with codes (INVALID_REQUEST, UNAUTHORIZED) as the doc's error
  contract promises. 401 negatives on no-auth and bad-key both correct.
- SSE `/api/v1/events?session_id=...` streams live (connected + session_update frames).

### FINDING A — P1: the cost ledger never records anything (billing is silently dead)
- Two real turns + two LLM calls later: `agent_billing` has 0 rows, session
  `tokens_used_in/tokens_used_out` = 0/0, `GET .../billing` returns totals 0 with empty entries.
- Meanwhile the server LOG shows the true usage (prompt_tokens=1638, completion_tokens=158,
  model=deepseek-flash). So the data exists at the LLM layer and is dropped before the ledger.
- Why it matters: the README sells "database is the runtime, ACID, every state change
  committed" and billing is one of the four headline tables. A user running this against a
  paid API sees $0.00 forever while spending real money. Note DF-CONSENSUS-6's fix history
  already covered "tokens_used_in/out stay 0" once — that fix apparently restored the LLM
  path but not the billing write.
- Repro: fresh DB, any live key, 2 turns, then
  `sqlite3 <db> "select count(*) from agent_billing"` → 0.

### FINDING B — P1: DELETE /api/v1/sessions/{id} does not delete (200 + row survives)
- `DELETE` → 200; a second DELETE → 200 again (idempotently lying).
- The row REMAINS: still in `GET /api/v1/sessions`, still fetchable by id (status flipped to
  `failed`), DB `sessions` table still has it, and `POST .../message` on the "deleted"
  session still returns 200 message_received and the turn runs.
- This is the worst kind of leak for a DB-native product: the API contract says deleted, the
  database (the thing this project IS) says alive. Either DELETE should remove the row (and
  404 afterwards) or return 405/410 with a documented tombstone semantic.
- Repro: create session → DELETE → GET (200) → message (200, turn executes).

### FINDING C — P1: key-mint response leaks a REDACTED placeholder instead of the key
- `POST /api/v1/auth/keys` (admin key) → `{"api_key": "cs_sk_...be2e", ...}` — the `api_key`
  field is a `cs_sk_...` PREFIX+suffix placeholder, NOT a usable secret. Three mints, same shape.
- Docs are explicit: "the response includes the new key's secret in the api_key field —
  shown once". Following the docs, the key is unrecoverable (stored hashed, never shown
  again) → the minted key is dead on arrival and the docs' worked sequence cannot be completed.
- The admin bootstrap key works, so auth itself functions; but the scoped-key feature is
  unusable through the documented path.
- Repro: bootstrap key → POST /api/v1/auth/keys {"scope":"readonly"} → inspect .api_key (contains "...").

### FINDING D — P2: SSE event stream has no authentication (read-any-session-by-UUID)
- docs/API.md documents `/api/v1/events` as "No authentication — session isolation is
  enforced via the session_id query parameter". Practically: ANY unauthenticated client that
  obtains/queries a session UUID streams that session's live events (status, iteration,
  update payloads). Verified live: no Bearer header, got `connected` + `session_update`
  frames for a session created under a different credential.
- The session UUID also appears in unauthenticated-reachable surfaces, so the "isolation"
  here is possession of a 128-bit ID, not authorization. Session-scoped API keys DO enforce
  403 on foreign session GETs (good) — the SSE route is the hole in that same wall.
- Fix direction: require a valid key (admin or scoped to that session) on /api/v1/events, or
  issue short-lived stream tokens. Update the doc to stop calling it "isolation".

### FINDING E — P2: skill artifact drifted to the dead repo path (wojons/consensus)
- `skills/consensus-usage/SKILL.md` still says module `github.com/wojons/consensus` (lines
  ~23, 97-98) while the real origin is `totalwindupflightsystems/consensus` (wojons/consensus
  is 404 anonymous — exactly the stale-URL class DF-CONSENSUS-25/DF-CONSENSUS-7 already
  fixed elsewhere). The usage skill is the artifact agents load FIRST; it must not carry the
  dead path.

### Crash recovery — the README's flagship claim — HELD UP
- Killed the server with SIGKILL mid-turn (session in `thinking`, iteration 4):
  `PRAGMA integrity_check` → ok; committed memory events (7) intact; session row survived.
- Restart → health ok, session resumed at `idle`, the post-crash turn even completed (8th
  memory event present). No stranded/staging residue reported, none needed.
- This claim is REAL. The database-runtime architecture delivers here.

### Session isolation — mostly REAL, one hole (Finding D)
- Session-scoped key (scope=session, session_id=SID): own session GET 200, foreign session
  GET/memory 403, cannot create sessions or mint keys (401), list shows only its own
  session. Solid scoping at the REST layer.
- The unauthenticated SSE route above is the exception that leaks the stream.

### Lifecycle quirks (minor, not filed individually — folded into F-B)
- PATCH pause on a `booting` session → 409 CONFLICT "cannot pause session in status booting"
  (correct envelope, but a session that just booted is unpausable for its first seconds).
- Session-scoped key creation initially returned 401 on my first attempt with expires_in —
  could not reproduce on retry; not filed (may have been my payload).

## Perf (Step 2b, one number per operation — no profile taken, nothing justified one)

- Warm conversational turn (POST message → poll idle, SQLite, live DeepSeek key):
  ~3.1s first turn, ~7.4s with a 500-token reply — LLM latency dominates (server log:
  elapsed_ms 1578–3181 per call). REST control operations (health, GET session) are
  single-digit ms. Cold server start → health ok: ~2-3s.
- Verdict per the perf law: nothing here is slow enough that a user would notice beyond the
  inherent LLM call time; no PERF row filed (a row would devalue the real findings).

## Verdict: 🟡 PROMISING-BUT-ROUGH (unchanged from previous runs, different reasons)

The core architecture delivers where it counts (crash recovery is genuinely impressive,
isolation scoping works at REST, errors are well-shaped, the async contract is now documented
honestly). But three P1s hit a real user within 15 minutes: billing silently dead (spend
invisible), DELETE lying (data you asked to remove stays alive), and the key-mint response
containing a placeholder instead of a key (scoped keys unusable via docs).

- Time-to-first-success: ~4 min (init → serve → session → first reply). Friction count: 5.
- Install leg: SKIPPED-install-bunker (bunker-las-03 unreachable; see board row) — runtime
  use ran on the control host with a scratch DB instead.
