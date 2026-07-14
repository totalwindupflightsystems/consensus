# Consensus — Product Requirements Document

**Version:** 1.0  
**Date:** 2026-07-14  
**Status:** Phase 1 Complete, Phase 2 In Progress

---

## 1. Executive Summary

Consensus is an **agent runtime where the database IS the harness.** Every agent action — reasoning, tool calls, memory writes, session state — lives in SQL tables with append-only enforcement, schema-level trust policies, and row-level security. There is no separate orchestrator script. The database is the agent's brain, not just its filing cabinet.

**The product:** Chronicle — an AI Investigation Workbench for regulated industries. Security analysts, investigative journalists, and compliance officers run AI-powered investigations where every reasoning trace, finding, and evidence citation is immutably recorded. When regulators audit, you produce the ledger, not the excuse.

**The business model:** Open-source engine (Consensus), enterprise product (Chronicle). The runtime is free and auditable. The workbench, compliance dashboards, and SSO are paid.

---

## 2. Problem Statement

### 2.1 The Agent Trust Gap

Current agent frameworks (LangChain, CrewAI, AutoGen, raw OpenAI function calling) share a fatal flaw: **the agent's reasoning is ephemeral.** When an AI agent makes a decision that costs money, violates policy, or produces harmful output, there is no audit trail. You can't answer:

- What did the agent **think** before it acted?
- What evidence did it **cite** for its conclusion?
- Who **approved** the action, and when?
- Can I **prove** to a regulator that the agent followed policy?

Every existing framework treats the database as a persistence bucket — dump the final output, move on. The reasoning trace is lost. The evidence chain is broken. The memory is overwritable.

### 2.2 The Regulated Industry Mandate

Finance (SOX, Basel III), healthcare (HIPAA), legal (e-discovery rules), and defense (AUTHORIZATION frameworks) require:

| Requirement | Current State | Consensus Solution |
|-------------|--------------|-------------------|
| Immutable audit trail | Best-effort logging | Database-level append-only triggers |
| Reasoning transparency | Lost in prompt/response cycle | THINK/SAYS split-pane, stored per-turn |
| Multi-tenant isolation | Shared state, accidental leaks | Row-level security per session/project |
| Evidence chain of custody | Manual, ad-hoc | Citation linking to source evidence |
| Compliance attestation | Months of manual audit prep | One query: `SELECT * FROM memory_events WHERE session_id = X` |

---

## 3. Product Vision

### 3.1 Chronicle — AI Investigation Workbench

A dark-theme operational dashboard for AI-powered investigations. Operators run investigations where:

1. **THINK pane** streams the agent's reasoning in real-time — every assumption, every deduction, every doubt
2. **SAYS pane** displays findings with evidence citations, trust badges, and approval workflow
3. **Memory ledger** stores every thought, finding, and evidence link as append-only rows
4. **Timeline Explorer** reconstructs the full investigation chronologically
5. **Entity Graph** visualizes relationships between entities, sources, and findings

### 3.2 Core Value Proposition

> **Trust through transparency.** When your SOC analyst uses AI to investigate a breach, or your journalist uses AI to verify a source, every step is recorded, every reasoning trace is preserved, every finding is backed by evidence. When the regulator asks "how did you reach this conclusion?", you show them the ledger — not a blog post about your AI policy.

### 3.3 Target Users

| Persona | Use Case | Why Consensus |
|---------|----------|---------------|
| **SOC Analyst** | Investigate security incidents with AI | Immutable chain of custody, evidence citations |
| **Investigative Journalist** | Verify sources, trace information provenance | Source protection, reasoning transparency |
| **Compliance Officer** | Audit AI-assisted decisions | Regulatory-grade audit trail, policy enforcement |
| **Legal Discovery** | AI-assisted document review | Defensible process, judge-ready evidence chains |
| **Defense/Intel Analyst** | Intelligence synthesis | Classification-aware isolation, tamper-proof records |

---

## 4. Architecture

### 4.1 The Database IS the Runtime

```
┌─────────────────────────────────────────────────────┐
│  Consensus Runtime (Go binary, single process)       │
│                                                       │
│  ┌─────────┐  ┌──────────┐  ┌───────────────┐       │
│  │ REST API │  │ OpenCode │  │ MCP Server     │       │
│  │ (chi)    │  │ Shim     │  │ (agent tools)  │       │
│  └────┬─────┘  └────┬─────┘  └───────┬───────┘       │
│       │              │               │               │
│  ┌────┴──────────────┴───────────────┴───────────┐   │
│  │              Service Layer                      │   │
│  │  sessions │ planning │ memory │ retrieval │ billing │
│  └──────────────────────┬────────────────────────┘   │
│                         │                            │
│  ┌──────────────────────┴────────────────────────┐   │
│  │              Database Layer                     │   │
│  │  SQLite (dev) │ PostgreSQL + pgvector (prod)    │   │
│  │  Append-only triggers │ Row-level security      │   │
│  │  Vector embeddings │ Semantic retrieval         │   │
│  └────────────────────────────────────────────────┘   │
│                                                       │
│  ┌────────────────────────────────────────────────┐   │
│  │  Chronicle UI (embedded, dark-theme, 300KB+)    │   │
│  │  THINK pane │ SAYS pane │ Dashboard │ Timeline  │   │
│  └────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

### 4.2 Key Technical Innovations

1. **Append-only memory** — `CREATE TRIGGER ... BEFORE UPDATE ... RAISE(ABORT, 'memory_events is append-only')` at the database level. Not application-level. Not "please don't mutate." Enforced by the DB engine.

2. **Row-level security** — PostgreSQL RLS policies. Session A cannot read Session B's memory events. Project isolation enforced by the database, not middleware.

3. **Split-pane consciousness** — Every agent turn produces THINK (internal reasoning) and SAYS (public output). Both stored. Both retrievable. The agent can lie in SAYS while reasoning honestly in THINK — and both are recorded.

4. **Semantic retrieval** — pgvector-backed. `FindSimilar(query, threshold)` returns relevant past memories. No RAG framework needed — the database does it.

5. **OpenCode shim** — 26 endpoints translating OpenCode HTTP protocol into Consensus native API. Any tool that speaks OpenCode can use Consensus as its runtime.

6. **Dual backend** — SQLite for development/embedded, PostgreSQL for production. Same schema, same migrations, same API.

---

## 5. Current State (July 2026)

### 5.1 Phase 1 — Chronicle Investigation Workbench ✅ COMPLETE

| Component | Status | Lines |
|-----------|--------|-------|
| Design system (CSS tokens) | ✅ | 541 |
| Layout shell + command palette | ✅ | — |
| Component library (9 components) | ✅ | 1,089 |
| THINK pane (streaming thought cards) | ✅ | — |
| SAYS pane (findings, trust badges) | ✅ | — |
| Input Area (composer, model selector) | ✅ | — |
| Evidence Panel + Discovery Panel | ✅ | — |
| WebSocket/SSE streaming | ✅ | — |
| Overview Dashboard (KPIs, activity) | ✅ | — |
| Health Dashboard (system status) | ✅ | 604 |
| Timeline Explorer (zoom, filter, bookmarks) | ✅ | 1,596 |
| Entity Graph (D3.js force-directed) | ✅ | 827 |
| API wiring (sessions, memory, retrieval) | ✅ | — |
| **E2E integration test (real LLM)** | ✅ | 310 |
| **Chronicle UI serving (311KB)** | ✅ | — |

### 5.2 Phase 2 — Shim Health & Compatibility ⏳ IN PROGRESS

- [ ] Verify all 26 shim endpoints respond correctly
- [ ] Add shim smoke test to CI
- [ ] Document shim endpoints in OpenAPI
- [ ] Test OpenCode CLI against Consensus shim
- [ ] Test VSCode/Cursor extension against shim
- [ ] Keep shim_session_map migration current
- [ ] **models.dev integration** — auto-sync model_registry

### 5.3 Phase 3 — Production Readiness

- [ ] Docker image published to GHCR
- [ ] Deployment quickstart guide
- [ ] docker-compose production profile
- [ ] Postgres agent_role connection verification
- [ ] Go SDK/client library

### 5.4 Phase 4 — Hardened Testing

- [ ] Provider failure + retry
- [ ] Concurrent sessions
- [ ] Budget exhaustion
- [ ] Memory poisoning via quarantine
- [ ] Schema migration under load
- [ ] 100+ iteration durability

---

## 6. Competitive Analysis

### 6.1 Direct Comparison

| Feature | Consensus | LangChain | CrewAI | AutoGen | Raw OpenAI |
|---------|-----------|-----------|--------|---------|------------|
| Append-only memory | ✅ DB triggers | ❌ | ❌ | ❌ | ❌ |
| Reasoning transparency | ✅ THINK/SAYS split | ❌ | ❌ | ❌ | ❌ |
| Multi-tenant isolation | ✅ RLS | ❌ | ❌ | ❌ | ❌ |
| Semantic retrieval | ✅ pgvector built-in | 🔶 via plugins | ❌ | ❌ | ❌ |
| Audit trail | ✅ One SQL query | 🔶 best-effort | ❌ | ❌ | ❌ |
| OpenCode-compatible | ✅ Full shim (26 endpoints) | ❌ | ❌ | ❌ | ❌ |
| Self-hosted | ✅ Single binary | 🔶 | 🔶 | 🔶 | ❌ Cloud only |
| UI included | ✅ Chronicle workbench | ❌ | ❌ | ❌ | ❌ |

### 6.2 Why Consensus Wins for Regulated Use

**LangChain/CrewAI/AutoGen** — Great for prototyping. But when the regulator audits, your "memory" is a Python dict. There is no ledger. No chain of custody. No defense.

**Raw OpenAI/Anthropic API** — The LLM call is just a request/response. Everything between calls — state, reasoning, evidence — is your problem. And if you're in finance or healthcare, "your problem" means "your liability."

**Consensus** — The database IS the defense. Every thought, every finding, every evidence link is immutably stored. When the SEC, HIPAA auditor, or opposing counsel asks "how did your AI reach this conclusion?", you produce the ledger. Not a script. Not a prompt. The ledger.

---

## 7. Business Model

### 7.1 Open-Source Core (Consensus)

- MIT license
- Full runtime: sessions, memory, retrieval, planning, tools
- SQLite + PostgreSQL
- OpenCode shim
- CLI + REST API + MCP server
- Community support via GitHub

### 7.2 Enterprise Product (Chronicle)

- Chronicle Investigation Workbench (dark-theme UI)
- Multi-project dashboards
- SSO (OIDC/SAML)
- Compliance reporting (SOC2, HIPAA attestation templates)
- Audit export (signed PDF, immutable log)
- Role-based access control
- Priority support + SLA
- On-premise deployment support

### 7.3 Pricing (TBD)

| Tier | Price | Includes |
|------|-------|----------|
| Community | Free | Consensus runtime, open-source |
| Pro | $299/mo | Chronicle Workbench, 5 projects |
| Team | $999/mo | Multi-user, SSO, compliance reports |
| Enterprise | Custom | On-prem, SLA, audit support |

---

## 8. Success Metrics

### 8.1 Technical

- **28/28 packages green** — Build + test on every commit
- **20/20 GitReins tasks passing** — Tier 1 + Tier 2 quality gates
- **8 CI jobs green** — Build, test, Postgres, cross-compile, Docker, E2E
- **Sub-10s E2E test** — Real LLM investigation workflow under 10 seconds
- **<50ms P95 API latency** — Session create, message send, memory list

### 8.2 Product

- **Chronicle UI loads in <2s** — 311KB with design system, workbench, dashboards
- **One-command deploy** — `consensus serve` starts everything
- **Drop-in OpenCode replacement** — Any tool using OpenCode protocol works with Consensus

### 8.3 Business

- **10 GitHub stars** → **100** → **1,000**
- **First enterprise pilot** (target: Q4 2026)
- **Compliance attestation template published** (SOC2, HIPAA)

---

## 9. Roadmap

| Quarter | Milestone |
|---------|-----------|
| **Q3 2026** | Phase 2 complete (shim health, models.dev). Phase 3 complete (Docker, docs, SDK). First external testers. |
| **Q4 2026** | Phase 4 complete (hardened testing). Enterprise SSO. Compliance templates. First pilot customer. |
| **Q1 2027** | Phase 5 complete (AC gap closure). Phase 6 complete (UI spec). Multi-project dashboards. Public launch. |
| **Q2 2027** | Enterprise features: RBAC, audit export, on-prem deployment. SOC2 Type I attestation. |

---

## 10. Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| LLM cost unpredictability | High | Tiered model selection, budget enforcement, cost tracking dashboard |
| Open-source competition | Medium | Superior architecture (DB-as-runtime) is defensible; Chronicle UI adds enterprise value |
| Adoption friction | Medium | OpenCode shim = drop-in replacement; one-binary deploy; no infra dependencies |
| Regulatory changes | Low | Append-only ledger is the hardest requirement; schema-level enforcement survives policy changes |
| Team bandwidth | Medium | coding-hermes foreman fleet (25 projects) demonstrates autonomous dev capacity |

---

## Appendices

### A. Glossary

| Term | Definition |
|------|------------|
| **THINK** | Agent's internal reasoning trace, streamed in real-time |
| **SAYS** | Agent's public output — findings, recommendations, evidence citations |
| **Append-only memory** | Database-level enforcement preventing UPDATE/DELETE on memory_events |
| **Shim** | Protocol translator — makes Consensus speak OpenCode's HTTP dialect |
| **RLS** | Row-Level Security — PostgreSQL feature for multi-tenant data isolation |
| **Chronicle** | The investigation workbench UI, dark-theme operational dashboard |

### B. Repository

- **GitHub:** `https://github.com/totalwindupflightsystems/consensus`
- **Branch:** `master`
- **Stack:** Go 1.26, SQLite, PostgreSQL + pgvector
- **CI:** 8 jobs, GitHub Actions
- **Quality:** GitReins v0.7.9 (Tier 1 + Tier 2 guards)
- **Autonomy:** coding-hermes foreman (every 2h, deepseek-v4-pro)

### C. Key Specs

- `specs/026-dashboard-ui.md` — Chronicle UI specification (9,652 lines, Phase 6 expands to 35,000)
- `specs/018-openapi-contract.md` — REST API + shim contracts
- `DESIGN.md` — Design system (Google format, 65 colors, 15 typography, 36 components)
- `docs/diagrams.md` — 11 Mermaid diagrams (UX, state, navigation, data flow)
