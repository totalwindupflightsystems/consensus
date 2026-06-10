# opencode-programming-axiom

Reusable integration playbook for building against the OpenCode HTTP server API: long-running /message semantics, /event monitoring, liveness-based completion, timeout layering, request-shape verification via /doc, SDK caution, and fail-closed session handling.

## When to Load

Load it when you are:
- Implementing a new OpenCode client or adapter (any language).
- Debugging `/command`, `/message`, or `/prompt_async` behavior.
- Deciding whether to use `/event`, polling, or both.
- Tuning timeouts or fallback behavior.

## Files

| File | Purpose |
|------|---------|
| `SKILL.md` | Full skill instructions |

## Related

See [`docs/skills.md`](/docs/skills.md) for the full skill inventory.
