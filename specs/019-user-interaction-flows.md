# SPEC-019: User Interaction Flows

**Status:** Draft
**Depends On:** SPEC-015 (API), SPEC-016 (CLI), SPEC-017 (UI Adapter Layer), SPEC-014 (HITL)
**Created:** 2026-04-12

---

## 1. Overview

Specs 000-015 define how the machine thinks. This spec defines how the human interacts. Conscience has three distinct user types, each with different needs, workflows, and preferred tools. No user should ever need to "learn Conscience" — they should use the tool they already know, with Conscience running invisibly behind it.

---

## 2. User Personas

### 2.1 The Developer

**Who:** A software developer who wants an AI agent with persistent memory and reliable behavior. Currently uses opencode, aider, or Claude Code for coding tasks.

**Needs:**
- Use their existing AI tool without changing workflow
- Agent remembers context across sessions (not just conversation context window)
- Agent doesn't hallucinate or break things (database constraints catch errors)
- Can check on what the agent is doing without interrupting it
- Gets notified when the agent needs approval

**How they interact:** Via `opencode attach` (SPEC-017 opencode adapter) for full TUI experience, or via MCP tools from inside their existing AI tool.

### 2.2 The Operator

**Who:** A DevOps engineer, team lead, or technical manager responsible for running Conscience instances and overseeing agent activity.

**Needs:**
- Deploy and configure Conscience instances
- Monitor active sessions, costs, and errors
- Review and respond to HITL approval requests
- Kill stuck or runaway agents
- Audit what agents have done

**How they interact:** Via the CLI (SPEC-016) and the REST API (SPEC-015). May also use alerts (Slack, email) for approval notifications.

### 2.3 The Integrator

**Who:** A developer building a custom application on top of Conscience — a dashboard, a Slack bot, a CI/CD pipeline integration, or a custom workflow.

**Needs:**
- Machine-readable API spec (SPEC-018)
- MCP server for tool integration
- Webhooks for event-driven workflows
- SDK in their preferred language

**How they interact:** Via the REST API, MCP server, and webhooks. Reads the OpenAPI spec.

---

## 3. Primary Workflows

### 3.1 Developer: First Connection

The developer's first experience connecting their tool to Conscience.

**Prerequisites:** A Conscience server is running (deployed by an operator or running locally).

```
Step 1: Install Conscience (if running locally)
  $ brew install conscience
  $ conscience init --llm-key sk-...
  $ conscience serve --port 8090 --adapter opencode

Step 2: Connect with opencode
  $ opencode attach http://localhost:8090

Step 3: Use as normal
  > Analyze the auth module for security issues

  # Conscience agent processes with full cognitive architecture
  # User sees response in opencode's TUI as normal
```

**What the developer sees:** Nothing different. Their tool works the same way. Conscience is invisible.

**What's different under the hood:** The agent has persistent memory (survives session restarts), database-backed transaction safety, and constrained behavior. If the agent hits a destructive action, it pauses for approval.

### 3.2 Developer: Ongoing Work

The developer works across multiple sessions. Conscience remembers.

```
Day 1:
  > Look at the codebase and understand the auth module
  Agent: [reads context, explores code, stores findings in memory]
  Agent: "I've analyzed the auth module. Here's what I found..."

Day 2 (new session, same project):
  > Refactor the auth module based on your analysis yesterday
  Agent: [resumes session, reads memory from Day 1]
  Agent: "Based on my analysis from yesterday, I'll refactor the token validation..."

  ⚠️ Agent paused: "I need to modify the users table schema. Approve?"
  > (developer switches to terminal)
  $ conscience approve list
  $ conscience approve abc-123 --notes "OK but add a migration"
  Agent: [resumes with approved modification]
  Agent: "Done. I've added the migration and refactored the validation..."
```

**Key difference from a standard LLM:** The agent remembers Day 1's analysis. The developer doesn't need to re-explain the codebase.

### 3.3 Developer: Multi-Tool Workflow

A developer uses different tools for different tasks, all backed by the same Conscience agent.

```
Morning: opencode for coding
  $ opencode attach http://localhost:8090
  > Implement the payment processing module
  [Conscience agent works through opencode's TUI]

Afternoon: Claude Code for code review (via MCP tools)
  # Claude Code has Conscience as an MCP server
  > Use the conscience tool to review the payment module I built this morning
  [Claude Code calls Conscience MCP tools, agent uses memory from morning]

Evening: CLI for quick status check
  $ conscience session list
  $ conscience session cost abc-123
  $ conscience memory list abc-123 --limit 5
```

**Key difference:** The agent's memory persists regardless of which tool is used. Switching tools doesn't mean starting over.

### 3.4 Operator: Deployment & Monitoring

```
Day 0: Initial deployment
  $ conscience init --supabase --db-url postgresql://...
  Admin API key: cs_ak_a1b2c3d4...
  Schema version: 0.1.0
  Server URL: https://my-project.supabase.co/functions/v1

Day 1: Daily check
  $ conscience status
  Active sessions: 5
  Pending approvals: 1
  Total cost today: $4.20

  $ conscience approve list
  [HIGH] delete_old_logs (session: abc-123) — "Delete logs older than 90 days"
  $ conscience approve abc-123

Day 7: Incident
  # Alert arrives via Slack: "Agent session xyz-789 has 5 consecutive errors"
  $ conscience session logs xyz-789 --iterations 3
  # Error: agent keeps trying to access a dropped table
  $ conscience session cancel xyz-789
  $ conscience session create --agent-name analyst --goal "Re-analyze the dataset with the new schema"

Day 30: Cost review
  $ conscience session list --format json | jq '[.[] | .cost_cents] | add'
  42350  # $423.50 total spend
```

### 3.5 Operator: HITL Approval

The operator's most common interactive workflow.

```
Scenario: Agent requests destructive action

Notification arrives:
  Slack: "Conscience approval needed: HIGH — Delete 5000 rows from orders where status='cancelled'"

Operator reviews:
  $ conscience approve show abc-456
  Type: destructive_action
  Session: researcher-session-1
  Iteration: 23
  Risk: high
  Target SQL: DELETE FROM orders WHERE status = 'cancelled' AND created_at < '2026-01-01'
  Context:
    - Agent monologue: "User asked to clean up old cancelled orders. This affects 5000 rows."
    - Previous actions: [SELECT count(*) ...] → 5000 rows

Decision options:
  A) Approve as-is:
    $ conscience approve abc-456

  B) Reject:
    $ conscience reject abc-456 --reason "Don't delete orders, archive them instead"

  C) Modify and approve:
    $ conscience approve abc-456 --modified-sql "UPDATE orders SET status='archived' WHERE status='cancelled' AND created_at < '2026-01-01'"

  D) Cancel the whole session:
    $ conscience session cancel researcher-session-1
```

### 3.6 Integrator: Custom Dashboard

```
Scenario: Building a custom web dashboard for the team

Step 1: Read the spec
  # Get OpenAPI spec
  GET http://conscience:8090/openapi.json

Step 2: Generate SDK
  npx openapi-typescript http://conscience:8090/openapi.json -o src/types.ts

Step 3: Build dashboard
  const sessions = await client.GET('/api/v1/sessions');
  const approvals = await client.GET('/api/v1/approvals', {
      params: { query: { status: 'pending' } }
  });

  // Subscribe to real-time events
  const events = new EventSource('http://conscience:8090/api/v1/events');
  events.onmessage = (e) => updateDashboard(JSON.parse(e.data));

  // Handle approval from dashboard
  await client.POST('/api/v1/approvals/{id}/review', {
      body: { decision: 'approved', notes: 'LGTM' }
  });
```

---

## 4. Onboarding Flows

### 4.1 Local Development (PocketBase Path)

Target: Individual developer wanting to try Conscience locally.

```
1. Install
   $ brew install conscience
   # or: curl -fsSL https://conscience.dev/install | bash

2. Initialize
   $ conscience init
   ? LLM provider: OpenAI / Anthropic
   ? API key: sk-...
   ? Default model: gpt-4o (recommended)
   ✓ Database initialized (SQLite)
   ✓ Admin key: cs_ak_a1b2c3d4...
   ✓ Config saved: ./conscience.yaml

3. Start
   $ conscience serve
   ✓ Server running at http://localhost:8090
   ✓ opencode adapter: http://localhost:8090 (run `opencode attach http://localhost:8090`)
   ✓ MCP server: http://localhost:8090/mcp/sse
   ✓ API docs: http://localhost:8090/doc

4. Connect with opencode
   $ opencode attach http://localhost:8090
   → opencode TUI starts, connected to Conscience backend

5. First interaction
   > Hello, what can you help me with?
   Agent: "I'm a Conscience agent with persistent memory. I can help with..."
```

**Time to first interaction:** Under 5 minutes from install.

### 4.2 Team Deployment (Supabase Path)

Target: Team lead setting up Conscience for multiple developers.

```
1. Create Supabase project
   → Sign up at supabase.com
   → Create new project

2. Install Conscience schema
   → Open SQL Editor
   → Paste install_conscience.sql
   → Run

3. Configure LLM keys
   $ conscience config set llm.api_key sk-...
   → Stored in config file (or Supabase Vault if available)

 4. Start the server
    $ conscience serve --db postgres://your-project.supabase.co:5432/postgres

 5. Share connection info with team
   Server: https://your-project.supabase.co
   Each developer gets their own API key:
   $ conscience session create --api-only
   → Returns: cs_sk_... (share this with the developer)

6. Each developer connects
   → opencode: `opencode attach https://your-project.supabase.co`
   → Claude Code: add Conscience as MCP server
   → Or use CLI directly
```

### 4.3 MCP-Only Integration

Target: Developer who just wants Conscience tools inside Claude Code or opencode, not full agent replacement.

```
1. Start Conscience
   $ conscience serve

2. Add as MCP server in Claude Code
   $ claude mcp add conscience --transport http http://localhost:8090/mcp/sse

3. Use Conscience tools in Claude Code
   > Create a Conscience agent session to analyze my database
   Claude: [calls MCP tool: create_session]
   Claude: [calls MCP tool: send_message]

4. Check status via MCP
   > What's my agent doing?
   Claude: [calls MCP tool: get_session_status]
   Claude: "Your agent is on iteration 5, analyzing the users table..."
```

---

## 5. Error Recovery UX

### 5.1 Agent Stuck (Consecutive Errors)

**What the user sees (in their AI tool):**
```
⚠️ Agent encountered repeated errors and has paused.
Error: column "user_email" does not exist
Iterations failed: 3/3

The agent is waiting for human review.
Use: conscience approve list
```

**What the operator does:**
```
$ conscience approve show <id>
# Sees the error context, agent's monologue, what it was trying to do

# Options:
$ conscience approve <id>              # Let it try again
$ conscience reject <id> --reason "Use email column instead"
$ conscience session cancel <id>       # Give up
```

### 5.2 Agent Needs Approval (HITL)

**What the user sees (in their AI tool):**
```
I need to execute a potentially destructive operation:
DROP TABLE temp_cache

This requires human approval. I've submitted a request and paused.

You'll be notified when this is reviewed.
```

**What happens:** The agent pauses. Notification goes to the operator (Slack, email, dashboard). Operator reviews and approves/rejects. Agent resumes.

### 5.3 Budget Exceeded

**What the user sees:**
```
⚠️ Budget limit reached ($5.00 / $5.00)
Session has been paused.

To continue: conscience session resume <id>
To increase budget: conscience config set harness.budget_limit_cents 1000
```

### 5.4 Server Unreachable

**What the user sees (in their AI tool):**
```
Error: Connection refused at http://localhost:8090

Is the Conscience server running?
  $ conscience serve
```

### 5.5 Schema Migration Needed

**What the operator sees:**
```
$ conscience status
Server:    running
Schema:    OUTDATED (current: 0.2.0, required: 0.3.0)
Sessions:  PAUSED (5 sessions waiting for migration)

$ conscience migrate
Running migration 003_add_memory_pages.sql...
Running migration 004_update_tools_registry.sql...
✓ Schema updated to 0.3.0
✓ 5 sessions resumed
```

---

## 6. Feedback Mechanisms

### 6.1 Agent → User

| Channel | When | How |
|---|---|---|
| Chat response | Every interaction | Through the adapter (opencode TUI) or MCP tool |
| HITL pause | Destructive action, error threshold | Via TUI permission prompt + notification |
| Session complete | Agent finishes goal | Via TUI chat |
| Cost warning | Budget at 80% | Via notification channel |

### 6.2 System → Operator

| Channel | When | How |
|---|---|---|
| Approval needed | Agent requests HITL | Slack/email/webhook (SPEC-014 §6) |
| Session failed | Circuit breaker trips | Slack/email/webhook |
| Budget alert | Approaching limits | Slack/email/webhook |
| Daily digest | End of day (configurable) | Email |

### 6.3 User → Agent

| Channel | When | How |
|---|---|---|
| Chat message | Any time | Through the adapter (opencode TUI) or MCP tool |
| Approval decision | HITL pause | TUI permission prompt, CLI, or REST API |
| Session control | Any time | CLI `conscience session pause/cancel/resume` |
| Configuration change | As needed | CLI `conscience config set` |

---

## 7. Key UX Principles

### 7.1 Zero Learning Curve

The developer should never need to learn "how to use Conscience." They run `opencode attach http://localhost:8090` and get the same opencode experience with Conscience's brain underneath. Zero config friction.

### 7.2 Progressive Disclosure

- **Level 0:** Just chat. Agent works. (Developer using opencode/aider)
- **Level 1:** Approvals. Agent pauses, operator reviews. (Operator using CLI)
- **Level 2:** Inspection. Check memory, costs, iteration history. (Operator using CLI/API)
- **Level 3:** Configuration. HITL settings, model routing, budget limits. (Operator using CLI/config)
- **Level 4:** Custom integration. Build on the API, extend with MCP. (Integrator using OpenAPI spec)

### 7.3 Non-Blocking by Default

Most interactions are non-blocking:
- Agent runs autonomously, user gets responses when done
- Approvals are the only blocking interaction
- Cost monitoring and error recovery happen automatically
- User only needs to intervene for high-stakes decisions

### 7.4 State Visibility

At any point, the operator can answer:
- What is the agent doing right now? → `conscience session show <id>`
- What has it done? → `conscience memory list <id>`
- How much has it cost? → `conscience session cost <id>`
- Is it stuck? → `conscience status`
- What needs my attention? → `conscience approve list`

---

## 8. Anti-Patterns

| Don't | Why |
|---|---|
| Build a custom TUI | Existing tools are better. Invest in the runtime, not the UI. |
| Require users to learn SQL | The agent writes SQL, not the user. Users interact in natural language. |
| Make HITL required for every action | Default to autonomous. Only interrupt for genuinely risky operations. |
| Show internal state to developers | Memory events, iteration commits, SQL statements are operator concerns. Developers just chat. |
| Couple to a single tool | The shim layer exists specifically to avoid vendor lock-in. opencode today, pi-agent tomorrow. |
| Make onboarding multi-step | 5 minutes from install to first interaction, or the design is wrong. |
| Build a web admin UI before the runtime works | CLI-first. The native API already exposes everything a future web UI would need. Prove the surface is complete with the CLI first. |

---

## 9. Interface Layer Summary

| Interface | Who Uses It | Status | Spec |
|---|---|---|---|
| `conscience` CLI | Operator | Primary management interface | SPEC-016 |
| `opencode attach` | Developer | Primary chat/interaction interface (via shim) | SPEC-017 |
| MCP tools | Developer (from any AI tool) | Tool-level access to Conscience | SPEC-015 §5 |
| Native REST API | Integrator, future web UI | Programmatic access | SPEC-015 |
| Web admin UI | Operator | **Future phase, not specced** | — |
| TUI dashboard | Operator | **Future phase, not specced** | — |

**The CLI + opencode shim + MCP cover 100% of use cases for the initial release.** Web UI is a polish step for later — the API surface is already designed to support it.

---

## 9. Success Metrics

| Metric | Target | How to Measure |
|---|---|---|
| Time to first interaction | < 5 minutes (local) | Fresh install → first agent response |
| Configuration friction | 1 command: `opencode attach` | Number of manual steps in onboarding |
| Tool compatibility | opencode TUI + MCP clients | opencode attach, Claude Code MCP, etc. |
| HITL interruption rate | < 10% of sessions | Sessions that pause vs. complete autonomously |
| Developer awareness of Conscience | Minimal | Developer should rarely think about Conscience directly |
| Operator response time to approvals | < 15 minutes | Time from approval request to resolution |

---

## 10. Open Questions

1. **Session reset UX**: How does a developer "start fresh" in `opencode attach`? New session via `/session`? CLI command? Time-based expiry?
2. **Multi-session awareness**: The opencode TUI has a session picker. Should Conscience expose sessions through the adapter so the picker works?
3. **Team approval routing**: In a team setting, how are approval requests routed to the right person? By risk level? By project?
4. **File system mismatch**: If Conscience runs on a server but the codebase is local, how do file operations (read/write/edit) work through the adapter?
5. **pi-agent protocol**: Does pi-agent have a similar `serve` + `attach` model? If so, same adapter pattern applies.
