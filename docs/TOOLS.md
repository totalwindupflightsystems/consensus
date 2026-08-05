# Tools & Skills

Consensus exposes tools and skills to agents through the registry tables
(`tools_registry`, `skills_registry`) and the API/CLI surfaces:

- `GET /api/v1/tools` / `GET /api/v1/skills`
- `consensus tool list` / `consensus skill list`

## On a fresh install

Registries start **empty**. This is expected — there are no built-in tools
registered at init. The API and CLI both return empty lists (the CLI prints a
hint pointing at this document).

## Registering a tool

Tools are rows in `tools_registry`. Insert a row to register one, e.g.:

```sql
INSERT INTO tools_registry (name, description, hemisphere, handler_type, requires_approval)
VALUES ('sql_query', 'Run a read-only SQL query against the workspace', 'right', 'sql', false);
```

Required columns and CHECK constraints are defined in the migration
(`internal/migrate/migrations/`). After registering, `consensus tool list` and
`GET /api/v1/tools` will show the tool.

## Installing a skill

Skills are rows in `skills_registry`:

```sql
INSERT INTO skills_registry (name, instructions, metadata, linked_tool_ids)
VALUES ('my-skill', 'Step-by-step instructions for the agent...', '{}', NULL);
```

The `instructions` text is injected into the agent context when the skill is
used; `linked_tool_ids` binds prerequisite tools.

## Programmatic registration

The harness also supports creating tools at runtime — see
`internal/tools/` for the registry-backed registration path used by agent
sessions (`custom_agent_tools`).
