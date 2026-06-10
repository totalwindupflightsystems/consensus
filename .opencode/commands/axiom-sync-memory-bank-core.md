---
description: Ensure Memory Bank core files + folder prompts/indexes exist and are linked.
agent: tower-axiom
---

Sync Memory Bank baseline structure and navigation.

Skills (load on demand):
- `axiom-xml-protocol` — XML envelope format and required tag set.
- `memory-bank-axiom` — Memory bank folder structure, required files, index format, and navigation rules. Always load this skill before creating or repairing memory bank files.

Do
1) Validate required core files exist:
   - `.memory-bank/_index.md`, `.memory-bank/_prompt.md`
   - `.memory-bank/TODO.md`
   - `.memory-bank/projectBrief.md`, `productContext.md`, `systemPatterns.md`, `techContext.md`, `decisionLog.md`, `activeContext.md`, `progress.md`
2) Validate required folder prompts/indexes exist:
   - `.memory-bank/work-items/_index.md`, `.memory-bank/work-items/_prompt.md`
   - `.memory-bank/implementation-plans/_index.md`, `.memory-bank/implementation-plans/_prompt.md`
3) Ensure `.memory-bank/_index.md` links to these subfolders.
4) Create missing files using the scaffold templates as the portable defaults.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope with:
  - `<command>/axiom-sync-memory-bank-core</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>`
  - `<evidence><files_changed>` list of ALL files created/modified (full paths, semicolon-separated)
  - `<diagnostics>`

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many core files were created/repaired.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
- `evidence.files_created`: count of new files created
- `evidence.files_repaired`: count of existing files that were updated
- `evidence.files_ok`: count of files already in correct state
- `related_commands`: suggested follow-up commands
  - "To sync all indexes after core repair, run: `/axiom-sync-indexes`"
  - "To run the full sync suite, run: `/axiom-sync-all`"

### Cross-References
- "Memory bank rules are in: `.memory-bank/_prompt.md`"
- "Scaffold templates are in: `.axiom/scaffold/`"
- "To bootstrap a new repo, run: `/axiom-init` then `/axiom-bootstrap`"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
