---
name: code-analysis-axiom
description: >-
  Multi-language code analysis using axiom analyze. Covers CLI usage,
  HTTP API, MCP tools, scoring model, adapter inventory, output formats,
  configuration, and integration patterns. Load this skill when running
  code analysis, interpreting health scores, integrating analysis into
  CI/CD, or when any agent needs to assess code quality for a repo.
version: "1.0"
tags:
  vertical: [coding]
  category: development
  core: false
---

# Code Analysis (axiom analyze)

`axiom analyze` is a multi-language code analysis tool that wraps 23
battle-tested per-language analyzers behind a unified interface. It produces
a single health score (0–100, A–F letter grade) with issues normalized into
a common schema across all languages.

**Spec**: `specs/81-Axiom-Analyze-Multi-Language-Code-Analysis.md`

## When to Load This Skill

Load this skill when:
- Running code analysis on any repo (`axiom analyze`)
- Interpreting health scores or letter grades
- Integrating analysis into CI/CD pipelines
- An agent needs to assess code quality before claiming work is done
- Reviewing analysis results in PR comments
- Configuring analysis thresholds or tool weights
- Debugging why a tool isn't finding issues or scoring seems wrong

## Quick Start

```bash
# Analyze current directory with score
axiom analyze --score

# Analyze a specific path, Python only
axiom analyze --path /path/to/repo --language python --score

# JSON output for programmatic use
axiom analyze --format json --score

# Fast mode (ruff + shellcheck + hadolint only)
axiom analyze --fast --score

# Dead code only
axiom analyze --dead-code

# Complexity only
axiom analyze --complexity

# Audit mode — verdict for changed files (pass/warn/fail, exit 0/1/2)
axiom analyze --audit --changed-since main

# SARIF output for GitHub Code Scanning
axiom analyze --format sarif > results.sarif

# Markdown output for PR comments
axiom analyze --format markdown --score
```

## Supported Languages and Tools (23 adapters)

| Language | Tools | Categories |
|----------|-------|-----------|
| **Python** | vulture, radon, deptry, ruff, deadcode | dead-code, complexity, unused-deps, lint |
| **Go** | gocyclo, gocognit, golangci-lint, deadcode | complexity, lint, dead-code |
| **TypeScript/JS** | biome, eslint, fallow | lint, dead-code, complexity, duplication |
| **Rust** | clippy, cargo-machete, rust-code-analysis | lint, unused-deps, complexity |
| **Shell** | shellcheck | lint |
| **Dockerfile** | hadolint | lint |
| **Terraform** | tflint, trivy-config | lint, security |
| **Helm** | helm lint | lint |
| **YAML** | yamllint, kubeconform | lint |
| **Jupyter** | nbqa-ruff (ruff only, no vulture — DA-005) | lint |
| **Generic** | jscpd (all languages) | duplication |

### Tool detection

Tools are auto-detected via `shutil.which()`. Missing tools produce a
warning but don't fail the analysis — other tools still run. Use
`--strict-tools` to fail if any required tool is missing (for CI).

### Biome and ESLint

Biome always runs on TS/JS files — it works without project configuration.
ESLint additionally runs when `eslint.config.js` is present (ESLint v9+
requires this file). Both results are included in the output. For projects
without ESLint configured, biome is the only TS/JS linter that runs.

### Vulture false positives

Vulture uses static analysis and cannot detect runtime usage by frameworks
(FastAPI route handlers, pytest fixtures, Django views). We use
`--min-confidence 80` and `--exclude tests,docs_src` to reduce false
positives. Users can further reduce FPs with `.vulture_whitelist.py`.

### Shellcheck shebang filter

Only `.sh`/`.bash`/`.zsh` files with a shebang (`#!`) on the first line
are analyzed. Data files with `.sh` extension are skipped.

### jscpd defaults

jscpd runs with `--min-lines 10` and ignores `docs/`, `examples/`,
`fixtures/`, `vendor/`, `node_modules/` by default to reduce noise
from intentional duplication.

## Health Score Model

The score uses a **remediation-ratio model** inspired by SonarQube SQALE:

```
score = f(remediation_effort / development_effort)
```

### How it works

1. Each issue has an estimated **remediation cost** in minutes
   (see `scorer.py` `calculate_score()` for exact formulas):
   - Dead code: 5 min
   - High complexity (cc 15–24): 10 min base + 1 min per cc point above 15
   - Very high complexity (cc 25+): 20 min base + 1 min per cc point above 25
   - Unused deps: 5 min
   - Lint error: 5 min
   - Lint warning: 2 min
   - Duplication: 5 min
   - Security: 60 min

   > **Note**: These values are the actual formulas in `calculate_score()`.
   > The `DEFAULT_REMEDIATION_MINUTES` dict in `scorer.py` contains different
   > values for complexity (30/60) that are NOT used for complexity scoring —
   > the function uses the scaled formulas above instead. This is a known
   > inconsistency tracked for cleanup.

2. **Development cost** is estimated at 120 min per source file.

3. The **ratio** maps to a score via piecewise linear interpolation:
   - 0% → 100, 5% → 90 (A), 10% → 80 (B), 20% → 70 (C), 50% → 60 (D), 100% → 0 (F)

### Key scoring rules

- **Complexity below cc=15 does NOT affect the score.** These issues are
  reported for visibility but are informational only.
- **Complexity cc=15–24** affects the score (high complexity).
- **Complexity cc=25+** affects the score more heavily (very high complexity).
- Scores scale naturally with codebase size — same issues in a larger
  project produce a higher score.

### Grade thresholds

| Grade | Score range | Meaning |
|-------|-----------|---------|
| A | 90–100 | Excellent — minimal remediation needed |
| B | 80–89 | Good — some issues but well-maintained |
| C | 70–79 | Fair — noticeable technical debt |
| D | 60–69 | Poor — significant remediation needed |
| F | <60 | Failing — major quality issues |

### Real-world calibration

Validated against 6 open-source repos:

| Repo | Score | Grade | Notes |
|------|-------|-------|-------|
| go-chi/chi | 85 | B | Well-maintained Go router |
| encode/httpx | 84 | B | Clean Python HTTP client |
| BurntSushi/ripgrep | 89 | B | Very clean Rust CLI |
| fastapi/fastapi | 80 | B | Large framework, well-structured |
| terragrunt example | 100 | A | No issues found |

## Output Formats

### JSON (default for programmatic use)

```bash
axiom analyze --format json --score
```

Top-level fields: `version`, `health_score`, `health_grade`, `issues[]`,
`tools_status[]`, `languages_detected`, `languages_analyzed`,
`by_language`, `by_file`, `duration_ms`, `duration_seconds`,
`total_files_scanned`, `partial`, `score_unavailable`, `timestamp`, `path`.

Each issue: `file`, `line`, `column`, `message`, `severity` (error/warn/info),
`category` (dead-code/complexity/lint/duplication/unused-deps/security),
`tool`, `language`, `rule`, `confidence`, `extra`.

### Terminal (default for humans)

Colored output with issue list, tool status, and score badge.

### SARIF 2.1.0 (for GitHub Code Scanning)

```bash
axiom analyze --format sarif > results.sarif
```

Upload to GitHub: `gh api repos/{owner}/{repo}/code-scanning/sarifs -f sarif=@results.sarif`

### Markdown (for PR comments)

```bash
axiom analyze --format markdown --score
```

Produces a PR-ready comment with score badge, issue summary table, and
top issues list.

## HTTP API

### Synchronous (Phase 2 — current)

```bash
# Run analysis
curl -X POST http://localhost:8200/api/v1/analyze \
  -H "Content-Type: application/json" \
  -d '{"repo": "/path/to/repo", "languages": ["python", "go"]}'

# Health check
curl http://localhost:8200/api/v1/analyze/health

# History
curl http://localhost:8200/api/v1/analyze/history
```

POST returns 200 with full results (max 120s timeout).

## MCP Tools

Three semantic MCP tools (not auto-generated from OpenAPI):

| Tool | What it does |
|------|-------------|
| `check_code_health` | Run full analysis, return score + issues |
| `find_dead_code` | Run dead-code-only analysis |
| `get_analysis_history` | Get recent analysis results |

## Configuration

In `.axiom/axiom.config.yaml`:

```yaml
analyze:
  exclude:
    - "vendor/"
    - "node_modules/"
    - "*.generated.go"
  health_score:
    weights:
      dead_code: 1.0        # multiplier on remediation cost
      complexity: 1.0
      complexity_very_high: 1.0
      unused_deps: 1.0
      lint: 1.0
      duplication: 1.0
      security: 1.0
    thresholds:
      a: 90
      b: 80
      c: 70
      d: 60
```

Weights are **positive multipliers** on remediation cost. A weight of 2.0
means "treat this category as twice as costly to fix."

## Integration Patterns

### Agent verification

When an agent claims a work item step is done, run analysis to check
for new issues:

```bash
# Run analysis and check score
axiom analyze --format json --score > analysis.json

# Save baseline before changes, compare after
axiom analyze --save-baseline baseline.json --format json > /dev/null
# ... make changes ...
axiom analyze --baseline baseline.json --fail-on-regression --format json --score

# Analyze only files changed since a branch
axiom analyze --changed-since main --format json --score
```

### CI pipeline

A complete, working GitHub Actions workflow is provided at
`.github/workflows/analyze.yml`. It demonstrates the full CI pattern:
checkout with full history, baseline save on the base branch, changed-files
analysis with regression detection, SARIF upload to GitHub Code Scanning,
and a markdown PR comment.

Key flags used together in CI:

```bash
# 1. Save baseline from the base branch (run once before changes)
axiom analyze --save-baseline /tmp/baseline.json --format json > /dev/null

# 2. Analyze only changed files, compare to baseline, exit non-zero on regression
axiom analyze \
  --ci \
  --changed-since main \
  --baseline /tmp/baseline.json \
  --fail-on-regression \
  --format sarif > results.sarif

# 3. Post markdown summary as PR comment
axiom analyze \
  --changed-since main \
  --baseline /tmp/baseline.json \
  --format markdown > pr-comment.md
gh pr comment $PR_NUMBER --body-file pr-comment.md
```

Minimal inline snippet for existing workflows:

```yaml
# GitHub Actions snippet
- name: Code Analysis
  run: |
    axiom analyze --format sarif --score > results.sarif
    axiom analyze --format json --score > analysis.json
- name: Upload SARIF
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: results.sarif
```

### PR comments

```bash
axiom analyze --format markdown --score > pr-comment.md
gh pr comment $PR_NUMBER --body-file pr-comment.md
```

## Audit Mode

`--audit` produces a single verdict for changed files — designed for PR gates and CI checks.

### Usage

```bash
# Audit changed files since main (auto-detects default branch if omitted)
axiom analyze --audit --changed-since main

# Audit + markdown summary for PR comment
axiom analyze --audit --changed-since main --format markdown
```

### Verdict logic

| Verdict | Condition | Exit code |
|---------|-----------|-----------|
| `pass` | No new issues, or issue count decreased | 0 |
| `warn` | New issues introduced, but score still ≥ 70 | 1 |
| `fail` | New issues introduced AND score dropped below 70 | 2 |

### Exit codes in CI

```yaml
# GitHub Actions — fail PR if audit verdict is "fail"
- name: Audit changed files
  run: axiom analyze --audit --changed-since main
  # exits 0 (pass), 1 (warn — non-blocking), or 2 (fail — blocks merge)
```

Use `|| true` to treat `warn` as non-blocking while still failing on `fail`:

```bash
axiom analyze --audit --changed-since main
EXIT=$?
if [ $EXIT -eq 2 ]; then exit 2; fi   # fail → block
exit 0                                  # pass or warn → allow
```

### Spec ref

`specs/81-Axiom-Analyze-Multi-Language-Code-Analysis.md#REQ-ANALYZE-018`

## Dual-Branch Comparison

Compare two git branches to produce a **merge-readiness verdict** before merging.

axiom:trace work_item=dual-branch-analysis-01 spec=specs/81-Axiom-Analyze-Multi-Language-Code-Analysis.md#12D plan=phase-4/task-4-1/step-4-1-1

### Usage

```bash
axiom analyze --compare-branch <ref> [--language LANG] [--path PATH] [--format json|terminal|markdown]

# Example: Compare current branch against main
axiom analyze --compare-branch main --format json

# Scoped to Python files in src/
axiom analyze --compare-branch main --language python --path src/ --format json
```

### Merge-Readiness Workflow

Before merging a feature branch, run:
```bash
axiom analyze --compare-branch main --format json
```

The output includes a `comparison` key with:
- `verdict`: one of `safe` | `caution` | `risky` | `blocked`
- `score_current`: health score for current branch (0-100)
- `score_compare`: health score for compare ref (0-100)
- `score_delta`: positive = improvement, negative = regression
- `new_issues`: issues introduced in current branch (by fingerprint)
- `fixed_issues`: issues resolved in current branch (by fingerprint)

### Verdict Interpretation

| Verdict | Condition | Recommended Action |
|---------|-----------|-------------------|
| `safe` | No regressions; score improved or stable | Proceed with merge |
| `caution` | New warnings or small score drop (< 5pts) | Review new warnings before merging |
| `risky` | New errors or score drop ≥ 5pts | Fix issues before merging |
| `blocked` | New security issue detected (sql-injection, hardcoded-secret, etc.) | Must fix before merging |

Verdict is evaluated in priority order: `blocked > risky > caution > safe` (first match wins).

### HTTP API

```bash
# POST /api/v1/analyze/compare — async (202 + SSE stream)
curl -s -X POST http://localhost:8100/api/v1/analyze/compare \
  -H "Content-Type: application/json" \
  -d '{"repo": ".", "base_ref": "main", "compare_ref": "HEAD"}' | jq -e '.analysis_id'
# Returns: {"analysis_id": "...", "stream_url": "/api/v1/analyze/<id>/stream", "status": "running"}
```

### Composability (REQ-ANALYZE-055)

Both `--language` and `--path` flags are applied to BOTH analysis runs:
```bash
axiom analyze --compare-branch main --language python --format json
axiom analyze --compare-branch main --path src/ --fast
```

### Spec ref

`specs/81-Axiom-Analyze-Multi-Language-Code-Analysis.md#12D.1 (REQ-ANALYZE-051–058)`

## Troubleshooting

### "Score seems too low"

- Check if yamllint is producing thousands of `document-start` and
  `line-length` warnings. Add a `.yamllint` config to relax rules.
- Check if jscpd is flagging documentation directories. The default
  ignores cover `docs/`, `examples/`, `fixtures/` but your project
  may have other directories with intentional duplication.
- Complexity below cc=15 doesn't affect the score. If you see many
  info-severity complexity issues, they're informational only.

### "Tool X found 0 issues"

- Check `tools_status` in JSON output — the tool may be `missing`
  (not installed) or `error` (crashed).
- ESLint requires `eslint.config.js` for v9+. Use biome instead.
- Biome needs explicit file discovery — if it found 0 issues, check
  that TS/JS files exist in the scanned path.

### "Vulture found false positives"

- Framework parameters (FastAPI, pytest fixtures) are inherently
  false positives for static analysis. Create `.vulture_whitelist.py`.
- We exclude `tests/` and `docs_src/` by default.

## Architecture

```
CLI / HTTP API / MCP
       ↓
  Language Detector (extensions + config files)
       ↓
  Async Runner (asyncio.gather + Semaphore(4))
       ↓
  23 Tool Adapters (ToolAdapter ABC)
  Each: detect() → run() → parse_output() → normalize()
       ↓
  Deduplication → Path Normalization → Secret Scrubbing
       ↓
  Health Scorer (remediation-ratio model)
       ↓
  Formatter (JSON / Terminal / SARIF / Markdown)
```

### Key source files

| File | What it does |
|------|-------------|
| `analyze/__init__.py` | Public API: `run_analysis()` |
| `analyze/runner.py` | Async runner, path normalization, config wiring |
| `analyze/normalizer.py` | `Issue` model, `AnalysisResult`, dedup, `scrub_secrets()` |
| `analyze/scorer.py` | Remediation-ratio health score |
| `analyze/detector.py` | Language detection |
| `analyze/adapters/` | All 23 tool adapters |
| `analyze/formatters/` | JSON, terminal, SARIF, markdown |
| `cli/analyze.py` | CLI subcommand handler |
| `control_plane/api/routes_analyze.py` | HTTP API routes |
| `mcp_server/server.py` | MCP tools (tools 17–19) |

### Regression baseline

`tests/fixtures/analyze/regression-baseline/` contains intentionally
bad code across 8 languages with `expected-findings.json` manifest.
`test_analyze_regression_baseline.py` validates all 22 expected findings
are detected. If any finding disappears after a code change, the change
broke something.

axiom:trace work_item=analyze-01 spec=specs/81-Axiom-Analyze-Multi-Language-Code-Analysis.md plan=phase-3/task-3-2/step-3-2-3 jira_ref=DEX-386
