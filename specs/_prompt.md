# Specs Folder Prompt

This folder contains the project's specifications.

Portability contract
- If this repo is created from the Axiom template or installed via scaffold, the full upstream Axiom `specs/` set is not included.
- Treat these as the portable contracts:
  - `specs/_prompt.md` and `specs/_index.md`
  - `.opencode/skills/*/SKILL.md`
  - `.memory-bank/_prompt.md` and the Memory Bank folder prompts

Writing rules
- Specs are contracts. Prefer explicit requirements/invariants over prose.
- If you change behavior, update or create a spec first, then implement.
- Use stable, grep-friendly references (paths + headings).
- Avoid project secrets and sensitive data; redact as `[REDACTED]`.

Format
- Keep docs in Markdown.
- Prefer short sections and bullets.
- Include at least one example when specifying an interface.

Traceability (portable)
- When a change is tied to a work item, include at least one trace line:
`axiom:trace work_item=<WORK_ITEM_ID> spec=specs/<doc>.md plan= impl= test= doc= evidence= commit=`
