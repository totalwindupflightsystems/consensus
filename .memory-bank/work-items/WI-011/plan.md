# WI-011: go.mod Dependencies

**Status**: In Progress
**Spec**: SPEC-022 (Library Research), SPEC-021 (Repository Layout)
**Gap**: CS-GAP-015 (MEDIUM) — go.mod missing 5/10 SPEC-022 dependencies
**Estimated**: ~10h

## Scope

1. **Add `chi/v5`** — Replace `net/http.ServeMux` in the API server with chi router for route groups, middleware, path parameters.
2. **Evaluate `goose/v3`** — Either add it or document why the custom migration runner is kept.
3. **Evaluate `mcp-go`** — Either add it or document why the custom JSON-RPC impl is kept.
4. **Document rationale** — Update `internal/config/library-decisions.md` with decisions.

## Plan Steps

### Step 1: Add chi/v5 dependency
- `go get github.com/go-chi/chi/v5`
- Update `internal/api/server.go` to use `chi.NewRouter()` instead of `http.NewServeMux()`
- Update route registration to use chi's `Get`, `Post`, `Put`, `Patch`, `Delete`, `With`, `Route`, `Group`
- Leverage chi middleware for auth, CORS, rate limiting
- Update `Start()`, `Handler()`, route dispatching methods
- Update `cmd/conscience/main.go` references to `*http.ServeMux` if needed

### Step 2: Evaluate goose/v3
- Review `internal/migrate/migrate.go` — 700+ line custom migration runner
- It handles: bootstrap, loading, status, up, down, drift detection, auto-migrate, SQLite filtering
- Decision: Document that custom runner is kept for dual-backend SQLite filtering, drift detection, and embedded migration management
- No code changes needed

### Step 3: Evaluate mcp-go
- Review `internal/mcp/` — custom JSON-RPC implementation
- Decision: Document that custom implementation is kept for now. mcp-go would provide standardized protocol handling but current implementation is stable.
- No code changes needed

### Step 4: Update documentation
- Update `internal/config/library-decisions.md` with chi/v5 decision, goose decision, mcp-go decision
- Update timestamp and status

### Step 5: Tests & verification
- Run `go test ./...`
- Commit with conventional commit message

## Trace
axiom:trace work_item=WI-011 spec=specs/022-library-research.md,specs/021-repository-layout.md plan=.memory-bank/work-items/WI-011/plan.md
