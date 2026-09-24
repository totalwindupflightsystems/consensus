-- ============================================================================
-- Conscience — 006_dynamic_entity_generator.sql
-- ============================================================================
-- Dynamic entity creation for Postgres: SECURITY DEFINER function that
-- provisions new tables with required system columns, JSONB payload, and
-- automatic RLS policies — without granting agents raw DDL privileges.
--
-- Also defines soft_delete_intercept() trigger to convert DELETE INTO
-- soft-delete (sets deleted_at instead of removing rows).
--
-- axiom:trace work_item=spec-003-hardening-01
--   spec=specs/003-database.md,specs/011-canonical-definitions.md
--   plan=sweep-007-remediation
--   source_sweep=sweep-007
--
-- Gaps:
--   HIGH: create_agent_memory_table() — documented but not executable
--   HIGH: soft_delete_intercept() — documented but not executable
-- Fix: Full PL/pgSQL implementations per SPEC-003 §3.2 and §9.1.
-- ============================================================================

BEGIN;

-- ============================================================================
-- SECTION 1 — soft_delete_intercept() Trigger Function
-- ============================================================================
-- Converts DELETE operations on dynamic tables into soft deletes.
-- Instead of removing the row, sets deleted_at = now() for the target row.
-- The calling agent believes the delete succeeded; the data remains for
-- forensic/time-travel recovery.
--
-- This function works on ANY table with a deleted_at column.
-- Source: SPEC-003 §9.1.

CREATE OR REPLACE FUNCTION soft_delete_intercept()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
    EXECUTE format(
        'UPDATE %I SET deleted_at = now() WHERE id = $1',
        TG_TABLE_NAME
    ) USING OLD.id;
    RETURN NULL; -- Cancel the actual DELETE
END;
$$;

COMMENT ON FUNCTION soft_delete_intercept() IS
'Converts DELETE into soft-delete on any table with a deleted_at column.';

-- ============================================================================
-- SECTION 2 — create_agent_memory_table() Function
-- ============================================================================
-- Provisions a new agent-owned table with all required system columns, RLS
-- isolation policies, soft-delete trigger, and updated_at auto-update trigger.
--
-- Uses SECURITY DEFINER so agents can call this without DDL privileges.
-- Source: SPEC-003 §3.2.

CREATE OR REPLACE FUNCTION create_agent_memory_table(p_table_name TEXT)
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $$
DECLARE
    fq_name TEXT;
BEGIN
    -- ========================================================================
    -- Step 1 — Sanitize the table name
    -- Replace non-alphanumeric characters with underscores, lowercase.
    fq_name := lower(regexp_replace(p_table_name, '[^a-z0-9_]', '_', 'g'));
    fq_name := regexp_replace(fq_name, '_+', '_', 'g');  -- collapse multiple underscores
    fq_name := trim(fq_name, '_');  -- remove leading/trailing underscores

    -- ========================================================================
    -- Step 2 — Validate table name
    IF fq_name !~ '^[a-z_][a-z0-9_]{0,62}$' THEN
        RAISE EXCEPTION 'Invalid table name: "%". Must be 1-63 chars, start with a-z or underscore, contain only a-z, 0-9, underscore.', fq_name;
    END IF;

    -- ========================================================================
    -- Step 3 — Reserved name blocklist (31 reserved names)
    IF fq_name IN (
        'sessions', 'memory_events', 'display_modes', 'iteration_commits',
        'memory_pages', 'tasks', 'tool_requests', 'tool_results',
        'tools_registry', 'skills_registry', 'agent_billing', 'workflows',
        'custom_agent_tools', 'tool_files', 'external_quarantine',
        'compression_queue', 'model_registry', 'staging_buffer', 'audit_logs',
        'system_settings', 'agent_messages', 'api_keys', 'api_rate_limits',
        'external_events', 'webhook_registrations', 'routing_rules',
        'agent_circuit_breakers', 'agent_budget_limits', 'secret_access_audit',
        'approval_requests', 'schema_versions'
    ) THEN
        RAISE EXCEPTION 'Cannot create table with reserved name: "%"', fq_name;
    END IF;

    -- ========================================================================
    -- Step 4 — Check if table already exists
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = current_schema() AND table_name = fq_name
    ) THEN
        RETURN 'Table already exists: ' || fq_name;
    END IF;

    -- ========================================================================
    -- Step 5 — Create the table with system columns
    EXECUTE format('
        CREATE TABLE %I (
            id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            session_id          UUID NOT NULL REFERENCES sessions(id),
            iteration_created   BIGINT,
            deleted_at          TIMESTAMPTZ,
            linked_memory_pages UUID[],
            data                JSONB NOT NULL DEFAULT ''{}''::jsonb,
            created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
        )
    ', fq_name);

    -- ========================================================================
    -- Step 6 — Enable RLS and create session isolation policy
    EXECUTE format('
        ALTER TABLE %I ENABLE ROW LEVEL SECURITY
    ', fq_name);

    EXECUTE format('
        CREATE POLICY isolate_session_%s ON %I
            FOR ALL
            TO agent_role, compression_worker, tool_executor
            USING (session_id = current_setting(''conscience.session_id'', true)::UUID)
    ', fq_name, fq_name);

    -- Allow alt_mode_role to bypass RLS (they have BYPASSRLS attribute)
    EXECUTE format('
        CREATE POLICY alt_mode_full_%s ON %I
            FOR ALL
            TO alt_mode_role
            USING (true)
    ', fq_name, fq_name);

    -- ========================================================================
    -- Step 7 — Soft-delete intercept trigger
    EXECUTE format('
        CREATE TRIGGER soft_delete_%s
            BEFORE DELETE ON %I
            FOR EACH ROW
            EXECUTE FUNCTION soft_delete_intercept()
    ', fq_name, fq_name);

    -- ========================================================================
    -- Step 8 — Auto-update updated_at trigger
    EXECUTE format('
        CREATE TRIGGER update_updated_at_%s
            BEFORE UPDATE ON %I
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at()
    ', fq_name, fq_name);

    -- ========================================================================
    -- Step 9 — Grant appropriate privileges to the four canonical roles
    -- agent_role: read/write but cannot drop or alter
    EXECUTE format('
        GRANT SELECT, INSERT, UPDATE ON %I TO agent_role
    ', fq_name);

    -- compression_worker: same as agent_role for dynamic tables
    EXECUTE format('
        GRANT SELECT, INSERT, UPDATE ON %I TO compression_worker
    ', fq_name);

    -- tool_executor: read + insert only (tools write results, don't update)
    EXECUTE format('
        GRANT SELECT, INSERT ON %I TO tool_executor
    ', fq_name);

    -- alt_mode_role: full access (already has ALL from bootstrap, but explicit)
    EXECUTE format('
        GRANT ALL ON %I TO alt_mode_role
    ', fq_name);

    RETURN 'Created table: ' || fq_name || E'\nColumns: id, session_id, iteration_created, deleted_at, linked_memory_pages, data, created_at, updated_at\nPolicies: session isolation (agent_role), full access (alt_mode_role)\nTriggers: soft_delete, update_updated_at';
END;
$$;

COMMENT ON FUNCTION create_agent_memory_table(TEXT) IS
'Provision a new agent-owned dynamic table with system columns, RLS, soft-delete trigger, and updated_at trigger. Reserved name blocklist enforced.';

-- ============================================================================
-- SECTION 3 — verify_dynamic_table() Helper
-- ============================================================================
-- Checks that a dynamically created table has all required system columns.
-- Returns a JSON object with column presence and any missing columns.
-- Used by spec conformance sweeps and migration verification tests.

CREATE OR REPLACE FUNCTION verify_dynamic_table(p_table_name TEXT)
RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
    v_columns TEXT[];
    v_result JSONB := '{}'::jsonb;
    v_column TEXT;
    v_required TEXT[] := ARRAY[
        'id', 'session_id', 'iteration_created', 'deleted_at',
        'linked_memory_pages', 'data', 'created_at', 'updated_at'
    ];
BEGIN
    -- Collect actual column names for the target table
    SELECT array_agg(column_name ORDER BY ordinal_position)
    INTO v_columns
    FROM information_schema.columns
    WHERE table_schema = current_schema()
      AND table_name = p_table_name;

    IF v_columns IS NULL THEN
        RETURN jsonb_build_object(
            'exists', false,
            'error', 'Table "' || p_table_name || '" not found in schema ' || current_schema()
        );
    END IF;

    -- Check each required column
    v_result := jsonb_build_object('exists', true, 'table', p_table_name, 'columns', to_jsonb(v_columns));

    -- Build a presence map for each required column
    FOR v_column IN SELECT unnest(v_required)
    LOOP
        IF v_column = ANY(v_columns) THEN
            v_result := v_result || jsonb_build_object(v_column, true);
        ELSE
            v_result := v_result || jsonb_build_object(v_column, false);
        END IF;
    END LOOP;

    v_result := v_result || jsonb_build_object(
        'all_present',
        (v_result ->> 'id')::boolean
        AND (v_result ->> 'session_id')::boolean
        AND (v_result ->> 'iteration_created')::boolean
        AND (v_result ->> 'deleted_at')::boolean
        AND (v_result ->> 'linked_memory_pages')::boolean
        AND (v_result ->> 'data')::boolean
        AND (v_result ->> 'created_at')::boolean
        AND (v_result ->> 'updated_at')::boolean
    );

    RETURN v_result;
END;
$$;

COMMIT;

-- ============================================================================
-- END OF MIGRATION 006
-- Summary of objects created:
--   2 functions (create_agent_memory_table, soft_delete_intercept)
--   1 helper function (verify_dynamic_table)
-- ============================================================================
