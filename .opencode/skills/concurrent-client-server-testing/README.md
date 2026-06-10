# concurrent-client-server-testing

Multi-agent concurrent client/server API testing with a fixed runtime window. One agent owns server uptime (offset start + bounded duration), while worker agents synchronize to shared epoch timestamps, run protocol tests in parallel, and produce per-view evidence that can be merged into a single report.

## When to Load

Load this skill when you need multi-agent concurrent client/server api testing with a fixed runtime window.

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
