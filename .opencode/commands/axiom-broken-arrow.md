---
description: "Launch an emergency concurrent agent swarm to diagnose a problem. Time-boxed, inbox-driven, multi-agent diagnosis."
agent: sitrep-axiom
---

Launch a Broken Arrow emergency swarm to diagnose the problem described in the arguments.

Inputs:
- Problem statement: $PROBLEM (required; description of what is broken)
- Max time: $MAX_TIME (optional; seconds, default: 300)
- Poll interval: $POLL_INTERVAL (optional; seconds, default: 15)
- Agents: $AGENTS (optional; "auto", "all", or comma-separated list, default: auto)
- Max agents: $MAX_AGENTS (optional; integer, default: 8)
- Repo: $REPO (optional; path to repository, default: current directory)
- Output format: $OUTPUT_FORMAT (optional; "text" or "json", default: text)
- Dry run: $DRY_RUN (optional; "true" to skip session creation, default: false)

Spec contract: `specs/46-Broken-Arrow-Emergency-Swarm.md`

Skills (load on demand):
- `axiom-broken-arrow-emergency-swarm` — Full Broken Arrow protocol, agent roster, polling loop, communication protocol, and report format.

Do:

1) **Parse arguments** from the user's message:
   - Extract the problem statement (everything after `/broken-arrow` or the `--problem` flag)
   - Extract optional flags: `--max-time`, `--poll-interval`, `--agents`, `--max-agents`, `--repo`, `--output-format`, `--dry-run`
   - If no problem statement is provided, ask the user: "What is broken? Please describe the problem."

2) **Build the command** to invoke the orchestrator script:
   ```bash
   python3 scripts/broken_arrow.py \
     --problem "<problem statement>" \
     --repo <repo_path> \
     --max-time <max_time> \
     --poll-interval <poll_interval> \
     --agents <agents> \
     --max-agents <max_agents> \
     --output-format <output_format> \
     [--dry-run]
   ```

3) **Execute the command** using the bash tool:
   - Run the orchestrator script with the constructed arguments
   - Stream output to the user as it appears
   - Capture the final report text

4) **Return the result** in the required XML format:
   - If the script exits 0: status = "ok"
   - If the script exits 1: status = "fail"
   - Include the full Broken Arrow Report in `outputs.broken_arrow_report`

Example invocations:
- `/axiom-broken-arrow axiom run crashes on startup with AttributeError`
- `/axiom-broken-arrow DB migration failed after deploy --agents dev-axiom,db-architect-axiom`
- `/axiom-broken-arrow test --dry-run --max-time 30`

axiom:trace work_item=broken-arrow-01 spec=specs/46-Broken-Arrow-Emergency-Swarm.md plan=P-27/phase-27.3/task-27.3.1 impl=.opencode/commands/broken-arrow.md
