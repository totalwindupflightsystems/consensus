---
name: notion-mcp-axiom
description: Read, write, and update content in Notion workspaces using the Notion MCP integration. Use this skill when any agent needs to interact with Notion pages, databases, comments, or meeting notes. Covers searching, fetching, creating, updating, querying, and organizing Notion content.
version: "1.0"
tags:
  vertical: [planning, writing, personal-context]
  category: tooling
  core: false
---

# Notion MCP Skill for Axiom

This skill provides Axiom agents with structured guidance for reading, writing, and updating content in Notion via the Notion MCP server. Any agent that needs to interact with Notion should load this skill first.

## When to Use This Skill

Load this skill when:
- You need to **read** Notion pages, databases, or meeting notes.
- You need to **create** new pages, databases, or comments in Notion.
- You need to **update** existing Notion pages (properties, content, or verification status).
- You need to **search** across a Notion workspace for information.
- You need to **query** structured data from Notion databases.
- You need to **organize** pages by moving them between parents.
- You need to **manage** database schemas (add/remove/rename columns).

## Available Notion MCP Tools

The Notion MCP exposes the following tool groups. Each tool is prefixed with `notion_notion-`.

### Search & Discovery

| Tool | Purpose | When to Use |
|------|---------|-------------|
| `notion_notion-search` | Semantic search over workspace and connected sources (Slack, Google Drive, GitHub, Jira, etc.) | Finding pages, databases, or content by topic. Also supports user search by name/email. |
| `notion_notion-get-teams` | List teamspaces in the workspace | Discovering team structure, filtering search by teamspace. |
| `notion_notion-get-users` | List workspace members and guests | Finding user IDs for filters, mentions, or assignments. |

### Reading Content

| Tool | Purpose | When to Use |
|------|---------|-------------|
| `notion_notion-fetch` | Fetch full content of a page, database, or data source by URL or ID | Reading page content, getting database schemas, discovering data source IDs. |
| `notion_notion-get-comments` | Get discussions and comments from a page | Reading feedback, review threads, or inline discussions. |
| `notion_notion-query-data-sources` | Query database data using SQL or by view | Extracting structured data, filtering records, generating reports, aggregating across databases. |
| `notion_notion-query-meeting-notes` | Query the current user's meeting notes | Finding meetings by date, attendee, or title. |

### Creating Content

| Tool | Purpose | When to Use |
|------|---------|-------------|
| `notion_notion-create-pages` | Create one or more pages (standalone, under a page, or in a database) | Adding new documentation, project pages, task entries, or notes. |
| `notion_notion-create-database` | Create a new database with SQL DDL schema | Setting up structured tracking (tasks, issues, inventories, etc.). |
| `notion_notion-create-comment` | Add a page-level, inline, or reply comment | Leaving feedback, review notes, or discussion threads. |

### Updating Content

| Tool | Purpose | When to Use |
|------|---------|-------------|
| `notion_notion-update-page` | Update page properties, replace/insert content, apply templates, or manage verification | Editing existing pages, updating status fields, appending content, verifying pages. |
| `notion_notion-update-data-source` | Modify database schema (add/drop/rename/alter columns) | Evolving database structure, adding new properties, renaming fields. |
| `notion_notion-move-pages` | Move pages or databases to a new parent | Reorganizing workspace structure. |
| `notion_notion-duplicate-page` | Duplicate an existing page | Creating copies for templates or branching. |

## Key Concepts

### Pages vs Databases vs Data Sources

- **Page**: A document with rich content (Markdown-like) and optional properties.
- **Database**: A structured collection with a schema. May contain multiple **data sources**.
- **Data Source**: The actual collection backing a database view. Identified by `collection://` URLs in fetch output. Multi-source databases (e.g., with linked sources) have multiple data sources.

**Important**: When creating pages in a database, always `fetch` the database first to get the schema and data source IDs from `<data-source url="collection://...">` tags.

### Content Format

Notion uses an enhanced Markdown format. Before writing or updating page content:
1. Fetch the spec at `notion://docs/enhanced-markdown-spec` for the complete syntax.
2. Do NOT guess Markdown syntax — the Notion flavor has extensions (colored headings, toggles, callouts, etc.).

### Special Blocks

Notion's enhanced Markdown supports special self-closing block tags:

| Block | Syntax | Purpose |
|-------|--------|---------|
| Table of Contents | `<table-of-contents/>` | Renders Notion's built-in auto-updating TOC. Automatically lists all headings on the page and stays in sync as headings change. **Always prefer this over manually building a TOC from a bulleted list.** |
| Divider | `---` | Horizontal rule / section divider. |
| Empty block | `<empty-block/>` | An intentional empty paragraph (avoid in most cases). |

**Important**: The `<table-of-contents/>` block is a native Notion feature — it auto-updates when headings are added, removed, or reordered. Never manually construct a TOC from a bulleted list of section links; it will go stale immediately.

### Property Types

When setting page properties in databases:
- **Date**: Split into `date:{property}:start`, `date:{property}:end` (optional), `date:{property}:is_datetime` (0 or 1).
- **Place**: Split into `place:{property}:name`, `place:{property}:address`, `place:{property}:latitude`, `place:{property}:longitude`.
- **Number**: Use JavaScript numbers (not strings).
- **Checkbox**: Use `__YES__` for checked, `__NO__` for unchecked.
- **Special naming**: Properties named "id" or "url" must be prefixed with `userDefined:`.

### Search Modes

`notion_notion-search` supports two query types:
- `internal` — Semantic search over workspace and connected sources. Supports date and creator filters.
- `user` — Search for users by name or email.

For searching within a specific database, first `fetch` the database to get data source URLs, then pass the `data_source_url` parameter.

## Common Workflows

### Workflow 1: Find and Read a Page

```
1. Search:  notion_notion-search(query="project kickoff notes")
2. Fetch:   notion_notion-fetch(id="<page_id_from_search>")
```

### Workflow 2: Create a Page in a Database

```
1. Fetch DB: notion_notion-fetch(id="<database_url>")
   → Note the schema and data source ID from <data-source> tags
2. Create:   notion_notion-create-pages(
     parent={data_source_id: "<id>"},
     pages=[{properties: {Title: "New Item", Status: "To Do"}}]
   )
```

### Workflow 3: Update Page Content

```
1. Fetch:  notion_notion-fetch(id="<page_id>")
   → Read current content to find exact text for selection
2. Update: notion_notion-update-page(
     page_id="<id>",
     command="replace_content_range",
     selection_with_ellipsis="# Old Secti...end of section",
     new_str="# Updated Section\nNew content here"
   )
```

### Workflow 4: Query Database Records

```
1. Fetch DB: notion_notion-fetch(id="<database_url>")
   → Get data source URL (collection://...)
2. Query:    notion_notion-query-data-sources(data={
     data_source_urls: ["collection://<id>"],
     query: "SELECT * FROM \"collection://<id>\" WHERE Status = ? LIMIT 20",
     params: ["In Progress"]
   })
```

### Workflow 5: Add a Comment to a Page

```
1. Comment: notion_notion-create-comment(
     page_id="<id>",
     rich_text=[{text: {content: "Review feedback here"}}]
   )
```

### Workflow 6: Create a New Database

```
1. Create: notion_notion-create-database(
     title="Sprint Tasks",
     parent={page_id: "<parent_page_id>"},
     schema="CREATE TABLE (\"Task\" TITLE, \"Status\" SELECT('To Do':red, 'In Progress':yellow, 'Done':green), \"Assignee\" PEOPLE, \"Due\" DATE)"
   )
```

### Workflow 7: Find and Read Meeting Notes

```
1. Query: notion_notion-query-meeting-notes(filter={
     operator: "and",
     filters: [{
       property: "created_time",
       filter: {operator: "date_is_within", value: {type: "relative", value: "the_past_week"}}
     }]
   })
2. Fetch: notion_notion-fetch(id="<meeting_page_id>")
```

## Safety Rules

- **Never delete child pages** without explicit user confirmation. If `replace_content` or `replace_content_range` would delete child pages, the operation fails. Do NOT automatically set `allow_deleting_content: true` — always ask the user first.
- **Preserve existing content** when updating. Always `fetch` before updating to understand current state.
- **Respect workspace permissions**. Operations may fail if the agent lacks access to certain pages or databases.
- **No secret leakage**. Never write secrets, tokens, or credentials into Notion pages. Redact as `[REDACTED]` if encountered.
- **Duplication is async**. After `duplicate-page`, the new page populates asynchronously — do not immediately fetch and expect full content.

## Integration with Axiom Agents

Any Axiom agent can load this skill. Common use cases by agent:

| Agent | Notion Use Case |
|-------|----------------|
| `@pm-axiom` | Create/update project tracking databases, query task status, manage roadmaps |
| `@specwriter-axiom` | Publish specs to Notion, read requirements from Notion pages |
| `@docs-runbooks-axiom` | Publish documentation and runbooks to Notion |
| `@sitrep-axiom` | Read/write situation reports, query meeting notes for context |
| `@incident-commander-axiom` | Create incident pages, update timelines, post comments |
| `@release-manager-axiom` | Update release tracking databases, publish changelogs |
| `@memory-bank-axiom` | Sync durable knowledge between memory bank and Notion |
| `@tower-axiom` | Orchestrate Notion reads/writes across agent workflows |

## Troubleshooting

| Problem | Solution |
|---------|----------|
| "Page not found" | Verify the page ID/URL is correct and accessible. Try searching first. |
| "Cannot create page in database" | Fetch the database first to get the correct data source ID. Use `data_source_id`, not `database_id`, for multi-source databases. |
| "Property not found" | Fetch the database schema and use exact property names from the schema. |
| "Content deletion blocked" | The update would delete child pages. Include `<page url="...">` tags to preserve them, or ask the user before setting `allow_deleting_content: true`. |
| "Template content not appearing" | Template application is async. Wait a moment and re-fetch. |
| SQL query returns no results | Verify data source URL matches the `collection://` format from fetch output. Check column names match schema exactly. |
