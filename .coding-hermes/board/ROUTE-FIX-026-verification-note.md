# ROUTE-FIX-026 live probe: the route already answers per contract — did NOT fix code, verified and closed the stale row

## Summary
- Task was [P2] POST /tui/open-sessions "answers WRONG" (declared 200,400; triage said served 501, source SHIM-DRIFT-137).
- The bug was fixed earlier by ROUTE-FIX-023-CLUSTER (commit 2ca4b0d): tuiOpenSessions (internal/shim/opencode/server.go:4961) validates method, selectors, and body, then answers the declared boolean 200 false. Error arms: 400 BadRequest envelope (undeclared body / bad directory/workspace selectors), 405 on non-POST.
- Declared surface: specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json (served_outcome 200), specs/openapi/upstream/consensus-shim-served-surface.yaml, specs/openapi/paths/shim.yaml tuiAction.
- Verification: go build ./... OK; go vet clean; gofmt clean; go test ./internal/shim/opencode/ -count=1 PASS (TestTUIOpenSessions passes happy path + all error arms). No code change needed; the row was stale, exactly like ROUTE-FIX-022/024-028 closed in commit 48a5032.
- Board: appended work-started event to .coding-hermes/board/events.jsonl; left tasks.jsonl untouched per brief (foreman owns row state).

## Gates
- gofmt -l internal/shim/opencode/: empty
- go vet ./...: pass
- go build ./...: pass
- go test ./internal/shim/opencode/: pass
- gitreins guard: PASS (test mode: diff, tests for changed package: none code-change; suite run directly green)

No code, test, or spec files were changed by this commit on purpose: the only staged change is the board event line. The contract evidence above is from the existing tree at HEAD.
