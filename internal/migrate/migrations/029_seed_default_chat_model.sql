-- ============================================================================
-- Consensus — 029_seed_default_chat_model.sql
-- ============================================================================
-- REVIEW-CONSENSUS-3 Finding 2: on a fresh Postgres database every
-- POST /api/v1/sessions fails the sessions.model_id foreign key.
--
-- Evidence (2026-10-15 Postgres probe, docs/testing/evidence/review-consensus-3):
--   ERROR: insert or update on table "sessions" violates foreign key
--          constraint "fk_sessions_model"
--   DETAIL: Key (model_id)=(default) is not present in table "model_registry".
--
-- Root cause: after migrations a fresh database's model_registry holds only the
-- embedding model seeded by 016 (tagged 'embedding'). CreateSession
-- (internal/api/service.go) selects the cheapest enabled chat-capable model,
-- deliberately excludes embedding models, and — finding no candidate — falls
-- back to the literal sentinel "default", which is not a row in
-- model_registry and therefore cannot satisfy fk_sessions_model. The
-- RegisterIfMissing sync path is skipped for that sentinel by design, so the
-- first session insert on a fresh Postgres database always fails. SQLite never
-- surfaced this: its driver does not enforce foreign keys and the pg→SQLite
-- migration filter drops Postgres-only constraints.
--
-- Fix: seed the model the shipped consensus.yaml already names as its default
-- (llm.default_model: deepseek-v4-flash) as an enabled, chat-capable registry
-- row. A fresh database now has a valid FK target, and CreateSession's
-- "cheapest enabled non-embedding model" query returns a real model id instead
-- of the sentinel. Nothing else changes for an installation whose models.dev
-- syncer has already populated the registry: ON CONFLICT (model_id) DO NOTHING
-- never overwrites an existing registration (a syncer row keeps its own tier,
-- pricing, tags and sync_source).
--
-- Shape notes:
--   * The column list mirrors 016_embedding_model.sql (the model_registry shape
--     from 001 §3.5). The columns 020 added (sync_source, synced_at,
--     provider_id) are left to their defaults — sync_source defaults to
--     'static', which is exactly the "do not overwrite me on re-sync" marker.
--   * tier=2 rather than 1: the shipped deepseek-v4-flash is priced at
--     $0.15/M in, and modelsync.classifyTier reserves tier 1 for models under
--     $0.15/M in and tier 3 for the premium families (gpt-/claude-/gemini-2.5/
--     grok-3). classifyTier therefore also lands this model on tier 2.
--   * classifier_tags starts with 'chat' (the tag capabilitiesToTags() gives
--     every chat model), so "chat-capable" is discoverable by tag; the tool
--     tag is spelled function_calling to match capabilitiesToTags.
--
-- Idempotency: ON CONFLICT (model_id) DO NOTHING; the file is safe to re-run
-- and is a no-op on SQLite beyond the same seed row (ON CONFLICT is supported
-- there and 016 already relies on it).
--
-- axiom:trace work_item=REVIEW-CONSENSUS-3
--   spec=specs/003-database.md,specs/015-api-and-mcp.md
--   impl=internal/migrate/migrations/029_seed_default_chat_model.sql
-- ============================================================================

BEGIN;

-- ============================================================================
-- SECTION 1 — SEED THE SHIPPED DEFAULT CHAT MODEL
-- ============================================================================

INSERT INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out, classifier_tags, enabled)
VALUES ('deepseek-v4-flash', 2, 128000, 0.1500, 0.6000, '{chat,function_calling}', true)
ON CONFLICT (model_id) DO NOTHING;

COMMIT;

-- ============================================================================
-- END OF MIGRATION
-- ============================================================================
-- Summary:
--   - 1 model_registry row (deepseek-v4-flash, tier 2, tag 'chat') — the
--     fk_sessions_model target a fresh database was missing, so CreateSession
--     no longer falls back to the "default" sentinel.
-- ============================================================================
