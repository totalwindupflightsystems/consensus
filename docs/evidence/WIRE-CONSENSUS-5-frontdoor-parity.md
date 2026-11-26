# WIRE-CONSENSUS-5 — Route-to-front-door parity evidence

Board row: WIRE-CONSENSUS-5 ([P2] Route-to-front-door parity).
Guard: `TestFrontDoorParityRealBinary` in `internal/api/openapi_frontdoor_test.go`
(run with `go test ./internal/api/ -run TestFrontDoorParityRealBinary -count=1 -v`;
skipped in `-short` like the other real-binary integration test,
`TestDF65ToolCallHandoffRealBinary`).

## What the guard does

1. Builds `./cmd/consensus` from the working tree.
2. `consensus init --db-url sqlite://<scratch>` migrates a throwaway SQLite
   database offline and mints the bootstrap admin key the probes authenticate
   with.
3. Seeds one real `sessions` row and one real `approval_requests` row
   (`status='pending'`, real 008 columns) so the read paths and the T1-D4/D5
   reprobes have a row to address.
4. Boots the REAL binary on a free loopback port (hermetic fake
   OpenAI-compatible upstream, no network egress, no API keys).
5. Fetches `GET /openapi.json` from the running server and takes the declared
   operation set from what the binary itself serves — never from `specs/`.
6. Exercises every declared operation with one real HTTP request (admin
   bearer, minimal `{}` body for writes). An operation counts as exercised
   when a handler answers — any status except 405 or chi's bare
   `404 page not found` (a mux miss). This distinguishes a declared 501 stub
   from an unmounted route, which static route-list comparisons cannot do.
7. Enforces the committed exclusion ledger
   `internal/api/testdata/frontdoor-parity-exclusions.json`: every operation
   must be exercised or excluded with a named reason; stale entries (not
   declared / exercised anyway) fail the run. Silent skipping is forbidden.
8. Re-probes the five T1 contract defects from
   `docs/evidence/t1-openapi-sweep-2026-09-30.md`, asserting BOTH directions
   (the served document declares the code AND the real binary answers it).

The pre-existing guards are untouched and still authoritative for their own
arms: `TestSpecParityGuard` (in-process declared-vs-served + route
registration) and `make check-spec-parity`
(`scripts/check-spec-parity.sh` + `scripts/spec-parity-check.py`). This guard
adds the real-front-door exercise, the exclusion ledger, and the T1 arms on
top of them; nothing was weakened or duplicated.

## Measured result (this run)

Run at commit `49fab216a1b495552cdc9d2640953779bc3e7ba8` (pre-change HEAD),
2026-11-26:

```text
front-door parity: N=93 declared ops (served /openapi.json), M=93 exercised,
K=0 excluded, E=0 exclusion entries
exercised status histogram 2xx: 55, 4xx: 35, 5xx: 3
```

- N=93: every operation declared by the served `/openapi.json` (75 paths).
- M=93: every one exercised by a real HTTP request that reached a handler.
- K=0: no operation needed excluding — with a scratch SQLite database, an
  admin bearer and a hermetic fake LLM upstream, every declared operation is
  safely invokable, so the exclusion ledger ships empty (schema + rules in
  place; adding an entry is the escape hatch for a future operation that
  cannot safely be invoked).
- E=0: consistent with K.
- The three 5xx answers are the declared 501 stubs
  (`GET /vcs/{vcsId}` `x-not-implemented`, `POST /api/pty`, `POST /pty`) —
  mounted handlers serving their documented 501, not mux misses.

PASS condition N = M + K = M + E holds (93 = 93 + 0).

## T1 defect reprobes (all pass against the real binary)

| defect | defect (sweep, 2026-09-30) | arm in this guard | result |
|---|---|---|---|
| T1-D1 | POST /api/v1/auth/keys returned 200, contract declares 201 | served doc declares 201 AND binary answers 201 for `{"scope":"readonly"}` | PASS |
| T1-D2 | POST /api/v1/sessions/{id}/tasks returned 200, declares 201 | served doc declares 201 AND binary answers 201 for `{"title":...}` | PASS |
| T1-D3 | GET /project/{projectId} answered 404 outside the then-declared set with a foreign error body | answer must be inside the served declared set (now 200/401/404) and a typed envelope, not a mux miss | PASS |
| T1-D4 | GET /permission/{id}: documented 200 unreachable for an existing row (schema drift masked as 404) | seeded pending row answers 200 with the row's id/status | PASS |
| T1-D5 | POST /permission/{id}/resolve returned 500 `no such column: decision_reason` on a well-formed body | seeded pending row + `{"decision":"approved"}` answers 200 | PASS |

## Red proofs (the guard can fail)

Both arms were proven to fail on a deliberately broken tree, then reverted:

1. T1 arm: `internal/api/billing.go` `handleCreateAPIKey` reverted to
   `WriteHeader(http.StatusOK)` →
   `T1-D1: POST /api/v1/auth/keys = 200, want the declared 201` and test FAIL.
2. Route arm: `r.Get("/api/v1/metrics", s.handleGetMetrics)` registration
   removed from `internal/api/server.go` →
   `declared operation is NOT mounted on the real front door: GET /api/v1/metrics — mux miss (status 404)`
   plus `N=93 != M=92 + K=1 (E=0)` and test FAIL.

## Commands

```bash
go build ./... && go vet ./...          # clean
CGO_ENABLED=0 go test ./... -count=1    # full suite
bash scripts/check-spec-parity.sh       # PASS — declared == served (75 paths / 93 ops)
go test ./internal/api/ -run TestFrontDoorParityRealBinary -count=1 -v
```
