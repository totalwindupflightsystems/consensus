---
name: axiom-autonomous-intake
description: Portable autonomous intake and lifecycle contract — dual-path intake normalization, deterministic idempotency, shared lifecycle state machine, onboarding, install/refresh, in-progress update merge, and post-completion refinement.
version: "1.0"
synopsis: |
  Defines a deterministic, fail-closed lifecycle for autonomous work intake across Jira-triggered
  and runtime-local (CLI/API) paths. Covers canonical envelope normalization, idempotency key
  generation, shared lifecycle stages (received → normalized → onboarding → install_refresh →
  planning → executing → verifying → completed), free-text intent as first-class input, autonomous
  onboarding for non-Axiom repos, install lifecycle (scaffold/link/upgrade), per-work-item refresh,
  in-progress update merge, and post-completion refinement loops.
when-to-use: |
  Load this skill when implementing intake normalization, building idempotency logic, designing
  lifecycle state transitions, handling free-text intent, implementing autonomous onboarding,
  building install/refresh preflight checks, or designing update merge and refinement loops.
tags:
  vertical: [planning, onboarding]
  category: planning
  core: false
---

# Autonomous Intake and Lifecycle (Portable)

This skill defines the deterministic, fail-closed lifecycle for autonomous work intake.

Source spec: `specs/44-Autonomous-Intake-And-Lifecycle.md`

---

## Contract Vocabulary

Axiom uses three distinct lifecycle fields to prevent vocabulary drift:

| Field | Scope | Values |
|-------|-------|--------|
| `run_status` | API / persistence | `pending`, `in_progress`, `blocked`, `completed`, `failed`, `cancelled` |
| `lifecycle_stage` | Intake pipeline | `received`, `normalized`, `onboarding`, `install_refresh`, `planning`, `executing`, `verifying`, `completed` (+ optional: `blocked`, `failed`, `cancelled`, `refinement_pending`, `refinement_executing`) |
| `workflow_state` | Work progression | `intake`, `study`, `plan`, `implement`, `verify`, `review`, `done`, `blocked` |

Rules:
- Contracts MUST NOT overload one field name to represent multiple vocabularies.
- External API and runtime docs MUST label fields with these exact meanings.
- Legacy field aliases are allowed only as explicit compatibility aliases with migration notes.

---

## Dual-Path Intake

Axiom implements one lifecycle contract shared by Jira-triggered and runtime-local intake. Jira is an intake source, not a required dependency.

### Accepted Sources

| Source | Description |
|--------|-------------|
| `jira_event` | Jira webhook event |
| `jira_message` | Jira message/comment |
| `local_cli` | CLI-initiated run |
| `local_api` | HTTP API-initiated run |
| `local_file` | Local automated runner input file |

### Canonical Envelope

All intake MUST be normalized into a canonical envelope before scheduling:

| Field | Required | Source |
|-------|----------|--------|
| `work_item_id` | Yes | Provided or derived from intent |
| `repo` | Yes | Explicit path or deterministic detection |
| `intent` | Yes | Raw user intent text |
| `acceptance_criteria` | Yes | From ticket or user input |
| `intake_source` | Yes (server-derived) | Transport-origin label |
| `intake_event_id` | Yes (server-derived) | Deterministic source key |
| `received_at` | Yes (server-derived) | Server clock timestamp |

Rules:
- Source-specific fields MAY be present but MUST NOT replace canonical fields.
- `intake_source` labels are transport-origin labels only — NOT operating mode labels.
- Missing `repo` after source-specific resolution → MUST fail closed with `status=blocked`.
- Empty `intent` AND empty `acceptance_criteria` → MUST fail closed; do not start plan generation.

---

## Free-Text Intent as First-Class Input

The `intent` field accepts free-text descriptions as first-class input:

- `"Add a /health endpoint that returns server status"` — free text, no Jira
- `"ABC-123"` — Jira key (auto-detected by pattern match)
- `"Fix the login timeout bug from ABC-123"` — mixed (Jira key extracted, free text preserved)

Rules:
- All intake surfaces MUST accept any non-empty string as `intent`.
- Jira key auto-detection: `/^[A-Z][A-Z0-9]+-\d+$/` for pure keys; `/[A-Z][A-Z0-9]+-\d+/` for embedded.
- Detection is best-effort and MUST NOT reject intent without a Jira key.
- When Jira integration is not configured, proceed without Jira context (graceful degradation).
- Mixed intent: preserve full original text as `intent`; extract Jira key into `jira_key` as supplementary metadata.

---

## Deterministic Idempotency Keys

Each intake event MUST generate a deterministic idempotency key.

### Key Formats

| Source | Key Format |
|--------|------------|
| Jira events | `jira:{issue_key}:{event_type}:{event_revision}` |
| Local API | `local_api:{repo}:{work_item_id}:{client_token_or_payload_hash}` |
| Local CLI | `local_cli:{repo}:{work_item_id}:{request_hash}` |
| Local file | `local_file:{repo}:{work_item_id}:{request_hash}` |

### Canonical Payload Hash Algorithm

When no caller-provided client token exists:

1. Build canonical object with: `intent`, `work_item_id`, `repo`, `acceptance_criteria`.
2. Remove keys with `null` values.
3. Normalize strings: trim ASCII whitespace, CRLF → LF.
4. Sort `acceptance_criteria` lexicographically.
5. Serialize as JSON with sorted keys, no extra spaces.
6. Hash as UTF-8 using SHA-256, encode as lowercase hex.

### Derived work_item_id Algorithm

When `work_item_id` is omitted:

1. Normalize `intent` (trim whitespace, CRLF → LF).
2. Lowercase.
3. Replace any run of non `[a-z0-9]` with `-`.
4. Collapse repeated `-`, trim leading/trailing `-`.
5. If empty, use `work-item` as base slug.
6. Truncate to 48 characters.
7. Append `-` + first 8 hex chars of canonical payload hash.
8. Result: `<base-slug>-<hash8>`.

### Dedupe Rules

- Duplicate keys MUST be accepted idempotently without creating duplicate runs.
- First-write-wins; return original run reference for duplicates.
- Retention: at least 7 days.
- Dedupe scope is source-namespaced in v1 — cross-source dedupe is forbidden.
- Hash input MUST exclude server-derived fields (`intake_source`, `intake_event_id`, `received_at`, `run_id`, etc.).

---

## Shared Lifecycle State Machine

All runs use the same lifecycle stages regardless of intake source.

### Required Stages

```
received → normalized → onboarding → install_refresh → planning → executing → verifying → completed
```

### Optional Stages

`blocked`, `failed`, `cancelled`, `refinement_pending`, `refinement_executing`

### Transition Rules

- Transitions MUST be monotonic except `refinement_pending → refinement_executing → completed` loops.
- Transition conditions MUST be explicit and deterministic.
- `blocked` and `failed` MUST include machine-readable `reason_code` and actionable `operator_action` text.

### Key Transitions

| From | To | Condition |
|------|----|-----------|
| `received` | `normalized` | Normalization succeeds |
| `received` | `blocked` | Normalization fails (missing fields) |
| `normalized` | `onboarding` | Repo needs Axiom assets |
| `normalized` | `install_refresh` | Repo already onboarded |
| `onboarding` | `install_refresh` | Onboarding succeeds |
| `onboarding` | `blocked` | Governance forbids writes |
| `install_refresh` | `planning` | Assets validated |
| `planning` | `executing` | Plan generated |
| `planning` | `failed` | Planning fails |
| `executing` | `verifying` | All steps complete |
| `executing` | `blocked` | Human input needed |
| `verifying` | `completed` | All gates pass |
| `verifying` | `executing` | Inject corrective work |
| `completed` | `refinement_pending` | Post-completion update arrives |

---

## Autonomous Onboarding

When intake targets a repository lacking required Axiom assets, runtime MUST perform autonomous onboarding.

### Required Assets Detection

- `.opencode/`
- `.axiom/`
- `.memory-bank/`
- `AGENTS.md`
- Minimum `specs/` stubs

Rules:
- If governance allows repo writes: run install lifecycle automatically.
- If governance forbids writes: block with exact patch/install instructions.
- Do NOT proceed to planning/execution when required assets are missing and onboarding failed.

---

## Install Lifecycle Contract

### Modes

| Mode | Behavior |
|------|----------|
| `scaffold` | Copy-based install, idempotent, skip-or-force policy |
| `link` | Submodule/symlink-based install, idempotent, verify link integrity |

### Upgrade Behavior

- Scaffold: installer re-run with deterministic diff behavior.
- Link: submodule update + link verification.

### Refresh-on-Start

Before planning each work item run, runtime MUST execute deterministic compatibility refresh:

- Scope: `.memory-bank/`, `.opencode/`, `opencode.jsonc`, `.axiom/` critical assets.
- Ownership: driven by `.axiom/install/ownership-manifest.yaml`.
- Allowed writes: create missing required assets; update only installer-managed allowlist paths.
- Non-allowlisted paths: validate-only; MUST NOT overwrite automatically.
- Content mismatch outside allowlist: `status=blocked`, `reason_code=refresh_conflict_non_allowlisted_path`.
- Refresh failure MUST block before execution.

### Install Results

Machine-readable: `created`, `updated`, `skipped`, `conflicted`, `failed`.

---

## Bug-Fix Intake Gate (Gate 1)

For bug-fix work items (Jira type Bug/Hotfix, `mode=bugfix`, or `[bugfix]`/`[hotfix]` label), Gate 1 (Staleness/Already-Resolved Check) MUST run as the **first action** before any planning or work item creation.

**Gate 1 behavior**:
- **HARD BLOCK** (ticket already resolved): Stop immediately. Write `## Staleness Decision` to verification.md. Do NOT create a plan. Recommend closing the ticket or verifying the existing fix.
- **WARN** (stale >7 days, no activity): Continue with WARN flag. Note the stale signal in Phase 0 of the plan.
- **Jira MCP unavailable**: Proceed with git-only staleness signals. Apply WARN (not HARD BLOCK). Note: "Jira MCP unavailable — git-only staleness check applied."

**Already-resolved signals** (any one → HARD BLOCK):
- Jira ticket status is Done/Resolved/Closed/Won't Fix
- Recent commit message references ticket key AND contains "fix"/"resolve"/"close"/"revert"
- Merged PR references the ticket key
- Linked support ticket is resolved

**Staleness signals** (any one → WARN):
- Last commit to target files within 7 days
- Open PR exists touching same files
- Jira comment within 7 days indicating work in progress
- Related ticket resolved within 30 days

**Override**: If the user provides `override=staleness-check` with a justification, convert HARD BLOCK to WARN and proceed. Record the override in verification.md.

**Full spec**: `specs/02-Workflows.md#staleness-and-already-resolved-check`

<!-- axiom:trace work_item=sprint-44-gate-integration-01 spec=specs/02-Workflows.md plan=phase-3/task-3-1/step-3-1-2 evidence=.memory-bank/work-items/sprint-44-gate-integration-01/verification.md#ac-5 -->

---

## In-Progress Update Merge

Updates arriving during active runs MUST be queued and merged deterministically.

### Queue Rules

- Ordering key: `(ingested_at, source_id)`.
- `ingested_at` assigned by receiving runtime clock at enqueue time — immutable.
- `source_timestamp` from external payloads is untrusted metadata — ignored for ordering.
- Safe merge boundary: phase end (unless update marked `urgent` by governance).

### Processing Results

`applied_to_current_plan`, `deferred_to_refinement`, `ignored_not_relevant`, `blocked_requires_human`

---

## Post-Completion Refinement Loop

Updates received after run completion create deterministic refinement behavior:

1. Post-completion update → transition to `refinement_pending`.
2. Refinement creates a new run linked via `parent_run_id`.
3. Refinement runs reuse existing work item context and record the triggering update set.

---

## Non-Functional Requirements

### Lifecycle Determinism

For identical canonical intake envelope and repo state, lifecycle decisions MUST be deterministic.

### Fail-Closed Safety

If onboarding/install-refresh/update merge cannot complete safely, runtime MUST block without partial execution of plan steps.

---

## Prompt Mirror Requirement

If modules, APIs, or invariants are introduced for this lifecycle, prompt mirror artifacts MUST be updated to include:
- Intake source normalization rules.
- Install-refresh preflight invariants.
- Update/refinement state semantics.
