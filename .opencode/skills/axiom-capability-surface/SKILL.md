---
name: axiom-capability-surface
description: Full catalog of Axiom capabilities (commands, agents, skills, spec categories) for intent resolution and platform awareness.
version: "1.0"
tags:
  vertical: [onboarding, planning]
  category: onboarding
  core: false
---

# Axiom Capability Surface

This skill catalogs the full capability surface of the Axiom platform. Load it when you need to understand what Axiom can do, route complex intents, or discover the right tool/agent/skill for a task.

axiom:trace work_item=collaborative-intent-resolution-01 spec=specs/59-Collaborative-Intent-Resolution.md#REQ-CIR-009 plan=phase-79-1b/task-79-1b-1/step-79-1b-1-1

---

## Commands (33)

All commands are invoked as `/command-name` in OpenCode. Grouped by workflow stage.

### Intake and Kickoff
| Command | Description |
|---------|-------------|
| `/axiom-intake` | Resolve raw user intent into a plan, questions, or blocked status |
| `/axiom-kickoff` | One-command kickoff for a new request (specs + work item + roadmap + loop) |
| `/axiom-spec-request` | Turn a feature request into spec updates, then run spec verification |
| `/axiom-spec-extract` | Reverse-engineer specs from an existing codebase (code-first to spec-first) |

### Setup and Onboarding
| Command | Description |
|---------|-------------|
| `/axiom-init` | Initialize Axiom in a blank repo or multi-repo workspace (create baseline structure) |
| `/axiom-bootstrap` | One-command bootstrap: scaffold checks, TODO/plans, and Ralph loop. Workspace-aware |
| `/axiom-onboarding` | Run onboarding flow after Axiom install (TODO, plans, work-item, loop prompts) |

### Planning
| Command | Description |
|---------|-------------|
| `/axiom-meta-plan` | Produce/update meta-planning and an implementation plan for a work item |
| `/axiom-plan` | Produce meta-planning + implementation plan for a work item |
| `/axiom-implementation-plans` | Create/update `.memory-bank/implementation-plans/` aligned to `.memory-bank/TODO.md` |
| `/axiom-todo` | Create/update `.memory-bank/TODO.md` (project roadmap) aligned to specs |
| `/axiom-work-item` | Create or refresh a work item (meta-plan + plan artifacts) |
| `/axiom-roadmap-refresh` | Refresh TODO and implementation plans from current specs/memory |

### Execution
| Command | Description |
|---------|-------------|
| `/axiom-step` | Execute one plan step and report evidence |
| `/axiom-loop` | Build/refresh a Ralph loop scaffold for this repo |
| `/axiom-prompt-update` | Refresh PROMPT.md and PROMPT-VERIFY.md (Ralph loop prompt bundle) |

### Verification and Review
| Command | Description |
|---------|-------------|
| `/axiom-verify` | Verify recent changes against specs and evidence bundle |
| `/axiom-benchmark-sink` | Bootstrap, score, check status, or compare Benchmark Sink subscriptions |
| `/axiom-best-practices` | Get best-practices guidance for this repo and store it in Memory Bank |
| `/axiom-sitrep` | Generate a deterministic sitrep report |
| `/axiom-skill-map` | Navigate the skill map decision tree (display, query, update, validate) |

### Sync and Maintenance
| Command | Description |
|---------|-------------|
| `/axiom-sync-all` | Run all sync commands (indexes, traceability, distribution) |
| `/axiom-sync-indexes` | Sync and repair `_index.md` inventories (Memory Bank + specs) |
| `/axiom-sync-trace` | Audit and repair traceability markers and links |
| `/axiom-sync-specs-inventory` | Sync specs inventory files (specs/README.md + specs/_index.md) |
| `/axiom-sync-work-items` | Audit and sync work item folder hygiene (plans, evidence, runs) |
| `/axiom-sync-memory-bank-core` | Ensure Memory Bank core files + folder prompts/indexes exist and are linked |
| `/axiom-sync-version-manifest` | Validate and regenerate .axiom/.version.md if stale |
| `/axiom-sync-command-registry` | Sync .axiom/command-registry.yaml with installed /commands |
| `/axiom-sync-jira` | Sync Jira ticket state with repo work items (status, due dates, evidence, health) |
| `/axiom-sync-distribution` | Sync distribution artifacts (installer, manifests, version file, template sync) |
| `/axiom-sync-template` | Sync axiom-template/ from this repo |

### Git and Memory
| Command | Description |
|---------|-------------|
| `/axiom-batch-commit` | Batch commit dirty-tree changes into logical, well-messaged commits with optional push |
| `/memory-bank-update` | Update or audit the Memory Bank using the memory-bank subagent |

### Forensics & Investigation
| Command | Description |
|---------|-------------|
| `/axiom-forensics` | Master investigation command (search/trace/cost/replay/summary) |
| `/axiom-forensics-search` | Search sessions by keyword, agent, model, time range |
| `/axiom-forensics-trace` | Trace subagent hierarchy tree |
| `/axiom-forensics-cost` | Aggregate costs by agent/model/day/session |
| `/axiom-forensics-replay` | Reconstruct conversation transcript |

---

## Agents (38)

All agents are invoked as `@agent-name` subagents. Grouped by domain.

### Core Build Loop
| Agent | Role | Invoke when... |
|-------|------|----------------|
| `@tower-axiom` | Primary orchestrator (delegates to all others) | Coordinating multi-agent work, complex tasks |
| `@team-axiom` | Human-facing primary agent, coordinates full roster | User-facing entry point |
| `@specwriter-axiom` | Spec writer/librarian (contracts, traceable specs) | Writing or updating specs |
| `@spec-verifier-axiom` | Spec/contract verifier (alignment + trace integrity) | Verifying spec compliance |
| `@pm-axiom` | Planning PM (TODO, implementation plans, work breakdown) | Creating plans, breaking down work |
| `@dev-axiom` | Implementation (code changes + tests + trace markers) | Writing code, implementing features |
| `@qa-axiom` | QA verifier (tests, regressions, evidence integrity) | Testing, regression checks |
| `@trace-auditor-axiom` | Trace auditor (completeness + plan-to-repo gap analysis) | Auditing traceability |
| `@ralph-wiggum-verify` | Meta-loop verifier captain (audits builder output, steers next step) | Ralph loop verification |

### Knowledge and Durability
| Agent | Role | Invoke when... |
|-------|------|----------------|
| `@memory-bank-axiom` | Memory Bank steward (bootstrap/maintain .memory-bank/) | Memory bank operations |
| `@prompt-mirror-axiom` | Prompt mirror (promptable repo mirrors, drift detection) | Checking prompt drift |
| `@sitrep-axiom` | SitRep officer (situation reports + debriefs) | Status reports, debriefs |
| `@rfc-writer-axiom` | RFC writer (reads, writes, updates RFCs in Notion via Notion MCP) | Writing or updating RFCs in Notion |
| `@best-practices-axiom` | Best practices (portable engineering playbooks) | Engineering guidance |
| `@devguide-axiom` | Dev guide (reusable engineering playbooks; MUST/SHOULD/MAY) | Developer documentation |

### Engineering Specialists
| Agent | Role | Invoke when... |
|-------|------|----------------|
| `@db-architect-axiom` | DB architect (data modeling, migrations, indexing) | Database work |
| `@performance-axiom` | Performance engineer (budgets, profiling, benchmarks) | Performance optimization |
| `@cloud-engineer-axiom` | Cloud engineer (IaC, IAM, networking, environments) | Infrastructure work |
| `@ci-cd-axiom` | CI/CD (pipelines, build/test automation, provenance) | Pipeline setup/maintenance |
| `@sre-ops-axiom` | SRE/Ops (deploy safety, observability, runbook linkage) | Production operations |
| `@release-manager-axiom` | Release manager (versioning, changelog, release notes) | Preparing releases |
| `@dependency-bot-axiom` | Dependency bot (upgrades/CVEs with rollback) | Dependency updates, CVE fixes |
| `@repo-researcher-axiom` | Repo researcher (learn/fork/track upstream) | Upstream tracking, research |
| `@docs-runbooks-axiom` | Docs/runbooks (user docs, operator docs, runbooks) | Writing documentation |
| `@ux-writer-axiom` | UX writer (user-facing copy, error messages) | User-facing text |
| `@incident-commander-axiom` | Incident commander (coordination + timeline + comms) | Incident response |
| `@frontend-dev` | Frontend dev (UI builder + test author + browser verifier) | Frontend/UI work |
| `@expert-axiom` | Expert reader (answers external queries routed via Expert Platform Gateway; activated in expert mode repos) | Querying a configured expert |
| `@expert-writer-axiom` | Expert writer (manages knowledge ingestion: git auto-pull, Pandora Box, direct endpoint; hidden by default, expert mode only) | Curating expert knowledge |

### Security, Risk, and Adversarial
| Agent | Role | Invoke when... |
|-------|------|----------------|
| `@security-review-axiom` | Security review (threat model, secrets hygiene, risk gates) | Security assessments |
| `@security-engineer-axiom` | Security engineer (implements mitigations, secure defaults) | Building security controls |
| `@whitehat-axiom` | White hat (defensive penetration validation + retest) | Penetration testing |
| `@privacy-compliance-axiom` | Privacy & compliance (data protection controls) | Privacy/data protection |
| `@accessibility-review-axiom` | Accessibility reviewer (WCAG audit + fix guidance) | Accessibility audits |
| `@finops-cost-axiom` | FinOps/cost engineer (cost visibility + guardrails) | Cost analysis |
| `@chaos-engineer-axiom` | Chaos engineer (resilience testing, fault injection) | Resilience testing |
| `@redteam-axiom` | Red team (adversarial falsification of DoD claims) | Challenging claims |
| `@devils-advocate-axiom` | Devil's advocate (challenge specs/plans/designs) | Design review |
| `@assumption-buster-axiom` | Assumption buster (surface undocumented prerequisites) | Validating assumptions |
| `@strategy-falsifier-axiom` | Strategy falsifier (pre-implementation hypothesis challenge — Gate 3 in bug-fix gate order) | Before writing code for any non-mechanical bug fix or new feature |

### Forensics & Investigation
| Agent | Role | Invoke when... |
|-------|------|----------------|
| `@forensics-axiom` | Forensic investigator (search/trace/cost/replay sessions) | Investigating OpenCode sessions, cost analysis, conversation replay |

---

## Skills (143 — see .opencode/skills/ for the authoritative full list)

All skills are loaded via the `skill` tool by name. Grouped by category.

### Core Methodology
| Skill | Description |
|-------|-------------|
| `baby-steps-methodology` | Portable Baby Steps rules (smallest meaningful change, validate each step, document with evidence) |
| `middle-out-planning-axiom` | Middle-Out Implementation Planning — start from the critical integration boundary, prove it works first, expand outward. Prevents top-down isolation and bottom-up avoidance. Load when planning boundary-crossing features. |
| `axiom-mission-north-star` | Portable Mission & North Star doctrine (specs, verification, auditability, humans as traffic control) |
| `traceability-doctrine` | Portable traceability rules (trace markers, required links, validation checks, commit/PR templates) |
| `evidence-bundle-schema` | Portable evidence bundle schema (verification.md + outputs.md) and validation rules |
| `axiom-confidence-scoring` | Portable confidence scoring model (signals, weights, formula, thresholds) |
| `axiom-retry-escalation` | Portable retry and escalation rules when structured outputs are missing or verification fails |

### Planning and Specs
| Skill | Description |
|-------|-------------|
| `spec-kickoff-axiom` | Kick off and iteratively refine high-quality project specs from minimal human input |
| `meta-plan-axiom` | Produce a Axiom meta-plan and implementation plan aligned to specs |
| `axiom-meta-planning-contract` | Portable meta-planning contract (required sections, light vs standard) |
| `axiom-plan-schema` | Portable plan.yaml schema summary for Axiom planning/execution |
| `axiom-implementation-plans` | Maintain `.memory-bank/implementation-plans/` as phase-level plans aligned to TODO |
| `axiom-todo` | Maintain `.memory-bank/TODO.md` as the specs-aligned roadmap |
| `axiom-gap-analysis` | Orchestrate multi-agent gap analysis and progress reporting |
| `implementation-plan-history` | Portable rules for current vs historical plans, run snapshots, and comment queue handling |
| `prd-spec-merge-axiom` | Merge a finalized PRD into the repo's specs/ directory with traceable requirements |
| `prd-generator-axiom` | Agentic-optimized PRD compiler (PM to ENG LEAD to spec artifacts) |

### Testing and Quality
| Skill | Description |
|-------|-------------|
| `enterprise-testing-standard` | Portable enterprise-grade tiered testing standard (6-tier verification hierarchy) |
| `test-quality-gates-axiom` | Portable test quality gate workflow (no assertionless tests, Tier-3+ required) |
| `code-analysis-axiom` | Static code analysis via `axiom analyze`: health score, dead code (vulture/deadcode), complexity (radon/gocyclo), lint (ruff/biome). Use as Tier-3+ evidence supplement alongside test execution |
| `code-graph-intelligence-axiom` | Structural call graph engine via `axiom-code-intel`: callers, callees, blast-radius, call-chain, cross-language edges, change impact, 14-language adapter table, schema v3 import namespaces |
| `conformance-testing-loop` | Behavior-first conformance loop for any testing surface (CLI, API, SDK, UI) |
| `runtime-spec-conformance-loop` | Real runtime conformance loop for any surface (CLI, HTTP API, SSE, workers) |
| `concurrent-client-server-testing` | Multi-agent concurrent client/server API testing with fixed runtime window |
| `protocol-testing` | Real, tool-driven API and protocol testing (HTTP, gRPC, GraphQL, WebSocket, SSE) |
| `regression-testing-bug-fixes` | Portable regression testing rules for bug fixes |
| `idle-spec-conformance-sweep` | Idle-time spec conformance sweep policy (audit random specs when no unblocked work remains) |
| `strategy-falsification-axiom` | Portable inline skill for Gate 3 (Strategy Falsification) — produces 5-element pack (hypothesis, alternatives, falsification criteria, blast radius, existing-fix check) without subagent dispatch. Load when pm-axiom or dev-axiom needs Gate 3 inline. |

### Operations and Infrastructure
| Skill | Description |
|-------|-------------|
| `sre-ops-axiom` | SLO/SLI definition, error budget calculation, runbook creation, deploy safety, observability |
| `opencode-programming-axiom` | Reusable OpenCode HTTP integration playbook: `/doc` contract checks, long-running `/message`, `/event` monitoring, liveness completion, timeout layering, SDK/generated-client caution, fallback chains |
| `chaos-engineer-axiom` | Fault injection patterns, resilience testing, runbook validation under failure |
| `axiom-operating-modes` | Portable guide to Axiom operating modes (Local CLI, Local Automated, Full Automated) |
| `axiom-runtime-logging-controls` | Portable contract for runtime logging controls (profile/depth/categories/format/sink) |
| `axiom-structured-logging-events` | Portable rules for emitting Axiom structured log events (schema, correlation, redaction) |
| `performance-benchmark-axiom` | Load/stress/soak test generation, baseline establishment, regression detection, performance budgeting |
| `runtime-profiler-axiom` | Universal attach-anywhere runtime profiler (Rust). Covers attach/wrap, platform caps, symbol resolution, output formats (JSON/speedscope/SARIF), HTTP API, CI integration. Load for runtime perf verification |
| `dashboard-design-axiom` | Dashboard design principles, panel types, layout patterns, tool-specific guidance, and anti-patterns for observability dashboards |
| `alert-engineering-axiom` | Alert design philosophy, threshold types, severity/routing/escalation patterns, noise-reduction, and alert-to-runbook linkage |
| `distributed-tracing-axiom` | Distributed tracing fundamentals, OpenTelemetry SDK setup, span design, context propagation, sampling strategies |
| `metrics-instrumentation-axiom` | Metric types, `codeops_` naming rules, bounded-label/cardinality guidance, instrumentation patterns, exposition formats |
| `observability-diagnosis-axiom` | Systematic diagnostic workflow for consuming and correlating observability signals (logs, metrics, traces, dashboards) |
| `predictive-observability-axiom` | Predictive observability for trend analysis, anomaly detection, error budget burn-rate forecasting, capacity planning |
| `visual-observability-evidence-axiom` | Visual observability evidence guidance: screenshot-as-evidence, flame graphs, trace waterfalls, heatmaps, log-panel reading |
| `rick-and-morty-axiom` | Morty orchestration: config writing, hot-reload (§18), stage group retry (§19), API control plane (§20), model fallback, session limits, fleet management, rick.sh supervision |

### Security and Compliance
| Skill | Description |
|-------|-------------|
| `security-review-axiom` | Threat model generation, secrets hygiene, vulnerability detection, security gate checklist |
| `privacy-compliance-axiom` | PII detection, data retention verification, consent flow validation, GDPR/CCPA/HIPAA controls |

### Hardening (Codebase Audit Battery)

Load these skills to audit and harden any codebase. Start with `hardening-anti-patterns-axiom` (master catalog), then load category skills as needed. End with `hardening-intake-axiom` to wire findings into the Axiom lifecycle.

| Skill | Description |
|-------|-------------|
| `hardening-anti-patterns-axiom` | **Start here.** Master catalog of anti-patterns across all 6 categories. Shared audit header, finding format (HARDEN-\<category\>-\<slug\>), severity rubric, and cross-category quick-scan grep commands. |
| `hardening-spof-axiom` | Single points of failure — detection, blast radius calculation, circuit breaker/fallback/timeout remediation patterns. |
| `hardening-security-axiom` | Security audit — injection (SQL, command, template), auth/authz gaps, hardcoded secrets, input validation, CVE scanning, PII in logs. All findings `requires_human_review: true`. |
| `hardening-database-axiom` | DB and data layer — N+1 queries, connection pool exhaustion, transactions spanning HTTP calls, table-locking migrations, dual-write consistency. Migration findings `requires_human_review: true`. |
| `hardening-sre-axiom` | Reliability/SRE — missing timeouts, retry without backoff, no circuit breakers, rate limiting gaps, goroutine/thread leaks, missing graceful shutdown, swallowed errors. |
| `hardening-quality-axiom` | Test coverage gaps — critical paths with no tests, error paths with no tests, assertionless tests, tautology tests, over-mocking, flaky tests, integration boundary gaps. |
| `hardening-observability-axiom` | Observability — unstructured logging, missing correlation IDs, high-cardinality metric labels, PII in logs, alerts without runbooks, missing SLIs/SLOs, "3am debugging" checklist. |
| `hardening-intake-axiom` | **Load last.** How to run the battery (Path A: direct Claude, Path B: Axiom work items), Jira hierarchy, confidence bands, deduplication rules, and quarterly cadence. |

### Repository and Onboarding
| Skill | Description |
|-------|-------------|
| `axiom-install` | Guide users through installing or upgrading Axiom in their project repository |
| `axiom-onboarding` | Onboard a newly installed Axiom repo into runnable state (TODO, plans, loops) |
| `axiom-glossary` | Generate, maintain, and ship a Axiom glossary of terms and conventions for every workspace |
| `repo-filesystem-layout` | Audit and regenerate a repository filesystem layout spec (what belongs where) |
| `multi-repo-coordinator-axiom` | Cross-repo dependency management, workspace-level planning, unified CI/CD coordination |
| `notion-mcp-axiom` | Read, write, and update content in Notion workspaces via Notion MCP (pages, databases, comments, meeting notes) |
| `personal-context-axiom` | Transform Axiom into a personal OS — specs as hard SOPs, skills as soft SOPs, memory bank for life/work context, work items for goals. Covers AGENTS.md persona wiring, captures/signals/contacts/reference folders, and MCP-heavy personal workflows |
| `expert-mode-axiom` | Set up, activate, and manage a Axiom repo as an Expert Platform expert. Installs reader + writer agents, initializes knowledge bank layout, wires data ingestion paths (git PR, Pandora Box, direct endpoint), and configures auto-pull/event listeners |

### Release and Maintenance
| Skill | Description |
|-------|-------------|
| `enterprise-release-quality` | Portable enterprise release quality gates, checklists, CI/CD patterns, rollback procedures |
| `migration-guide-generator-axiom` | Version-to-version upgrade guides, breaking change detection, deprecation path planning |
| `api-contract-validator-axiom` | OpenAPI/AsyncAPI spec generation, contract drift detection, API versioning discipline |
| `batch-commits-axiom` | Batch commit workflow (group dirty-tree changes into logical, well-messaged commits) |
| `todo-archive-scripts` | Archive and query TODO blocks using JSONL format |

### Documentation
| Skill | Description |
|-------|-------------|
| `docs-runbooks-axiom` | Operational procedure documentation, troubleshooting flowcharts, recovery runbooks |
| `adr-manager-axiom` | Architecture Decision Record creation, lifecycle management, cross-reference with specs |

### Writing and Communication
| Skill | Description |
|-------|-------------|
| `jira-workflow-axiom` | Parent Jira operations skill for issue taxonomy, comments, transitions, evidence mirroring, and pm-axiom ownership |
| `jira-field-standard-axiom` | Standard Jira custom-field contract for Axiom workspaces, including required fields, naming, and admin rollout guidance |
| `writing-style-system-axiom` | Parent routing skill for writing surfaces; selects child artifact-writing skills and applies format-balancing rules |
| `too-much-of-a-good-thing-axiom` | Balance-restoring skill for outputs that became over-optimized, over-structured, or too strict to use comfortably |
| `user-response-writing-axiom` | Cross-cutting overlay for assistant responses to humans; balances prose, bullets, and tables |
| `pull-request-writing-axiom` | Style guide for PR titles and bodies with reviewer context, verification, and trace sections |
| `git-commit-writing-axiom` | Conventional-commit style guide for commit subjects, bodies, and trailers |
| `jira-ticket-writing-axiom` | Style guide for Jira ticket summaries and descriptions with scope and AC clarity |
| `jira-comment-writing-axiom` | Style guide for Jira progress comments, blocker notes, and evidence updates |
| `documentation-writing-axiom` | Style guide for technical documentation, guides, and reference pages |
| `spec-writing-axiom` | Style guide for technical specs and behavior contracts |
| `adr-writing-axiom` | Style guide for architecture decision records |
| `runbook-writing-axiom` | Style guide for incident and operational runbooks |
| `changelog-release-notes-writing-axiom` | Style guide that separates developer changelogs from stakeholder release notes |
| `rfc-writing-axiom` | Style guide for RFCs and major design proposals |

### Frontend and UX
| Skill | Description |
|-------|-------------|
| `frontend-design` | Create distinctive, production-grade frontend interfaces with high design quality |
| `chrome-devtools-mcp` | Portable guide for browser automation via Chrome DevTools MCP (UI verification, screenshots, performance, accessibility, SSE, memory leaks) |

### Git and Developer Tools
| Skill | Description |
|-------|-------------|
| `git-hooks-builder-axiom` | Scan a Git repo to recommend and implement useful Git hooks |
| `gitignore-axiom` | Portable rules for managing .gitignore and the OpenCode .ignore override file |

### Agent Coordination
| Skill | Description |
|-------|-------------|
| `swarm-queen-axiom` | Inbox-driven dispatcher that coordinates subagents using `.memory-bank/inbox/` messages |
| `swarm-worker-axiom` | Execute exactly one inbox message request, then reply via inbox |
| `ralph-wiggum-loop` | Generate repo-aware Ralph loop prompts (PROMPT.md + runner script) |
| `memory-bank-steward` | Bootstrap, maintain, and improve the long-lived flat-file memory system |
| `self-improvement-findings-axiom` | Portable rules for the findings/ self-improvement directory in `.memory-bank/` |
| `sitrep-ascii-graphs` | Portable ASCII progress graph renderer for SitRep reports (progress bars, phase breakdowns, spec coverage heatmaps, velocity sparklines) |

### Protocol and XML
| Skill | Description |
|-------|-------------|
| `axiom-xml-protocol` | Portable XML envelope rules for Axiom /commands |

### Interactive Guidance
| Skill | Description |
|-------|-------------|
| `axiom-copilot` | Always-available interactive copilot. Assumes you know nothing. Checks where you are, tells you what to do next, survives compaction. Load when stuck or new to Axiom. (Aliases: axiom-interactive-guide, axiom-guide, guide, copilot, help) |

### Platform Awareness and Routing
| Skill | Description |
|-------|-------------|
| `axiom-capability-surface` | Full catalog of Axiom capabilities (this skill) |
| `axiom-skill-map` | Structured decision tree that routes problem types to recommended skill sets. Complements this flat catalog with navigable routing: "given problem X, load skills Y and Z." See `specs/85-Skill-Map-Decision-Tree.md`. |

---

## Spec Categories

From `specs/README.md` Spec Range Quick Guide:

| Range | Category | Key Specs |
|-------|----------|-----------|
| 00-02 | Core Product | PRD, architecture, workflows |
| 03-06 | Contracts and Protocols | Plan schema, XML protocol, Jira, config |
| 07-09 | Philosophy and Methodology | Mission, memory bank, baby steps |
| 10-12 | Runtime Behavior | Lifecycle, confidence, retry |
| 13-14 | Integration and Registry | Command registry, Jira/GitHub integrations |
| 15-21 | Planning, Traceability, Repo Layout | Plan history, scaffold, filesystem, code layout, meta-planning |
| 22 | Agent System | Agent roster and interactions |
| 23-31 | API, Networking, Infrastructure | OpenCode contracts, state persistence, logging, test harness, evidence, operating modes, external API |
| 32-34 | Security and Observability | Security hardening, network policy, observability |
| 35-39 | UI and UX | Dashboard, components, UX patterns, design principles |
| 40-51 | Advanced Features | Multi-repo, snapshots, upstream tracking, intake, TODO lifecycle, emergency swarm, cost tracking, test gates, self-unblocking, logging modes, container provisioning |
| 54 | Repository and Onboarding | Scaffold HTTP Install and Onboarding |
| 56 | Repository and Onboarding | Embedded Scaffold Distribution |
| 57 | API and Networking | OpenCode Command Execution Surfaces |
| 58 | Observability | Execution Duration Analytics (deferred) |
| 59 | Advanced Features | Collaborative Intent Resolution |
| 60-66 | Observability | Dashboard design, alert engineering, distributed tracing, metrics instrumentation, diagnostic workflows, predictive observability |
| 67-69 | Orchestration and Admin | Go agent orchestration engine (Morty), web admin panel, test strategy. Morty features: hot-reload (§18), stage group retry (§19), API control plane (§20), session limits, model fallback, fleet routing |
| 70 | Plugin System | OpenCode plugin architecture |
| 71-72 | Cloud and AI | AWS Lambda backend infrastructure, AI image generation skill |
| 73-99 | Advanced Platform | MCP proxy, plan execution, scheduled execution, adversarial review, session forensics, code analysis, multi-modal intake, skill modes, install, listen/notify, TUI dashboard, CI flamegraph/security, multi-Axiom orchestration, AutoTune, middle-out planning, activity reporting, Morty admin API lifecycle, Morty monitor TUI, agent safety, DeeDee QA harness |
| 100-112 | Ecosystem | Pandora Memory Bridge, Harness Engineering, Graph Harness, Runtime Profiler, Expert Platform, Feed Ingestion, Context Stash, Conductor, Distributed Graph, Agent Wire Protocol, Agent Scheduler, Morty Concurrent Execution, Plugin Config Management |
| 113-120 | New Products | Tree Memory (DuckDB), ShellOps (PRD, Architecture, Terminal Mgmt, Log Intelligence, Action Classification, Operating Modes, Sensory Model) |

---

## Key Workflow Patterns

### How Work Flows Through Axiom

1. **Intent arrives** (user request, Jira ticket, or automated trigger)
2. **Intake resolves intent** (`/axiom-intake`) into a plan, questions, or blocked status
3. **Specs are the contract** -- if behavior changes, update `specs/` first
4. **Plans break work into baby steps** -- each step is a single testable accomplishment
5. **Execution is atomic at step boundaries** -- the runner controls ordering and gates
6. **Verification is non-negotiable** -- every step requires evidence (Tier 3+ runtime minimum)
7. **Evidence is durable** -- stored in `.memory-bank/work-items/<ID>/runs/<RUN_ID>/`

### How to Approach Any Task

1. **Read the spec** -- find the relevant `specs/NN-*.md` file
2. **Check existing work** -- look in `.memory-bank/TODO.md` and work items
3. **Pick the right command** -- use the Commands table above
4. **Pick the right agent** -- use the Agents table above
5. **Load the right skill** -- use the Skills table above, or run `/axiom-skill-map query <problem>` for structured routing
6. **Execute in baby steps** -- smallest meaningful change, validate after each step
7. **Capture evidence** -- never claim done without concrete outputs

### What to Do When Stuck

1. **Load `axiom-copilot`** for hands-on interactive guidance (assumes you know nothing, walks you through it)
2. **Run `/axiom-skill-map query <problem>`** for structured skill routing based on your problem
3. **Load this skill** (`axiom-capability-surface`) for full platform awareness
4. **Run `/axiom-sitrep`** for current status
5. **Check `.memory-bank/TODO.md`** for active routing
6. **Invoke `@assumption-buster-axiom`** to surface hidden prerequisites
7. **Invoke `@devils-advocate-axiom`** to challenge your approach
8. **Check `specs/12-Retry-And-Escalation.md`** for retry/escalation rules

### Bug-Fix Gate System (7 gates, automatic for bug/hotfix work items)

When `@pm-axiom` and `@dev-axiom` handle bug-fix work items, the following gates run automatically in order:

Gate 1 (Staleness) → Gate 2 (Bug Fix Mode) → Gate 3 (Strategy Falsification) → Gate 4 (Reproduce-or-Flag) → Gate 5 (Live/Dead Path) → Gate 6 (PR Scope) → Gate 7 (Post-PR Review Bot)

See `specs/20-Meta-Planning.md#gate-order` for the full gate sequence, semantics, and conflict resolution rules.

---

## Maintenance

This skill MUST be updated when:
- A skill is added to or removed from `.opencode/skills/`
- A command is added to or removed from `.opencode/commands/`
- An agent is added to or removed from `.opencode/agents/`
- A new spec category is added to `specs/README.md`

Verification counts (must match source directories):
- Commands: 38 (source: `.opencode/commands/*.md`)
- Agents: 38 (source: `.opencode/agents/*.md`)
- Skills: 143 (source: `.opencode/skills/*/SKILL.md`, including this skill)
- Spec files: 74 numbered specs (source: `specs/README.md`)

axiom:trace work_item=hardening-skills-01 spec=axiom-capability-surface jira_ref=SWDE-7 plan=phase-1/task-9/step-1
