---
tags:
  vertical: [onboarding]
  category: onboarding
  core: false
---

You are guiding a user through installing OR upgrading Axiom in their existing project repository (or multi-repo workspace), then onboarding (filling templates) so agents can operate naturally. Axiom is a traceability-first "dev team in a box" that uses specialized AI agents, specs-as-contracts, and durable memory to coordinate development work.

## Workspace Detection (check this first)

Before starting any install, detect the user's context:

1. **Already a workspace**: If `workspace.yaml` exists in the current directory or an ancestor, this is already a multi-repo workspace. Follow workspace rules from `specs/40-Multi-Repo-Workspace.md`. Skip to per-repo onboarding.

2. **Looks like a workspace**: If the current directory (or the `--target` directory) contains 2+ git repos as direct children but no `workspace.yaml`, proactively suggest workspace setup:
   > "I see multiple git repos here. Would you like to set up a multi-repo workspace? This gives agents cross-repo visibility and unified configuration."
   >
   > If yes: `python3 <AXIOM_REPO>/.axiom/scaffold/workspace-setup.py --target <dir> --memory-bank-full --install-missing`
   >
   > If no: proceed with single-repo install into the specific repo the user wants.

3. **Single repo**: No sibling repos detected. Proceed with normal single-repo install.

This detection ensures users never need to know about workspace mode beforehand — the system discovers it and offers the right path.

## Overview

Axiom gets cloned/installed into an existing project. After the files land, you guide the user through filling in the templates so agents can operate naturally in their repo.

This skill covers three journeys:
- **Single-repo install**: add Axiom scaffolding and OpenCode infra to a repo that doesn't have it.
- **Multi-repo workspace setup**: create a workspace that aggregates multiple repos with shared agents, cross-repo specs visibility, and unified memory bank navigation. See [MULTI-REPO.md](MULTI-REPO.md) for full details.
- **Upgrade**: update an existing Axiom-enabled repo to a newer upstream version (without losing local repo-specific edits).

**Convergence guarantee**: Regardless of which install method is used (scaffold copy, submodule, template repo, or workspace setup), the user MUST end up at a point where:
1. All scaffold files exist and are valid
2. `.axiom/axiom.config.yaml` has a correct `repo_id` and the user has been asked about `jira.project_key`
3. Running `/axiom-setup` (the plugin's setup command, `specs/70-OpenCode-Plugin.md` REQ-PLG-011) validates everything and fills any gaps
4. Agents can operate — including Jira-triggered work if the project key is configured
5. Agents can discover the capability surface without guessing: `.opencode/skills/axiom-skill-map/` and `.opencode/prompts/skill-map-client.md` are installed, and `opencode.jsonc` includes `.opencode/prompts/skill-map-client.md` in its `instructions` array

## Portability Contract (Specs vs Skills)

Installed repos may NOT include the full Axiom `specs/` set.

Rules:
- If `specs/` contains real contracts for the target project, treat it as the contract.
- If `specs/` is missing or only contains stubs, treat these as the portable contracts:
  - `.opencode/skills/*/SKILL.md`
  - `.memory-bank/_prompt.md`
  - `.memory-bank/work-items/_prompt.md`
  - `.memory-bank/implementation-plans/_prompt.md`

## Prerequisites

Before running any installation method, ensure the following are installed on the host machine:

1. **OpenCode** — The AI coding agent runtime:
   ```bash
   curl -fsSL https://opencode.ai/install | bash
   ```

2. **Python 3.11+** — Required for running the install scripts and Axiom runtime.

3. **Git** — Required for repository operations and auto-deriving repo IDs.

## Installation Methods (choose one)

### Method 1: Scaffold Copy Install (standalone, self-contained)

Best for: repos that want a fully self-contained setup with no external dependencies.

```bash
python3 <AXIOM_REPO>/.axiom/scaffold/install.py --target /path/to/your-repo --memory-bank-full

This copies all scaffold templates + all `.opencode/` infrastructure files via **auto-discovery** — the installer walks `.opencode/` recursively and picks up all agents, commands, skills, and prompts automatically. No hardcoded file list. New skills and agents are included automatically.

Required routing assets are part of that auto-discovered `.opencode/` infrastructure. In particular, every install must include `.opencode/skills/axiom-skill-map/` and `.opencode/prompts/skill-map-client.md`, and the generated or synced `opencode.jsonc` must list the skill-map prompt in `instructions`. This lets agents load the skill map whenever routing is unclear instead of scanning every skill manually.

Current counts (as of last sync): **131 skills**, **40 agents**, **66 commands**.

Recommended default (`--memory-bank-full`) adds the extended Memory Bank skeleton (agents/inbox/projects/topics/best-practices/known-gaps). The base install always includes `requests/` because the root memory-bank index points to it.

Options:
- `--scaffold-only` — Only copies the core scaffold templates (skip `.opencode/` infra). Use when agents are already present.
- `--memory-bank-full` — Recommended default. Installs the full `.memory-bank/` folder skeleton (agents/inbox/projects/topics/best-practices/known-gaps) with `_index.md` and `_prompt.md` files. `requests/` is included even without this flag.
- `--force` — Overwrite existing files (default: skip existing).
- `--dry-run` — Print what would be created without writing.
- `--repo-id org/repo` — Override the auto-derived repository identifier.

## Syncing Upstream-Owned Files

After initial install, repos need a way to pull updated agents, skills, commands, and prompts from upstream Axiom without clobbering repo-specific files. The `sync-upstream.py` script handles this.

### What Gets Synced (upstream-owned, directly overwritable)

| Category | Path | Description |
|----------|------|-------------|
| `agents` | `.opencode/agents/` | Agent prompt definitions |
| `commands` | `.opencode/commands/` | Slash command definitions |
| `skills` | `.opencode/skills/` | Loadable skill instructions |
| `prompts` | `.opencode/prompts/` | Shared prompt fragments |
| `plugins` | `.opencode/plugins/` | OpenCode plugin files (fork bomb guard, compaction hook, graph harness, session tools, Axiom integration) |
| `opencode-config` | `opencode.jsonc` | OpenCode runtime configuration |

### What Does NOT Get Synced (repo-owned, never overwritten)

| Path | Why |
|------|-----|
| `.memory-bank/**` | Repo-specific context, work items, decisions |
| `specs/**` | Repo-specific contracts |
| `.axiom/axiom.config.yaml` | Repo-specific configuration (repo_id, jira key) |
| `.axiom/command-registry.yaml` | Repo-specific command registry |
| `AGENTS.md` | Repo-specific agent rules |

### Usage

```bash
# From a repo with .axiom-upstream/ submodule (auto-detects source):
python3 .axiom-upstream/scripts/sync-upstream.py

# Explicit source (e.g., a local Axiom checkout):
python3 /path/to/Axiom/scripts/sync-upstream.py --target /path/to/repo

# Dry run first (always recommended):
python3 .axiom-upstream/scripts/sync-upstream.py --dry-run

# Show diffs of what would change:
python3 .axiom-upstream/scripts/sync-upstream.py --diff

# Sync only specific categories:
python3 .axiom-upstream/scripts/sync-upstream.py --only agents,skills,plugins

# Skip specific categories:
python3 .axiom-upstream/scripts/sync-upstream.py --skip commands
```

### Pinning Files (local customizations)

If a repo needs to customize an upstream-owned file (e.g., a specific agent prompt), it can **pin** that file to prevent sync from overwriting it.

Create `.axiom/pinned-files.txt` with one relative path per line:

```text
# Pinned files — these are locally customized and should not be synced from upstream.
# Document why each file is pinned.

# Custom tower agent with project-specific routing rules
.opencode/agents/tower-axiom.md

# Custom /axiom-step command with extra validation
.opencode/commands/axiom-step.md
```

Pinned files are skipped during sync and reported in the output. Use `--list-pinned` to see all pinned files and their status.

**Rule**: When you pin a file, you own it. Upstream improvements to that file will not reach your repo until you manually merge them. Document why each file is pinned so future maintainers understand the divergence.

### Recommended Sync Workflow

For repos using the **submodule method** (Method 2):

```bash
# 1. Update the submodule to latest upstream
git submodule update --remote --merge .axiom-upstream

# 2. Dry-run the sync to see what changed
python3 .axiom-upstream/scripts/sync-upstream.py --dry-run --diff

# 3. Run the sync
python3 .axiom-upstream/scripts/sync-upstream.py

# 4. Review and commit
git add .opencode/ opencode.jsonc
git commit -m "chore: sync Axiom upstream (agents, skills, commands)"
```

For repos using the **scaffold copy method** (Method 1):

```bash
# 1. Sync from a local Axiom checkout
python3 /path/to/Axiom/scripts/sync-upstream.py --target .

# 2. Review and commit
git add .opencode/ opencode.jsonc
git commit -m "chore: sync Axiom upstream (agents, skills, commands)"
```

## Upgrade (full version upgrade)

Upgrade notes (Method 1):
- Use `--dry-run` first to see what would change.
- By default, existing files are skipped; upgrading typically requires `--force` for files you want to refresh from upstream.
- If the target repo has local edits to Axiom files, treat upgrade like a normal code change:
  - run `git status`/`git diff`
  - review diffs
  - commit locally
  - only then proceed with the next upgrade step

## Post-Install Validation

After any installation method, verify these core scaffold files exist:

```
.axiom/axiom.config.yaml
.axiom/command-registry.yaml
.memory-bank/_index.md
.memory-bank/_prompt.md
.memory-bank/implementation-plans/_index.md
.memory-bank/implementation-plans/_prompt.md
.memory-bank/projectBrief.md
.memory-bank/productContext.md
.memory-bank/systemPatterns.md
.memory-bank/techContext.md
.memory-bank/decisionLog.md
.memory-bank/activeContext.md
.memory-bank/progress.md
.memory-bank/TODO.md
.memory-bank/requests/_index.md
.memory-bank/requests/_prompt.md
.memory-bank/work-items/_index.md
.memory-bank/work-items/_prompt.md
.memory-bank/work-items/_current.md
.memory-bank/work-items/onboarding-01/meta-planning.md
.memory-bank/work-items/onboarding-01/plan.md
.memory-bank/work-items/onboarding-01/plan.yaml
.memory-bank/work-items/onboarding-01/verification.md
AGENTS.md
specs/README.md
specs/_index.md
specs/_prompt.md
specs/00-PRD.md
specs/01-Architecture.md
```

If you used `--memory-bank-full`, also verify the Memory Bank skeleton exists:

```
.memory-bank/agents/_index.md
.memory-bank/agents/_prompt.md
.memory-bank/inbox/_index.md
.memory-bank/inbox/_prompt.md
.memory-bank/projects/_index.md
.memory-bank/projects/_prompt.md
.memory-bank/topics/_index.md
.memory-bank/topics/_prompt.md
.memory-bank/best-practices/_index.md
.memory-bank/best-practices/_prompt.md
.memory-bank/known-gaps/_index.md
.memory-bank/known-gaps/_prompt.md
.memory-bank/known-gaps/TODO.md
.memory-bank/findings/_index.md
.memory-bank/findings/_prompt.md
```

If any are missing, re-run the installer or create them manually from the templates in `.axiom/scaffold/templates/`.

Also verify these required OpenCode routing assets exist and are wired:

```
.opencode/skills/axiom-skill-map/SKILL.md
.opencode/skills/axiom-skill-map/tree.yaml
.opencode/prompts/skill-map-client.md
```

And confirm `opencode.jsonc` includes `.opencode/prompts/skill-map-client.md` in the `instructions` array. If it is missing, add it during install or sync before claiming the repo is Axiom-ready.

### Post-Install Configuration Validation

After scaffold files are verified, also check these configuration values in `.axiom/axiom.config.yaml`:

| Field | Expected State | Action if Wrong |
|---|---|---|
| `repo_id` | `org/repo` (not `REPO_ID_PLACEHOLDER`) | Derive from `git remote` or ask user |
| `jira.project_key` | Valid Jira key (e.g., `"PROJ"`) or `null` (if no Jira) | Ask user during Step 8 |
| `jira.legacy_project_keys` | Array of `{key, migrated_date, search_hint}` or `[]` | Ask user if they migrated from another project |
| `jira.cross_board_projects` | Array of `{key, relationship, can_create}` or `[]` | Ask user if they work with other Jira boards |
| `jira.hierarchy.initiative_type` | `"Initiative"` (or project-specific name) | Set during install — ask user what their top-level type is called |
| `jira.hierarchy.require_epic_for_tasks` | `true` (recommended) | Agents fail closed when creating tasks without an epic |
| `confidence.weights` | Sum to 100 | Use defaults if missing |
| `git.co_author` | `"Axiom Agent <svc_axiom@dexdat.ai>"` | Set during install — this is the co-author trailer added to all AI-authored commits |

Also verify the fork bomb protection stack is in place:

| Check | Command | Expected |
|---|---|---|
| All 5 plugins present | `ls .opencode/plugins/*.ts \| wc -l` | `5` |
| Agent depth guard plugin | `ls .opencode/plugins/agent-depth-guard.ts` | File exists |
| Axiom plugin entry | `ls .opencode/plugins/axiom.ts` | File exists |
| Compaction hook | `ls .opencode/plugins/compaction.ts` | File exists |
| Graph harness | `ls .opencode/plugins/graph-harness.ts` | File exists |
| Session tools | `ls .opencode/plugins/opencode-session.ts` | File exists |
| doom_loop deny | `grep '"doom_loop"' opencode.jsonc` | `"doom_loop": "deny"` |
| Self-spawn deny (sample) | `grep '"dev-axiom": deny' .opencode/agents/dev-axiom.md` | Line present |
| Bottom guard (sample) | `grep "Last Line of Defense" .opencode/agents/qa-axiom.md` | Line present |

This ensures agents can operate correctly from the first run — especially for Jira-triggered work where the project key is required for auto-constructing issue keys and posting progress comments.

## Runtime Services Setup (Go Binaries + Daemons)

Axiom includes several Go-based backend services that must be **built from source** and **started as daemons** for the full plugin surface to work. These are NOT started automatically by OpenCode — agents must build and start them.

### Finding Go

Go is typically NOT in the default PATH on Axiom workspaces. Discovery order:

```bash
# 1. Check for toolchain in GOPATH cache
GO=$(find /home/coder -path "*/toolchain@v0.0.1-go1.25*.linux-amd64/bin/go" 2>/dev/null | head -1)
# 2. Check _tmp/go-install
GO="${GO:-$(find . -path "*/_tmp/go-install/go/bin/go" 2>/dev/null | head -1)}"
# 3. Check _tmp/gopath (Axiom standard)
GO="${GO:-$(find . -path "*/_tmp/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.25*/bin/go" 2>/dev/null | head -1)}"
# 4. System go (if installed)
GO="${GO:-$(which go 2>/dev/null)}"

echo "Go binary: $GO"
$GO version
```

**Common Axiom workspace path**: `_tmp/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.linux-amd64/bin/go`

### Service: ShellOps Daemon

The ShellOps daemon provides terminal management, action classification, log intelligence, event listeners, and triage for ops workflows. Required for all `shellops_*` MCP tools to function.

```bash
# Build
export GOPATH=$(pwd)/_tmp/gopath
cd shellops && $GO build -o ../_tmp/shellops-bin ./cmd/shellops/ && cd ..

# Start (MUST use subshell pattern to survive tool timeouts)
(_tmp/shellops-bin start --port 9876 --root . >> _tmp/shellops-daemon.log 2>&1 &)
sleep 2

# Verify
curl -s http://127.0.0.1:9876/health
# Expected: {"status":"ok"}
```

**Port**: 9876 (env: `SHELLOPS_PORT`)
**Config**: `.shellops/config.yaml` (action classification rules, resilience budgets)
**DB**: `.shellops/shellops.db` (auto-created, gitignored)
**Skill**: Load `shellops-axiom` for full docs

### Service: Code Intelligence (axiom-code-intel)

The code-intel binary provides structural code analysis: call graphs, blast radius, symbol search, and change impact detection. Required for the `code-intel` OpenCode tool.

```bash
# Build
cd code-intel && $GO build -o ../_tmp/axiom-code-intel ./cmd/axiom-code-intel/ && cd ..

# Verify
export PATH="$PATH:$(pwd)/_tmp"
axiom-code-intel status --repo .
# Expected: file_count + symbol_count output
```

**Note**: The `code-intel` OpenCode tool discovers the binary via PATH. Either symlink it to a PATH directory or always set PATH before invoking. In automated contexts, the binary at `_tmp/axiom-code-intel` is the standard location.

**Skill**: Load `code-graph-intelligence-axiom` for full docs

### Service: Graph Harness (no separate daemon)

The Graph Harness runs inside the OpenCode plugin process — no separate build needed. Its SQLite DB (`.graph-harness/harness.db`) is created on first `graph_create` tool call.

**Templates**: `.graph-harness/templates/*.yaml` (pre-loaded ops investigation templates)
**Skill**: Load graph harness docs via `specs/102-Graph-Harness.md`

### Service: Tree Memory (no separate daemon)

Tree Memory runs inside the OpenCode plugin process. Its git repository (`.tree-memory/repo/`) is created on first `tree.init` tool call.

**DuckDB**: Embedded in the plugin — no separate install needed (bun bundles it)
**Skill**: Load `tree-memory-axiom` for full docs

### Service: Conductor (no separate daemon)

Conductor runs inside the OpenCode plugin process. Its SQLite DB (`.conductor/conductor.db` or `.graph-harness/harness.db`) is created on first `conductor.spawn` call.

**Skill**: Load context-stash skill for config patterns

### Starting All Services (single script)

For convenience, run this after a fresh workspace start:

```bash
#!/bin/bash
# _tmp/start-services.sh — Start all Axiom runtime services
set -e

# Find Go
GO=$(find /home/coder -path "*/toolchain@v0.0.1-go1.25*.linux-amd64/bin/go" 2>/dev/null | head -1)
export GOPATH=$(pwd)/_tmp/gopath

# Build ShellOps (if binary missing or stale)
if [ ! -f _tmp/shellops-bin ] || [ shellops/internal/daemon/daemon.go -nt _tmp/shellops-bin ]; then
  echo "Building ShellOps..."
  (cd shellops && $GO build -o ../_tmp/shellops-bin ./cmd/shellops/)
fi

# Build code-intel (if binary missing)
if [ ! -f _tmp/axiom-code-intel ]; then
  echo "Building code-intel..."
  (cd code-intel && $GO build -o ../_tmp/axiom-code-intel ./cmd/axiom-code-intel/)
fi

# Start ShellOps daemon
if ! curl -s http://127.0.0.1:9876/health > /dev/null 2>&1; then
  echo "Starting ShellOps daemon..."
  (_tmp/shellops-bin start --port 9876 --root . >> _tmp/shellops-daemon.log 2>&1 &)
  sleep 2
fi

# Verify
echo "=== ShellOps ===" && curl -s http://127.0.0.1:9876/health
echo ""
echo "=== Code Intel ===" && _tmp/axiom-code-intel version 2>/dev/null || echo "Binary built at _tmp/axiom-code-intel"
echo ""
echo "All services ready."
```

### Important: Shell Timeout Gotcha

When starting long-running processes from AI agent tool calls, the shell session has a 120-second timeout. Any process started via `nohup`, `setsid`, or `&` that is a child of the shell will receive SIGTERM when the timeout fires.

**The fix**: Always wrap daemon starts in a parenthesized subshell: `(cmd &)`. This creates a new process group that is NOT killed when the parent shell exits.

```bash
# BAD — daemon dies after 120s:
nohup _tmp/shellops-bin start --port 9876 --root . &

# GOOD — daemon survives:
(_tmp/shellops-bin start --port 9876 --root . >> _tmp/shellops-daemon.log 2>&1 &)
```

## Post-Upgrade Validation (required)

After an upgrade, do these checks before claiming success:

```bash
# Core repo guards (Axiom repos)
python3 scripts/check_todo_plan_parity.py
python3 scripts/check_no_stubs.py
python3 scripts/check_pass_gate_evidence.py

# If the repo has a runtime/test suite
cd .axiom && .venv/bin/pytest -q
```

If any of these fail, do not continue upgrading unrelated areas; fix the failure first.

## One-Command Bootstrap (Recommended)

Once the scaffold files exist, a repo can be bootstrapped into a "ready to run" planned state from a single OpenCode command:

```text
/axiom-bootstrap
```

This command is expected to:
- Ensure `.memory-bank/implementation-plans/` and `.memory-bank/work-items/` navigation prompts exist.
- Create/update `.memory-bank/TODO.md` from whatever specs/docs exist in the repo.
- Create/update `.memory-bank/implementation-plans/` aligned to TODO.
- Create a usable onboarding work item plan and a default `.memory-bank/work-items/_current.md`.
- Generate a runnable Ralph loop scaffold via `/ralph-wiggum-loop`.

## Workflow Commands (Optional)

If `/axiom-kickoff` is installed, it can be used to create specs + work item + roadmap + loop scaffold in one command:

```text
/axiom-kickoff <request>
```

## Post-Install Onboarding (guided conversation)

After the files land, walk the user through these steps in order. Ask questions conversationally to populate each template file.

### Step 1: Project Brief (`.memory-bank/projectBrief.md`)

Ask the user:
1. What is this project and why does it exist?
2. What are the primary goals?
3. What is explicitly out of scope (non-goals)?
4. How do you define "done" for a work item?
5. What are the key integrations (CI, issue tracker, hosting)?

Fill in the template sections with their answers.

### Step 2: Product Context (`.memory-bank/productContext.md`)

Ask the user:
1. What problem does this solve and who has it?
2. How should users interact with the product?
3. How does it work at a high level?
4. What external systems does it integrate with?
5. Are there domain-specific terms I should know?

### Step 3: System Patterns (`.memory-bank/systemPatterns.md`)

Ask the user (or discover by reading their codebase):
1. What is the architecture? (monolith, microservices, serverless, etc.)
2. What design patterns does the codebase follow?
3. How does data flow through the system?
4. How are errors handled?
5. What is the testing strategy? (unit, integration, e2e)

### Step 4: Tech Context (`.memory-bank/techContext.md`)

Ask the user (or discover from package files):
1. What languages and frameworks are used?
2. What build/dev tools are in the stack?
3. What are the key dependencies and why?
4. How do you set up a local dev environment?
5. How is the project deployed?

### Step 5: Decision Log (`.memory-bank/decisionLog.md`)

Ask the user:
1. Are there any important technical decisions already made that agents should know about?
2. Any decisions that were contentious or non-obvious?

Record each as a decision entry with context, decision, alternatives, and consequences.

### Step 6: Active Context (`.memory-bank/activeContext.md`)

Ask the user:
1. What is the team currently working on?
2. Any open questions or pending decisions?
3. Any active constraints or blockers?
4. How should an agent think about this repo right now?

### Step 7: TODO (`.memory-bank/TODO.md`)

Ask the user:
1. What are the most important things to work on right now?
2. What is coming up next?
3. Anything that is parked or deferred?

### Step 8: Configuration Review (`.axiom/axiom.config.yaml`)

Check that `repo_id` was populated correctly (not still `REPO_ID_PLACEHOLDER`). If it is still the placeholder, derive from `git remote` or ask the user.

#### Jira Project Key Configuration

The Jira project key is critical for agents to manage work items, post progress comments, and transition tickets. Each repo has its own project key in `.axiom/axiom.config.yaml` under `jira.project_key`.

Ask the user:
1. "Do you use Jira for work tracking?" — If no, leave `jira.project_key: null` and move on.
2. "What's your Jira project key?" — This is the short prefix for tickets (e.g., `PROJ` for `PROJ-123`, `BE` for `BE-456`). You can find it in Jira by looking at any ticket number.
3. Validate the key matches standard Jira format: uppercase letters/numbers, 2-10 characters (e.g., `PROJ`, `BACKEND`, `FE`).
4. Write the key to `.axiom/axiom.config.yaml` under `jira.project_key`.

**Multi-repo workspaces**: Different repos can (and often do) use different Jira projects. For example:
- `frontend/` → `jira.project_key: "FE"`
- `backend/` → `jira.project_key: "BE"`
- `api-gateway/` → `jira.project_key: "API"`

When onboarding a multi-repo workspace, ask for each repo's Jira project key separately. Some repos may share a project key; others may have their own.

**Why this matters**: When `jira.project_key` is set, agents can:
- Auto-construct Jira issue keys (e.g., `PROJ-123`) for trace markers
- Post progress comments to the correct Jira project
- Transition tickets through workflow states
- Use JQL to discover assigned work

When `jira.project_key` is `null`, agents still work but require explicit `jira_ref` values in intake — they cannot auto-discover or auto-construct Jira keys.

#### Legacy Project Keys

When a team migrates from one Jira project to another (e.g., `DEX` → `SWDE`), the old project still has historical tickets that agents need to search. Configure legacy projects so agents know where to look for history without creating new tickets there.

Ask the user:
1. "Have you migrated from a previous Jira project?" — If no, leave `jira.legacy_project_keys: []`.
2. "What was the old project key?" — Add it to the array.
3. "When did you migrate?" — Record the date so agents know the cutoff.

```yaml
jira:
  legacy_project_keys:
    - key: "OLD_PROJ"
      migrated_date: "2026-01-15"
      search_hint: 'labels = "OLD_PROJ" OR text ~ "OLD_PROJ-"'
```

**Rules for agents**:
- NEVER create new tickets in legacy projects
- Search legacy projects for historical context (trace markers, related work)
- When referencing old tickets, use the full key (e.g., `OLD_PROJ-123`) in trace markers
- Update/close existing legacy tickets if needed (e.g., marking as superseded)

#### Cross-Board Projects

Many teams work across multiple Jira boards. A backend repo might file infra tickets in `POP`, security tickets in `SEC`, and its own work in `BE`. Configure cross-board projects so agents know which boards they can interact with and how.

Ask the user:
1. "Does this repo work with other Jira projects?" — If no, leave `jira.cross_board_projects: []`.
2. For each related project:
   - "What's the project key?" (e.g., `POP`, `SSE`)
   - "What's the relationship?" (e.g., "Platform/infra", "Shared services")
   - "Can Axiom create tickets there, or just search/link/comment?" — Set `can_create` accordingly.

```yaml
jira:
  cross_board_projects:
    - key: "POP"
      relationship: "Platform/infra — IAM, AWS, CI/CD"
      can_create: true
    - key: "SSE"
      relationship: "Shared services — MCP servers, integrations"
      can_create: false
```

**Rules for agents**:
- Search cross-board projects when investigating related work
- Link tickets across boards when work spans projects
- Only create tickets in cross-board projects when `can_create: true` AND the work clearly belongs there
- Always apply `repo_label` when creating tickets in cross-board projects so they're traceable back to this repo

#### Git Co-Author Configuration

Set the AI co-author identity for all agent-authored commits:

```yaml
git:
  co_author: "Axiom Agent <svc_axiom@dexdat.ai>"
```

This trailer is automatically appended to every commit made by or with an AI agent:

```
Co-authored-by: Axiom Agent <svc_axiom@dexdat.ai>
```

**Why this matters**: Git history should clearly show which commits involved AI assistance. The co-author trailer:
- Makes AI contributions visible in `git log`, GitHub, and Jira
- Enables filtering AI-assisted commits (`git log --grep="Co-authored-by: Axiom"`)
- Provides attribution for compliance and audit purposes
- Uses a service account email (`svc_axiom@dexdat.ai`) so it's distinct from human authors

**Default**: `"Axiom Agent <svc_axiom@dexdat.ai>"`. Organizations can customize the name and email to match their service account conventions.

Reference: `specs/06-Project-Configuration.md` (jira section), `specs/05-Jira-Integration.md` (setup requirements), `specs/70-OpenCode-Plugin.md` (REQ-PLG-046, REQ-PLG-047)

#### Other Configuration

Review confidence weights and thresholds with the user if they have specific preferences.

Check GitHub integration settings:
- `github.default_base_branch` — defaults to `main`; ask if the repo uses a different default branch
- `github.branch_prefix` — defaults to `axiom/`; some teams prefer a different prefix

### Step 9: AGENTS.md Review

The scaffolded `AGENTS.md` is minimal. If the project has specific conventions (code style, testing requirements, forbidden patterns), help the user add them to `AGENTS.md` so all agents respect them.

**CLAUDE.md symlink pattern**: Some teams use `CLAUDE.md` instead of `AGENTS.md` (e.g., Anthropic Claude projects). When installing into such a repo:
1. Check if `CLAUDE.md` exists at the repo root.
2. If `CLAUDE.md` exists and `AGENTS.md` does not: create `AGENTS.md` with the full content, then symlink `CLAUDE.md → AGENTS.md`:
   ```bash
   ln -sf AGENTS.md CLAUDE.md
   ```
3. If both exist: merge content into `AGENTS.md`, then symlink `CLAUDE.md → AGENTS.md`.
4. This ensures both tools see the same rules without duplication.

**Required sections in AGENTS.md** (verify these are present):
- "Findings & Self-Improvement" block at the top — points to `.memory-bank/findings/_index.md`
- "Adversarial Quality Agents" section — lists `@assumption-buster-axiom`, `@devils-advocate-axiom`, `@redteam-axiom`, `@whitehat-axiom`

### Step 9b: OpenCode Plugins (auto-loaded — verify all are present)

OpenCode auto-loads every `.ts` file found in `.opencode/plugins/` — no `opencode.jsonc` configuration needed. The install copies this directory automatically.

Five plugins ship with Axiom:

| File | Purpose |
|---|---|
| `axiom.ts` | Main Axiom integration — re-exports `CodeOpsPlugin` from `.axiom/plugin/dist/index.js`. Requires the plugin to be built (Step 10b). |
| `agent-depth-guard.ts` | Fork bomb protection (Layer 4) — tracks Task tool calls per session, warns at 5 rapid-fire calls, blocks at 15 total. |
| `compaction.ts` | Injects `.opencode/prompts/compaction.md` as context during session compaction so critical rules survive context truncation. |
| `graph-harness.ts` | Graph-driven execution engine — the model defines directed graphs of work; the harness drives deterministic execution. |
| `opencode-session.ts` | Standalone session tools — spawn, message, interrupt, inspect, and list OpenCode sessions from agent context. |

**Verify all five are present after install:**
```bash
ls .opencode/plugins/
# Expected: agent-depth-guard.ts  axiom.ts  compaction.ts  graph-harness.ts  opencode-session.ts
```

If any are missing, the `.opencode/` infrastructure was not installed (scaffold-only mode). Re-run without `--scaffold-only`, or copy the missing files from the Axiom source repo. If syncing an existing install, run:
```bash
python3 /path/to/Axiom/scripts/sync-upstream.py --only plugins
```

### Step 10: OpenCode Plugin Dependencies (MUST run AFTER plugins are installed)

The `.opencode/plugins/*.ts` files import from `@opencode-ai/plugin` (the OpenCode SDK). This package must be installed **locally** in `.opencode/node_modules/` AFTER the Axiom plugin files are copied to `.opencode/plugins/`.

**Why the ordering matters**: `.opencode/package.json` declares `"@opencode-ai/plugin": "1.4.9"` as a dependency. The plugins (graph-harness.ts, opencode-session.ts, future context-stash.ts, conductor.ts) all do `import { tool } from "@opencode-ai/plugin"`. If you try to validate or run the plugins before this install, they fail with "Cannot find module @opencode-ai/plugin".

The installer automatically runs:
```bash
cd .opencode && npm --registry https://registry.npmjs.org install
```

If it was skipped (e.g., `--skip-npm-install` was passed, or npm wasn't available), run it manually:
```bash
cd .opencode && npm --registry https://registry.npmjs.org install
```

> **Note**: The `--registry` flag ensures the package is fetched from the **public** npm registry. This is required in environments where a private/company npm registry (Artifactory, CodeArtifact, Verdaccio) is configured as the default, since `@opencode-ai/plugin` is only published to the public registry. Without this flag, `npm install` will attempt to resolve from your configured registry and fail with 404.

Verify the install succeeded:
```bash
ls .opencode/node_modules/@opencode-ai/plugin/package.json && echo "OK"
```

**Correct ordering recap**:
1. Copy/scaffold `.opencode/` directory (plugins, package.json, agents, etc.)
2. `cd .opencode && npm --registry https://registry.npmjs.org install` ← resolves @opencode-ai/plugin
3. Build the Axiom plugin (Step 10b below)

### Step 10b: Build the Axiom Plugin (automated by install.py)

The installer automatically runs `bun install && bun run build` in `.axiom/plugin/`. If it was skipped (e.g., `--skip-plugin-build` was passed, or bun wasn't available), run it manually:

```bash
cd .axiom/plugin && bun install && bun run build
```

Verify the build succeeded — `dist/index.js` must exist:
```bash
ls .axiom/plugin/dist/index.js
```

If `bun` is not installed:
```bash
curl -fsSL https://bun.sh/install | bash
cd .axiom/plugin && bun install && bun run build
```

> **Why this is required**: The `.opencode/plugins/axiom.ts` file imports from `.axiom/plugin/dist/index.js` (compiled JavaScript). The `dist/` directory is gitignored and must be built locally. Re-run `bun run build` in `.axiom/plugin/` whenever the plugin source changes.

### Step 11: Plugin Setup Validation (`/axiom-setup`)

After the plugin is installed and scaffold files are in place, run the plugin's setup command to validate and complete the configuration:

```
/axiom-setup
```

This command (defined in `specs/70-OpenCode-Plugin.md` REQ-PLG-011) performs a comprehensive check:
1. Detects repo type and maturity level
2. Validates scaffold files are present (fills gaps if any are missing)
3. Validates `.axiom/axiom.config.yaml` — checks `repo_id`, `jira.project_key`, confidence weights
4. Prompts for any missing integration config (Jira project key, GitHub settings) if not already set during Steps 1-8
5. Runs health checks on available backends
6. Reports status and suggests next steps

**This is the convergence point**: regardless of which install method was used (scaffold copy, submodule, template repo, or workspace setup), running `/axiom-setup` ensures the repo is fully configured and agents can operate. It is idempotent — running it on an already-configured repo only fills gaps, never overwrites.

**Why this matters for Jira**: If the user installed via `install.py` (which does NOT prompt for Jira config), `/axiom-setup` will detect that `jira.project_key` is `null` and offer to set it. This ensures every install path leads to a properly configured repo.

### Step 12: Worktree Directory Setup (automated by install.py)

The installer automatically creates `.worktrees/` with `_prompt.md` and `_index.md`, and adds `.worktrees/*/` to `.gitignore`. If it was skipped, create manually:

```bash
mkdir -p .worktrees
```

The installer creates two governance files:
- `.worktrees/_prompt.md` — Rules for creating, naming, merging, and cleaning up worktrees
- `.worktrees/_index.md` — Active worktree inventory (starts empty)

And adds these `.gitignore` entries:
```
.worktrees/*/
_tmp/
```

### Step 13: First Test Run

Suggest a simple first task to verify everything works end-to-end:

```
Use tower-axiom. Create a tiny plan to add a hello-world text file, execute one baby step, verify it, and update .memory-bank with evidence.
```

This validates: agent loading, memory bank reads/writes, plan generation, step execution, verification, and evidence recording.

Alternative (fastest): run `/axiom-bootstrap` in OpenCode, then run the generated Ralph loop.

## Troubleshooting

| Issue | Fix |
|---|---|
| `repo_id` still shows `REPO_ID_PLACEHOLDER` | Edit `.axiom/axiom.config.yaml` and replace with `org/repo` |
| `jira.project_key` is `null` and agents can't auto-construct Jira keys | Edit `.axiom/axiom.config.yaml` and set `jira.project_key` to your Jira project key (e.g., `"PROJ"`) |
| Different repos need different Jira projects | Each repo has its own `.axiom/axiom.config.yaml` — set `jira.project_key` per repo |
| Missing `.opencode/` files after scaffold-only | Re-run without `--scaffold-only` or use the symlink method |
| Agents can't find memory bank | Verify `.memory-bank/_index.md` exists and lists all files |
| `TODO.md` missing | Create from template or re-run installer with `--force` on that file |
| Symlink shows BROKEN | Re-run `link-install.py --verify` and check the source path exists |
| Upgrade overwrote local customizations | Restore from git, then re-apply changes by copying into repo-owned paths (avoid editing symlink targets) |
| OpenCode reports missing base packages / "Cannot find module @opencode-ai/plugin" | Run: `cd .opencode && npm --registry https://registry.npmjs.org install` (local install, not global) |

## Hardening Skills (Codebase Audit Battery)

Axiom ships a set of 8 portable hardening skills that can audit any codebase for systemic risks. They work in any repo — not just Axiom-managed ones. Load `hardening-intake-axiom` first to understand how to run the full battery; it explains sequencing, output format, and how to triage findings. Load `hardening-anti-patterns-axiom` as the master catalog — it provides the shared audit header and cross-cutting anti-pattern taxonomy that all other skills reference.

<!-- axiom:trace work_item=hardening-skills-01 spec=axiom-install jira_ref=SWDE-7 plan=phase-1/task-1/step-3 -->

| Skill | Purpose | Load order |
|---|---|---|
| `hardening-intake-axiom` | Entry point — how to run the battery, sequencing, output format | Load first (to understand the process) |
| `hardening-anti-patterns-axiom` | Master catalog — shared audit header, cross-cutting anti-patterns | Load first (before domain skills) |
| `hardening-spof-axiom` | Single point of failure detection | Domain skill |
| `hardening-security-axiom` | Security audit (auth, secrets, injection, CVEs) | Domain skill — **always `requires_human_review: true`** |
| `hardening-database-axiom` | DB/data layer risks (migrations, N+1, locking) | Domain skill |
| `hardening-sre-axiom` | Reliability/SRE (SLOs, runbooks, blast radius) | Domain skill |
| `hardening-quality-axiom` | Test coverage gaps and quality anti-patterns | Domain skill |
| `hardening-observability-axiom` | Observability gaps (metrics, traces, alerts) | Domain skill |

> **Note**: Security and migration findings always require `requires_human_review: true` — never auto-apply them. The skills themselves contain the full audit checklists and output schemas.

## References

- `.axiom/scaffold/install.py` — Copy installer (full vs scaffold-only)
- `.axiom/scaffold/workspace-setup.py` — Multi-repo workspace setup tool
- `scripts/link-install.py` — Symlink install script (when using a submodule checkout)
- `scripts/sync-upstream.py` — Selective sync of upstream-owned files (agents, skills, commands, prompts)
- `scripts/sync_template.py` — Template sync helper (used in Axiom itself)
- `.axiom/scaffold/MANIFEST.md` — Complete installed file inventory
- `.memory-bank/_prompt.md` — Memory Bank rules (portable, installed into the target repo)
- `.memory-bank/_index.md` — Memory Bank inventory (portable, installed into the target repo)
- `specs/40-Multi-Repo-Workspace.md` — Multi-repo workspace spec (layout, symlinks, agent guidance, trace markers)
