---
description: Report a bug, usability issue, or improvement idea about Axiom itself directly to Jira.
agent: dispatch-axiom
---

File a self-report ticket in Jira for a Axiom platform issue discovered during agent execution.
This is the "agent-to-Jira" feedback loop — agents report problems they find so humans can fix them.

Use it when:
- You find a bug in a Axiom skill, command, or agent
- You encounter confusing instructions or poor defaults
- You want to suggest an improvement to the platform
- You're about to work around a known limitation and want to document it

**Scope**: Axiom platform issues only. Do NOT use this for issues in the user's codebase.

Inputs
- `$ARGUMENTS` required/optional:
  - `title="<brief description>"` — REQUIRED. Short description of the issue (under 80 chars).
  - `type=bug|usability|improvement|missing-feature|performance|security` — REQUIRED. Issue type.
  - `severity=critical|high|medium|low` — REQUIRED. How bad is it?
  - `description="<detailed description>"` — optional. Detailed description (overrides auto-generated).
  - `component=<name>` — optional. Which component is affected (e.g., `git-history-backfill`, `spec-extract`). Auto-detected from context if omitted.
  - `log=<path>` — optional. Path to a log file to attach (e.g., `.memory-bank/work-items/.../runs/.../builder.log`).
  - `epic=<jira-key>` — optional. Parent epic key. Read from `jira.incoming_epic` in `.axiom/axiom.config.yaml`. Default: `DEX-379` if config key is absent.
  - `work_item=<id>` — optional. Work item ID for context (auto-detected from `_current.md` if omitted).
  - `dry-run` — optional. Show what would be created without actually creating the ticket.

Skills (load on demand):
- `self-report-axiom` — Always load. Governs ticket structure, duplicate detection, Jira MCP integration.

Do
1) Load skill `.opencode/skills/self-report-axiom/SKILL.md`.
2) Parse `$ARGUMENTS` for title, type, severity, description, component, log, epic, work_item.
3) Auto-detect missing context:
   - `work_item`: read `.memory-bank/work-items/_current.md` for the active work item ID.
   - `component`: infer from the active work item or the log file path.
   - `run_id`: use the current timestamp-based run ID.
4) **Duplicate detection**: search Jira for existing tickets with similar titles.
   ```
   JQL: project = DEX AND summary ~ "<title>" AND status != Done ORDER BY created DESC
   ```
   - If a matching ticket exists: add a comment with new context; do NOT create a duplicate.
   - If no match: proceed to create.
5) **Build ticket content** (per `self-report-axiom` skill):
   - Summary: `[Self-Report] <component>: <title>`
   - Description: structured markdown with What Happened, Expected Behavior, Steps to Reproduce, Context, Debugging Context, Suggested Fix, Log File.
   - Labels: `axiom-self-report`, `<type>`, `<severity>`, `<component>`
   - Parent epic: `<epic>` (default: `DEX-299`)
   - Priority: derived from severity (critical→Highest, high→High, medium→Medium, low→Low)
6) If `dry-run`: show the ticket content and stop.
7) **Create or update the Jira ticket** using Atlassian MCP:
   - New ticket: `atlassian_createJiraIssue`
   - Existing ticket: `atlassian_addCommentToJiraIssue`
8) If Jira MCP is unavailable: write the report to `.memory-bank/inbox/MB-Steward/self-report-<timestamp>.md` and continue.
9) Report the result: ticket URL, action taken (created/commented), labels applied.

Stop conditions
- If `title` is missing: BLOCKED — title is required.
- If `type` is missing: BLOCKED — type is required.
- If `severity` is missing: BLOCKED — severity is required.
- If Jira MCP is unavailable: fall back to inbox message (do NOT block execution).
- If `dry-run`: show ticket content and stop.

Output (machine-consumable)
- Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`) with:
  - `<command>/axiom-report-issue</command>`
  - `<status>ok|fail|blocked</status>` — `ok` if ticket created/updated; `fail` if Jira error; `blocked` if required args missing
  - `<summary>` one sentence: action taken and ticket key/URL
  - `<detailed_summary>` ticket content summary: title, type, severity, labels, parent epic
  - `<evidence>` include:
    - `<jira_ticket_key>` the created/updated ticket key (e.g., `DEX-308`)
    - `<jira_ticket_url>` full URL to the ticket
    - `<action>` `created` or `commented_on_existing`
    - `<duplicate_found>` `true` if an existing ticket was found; `false` otherwise
    - `<log_attached>` path to log file included in description, or `none`
  - `<diagnostics>` for warnings (Jira MCP unavailable, duplicate found, log truncated)
  - `<review.assumptions>` assumptions made (e.g., auto-detected component, work item)
  - `<modify_plan>` false
  - `<memory_updates>` path to inbox message if Jira MCP was unavailable

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating whether the ticket was created or commented on, and the ticket key.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - Typically: `.memory-bank/inbox/MB-Steward/self-report-<timestamp>.md` if Jira MCP unavailable
- `evidence.jira_ticket_key`: the created/updated ticket key (e.g., `DEX-308`)
- `evidence.jira_ticket_url`: full URL to the ticket
- `evidence.action`: `created` or `commented_on_existing`
- `evidence.duplicate_found`: true|false
- `related_commands`: suggested follow-up commands
  - "To view the filed ticket, open: <jira_ticket_url>"
  - "To file another issue, run: `/axiom-report-issue title=... type=... severity=...`"

### Cross-References
- "Self-report skill is in: `.opencode/skills/self-report-axiom/SKILL.md`"
- "Spec: `specs/78-User-Feedback-And-Issue-Reporting.md`"
- "Jira integration spec: `specs/05-Jira-Integration.md`"

axiom:trace work_item=DEX-307 spec=specs/78-User-Feedback-And-Issue-Reporting.md,specs/05-Jira-Integration.md jira_ref=DEX-307,DEX-379 doc=.opencode/commands/axiom-report-issue.md commit=
