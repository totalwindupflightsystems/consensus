---
name: writing-style-system-axiom
description: Parent routing skill for Axiom writing surfaces. Selects the right child writing skill and applies user-facing response balancing rules when humans are the audience.
version: "1.0"
tags:
  vertical: [coding, ops, sre, writing, security, planning, onboarding, benchmarking, personal-context]
  category: writing
  core: true
---

# Writing Style System

Use this as the parent skill for artifact-specific writing. It routes to the correct child skill instead of forcing one generic writing style onto every surface.

axiom:trace work_item=DEX-73 spec= plan= test= doc=.memory-bank/explorations/writing-style-skill-collection.md prompt=.opencode/skills/writing-style-system-axiom/SKILL.md evidence= commit= jira_ref=DEX-73

## Routing Rule

Pick the primary surface first, then optionally layer `user-response-writing-axiom` if a human will read the final answer directly.

| Surface | Load this child skill |
|---|---|
| Pull request title/body | `pull-request-writing-axiom` |
| Git commit message | `git-commit-writing-axiom` |
| Jira issue description | `jira-ticket-writing-axiom` |
| Jira progress/update comment | `jira-comment-writing-axiom` |
| Product or technical documentation | `documentation-writing-axiom` |
| Technical spec / contract | `spec-writing-axiom` |
| Architecture decision record | `adr-writing-axiom` |
| Operational runbook | `runbook-writing-axiom` |
| Changelog or release notes | `changelog-release-notes-writing-axiom` |
| RFC / design proposal | `rfc-writing-axiom` |
| Assistant response to user | `user-response-writing-axiom` |
| Assistant response is becoming list-heavy or robotic | `gpt-paragraph-first-writing-axiom` |

## Composition Rule

Do not default to one presentation form.

- Use prose for framing, tradeoffs, and nuance.
- Use bullets for actions, outcomes, and checklists.
- Use tables for comparisons, mappings, and dense reference data.
- Prefer one short prose block plus one short list before adding a table.
- If a table has only one meaningful column, rewrite it as bullets.
- If the content is sequential, use numbered steps instead of a table.

## Selection Heuristics

1. If the output changes workflow state, prefer the workflow-native skill (`jira-*`, `pull-request-*`, `git-commit-*`).
2. If the output defines system truth, prefer `spec-writing-axiom`, `adr-writing-axiom`, or `rfc-writing-axiom`.
3. If the output teaches or guides execution, prefer `documentation-writing-axiom` or `runbook-writing-axiom`.
4. If the output is shown directly to the user, also apply `user-response-writing-axiom`.
5. If the user complains about list-heavy or robotic style, also apply `gpt-paragraph-first-writing-axiom` and prefer prose over bullets unless structure is explicitly requested.

## Output Contract

Each child skill should answer four questions:

- Who is the audience?
- What structure is required?
- What tone improves comprehension?
- What anti-patterns must be avoided?

## Child Skills

- `pull-request-writing-axiom`
- `git-commit-writing-axiom`
- `jira-ticket-writing-axiom`
- `jira-comment-writing-axiom`
- `documentation-writing-axiom`
- `spec-writing-axiom`
- `adr-writing-axiom`
- `runbook-writing-axiom`
- `changelog-release-notes-writing-axiom`
- `rfc-writing-axiom`
- `user-response-writing-axiom`
- `gpt-paragraph-first-writing-axiom`
