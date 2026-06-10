# /axiom-schedule

Manage Axiom scheduled execution entries from within an active OpenCode session.

This command provides the same functionality as `axiom schedule` CLI but accessible directly from the OpenCode conversation.

## Usage

```
/axiom-schedule list
/axiom-schedule add "<cron>" <type> <target> [--name <name>]
/axiom-schedule validate
/axiom-schedule next [N]
/axiom-schedule history [N]
/axiom-schedule run <name>
/axiom-schedule enable <name>
/axiom-schedule disable <name>
/axiom-schedule remove <name>
```

## Actions

| Action | Description |
|--------|-------------|
| `list` | List all schedule entries with status (active/disabled/due/waiting) |
| `add "<cron>" <type> <target>` | Add a new entry to `.axiom/schedules.yaml` |
| `validate` | Validate `.axiom/schedules.yaml` syntax |
| `next [N]` | Show the next N scheduled executions (default 5) |
| `history [N]` | Show the last N execution results (default 10) |
| `run <name>` | Execute an entry immediately (ad-hoc, regardless of schedule) |
| `enable <name>` | Re-enable a disabled entry |
| `disable <name>` | Disable an entry (skips execution until re-enabled) |
| `remove <name>` | Remove an entry from the schedule file |

## Examples

```
# See what's scheduled
/axiom-schedule list

# Add a daily spec conformance sweep
/axiom-schedule add "@daily" command axiom-spec-sweep --name daily-sweep

# Run a scheduled entry immediately
/axiom-schedule run daily-sweep

# Show next 10 upcoming executions
/axiom-schedule next 10

# Validate the schedule file
/axiom-schedule validate
```

## Schedule Expression Syntax

| Expression | Meaning |
|------------|---------|
| `* * * * *` | Standard 5-field cron (minute hour dom month dow) |
| `@hourly` | Every hour |
| `@daily` | Every day at midnight UTC |
| `@weekly` | Every Sunday at midnight UTC |
| `@monthly` | First day of each month |
| `@every 15m` | Every 15 minutes |
| `@every 2h30m` | Every 2 hours 30 minutes |
| `@once 2026-04-15T14:00:00Z` | One-time execution at specified UTC datetime |

## Target Types

| Type | Description |
|------|-------------|
| `command` | OpenCode slash command (e.g., `axiom-spec-sweep`) |
| `agent` | OpenCode agent invocation (e.g., `qa-axiom`) |
| `script` | Bash command/script (executed in repo root) |
| `tool` | Named L6 tool from `.axiom/plugin.yaml` |

## Spec Reference

`specs/76-Scheduled-Execution-And-Ad-Hoc-Tools.md#REQ-SCHED-012`

## Skills (load on demand)

- `axiom-xml-protocol` — XML envelope format and required tag set.

## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating which schedule action ran and its result.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL files created/modified (full paths, semicolon-separated)
  - For `add`/`remove`/`enable`/`disable`: `.axiom/schedules.yaml`
  - For `list`/`next`/`history`/`validate`/`run`: empty (read-only or execution)
- `evidence.action`: which action was performed (list|add|validate|next|history|run|enable|disable|remove)
- `evidence.schedule_name`: the schedule entry name (for add/run/enable/disable/remove)
- `related_commands`: suggested follow-up commands
  - "To list all schedules, run: `/axiom-schedule list`"
  - "To validate the schedule file, run: `/axiom-schedule validate`"
  - "To see upcoming executions, run: `/axiom-schedule next 10`"

### Cross-References
- "Schedule file is at: `.axiom/schedules.yaml`"
- "Spec: `specs/76-Scheduled-Execution-And-Ad-Hoc-Tools.md#REQ-SCHED-012`"

axiom:trace work_item=DEX-285 spec=specs/76-Scheduled-Execution-And-Ad-Hoc-Tools.md#REQ-SCHED-012 plan=phase-D/task-D-2/step-D-2-1 jira_ref=DEX-285 work_item=command-quality-01
