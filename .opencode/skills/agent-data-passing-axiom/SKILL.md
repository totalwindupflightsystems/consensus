---
skill: agent-data-passing-axiom
description: "How to consume artifact paths from upstream command responses and pass detailed references between agents."
version: 1
spec_refs:
  - specs/13-Command-Registry.md
  - specs/04-XML-Protocol.md
tags:
  vertical: [coding, ops]
  category: tooling
  core: false
---

# Agent Data Passing — Consuming Upstream Artifacts

## When to load this skill

Load this skill when ANY of these conditions are true:

1. **You receive a `<delegate>` tag** in an upstream command response — the delegate context contains artifact paths you need to pass to the next agent.
2. **You are orchestrating a multi-step workflow** where Agent A produces files and Agent B needs to read them — load before dispatching Agent B so you know what paths to include in the prompt.
3. **You are about to call a second command** and the first command's response contained `evidence.files_changed` or `evidence.*_path` fields — load to understand how to extract and forward those paths.
4. **You receive an XML response with evidence fields** and you are unsure which files to read for detailed context — load to understand the `evidence.files_changed` vs `evidence.findings_paths` distinction.
5. **You are writing a subagent prompt** that needs to reference files produced by a previous step — load to ensure you include the right paths from the upstream evidence.

**Do NOT load** for single-agent tasks where you are both producing and consuming the output in the same context.

## The Core Pattern

When Agent A calls `/axiom-adversary-security` and gets back a response, the response XML contains:
```xml
<evidence>
  <files_changed>.memory-bank/findings/security/finding-1.md;.memory-bank/findings/security/finding-2.md</files_changed>
  <findings_paths>.memory-bank/findings/security/finding-1.md;.memory-bank/findings/security/finding-2.md</findings_paths>
</evidence>
```

Agent B should:
1. Extract `evidence.files_changed` (semicolon-separated list of all files touched)
2. Extract command-specific paths like `evidence.findings_paths`, `evidence.run_path`, `evidence.plan_path`
3. Read those files for detailed context before proceeding

## Step-by-step: How to consume upstream artifacts

### Step 1: Extract paths from the XML response
After calling a command, parse the response for evidence fields:
- `evidence.files_changed` — all files created/modified (always present for file-producing commands)
- `evidence.findings_paths` — finding files (adversary commands)
- `evidence.run_path` — run bundle directory (step/verify commands)
- `evidence.plan_path` — plan.yaml path (plan/meta-plan commands)
- `evidence.verification_path` — verification.md path (verify commands)

### Step 2: Read the artifact files
For each path in `evidence.files_changed` (or the specific path field), read the file:
```
Read tool: filePath = <path from evidence.files_changed>
```

### Step 3: Pass paths to downstream agents
When dispatching a downstream agent, include the artifact paths explicitly in the prompt:
```
"The upstream command produced these findings:
- .memory-bank/findings/security/finding-1.md
- .memory-bank/findings/security/finding-2.md
Read these files before proceeding."
```

## Common patterns by command type

| Command | Key evidence field | What to read |
|---|---|---|
| `/axiom-adversary*` | `evidence.findings_paths` | Individual finding files for detailed context |
| `/axiom-step` | `evidence.run_path` | Run bundle for step evidence |
| `/axiom-verify` | `evidence.verification_path` | Verification report |
| `/axiom-plan` | `evidence.plan_path` | plan.yaml for execution context |
| `/axiom-sync-*` | `evidence.files_changed` | Updated index/config files |
| `/axiom-spec-*` | `evidence.files_changed` | Updated spec files |

## Anti-patterns to avoid

❌ **Don't rely on the summary alone**: `<summary>3 findings found</summary>` tells you the count but not the content. Read the finding files.

❌ **Don't re-run the upstream command**: If Agent A already ran `/axiom-adversary-security`, Agent B should read the finding files from `evidence.findings_paths`, not re-run the command.

❌ **Don't assume paths**: Always extract paths from the response XML. Don't construct paths from work item IDs — the actual paths are in `evidence.files_changed`.

## Enforcement

The command registry enforces that file-producing commands include `evidence.files_changed` via:
1. `required_tags` — commands that MUST include the field
2. `tag_prompts` — micro-prompts that guide agents to fill the field
3. `v2_variant` — retry mechanism that fires when required fields are missing

See: `specs/13-Command-Registry.md`, `specs/04-XML-Protocol.md`

axiom:trace work_item=command-quality-01 spec=specs/13-Command-Registry.md