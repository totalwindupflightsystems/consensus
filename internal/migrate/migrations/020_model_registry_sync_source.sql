-- Migration 020: Add sync_source to model_registry for models.dev integration.
-- +goose Up
ALTER TABLE model_registry ADD COLUMN sync_source TEXT NOT NULL DEFAULT 'static';
ALTER TABLE model_registry ADD COLUMN synced_at TEXT;
ALTER TABLE model_registry ADD COLUMN provider_id TEXT;

-- Don't overwrite 'static' entries on re-sync.
CREATE INDEX idx_model_registry_sync_source ON model_registry(sync_source);

-- +goose Down
DROP INDEX IF EXISTS idx_model_registry_sync_source;
ALTER TABLE model_registry DROP COLUMN IF EXISTS provider_id;
ALTER TABLE model_registry DROP COLUMN IF EXISTS synced_at;
ALTER TABLE model_registry DROP COLUMN IF EXISTS sync_source;
