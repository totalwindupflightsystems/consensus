# sitrep-ascii-graphs

Portable ASCII progress graph renderer for Axiom SitRep reports.

## What This Is

This skill provides deterministic ASCII rendering for:
- **Work item progress bars** — horizontal bars showing done/total steps per work item
- **Phase breakdowns** — per-phase bars within a single work item
- **Spec coverage heatmaps** — CONFORMANT / PARTIAL / NOT SWEPT status per spec file
- **Velocity sparklines** — work items completed per week over the last N weeks

## How to Use

Load this skill when generating SitRep reports or any progress summary:

```
/skill sitrep-ascii-graphs
```

See [SKILL.md](./SKILL.md) for full documentation, rendering formulas, and step-by-step workflows.

## Quick Reference

| Graph Type | When to Use |
|------------|-------------|
| Work Item Progress Bar | Show completion % for each active work item |
| Phase Breakdown | Show per-phase progress within a single work item |
| Spec Coverage Heatmap | Show spec sweep status from conformance sweep runs |
| Velocity Sparkline | Show work items completed per week (trend) |

## See Also

- `sitrep-axiom` agent (`.opencode/agents/sitrep-axiom.md`) — primary consumer of this skill
- `axiom-todo` skill — TODO tracking and archive
- `evidence-bundle-schema` skill — verification evidence format
