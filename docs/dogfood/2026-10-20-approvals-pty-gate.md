# Dogfood 2026-10-20 — approvals end-to-end, GET /api/pty, DF-58 re-proof

Run: consensus-dogfood-2026-10-20-06-54-47 (read-only lane; no code changed, findings filed as rows).

## Promise under test

README: "the database is the runtime with real-time streams and kill-9 durability."
This tick's angle drove the newest promise surface the README/docs make:

1. the approvals API (docs/API.md Approvals section) — first real drive of the
   APPROV-GATE-001 user surface (fix landed 2026-10-17, never dogfooded);
2. `GET /api/pty` — brand-new route (634517b, merged to main Oct-19 00:29);
3. DF-CONSENSUS-58 re-proof (restart without key env; pprof collision);
4. DF-CONSENSUS-56 fix verification (SSE terminal transitions, b88c591).

## Environment

- Local scratch: binary built from HEAD 6736542 (main), sqlite `/tmp/dfsync-df2/scratch.db`,
  port 18777, real DeepSeek provider (config `api_key: ${DEEPSEEK_API_KEY}`).
- Install leg: fresh GitHub clone on a clean bunker agent (dfsync-df1020, bunker-mvp),
  README path verbatim, no key exported.

## What worked (with numbers)

- **Real turns, cold and warm.** create 55ms, ack 35ms, cold turn 2.09s, warm 2.06s,
  correct answers end to end (17*23=391). Billing rows written per turn.
- **kill -9 durability.** Pre/post restart: session row, 3 memory rows, admin key,
  billing byte-identical (`diff` = YES). Two separate kill -9 cycles across the run.
- **Documented resume.** `PATCH /api/v1/sessions/{id} {"status":"resume"}` → idle;
  next turn ran clean on a keyed server (2.06s, billed, breaker reset to 0).
- **SSE terminal transitions — DF-56 VERIFIED FIXED.** `/api/v1/events` stream for a
  fresh session now carries `thinking` (iteration 1) AND `idle` events; prior runs
  (Oct-16) ended at thinking forever.
- **Approval loop end-to-end (when seeded).** Gated tool row + pending tool_request →
  `awaiting_approval` within one 500ms executor poll with a linked approval_request;
  `GET /api/v1/approvals` lists it; `GET /{id}` returns it; `POST /{id}/review`
  `{"decision":"approved"}` → 200, status approved; executor then executed the
  request (failed only because the seeded handler was a dummy — the gate mechanics
  all work).
- **Occupied-port diagnostic — excellent new UX.** A second serve on an occupied port
  now refuses with a class-consensus probe ("port 127.0.0.1:18777 is occupied by a
  Consensus server … Start on a free port instead"), names `ss -tlnp`, and refuses to
  kill. Caught a real collision: host demo process `imhotep-demo` (pid 7506) holds
  127.0.0.1:8095, so pprof WARNs "address already in use" — DF-58's second symptom,
  live and host-verified.
- **GET /config is proper JSON** (DF-54 stays fixed): `{"system_settings":{"embedding_model":"text-embedding-3-small"}}`, no raw Go map strings.
- **Message dedupe working as designed** (DF-9): byte-identical resend inside the
  5s window dropped with a log line; unique content delivers normally.

## What broke (rows filed)

1. **DF-CONSENSUS-57-R2 (P2)** — origin/master (92050ad) lags main again; GET /api/pty
   is unpushed. Fresh-clone probe: clone_HEAD=92050ad → `/api/pty` answers 404 with a
   valid admin key; same probe on a binary from main answers 200 `{location, data:[]}`.
   Master DOES get board-only commits, so the push path works — the gap is specifically
   code commits landing on main without the master push. Same class as closed DF-57.
2. **DF-CONSENSUS-60 (P2)** — docs say review body `{"decision":"approve"}`/`"reject"`;
   the endpoint validates `approved|rejected|modified` and 400s the documented bodies
   (`decision must be: approved, rejected, or modified`).
3. **DF-CONSENSUS-61 (P2)** — approvals API serializes timestamps as zero-time:
   GET returns `created_at`/`reviewed_at` = `0001-01-01T00:00:00Z` and `expires_at` null
   while the sqlite row carries correct values (created 02:41:20, expires +1h,
   reviewed 02:41:23, reviewer admin). Expiry is invisible to API clients.
4. **DF-CONSENSUS-62 (P3)** — the approval gate has no user on-ramp: tools_registry
   ships 0 rows on a fresh install (no seed migration; `consensus tool` is
   inspect-only; no REST create route; docs document only GET /tools and
   POST /tools/{name}/execute). The docs' own example `POST /tools/query/execute`
   400s with `session_id is required` (body field undocumented, tool doesn't exist).
   This run could only exercise the gate via direct SQL inserts.

## Re-proofs on HEAD

- **DF-58 premise live:** keyless restart → startup WARN, then the first turn burned
  two LLM 401 iterations (server ERROR lines with provider auth text) and paused the
  session at iteration 3. Fail-closed works; the restart-time env dependency remains
  invisible at turn time. Row stays pending; still valid.
- **DOGFOOD-111 override WARN re-proven:** literal config key + env var →
  "LLM API key: using DEEPSEEK_API_KEY from environment (overrides config file value)".
- **DF-53/54 stay fixed** (config mcp flatten not re-driven this tick; /config JSON shape re-verified only).

## Run-integrity lesson (for future ticks)

My keyless restart silently failed to bind ("port occupied") and the leg continued
against the PREVIOUS (keyless) server. Two conclusions were contaminated
("resume doesn't clear the breaker"; "session wedged at booting") until the A/B was
re-run on a verified-keyed instance. Rule: after EVERY server restart inside a leg,
assert pid alive + key-probe line in the fresh log + bind-fail grep == 0 before
sending probes. The 15ms "turn" that returns nothing is the tell: real turns never
finish that fast; check what actually answered.

## Install leg (fresh user, clean box)

PASSED on bunker-mvp agent dfsync-df1020: clone 1s (@ 92050ad), go1.26.5 preinstalled,
build 89s, init 1s (first_admin_key minted), serve + health 200 (version 0.1.0) +
Chronicle 200 (312KB), keyless boot WARN printed as documented, pprof bound clean on
the empty box. Agent destroyed after evidence capture.

## Perf (Step 2b)

cold turn 2.09s, warm 2.06s, ack 35ms, approval poll <500ms — nothing a user would
feel; no PERF row (skill law: a win nobody can feel is not a finding).

## Board

Rows appended to `.coding-hermes/board/tasks.jsonl`: DF-CONSENSUS-57-R2, -60, -61, -62
(ids minted via board_next_id.py; append pre/post census verified, 441→445 lines;
boardctl validate OK). ch:trace row=DF-CONSENSUS-57-R2 (campaign row = this run's log
entry in .coding-hermes/dogfood-log.md, evidence=/tmp/df-oct20/*.sh outputs cited in-row).
