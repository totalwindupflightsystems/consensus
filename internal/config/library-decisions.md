# Library Decisions — Conscience

Concrete dependency decisions for the Conscience Go binary, based on SPEC-022 research.
Each entry includes the chosen library, rationale, and the decision date.

axiom:trace work_item=WI-011 spec=specs/022-library-research.md plan=.memory-bank/work-items/WI-011/plan.md evidence=go.mod,internal/config/library-decisions.md

---

## Decision Map

| Decision | Library | Status | Rationale (abridged SPEC-022) |
|---|---|---|---|
| **D-1** PostgreSQL driver | `pgx/v5` (v5.9+) | ✅ In go.mod | De facto standard. Connection pooling, LISTEN/NOTIFY, FOR UPDATE SKIP LOCKED, MIT license. Pinned to v5.9.x. |
| **D-2** SQLite driver | `modernc.org/sqlite` (pure Go) | ✅ In go.mod | No CGO. Pure Go builds with CGO_ENABLED=0. Vector search deferred to Postgres-only for v1. JSON Schema validation in Go application layer. |
| **D-3** OpenAI API | `openai-go/v3` | ⏳ Deferred | Official SDK currently deferred. Custom LLM client interface (`internal/llm/client.go`) works with both OpenAI-compatible and Anthropic providers via REST. The OpenAI SDK will add structured outputs (strict mode) and streaming. Add when implementing CS-GAP-007 (Structured Outputs). |
| **D-4** Anthropic API | `anthropic-sdk-go` | ⏳ Deferred | Official SDK deferred for same reason as D-3. The Anthropic client in `internal/llm/anthropic.go` is a stub. Add alongside D-3 when implementing CS-GAP-011 (Anthropic Client Stub). |
| **D-5** MCP server | Custom JSON-RPC (not `mcp-go`) | ✅ Custom impl | The existing MCP implementation at `internal/mcp/` uses a hand-rolled JSON-RPC 2.0 server over SSE. It handles tool registration, resource URIs, and API key auth. `mcp-go` would provide stdio/SSE/Streamable HTTP and OAuth, but the current implementation is stable and passes all tests. **Decision**: Keep custom implementation. Re-evaluate `mcp-go` when adding stdio transport (CS-GAP-019) or OAuth — the SDK's fluent builder API would then provide clear value. |
| **D-6** HTTP router | `chi/v5` (v5.2.5) | ✅ In go.mod | Zero dependencies, 100% net/http compatible, SSE-friendly (direct ResponseWriter + Flusher). Replaced `http.NewServeMux()` in `internal/api/server.go`. Clean route groups, path parameters, and middleware composition. **2026-05-29**: Integrated and deployed. |
| **D-7** Server-Sent Events | `net/http` (stdlib) | ✅ No dependency | SSE in Go is simple enough without a library. r3labs/sse is unmaintained. |
| **D-8** CLI framework | `cobra` | ✅ In go.mod | Industry standard (Kubernetes, Hugo, GitHub CLI). Shell completions, help generation. |
| **D-9** YAML config | `gopkg.in/yaml.v3` | ✅ In go.mod | Simple, no Viper's 8+ dependencies. Load YAML struct directly, apply env/flag overrides manually. |
| **D-10** Migrations | Custom runner (not `goose/v3`) | ✅ Custom impl | The custom migration runner at `internal/migrate/migrate.go` (700+ lines) handles: bootstrap, embedded SQL loading, status with drift detection, Up/Down, auto-migrate on startup, and SQLite SQL transformation (PG→SQLite type/function translation). `goose/v3` was evaluated but lacks dual-backend SQL translation and drift detection. **Decision**: Keep custom migration runner. The dual-backend SQLite filtering (PG extensions, triggers, functions, RLS, type casts) is essential for our architecture and would be impossible with goose's simple `database/sql` approach. |
| **D-11** SQLite vector search | Deferred to Postgres-only for v1 | ⏳ Deferred | modernc.org/sqlite cannot load native C extensions (sqlite-vec). Not needed for local dev. |
| **D-12** SQLite JSON Schema validation | Go application layer for v1 | ⏳ Deferred | Same extension limitation. Validate in Go before INSERT/UPDATE for both backends. |

## Rationale Details

### D-10: Why Not goose/v3

The SPEC-022 recommendation was `goose/v3`, but the custom migration runner at `internal/migrate/migrate.go` already provides:

1. **Dual-backend SQL translation** — The `filterForSQLite()` function transforms PG-specific SQL (extensions, triggers, functions, RLS policies, pg_cron, UUID types, TIMESTAMPTZ, JSONB) into SQLite-compatible equivalents. goose runs raw SQL files against `database/sql` and provides no per-backend transformation.
2. **Drift detection** — Checks that all applied migrations have matching embedded files, catching schema drift before new migrations are applied. goose does not have this.
3. **Embedded-only** — Migrations are embedded via `//go:embed` and never read from the filesystem at runtime. goose supports both embedded and filesystem modes.
4. **No new dependency** — Avoids pulling in `pressly/goose/v3` and its driver dependencies.

If goose adds SQL transformation hooks in the future, we could revisit. For v1, the custom runner is more capable for our use case.

### D-5: Why Not mcp-go

The SPEC-022 recommendation was `github.com/mark3labs/mcp-go`, but the custom JSON-RPC implementation at `internal/mcp/` already provides:

1. **All required tools** — create_session, send_message, get_session_status, list_sessions, pause_session, resume_session with JSON Schema parameter definitions.
2. **Resource URI handling** — `conscience://sessions/{id}` resource templates with MCP-compatible URI parsing.
3. **SSE transport** — Proper SSE framing with event types, JSON-RPC 2.0 compliance.
4. **Auth integration** — API key validation via middleware.
5. **Zero new dependency** — Avoids a pre-v1.0 SDK with potential API instability.

`mcp-go` would add stdio transport, Streamable HTTP, and OAuth — features we don't need yet. When implementing CS-GAP-019 (stdio transport), evaluate whether to keep the custom implementation or adopt mcp-go.

## Implementation Status

| Dependency | go.mod | Code Usage | Tested |
|---|---|---|---|
| `pgx/v5` | ✅ v5.9.2 | `internal/db/postgres/` | ✅ |
| `modernc.org/sqlite` | ✅ v1.50.0 | `internal/db/sqlite/` | ✅ |
| `chi/v5` | ✅ v5.2.5 | `internal/api/server.go` | ✅ (2026-05-29) |
| `cobra` | ✅ v1.10.2 | `internal/cli/` | ✅ |
| `gopkg.in/yaml.v3` | ✅ v3.0.1 | `internal/config/`, `internal/cli/` | ✅ |
| Custom migration runner | — | `internal/migrate/` | ✅ |
| Custom MCP (JSON-RPC) | — | `internal/mcp/` | ✅ |
| OpenAI SDK | ❌ Planned | `internal/llm/openai.go` | ⏳ CS-GAP-007 |
| Anthropic SDK | ❌ Planned | `internal/llm/anthropic.go` | ⏳ CS-GAP-011 |
| `goose/v3` | ❌ Not needed | — | 📋 Evaluated, custom kept |
| `mcp-go` | ❌ Not needed | — | 📋 Evaluated, custom kept |

## Build Contract

- `CGO_ENABLED=0 go build ./...` — always works
- Single static binary, zero runtime dependencies
- No Node.js, no Python, no C toolchain required
