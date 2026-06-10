---
description: Sync axiom-template/ from this repo.
agent: tower-axiom
---

Sync `axiom-template/` so the GitHub template stays up to date.

Skills (load on demand):
- `axiom-xml-protocol` — XML envelope format and required tag set.
- `axiom-repository-scaffold` — Scaffold file manifest and template contents. Load when validating template sync.

Do
1) Run `python3 scripts/sync_template.py --dry-run` and inspect missing files.
2) If needed, run `python3 scripts/sync_template.py --force`.
3) Report template repo git status and list uncommitted changes.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope with:
  - `<command>/axiom-sync-template</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>`
  - `<evidence><commands_ran>` commands actually executed
  - `<evidence><files_changed>` list of ALL files created/modified (full paths, semicolon-separated)
  - `<diagnostics>`

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating whether the template was already in sync or was updated.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified in `axiom-template/` (full paths, semicolon-separated)
- `evidence.commands_ran`: list of commands actually executed
- `evidence.was_in_sync`: true|false — whether the template was already current
- `related_commands`: suggested follow-up commands
  - "To run the full sync suite, run: `/axiom-sync-all`"
  - "To validate distribution artifacts, run: `/axiom-sync-distribution`"

### Cross-References
- "Template sync script is at: `scripts/sync_template.py`"
- "Template directory is at: `axiom-template/`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
