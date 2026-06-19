# SPEC-002: Cognitive Memory Engine

**Status:** Draft  
**Source:** Gemini Chat Turns 4-10, 23-27, 30-31  
**Depends On:** SPEC-001-ARCHITECTURE  
**Last Updated:** 2026-04-08  
**Amended By:** SPEC-011 (Canonical Definitions) — where this spec contradicts SPEC-011, SPEC-011 takes precedence.

---

## 1. Overview

The Cognitive Memory Engine is the subsystem that manages what an agent **remembers**, **sees**, and **can recall** across iterations. It replaces monolithic prompt strings with a database-native memory architecture built on Event Sourcing, dynamic SQL views, and vector-validated compression.

The core analogy is virtual memory from operating systems:

| Concept | OS Equivalent | Consensus Mechanism |
|---|---|---|
| Working memory (context window) | RAM | `active_context_view` (SQL VIEW) |
| Long-term storage | Hard Drive | `memory_events` (append-only ledger) |
| Memory addressing | Page tables | Active pointer arrays + Memory Pages |
| Page fault | Swap-in from disk | `SELECT full_output FROM subagent_runs WHERE memory_id = X` |
| Process snapshot | Core dump / PCB | `iteration_commits` table |
| Memory compression | ZRAM / swap compression | Vector-validated summary generation |

**Source:** Turns 4-5 (virtual memory analogy), Turn 8 (context as dynamic view)

---

## 2. Append-Only Ledger (Event Sourcing)

### 2.1 Principle

The `memory_events` table is **never updated and never deleted**. Every agent output, tool result, thinking trace, and internal monologue is logged as a new sequential row. This is the immutable source of truth—the agent's complete cognitive history.

**Source:** Turn 9 (append-only ledger design)

### 2.2 Schema

```sql
CREATE TABLE memory_events (
    id              BIGSERIAL PRIMARY KEY,
    type            TEXT NOT NULL CHECK (type IN ('header', 'text_block', 'tool_call', 'tool_result', 'thinking', 'system', 'inherited_pointer', 'user_message')),
    content         TEXT NOT NULL,
    summary_text    TEXT,
    session_id      UUID NOT NULL REFERENCES sessions(id),
    iteration_created BIGINT NOT NULL,
    embedding       vector(1536),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

> **Note:** Display mode is tracked in a separate `display_modes` table (SPEC-011 §3) to preserve append-only semantics.

### 2.3 Column Semantics

| Column | Purpose |
|---|---|
| `id` | Globally unique, monotonic memory pointer. Referenced by iteration commits and memory pages. |
| `type` | Determines Markdown rendering format (see §6). Also drives display rules in the active view. |
| `content` | The full, original text. Never mutated after INSERT. |
| `summary_text` | Populated by the compression loop (§7). NULL until a summary is generated. |
| `session_id` | Owner scope—ties the event to a specific agent session for RLS isolation. |
| `iteration_created` | The loop iteration number when this event was first written. Enables chronological reconstruction. |
| `embedding` | Vector embedding of `content`, populated asynchronously by trigger. Used for semantic search (§7.3). |

> **Note:** The `display_mode` column has been removed from `memory_events`. Display state is tracked in a separate `display_modes` table (SPEC-011 §3), preserving true append-only semantics.

### 2.4 Immutability Enforcement

```sql
-- Revoke UPDATE and DELETE from agent_role
REVOKE UPDATE, DELETE ON memory_events FROM agent_role;
```

The `display_mode` column has been moved to a separate `display_modes` table (SPEC-011 §3). `memory_events` is now truly append-only: no UPDATE is ever performed on this table by any role. Compression workers update `summary_text` only (SPEC-011 §3.5).

**Source:** Turn 9 (append-only, never update/delete), Turn 11 (transaction safety)

---

## 3. Context Window as Dynamic SQL VIEW

### 3.1 Principle

The agent's active prompt is **not a static string**. It is a SQL VIEW that filters, formats, and compresses the ledger based on active pointer arrays and display rules. The harness never manually constructs the prompt—it queries the view.

**Source:** Turn 8 (context as dynamic view)

### 3.2 The View Definition

```sql
CREATE OR REPLACE VIEW active_context_view AS
WITH active_ids AS (
    SELECT unnest(active_pointers) AS ptr_id
    FROM iteration_commits
    WHERE session_id = current_setting('consensus.session_id')::UUID
    ORDER BY iteration_id DESC
    LIMIT 1
)
SELECT
    me.id,
    me.iteration_created,
    me.type,
    COALESCE(dm.mode, 'full') AS display_mode,
    CASE
        WHEN COALESCE(dm.mode, 'full') = 'compressed' AND me.summary_text IS NOT NULL
            THEN me.summary_text
        WHEN COALESCE(dm.mode, 'full') = 'hidden'
            THEN NULL
        ELSE me.content
    END AS rendered_text
FROM memory_events me
JOIN active_ids ai ON me.id = ai.ptr_id
LEFT JOIN display_modes dm ON dm.memory_id = me.id
WHERE COALESCE(dm.mode, 'full') != 'hidden'
    AND me.session_id = current_setting('consensus.session_id')::UUID
ORDER BY me.iteration_created, me.id;
```

### 3.3 Harness Consumption

The harness simply executes:

```sql
SELECT rendered_text FROM active_context_view
WHERE session_id = 'current_session'
ORDER BY iteration_created, id;
```

All pruning, compression, and deduplication happens inside the database. The harness receives a clean, ordered set of text rows.

### 3.4 Compression Toggle

When the agent flags `display_mode = 'compressed'` for a memory row, the VIEW's `CASE` statement automatically substitutes `summary_text` for the full `content`. This happens at the SQL level—no harness code changes required.

```sql
-- Agent's JSON output triggers this INSERT into display_modes:
INSERT INTO display_modes (memory_id, mode, set_by_iteration, session_id)
VALUES (104, 'compressed', 6, 'session-uuid');
-- In practice, this is typically called via stored procedure:
-- SELECT set_display_mode(event_id := 104, mode := 'compressed');
```

**Source:** Turn 8 (compression toggle in view)

### 3.5 Tool Call Rules via SQL Window Functions

The view enforces display rules that would otherwise bloat the context. Example: "If more than 10 tool calls, only show the last 2 failures."

```sql
-- Window function for tool call collapse
CASE
    WHEN type = 'tool_call' AND COUNT(*) FILTER (WHERE type = 'tool_call')
        OVER (PARTITION BY session_id) > 10
    THEN
        CASE
            WHEN status = 'failed'
                AND ROW_NUMBER() OVER (
                    PARTITION BY session_id, status
                    ORDER BY iteration_created DESC
                ) <= 2
            THEN rendered_text
            ELSE '[Tool call collapsed: ' || type || ' #' || id || ']'
        END
    ELSE rendered_text
END
```

This is pure SQL—no middleware logic, no prompt engineering. The database enforces the rule deterministically.

**Source:** Turn 8 (tool call collapse rules)

### 3.6 Zero-Cost Deduplication

When two or more Memory Pages (§5) both reference the same `memory_event` ID, the VIEW uses `DISTINCT ON id` to ensure that event appears only once in the rendered prompt—no duplicate tokens.

```sql
SELECT DISTINCT ON (id) rendered_text, type, iteration_created
FROM active_context_view
WHERE session_id = 'current_session'
ORDER BY id, iteration_created;
```

**Source:** Turn 8 (DISTINCT deduplication for pages)

---

## 4. Semantic Context Paging

### 4.1 Principle

The context window is **RAM**: fast, expensive, limited. The database is the **Hard Drive**: slow, cheap, infinite. The agent sees high-level pointers in its active context. It can query full data on demand, exactly like a page fault triggers a swap-in from disk.

**Source:** Turns 4-5 (semantic context paging)

### 4.2 Pointer Representation

In its active context, the agent sees:

```
[Memory ID: 402] Subagent Alpha deployed to scrape React docs (Status: Success)
[Memory ID: 405] User requested dark mode UI refactor
```

These are the `summary_text` or `content` (depending on display_mode) from the `memory_events` rows currently in its active pointer array.

### 4.3 On-Demand Recall (Page Fault)

When the agent needs the full output behind a pointer:

```sql
SELECT content FROM memory_events WHERE id = 402;
```

Or for a subagen's complete run log:

```sql
SELECT full_output FROM subagent_runs WHERE memory_id = 402;
```

The agent issues this query as part of its JSON output. The harness executes it within the same transaction, and the result is injected into the next iteration's context.

### 4.4 Eviction

If the agent determines a pointer is no longer needed in working memory, it omits the ID from its next active_pointers commit (§5). The full data remains in the ledger—it is simply not loaded into the VIEW.

**Source:** Turn 4 (pointer-based memory), Turn 5 (pruning without deletion)

---

## 5. Memory Pages

### 5.1 Principle

Instead of listing individual memory IDs (e.g., `[12, 45, 102, 104, 105, 106]`), agents group IDs into named **Pages** referenced by a single name. This saves massive token counts—like OS virtual memory pages mapping many addresses to one page table entry.

**Source:** Turn 5 (memory pages concept)

### 5.2 Schema

```sql
CREATE TABLE memory_pages (
    id              BIGSERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    target_ids      BIGINT[] NOT NULL,
    linked_page_ids BIGINT[] NOT NULL DEFAULT '{}',
    session_id      UUID NOT NULL REFERENCES sessions(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(name, session_id)
);
```

`target_ids` references `memory_events.id` — the actual content rows.

`linked_page_ids` references other `memory_pages.id` — enabling page-to-page composition for deep agent hierarchies. A parent agent can create a "chapter" page that groups several "section" pages. Nesting is limited to 1 level: a page can reference other pages, but those referenced pages should not themselves contain `linked_page_ids` with further page references. The active_context_view resolves linked pages with a single-level expansion:

```sql
-- Resolution expands both target_ids and linked_page_ids
WITH direct_ids AS (
    SELECT unnest(target_ids) AS ptr_id
    FROM memory_pages
    WHERE name = ANY(ARRAY['api_research', 'ui_context'])
      AND session_id = current_setting('consensus.session_id')::UUID
),
linked_ids AS (
    SELECT unnest(mp.target_ids) AS ptr_id
    FROM memory_pages mp
    JOIN memory_pages parent ON mp.id = ANY(parent.linked_page_ids)
    WHERE parent.name = ANY(ARRAY['api_research', 'ui_context'])
      AND parent.session_id = current_setting('consensus.session_id')::UUID
)
SELECT DISTINCT ptr_id FROM (
    SELECT ptr_id FROM direct_ids
    UNION
    SELECT ptr_id FROM linked_ids
) all_ids;
```

### 5.3 Usage

Instead of the agent passing six individual IDs, it references the page:

```sql
-- Agent creates a page grouping research-related memories
INSERT INTO memory_pages (name, target_ids, session_id)
VALUES ('api_research', ARRAY[105, 106, 107, 108, 109, 110], 'session-uuid');
```

The agent then tells a sub-agent:

> "Use Memory Page 'api_research' for the API documentation context."

The sub-agent's harness resolves the page:

```sql
SELECT unnest(target_ids) AS memory_id
FROM memory_pages
WHERE name = 'api_research' AND session_id = 'sub-session-uuid';
```

### 5.4 Page Resolution in Active View

The `active_context_view` resolves pages by expanding their IDs into the pointer set before querying `memory_events`:

```sql
-- Expand pages into the active pointer set
WITH page_ids AS (
    SELECT unnest(target_ids) AS ptr_id
    FROM memory_pages
    WHERE name = ANY(ARRAY['api_research', 'ui_context'])
      AND session_id = current_setting('consensus.session_id')::UUID
)
-- Union with directly referenced IDs
-- Then DISTINCT to deduplicate overlapping pages
```

**Source:** Turn 5 (memory pages saving tokens)

---

## 6. Iteration Commits (Time Travel)

### 6.1 Principle

Every loop iteration saves a snapshot of the `active_pointers` array. This is the agent's "commit." Pruning is simply omitting an ID from the next commit. Rollback is querying an older commit. The data itself is never destroyed.

**Source:** Turns 9-10 (time travel, event sourcing)

### 6.2 Schema

```sql
CREATE TABLE iteration_commits (
    iteration_id    BIGSERIAL PRIMARY KEY,
    session_id      UUID NOT NULL REFERENCES sessions(id),
    active_pointers BIGINT[] NOT NULL,
    display_rules   JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.3 Commit Example

```
iteration_id:    6
session_id:      "alpha_agent_1"
active_pointers: [12, 45, 102, 104]
display_rules:   {"compress": [12, 45]}
```

At the end of iteration 6, the harness commits this snapshot. On iteration 7, if the agent decides to prune memory 45:

```
iteration_id:    7
session_id:      "alpha_agent_1"
active_pointers: [12, 102, 104]  -- 45 omitted
display_rules:   {"compress": [12]}
```

Memory 45 still exists perfectly in `memory_events`. It is simply not in the active pointer set.

### 6.4 Rollback

To restore the agent's context to iteration 5:

```sql
SELECT active_pointers, display_rules
FROM iteration_commits
WHERE iteration_id = 5 AND session_id = 'alpha_agent_1';
```

The harness takes the returned pointer array, sets it as the current active state, and the agent continues from that exact cognitive frame. No data has been lost.

### 6.5 Audit Trail

Because `memory_events` is append-only and `iteration_commits` records every state, you can reconstruct the agent's exact context window at **any millisecond** in its history—a perfect cognitive DVR.

**Source:** Turn 9 (Git-like commit model), Turn 10 (ledger tracks all context iterations)

---

## 7. Native Markdown Generation

### 7.1 Principle

Memory is formatted as structured Markdown before being sent to the LLM. PostgreSQL's `string_agg()` combines rows into a single Markdown document. LLMs process Markdown headers and lists far more efficiently than raw JSON or prose.

**Source:** Turn 9 (native markdown generation)

### 7.2 Type-to-Markdown Mapping

| `type` value | Markdown output |
|---|---|
| `header` | `## {content}` |
| `text_block` | `{content}` (paragraph block) |
| `tool_call` | `**Tool Call:** {content}` |
| `tool_result` | `> **Result:** {content}` |
| `thinking` | `<!-- thinking: {content} -->` |
| `system` | `***{content}***` |

### 7.3 Aggregation Query

```sql
SELECT string_agg(
    CASE me.type
        WHEN 'header'      THEN E'## ' || me.rendered_text || E'\n\n'
        WHEN 'text_block'  THEN me.rendered_text || E'\n\n'
        WHEN 'tool_call'   THEN E'**Tool Call:** ' || me.rendered_text || E'\n\n'
        WHEN 'tool_result' THEN E'> **Result:** ' || me.rendered_text || E'\n\n'
        WHEN 'thinking'    THEN E'<!-- thinking: ' || me.rendered_text || E' -->\n\n'
        WHEN 'system'      THEN E'***' || me.rendered_text || E'***\n\n'
    END,
    ''
    ORDER BY me.iteration_created, me.id
) AS markdown_prompt
FROM active_context_view me
WHERE me.session_id = current_setting('consensus.session_id')::UUID;
```

The harness receives a single string: a perfectly formatted Markdown document ready to inject into the LLM prompt.

### 7.4 Why Markdown

1. **Token efficiency**: LLMs assign strong attention to `##` headers, making context navigation more reliable
2. **Native parsing**: Every major LLM is trained on massive Markdown corpora
3. **Zero middleware**: The database is the rendering engine; no TypeScript formatting code needed
4. **Human readability**: The Alt-Mode dashboard can display the same view directly

**Source:** Turn 9 (Postgres string_agg generates markdown)

---

## 8. Vector-Validated Compression Loop

### 8.1 Principle

When compressing memory (shrinking `content` into `summary_text`), the system does not blindly trust the summarizer. It validates the summary against the original text using **cosine similarity** of vector embeddings. If the score is below a threshold, the system escalates to a more capable model.

**Source:** Turn 30 (vector-validated compression), Turn 31 (2D routing matrix)

### 8.2 The Loop

```
┌──────────────────────────────────────────────────┐
│         VECTOR-VALIDATED COMPRESSION LOOP         │
├──────────────────────────────────────────────────┤
│                                                   │
│  1. Raw memory event written to memory_events     │
│  2. Trigger fires → selects Tier 1 model         │
│     (horizontal routing by context length)        │
│  3. Tier 1 model generates summary               │
│  4. Compute embeddings:                           │
│     - embedding_raw    ← embed(original content)  │
│     - embedding_summary ← embed(summary)          │
│  5. Cosine similarity check:                      │
│     SELECT 1 - (embedding_raw <=> embedding_summary) │
│            AS cosine_score;                        │
│  6. IF cosine_score >= threshold (e.g., 0.82):   │
│     → ACCEPT: write summary to summary_text       │
│       set mode = 'compressed' in display_modes    │
│  7. IF cosine_score < threshold:                  │
│     → REJECT: escalate to Tier 2 model            │
│       → repeat from step 3 with stronger model    │
│                                                   │
└──────────────────────────────────────────────────┘
```

### 8.3 pgvector Implementation

```sql
-- Compute cosine distance between original and summary
SELECT 1 - (embedding_raw <=> embedding_summary) AS cosine_score
FROM memory_events
WHERE id = target_event_id;

-- Threshold enforcement (in stored procedure)
IF v_cosine_score >= 0.82 THEN
    UPDATE memory_events
    SET summary_text = v_summary
    WHERE id = target_event_id;

    INSERT INTO display_modes (memory_id, mode, set_by_iteration, session_id)
    VALUES (target_event_id, 'compressed', v_iteration, v_session_id)
    ON CONFLICT (memory_id) DO UPDATE SET mode = 'compressed';
ELSE
    -- Queue for Tier 2 re-compression
    INSERT INTO compression_queue (event_id, current_tier, next_tier)
    VALUES (target_event_id, 1, 2);
END IF;
```

### 8.4 Background Processing

Embeddings and compression are **never** in the hot path of the agent loop. They execute asynchronously:

1. Agent writes to `memory_events` → transaction commits
2. `AFTER INSERT` trigger fires
3. Trigger enqueues the event into a `compression_queue` table
4. Background worker (Go goroutine in the binary) processes the queue
5. Summary is written back to `summary_text` and `mode` is set in `display_modes`

The agent continues operating with `mode = 'full'` until the compression completes, at which point the next iteration's VIEW automatically renders the compressed summary.

**Source:** Turn 30 (cosine similarity threshold), Turn 10 (background embedding via trigger)

---

## 9. Model Cascade / 2D Routing Matrix

### 9.1 Principle

The compression loop does not use a single model. It routes through a multi-dimensional matrix: **horizontally** within a cheap tier (selecting by context length), then **vertically** to expensive tiers only if the quality gate fails.

**Source:** Turn 31 (2D routing matrix, intra-tier selection)

### 9.2 Model Registry Schema

```sql
CREATE TABLE model_registry (
    id              BIGSERIAL PRIMARY KEY,
    model_id        TEXT NOT NULL UNIQUE,
    tier            INT NOT NULL CHECK (tier IN (1, 2, 3)),
    max_context     INT NOT NULL,
    cost_per_m_in   NUMERIC(8,4) NOT NULL,
    cost_per_m_out  NUMERIC(8,4) NOT NULL,
    classifier_tags TEXT[] DEFAULT '{}',
    enabled         BOOLEAN NOT NULL DEFAULT true
);
```

> **Note:** Embedding model is fixed system-wide (SPEC-011 §10). All tiers use the same embedding model for vector similarity — LLM tier affects summarization quality, not embedding.

### 9.3 Tier Definitions

| Tier | Models (examples) | Context Range | Cost Profile |
|---|---|---|---|
| **1** | Nemotron 3, Step Fun 3.5, Qwen 3.5 (1-3B), Gemma 4, Qwen 3.5, GPT-5 Nano, Mercury (4-8B) | 8k–128k | Sub-cent per million tokens |
| **2** | Haiku 4.5, Gemini Flash, Gemini OSs | 128k–256k | Multiple dollars per million tokens |
| **3** | GPT-5, Claude Opus | 200k+ | Premium pricing |

### 9.4 Routing Logic

```sql
-- Horizontal routing: select cheapest model in Tier 1 that fits the token count
SELECT model_id
FROM model_registry
WHERE tier = 1
  AND max_context >= v_token_count
  AND enabled = true
ORDER BY cost_per_m_out ASC
LIMIT 1;

-- If Tier 1 fails quality gate, escalate vertically
SELECT model_id
FROM model_registry
WHERE tier = 2
  AND max_context >= v_token_count
  AND enabled = true
ORDER BY cost_per_m_out ASC
LIMIT 1;
```

**Source:** Turn 31 (horizontal routing within tier, vertical escalation), Turn 30 (model cascade intro)

### 9.5 User Configuration

Users sort models by preference or cost. Rules can be defined:

```json
{
  "routing_rules": [
    {"if_token_count_gt": 10000, "skip_tier_1_models_with_max_context_lt": 12000},
    {"prefer_model": "qwen-3.5-8b", "for_classifier_tag": "code"},
    {"budget_ceiling_per_compression": 0.05}
  ]
}
```

**Source:** Turn 30 (user preference sorting, text-length routing rules)

---

## 10. Dynamic Entities with JSONB

### 10.1 Principle

Agents can request new tables (e.g., `order_tracking`). The framework provisions them with **required system columns** plus a **JSONB data column** for flexible NoSQL-style payloads. This gives agents the freedom to design their own data structures without raw DDL access.

**Source:** Turn 27 (dynamic entities, not single scratchpad), Turn 26 (JSONB tables)

### 10.2 Framework-Provisioned Schema

When an agent calls the stored procedure:

```sql
SELECT create_agent_table('order_tracking');
```

The framework generates:

```sql
CREATE TABLE order_tracking (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id          UUID NOT NULL REFERENCES sessions(id),
    iteration_created   BIGINT,
    deleted_at          TIMESTAMPTZ,
    linked_memory_pages TEXT[],
    data                JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Automatic RLS
ALTER TABLE order_tracking ENABLE ROW LEVEL SECURITY;
CREATE POLICY isolate_session ON order_tracking
    FOR ALL USING (session_id = current_setting('consensus.session_id')::UUID);

-- Automatic soft-delete trigger (intercept DELETE → UPDATE deleted_at)
CREATE TRIGGER soft_delete_order_tracking
    BEFORE DELETE ON order_tracking
    FOR EACH ROW EXECUTE FUNCTION soft_delete_intercept();
```

### 10.3 System Column Guarantees

| Column | Purpose |
|---|---|
| `id` | Primary key, unique reference |
| `session_id` | RLS isolation scope |
| `iteration_created` | Ledger audit trail |
| `deleted_at` | Soft delete / Alt-Mode recovery |
| `linked_memory_pages` | Cross-reference to Memory Pages |
| `data` | Agent-defined JSONB payload |

### 10.4 Alt-Mode JSON Schema Validation

An administrative agent (in Alt-Mode) can lock down a dynamic table's JSONB column with a strict schema:

```sql
-- Postgres: pg_jsonschema extension
ALTER TABLE order_tracking
ADD CONSTRAINT strict_order_schema
CHECK (jsonb_matches_schema('{
    "type": "object",
    "required": ["item", "sku", "qty"],
    "properties": {
        "item": {"type": "string"},
        "sku":  {"type": "string"},
        "qty":  {"type": "integer", "minimum": 1}
    }
}', data));
```

```sql
-- SQLite: sqlite-jsonschema extension (identical syntax)
ALTER TABLE order_tracking
ADD CONSTRAINT strict_order_schema
CHECK (jsonschema_matches('{
    "type": "object",
    "required": ["item", "sku", "qty"]
}', data));
```

If a working agent submits malformed JSON (e.g., missing `sku`), the transaction is **rejected** by the database. The agent receives the constraint violation error and must correct its payload.

**Source:** Turn 27 (dynamic entities with JSONB), Turn 28 (JSON Schema parity in SQLite), Turn 34 (schema-gated triggers)

---

## 11. Reactive Context Truncation

### 11.1 Principle

Instead of proactively running tokenizers on every loop (expensive, slow), the system **reactively** catches the `400 Context Limit` error from the LLM API, truncates the overflowing payload, and instructs the agent to use chunking or semantic search.

**Source:** Turn 23 (reactive vs proactive truncation)

### 11.2 Flow

```
┌────────────────────────────────────────────────────────┐
│         REACTIVE CONTEXT TRUNCATION FLOW               │
├────────────────────────────────────────────────────────┤
│                                                         │
│  1. Harness builds prompt from active_context_view      │
│  2. Harness sends to LLM API                           │
│  3. LLM API returns: 400 Bad Request                   │
│     "content exceeds context window limit"              │
│  4. Harness catches the error                          │
│  5. Harness identifies the largest recent injection     │
│     (usually a tool_result row)                        │
│  6. Harness truncates the offending row:                │
│     - Keeps first N tokens                             │
│     - Appends: "[SYSTEM: Result truncated. Data saved  │
│       to Memory ID X. Use semantic search or chunking  │
│       tools to access remainder.]"                     │
│  7. Harness retries the API call with truncated prompt │
│  8. Agent wakes up with truncated context + instructions│
│  9. Agent decides how to process the full data         │
│     (sub-agent, chunked reads, or semantic search)     │
│                                                         │
└────────────────────────────────────────────────────────┘
```

### 11.3 Why Reactive Over Proactive

| Approach | Cost | Latency | Accuracy |
|---|---|---|---|
| **Proactive** (tokenizer on every call) | High (tiktoken on 100k+ tokens per tick) | Slower | Approximate |
| **Reactive** (catch API rejection) | Zero (only fires when needed) | Faster on normal calls | Exact (API tells you precisely) |

The 400 error itself is the most accurate and cheapest "tokenizer"—it comes from the model provider's exact counting logic.

### 11.4 Agent Autonomy

Crucially, the harness does **not** decide how to handle the overflow. It truncates minimally to unblock the loop and tells the agent:

```
Your context exceeded the model's token limit. The full data is stored at Memory ID 402.
Options: (1) Write SQL to chunk the data into smaller pages, 
         (2) Spawn a sub-agent to process it,
         (3) Use semantic search to extract only relevant sections.
```

The agent manages its own cognitive overload.

**Source:** Turn 23 (reactive truncation, agent self-management), Turn 22 (context crash scenario)

---

## 12. Subsystem Interactions

The following diagram shows how all memory subsystems interact within a single iteration:

```
┌─────────────────────────────────────────────────────────────────┐
│                    ITERATION FLOW                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. Harness reads current iteration commit                       │
│     └─ iteration_commits → active_pointers: [12, 45, 102, 104] │
│                                                                  │
│  2. Harness expands Memory Pages into pointer set                │
│     └─ memory_pages 'api_research' → +[105, 106]               │
│     └─ DISTINCT deduplication                                    │
│                                                                  │
│  3. active_context_view filters + compresses                    │
│     ├─ ID 12: display_mode='compressed' → summary_text          │
│     ├─ ID 45: display_mode='compressed' → summary_text          │
│     ├─ ID 102: display_mode='full' → full content               │
│     ├─ ID 104: display_mode='full' → full content               │
│     └─ tool_call collapse rule applied                           │
│                                                                  │
│  4. string_agg() generates Markdown prompt                      │
│     └─ ## headings + paragraphs ordered by iteration_created    │
│                                                                  │
│  5. Harness sends Markdown to LLM API                           │
│     ├─ IF 200 OK → parse JSON, execute SQL in transaction       │
│     └─ IF 400 Context Exceeded → reactive truncation (§11)      │
│                                                                  │
│  6. LLM JSON output → SQL transaction                          │
│     ├─ INSERT into memory_events (new events)                   │
│     ├─ UPDATE display_mode on existing rows (compress/hide)     │
│     ├─ INSERT/UPDATE memory_pages (page management)             │
│     └─ IF error → ROLLBACK, inject error into next context     │
│                                                                  │
│  7. Compression trigger fires asynchronously                    │
│     ├─ Tier 1 model generates summary                           │
│     ├─ pgvector cosine similarity check                          │
│     └─ Escalate to Tier 2 if score < 0.82                      │
│                                                                  │
│  8. New iteration commit saved                                  │
│     └─ INSERT iteration_commits with updated active_pointers    │
│                                                                  │
│  9. Loop back to step 1                                         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## 13. Cross-Reference Map

| Spec Section | Source Turns | Related SPEC-001 Sections |
|---|---|---|
| §2 Append-Only Ledger | 9, 10, 11 | §2.2 Atomic Cognition |
| §3 Dynamic VIEW | 8, 9 | §4 Core Data Flow |
| §4 Semantic Paging | 4, 5 | §2.1 DBaaR |
| §5 Memory Pages | 5 | §6 Key Differentiators |
| §6 Iteration Commits | 9, 10 | §2.2 Atomic Cognition |
| §7 Markdown Generation | 9 | §4 Core Data Flow |
| §8 Compression Loop | 30, 10 | §6 Key Differentiators |
| §9 2D Routing | 31, 30 | — (new in this spec) |
| §10 Dynamic Entities | 27, 28, 34 | §2.3 Write Once |
| §11 Reactive Truncation | 23, 22 | §6 Key Differentiators |

---

## 14. Open Questions

1. **Display mode mutability**: *Resolved* — `display_mode` moved to separate table (SPEC-011 §3).
2. **Compression threshold tuning**: What is the optimal default cosine similarity threshold? 0.82 is proposed; real-world testing across model pairs needed.
3. **Page nesting**: Can a Memory Page reference another Memory Page (recursive composition)? Or only raw event IDs?
4. **Embedding model choice**: Which embedding model for the cosine check? Must be consistent—mixing models produces incomparable vectors.
5. **SQLite parity for window functions**: SQLite 3.25+ supports window functions, but complex `FILTER` clauses may need testing for the tool-call collapse rules (§3.5).