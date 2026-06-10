---
name: prototype-management-axiom
description: >
  Manage prototype explorations in Axiom: create, track, evaluate, and promote or archive prototypes using git worktrees and memory-bank exploration briefs.
version: "1.0"
tags:
  vertical: ['planning', 'coding']
  category: planning
  core: false
---
# prototype-management-axiom — Prototype Exploration Lifecycle (Create, Track, Promote, Archive)

Manage prototype explorations in Axiom: create, track, evaluate, and promote or archive prototypes using git worktrees and memory-bank exploration briefs.

## When to Load

- User wants to prototype a concept before committing to a spec
- User wants to set up a parallel exploration branch for another agent
- User wants to evaluate a third-party tool/library against Axiom
- User wants to check status of existing prototypes

## Prototype Lifecycle

```
Idea → Exploration Brief → Git Worktree → Prototype Work → Findings → Decision
                                                                        ↓
                                                              Promote to Spec  OR  Archive
```

## Creating a Prototype

### 1. Create the Exploration Brief

Write to `.memory-bank/explorations/<name>.md` with YAML frontmatter:

```yaml
---
mb: true
type: exploration
title: "<Descriptive title>"
created: <date>
updated: <date>
status: Active Prototype
tags: [prototype, <domain-tags>]
links:
  - <relevant-specs>
source: <origin of the idea>
git:
  branch: prototype/<short-name>
  worktree: _tmp/<short-name>/
---
```

Required sections:
- **Executive Summary** — one paragraph, what and why
- **Problem Statement** — what gap this fills
- **What This Prototype Should Prove** — numbered success criteria
- **Architecture** — diagram if applicable
- **Prototype Scope** — in-scope / out-of-scope
- **Key Files** — table of important paths
- **Success Criteria** — testable outcomes
- **Risk Assessment** — table of risks
- **Verdict** — proceed / defer / reject

Update `.memory-bank/explorations/_index.md` with the new entry.

### 2. Create the Git Worktree

```bash
# Create branch from main
git checkout -b prototype/<short-name>
git checkout main

# Create worktree in _tmp/ (gitignored, inside repo boundary)
git worktree add _tmp/<short-name> prototype/<short-name>
```

### 3. Create the Prototype README

Write `_tmp/<short-name>/PROTOTYPE.md` with:
- What this is
- Prerequisites
- Quick start instructions
- Prototype tasks (numbered)
- Key context the agent needs
- Deliverables list

### 4. Hand Off to Another Agent

Provide the user with a prompt they can paste into a new agent session. The prompt should include:
- Path to the worktree
- Path to the PROTOTYPE.md
- Path to the exploration brief
- What the agent should do (numbered steps)
- What the agent should NOT do
- How to verify success

## Evaluating a Prototype

When a prototype is complete, update the exploration brief with:
- **Findings** section — what worked, what didn't
- **Comparison** — vs existing approach (if applicable)
- **Recommendation** — promote to spec, iterate, or archive
- **Status** — change from "Active Prototype" to "Completed" or "Archived"

## Promoting a Prototype

If the prototype proves the concept:
1. Create a spec in `specs/` based on the findings
2. Create a work item and implementation plan
3. Update the exploration brief status to "Promoted to Spec"
4. Link the exploration to the new spec
5. Clean up the worktree: `git worktree remove _tmp/<short-name>`

## Archiving a Prototype

If the prototype is rejected or deferred:
1. Update the exploration brief status to "Archived" or "Rejected"
2. Document the reason in the brief
3. Clean up the worktree: `git worktree remove _tmp/<short-name>`
4. Optionally delete the branch: `git branch -d prototype/<short-name>`

## Conventions

- Prototype branches: `prototype/<short-name>`
- Prototype worktrees: `_tmp/<short-name>/` (inside repo, gitignored)
- Exploration briefs: `.memory-bank/explorations/<name>.md`
- Prototype README: `_tmp/<short-name>/PROTOTYPE.md`
- All prototype work stays on the prototype branch — never merge to main without promotion decision
- Prototypes are time-boxed: if no findings after 2 sessions, force a verdict

## Active Prototypes

Check `.memory-bank/explorations/_index.md` for entries with status "Active Prototype".

List worktrees: `git worktree list`
