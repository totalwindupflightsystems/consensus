---
name: documentation-writing-axiom
description: Style guide for technical documentation, guides, and reference pages. Prioritizes active voice, second person, accessible structure, and practical sequencing.
version: "1.0"
tags:
  vertical: [writing]
  category: writing
  core: false
---

# Documentation Writing

Use for technical docs, how-to guides, onboarding pages, and reference material.

axiom:trace work_item=DEX-73 spec= plan= test= doc=.memory-bank/explorations/writing-style-skill-collection.md prompt=.opencode/skills/documentation-writing-axiom/SKILL.md evidence= commit= jira_ref=DEX-73

## Core Style

- Be conversational and friendly without being casual fluff.
- Use second person when instructing the reader.
- Prefer active voice and present tense.
- Put conditions before instructions when that improves clarity.
- Use sentence case for headings.

## Structure Rules

- Start with purpose and audience.
- Put prerequisites before steps.
- Use numbered lists for sequences.
- Use bullets for non-sequential facts.
- Use tables for comparisons, options, or field references.

## Default Template

```markdown
# <Title>

## What this is
<purpose>

## Before you begin
- prerequisite

## Steps
1. action
2. action

## Verify
<how to confirm success>

## Troubleshooting
- symptom -> next check
```

## Avoid

- long introductions before the task starts
- passive-voice instructions
- unexplained jargon
- giant tables full of prose paragraphs
