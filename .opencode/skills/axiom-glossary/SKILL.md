---
name: axiom-glossary
description: Generate, maintain, and ship a Axiom glossary of terms, concepts, and conventions. Use this skill when onboarding new repos, writing docs, or when any agent needs to define or look up Axiom-specific terminology. Produces a glossary file for the memory bank that ships with every Axiom-managed workspace.
version: "1.0"
tags:
  vertical: [onboarding, writing]
  category: onboarding
  core: false
---

# Axiom Glossary Skill (Portable)

> **Purpose**: Every Axiom-managed workspace ships a glossary so agents and humans share the same vocabulary. This skill defines how to generate, maintain, and consume that glossary.

Load this skill when:
- Onboarding a new Axiom repo (the glossary should be created during onboarding)
- Writing docs, specs, or runbooks that use Axiom-specific terms
- An agent encounters an unfamiliar Axiom term and needs a definition
- Updating the glossary after new concepts, agents, or workflows are introduced
- Reviewing whether terminology is used consistently across artifacts

---

## Glossary Location

The canonical glossary lives at:

```
.memory-bank/topics/axiom-glossary.md
```

This file ships with every Axiom-managed workspace via the scaffold/template. It is a **memory bank topic** (cross-project evergreen knowledge), not a spec or a work item.

### Why memory bank, not specs?

- Specs define contracts and requirements. The glossary defines vocabulary.
- The glossary is consumed by agents for context, not enforced as a gate.
- It evolves as the platform evolves — it's living documentation, not a frozen contract.

---

## Glossary Format

Each entry follows this structure:

```markdown
### Term Name

**Also known as**: alias1, alias2 (if any)

Definition in 1-3 sentences. Be concrete. Prefer "X is Y that does Z" over abstract descriptions.

**Context**: Where this term appears (specs, agents, CLI, memory bank, etc.)
**See also**: [Related Term](#related-term), [Spec Reference](../../specs/NN-Spec.md)
```

### Format Rules

1. **Alphabetical order** within each category section.
2. **One definition per term**. If a term has multiple meanings in different contexts, list them as numbered sub-definitions.
3. **No circular definitions**. Don't define "trace marker" as "a marker used for tracing". Say what it actually is and does.
4. **Include the canonical form**. If the term has a specific format (e.g., `axiom:trace` markers), show the exact syntax.
5. **Link to the authoritative source**. Every term should link to the spec, skill, or agent that defines it.
6. **Use examples** for terms that are hard to understand abstractly.

---

## Glossary Categories

Organize terms into these sections:

| Section | What goes here |
|---------|---------------|
| Core Concepts | Fundamental Axiom ideas: specs-first, fail-closed, evidence-based, baby steps |
| Agents | Agent names, roles, and when they're invoked |
| Artifacts | Files and structures Axiom produces: specs, plans, evidence bundles, memory bank |
| Workflows | Named processes: Ralph loop, meta-cycle, fan-out/fan-in, verification loop |
| Trace & Evidence | Traceability terms: trace markers, evidence bundles, verification tiers |
| Configuration | Config files, schemas, settings: opencode.jsonc, axiom.config.yaml |
| Infrastructure | Runtime concepts: workspace boundary, harness, sandbox, MCP servers |
| Skills & Commands | Skill names, slash commands, and what they do |

---

## When to Update the Glossary

### Mandatory Updates (MUST)

- A new agent is added to the roster → add its entry
- A new skill is created → add its entry
- A new spec introduces a new concept or term → add the term
- A term's definition changes (e.g., a workflow is renamed or restructured) → update the entry
- Onboarding a new repo → verify the glossary is present and current

### Recommended Updates (SHOULD)

- After a major feature lands that introduces new vocabulary
- When a user or agent is confused by terminology (add or clarify the entry)
- During idle-time spec conformance sweeps (check glossary coverage)

### How to Detect Stale Entries

- If a term references a spec that no longer exists → update or remove
- If a term references an agent that was renamed → update
- If a term's definition contradicts current behavior → update definition or flag as a spec drift issue

---

## Generating a Glossary from Scratch

When onboarding a new Axiom workspace or regenerating the glossary:

### Step 1: Discover Terms

Scan these sources for Axiom-specific vocabulary:

1. `specs/README.md` and `specs/_index.md` — spec titles and categories
2. `specs/00-PRD.md` — product-level concepts
3. `AGENTS.md` — agent names, rules, conventions
4. `.opencode/agents/` — agent definitions and roles
5. `.opencode/skills/` — skill names and descriptions
6. `.opencode/commands/` — slash command names
7. `specs/07-Mission-North-Star.md` — philosophy terms
8. `specs/09-Baby-Steps-Methodology.md` — methodology terms
9. `specs/21-Traceability-Doctrine.md` — trace terms
10. `specs/27-Evidence-Bundle-Schema.md` — evidence terms
11. `.memory-bank/_prompt.md` — memory bank structure terms

### Step 2: Draft Entries

For each discovered term:
1. Write a concrete 1-3 sentence definition
2. Identify the authoritative source (spec, agent, skill)
3. Add cross-references to related terms
4. Place in the correct category section

### Step 3: Review for Consistency

- No circular definitions
- No undefined terms used in definitions (if term A's definition uses term B, term B must also be in the glossary)
- Alphabetical within sections
- All links resolve

### Step 4: Place and Index

1. Write to `.memory-bank/topics/axiom-glossary.md`
2. Update `.memory-bank/topics/_index.md` to include the glossary
3. Verify the glossary follows the memory bank topic template (YAML frontmatter, required sections)

---

## Consuming the Glossary

### For Agents

When writing docs, specs, or user-facing content:
- Use terms as defined in the glossary (don't invent synonyms)
- If you need a new term, add it to the glossary first
- Link to the glossary entry when using a term that might be unfamiliar to the reader

### For Humans

The glossary is the "what does this mean?" reference. When you encounter an unfamiliar term in Axiom output, check the glossary first.

### For Onboarding

New repos get the glossary as part of the scaffold. New team members read it as part of orientation. The glossary is the shared vocabulary contract.

---

## Glossary Maintenance Checklist

Use this checklist during glossary reviews:

- [ ] All agents in `.opencode/agents/` have glossary entries
- [ ] All skills in `.opencode/skills/` have glossary entries (or are grouped under a parent entry)
- [ ] All spec-defined concepts have glossary entries
- [ ] No circular definitions
- [ ] No broken links
- [ ] Alphabetical order within sections
- [ ] YAML frontmatter is valid
- [ ] `.memory-bank/topics/_index.md` includes the glossary

---

## Anti-Patterns

| Anti-pattern | Why it's bad | Fix |
|-------------|-------------|-----|
| Glossary only in someone's head | Agents can't read minds; humans forget | Write it down in the canonical location |
| Glossary in a spec file | Specs are contracts, not dictionaries | Keep glossary in memory bank topics |
| Multiple competing glossaries | Agents use different terms for the same thing | One canonical glossary, one location |
| Glossary with no links | Definitions without context are useless | Every entry links to its authoritative source |
| Glossary updated only at release | Terms drift between releases | Update on every new agent/skill/spec/concept |
| Overly abstract definitions | "A mechanism for ensuring quality" tells you nothing | Be concrete: "X is Y that does Z" |

---

## Scaffold Integration

When the Axiom scaffold (`axiom-template/`) ships a new repo, it SHOULD include:

1. A baseline `axiom-glossary.md` in `.memory-bank/topics/` with core Axiom terms
2. A reference in `.memory-bank/topics/_index.md`
3. A note in the onboarding checklist to review and extend the glossary for project-specific terms

The baseline glossary covers universal Axiom vocabulary. Project-specific terms (e.g., domain models, service names) are added by the team during onboarding.
