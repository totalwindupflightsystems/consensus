---
description: Sync .axiom/command-registry.yaml with installed /commands.
agent: tower-axiom
---

Sync `.axiom/command-registry.yaml` so it covers the installed `.opencode/commands/*.md` set.

Skills (load on demand):
- `axiom-command-registry` — Command registry YAML schema, required fields, tag prompts, and validation rules. Always load this skill before editing the registry.
- `axiom-xml-protocol` — XML envelope format and required tag set.

Do
1) List commands present in `.opencode/commands/`.
2) Read `.axiom/command-registry.yaml`.
3) Ensure there is an entry for each Axiom `/axiom-*` command.
4) Remove stale entries for commands that no longer exist.
5) Keep required tags minimal and consistent with `.opencode/skills/axiom-xml-protocol/SKILL.md`.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope with:
  - `<command>/axiom-sync-command-registry</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>`
  - `<evidence><files_changed>` — always `.axiom/command-registry.yaml` if changed
  - `<diagnostics>`

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many entries were added/removed/unchanged.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: `.axiom/command-registry.yaml` if modified (full path)
- `evidence.entries_added`: count of new command entries added
- `evidence.entries_removed`: count of stale entries removed
- `evidence.entries_unchanged`: count of entries already in sync
- `related_commands`: suggested follow-up commands
  - "To run the full sync suite, run: `/axiom-sync-all`"
  - "To verify command registry integrity, check: `.axiom/command-registry.yaml`"

### Cross-References
- "Registry schema is defined in: `.opencode/skills/axiom-command-registry/SKILL.md`"
- "To add a new command to the registry, follow the agent checklist in the `axiom-command-registry` skill."

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
