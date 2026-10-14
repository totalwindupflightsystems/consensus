# ROUTE-ADD-014..018 wave — foreman live verification (2026-10-14)

Wave: consensus-foreman-2026-10-14-17-13-08 (5 workers, openai-codex/gpt-5.6-sol, own worktrees).
Foreman independent probe on the MERGED tree @ 49c17d9, binary /tmp/consensus-wave-verify,
scratch DB /tmp/cons-probe/probe2.db, listener 127.0.0.1:18972:

| probe | result |
|---|---|
| unauth GET /api/integration | 401 `{"_tag":"UnauthorizedError","message":"invalid credentials"}` |
| GET /api/integration (auth) | 200 empty-truthful `{data:[], location:{...}}` |
| GET /api/integration/int_1 (auth) | 200 (ROUTE-ADD-018 one-segment get) |
| DELETE /api/integration/attempt/att_1 (auth) | 204 idempotent no-op cancel (ROUTE-ADD-015) |
| GET /api/integration/attempt/att_1 (auth) | 200 empty-truthful status (ROUTE-ADD-016) |
| POST /api/integration/attempt/att_1/complete (auth) | 204 (ROUTE-ADD-017) |
| POST /api/integration (wrong method) | 405 METHOD_NOT_ALLOWED |
| GET /api/integration/x/extra (deep unregistered) | 404 (no /api/integration/* catch-all) |

Comparator after all 5 merges: integration-family drift_declared_not_served = 3 (was 7;
the remaining 3 are the connect/key + connect/oauth + credential family rows = ROUTE-ADD-019/020 scope).

Merge-resolution notes (recorded for future waves):
- ROUTE-ADD-015 and 016 both registered the identical pattern
  /api/integration/attempt/{attemptID} — net/http PANICS on duplicate mux patterns.
  Merged tree resolves to one entry dispatching by method
  (internal/shim/opencode/api_integration_attempt_dispatch.go).
- isV2EnvelopeRoutePath ordering: the /complete suffix rule must run BEFORE the
  one-segment attempt rule; the one-segment /api/integration/ rule (ROUTE-ADD-018)
  runs LAST.
- Worker tests that pinned wave-isolation assumptions (sibling method 405, one-segment
  bogus 404) were updated to the merged contract with comments citing the merge.
- Full short suite `go test -short -count=1 ./...` exit 0 on the merged tree.
