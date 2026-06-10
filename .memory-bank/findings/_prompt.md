# Findings & Self-Improvement Prompt

This folder stores agent-discovered findings, recurring patterns, anti-patterns, and self-improvement notes.

**Scope**: What belongs here vs. elsewhere:
- **Belongs here**: Recurring mistakes, friction points, anti-patterns, adversarial agent findings, self-improvement rules, checklist updates, gap discoveries.
- **Does NOT belong here**: Work-item execution state (→ `work-items/`), project decisions (→ `decisionLog.md`), best-practice playbooks (→ `best-practices/`), known repo gaps (→ `known-gaps/`).

---

## Subfolder Structure

Each finding type gets its own subfolder with `_index.md` and `_prompt.md`:

```
findings/
  _index.md          ← this folder's map (read first)
  _prompt.md         ← this file
  adversarial/       ← findings from redteam/assumption-buster/devils-advocate/whitehat
  anti-patterns/     ← recurring mistakes and how to avoid them
  agent-reflections/ ← cross-agent patterns (agent-specific reflections stay in agents/<agent>/)
  process/           ← process friction, workflow improvements
```

Create a new subfolder when 3+ findings of the same type accumulate and retrieval becomes hard.

---

## When to Write a Finding

Write a finding when:
1. An adversarial agent (`@redteam-axiom`, `@assumption-buster-axiom`, `@devils-advocate-axiom`, `@whitehat-axiom`) surfaces a gap, risk, or assumption failure.
2. You repeat a mistake more than once.
3. A self-improvement loop produces a rule change or checklist update.
4. You discover a recurring friction point that slows down work.

**Do NOT** write a finding for one-off issues that are already fixed and unlikely to recur.

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

## Traceability
- **Source**: adversarial agent / work item / QA sweep
- **Git**: commit / paths (leave blank if unavailable)
```

---

## Naming Conventions

- Files: `YYYY-MM-DD-short-slug.md`
- Subfolders: `kebab-case/`

---

## Prompt Changelog

| Date | Change | Reason |
|------|--------|--------|
| YYYY-MM-DD | Created | Bootstrap findings/self-improvement directory |
