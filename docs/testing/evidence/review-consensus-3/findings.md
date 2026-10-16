# REVIEW-CONSENSUS-3 — Postgres load probe: findings

Run date: 2026-10-15. Probe: `scripts/loadprobe/` (commit in this changeset).
Server: `bin/consensus serve` built from `a9a10c2` against a fresh
`pgvector/pgvector:pg16` container (`postgres://consensus:consensus@127.0.0.1:15439/consensus`).

## Result

The probe compiles, vets, and runs, but **the Consensus REST API cannot create
a session against a fresh Postgres backend**. Every `POST /api/v1/sessions`
fails, so zero sessions exist and the probe records `error_rate: 1` at every
concurrency level (raw output: `loadprobe-postgres.json`).

A SQLite control run (`loadprobe-sqlite-control.json`) shows the same probe
producing real metrics on a healthy backend (concurrency 1: 20/20 turns, 45
turns/s, p95 49 ms), which isolates the failure to the Postgres path — not the
probe.

Three independent defects block the Postgres API path. They are listed in
dependency order: fixing only #1 exposes #2, fixing #1 and #2 exposes #3.

---

## Finding 1 — RLS policy blocks session creation (the hard ceiling)

**Evidence**

```
POST /api/v1/sessions (admin key) → HTTP 400
{"error":{"code":"INVALID_REQUEST","message":"failed to create session: postgres: exec: ERROR: new row violates row-level security policy for table \"sessions\" (SQLSTATE 42501)"}}
```

`GET /api/v1/sessions/{id}` and `POST /api/v1/sessions/{id}/message` with a
valid admin key return `404 session not found` even for a session row that
exists (verified by inserting the row directly through the admin pool): the
agent pool cannot see any session rows.

**Root cause**

The REST API runs all session-scoped SQL through the *agent* pool, whose
`AfterConnect` hook does `SET ROLE agent_role`
(`internal/db/postgres/postgres.go`). Every RLS policy reads
`current_setting('consensus.session_id', true)`, which the API layer never
sets — only the harness sets it with `SET LOCAL` inside its transactions
(`internal/harness/*.go` → `Tx.SetSessionContext`).

The `sessions` policy (`internal/migrate/migrations/027_..._missing_ok.sql`):

```sql
CREATE POLICY session_isolate_sessions ON sessions
    FOR ALL USING (id = NULLIF(current_setting('consensus.session_id', true), '')::UUID);
```

is `FOR ALL` with no `WITH CHECK`, so for INSERT the `USING` expression is the
check. On an API connection the GUC is unset, `NULLIF(NULL,'')::UUID` is
`NULL`, `id = NULL` is `NULL`, and the INSERT is rejected with 42501. Reads
fail closed the same way (the SELECT filters every row → 404).

This is an architecture decision, not a typo: the API and the harness share
the agent pool, but only the harness establishes the RLS session identity.
The API does its own auth/authz at the application layer (Bearer key → scope →
session_id) but then hits a DB-level RLS wall it has no way to satisfy.

**Recommended fix direction (needs a design decision, not done here)**

Either (a) give the API a pool that does not `SET ROLE agent_role` (it already
uses `admindb` for auth/rate-limit/health — extend that pattern to the
session-scoped handlers and the service layer), or (b) establish
`consensus.session_id` per request. (a) matches the codebase's own "admindb
bypasses RLS for queries that must succeed" convention and the
`alt_mode_role BYPASSRLS` intent in migration 001. A worker should not pick
between these unilaterally.

---

## Finding 2 — `sessions.model_id` FK rejects the "default" fallback

**Evidence**

With Finding 1 bypassed (session row inserted through the admin pool), an
insert with the fallback model id fails:

```
ERROR: insert or update on table "sessions" violates foreign key constraint "fk_sessions_model"
DETAIL: Key (model_id)=(default) is not present in table "model_registry".
```

**Root cause**

`sessions.model_id` is `NOT NULL` with `FK → model_registry(model_id)`
(migration 001). On a fresh database `model_registry` is empty except for
`text-embedding-3-small` (seeded by migration 016). `CreateSession`
(`internal/api/service.go`) selects a chat-capable model, excludes embedding
models, and — finding none — falls back to `modelID = "default"`, which is
never registered. `RegisterIfMissing` is only called for non-"default" model
ids, so the sentinel always violates the FK on a fresh Postgres database.

On SQLite this never surfaces because the SQLite driver does not enforce the
FK (and the pg→SQLite migration filter drops Postgres-only constraints).

**Recommended fix** (needs a decision): seed a default chat model in
`model_registry`, or make `model_id` nullable / drop the FK, or have the
modelsync syncer register the "default" sentinel. Not done here.

---

## Finding 3 — Postgres UUID columns are not normalized to strings

**Evidence**

A session + session-scoped key inserted directly through the admin pool, then
messaged with that key:

```
POST /api/v1/sessions/{id}/message (session key for {id}) → HTTP 403
{"error":{"code":"FORBIDDEN","message":"session key can only message its own session"}}
```

The key's `session_id` and the URL path id are the same UUID, so the check
`scope == "session" && sessionID != id` should pass; it fails because the auth
middleware's extracted `session_id` is empty.

**Root cause**

`pgx` v5 decodes a `uuid` column as `[16]byte` (`UUIDCodec.DecodeValue`
returns `uuid.Bytes`, a `[16]byte`). `normalizeValue`
(`internal/db/postgres/postgres.go`) only converts the `[]byte` slice type to
string; a `[16]byte` array falls through unchanged. `toString` then fails the
`.(string)` assertion and returns `""`.

This affects every UUID column read through the Postgres driver and any
Go-side UUID comparison or formatting, not just this one check (memory event
ids, task ids, etc.).

**Recommended fix** (small, clearly correct — left for the fix row to avoid an
unverified change): in `normalizeValue`, convert `[16]byte` (and
`pgtype.UUID` if seen) to the canonical UUID string via `github.com/google/uuid`
(already a dependency), plus a regression test.

---

## What was and was not done

- **Done**: wrote and committed the probe (`scripts/loadprobe/`), ran it
  against Postgres (raw blocker evidence) and SQLite (control), and documented
  the three defects above.
- **Not done**: no main-service code was modified. Findings 1 and 2 need a
  design decision (which pool / which seeding strategy); Finding 3 is a safe
  one-line fix but was left unapplied so it is not committed unverified. All
  three should become board rows with owners.
