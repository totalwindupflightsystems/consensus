---
name: git-hooks-builder
description: Scans a Git repository to recommend and implement useful Git hooks (e.g., pre-commit, commit-msg, pre-push), choosing an appropriate hooks mechanism (existing manager or core.hooksPath) and honoring user constraints like speed, strictness, and tooling preferences.
tags:
  vertical: [coding]
  category: development
  core: false
---

Purpose
This Skill helps install or improve Git hooks in a way that matches the repository’s existing conventions and developer workflow. It can either:
- scan the repo and propose a hook plan that adds value with minimal friction, or
- implement a user-requested hook and fit it into the repo cleanly.

Core behaviors (defaults, not rigid rules)
- Prefer extending what the repo already uses (existing hook manager, scripts, lint/test commands) over introducing new tooling.
- Prefer “warn-first” when uncertainty is high; allow the user to opt into “block” behavior.
- Prefer fast checks in pre-commit; prefer heavier checks in pre-push or CI.
- Prefer solutions that are shareable across the team (tracked hooks folder + setup instructions or an existing hook manager).
- Prefer deterministic hooks (no network calls, no flakey environment dependencies) unless the user explicitly wants otherwise.

Activation cues
Use this Skill when the user asks to:
- add, update, or standardize git hooks
- enforce commit message policy
- run lint/format/tests automatically on commit or push
- integrate Husky / Lefthook / pre-commit / core.hooksPath style hooks
- “scan this repo and recommend hooks”

Workflow
1) Reconnaissance
   - Detect repo languages, build systems, test runners, and existing commands (package scripts, Makefile, CI configs).
   - Detect existing hooks mechanisms:
     - existing hooks manager configs/directories
     - presence of tracked hooks directory
     - existing hook files and behavior
   - Identify constraints:
     - user preference (warn vs block)
     - speed budget for pre-commit
     - OS/cross-platform expectations
     - whether changes should be minimal vs “standardize across team”

2) Propose a Hook Plan (before writing)
   - Produce a structured plan describing:
     - hook events to implement
     - what each hook runs, scoped to staged files when possible
     - whether each hook is warn-only or blocking
     - install approach (existing manager vs core.hooksPath tracked hooks)
     - verification steps
   - Include tradeoffs and alternatives for any risky/slow choices.

3) Implement
   - Write hook scripts and any helper scripts/config.
   - Ensure hooks are executable and placed where Git will actually run them.
   - Add/update lightweight docs: how to install, how to bypass when appropriate, how to troubleshoot.

4) Verify
   - Run a small verification routine:
     - simulate each hook (or run it directly) and confirm exit codes/messages
     - run the repo’s relevant lint/test commands for the touched areas

Output format (use XML tags)
Wrap outputs like this:
<hook_plan>…</hook_plan>
<changes_made>…</changes_made>
<install_instructions>…</install_instructions>
<verification>…</verification>
<troubleshooting>…</troubleshooting>

User overrides
When the user requests a non-default approach:
- Aim to implement it as asked.
- Clearly state expected tradeoffs (speed, flakiness, platform support).
- Offer a “safer variant” (timeouts, warn-only, move heavy checks to pre-push) without blocking the user.

Guardrails (soft, but important)
- Avoid surprising global changes (e.g., don’t change the user’s global git config).
- Avoid making commits fail for long-running checks unless the user wants strict enforcement.
- Avoid duplicating what CI already enforces unless it saves meaningful time locally.
- Prefer clear, actionable error messages and minimal noise on success.

References
See reference/hooks-reference.md, reference/decision-guide.md, and reference/pitfalls.md as needed.

axiom:trace work_item=git-hooks-builder-axiom spec=specs/00-PRD.md plan= prompt=.opencode/skills/git-hooks-builder-axiom/SKILL.md evidence= doc= test= commit=
