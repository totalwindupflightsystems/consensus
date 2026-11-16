-- ============================================================================
-- Consensus — 033_seed_filesystem_tools.sql
-- ============================================================================
-- DF-CONSENSUS-66: a fresh install must give the core agent real filesystem
-- and shell capabilities. These registry names are the public builtin surface;
-- handler_ref names are the proven native handlers in internal/tools/native.go.
--
-- This is a data-only, idempotent migration. Builtin names are reserved by
-- this migration: an existing row is repaired only in the dispatch fields below
-- (handler type/ref, lifecycle status, and enabled flag). Operator-owned metadata
-- such as description, parameter schema, ownership, and rate limits is preserved.
-- Unrelated tool names are never touched.
-- ============================================================================

BEGIN;

INSERT INTO tools_registry
    (name, description, hemisphere, parameter_schema, handler_type, handler_ref,
     status, enabled, requires_approval)
VALUES
    ('read',
     'Read a UTF-8 file relative to the session workspace.',
     'external',
     '{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}',
     'go_native',
     'read_file',
     'active',
     true,
     false),
    ('write',
     'Write a file relative to the session workspace, creating parent directories.',
     'external',
     '{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}',
     'go_native',
     'write_file',
     'active',
     true,
     false),
    ('edit',
     'Replace an exact text fragment in a file relative to the session workspace.',
     'external',
     '{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["path","old_string","new_string"],"additionalProperties":false}',
     'go_native',
     'edit',
     'active',
     true,
     false),
    ('glob',
     'List workspace-relative paths matching a glob pattern.',
     'external',
     '{"type":"object","properties":{"pattern":{"type":"string"}},"required":["pattern"],"additionalProperties":false}',
     'go_native',
     'glob',
     'active',
     true,
     false),
    ('bash',
     'Run a shell command with the session workspace as its working directory.',
     'external',
     '{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}',
     'go_native',
     'bash',
     'active',
     true,
     false)
ON CONFLICT (name) DO UPDATE SET
    handler_type = EXCLUDED.handler_type,
    handler_ref = EXCLUDED.handler_ref,
    status = EXCLUDED.status,
    enabled = EXCLUDED.enabled,
    updated_at = CURRENT_TIMESTAMP;

COMMIT;
