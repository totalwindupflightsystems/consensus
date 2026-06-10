---
name: forensics-axiom
description: >
  Portable forensic analysis system for investigating OpenCode sessions. Provides query patterns,
  investigation methodology, database schema reference, and report format. Fully self-contained —
  works in installed repos without specs/. Source spec: specs/80-Session-Forensics-And-Self-Inspection.md.
version: "1.0"
synopsis: |
  Enables Axiom to investigate its own session history: search conversations, trace subagent
  hierarchies, analyze costs, reconstruct transcripts, and produce investigation reports. All
  queries are read-only against the OpenCode SQLite database.
when-to-use: |
  Load when debugging agent behavior, investigating cost anomalies, tracing subagent dispatch
  trees, searching for specific content across sessions, or producing post-incident investigation
  reports. Also load when /axiom-forensics or any sub-command is invoked.
tags:
  vertical: [ops, sre]
  category: forensics
  core: false
# axiom:trace work_item=profiles-01 spec=specs/83-Skill-Mode-Switching.md plan=phase-3/task-3-2/step-3-2-1
---

# Session Forensics (Axiom — Portable)

> **"To understand what went wrong, you must be able to see what happened."**
>
> **"Read-only. Never modify the database. Never read auth files."**

Source spec (Axiom repo only): `specs/80-Session-Forensics-And-Self-Inspection.md`

---

## 1. Database Location and Access

### Path Resolution

```python
import os, sys

def get_db_path() -> str:
    data_dir = os.environ.get("OPENCODE_DATA_DIR")
    if not data_dir:
        data_dir = os.path.expanduser("~/.local/share/opencode")
    return os.path.join(data_dir, "opencode.db")

def get_tool_output_dir() -> str:
    data_dir = os.environ.get("OPENCODE_DATA_DIR")
    if not data_dir:
        data_dir = os.path.expanduser("~/.local/share/opencode")
    return os.path.join(data_dir, "tool-output")
```

### Safety Rules (MANDATORY)

1. **Read-only**: Always open the database in read-only mode: `sqlite3.connect(f"file:{path}?mode=ro", uri=True)`
2. **Never read**: `auth.json`, `mcp-auth.json`, or any `*auth*.json` file
3. **Never write**: Do not INSERT, UPDATE, DELETE, DROP, CREATE, or ALTER
4. **Timestamps**: All timestamps are in **MILLISECONDS** — divide by 1000 for Unix seconds
5. **Part table**: 2.5M+ rows — NEVER do `LIKE '%keyword%'` on it. Use tool-output grep instead.
6. **Limits**: Always use `LIMIT` (default 100, max 1000)

### Bash Quick Access

```bash
# Database path
DB="${OPENCODE_DATA_DIR:-$HOME/.local/share/opencode}/opencode.db"

# Read-only connection
sqlite3 "file:${DB}?mode=ro" "SELECT count(*) FROM session;"

# Tool output directory
TOOL_DIR="${OPENCODE_DATA_DIR:-$HOME/.local/share/opencode}/tool-output"
```

---

## 2. Database Schema (3 Core Tables)

### session — One row per conversation

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | `ses_` prefix |
| `project_id` | TEXT | SHA1 hash of project directory |
| `parent_id` | TEXT | Parent session (subagent hierarchy) |
| `slug` | TEXT | Human-readable name |
| `directory` | TEXT | Working directory path |
| `title` | TEXT | Auto-generated title |
| `version` | TEXT | OpenCode version |
| `time_created` | INTEGER | Unix epoch **MILLISECONDS** |
| `time_updated` | INTEGER | Unix epoch **MILLISECONDS** |

### message — One row per user/assistant turn

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | `msg_` prefix |
| `session_id` | TEXT FK | Links to session |
| `time_created` | INTEGER | Milliseconds |
| `data` | TEXT | JSON: `{role, agent, modelID, providerID, cost, tokens, parentID}` |

**data JSON fields:**
- `role`: "user" or "assistant"
- `agent`: agent name (e.g., "build", "dev-axiom")
- `modelID`: model identifier
- `cost`: cost in USD (assistant messages only)
- `tokens`: `{input, output, reasoning, cache: {read, write}}`
- `parentID`: links assistant reply to user message

### part — Actual content (text, tool calls, step markers)

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | `prt_` prefix |
| `message_id` | TEXT FK | Links to message |
| `session_id` | TEXT FK | Denormalized for performance |
| `data` | TEXT | JSON: varies by type |

**data JSON by type:**

Text: `{"type": "text", "text": "..."}`

Tool call: `{"type": "tool", "tool": "bash", "callID": "...", "state": {"status": "completed", "input": {...}, "output": "..."}}`

Step markers: `{"type": "step-start"}` / `{"type": "step-finish", "tokens": {...}, "cost": 0.05}`

---

## 3. Query Cookbook

### Search sessions by project

```sql
SELECT id, title, parent_id, directory,
       datetime(time_created/1000, 'unixepoch', 'localtime') AS created
FROM session
WHERE directory LIKE '%/MyProject%'
ORDER BY time_created DESC
LIMIT 50;
```

### Get messages in a session

```sql
SELECT id,
       json_extract(data, '$.role') AS role,
       json_extract(data, '$.agent') AS agent,
       json_extract(data, '$.modelID') AS model,
       json_extract(data, '$.cost') AS cost,
       datetime(time_created/1000, 'unixepoch', 'localtime') AS created
FROM message
WHERE session_id = ?
ORDER BY time_created;
```

### Get text content for a message

```sql
SELECT json_extract(data, '$.text') AS text
FROM part
WHERE message_id = ?
  AND json_extract(data, '$.type') = 'text';
```

### Get tool calls for a message

```sql
SELECT json_extract(data, '$.tool') AS tool,
       json_extract(data, '$.callID') AS call_id,
       json_extract(data, '$.state.status') AS status,
       substr(json_extract(data, '$.state.output'), 1, 500) AS output_preview
FROM part
WHERE message_id = ?
  AND json_extract(data, '$.type') = 'tool';
```

### Trace subagent hierarchy (recursive)

```sql
WITH RECURSIVE chain(id, title, parent_id, depth) AS (
    SELECT id, title, parent_id, 0 FROM session WHERE id = ?
    UNION ALL
    SELECT s.id, s.title, s.parent_id, c.depth + 1
    FROM session s JOIN chain c ON s.parent_id = c.id
)
SELECT * FROM chain ORDER BY depth;
```

### Cost by agent (last 30 days)

```sql
SELECT json_extract(data, '$.agent') AS agent,
       SUM(json_extract(data, '$.cost')) AS total_cost,
       COUNT(*) AS messages,
       AVG(json_extract(data, '$.cost')) AS avg_cost
FROM message
WHERE json_extract(data, '$.role') = 'assistant'
  AND time_created > (strftime('%s', 'now') - 30*86400) * 1000
GROUP BY agent
ORDER BY total_cost DESC;
```

### Cost by model (last 30 days)

```sql
SELECT json_extract(data, '$.modelID') AS model,
       SUM(json_extract(data, '$.cost')) AS total_cost,
       COUNT(*) AS messages
FROM message
WHERE json_extract(data, '$.role') = 'assistant'
  AND time_created > (strftime('%s', 'now') - 30*86400) * 1000
GROUP BY model
ORDER BY total_cost DESC;
```

### Find sessions active during a time window

```sql
SELECT id, title, directory,
       datetime(time_created/1000, 'unixepoch', 'localtime') AS created,
       datetime(time_updated/1000, 'unixepoch', 'localtime') AS updated
FROM session
WHERE time_created/1000 BETWEEN ? AND ?
ORDER BY time_created;
```

### Keyword search (FAST PATH — grep tool outputs first)

```bash
# Step 1: grep tool outputs (fast — plain text files)
grep -rl "keyword" "${OPENCODE_DATA_DIR:-$HOME/.local/share/opencode}/tool-output/"

# Step 2: get file timestamps to correlate to sessions
stat -f "%m %N" <matched-file>  # macOS
stat -c "%Y %n" <matched-file>  # Linux

# Step 3: find sessions active at that time (use the SQL query above)
```

**NEVER** do `SELECT * FROM part WHERE data LIKE '%keyword%'` — this scans 2.5M rows and will timeout.

---

## 4. Investigation Methodology

### Step-by-Step Process

1. **Define the question** — What are you trying to understand? (e.g., "Why did the agent produce empty output?", "Why was this session so expensive?", "What happened during the auth-l3l4 work?")

2. **Gather context** — Identify: time range, project, agent name, session ID (if known), keywords

3. **Search tool outputs first** — `grep -rl "keyword" tool-output/` is the fastest path for content search

4. **Query sessions** — Use indexed queries on `session` table (fast, 30K rows)

5. **Trace hierarchies** — If the session is a subagent, walk `parent_id` chain to find the root

6. **Reconstruct conversations** — For key sessions, query message → part to read the full context

7. **Analyze costs** — Aggregate by agent, model, or time period to find anomalies

8. **Document findings** — Write investigation report to `.memory-bank/findings/forensics/`

9. **File bugs** — If findings reveal platform bugs, use `/axiom-report-issue` to file to Jira

### Common Investigation Scenarios

| Scenario | Start With | Key Queries |
|----------|-----------|-------------|
| "Why did agent X fail?" | Session ID | Messages → Parts → Tool calls |
| "Why was this expensive?" | Time range | Cost by agent/model → Drill into top sessions |
| "What happened with work item Y?" | Project + keyword | Search tool outputs → Correlate sessions |
| "Agent returned empty" | Session ID + parent_id | Trace hierarchy → Check subagent sessions |
| "Find all security-related sessions" | Keyword "security" | Grep tool outputs → Session correlation |

---

## 5. Investigation Report Format

Write to: `.memory-bank/findings/forensics/<investigation-id>.md`

```markdown
---
mb:
  type: finding
  title: "<Investigation Title>"
  created: YYYY-MM-DD
  updated: YYYY-MM-DD
  tags: [forensics, investigation, <type>]
  severity: low|medium|high|critical
  status: open|addressed|wont-fix
  agent: forensics-axiom
  category: forensics
  links:
    up: "../_index.md"
  source:
    type: forensic-investigation
    ref: "sessions=<list> time_range=<range>"
---

# Investigation: <Title>

**Date**: <ISO 8601>
**Investigator**: @forensics-axiom
**Question**: <What were you trying to understand?>

## Summary
<1-2 paragraph summary>

## Sessions Analyzed
| Session ID | Agent | Model | Cost | Duration |
|---|---|---|---|---|

## Key Findings
1. **Finding**: <description>
   - **Evidence**: <session/message/tool-output path>
   - **Impact**: <why this matters>

## Cost Analysis
- Total cost analyzed: $X.XX
- Anomalies: <if any>

## Recommendations
1. <action item>

## Artifacts Produced
- Report: `.memory-bank/findings/forensics/<id>.md`
- Transcript: `<path if replay was done>`
```

---

## 6. Command Family

| Command | Purpose | Key Parameters |
|---------|---------|---------------|
| `/axiom-forensics` | Master investigation command | `<type>`, `--session`, `--keyword`, `--from`, `--to` |
| `/axiom-forensics-search` | Search sessions/messages/tool-outputs | `--keyword`, `--agent`, `--model`, `--from` |
| `/axiom-forensics-trace` | Trace subagent hierarchy | `--session` |
| `/axiom-forensics-cost` | Cost analysis | `--by agent|model|day`, `--from`, `--to` |
| `/axiom-forensics-replay` | Reconstruct conversation transcript | `--session`, `--output` |

All commands load this skill (`forensics-axiom`) for query patterns and methodology.

---

## 7. Agent: @forensics-axiom

**Role**: Investigate OpenCode sessions, analyze agent behavior, trace subagent hierarchies, produce investigation reports.

**Capabilities**: bash (read-only SQLite + grep), memory bank write (reports), Jira (file bugs)

**When to invoke**:
- After a problematic session
- When investigating cost anomalies
- When debugging agent decision-making
- When analyzing patterns across sessions
- When producing a post-mortem report

**Skills to load**: `forensics-axiom` (this skill), `adversarial-review-axiom` (if investigating adversarial findings), `self-report-axiom` (if filing bugs)

---

## 8. Checklist

Before investigating:
- [ ] Database path resolved (`$OPENCODE_DATA_DIR` or default)
- [ ] Database exists and is readable
- [ ] Connection opened in read-only mode
- [ ] Question defined (what are you trying to understand?)

During investigation:
- [ ] Tool outputs searched first (grep, not SQL LIKE)
- [ ] Queries use indexed columns (session_id, message_id)
- [ ] LIMIT clause on all queries
- [ ] Timestamps divided by 1000 for human-readable dates
- [ ] No auth files read

After investigation:
- [ ] Report written to `.memory-bank/findings/forensics/`
- [ ] Findings index updated
- [ ] Bugs filed via `/axiom-report-issue` if applicable
- [ ] Artifact paths returned to caller
