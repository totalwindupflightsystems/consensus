# Meta-Planning: Vector Compression Pipeline (WI-012)

**Work Item**: vector-compression-01  
**Specs**: SPEC-002 §8 (Vector-Validated Compression Loop), SPEC-003 §2.14 (model_registry), SPEC-011 §10 (Embedding Parity), SPEC-011 §13 (RLS Roles)  
**Gap**: CS-GAP-001 (CRITICAL)  
**Created**: 2026-05-29

axiom:trace work_item=vector-compression-01 spec=specs/002-memory.md,specs/003-database.md,specs/011-canonical-definitions.md plan=meta-planning doc=.memory-bank/work-items/vector-compression-01/meta-planning.md

---

## 1. What Exists Today

### Data Layer
- `memory_events` table with `summary_text` (nullable TEXT), `embedding` (vector(1536)) columns
- `compression_queue` table: `event_id`, `current_tier`, `next_tier`, `status`, `attempts`, `max_attempts` — fully DDL'd in migration 001
- `display_modes` table: `mode IN ('full', 'compressed', 'hidden')` — per SPEC-011 §3
- `model_registry` table: tier-based model catalog with cost tracking
- `system_settings` table: key-value config storage
- `agent_billing` table with `category IN ('cognition', 'compression', 'embedding', 'tool_call')` — billing for compression and embedding already modeled

### RLS/Permissions
- `compression_worker` role already created in migration 001 with `GRANT UPDATE (summary_text) ON memory_events`
- RLS policy `session_isolate_compression` on compression_queue handles `compression_worker` bypass
- 4-role model (agent_role, compression_worker, alt_mode_role, tool_executor) fully defined

### What's Missing (CS-GAP-001)
1. No embedding client — no way to call OpenAI embeddings API
2. No compression worker goroutine — no background queue processing
3. No cosine similarity computation in Go — no validation gate
4. No tier escalation logic — raw→compressed→abstract→canonical not coded
5. No progressive disclosure wiring at the Go/VIEW level (VIEW already supports it via display_modes)
6. No embedding model registered in model_registry
7. No wiring in main.go to start the worker

## 2. Architecture Decisions

### Decision 1: Embedding Storage — JSON Arrays, not pgvector
**Rationale**: The constraint says "no pgvector extension dependency". We store embeddings as JSON arrays in-memory for validation (not as a persistent pgvector column). The worker reads original content, generates embedding₁, generates summary, generates embedding₂, compares both in Go code. Embeddings are computed on-demand and discarded after validation.

For durability/audit, we add embedding storage as JSONB (works on both Postgres and SQLite).

### Decision 2: Single Embedding Model (SPEC-011 §10)
All embeddings use the SAME model (`text-embedding-3-small`). The embedding model is stored in `system_settings` key `embedding_model` and also registered in `model_registry` with `tier=0` (embedding tier). This guarantees cosine similarity is mathematically valid.

### Decision 3: Go-Native Cosine Similarity
Compute cosine similarity in Go code:
```
cosine_similarity(a, b) = dot(a, b) / (norm(a) * norm(b))
```
Standard formula, works on float64 slices. Threshold: 0.85 per SPEC-002 §8.2.

### Decision 4: Tier Ladder
| Tier | Display Mode | Description |
|------|-------------|-------------|
| 0    | full        | Raw content (original) |
| 1    | compressed  | First-level summary (40-60% compression) |
| 2    | abstract    | Abstract-level summary (70-80% compression) |
| 3    | canonical   | Canonical form (~90% compression, fixed structure) |

The compression_queue tracks `current_tier` and escalates when cosine similarity < 0.85.

### Decision 5: Worker Identity
The compression worker goroutine connects as the existing `compression_worker` role. Compression writes are tagged via `SET LOCAL` context (Postgres) or session metadata (SQLite). The existing RLS policy already handles role-based isolation.

## 3. Flow Design

```
┌──────────────────────────────────────────────────────────┐
│              COMPRESSION WORKER LOOP                     │
├──────────────────────────────────────────────────────────┤
│                                                           │
│  Every N seconds (configurable, default 5s):             │
│                                                           │
│  1. BEGIN transaction                                    │
│  2. SELECT FROM compression_queue                        │
│     WHERE status = 'pending'                             │
│     ORDER BY created_at ASC LIMIT batch_size             │
│     FOR UPDATE SKIP LOCKED (Postgres)                    │
│                                                           │
│  3. FOR each pending event:                              │
│     a. Mark status = 'processing'                        │
│     b. SELECT content FROM memory_events                 │
│     c. Generate embedding₁ = embed(content)               │
│     d. Select Tier N model from model_registry            │
│     e. Generate summary via LLM call                     │
│     f. Generate embedding₂ = embed(summary)               │
│     g. Compute cosine_similarity(embedding₁, embedding₂)  │
│                                                           │
│     h. IF cosine >= 0.85:                                │
│        - UPDATE memory_events SET summary_text           │
│        - UPSERT display_modes SET mode                   │
│        - Mark compression_queue status = 'completed'     │
│        - Record billing (category='compression')         │
│                                                           │
│     i. IF cosine < 0.85 AND attempts < max_attempts:     │
│        - Increment attempts, escalate current_tier       │
│        - Mark status = 'pending' (for retry with next tier)│
│                                                           │
│     j. IF cosine < 0.85 AND attempts >= max_attempts:    │
│        - Mark compression_queue status = 'failed'        │
│                                                           │
│  4. COMMIT                                               │
│                                                           │
└──────────────────────────────────────────────────────────┘
```

## 4. Progressive Disclosure

The `active_context_view` already supports progressive disclosure via the `display_modes` join:
- `mode = 'full'` → shows `content` (cache tier 2)
- `mode = 'compressed'` → shows `summary_text` (cache tier 2, smaller)
- `mode = 'hidden'` → excluded from view

No VIEW changes needed. The compression worker writes display_modes rows, and the VIEW automatically picks them up on the next iteration.

Cache tier ordering (SPEC-003 §6.2): Compressed events remain in Layer 2 (Immutable Event Ledger) but are smaller. The agent sees summaries first; if it needs the full content, it loads via page fault (SELECT content FROM memory_events).

## 5. Files to Create/Modify

### New Files
1. `internal/llm/embedding.go` — Embedding client interface + OpenAI impl
2. `internal/llm/embedding_test.go` — Embedding tests
3. `internal/compression/compression.go` — Core compression types, cosine similarity, tier logic
4. `internal/compression/compression_test.go` — Core compression tests
5. `internal/compression/worker.go` — Compression worker goroutine
6. `internal/compression/worker_test.go` — Worker tests
7. `migrations/015_embedding_model.sql` — Seed embedding model data

### Modified Files
8. `cmd/consensus/main.go` — Start compression worker
9. `internal/config/config.go` — Add compression config

## 6. Acceptance Criteria

1. Embedding client can call OpenAI `/v1/embeddings` and return a vector
2. Cosine similarity computed correctly in Go (validated with known values)
3. Compression worker polls queue, processes events, writes results
4. Tier escalation: failed cosine check promotes to next tier
5. Progressive disclosure: compressed events show summary in VIEW
6. Billing recorded for compression and embedding operations
7. Works on both SQLite (test) and Postgres (production)
