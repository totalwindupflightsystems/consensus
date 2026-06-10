---
description: Run all sync commands (indexes, traceability, distribution).
agent: tower-axiom
---

Run the standard Axiom sync suite.

Skills (load on demand):
- `axiom-xml-protocol` — XML envelope format and required tag set.
- `traceability-doctrine` — Trace marker format and required links (used by sync-trace sub-step).
- `axiom-command-registry` — Command registry schema and validation rules (used by sync-command-registry sub-step).

Do
1) Run `/axiom-sync-indexes`.
2) Run `/axiom-sync-trace`.
3) Run `/axiom-sync-command-registry`.
4) Run `/axiom-sync-specs-inventory`.
5) Run `/axiom-sync-memory-bank-core`.
6) Run `/axiom-sync-work-items`.
7) Run `/axiom-sync-version-manifest`.
8) Run `/axiom-sync-distribution`.
9) Run `/axiom-sync-template`.
10) Run `/axiom-sync-jira` (if Atlassian MCP is available; skip gracefully if not).

Fail closed
- If any step is `blocked` or `fail`, stop and report it; do not continue.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope.
- Use:
  - `<command>/axiom-sync-all</command>`
  - `<status>ok|fail|blocked</status>`
  - `<summary>` one sentence
  - `<evidence><files_changed>` union of all files changed by sub-steps (semicolon-separated)
  - `<diagnostics>` include which sub-step failed/blocked

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many sub-steps ran and whether any failed.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: union of all files created/modified across all sub-steps (full paths, semicolon-separated)
- `evidence.steps_ran`: list of sub-commands that executed successfully
- `evidence.steps_failed`: list of sub-commands that failed or were blocked (empty if all passed)
- `related_commands`: suggested follow-up commands
  - "To audit trace completeness after sync, run: `/axiom-verify --work-item current`"
  - "To commit the synced changes, run: `/axiom-batch-commit`"

### Cross-References
- Sub-commands: `/axiom-sync-indexes`, `/axiom-sync-trace`, `/axiom-sync-command-registry`, `/axiom-sync-specs-inventory`, `/axiom-sync-memory-bank-core`, `/axiom-sync-work-items`, `/axiom-sync-version-manifest`, `/axiom-sync-distribution`
- "To run only a specific sync, call the individual sub-command directly."

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
