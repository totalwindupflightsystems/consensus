---
description: "Master forensics command — investigate OpenCode sessions by searching, tracing, analyzing costs, or replaying conversations."
agent: forensics-axiom
---

Investigate OpenCode sessions. Dispatches to @forensics-axiom which loads the `forensics-axiom` skill for all query patterns, schema reference, and methodology.

## Inputs

- `$TYPE` (required): Investigation type — `search`, `trace`, `cost`, `replay`, or `summary`
- `$SESSION` (optional): Session ID to investigate (e.g., `ses_abc123`)
- `$KEYWORD` (optional): Keyword to search for across tool outputs and sessions
- `$AGENT` (optional): Filter by agent name (e.g., `dev-axiom`, `tower-axiom`)
- `$MODEL` (optional): Filter by model ID
- `$FROM` (optional): Start time — ISO date (`2026-01-01`) or relative (`-7d`, `-30d`)
- `$TO` (optional): End time — ISO date or relative. Default: now
- `$OUTPUT` (optional): Output file path for the investigation report. Default: auto-generated in `.memory-bank/findings/forensics/`

## Skills

Load on startup:
- `forensics-axiom` — All query patterns, schema reference, investigation methodology, report format

## Do

1. **Load the `forensics-axiom` skill** — contains all query patterns, schema, and methodology.

2. **Resolve database path**:
   ```bash
   DB="${OPENCODE_DATA_DIR:-$HOME/.local/share/opencode}/opencode.db"
   TOOL_DIR="${OPENCODE_DATA_DIR:-$HOME/.local/share/opencode}/tool-output"
   ```

3. **Verify database access** (walking skeleton check):
   ```bash
   sqlite3 "file:${DB}?mode=ro" "SELECT count(*) FROM session;"
   ```
   If this fails, report the error and stop. Do not proceed.

4. **Dispatch by $TYPE**:

   - **`search`**: Search tool outputs via grep first, then correlate to sessions. See `/axiom-forensics-search` for full workflow.
   - **`trace`**: Trace subagent hierarchy for `$SESSION`. See `/axiom-forensics-trace`.
   - **`cost`**: Aggregate costs by agent/model/time. See `/axiom-forensics-cost`.
   - **`replay`**: Reconstruct conversation transcript for `$SESSION`. See `/axiom-forensics-replay`.
   - **`summary`**: Quick summary of recent sessions — count, cost, top agents, last 24h activity.

5. **For `summary` type** (default when no type given):
   ```sql
   -- Session count
   SELECT count(*) FROM session;
   
   -- Recent sessions (last 24h)
   SELECT count(*) FROM session
   WHERE time_created > (strftime('%s', 'now') - 86400) * 1000;
   
   -- Cost last 7 days by agent
   SELECT json_extract(data, '$.agent') AS agent,
          round(SUM(json_extract(data, '$.cost')), 4) AS total_cost,
          COUNT(*) AS messages
   FROM message
   WHERE json_extract(data, '$.role') = 'assistant'
     AND time_created > (strftime('%s', 'now') - 7*86400) * 1000
   GROUP BY agent
   ORDER BY total_cost DESC
   LIMIT 10;
   ```

6. **Write investigation report** to `.memory-bank/findings/forensics/` using the report format from the `forensics-axiom` skill (Section 5).

7. **Return artifact paths** — always include the report path in the output.

## Output Contract

Return:
- `report_path`: path to the investigation report
- `sessions_analyzed`: count and list of session IDs examined
- `key_findings`: top findings from the investigation
- `cost_summary`: total cost analyzed (if applicable)
- `db_session_count`: total sessions in database (from walking skeleton check)

## Safety

- Database MUST be opened read-only: `sqlite3 "file:${DB}?mode=ro"`
- NEVER read `auth.json`, `mcp-auth.json`, or any `*auth*.json`
- NEVER do `LIKE '%keyword%'` on the `part` table
- All queries MUST use `LIMIT`
- Timestamps are milliseconds — divide by 1000

axiom:trace work_item=forensics-01 spec=specs/80-Session-Forensics-And-Self-Inspection.md
