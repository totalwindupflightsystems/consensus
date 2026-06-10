# WI-010: Complete CLI Commands

**Status**: In Progress
**Spec**: SPEC-016 (CLI Interface)
**Gap**: CS-GAP-014 (MEDIUM) — CLI Missing Commands
**Estimated**: ~5h

## Scope

1. **Direct DB migration mode** — Add `--db-url` flag + `MigrateFunc` hook so `migrate up/down` work directly against a local DB (like `init` does), not just via REST proxy.
2. **`session cost`** — Add `GetSessionBilling` client method. Improve output with summary totals.
3. **`session logs --follow`** — Wire follow mode for live event streaming via SSE. Improve stub.
4. **`config list`** — Already exists. Verify coverage.
5. **`config edit`** — Already exists. Verify coverage.

## Plan Steps

### Step 1: MigrateFunc hook + --db-url flag
- Add `var MigrateFunc func(action string, dbURL string) error` to `internal/cli/migrate.go`
- Add `--db-url` flag to migrate up/down/run/rollback
- Wire `MigrateFunc` in `cmd/conscience/main.go` to run migrations directly against the DB
- Handle: "up" (run pending), "down" (rollback last), "status" (show version)

### Step 2: Session billing client method
- Add `GetSessionBilling(sessionID string) (map[string]any, error)` to `internal/cli/client.go`
- Update `newSessionCostCmd()` to use the new method
- Improve output formatting with total cost summary

### Step 3: Session logs --follow improvement
- Improve the follow stub to attempt SSE streaming
- At minimum, document the SSE requirement clearly

### Step 4: Tests & verification
- Run `go test ./internal/cli/...` and `go test ./...`
- Commit with conventional commit message

## Trace
axiom:trace work_item=WI-010 spec=specs/016-cli-interface.md plan=.memory-bank/work-items/WI-010/plan.md
