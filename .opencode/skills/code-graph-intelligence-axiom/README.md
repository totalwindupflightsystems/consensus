# code-graph-intelligence-axiom

Multi-language call graph engine for structural code intelligence. Covers graph building, callers/callees lookup, blast-radius analysis, cross-language edge detection, change-impact queries, symbol search, package clustering, and the adapter-test CLI. Load this skill when any agent needs to reason about code structure, call relationships, or cross-language dependencies in any repo managed by Axiom.

## When to Load

Load when an agent needs to:
- Find all callers or callees of a function/method
- Assess the blast radius of a proposed change
- Detect cross-language dependencies (Python calling shell scripts, Terraform provisioners, etc.)
- Identify which symbols are affected by a set of changed files

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
