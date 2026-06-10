---
name: git-history-backfill-axiom
description: >
  Backfill memory bank context from git commit history for repos onboarding into Axiom.
  Reads commit log oldest-to-newest, extracts project evolution, decisions, milestones,
  and contributor patterns, then generates .memory-bank/ files so agents have historical
  context even though the repo didn't start with Axiom. Companion to /axiom-spec-extract
  (which backfills specs from code) — this skill backfills memory from git.
version: "1.2"
created: "2026-03-26"
updated: "2026-04-04"
primary_spec: specs/00-PRD.md
secondary_specs:
  - specs/08-Memory-Bank-Base-Prompt.md
  - specs/21-Traceability-Doctrine.md
  - specs/09-Baby-Steps-Methodology.md
tags:
  vertical: [onboarding, coding]
  category: onboarding
  core: false
---

# Git History Backfill

> **"A repo's git log is its autobiography. Read it cover to cover before claiming you understand the project."**

This skill reads a repository's full git history (oldest to newest) and generates memory bank
files that give Axiom agents historical context about a project that didn't start with Axiom.
It's the "memory transplant" for onboarding existing repos.

It doesn't give the same depth as a repo that grew up with Axiom — there are no spec-linked
trace markers, no per-step verification evidence, no agent reflection notes. But it gives agents
enough context to understand what happened, when, why (when commit messages are good), and who
was involved. That's dramatically better than starting from zero.

axiom:trace work_item=git-history-backfill-01 spec=specs/08-Memory-Bank-Base-Prompt.md plan= test= doc=.opencode/skills/git-history-backfill-axiom/SKILL.md evidence= commit=

---

## Activation

Load this skill when:
- Onboarding an existing repo into Axiom that has git history but no `.memory-bank/`
- Running `/axiom-backfill-git` command
- A user says "import this repo's history" or "backfill context from git"
- After `/axiom-spec-extract` has run (specs from code) and you want memory from git
- A team wants agents to understand the project's evolution before starting new work

**When NOT to load this skill**:
- The repo already has a populated `.memory-bank/` with good historical context
- The repo has fewer than ~10 commits (just read them manually)
- You only need to understand the current state (use `/axiom-spec-extract` instead)
- The repo is a fresh clone with no meaningful history (squashed imports, vendor drops)

---

## Batch Processing Mode (DEX-300)

For large repos (hundreds to thousands of commits), process in **batches** rather than all at once.
Batch mode prevents context overflow, produces incremental evidence, and lets the loop resume
from a cursor if interrupted.

### When to Use Batch Mode

- Repos with **50+ commits** (default threshold)
- Any run with `batch_mode=true` argument
- Incremental catchup runs (DEX-301)
- When the agent context window is limited

### Batch Processing Protocol

#### Step 0: Triage Phase (before writing anything)

Before processing any batch, run a **triage scan** to classify commits:

```bash
# Get commit list oldest-first
git log --reverse --format="%H|%aI|%ae|%s" --shortstat | head -N  # N = batch_size * 3

# For each commit in the batch, classify with git show --stat
git show --stat <hash>
```

**Classify each commit as**:

| Class | Criteria | Action |
|-------|----------|--------|
| `WRITE` | High-signal: architecture change, dependency add/remove, migration, release tag, CI change, security change, large refactor (10+ files), first/last commit of year | Process fully — extract narrative, decisions, tech changes |
| `SKIP` | Low-signal: typo fix, formatting, single-file minor edit, auto-generated file update, merge commit with no meaningful changes | Record in lightweight SKIP log only |
| `AMBIGUOUS` | Unclear from stats alone: medium-sized change (3-9 files), unclear commit message | Process with reduced depth; note ambiguity |

**Triage output** (emit before processing):
```
TRIAGE BATCH 1/N (commits abc123..def456):
  WRITE:      12 commits (architecture, deps, releases)
  SKIP:       18 commits (formatting, typos, minor)
  AMBIGUOUS:   5 commits (medium changes, unclear messages)
  Total:      35 commits
```

#### Step 1: Process WRITE Commits (full depth)

For each `WRITE` commit, apply the full Phase 2 signal detection and Phase 3 narrative construction.

#### Step 2: Record SKIP Commits (lightweight format)

For `SKIP` commits, write a **lightweight SKIP entry** — do NOT write a full memory bank note:

```yaml
# In batch evidence file: .memory-bank/work-items/<work_item>/runs/<batch_id>/skipped.yaml
skipped:
  - hash: "abc1234"
    date: "2024-03-15"
    message: "fix typo in README"
    reason: "single-file doc edit, no signal"
  - hash: "def5678"
    date: "2024-03-16"
    message: "format: run prettier"
    reason: "formatting only, no logic change"
```

This lightweight format preserves auditability without bloating the memory bank.

#### Step 3: Process AMBIGUOUS Commits (reduced depth)

For `AMBIGUOUS` commits, extract:
- File paths changed (to detect area of codebase)
- Commit message subject only (no body analysis)
- Whether it touches a high-signal path (auth/, migrations/, CI, deps)

If an AMBIGUOUS commit touches a high-signal path, promote it to `WRITE`.

#### Step 4: Consolidated Batch Evidence

After each batch, write **one consolidated evidence snapshot** (not one file per commit):

```
.memory-bank/work-items/<work_item>/runs/<batch_id>/
  verification.md      # batch summary: counts, cursor, confidence
  skipped.yaml         # lightweight SKIP log
  triage-table.md      # human-readable triage results
```

**verification.md format**:
```markdown
# Batch <N> Evidence

- **Batch**: <N> of <total>
- **Commits processed**: <count>
- **Cursor**: <start_sha>..<end_sha>
- **Date range**: <start_date> to <end_date>
- **WRITE**: <count> (fully processed)
- **SKIP**: <count> (lightweight log only)
- **AMBIGUOUS**: <count> (<X> promoted to WRITE)
- **Memory bank files written**: <list>
- **Confidence**: <low|medium|high>
- **Next cursor**: <next_sha> (or "COMPLETE" if done)
```

#### Step 5: Advance Cursor

After each batch, record the cursor (last processed SHA) in the work item:

```yaml
# In .memory-bank/work-items/<work_item>/plan.yaml
cursor:
  last_processed_sha: "abc1234"
  last_processed_date: "2024-03-16"
  batches_completed: 3
  total_commits_processed: 105
  status: in_progress  # or: complete
```

This enables **incremental catchup** (DEX-301): on the next run, start from `last_processed_sha`.

### Default Batch Size

- **Default**: 15 commits per batch (optimized for agent context window)
- **Override**: `batch_size=N` argument (e.g., `batch_size=5` for very large commits, `batch_size=50` for small repos)
- **Rationale**: 15 commits balances context window usage with meaningful progress per cycle

### Batch Mode Evidence Structure

```
.memory-bank/work-items/<work_item_id>/
  plan.yaml                    # cursor tracking
  runs/
    batch-001/
      verification.md          # batch 1 summary
      skipped.yaml             # SKIP log for batch 1
      triage-table.md          # triage results
    batch-002/
      verification.md
      skipped.yaml
      triage-table.md
    ...
    final/
      verification.md          # overall backfill summary
      backfill-manifest.md     # complete provenance record
```

### Batch Mode Rules

1. **Always triage before writing** — classify all commits in the batch before writing any memory bank files.
2. **One evidence snapshot per batch** — not one file per commit.
3. **SKIP entries are lightweight** — YAML only, no full memory bank notes.
4. **Cursor is sacred** — always write the cursor after each batch so the run can resume.
5. **Spot-check SKIPs** — verifier samples 10% of SKIP entries to confirm they were correctly classified.
6. **Batch size is configurable** — default 15, override with `batch_size=N`.
7. **Triage table is human-readable** — emit it before processing so the user can see what's coming.

---

## What This Produces

The backfill generates these memory bank artifacts:

| Artifact | Path | What It Contains |
|----------|------|-----------------|
| **Project timeline** | `.memory-bank/projects/<project>/timeline.md` | Chronological narrative of the project's evolution in phases |
| **Decision log seed** | `.memory-bank/decisionLog.md` (append) | Decisions inferred from significant commits (architecture changes, dependency additions, migrations) |
| **Tech context seed** | `.memory-bank/techContext.md` (create/augment) | Tech stack evolution detected from dependency manifests over time |
| **Contributor map** | `.memory-bank/projects/<project>/contributors.md` | Who worked on what, knowledge areas, active periods |
| **Milestone markers** | `.memory-bank/projects/<project>/milestones.md` | Tags, releases, major merges, and inflection points |
| **Backfill manifest** | `.memory-bank/projects/<project>/backfill-manifest.md` | What was backfilled, when, confidence levels, and gaps |

All generated files include a `[BACKFILLED]` epistemic label so agents know this context was
reconstructed, not captured in real time.

---

## The Backfill Method

### Phase 1: Git Log Extraction

Read the full commit history oldest-to-newest. Extract structured data from each commit.

**What to extract per commit**:
```yaml
commit:
  hash: "abc1234"
  date: "2024-01-15T10:30:00Z"
  author: "alice@example.com"
  message_subject: "feat: add user authentication with JWT"
  message_body: "Implements RFC 7519 JWT tokens for API auth..."
  files_changed: 12
  insertions: 450
  deletions: 30
  paths_touched:
    - "src/auth/"
    - "src/middleware/"
    - "tests/auth/"
  is_merge: false
  tags: ["v0.2.0"]  # if tagged
```

**Batch processing**: for repos with thousands of commits, process in batches of ~200.
Don't try to hold the entire log in memory at once. Process oldest-first so the narrative
builds chronologically.

**Git commands to use**:
```bash
# Full log, oldest first, with stats
git log --reverse --format="%H|%aI|%ae|%s" --shortstat

# For deeper analysis of specific commits
git show --stat <hash>

# Tags and releases
git tag --sort=creatordate --format="%(refname:short)|%(creatordate:iso)"

# File-level change history for key files
git log --follow --format="%H|%aI|%s" -- <path>

# Dependency manifest changes over time
git log --all --format="%H|%aI|%s" -- "package.json" "requirements.txt" "go.mod" "Cargo.toml" "Gemfile" "pom.xml"
```

### Phase 2: Signal Detection

Scan the extracted commits for high-signal events. Not every commit matters — focus on
the ones that tell the story.

**High-signal commit patterns**:

| Signal | Detection Pattern | What It Means |
|--------|------------------|---------------|
| **Project birth** | First commit(s) | Initial tech choices, starting architecture |
| **Architecture shift** | Large refactors touching many files; new top-level directories appearing | Major design decision |
| **Dependency addition** | Changes to package.json/requirements.txt/go.mod etc. | New capability or tech choice |
| **Dependency removal** | Same manifests, but removals | Migration away from something |
| **Migration files** | New files in `migrations/`, `alembic/`, `db/migrate/` etc. | Schema evolution |
| **CI/CD changes** | Changes to `.github/workflows/`, `Jenkinsfile`, `.gitlab-ci.yml` etc. | Process evolution |
| **Config surface changes** | New env vars, config files, feature flags | Operational evolution |
| **Security changes** | Auth, encryption, CORS, CSP, secrets management | Security posture changes |
| **Test infrastructure** | Test framework setup, CI test additions, coverage config | Quality evolution |
| **Documentation** | README changes, docs/ additions, API docs | Communication milestones |
| **Release tags** | Semver tags, release branches | Delivery milestones |
| **Large merges** | Merge commits with many files | Feature branch completions |
| **Breaking changes** | `BREAKING CHANGE:` in commit messages; major version bumps | Compatibility boundaries |
| **Contributor changes** | New authors appearing; authors stopping | Team evolution |
| **Revert commits** | `Revert "..."` pattern | Something went wrong |
| **Hotfix patterns** | Rapid commits after a release tag | Incident response |

### Phase 3: Narrative Construction

Build a chronological narrative from the high-signal commits. Group commits into **eras**
— periods where the project had a consistent direction or focus.

**Era detection heuristics**:
- A new era starts when: a major dependency is added/removed, the architecture shifts,
  a new contributor joins and changes the style, or there's a long gap followed by resumed activity
- Eras are named descriptively: "Initial Build (Jan–Mar 2024)", "API Rewrite (Apr–Jun 2024)",
  "Production Hardening (Jul–Sep 2024)"

**For each era, capture**:
```yaml
era:
  name: "API Rewrite"
  period: "2024-04-01 to 2024-06-15"
  summary: |
    Rewrote the REST API from Express to Fastify. Motivation appears to be
    performance (commit messages reference "p99 latency" and "connection pooling").
    Added OpenAPI spec generation. Introduced structured error responses.
  key_commits:
    - hash: "def5678"
      message: "feat: migrate API framework from Express to Fastify"
      significance: "Architecture shift — new HTTP framework"
    - hash: "ghi9012"
      message: "feat: add OpenAPI spec auto-generation"
      significance: "API contract formalization"
  decisions_inferred:
    - decision: "Chose Fastify over Express"
      confidence: medium
      evidence: "Commit messages mention performance; Fastify is known for speed"
      caveat: "[BACKFILLED] — inferred from commits, not from a documented decision"
  tech_changes:
    added: ["fastify", "fastify-swagger"]
    removed: ["express", "express-validator"]
  contributors_active: ["alice@example.com", "bob@example.com"]
```

### Phase 4: Memory Bank File Generation

Generate the memory bank files from the narrative. Follow `.memory-bank/_prompt.md` conventions.

#### 4a: Project Timeline

```markdown
---
mb:
  type: overview
  title: "Project Timeline (Backfilled from Git)"
  created: 2026-03-26
  updated: 2026-03-26
  tags: [backfill, timeline, git-history]
  links:
    up: "../_index.md"
  source:
    type: git-backfill
    ref: "git log --reverse (full history)"
---

# Project Timeline [BACKFILLED]

> This timeline was reconstructed from git commit history by the git-history-backfill skill.
> Confidence varies by era — well-messaged commits produce high-confidence context;
> terse commits produce low-confidence inferences. See the backfill manifest for details.

## Era 1: Initial Build (Jan–Mar 2024)
...

## Era 2: API Rewrite (Apr–Jun 2024)
...
```

#### 4b: Decision Log Seed

Append inferred decisions to `decisionLog.md`. Each entry is clearly marked as backfilled:

```markdown
### [BACKFILLED] Chose Fastify over Express (2024-04-03)

**Context**: API performance was a concern (commit messages reference p99 latency).
**Decision**: Migrated from Express to Fastify.
**Evidence**: Commit def5678 — "feat: migrate API framework from Express to Fastify"
**Confidence**: Medium — inferred from commit messages, not from a documented decision.
**Alternatives considered**: Unknown — no ADR found.
**Status**: Active (Fastify is still the current framework as of latest commit).
```

#### 4c: Tech Context Seed

Create or augment `techContext.md` with the tech stack evolution:

```markdown
## Tech Stack Evolution [BACKFILLED]

| Period | Language | Framework | Database | Key Dependencies |
|--------|----------|-----------|----------|-----------------|
| Jan–Mar 2024 | Node.js 18 | Express 4 | PostgreSQL 14 | passport, sequelize |
| Apr–Jun 2024 | Node.js 20 | Fastify 4 | PostgreSQL 15 | fastify-swagger, prisma |
| Jul–present | Node.js 20 | Fastify 4 | PostgreSQL 15 + Redis | prisma, bullmq, ioredis |
```

#### 4d: Contributor Map

**Privacy note**: The contributor map exposes email addresses from git history. Some teams or
repos may not want this in memory bank files (open-source contributors, contractors, departed
employees). Respect the `--no-contributors` option (see command) to skip this artifact entirely.
When generating the map, prefer display names over raw emails when available via `git log --format="%aN"`.

**Caveat**: Commit count is a proxy for activity, **not** for knowledge depth or expertise.
A contributor with 10 carefully-reviewed architectural commits may understand the system better
than one with 200 auto-formatted lint fixes. Do not use commit counts to assess expertise —
use them only to identify active periods and area focus.

```markdown
## Contributors [BACKFILLED]

> ⚠️ Commit counts reflect activity volume, not knowledge depth or expertise.
> A few high-impact commits may represent deeper understanding than many small ones.

| Contributor | Active Period | Primary Areas | Commit Count |
|-------------|--------------|---------------|-------------|
| Alice (alice@example.com) | Jan 2024–present | API, auth, data model | 234 |
| Bob (bob@example.com) | Mar–Aug 2024 | Frontend, CI/CD | 89 |
| Carol (carol@example.com) | Jul 2024–present | Infrastructure, monitoring | 56 |

### Knowledge Concentration Risk
- Alice is the sole contributor to `src/auth/` — bus factor = 1
- `src/billing/` has had no commits in 6 months — may be stale or stable
```

#### 4e: Milestone Markers

```markdown
## Milestones [BACKFILLED]

| Date | Milestone | Evidence |
|------|-----------|---------|
| 2024-01-15 | First commit | Initial project setup |
| 2024-03-01 | v0.1.0 | First tagged release |
| 2024-04-03 | API rewrite begins | Express → Fastify migration |
| 2024-06-15 | v0.2.0 | API rewrite complete |
| 2024-07-20 | Redis added | Background job processing |
| 2024-09-01 | v1.0.0 | First production release |
```

#### 4f: Backfill Manifest

Always produce this — it's the "provenance" for the backfill itself:

```markdown
## Backfill Manifest

| Field | Value |
|-------|-------|
| Backfill date | 2026-03-26 |
| Skill version | 1.0 |
| Commits analyzed | 478 |
| Date range | 2024-01-15 to 2026-03-25 |
| Eras detected | 4 |
| Decisions inferred | 12 |
| Milestones detected | 8 |
| Contributors found | 5 |
| Overall confidence | Medium |

### Confidence by Era

| Era | Confidence | Reason |
|-----|-----------|--------|
| Initial Build | Low | Terse commit messages; no conventional commits |
| API Rewrite | High | Good commit messages; clear PR references |
| Production Hardening | Medium | Mixed message quality |
| Current | Medium | Conventional commits but sparse bodies |

### Known Gaps

- No ADRs found — all decisions are inferred
- Commit messages before April 2024 are terse (e.g., "fix bug", "update")
- Squash merges hide individual commit context in some periods
- No Jira/ticket references found in commit messages
```

---

## Handling Poor Commit History

Not every repo has good commit messages. The skill must degrade gracefully.

| History Quality | Signal | Approach |
|----------------|--------|----------|
| **Excellent** | Conventional commits, PR references, ticket IDs, detailed bodies | Extract rich narrative; high confidence |
| **Good** | Descriptive subjects, occasional bodies, some ticket refs | Extract solid narrative; medium confidence |
| **Mediocre** | Short subjects, no bodies, no ticket refs | Focus on file-change patterns and dependency diffs; low-medium confidence |
| **Poor** | "fix", "update", "wip", single-word messages | Rely almost entirely on file-change patterns, tag dates, and dependency manifest diffs; low confidence |
| **Squashed** | Few large commits with many files | Treat each squash as an era boundary; extract what you can from the squash message |
| **Rewritten** | Force-pushed history, rebased, amended | Note that history may be incomplete; mark confidence as low |

**Key rule**: never invent rationale that isn't supported by the commit data. If the commit
message says "fix bug" and nothing else, the decision log entry says:
"A bug was fixed in `src/auth/login.py` on 2024-03-15. No further context available. [BACKFILLED, confidence: low]"

---

## Integration with Other Skills and Commands

**Standalone usage**: This skill works independently. It reads git history and writes memory
bank files. It does not require `/axiom-spec-extract` to have run first, though running
both gives the most complete onboarding picture.

| Skill/Command | Integration Point |
|---------------|-------------------|
| `/axiom-spec-extract` | Extracts specs from code; this skill extracts memory from git. Together they give full onboarding. |
| `/axiom-onboarding` | After backfill + spec-extract, run onboarding to set up TODO/plans/loops |
| `decision-archaeology-axiom` | Backfilled decisions become the starting layer for future archaeology |
| `@memory-bank-axiom` | Backfilled files follow memory bank conventions; MB-Steward can refine them |
| `@repo-researcher-axiom` | Can provide deeper analysis of specific eras or decisions |
| `axiom-onboarding` | Backfill is a pre-step to onboarding; provides context that onboarding scaffolding needs |

---

## Recommended Onboarding Sequence

For a repo that has code and git history but no Axiom:

1. **Install Axiom** (`.opencode/`, `.axiom/`, `.memory-bank/` scaffolding)
2. **`/axiom-backfill-git`** — backfill memory from git history (this skill)
3. **`/axiom-spec-extract`** — extract specs from current code
4. **`/axiom-onboarding`** — set up TODO, plans, loops, prompt bundles
5. **Start working** — agents now have historical context + specs + execution scaffolding

---

## Rules

1. **Always process oldest-to-newest** — the narrative must build chronologically.
2. **Never invent rationale** — if the commit message doesn't explain why, say so.
3. **Always mark backfilled content** — use `[BACKFILLED]` labels on every generated artifact.
4. **Include confidence levels** — per-era and per-decision.
5. **Produce the backfill manifest** — always. It's the provenance record.
6. **Don't overwrite existing memory bank files** — append or augment. If `decisionLog.md` already has content, append to it with a clear `## Backfilled Decisions` section.
7. **Respect `.memory-bank/_prompt.md` conventions** — frontmatter, naming, linking.
8. **Cap analysis time** — for repos with 1000+ commits at `standard` depth, sample strategically (all tags, all dependency changes, first/last 50 commits per year, all commits touching architecture-significant paths). At `full` depth, analyze every commit regardless of count. Document what was sampled vs exhaustively analyzed in the backfill manifest.
9. **Redact sensitive data** — if commit messages contain secrets, tokens, or PII, redact as `[REDACTED]`.
10. **Support manual augmentation** — humans will improve backfilled files over time. When a human adds or corrects content in a backfilled file, they should use the `[MANUAL]` label to distinguish human-curated content from machine-reconstructed content. Example: `### [MANUAL] Chose Fastify for plugin ecosystem (2024-04-03)`. This preserves provenance: `[BACKFILLED]` = reconstructed from git, `[MANUAL]` = added by a human after the fact. Neither label should be removed — they are permanent epistemic markers.

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|-------------|---------|-----|
| Treating backfilled context as authoritative | It's reconstructed, not captured in real time | Always label `[BACKFILLED]` with confidence |
| Inventing decisions from ambiguous commits | Creates false confidence in rationale | Say "unknown rationale" when you don't know |
| Processing commits newest-first | Narrative doesn't make sense backwards | Always oldest-to-newest |
| Trying to analyze every commit in a huge repo | Takes forever; most commits are noise | Sample strategically; focus on high-signal commits |
| Overwriting existing memory bank content | Destroys manually-curated context | Append/augment only |
| Ignoring squash merges | Loses the context that was in the individual commits | Treat squash messages as era summaries |
| Skipping the backfill manifest | No provenance for the generated context | Always produce it |

---

## Output Format

When invoked via `/axiom-backfill-git`, produce:

1. **Backfill summary** — what was analyzed, how many commits, date range, eras detected
2. **Files created/updated** — paths to all generated memory bank files
3. **Confidence assessment** — overall and per-era
4. **Known gaps** — what couldn't be determined from git history alone
5. **Recommended next steps** — typically `/axiom-spec-extract` then `/axiom-onboarding`

---

## One-Line Reminder

Read the git log like a biography — oldest to newest — and give agents the context they'd have if they'd been there from the start.
