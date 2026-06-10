---
name: research-and-recon-axiom
description: >
  Research and reconnaissance skill for Axiom agents. Covers when and how to
  search the web, read documentation, investigate error messages, and find
  answers to problems encountered during planning, step execution, and verify
  cycles. Prevents agents from guessing or relying solely on training data when
  current, accurate information is available online. Always get the current date
  before searching to avoid returning stale results.
version: "1.0"
tags:
  vertical: [coding, planning, debugging]
  category: methodology
  core: false
---

# Research and Reconnaissance (Axiom)

> **"Don't guess when you can look it up. Don't rely on training data when the answer is one search away."**
>
> **"An agent that searches is more useful than an agent that assumes."**
>
> **"The current date matters. A search for 'best practice X' that returns 2021 results may be actively harmful in 2026."**

## Purpose

This skill teaches Axiom agents to use available search and research tools effectively during planning, step execution, and verify cycles. It covers:

- **When to search** — recognising the situations where searching beats guessing
- **How to search well** — query construction, date anchoring, source evaluation
- **What tools to use** — searxng MCP, webfetch, documentation sites
- **How to integrate findings** — turning search results into plan steps, spec updates, or evidence

## When to Load This Skill

Load when:
- A step is blocked or failing and the error message is unfamiliar
- Planning a feature that uses a library, API, or protocol you haven't used recently
- A verify cycle keeps failing and the root cause isn't obvious from the code
- You're about to make an assumption about how a third-party system works
- You need to know the current version, API shape, or best practice for something
- You're stuck in a step-loop/verify cycle and internal knowledge isn't resolving it
- A spec references an external standard, RFC, or protocol you need to understand

Do NOT load when:
- The answer is clearly in the repo's own specs, code, or memory bank
- The question is about Axiom internals (use the skills and specs instead)
- You already have recent, verified information from a previous search in this session

---

## Step 0 — Always Get the Current Date First

**Before searching, get the current date.** This is not optional. Training data has a cutoff; the web does not. A search without date context may return outdated results that are actively wrong.

```bash
date -u +"%Y-%m-%d"
# or in the system prompt context: today's date is available as a variable
```

Use the current date to:
- Add `after:YYYY-MM-DD` or `site:docs.example.com` filters to searches
- Evaluate whether a result is recent enough to trust
- Anchor your query: "rust tokio 2025" not just "rust tokio"
- Recognise when a result is from before a major version change

---

## Available Research Tools

### 1. SearXNG MCP (`searxng_searxng_web_search`)

A privacy-respecting meta-search engine that aggregates results from multiple sources. Available as an MCP tool.

```
Tool: searxng_searxng_web_search
Parameters:
  query: string        — the search query
  language: string     — language code (default: "en")
  time_range: string   — "day" | "month" | "year" (optional, for recency)
  pageno: int          — page number (default: 1)
```

**Best for**: General web searches, finding documentation, error messages, library APIs, blog posts, Stack Overflow answers.

### 2. URL Fetch (`searxng_web_url_read`)

Fetches and reads the content of a specific URL. Use after finding a promising result from search.

```
Tool: searxng_web_url_read
Parameters:
  url: string          — the URL to fetch
  maxLength: int       — max characters to return (optional)
  section: string      — extract content under a specific heading (optional)
```

**Best for**: Reading documentation pages, GitHub READMEs, RFC text, API references.

### 3. WebFetch (built-in)

The `webfetch` tool fetches a URL and returns its content as markdown. Available to all agents with `webfetch: true`.

**Best for**: Quick reads of known URLs, documentation pages, GitHub issues.

---

## How to Search Well

### Query Construction

**Be specific and version-aware:**
```
# Bad — too vague, may return old results
"rust async error handling"

# Good — specific, version-anchored
"rust tokio 1.x async error handling 2024"
"rust anyhow thiserror difference 2025"
```

**Include the error message verbatim (in quotes):**
```
# For error messages, quote the exact text
'"cannot borrow as mutable because it is also borrowed as immutable" rust fix'
'"ECONNREFUSED 127.0.0.1:5432" docker postgres connection'
```

**Use site: for authoritative sources:**
```
"site:docs.rs tokio::sync::Mutex"
"site:developer.mozilla.org fetch API abort signal"
"site:kubernetes.io pod security context"
```

**Add recency filters when currency matters:**
```
"kubernetes ingress nginx 2025 after:2024-01-01"
```

### Source Evaluation

Prefer in this order:
1. **Official documentation** (docs.rs, developer.mozilla.org, kubernetes.io, etc.)
2. **Official GitHub repos** (README, CHANGELOG, issues with maintainer responses)
3. **Recent Stack Overflow answers** (check the date — answers from 3+ years ago may be outdated)
4. **Blog posts from known maintainers** (check author and date)
5. **General blog posts** (lowest trust — verify against official docs)

**Red flags:**
- Answer is from before a major version change (e.g., Python 2 answer for Python 3 question)
- Answer has many downvotes or comments saying "this is outdated"
- Blog post has no date or is clearly old
- Result is from a content farm or AI-generated article site

### Reading Results

Don't just read the first result. When stuck:
1. Search for the error/question
2. Scan the top 3-5 results for relevance
3. Fetch the most promising 1-2 URLs
4. Cross-reference: if two independent sources agree, higher confidence
5. Check the official docs to confirm

---

## When to Search During Step Execution

### Blocked on an error

If a step fails with an error you don't immediately recognise:
1. Search the exact error message
2. Search `<library> <error> fix <year>`
3. Check the library's GitHub issues for the error
4. Check the library's CHANGELOG for breaking changes

### Unfamiliar API or library

Before writing code that uses an external library or API:
1. Search for the current API shape (it may have changed since training)
2. Fetch the official docs page for the specific function/endpoint
3. Check for deprecation notices or migration guides

### Best practice questions

When the plan says "implement X using best practices":
1. Search for current best practices for X (with year)
2. Check if there's an official style guide or recommendation
3. Look for recent blog posts from the library maintainers

### Version compatibility

When combining libraries or tools:
1. Search for compatibility matrix or known issues between the versions you're using
2. Check the library's CHANGELOG for the version range you're targeting
3. Search for `<lib-a> <lib-b> compatibility <year>`

---

## When to Search During Verify Cycles

If a verify cycle keeps failing and you've tried the obvious fixes:

1. **Search the specific test failure** — the exact assertion message, not a paraphrase
2. **Search the spec requirement** — maybe the spec references an RFC or standard that has clarifying text online
3. **Search for known issues** — `<library> <version> known issues` or `<library> <version> bug`
4. **Search for the pattern** — if the verify is failing on a wiring gap, search for how others have wired the same components

**Don't keep retrying the same fix.** If you've tried 2-3 approaches and they're all failing, stop and search. The answer is probably in the docs or a GitHub issue.

---

## Integrating Research Findings

### Into plan steps

If research reveals a better approach than what's in the plan:
1. Note the source URL in the step's evidence
2. Update the step's `actions` with the correct approach
3. Add a `axiom:trace` comment referencing the source

### Into specs

If research reveals a spec is wrong or incomplete:
1. Note the discrepancy
2. Update the spec with the correct information and a source citation
3. Flag it as a spec update in the verification evidence

### Into memory bank

If research reveals something that will be useful in future sessions:
1. Write a note to `.memory-bank/topics/` with the finding
2. Include the source URL, date found, and why it's relevant
3. This prevents re-searching for the same thing in future sessions

### Into evidence

Always record what you searched for and what you found:
```
## Research Evidence
- Query: "rust tokio 1.x broadcast channel capacity 2025"
- Source: https://docs.rs/tokio/latest/tokio/sync/broadcast/index.html
- Finding: Capacity must be a power of 2; non-power-of-2 values are rounded up
- Date: 2026-05-22
```

---

## Anti-Patterns to Avoid

| Anti-pattern | Why it's bad | What to do instead |
|---|---|---|
| Guessing API shapes from memory | Training data may be outdated | Fetch the current docs |
| Using the first search result without reading it | May be outdated or wrong | Read and evaluate 2-3 results |
| Searching without a date anchor | May return stale results | Add year or `after:` filter |
| Re-trying the same fix 5 times | Wastes time, same result | Search after 2 failed attempts |
| Trusting AI-generated content about APIs | Often hallucinated | Use official docs only |
| Not recording what you found | Future agents re-search the same thing | Write to memory bank |
| Searching for Axiom internals online | Axiom is internal | Use specs/ and skills/ instead |

---

## Quick Reference

```
# Get current date first
date -u +"%Y-%m-%d"

# Search for an error
searxng_searxng_web_search(
  query='"exact error message here" fix 2025',
  time_range="year"
)

# Search for API docs
searxng_searxng_web_search(
  query='site:docs.rs tokio broadcast channel'
)

# Fetch a specific page
searxng_web_url_read(
  url='https://docs.rs/tokio/latest/tokio/sync/broadcast/',
  section='Examples'
)

# Search for best practices
searxng_searxng_web_search(
  query='rust async best practices 2025 after:2024-06-01'
)
```

axiom:trace work_item=command-quality-01 spec=specs/09-Baby-Steps-Methodology.md doc=.opencode/skills/research-and-recon-axiom/SKILL.md
