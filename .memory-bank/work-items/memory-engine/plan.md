# memory-engine — Layer 4: Append-only ledger, context views, pages, commits

## Goal
Implement and verify SPEC-002 memory engine features (AC-020 through AC-025).

## Affected ACs
- AC-020: memory_events append-only — UPDATE/DELETE rejected
- AC-021: Active context view returns formatted markdown
- AC-022: Memory pages — create, resolve, deduplicate
- AC-023: Iteration commits — snapshot and rollback
- AC-024: Display mode compression — summary_text substitution
- AC-025: Markdown generation — type-to-markdown mapping

## Specs
- specs/002-memory.md (full spec)
- specs/003-database.md §2.2-2.4

## Steps
1. Verify memory_events has no UPDATE/DELETE grants for agent_role (REVOKE already in schema)
2. Test active_context_view — create session with events, query view, assert markdown format
3. Test memory_pages — INSERT page with target_ids, query view with page resolution, assert dedup
4. Test iteration_commits — run 3 iterations, query commit snapshots, verify rollback
5. Test display_modes — set mode=compressed, verify summary_text substitution in view
6. Test markdown generation — insert events of each type, verify rendered format
