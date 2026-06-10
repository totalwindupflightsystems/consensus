---
description: Validate and regenerate .axiom/.version.md if stale.
agent: tower-axiom
---

Validate and sync `.axiom/.version.md`.

Skills (load on demand):
- `axiom-xml-protocol` — XML envelope format and required tag set.
- `enterprise-release-quality` — Release quality gates. Load when validating version manifest as part of a release preparation.

Do
1) Run `python3 scripts/generate_version.py --check`.
2) If stale, regenerate: `python3 scripts/generate_version.py`.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope with:
  - `<command>/axiom-sync-version-manifest</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>`
  - `<evidence><commands_ran>` commands actually executed (semicolon-separated)
  - `<evidence><files_changed>` `.axiom/.version.md` if regenerated
  - `<diagnostics>`

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating whether the version manifest was already current or was regenerated.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: `.axiom/.version.md` if regenerated (full path); empty if already current
- `evidence.commands_ran`: list of commands actually executed
- `evidence.was_stale`: true|false — whether the manifest needed regeneration
- `related_commands`: suggested follow-up commands
  - "To run the full sync suite, run: `/axiom-sync-all`"
  - "To validate distribution artifacts, run: `/axiom-sync-distribution`"

### Cross-References
- "Version manifest is at: `.axiom/.version.md`"
- "Version generation script is at: `scripts/generate_version.py`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
