---
name: too-much-of-a-good-thing-axiom
description: Balance-restoring skill for outputs that have become over-optimized, over-structured, or too strict to read, use, or maintain comfortably.
version: "1.0"
tags:
  vertical: [coding, writing]
  category: methodology
  core: false
---

# Too Much of a Good Thing

Use this skill when an output is technically correct but harder to read, trust, or work with because it has too much structure, too much ceremony, too many caveats, or too much optimization for one dimension.

axiom:trace work_item=writing-balance-01 spec= plan= test= doc=.opencode/skills/too-much-of-a-good-thing-axiom/SKILL.md evidence= commit=

## Core Principle

Good patterns can become bad outcomes when pushed past the point of usefulness.

- Bullets help until they turn into checklist soup.
- Prose helps until it becomes a wall.
- Rules help until they make the work brittle.
- Verification helps until it drowns the main point.
- Precision helps until the reader loses the thread.

Optimize for the reader or operator finishing the task successfully, not for local perfection inside one formatting rule.

## External Anchors

These references support the same balance principle:

- Nielsen Norman Group notes that bullets improve scanability, but only when they actually make information easier to grasp.
- Technical writing guides warn that too many list items can have the reverse effect and reduce readability.
- Goldilocks-style design guidance points to the same pattern: too much detail distracts, too little detail under-informs, and the right amount helps comprehension.

Do not cargo-cult these sources. Use them as reminders that readability is a balance problem, not a maximization problem.

## When To Load This Skill

Load this skill when you see any of these failure modes:

- the page is technically organized but visually flat
- every section uses the same structure and starts blending together
- the same fact appears in prose, bullets, and tables
- headings are present but do not create real separation
- every caveat is in the main path instead of being staged by importance
- the output feels correct but annoying to read
- process safeguards make the work hard to execute in practice
- the user says some version of "too much of a good thing"

## Red Flags

### Writing and docs

- one section contains both a paragraph, a list, and a table that say nearly the same thing
- every item gets the same visual weight regardless of importance
- labels like `Problem`, `Design`, `Status`, `Why it matters`, and `Next step` are repeated so often that they become noise
- short sections are over-broken into many tiny subheadings
- long sections have no internal anchors at all

### Plans and process

- every step has the same strict ceremony even when the step is trivial
- the rollback or evidence requirement is more work than the change itself
- edge cases dominate the default path
- the plan optimizes for completeness at the cost of momentum

### UX and interfaces

- everything is highlighted so nothing stands out
- every card, panel, or block has the same density and treatment
- too many controls are visible at once because every possible need was surfaced immediately

## Balancing Heuristics

### 1. Pick one primary organizing device per section

Use one of these as the dominant structure:

- short prose block
- bullet list
- table
- numbered steps

You can mix formats, but one should clearly lead. If three formats compete, collapse to one primary form and one supporting form.

### 2. Separate ideas by function, not by habit

Break content apart when the pieces do different jobs.

- heading = change of topic
- bullet = set of parallel items
- table = comparison or dense mapping
- prose = framing, tradeoff, or narrative

Do not add structure just because structure exists.

### 3. Keep the main path obvious

Put the default understanding or action first. Move caveats, exceptions, and edge cases later unless they are safety-critical.

### 4. Collapse repeated truth

If the same point appears in multiple forms, keep the strongest one and cut the rest.

Priority order:

1. table for dense comparison
2. bullet for parallel scan
3. prose for nuance

Do not restate the same fact in all three.

### 5. Use just enough labeling

Labels are useful when they create contrast. They stop working when every paragraph starts with a bold label.

Prefer labels when:

- the reader needs a stable pattern to scan
- the section compares the same dimensions repeatedly

Avoid labels when:

- the section is already short and obvious
- labels repeat without adding navigation value

### 6. Let importance control density

Spend density where the stakes are high.

- core message: shortest and clearest form
- supporting detail: compact
- deep nuance: linked or deferred

Do not give minor details the same space and emphasis as the main takeaway.

## Repair Loop

When something feels "correct but ugly," run this loop:

1. Identify the dominant irritation.
   - too wall-of-text
   - too listy
   - too repetitive
   - too same-weight
   - too many caveats in the main flow

2. Remove one layer of structure.
   - convert sub-bullets into a sentence
   - collapse multiple bullets into one grouped bullet
   - merge tiny headings into one stronger heading

3. Strengthen one visual anchor.
   - promote a heading level
   - add one short summary sentence
   - split one paragraph where the topic actually changes

4. Re-scan top to bottom in 5 seconds.
   Ask:
   - Can I tell what the section is about before reading full paragraphs?
   - Do the headings visually separate ideas?
   - Is the default path obvious?
   - Does any section feel like too much of the same thing?

5. Stop when the document becomes easier to use.
   Do not keep polishing past the point of improvement.

## Practical Patterns

### Better than wall-of-text

```markdown
### Morty: parallel agent orchestration

Ralph runs sequentially. Morty is designed to remove that bottleneck.

- Problem: sequential loop burns time with idle LLM capacity.
- What it is: config-driven Go orchestrator with worktree-based concurrency.
- Status: spec complete, plan written, scaffold in repo.
```

### Better than bullet overload

```markdown
### Also shipped

- Traceability: external refs, commit enforcement, config-driven Jira lookup.
- Operator tooling: copilot guidance, workflow skills, emergency scaffolding.
- Verification: debug inspection, registry conformance, lifecycle sequencing tests.
```

### Better than repeating everything three times

If a roadmap table already explains what and why, do not follow it with three prose sections that restate the same roadmap item in full.

## Decision Rules

When choosing between two valid versions, prefer the one that:

- reduces reader effort
- keeps the main claim honest
- makes hierarchy visible faster
- preserves flexibility for future edits
- can be maintained without ceremony

## Anti-Patterns

- optimizing every section for maximum structure
- turning every paragraph into a labeled pattern
- using more headings to solve a clarity problem caused by weak wording
- overfitting to one user's last criticism and creating the opposite problem
- adding process friction in the name of quality without checking usability

## One-Line Reminder

Too much of a good thing is still too much. Aim for useful tension, not total control.
