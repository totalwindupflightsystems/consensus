---
tags:
  vertical: [ops, sre, planning]
  category: observability
  core: false
---

# Skill: sitrep-ascii-graphs

> **"A graph without data is a guess. Never invent counts — show UNKNOWN when artifacts are missing."**

Portable ASCII progress graph renderer for Axiom SitRep reports. Renders work item progress bars, phase breakdowns, spec coverage heatmaps, and velocity sparklines from plan/TODO artifacts. Load this skill when generating SitRep reports, progress summaries, or any output that needs visual progress representation without a full charting library.

## When to Load This Skill

Load this skill when:
- Generating SitRep reports (`sitrep_now`, `daily_sitrep`, `weekly_sitrep`, `release_readiness_report`)
- Creating progress summaries for work items or phases
- Rendering spec coverage heatmaps for conformance sweeps
- Displaying velocity trends over time
- Any output that needs visual progress representation without charting libraries

---

## Core Rules

1. **Never invent data.** If artifacts are missing or unreadable, show UNKNOWN.
2. **Always show raw counts.** Percentages alone are insufficient — always include `(done/total)`.
3. **Use deterministic rendering.** Same inputs → same outputs every time.
4. **ASCII-safe characters only.** Use `█`, `░`, `▁▂▃▄▅▆▇█`, `✓`, `⚠`, `→`, `?`.
5. **24-char bar width.** Standardized for terminal and GitHub Markdown readability.
6. **Evidence-backed only.** 100% MUST have closure evidence in `verification.md` — not just plan text.

---

## ASCII Rendering Reference

### Character Set

| Purpose | Characters |
|---------|------------|
| Bar fill | `█` |
| Bar empty | `░` |
| Sparkline 0/8 | `▁` |
| Sparkline 1/8 | `▂` |
| Sparkline 2/8 | `▃` |
| Sparkline 3/8 | `▄` |
| Sparkline 4/8 | `▅` |
| Sparkline 5/8 | `▆` |
| Sparkline 6/8 | `▇` |
| Sparkline 7-8/8 | `█` |
| Status: done | `✓` |
| Status: blocked | `⚠` |
| Status: in progress | `→` |
| Status: unknown | `?` |

### Formulas

```text
// Progress bar (24 chars wide)
pct          = (done / total * 100) if total > 0 else 0
filled_chars = round(pct * 24 / 100)
bar          = "█" * filled_chars + "░" * (24 - filled_chars)
icon         = "✓" if done == total else ("⚠" if blocked else "→")
output       = f"[{bar}] {pct:3.0f}% ({done}/{total}) {icon}"

// Velocity sparkline (N weeks)
range_val    = max(weekly_counts) - min(weekly_counts) or 1
normalized   = [(v - min(weekly_counts)) / range_val * 7 for v in weekly_counts]
spark_chars  = "▁▂▃▄▅▆▇█"
sparkline    = "".join(spark_chars[min(7, int(round(n)))] for n in normalized)
```

---

## Tool Reference Card

| Function | Description | Example Output |
|----------|-------------|----------------|
| `render_ascii_progress_bar(done, total, width=24)` | Single bar with pct + count + icon | `[████████████████████░░░░]  84% (21/25) →` |
| `render_work_item_progress_table(work_items[])` | Table of all active work items | Multi-line table (see Workflow 1) |
| `render_phase_breakdown(work_item_id, phases[])` | Per-phase bars for one work item | Indented phase list (see Workflow 2) |
| `render_spec_coverage_map(specs[], results[])` | Spec sweep status heatmap | CONFORMANT/PARTIAL/NOT SWEPT (see Workflow 3) |
| `render_velocity_sparkline(weekly_counts[])` | N-week velocity trend line | `▁▂▃▄▅▆▇█▇▆  (last 10 weeks)` |
| `derive_progress_counts(work_item_id, plan_paths[])` | Extract done/total from artifacts | `{done: 21, total: 25}` or `UNKNOWN` |

---

## Workflow 1: Work Item Progress Table

### Step 1 — Locate Plan Artifacts

For each work item, look in this precedence order:

1. `.memory-bank/work-items/<WORK_ITEM_ID>/plan.yaml` — structured step list
2. `.memory-bank/work-items/<WORK_ITEM_ID>/plan.md` — Markdown plan with checkboxes
3. `.memory-bank/TODO.md` — global TODO with checkbox format for this work item
4. `.memory-bank/work-items/<WORK_ITEM_ID>/verification.md` — closure evidence

### Step 2 — Extract Counts

**From `plan.yaml`** (look for `status` fields):
```yaml
steps:
  - id: step-84-1-1-1
    status: done        # ← count this
  - id: step-84-1-1-2
    status: done        # ← count this
  - id: step-84-1-1-3
    status: in_progress # ← not done
```
Count entries where `status` is `done`, `complete`, or `closed` vs total entries.

**From `plan.md` or `TODO.md`** (checkbox format):
```markdown
- [x] step-84-1-1-1 — branch naming complete     ← done
- [x] step-84-1-1-2 — config validation complete  ← done
- [ ] step-84-1-1-3 — security review pending     ← not done
```
Count `- [x]` (done) vs `- [ ]` (pending).

**From `verification.md`** (acceptance criteria table):
```markdown
| AC-1 | Branch name derivation | pass       |  ← done
| AC-2 | Config validation      | pass       |  ← done
| AC-3 | Security review        | unverified |  ← not done
```
Count `pass` rows vs total rows.

**If no structured data found**: show `[??????????????????] UNKNOWN` with verification steps.

### Step 3 — Render the Table

```text
Work Item Progress
────────────────────────────────────────────────────────────
branch-management-01  [████████████████████████] 100% (25/25) ✓
idle-sweep-01         [████████████░░░░░░░░░░░░]  48% (12/25) →
multi-channel-01      [████████████████████████] 100% (25/25) ✓
jira-live-integration [████████░░░░░░░░░░░░░░░░]  32%  (8/25) →
ui-basic-auth-01      [████████████████████████] 100% (25/25) ✓
────────────────────────────────────────────────────────────
5 work items: 3 complete, 2 in progress, 0 blocked
```

**Column alignment rules:**
- Work item name: left-aligned, padded to longest name
- Bar: always `[` + 24 chars + `]`
- Percentage: right-aligned 3 chars + `%`
- Counts: `(done/total)` in parentheses
- Icon: space + icon char

---

## Workflow 2: Phase Breakdown

### Step 1 — Identify Phases

Phases are named `phase-XX-Y` in plan artifacts. Find them by:
- Reading `plan.yaml` for `phase_id` or `phase` fields
- Reading `plan.md` headings like `### Phase 84.1`
- Reading `TODO.md` for phase-labeled sections

### Step 2 — Count Steps per Phase

For each phase, count done/total steps using the same method as Workflow 1, scoped to that phase's step IDs.

### Step 3 — Render Phase Breakdown

```text
branch-management-01
  Phase 84.1 (naming+config)    [████████████████████████] 100% (5/5) ✓
  Phase 84.2 (merge/rebase)     [████████████████████████] 100% (5/5) ✓
  Phase 84.3 (API+UI)           [████████████████████████] 100% (5/5) ✓
  Phase 84.4 (container)        [████████████████████████] 100% (5/5) ✓
  Phase 84.5 (integration)      [████████████████████████] 100% (5/5) ✓

idle-spec-conformance-sweep-01
  Phase 80.1 (specs/33-37)      [████████████████████████] 100% (5/5) ✓
  Phase 80.2 (specs/38-39)      [████████████████████████] 100% (5/5) ✓
  Phase 80.3 (specs/40-41)      [████████████████████████] 100% (5/5) ✓
  Phase 80.4 (specs/06)         [████████░░░░░░░░░░░░░░░░]  33% (1/3) →
```

---

## Workflow 3: Spec Coverage Heatmap

### Step 1 — Locate Sweep Results

Sweep results are stored in:
- `.memory-bank/work-items/idle-spec-conformance-sweep-01/runs/<RUN_ID>/verification.md`
- Look for lines containing `Conformance verdict:` or `CONFORMANT`/`PARTIAL`/`NOT SWEPT`

### Step 2 — Map Specs to Status

| Status | Meaning | Filled chars |
|--------|---------|-------------|
| `CONFORMANT` | All requirements met with evidence | 24 |
| `SUBSTANTIALLY CONFORMANT` | Minor documented gaps | 20 |
| `PARTIAL` | Significant gaps remain | 12 |
| `NOT SWEPT` | No verification run yet | 0 |
| `UNKNOWN` | Cannot determine from artifacts | `??` |

### Step 3 — Render Heatmap

```text
Spec Coverage
  specs/33 (Confidence Scoring)        [████████████████████████] CONFORMANT
  specs/34 (Analytics)                 [████████████████████████] CONFORMANT
  specs/35 (Web UI Dashboard)          [████████████████████████] CONFORMANT
  specs/36 (UI Component Contracts)    [████████████████████████] CONFORMANT
  specs/37 (UX Copy)                   [████████████████████░░░░] SUBSTANTIALLY CONFORMANT
  specs/38 (UX Design Principles)      [████████████░░░░░░░░░░░░] PARTIAL
  specs/39 (UI Advanced Features)      [░░░░░░░░░░░░░░░░░░░░░░░░] NOT SWEPT
  specs/06 (Project Configuration)     [??????????????????] UNKNOWN — verify: check run _20
```

---

## Workflow 4: Velocity Sparkline

### Step 1 — Count Completed Work Items per Week

Sources (in order of preference):
1. Git log: count merge commits or work-item-tagged commits per calendar week
2. Memory bank: count `verification.md` files with `COMPLETE` status per week
3. TODO.md: count `[x]` phase completions per week (approximate)

```text
Week 1:  2 items completed
Week 2:  3 items completed
Week 3:  1 item  completed
...
Week 10: 3 items completed
```

### Step 2 — Render Sparkline

```text
Velocity (items/week): ▁▂▃▄▅▆▇█▇▆  (last 10 weeks)
                        ↑           ↑
                      week 1      week 10
```

If fewer than 3 data points: show `Velocity: INSUFFICIENT DATA (need ≥3 weeks)`.

---

## Guardrails

### DO ✅
- Derive all counts from actual artifacts (`plan.yaml`, `plan.md`, `TODO.md`, `verification.md`)
- Show raw counts alongside percentages: `84% (21/25)`
- Use ASCII-safe block characters for terminal and Markdown compatibility
- Include "How to verify" steps when data is UNKNOWN
- Keep bars to 24 characters for consistent readability
- Show `✓` ONLY when `verification.md` has closure evidence (not just plan text)

### DON'T ❌
- Never invent counts — show `UNKNOWN` instead
- Never show 100% without closure evidence in `verification.md`
- Never use non-ASCII characters that may not render in all terminals
- Never omit raw counts (percentages alone are insufficient)
- Never conflate "plan says done" with "verified done"

---

## Integration with SitRep Output Contract

The `progress_graphs` section is item **6.5** in the SitRep output order:

```
6.   workstream_map    (streams → owners/agents → state → evidence pointers)
6.5. progress_graphs   (ASCII bar charts: work items, phases, spec coverage, velocity)
7.   blockers_and_risks (ranked; owner; evidence or reason; next action)
```

### Required Modes (ALWAYS include `progress_graphs`):
- `sitrep_now`
- `daily_sitrep`
- `weekly_sitrep`
- `release_readiness_report`

### Optional Modes (include if data available):
- `debrief`
- `delta_since`
- `blockers_only`
- `risk_review`

### When No Data Is Available

```markdown
## progress_graphs

**status**: UNKNOWN — no plan/TODO artifacts accessible to derive progress counts.

**How to verify:**
1. Check `.memory-bank/work-items/<WORK_ITEM_ID>/plan.yaml` exists
2. Check `.memory-bank/work-items/<WORK_ITEM_ID>/plan.md` exists
3. Check `.memory-bank/TODO.md` has checkbox format for this work item
4. Check `.memory-bank/work-items/<WORK_ITEM_ID>/verification.md` for closure evidence
```

---

## Memory Bank Artifact Locations

| Artifact | Path | Used For |
|----------|------|----------|
| Work item plan (structured) | `.memory-bank/work-items/<ID>/plan.yaml` | Step counts |
| Work item plan (Markdown) | `.memory-bank/work-items/<ID>/plan.md` | Checkbox counts |
| Global TODO | `.memory-bank/TODO.md` | Checkbox counts, phase status |
| Closure evidence | `.memory-bank/work-items/<ID>/verification.md` | 100% confirmation |
| Spec sweep results | `.memory-bank/work-items/idle-spec-conformance-sweep-01/runs/<RUN>/verification.md` | Spec coverage |
| Current work item | `.memory-bank/work-items/_current.md` | Active item pointer |

---

## Trace

`axiom:trace work_item=sitrep-ascii-graphs-skill spec=specs/00-PRD.md plan=sitrep-ascii-graphs/skill doc=.opencode/skills/sitrep-ascii-graphs/SKILL.md`

Base directory for this skill: file:///Users/aokuwa/code/Axiom/.opencode/skills/sitrep-ascii-graphs
