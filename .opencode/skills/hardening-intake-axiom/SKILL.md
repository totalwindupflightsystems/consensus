---
name: hardening-intake-axiom
description: >
  How to run the hardening battery and wire findings into the Axiom lifecycle.
  Covers Path A (direct Claude audit) and Path B (Axiom work items), Jira hierarchy
  (Initiative → Epic → Task → Subtask), confidence bands for auto-resolution vs human
  review, and the quarterly cadence. Load this skill last — after loading the 6 category
  skills and collecting findings.
version: "1.0"
tags:
  vertical: [onboarding, planning]
  category: hardening
  core: false
metadata:
  related_skills:
    - hardening-anti-patterns-axiom
    - hardening-spof-axiom
    - hardening-security-axiom
    - hardening-database-axiom
    - hardening-sre-axiom
    - hardening-quality-axiom
    - hardening-observability-axiom
---

# Hardening: Intake & Axiom Lifecycle Integration

> **"The hardening battery becomes a recurring Axiom-driven process — run quarterly, decompose findings, let the system work through them in parallel. You end up with a continuous hardening flywheel instead of a one-off project."**

This skill explains how to run the hardening battery and wire findings into the Axiom lifecycle. Load it after collecting findings from the 6 category skills.

## When to Load This Skill

- Starting a hardening audit for the first time
- Wiring findings into Jira as work items
- Setting up a quarterly hardening cadence
- After collecting findings from category skills

---

## The Two Delivery Paths

### Path A: Direct Claude Audit (Available Today)

Run the battery manually, one category at a time:

1. **Scope the repo** — don't paste 500k LOC. Scope to one service or module.
2. **Run all 6 prompts** with the shared header, one at a time:
   - Load `hardening-anti-patterns-axiom` for the shared header
   - Run SPOF prompt → collect findings
   - Run Security prompt → collect findings
   - Run Database prompt → collect findings
   - Run SRE prompt → collect findings
   - Run Quality prompt → collect findings
   - Run Observability prompt → collect findings
3. **Collate findings** — dedupe across categories (some findings span multiple)
4. **Generate a report** — see [Report Generation](#report-generation) below
5. **Triage** — critical + high in a team meeting; medium + low in backlog
6. **Re-run quarterly** or after major architecture changes

**Path A is the right starting point.** Run it before Path B to understand the finding volume and quality.

---

## Report Generation

After collecting findings, generate a report to share with the team, use as a teaching tool, or track progress over time. Three output formats are supported.

### Format 1: Markdown Report (default)

Best for: GitHub PRs, Confluence pages, Notion, memory bank evidence bundles.

```markdown
# Hardening Audit Report — <repo-name>
**Date**: 2026-04-17
**Scope**: <service or module>
**Categories run**: SPOF, Security, Database, SRE, Quality, Observability

## Executive Summary
- **Total findings**: N
- **Critical**: N | **High**: N | **Medium**: N | **Low**: N
- **Requires human review**: N (security + migrations)
- **Estimated remediation effort**: N days

## Findings by Severity

### 🔴 Critical (fix this week)
| ID | Category | Location | Description | AC |
|---|---|---|---|---|
| HARDEN-SRE-NO-TIMEOUT | SRE | `api/client.py:42` | No timeout on external HTTP call | Timeout fires within 5s under delay injection |

### 🟠 High (fix this sprint)
...

### 🟡 Medium (fix this quarter)
...

### 🟢 Low (fix when nearby)
...

## Anti-Patterns Found
List of recurring anti-patterns detected across findings (from hardening-anti-patterns-axiom).

## Teaching Notes
For each critical/high finding, explain:
- **Why this is a problem**: concrete failure scenario
- **How to find it**: what to grep/search for in any codebase
- **How to fix it**: the pattern to apply
- **How to verify it's fixed**: the Tier 3+ test to run

## Next Steps
- [ ] Triage critical + high with team
- [ ] Create Jira tickets (Path B) or assign owners (Path A)
- [ ] Schedule re-run in 90 days
```

### Format 2: HTML Report (teaching tool)

Best for: sharing with teams who don't use markdown, onboarding new engineers, executive summaries.

Generate with:
```
/hardening-battery --format html --output hardening-report.html
```

The HTML report includes:
- **Color-coded severity badges** (red/orange/yellow/green)
- **Expandable finding cards** — click to see full description, code snippet, and fix
- **Anti-pattern gallery** — visual catalog of what each anti-pattern looks like in code
- **Teaching sections** — "Why this matters", "How to find it", "How to fix it" for each finding
- **Progress tracker** — checkboxes for each finding, persists in localStorage
- **Copy-to-clipboard** for each finding's acceptance criteria (paste into Jira)

HTML template structure:
```html
<!DOCTYPE html>
<html>
<head>
  <title>Hardening Audit — {repo}</title>
  <!-- Minimal CSS, no external dependencies -->
</head>
<body>
  <header>
    <h1>🔒 Hardening Audit Report</h1>
    <div class="summary-badges">
      <span class="badge critical">{n} Critical</span>
      <span class="badge high">{n} High</span>
      <span class="badge medium">{n} Medium</span>
      <span class="badge low">{n} Low</span>
    </div>
  </header>

  <!-- Executive summary -->
  <!-- Findings by severity (expandable cards) -->
  <!-- Anti-pattern gallery -->
  <!-- Teaching notes per finding -->
  <!-- Remediation checklist -->
</body>
</html>
```

### Format 3: POC/Proof-of-Concept Report (developer-facing)

Best for: convincing skeptical engineers that a finding is real, not theoretical.

For each critical/high finding, generate a **self-contained runnable script** that demonstrates the problem and the fix. The script is a teaching tool — it shows the failure, explains why it happens, applies the fix, and verifies the fix works. It cannot be repurposed as an attack tool because it only operates on localhost with synthetic data.

#### POC Script Structure

Each POC is a single runnable file (Python, Bash, or Node.js depending on the stack) with this structure:

```python
#!/usr/bin/env python3
"""
POC: HARDEN-SRE-NO-TIMEOUT
Finding: External HTTP call with no timeout
Repo: <repo-name>
Location: api/client.py:42

TEACHING GOAL: Show what happens when an external call has no timeout,
then show how adding a timeout fixes it.

SAFE TO RUN: This script only makes calls to localhost. No external
network access. No data is written to disk. No credentials required.
Run time: ~15 seconds.
"""

import time
import threading
import http.server
import requests

# ─────────────────────────────────────────────
# STEP 1: Start a slow server (simulates a hanging dependency)
# ─────────────────────────────────────────────

class SlowHandler(http.server.BaseHTTPRequestHandler):
    """Simulates an external service that hangs for 30 seconds."""
    def do_GET(self):
        print("  [slow-server] Request received — sleeping 30s (simulating hang)...")
        time.sleep(30)
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"finally!")
    def log_message(self, *args):
        pass  # suppress default logging

server = http.server.HTTPServer(("localhost", 18888), SlowHandler)
thread = threading.Thread(target=server.serve_forever)
thread.daemon = True
thread.start()
print("✓ Slow server started on localhost:18888")

# ─────────────────────────────────────────────
# STEP 2: Demonstrate the PROBLEM (no timeout)
# ─────────────────────────────────────────────

print("\n━━━ PROBLEM: Call with no timeout ━━━")
print("Making request to slow server (no timeout set)...")
print("This will hang for 30 seconds — Ctrl+C to skip ahead\n")

start = time.time()
try:
    # THIS IS THE ANTI-PATTERN — no timeout
    response = requests.get("http://localhost:18888/data")
    elapsed = time.time() - start
    print(f"  Response received after {elapsed:.1f}s")
except KeyboardInterrupt:
    elapsed = time.time() - start
    print(f"\n  ✗ You interrupted after {elapsed:.1f}s")
    print("  In production: all threads would be blocked like this.")
    print("  Under load: service becomes completely unresponsive.")

# ─────────────────────────────────────────────
# STEP 3: Demonstrate the FIX (with timeout)
# ─────────────────────────────────────────────

print("\n━━━ FIX: Call with 5-second timeout ━━━")
print("Making same request with timeout=5.0...")

start = time.time()
try:
    # THIS IS THE FIX — add timeout
    response = requests.get("http://localhost:18888/data", timeout=5.0)
except requests.Timeout:
    elapsed = time.time() - start
    print(f"  ✓ Timeout fired after {elapsed:.1f}s (expected ~5s)")
    print("  In production: caller gets an error quickly.")
    print("  Under load: other requests are not blocked.")

# ─────────────────────────────────────────────
# STEP 4: Show the fix in context
# ─────────────────────────────────────────────

print("\n━━━ THE FIX IN YOUR CODE ━━━")
print("""
  # BEFORE (api/client.py:42) — ANTI-PATTERN
  response = requests.get(url)

  # AFTER — FIXED
  response = requests.get(url, timeout=5.0)

  # EVEN BETTER — configurable timeout with fallback
  TIMEOUT = float(os.getenv("EXTERNAL_API_TIMEOUT", "5.0"))
  response = requests.get(url, timeout=TIMEOUT)
""")

# ─────────────────────────────────────────────
# STEP 5: Acceptance criteria verification
# ─────────────────────────────────────────────

print("━━━ VERIFICATION (Tier 3) ━━━")
print("Running acceptance criteria check...")

start = time.time()
try:
    requests.get("http://localhost:18888/data", timeout=5.0)
    print("  ✗ FAIL: Request did not timeout (unexpected)")
except requests.Timeout:
    elapsed = time.time() - start
    if elapsed < 6.0:
        print(f"  ✓ PASS: Timeout fired in {elapsed:.1f}s (within 5s budget)")
    else:
        print(f"  ✗ FAIL: Timeout took {elapsed:.1f}s (exceeded 5s budget)")

server.shutdown()
print("\n✓ POC complete. Slow server stopped.")
print("\nACCEPTANCE CRITERIA MET:")
print("  External call to slow dependency times out within 5 seconds")
print("  and raises requests.Timeout (not hanging indefinitely).")
```

#### POC Script Rules (non-negotiable)

1. **Localhost only** — all network calls go to `localhost` or `127.0.0.1`. Never to external services.
2. **Synthetic data only** — no real credentials, no real user data, no real API keys.
3. **Self-contained** — the script starts its own mock server/service. No external setup required.
4. **Teaches, doesn't attack** — the script demonstrates the failure mode, not how to exploit it in another system.
5. **Cleans up after itself** — stops any servers it started, removes any temp files.
6. **Runs in under 2 minutes** — engineers won't run it if it takes too long.
7. **Explains every step** — comments explain what's happening and why, not just what.
8. **Shows before AND after** — always demonstrates the broken state first, then the fixed state.

#### POC Script Templates by Category

| Category | What the script demonstrates | Mock infrastructure |
|---|---|---|
| **SPOF** | Service crashes when dependency is unavailable | Mock dependency that returns 503 |
| **Security** | Input reaches a sensitive sink without validation | In-memory SQL-like processor (no real DB) |
| **Database** | N+1 query pattern vs batched query | In-memory list simulating DB rows |
| **SRE** | No timeout / no retry / no circuit breaker | Slow/flaky localhost server |
| **Quality** | Test that always passes vs test that catches the bug | Pure Python, no external deps |
| **Observability** | Request with no correlation ID vs with ID | Logging to stdout, no external sink |

#### Generating POC Scripts

When generating a POC script for a finding:

1. **Identify the failure mode** — what exactly breaks, and when
2. **Build the minimal mock** — the simplest localhost server/service that reproduces it
3. **Show the broken state** — run the anti-pattern against the mock, observe the failure
4. **Apply the fix** — change the minimum amount of code
5. **Show the fixed state** — run the fix against the same mock, observe the improvement
6. **Write the verification** — the exact assertion that proves the fix works (maps to `acceptance_criteria`)
7. **Add teaching comments** — explain why each step matters, not just what it does

### Choosing a Format

| Situation | Recommended format |
|---|---|
| Sharing with team in PR/Confluence | Markdown |
| Onboarding new engineers | HTML (teaching tool) |
| Convincing skeptics a finding is real | POC |
| Evidence bundle for Axiom work item | Markdown |
| Executive summary for non-technical stakeholders | HTML |
| All three | Run all formats: `--format all` |

### Report Commands

```bash
# Generate markdown report (default)
/hardening-battery --report

# Generate HTML report
/hardening-battery --report --format html

# Generate POC report for critical/high findings only
/hardening-battery --report --format poc --severity critical,high

# Generate all three formats
/hardening-battery --report --format all

# Save to specific path
/hardening-battery --report --format html --output docs/hardening-report.html
```

### Teaching Tool Design Principles

When generating reports as teaching tools:

1. **Show the failure, not just the fix** — engineers learn better from seeing what breaks than from reading rules
2. **Concrete > abstract** — "this call blocks for 60 seconds under load" beats "missing timeout"
3. **Reproducible** — every POC should be runnable in under 5 minutes
4. **Graded** — start with the simplest reproduction, then show the worst-case scenario
5. **Actionable** — every finding ends with a copy-pasteable fix and a verification command
6. **Honest about confidence** — `confirmed` findings get full POCs; `suspected` findings get "how to verify" steps instead

---

### Path B: Axiom Work Items (Full Flywheel)

Wire findings into the Axiom lifecycle for automated resolution:

#### Adapting to Your Tracker

Path B uses Jira as the default tracker. If your team uses a different issue tracker, map the hierarchy as follows:

| Jira | GitHub Issues | Linear | Asana |
|---|---|---|---|
| Initiative | Milestone | Project | Portfolio |
| Epic | Label / Project | Cycle | Project |
| Task | Issue | Issue | Task |
| Subtask | Sub-issue (beta) | Sub-issue | Subtask |

**GitHub Issues example:**
- Create a Milestone: "Hardening Audit — `<repo>` Q2 2026"
- Create Issues with label `hardening` for each finding
- Use `HARDEN-<category>-<slug>` as the issue title prefix
- Link issues to the milestone for progress tracking

**Linear example:**
- Create a Project: "Hardening Audit — `<repo>`"
- Create Issues in the project for each finding
- Use Cycles for sprint-level batching

The `HARDEN-<category>-<slug>` ID format works in any tracker — use it as the issue title prefix regardless of tracker.

#### Step 1: Create Jira Hierarchy

```
Initiative: "Hardening Audit — <repo-name> Q2 2026"
  Epic: "SPOF Hardening — <repo-name>"
    Task: HARDEN-SPOF-NO-TIMEOUT (work item ID)
    Task: HARDEN-SPOF-NO-CIRCUIT-BREAKER
  Epic: "Security Hardening — <repo-name>"
    Task: HARDEN-SEC-SQL-INJECTION (requires_human_review: true)
    Task: HARDEN-SEC-HARDCODED-SECRET (requires_human_review: true)
  Epic: "Database Hardening — <repo-name>"
    Task: HARDEN-DB-N-PLUS-ONE
    Task: HARDEN-DB-LOCKING-MIGRATION (requires_human_review: true)
  Epic: "SRE Hardening — <repo-name>"
    Task: HARDEN-SRE-NO-TIMEOUT
    Task: HARDEN-SRE-SWALLOWED-ERROR
  Epic: "Quality Hardening — <repo-name>"
    Task: HARDEN-QUAL-NO-ERROR-PATH
    Task: HARDEN-QUAL-FLAKY-TEST
  Epic: "Observability Hardening — <repo-name>"
    Task: HARDEN-OBS-NO-CORRELATION-ID
    Task: HARDEN-OBS-HIGH-CARDINALITY-LABELS
```

#### Step 2: Create Work Items from Findings

Each finding becomes a Axiom work item. Use the finding's `id` as the `work_item_id`:

```yaml
# Example work item from finding
work_item_id: HARDEN-SRE-NO-TIMEOUT
jira_ref: PROJ-456
request: >
  Add timeout to HTTP call to payment-service in src/payment/client.py:42.
  Currently no timeout configured — will hang indefinitely if service is down.
acceptance_criteria:
  - "HTTP call to payment-service has timeout=(3.05, 30)"
  - "Timeout triggers fallback response, not unhandled exception"
  - "Metrics show request duration bounded by timeout value"
constraints:
  no_breaking_changes: true
  requires_human_review: false
verification_tier: 3
```

#### Step 3: Route by Confidence Band

| Finding type | Confidence | Action |
|---|---|---|
| `requires_human_review: true` | Any | Human reviews before Axiom executes |
| `confidence: confirmed` + `severity: critical/high` | HIGH | Human approves, Axiom executes |
| `confidence: confirmed` + `severity: medium/low` | HIGH | Axiom can auto-draft PR |
| `confidence: suspected` | MEDIUM | Human verifies finding first |

**Default policy:** Security and migration findings always require human review, regardless of confidence.

#### Step 4: Track Progress via Parent Ticket

The Initiative ticket tracks overall hardening progress:
- Total findings: N
- Resolved: X
- In progress: Y
- Blocked (human review): Z
- Error budget improvement: before/after `axiom analyze` score

---

## Finding Intake Format

When creating a Axiom work item from a hardening finding, use this format:

```yaml
# .memory-bank/work-items/HARDEN-SRE-NO-TIMEOUT/work-item.yaml
work_item_id: HARDEN-SRE-NO-TIMEOUT
jira_ref: PROJ-456
parent_epic: PROJ-450  # SRE Hardening epic
source_finding:
  id: HARDEN-SRE-NO-TIMEOUT
  severity: high
  category: sre
  location: "src/payment/client.py:42"
  confidence: confirmed
  requires_human_review: false
request: >
  Add explicit timeout to HTTP call to payment-service.
  Currently: requests.post(url, json=data)
  Required: requests.post(url, json=data, timeout=(3.05, 30))
  Also add fallback: return cached result or raise ServiceUnavailableError on timeout.
acceptance_criteria:
  - "HTTP call to payment-service has timeout=(3.05, 30), verified by blocking service and observing timeout within 30s"
  - "Timeout triggers fallback response (not unhandled exception), verified by checking response on timeout"
  - "Structured log event emitted on timeout: {event: payment_service_timeout, order_id: ..., timeout_seconds: 30}"
constraints:
  no_breaking_changes: true
  requires_human_review: false
verification_tier: 3
```

---

## Deduplication Rules

Some findings span multiple categories. Deduplicate before creating work items:

| Pattern | Primary category | Secondary |
|---|---|---|
| Missing timeout | SRE | SPOF |
| No circuit breaker | SPOF | SRE |
| Swallowed error | SRE | Quality (missing error path test) |
| PII in logs | Security | Observability |
| N+1 query | Database | SRE (performance) |
| Missing correlation ID | Observability | Quality (untestable) |

**Rule:** Create one work item per finding. Use the primary category for the `id`. Note the secondary category in the description.

---

## Triage Matrix

Use this matrix to prioritize findings after collection:

| Severity | `requires_human_review` | Action |
|---|---|---|
| critical | true | Immediate human review; fix this week |
| critical | false | Axiom can draft PR; human approves; fix this week |
| high | true | Human review this sprint |
| high | false | Axiom can draft PR; fix this sprint |
| medium | any | Backlog; fix this quarter |
| low | any | Backlog; fix when touching nearby code |

**Maximum open PRs:** Respect `max_open_prs` from Axiom config. Don't flood the team with 50 PRs at once. Batch by epic.

---

## Quarterly Cadence

The hardening battery is most valuable as a recurring process, not a one-off:

```
Q1: Run battery → collect findings → create work items → resolve critical/high
Q2: Re-run battery → compare to Q1 → measure improvement → resolve medium
Q3: Re-run battery → few new findings (flywheel working) → resolve low
Q4: Re-run battery → establish baseline for next year
```

**Measuring improvement:**
```bash
# Before hardening
axiom analyze --repo . > baseline-score.json

# After resolving findings
axiom analyze --repo . > post-hardening-score.json

# Compare
diff baseline-score.json post-hardening-score.json
```

### Enforcing the Cadence

The quarterly cadence is only valuable if it actually runs. Options for enforcement:

**Option 1: CI/CD scheduled job (recommended)**
```yaml
# .github/workflows/hardening-quarterly.yml
name: Quarterly Hardening Battery
on:
  schedule:
    - cron: '0 9 1 1,4,7,10 *'  # First day of each quarter at 9am UTC
  workflow_dispatch:  # Allow manual trigger
jobs:
  hardening:
    runs-on: ubuntu-latest
    steps:
      - name: Trigger hardening battery
        run: echo "Run /hardening-battery via Axiom CLI or OpenCode"
```

**Option 2: Calendar reminder**
- Add a recurring quarterly calendar event: "Run hardening battery on `<repo>`"
- Link to this skill in the event description
- Assign to the team lead or on-call rotation

**Option 3: Jira recurring task**
- Create a recurring Jira task: "Quarterly hardening audit — `<repo>`"
- Set recurrence: every 3 months
- Assign to the team

**Minimum viable enforcement:** A calendar reminder is better than nothing. The goal is that the battery runs at least once per quarter, not that it runs automatically.

---

## First-Run Triage Protocol

The first run of the hardening battery on a legacy codebase can produce 50+ findings. Do not try to fix everything at once.

### Recommended first-run approach

1. **Cap at top 10 by severity.** Sort all findings by severity (critical → high → medium → low). Take the top 10. Create work items only for those 10.

2. **Security findings first.** Any `critical` or `high` security finding goes to the top of the list, regardless of category. These require human review and should be triaged in a dedicated security meeting.

3. **Batch by category.** Group the top 10 into their categories. Fix all SPOF findings together (one PR), all SRE findings together (one PR), etc. This reduces context-switching and makes PRs reviewable.

4. **Defer medium and low.** Add medium and low findings to the backlog. Run the battery again next quarter — some will be fixed as a side effect of the high-priority work.

5. **When security audit produces 50+ findings:**
   - Do NOT create 50 Jira tickets at once.
   - Triage in a 30-minute meeting: group by root cause (e.g., "all SQL injection findings are the same pattern in 20 files" = 1 work item, not 20).
   - Create one work item per root cause, not per finding instance.

### Finding volume by category (typical first run)

| Category | Typical finding count | Notes |
|---|---|---|
| Security | 5–20 | Depends heavily on codebase age and security practices |
| SRE | 10–30 | Missing timeouts are very common in legacy code |
| Database | 5–15 | N+1 queries are common in ORM-heavy codebases |
| SPOF | 3–10 | Usually a few critical dependencies |
| Quality | 10–40 | Test gaps are common; batch by module |
| Observability | 5–15 | Structured logging gaps are common |

**Rule of thumb:** If the battery produces > 20 findings, you have a healthy codebase with normal technical debt. If it produces > 50, prioritize ruthlessly and fix the critical/high items first.

---

## Anti-Patterns in Running the Battery

From `hardening-anti-patterns-axiom`:

| Anti-pattern | Why it fails | Do this instead |
|---|---|---|
| "Fix everything you find" in the prompt | Model invents issues; no evidence-backed AC | Ask for findings with AC; let Axiom execute |
| Running all 6 prompts as one mega-prompt | Output truncated; categories bleed | One prompt per category, separate runs |
| Treating findings as work items without AC | Axiom can't verify vague criteria → spins | Require `acceptance_criteria` on every finding |
| Running once and calling it done | Foundational issues return as code evolves | Quarterly cadence |
| Auto-executing security/migration findings | These need human judgment | Gate with `requires_human_review: true` |
| Filing findings without Jira hierarchy | Findings become orphaned tasks | Every Task needs an Epic parent |
| Running without analyze baseline | No way to measure improvement | Run `axiom analyze` first |

---

## Few-Shot Refinement Path

The first run produces findings. Resolved findings produce evidence bundles. Evidence bundles become few-shot examples:

```
Run 1: Zero-shot audit → findings → work items → resolved findings → evidence bundles
Run 2: Add few-shot examples from resolved findings → higher quality findings
Run 3: Examples in specs/hardening-examples/ → spec_alignment confidence signal improves
```

**Store examples in:** `specs/hardening-examples/<category>/<finding-slug>.md`

**Example structure:**
```markdown
# Example: HARDEN-SRE-NO-TIMEOUT (Resolved)

## Finding
[original finding YAML]

## Resolution
[code change made]

## Evidence
[test output showing timeout fires within 30s]
[metrics showing bounded request duration]

## Lessons
[what made this finding easy/hard to resolve]
```

---

## Command Files (Optional)

For teams that run the battery frequently, create command files:

```markdown
<!-- .opencode/commands/hardening-spof.md -->
---
name: hardening-spof
description: Run SPOF hardening audit on this repo
---

Load the hardening-spof-axiom skill and run the SPOF audit prompt
against the codebase. Produce findings in HARDEN-SPOF-* format with
Tier-3+ acceptance criteria. Output a structured list of findings
ready to be converted to work items.
```

---

## Integration with Axiom Runners (Path B Advanced)

For fully automated quarterly runs, wire the battery as a Axiom runner:

> ⚠️ **Future capability (not yet implemented in Axiom v1.x).** The runner.yaml format below is a design preview for a planned quarterly automation feature. As of the current release, quarterly runs require manual initiation via `/hardening-battery`. The runner format will be implemented in a future release.

```yaml
# .axiom/runners/hardening-quarterly-runner/runner.yaml
name: hardening-quarterly-runner
description: Quarterly hardening battery
trigger:
  schedule: "0 9 1 */3 *"  # First day of each quarter at 9am
  manual: true
skills:
  - hardening-anti-patterns-axiom
  - hardening-spof-axiom
  - hardening-security-axiom
  - hardening-database-axiom
  - hardening-sre-axiom
  - hardening-quality-axiom
  - hardening-observability-axiom
output:
  jira_parent: "PROJ-HARDENING-INITIATIVE"
  create_work_items: true
  human_review_required_for:
    - security
    - migration
```

---

## Checklist: Before Claiming Battery Complete

- [ ] All 6 category prompts run (SPOF, Security, DB, SRE, Quality, Observability)
- [ ] Findings deduplicated across categories
- [ ] All critical findings triaged
- [ ] All high findings triaged
- [ ] Security findings flagged `requires_human_review: true`
- [ ] Migration findings flagged `requires_human_review: true`
- [ ] Work items created in Jira with parent Epic
- [ ] `axiom analyze` baseline captured
- [ ] Next quarterly run scheduled
- [ ] Resolved findings saved to `specs/hardening-examples/<category>/` (at least 1 example per category with findings) — feeds the few-shot flywheel for future runs

---

axiom:trace work_item=hardening-skills-01 spec=hardening-intake-axiom jira_ref=SWDE-7 plan=phase-1/task-8/step-1
