---
name: assist-axiom
description: >
  Setup, activation, and capability guide for the assist-axiom personal AI chief of
  staff agent. Covers opencode.jsonc wiring, memory bank bootstrap, naming conventions
  for auto-generated artifacts, and handoff patterns for other Axiom agents.
  Load this skill when setting up a personal Axiom workspace, when another agent
  needs to understand assist-axiom capabilities, or when reviewing auto-generated
  skills/commands/scripts created by the agent.
version: "1.0"
license: MIT
compatibility: opencode
metadata:
  workflow: personal-os
  agent: assist-axiom
  tags:
    - personal
    - chief-of-staff
    - setup
    - activation
    - naming-conventions
tags:
  vertical: [personal-context, planning]
  category: personal
  core: false
---

# assist-axiom Skill — Setup, Activation, and Conventions

> This skill is the companion to `.opencode/agents/assist-axiom.md`.
> The agent file defines runtime behavior. This skill defines how to set it up,
> how to recognize its outputs, and how other agents should interact with it.

---

## What assist-axiom Is

`assist-axiom` is a personal AI chief of staff — not a code generator. It:

- Tracks context across sessions via the Axiom memory bank
- Proactively recalls prior context using `rg` before asking you to repeat yourself
- Searches the live web (`websearch`) when the memory bank has nothing
- Captures inbound content (Slack, email, notes) into `.memory-bank/captures/`
- Detects repeated workflows and proposes reusable skills, commands, and scripts
- Self-improves by observing patterns and logging them in `.memory-bank/agents/assist-axiom/`
- Manages goals as work items with the same lifecycle as engineering work items

It is **not** the default Axiom agent. It is activated per-project.

---

## Activation — opencode.jsonc

To activate `assist-axiom` as the default agent for a personal project, set
`default_agent` in that project's `opencode.jsonc`:

```jsonc
{
  "$schema": "https://opencode.ai/config.json",

  // Activate the personal AI chief of staff for this project
  "default_agent": "assist-axiom",

  "permission": {
    "bash": "allow",
    "edit": "allow",
    "write": "allow",
    "read": "allow",
    "glob": "allow",
    "grep": "allow",
    "webfetch": "allow",
    "patch": "allow"
  },

  // Add MCP servers available in your environment
  "mcp": {
    // Chrome DevTools — browser automation and research
    "chrome-devtools": {
      "enabled": true,
      "type": "local",
      "command": ["npx", "-y", "--registry", "https://registry.npmjs.org", "chrome-devtools-mcp@latest"]
    }
    // Add atlassian, notion, etc. as needed
  }
}
```

**For engineering repos**: do NOT change the global `opencode.jsonc`. Use a separate
personal workspace directory with its own `opencode.jsonc`, or invoke `assist-axiom`
explicitly by name in a session.

**Enable web search**: set `OPENCODE_ENABLE_EXA=1` in your environment to activate
the `websearch` tool (Exa AI, no API key required).

---

## AGENTS.md Wiring

`assist-axiom` reads `AGENTS.md` at every session start. The minimum required
section to activate personal mode:

```markdown
## Personal Operating Mode

This repo is a personal operating system, not a software project.
Specs are hard SOPs. Skills are soft SOPs. Work items are goals.
The memory bank is my second brain. Always check it before asking me to repeat context.

## Who I Am
[2-3 sentences: who you are, what you do, what you're focused on]

## My Hard Rules
1. [Rule 1]
2. [Rule 2]
3. [Rule 3]

## My Tool Stack
MCP tools available: [Notion / Jira / GitHub / browser / none]
```

See `.opencode/skills/personal-context-axiom/SKILL.md` for the full template.

---

## Naming Conventions for Auto-Generated Artifacts

> **Every artifact auto-created by `assist-axiom` uses a `-assist` suffix
> so you can instantly identify it as agent-generated vs. human-authored.**

### Skills → `-assist` suffix

```
.opencode/skills/<workflow-name>-assist/SKILL.md
```

Examples:
- `.opencode/skills/weekly-review-assist/SKILL.md`
- `.opencode/skills/travel-planning-assist/SKILL.md`
- `.opencode/skills/meeting-prep-assist/SKILL.md`
- `.opencode/skills/research-workflow-assist/SKILL.md`

### Commands → `-assist` suffix

```
.opencode/commands/<action>-assist.md
```

Examples:
- `.opencode/commands/capture-slack-assist.md`
- `.opencode/commands/process-inbox-assist.md`
- `.opencode/commands/weekly-review-assist.md`

### Scripts → `-assist` suffix

```
scripts/<action>-assist.sh   (or .py)
```

Examples:
- `scripts/sync-notion-assist.sh`
- `scripts/tag-index-assist.sh`
- `scripts/capture-batch-assist.py`

### Agent personas → `-assist` suffix

```
.opencode/agents/<domain>-assist.md
```

Examples:
- `.opencode/agents/health-coach-assist.md`
- `.opencode/agents/financial-advisor-assist.md`
- `.opencode/agents/research-assistant-assist.md`

### Why this matters

When you `ls .opencode/skills/` or `ls .opencode/commands/`, anything ending in
`-assist` was proposed and created by `assist-axiom`. You know immediately:
- It was generated from an observed pattern, not hand-authored
- It may need review and refinement
- It can be safely deleted if the pattern no longer applies
- It was staged in `.opencode/proposed/` before being promoted here

---

## Staging Area

All auto-generated artifacts are drafted here first — never written directly to
their final location:

```
.opencode/proposed/
  skills/     → promoted to .opencode/skills/<name>-assist/
  commands/   → promoted to .opencode/commands/<name>-assist.md
  scripts/    → promoted to scripts/<name>-assist.sh
  agents/     → promoted to .opencode/agents/<name>-assist.md
```

The agent will tell you when something is in `proposed/` and ask for approval
before moving it. You can review, edit, approve, or discard.

---

## Memory Bank Locations

`assist-axiom` reads and writes these locations:

| What | Where |
|------|-------|
| Session context | `.memory-bank/activeContext.md` |
| Decisions | `.memory-bank/decisionLog.md` |
| Inbound captures | `.memory-bank/captures/` |
| Watch items | `.memory-bank/signals/` |
| People context | `.memory-bank/contacts/` |
| Saved research | `.memory-bank/reference/` |
| Active goals | `.memory-bank/work-items/` |
| User profile | `.memory-bank/agents/assist-axiom/user-profile.md` |
| Observation log | `.memory-bank/agents/assist-axiom/observation-log.md` |
| Suggestion log | `.memory-bank/agents/assist-axiom/suggestion-log.md` |

---

## Handoff Patterns for Other Agents

When another Axiom agent (e.g., `tower-axiom`, `pm-axiom`) needs to interact
with a personal workspace managed by `assist-axiom`:

- **Read context from**: `.memory-bank/activeContext.md` and `.memory-bank/work-items/_current.md`
- **Write decisions to**: `.memory-bank/decisionLog.md`
- **Create captures via**: `.memory-bank/captures/<YYYY-MM-DD>-<slug>.md`
- **Do NOT overwrite**: `.memory-bank/agents/assist-axiom/user-profile.md` — that belongs to the agent
- **Respect the staging area**: never write directly to `.opencode/` without going through `proposed/`
- **Recognize `-assist` artifacts**: anything ending in `-assist` was agent-generated — treat as
  user-approved but potentially refinable

---

## Trace

```
axiom:trace work_item=personal-context-01 spec=.opencode/skills/assist-axiom/SKILL.md plan=personal-os/skill-creation
```
