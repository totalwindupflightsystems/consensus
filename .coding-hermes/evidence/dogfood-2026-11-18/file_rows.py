#!/usr/bin/env python3
"""Dogfood 2026-11-18: stage six board rows via boardctl create (argv lists)."""

import subprocess
import sys

BOARD = ["-C", "/home/kara/totalwindupflightsystems/consensus/.coding-hermes/board"]

ROWS = [
    dict(
        id="DF-CONSENSUS-083",
        type="bug",
        priority="P1",
        complexity="M",
        title="P1 — session iteration budget (planning_max_turns=10) is a lifetime counter: ordinary multi-turn use permanently tool-kills the session",
        reasoning="Dogfood 2026-11-18, scratch :18461 @ HEAD 0e697977, real DeepSeek key. A 5-message session exhausted iteration 10/10 after ordinary turns; every later tool hand-off is refused and the session went status=failed. The gate reads session.iteration (bumped by EVERY provider call, all turns) against planning_max_turns, so the documented per-run max_turns behaves as a session-lifetime budget. No remedy: PATCH /api/v1/sessions/{id} accepts status only (sessions.go:158-161 rejects planning_max_turns with 'status field is required'); docs/API.md:182 promises 'or settings'. Fresh sessions work (a238a831 completed the same task in ~8s) until they too reach 10. Fix direction: per-run turn window or an auto-reset on user_message, plus an API/settings path to raise it.",
        ac="Reproduce on a fresh sqlite install: send 5 tool-involving user messages to one session (alias-name bash), then send a 6th asking for a bash call; observe serve log 'planning: tool hand-off refused: session iteration budget exhausted (10/10)' (internal/harness/planning.go:1292-1307) and GET session status=failed. Then show PATCH {\"planning_max_turns\":50} returns INVALID_REQUEST 'status field is required' (internal/api/sessions.go:158-161) while the session column planning_max_turns exists (sessions table). Fix lands when a multi-turn session can still hand off tools after >10 lifetime provider calls, or the limit is documented as lifetime AND a supported knob raises it.",
    ),
    dict(
        id="DF-CONSENSUS-084",
        type="bug",
        priority="P1",
        complexity="M",
        title="P1 — tool-continuation turn: billed assistant reply never persisted (no text_block, last_message stays null); queued message consumed with no call and no answer",
        reasoning="Dogfood 2026-11-18, session a238a831-e1b9-4dd1-b22a-eb8126097835, scratch :18461 @ 0e697977. Continuation after a successful bash call billed 311 completion tokens (serve log 20:15:19 'llm: response received ... completion_tokens=311' then 'planning: committing turn=1') yet memory_events holds NO text_block for that iteration, last_message stayed null, and iteration_commits has zero rows for the session. The user polling GET /sessions/{id} per docs/API.md §message sees nothing after a successful tool call. Asymmetric: session 933db6f9 DID persist its continuation reply (text_block id=6 'Created hello.py...', iter 6) — the loss is path/race dependent. Second leg: a user message sent while the harness was mid-continuation ('go', memory_events id=12) was consumed at iteration 2 with NO provider call (agent_billing count=3 total), no reply, no error — a silently swallowed message. The next PLAIN turn persists fine (text_block id=15) and is stateful (reply cites 17*23=391 from the tool result), so multi-turn memory works; the defect is specific to the continuation commit path and to messages that arrive during it.",
        ac="Reproduce on a fresh install: POST a session, send 'Use the bash tool to compute 17*23 and report the exact stdout', then while it is in planning/tool_exec send a second message; after it settles assert GET /sessions/{id} .last_message == null, GET /memory has no text_block for the continuation iteration, agent_billing shows the 311-token call, and the raced message has no billing row. Fix lands when the billed continuation reply always produces a text_block + last_message, and a message consumed by the planner either gets a billed answer or an explicit queued/rejected surface.",
    ),
    dict(
        id="DF-CONSENSUS-085",
        type="bug",
        priority="P2",
        complexity="S",
        title="P2 — fresh-install first shell task fails: model-native tool name (shell_exec) does not resolve; registry seeds aliases for read/write but not shell",
        reasoning="Dogfood 2026-11-18, first real task on a fresh install asked the agent to 'create hello.py and run it with the shell'. The model requested tool 'shell_exec' (tool_requests row 1: status=failed, tool_results TOOL_ERROR 'tool \"shell_exec\" is not registered or enabled') while the shipped registry (migration 032) seeds rows read->read_file, write->write_file, edit, glob, bash — handler_ref aliasing (internal/tools/execute.go:131-146) covers renamed rows but NO shell name the model naturally picks (shell_exec/shell_execute/run_shell). The flagship README flow failed on the first attempt and only recovered when the user said 'use the bash tool (that exact name)'. Fail-closed behavior itself is correct (DF-CONSENSUS-069's fail-open premise is reversed on this HEAD: status=failed, error surfaced to the model, honest assistant reply). Fix direction: seed alias rows (shell/shell_exec/shell_execute/run_command -> bash handler_ref) in migration 032 or extend the alias arm with a canonical alias table.",
        ac="On a fresh sqlite install, GET /api/v1/tools shows bash but a model turn requesting shell_exec produces tool_results error_code=TOOL_ERROR 'tool \"shell_exec\" is not registered or enabled' (tool_requests status=failed); sqlite dump of tools_registry shows no row with name or handler_ref 'shell_exec'. Fix lands when the same natural-language task completes on the FIRST attempt on a fresh install, or the docs' task guidance tells users the exact tool name up front.",
    ),
    dict(
        id="DF-CONSENSUS-086",
        type="bug",
        priority="P3",
        complexity="S",
        title="P3 — README's suggested .env (DEEPSEEK_API_KEY + CONSENSUS_API_KEY) self-sabotages: auth key wins rank 1 of the llm.api_key ladder",
        reasoning="Dogfood 2026-11-18: booting with CONSENSUS_API_KEY (the API auth key) AND DEEPSEEK_API_KEY made the server use the cs_ak_ auth key as the LLM key (llm key probe 401, 'LLM AUTH FAILED for key ...81c9'), circuit breaker tripped at 3 consecutive 403/401s, session paused — exactly the README:442 '# .env file with DEEPSEEK_API_KEY, CONSENSUS_API_KEY' recipe. docs/CONFIG.md:105 documents CONSENSUS_API_KEY as ladder rank 1 for llm.api_key (internal/config/config.go:422-425), so it is intentional-but-trapping: the documented fresh-install .env cannot work as written. The startup probe caught it loudly (excellent — ENV-CONSENSUS-1), but a user following the README burns a boot + breaker trip before reading CONFIG.md. Fix direction: rank CONSENSUS_API_KEY for llm.api_key only when llm.provider is explicitly configured for the same surface, or change the README .env guidance to separate CONSENSUS_API_KEY from LLM keys.",
        ac="Reproduce: source an env with both DEEPSEEK_API_KEY=<valid> and CONSENSUS_API_KEY=<admin key>, serve a fresh sqlite DB; observe the llm key probe 401 naming the cs_ak_ tail and base_url=https://api.openai.com/v1 (config.go:422-425), README.md:442 shows the recipe. Fix lands when the README .env recipe boots with a working provider key, or the probe's error message explicitly says 'CONSENSUS_API_KEY was used as the LLM key because it set llm.api_key (ladder rank 1)'.",
    ),
    dict(
        id="DF-CONSENSUS-073-R2",
        type="bug",
        priority="P2",
        complexity="S",
        title="P2 — recurrence: GET /api/v1/sessions/{id}/iterations still returns [] for sessions with iteration>0",
        reasoning="Dogfood 2026-11-18 re-proof on HEAD 0e697977: session 933db6f9 (iteration 10) and a238a831 (iteration 4) both answer GET .../iterations with []; the sessions rows report iteration>0 and agent_billing rows exist per iteration. Original DF-CONSENSUS-073 evidence (2026-11-14) unchanged in kind. Owner should close 073 with this row's evidence once the endpoint is implemented or its promise corrected in docs.",
        ac="GET /api/v1/sessions/a238a831-e1b9-4dd1-b22a-eb8126097835/iterations on the 2026-11-18 scratch DB returns [] while GET /sessions/{id} reports iteration>=4 and agent_billing has >=3 rows; fix lands when the endpoint returns per-iteration data or is removed from docs/API.md.",
    ),
    dict(
        id="DF-CONSENSUS-074-R2",
        type="bug",
        priority="P3",
        complexity="S",
        title="P3 — recurrence: sqlite 'database is locked (517)' on the planning path, still masked by the 10ms retry",
        reasoning="Dogfood 2026-11-18 re-proof on HEAD 0e697977, single-user serve: serve log shows 'planning: retrying user-message consumption after sqlite busy ... attempt=1 backoff=10ms error=\"sqlite tx: exec: database is locked (517)\"' at 20:12:53, 20:13:07 and 20:15:19 — three occurrences across two sessions in ~10 minutes of light single-client use. Original DF-CONSENSUS-074 (2026-11-14) premise unchanged: WAL/masking retry hides it today.",
        ac="grep serve log of a single-user session run for 'database is locked (517)' returns >=1 hit with the 'retrying user-message consumption' WARN; fix lands when the planning write path no longer contends (busy_timeout/WAL or serialized writes) or the retry is made durable (bounded retries, not one 10ms attempt).",
    ),
]

fails = 0
for r in ROWS:
    cmd = (
        ["boardctl"]
        + BOARD
        + [
            "create",
            "--id",
            r["id"],
            "--title",
            r["title"],
            "--type",
            r["type"],
            "--status",
            "pending",
            "--priority",
            r["priority"],
            "--complexity",
            str({"M": 3, "S": 2}[r["complexity"]]),
            "--reasoning",
            r["reasoning"],
            "--acceptance-criteria",
            r["ac"],
            "--capability-tags",
            "dogfood,2026-11-18",
        ]
    )
    p = subprocess.run(cmd, capture_output=True, text=True)
    print(r["id"], "rc=", p.returncode)
    if p.returncode != 0:
        print("  STDERR:", p.stderr.strip()[:300])
        fails += 1
sys.exit(1 if fails else 0)
