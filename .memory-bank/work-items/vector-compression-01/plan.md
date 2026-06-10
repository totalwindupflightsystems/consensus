# Vector Compression Pipeline — Implementation Plan

**Work Item**: vector-compression-01  
**Specs**: SPEC-002 §8, SPEC-003 §2.14, SPEC-011 §10, SPEC-011 §13  
**Gap**: CS-GAP-001 (CRITICAL)  
**Status**: In Progress

axiom:trace work_item=vector-compression-01 spec=specs/002-memory.md,specs/003-database.md,specs/011-canonical-definitions.md plan=.memory-bank/work-items/vector-compression-01/plan.md

---

## Phase 1: Foundation — Embedding Client & Core Types

### Task 1.1: Embedding Client Interface

**Step 1.1.1**: Create `internal/llm/embedding.go`
- `EmbeddingClient` interface: `Embed(ctx context.Context, input string) ([]float64, error)`
- `EmbedBatch(ctx context.Context, inputs []string) ([][]float64, error)`
- `openaiEmbeddingClient` struct implementing via POST /v1/embeddings
- Model name configurable (default: `text-embedding-3-small`)
- Returns 1536-dim float64 slice

**Step 1.1.2**: Create `internal/llm/embedding_test.go`
- Mock HTTP server tests
- Tests for error handling, response parsing

### Task 1.2: Compression Core Types & Cosine Similarity

**Step 1.2.1**: Create `internal/compression/compression.go`
- `CompressionTier` type (0=full, 1=compressed, 2=abstract, 3=canonical)
- `TierConfig` with model routing per tier
- `CompressionQueueItem` matching compression_queue schema
- `CompressionResult` (ACCEPTED, REJECTED_ESCALATE, FAILED)

**Step 1.2.2**: Cosine similarity in Go
```
cosine(a, b) = sum(a[i]*b[i]) / (sqrt(sum(a[i]^2)) * sqrt(sum(b[i]^2)))
```
- Pure Go, no extensions
- Handles zero-vector edge case

**Step 1.2.3**: Tier escalation
- `NextTier(current int) int` — maps current_tier to next
- `ShouldEscalate(cosine float64, threshold float64) bool` — standard: 0.85

## Phase 2: Compression Worker

### Task 2.1: Worker Implementation

**Step 2.1.1**: `internal/compression/worker.go`
- `NewWorker(db, embedClient, llmClient, config) *Worker`
- `Start(ctx)` / `Stop()` lifecycle
- Heartbeat ticker (default: 5s interval)
- Polls `compression_queue` for pending items
- PgSQL: `FOR UPDATE SKIP LOCKED` for concurrency
- SQLite: no lock needed (single-writer)

**Step 2.1.2**: `ProcessOne(ctx, item) error`
- SELECT content FROM memory_events
- Generate embedding of content
- Select tier model from model_registry
- Call LLM to generate summary
- Generate embedding of summary
- Cosine similarity validation
- On accept: write summary_text, update display_modes, mark completed
- On reject: escalate tier, retry, or mark failed

**Step 2.1.3**: Billing
- INSERT into agent_billing with category='embedding' for embedding calls
- INSERT into agent_billing with category='compression' for summarization calls

## Phase 3: Database Migration & Wiring

### Task 3.1: Migration 015
- INSERT INTO model_registry for text-embedding-3-small (tier=0)
- INSERT INTO system_settings key='embedding_model', value='text-embedding-3-small'

### Task 3.2: Wiring
- Add `CompressionConfig` to config
- Create embedding client from config
- Start compression worker in main.go

## Key Spec References

| Spec | Section | What It Requires |
|------|---------|-----------------|
| SPEC-002 | §8.1-8.4 | Vector-validated compression loop |
| SPEC-002 | §9.2 | model_registry for 2D routing |
| SPEC-003 | §2.14 | model_registry schema |
| SPEC-011 | §3.5 | compression_worker GRANT summary_text |
| SPEC-011 | §10 | One embedding model for all tiers |
| SPEC-011 | §13 | compression_worker role + grants |
| SPEC-003 | §6.2 | Cache tier ordering (static > ledger > dynamic) |
| SPEC-011 | §6.2 | agent_billing.category includes 'compression', 'embedding' |
