---
name: personal-context-axiom
description: >
  Transforms Axiom into a personal operating system for non-code work. Specs become
  hard SOPs (contracts), skills become soft SOPs (guidance), the memory bank tracks life
  and work context across sessions, and work items track goals and projects — not just
  code. Covers AGENTS.md persona wiring, SOP authoring, the full personal memory bank
  folder taxonomy (including captures/, signals/, contacts/, and reference/), and
  MCP-heavy workflows for reaching Notion, Jira, browser, Slack, email, and CLI.
version: "1.1"
license: MIT
compatibility: opencode
metadata:
  workflow: personal-os
  outputs: >
    AGENTS.md persona sections, .opencode/prompts/personal-context-mode.md (instruction file — added to opencode.jsonc instructions array),
    .memory-bank/ personal folder structure,
    specs/ SOP files, .opencode/skills/ personal workflow skills,
    .opencode/agents/ domain-specialist agents, work-item goal artifacts
  aliases:
    - personal-os
    - personal-assistant
    - life-ops
    - personal-axiom
    - chief-of-staff
  tags:
    - personal
    - non-code
    - sop
    - life-management
    - goals
    - mcp-heavy
    - context
    - captures
    - signals
tags:
  vertical: [personal-context]
  category: personal
  core: false
---

# Personal Context — Axiom as a Personal Operating System

> **The mental model shift:** Axiom was built for software teams. But the underlying
> machinery — specs as contracts, memory bank as durable context, work items as tracked
> goals, agents as specialists, MCP tools as reach — works just as well for *life* as
> for code. This skill teaches you how to "hack" Axiom into a personal AI chief of staff.

---

## Prerequisites (check these before starting)

Before setting up personal mode, verify you have:

- [ ] **Git** — `git --version` should return a version number
- [ ] **A code editor** — VS Code, Cursor, Zed, or any editor that can open markdown files
- [ ] **An AI assistant that reads file-based prompts** — OpenCode, Claude Code, or similar.
  The entire value of this system depends on the AI reading `AGENTS.md` at session start.
  Verify your tool supports this before investing time in setup.
- [ ] **A private repo** (strongly recommended) — personal content (contacts, health,
  finances) should not be in a public repo. Create a private GitHub/GitLab repo or
  keep it local-only.
- [ ] **MCP tools** (optional but powerful) — Notion, Jira, browser MCPs extend what
  the AI can reach. These are configured in `opencode.jsonc`. See the MCP section below.
  The system works without them — MCPs just add reach.

**If you're missing the AI assistant**: stop here. The rest of this skill assumes an
AI agent that reads `AGENTS.md` and the memory bank. Without that, you're just
creating markdown files.

---

## When to Load This Skill

Load this skill when:

- You want Axiom to help you manage goals, projects, or routines that are **not code**
- You are setting up a personal Axiom instance (not a software repo)
- You want to author **SOPs** (standard operating procedures) that the AI must follow
- You want the memory bank to track **personal context** — goals, decisions, Slack messages, emails, Notion docs, meeting notes, random captures
- You want work items to represent **goals and objectives**, not just software tickets
- You are building workflows that rely heavily on **MCP tools** (browser, Jira, Notion, GitHub, calendar, email, etc.)
- You want to override the default "software engineer" persona in `AGENTS.md`

---

## Core Mental Model

### The Four Layers (Personal Edition)

| Layer | Software Axiom | Personal Axiom |
|-------|-----------------|-----------------|
| **Specs** | API contracts, system behavior | SOPs — hard rules you always follow |
| **Skills** | Engineering playbooks | SOPs — soft guidance, best practices, checklists |
| **Memory Bank** | Project context, decisions, evidence | Life context, goals, decisions, captures, signals |
| **Work Items** | Code features, bugs, releases | Goals, projects, habits, research tasks |

### What Changes

- **Specs are your hard SOPs.** If you write `specs/morning-routine.md`, the AI treats it as a contract. It will follow it, verify against it, and flag deviations. Use this for things that must always happen a certain way.
- **Skills are your soft SOPs.** If you write a skill for "how I like to research topics" or "my weekly review format", the AI loads it when relevant and follows it as guidance — not a hard contract.
- **The memory bank is your second brain.** It tracks context across sessions. What you're working on, decisions you've made, people you've talked to, goals you're pursuing, random things that landed in your inbox.
- **Work items are your goals.** A work item doesn't have to produce code. It can be "plan Q2 travel", "research mortgage refinancing", or "build a reading habit". The same lifecycle (intake → plan → execute → verify → done) applies.

---

## AGENTS.md — The Most Important File to Get Right

> **This is the highest-leverage change you can make.** `AGENTS.md` is read by every agent
> at the start of every session. Getting it right means the AI always has your context,
> your rules, and your persona — without you having to repeat yourself.

### What AGENTS.md Does in Personal Mode

In software mode, `AGENTS.md` tells agents about the codebase, build commands, and engineering rules. In personal mode, it tells agents:

1. **Who you are and what you're working on** — so responses are relevant
2. **Your hard rules** — things the AI must always or never do
3. **Where your context lives** — pointers into the memory bank so agents know where to look
4. **Your tool stack** — which MCP tools are available and how to use them
5. **Your persona preference** — how you want the AI to behave and communicate

### Minimum Viable AGENTS.md (start here)

> **Fill in these 4 fields first.** This is enough to get a useful experience.
> Add the full template sections later as you need them.

```markdown
## Personal Operating Mode

This repo is a personal operating system, not a software project.
Specs are hard SOPs. Skills are soft SOPs. Work items are goals.
The memory bank is my second brain. Always check it before asking me to repeat context.

## Who I Am

[2-3 sentences: who you are, what you do, what you're currently focused on]

## My Hard Rules

1. [Rule 1 — specific and non-negotiable]
2. [Rule 2]
3. [Rule 3]

## My Tool Stack

MCP tools available: [list which ones: Notion / Jira / GitHub / browser / none]
Primary external services: [Notion workspace / Jira project / GitHub org / etc.]
```

That's it. The AI can work with just these 4 fields. Add more sections as you go.

---

### The Full Personal AGENTS.md Template

Add this section to your `AGENTS.md`. Replace every `[...]` placeholder with your actual content.

```markdown
## Personal Operating Mode

This repo is a personal operating system, not a software project.

- `specs/` contains SOPs (standard operating procedures). Treat them as hard contracts —
  follow them exactly, verify against them, and flag deviations.
- `.opencode/skills/` contains soft SOPs and workflow guides. Load them when relevant.
- Work items in `.memory-bank/work-items/` track goals and projects, not code.
- The memory bank is the primary context store. Always check it before asking me to
  repeat context I've already provided.
- MCP tools (browser, Jira, Notion, Slack, email, CLI) are primary execution surfaces.
  Reach out to them proactively when a task requires it.
- "Done" means the goal was achieved and evidence exists — not that code was shipped.

---

## My Identity and Context

> [Write 3-5 sentences about yourself: who you are, what you do, what you're currently
> focused on. This is the single most important thing to fill in — it makes every
> response more relevant.]

Example:
> I'm a product manager at a Series B startup. I'm currently focused on launching our
> Q3 roadmap, managing a team of 6, and building a personal knowledge management system.
> I work remotely and use Notion as my primary external brain.

---

## My Hard Rules (Always Follow These)

> [List 3-7 non-negotiable rules. These are things the AI must always or never do.
> Be specific. Vague rules get ignored.]

Examples — replace with your own:
- Always check `.memory-bank/activeContext.md` before starting any task.
- Always check `.memory-bank/captures/` for relevant prior captures before researching a topic.
- Never schedule or suggest work on Fridays — that's my deep work / no-meeting day.
- When I ask about a person, check `.memory-bank/contacts/` first.
- When I mention a Notion page, fetch it via MCP before responding.
- When a task will take more than one session, create a work item before starting.
- Always end a session by updating `.memory-bank/activeContext.md` with what changed.

---

## Memory Bank Navigation

> The memory bank is organized as a map-of-maps. Here are the key locations:

| What you need | Where to look |
|---------------|--------------|
| Current priorities and open questions | `.memory-bank/activeContext.md` |
| Decisions and rationale | `.memory-bank/decisionLog.md` |
| Goals and projects | `.memory-bank/work-items/` |
| Random captures (Slack, email, notes) | `.memory-bank/captures/` |
| Signals (things to watch, track, or follow up) | `.memory-bank/signals/` |
| People and relationships | `.memory-bank/contacts/` |
| Reference material (docs, research, saved content) | `.memory-bank/reference/` |
| Domain knowledge (health, finances, learning, etc.) | `.memory-bank/topics/` |
| My personal doctrine and non-negotiables | `.memory-bank/soul.md` |
| Full memory bank map | `.memory-bank/_index.md` |

---

## My Tool Stack

> [List the MCP tools and external services you have available. Agents use this to
> know what they can reach without asking you.]

Available MCP tools:
- [ ] Notion MCP — for reading/writing Notion pages and databases
- [ ] Jira MCP — for reading/writing Jira tickets
- [ ] GitHub MCP — for reading/writing GitHub issues and PRs
- [ ] Chrome DevTools MCP — for browser automation and research
- [ ] Atlassian MCP — for Confluence pages

External services I use:
- [ ] Notion workspace: [workspace name or URL hint]
- [ ] Jira project: [project key]
- [ ] GitHub org: [org name]
- [ ] Slack workspace: [workspace name]
- [ ] Email: [provider — Gmail, Outlook, etc.]

---

## My Communication Preferences

> [Tell the AI how you want it to communicate with you.]

- Tone: [Direct / Warm / Formal / Casual]
- Length: [Concise — get to the point / Thorough — explain your reasoning]
- Format: [Bullets preferred / Prose preferred / Tables for comparisons]
- When uncertain: [Ask me / Make a reasonable assumption and note it / Fail closed]
- Preamble: [Skip it — start with the answer / Brief framing is fine]

---

## My AI Persona for This Workspace

You are my personal AI chief of staff. Your job is to help me:
- Execute on my goals with the same rigor a great team brings to complex work
- Track context across sessions so I don't have to repeat myself
- Follow my SOPs (specs) exactly and my soft guidelines (skills) thoughtfully
- Reach external tools (Notion, Jira, browser, CLI) to get things done
- Surface risks, open questions, and blockers before they become problems
- Capture things I mention in passing into the right place in the memory bank

You are NOT a code generator in this workspace. You are an execution partner.
```

### Key Pointers in AGENTS.md

The memory bank navigation table in `AGENTS.md` is critical — it tells every agent exactly where to look for different types of context. **Keep it updated as you add new folders.** When you add a new folder to the memory bank, add a row to this table.

---

## Personal Onboarding

### Step 1 — Activate assist-axiom in Your Project's opencode.jsonc

> **This is the one config change that wires everything together.**
> `assist-axiom` is not the global default agent — it lives in `.opencode/agents/`
> and you activate it per-project. The global Axiom default (`tower-axiom`) stays
> untouched for engineering repos.

In your personal project's `opencode.jsonc`, set `default_agent` and add the
personal-context instruction prompt to the `instructions` array:

```jsonc
{
  "$schema": "https://opencode.ai/config.json",

  // Activate the personal AI chief of staff for this project
  "default_agent": "assist-axiom",

  // Instruction files loaded into every session — these define how the agent behaves.
  // personal-context-mode.md is the key file: it tells the agent "you are a personal
  // assistant, not a software engineer" and defines the operating rules.
  "instructions": ["AGENTS.md", ".opencode/prompts/personal-context-mode.md", ".opencode/prompts/memory-bank-client.md"],

  // Keep the same model and permissions as your Axiom install
  // (copy from your Axiom opencode.jsonc and adjust as needed)
  "permission": {
    "bash": "allow",
    "edit": "allow",
    "write": "allow",
    "read": "allow",
    "glob": "allow",
    "grep": "allow",
    "webfetch": "allow",
    "patch": "allow"
  },

  // Add MCP servers you want available in this personal project
  // (Atlassian MCP gives you Jira + Confluence; add others as needed)
  "mcp": {
    "chrome-devtools": {
      "enabled": true,
      "type": "local",
      "command": ["npx", "-y", "--registry", "https://registry.npmjs.org", "chrome-devtools-mcp@latest"]
    }
  }
}
```

**The instruction file** (`.opencode/prompts/personal-context-mode.md`) is what makes
the agent behave as a personal chief of staff instead of a software engineer. It defines:
- Context-first behavior (check memory bank before acting)
- SOP enforcement (specs/ are hard rules)
- Privacy awareness (personal data handling)
- Proactive surfacing (reminders, deadlines, conflicts)
- Memory bank navigation and writing rules

**Copy it from the skill bundle during installation:**
```bash
mkdir -p .opencode/prompts
cp $(axiom locate-skill personal-context-axiom)/prompts/personal-context-mode.md .opencode/prompts/
```

Or create it manually from the reference in the Axiom repo at `.opencode/prompts/personal-context-mode.md`.

**If you're adding personal mode to an existing Axiom engineering repo** (not a
separate personal repo): don't change the global `opencode.jsonc`. Instead, you can
invoke `assist-axiom` explicitly in a session by mentioning it by name, or create
a separate personal workspace directory with its own `opencode.jsonc`.

**If you're starting a fresh personal repo**: this is your entire `opencode.jsonc`.
Copy the snippet above, adjust the MCP section for the tools you have, and you're done.

> **Note on MCP tools**: `mcp.atlassian` (Jira + Confluence) and `mcp.chrome-devtools`
> (browser automation) are the two MCPs configured in the base Axiom install. If you
> have Notion, GitHub, or other MCP servers, add them here. The agent degrades gracefully
> when MCPs are unavailable — it falls back to local memory bank files.

### Step 2 — Reframe Your Repo

Your "repo" is your personal workspace. It doesn't need code. It needs:

```
opencode.jsonc         ← sets assist-axiom as default_agent (done in Step 1)
.memory-bank/          ← your second brain (see full structure below)
specs/                 ← your hard SOPs
.opencode/skills/      ← your soft SOPs and playbooks
.opencode/agents/      ← your AI team personas (assist-axiom lives here)
AGENTS.md              ← global persona and rules (fill this in next)
```

If you're starting fresh, run `/axiom-init` and then follow this skill to reshape the defaults.

### Step 3 — Fill In AGENTS.md

Use the template above. The minimum viable fill-in is:
1. **My Identity and Context** — who you are and what you're working on
2. **My Hard Rules** — 3-5 non-negotiable rules
3. **My Tool Stack** — which MCP tools are available

Everything else can be added incrementally.

### Step 4 — Seed Your Memory Bank Root Files

| File | What to put in it |
|------|------------------|
| `projectBrief.md` | Your current major goals, domains you're managing, definition of "done" for each |
| `soul.md` | Your non-negotiables, how you make decisions under pressure, trade-offs you always make the same way |
| `activeContext.md` | Current top 3 priorities, open questions, constraints active this week/month |
| `techContext.md` | Your tool stack, MCP tools available, external services, CLI tools |
| `decisionLog.md` | Start with one entry: the decision to use Axiom as a personal OS |

> **If `soul.md` already exists** (e.g., you're adding personal mode to an existing
> engineering repo): don't overwrite it. Instead, add a `## Personal Doctrine` section
> at the bottom of the existing file. The engineering doctrine and personal doctrine
> can coexist in the same file — they're both about how you operate under pressure.

### Step 5 — Write Your First SOP Spec

Pick one routine or process you want the AI to always follow. Write it as a spec. See the SOP Authoring Guide below.

### Step 6 — Create Your First Goal Work Item

Pick your most important current goal. Run `/axiom-work-item` or create the folder manually:

```
.memory-bank/work-items/<your-goal-id>/
  overview.md    ← what the goal is, why it matters, definition of done
  plan.md        ← phases, tasks, steps with verification
  context.md     ← relevant background, constraints, decisions
```

---

## The Personal Memory Bank — Full Folder Taxonomy

The standard Axiom memory bank covers code projects well. Personal use needs additional folders for the kinds of things that actually land in your life: random Slack messages, emails, Notion docs, meeting notes, people you need to remember, things you're tracking.

### Core Folders (always present)

```
.memory-bank/
  _index.md              ← root map — update this when you add folders
  _prompt.md             ← global rules (highest authority)
  projectBrief.md        ← your life/work context overview
  soul.md                ← your personal doctrine and non-negotiables
  activeContext.md       ← current focus, open questions, active constraints
  decisionLog.md         ← append-only: decisions you've made and why
  techContext.md         ← your tool stack and MCP availability
  
  work-items/            ← one folder per active goal or project
  topics/                ← domain knowledge (health, finances, learning, work, etc.)
  inbox/                 ← agent-to-agent messages (internal)
  agents/                ← agent profiles and reflections
  findings/              ← lessons learned, anti-patterns, adversarial findings
```

### Personal Extension Folders

These folders are specific to personal-mode use. Create them as you need them.

#### `captures/` — The Inbox for Random Things

> **Purpose:** A landing zone for anything that arrives and needs to be processed later.
> Slack messages, emails, random notes, things someone said, articles you want to read,
> ideas that hit you in the shower. If it doesn't have a home yet, it goes here.

```
.memory-bank/captures/
  _index.md              ← list of unprocessed and recently processed captures
  _prompt.md             ← rules: what goes here, how to process, when to promote
  <date>-<slug>.md       ← one file per capture (or batch captures by date)
```

**What goes in a capture:**
- The raw content (paste the Slack message, email snippet, note)
- Source: where it came from (Slack #channel, email from X, meeting with Y)
- Date received
- Status: `unprocessed` | `processed` | `promoted` (moved to a better home)
- Optional: tags, related work items, action needed

**Processing rule:** Captures are temporary. During your weekly review (or whenever you process your inbox), each capture should either:
- Be **promoted** to a better home (work item, topic, contact, reference, decision)
- Be **discarded** (not worth keeping)
- Stay as a capture with a "revisit" date if you're not ready to decide

**Capture template:**
```markdown
---
mb:
  type: capture
  title: "[Source] Brief description"
  created: YYYY-MM-DD
  tags: []
  status: unprocessed
  source:
    type: slack | email | meeting | note | article | other
    ref: "#channel-name | from: person@email.com | meeting: YYYY-MM-DD"
  links:
    up: "../_index.md"
    related: []
---

# Capture: [Brief Description]

## Raw Content
[Paste the original content here — Slack message, email, note, etc.]

## Context
[Any context that helps make sense of this — who sent it, why it matters, what triggered it]

## Possible Action
[What might need to happen with this? Leave blank if unknown.]

## Status
- [ ] Processed
- [ ] Promoted to: [location]
- [ ] Discarded
```

---

#### `signals/` — Things You're Watching or Tracking

> **Purpose:** Ongoing things you want to keep an eye on. Not a task, not a decision —
> just something you're monitoring. A competitor's product, a person's career trajectory,
> a market trend, a health metric, a relationship dynamic.

```
.memory-bank/signals/
  _index.md              ← table of active signals with last-updated dates
  _prompt.md             ← rules: what's a signal vs. a topic vs. a work item
  <slug>.md              ← one file per signal
```

**What makes something a signal (vs. a work item or topic):**
- You're not actively doing anything about it right now
- You want to be notified or reminded if something changes
- It's a "watch" not a "do"

**Signal template:**
```markdown
---
mb:
  type: signal
  title: "Signal: [What you're watching]"
  created: YYYY-MM-DD
  updated: YYYY-MM-DD
  status: active | paused | resolved | promoted
  tags: []
  links:
    up: "../_index.md"
    related: []
---

# Signal: [What You're Watching]

## What I'm Watching
[Describe the thing you're monitoring — be specific enough that future-you knows what to look for]

## Why It Matters
[Why does this signal matter to you? What decision or action might it inform?]

## Check Frequency
[How often should this be reviewed? Weekly / Monthly / On trigger]

## Trigger Condition
[What would cause you to act on this? "If X happens, then Y"]

## Updates
| Date | What I observed |
|------|----------------|
| YYYY-MM-DD | [observation] |

## Status
- [ ] Still active — last checked: YYYY-MM-DD
- [ ] Promoted to work item: [work-item-id]
- [ ] Resolved: [what happened]
```

---

#### `contacts/` — People Context

> **Purpose:** Context about people that matters across sessions. Not a CRM — just
> enough context so the AI can give relevant advice when you mention someone by name.
> Relationship history, communication preferences, shared context, commitments made.

```
.memory-bank/contacts/
  _index.md              ← alphabetical list of contacts with one-line descriptions
  _prompt.md             ← rules: what goes here, privacy, what NOT to store
  <firstname-lastname>.md  ← one file per person
```

**Privacy rule:** Only store context that is professional/appropriate to have in a work tool. No sensitive personal information. No health details about others. No private communications without consent. When in doubt, leave it out.

**Contact template:**
```markdown
---
mb:
  type: contact
  title: "[Name]"
  created: YYYY-MM-DD
  updated: YYYY-MM-DD
  tags: [colleague | client | vendor | friend | family | advisor]
  links:
    up: "../_index.md"
    related: []
---

# [Name]

## Who They Are
[Role, company, how you know them — 1-3 sentences]

## Why They Matter to Me
[What's the relationship? What do we work on together? What do I need to remember?]

## Communication Notes
[How do they prefer to communicate? What's their style? What works well?]

## Shared Context
[Projects, decisions, or commitments we share. Keep this factual and professional.]

## Recent Interactions
| Date | What happened |
|------|--------------|
| YYYY-MM-DD | [brief note] |

## Open Items
- [ ] [Something I owe them or they owe me]
```

---

#### `reference/` — Saved Content and Research

> **Purpose:** Long-form content you've saved for future reference. Research findings,
> saved articles, documentation you've read, notes from books or courses, external
> documents you want to be able to find again. The difference from `topics/` is that
> reference is about specific artifacts (a document, an article, a book) while topics
> are about domains of knowledge you're building.

```
.memory-bank/reference/
  _index.md              ← searchable table of saved content (title, source, date, tags)
  _prompt.md             ← rules: what goes here vs. topics, how to tag, when to promote
  <slug>.md              ← one file per saved item
```

**Reference template:**
```markdown
---
mb:
  type: reference
  title: "[Title of the thing you're saving]"
  created: YYYY-MM-DD
  tags: []
  source:
    type: article | book | doc | video | meeting-notes | email | slack | notion
    ref: "[URL or identifier]"
    author: "[Author if known]"
  links:
    up: "../_index.md"
    related: []
---

# [Title]

## Why I Saved This
[One sentence: why this is worth keeping]

## Key Points
[Bullets — the things you actually want to remember]

## Quotes Worth Keeping
> [Any direct quotes that are worth preserving verbatim]

## How This Connects to My Work
[Which goals, decisions, or topics does this inform?]

## Source
[Full citation or URL]
```

---

#### `topics/` — Domain Knowledge (Extended for Personal Use)

The standard `topics/` folder exists in Axiom for evergreen cross-project knowledge. In personal mode, it holds your domain expertise and life context. Recommended subfolders:

```
.memory-bank/topics/
  _index.md
  _prompt.md
  health/          ← health goals, protocols, tracking, medical context
  finances/        ← financial goals, rules, accounts, tracking
  learning/        ← books, courses, skills you're building, learning log
  work/            ← career context, professional goals, work patterns
  relationships/   ← relationship patterns, social context (not individual contacts)
  [your domains]/  ← add whatever domains matter to you
```

Each subfolder needs its own `_index.md` and `_prompt.md`. Keep them lightweight — a few sentences of scope and a template.

---

### The Full Personal Memory Bank Map

```
.memory-bank/
  _index.md              ← ROOT MAP — always update this
  _prompt.md             ← global rules (highest authority)
  
  ## Root Context Files
  projectBrief.md        ← life/work context overview
  soul.md                ← personal doctrine and non-negotiables
  activeContext.md       ← current focus, open questions, active constraints
  decisionLog.md         ← append-only decisions and rationale
  techContext.md         ← tool stack, MCP availability, external services
  
  ## Goal Tracking
  work-items/            ← one folder per active goal or project
    _index.md
    _prompt.md
    <goal-id>/
      overview.md
      plan.md
      context.md
      runs/
  
  ## Capture and Processing
  captures/              ← landing zone for random things (Slack, email, notes)
    _index.md
    _prompt.md
  
  signals/               ← things you're watching or tracking
    _index.md
    _prompt.md
  
  ## People
  contacts/              ← context about people that matters across sessions
    _index.md
    _prompt.md
  
  ## Knowledge
  reference/             ← saved content, research, articles, book notes
    _index.md
    _prompt.md
  
  topics/                ← domain knowledge (health, finances, learning, work, etc.)
    _index.md
    _prompt.md
    health/
    finances/
    learning/
    work/
  
  ## Standard Axiom Folders
  inbox/                 ← agent-to-agent messages
  agents/                ← agent profiles and reflections
  findings/              ← lessons learned, anti-patterns
  prds/                  ← PRD files and merge notes (if using PRD workflow)
  known-gaps/            ← quality evaluations and improvement notes
```

---

## Folder Decision Tree — Where Does This Note Go?

> **The most common friction point**: a note could fit in 2+ folders. Use this tree.

```
Is this content temporary (needs processing, then goes somewhere else)?
  YES → captures/
  NO  → continue

Is this something I'm actively watching/monitoring (not a task, not knowledge)?
  YES → signals/
  NO  → continue

Is this about a specific person?
  YES → contacts/<firstname-lastname>.md
  NO  → continue

Is this a specific artifact I saved (article, book, doc, research)?
  YES → reference/<slug>.md
  NO  → continue

Is this synthesized knowledge about a domain I'm building expertise in?
  YES → topics/<domain>/
  NO  → continue

Is this a goal or project I'm actively working on?
  YES → work-items/<goal-id>/
  NO  → continue

Is this a decision I made?
  YES → decisionLog.md (append)
  NO  → continue

Is this a lesson learned or pattern I noticed?
  YES → findings/ (or agents/assist-axiom/observation-log.md)
  NO  → activeContext.md or discard
```

**Ambiguous cases:**
- "Article about health I want to remember" → `reference/` (it's an artifact) not `topics/health/`
- "My health goals and protocols" → `topics/health/` (it's synthesized knowledge)
- "Slack message from my manager about a project" → `captures/` first, then promote
- "Competitor I'm watching" → `signals/` (it's a watch item, not knowledge)
- "Notes from a meeting with a client" → `captures/` first; promote to `contacts/` or `reference/`

---

## Privacy and Security

> **This is the most important section for personal mode.** Read it before creating
> any personal content.

### What NOT to store in the repo

**Never store:**
- Passwords, API keys, tokens, or credentials (use a password manager; reference by name only)
- Other people's health, financial, or medical information
- Private communications without the other person's knowledge
- Sensitive financial account numbers or full financial records
- Anything you would not want your employer, family, or the public to see

**Why this matters:** A single accidental `git push` to a public repo exposes
everything in your commit history. Even if you delete the file later, the history
remains. Git is not a private diary.

### Recommended gitignore additions for personal mode

Add these to your `.gitignore` if you're using a shared or potentially-public repo:

```gitignore
# Personal mode — sensitive folders
.memory-bank/contacts/
.memory-bank/topics/health/
.memory-bank/topics/finances/
.memory-bank/topics/relationships/
.memory-bank/captures/
```

**Or**: keep the entire `.memory-bank/` out of git and use a separate private backup.

### Repo privacy recommendation

For personal mode, use a **private repo**. If you're using GitHub:
- Create a new private repo: `gh repo create my-personal-os --private`
- Or make an existing repo private in Settings → Danger Zone → Change visibility

### What IS safe to store

- Professional context about people (role, company, how you work together)
- Your own goals, decisions, and priorities
- Research findings and saved articles
- Signals and trends you're watching
- Work-related context and notes

---

## SOP Authoring Guide

### Hard SOPs (Specs)

Use `specs/` for things that **must** happen a certain way. The AI treats these as contracts.

**Good candidates for hard SOPs:**
- Recurring routines (morning routine, weekly review, monthly close)
- Decision frameworks (how you evaluate job offers, investments, major purchases)
- Communication standards (how you write emails, how you handle conflict)
- Health protocols (medication schedules, exercise minimums)
- Financial rules (spending limits, savings targets, investment criteria)
- Capture processing rules (how you process your inbox)

**SOP spec template:**

```markdown
# specs/NN-[SOP-Name].md

## Summary
One sentence: what this SOP governs.

## Trigger
When does this SOP activate? (time-based, event-based, request-based)

## Scope
What's in scope. What's explicitly not in scope.

## Steps
1. [Step 1 — specific and verifiable]
2. [Step 2]
3. [Step 3]

## Acceptance Criteria
- [ ] [Testable condition 1]
- [ ] [Testable condition 2]

## Exceptions
When is it acceptable to deviate? Who decides?

## Verification
How to check compliance after the fact.
```

### Soft SOPs (Skills)

Use `.opencode/skills/` for things that are **guidance** — best practices, preferred formats, checklists the AI should follow when relevant but can adapt.

**Good candidates for soft SOPs:**
- Research methodology ("how I like to research a topic")
- Writing style ("my preferred email tone and format")
- Meeting prep ("what I want to know before a 1:1")
- Travel planning ("my checklist for booking trips")
- Learning workflows ("how I process a new book or course")
- Capture processing ("how I triage my captures folder")

**Skill stub template:**

```markdown
---
name: my-[workflow-name]
description: >
  [One sentence: what this skill guides.]
version: "1.0"
---

# [Workflow Name]

## When to Load
[Describe the trigger: what situation or request should cause this skill to be loaded?]

## Preferred Approach
[Describe how you like this done. Be specific about format, order, tools, tone.]

## Checklist
- [ ] Step 1
- [ ] Step 2
- [ ] Step 3

## Output Format
[Describe what a good output looks like. Include an example if helpful.]

## Anti-Patterns
[What should the AI avoid doing in this workflow?]
```

---

## Work Items for Goals

### Goal Work Item Structure

```
.memory-bank/work-items/<goal-id>/
  overview.md          ← what the goal is, why it matters, definition of done
  plan.md              ← phases, tasks, steps with verification
  context.md           ← relevant background, constraints, decisions
  runs/                ← session logs and evidence
    <date>_<seq>/
      notes.md
      evidence.md
```

**Example goal IDs:** `q2-travel-planning`, `mortgage-refinance-research`, `reading-habit-build`, `career-transition-2026`, `home-renovation-q3`

### Goal Intake Template

```
Request: [What do you want to achieve?]
Acceptance Criteria:
  - [Specific, measurable outcome 1]
  - [Specific, measurable outcome 2]
Constraints:
  - Budget: [if applicable]
  - Timeline: [if applicable]
  - No-go zones: [things you won't do]
Context Refs:
  - Related Notion pages: [URLs]
  - Related captures: [.memory-bank/captures/<file>]
  - Related contacts: [.memory-bank/contacts/<person>]
```

### Verification for Goals

"Done" for a goal means evidence exists that the acceptance criteria were met:
- A Notion page updated with the outcome
- A Jira ticket closed with a comment
- A file created with research findings
- A screenshot of a booking confirmation
- A note in the memory bank with a decision recorded

---

## MCP-Heavy Workflows

Personal Axiom relies heavily on MCP tools to reach outside the repo.

### Notion as Your External Brain

When Notion MCP is available:
- Use Notion pages as the "source of truth" for long-form content (meeting notes, research, project docs)
- Use the memory bank as the "index and context layer" — pointers to Notion, not duplicates
- When you mention a Notion page, the AI should fetch it via MCP before responding
- Sync decisions: when you make a decision in a Notion page, capture it in `decisionLog.md`
- Use `notion_ref=` in trace markers to link work items to Notion pages

**Pattern: Notion doc arrives → Capture → Process → Promote**
1. Notion doc lands (meeting notes, research, etc.)
2. Create a capture in `.memory-bank/captures/` with the Notion URL and key points
3. During weekly review, promote key decisions to `decisionLog.md` and key context to the right topic folder
4. Link the Notion page in any related work item's `context.md`

### Slack and Email as Captures

When a Slack message or email contains something worth keeping:
1. Create a capture in `.memory-bank/captures/` with the raw content and source
2. Tag it with relevant topics and people
3. Process it during your next review session — promote, act, or discard

**The AI can help:** "I just got this Slack message from [person] about [topic]. Can you capture it and flag if it needs action?"

### Browser MCP for Research and Verification

When chrome-devtools MCP is available:
- Use it to research topics by navigating live web content
- Capture findings as reference notes in `.memory-bank/reference/`
- Use it to verify that external actions happened (booking confirmed, form submitted)
- Capture screenshots as evidence in work item `runs/` folders

### Jira as Your Task Surface

When Jira MCP is available:
- Personal goals can be Jira tickets (use a personal project key)
- Use Jira for anything with a deadline, assignee, or status workflow
- Sync: when a goal work item completes, close the Jira ticket with evidence

### When MCPs Are Unavailable (Degraded Mode)

The system works without MCPs. Here's what changes:

| MCP | When available | When unavailable |
|-----|---------------|-----------------|
| Notion | Fetch pages, update content | Create captures with Notion URLs; process manually |
| Jira | Read/write tickets | Track in work-items/ locally; sync when MCP returns |
| Browser | Research, verify, screenshot | Use web search; save findings to reference/ manually |
| GitHub | Read PRs/issues | Reference by URL in captures/ |

**MCP health check**: If something isn't working, ask: "Check my MCP configuration."
The AI will read `opencode.jsonc` and report what's configured vs. what's needed.

**MCP setup**: MCPs are configured in `opencode.jsonc` at the repo root. Each MCP
server requires installation and configuration. See the Axiom README or
`opencode.jsonc` for examples. You do not need MCPs to start — add them as you need them.

---

## Agent Specialization for Personal Domains

Create personal agent files in `.opencode/agents/` for specialized domains. Each agent should know where to look in the memory bank for its domain context.

**Example: `health-coach.md`**
```markdown
---
name: health-coach
description: Personal health and wellness advisor
---
You are my health coach. When I ask about health topics:
1. Check my health SOPs in specs/ (look for specs with "health" in the name)
2. Read my health context in .memory-bank/topics/health/
3. Check .memory-bank/activeContext.md for any current health constraints
4. Give practical, evidence-based guidance aligned to my SOPs
5. Capture any decisions in .memory-bank/decisionLog.md
6. If I mention a new health-related thing, offer to create a capture or update my health topic files
```

**Example: `financial-advisor.md`**
```markdown
---
name: financial-advisor
description: Personal finance advisor
---
You are my financial advisor. When I ask about financial topics:
1. Check my financial SOPs in specs/ (look for specs with "finance" or "money" in the name)
2. Read my financial context in .memory-bank/topics/finances/
3. Apply my decision framework before making recommendations
4. Never suggest anything that violates my hard financial rules (in specs/)
5. Capture any financial decisions in .memory-bank/decisionLog.md
6. If I mention a new financial signal (market change, rate change, etc.), offer to create a signal note
```

**Example: `research-assistant.md`**
```markdown
---
name: research-assistant
description: Research and knowledge management assistant
---
You are my research assistant. When I ask you to research something:
1. Check .memory-bank/reference/ for anything I've already saved on this topic
2. Check .memory-bank/captures/ for any unprocessed captures on this topic
3. Use browser MCP to research if needed
4. Save findings as a reference note in .memory-bank/reference/
5. Flag any signals worth tracking in .memory-bank/signals/
6. Link findings to any relevant work items
```

---

## Guardrails and Anti-Patterns

**Over-speccing:** Don't write a spec for every tiny preference. Specs are for things that must always happen a certain way. If you'd be fine deviating, use a skill instead.

**Memory bank bloat:** The memory bank is a second brain, not a diary. Captures are temporary — process them. Reference notes should be worth keeping for months. If a note won't matter in a month, discard it.

**Treating goals like code tickets:** Goals have fuzzier acceptance criteria than code. "I made a decision and I'm at peace with it" is valid evidence — just capture it.

**Ignoring the lifecycle:** The intake → plan → execute → verify → done lifecycle works for goals too. A 5-minute plan for a 2-hour task saves time.

**Secrets in the repo:** Never put passwords, API keys, or sensitive personal data in the memory bank or specs. Use a password manager and reference it by name only.

**Stale AGENTS.md:** The memory bank navigation table in AGENTS.md is only useful if it's accurate. Update it every time you add a new folder.

---

## Quick Start Checklist

### Phase 1 — Minimum Viable Setup (30 minutes)
- [ ] Verify prerequisites: git, editor, AI assistant with file-prompt support
- [ ] Create a **private** repo (or verify existing repo is private)
- [ ] **Set `default_agent: assist-axiom` in your project's `opencode.jsonc`** (Step 1 above)
- [ ] Fill in the **minimum viable AGENTS.md** (4 fields: identity, hard rules, tool stack, operating mode)
- [ ] Seed `activeContext.md` with current top 3 priorities
- [ ] Test: start a session — verify `assist-axiom` greets you with context, not "how can I help?"

### Phase 2 — Memory Bank Setup (1-2 hours)
- [ ] Seed `projectBrief.md`, `soul.md`, `techContext.md`, `decisionLog.md`
- [ ] Create `captures/` folder with `_index.md` and `_prompt.md`
- [ ] Add sensitive folders to `.gitignore` (contacts/, topics/health/, topics/finances/)
- [ ] Add the Memory Bank Navigation table to `AGENTS.md`
- [ ] Create a work item for your most important current goal

### Phase 3 — Personalization (ongoing)
- [ ] Create `contacts/`, `reference/`, `signals/` folders as you need them
- [ ] Create `topics/` subfolders for your active domains
- [ ] Write your first hard SOP as a spec (pick one recurring routine)
- [ ] Write your first soft SOP as a skill (pick one workflow you want to standardize)
- [ ] Test capture processing: drop a Slack message or email into captures and ask the AI to process it
- [ ] Configure MCP tools you have available (Notion, Jira, browser, etc.)

---

## Trace

```
axiom:trace work_item=personal-context-01 spec=.opencode/skills/personal-context-axiom/SKILL.md plan=personal-os/onboarding
```
