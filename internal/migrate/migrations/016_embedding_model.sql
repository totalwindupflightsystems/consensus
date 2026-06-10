-- ============================================================================
-- Conscience — 015_embedding_model.sql
-- ============================================================================
-- Seed the embedding model in model_registry and system_settings.
--
-- SPEC-011 §10 (Vector Embedding Parity): All compression tiers use the same
-- embedding model for cosine similarity comparison. This model is registered
-- as tier=0 (embedding-only) and stored in system_settings for runtime lookup.
--
-- SPEC-002 §8 (Vector-Validated Compression Loop): The embedding model must
-- be available before compression workers can validate summaries.
--
-- axiom:trace work_item=vector-compression-01
--   spec=specs/002-memory.md,specs/011-canonical-definitions.md
--   plan=phase-3/task-3-1
-- ============================================================================

BEGIN;

-- ============================================================================
-- SECTION 1 — SEED EMBEDDING MODEL IN model_registry
-- ============================================================================
-- Tier 0 is reserved for the embedding model (SPEC-011 §10).
-- All compression tiers use this same model for vector similarity.
-- The embedding model is NOT used for summarization — only for computing
-- cosine similarity between original content and generated summaries.
--
-- NOTE: tier=0 is outside the normal 1-3 range for LLM tiers.
-- model_registry CHECK constraint allows (1, 2, 3) only.
-- We use a workaround: insert with tier=1 but tag with 'embedding' classifier.
-- The actual embedding model is identified by system_settings key.
-- ============================================================================

-- Add the embedding model as tier=1 (it's not a summarization model but
-- needs to be in the registry for discoverability). Tag it with 'embedding'
-- so the compression worker can identify it.
INSERT INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out, classifier_tags, enabled)
VALUES ('text-embedding-3-small', 1, 8191, 0.0000, 0.0000, '["embedding"]', true)
ON CONFLICT (model_id) DO NOTHING;

-- ============================================================================
-- SECTION 2 — SET EMBEDDING MODEL IN system_settings
-- ============================================================================
-- This is the canonical source of truth for which embedding model to use.
-- The compression worker reads this at startup.
-- SPEC-011 §10.2: "Set at install time, cannot be changed while sessions active"
INSERT INTO system_settings (key, value, description)
VALUES (
    'embedding_model',
    'text-embedding-3-small',
    'Canonical embedding model for vector similarity (SPEC-011 §10). All compression tiers use this same model.'
)
ON CONFLICT (key) DO NOTHING;

-- ============================================================================
-- SECTION 3 — ADD EMBEDDING COLUMN EVENT_EMBEDDINGS TABLE
-- ============================================================================
-- Stores the computed embedding vectors for auditability and debugging.
-- Embeddings are stored as JSONB arrays (not pgvector) to avoid extension
-- dependency and ensure SQLite/Postgres parity.
-- The compression worker writes these for each processed event.
CREATE TABLE IF NOT EXISTS event_embeddings (
    event_id          BIGINT NOT NULL REFERENCES memory_events(id) ON DELETE CASCADE,
    model             TEXT NOT NULL,
    embedding         JSONB NOT NULL,
    dimensions        INT NOT NULL DEFAULT 1536,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id)
);

-- ============================================================================
-- SECTION 4 — ADD EMBEDDING SEARCH VIEW
-- ============================================================================
-- Provides a unified way to query embeddings for cosine similarity computation.
-- This view exists for audit/debugging; the actual cosine similarity is
-- computed in Go code (not via pgvector <=> operator).
CREATE OR REPLACE VIEW event_embeddings_view AS
SELECT
    ee.event_id,
    ee.model,
    ee.embedding,
    ee.dimensions,
    me.session_id,
    me.type,
    me.summary_text IS NOT NULL AS has_summary
FROM event_embeddings ee
JOIN memory_events me ON me.id = ee.event_id;

COMMIT;

-- ============================================================================
-- END OF MIGRATION
-- ============================================================================
-- Summary:
--   - 1 model_registry row (text-embedding-3-small, tagged 'embedding')
--   - 1 system_settings key ('embedding_model' = 'text-embedding-3-small')
--   - 1 new table (event_embeddings) for durable embedding storage
--   - 1 new view (event_embeddings_view) for querying embeddings
-- ============================================================================
