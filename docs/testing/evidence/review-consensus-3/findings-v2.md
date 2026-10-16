# REVIEW-CONSENSUS-3 — Postgres load probe v2: findings

Run date: 2026-10-16. Worktree `~/worktrees/consensus-REVIEW-CONSENSUS-3`
(branch `wt/REVIEW-CONSENSUS-3`, base `cb161ba` = `origin/main`), binary built from
this changeset.

**This file supersedes `loadprobe-postgres.json`** (the 2026-10-15 blocked run).
That file is kept as the historical record; the v2 run below replaces its results.
Raw probe output: `loadprobe-postgres-v2.json` (all four arms, verbatim, plus the
commands/config/provenance), with the per-arm files
`loadprobe-postgres-v2-arm{A,B,M,N}-*.json`.

## What this run measures

A local OpenAI-compatible stub (`docs/testing/evidence/review-consensus-8/raw/mock-llm.py`,
copied verbatim to `/tmp/rc3/mock-llm.py`, listening on `127.0.0.1:18123`) is wired into
the server as `llm.base_url`. The stub answers instantly and never leaves the host, so:

* every number below measures the **Consensus DB + API write path** —
  auth lookup, rate-limit accounting, `memory_events` INSERT, `sessions` UPDATE — and
* **NOT** LLM latency, provider availability, or token throughput.

Stub config (mirrors the shipped `consensus.yaml` llm section):

```yaml
llm:
  default_model: deepseek-v4-flash
  provider: openai
  base_url: http://127.0.0.1:18123/v1
  api_key: "rc3-stub-key-never-used"
  max_context_tokens: 128000
  max_output_tokens: 16384
```

## Provenance

| item | value |
|---|---|
| server binary | `/tmp/consensus-rc3wt`, `go build -o /tmp/consensus-rc3wt ./cmd/consensus` |
| binary sha256 | `eeaf986b50072fee648eaa083d912cc9a6f66317130e2afa35f1e421e0e1d4ee` |
| reproducibility | byte-identical sha256 to a fresh rebuild of the same tree (so the run is attributable to exactly this source; the binary embeds migration 029) |
| probe binary | `/tmp/loadprobe-rc3` — `scripts/loadprobe/` was **not** modified |
| database | `pgvector/pgvector:pg16`, one fresh container + fresh database per arm, `init` before `serve` (init captures the admin key) |

Exact commands:

```bash
docker run -d --rm --name consensus-pg-rc3<m> -e POSTGRES_USER=consensus \
  -e POSTGRES_PASSWORD=rc3pass -e POSTGRES_DB=consensus -p <port>:5432 pgvector/pgvector:pg16
cd ~/worktrees/consensus-REVIEW-CONSENSUS-3
/tmp/consensus-rc3wt init  --config /tmp/rc3/consensus-rc3<m>.yaml    # migrations + admin key
/tmp/consensus-rc3wt serve --config /tmp/rc3/consensus-rc3<m>.yaml    # port 18091
/tmp/loadprobe-rc3 --base-url http://127.0.0.1:18091 --admin-key "$ADMIN_KEY" \
  --concurrency 1,4,16 --turns 20 --backend postgres --out <arm>.json
```

## Results

| arm | db modifications | c | sessions | turns ok/total | error rate | throughput | p50 / p95 |
|---|---|---|---|---|---|---|---|
| **A — unmodified main** | none | 1 / 4 / 16 | **0 / 0 / 0** | 0/0 | 1.0 | 0 | — |
| B — RLS bypass only | `agent_role BYPASSRLS` | 1 | 1 | 0/20 | 1.0 | 195.1* | 3 / 6 ms |
| B | | 4 | 4 | 0/80 | 1.0 | 610.3* | 4 / 17 ms |
| B | | 16 | 16 | 0/320 | 1.0 | 635.3* | 16 / 80 ms |
| **M — measured, pool=5** | + sequence grant | 1 | 1 | 20/20 | 0 | 83.6 | 11 / 18 ms |
| M | | 4 | 4 | 80/80 | 0 | 131.2 | 24 / 57 ms |
| M | | 16 | 16 | 238/320 | 0.256 | 1.77 | 89 / 30003 ms |
| **N — measured, pool=10** | + sequence grant | 1 | 1 | 20/20 | 0 | 76.2 | 9 / 36 ms |
| N | | 4 | 4 | 80/80 | 0 | 130.8 | 20 / 63 ms |
| N | | 16 | 16 | 320/320 | 0 | 167.9 | 91 / 120 ms |

\* armB throughput is **error throughput** (each rejected request returns in a few ms) —
it is not a good number and must not be read as one.

### Headline

1. **On unmodified main the probe still measures nothing**: every
   `POST /api/v1/sessions` is rejected by the `sessions` RLS policy (`SQLSTATE 42501`),
   exactly as in the 2026-10-15 run. Finding 1 is **not** fixed (see below).
2. With the two database-level blockers neutered, the write path is healthy at
   concurrency 1 and 4 (≈76–131 turns/s, p95 ≤ 63 ms).
3. At concurrency 16 the shipped pool size decides the outcome: **5 connections
   collapses** (82 of 320 turns hit the probe's 30 s client timeout; p95 = 30003 ms;
   1.77 turns/s) while **10 connections holds** (320/320, p95 120 ms, 167.9 turns/s).

## Defect status

### Finding 3 — UUID columns not normalized (FIXED in this changeset)

`internal/db/postgres/postgres.go`: `normalizeValue` now converts `[16]byte`
(pgx's `UUIDCodec.DecodeValue` shape), `pgtype.UUID` and `google/uuid.UUID` to the
canonical 8-4-4-4-12 string.

Evidence:

* unit + live regression tests in `internal/db/postgres/postgres_test.go`
  (`TestNormalizeValueUUIDToCanonicalString`, `TestPostgresUUIDNormalization`);
  the live test was executed against a real Postgres
  (`CONSENSUS_TEST_POSTGRES_URL=…:15439/consensus`) — PASS.
* RED proof: reverting only `postgres.go` and re-running the new unit test fails on 4
  subtests, quoting the production shape (`normalizeValue([16]uint8 [63 37 4 …])`).
* Live in this run: session-scoped keys authenticated and matched their own sessions.
  In armB (where the RLS wall was bypassed but the sequence grant was not) all 420
  turn requests passed the `scope == "session" && sessionID != id` authorization check
  and failed later, at the `memory_events` INSERT — i.e. the session id read back out
  of `api_keys` now equals the id in the URL. Pre-fix that check returned 403
  `FORBIDDEN: session key can only message its own session`.

### Finding 2 — `sessions.model_id` FK rejects the "default" sentinel (FIXED in this changeset)

`internal/migrate/migrations/029_seed_default_chat_model.sql` seeds the shipped
`llm.default_model` (`deepseek-v4-flash`, tier 2, tag `chat`) into `model_registry`
with `ON CONFLICT (model_id) DO NOTHING`, so a fresh database has a valid FK target and
`CreateSession`'s "cheapest enabled non-embedding model" query returns a real model id.
`internal/api/service.go` was **not** touched (the migration alone clears the FK).

Evidence:

* `internal/migrate/seed_default_chat_model_test.go` — backend-agnostic SQLite test
  (seed row present, chat-capable, is what the API's own selection query returns, and a
  re-apply does not overwrite an existing registration) and a live Postgres test
  (row present after migrations, session INSERT with it satisfies the FK, and the
  **negative control** `model_id='default'` still fails with
  `fk_sessions_model` SQLSTATE 23503 — so it is the seed, not a dropped constraint,
  that unblocks creation).
* Both were executed: SQLite PASS, live Postgres PASS
  (`✓ sentinel still rejected: … violates foreign key constraint "fk_sessions_model"`).
* RED proof: moving the 029 file aside makes the SQLite test fail with
  *"model_registry has no "deepseek-v4-flash" row after migrations"*.
* Live: on the arm M/N databases the session rows created by the probe carry
  `model_id='deepseek-v4-flash'` (selected via migration 029, not the sentinel).

### Finding 1 — RLS wall on session creation (STILL LIVE — the premise of this row is wrong)

The row states Defect 1 is fixed on main (`36e6485` + `c5d6447`) and "verified live: …
now gets PAST session creation (fails only on MISSING_LLM_CONFIG)". Both commits are
real, but neither touches the `sessions` INSERT path: `36e6485` moved the **auth key
lookup** and **rate-limit accounting** to the admin pool, and `c5d6447` added
`missing_ok` to `current_setting(...)` (the 42704 error class). `POST /api/v1/sessions`
still inserts through `s.svc.Sessions.CreateSession` → `svc.db` = the pool whose
`AfterConnect` runs `SET ROLE agent_role`, and the policy is still

```sql
CREATE POLICY session_isolate_sessions ON sessions
    FOR ALL USING (id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);
```

which PostgreSQL uses as the INSERT check (`UNSET GUC → NULL predicate → rejected`).
The earlier verification stopped at the `MISSING_LLM_CONFIG` gate, which returns
**before** any DB write — so it never reached the INSERT.

Live evidence (both collected today, on current main):

```
$ docker exec consensus-pg-rc3 psql -U consensus -d consensus \
    -c "SET ROLE agent_role; INSERT INTO sessions (id, agent_name, model_id, status)
        VALUES ('00000000-…-ff','rls-probe','deepseek-v4-flash','idle');"
ERROR:  new row violates row-level security policy for table "sessions"

probe arm A (unmodified):  POST /api/v1/sessions → 400
  {"error":{"code":"INVALID_REQUEST","message":"failed to create session: postgres:
   exec: ERROR: new row violates row-level security policy for table \"sessions\"
   (SQLSTATE 42501)"}}
```

Fix direction is unchanged from the original findings: give the API a pool that does
not `SET ROLE agent_role` (extend the existing `admindb` pattern to the session-scoped
service layer), or establish `consensus.session_id` per request. That is a design
decision with a security-model consequence, so it was **not** made unilaterally here.

### Finding 4 — NEW: `agent_role` has no USAGE on ANY sequence (blocks every turn)

With Finding 1 bypassed, session creation succeeds and **every turn** fails:

```
ERROR api: failed to insert memory event
  error="postgres: exec: ERROR: permission denied for sequence memory_events_id_seq (SQLSTATE 42501)"
POST /api/v1/sessions/{id}/message → 500 INTERNAL_ERROR   (20/20, 80/80, 320/320 turns)
```

`POST /api/v1/sessions/{id}/message` inserts into `memory_events`, whose id comes from
the `memory_events_id_seq` sequence owned by the table owner. Migration 028
(`PG-GRANT-42501`) censused the SQL the non-admin pool executes and granted **table**
verbs — sequences were missed. Pre-state, measured on the fresh arm-M database:

```
$ SELECT sequence_name, has_sequence_privilege('agent_role', sequence_name, 'USAGE')
  FROM information_schema.sequences WHERE sequence_schema='public';
agent_billing_id_seq|f    agent_messages_id_seq|f   audit_logs_id_seq|f
compression_queue_id_seq|f  external_events_id_seq|f  external_quarantine_id_seq|f
iteration_commits_iteration_id_seq|f  memory_events_id_seq|f  memory_pages_id_seq|f
model_registry_id_seq|f   notification_log_id_seq|f  staging_buffer_id_seq|f
tool_requests_id_seq|f    tool_results_id_seq|f
```

All 14 sequences read `f` (no USAGE, no SELECT). Suggested fix: extend the 028 census
to sequences (`GRANT USAGE, SELECT ON SEQUENCE … TO agent_role`) in a new migration —
this is the same defect class as `PG-GRANT-42501`, not a new design question.

### Finding 5 — NEW: harness planning fails on Postgres (`invalid input syntax for type uuid: ""`)

Exposed once the harness actually runs on Postgres:

```
ERROR harness: planning failed session_id=9382dadb-… error="planning: initial context:
  read session: postgres tx: query: ERROR: invalid input syntax for type uuid: ""
  (SQLSTATE 22P02)"
```

Root cause (reproduced directly against the live database):

```
$ SELECT COALESCE(parent_id, '') FROM sessions LIMIT 1;
ERROR:  invalid input syntax for type uuid: ""
$ SELECT COALESCE(parent_id::text, '') FROM sessions LIMIT 1;
(1 row, NULL)
```

`internal/harness/context.go:511` (`readSessionTx`) uses `COALESCE(parent_id, '')` on a
`UUID` column; the `''` literal is coerced to `uuid` at parse time, so the query fails
for every session on Postgres regardless of whether `parent_id` is NULL. SQLite is
unaffected (no uuid type). 27 such failures were logged during the measured run; the
fix shape is `COALESCE(parent_id::text, '')`. It is independent of findings 2–4 and of
this changeset's two fixes.

### Finding 6 — NEW: the shipped 5-connection pool saturates at 16 concurrent sessions

armM (`database.max_open_conns: 5`, i.e. the shipped `consensus.yaml` value) vs armN
(`10`, the fallback `internal/db/postgres/postgres.go` applies when the config is
unset) — same image, fresh DB each, same two DB modifications:

| concurrency | pool 5 | pool 10 |
|---|---|---|
| 1 | 20/20 ok, 83.6 turns/s, p95 18 ms | 20/20 ok, 76.2 turns/s, p95 36 ms |
| 4 | 80/80 ok, 131.2 turns/s, p95 57 ms | 80/80 ok, 130.8 turns/s, p95 63 ms |
| 16 | **238/320 ok, 82 × 30 s timeouts, 1.77 turns/s, p95 30003 ms** | **320/320 ok, 167.9 turns/s, p95 120 ms** |

The 82 failed turns are client-side timeouts (`errors_by_status: {"0": 82}` — the
probe's 30 s `requestTimeout`); the server log shows the matching in-flight
cancellations (`api: failed to insert memory event … context canceled`, `dedupe probe
… context canceled`, `read context: begin tx … context deadline exceeded`), i.e. the
requests were queued behind a saturated pool, not rejected by the DB. No deadlock, no
lock timeout, and no permission error appears in the pool-5 run. Causation is
therefore attributed to pool sizing rather than to a DB-level defect — with the caveat
that the harness was also erroring concurrently (Finding 5) and competing for the same
connections, so the armM concurrency-16 numbers are a **lower bound** for the shipped
configuration, not a clean profile.

## What was and was not done

**Done**: fixed Finding 3 (driver UUID normalization, with unit + live regression tests
and a RED proof); fixed Finding 2 (migration 029 seed + tests, SQLite and live Postgres,
with the FK negative control); ran the probe on Postgres at four arms and captured real
numbers; documented two new defects the run exposed (Findings 4 and 5) and a
configuration finding (6), each with a live reproduction.

**Not done**: no product file outside the review's scope was changed. In particular the
`internal/harness` `COALESCE` bug (Finding 5) and the sequence grants (Finding 4) are
reported, not fixed — they are new findings that need their own rows. The two database
modifications used to obtain the measured numbers are **scratch-container only**
(`ALTER ROLE agent_role BYPASSRLS; GRANT USAGE, SELECT ON SEQUENCE memory_events_id_seq
TO agent_role;`) and are disclosed in the evidence JSON; the probe harness itself was
not tuned.
