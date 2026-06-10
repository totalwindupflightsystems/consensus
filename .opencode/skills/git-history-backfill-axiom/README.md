# git-history-backfill-axiom

Backfill memory bank context from git commit history for repos onboarding into Axiom. Reads commit log oldest-to-newest, extracts project evolution, decisions, milestones, and contributor patterns, then generates .memory-bank/ files so agents have historical context even though the repo didn't start with Axiom. Companion to /axiom-spec-extract (which backfills specs from code) — this skill backfills memory from git.

## When to Load

- Repos with **50+ commits** (default threshold)
- Any run with `batch_mode=true` argument
- Incremental catchup runs (DEX-301)
- When the agent context window is limited

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
