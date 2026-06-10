# axiom-install

Portable guidance for installing, upgrading, syncing, and onboarding Axiom into an existing repository or multi-repo workspace.

Every install must include the skill map routing layer: `.opencode/skills/axiom-skill-map/`, `.opencode/prompts/skill-map-client.md`, and an `opencode.jsonc` `instructions` entry for the skill-map prompt. This keeps agents from guessing which skill to load and applies required runtime-aware routing, including GPT/OpenAI writing correction when applicable.

See
- `.opencode/skills/axiom-install/SKILL.md` — Main install/upgrade/sync/onboarding guide
- `.opencode/skills/axiom-install/MULTI-REPO.md` — Multi-repo workspace setup guide
- `.axiom/scaffold/install.py` — Single-repo installer script
- `.axiom/scaffold/workspace-setup.py` — Multi-repo workspace setup script
- `scripts/sync-upstream.py` — Selective sync of upstream-owned files (agents, skills, commands, prompts)
- `scripts/link-install.py` — Symlink install script (submodule method)
