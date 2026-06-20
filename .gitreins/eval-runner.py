#!/usr/bin/env python3
"""GitReins Evaluator Runner — runs Tier 2 evaluation against Consensus tasks.

Usage:
  cd ~/gitreins-poc && PYTHONPATH=. python3 ~/consensus/.gitreins/eval-runner.py build-gate
  cd ~/gitreins-poc && PYTHONPATH=. python3 ~/consensus/.gitreins/eval-runner.py --all

Requires: GITREINS_LLM_API_KEY, GITREINS_LLM_BASE_URL, GITREINS_LLM_MODEL in env
"""

import json
import os
import sys
import yaml

# Add gitreins-poc to path (caller should be in that directory)
sys.path.insert(0, os.getcwd())

try:
    from engine.llm import LLMClient
    from engine.evaluator import AgenticEvaluator
    from engine.eval_cap import parse_eval_cap
except ImportError:
    print("GitReins engine not found. Install: pip install gitreins==0.3.0")
    sys.exit(1)

TASKS_FILE = os.path.expanduser("~/consensus/.gitreins/tasks.yaml")
WORKDIR = os.path.expanduser("~/consensus")


def load_tasks():
    with open(TASKS_FILE) as f:
        data = yaml.safe_load(f)
    return data.get("tasks", {})


def run_evaluation(task_id, task_def, budget_override=None):
    llm = LLMClient()
    # Caps are loaded from .gitreins/config.yaml (v0.3.0+):
    #   evaluator.max_iterations, max_time, max_input_tokens, max_output_tokens
    # Budget override: pass eval_cap='200k' to cap output tokens, '100k/50k' for in/out
    kwargs = {"llm": llm, "workdir": WORKDIR}
    if budget_override:
        kwargs["eval_cap"] = parse_eval_cap(budget_override)
        print(f"Budget override: {budget_override}")
    evaluator = AgenticEvaluator(**kwargs)

    task = {
        "id": task_def["id"],
        "title": task_def["title"],
        "criteria": task_def["criteria"],
    }

    print(f"\n{'='*60}")
    print(f"GitReins Tier 2 — Evaluating: {task['title']}")
    print(f"Criteria: {len(task['criteria'])}")
    print(f"{'='*60}\n")

    verdict = evaluator.evaluate(task)

    passed = sum(1 for i in verdict.items if i.status == "PASS")
    failed = sum(1 for i in verdict.items if i.status == "FAIL")
    total = len(verdict.items)

    print(f"\n{'─'*60}")
    print(f"VERDICT: {verdict.verdict}  ({passed}/{total} PASS)")
    print(f"{'─'*60}")
    for item in verdict.items:
        icon = "✓" if item.status == "PASS" else "✗"
        print(f"\n{icon} {item.criterion}")
        print(f"   {item.detail}")

    if failed > 0:
        print(f"\n❌ {failed} criteria FAILED")
        sys.exit(1)
    else:
        print(f"\n✅ All {total} criteria PASSED")
        sys.exit(0)


def main():
    tasks = load_tasks()

    # Parse budget override from argv (second argument, optional)
    budget = None
    args = [a for a in sys.argv[1:] if a and not a.startswith('-')]
    if len(args) >= 2:
        budget = args[1]
    task_arg = args[0] if args else None

    if len(sys.argv) < 2 or sys.argv[1] == "--all":
        # Run all tasks
        exit_code = 0
        for task_id, task_def in tasks.items():
            try:
                run_evaluation(task_id, task_def, budget_override=budget)
            except SystemExit as e:
                if e.code != 0:
                    exit_code = e.code
        sys.exit(exit_code)

    task_id = task_arg
    if task_id not in tasks:
        print(f"Unknown task: {task_id}")
        print(f"Available: {', '.join(tasks.keys())}")
        sys.exit(2)

    run_evaluation(task_id, tasks[task_id], budget_override=budget)


if __name__ == "__main__":
    main()
