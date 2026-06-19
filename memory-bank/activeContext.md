# Active Context

## Current Focus
All specs updated to the Go-first unified binary model. The architecture is now: one Go binary (`consensus`) containing harness, REST API, MCP server, and CLI. Two database backends (Postgres via pgx, SQLite via modernc.org/sqlite). No Deno, no Edge Functions, no TypeScript in the runtime.

## Recent Changes
1. **SPEC-008 fully rewritten** — Harness loop rewritten in Go. Removed all Deno/Edge Function references. Added deployment model section explaining Go binary + `--db` flag.
2. **SPEC-009 fully rewritten** — Unified deployment model. One binary, two database backends. Topologies for local dev, Supabase Cloud, self-hosted Supabase, any Postgres provider, horizontal scaling. Added "Why Not Deno Edge Functions" section.
3. **SPEC-001 updated** — Architecture diagram updated to show Go binary with DB driver interface. "Two Hemispheres" updated.
4. **SPEC-003 updated** — pg_cron section marked as Postgres-only with Go-cron fallback. Handler types updated (sql_function, http_endpoint, go_native, subprocess). PL/v8 open question resolved.
5. **SPEC-015 updated** — PocketBase parity table → backend parity table. SSE implementation unified. MCP transport simplified.
6. **SPEC-010 updated** — External hemisphere → sandboxed subprocess. Tool lifecycle rewritten. Handler types updated.
7. **SPEC-012 updated** — Tool handler type updated.
8. **SPEC-013 updated** — Webhook handler unified to single Go implementation. Parity table updated.
9. **SPEC-014 updated** — HITL notification channels unified. Parity table updated.
10. **SPEC-016 updated** — CLI references updated from PocketBase to Go binary.
11. **SPEC-017 updated** — UI adapter parity table updated.
12. **SPEC-018 updated** — OpenAPI server URLs updated. Parity table updated.
13. **SPEC-019 updated** — Enterprise onboarding flow updated (no Edge Function deploy step).
14. **SPEC-000 updated** — North Star principle updated. Architecture diagram updated.
15. **SPEC-011 updated** — Canonical handler_type values updated.

## Key Architectural Decisions
- **One Go binary** — contains harness loop, REST API, MCP server, CLI, migrations
- **Two database backends** — Postgres (any provider: Supabase Cloud, self-hosted Supabase, Neon, Railway, RDS) and SQLite (embedded)
- **`--db` flag** — `sqlite://data.db` or `postgres://connection-string`
- **No Deno, no Edge Functions** — harness is a persistent worker, not serverless
- **pg_cron is optional** — Go binary has its own cron fallback for maintenance jobs
- **Supabase Cloud is just a Postgres connection string** — binary runs on user's machine, fly.io, Railway, or anywhere

## Next Steps
1. Draft consolidated SQL schema file (all CREATE TABLE statements)
2. Start Go project structure (`cmd/consensus/main.go`, `internal/harness/`, etc.)
3. Implement database driver interface
4. Build harness loop prototype
5. Implement MCP server (SPEC-015 §5)
6. First end-to-end test

## Remaining Open Questions
- Dynamic table cleanup TTL
- Sub-agent depth limit
- Embedding model migration procedure
- opencode version compatibility
- File system mismatch (server-side Consensus, local codebase)
- Partial commit semantics in interactive transactions (SPEC-020 Q5)
- Turn-level vs iteration-level billing for multi-turn planning (SPEC-020 Q6)
