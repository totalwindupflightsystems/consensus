# pattern-investigate-changes

Structured investigation protocol for analyzing recent code changes. Combines Code Intelligence, ShellOps, Graph Harness, Tree Memory, and Context Stash to assess what changed, what's affected, and what's risky.

## When to Load

Load when:
- You need to understand what happened in recent commits
- Someone asks "what changed?" or "is this risky?"
- A PR or deploy needs impact analysis
- You want to assess blast radius of recent work

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full pattern: prerequisites, tool chain, flow diagram, pseudocode, signals, adjustment protocol, execution trace |

## Quick Summary

```
Step 0: Check prerequisites (daemon up, code-intel ready, tree initialized)
Step 1: graph_create → structure the investigation as a DAG (optional)
Step 2: shellops.terminal_send → git diff --stat (get changed files)
Step 3: code-intel search → find affected symbols (optional)
Step 4: code-intel callers → assess blast radius (enriching)
Step 5: tree.commit → store findings permanently (required)
Step 6: stash.push → handoff context to other agents (optional)
```

## Key Gotcha

`stash.push` tags must be a **comma-separated string**, NOT an array:
- ✅ `tags: "investigation,low-risk"`
- ❌ `tags: ["investigation", "low-risk"]`

## Related

- **Spec**: `specs/121-Pattern-Generator.md`
- **Generator skill**: `.opencode/skills/axiom-pattern-generator/SKILL.md`
- **Tool reference**: `.opencode/skills/axiom-pattern-generator/REFERENCE.md`

<!-- axiom:trace work_item=pattern-generator-01 spec=specs/121-Pattern-Generator.md -->
