# Dogfood — Provider contracts, SSE truth, crash recovery (2026-10-16)

Run: consensus-dogfood tick 2026-10-16-16-58-40 · build **4c171c7** (local master at run time) ·
scratch SQLite instance `:18977` · real DeepSeek key (DEEPSEEK_FOREMAN_API_KEY, probed live first:
`GET /v1/models` 200 → `deepseek-flash`, `deepseek-v4-pro`).

Angle: everything the Oct-13 runs did not touch and the Oct-15 delta shipped —
the **provider-facing contracts** (spec 024 §B2 thinking-mode/sampling, §D streaming),
the **new `/api/*` REST families** (ROUTE-ADD-006/009/011/014-026), the **native v1
event stream** (`GET /api/v1/events`, ROUTE-ADD-009), re-proofs of closed fixes
DF-CONSENSUS-53/54 on the new HEAD, and the README's **crash-recovery promise**
(`kill -9`) — last driven in August.

## What a real user flow looks like (verified recipe, 2026-10-16)

1. Build: `go build -o bin/consensus ./cmd/consensus/` (86s cold on a fresh bunker box,
   3s warm locally; go1.26.5).
2. `bin/consensus init --config consensus.yaml` — one-time admin key
   (`cs_ak_…`, 70 chars) printed once; also in serve log on first boot.
3. `bin/consensus serve --config consensus.yaml` — health 200 in ~3s;
   `GET /api/v1/health` carries `db_tables:38, schema_version:30`.
4. Create a session: `POST /api/v1/sessions {"agent_name","model_id":"deepseek-v4-flash","goal"}`.
5. Turn: `POST /api/v1/sessions/{id}/message {"type":"user_instruction","content":…}`
   → ack `{"status":"message_received"}` in ~40ms → harness runs the turn
   (thinking-mode DeepSeek call) → idle in **4.1s cold / 4.1s warm / 2.6-5.2s via shim**.
6. Read the reply: `GET /api/v1/sessions/{id}/memory` → entries typed
   `user_message` / `text_block` (the monologue + the answer; billing entry
   recorded per turn with `model_id: deepseek-flash` — the provider's served
   alias for the pinned `deepseek-v4-flash`).

The shim's typed surface also works as a synchronous API:
`POST /session/{id}/message {"parts":[{"type":"text","text":…}]}` returns the
finished reply **in the POST response** (4.5s) — but see SSE gap below: the
event stream does not mirror that completion.

## Provider contract findings (spec 024 §B2, live)

- Pinned model id `deepseek-v4-flash` still works (provider alias → served as
  `deepseek-flash`); `max_tokens:10` thinking-default probe returned empty
  content with `reasoning_content` populated — matching the documented
  default-on thinking behavior. Nothing to file.
- **Not live-provable**: the Anthropic-format streaming path (SHIM-SUPPORT-004)
  needs an Anthropic-key provider; none exists in this environment. The
  conformance suite covers it (`internal/llm` tests); a real-transport drive
  remains open.
- Sampling contract (temperature/top_p drop rules) is observable only through
  provider-side request inspection; no user-facing REST knob exposes thinking
  or temperature on session creation — out of scope for a REST dogfood run.

## Findings filed (board rows)

- **DF-CONSENSUS-56 (P1)** — `GET /api/v1/events` never publishes the terminal
  transition. Six captures (concurrent readers included) all end at
  `session_update {status:"thinking"}` while REST shows `idle` seconds later;
  harness `paused` transitions are equally unpublished. Pinned:
  `internal/harness/executor.go:822` applies `NextStatus` by SQL only, no
  `PublishSessionUpdate` (all publish sites live in `internal/api`). An
  event-driven client waits forever.
- **DF-CONSENSUS-57 (P1)** — origin/master lags local master 7+ commits
  (`c9b7747` vs `4c171c7`): a fresh user's clone gets none of the Oct-15
  thinking-mode/streaming work and migrations 029/030 (fresh clone showed
  schema_version 28 vs 30).
- **DF-CONSENSUS-58 (P2)** — restart-without-key-env trap: config's
  `${DEEPSEEK_API_KEY}` interpolates empty, startup only WARNs, the harness
  then burns 3 auth-fail iterations and pauses the session; plus a fixed
  pprof `127.0.0.1:8095` collision on any second instance on one host.
  The fail-closed pause itself is correct and the documented
  `PATCH {"status":"resume"}` escape works.

## Re-proofs (closed rows, current HEAD)

- **DF-CONSENSUS-53** (mcp-block flatten): `PATCH /config {"mcp":{"dfprobe16":…}}`
  → 200, stored flattened under `settings.mcp.dfprobe16`; `POST /mcp/dfprobe16/auth`
  resolves the name immediately (typed 400 `McpUnsupportedOAuthError` — honest,
  the runtime mints no OAuth URLs). No literal `"mcp"` key remains. FIXED.
- **DF-CONSENSUS-54** (config serialization): `GET /config` returns native JSON
  settings (`.settings.mcp` = a real object, jq-round-trips). FIXED.

## New surface first drive (all conformant)

`GET /api/agent` (populated catalog), `/api/command`, `/api/health`,
`/api/model`, `/api/skill`, `/api/reference`, `/api/provider`,
`/api/integration` (truthful empty list), `/api/integration/df-nonexistent`
(200 empty-truthful per contract), `GET /api/fs/list` (real directory listing),
`PATCH /api/credential/df-c1` (typed 400 `InvalidRequestError` naming the
missing required field `label`), `DELETE /api/credential/df-c1` (204 no-op per
the declared no-store narrowing). `GET /api/event` (v2) refuses honestly with
a typed error explaining the v2 event store does not exist and naming the v1
stream — the ROUTE-ADD-009 narrowing behaves exactly as declared.

## Crash recovery (README promise, re-proven)

Two `kill -9` cycles. After each: restart → health 200 → same session, same
12 memory entries, same billing rows, admin key still valid (stored hashed).
Restart-without-key produced the fail-closed pause (see DF-CONSENSUS-58) and
resume restored it. No corruption, no stranded state.

## Perf (Step 2b)

Headline op (create session + one turn, real LLM): **cold 4.1s / warm 4.1s**,
ack 39ms; shim synchronous turns 2.6-5.2s across 4 samples. Nothing slow
enough that a user would notice; no profile taken, no PERF row (skill law:
a win nobody can feel is not a finding). Sibling run's PERF-CONS-13 covers the
interactive-session envelope on the same day.

## Install leg (bunker)

PASSED on fresh agent `dfsync-df` @ bunker-mvp (las-03 unreachable 6th
consecutive run): clone 1s (c9b7747 — the origin lag, DF-CONSENSUS-57),
go1.26.5 preinstalled, build 86s, init 2s, serve + health 200 + Chronicle 200,
documented env-var path re-proven fixed (old DF-CONSENSUS-10 wedge). Agent
destroyed after the run. Total install_seconds ≈ 89 (toolchain present).
