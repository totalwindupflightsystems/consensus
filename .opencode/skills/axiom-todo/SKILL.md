---
name: axiom-todo
description: Maintain `.memory-bank/TODO.md` as the specs-aligned roadmap.
version: "1.1"
tags:
  vertical: [planning]
  category: planning
  core: false
---

Use this skill when you need to update the project roadmap in `.memory-bank/TODO.md`.

Preferred mechanism:
- Run the slash command `/axiom-todo`.

What "good" looks like:
- TODO stays aligned with `specs/`.
- Baby Steps checkboxes.
- Clear "Start Here" section.
- Each task has "Spec refs" and "Done evidence" when appropriate.

## Jira Ticket Tracking in TODO (REQUIRED when Jira is configured)

When the repo has Jira integration configured (`.axiom/axiom.config.yaml` → `jira.project_key` is set), the TODO MUST integrate Jira awareness so that work items are traceable between the TODO roadmap and Jira.

### Rules

1. **Jira key as work item ID**: When a TODO item maps to a Jira ticket, use the Jira key as the work item identifier (e.g., `PROJ-123`) rather than a slug. This ensures trace markers, memory bank paths, and Jira comments all use the same identifier.

2. **Jira reference in TODO items**: Each TODO item that maps to a Jira ticket SHOULD include the Jira key inline:
   ```markdown
   - [ ] **PROJ-123**: Add rate limiting to /login endpoint
     - Spec refs: `specs/auth.md#rate-limiting`
     - Done evidence: Integration tests pass; Jira ticket transitioned to Human Review
   ```

3. **Done evidence includes Jira state**: When a TODO item is marked complete, the "Done evidence" SHOULD note the expected Jira ticket state (e.g., "Jira ticket in Human Review" or "Jira ticket Done").

4. **New work items from Jira**: When new Jira tickets are assigned to Axiom, they SHOULD be reflected in the TODO as new items (either manually or via the planning process). The TODO is the repo-local roadmap; Jira is the external tracking system.

5. **Archiving completed items**: When archiving completed TODO blocks (per `specs/45-TODO-Lifecycle-And-Archive.md`), include the Jira key in the archive metadata so historical queries can cross-reference Jira.

### Non-Jira Work Items

When `jira.project_key` is `null` or the work item is not Jira-sourced:
- Use stable slugs as work item IDs (e.g., `bootstrapping-01`)
- Omit Jira-specific fields
- All other TODO rules still apply

References:
- `specs/05-Jira-Integration.md` (ticket-as-work-unit)
- `specs/45-TODO-Lifecycle-And-Archive.md` (TODO lifecycle and archive)
- `specs/21-Traceability-Doctrine.md#external-reference-fields` (jira_ref in trace markers)
