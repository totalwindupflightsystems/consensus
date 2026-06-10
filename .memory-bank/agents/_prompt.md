# Agents Folder Rules

Scope:
- Store durable agent-specific context here (profiles, preferences, patterns, reflections).
- Do not store project decisions here (use `.memory-bank/projects/`).
- Do not store cross-project evergreen knowledge here (use `.memory-bank/topics/`).

Invariants:
- Each agent subfolder MUST contain `_index.md` and `_prompt.md`.
- Never store secrets.

Default files per agent:
- `profile.md` - what this agent is for and how it behaves
- `reflection.md` - recurring mistakes and improvements
