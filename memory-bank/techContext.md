# Tech Context

## Technologies
- **Go**: Primary language. Single binary containing harness, REST API, MCP server, CLI
- **PostgreSQL** (any provider): Primary database with RLS, triggers, pg_cron (optional), pgvector, PL/pgSQL
- **SQLite** (embedded): Local deployment with sqlite-vec, sqlite-jsonschema, WAL mode, Go hooks
- **pgx**: Postgres driver for Go
- **modernc.org/sqlite**: Pure-Go SQLite driver (no CGO required)
- **LLM APIs**: OpenAI, Anthropic (Structured Outputs, prompt caching)

## Development Setup
- Specs in `/specs/` directory (000-020)
- Source conversation preserved at `gemini_chat.md`
- No implementation code yet — still in design phase

## Technical Constraints
- SQLite has single-writer constraint (WAL mode for concurrent reads)
- No native RLS on SQLite — session isolation via Go-layer enforcement
- pg_cron not available on all Postgres providers — Go binary has cron fallback
- sqlite-vec uses brute-force KNN (no HNSW index yet); adequate for <100K vectors
- SET LOCAL not available in SQLite — session context via Go context passing
- Structured Outputs required for LLM JSON response format
- Supavisor transaction mode does NOT support prepared statements — use simple query protocol

## Dependencies
- Go standard library + pgx + modernc.org/sqlite
- Optional: pg_cron, pg_net (Postgres-only features with Go fallbacks)
- LLM APIs: OpenAI, Anthropic (Structured Outputs, prompt caching)
- Optional: Deno runtime for sandboxed tool execution (subprocess, not runtime dependency)

## Project Structure (Planned)
```
cmd/
  conscience/main.go         # Entry point
internal/
  harness/                   # Agent iteration loop (SPEC-008, SPEC-020)
  api/                       # REST API handlers (SPEC-015)
  mcp/                       # MCP server (SPEC-015 §5)
  db/                        # Database driver interface + Postgres/SQLite implementations
  tools/                     # Tool execution sandbox (SPEC-010)
  secrets/                   # Secret injection/scrubbing (SPEC-005)
  migrations/                # Embedded SQL migration files
  shim/                      # opencode server protocol shim (SPEC-017)
  cli/                       # CLI commands (SPEC-016)
migrations/
  001_initial_schema.sql
  ...
```

## Resolved Technical Questions
- Go binary over Deno Edge Functions — harness is a persistent worker, not serverless
- Direct SQL over ORM — database/sql + pgx simpler than Kysely/Drizzle for Go
- SET LOCAL safe with Supavisor/PgBouncer transaction mode
- Statement-level pooling NOT supported for Conscience
- sqlite-vec and sqlite-jsonschema load via Go driver config
- Single binary can be deployed to fly.io, Railway, bare metal, Kubernetes
- Multiple binary instances can share one Postgres backend via FOR UPDATE SKIP LOCKED
