# Tech Context

## Languages and Frameworks
- **Go**: Primary language. Single binary containing harness, REST API, MCP server, CLI
- **PostgreSQL** (any provider): Primary database with RLS, triggers, pg_cron (optional), pgvector, PL/pgSQL
- **SQLite** (embedded): Local deployment with sqlite-vec, sqlite-jsonschema, WAL mode
- **pgx**: Postgres driver for Go
- **modernc.org/sqlite**: Pure-Go SQLite driver (no CGO required)
- **LLM APIs**: OpenAI, Anthropic (Structured Outputs, prompt caching)

## Build and Dev Tools
- Go toolchain (go build, go test)
- SQL migration files in `migrations/`
- Specs in `specs/` directory (000-020)

## Dependencies
- Go standard library + pgx + modernc.org/sqlite
- Optional: pg_cron, pg_net (Postgres-only features with Go fallbacks)
- LLM APIs: OpenAI, Anthropic (Structured Outputs, prompt caching)

## Development Environment
- Go 1.22+ installed
- PostgreSQL for full-featured development (or SQLite for lightweight testing)
- Source conversation preserved at `gemini_chat.md`
- No implementation code yet — design phase

## Deployment
- Single Go binary deployed to fly.io, Railway, bare metal, or Kubernetes
- `--db` flag selects backend at startup
- Multiple binary instances can share one Postgres backend via `FOR UPDATE SKIP LOCKED`
- Supabase Cloud is just a Postgres connection string — binary runs on user's machine, fly.io, or anywhere

## Constraints
- SQLite has single-writer constraint (WAL mode for concurrent reads)
- No native RLS on SQLite — session isolation via Go-layer enforcement
- pg_cron not available on all Postgres providers — Go binary has cron fallback
- sqlite-vec uses brute-force KNN (no HNSW index yet); adequate for <100K vectors
- SET LOCAL not available in SQLite — session context via Go context passing
- Structured Outputs required for LLM JSON response format
- Supavisor transaction mode does NOT support prepared statements — use simple query protocol
