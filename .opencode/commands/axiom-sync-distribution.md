---
description: Sync distribution artifacts (installer, manifests, version file, template sync).
agent: tower-axiom
---

Sync and repair Axiom distribution artifacts so installs and template sync stay correct.

Skills (load on demand):
- `axiom-xml-protocol` — XML envelope format and required tag set.
- `axiom-repository-scaffold` — Scaffold file manifest, template contents, and validation rules. Load when repairing MANIFEST.md or installer drift.
- `enterprise-release-quality` — Release quality gates and checklist. Load when validating distribution artifacts before a release.

Do
1) Validate installer still runs:
   - `python3 -m py_compile .axiom/scaffold/install.py`
   - Run a dry-run install to `/tmp/` and confirm counts match docs.

2) Validate distribution version manifest:
   - `python3 scripts/generate_version.py --check`
   - If stale, regenerate: `python3 scripts/generate_version.py`

3) Validate template sync:
   - `python3 scripts/sync_template.py --dry-run`
   - If needed, sync: `python3 scripts/sync_template.py --force`

4) Repair drift:
   - Update `.axiom/scaffold/MANIFEST.md` counts/sections.
   - Update any docs that mention old file counts.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope.
- Use:
  - `<command>/axiom-sync-distribution</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence
  - `<evidence><commands_ran>` list the commands you actually ran (semicolon-separated)
  - `<evidence><files_changed>` list of files modified (semicolon-separated)
  - `<diagnostics>` for warnings/errors

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating which distribution artifacts were validated/repaired.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `.axiom/scaffold/MANIFEST.md`, `.axiom/.version.md` if regenerated
- `evidence.commands_ran`: list of validation commands actually executed
- `evidence.installer_valid`: true|false — whether installer compiled successfully
- `evidence.version_manifest_valid`: true|false — whether version manifest is current
- `related_commands`: suggested follow-up commands
  - "To run the full sync suite, run: `/axiom-sync-all`"
  - "To validate version manifest only, run: `/axiom-sync-version-manifest`"

### Cross-References
- "Scaffold manifest is at: `.axiom/scaffold/MANIFEST.md`"
- "Version manifest is at: `.axiom/.version.md`"
- "To prepare a release, run: `/axiom-sync-all` then check with `@release-manager-axiom`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
