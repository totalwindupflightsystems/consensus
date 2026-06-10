---
name: pattern-explore-codebase
description: >-
  Explore a symbol, file, or module: find its definition, understand its
  callers/callees, and store findings in tree memory for other agents.
  Uses code-intel CLI + tree.commit + stash. Runtime-tested 2026-05-18.
version: "1.0"
tags:
  vertical: [exploration, analysis, workflow]
  category: pattern
  core: false
spec: specs/121-Pattern-Generator.md
trigger_conditions:
  - "Agent needs to understand how a symbol is used"
  - "Agent is asked 'how does X work?' or 'who calls Y?'"
  - "Before refactoring a function — assess blast radius first"
  - "Onboarding to an unfamiliar module"
tools_required:
  - bash (code-intel CLI at _tmp/axiom-code-intel)
  - tree_branch
  - tree_commit
  - tree_merge
  - stash_push
estimated_steps: 5
estimated_duration: "15-30 seconds"
lifecycle:
  state: active
  created: "2026-05-18"
  last_validated: "2026-05-18"
  validation_count: 1
---

# Pattern: Explore Codebase

Understand a symbol or module: find where it's defined, who calls it, what it
calls, and store a structured finding in tree memory so other agents can use it.

**Spec**: `specs/121-Pattern-Generator.md`
**Observed from**: 1 real execution on 2026-05-18 (target: `ShellOpsPlugin`)

<!-- axiom:trace work_item=pattern-design-01 spec=specs/121-Pattern-Generator.md -->

---

## Prerequisites

| Requirement | How to Verify | Expected | If Missing |
|-------------|---------------|----------|------------|
| code-intel binary | `ls _tmp/axiom-code-intel` | file exists, executable | Build: `cd code-intel && go build -o ../_tmp/axiom-code-intel ./cmd/axiom-code-intel/` |
| Pre-built index | `ls _tmp/code-intel.idx` | file exists | Run: `_tmp/axiom-code-intel index --repo . --out _tmp/code-intel.idx` |
| Tree Memory initialized | Call `tree_status` | `initialized: true` | Call `tree_init` |
| Git repo accessible | `git log --oneline -1` | returns a commit | Pattern requires a git repo |

> ⚠️ **code-intel MCP tool** (`operation: query`) currently fails with "index command failed". Always use the **CLI directly** via `bash`:
> ```bash
> _tmp/axiom-code-intel search --repo . --query "SymbolName"
> _tmp/axiom-code-intel callers --repo . --symbol "qualified.Name"
> _tmp/axiom-code-intel detect-changes --repo . --base HEAD~5
> ```

---

## Tool Chain

| Step | Purpose | Tool | Key Input | Key Output | On Failure | Criticality |
|------|---------|------|-----------|------------|------------|-------------|
| 1 | Verify tree initialized | `tree_status` | — | `{initialized: true}` | Call `tree_init` then retry | Required |
| 2 | Find symbol definition | `bash` (`axiom-code-intel search`) | `--query "SymbolName"` | `{results: [{name, file, line, qualified_name}]}` | Try broader query; proceed with file-only analysis | Required |
| 3 | Find callers | `bash` (`axiom-code-intel callers`) | `--symbol "qualified.Name"` | `{results: [{...}]}` | Note "callers unknown"; proceed | Enriching |
| 4 | Store findings | `tree_branch` + `tree_commit` | `{file, content, message}` | `{status: "committed"}` | Use `stash_push` as fallback | Required |
| 5 | Handoff context | `stash_push` | `{summary, tags}` | `{stash_id}` | WARN; findings still in tree | Optional |

---

## Pseudocode

```text
PATTERN explore_codebase(symbol_name, repo_path?):

  // Step 1: Verify tree
  status = CALL tree_status()
  IF NOT status.initialized:
    CALL tree_init()

  // Step 2: Find definition
  result = RUN "_tmp/axiom-code-intel search --repo . --query {symbol_name}"
  IF result.results.length == 0:
    WARN "Symbol not found — try a broader query or check spelling"
    qualified_name = symbol_name  // fall back to raw name
  ELSE:
    hit = result.results[0]
    qualified_name = hit.node.qualified_name
    file = hit.node.file
    line = hit.node.line

  // Step 3: Find callers (enriching)
  callers_result = RUN "_tmp/axiom-code-intel callers --repo . --symbol {qualified_name}"
  callers_count = callers_result.results.length

  // Step 4: Store in tree
  branch = "explore-{slug(symbol_name)}-{date()}"
  CALL tree_branch(action: "create", name: branch)
  finding = {
    symbol: symbol_name,
    qualified_name: qualified_name,
    file: file,
    line: line,
    callers_count: callers_count,
    risk: "HIGH" if callers_count > 5 else "MEDIUM" if callers_count > 0 else "LOW",
    note: "0 callers = either new code, dynamic dispatch, or config-loaded (check loader)"
  }
  CALL tree_commit(
    file: "findings/explore-{symbol_name}-{date()}.json",
    content: JSON.stringify(finding),
    message: "explore: {symbol_name} [{callers_count} callers]"
  )

  // Step 5: Handoff
  stash_result = CALL stash_push(
    summary: "Explored {symbol_name}: {file}:{line}, {callers_count} callers, {finding.risk} risk",
    tags: "exploration,{finding.risk.toLowerCase()}"
  )

  RETURN {
    status: "PATTERN_COMPLETE",
    symbol: symbol_name,
    file: file,
    line: line,
    callers_count: callers_count,
    risk: finding.risk,
    stash_id: stash_result.stash_id
  }
```

---

## On-Track / Off-Track Signals

| Signal | Type | After Step | Indicator | Response |
|--------|------|-----------|-----------|----------|
| SIG-01 | on_track | 1 | `tree_status` returns `initialized: true` | Continue |
| SIG-02 | off_track | 1 | `tree_status` returns error | Call `tree_init` then retry |
| SIG-03 | on_track | 2 | search returns `results.length ≥ 1` | Use `results[0]` |
| SIG-04 | off_track | 2 | search returns `results.length == 0` | Broaden query; try substring |
| SIG-05 | on_track | 3 | callers returns result (even `results: []`) | `results: []` = new/config-loaded — still valid |
| SIG-06 | off_track | 3 | callers hangs >10s | Kill and note "callers: timeout" |
| SIG-07 | on_track | 4 | `tree_commit` returns `status: "committed"` | Continue |
| SIG-08 | off_track | 4 | `tree_commit` fails | Retry once; fall back to `stash_push` |
| SIG-09 | on_track | 5 | `stash_push` returns `stash_id` | PATTERN_COMPLETE |
| SIG-10 | off_track | 5 | `stash_push` returns error | WARN only; findings in tree |

---

## Adjustment Protocol

```
callers = 0 but symbol exists:
  → Normal for: plugin factories loaded via config (like ShellOpsPlugin)
  → Normal for: test entry points, main() functions
  → Normal for: methods on types only used as interfaces
  → Document the reason in the finding.note field

code-intel MCP tool fails with "index command failed":
  → ALWAYS use bash + CLI directly (SIG-KNOWN-01)
  → CLI: _tmp/axiom-code-intel search --repo . --query "Name"
  → Do NOT call code-intel() MCP tool with operation: "query"

search returns no results for known symbol:
  → Try callers: --symbol "package.SymbolName" (fully qualified)
  → Try search --query with substring of the name
  → Check if the index is stale (re-run index command)
```

---

## Example Execution Trace (Observation #1: ShellOpsPlugin)

```
─── Pattern Instance: explore-codebase ───
Input: { symbol_name: "ShellOpsPlugin" }

[Step 1] tree_status
→ tree_status()
← { initialized: true, branches: 2, current_branch: "qa-agent-sweep" }
✓ SIG-01: initialized

[Step 2] search
→ bash: _tmp/axiom-code-intel search --repo . --query "ShellOpsPlugin"
← { results: [{ node: { qualified_name: "shellops.ShellOpsPlugin", file: ".opencode/lib/shellops.ts", line: 612 } }] }
✓ SIG-03: 1 result

[Step 3] callers
→ bash: _tmp/axiom-code-intel callers --repo . --symbol "shellops.ShellOpsPlugin"
← { results: [] }
✓ SIG-05: 0 callers — expected, plugin loaded via opencode.jsonc config (dynamic dispatch)

[Step 4] tree_commit
→ tree_branch(action: "create", name: "explore-shellops-2026-05-18")
← { action: "created", branch: "explore-shellops-2026-05-18" }
→ tree_commit({ file: "findings/explore-shellops-2026-05-18.json", content: "{...}", message: "explore: ShellOpsPlugin [0 callers]" })
← { status: "committed", file: "findings/explore-shellops-2026-05-18.json" }
✓ SIG-07

[Step 5] stash_push
→ stash_push({ summary: "Explored ShellOpsPlugin: .opencode/lib/shellops.ts:612, 0 callers, LOW risk", tags: "exploration,low" })
← { stash_id: "pattern-explore-codebase-trial-run", state: "suspended" }
✓ SIG-09

RESULT: PATTERN_COMPLETE
  symbol: ShellOpsPlugin
  file: .opencode/lib/shellops.ts:612
  callers_count: 0
  risk: LOW
  note: 0 callers = plugin loaded dynamically via opencode.jsonc config
```

---

## When NOT to Use This Pattern

- **You already know the file and just need to read it**: use `Read` directly — no need for code-intel
- **More than 10 changed files**: use `pattern-investigate-changes` instead (it handles multi-file impact analysis)
- **You need runtime behavior, not structural analysis**: explore finds call graph edges, not runtime paths
