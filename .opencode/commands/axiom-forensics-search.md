---
description: "Search OpenCode sessions by keyword, agent, model, or time range. Searches tool outputs first (fast), then correlates to sessions."
agent: forensics-axiom
---

Search OpenCode sessions for a keyword, agent, model, or time range. Uses the fast-path grep strategy: search `tool-output/` files first, then correlate to sessions via timestamps.

## Inputs

- `$KEYWORD` (optional): Keyword to search for in tool outputs and session content
- `$AGENT` (optional): Filter sessions by agent name (e.g., `dev-axiom`, `tower-axiom`)
- `$MODEL` (optional): Filter sessions by model ID (e.g., `anthropic.claude-sonnet-4-6`)
- `$FROM` (optional): Start time — ISO date (`2026-01-01`) or relative (`-7d`, `-30d`). Default: `-7d`
- `$TO` (optional): End time. Default: now
- `$LIMIT` (optional): Max results. Default: 50, max: 200
- `$OUTPUT` (optional): Output file path. Default: auto-generated in `.memory-bank/findings/forensics/`

## Skills

Load on startup:
- `forensics-axiom` — Query patterns, schema, methodology

## Do

1. **Load the `forensics-axiom` skill**.

2. **Resolve paths**:
   ```bash
   DB="${OPENCODE_DATA_DIR:-$HOME/.local/share/opencode}/opencode.db"
   TOOL_DIR="${OPENCODE_DATA_DIR:-$HOME/.local/share/opencode}/tool-output"
   ```

3. **If $KEYWORD provided — fast-path grep first**:
   ```bash
   # Step 1: grep tool outputs (fast — plain text files, no SQL)
   grep -rl "${KEYWORD}" "${TOOL_DIR}/" 2>/dev/null | head -100
   
   # Step 2: get timestamps of matched files to correlate to sessions
   # macOS:
   stat -f "%m %N" <matched-file>
   # Linux:
   stat -c "%Y %n" <matched-file>
   ```
   
   **NEVER** do `SELECT * FROM part WHERE data LIKE '%keyword%'` — this scans 2.5M rows.

4. **Query sessions by filters**:
   ```sql
   SELECT s.id, s.title, s.parent_id, s.directory,
          datetime(s.time_created/1000, 'unixepoch', 'localtime') AS created,
          datetime(s.time_updated/1000, 'unixepoch', 'localtime') AS updated
   FROM session s
   WHERE 1=1
   -- Add time filter if $FROM provided:
   -- AND s.time_created/1000 >= strftime('%s', '$FROM_ISO')
   -- Add agent filter via message join if $AGENT provided
   ORDER BY s.time_created DESC
   LIMIT 50;
   ```

5. **If $AGENT or $MODEL filter** — join through message table:
   ```sql
   SELECT DISTINCT s.id, s.title,
          datetime(s.time_created/1000, 'unixepoch', 'localtime') AS created,
          json_extract(m.data, '$.agent') AS agent,
          json_extract(m.data, '$.modelID') AS model,
          round(SUM(json_extract(m.data, '$.cost')), 4) AS total_cost
   FROM session s
   JOIN message m ON m.session_id = s.id
   WHERE json_extract(m.data, '$.role') = 'assistant'
   -- AND json_extract(m.data, '$.agent') = '$AGENT'
   -- AND json_extract(m.data, '$.modelID') LIKE '%$MODEL%'
   GROUP BY s.id
   ORDER BY s.time_created DESC
   LIMIT 50;
   ```

6. **Correlate grep results to sessions** — for each matched tool-output file, find sessions active at that timestamp:
   ```sql
   SELECT id, title, directory,
          datetime(time_created/1000, 'unixepoch', 'localtime') AS created,
          datetime(time_updated/1000, 'unixepoch', 'localtime') AS updated
   FROM session
   WHERE time_created/1000 <= ? AND time_updated/1000 >= ?
   ORDER BY time_created DESC
   LIMIT 20;
   ```

7. **Write search results** to `.memory-bank/findings/forensics/search-<timestamp>.md` with:
   - Search parameters used
   - Matched tool-output files (with snippets)
   - Correlated sessions (ID, title, agent, cost, time)
   - Total matches found

8. **Return artifact paths** — always include the results file path.

## Output Contract

Return:
- `report_path`: path to the search results file
- `matches_found`: count of tool-output files matching keyword
- `sessions_found`: count of sessions matching filters
- `sessions`: list of matching session IDs with titles and costs

## Safety

- Database MUST be opened read-only
- NEVER `LIKE '%keyword%'` on `part` table — use grep on tool-output/ instead
- All queries MUST use `LIMIT`
- NEVER read auth files

axiom:trace work_item=forensics-01 spec=specs/80-Session-Forensics-And-Self-Inspection.md
