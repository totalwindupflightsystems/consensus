---
name: tree-memory-axiom
description: >-
  DuckDB-native tree-structured memory for Axiom. Covers hierarchical knowledge
  storage, path-based queries, two-path query architecture (direct + semantic),
  memory lifecycle, pruning strategies, and integration with the OpenCode plugin
  system. Load this skill when working with structured memory, knowledge graphs,
  or hierarchical context storage in any Axiom-managed repo.
version: "1.0"
tags:
  vertical: [knowledge, memory, data]
  category: memory
  core: false
spec: specs/113-Tree-Memory.md
---

# Tree Memory (DuckDB Native)

Tree-structured memory backed by DuckDB for fast hierarchical queries. Stores knowledge as trees (parent-child relationships) with path-based addressing and supports both direct path queries and semantic similarity search.

**Spec**: `specs/113-Tree-Memory.md`
**Plugin**: `.opencode/plugins/tree-memory.ts`
**Tests**: `.opencode/tests/tree-memory.test.ts`
**CI**: `.github/workflows/tree-memory-duckdb.yml`

<!-- axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md -->

---

## Quick Start (Setup + First Use)

```bash
# Install DuckDB dependency (one-time)
npm --registry https://registry.npmjs.org install duckdb --prefix .opencode

# Verify plugin loads
cd .opencode && bun test tests/tree-memory.test.ts

# The plugin auto-creates .tree-memory/tree.duckdb on first use — no manual DB setup needed
```

Once installed, agents use tree-memory through OpenCode plugin tools:

```typescript
// Store knowledge at a path
await tree.store("/projects/axiom/decisions/adr-001", {
  content: "Decision: Use DuckDB for tree memory...",
  metadata: { type: "adr", status: "accepted" }
});

// Query by path (exact)
const node = await tree.query("/projects/axiom/decisions/adr-001");

// Query by prefix (all decisions)
const decisions = await tree.query("/projects/axiom/decisions/", { prefix: true });

// List children
const children = await tree.list("/projects/axiom/");
```

---

## When to Load This Skill

Load when an agent needs to:
- Store structured knowledge hierarchically (not flat files)
- Query memory by path (`/projects/axiom/decisions/adr-001`)
- Find related memories via semantic similarity
- Manage memory lifecycle (TTL, pruning, archiving)
- Understand the tree-memory plugin's API and tools
- Debug DuckDB query issues or schema problems

---

## Architecture

```
Agent / Command
      ↓ (tree.store, tree.query, tree.diff)
OpenCode Plugin (tree-memory.ts)
      ↓
DuckDB Engine (embedded, zero-config)
      ↓
Two Query Paths:
  ├── Direct: path-based lookup (O(1) by path, O(log n) by prefix)
  └── Semantic: embedding similarity (requires vector column)
```

---

## Core Concepts

### Tree Nodes
Every memory entry is a node in a tree:
```json
{
  "path": "/projects/axiom/decisions/adr-001",
  "content": "Decision: Use DuckDB for tree memory storage...",
  "metadata": { "type": "adr", "status": "accepted", "created": "2026-05-17" },
  "parent": "/projects/axiom/decisions",
  "children": []
}
```

### Path-Based Addressing
Paths are hierarchical (like a filesystem):
- `/` — root
- `/projects/` — all projects
- `/projects/axiom/` — Axiom project subtree
- `/projects/axiom/decisions/` — all decisions for Axiom

### Two Query Paths (§4)
1. **Direct query**: Path lookup, prefix scan, parent/child traversal. Fast, deterministic.
2. **Semantic query**: Embedding similarity search. Requires vector column populated. Approximate, best-effort.

---

## Plugin Tools

| Tool | Description |
|------|-------------|
| `tree.store` | Store a node at a path (creates parents if needed) |
| `tree.query` | Query by path (exact or prefix) |
| `tree.diff` | Compare two subtrees or snapshots |
| `tree.prune` | Remove nodes by TTL, path pattern, or manual selection |
| `tree.list` | List children of a path |
| `tree.move` | Move a subtree to a new path |

---

## DuckDB Integration

Tree memory uses embedded DuckDB (zero external dependencies):
- **File**: `.tree-memory/tree.duckdb` (gitignored)
- **Schema**: `nodes` table with path, content, metadata, parent_path, embedding, created_at, expires_at
- **Indexes**: B-tree on path, prefix index for subtree queries

---

## Key Files

| Path | Purpose |
|------|---------|
| `.opencode/plugins/tree-memory.ts` | OpenCode plugin (DuckDB queries, tool definitions) |
| `.opencode/tests/tree-memory.test.ts` | Test suite |
| `specs/113-Tree-Memory.md` | Authoritative spec |
| `.github/workflows/tree-memory-duckdb.yml` | CI workflow |
| `.memory-bank/work-items/tree-memory-01/` | Work item artifacts |
| `.memory-bank/adrs/ADR-TM-001-duckdb-v1-deviation.md` | Design decisions |

axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md
