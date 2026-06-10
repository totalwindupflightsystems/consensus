---
name: concurrent-client-server-testing
description: >
  Multi-agent concurrent client/server API testing with a fixed runtime window.
  One agent owns server uptime (offset start + bounded duration), while worker
  agents synchronize to shared epoch timestamps, run protocol tests in parallel,
  and produce per-view evidence that can be merged into a single report.
version: "1.0"
tags:
  vertical: [coding]
  category: testing
  core: false
---

# Concurrent Client-Server Testing Skill

Use this skill when you want controlled, parallel API testing where:
- one agent starts the server,
- many client agents run different test slices,
- all slices run inside the same fixed window,
- and all reports are merged consistently.

## Core Contract

1. Define a shared schedule first (UTC epoch times).
2. Server owner starts server with offset + hard stop.
3. Worker agents sync to the same `CLIENT_START_EPOCH` and `END_EPOCH`.
4. Workers may load context before start, then re-check time.
5. Workers run bounded tests only within the active window.
6. Every worker writes a structured report.
7. Coordinator merges worker reports into one summary.

## Shared Timing Model

Coordinator defines:

```bash
NOW_EPOCH="$(date -u +%s)"
SERVER_OFFSET_SEC=90
SERVER_BOOT_LEAD_SEC=5
SERVER_WINDOW_SEC=420
CLIENT_START_EPOCH="$((NOW_EPOCH + SERVER_OFFSET_SEC))"
SERVER_START_EPOCH="$((CLIENT_START_EPOCH - SERVER_BOOT_LEAD_SEC))"
if [ "$SERVER_START_EPOCH" -lt "$NOW_EPOCH" ]; then SERVER_START_EPOCH="$NOW_EPOCH"; fi
END_EPOCH="$((CLIENT_START_EPOCH + SERVER_WINDOW_SEC))"
```

Share these exact values with all agents.

### Why this model works

- Offset gives workers time to load specs/context.
- Boot lead starts server slightly earlier (for example, 5s) so clients do not hit early startup errors.
- Fixed end timestamp forces synchronized stop.
- The same epoch values prevent drift between agents.

## Role 1: Server Owner Agent

The server owner is the only agent that runs the server process.

### Required inputs

- `SERVER_START_EPOCH`
- `CLIENT_START_EPOCH`
- `END_EPOCH`
- `SERVER_WINDOW_SEC`
- server command (example: `axiom serve --port 8100`)

### Server owner sequence

```bash
NOW_EPOCH="$(date -u +%s)"
WAIT_SEC="$((SERVER_START_EPOCH - NOW_EPOCH))"
if [ "$WAIT_SEC" -lt 0 ]; then WAIT_SEC=0; fi

# Preferred path (uses timeout command as requested)
sleep "$WAIT_SEC" && timeout "$SERVER_WINDOW_SEC" axiom serve --port 8100
```

### Health gate (recommended)

Before workers start heavy tests, they should verify readiness:

```bash
curl -sf "http://127.0.0.1:8100/health"
```

If health fails, worker marks slice `blocked` and records output.

## Role 2: Worker Client Agents

Each worker receives:

- shared timestamps (`CLIENT_START_EPOCH`, `END_EPOCH`),
- test slice id,
- commands for that slice,
- report output path.

### Worker sync algorithm (must follow)

1. Capture time on receipt.
2. Load context (specs/docs/test files).
3. Capture time again.
4. Sleep until `CLIENT_START_EPOCH` if still early.
5. Compute remaining budget from `END_EPOCH`.
6. Run tests bounded by `timeout` using remaining budget.
7. Write report file.

### Worker timing snippet

```bash
NOW1="$(date -u +%s)"
# ...load context...
NOW2="$(date -u +%s)"

WAIT_SEC="$((CLIENT_START_EPOCH - NOW2))"
if [ "$WAIT_SEC" -gt 0 ]; then
  sleep "$WAIT_SEC"
fi

NOW3="$(date -u +%s)"
REMAINING_SEC="$((END_EPOCH - NOW3))"
if [ "$REMAINING_SEC" -le 0 ]; then
  echo "window missed" >&2
  exit 2
fi

timeout "$REMAINING_SEC" python3 -m pytest tests/protocol/ -q -k "${SLICE_EXPR}"
```

## Recommended Parallel Slices

Use multiple workers with non-overlapping focus:

1. `http-positive` -> happy-path endpoint contracts
2. `http-negative` -> 4xx/5xx/input/auth/content-type checks
3. `sse-contract` -> stream headers/events/resume/error handling
4. `schema-drift` -> OpenAPI/runtime contract parity
5. `load-boundary` -> boundary, timeout, concurrency checks

## Report Format (Per Worker)

Each worker writes one markdown report:

`reports/concurrent/<RUN_ID>/<SLICE_ID>.md`

Template:

```markdown
# Slice Report: <SLICE_ID>

- status: pass|fail|blocked|timeout
- start_epoch: <int>
- end_epoch: <int>
- command: `<exact command>`
- exit_code: <int>
- duration_sec: <int>

## Checks
- total: <int>
- passed: <int>
- failed: <int>
- skipped: <int>

## Key Findings
- <bullet>

## Evidence
- stdout/stderr: `<path>`
- artifacts: `<path>`
```

## Merge Report (Coordinator)

After all workers exit, coordinator generates:

`reports/concurrent/<RUN_ID>/summary.md`

Required sections:

1. window metadata (start/end, offset, duration)
2. per-slice status table
3. failures grouped by endpoint/protocol
4. pass/fail decision
5. follow-up work items

## Fail-Closed Rules

- Missing shared timestamps -> do not run; mark blocked.
- Worker starts after `END_EPOCH` -> fail slice as `window_missed`.
- Health check fails at start -> mark blocked, no fake pass.
- Timeout hit -> mark `timeout`, include partial output.
- No report file -> slice is failed.

## Command Bundle Example

Coordinator can pass this payload to all workers:

```text
RUN_ID=2026-02-26T08-00-00Z_01
SERVER_BOOT_LEAD_SEC=5
SERVER_START_EPOCH=1772092795
CLIENT_START_EPOCH=1772092800
END_EPOCH=1772093220
BASE_URL=http://127.0.0.1:8100
REPORT_DIR=reports/concurrent/2026-02-26T08-00-00Z_01
```

Worker-specific:

```text
SLICE_ID=http-negative
SLICE_EXPR=negative
TEST_CMD=python3 -m pytest tests/protocol/test_external_api_contract.py -q -k negative
```

## Integration with Existing Skills

- Pair with `protocol-testing` for slice definitions and protocol tool choices.
- Pair with `enterprise-testing-standard` for tier mapping and evidence gates.
- Pair with `test-quality-gates-axiom` to ensure tests are high-signal.

## Operator Notes

- Use UTC only (`date -u`).
- Keep one source of truth for timestamps (coordinator message).
- Prefer small offset (60-180 sec) and explicit window (300-900 sec).
- Keep server command deterministic and single-owner.
