# REFERENCE.md — Tool API Quick Reference for Pattern Building

This file documents the exact tool interfaces available for pattern composition.
Use this when building or debugging patterns — it has the correct field names, types, and gotchas.

<!-- axiom:trace work_item=pattern-generator-01 spec=specs/121-Pattern-Generator.md -->

---

## Tool Access Methods

| Plugin | Access Type | How Agents Call It |
|--------|------------|-------------------|
| Graph Harness | MCP tool | `graph_create`, `graph_status`, etc. — direct tool call |
| Tree Memory | MCP tool | `tree.commit`, `tree.query`, etc. — direct tool call |
| Context Stash | MCP tool | `stash.push`, `stash.pop`, etc. — direct tool call |
| Conductor | MCP tool | `conductor.spawn`, `conductor.status`, etc. — direct tool call |
| ShellOps | MCP tool (HTTP proxy) | `shellops.*` tools — proxied to Go daemon on :9876 |
| Code Intelligence | Built-in tool | `code-intel` tool with `operation` parameter |
| Feed Ingestion | MCP tool | `feed.poll`, `feed.status`, etc. — direct tool call |

---

## Graph Harness

### `graph_create`
```
Input:  { name: string, nodes: Node[] }
Node:   { id: string, title: string, type: "task"|"gate"|"decision", depends_on?: string[] }
Output: { graph_id: string, node_count: number, edge_count: number, status: string }
```

**Gotchas:**
- `name` is the graph name (not `title` — that's for nodes)
- `depends_on` creates edges automatically
- Returns `warning` about session not registered — safe to ignore in patterns

### `graph_status`
```
Input:  { graph_id?: string }
Output: (with ID) { graph_id, status, nodes: [...], progress: {...} }
Output: (without ID) { error: "graph_id is required", recent_graphs: [...] }
```

**Gotchas:**
- Calling with no args returns a helpful list of recent graphs
- Use this to discover existing graphs

### `graph_template_load`
```
Input:  { template_name: string, params?: Record<string, string> }
Output: { graph_id: string, nodes_injected: number, unresolved_placeholders?: string[] }
```

### `graph_inject`
```
Input:  { graph_id: string, nodes: Node[] }
Output: { injected: number }
```

---

## Tree Memory

### `tree.status`
```
Input:  {}
Output: { initialized: boolean, branches: number, active_agents: number, current_branch: string }
```

### `tree.init`
```
Input:  {}
Output: { status: "initialized"|"already_initialized", path: string }
```

### `tree.branch`
```
Input:  { action: "create"|"list"|"delete", name?: string }
Output: (create) { status: "created", branch: string }
Output: (list) { branches: string[] }
```

**Gotchas:**
- `action` is REQUIRED — omitting it causes "Unknown action: undefined"
- Branch name is in `name` field

### `tree.commit`
```
Input:  { file: string, content: string, message: string }
Output: { status: "committed", file: string, branch: string }
```

**Gotchas:**
- `file` must be a path relative to the tree root (e.g., `findings/my-finding.json`)
- `content` must be a string (JSON.stringify objects first)
- All three fields are REQUIRED

### `tree.query`
```
Input:  { surface: "findings"|"peers"|"watches"|"events"|"trends" }
Output: string (framed log match content)
```

### `tree.diff`
```
Input:  { branch_a: string, branch_b?: string }
Output: { diff: string }
```

**Gotchas:**
- `branch_a` is REQUIRED
- `branch_b` defaults to the repo's default branch (auto-detected from git)

---

## Context Stash

### `stash.push`
```
Input:  { summary: string, tags?: string, name?: string }
Output: { stash_id: string, name: string, state: string, message: string }
```

**Gotchas:**
- `summary` is REQUIRED — omitting causes clear error
- `tags` is a COMMA-SEPARATED STRING, not an array! `"tag1,tag2"` ✅ `["tag1","tag2"]` ❌
- This is the #1 mistake agents make with stash

### `stash.list`
```
Input:  {}
Output: { count: number, stashes: Stash[] }
Stash:  { stash_id, name, state, tags: string[], age, created_at, last_agent }
```

### `stash.pop`
```
Input:  { id: string }
Output: (the stash content)
```

### `stash.peek`
```
Input:  { id: string }
Output: (the stash content without removing it)
```

### `stash.drop`
```
Input:  { id: string }
Output: { status: "dropped" }
```

---

## ShellOps (via MCP → HTTP daemon on :9876)

### Prerequisites
The ShellOps daemon MUST be running. Verify: `curl http://127.0.0.1:9876/health`

### `shellops.classify`
```
Input:  { command: string }
Output: { level: "SAFE"|"CAUTIOUS"|"DANGEROUS"|"BLOCKED", reason: string, environment: string }
```

### `shellops.terminal_create`
```
Input:  { name: string }
Output: { session_id: string, name: string, multiplexer: string, working_dir: string }
```

### `shellops.terminal_send`
```
Input:  { session_id: string, command: string }
Output: { output: string, exit_code: number, duration: number }
```

### `shellops.terminal_read`
```
Input:  { session_id: string }
Output: { output: string }
```

### `shellops.terminal_kill`
```
Input:  { session_id: string }
Output: { status: "destroyed" }
```

### `shellops.watch_start`
```
Input:  { file_path: string, pattern: string, id: string }
Output: { watch_id: string, file_path: string, pattern: string, active: true, match_count: 0 }
```

**Gotchas:**
- `file_path` must be absolute or relative to daemon root
- `pattern` is a regex (use `|` for OR: `"ERROR|WARN"`)
- `id` becomes the `name`, but the system assigns its own `watch_id`
- Use the returned `watch_id` for query/stop — NOT the `id` you passed in

### `shellops.watch_query`
```
Input:  { watch_id: string, limit?: number }
Output: { watch_id: string, match_count: number, matches: Match[] }
Match:  { line_number, line_content, framed_content, matched_at }
```

### `shellops.watch_stop`
```
Input:  { id: string }  // This is the watch_id from create response
Output: { status: "stopped" }
```

### `shellops.triage`
```
Input:  { error_rate?: string, latency_p99?: string, cpu?: string, service?: string, context?: string }
Output: { severity: string, mode: string, score: number, template: string, spawn_count: number }
```

**Gotchas:**
- Fields are FLAT (not nested in a `signals` object)
- All fields are optional

### `shellops.investigate`
```
Input:  { service: string, symptom: string, context?: string }
Output: { id: string, status: "started", timebox_minutes: number }
```

### `shellops.broadcast`
```
Input:  { channel: string, severity: string, text: string, service?: string }
Output: { status: "sent", records: Record[] }
```

---

## Code Intelligence (Built-in Tool)

### `code-intel` (status)
```
Input:  { operation: "status" }
Output: { file_count, symbol_count }
```

### `code-intel` (query)
```
Input:  { operation: "query", symbol?: string, path?: string }
Output: { matches: Match[] }
Match:  { name, kind, path, line, language }
```

### `code-intel` (changes)
```
Input:  { operation: "changes", base?: string }
Output: { changed_files, affected_symbols, limitations, freshness }
```

**Gotchas:**
- Requires clean git state for accurate change detection
- `base` defaults to HEAD (use `HEAD~N` for recent changes)

---

## Feed Ingestion

### `feed.poll`
```
Input:  { feed_id: string }
Output: { items_found, items_evaluated, new_items, stored }
```

### `feed.status`
```
Input:  { feed_id?: string }
Output: { feeds: Feed[], total_items: number }
```

### `feed.configure`
```
Input:  { feed_id: string, url: string, schedule?: string }
Output: { status: "configured" }
```

---

## Conductor

### `conductor.spawn`
```
Input:  { name: string, task: string, session_id?: string }
Output: { agent_id: string, status: "spawned" }
```

**Gotchas:**
- Requires SPIRE auth OR `allow_spawn_secret_fallback: true` in config
- Without auth: throws CRITICAL error (this is correct spec behavior)
- For local dev/testing: set env `AXIOM_CONDUCTOR__AUTH__ALLOW_SPAWN_SECRET_FALLBACK=true`

### `conductor.status`
```
Input:  { agent_id?: string }
Output: (with ID) { agent_id, status, elapsed, cost }
Output: (without ID) { agents: Agent[] }
```

---

## Common Type Mismatches (Top 5 Mistakes)

| Tool | Field | Wrong | Right |
|------|-------|-------|-------|
| `stash.push` | `tags` | `["a","b"]` (array) | `"a,b"` (string) |
| `tree.branch` | `action` | omitted | `"create"` or `"list"` |
| `tree.commit` | `content` | `{obj}` (object) | `JSON.stringify({obj})` (string) |
| `shellops.watch_query` | `watch_id` | the name you passed | the `watch_id` returned by create |
| `shellops.triage` | signals | `{signals:{...}}` (nested) | `{error_rate:..., cpu:...}` (flat) |

---

## Verification Commands (for pattern testing only)

When testing a pattern implementation (NOT for the pattern itself):

```bash
# Build ShellOps if needed
GO=$(find /home/coder -path "*/bin/go" 2>/dev/null | grep -v gopath | head -1)
cd shellops && $GO build -o ../_tmp/shellops-bin ./cmd/shellops/ && cd ..

# Start daemon
(_tmp/shellops-bin start --port 9876 --root . >> _tmp/shellops-daemon.log 2>&1 &)
sleep 2

# Verify all systems
curl -s http://127.0.0.1:9876/health                    # ShellOps
bun test .opencode/tests/graph-harness.test.ts          # Graph Harness
bun test .opencode/tests/tree-memory.test.ts            # Tree Memory
bun test .opencode/tests/context-stash.test.ts          # Context Stash
_tmp/axiom-code-intel index --repo . --out /dev/null  # Code Intelligence
```
