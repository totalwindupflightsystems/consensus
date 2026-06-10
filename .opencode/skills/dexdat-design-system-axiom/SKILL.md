---
name: dexdat-design-system-axiom
description: Parent design doctrine and spec-generation framework for Dexdat design systems across web, mobile, tablet, and app surfaces.
version: "0.1-draft"
tags:
  vertical: [coding]
  category: development
  core: false
---

# Dexdat Design System

Use this as the parent design doctrine skill when you need to build or extend a
product design system from a deep canonical source.

This skill is the "design bible" for downstream Dexdat design work. It does not
try to encode every final rule in `SKILL.md`. Instead it routes agents through a
large reference pack plus a fillable template so project-specific design specs
can be generated into `specs/` with enough precision to guide implementation,
review, and iteration.

The intended operating model is:

1. Read the parent doctrine and the reference pack.
2. Read the current designer-provided source material.
3. Fill the project-specific template with the parts that are fixed for this
   product/system.
4. Write the resulting design contract into `specs/`.
5. Keep mutable project choices in the generated spec, not in the parent
   doctrine, unless the choice should become a reusable Dexdat-wide rule.

axiom:trace work_item=dexdat-design-system-01 spec=specs/README.md plan=.memory-bank/work-items/dexdat-design-system-01/plan.md doc=.opencode/skills/dexdat-design-system-axiom/SKILL.md evidence=.memory-bank/work-items/dexdat-design-system-01/verification.md commit=

## Purpose

This skill exists to support a very large, highly detailed design doctrine that
can eventually grow to tens of thousands of lines without collapsing into one
unmaintainable file.

The doctrine must be able to cover:
- brand and visual language
- design tokens and theming
- layout systems and spacing laws
- typography systems
- interaction patterns
- motion and animation rules
- responsive behavior across web/mobile/tablet
- component family rules
- platform adaptation rules
- accessibility and inclusion rules
- copy, tone, and affordance language
- review rubrics and exception handling

## Required Resource Files

Load and use these files together:

- `references/current-rules.md` - current reusable Dexdat-wide doctrine and stable rules
- `references/surface-taxonomy.md` - surface map and scope inventory across web/mobile/tablet/app contexts
- `templates/project-design-spec-template.md` - fillable template for product/system-specific design specs
- `checklists/design-spec-readiness-checklist.md` - completeness gate before claiming a generated design spec is ready

## Workflow

### Step 1: Classify the target surface

Determine which surfaces the target system needs:
- web marketing
- web app
- mobile app
- tablet app
- desktop app
- shared cross-platform system

Do not assume one universal answer. A product can use one parent doctrine with
different per-surface implementations.

### Step 2: Split fixed doctrine from project choices

Treat doctrine in two buckets:

- **Fixed doctrine**: rules that should remain stable across multiple products
- **Project choices**: rules that should vary for the current product/system

Examples of likely project choices:
- motion speeds
- density level
- visual tone within the allowed brand envelope
- breakpoint thresholds
- component emphasis and prominence
- haptic intensity

Examples of likely fixed doctrine:
- design philosophy
- token layering model
- accessibility minimums
- interaction-state model
- spacing rhythm model
- review criteria

### Step 3: Generate a project-specific design contract

Use `templates/project-design-spec-template.md` and write the generated artifact
into `specs/`.

Expected output shapes may include:
- `specs/NN-Dexdat-Design-System.md`
- `specs/NN-Web-UI-Design-Contract.md`
- `specs/NN-Mobile-UI-Design-Contract.md`
- `specs/NN-Interaction-And-Motion.md`
- `specs/NN-Design-Tokens-And-Theming.md`

### Step 4: Mark uncertainty explicitly

If a designer rule is missing, ambiguous, or contradictory:
- do not invent the final rule
- mark it as an open decision in the generated spec
- propose bounded options with tradeoffs
- preserve the doctrine/template split

## Output Contract

When using this skill to generate a project-specific design spec, the output
SHOULD include at minimum:

- summary and intent
- surface scope
- fixed doctrine inherited from Dexdat
- project-specific overrides
- design token rules
- layout and responsive rules
- component rules
- interaction and motion rules
- accessibility rules
- verification/review checklist
- explicit open decisions

## Guardrails

- Do not collapse the doctrine into a single giant `SKILL.md`.
- Keep reusable rules in `references/` and generation structure in `templates/`.
- Keep project-specific decisions in `specs/`, not the parent doctrine, unless
  they should become reusable Dexdat-wide rules.
- If a rule changes system behavior or operator expectations, write it as a spec
  contract, not just a style note.
- Do not present aesthetic opinions as fixed doctrine unless the designer source
  material supports them.

## Planned Child Skills

This parent skill is expected to eventually route or pair with child skills such
as:
- `dexdat-web-design-axiom`
- `dexdat-mobile-design-axiom`
- `dexdat-tablet-design-axiom`
- `dexdat-motion-design-axiom`
- `dexdat-design-tokens-axiom`
- `dexdat-accessibility-design-axiom`
- `dexdat-design-review-axiom`

## Draft Status

This is a foundation draft. It intentionally creates the package shape and the
generation workflow before the final doctrine has been supplied by the designer.
