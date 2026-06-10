---
description: Navigate the skill map decision tree to find the right skills for any problem.
agent: dispatch-axiom
---

Navigate the skill map decision tree to find the right skills for a problem.

axiom:trace work_item=skill-map-01 spec=specs/85-Skill-Map-Decision-Tree.md

## Inputs

- `$ARGUMENTS` — subcommand and optional arguments:
  - *(no args)* — display the current decision tree as a summary (domain list with skill counts)
  - `query <problem>` — traverse the tree and recommend skills for the stated problem
  - `update` — process accumulated feedback messages and propose tree updates
  - `validate` — verify tree.yaml is well-formed; report stale references, uncovered skills, malformed YAML

## Skills (load always)

- `axiom-skill-map` — the decision tree and traversal rules

## Subcommand: display (default, no args)

1. Load `axiom-skill-map` skill.
2. Read `tree.yaml` from the skill directory.
3. For each domain, count the total unique skills across all problem types.
4. Output a summary table:

| Domain | Problem Types | Skills | Top Signals |
|--------|--------------|--------|-------------|
| security | 4 | 3 | threat model, secrets, vulnerability |
| ... | ... | ... | ... |

5. Include the global skills list.
6. Report tree version and last-updated date.

## Subcommand: query `<problem>`

1. Load `axiom-skill-map` skill.
2. Read `tree.yaml`.
3. Match the problem description against domain `signals` using case-insensitive substring matching.
4. For each matched domain, identify the most relevant problem type(s).
5. Collect the union of recommended skills from all matched problem types, deduplicated.
6. Always include global skills.
7. Output:

```
## Skill Map Recommendation

**Problem:** <problem description>
**Matched domains:** <list>
**Matched problem types:** <list with brief descriptions>

### Recommended Skills (load these)
1. <skill-name> — <brief rationale>
2. ...

### Recommended Agents
- @agent-name — <when to invoke>

### Global Skills (always loaded)
- baby-steps-methodology, traceability-doctrine, evidence-bundle-schema, ...
```

8. If no domain matches:
   - Fall back to `axiom-capability-surface` skill for flat catalog discovery.
   - Write a feedback message to `.memory-bank/inbox/memory-bank-axiom/` with `matched_domain: null`.
   - Tell the user: "No domain matched. Falling back to flat catalog. A feedback message has been written to improve future routing."

## Subcommand: validate

1. Load `axiom-skill-map` skill.
2. Read `tree.yaml`.
3. List all skill directories in `.opencode/skills/`.
4. Check for:
   - **Stale references**: skills in tree.yaml that don't exist in `.opencode/skills/`
   - **Uncovered skills**: skills in `.opencode/skills/` that don't appear in any tree leaf (excluding `axiom-skill-map` itself)
   - **Malformed YAML**: syntax errors in tree.yaml
   - **Missing required fields**: domains without `signals`, problem types without `skills`
   - **Version/updated fields**: present and valid
5. Output a validation report:

```
## Skill Map Validation

**Tree version:** 1
**Updated:** 2026-04-12
**Domains:** 21
**Total problem types:** N
**Total skill references:** N

### Stale References (skills in tree but not installed)
- (none)

### Uncovered Skills (installed but not in tree)
- (none)

### Missing Required Fields
- (none)

### Result: PASS | FAIL
```

## Subcommand: update

1. Load `axiom-skill-map` skill.
2. Scan `.memory-bank/inbox/memory-bank-axiom/` for files matching `type: skill-map-feedback`.
3. Deduplicate: same problem-type + skill pair = one suggestion.
4. For each unique suggestion:
   - Propose adding the skill to the relevant problem type in tree.yaml.
   - If `matched_domain: null`, propose a new domain or signal addition.
5. Present proposed changes for human review.
6. Updates are **additive only** — skills are added but never removed without `--prune` flag.
7. If `--prune` is specified, also identify skills that appear in the tree but have zero feedback support and propose removal (requires human confirmation).

## Output (machine-consumable)

Emit a `<axiom>` XML envelope per `specs/04-XML-Protocol.md`:
- `<command>/axiom-skill-map</command>`
- `<status>ok|fail|blocked</status>`
- `<summary>` one sentence describing the result
- `<confidence>` 0-100

## Rules

- The tree is advisory. Never force-load skills the agent doesn't need.
- Signal matching is case-insensitive substring.
- Multi-domain matches return the union of skill sets, deduplicated.
- Global skills are always included in query results.
- Validate should catch drift early — run it as part of `/axiom-sync-all` when available.

See: `specs/85-Skill-Map-Decision-Tree.md`, `.opencode/skills/axiom-skill-map/SKILL.md`
