# SCN-CONSENSUS-001 — s2 read+edit+glob coverage, default-ALL live run

- Date: 2026-11-25 18:29 UTC (run window started ~18:22 UTC)
- Worktree: /home/kara/worktrees/consensus-SCN-CONSENSUS-001 (branch wt/SCN-CONSENSUS-001, base 16be01d918ef351859d61bd0cbc52dfaeae7e6a8)
- Command: `PORT=18463 bash scripts/e2e/harness-scenarios.sh` (worktree root, no selector, no --auto-approve; DEEPSEEK_API_KEY picked up from ~/.hermes/.env)
- Result: exit code 0, 5/5 pass, failures: 0. Single run — no transient retry needed.
- Build sanity: `go build ./...` exit 0 (plus `bash -n` on the runner).
- Runner changed: scripts/e2e/harness-scenarios.sh (s2_read_file only; ALL list untouched).

## Run stdout (verbatim)

    server up on :18463 (pid 1973924)
    --- s1_write_file
    {"scenario":"s1","status":"pass","detail":"wrote /mnt/ssd/tmp/consensus-scn.gLfqnO/tmp/consensus-workspaces/be5f3b84-2ef2-4ede-99dc-3b67f5235ae2/scn1.txt"}
    --- s2_read_file
    {"scenario":"s2","status":"pass","detail":"read+edit+glob tool_results: read=1 edit_calls=1 edit_content=1 glob=1"}
    --- s3_bash
    {"scenario":"s3","status":"pass","detail":"bash executed"}
    --- s4_conversation_loop
    {"scenario":"s4","status":"pass","detail":"two turns, two files"}
    --- s5_containment
    {"scenario":"s5","status":"pass","detail":"no escape file outside the session workspace"}
    === summary ===
    s1_write_file=PASS
    s2_read_file=PASS
    s3_bash=PASS
    s4_conversation_loop=PASS
    s5_containment=PASS
    scenarios run: 5
    failures: 0
    EXIT_CODE=0

## s2 proof queries (sqlite3 tool_results, counts from the run's DB)

The s2 workspace was seeded with: seed.txt = `SEED-READ-42`, edit-target.txt = `STALE-DRAFT v1`,
reports/glob-found.log = `SEED-GLOB-99`. The session was instructed in three turns to (a) read
seed.txt, (b) edit edit-target.txt to contain SEED-EDIT-77 and read it back, (c) glob `reports/*.log`.

Note on the edit leg: the edit tool's own result output is `edited N occurrence(s) in <file>` and
does not echo the new content, so leg 2 is proven two ways — an edit tool call completed, and
SEED-EDIT-77 visible in a tool result via the instructed read-back. All three legs are backed by
tool_results rows, as required.

    -- leg 1 (read): count = 1
    SELECT count(*) FROM tool_results r JOIN tool_requests q ON r.request_id=q.id
    WHERE q.tool_name='read' AND r.is_error=0 AND r.output LIKE '%SEED-READ-42%';

    -- leg 2 (edit call): count = 1
    SELECT count(*) FROM tool_results r JOIN tool_requests q ON r.request_id=q.id
    WHERE q.tool_name='edit' AND r.is_error=0;

    -- leg 2b (edit content via read-back): count = 1
    SELECT count(*) FROM tool_results WHERE is_error=0 AND output LIKE '%SEED-EDIT-77%';

    -- leg 3 (glob): count = 1
    SELECT count(*) FROM tool_results r JOIN tool_requests q ON r.request_id=q.id
    WHERE q.tool_name='glob' AND r.is_error=0 AND r.output LIKE '%glob-found.log%';

s2 emitted exactly one pass line only after all four counts were >= 1
(`read=1 edit_calls=1 edit_content=1 glob=1` in the detail). The DB was deleted with the throwaway
state dir on exit (runner default, no --keep), so the counts above are recorded at run time by the
scenario's own sqlite3 assertions.
