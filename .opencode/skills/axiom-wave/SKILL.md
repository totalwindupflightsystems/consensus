---
name: axiom-wave
description: >
  Wave skill for producing Wave-compatible output. Load this skill when using Wave tools
  to interact with Spaces, write rich content, propose actions, or read comments. Phase 1
  focuses on session review, sharing, and basic comments (comment + highlight only).
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  primary_spec: specs/79-Axiom-Weave-Interactive-Review-Portal.md
  supporting_specs:
    - specs/70-OpenCode-Plugin.md
  phase: 1
  tools:
    - codeops_wave_read
    - codeops_wave_write
    - codeops_wave_propose_action
    - codeops_wave_comments
---

# Axiom Wave Skill

> **"Wave is where humans and agents collaborate in real time. The agent writes, the human reviews, and both see changes live."**

This skill teaches you how to produce Wave-compatible output using the Wave tools. Load this skill when you need to interact with a Wave Space during an OpenCode session.

axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-103 plan=phase-1.6/task-1.6.wave-skill/step-1

---

## When to Load This Skill

Load this skill when:
- You need to write rich content to a Wave Space (reports, charts, status pages)
- You need to read comments or action items from a Space
- You want to propose an action for human approval
- You're working in a session that has an associated Wave Space
- The user mentions "Wave", "Space", or asks you to share your work

---

## What is Wave?

**Wave** is Axiom' interactive collaboration portal — a real-time surface where humans and AI agents work together. Think of it as:

- **Session viewer**: See OpenCode sessions scroll in real time, with comments
- **Agentic Pastebin**: Share rich output (charts, reports, slides) via URL
- **Collaboration space**: Chat, approve actions, annotate output

**Key terminology**:
- **Space**: A collaborative container (like a Google Wave "wave")
- **Panel**: A section within a Space (chat, session replay, rendered output)
- **Item**: A unit of content (message, tool call, file change, rich render)
- **Comment**: Inline feedback on an item
- **Action**: Something you propose to do, awaiting human approval

---

## Wave Tools (Phase 1)

Phase 1 provides 4 tools for interacting with Spaces.

### `codeops_wave_read` — Read Space State

Read the current state of a Space: panels, comments, and action items.

```json
{
  "name": "codeops_wave_read",
  "parameters": {
    "space_id": "string (required)",
    "panel_id": "string (optional, filter to specific panel)",
    "include_comments": "boolean (default: true)",
    "include_action_list": "boolean (default: true)"
  }
}
```

**Returns**:
```json
{
  "space_id": "space-abc123",
  "panels": [
    { "panel_id": "panel-1", "type": "session_replay", "items": [...] }
  ],
  "comments": [
    { "comment_id": "c1", "author": "human", "content": "Please fix the error handling" }
  ],
  "actions": [
    { "action_id": "a1", "title": "Refactor error handling", "status": "proposed" }
  ]
}
```

**When to use**:
- Check for new comments on your session
- See if any actions were approved/rejected
- Read the current state before writing new content

---

### `codeops_wave_write` — Write Content to a Space

Write rich content to a Space panel. Creates a new panel if `panel_id` is omitted.

```json
{
  "name": "codeops_wave_write",
  "parameters": {
    "space_id": "string (required)",
    "panel_id": "string (optional, creates new panel if omitted)",
    "content_type": "markdown | html | json | template",
    "content": "string (required)",
    "template": "string (optional, template name)",
    "data": "object (optional, data for templates)"
  }
}
```

**Returns**:
```json
{
  "item_id": "item-xyz789",
  "panel_id": "panel-2",
  "space_id": "space-abc123"
}
```

**Content types**:
- `markdown`: Standard markdown (rendered as HTML)
- `html`: Raw HTML (sanitized, sandboxed in Phase 1)
- `json`: Structured data (for templates or charts)
- `template`: Use a pre-built template (report, dashboard, etc.)

**When to use**:
- Write a status report to the Space
- Render a chart or visualization
- Share a summary of your work
- Create a live-updating dashboard panel

**Example**:
```json
{
  "space_id": "space-abc123",
  "content_type": "markdown",
  "content": "## Summary\n\nI've completed the error handling refactor. Key changes:\n\n- Added retry logic to API calls\n- Improved error messages\n- Added structured logging\n\nSee the diff in the session replay panel."
}
```

---

### `codeops_wave_propose_action` — Propose an Action

Propose an action for human approval. Actions appear in the Space's action list with Approve/Reject buttons.

```json
{
  "name": "codeops_wave_propose_action",
  "parameters": {
    "space_id": "string (required)",
    "title": "string (required, short description)",
    "description": "string (required, what will be done)",
    "priority": "high | medium | low (default: medium)",
    "source_item_id": "string (optional, which item triggered this)"
  }
}
```

**Returns**:
```json
{
  "action_id": "action-123",
  "status": "proposed",
  "title": "Refactor error handling"
}
```

**When to use**:
- You've identified work that needs human approval
- The user asked you to propose next steps
- You want to give the human control over what you do next

**Example**:
```json
{
  "space_id": "space-abc123",
  "title": "Add integration tests for error handling",
  "description": "I'll add tests covering the new retry logic and error message formatting. Estimated 15 minutes.",
  "priority": "medium"
}
```

---

### `codeops_wave_comments` — Read Comments

Read comments on session items. Use this to see human feedback on your work.

```json
{
  "name": "codeops_wave_comments",
  "parameters": {
    "space_id": "string (required)",
    "item_id": "string (optional, filter to specific item)",
    "status": "open | resolved | all (default: all)"
  }
}
```

**Returns**:
```json
{
  "comments": [
    {
      "comment_id": "c1",
      "author": "human",
      "item_id": "item-5",
      "type": "comment",
      "content": "Can you also handle timeout errors?",
      "status": "open"
    }
  ],
  "total": 1
}
```

**When to use**:
- Check for new feedback after writing content
- See if the human has questions or requests
- Find comments that need responses

---

## Comment Types (Phase 1)

Phase 1 supports only 2 comment types. The full 8-type system comes in Phase 2+.

### `comment` — General Feedback

A general comment on an item. The human is providing feedback or asking a question.

**How to respond**:
- Acknowledge factually: "Got it. I'll revise the error handling section."
- If unclear, ask for clarification
- If actionable, propose an action

### `highlight` — Attention Marker

The human highlighted something for attention. No specific action requested, just "look at this."

**How to respond**:
- Review the highlighted section
- If you see an issue, acknowledge and propose a fix
- If unclear why it was highlighted, ask

---

## Action Item Lifecycle

Actions go through these states:

```
proposed → approved → executing → completed
         ↘ rejected
         ↘ stale
```

| Status | Meaning | Your Action |
|--------|---------|-------------|
| `proposed` | Awaiting human approval | Wait for approval/rejection |
| `approved` | Human approved, ready to execute | Start work on the action |
| `executing` | Work in progress | Continue until done |
| `completed` | Done | No further action needed |
| `rejected` | Human declined | Do not execute; ask if alternative needed |
| `stale` | Superseded by newer conversation | Ignore; focus on newer actions |

**Important**: You cannot transition actions yourself. The human approves/rejects via the Wave UI. You can only propose actions and check their status.

---

## Agent Voice in Wave

When writing to a Space or responding to comments, follow these voice guidelines:

### Do:
- Use "I" sparingly and only for **actions**, not feelings
- Acknowledge comments factually: "Got it. I'll revise the error handling section."
- Propose actions as **options**: "I think the next step is X. Want me to do that?"
- Be specific about blockers: "I'm pausing because the spec and AC disagree on error format."

### Don't:
- Narrate your thinking process
- Use emotional language ("I feel", "I'm excited")
- Make decisions without human approval for significant changes
- Claim work is done without verification

---

## Phase 1 Limitations

Phase 1 is focused on session review and sharing. These features are **NOT available** yet:

| Feature | Available In |
|---------|--------------|
| Rich HTML/CSS/JS rendering | Phase 2 (HTML/CSS only) |
| Agent-generated JavaScript | Phase 3 (after security review) |
| WYSIWYG collaborative editing | Phase 4 |
| Automations/triggers | Phase 4 |
| WebSocket real-time collaboration | Phase 3 |
| Full comment types (suggest_change, approve, decline, insert, remove, question) | Phase 2 |
| Playback/history scrubbing | Phase 4 |
| Chat panels | Phase 3 |
| Action list execution (spawning sessions) | Phase 3 |

**What IS available in Phase 1**:
- Session replay viewer (read-only, with comments)
- Basic comments (`comment` and `highlight` types only)
- Action proposals (human approves via UI, you check status)
- Sharing via URL (E2E encrypted, S3-backed)
- Writing markdown content to panels

---

## Quick Start: Using Wave in a Session

### Step 1: Check for a Space

If you're in a session that might have an associated Space, check for comments:

```json
{
  "name": "codeops_wave_read",
  "parameters": {
    "space_id": "space-abc123",
    "include_comments": true,
    "include_action_list": true
  }
}
```

### Step 2: Respond to Comments

If there are open comments, acknowledge and act:

```json
{
  "name": "codeops_wave_write",
  "parameters": {
    "space_id": "space-abc123",
    "content_type": "markdown",
    "content": "I've addressed the timeout error feedback. The retry logic now includes a 30-second timeout with exponential backoff."
  }
}
```

### Step 3: Propose Next Steps

If you've completed work and see a logical next step, propose an action:

```json
{
  "name": "codeops_wave_propose_action",
  "parameters": {
    "space_id": "space-abc123",
    "title": "Add integration tests for retry logic",
    "description": "I'll add tests covering timeout scenarios, backoff timing, and error propagation. This will take about 20 minutes.",
    "priority": "medium"
  }
}
```

---

## Common Patterns

### Pattern: Status Update

After completing a significant chunk of work, write a summary:

```json
{
  "space_id": "space-abc123",
  "content_type": "markdown",
  "content": "## Progress Update\n\n**Completed**:\n- Error handling refactor (3 files changed)\n- Retry logic with exponential backoff\n- Structured logging integration\n\n**Next**:\n- Integration tests (proposed action)\n- Documentation update\n\n**Blockers**: None"
}
```

### Pattern: Asking for Clarification

If a comment is unclear, respond with a question:

```json
{
  "space_id": "space-abc123",
  "content_type": "markdown",
  "content": "Re: your comment on the retry logic — should the backoff cap at 30 seconds or 60 seconds? The spec says 30s but the existing code uses 60s."
}
```

### Pattern: Reporting a Blocker

If you're stuck, write to the Space and propose an action:

```json
{
  "space_id": "space-abc123",
  "content_type": "markdown",
  "content": "## Blocker\n\nI'm pausing because the spec and acceptance criteria disagree on the error format:\n\n- Spec says: `{\"error\": \"message\"}`\n- AC example shows: `{\"error\": {\"code\": \"E001\", \"message\": \"...\"}}`\n\nI've proposed an action to clarify with the spec owner."
}
```

---

## Security Notes

- **Never write secrets** to a Space. All content is redacted at read time, but don't rely on this.
- **Comments are redacted** before sharing. If a comment contains a secret, it becomes `[REDACTED]`.
- **E2E encrypted shares**: The decryption key lives in the URL fragment (`#key`) and is never sent to the server.
- **Sandboxed rendering**: Agent-generated HTML runs in a sandboxed iframe with no network access.

---

## Spec References

- `specs/79-Axiom-Weave-Interactive-Review-Portal.md` — Full Wave specification
- `specs/70-OpenCode-Plugin.md` — Plugin integration, Wave tools layer
- `specs/79#REQ-WEAVE-103` — This skill requirement
- `specs/79#REQ-WEAVE-112` — `codeops_wave_write` tool
- `specs/79#REQ-WEAVE-113` — `codeops_wave_read` tool
- `specs/79#REQ-WEAVE-114` — `codeops_wave_propose_action` tool
- `specs/79#Section-3.4` — Comment types
- `specs/79#Section-3.5` — Action items
- `specs/79#Section-12.2` — Agent voice guidelines