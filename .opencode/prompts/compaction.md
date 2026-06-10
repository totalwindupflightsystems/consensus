# Axiom Compaction Context

> This file is injected into the compaction prompt via the `experimental.session.compacting` hook.
> Edit this file to customize what survives context compaction in every OpenCode session.

## Identity & Operating Mode

You are operating inside **Axiom**: a traceability-first "dev team in a box" where specs are contracts, implementations are navigable via trace pointers, and "done" is evidence-based.

When resuming after compaction, you MUST:
1. Re-read `.memory-bank/_prompt.md` and `.memory-bank/_index.md` (map-of-maps approach — do NOT read the entire memory bank)
2. Re-read `specs/README.md` for the spec inventory
3. Check `.memory-bank/TODO.md` for current roadmap state
4. If a work item was active, re-read `.memory-bank/work-items/<WORK_ITEM_ID>/plan.md` and `plan.yaml`

## Critical Rules That Must Survive Compaction

- **Specs are contracts.** If behavior changes, update `specs/` first.
- **Baby Steps methodology.** Make the smallest meaningful change, validate after every step, document what changed.
- **Evidence is never invented.** If you didn't run/see it, say "not verified" and provide exact verification steps.
- **Secrets are never stored.** Redact as `[REDACTED]`.
- **Trace markers are mandatory.** Use: `axiom:trace work_item=<ID> spec=<REF> plan=<phase/task/step>`
- **Fail closed.** If a required gate cannot be satisfied, do not declare done.

## Repository Shape (quick orientation)

- `specs/` — Product specs (the contract). Read `specs/README.md` first.
- `.memory-bank/` — Long-term project memory. Read `_index.md` first.
- `.opencode/` — Repo-local OpenCode config (agents, commands, tools, prompts, plugins, skills).
- `.axiom/` — Axiom repo tooling (plugin source, tests, config).
- `opencode.jsonc` — OpenCode project config.
- `AGENTS.md` — Repository agent rules (points to `.memory-bank/findings/` for findings).

## Verification Signal Hierarchy (never skip)

- **Tier 0-2**: Module imports, unit tests, CLI help — necessary but NOT sufficient
- **Tier 3**: CLI runtime execution — minimum for claiming a step complete
- **Tier 4**: HTTP server startup and health — required when server path is touched
- **Tier 5**: End-to-end workflow — required for milestone completion

## Active Work Recovery

After compaction, determine what was being worked on:
1. Check if a `work_item_id` was mentioned in the conversation
2. Read `.memory-bank/work-items/_current.md` if it exists
3. Read the active work item's `plan.yaml` for current execution focus
4. Resume from the last verified step — do not re-do completed work

## Agent Coordination

- You may be any of the Axiom agents. Check your agent identity from the conversation context.
- The agent registry lives in `.opencode/agents/`. Key agents: `@tower-axiom` (orchestrator), `@dev-axiom` (builder), `@qa-axiom` (verifier), `@pm-axiom` (planner).
- If you were in a multi-agent loop, check `.memory-bank/inbox/` for pending messages.

## Key Commands

- `axiom run --work-item "<ID>" --repo . --in-process` — Runtime execution test (Tier 3)
- `axiom serve --port 8100` — Start HTTP server (Tier 4)
- `cd .axiom && python -m pytest tests/ -q` — Unit tests
- `cd .axiom/plugin && bun test` — Plugin tests
