## Skill Map Client Rules

Axiom includes a loadable skill map at `.opencode/skills/axiom-skill-map/SKILL.md`. Use it whenever the right skill or agent is unclear, when the request spans multiple domains, or when a task starts feeling like “which Axiom capability should handle this?”

The skill map is normally advisory, but its `global.conditional_required` rules are mandatory when the harness/runtime condition matches. In particular, if the harness says the active model is OpenAI-provided or GPT-family, load `gpt-paragraph-first-writing-axiom` for human-facing answers so responses use organized writing: intro/body/exit, paragraphs with subject/support/closure, and lists or tables only when they genuinely fit the information.

Do not scan every skill description by hand unless the skill map fails to route the task. Start with the map, then load the specific skills it recommends.

axiom:trace work_item=writing-style-humanization-01 spec=specs/85-Skill-Map-Decision-Tree.md prompt=.opencode/prompts/skill-map-client.md doc=.opencode/skills/axiom-skill-map/SKILL.md
