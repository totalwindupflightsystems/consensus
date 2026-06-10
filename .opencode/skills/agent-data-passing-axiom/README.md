# agent-data-passing-axiom

How to consume artifact paths from upstream command responses and pass detailed references between agents.

## When to Load

Load this skill when ANY of these conditions are true:
1. **You receive a `<delegate>` tag** in an upstream command response — the delegate context contains artifact paths you need to pass to the next agent.
2. **You are orchestrating a multi-step workflow** where Agent A produces files and Agent B needs to read them — load before dispatching Agent B so you know what paths to include in the prompt.
3. **You are about to call a second command** and the first command's response contained `evidence.files_changed` or `evidence.*_path` fields — load to understand how to extract and forward those paths.
4. **You receive an XML response with evidence fields** and you are unsure which files to read for detailed context — load to understand the `evidence.files_changed` vs `evidence.findings_paths` distinction.

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
