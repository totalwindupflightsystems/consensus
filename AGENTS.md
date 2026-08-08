# Agent Rules

These rules are given to agentic coding agents operating in this repo.

---

## GitReins Quality Harness (MANDATORY)

This repo uses GitReins as its quality gate. Every commit runs static guards.
If guards fail, the commit is BLOCKED. You cannot skip this.

### Quick check before committing:
```bash
gitreins guard
```

### What's checked:
- **secrets** — API keys, tokens, passwords (BLOCKS on fail — no exceptions)
- **build** — compiles the project (BLOCKS on fail)
- **lint** — go vet (WARNS on fail)
- **tests** — runs tests for changed packages only (BLOCKS on fail)

### Test mode: diff
Only packages with staged changes are tested. Pre-existing failures in
untouched code will NOT block your commit. If you change pyproject.toml,
Makefile, .gitreins/config.yaml, or a config file, the full suite runs
as a safety net.

### Tasks and evaluation:
```bash
# Create a task with criteria
gitreins task create fix-auth "Fix authentication" \
  "Login accepts email+password and returns JWT" \
  "Invalid credentials return 401" \
  "Rate limiting works after 5 failed attempts"

# Do the work, then evaluate:
gitreins task start fix-auth
# ... implement ...
gitreins task complete fix-auth    # triggers LLM evaluation

# Or evaluate standalone:
gitreins judge fix-auth
```

### If guards fail:
1. READ the output — the guard tells you exactly what failed and where
2. Fix the issues. Do NOT commit with --no-verify unless it's a docs-only
   change or a GitReins self-upgrade.
3. Re-run `gitreins guard` until it passes
4. Then commit

### Never:
- Commit API keys or tokens — secrets guard catches these, and it's correct
- Skip guards with --no-verify for code changes
- Push if guards failed (let CI catch it if you must, but fix locally)
- Commit `.gitreins/tasks.yaml` — it's local task state

---

## Findings & Self-Improvement

Agents accumulate findings, patterns, and self-improvement notes in the memory bank — NOT in this file.

**Do not flood this file with findings.** Instead:

- Write findings to `.memory-bank/findings/` (see `_index.md` there for navigation).
- Each finding type has its own subfolder with `_index.md` and `_prompt.md`.
- This file only points here; agents open the findings index when they need it.

**When to write a finding:**
- You discover a recurring mistake, friction point, or anti-pattern.
- An adversarial agent (see below) surfaces a gap, risk, or assumption failure.
- A self-improvement loop produces a rule change or checklist update.

**Finding index:** `.memory-bank/findings/_index.md`

---

## Adversarial Quality Agents

Axiom ships a team of adversarial agents whose job is to challenge, falsify, and stress-test outputs. Running them as subagents is **critical** for maintaining high output quality across all parts of the system.

**Always consider invoking these agents before claiming work is done:**

| Agent | Role | When to invoke |
|---|---|---|
| `@assumption-buster-axiom` | Surfaces undocumented prerequisites, ambiguous specs, non-verifiable work | Before finalizing specs or plans; after any "obvious" assumption is made |
| `@devils-advocate-axiom` | Challenges specs/plans/designs; reduces risk and complexity; forces explicit tradeoffs | When a plan feels settled; before committing to an approach |
| `@redteam-axiom` | Adversarial falsification of claims; attack matrix; exploitable paths | Before claiming PASS; when security/ops/safety claims are made |
| `@whitehat-axiom` | Authorized penetration validation; exploitability checks; retest after fixes | After security fixes; when a finding needs practical validation |

**This list can expand.** Other adversarial/risk agents include:
- `@security-review-axiom` — threat model, secrets hygiene, risk gates
- `@chaos-engineer-axiom` — resilience testing, fault injection, runbook validation
- `@privacy-compliance-axiom` — PII/retention/consent engineering controls
- `@finops-cost-axiom` — cost-risk detection, cardinality guardrails

**Rule:** If you are about to declare a work item complete and you have NOT run at least one adversarial agent, you should either run one or explicitly document why it was skipped (with risk acceptance).

---

## Repository Shape
- Product specs live in `specs/` (this is the contract).
- Long-term project memory lives in `.memory-bank/`.
- Axiom config lives in `.axiom/`.
- Agent config lives in `.opencode/`.

## Required Reading (start of every task)
1. `specs/README.md` (spec inventory, if it exists)
2. `.memory-bank/_index.md` (memory inventory)
3. Relevant `.memory-bank/` files for the work item

## Process
- Make the smallest meaningful change.
- Validate after every step.
- Document what changed and why.

## Specs Are Contracts
- If behavior changes, update `specs/` first, then implement.
- If only internal implementation changes, confirm no spec update is needed.

## Secrets and Privacy
- Never write secrets to `specs/`, `.memory-bank/`, git history, or logs.
- Redact sensitive values as `[REDACTED]`.

## Memory Bank
- Follow `.memory-bank/_prompt.md` for rules.
- Use `.memory-bank/_index.md` to navigate.
- For work items, use `.memory-bank/work-items/<WORK_ITEM_ID>/`.

## Git and Workspace Hygiene
- All commits made by or with an AI agent MUST include a `Co-authored-by` trailer. Read the identity from `.axiom/axiom.config.yaml` under `git.co_author`.
- Always `git pull --rebase` before committing. Stash first if the worktree is dirty.
- Use `git mv` for all file and folder moves — never filesystem `mv` or `cp` + `rm`.
- NEVER revert unrelated changes you did not make unless explicitly requested.
- NEVER run destructive git commands (`reset --hard`, `push --force`) unless explicitly requested.
- Do not amend commits unless explicitly requested.
- Do not push to remote unless the user explicitly asks.
- See `.opencode/prompts/git-rules.md` for the full git rules.
