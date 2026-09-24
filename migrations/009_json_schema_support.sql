-- ============================================================================
-- Conscience — 009_json_schema_support.sql
-- ============================================================================
-- JSON Schema enforcement for dynamic tables (CS-GAP-002).
--
-- Adds:
--   1. output_schema column to model_registry for per-model output declarations
--   2. dynamic_table_schemas table for per-table JSON Schema tracking + versioning
--   3. Schema versioning: when output_schema changes, old constraint is dropped
--      and a new one is added
--
-- axiom:trace work_item=WI-003
--   spec=specs/003-database.md#4,specs/007-json-schema.md
--   plan=phase-2/task-1
-- ============================================================================

BEGIN;

-- ============================================================================
-- SECTION 1 — Add output_schema to model_registry
-- ============================================================================
-- Each model can declare the expected JSON Schema for its output.
-- This enables per-model output validation constraints on dynamic tables.
-- The schema is a JSON Schema draft-07 document stored as TEXT.
-- NULL means no schema enforcement for this model.

ALTER TABLE model_registry
ADD COLUMN output_schema TEXT;

COMMENT ON COLUMN model_registry.output_schema IS
'JSON Schema draft-07 document for model output validation. NULL = no enforcement.';

-- ============================================================================
-- SECTION 2 — dynamic_table_schemas table
-- ============================================================================
-- Tracks JSON Schema constraints applied to dynamic tables.
-- Each row represents a schema constraint that was or is applied to a table's
-- data column. Supports versioning: when a schema changes, a new row is added
-- with an incremented version, and the old constraint is dropped.
--
-- Use cases:
--   - Track which tables have active schema constraints
--   - Version schema changes over time
--   - Enable rollback to previous schema version
--   - Support both Postgres (CHECK constraint) and SQLite (app-layer validation)

CREATE TABLE IF NOT EXISTS dynamic_table_schemas (
    id                BIGSERIAL PRIMARY KEY,
    table_name        TEXT NOT NULL,
    schema_document   TEXT NOT NULL,
    version           INT NOT NULL DEFAULT 1,
    is_active         BOOLEAN NOT NULL DEFAULT true,
    applied_by        UUID REFERENCES sessions(id),
    constraint_name   TEXT,
    notes             TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    superseded_at     TIMESTAMPTZ,
    UNIQUE(table_name, version)
);

CREATE INDEX idx_dynamic_schemas_active ON dynamic_table_schemas(table_name, is_active)
    WHERE is_active = true;

COMMENT ON TABLE dynamic_table_schemas IS
'Tracks JSON Schema constraints applied to dynamic table data columns.';

COMMENT ON COLUMN dynamic_table_schemas.table_name IS
'Name of the dynamic table this schema constrains.';

COMMENT ON COLUMN dynamic_table_schemas.schema_document IS
'The JSON Schema draft-07 document as a TEXT string.';

COMMENT ON COLUMN dynamic_table_schemas.version IS
'Monotonically increasing version. Starts at 1 for each table.';

COMMENT ON COLUMN dynamic_table_schemas.is_active IS
'Only one version per table can have is_active=true at a time.';

COMMENT ON COLUMN dynamic_table_schemas.constraint_name IS
'Name of the CHECK constraint in Postgres (e.g., tbl_json_schema_v1).';

COMMIT;

-- ============================================================================
-- END OF MIGRATION 009
-- Summary of objects created:
--   1 column added (model_registry.output_schema)
--   1 table created (dynamic_table_schemas)
-- ============================================================================
