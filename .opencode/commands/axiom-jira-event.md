---
description: Route an incoming Jira event (comment, field change, status transition) back to the correct Axiom work item (comment, field change, status transition) and route it to the correct work item. Resumes waiting work items when humans reply to questions.
agent: tower-axiom
---

Process an incoming Jira update and route it to the correct Axiom work item.
This command is called by the Jira automation pipeline when a ticket receives
a new comment, field update, or status transition. Its primary job is to wake
up work items that are in `waiting_for_reply` state when the human responds.

axiom:trace work_item=jira-coder-pipeline-01 spec=specs/05-Jira-Integration.md,specs/44-Autonomous-Intake-And-Lifecycle.md plan=phase-4 jira_ref=DEX-436

## Inputs

- `$JIRA_KEY` — Jira issue key (e.g. `SWDE-13`)
- `$EVENT_TYPE` — type of update: `comment_added` | `field_updated` | `status_changed` | `assignee_changed`
- `$COMMENT_BODY` — comment text (when `$EVENT_TYPE=comment_added`)
- `$COMMENT_AUTHOR_NAME` — display name of the commenter
- `$COMMENT_AUTHOR_EMAIL` — email of the commenter
- `$FIELD_NAME` — field that changed (when `$EVENT_TYPE=field_updated`)
- `$FIELD_NEW_VALUE` — new field value
- `$NEW_STATUS` — new ticket status (when `$EVENT_TYPE=status_changed`)

Skills (load on demand):
- `jira-workflow-axiom` — Jira operating model, comment format, ADF structure. Load always.
- `axiom-autonomous-intake` — Lifecycle state machine for intake/update/refinement. Load when routing updates.

## Decision Tree

### Phase 1: Load Work Item State

1. **Find the work item** for `$JIRA_KEY`:
   - Check `.memory-bank/work-items/{JIRA_KEY}/` exists
   - Read `plan.yaml` for current cursor and status
   - Read `waiting.yaml` if present (indicates waiting state)

2. **If work item does not exist**:
   - This may be a ticket that was never picked up (e.g. assigned while poller was down)
   - Call `/axiom-jira-intake` to process it fresh
   - Return `status=ok` with `action=intake_triggered`

### Phase 2: Route by Event Type

#### `comment_added` — Human replied to a question

3. **Check if work item is in `waiting_for_reply` state** (waiting.yaml exists):

   **YES — work item is waiting**:
   - Read `waiting.yaml` to get the original questions
   - Parse `$COMMENT_BODY` to extract answers to the questions
   - Assess if the answers are sufficient to proceed:
     - **Sufficient** → proceed to Phase 3 (resume execution)
     - **Still unclear** → post a follow-up question, update `waiting.yaml`, return `status=waiting`
   - Post acknowledgment comment to Jira:
     ```
     ✅ Got it — resuming implementation with your answers.
     ```

   **NO — work item is not waiting** (comment is informational):
   - Check if the comment contains new requirements or scope changes
   - If scope change detected: update `plan.yaml` with new steps, post acknowledgment
   - If informational: log and ignore
   - Return `status=ok` with `action=comment_noted`

#### `field_updated` — A field changed

4. **Route by field name**:
   - `codeops_repositories` changed → update `WorkItem.repo_list`, re-clone if needed
   - `codeops_branch` changed → update `WorkItem.target_branch`
   - `summary` changed → update work item title in `plan.yaml`
   - Other fields → log and ignore
   - Return `status=ok` with `action=field_updated`

#### `status_changed` — Ticket status transitioned

5. **Route by new status**:
   - `Done` / `Resolved` → if PR not yet created, log warning; if PR merged, cleanup workspace
   - `Cancelled` / `Won't Do` → stop execution, cleanup workspace, post acknowledgment
   - `Reopened` → if work item exists and is done, create a new plan for the follow-up work
   - Other → log and ignore
   - Return `status=ok` with `action=status_routed`

#### `assignee_changed` — Ticket reassigned

6. **Route by new assignee**:
   - Assigned to Axiom service account → trigger `/axiom-jira-intake` (new ticket pickup)
   - Assigned away from Axiom → stop execution, post handoff comment, cleanup workspace
   - Return `status=ok` with `action=assignee_routed`

### Phase 3: Resume Execution (after waiting state resolved)

7. **Update work item with answers**:
   - Incorporate the human's answers into `plan.yaml` acceptance criteria
   - Remove or archive `waiting.yaml`
   - Update `meta-planning.md` with the resolved questions

8. **Post resume comment to Jira**:
   ```
   🚀 Resuming implementation.

   **Answers incorporated**:
   - Q: {question_1} → A: {answer_1}
   - Q: {question_2} → A: {answer_2}

   **Next step**: {plan_cursor}
   ```

9. **Trigger execution** — signal the pipeline to resume `axiom run` for this work item.
   - Write `resume_trigger.yaml` to `.memory-bank/work-items/{JIRA_KEY}/`:
     ```yaml
     jira_key: {JIRA_KEY}
     trigger: resume
     triggered_at: {ISO_timestamp}
     answers:
       - question: {question_1}
         answer: {answer_1}
     ```
   - The pipeline's `GitHubEventRouter` / `ContainerLauncher` picks this up and restarts the container.

10. **Return `status=ok`** with `action=execution_resumed`.

## Fail Closed Rules

- If `$JIRA_KEY` is missing → `status=blocked`
- If Atlassian MCP is unavailable → `status=blocked` with instructions
- If `$EVENT_TYPE` is unrecognized → log warning, return `status=ok` with `action=ignored`
- Never modify `plan.yaml` based on a comment unless the comment is a direct reply to a Axiom question (identified by `waiting.yaml` presence)
- Never restart execution based on a comment from the Axiom service account itself (avoid loops)

## Loop Prevention

- Check `$COMMENT_AUTHOR_EMAIL` against the Axiom service account email (`svc_axiom@dexdat.ai` or `AXIOM_SERVICE_ACCOUNT_EMAIL` env var)
- If the comment is from Axiom itself → ignore, return `status=ok` with `action=self_comment_ignored`
- This prevents infinite loops where Axiom posts a comment, which triggers an update, which triggers another comment

## Output Contract

### For Human Consumption
- Summary: one sentence — what action was taken in response to the Jira update.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.jira_key`: the Jira ticket key processed
- `evidence.event_type`: the event type that was processed
- `evidence.action`: what action was taken (`execution_resumed` | `comment_noted` | `field_updated` | `status_routed` | `assignee_routed` | `intake_triggered` | `self_comment_ignored` | `ignored`)
- `evidence.files_changed`: files modified (semicolon-separated)
- `related_commands`:
  - "To check work item status, run: `/axiom-sitrep --work-item {JIRA_KEY}`"
  - "To continue execution, run: `/axiom-step-loop --work-item {JIRA_KEY}`"

### Cross-References
- "Work item is at: `.memory-bank/work-items/{JIRA_KEY}/`"
- "Waiting state is at: `.memory-bank/work-items/{JIRA_KEY}/waiting.yaml`"
- "Resume trigger is at: `.memory-bank/work-items/{JIRA_KEY}/resume_trigger.yaml`"
- "Spec: `specs/05-Jira-Integration.md`"
- "Intake lifecycle: `specs/44-Autonomous-Intake-And-Lifecycle.md`"

## Output Format

Emit a `<axiom>` XML envelope per `specs/04-XML-Protocol.md`:

```xml
<axiom>
  <run>
    <run_id>$RUN_ID</run_id>
    <work_item_id>$JIRA_KEY</work_item_id>
    <intake_source>jira_event</intake_source>
  </run>
  <command>/axiom-jira-event</command>
  <status>ok|waiting|blocked</status>
  <confidence>0-100</confidence>
  <summary>One sentence — what action was taken</summary>
  <detailed_summary>Event type, routing decision, action taken, Jira comment posted</detailed_summary>
  <evidence>
    <jira_key>$JIRA_KEY</jira_key>
    <event_type>$EVENT_TYPE</event_type>
    <action>execution_resumed|comment_noted|field_updated|status_routed|ignored</action>
    <files_changed>paths;semicolon;separated</files_changed>
  </evidence>
  <diagnostics>warnings or errors</diagnostics>
</axiom>
```

See: `specs/05-Jira-Integration.md`, `specs/44-Autonomous-Intake-And-Lifecycle.md`, `specs/04-XML-Protocol.md`
