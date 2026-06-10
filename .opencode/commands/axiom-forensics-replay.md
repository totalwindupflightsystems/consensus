---
description: "Reconstruct a readable conversation transcript for an OpenCode session — messages, tool calls, costs, and agent turns."
agent: forensics-axiom
---

Reconstruct a readable conversation transcript for an OpenCode session. Queries messages and parts to produce a chronological, human-readable transcript with agent turns, tool calls, costs, and token counts.

## Inputs

- `$SESSION` (required): Session ID to replay (e.g., `ses_abc123`)
- `$OUTPUT` (optional): Output file path. Default: auto-generated in `.memory-bank/findings/forensics/`
- `$INCLUDE_TOOL_CALLS` (optional): Include tool call inputs/outputs. Default: true
- `$INCLUDE_COSTS` (optional): Include per-message cost. Default: true
- `$MAX_TOOL_OUTPUT` (optional): Max characters of tool output to include per call. Default: 500
- `$FOLLOW_CHILDREN` (optional): Also replay child sessions (subagents). Default: false

## Skills

Load on startup:
- `forensics-axiom` — Query patterns, schema, methodology

## Do

1. **Load the `forensics-axiom` skill**.

2. **Resolve database path**:
   ```bash
   DB="${OPENCODE_DATA_DIR:-$HOME/.local/share/opencode}/opencode.db"
   ```

3. **Get session metadata**:
   ```sql
   SELECT id, title, parent_id, directory, slug,
          datetime(time_created/1000, 'unixepoch', 'localtime') AS created,
          datetime(time_updated/1000, 'unixepoch', 'localtime') AS updated
   FROM session
   WHERE id = ?;
   ```

4. **Get all messages in chronological order**:
   ```sql
   SELECT id,
          json_extract(data, '$.role') AS role,
          json_extract(data, '$.agent') AS agent,
          json_extract(data, '$.modelID') AS model,
          round(json_extract(data, '$.cost'), 6) AS cost,
          json_extract(data, '$.tokens.input') AS input_tokens,
          json_extract(data, '$.tokens.output') AS output_tokens,
          json_extract(data, '$.parentID') AS parent_id,
          datetime(time_created/1000, 'unixepoch', 'localtime') AS created
   FROM message
   WHERE session_id = ?
   ORDER BY time_created;
   ```

5. **For each message, get parts** (text + tool calls):
   ```sql
   -- Text parts
   SELECT json_extract(data, '$.text') AS text
   FROM part
   WHERE message_id = ?
     AND json_extract(data, '$.type') = 'text'
   ORDER BY rowid;
   
   -- Tool call parts
   SELECT json_extract(data, '$.tool') AS tool,
          json_extract(data, '$.callID') AS call_id,
          json_extract(data, '$.state.status') AS status,
          json_extract(data, '$.state.input') AS input,
          substr(json_extract(data, '$.state.output'), 1, 500) AS output_preview
   FROM part
   WHERE message_id = ?
     AND json_extract(data, '$.type') = 'tool'
   ORDER BY rowid;
   ```

6. **Format the transcript** as readable Markdown:
   ```markdown
   # Session Transcript: ses_abc123
   
   **Title**: Build forensics agent
   **Created**: 2026-04-06 10:30:00
   **Directory**: /Users/user/code/Axiom
   
   ---
   
   ## Turn 1 — User [10:30:01]
   
   Build the forensics agent for the Axiom project.
   
   ---
   
   ## Turn 2 — Assistant (tower-axiom / claude-sonnet-4-6) [10:30:05] — $0.0234
   
   I'll build the forensics agent. Let me start by reading the spec...
   
   **Tool: bash** [completed]
   Input: `cat specs/80-Session-Forensics-And-Self-Inspection.md`
   Output (preview): `# 80 — Session Forensics...`
   
   ---
   ```

7. **If $FOLLOW_CHILDREN** — find and replay child sessions:
   ```sql
   SELECT id, title, datetime(time_created/1000, 'unixepoch', 'localtime') AS created
   FROM session
   WHERE parent_id = ?
   ORDER BY time_created;
   ```

8. **Write transcript** to `.memory-bank/findings/forensics/replay-<session>-<timestamp>.md`.

9. **Return artifact paths**.

## Output Contract

Return:
- `report_path`: path to the transcript file
- `session_id`: session ID replayed
- `session_title`: session title
- `message_count`: total messages in transcript
- `total_cost`: total cost for the session
- `tool_calls`: count of tool calls made
- `child_sessions`: list of child session IDs (if $FOLLOW_CHILDREN)

## Safety

- Database MUST be opened read-only
- All queries MUST use `LIMIT`
- NEVER read auth files
- Tool output is truncated to $MAX_TOOL_OUTPUT characters to avoid huge files
- Timestamps are milliseconds — divide by 1000

axiom:trace work_item=forensics-01 spec=specs/80-Session-Forensics-And-Self-Inspection.md
