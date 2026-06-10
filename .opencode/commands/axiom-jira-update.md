---
description: Write back to a Jira ticket — post a progress comment, update a custom field, or transition the ticket status. Called by Axiom during execution to keep Jira in sync.
agent: pm-axiom
---

Post an update to a Jira ticket on behalf of Axiom. This command is called
during execution to mirror evidence, post progress comments, update custom fields
(Axiom Work-Item, Axiom Trace), and transition ticket status to match the
current execution state.

axiom:trace work_item=jira-comms-overhaul-01 spec=specs/05-Jira-Integration.md#comment-format plan=phase-3/task-3-2 jira_ref=SWDE-28

## Inputs

- `$JIRA_KEY` — Jira issue key (e.g. `SWDE-13`)
- `$UPDATE_TYPE` — what to update: `progress_comment` | `evidence_comment` | `pr_comment` | `blocker_comment` | `field_update` | `status_transition` | `question_comment` | `intake_comment` | `plan_comment` | `step_progress_comment`
- `$MESSAGE` — the comment body or field value to write (Markdown; converted to ADF automatically)
- `$NEW_STATUS` — target Jira status name (only for `status_transition`)
- `$FIELD_NAME` — Jira field name or ID to update (only for `field_update`)
- `$FIELD_VALUE` — new field value (only for `field_update`)
- `$PR_URL` — GitHub PR URL (only for `pr_comment`)
- `$WORK_ITEM_ID` — Axiom work item ID to write to `Axiom Work-Item` field
- `$TRACE_MARKER` — `axiom:trace` string to write to `Axiom Trace` field

Skills (load on demand):
- `jira-workflow-axiom` — Jira operating model, comment format, ADF structure. Load always.
- `jira-comment-writing-axiom` — Style guide for Jira comments. Load when writing comments.

## Update Types

### `progress_comment` — Execution progress update

Post a structured progress comment showing what step just completed:

```
🔄 **Axiom Progress Update** — {JIRA_KEY}

**Step completed**: {step_id} — {step_title}
**Status**: {ok|fail|blocked}
**Evidence**: {evidence_summary}

**Next step**: {next_step_id} — {next_step_title}

---
*Posted by Axiom automation · {timestamp}*
```

### `evidence_comment` — Evidence bundle summary

Post a summary of verification evidence after a phase completes:

```
✅ **Phase {N} Complete** — {JIRA_KEY}

**Tests**: {test_count} passed
**Quality gate**: score={score}, hard_fail={false|true}
**Runtime tier**: Tier {N}
**Commits**: {commit_sha}

**Files changed**:
- {file_1}
- {file_2}

---
*Posted by Axiom automation · {timestamp}*
```

### `pr_comment` — PR created notification

Post when a GitHub PR is opened:

```
🚀 **Pull Request Created** — {JIRA_KEY}

**PR**: [{pr_title}]({PR_URL})
**Branch**: `{branch_name}`
**Target**: `{target_branch}`

Please review and merge when ready. Axiom will monitor CI status and
fix any failing checks automatically.

---
*Posted by Axiom automation · {timestamp}*
```

### `blocker_comment` — Execution blocked

Post when Axiom hits a blocker it cannot resolve autonomously:

```
🚫 **Axiom Blocked** — {JIRA_KEY}

**Blocked at**: {step_id} — {step_title}
**Reason**: {blocker_reason}

**To unblock**:
{unblock_instructions}

Please reply to this comment when the blocker is resolved. Axiom will
resume automatically.

---
*Posted by Axiom automation · {timestamp}*
```

### `question_comment` — Clarifying questions

Post when Axiom needs more information before proceeding (used by `/axiom-jira-intake`):

```
👋 **Axiom needs clarification** — {JIRA_KEY}

Before starting, I need answers to the following:

1. {question_1}
2. {question_2}

Please reply to this comment with your answers. I'll resume automatically
when you respond.

---
*Posted by Axiom automation · {timestamp}*
```

### `intake_comment` — Pipeline starting notification

Post when the pipeline begins working on a ticket:

```
👋 **Axiom starting work on {JIRA_KEY}**

**What I understand:** {ticket_summary}
**Repo:** `{repo_url}`
**Branch:** `{branch_name}`

I'll post updates as I go. Starting intake now...

---
*Posted by Axiom automation · {timestamp}*
```

### `plan_comment` — Plan ready notification

Post when intake completes and implementation is starting:

```
📋 **Plan ready for {JIRA_KEY}**

Intake complete. I have a plan and I'm starting implementation now.

I'll post progress updates as each step completes.

---
*Posted by Axiom automation · {timestamp}*
```

### `step_progress_comment` — Step completion update

Post after the implementation loop completes with git context:

```
🔨 **Implementation complete for {JIRA_KEY}**

**Last commit:** {last_commit_message}
**Changes:** {diff_stat}
**Files changed:**
{file_list}

Pushing branch and creating PR now...

---
*Posted by Axiom automation · {timestamp}*
```

### `field_update` — Update a Jira custom field

Update one of the Axiom custom fields:

| Field | ID | When to update |
|---|---|---|
| Axiom Work-Item | `customfield_11572` | When work item is created |
| Axiom Trace | `customfield_11573` | When PR is merged |
| Axiom Repositories | `customfield_11906` | When repos are resolved |
| Axiom Branch | `customfield_11907` | When branch is created |

### `status_transition` — Transition ticket status

Move the ticket through the Jira workflow:

| Trigger | Target status |
|---|---|
| Work item created, execution starting | `In Progress` |
| PR opened | `In Review` (if available) |
| PR merged | `Done` |
| Execution blocked | keep current status, post blocker comment |
| Ticket cancelled by human | `Cancelled` (do not transition autonomously) |

## Execution

1. **Load Jira credentials** from environment (`AXIOM_JIRA_BASE_URL`, `AXIOM_JIRA_API_TOKEN`, etc.)

2. **For comments** (`progress_comment`, `evidence_comment`, `pr_comment`, `blocker_comment`, `question_comment`, `intake_comment`, `plan_comment`, `step_progress_comment`):
   - Convert `$MESSAGE` from Markdown to ADF format
   - POST to `/rest/api/3/issue/{JIRA_KEY}/comment`
   - Record comment ID in `.memory-bank/work-items/{JIRA_KEY}/jira-comments.yaml`

3. **For field updates** (`field_update`):
   - PUT to `/rest/api/3/issue/{JIRA_KEY}` with `{"fields": {"{FIELD_NAME}": "{FIELD_VALUE}"}}`
   - Log the update

4. **For status transitions** (`status_transition`):
   - GET `/rest/api/3/issue/{JIRA_KEY}/transitions` to find the transition ID for `$NEW_STATUS`
   - POST to `/rest/api/3/issue/{JIRA_KEY}/transitions` with the transition ID
   - Log the transition

5. **Auto-update standard fields** on every call:
   - If `$WORK_ITEM_ID` is set → update `customfield_11572` (Axiom Work-Item)
   - If `$TRACE_MARKER` is set → update `customfield_11573` (Axiom Trace)

## Fail Closed Rules

- If `$JIRA_KEY` is missing → `status=blocked`
- If Atlassian MCP or Jira credentials are unavailable → log "Jira sync deferred", write to `.memory-bank/work-items/{JIRA_KEY}/jira-sync-queue.yaml` for retry, return `status=ok` (non-fatal — execution continues)
- If a status transition fails (transition not available) → log warning, do not fail execution
- Never post a comment that contains secrets, tokens, or internal stack traces
- Never transition a ticket to `Done` unless a PR has been merged or the work is explicitly verified complete

## Output Contract

### For Human Consumption
- Summary: one sentence — what was posted or updated in Jira.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.jira_key`: the Jira ticket key updated
- `evidence.update_type`: the type of update performed
- `evidence.comment_id`: Jira comment ID (when a comment was posted)
- `evidence.transition_applied`: status transition applied (or empty)
- `evidence.fields_updated`: semicolon-separated list of field IDs updated
- `related_commands`:
  - "To check ticket state, run: `/axiom-sync-jira {JIRA_KEY}`"
  - "To post a blocker, run: `/axiom-jira-update --jira-key {JIRA_KEY} --type blocker_comment --message '...'`"

### Cross-References
- "Comment log is at: `.memory-bank/work-items/{JIRA_KEY}/jira-comments.yaml`"
- "Sync queue is at: `.memory-bank/work-items/{JIRA_KEY}/jira-sync-queue.yaml`"
- "Spec: `specs/05-Jira-Integration.md`"
- "Comment format: `specs/05-Jira-Integration.md#comment-format`"

## Output Format

Emit a `<axiom>` XML envelope per `specs/04-XML-Protocol.md`:

```xml
<axiom>
  <run>
    <run_id>$RUN_ID</run_id>
    <work_item_id>$JIRA_KEY</work_item_id>
    <intake_source>jira_event</intake_source>
  </run>
  <command>/axiom-jira-update</command>
  <status>ok|fail|blocked</status>
  <confidence>0-100</confidence>
  <summary>One sentence — what was posted or updated in Jira</summary>
  <detailed_summary>Update type, what was written, any errors encountered</detailed_summary>
  <evidence>
    <jira_key>$JIRA_KEY</jira_key>
    <update_type>$UPDATE_TYPE</update_type>
    <comment_id>optional — Jira comment ID if comment was posted</comment_id>
    <transition_applied>optional — status transition applied</transition_applied>
    <fields_updated>optional — semicolon-separated field IDs updated</fields_updated>
  </evidence>
  <diagnostics>warnings or errors</diagnostics>
</axiom>
```

See: `specs/05-Jira-Integration.md`, `specs/14-Integrations-Jira-GitHub.md`, `specs/04-XML-Protocol.md`
