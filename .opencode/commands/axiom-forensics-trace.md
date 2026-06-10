---
description: "Trace the subagent hierarchy for an OpenCode session — walks parent_id chain to produce a tree of parent and child sessions."
agent: forensics-axiom
---

Trace the subagent hierarchy for an OpenCode session. Walks the `parent_id` chain upward to find the root session, then traces all children downward to produce a complete hierarchy tree.

## Inputs

- `$SESSION` (required): Session ID to trace (e.g., `ses_abc123`). Can be any session in the hierarchy — the command will find the root.
- `$DEPTH` (optional): Maximum depth to trace. Default: 10
- `$OUTPUT` (optional): Output file path. Default: auto-generated in `.memory-bank/findings/forensics/`
- `$INCLUDE_COSTS` (optional): Include cost summary per session. Default: true

## Skills

Load on startup:
- `forensics-axiom` — Query patterns, schema, methodology

## Do

1. **Load the `forensics-axiom` skill**.

2. **Resolve database path**:
   ```bash
   DB="${OPENCODE_DATA_DIR:-$HOME/.local/share/opencode}/opencode.db"
   ```

3. **Find the root session** — walk parent_id chain upward:
   ```sql
   WITH RECURSIVE ancestors(id, title, parent_id, depth) AS (
       SELECT id, title, parent_id, 0 FROM session WHERE id = ?
       UNION ALL
       SELECT s.id, s.title, s.parent_id, a.depth - 1
       FROM session s JOIN ancestors a ON s.id = a.parent_id
       WHERE a.parent_id IS NOT NULL AND a.depth > -20
   )
   SELECT * FROM ancestors ORDER BY depth;
   ```

4. **Trace full hierarchy from root** — walk children downward:
   ```sql
   WITH RECURSIVE tree(id, title, parent_id, directory, depth,
                        time_created, time_updated) AS (
       SELECT id, title, parent_id, directory, 0, time_created, time_updated
       FROM session WHERE id = ?  -- root session ID
       UNION ALL
       SELECT s.id, s.title, s.parent_id, s.directory,
              t.depth + 1, s.time_created, s.time_updated
       FROM session s JOIN tree t ON s.parent_id = t.id
       WHERE t.depth < 10
   )
   SELECT id,
          substr('                ', 1, depth*2) || title AS indented_title,
          parent_id,
          depth,
          datetime(time_created/1000, 'unixepoch', 'localtime') AS created,
          datetime(time_updated/1000, 'unixepoch', 'localtime') AS updated
   FROM tree
   ORDER BY time_created;
   ```

5. **If $INCLUDE_COSTS** — get cost per session:
   ```sql
   SELECT session_id,
          round(SUM(json_extract(data, '$.cost')), 4) AS total_cost,
          COUNT(*) AS message_count,
          json_extract(data, '$.agent') AS agent,
          json_extract(data, '$.modelID') AS model
   FROM message
   WHERE session_id IN (/* session IDs from tree */)
     AND json_extract(data, '$.role') = 'assistant'
   GROUP BY session_id, json_extract(data, '$.agent')
   ORDER BY total_cost DESC;
   ```

6. **Format the hierarchy tree** as ASCII art:
   ```
   ses_root123 [tower-axiom] $0.45 — "Build forensics agent"
   ├── ses_child456 [dev-axiom] $0.12 — "Implement agent file"
   │   └── ses_grandchild789 [qa-axiom] $0.08 — "Verify implementation"
   └── ses_child012 [spec-verifier-axiom] $0.05 — "Verify spec alignment"
   ```

7. **Write trace report** to `.memory-bank/findings/forensics/trace-<session>-<timestamp>.md` with:
   - Root session info
   - Full hierarchy tree (ASCII art)
   - Per-session cost summary
   - Total cost for the entire hierarchy
   - Timeline (start → end)

8. **Return artifact paths**.

## Output Contract

Return:
- `report_path`: path to the trace report
- `root_session`: root session ID and title
- `session_count`: total sessions in hierarchy
- `total_cost`: total cost across all sessions in hierarchy
- `max_depth`: deepest level found
- `hierarchy_tree`: ASCII representation of the tree

## Safety

- Database MUST be opened read-only
- All recursive queries MUST have depth limits (default 10)
- All queries MUST use `LIMIT`
- NEVER read auth files

axiom:trace work_item=forensics-01 spec=specs/80-Session-Forensics-And-Self-Inspection.md
