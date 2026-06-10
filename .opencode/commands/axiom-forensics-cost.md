---
description: "Analyze OpenCode session costs — aggregate by agent, model, day, or session. Identify cost anomalies and top spenders."
agent: forensics-axiom
---

Analyze OpenCode session costs. Aggregates costs by agent, model, day, or session to identify anomalies, top spenders, and cost trends.

## Inputs

- `$BY` (optional): Aggregation dimension — `agent`, `model`, `day`, `session`, `all`. Default: `all`
- `$FROM` (optional): Start time — ISO date (`2026-01-01`) or relative (`-7d`, `-30d`). Default: `-30d`
- `$TO` (optional): End time. Default: now
- `$TOP` (optional): Show top N results per dimension. Default: 20
- `$SESSION` (optional): Analyze costs for a specific session (and its hierarchy)
- `$OUTPUT` (optional): Output file path. Default: auto-generated in `.memory-bank/findings/forensics/`

## Skills

Load on startup:
- `forensics-axiom` — Query patterns, schema, methodology

## Do

1. **Load the `forensics-axiom` skill**.

2. **Resolve database path**:
   ```bash
   DB="${OPENCODE_DATA_DIR:-$HOME/.local/share/opencode}/opencode.db"
   ```

3. **Total cost for time range**:
   ```sql
   SELECT round(SUM(json_extract(data, '$.cost')), 4) AS total_cost,
          COUNT(*) AS total_messages,
          COUNT(DISTINCT session_id) AS total_sessions
   FROM message
   WHERE json_extract(data, '$.role') = 'assistant'
     AND time_created > (strftime('%s', 'now') - 30*86400) * 1000;
   ```

4. **Cost by agent** (when $BY = 'agent' or 'all'):
   ```sql
   SELECT json_extract(data, '$.agent') AS agent,
          round(SUM(json_extract(data, '$.cost')), 4) AS total_cost,
          COUNT(*) AS messages,
          round(AVG(json_extract(data, '$.cost')), 6) AS avg_cost_per_msg,
          round(MAX(json_extract(data, '$.cost')), 4) AS max_single_cost
   FROM message
   WHERE json_extract(data, '$.role') = 'assistant'
     AND time_created > (strftime('%s', 'now') - 30*86400) * 1000
   GROUP BY agent
   ORDER BY total_cost DESC
   LIMIT 20;
   ```

5. **Cost by model** (when $BY = 'model' or 'all'):
   ```sql
   SELECT json_extract(data, '$.modelID') AS model,
          round(SUM(json_extract(data, '$.cost')), 4) AS total_cost,
          COUNT(*) AS messages,
          round(AVG(json_extract(data, '$.cost')), 6) AS avg_cost_per_msg
   FROM message
   WHERE json_extract(data, '$.role') = 'assistant'
     AND time_created > (strftime('%s', 'now') - 30*86400) * 1000
   GROUP BY model
   ORDER BY total_cost DESC
   LIMIT 20;
   ```

6. **Cost by day** (when $BY = 'day' or 'all'):
   ```sql
   SELECT date(time_created/1000, 'unixepoch', 'localtime') AS day,
          round(SUM(json_extract(data, '$.cost')), 4) AS daily_cost,
          COUNT(*) AS messages,
          COUNT(DISTINCT session_id) AS sessions
   FROM message
   WHERE json_extract(data, '$.role') = 'assistant'
     AND time_created > (strftime('%s', 'now') - 30*86400) * 1000
   GROUP BY day
   ORDER BY day DESC
   LIMIT 30;
   ```

7. **Top sessions by cost** (when $BY = 'session' or 'all'):
   ```sql
   SELECT m.session_id,
          s.title,
          round(SUM(json_extract(m.data, '$.cost')), 4) AS total_cost,
          COUNT(*) AS messages,
          json_extract(m.data, '$.agent') AS agent,
          datetime(s.time_created/1000, 'unixepoch', 'localtime') AS created
   FROM message m
   JOIN session s ON s.id = m.session_id
   WHERE json_extract(m.data, '$.role') = 'assistant'
     AND m.time_created > (strftime('%s', 'now') - 30*86400) * 1000
   GROUP BY m.session_id
   ORDER BY total_cost DESC
   LIMIT 20;
   ```

8. **Token breakdown** (input/output/cache):
   ```sql
   SELECT json_extract(data, '$.agent') AS agent,
          SUM(json_extract(data, '$.tokens.input')) AS input_tokens,
          SUM(json_extract(data, '$.tokens.output')) AS output_tokens,
          SUM(json_extract(data, '$.tokens.cache.read')) AS cache_read,
          SUM(json_extract(data, '$.tokens.cache.write')) AS cache_write
   FROM message
   WHERE json_extract(data, '$.role') = 'assistant'
     AND time_created > (strftime('%s', 'now') - 30*86400) * 1000
   GROUP BY agent
   ORDER BY input_tokens DESC
   LIMIT 20;
   ```

9. **Identify anomalies** — flag sessions with cost > 2x the average.

10. **Write cost report** to `.memory-bank/findings/forensics/cost-<timestamp>.md` with:
    - Time range analyzed
    - Total cost and message count
    - Cost by agent table
    - Cost by model table
    - Daily cost trend
    - Top sessions by cost
    - Anomalies flagged

11. **Return artifact paths**.

## Output Contract

Return:
- `report_path`: path to the cost report
- `total_cost`: total cost for the analyzed period
- `total_sessions`: sessions analyzed
- `top_agent`: highest-cost agent
- `top_model`: highest-cost model
- `anomalies`: sessions with anomalous costs

## Safety

- Database MUST be opened read-only
- All queries MUST use `LIMIT`
- NEVER read auth files
- Timestamps are milliseconds — divide by 1000

axiom:trace work_item=forensics-01 spec=specs/80-Session-Forensics-And-Self-Inspection.md
