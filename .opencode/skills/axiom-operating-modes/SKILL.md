---
name: axiom-operating-modes
description: Portable guide to Axiom operating modes (Local CLI, Local Automated, Full Automated), execution core identity, command-driven intake, containerized operation, dry-run preflight, and serve-mode defaults.
version: "2.0"
license: MIT
compatibility: opencode
metadata:
  workflow: doctrine
  outputs: "none (behavioral guidance only)"
tags:
  vertical: [onboarding, ops]
  category: onboarding
  core: false
---

# Axiom Operating Modes (Portable)

Use this skill to keep guidance and implementation decisions mode-correct. All rules are inlined — this skill is the portable contract.

## Mode Summary (Contract)

| Mode | Who drives the loop | Where it runs |
|---|---|---|
| `cli` (Local CLI) | Human | OpenCode CLI / IDE, no runner enforcement |
| `local` (Local Automated) | Repo runner binary | Local process (in-process or containerized) |
| `auto` (Full Automated) | Control plane + repo runner | Kubernetes |

Rule: mode changes orchestration ownership, not spec semantics.

## Execution Core Identity (REQ-OM-EXEC-001, REQ-OM-EXEC-002)

The execution core -- plan loading, step execution, verification, evidence writing, checkpoint persistence -- is **identical** regardless of trigger source or provisioning mode. Mode-specific behavior is limited to:

1. **How the workspace is provisioned** (in-process Python, Docker container, Kubernetes pod).
2. **How the run request enters the system** (CLI flag, HTTP POST, Jira webhook, ralph steering packet).
3. **How results are reported back** (CLI stdout, HTTP response/SSE, Jira comment, PR).

All other execution semantics -- plan cursor advancement, retry/escalation, verification gates, evidence recording, checkpoint writes, delegation stack handling -- MUST be shared code paths.

### Trigger x Provisioning Matrix

| Trigger | In-Process | Docker | Kubernetes |
|---|---|---|---|
| CLI (`axiom run`) | Yes | Yes | -- |
| HTTP API (`POST /api/v1/runs`) | Yes | Yes | Yes |
| Jira webhook | -- | Yes | Yes |
| Ralph meta-loop | Yes (host) | -- | -- |

### CLI as Thin Client (REQ-OM-EXEC-002)

CLI (`axiom run`) SHOULD be a thin client that delegates to the HTTP server. The `--in-process` flag starts an embedded server, not a separate execution path. CLI-specific logic is limited to output formatting (progress bars, color, terminal width).

## Terminology Guardrail

Do not mix these namespaces:

- `operating_mode`: who drives orchestration and where runtime is deployed.
- `intake_source`: how a run request entered the lifecycle (`jira_event`, `local_api`, `local_cli`, etc.).

Operator copy SHOULD render both when present: `mode=local`, `intake_source=local_api`.

## Command-Driven Intake (CLI)

`axiom run` supports explicit command-driven intake for custom repo workflows (e.g., PRD-to-spec bootstrapping).

| Flag | Type | Required | Default | Description |
|---|---|---|---|---|
| `--entry-command` | string | No | runner default planning command | Slash command to execute for intake (e.g., `/axiom-plan`, `/prd-to-spec`). |
| `--entry-input` | string | No | empty | Inline input passed to the entry command. |
| `--entry-input-file` | path | No | unset | UTF-8 file whose contents are passed as entry input. |

Rules:
- `--entry-input` and `--entry-input-file` are mutually exclusive.
- `--entry-command` MUST begin with `/` and resolve to an installed command in `.opencode/commands/`.
- If `--entry-command` is omitted, existing behavior is preserved (backward compatible).
- File read failures for `--entry-input-file` MUST fail closed before runtime execution starts.

Example:
```bash
axiom run \
  --work-item "prd-upgrade-01" \
  --repo . \
  --in-process \
  --entry-command "/prd-to-spec" \
  --entry-input-file ".memory-bank/prds/platform/auth-refresh.md"
```

## Containerized Local Automated

Local Automated mode can run either **in-process** (direct Python on host) or **containerized** (via Docker). The containerized variant provides stronger isolation and closer parity with production Kubernetes topology.

| Aspect | In-Process | Containerized |
|---|---|---|
| Execution environment | Host Python process | Docker containers |
| Isolation | OS process boundaries | Container boundaries (filesystem, network, PID) |
| Startup | `axiom run` / `axiom serve` directly | `docker compose up` starts control plane + dispatch daemon |
| Workspace provisioning | Python subprocess or in-process | Docker API via Docker socket |
| Reproducibility | Depends on host environment | Deterministic container image |
| Parity with production | Lower (no container lifecycle) | Higher (same lifecycle as Kubernetes mode) |

Mode detection:
- `AXIOM_MODE=local` (same as in-process)
- `AXIOM_CONTAINERIZED=true` (distinguishes containerized from in-process)
- Docker socket available at `/var/run/docker.sock`

## Dry-Run Preflight Mode

The `--dry-run` flag on `axiom run` performs a full preflight check without executing any work. It validates environment readiness and reports what WOULD happen.

### Guarantees (normative)

| Guarantee | Description |
|---|---|
| **No network** | No HTTP requests to OpenCode, Jira, GitHub, or any external service. |
| **No file writes** | No files created, modified, or deleted anywhere. |
| **No subprocesses** | No child processes spawned. No Docker containers, no OpenCode lifecycle. |
| **Read-only git** | May read git status but MUST NOT modify index, working tree, refs, or config. |

Violation of any guarantee is a product bug.

### Preflight Checks (ordered, fail on first failure)

1. **Config file exists** -- Resolves config path per discovery rules.
2. **Config is valid YAML** -- Parses the config file.
3. **Config schema validates** -- Runs Pydantic `model_validate`.
4. **Repo is a git repository** -- Checks `--repo` points to valid git working tree.
5. **Worktree is clean** -- Checks for uncommitted changes.
6. **Plan is loadable** -- If `plan.yaml` exists, validates it loads without errors. Skipped if no plan.
7. **Branching resolves** -- Resolves effective `source_branch` and `pr_target_branch`.

Exit codes: `0` = all passed (ready), `2` = one or more failed (not ready). Never exits `1` (no runtime logic).

### Differentiation from `axiom validate`

| Aspect | `axiom validate` | `axiom run --dry-run` |
|---|---|---|
| Scope | Config file only | Full environment (config + repo + plan + branching) |
| Dirty worktree | PASSES (not checked) | FAILS |
| Plan loading | Not checked | Checked (if plan exists) |
| Work item | Not required | Required (`--work-item`) |

`axiom run --dry-run` is a strict superset of `axiom validate`.

## Server Default Execution Mode (REQ-OM-SERVE-001, REQ-OM-SERVE-002)

When `axiom serve` starts, it resolves a default execution mode for `POST /api/v1/runs` requests that omit `execution.mode`.

### Resolution Priority (highest wins)

1. **Environment variable** `AXIOM_EXECUTION_DEFAULT_MODE` -- when set to a valid mode (`in_process`, `docker`, `kubernetes`).
2. **Config file** `execution.default_mode` in `.axiom/axiom.config.yaml`.
3. **Hardcoded fallback** -- `"in_process"`.

Rules:
- Invalid mode values MUST fail startup with a clear error.
- Resolved mode MUST be logged as a structured event at startup.
- Stored in `app.state.default_execution_mode`.

### OpenCode Auto-Management in Serve Mode (REQ-OM-SERVE-002)

When the server runs in `in_process` default mode, it MUST manage the OpenCode server lifecycle:

**Startup**:
1. Resolve OpenCode URL: `AXIOM_OPENCODE_BASE_URL` -> `opencode.base_url` config -> default loopback.
2. Health-check existing server (`GET /global/health`).
3. If healthy, use existing server.
4. If unhealthy and `opencode.spinup_enabled` is `true`, auto-start with `opencode serve --port 0 --hostname 127.0.0.1`.
5. Parse actual port; store in `app.state.opencode_base_url`.

**Shutdown**: SIGTERM -> wait 5s -> SIGKILL if needed.

Auto-management activates ONLY for `in_process` mode. Docker/Kubernetes modes manage OpenCode through their own provisioning.

## Self-Unblocking via @sitrep-axiom

When the builder+verifier loop reaches a BLOCKED/stop condition in automated modes, the loop runner can invoke `@sitrep-axiom` to discover unblocked work and continue without human intervention.

- Defined in `specs/49-Loop-Self-Unblocking.md`.
- Can be disabled with `--no-sitrep-unblock` or `AUTO_SITREP_UNBLOCK=false` for debugging, CI, or testing.

## Practical Guidance

- In `cli` mode: do not assume checkpoint persistence or automatic retry/escalation; the human is the loop.
- In `local` and `auto` modes: require fail-closed automation semantics (plan cursor, verification gates, evidence bundles).
- Keep parity across intake sources: Jira-triggered and local-triggered flows MUST converge to the same normalized work packet.

## Verification Expectations

- Mode-dependent runtime evidence MUST match the verification signal hierarchy in `specs/00-PRD.md#verification-signal-hierarchy`.
- If you touch server path behavior, include Tier 4 health evidence.

## References

- `specs/29-Operating-Modes.md`
- `specs/44-Autonomous-Intake-And-Lifecycle.md`
- `specs/49-Loop-Self-Unblocking.md`

axiom:trace work_item=doctrine spec=specs/29-Operating-Modes.md plan= test= doc=.opencode/skills/axiom-operating-modes/SKILL.md evidence= commit=
