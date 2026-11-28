-- ============================================================================
-- Consensus — 037_seed_edit_file_alias.sql
-- ============================================================================
-- SCN-CONSENSUS-025: the s2_read_file live e2e flake's edit leg failed closed
-- when the model reached for `edit_file` (the conventional agent vocabulary,
-- the same drift DF-CONSENSUS-85 fixed for shell_exec/shell/run_shell/run_command
-- and SCN-CONSENSUS-009 fixed for read_file/write_file). Migrations 033/034
-- seed the public names read/write with handler_ref read_file/write_file, so a
-- drifted read_file call already resolves through the handler_ref arm of
-- ResolveToolRegistryRow — but the `edit` row's handler_ref is 'edit', so every
-- drifted edit_file call was rejected with
-- `tool "edit_file" is not registered or enabled`, the edit never ran, and the
-- scenario's edit leg (and any real task relying on it) was lost.
--
-- Mechanism: seed the alias as its OWN registry row whose handler_ref points
-- at the proven native edit handler (same seed-row pattern as 035). The
-- exact-name arm of ResolveToolRegistryRow resolves it on the first attempt,
-- and the row remains individually enable/disable-able by the operator.
--
-- This is a data-only, idempotent migration. Existing operator registrations
-- (including an operator-defined row named edit_file) are left untouched by
-- ON CONFLICT (name) DO NOTHING. The `edit` row itself is not modified; a
-- disabled edit row and this alias row are independent registry entries,
-- exactly like every other builtin row.
-- ============================================================================

BEGIN;

INSERT INTO tools_registry
    (name, description, hemisphere, parameter_schema, handler_type, handler_ref,
     status, enabled, requires_approval)
VALUES
    ('edit_file',
     'Replace an exact text fragment in a file relative to the session workspace.',
     'external',
     '{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["path","old_string","new_string"],"additionalProperties":false}',
     'go_native',
     'edit',
     'active',
     true,
     false)
ON CONFLICT (name) DO NOTHING;

COMMIT;
