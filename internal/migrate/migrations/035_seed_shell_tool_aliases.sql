-- ============================================================================
-- Consensus — 035_seed_shell_tool_aliases.sql
-- ============================================================================
-- DF-CONSENSUS-85: on a fresh install the registry seeds `bash` (handler_ref
-- 'bash'), but models naturally reach for shell_exec / shell / shell_execute /
-- run_shell / run_command. Before this migration every such first tool call
-- failed closed with `tool "shell_exec" is not registered or enabled` and the
-- first real task was lost.
--
-- Mechanism: seed each alias as its OWN registry row whose handler_ref points
-- at the proven native bash handler (same seed-row pattern as 033/034). The
-- exact-name arm of ResolveToolRegistryRow resolves them on the first attempt,
-- and the rows remain individually enable/disable-able by the operator.
--
-- This is a data-only, idempotent migration. Existing operator registrations
-- (including an operator-defined row named shell/shell_exec/…) are left
-- untouched by ON CONFLICT (name) DO NOTHING. The `bash` row itself is not
-- modified; a disabled bash row and these alias rows are independent registry
-- entries, exactly like every other builtin row.
-- ============================================================================

BEGIN;

INSERT INTO tools_registry
    (name, description, hemisphere, parameter_schema, handler_type, handler_ref,
     status, enabled, requires_approval)
VALUES
    ('shell_exec',
     'Run a shell command with the session workspace as its working directory.',
     'external',
     '{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}',
     'go_native',
     'bash',
     'active',
     true,
     false),
    ('shell',
     'Run a shell command with the session workspace as its working directory.',
     'external',
     '{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}',
     'go_native',
     'bash',
     'active',
     true,
     false),
    ('shell_execute',
     'Run a shell command with the session workspace as its working directory.',
     'external',
     '{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}',
     'go_native',
     'bash',
     'active',
     true,
     false),
    ('run_shell',
     'Run a shell command with the session workspace as its working directory.',
     'external',
     '{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}',
     'go_native',
     'bash',
     'active',
     true,
     false),
    ('run_command',
     'Run a shell command with the session workspace as its working directory.',
     'external',
     '{"type":"object","properties":{"command":{"type":"string"}},"required":["command"],"additionalProperties":false}',
     'go_native',
     'bash',
     'active',
     true,
     false)
ON CONFLICT (name) DO NOTHING;

COMMIT;
