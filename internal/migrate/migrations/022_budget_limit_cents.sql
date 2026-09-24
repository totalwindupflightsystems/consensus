-- Migration 022: Add budget_limit_cents to sessions for per-session budget enforcement.
-- +goose Up
ALTER TABLE sessions ADD COLUMN budget_limit_cents INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE sessions DROP COLUMN IF EXISTS budget_limit_cents;
