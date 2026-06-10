---
name: self-report-axiom
description: >
  Lets agents report bugs, usability issues, and improvement ideas directly to Jira.
  Attaches log files, debugging context, labels, and parent epic linking.
  Use this skill when an agent discovers a problem with Axiom itself during execution.
version: "1.0"
created: "2026-04-04"
primary_spec: specs/05-Jira-Integration.md
secondary_specs:
  - specs/13-Command-Registry.md
  - specs/21-Traceability-Doctrine.md
jira_ref: DEX-307
tags:
  vertical: [coding, planning]
  category: tooling
  core: false
---

# Self-Report Skill

> **"If you find a bug in the tool you're using, report it. Don't just work around it."**

This skill enables Axiom agents to report bugs, usability issues, and improvement ideas
directly to Jira — without requiring a human to manually file the ticket. It's the
"agent-to-Jira" feedback loop that makes Axiom self-improving.

axiom:trace work_item=DEX-307 spec=specs/05-Jira-Integration.md jira_ref=DEX-307 plan=phase-1/task-1-4/step-1-4-1 doc=.opencode/skills/self-report-axiom/SKILL.md commit=

---

## Activation

Load this skill when:
- An agent encounters a bug in a Axiom skill, command, or agent
- An agent finds a usability issue (confusing instructions, missing context, poor defaults)
- An agent wants to suggest an improvement to the platform
- Running `/axiom-report-issue` command
- An agent is about to work around a known limitation and wants to document it

**When NOT to load this skill**:
- Reporting issues in the user's codebase (not Axiom itself)
- Filing tickets for user-requested features (use normal Jira workflow)
- Reporting security vulnerabilities (use `@security-review-axiom` instead)

---

## Issue Types

| Type | When to Use | Jira Issue Type | Default Priority |
|------|-------------|-----------------|-----------------|
| `bug` | Something is broken or produces wrong output | Bug | Medium |
| `usability` | Instructions are confusing, defaults are wrong, UX is poor | Improvement | Low |
| `improvement` | A feature could work better | Improvement | Low |
| `missing-feature` | A needed capability doesn't exist | Story | Low |
| `performance` | Something is too slow or uses too much context | Improvement | Medium |
| `security` | A security concern (non-critical; critical → use security-review) | Bug | High |

---

## Severity Levels

| Severity | Criteria | Jira Priority |
|----------|----------|---------------|
| `critical` | Blocks all work; data loss risk; security vulnerability | Highest |
| `high` | Blocks specific workflow; incorrect output that misleads agents | High |
| `medium` | Degrades quality; workaround exists but is painful | Medium |
| `low` | Minor annoyance; cosmetic; nice-to-have improvement | Low |

---

## Duplicate Detection

Before creating a ticket, search Jira for existing tickets with similar titles:

```
JQL: project = DEX AND summary ~ "<title>" AND status != Done ORDER BY created DESC
```

If a matching ticket exists:
- Add a comment to the existing ticket with the new context
- Do NOT create a duplicate
- Report the existing ticket URL in the command output

---

## Ticket Structure

Every self-reported ticket MUST include:

### Summary (title)
- Format: `[Self-Report] <component>: <brief description>`
- Example: `[Self-Report] git-history-backfill: SKIP classification misses merge commits`
- Keep under 80 characters

### Description (ADF or Markdown)
```markdown
## What Happened
<concrete description of the issue — what the agent observed>

## Expected Behavior
<what should have happened>

## Steps to Reproduce
1. <step 1>
2. <step 2>
3. <step 3>

## Context
- **Work item**: <work_item_id>
- **Run ID**: <run_id>
- **Agent**: <agent_name>
- **Command**: <command that triggered the issue>
- **Repo**: <repo name>
- **Date**: <ISO 8601 date>

## Debugging Context
<relevant excerpts from logs, error messages, or unexpected outputs>
<redact any secrets or PII>

## Suggested Fix (optional)
<if the agent has a hypothesis about the fix>

## Log File
<path to attached log file, or "No log available">
```

### Labels
Always include:
- `axiom-self-report` — marks this as an agent-filed ticket
- `<type>` — e.g., `bug`, `usability`, `improvement`
- `<severity>` — e.g., `high`, `medium`, `low`
- `<component>` — e.g., `git-history-backfill`, `spec-extract`, `memory-bank`

### Parent Epic
- Always link to the parent epic (default: `DEX-299` for onboarding issues)
- Use the `Epic Link` field or `parent` field depending on Jira project config

---

## Log File Attachment

When a log file is available (e.g., a builder log or verification.md):
1. Identify the most relevant log file (prefer the current run's builder log)
2. Truncate to the last 200 lines if the file is large (to avoid Jira attachment limits)
3. Include the log path in the ticket description
4. Note: Jira MCP may not support direct file attachment — include the log path and key excerpts inline

---

## Jira MCP Integration

Use the Atlassian MCP tools to create the ticket:

```
1. Search for duplicates:
   atlassian_searchJiraIssuesUsingJql(
     jql="project = DEX AND summary ~ '<title>' AND status != Done",
     fields=["summary", "status", "key"]
   )

2. If no duplicate, create the ticket:
   atlassian_createJiraIssue(
     projectKey="DEX",
     issueTypeName="Bug" | "Improvement" | "Story",
     summary="[Self-Report] <component>: <description>",
     description="<structured description above>",
     additional_fields={
       "priority": {"name": "<priority>"},
       "labels": ["axiom-self-report", "<type>", "<severity>", "<component>"],
       "parent": {"key": "<epic_key>"}  # or Epic Link field
     }
   )

3. If duplicate found, add comment:
   atlassian_addCommentToJiraIssue(
     issueIdOrKey="<existing_key>",
     commentBody="<new context from this occurrence>"
   )
```

---

## Output

After creating or updating the ticket, report:
```
SELF-REPORT RESULT:
  Action: created | commented_on_existing
  Ticket: DEX-<number> (<title>)
  URL: https://dexdat.atlassian.net/browse/DEX-<number>
  Labels: axiom-self-report, bug, medium, git-history-backfill
  Parent: DEX-299
  Log attached: <path> | No log available
```

---

## Rules

1. **Always search for duplicates first** — do not create duplicate tickets.
2. **Redact secrets and PII** — never include tokens, passwords, or personal data in tickets.
3. **Be specific** — vague reports ("something is wrong") are not actionable. Include concrete steps to reproduce.
4. **Include context** — work item ID, run ID, agent name, and command are required.
5. **Use the `axiom-self-report` label** — this is how humans filter agent-filed tickets.
6. **Link to parent epic** — default is DEX-299 for onboarding issues; use the most specific applicable epic.
7. **Don't block on reporting** — if Jira MCP is unavailable, log the issue to `.memory-bank/inbox/MB-Steward/` and continue.
8. **One ticket per distinct issue** — don't bundle multiple issues into one ticket.

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|-------------|---------|-----|
| Creating a ticket without searching for duplicates | Clutters Jira with duplicates | Always search first |
| Vague title like "backfill broken" | Not actionable | Use `[Self-Report] <component>: <specific behavior>` |
| Including secrets in the description | Security risk | Redact as `[REDACTED]` |
| Blocking work to file a report | Slows down execution | File the report, then continue working |
| Filing tickets for user codebase issues | Wrong scope | Only report Axiom platform issues |
| Not linking to parent epic | Orphaned ticket | Always link to DEX-299 or more specific epic |
