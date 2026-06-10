---
name: traceability-doctrine
description: Portable traceability rules for Axiom (trace markers, required links, validation checks, commit/PR templates). Includes L0 prd→spec link and external reference fields for Jira/Notion/GitHub mirrors.
version: "1.2"
tags:
  vertical: [coding, ops, sre, writing, security, planning, onboarding, benchmarking, personal-context]
  category: traceability
  core: true
---

# Traceability Doctrine (Portable)

This skill is a portable, self-contained version of Axiom traceability rules.
Use it when authoring or reviewing:
- PRDs (product requirements documents in `prds/`)
- specs/requirements
- plans (`plan.yaml`)
- implementation code and tests
- evidence bundles
- commit messages and PR descriptions

## Core Principle

Everything links to everything else in a navigable graph. The graph MUST be:
- Grep-friendly
- Stable across refactors
- Incremental (placeholders allowed)
- Verifiable by an independent auditor

## Canonical Trace Marker Format

One line, grep-friendly:

```
axiom:trace work_item=<ID> spec=<path#anchor> plan=<phase/task/step> test=<path> doc=<path> prompt=<path> evidence=<path> commit=<sha>
```

Rules:
- `work_item` is REQUIRED. All other fields are optional but SHOULD be filled as they become known.
- Use `#` anchors for spec sections when possible.
- Placeholders are allowed: `test=` / `doc=` / `evidence=` etc.

Field meanings:
- `work_item`: Jira key or stable slug (e.g., `ABC-123`, `bootstrapping-01`)
- `spec`: spec path + optional `#anchor`
- `plan`: plan reference like `phase-1/task-2/step-3`
- `test`: concrete test reference (e.g., `tests/foo/test_bar.py::test_name`)
- `doc`: docs/runbook path
- `prompt`: prompt mirror / agent prompt path
- `evidence`: evidence bundle path (typically `.memory-bank/work-items/<ID>/verification.md`)
- `commit`: short sha (leave blank until it exists)

## External Reference Fields (Jira, Notion, GitHub)

When MCP integrations are available (Atlassian MCP, Notion MCP, GitHub MCP), trace markers MAY include external reference fields. These are **mirrors** — the git repo is always the source of truth.

**Source-of-truth principle**: The git repo (specs, code, tests, evidence in `.memory-bank/`) is always canonical. Jira tickets, Notion pages, and GitHub issues/PRs are mirrors that provide visibility and workflow integration. If a conflict exists between the repo and an external system, the repo wins.

Additional fields:

| Field | Value format | Example | When to include |
|---|---|---|---|
| `jira_ref` | Jira issue key or URL | `PROJ-123` | When a Jira ticket tracks this work item |
| `notion_ref` | Notion page ID or URL | `https://notion.so/workspace/Page-abc123` | When a Notion page documents this work |
| `github_ref` | GitHub issue/PR URL | `https://github.com/org/repo/pull/42` | When a GitHub PR or issue tracks this change |
| `prd` | PRD path | `.memory-bank/prds/billing/csv-export.md` | When a PRD produced this spec (L0 link) |

Extended format:
```
axiom:trace work_item=<ID> spec=<path#anchor> plan=<phase/task/step> test=<path> doc=<path> evidence=<path> jira_ref=<key> notion_ref=<url-or-id> github_ref=<url>
```

Rules for external references:
- External reference fields are OPTIONAL. They enrich the trace graph but are never required for trace completeness.
- `jira_ref` SHOULD be included when the work item is mirrored to Jira. Prefer short key format (`PROJ-123`).
- `notion_ref` SHOULD be included when a Notion page contains related documentation or RFCs.
- `github_ref` SHOULD be included when a GitHub PR or issue is directly related.
- External references MUST NOT replace repo-local references. A `spec=` field is always required alongside any `notion_ref=`.
- When MCP tools are unavailable, agents SHOULD still include known values (e.g., Jira keys from intake).

## Placement Rules (Behavior Boundaries)

Trace markers MUST appear near:
- function/method definitions implementing spec-defined behavior
- class definitions implementing spec-defined concepts
- API endpoint handlers
- test functions that verify spec requirements

Trace markers SHOULD appear in:
- configuration files implementing spec-defined defaults
- runbooks/docs for operational procedures
- prompt mirrors encoding API/data invariants

## Language Examples

Python:
```python
# axiom:trace work_item=ABC-123 spec=specs/<repo-spec>.md#anchor plan=phase-1/task-2/step-3 test=tests/test_x.py::test_y
def handler(...):
    ...
```

TypeScript:
```ts
// axiom:trace work_item=ABC-123 spec=specs/... plan=phase-1/task-1/step-1
export function doThing() {
  ...
}
```

YAML:
```yaml
# axiom:trace work_item=ABC-123 spec=specs/... plan=phase-1/task-1/step-1
steps:
  - id: "step-1"
    title: "..."
```

Markdown:
```markdown
axiom:trace work_item=ABC-123 spec=specs/... plan=phase-1/task-1/step-1
```

## Minimum Required Links (Contract)

Each row MUST be satisfiable for work to be considered trace-complete.

| # | From | To | Where | Format |
|---|------|----|-------|--------|
| L0 | PRD | Spec section(s) | `prds/<file>.md` Spec-Merge Appendix + `axiom:trace prd=` in spec | `prd=prds/<file>.md` field in trace marker; `prds/README.md` index row |
| L1 | work item | spec refs | `.memory-bank/work-items/<ID>/meta-planning.md` | list spec paths |
| L2 | spec section | realized-by code/tests | spec text near requirement | `Realized by: ...` |
| L3 | plan step | spec ref + verification ref | `.memory-bank/work-items/<ID>/plan.yaml` | `spec_ref`, `verification` |
| L4 | code | spec ref (+ plan ref when known) | inline `axiom:trace` marker | `spec=... plan=...` |
| L5 | tests | spec ref (+ code ref when known) | inline `axiom:trace` marker | `test=... spec=...` |
| L6 | evidence bundle | work item + run + AC coverage | `.memory-bank/work-items/<ID>/verification.md` | tables + frontmatter |
| L7 | PR description | work item + evidence path | PR body `## Trace` | filled template |
| L8 | commit message | work item + spec/plan refs | commit subject + footer | `Trace: spec=... plan=...` |

### External System Mirror Links (Optional, MCP-Enhanced)

These links are optional but SHOULD be maintained when MCP integrations are configured:

| # | From | To | Where | Format | MCP Required |
|---|---|---|---|---|---|
| E1 | Work item (repo) | Jira ticket | `axiom:trace` `jira_ref=` field | `jira_ref=PROJ-123` | Atlassian MCP |
| E2 | Spec section | Notion RFC/doc | `axiom:trace` `notion_ref=` field | `notion_ref=<page-url>` | Notion MCP |
| E3 | PR description | Jira + Notion | PR `## Trace` section | URL or key | GitHub + Atlassian MCP |
| E4 | Evidence bundle | Jira comment | Jira comment with evidence summary | Comment + repo link | Atlassian MCP |
| E5 | Work item (repo) | GitHub PR/issue | `axiom:trace` `github_ref=` field | `github_ref=<pr-url>` | GitHub MCP |

Mirror sync rules:
- Git repo is always the source of truth; external systems are downstream mirrors.
- When a work item is created/updated and Atlassian MCP is available, a Jira comment SHOULD be posted.
- When evidence is produced and Atlassian MCP is available, a Jira comment SHOULD summarize it.
- Notion pages are updated via Notion MCP when RFCs or documentation change. Repo spec is updated first.
- If MCP is unavailable, external reference fields are still included with known values. Validation is best-effort.

## Commit Message Footer

Use a `Trace:` footer (except pure chore commits):

```
Trace: spec=<comma-separated-spec-refs> plan=<comma-separated-plan-refs>
```

## PR Description Template (Trace Section)

Minimum trace block to include in PR body:

```markdown
## Trace
- Work item: <ID>
- Specs: <spec refs>
- Plan: <phase/task/step refs>
- Evidence: <path>
- Jira: <Jira ticket URL or key, or "N/A">
- Notion: <Notion page URL if applicable, or "N/A">
- GitHub: <related issue/PR URLs if applicable, or "N/A">
```

## Validation Checks (Mechanical)

Use these checks to audit traceability:

1) All changed code/test files contain at least one `axiom:trace` marker.
2) All `spec=` paths referenced by trace markers exist.
3) Plan steps include `spec_ref` (or `N/A - <reason>`).
4) Evidence bundle exists and maps AC -> verification paths.
5) Non-chore commits include `Trace:` footer.
6) External reference fields (`jira_ref=`, `notion_ref=`, `github_ref=`) are well-formed when present (valid Jira key format, valid URL format). Best-effort when MCP is unavailable.
7) PR `## Trace` section includes Jira/Notion/GitHub fields (may be "N/A").

## Failure Handling

If any required link is missing:
- do not declare PASS
- inject a remediation step to add the missing trace marker/link
- re-run trace audit
