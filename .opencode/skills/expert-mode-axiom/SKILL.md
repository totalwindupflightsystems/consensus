---
name: expert-mode-axiom
description: >
  Sets up, activates, and manages a Axiom repository as an Expert Platform expert.
  Provides the reader/writer agent installation pattern, knowledge bank layout,
  data ingestion path configuration (git PR, Pandora Box, direct endpoint), and
  auto-pull/event-listener wiring. Load this skill when: provisioning a new expert
  repo, activating expert mode on an existing repo, debugging knowledge ingestion,
  or managing the expert's storage layout.
version: "1.0"
license: MIT
compatibility: opencode
metadata:
  workflow: expert-platform
  outputs: >
    .opencode/agents/expert-axiom.md, .opencode/agents/expert-writer-axiom.md,
    .opencode/prompts/expert-mode.md (instruction file — added to opencode.jsonc instructions array),
    .memory-bank/ expert layout, opencode.jsonc expert mode config,
    AGENTS.md expert persona section, knowledge ingestion wiring
  aliases:
    - expert-setup
    - expert-install
    - expert-mode
    - expert-platform-setup
  tags:
    - expert
    - expert-platform
    - agent
    - knowledge-management
    - ingestion
tags:
  vertical: [expert-platform]
  category: platform
  core: false
---

# Expert Mode — Setting Up a Axiom Repo as an Expert

> **The mental model**: An expert repo is a Axiom instance specialized for a domain.
> It has two agents — a reader that answers external queries and a writer that manages
> knowledge. External callers reach it through the Expert Platform Gateway; internal
> developers use any Axiom agent in expert mode.

---

## Prerequisites

Before activating expert mode, verify:

- [ ] **Expert Platform running** — `curl http://localhost:8080/health` returns `{"status":"ok"}`
- [ ] **Expert registered** — `GET /api/experts/{id}` returns your expert definition
- [ ] **OpenCode server running** in this repo — `opencode serve --port 14100` works
- [ ] **Expert token exists** — an `is_expert=true` token for this expert's internal API key
- [ ] **Git remote configured** — `git remote -v` shows the authoritative remote
- [ ] **`.memory-bank/_prompt.md` exists with `Expert ID:` field** — required for Pandora direct query (Path 0). If missing, create it from the template: `cp .opencode/skills/expert-mode-axiom/templates/_prompt.md .memory-bank/_prompt.md` then fill in your expert's name, domain, and ID.
- [ ] **(Optional) Pandora Box URL set** — `PANDORA_BOX_URL` env var if using Pandora ingestion (Path 2)

---

## Phase 1: Install the Agent Files

If not already present, copy the expert agent templates into this repo's `.opencode/agents/`:

```bash
# Option A: Copy from Axiom repo (if Axiom is installed)
cp $(axiom locate-skill expert-mode-axiom)/templates/expert-axiom.md .opencode/agents/
cp $(axiom locate-skill expert-mode-axiom)/templates/expert-writer-axiom.md .opencode/agents/

# Option B: Copy from a reference expert repo
# (ask your Axiom operator for the template source)
```

Verify both files exist:
```bash
ls .opencode/agents/expert-axiom.md .opencode/agents/expert-writer-axiom.md
```

Both files have `expert_mode_only: true` in their frontmatter — they are hidden in normal mode.

---

## Phase 2: Configure Expert Identity

Edit `AGENTS.md` in this repo to add the expert persona section:

```markdown
## Expert Identity

This repository is an Expert Platform expert. When in expert mode:
- `expert-axiom` is the primary agent (answers external queries)
- `expert-writer-axiom` manages knowledge ingestion

**Domain**: <e.g., "Security Review — threat modeling, secrets hygiene, vulnerability detection">
**Expert ID**: <e.g., "security-review"> (must match the registered expert_id in the platform)
**Skills**: <list the .opencode/skills/ loaded by this expert>
**Memory Layout**: .memory-bank/ (see expert-mode-axiom skill for layout)
```

---

## Phase 3: Initialize the Memory Bank Layout

Run this to set up the expert knowledge storage structure:

```bash
mkdir -p .memory-bank/knowledge .memory-bank/procedures \
         .memory-bank/captures/git .memory-bank/captures/pandora \
         .memory-bank/captures/direct .memory-bank/signals
```

Create the root index files:

```bash
# .memory-bank/_prompt.md — expert identity (edit with your domain)
cat > .memory-bank/_prompt.md << 'EOF'
# Expert Identity

**Name**: <Expert Name>  
**Domain**: <Domain>  
**Expert ID**: <expert-id>  
**Role**: I answer questions about <domain>. I read from my knowledge base before answering.
I do NOT write to the knowledge base directly — that is the writer's job.

## Navigation Rules
1. Start at `.memory-bank/_index.md`
2. Follow links to the relevant knowledge area
3. Cite sources in every response
4. If I don't know, I say so. I do not hallucinate domain facts.
EOF

# .memory-bank/_index.md — knowledge map
cat > .memory-bank/_index.md << 'EOF'
# Expert Knowledge Map

## Knowledge Areas
- `knowledge/` — domain knowledge by topic
- `procedures/` — how-to guides and workflows
- `captures/` — raw ingested content (staging)
- `signals/` — watching / trending items

## Expert Index
- `expert-index.md` — ingestion audit trail (maintained by writer)
EOF

# .memory-bank/expert-index.md — ingestion log (starts empty)
cat > .memory-bank/expert-index.md << 'EOF'
# Expert Knowledge Index

Last updated: (not yet updated)

| Source | Date | Type | Content Summary | Path | Status |
|--------|------|------|-----------------|------|--------|

<!-- expert-writer-axiom maintains this file. Do not edit manually. -->
EOF
```

---

## Phase 4: Activate Expert Mode in opencode.jsonc

Switch the repo into expert mode. This does **three things**:
1. Sets `expert-axiom` as the default agent
2. Adds the expert-mode instruction prompt to the `instructions` array
3. Configures permissions for read-only expert behavior

```json
{
  "$schema": "https://opencode.ai/config.json",
  "default_agent": "expert-axiom",
  "share": false,
  "instructions": ["AGENTS.md", ".opencode/prompts/expert-mode.md", ".opencode/prompts/memory-bank-client.md"],
  "permission": {
    "bash": {
      "git status": "allow",
      "git log*": "allow",
      "git diff*": "allow",
      "*": "deny"
    },
    "edit": "deny",
    "write": "deny",
    "read": "allow",
    "glob": "allow",
    "grep": "allow"
  }
}
```

**Key**: The `.opencode/prompts/expert-mode.md` instruction file is what makes the agent behave as an expert. It defines:
- Read-only behavior (no file modifications)
- Knowledge-first answering (navigate `.memory-bank/` before responding)
- Source citation requirements
- Domain boundary enforcement (don't answer outside your expertise)
- Writer delegation pattern (dispatch to `expert-writer-axiom` for knowledge updates)

Without this instruction file, the agent would behave like a normal Axiom agent — building, planning, and modifying code. With it, the agent stays in "expert reader" mode.

**The instruction file lives at**: `.opencode/prompts/expert-mode.md`

Copy it from the Axiom template during installation:
```bash
# Copy the expert-mode instruction prompt
cp $(axiom locate-skill expert-mode-axiom)/prompts/expert-mode.md .opencode/prompts/expert-mode.md
```

Or if installing manually, create it with the content from the Axiom reference repo's `.opencode/prompts/expert-mode.md`.

> **Reversing expert mode**: Change `default_agent` back to `tower-axiom` (or remove it)
> and remove `.opencode/prompts/expert-mode.md` from the `instructions` array.
> The expert agents remain in `.opencode/agents/` but are hidden from the default flow.

---

## Phase 5: Wire Data Ingestion

Choose which paths to enable. **Path 0 is the simplest — start here.**

**Path 0 vs Path 2 — which to use:**

| Situation | Use |
|-----------|-----|
| Small/dynamic knowledge base, queried live | **Path 0** — direct MCP query at query time |
| Large knowledge base (hundreds+ memories) | **Path 2** — pre-index into `.memory-bank/` |
| Need offline access (no Pandora at query time) | **Path 2** — pre-indexed content always available |
| Evaluation / quick start | **Path 0** — minimal setup (3 env vars) |
| Latency-sensitive production deployment | **Path 2** — no live query overhead |

### Path 0 — Direct Pandora Query (minimal setup: 3 env vars)

If Pandora Box MCP is configured in `opencode.jsonc`, `expert-axiom` queries Pandora directly at query time. No webhook, no sweep, no writer agent needed.

Add to your `opencode.jsonc`:

```jsonc
{
  "mcp": {
    "pandora-box": {
      "enabled": true,
      "type": "local",
      "command": ["npx", "-y", "--registry", "https://registry.npmjs.org", "@fl97inc/pandora-box-mcp@latest"],
      "environment": {
        "PANDORA_BOX_URL": "<your-pandora-box-url>",
        "PANDORA_BOX_JWT": "<your-jwt>",
        "PANDORA_BOX_TENANT_UUID": "<your-tenant-uuid>"
      }
    }
  }
}
```

The `expert-mode.md` instruction prompt (already in your `opencode.jsonc` instructions array) tells `expert-axiom` to query Pandora at the start of each session using `tags: ["expert:<id>"]`. No additional configuration needed.

**When to use Path 0**: when you want the expert to surface recent Pandora memories at query time without pre-indexing them. Best for dynamic, frequently-updated knowledge.

---

### Path 1 — Git Auto-Pull (safe, recommended for code/docs)

Register a webhook on your git remote that POSTs to the Expert Platform:

```bash
# GitHub: Settings → Webhooks → Add webhook
# Payload URL: https://<expert-platform-host>/api/experts/<expert-id>/webhook/git
# Content type: application/json
# Secret: <your EXPERT_PLATFORM_INTERNAL_API_KEY>
# Events: Push events, Pull request events (merged)
```

The Expert Platform validates the HMAC and calls `expert-writer-axiom` to pull and reconcile.

### Path 2 — Pandora Box Ingestion (pre-index into .memory-bank/)

When writing a Pandora Box memory that should be pre-indexed into the expert's `.memory-bank/`, include the expert tag:

```python
# Via the Pandora Box API (PostgREST remember_to):
{
  "tenant_slug": "expert-platform",
  "kind": "knowledge",
  "payload": {
    "tenant_id": "<pandora_tenant_uuid>",
    "content_raw": "Your knowledge content here",
    "tags": ["expert-platform", "expert:security-review"],  # ← expert tag
    "title": "JWT Security Patterns"
  }
}
```

The Expert Platform sweep detects the `expert:<id>` tag and notifies the writer to ingest into `.memory-bank/`. **Use this when you want memories pre-indexed for offline access or large knowledge bases.**

### Path 3 — Direct Writer Endpoint (disabled by default)

To enable direct data writes, set in the expert config:

```yaml
# .axiom/experts/<expert-id>.yaml
id: security-review
# ... other fields ...
runtime:
  writer_endpoint_enabled: true   # ⚠️ enables dangerous path
```

When enabled, `POST /api/experts/{id}/write` with an admin token writes to `captures/direct/`. The writer does NOT auto-promote direct writes — they require operator review.

---

## Phase 6: Verify the Setup

Run these checks to confirm expert mode is working:

```bash
# 1. Check agent files exist
ls -la .opencode/agents/expert-*.md

# 2. Check memory bank layout
find .memory-bank -name "_index.md" | head -10

# 3. Check OpenCode picks up the expert agent
opencode serve --port 14100 &
curl -s -X POST http://localhost:14100/session | python3 -m json.tool
# Expect: session created; agent=expert-axiom

# 4. Test a query through the Expert Platform Gateway
curl -H "Authorization: Bearer $TOKEN" \
  -X POST http://localhost:8080/api/experts/<expert-id>/request \
  -d '{"message": "Hello, what is your domain?", "delivery": {"mode": "sync"}}'
# Expect: expert responds with domain description

# 5. Test git pull trigger (manual)
# In OpenCode session with expert-writer-axiom:
# "Pull the latest changes from git remote"
```

---

## Managing the Expert

### Updating domain knowledge

In **expert mode**, open a session and use the writer:

```
"I want to add a new knowledge item about [topic]"
→ expert-writer-axiom writes to captures/, then promotes
```

Or commit directly to the git repo and let auto-pull handle it:
```bash
git add .memory-bank/knowledge/security/oauth2-patterns.md
git commit -m "feat(expert): add OAuth2 patterns knowledge"
git push origin main
# → webhook fires → expert-writer-axiom pulls and indexes
```

### Navigating storage

The reader navigates your storage automatically. For manual inspection:

```
.memory-bank/_index.md          ← start here
.memory-bank/expert-index.md    ← see what has been ingested and when
.memory-bank/knowledge/         ← promoted, validated knowledge
.memory-bank/captures/          ← staged content awaiting review
```

### Switching back to development mode

When you want to modify the expert itself (not just answer queries):

1. Change `opencode.jsonc` → `"default_agent": "tower-axiom"` (or remove)
2. Now you have full Axiom capabilities: tower, dev-axiom, specwriter, etc.
3. Make your changes (add skills, update specs, refactor knowledge structure)
4. Switch back: `"default_agent": "expert-axiom"`

---

## Architecture Reference

```
Expert Repository
├── .opencode/
│   ├── agents/
│   │   ├── expert-axiom.md          ← reader (answers external queries)
│   │   └── expert-writer-axiom.md   ← writer (manages knowledge)
│   └── skills/                        ← domain skills loaded by the reader
├── specs/                             ← domain specs (contracts)
├── .memory-bank/
│   ├── _index.md                      ← knowledge map
│   ├── _prompt.md                     ← expert identity
│   ├── expert-index.md                ← ingestion audit trail
│   ├── knowledge/                     ← promoted domain knowledge
│   ├── procedures/                    ← workflows and how-tos
│   ├── captures/                      ← staging area (git/ pandora/ direct/)
│   └── signals/                       ← watching / trending
├── AGENTS.md                          ← expert persona + operating rules
└── opencode.jsonc                     ← default_agent: expert-axiom

Expert Platform Gateway
├── Receives external query
├── Routes to expert's OpenCode server
├── Creates session → expert-axiom answers
└── Returns result via delivery mode

Data Ingestion
├── Path 0: Pandora MCP configured → expert-axiom queries Pandora directly at query time
├── Path 1: git push → webhook → expert-writer-axiom → git pull + index
├── Path 2: Pandora Box write + tag → sweep → expert-writer-axiom → ingest into .memory-bank/
└── Path 3: POST /api/experts/{id}/write → expert-writer-axiom → captures/ (manual review)
```

---

axiom:trace work_item=SWDE-43 spec=specs/104-Expert-Platform.md#REQ-EXP-A-001,REQ-EXP-A-004,REQ-EXP-A-006,REQ-EXP-A-007,REQ-EXP-A-008,REQ-EXP-A-009 plan=expert-agent-architecture
