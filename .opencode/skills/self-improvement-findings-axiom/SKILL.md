---
tags:
  vertical: [coding, planning]
  category: methodology
  core: false
---

# self-improvement-findings-axiom

Portable rules for the `findings/` self-improvement directory in `.memory-bank/`.

## Purpose

This skill governs how agents accumulate findings, anti-patterns, adversarial results, and self-improvement notes in `.memory-bank/findings/` — without flooding `AGENTS.md` with details.

The key principle: **`AGENTS.md` is a pointer, not a dump.** It tells agents where to look. Agents open `.memory-bank/findings/_index.md` on demand when they need the details.

---

## Directory Layout

```
.memory-bank/findings/
  _index.md          ← curated map (read first)
  _prompt.md         ← local rules and templates
  adversarial/       ← findings from redteam/assumption-buster/devils-advocate/whitehat
    _index.md
    _prompt.md
  anti-patterns/     ← recurring mistakes and how to avoid them
    _index.md
    _prompt.md
  agent-reflections/ ← cross-agent patterns (agent-specific reflections stay in agents/<agent>/)
    _index.md
    _prompt.md
  process/           ← process friction, workflow improvements
    _index.md
    _prompt.md
```

**Subfolders are created on demand** — only when 3+ findings of the same type exist and retrieval becomes hard.

---

## When to Write a Finding

Write a finding when ANY of these are true:

1. **Adversarial agent surfaces a gap**: `@redteam-axiom`, `@assumption-buster-axiom`, `@devils-advocate-axiom`, or `@whitehat-axiom` finds a risk, assumption failure, or exploitable path.
2. **Repeated mistake**: You make the same mistake more than once.
3. **Self-improvement loop**: A rule change or checklist update is produced.
4. **Recurring friction**: A pattern slows down work repeatedly.

**Do NOT write** for one-off issues already fixed and unlikely to recur.

---

## Where to Write

| Finding type | Location |
|---|---|
| Adversarial agent finding | `.memory-bank/findings/adversarial/` (create subfolder when 3+ exist) |
| Recurring mistake / anti-pattern | `.memory-bank/findings/anti-patterns/` |
| Cross-agent pattern | `.memory-bank/findings/agent-reflections/` |
| Process friction / workflow improvement | `.memory-bank/findings/process/` |
| Agent-specific reflection | `.memory-bank/agents/<agent>/reflection.md` (NOT findings/) |

---

## AGENTS.md Contract

`AGENTS.md` (and the scaffold template) MUST contain a "Findings & Self-Improvement" section at the top that:
1. Tells agents NOT to flood `AGENTS.md` with findings.
2. Points to `.memory-bank/findings/_index.md`.
3. Lists when to write a finding.

**Do not add finding details to `AGENTS.md`.** Only the pointer block belongs there.

---

## CLAUDE.md Symlink Pattern

Some teams use `CLAUDE.md` instead of `AGENTS.md` (e.g., Anthropic Claude projects). When installing Axiom into such a repo:

1. Check if `CLAUDE.md` exists at the repo root.
2. If it does and `AGENTS.md` does not exist: create `AGENTS.md` with the full content, then create `CLAUDE.md` as a symlink to `AGENTS.md`:
   ```bash
   ln -sf AGENTS.md CLAUDE.md
   ```
3. If both exist: merge the content into `AGENTS.md`, then symlink `CLAUDE.md → AGENTS.md`.
4. Document the symlink in `.axiom/axiom.config.yaml` or a note in `.memory-bank/projects/<project>/overview.md`.

This ensures both tools see the same rules without duplication.

---

## Bootstrap Checklist (for new installs)

When installing Axiom into a new repo, ensure:

- [ ] `AGENTS.md` exists with the "Findings & Self-Improvement" section at the top
- [ ] `AGENTS.md` has the "Adversarial Quality Agents" section
- [ ] `.memory-bank/findings/_index.md` exists
- [ ] `.memory-bank/findings/_prompt.md` exists
- [ ] `.memory-bank/_index.md` has a `findings/` navigation entry
- [ ] `.memory-bank/_prompt.md` references `findings/` in its folder structure
- [ ] If `CLAUDE.md` is the convention: symlink `CLAUDE.md → AGENTS.md`

---

## Note Template

```markdown
---
mb:
  type: finding
  title: "Short descriptive title"
  created: YYYY-MM-DD
  updated: YYYY-MM-DD
  tags: [finding, adversarial|anti-pattern|process|reflection]
  severity: low|medium|high|critical
  status: open|addressed|wont-fix
  links:
    up: "../_index.md"
    related: []
  source:
    type: adversarial-agent|self-discovery|qa-sweep|user-report
    ref: "work_item=X or agent=Y or date=YYYY-MM-DD"
  git:
    commit: ""
    paths: []
    blame: ""
---

# Finding: [Title]

## Summary
What was found and why it matters (2-5 sentences).

## Details
- **Trigger**: What caused this finding to surface.
- **Impact**: What breaks or degrades if not addressed.
- **Root cause**: Why this happens.

## Prevention / Fix
- What rule, checklist, or process change prevents recurrence.
- Link to the updated `_prompt.md` or spec if a rule was changed.

## Links
- [Up: Findings Index](../_index.md)
- [Related: ...]

## Traceability
- **Source**: adversarial agent / work item / QA sweep
- **Git**: commit / paths (leave blank if unavailable)
```

---

## Context Budget

This skill is intentionally small. Load it when:
- Setting up a new repo (install/onboarding)
- Writing a finding after an adversarial agent run
- Reviewing whether `AGENTS.md` is correctly structured

Do NOT load this skill for routine work-item execution — it is not needed then.
