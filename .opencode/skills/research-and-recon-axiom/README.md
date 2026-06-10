# research-and-recon-axiom

Research and reconnaissance skill for Axiom agents. Teaches when and how to search the web, read documentation, and investigate problems encountered during planning, step execution, and verify cycles.

## When to use

Load this skill when:
- A step is blocked or failing with an unfamiliar error
- Planning a feature that uses an external library or API
- Stuck in a step-loop/verify cycle and internal knowledge isn't resolving it
- About to make an assumption about how a third-party system works
- Need current version, API shape, or best practice for something

## Key principle

**Always get the current date before searching.** Training data has a cutoff; the web does not. Add year anchors to queries to avoid stale results.

## Available tools

- `searxng_searxng_web_search` — meta-search engine (SearXNG MCP)
- `searxng_web_url_read` — fetch and read a specific URL
- `webfetch` — built-in URL fetcher

See `SKILL.md` for full guidance on query construction, source evaluation, and integrating findings into plans and memory bank.
