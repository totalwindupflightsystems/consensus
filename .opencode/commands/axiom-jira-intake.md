---
description: Intake a Jira ticket as a Axiom work item. Assesses completeness, asks clarifying questions via Jira comment if needed, and either produces a plan or enters waiting state.
agent: tower-axiom
---

Process a Jira ticket as a Axiom work item. This command is called automatically
by the Jira automation pipeline when a ticket is assigned to the Axiom service
account. It either produces a ready-to-execute plan or posts a clarifying question
to Jira and waits for the human to respond.

axiom:trace work_item=jira-coder-pipeline-01 spec=specs/05-Jira-Integration.md,specs/44-Autonomous-Intake-And-Lifecycle.md,specs/59-Collaborative-Intent-Resolution.md plan=phase-4 jira_ref=DEX-436

## Inputs

- `$JIRA_KEY` — Jira issue key (e.g. `SWDE-13`)
- `$JIRA_SUMMARY` — ticket summary (title)
- `$JIRA_DESCRIPTION` — ticket description body
- `$JIRA_URL` — Jira instance base URL
- `$REPO_LIST` — comma-separated GitHub repo slugs from `codeops_repositories` field (or default)
- `$TARGET_BRANCH` — target branch from `codeops_branch` field (or `main`)
- `$REPORTER_NAME` — display name of the ticket reporter (for co-authorship)
- `$REPORTER_EMAIL` — email of the ticket reporter

Skills (load on demand):
- `jira-workflow-axiom` — Jira operating model, comment format, ADF structure. Load always.
- `axiom-collaborative-intent-resolution` — Intake protocol for resolving raw intent. Load when assessing completeness.
- `spec-kickoff-axiom` — Spec kickoff for generating high-quality specs from minimal input.
- `kiss-axiom` — **Load always in runner mode.** This command runs autonomously without a human in the loop. KISS mode prevents over-engineering: do full planning and meta-planning to understand the ticket, but only generate steps that map to an acceptance criterion. Do not generate hardening, polish, or perfection steps that aren't required to meet the stated ACs. The goal is a plan that gets the ticket done, not a plan that makes it perfect.

> **Runner Mode Default**: This command runs in KISS mode by default. It is invoked automatically by the Jira automation pipeline, not by a human. Over-engineering in runner mode creates bloat that requires multiple verify/step-loop cycles to drain. KISS mode ensures the plan is scoped to exactly what the ticket requires.

## Decision Tree

### Phase 1: Read and Assess

1. **Read the ticket** — parse `$JIRA_SUMMARY` and `$JIRA_DESCRIPTION` for:
   - What the human wants built or changed
   - Which repos are affected (from `$REPO_LIST`)
   - Any explicit acceptance criteria
   - Any referenced specs, PRs, or related tickets

2. **Assess completeness** — can Axiom act on this ticket without asking questions?

   **Completeness checklist**:
   - [ ] The intent is clear enough to write at least one testable acceptance criterion
   - [ ] The target repo(s) are known (`$REPO_LIST` is set or default is acceptable)
   - [ ] There are no contradictions or impossible requirements
   - [ ] The scope is bounded (not "rewrite everything")

3. **Route based on assessment**:

   - **COMPLETE** → proceed to Phase 2 (produce plan)
   - **INCOMPLETE** → proceed to Phase 3 (ask questions, enter waiting state)
   - **IMPOSSIBLE** → proceed to Phase 4 (post blocker comment, close ticket)

### Phase 2: Produce Plan (ticket is complete)

4. **Create work item** in `.memory-bank/work-items/{JIRA_KEY}/`:
   - `meta-planning.md` — scope, constraints, open decisions
   - `plan.md` — human-readable plan
   - `plan.yaml` — machine-executable plan with phases/tasks/steps

5. **Write Jira comment files** — the entrypoint reads these and posts them to Jira.
   Write natural, conversational comments as if you're a colleague updating the ticket owner.
   Do NOT use templates or repeat the ticket summary back. Instead, explain what you understood,
   what your approach will be, and what the human should expect.

   **Write `.jira-intake-comment.md`** in the workspace root:
   ```
   Hey! I've picked up this ticket and here's my plan:

   I'm going to add a `/health` endpoint to the Express app that returns `{"status": "ok", "timestamp": "2026-04-28T12:00:00Z"}`. The endpoint will:
   - Return a 200 status with JSON content type
   - Include the current server time in ISO 8601 format
   - Be accessible at GET /health with no authentication required

   I'll also add a test that verifies the response shape and status code.

   Working on it now — should have a PR ready in a few minutes.
   ```

   The comment should feel like a smart colleague explaining their approach, NOT like a bot
   echoing the ticket back. Include:
   - Your understanding of what needs to be built (in your own words, not copy-paste)
   - Your technical approach (specific files, endpoints, patterns you'll use)
   - What the human should expect next
   - Any assumptions you're making

   **Write `.jira-completion-comment.md`** after implementation is done:
   ```
   Done! Here's what I built:

   Added a `GET /health` endpoint in `server.js` that returns:
   ```json
   {"status": "ok", "timestamp": "2026-04-28T12:34:56.789Z"}
   ```

   The endpoint uses `new Date().toISOString()` for the timestamp. I also added a test in `test/health.test.js` that verifies:
   - Status code is 200
   - Response has `status` and `timestamp` fields
   - Timestamp is valid ISO 8601

   Files changed:
   - `server.js` — added /health route (5 lines)
   - `test/health.test.js` — new test file (25 lines)

   PR is coming up next.
   ```

   The completion comment should describe what you actually built, not what was requested.
   Include specific details: file names, line counts, code snippets, test coverage.

6. **Transition ticket** to "In Progress" (if transition is available).

7. **Return `status=ok`** with the plan.

### Phase 3: Ask Questions (ticket is incomplete)

8. **Identify the minimum set of questions** needed to proceed (max 3 questions).
   - Be specific: "Which endpoint should return the data — `/api/v1/users` or `/api/v2/users`?"
   - Not vague: "Can you clarify the requirements?"

9. **Post questions comment to Jira** (ADF format):
   ```
   👋 Axiom picked up this ticket but needs a few clarifications before starting.

   **Questions**:
   1. {question_1}
   2. {question_2}

   Please reply to this comment with your answers. Axiom will resume automatically
   when you respond.

   **Waiting for**: human reply on this ticket
   ```

10. **Write waiting state** to `.memory-bank/work-items/{JIRA_KEY}/waiting.yaml`:
    ```yaml
    jira_key: {JIRA_KEY}
    state: waiting_for_reply
    questions_asked:
      - {question_1}
      - {question_2}
    comment_id: {jira_comment_id}
    waiting_since: {ISO_timestamp}
    ```

11. **Return `status=waiting`** — the pipeline pauses until `/axiom-jira-update` is called
    with the human's reply.

### Phase 4: Post Blocker (ticket is impossible)

12. **Post blocker comment to Jira** explaining why Axiom cannot proceed.

13. **Transition ticket** back to "To Do" or "Backlog".

14. **Return `status=blocked`** with the reason.

## Fail Closed Rules

- If `$JIRA_KEY` is missing or empty → `status=blocked`
- If `$JIRA_SUMMARY` is empty → post "ticket has no summary" comment → `status=blocked`
- If Atlassian MCP is unavailable → `status=blocked` with instructions
- If the work item already exists and has a plan cursor → do NOT overwrite; post "already in progress" comment → `status=ok` (idempotent)
- Never fabricate acceptance criteria. If the ticket is unclear, ask.

## Output Contract

### For Human Consumption
- Summary: one sentence — plan produced, questions asked, or blocked reason.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: work item files created (semicolon-separated)
- `evidence.jira_key`: the Jira ticket key processed
- `evidence.intake_result`: `plan_produced` | `waiting_for_reply` | `blocked`
- `evidence.questions_asked`: count of questions posted (0 if plan produced)
- `evidence.plan_phases_count`: number of phases (0 if waiting/blocked)
- `evidence.plan_steps_count`: total steps (0 if waiting/blocked)
- `related_commands`:
  - "When human replies, run: `/axiom-jira-update --jira-key {JIRA_KEY}`"
  - "To execute the plan, run: `/axiom-step-loop --work-item {JIRA_KEY} mode=full-queue --kiss`"

### Cross-References
- "Work item is at: `.memory-bank/work-items/{JIRA_KEY}/`"
- "Waiting state is at: `.memory-bank/work-items/{JIRA_KEY}/waiting.yaml`"
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
  <command>/axiom-jira-intake</command>
  <status>ok|waiting|blocked</status>
  <confidence>0-100</confidence>
  <summary>One sentence result</summary>
  <detailed_summary>What was assessed, what was decided, what was posted to Jira</detailed_summary>
  <evidence>
    <jira_key>$JIRA_KEY</jira_key>
    <intake_result>plan_produced|waiting_for_reply|blocked</intake_result>
    <questions_asked>0</questions_asked>
    <plan_phases_count>N</plan_phases_count>
    <plan_steps_count>M</plan_steps_count>
    <files_changed>paths;semicolon;separated</files_changed>
  </evidence>
  <diagnostics>warnings or errors</diagnostics>
</axiom>
```

See: `specs/05-Jira-Integration.md`, `specs/44-Autonomous-Intake-And-Lifecycle.md`, `specs/59-Collaborative-Intent-Resolution.md`, `specs/04-XML-Protocol.md`
