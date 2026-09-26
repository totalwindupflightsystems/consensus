# Consensus Dogfood — 2026-09-26 — Fix-verification + CLI surface (3rd angle)

**Verdict: SHIPPABLE (first time).** The three P1 fixes dogfooded on 09-25
(DF-CONSENSUS-27/28/29) are live-verified FIXED through real use, the CLI
management surface works end-to-end for inspection, and the only new findings
are P2-class CLI polish. No P0/P1 surfaced in this run.

## Why this angle

Two runs on 09-25 covered REST+billing+isolation (morning) and MCP (afternoon).
The 2026-09-25 22:26 wave merged fixes for all three P1s; a real user's first
question the next day is "do the fixes actually work for me?" The CLI surface
(`consensus session/memory/tool/models/status`) had never been driven end-to-end.

## Setup

- Control host, scratch instance `/tmp/dogfood-consensus` (config-file path,
  max_open_conns: 4 — the documented safe recipe), port 18124, binary rebuilt
  from HEAD 87340a0. DeepSeek key pre-probed live (ENV-CONSENSUS-1 practice).
- `consensus init` prints the bootstrap admin key once (cs_ak_…, expires
  2026-12-25); serve healthy in ~4s.

## Fix verification (the heart of this run)

### DF-CONSENSUS-29 (key mint) — FIXED
`POST /api/v1/auth/keys {"scope":"session","session_id":"<sid>"}` returns a REAL
secret (`cs_sk_` + 62 hex, len 70) — no more `cs_sk_...NNN` placeholder. The
minted key authenticated the session create + message flow. Note: the request
field is `scope` (singular), NOT `scopes` — the served OpenAPI schema is the
contract; a caller using `scopes` gets a clear 400 `INVALID_REQUEST`.

### DF-CONSENSUS-27 (billing ledger) — FIXED
After one real conversational turn (POST /sessions/{id}/message +
{"type":"user_instruction",...}): session tokens_used_in/out = 18202/5707,
`agent_billing` has 10 rows (per-iteration, per-category, cost_usd ~0.012/row),
`GET /sessions/{id}/billing` returns the entries with totals. The cost circuit
breaker/budget guards finally have something to count.

### DF-CONSENSUS-28 (DELETE) — FIXED (and correctly a SOFT delete)
DELETE returns 200; afterwards: GET returns 404, list omits the session,
message returns 410 GONE, PATCH 404 (per commit 3222cae). The raw sessions row
survives with `deleted_at` tombstone — that is the pinned spec (specs/015-api-and-mcp.md),
not a regression: idempotent re-DELETE returns 200 again. Verified all four surfaces.

## CLI surface (first real use)

Works: `status`, `session list/show/logs/cost`, `memory list <sid>` (positional
session id), `tool list`, `models --help`, `session create` (returns session +
session-scoped key in JSON). Output is clean JSON (`--format json` default on
show/create) — good for scripting.

Findings (filed as rows):
- **DF-CONSENSUS-33** — `CONSENSUS_API_KEY` env documented in flag help but
  never read ("UNAUTHENTICATED: missing API key" with a valid key exported);
  auth failures exit 0.
- **DF-CONSENSUS-34** — no `session message` subcommand: a CLI-only user's
  `session create --goal …` session sits in `booting` forever (a goal alone
  does not wake the harness; woke it via REST, then it ran to completion).

## Install leg (ephemeral bunker)

bunker-las-03 DOWN again (3rd consecutive run; DF-CONSENSUS-32 open).
bunker-las-01/04 down, las-02 bunkerd activating. Substituted **bunker-mvp**
(reachable, bunkerd active):
- anonymous clone of totalwindupflightsystems/consensus @ 87340a0 — 4.3s
- go 1.22.2 on the agent → auto toolchain download to go1.26.5
- `go build -o bin/consensus ./cmd/consensus/` — **INSTALL_SECONDS=91**
  (includes module downloads; the repo's go.mod toolchain line does the work)
- `init` OK (key printed once), serve → health 200 in ~3s
  (db sqlite, 37 tables, schema_version 24)
- agent destroyed (CLI destroy hit deadline_exceeded ×2 → manual root cleanup;
  noted in DF-CONSENSUS-35 for the bunker project)

## Performance (Step 2b)

Timed turns, scratch SQLite, real DeepSeek calls:
- COLD (create → wake → first reply): **78.2s** — includes boot + 2 planning
  iterations (the agent's multi-iteration design, each LLM call 1.2–3.9s per
  serve.log elapsed_ms; not a defect, but new users should know a "turn" is a
  planning loop, not one chat completion).
- WARM (turn 2 on idle session): **19.2s**.
- PERF-CONSENSUS-11's minute-long dispatch stalls did NOT reproduce (~30 turns
  observed across the run, worst case the 78s cold path above).
- Verdict: nothing here is slow enough that a user would call the system broken
  once they expect the multi-iteration shape; no profile taken (nothing
  justified one). A cold-start expectation note belongs in docs, not a fix.

## Verdict rationale

The 09-25 verdicts were PROMISING-BUT-ROUGH on the strength of three P1s
(billing dead, delete lying, keys dead-on-arrival). All three now verify fixed
in real use; the CLI works for every management verb and its gaps are P2 UX
(classic polish), and install-from-zero is proven again on a second distinct
host. That is SHIPPABLE for the REST runtime with honest caveats: the CLI
cannot yet complete an agent conversation (DF-34), and the las-03 bunker host
needs remediation (DF-35).
