---
name: hardening-battery
description: Run the full hardening battery across all 6 categories (SPOF, Security, Database, SRE, Quality, Observability). Produces HARDEN-<category>-<slug> findings with Tier-3+ verifiable acceptance criteria. Supports --report flag to generate Markdown, HTML (teaching tool), or POC reports.
---

# Hardening Battery — Full Audit

This command runs the complete hardening battery. It loads all 6 category skills in sequence and produces a consolidated finding list ready to wire into the Axiom lifecycle.

## Flags

| Flag | Values | Default | What it does |
|---|---|---|---|
| `--report` | — | off | Generate a report after collecting findings |
| `--format` | `markdown`, `html`, `poc`, `all` | `markdown` | Report output format |
| `--output` | file path | stdout | Save report to file |
| `--severity` | `critical`, `high`, `medium`, `low` | all | Filter findings by severity |
| `--category` | `spof`, `security`, `database`, `sre`, `quality`, `observability` | all | Run only specific categories |

## How to Run

Load `hardening-anti-patterns-axiom` first — it provides the shared audit header, finding format, and severity rubric used by all 6 category skills.

Then run each category audit in sequence:

1. Load `hardening-spof-axiom` → run the SPOF audit prompt
2. Load `hardening-security-axiom` → run the Security audit prompt
3. Load `hardening-database-axiom` → run the Database audit prompt
4. Load `hardening-sre-axiom` → run the SRE audit prompt
5. Load `hardening-quality-axiom` → run the Quality audit prompt
6. Load `hardening-observability-axiom` → run the Observability audit prompt

After collecting all findings:
- Deduplicate findings that span multiple categories
- **Generate a report** (see Report Generation below)
- Triage: critical + high findings first
- Load `hardening-intake-axiom` to wire findings into the Axiom lifecycle

## Report Generation

After collecting findings, generate a report to share with the team or use as a teaching tool:

```bash
# Markdown report (default — for PRs, Confluence, evidence bundles)
/hardening-battery --report

# HTML report (teaching tool — shows failure scenarios, how to find/fix each anti-pattern)
/hardening-battery --report --format html --output docs/hardening-report.html

# POC report (developer-facing proof — minimal reproduction for each critical/high finding)
/hardening-battery --report --format poc --severity critical,high

# All three formats
/hardening-battery --report --format all
```

### HTML Report (Teaching Tool)

The HTML report is designed as a teaching tool for engineers who are new to a codebase or unfamiliar with a class of problems. For each finding it includes:

- **Why this is a problem** — concrete failure scenario with blast radius
- **How to find it** — what to grep/search for in any codebase
- **How to fix it** — the pattern to apply with a code snippet
- **How to verify it's fixed** — the Tier 3+ test to run (copy-pasteable)
- **Color-coded severity** — red/orange/yellow/green badges
- **Progress tracker** — checkboxes that persist in localStorage

No external dependencies. Works offline. Safe to commit to the repo.

### POC Report (Proof of Concept)

For skeptical engineers who need to see the failure before they'll fix it. For each critical/high finding:

1. **Current code** — the problematic pattern as it exists
2. **Reproduction steps** — how to trigger the failure in under 5 minutes
3. **Fixed code** — the corrected pattern
4. **Verification** — the Tier 3+ test that proves the fix works

## Rules

- All security findings MUST have `requires_human_review: true`
- All migration findings MUST have `requires_human_review: true`
- Every finding MUST have `acceptance_criteria` with Tier 3+ verifiable conditions
- Run one category at a time — do not combine into a single mega-prompt
- POC reports MUST NOT include exploit code — only reproduction + remediation

## Output Format

Each finding uses the standard format from `hardening-anti-patterns-axiom`:
```yaml
id: HARDEN-<CATEGORY>-<slug>
severity: critical | high | medium | low
category: spof | security | database | sre | quality | observability
location: "path/to/file.py:line"
description: "..."
impact: "..."
recommendation: "..."
acceptance_criteria:
  - "Tier 3+ verifiable condition"
verification_tier: 3
confidence: confirmed | suspected
requires_human_review: true | false
```

axiom:trace work_item=hardening-skills-01 jira_ref=SWDE-7
