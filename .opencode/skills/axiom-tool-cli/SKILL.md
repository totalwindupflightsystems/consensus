---
tags:
  vertical: [ops, coding]
  category: tooling
  core: false
---

# axiom-tool-cli — Skill Guide

**Skill ID**: `axiom-tool-cli`  
**Version**: 1.0  
**Jira**: [DEX-17](https://dexdat.atlassian.net/browse/DEX-17), [DEX-285](https://dexdat.atlassian.net/browse/DEX-285)  
**Spec refs**: `specs/76-Scheduled-Execution-And-Ad-Hoc-Tools.md` §4, `specs/70-OpenCode-Plugin.md` §L6  
**axiom:trace**: `work_item=DEX-285 spec=specs/76-Scheduled-Execution-And-Ad-Hoc-Tools.md#REQ-SCHED-020 plan=phase-S/task-S-1/step-S-1-6 jira_ref=DEX-285,DEX-17`

---

## Purpose

This skill teaches agents how to use `axiom tool` — the ad-hoc CLI tool execution system — and how to wrap any CLI as a Axiom tool that agents can call, operators can run ad-hoc, and the scheduler can execute periodically.

**One config, three execution surfaces:**

```
.axiom/plugin.yaml  tools:
        │
        ├── Plugin L6 tool registry  →  agents call tools inside OpenCode sessions
        ├── axiom tool run         →  operators run tools ad-hoc from CLI
        └── axiom schedule         →  scheduler runs tools periodically
```

---

## Quick Reference

```bash
# List all available tools
axiom tool list

# Run a tool immediately
axiom tool run <tool-name>
axiom tool run <tool-name> [extra-args...]

# Show tool metadata
axiom tool info <tool-name>
```

---

## Defining a Tool

Add tools to `.axiom/plugin.yaml` under the `tools:` key:

```yaml
tools:
  - name: project_lint
    description: Run the project linter and return results
    command: npm run lint 2>&1

  - name: terraform_plan
    description: Run terraform plan for the current workspace
    command: terraform plan -no-color
    working_dir: infra/
    timeout: 10m

  - name: docker_build
    description: Build the application Docker image
    command: docker build -t myapp:latest .
    timeout: 15m

  - name: security_audit
    description: Run npm security audit
    command: npm audit --production --audit-level=high
    working_dir: ui/
    timeout: 2m

  - name: custom_lint
    description: Run project-specific linting
    command: ./scripts/lint.sh
    timeout: 3m
    env:
      LINT_STRICT: "true"

  - name: run_tests
    description: Run project tests with optional pattern
    command: npm test -- $test_pattern 2>&1
    parameters:
      - name: test_pattern
        type: string
        description: Test file or pattern to run
        required: false
```

### Tool definition fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | ✅ | Tool identifier (used in `axiom tool run <name>`) |
| `description` | ✅ | Human-readable description shown in `axiom tool list` |
| `command` | ✅ | Shell command to execute (via `sh -c`) |
| `working_dir` | ❌ | Working directory relative to repo root (default: repo root) |
| `timeout` | ❌ | Execution timeout in Go duration format (default: 60s) |
| `env` | ❌ | Additional environment variables as key-value pairs |
| `parameters` | ❌ | Typed parameters for `$variable` substitution in command |

### Naming convention

Tools are displayed with a `codeops_` prefix in `axiom tool list` output (e.g., `codeops_project_lint`). You can use either the bare name or the prefixed name when running:

```bash
axiom tool run project_lint
axiom tool run codeops_project_lint  # same thing
```

---

## Common Patterns

### Wrapping a linter

```yaml
tools:
  - name: lint
    description: Run ESLint on the project
    command: npx eslint . --ext .ts,.tsx --format compact 2>&1
    timeout: 2m
```

```bash
axiom tool run lint
```

### Wrapping a test runner with a filter

```yaml
tools:
  - name: test
    description: Run Jest tests (optionally filtered)
    command: npx jest $pattern --passWithNoTests 2>&1
    timeout: 5m
    parameters:
      - name: pattern
        type: string
        description: Test name pattern (optional)
        required: false
```

```bash
axiom tool run test
axiom tool run test auth  # runs tests matching "auth"
```

### Wrapping Terraform

```yaml
tools:
  - name: tf_plan
    description: Run terraform plan for the staging environment
    command: terraform plan -var-file=staging.tfvars -no-color 2>&1
    working_dir: infra/
    timeout: 10m
    env:
      TF_LOG: WARN
```

```bash
axiom tool run tf_plan
```

### Wrapping a deployment script

```yaml
tools:
  - name: deploy_staging
    description: Deploy the application to the staging environment
    command: ./scripts/deploy.sh --env staging 2>&1
    timeout: 20m
```

```bash
axiom tool run deploy_staging
```

### Wrapping a database migration

```yaml
tools:
  - name: db_migrate
    description: Run pending database migrations
    command: python manage.py migrate --no-input 2>&1
    timeout: 5m
    env:
      DJANGO_SETTINGS_MODULE: myapp.settings.staging
```

---

## Scheduling a Tool

Once a tool is defined in `plugin.yaml`, you can schedule it to run periodically:

```yaml
# .axiom/schedules.yaml
entries:
  - name: hourly-lint
    schedule: "@hourly"
    type: tool
    target: project_lint

  - name: daily-security-audit
    schedule: "@daily"
    type: tool
    target: security_audit
```

```bash
# Start the scheduler
axiom serve --scheduler --port 8100 &

# Check scheduled executions
axiom schedule list
axiom schedule history
```

---

## Using Tools Inside OpenCode Sessions (L6)

When `.axiom/plugin.yaml` is configured and approved, tools are automatically registered in the OpenCode plugin's L6 tool registry. Agents can call them directly:

```
# Agent can call:
codeops_project_lint()
codeops_run_tests(test_pattern="auth")
```

To approve the config:
```
# In an OpenCode session:
codeops_approve_config(confirm=True)
```

---

## Workspace Boundary

All tools run with the repo root as the working directory (unless `working_dir` is set). Tools cannot access paths outside the repo root.

**Security note (v1)**: The workspace boundary check validates the `command` token but does not sanitize `parameters` values. Treat `.axiom/plugin.yaml` as a trusted configuration file (same trust level as `Makefile` or `package.json` scripts). See spec §11 DA-005.

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| `Error: tool 'my_tool' not found` | Check `.axiom/plugin.yaml` exists and has a `tools:` section with the tool name |
| Tool times out | Increase `timeout` in the tool definition (e.g., `timeout: 10m`) |
| Tool runs in wrong directory | Set `working_dir` relative to repo root |
| Environment variable not set | Add it to the `env:` section of the tool definition |
| `axiom tool list` shows no tools | Ensure `.axiom/plugin.yaml` exists and has a `tools:` section |

---

## Related Commands

```bash
# Schedule management
axiom schedule list
axiom schedule add "@daily" tool my_tool --name daily-run
axiom schedule validate
axiom schedule history

# Plugin approval (for L6 tool registry in OpenCode sessions)
# Run inside an OpenCode session:
# codeops_approve_config(confirm=True)
```

---

## Spec References

- `specs/76-Scheduled-Execution-And-Ad-Hoc-Tools.md` §4 — Ad-Hoc Tool Execution (REQ-SCHED-020 through REQ-SCHED-024)
- `specs/70-OpenCode-Plugin.md` §L6 — Custom Tool Registration (REQ-PLG-500 through REQ-PLG-509)
- `specs/76-Scheduled-Execution-And-Ad-Hoc-Tools.md` §2 — Schedule Definition Format (for scheduling tools)
