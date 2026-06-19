# Consensus — Master Work Item List (Fresh Sweep)

**Generated**: 2026-05-29 | **Source**: master-gap-analysis-2026-05-29.md
**Total**: 22 work items, ~215h estimated

---

## Phase 1: Production Blockers (must fix to go live)

### WI-001: Remove Mock LLM Fallback + Wire Real Clients
- **Gap**: CS-RT-001 (CRITICAL) + CS-GAP-011 (HIGH)
- **Goal**: Fail fast on LLM client failure. Wire real OpenAI + Anthropic clients.
- **Tasks**:
  1. Remove `llm.NewMockClient()` fallback at `main.go:112-114` — return error instead
  2. Implement real Anthropic client (replace stub that always errors)
  3. Wire `response_format: json_schema` with `strict: true` for OpenAI (CS-GAP-007)
  4. Add `CONSENSUS_MOCK_LLM=1` opt-in flag for dev/testing
  5. Add Anthropic `cache_control` breakpoints (CS-GAP-016)
- **Specs**: 007, 012, 022
- **Effort**: 12h
- **Priority**: P0

### WI-002: Migrate lib/pq → pgx/v5
- **Gap**: CS-GAP-012 (MEDIUM) + CS-GAP-015 (MEDIUM)
- **Goal**: Replace `lib/pq` with `pgx/v5` for connection pooling, LISTEN/NOTIFY, FOR UPDATE SKIP LOCKED
- **Tasks**:
  1. Replace `github.com/lib/pq` with `github.com/jackc/pgx/v5` in go.mod
  2. Update `internal/db/postgres/postgres.go` to use pgx connection pool
  3. Implement `FOR UPDATE SKIP LOCKED` in `ClaimNextReadyTask`
  4. Wire Postgres LISTEN/NOTIFY for real-time events (CS-GAP-013 SSE fix)
  5. Update SPEC-022 library research doc to match
- **Specs**: 009, 022, 015
- **Effort**: 8h
- **Priority**: P0

---

## Phase 2: Core Architecture Gaps

### WI-003: Implement JSON Schema Enforcement
- **Gap**: CS-GAP-002 (CRITICAL)
- **Goal**: DB-level `jsonb_matches_schema()` CHECK constraints on dynamic tables
- **Tasks**:
  1. Install/build `pg_jsonschema` extension (Postgres) + find/write `sqlite-jsonschema` equivalent
  2. Modify `internal/db/dynamic/dynamic.go` → `CreateTable()` adds CHECK constraint using schema from `model_registry.output_schema`
  3. Wire `ALTER TABLE ADD CONSTRAINT CHECK(jsonb_matches_schema(...))` capability
  4. Add schema versioning for constraint evolution
- **Specs**: 003, 007, 011
- **Effort**: 20h
- **Priority**: P1

### WI-004: Implement Cognitive Firewall (Quarantine Scanner)
- **Gap**: CS-GAP-003 (CRITICAL)
- **Goal**: External data quarantined → scanned → approved/rejected before entering memory
- **Tasks**:
  1. Implement `ScanQuarantinedEvent()` — fast local model scan (regex + heuristic for MVP)
  2. Wire into webhook ingestion path: `IngestEvent()` → `external_quarantine` → scanner → `memory_events` or reject
  3. Add quarantine status API endpoint
  4. Connect SSE events for quarantine state changes
- **Specs**: 005, 013
- **Effort**: 15h
- **Priority**: P1

### WI-005: Implement External Tool Execution Sandbox
- **Gap**: CS-GAP-004 (CRITICAL)
- **Goal**: External (non-SQL) tools can actually execute in isolated sandbox
- **Tasks**:
  1. Implement `ExecuteExternalTool()` — subprocess isolation (chroot or WASM)
  2. Wire `custom_agent_tools` execution through sandbox
  3. Add tool execution result capture → `tool_results` table
  4. Add rate limiting per-tool (column exists, not enforced)
  5. Add tool approval gating (CS-GAP-021)
- **Specs**: 010, 014
- **Effort**: 25h
- **Priority**: P1

### WI-006: Implement Three-Tier SQL Execution Model
- **Gap**: CS-GAP-005 (HIGH) + CS-GAP-008 (HIGH)
- **Goal**: Tier 1 (stored-proc-only) and Tier 2 (parameterized) execution modes
- **Tasks**:
  1. Implement Tier 1 executor: only allows `SELECT function_name(...)` calls
  2. Implement Tier 2 executor: parameterized SQL with typed placeholders
  3. Wire `EnforceExecutionPolicy()` to route based on session trust level
  4. Add DB trigger creation: state transitions, prerequisite checks, rate limiting
  5. Wire trigger enforcement in Go for SQLite parity
- **Specs**: 003, 005, 008
- **Effort**: 18h
- **Priority**: P1

### WI-007: Wire `active_context_view` as Actual Context Source
- **Gap**: CS-GAP-006 (HIGH)
- **Goal**: Harness queries the VIEW instead of assembling context in Go
- **Tasks**:
  1. Replace `context.go:314-349` Go assembly with `SELECT * FROM active_context_view WHERE session_id = $1`
  2. Implement `SET LOCAL consensus.session_id` for RLS on view queries (CS-GAP-010)
  3. Wire VIEW's `DISTINCT ON` deduplication, `CASE` rendering, and cache tier ordering
  4. Add tool call collapse rules to the VIEW
- **Specs**: 001, 002, 003, 005
- **Effort**: 10h
- **Priority**: P1

---

## Phase 3: Completeness & Integration

### WI-008: Implement RBAC Scope Model
- **Gap**: CS-GAP-009 (HIGH)
- **Goal**: Global → Project → Sub-Agent role hierarchy
- **Tasks**:
  1. Define `project_id` column + `projects` table
  2. Implement scope checks in session creation, task claiming, memory access
  3. Add cross-project visibility rules
  4. Wire into subagent spawning (sub-agents inherit parent scope)
- **Specs**: 004, 005
- **Effort**: 10h
- **Priority**: P2

### WI-009: Wire SSE Event Stream
- **Gap**: CS-GAP-013 (MEDIUM)
- **Goal**: Real-time SSE push to connected clients
- **Tasks**:
  1. Implement DB polling goroutine or pgx LISTEN/NOTIFY listener
  2. Push session state changes, memory events, approval requests to EventBus
  3. Wire into shim event bridge (replace no-op)
- **Specs**: 015, 017
- **Effort**: 4h
- **Priority**: P2

### WI-010: Complete CLI Commands
- **Gap**: CS-GAP-014 (MEDIUM)
- **Goal**: All SPEC-016 commands functional
- **Tasks**:
  1. Add `session logs` subcommand (tail memory_events)
  2. Add `session cost` subcommand (query billing)
  3. Add `config list` and `config edit` subcommands
  4. Implement direct `migrate up/down` (not REST proxy)
- **Specs**: 016
- **Effort**: 5h
- **Priority**: P2

### WI-011: Add go.mod Dependencies (chi, goose, mcp-go)
- **Gap**: CS-GAP-015 (MEDIUM)
- **Goal**: Align go.mod with SPEC-022 recommendations
- **Tasks**:
  1. Add `chi/v5` — replace `net/http.ServeMux` with chi router
  2. Add `goose/v3` — replace custom migration runner (or document decision)
  3. Add `mcp-go` — replace custom JSON-RPC MCP impl (or document decision)
  4. Update SPEC-022 with rationale for any deviations
- **Specs**: 021, 022
- **Effort**: 10h
- **Priority**: P2

### WI-012: Implement Vector Compression Pipeline
- **Gap**: CS-GAP-001 (CRITICAL but large scope)
- **Goal**: Agent-triggered memory compression with vector validation
- **Tasks**:
  1. Select embedding model (e.g., text-embedding-3-small)
  2. Implement `compression_queue` worker (heartbeat goroutine)
  3. Implement cosine similarity validation before compression
  4. Implement compression worker role with RLS isolation
  5. Wire `compressed` display_mode promotion
- **Specs**: 002, 011
- **Effort**: 30h
- **Priority**: P3 (large scope, deferrable)

---

## Phase 4: Polish & Hardening

### WI-013: Shim File Operations
- **Gap**: CS-GAP-017 (MEDIUM)
- **Goal**: `/file/*`, `/find/*` work through shim
- **Effort**: 6h
- **Priority**: P3

### WI-014: Token Budget Uses Model Registry
- **Gap**: CS-GAP-018 (MEDIUM)
- **Goal**: `calculateCostUSD` queries `model_registry` instead of hardcoded map
- **Effort**: 2h
- **Priority**: P3

### WI-015: MCP stdio Transport
- **Gap**: CS-GAP-019 (LOW)
- **Effort**: 5h
- **Priority**: P4

### WI-016: OpenAPI Bundle/Build
- **Gap**: CS-GAP-020 (LOW)
- **Effort**: 3h
- **Priority**: P4

### WI-017: Fix sqlite.go Tx.IsActive
- **Gap**: CS-GAP-022 (LOW)
- **Goal**: Track committed/rolled-back state explicitly (like postgres.go)
- **Effort**: 2h
- **Priority**: P4

### WI-018: SPEC-021 File Inventory Sync
- **Gap**: Existing RT-006
- **Goal**: Update specs/021 to match actual file tree
- **Effort**: 2h
- **Priority**: P4

### WI-019: Ops Runbooks
- **Gap**: Existing RT-007
- **Goal**: Deployment checklist, backup/restore, admin key rotation, failure modes
- **Effort**: 4h
- **Priority**: P4

### WI-020: MCP Trace Markers
- **Gap**: Existing RT-004
- **Goal**: Add telemetry spans to MCP server handlers
- **Effort**: 3h
- **Priority**: P4

### WI-021: Sub-agent Prompt Filtering
- **Gap**: SPEC-012 gap
- **Goal**: Sub-agents get filtered tool list + compressed memory pointers
- **Effort**: 4h
- **Priority**: P4

### WI-022: No Drift Handling
- **Gap**: CS-GAP from 009
- **Goal**: Pause agents before migration, resume after
- **Effort**: 3h
- **Priority**: P4

---

## Execution Order

```
Phase 1 (Blockers):  WI-001 (LLM)  ──→  WI-002 (pgx)
Phase 2 (Core):      WI-003, WI-004, WI-005, WI-006, WI-007  [parallel where possible]
Phase 3 (Complete):   WI-008 → WI-009 → WI-010, WI-011  [WI-009 depends on WI-002]
Phase 4 (Polish):     WI-012–WI-022  [all parallel, any order]
```

**Dependency chain**: WI-002 (pgx) → WI-009 (SSE) → WI-013 (shim events)

**Total estimated effort**: ~215h across 22 work items

**Quick-start**: WI-001 (12h) + WI-002 (8h) = production-readiness foundation in ~20h
