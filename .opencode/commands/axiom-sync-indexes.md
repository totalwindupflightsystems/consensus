---
description: Sync and repair `_index.md` inventories (Memory Bank + specs).
agent: tower-axiom
---

Sync `_index.md` files so inventories do not drift.

Inputs
- Optional: `$ARGUMENTS` = space-separated list of `_index.md` paths to check.

Default scope (if no args)
- `.memory-bank/**/_index.md`
- `specs/_index.md` (if present)

Skills (load on demand):
- `axiom-xml-protocol` — XML envelope format and required tag set.
- `memory-bank-axiom` — Memory bank folder structure, index format, and navigation rules. Load when repairing or creating new index files.

Do
1) Determine target index paths:
   - If `$ARGUMENTS` is non-empty, treat those as the list.
   - Else glob the default scope.

2) For each index path (stable order):
   - Read the file.
   - List the sibling folder contents.
   - Update the index to include:
     - new items that exist on disk but are missing from the index
     - remove references that no longer exist
   - Keep edits minimal and deterministic.

3) Update parent indexes when needed:
   - If you add a new index or folder, ensure the parent `_index.md` links to it.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`).
- Use:
  - `<command>/axiom-sync-indexes</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence
  - `<evidence><files_changed>` list of `_index.md` files updated (semicolon-separated)
  - `<diagnostics>` for warnings/errors

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many index files were updated.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL `_index.md` files created/modified (full paths, semicolon-separated)
- `evidence.indexes_updated`: count of index files that had changes
- `evidence.indexes_unchanged`: count of index files already in sync
- `related_commands`: suggested follow-up commands
  - "To run the full sync suite, run: `/axiom-sync-all`"
  - "To verify memory bank structure, run: `/axiom-sync-memory-bank-core`"

### Cross-References
- "Memory bank index rules are in: `.memory-bank/_prompt.md`"
- "To add a new memory bank folder, follow the rules in `.opencode/skills/memory-bank-axiom/SKILL.md`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
