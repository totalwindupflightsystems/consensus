---
description: Sync specs inventory files (specs/README.md + specs/_index.md).
agent: tower-axiom
---

Sync `specs/` inventory so agents can discover the contract surface.

Skills (load on demand):
- `axiom-xml-protocol` — XML envelope format and required tag set.
- `spec-writing-axiom` — Spec style guide and required sections. Load when creating or repairing spec inventory files.

Do
1) Ensure `specs/` exists.
2) Ensure these exist:
   - `specs/README.md`
   - `specs/_index.md`
   - `specs/_prompt.md`
3) List `specs/*.md` excluding `_index.md` and `_prompt.md`.
4) Ensure `specs/README.md` inventory includes the spec files.
5) Ensure `specs/_index.md` has up-to-date links.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope with:
  - `<command>/axiom-sync-specs-inventory</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>`
  - `<evidence><files_changed>` list of ALL files created/modified (full paths, semicolon-separated)
  - `<diagnostics>`

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many spec files are now indexed.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `specs/README.md`, `specs/_index.md` if updated
- `evidence.specs_indexed`: count of spec files now in the inventory
- `evidence.specs_added_to_index`: count of spec files newly added to the index
- `related_commands`: suggested follow-up commands
  - "To run the full sync suite, run: `/axiom-sync-all`"
  - "To extract specs from existing code, run: `/axiom-spec-extract`"

### Cross-References
- "Spec inventory is at: `specs/README.md`"
- "Spec index is at: `specs/_index.md`"
- "To add a new spec, follow the style guide in: `.opencode/skills/spec-writing-axiom/SKILL.md`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
