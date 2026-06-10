---
name: pattern-monitor-logs
description: >-
  Start a reactive watch on a log file or any text file, wait for matching
  lines, query the matches, store findings, and stop the watch cleanly.
  Uses ShellOps watch tools + tree.commit. Runtime-tested 2026-05-18 with live daemon.
version: "1.0"
tags:
  vertical: [monitoring, observability, workflow]
  category: pattern
  core: false
spec: specs/121-Pattern-Generator.md
trigger_conditions:
  - "You need to watch a log file for errors while a process runs"
  - "You want to reactively grep a file as it grows"
  - "Verifying that a process produces expected log output"
  - "Monitoring a deployment or test run in real time"
tools_required:
  - shellops_health
  - shellops_watch_start
  - shellops_watch_query
  - shellops_watch_list
  - shellops_watch_stop
  - tree_commit (optional — persist matches)
  - stash_push (optional — handoff findings)
estimated_steps: 5
estimated_duration: "10s setup + watch duration"
lifecycle:
  state: active
  created: "2026-05-18"
  last_validated: "2026-05-18"
  validation_count: 1
---

# Pattern: Monitor Logs

Start a reactive file watch, let it accumulate matches while a process runs,
query the matches on demand, and store findings. Uses ShellOps watch tools.

**Spec**: `specs/121-Pattern-Generator.md`
**Observed from**: 1 real execution on 2026-05-18 (watched `.memory-bank/` file for pattern matches)

<!-- axiom:trace work_item=pattern-design-01 spec=specs/121-Pattern-Generator.md -->

---

## Prerequisites

| Requirement | How to Verify | Expected | If Missing |
|-------------|---------------|----------|------------|
| ShellOps daemon running | `shellops_health` → `{"status":"ok"}` | status: ok | Start: `(_tmp/shellops-bin start --port 9876 --root . &)` then wait 2s |
| File to watch exists | `ls <file_path>` | file exists | Create it first OR use a file that will be created by your process |
| Tree Memory (optional) | `tree_status` → `initialized: true` | | Only needed if storing findings in tree |

---

## Tool Chain

| Step | Purpose | Tool | Key Input | Key Output | On Failure | Criticality |
|------|---------|------|-----------|------------|------------|-------------|
| 0 | Verify daemon | `shellops_health` | — | `{status: "ok"}` | Start daemon; ABORT if still not up | Required |
| 1 | Start watch | `shellops_watch_start` | `{id, file_path, pattern, agent_id}` | `{watch_id, active: true, match_count: 0}` | ABORT | Required |
| 2 | Run your process | (bash / other tools) | your command | — | Continue watching | Context-dependent |
| 3 | Query matches | `shellops_watch_query` | `{watch_id}` | `{match_count, matches}` | WARN if empty | Required |
| 4 | Store findings | `tree_commit` (or `stash_push`) | `{file, content}` | `{status: "committed"}` | Use stash fallback | Optional |
| 5 | Stop watch | `shellops_watch_stop` | `{id: logical_id}` | `{status: "stopped"}` | Log and continue (stale watches are OK) | Required |

> **Important IDs**: `shellops_watch_start` takes `id` (your logical name) but returns `watch_id` (internal UUID like `watch-5ef8656a`). `shellops_watch_query` and `shellops_watch_stop` take the **internal `watch_id`**, not your logical `id`.

---

## Pseudocode

```text
PATTERN monitor_logs(file_path, regex_pattern, process_fn, watch_name?):

  // Step 0: Verify daemon
  health = CALL shellops_health()
  IF health.status != "ok":
    ABORT("ShellOps daemon not running. Start with: _tmp/shellops-bin start --port 9876 &")

  // Step 1: Start watch
  logical_id = watch_name OR "watch-{slug(file_path)}-{date()}"
  watch = CALL shellops_watch_start(
    id: logical_id,
    file_path: file_path,          // MUST be absolute path
    pattern: regex_pattern,        // regex: "ERROR|WARN" or "SIG-[0-9]+"
    agent_id: "my-agent-name"
  )
  watch_id = watch.watch_id        // ← use THIS for query/stop, not logical_id
  IF NOT watch.active:
    ABORT("Watch failed to start")

  // Step 2: Run your process
  result = process_fn()  // run a command, wait for a step, etc.

  // Step 3: Query matches
  matches = CALL shellops_watch_query(watch_id: watch_id)
  match_count = matches.match_count

  // Step 4: Store findings (optional)
  IF match_count > 0:
    CALL tree_commit(
      file: "findings/monitor-{slug(logical_id)}-{date()}.json",
      content: JSON.stringify({ watch_id, file_path, pattern: regex_pattern, match_count, matches: matches.matches }),
      message: "monitor: {logical_id} — {match_count} matches"
    )
  ELSE:
    CALL stash_push(
      summary: "Monitor run for {logical_id}: 0 matches (clean)",
      tags: "monitoring,clean"
    )

  // Step 5: Stop watch (always clean up)
  CALL shellops_watch_stop(id: watch_id)

  RETURN {
    status: "PATTERN_COMPLETE",
    watch_id: watch_id,
    match_count: match_count,
    verdict: "CLEAN" if match_count == 0 else "MATCHES_FOUND"
  }
```

---

## On-Track / Off-Track Signals

| Signal | Type | After Step | Indicator | Response |
|--------|------|-----------|-----------|----------|
| SIG-01 | on_track | 0 | `shellops_health` → `{"status":"ok"}` | Continue |
| SIG-02 | abort | 0 | daemon not reachable | Start daemon; wait 2s; retry once; ABORT |
| SIG-03 | on_track | 1 | `shellops_watch_start` returns `{watch_id, active: true, match_count: 0}` | Store `watch_id` |
| SIG-04 | off_track | 1 | `shellops_watch_start` returns error or `active: false` | Check file path is absolute; check daemon; ABORT |
| SIG-05 | on_track | 3 | `shellops_watch_query` returns `{match_count: N}` | `N > 0` = matches found; `N = 0` = clean |
| SIG-06 | off_track | 3 | watch_id not found | Watch may have been garbage-collected; check `shellops_watch_list` |
| SIG-07 | on_track | 5 | `shellops_watch_stop` returns `{status: "stopped"}` | PATTERN_COMPLETE |
| SIG-08 | off_track | 5 | stop fails | WARN and log stale watch_id; stale watches expire automatically |

---

## Adjustment Protocol

```
File path MUST be absolute:
  → shellops_watch_start requires absolute file_path
  → SIG-04: relative paths silently fail to match
  → FIX: prepend repo root: "/home/coder/code/Axiom/" + relative_path

watch_id vs logical id confusion:
  → watch_start takes id (your name) but returns watch_id (UUID)
  → watch_query and watch_stop take watch_id, NOT id
  → ALWAYS store the returned watch_id immediately after watch_start

match_count = 0 but matches expected:
  → File writes happen AFTER watch polls → small race window
  → Add a short sleep (1-2s) after the process completes before querying
  → Or query in a loop with up to 3 retries

Pattern is a regex — escape special chars:
  → "errors: [" → "errors: \\[" (escape the bracket)
  → "|" for OR: "ERROR|WARN|CRIT"
```

---

## Example Execution Trace (Observation #1: watching backlog file)

```
─── Pattern Instance: monitor-logs ───
Input: {
  file_path: "/home/coder/code/Axiom/.memory-bank/work-items/shellops-zod-migration-01/findings-backlog.md",
  pattern: "WONT-FIX|CLEARED|finding"
}

[Step 0] shellops_health
→ shellops_health()
← { "status": "ok" }
✓ SIG-01

[Step 1] shellops_watch_start
→ shellops_watch_start({
    id: "watch-verification-md",
    file_path: "/home/coder/.../findings-backlog.md",
    pattern: "WONT-FIX|CLEARED|finding",
    agent_id: "dispatch-axiom"
  })
← {
    watch_id: "watch-5ef8656a",   ← THIS is used for query/stop
    active: true,
    match_count: 0
  }
✓ SIG-03: watch_id = "watch-5ef8656a"

[Step 2] (process: appended a line to the watched file)

[Step 3] shellops_watch_query
→ shellops_watch_query({ watch_id: "watch-5ef8656a" })
← { watch_id: "watch-5ef8656a", match_count: 1 }
✓ SIG-05: 1 match found

[Step 5] shellops_watch_stop
→ shellops_watch_stop({ id: "watch-5ef8656a" })
← { "status": "stopped" }
✓ SIG-07

RESULT: PATTERN_COMPLETE
  watch_id: watch-5ef8656a
  match_count: 1
  verdict: MATCHES_FOUND
```

---

## When NOT to Use This Pattern

- **Streaming a process in real-time**: use `shellops_terminal_run` instead (returns live stdout)
- **Searching an existing file once**: use `Grep` tool directly — no daemon needed
- **Watching more than one file**: start multiple watches with different logical IDs
- **The file doesn't exist yet**: start the watch BEFORE launching the process that creates it
