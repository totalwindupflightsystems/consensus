# Progress

## What Works
- Complete spec suite (21 files, 000-020) covering all major subsystems
- All 12 critical cross-spec inconsistencies resolved
- All phantom table/column references eliminated
- memory_events.type enum unified to 8 canonical values
- Session status state machine unified to 10 states
- **All specs updated to Go-first unified binary model**
- Interactive transaction staging (SPEC-020) with timeout, crash recovery, rollback-retry prevention
- MCP authentication specified
- Missing REST endpoints added
- Tool staging conflict resolved
- 16 tables cross-referenced in SPEC-003 §10

## What's Left to Build
1. **Consolidated SQL schema** — All CREATE TABLE statements in one runnable file
2. **Go project structure** — `cmd/consensus/main.go`, `internal/harness/`, `internal/api/`, `internal/mcp/`, `internal/db/`
3. **Database driver interface** — Postgres (pgx) + SQLite (modernc.org/sqlite) behind shared interface
4. **Harness loop** — Go implementation of SPEC-008 / SPEC-020
5. **REST API** — Go HTTP server (SPEC-015 endpoints)
6. **MCP server** — Go MCP server (SPEC-015 §5)
7. **CLI** — Cobra or similar for SPEC-016 commands
8. **opencode shim** — SPEC-017 adapter
9. **Webhook handlers** — SPEC-013
10. **HITL approval flow** — SPEC-014
11. **End-to-end test**
12. **OpenAPI spec** — SPEC-018 file structure
13. **Web admin UI** — Future phase

## Known Issues
- (All previously known issues resolved)

## Resolved Questions
- ~~PocketBase parity requires full rewrite~~ → Single Go binary with driver interface
- ~~Deno Edge Functions for harness~~ → Go binary (persistent worker, not serverless)
- ~~Kysely/Drizzle ORM needed~~ → Direct SQL with Go's database/sql + pgx
- ~~SET LOCAL connection pooling risk~~ → Safe with transaction mode pooling
- ~~Multi-statement SQL injection~~ → Split on semicolons before classification
- ~~Missing parameterize() contract~~ → Three-tier execution model
- ~~SPEC-020 tool_call conflict~~ → tool_call_ref + execution outside transaction
- ~~MCP has no authentication~~ → Same API key mechanism
- ~~Missing REST endpoints~~ → Added /config, /health, /auth/keys
- ~~All phantom table references~~ → Fixed to canonical names
- ~~PL/v8 needed~~ → Go binary handles all procedural logic natively

## Evolution of Decisions
- 3 incompatible state machine definitions → unified in SPEC-011
- display_mode column → separate table (append-only)
- skills_registry dual purpose → split into tools + skills tables
- Tool execution inside main tx → two-phase model
- Mixed current_setting prefixes → unified consensus.*
- Deno Edge Functions + PocketBase Go → Single Go binary with driver interface
- Kysely/Drizzle ORM → Direct SQL (database/sql + pgx for Postgres, modernc.org/sqlite for SQLite)
- handler_type values → sql_function, http_endpoint, go_native, subprocess
- One-shot iteration only → + interactive transaction staging (SPEC-020)
- memory_events.type 6 values → 8 canonical values
