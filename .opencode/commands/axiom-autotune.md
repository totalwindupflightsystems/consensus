---
description: "AutoTune — autonomous configuration optimization via benchmark-driven parameter sweeps. Discovers optimal Axiom settings for a given model and workload."
agent: tower-axiom
---

Run AutoTune to discover optimal Axiom configuration settings for a given model and workload pattern.

Inputs:
- Subcommand: $SUBCOMMAND (required; one of: run, status, list, diff, apply, baseline, stale)
- Model: $MODEL (required for run/baseline; model ID to optimize for, e.g., claude-sonnet-4)
- Workload: $WORKLOAD (required for run/baseline; one of: concurrent-users, batch-ci, large-session, mixed)
- Objective: $OBJECTIVE (optional; one of: balanced, cost, latency, quality; default: balanced)
- Budget: $BUDGET (optional; e.g., 50-runs or 20-usd; default: 50-runs)
- Strategy: $STRATEGY (optional; one of: grid, random; default: random)
- Repeats: $REPEATS (optional; integer; default: 3)
- Profile: $PROFILE (required for diff/apply; profile name)
- Format: $FORMAT (optional; table or json; default: table)
- Dry run: $DRY_RUN (optional; true to preview without writing; default: false)
- Yes: $YES (optional; true to skip confirmation prompt; default: false)
- Force: $FORCE (optional; true to apply even with model mismatch; default: false)
- Parallel: $PARALLEL (optional; number of parallel workers; default: 1)
- Chaos: $CHAOS (optional; true to enable chaos fault injection; default: false)
- Apply: $APPLY (optional; true to apply best profile after sweep; default: false)
- Stale check: $STALE_CHECK (optional; true to skip sweep if profile is not stale; default: false)

Spec contract: `specs/93-Axiom-AutoTune.md`

Subcommands:
- `run` — Start a parameter sweep (REQ-CAT-072)
- `status` — Show active/running sweeps and progress (REQ-CAT-073)
- `list` — List stored profiles (REQ-CAT-053, REQ-CAT-077)
- `diff` — Show diff between profile and current config (REQ-CAT-054, REQ-CAT-075)
- `apply` — Apply a profile to axiom.config.yaml (REQ-CAT-055, REQ-CAT-056, REQ-CAT-076)
- `baseline` — Establish a baseline run with current config (REQ-CAT-074)
- `stale` — Check all profiles for staleness (REQ-CAT-060 through REQ-CAT-065)

Do:

1) **Parse arguments** from the user's message:
   - Extract the subcommand (run, status, list, diff, apply, baseline, stale)
   - Extract relevant flags based on the subcommand
   - If subcommand is missing, show available subcommands and ask the user

2) **Invoke the CLI**:
   ```bash
   axiom autotune <subcommand> [flags]
   ```

3) **For `run` subcommand**:
   - Validate that --model and --workload are provided
   - Check if a search space file exists at .axiom/autotune-search-space.yaml
   - If not, generate a default search space from the tunable registry
   - Invoke Morty with the autotune-sweep template
   - Report the sweep ID and expected profile location

4) **For `list` subcommand**:
   - List all profiles in .axiom/autotune-profiles/
   - Support --format json for machine-readable output

5) **For `diff` subcommand**:
   - Load the named profile from .axiom/autotune-profiles/
   - Compare against current .axiom/axiom.config.yaml (or baseline with --baseline)
   - Show parameter delta table

6) **For `apply` subcommand**:
   - Load the named profile (or best from most recent sweep if no --profile)
   - Check for model mismatch (REQ-CAT-057) — warn and require --force if different
   - Show preview of changes
   - Prompt for confirmation unless --yes
   - Write changes to axiom.config.yaml
   - Suggest commit message: `chore(config): apply autotune profile <name>`

7) **For `stale` subcommand**:
   - Check all profiles for staleness (model version, config schema, workload signature)
   - Report stale/not-stale status for each profile
   - Staleness is advisory — does not auto-invalidate profiles (REQ-CAT-062)

8) **Report results** to the user with:
   - Command output
   - Any warnings (model mismatch, high variance, stale profiles)
   - Next steps (e.g., "Run 'axiom autotune apply' to apply the best profile")

axiom:trace work_item=autotune-01 spec=specs/93-Axiom-AutoTune.md#REQ-CAT-070 jira_ref=DEX-438
