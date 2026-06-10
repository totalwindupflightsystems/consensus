---
name: changelog-release-notes-writing-axiom
description: Style guide for changelogs and release notes. Separates developer-facing chronological change logs from plain-language stakeholder release notes.
version: "1.0"
tags:
  vertical: [writing]
  category: writing
  core: false
---

# Changelog and Release Notes Writing

Use for versioned change summaries, release announcements, and customer-facing release narratives.

axiom:trace work_item=DEX-73 spec= plan= test= doc=.memory-bank/explorations/writing-style-skill-collection.md prompt=.opencode/skills/changelog-release-notes-writing-axiom/SKILL.md evidence= commit= jira_ref=DEX-73

## Two Modes

| Mode | Audience | Style |
|---|---|---|
| Changelog | developers and maintainers | chronological, grouped by change type, technical but curated |
| Release notes | customers and stakeholders | plain language, impact-first, links to docs and known issues |

## Changelog Rules

- Keep one entry per version.
- Group by change type such as Added, Changed, Fixed, Removed.
- Put newest version first.
- Include release dates and linkable versions.
- Curate; do not dump raw git log text.

## Release Notes Rules

- Explain what changed and why it matters.
- Use plain language.
- Highlight new features, fixes, known issues, and upgrade impacts.
- Link to docs when readers may need more detail.

## Avoid

- mixing customer prose into the developer changelog without labeling it
- raw commit-message dumps
- burying breaking changes under minor fixes
