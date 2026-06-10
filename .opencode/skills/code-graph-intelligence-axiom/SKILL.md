---
name: code-graph-intelligence-axiom
description: >-
  Multi-language call graph engine for structural code intelligence. Covers
  graph building, callers/callees lookup, blast-radius analysis, cross-language
  edge detection, change-impact queries, symbol search, package clustering,
  and the adapter-test CLI. Load this skill when any agent needs to reason
  about code structure, call relationships, or cross-language dependencies
  in any repo managed by Axiom.
version: "1.0"
tags:
  vertical: [coding, architecture, review]
  category: code-intelligence
  core: false
spec: specs/81-Axiom-Analyze-Multi-Language-Code-Analysis.md
---

# Code Graph Intelligence

The `axiom-code-intel` binary builds a **multi-language structural call graph** and answers questions about it: who calls what, what breaks if X changes, what crosses language boundaries, where does execution flow from an entry point.

It is distinct from `axiom analyze` (health scores and linting). Graph intelligence is about **structure and relationships**, not quality metrics.

**Spec:** `specs/81-Axiom-Analyze-Multi-Language-Code-Analysis.md`
**Schema:** `axiom.code_intel.graph.v3`
**Binary:** `axiom-code-intel` (built from `code-intel/`)

<!-- axiom:trace work_item=code-graph-intelligence-skill-01 spec=specs/81-Axiom-Analyze-Multi-Language-Code-Analysis.md -->

---

## When to Load This Skill

Load when an agent needs to:

- Find all callers or callees of a function/method
- Assess the blast radius of a proposed change
- Detect cross-language dependencies (Python calling shell scripts, Terraform provisioners, etc.)
- Identify which symbols are affected by a set of changed files
- Trace execution paths from an entry point
- Cluster a codebase into package-level groups
- Validate that a shell script or procedural script appears in the graph
- Understand why cross-language edges do or don't appear in a graph
- Debug adapter behaviour for a specific language/file

---

## Quick Start

### Building the Binary

Go is NOT in default PATH on Axiom workspaces. Find it first:

```bash
# Find Go (typical Axiom workspace path)
GO=$(find /home/coder -path "*/toolchain@v0.0.1-go1.25*.linux-amd64/bin/go" 2>/dev/null | head -1)
export GOPATH=$(pwd)/_tmp/gopath

# Build from repo root
cd code-intel && $GO build -o ../_tmp/axiom-code-intel ./cmd/axiom-code-intel/ && cd ..

# Add to PATH for this session
export PATH="$PATH:$(pwd)/_tmp"
```

The binary location `_tmp/axiom-code-intel` is the standard place in Axiom workspaces.

### Usage Examples

```bash
# Build the graph for a repo (writes JSON to stdout + optional file)
axiom-code-intel graph --repo /path/to/repo --out graph.json

# Find all callers of a symbol
axiom-code-intel callers --repo . --symbol "MyPackage.MyFunc"

# Find all callees of a symbol
axiom-code-intel callees --repo . --symbol "MyPackage.MyFunc"

# Blast radius: what breaks if this symbol changes?
axiom-code-intel blast-radius --repo . --symbol "MyPackage.MyFunc"

# Follow execution from an entry point
axiom-code-intel call-chain --repo . --symbol "main.Run"

# Search for symbols by name
axiom-code-intel search --repo . --query "Deploy"

# Show package-level clusters
axiom-code-intel clusters --repo .

# Test a specific file against the adapter for its language
axiom-code-intel adapter-test --file ./scripts/deploy.sh --repo . --json

# Detect which symbols are affected by recent changes
axiom-code-intel detect-changes --repo . --base main
```

---

## Full Command Reference

### `graph` — Build the full call graph

Walks all files in `--repo`, extracts nodes and edges via language adapters, emits JSON.

```bash
axiom-code-intel graph \
  --repo /path/to/repo \          # root to analyse (default: ".")
  --out graph.json \              # save output (also writes to stdout)
  --include-tests \               # include _test.go as test_function nodes
  --include-dir build,dist \      # allow normally-blocked dirs (build/, dist/, etc.)
  --cross-lang-only               # only emit edges where kind=cross_language
```

**Output schema** (`axiom.code_intel.graph.v3`):
```json
{
  "schema_version": "axiom.code_intel.graph.v3",
  "generated_at": "...",
  "repo_root": "...",
  "nodes": [...],
  "edges": [...],
  "languages": ["go", "python", "shell", ...],
  "limitations": [...]
}
```

**Node fields:** `id`, `kind`, `name`, `qualified_name`, `file`, `line`, `language`, `confidence`, `provenance`

**Edge fields:** `id`, `kind`, `source_id`, `target_id`, `confidence`, `reason`, `cross_lang`

---

### `callers` — Who calls this symbol?

```bash
axiom-code-intel callers \
  --repo .  \
  --symbol "pkg.FuncName" \       # required; qualified name
  --index graph.json              # use pre-built graph (skip rebuild)
```

---

### `callees` — What does this symbol call?

```bash
axiom-code-intel callees \
  --repo . \
  --symbol "pkg.FuncName" \
  --index graph.json
```

---

### `blast-radius` — What is affected if this symbol changes?

Returns all symbols reachable **from** the target via call/import edges (transitive callers).

```bash
axiom-code-intel blast-radius \
  --repo . \
  --symbol "pkg.FuncName" \
  --max-depth 10 \                # default 10; clamped to 20 by server
  --min-confidence medium \       # high | medium | low (default: medium)
  --include-stubs \               # include external/stdlib stub nodes
  --index graph.json
```

---

### `call-chain` — Trace execution from an entry point

Returns a breadth-first call chain from `--symbol` up to `--max-depth` hops.

```bash
axiom-code-intel call-chain \
  --repo . \
  --symbol "main.Run" \
  --max-depth 5 \                 # default 5
  --index graph.json
```

---

### `search` — Find symbols by name

Case-insensitive substring match across all nodes.

```bash
axiom-code-intel search \
  --repo . \
  --query "Deploy" \
  --max 20 \                      # max results (default 20)
  --index graph.json
```

---

### `clusters` — Show package-level groupings

Groups nodes by inferred package/directory. Useful for understanding module structure.

```bash
axiom-code-intel clusters \
  --repo . \
  --index graph.json
```

---

### `context` — Rich context packet for a symbol

Returns the symbol plus its immediate callers and callees — optimised for feeding into agent prompts.

```bash
axiom-code-intel context \
  --repo . \
  --symbol "pkg.FuncName" \
  --max-depth 2 \                 # BFS depth (default 2)
  --max-nodes 50 \                # max nodes in packet (default 50)
  --out context.json \
  --index graph.json
```

---

### `references` — All call/import references to a symbol

```bash
axiom-code-intel references \
  --repo . \
  --symbol "pkg.FuncName" \
  --index graph.json
```

---

### `owners` — Ownership lookup for a symbol

```bash
axiom-code-intel owners \
  --repo . \
  --symbol "pkg.FuncName" \
  --index graph.json
```

---

### `detect-changes` — Impact of a git diff

Given a base ref, returns all symbols in changed files plus transitive blast radius.

```bash
axiom-code-intel detect-changes \
  --repo . \
  --base HEAD \                   # git ref to diff against (default: "HEAD")
  --expand-imports \              # add import-dependent symbols (low-confidence)
  --out impact.json \
  --index graph.json
```

---

### `index` — Pre-build the index for fast repeated queries

```bash
axiom-code-intel index \
  --repo . \
  --out index.json
```

---

### `query` — Query a pre-built index

```bash
axiom-code-intel query \
  --index index.json \
  --symbol "pkg.FuncName" \       # filter by symbol
  --path "internal/foo"           # filter by path prefix
```

---

### `report` — Generate HTML report from JSON

```bash
axiom-code-intel report \
  --input graph.json \            # index, change-map, or run-path JSON
  --out report.html \
  --repo . \
  --open                          # attempt to open in browser
```

---

### `adapter-test` — Test a single file against its language adapter

Useful for debugging why a file produces 0 nodes or unexpected edges.

```bash
axiom-code-intel adapter-test \
  --file ./scripts/deploy.sh \    # required; absolute or relative path
  --repo . \                      # repo root for relative-path resolution
  --json                          # emit structured JSON instead of human text
```

**Output fields (JSON):** `adapter`, `level`, `file`, `repo`, `nodes`, `edges`, `limitations`, `zero_node_note`

---

## Language Adapter Support

14 language adapters, grouped by confidence tier:

| Language | Confidence | Node Kinds | Edge Kinds | Notes |
|---|---|---|---|---|
| **Go** | high | function, method, type, interface, package | calls, imports, implements | Receiver-aware; type-resolved |
| **Python** | medium | function, class, module | calls, imports, inherits | Intra-file calls only; dynamic dispatch not resolved |
| **TypeScript / JS** | medium | function, class, module | calls, imports | Dynamic dispatch not resolved |
| **Terraform / HCL** | medium | resource, module, variable | depends_on, references | Provider-specific types deferred |
| **Dockerfile** | medium | stage, instruction | from, copy | Multi-stage graph limited |
| **Shell / Bash** | low | function, **file** (synthetic) | calls, sources | Functions: medium confidence. Procedural scripts (no `function` declarations) emit a synthetic `NodeKindFile` node at confidence=low |
| **Rust** | low | function, struct, trait | calls, uses | Trait dispatch deferred; macros not expanded |
| **Java** | low | method, class, interface | calls, implements | Reflection deferred |
| **Kotlin** | low | function, class | calls, implements | Coroutines deferred |
| **Swift** | low | function, class, protocol | calls, conforms | Protocol dispatch deferred |
| **C / C++ / CUDA** | low | function, struct | calls, includes | Preprocessor not expanded |
| **SQL** | low | table, view, procedure | references, calls | Dynamic SQL deferred |
| **YAML** | low | key, anchor | references | Schema-specific semantics deferred |
| **Jupyter** | low | cell, function | calls, imports | Execution order deferred |

> **Shell note:** A purely procedural `.sh` file (no `function foo() {}` declarations) now produces 1 synthetic `NodeKindFile` node with `confidence=low`. This ensures every shell script is visible in the graph and cross-language edges from Python subprocess calls can resolve to a target.

---

## Edge Kinds

| Kind | Meaning | When set |
|---|---|---|
| `calls` | Function/method invocation | Go AST calls; intra-file Python/TS calls |
| `imports` | Module/package import | Python `import`, TS `import`, Go `import`, etc. |
| `implements` | Struct/class implements interface | Go interface method-set matching |
| `cross_language` | Explicitly detected cross-language invocation | Python `subprocess.run("./script.sh")`; Terraform `local-exec` |

### Cross-Language Edge Detection

Two patterns are detected automatically:

1. **Python → Shell** — `os.system("./script.sh")`, `subprocess.run("./deploy.sh")`, `subprocess.run(["./setup.sh"])`
2. **Terraform → Shell** — `local-exec` provisioner `command = "./provision.sh"`

These edges have `kind="cross_language"` and `cross_lang=true`. Use `--cross-lang-only` on `graph` to see only these edges.

**Checking cross-language edges:**
```bash
axiom-code-intel graph --repo . --cross-lang-only --out cross.json
jq '.edges[] | select(.kind == "cross_language")' cross.json
```

---

## Import Namespace Design (Schema v3)

Package import nodes are **namespaced by language** to prevent false cross-language edges:

| Language | NodeID qualifier |
|---|---|
| Go | `go/import` |
| Python | `py/import` |
| TypeScript | `ts/import` |
| Java | `java/import` |
| Kotlin | `kotlin/import` |
| Rust | `rust/import` |
| Swift | `swift/import` |
| C | `c/import` |

This means `import os` in Python and `import "os"` in Go produce **different NodeIDs** — they no longer create phantom Python→Go cross-language edges. If you see language-pair mismatches in old graph JSON files (`v2`), regenerate with the current binary.

---

## Schema Versioning

| Version | What changed |
|---|---|
| `graph.v1` | NodeID/EdgeID were 16 hex chars (8 bytes SHA-256) |
| `graph.v2` | NodeID/EdgeID are 32 hex chars (16 bytes SHA-256) |
| `graph.v3` | Package import nodes namespaced by language (`go/import`, `py/import`, etc.) |

Old cached graph JSON files trigger a `schema_version_mismatch` warning. Regenerate: `axiom-code-intel graph --repo . --out graph.json`.

---

## Confidence Levels

| Level | Meaning |
|---|---|
| `high` | Full AST analysis (Go); type-resolved edges |
| `medium` | Regex/partial-AST; some patterns may be missed |
| `low` | Stub adapter; structure visible but edges incomplete |

Use `--min-confidence medium` (default) on blast-radius to exclude low-confidence stubs.

---

## Agent Patterns

### "What would break if I change `Foo.Bar`?"
```bash
axiom-code-intel blast-radius --repo . --symbol "Foo.Bar"
```
Returns all callers up to 10 hops deep.

### "Show me what this PR touches"
```bash
axiom-code-intel detect-changes --repo . --base main
```
Returns changed symbols + transitive blast radius.

### "Why is this Python file not showing cross-language edges?"
```bash
axiom-code-intel adapter-test --file ./caller.py --repo . --json
# Check if subprocess calls match pyShellCallRe pattern
# Then:
axiom-code-intel graph --repo . --cross-lang-only
```

### "Does this shell script appear in the graph?"
```bash
axiom-code-intel adapter-test --file ./scripts/build.sh --repo . --json
# nodes: 0 = procedural script → 1 synthetic file node emitted (confidence=low)
# nodes: N = N function declarations found
```

### "Feed symbol context into an agent prompt"
```bash
axiom-code-intel context --repo . --symbol "pkg.HandleRequest" --out context.json
# Attach context.json to the agent's input
```

### `compare` — Dual-Branch Graph Diff

Compare call graphs between two git refs to understand the structural impact of a merge.

```bash
axiom-code-intel compare [flags]
  --repo        repository root (default: .)
  --base <ref>  base git ref (default: main)
  --head <ref>  head git ref (default: HEAD)
  --out <file>  write JSON to file (default: stdout)
  --parallel    build both graphs concurrently (faster for large repos — not yet implemented)
  --cross-lang-only  only output cross-language edge changes (not yet implemented)
```

**Agent usage pattern:**
```bash
# "Show me the structural impact of merging feature-branch into main"
axiom-code-intel compare --repo . --base main --head feature-branch --out compare.json
jq '{impact: .merge_impact, added: (.added_nodes | length), removed: (.removed_nodes | length), cross_lang: (.new_cross_lang_edges | length)}' compare.json
```

**Output schema** (`schema_version: "axiom.code_intel.compare.v1"`):

| Field | Type | Description |
|---|---|---|
| `added_nodes` | array | Nodes in head but not in base (matched by `language:file:QualifiedName`) |
| `removed_nodes` | array | Nodes in base but not in head |
| `changed_edges` | array | Edges with same src→tgt but different `kind` |
| `new_cross_lang_edges` | array | New cross-language edges in head |
| `blast_radius_delta` | object | Blast-radius changes (v1: stub, `blast_radius_delta_status: "not_implemented_v1"`) |
| `merge_impact` | string | One of: `low` / `medium` / `high` / `critical` |
| `summary` | string | Human-readable diff summary |

**merge_impact interpretation:**

| Rating | Condition |
|--------|-----------|
| `low` | < 10 changed edges — minimal structural change |
| `medium` | 10–50 changed edges — moderate refactor |
| `high` | 50+ changed edges — significant structural change |
| `critical` | New cross-language edges — high risk |

axiom:trace work_item=dual-branch-analysis-01 spec=specs/81-Axiom-Analyze-Multi-Language-Code-Analysis.md#12D.2 plan=phase-2/task-2-2/step-2-2-1

---

## Blocked / Skipped Directories

The following directories are **skipped by default** during graph builds:
`.git`, `node_modules`, `__pycache__`, `vendor`, `.venv`, `.tox`, `_build`, `dist`, `build`, `target`

To include a normally-blocked directory:
```bash
axiom-code-intel graph --repo . --include-dir build,dist
```

---

## MCP Tool (`code-intel`)

When the `code-intel` MCP server is running, agents can call it directly via the `code-intel` tool. The tool names all carry the `code_intel_` prefix:

| MCP Tool Name | What it does |
|---|---|
| `code_intel_status` | Index summary (node/edge counts, languages, schema version) |
| `code_intel_query` | Symbol/path search — returns matching nodes |
| `code_intel_changes` | Impact of the current git diff — returns affected symbols |
| `code_intel_run_path` | Command execution path from an entry point |
| `code_intel_graph_callers` | All callers of a named symbol |
| `code_intel_graph_blast_radius` | Transitive blast radius from a named symbol |
| `code_intel_graph_search` | Lexical search across graph node names |
| `code_intel_graph_references` | All non-call references to a named symbol |
| `code_intel_graph_owners` | Ownership info (file, language, kind) for a symbol |

The MCP server reads the index built by `axiom-code-intel index`. Rebuild the index after significant code changes to keep results fresh.

---

## Related Skills

- `code-analysis-axiom` — health scores and linting (`axiom analyze`); separate from graph intelligence
- `hardening-spof-axiom` — uses blast-radius concepts for SPOF detection
- `runtime-completeness-gate-axiom` — uses graph structure to verify wiring
- `decision-archaeology-axiom` — traces decisions through code history

---

## HTTP API & MCP Surface (Graph Harness)

The **Graph Harness** plugin (`specs/102-Graph-Harness.md`) also has an HTTP API surface exposed through `axiom serve`. These routes delegate to `.graph-harness/harness.db` via Python `sqlite3` and are auto-bridged to MCP tools via the OpenAPI spec.

### HTTP Endpoints

| HTTP Method | Path | Description | MCP Tool (auto-generated) |
|---|---|---|---|
| `POST` | `/api/v1/graph` | Create a new graph | `graph_post` |
| `GET` | `/api/v1/graph` | List graphs (filter: status) | `graph_get` |
| `GET` | `/api/v1/graph/templates` | List available templates | `graph_templates_get` |
| `GET` | `/api/v1/graph/{graph_id}` | Get graph status + nodes | `graph_graph_id_get` |
| `POST` | `/api/v1/graph/{graph_id}/inject` | Inject nodes into graph | `graph_graph_id_inject_post` |
| `POST` | `/api/v1/graph/{graph_id}/pause` | Pause graph execution | `graph_graph_id_pause_post` |
| `POST` | `/api/v1/graph/{graph_id}/resume` | Resume paused graph | `graph_graph_id_resume_post` |
| `POST` | `/api/v1/graph/{graph_id}/skip/{node_id}` | Skip a node | `graph_graph_id_skip_node_id_post` |
| `POST` | `/api/v1/graph/{graph_id}/abandon` | Abandon graph | `graph_graph_id_abandon_post` |
| `GET` | `/api/v1/graph/{graph_id}/stream` | SSE stream of node events | `graph_graph_id_stream_get` |

### Example HTTP Usage

```bash
# Start the API server
axiom serve --port 8100 &

# Create a graph
curl -s -X POST http://localhost:8100/api/v1/graph \
  -H 'Content-Type: application/json' \
  -d '{
    "title": "Investigate and fix auth bypass",
    "nodes": [
      {"id": "investigate", "title": "Find root cause", "execution_mode": "agent"},
      {"id": "fix", "title": "Implement fix", "dependencies": ["investigate"], "execution_mode": "agent"}
    ]
  }' | jq .

# Get graph status
curl -s http://localhost:8100/api/v1/graph/{graph_id} | jq .

# Stream events (SSE)
curl -s http://localhost:8100/api/v1/graph/{graph_id}/stream

# List templates
curl -s http://localhost:8100/api/v1/graph/templates | jq .
```

### MCP Auto-Bridge

The HTTP routes are auto-bridged to MCP tools via `uvx awslabs.openapi-mcp-server --openapi openapi.json`. All tool descriptions come from the OpenAPI spec `description` fields (per `specs/73-MCP-Proxy-Onboarding.md`).

**Scopes**: `graph:read` for GET/stream, `graph:write` for POST mutations. Maximum 5 concurrent active graphs per token (REQ-GH-HTTP-013).

**Spec ref**: `specs/102-Graph-Harness.md#18-http-api-surface`, `specs/30-External-API-And-Realtime.md#graph-harness-api`

<!-- axiom:trace work_item=api-feature-parity-01 spec=specs/102-Graph-Harness.md#REQ-GH-HTTP-001 plan=phase-4/task-4-1/step-4-1-1 -->
