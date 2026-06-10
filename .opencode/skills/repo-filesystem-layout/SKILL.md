---
name: repo-filesystem-layout
description: Audit and (re)generate a repository filesystem layout spec (what belongs where), and provide safe placement rules for new files.
version: "1.1"
tags:
  vertical: [onboarding, coding]
  category: onboarding
  core: false
---

# Repo Filesystem Layout (Portable)

This skill helps keep repository structure clean, discoverable, and stable over time.

Use it when:
- you need to add new files or directories that are not already planned,
- you suspect documentation drift between the actual repo tree and the documented layout,
- you want an explicit, auditable rule-set for where new artifacts belong.

## Principles

1) Align to the repo's established conventions
- Prefer existing patterns over inventing a new layout.

2) Document before refactor
- If files are "misplaced" but stable, prefer updating the layout documentation first.
- Move/rename only when the current placement is actively harmful or confusing.

3) Be explicit about scope
- This skill documents *this repo's* layout. Do not assume Axiom's layout applies.

4) Keep it navigable
- Every directory described should have: purpose, what belongs, what does not belong.

## Inputs

You may receive:
- a target repo path (default: current repo root)
- a preferred layout spec path (optional)
- constraints (read-only, no breaking changes, etc.)

If inputs are missing, make safe defaults:
- Repo root: `git rev-parse --show-toplevel` (or current working directory if not a git repo)
- Layout spec path:
  - If `specs/18-Repo-Filesystem-Layout.md` exists, update it.
  - Else, if `specs/` exists, create `specs/18-Repo-Filesystem-Layout.md`.
  - Else, create `docs/repo-filesystem-layout.md`.

## Required Outputs

1) A filesystem layout doc that includes:
- Root layout summary (top-level directories and key files)
- Per-directory sections describing purpose and placement rules
- "Adding New Files" placement rules
- A drift note when you find mismatches

2) Inventory linkage (fail-closed if you create a new doc)
- If you create a new spec under `specs/`, add it to `specs/README.md` (inventory) and/or `specs/_index.md` if present.
- If you create a new doc under `docs/`, link it from `docs/README.md` and `docs/_index.md` if present.

3) Traceability marker

Add a single-line marker near the top of the updated/created layout doc:

`axiom:trace work_item=<ID> spec=<layout-doc-path> plan=<phase/task/step> doc=<layout-doc-path> evidence=<path> commit=`

If the work item id is unknown, use a stable placeholder (do not invent Jira keys):
- `work_item=filesystem-layout-01`

## Procedure (Deterministic)

### Step 1: Discover the current layout

Collect evidence (do not guess):
- Top-level directories and files
- Presence of common anchors:
  - `README.md`, `docs/`, `specs/`
  - `.opencode/`, `.memory-bank/`, `.axiom/`
  - language/tool roots (e.g., `src/`, `tests/`, `ui/`, `scripts/`)

### Step 2: Find existing layout documentation

Prefer updating existing docs over creating new ones.
Check in order:
1. A dedicated layout spec/doc (examples: `specs/*filesystem*layout*`, `docs/*filesystem*layout*`)
2. The root `README.md` (often contains a "Repository Shape" section)
3. Any contributor docs (`CONTRIBUTING.md`)

### Step 3: Reconcile drift (doc vs repo)

List drift items explicitly (do not silently "fix"):
- New directories that are undocumented
- Documented directories that no longer exist
- Moved responsibilities (e.g., docs moved from README into `docs/`)

Resolution rules:
- If the repo structure is correct and stable: update the doc.
- If the structure is clearly wrong *and* easy to fix safely: move files and update the doc.
- If ambiguous: document the ambiguity and add an "Open decision" line.

### Step 4: Write placement rules

Add a short section that answers: "If I need to add X, where does it go?"

Recommended categories:
- Contracts/specs
- User docs
- Runtime implementation
- Tooling and scripts
- Tests
- Generated artifacts
- UI assets

### Step 5: Update inventories

Update the relevant index files so the doc is discoverable.

## Workspace Boundary Awareness

When auditing or documenting a repo layout, be aware of these safety-critical rules:

### `_tmp/` Scratch Directory
- `_tmp/` at the repo root is the designated scratch space for transient artifacts (build outputs, test fixtures, benchmark configs, temporary downloads).
- It is gitignored and safe for ephemeral use.
- Agents MUST NOT write scratch files outside the repo root (e.g., `/tmp/`, `~/`). Use `_tmp/` instead.

### Workspace Boundary (Harness-Enforced)
- AI coding harnesses enforce filesystem boundaries at the OS level. Agents that read/write outside the repo root are killed.
- All paths MUST resolve inside the repository root.
- `../` that escapes the repo root is forbidden.
- For multi-module repos (e.g., a Go module in a subdirectory), use the `workdir` parameter instead of `cd`.

### Docker Architecture (When Applicable)
- Axiom may use a two-image architecture: `axiom-controller` (orchestrator) and `axiom-workspace` (execution environment).
- Document container-specific paths and volume mounts when they affect the repo layout.

## Safety / Guardrails

- Never delete or move files without explicit justification.
- Never overwrite an existing layout doc wholesale; do minimal edits.
- Never invent paths; only document what exists.
- Never add secrets to docs.

## Minimal Verification

At minimum:
- Confirm the documented top-level directories exist.
- Confirm the referenced spec/doc inventory links resolve.

If a repo has a specs merge/build script, run it.
