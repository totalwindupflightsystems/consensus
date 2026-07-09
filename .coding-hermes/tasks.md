# Coding Hermes Task Queue — Consensus

## Phase 1: Ship the Product (Chronicle UI)

### 1.1 Design System Foundation
- [x] Create Chronicle design system CSS (color tokens §1.2, typography §1.3, spacing §1.4, iconography §1.5, animation tokens §1.6, theme support §1.7) — from specs/026-dashboard-ui.md §1
- [x] Create internal/chronicle/ Go package with server, /chronicle/ route wired in cmd/consensus/main.go
- [x] Serve CSS + minimal placeholder page at /chronicle/ with Chronicle header/nav

### 1.2 Layout Architecture
- [x] Build layout shell: app chrome (header, nav sidebar, main panel area, footer status) — §2
  → layout.css (CSS Grid shell, collapsible sidebar, status bar, responsive 3-tier breakpoints)
  → index.html (rewritten with proper top bar, sidebar nav sections, status bar, keyboard shortcuts)
- [x] Implement command palette (⌘K) with fuzzy search, keyboard shortcuts — §2.5
- [x] Responsive breakpoints (sidebar collapses, panels stack) — §2.7

### 1.3 Shared Components
- [x] Build component library: Button, Input, Select, Badge, Card, Modal, Toast, Tooltip, Skeleton — §3
  → chronicle/css/components.css (1,089 lines, 9 components, 30+ variants, responsive)
- [x] Data display: Table (sortable, paginated, selectable), CodeBlock, JSONViewer — §3.5-3.6
- [x] Status indicators: status dots, trust badges, entity type icons — §3.7

### 1.4 Investigation Workbench
- [x] Build THINK pane: streaming thought cards, step numbers, bidirectional finding links — §4.2
- [x] Build SAYS pane: finding cards, evidence citations, trust badges, approve/reject — §4.3
- [ ] Build Input Area: message composer, model selector, budget display, send/cancel — §4.1
- [ ] Build Evidence Panel: source list, citation preview, provenance chain — §4.4
- [ ] Build Discovery Panel: related sessions, similar findings, entity links — §4.5
- [ ] Wire WebSocket for live streaming (THINK cards update at 60fps) — §4.6

### 1.5 Dashboard & Analytics
- [ ] Build Overview Dashboard: active sessions, cost metrics, health status — §5
- [ ] Build Status Dashboard: system health, LLM availability, DB stats — §5.3

### 1.6 Timeline Explorer + Entity Graph
- [ ] Build Timeline Explorer: chronological memory view, zoom, filter by type — §6
- [ ] Build Entity Graph Visualizer: D3.js force-directed graph, entity nodes/edges — §7

### 1.7 Integration
- [ ] Wire Chronicle UI to Consensus API (sessions, memory, retrieval, timeline, entity graph)
- [ ] E2E test: full investigation workflow through Chronicle UI → Consensus API → real LLM → results

## Phase 2: Production Readiness
- [ ] Publish Docker image to GitHub Container Registry (ghcr.io/totalwindupflightsystems/consensus)
- [ ] Write deployment quickstart guide (README section: "5-Minute Setup")
- [ ] Add docker-compose production profile (Consensus + Postgres + pgvector, no dev tools)
- [ ] Verify Postgres agent_role connections in production scenario (are RLS roles actually used?)
- [ ] Write SDK/client library (Go client for consensus serve API)

## Phase 3: Hardened Testing
- [ ] Test: provider failure mid-call (retry with backoff, fallback to LM Studio)
- [ ] Test: 2+ concurrent sessions processing simultaneously
- [ ] Test: strict budget exhaustion ($0.01, verify agent stops LLM calls)
- [ ] Test: memory poisoning via external_quarantine (inject malicious data, verify sanitized)
- [ ] Test: schema migration under load (add column while sessions active, verify no data loss)
- [ ] Test: 100+ iteration durability (long-running session stability)

## Phase 4: AC Gap Closure
- [ ] AC-040: (verify acceptance criteria and implement/fix)
- [ ] AC-053: (verify acceptance criteria and implement/fix)
- [ ] AC-054: (verify acceptance criteria and implement/fix)
- [ ] AC-055: (verify acceptance criteria and implement/fix)
- [ ] Update AC-056 to reflect Postgres verification results
- [ ] Update AC-011 to reflect correct migration counts (17 SQLite, 19 Postgres)

## Phase 5: Expand UI Spec (ongoing)
- [ ] Write Section 8 (Search & Query) of specs/026-dashboard-ui.md at blind-developer density
- [ ] Write Section 9 (Reports & Export) at blind-developer density
- [ ] Write Section 10 (Settings & Configuration) at blind-developer density
- [ ] Write Sections 11-24 at outline completeness minimum

## Active
- [ ] Create .coding-hermes/tasks.md (BOOTSTRAP — this file)

## Completed
- [x] GitReins guards (secrets, lint, tests) — v0.7.9 configured, 20 tasks passing
- [x] Build passes on current toolchain (Go 1.26)
- [x] CI/CD pipeline (7 jobs green, cross-compile 5 platforms)
- [x] SQLite + Postgres dual-backend (17 + 19 migrations)
- [x] E2E real LLM tests passing
- [x] Semantic retrieval layer (internal/memory/retrieval.go)
- [x] Live demo (demo/demo_test.go) passing
- [x] GitReins Tier 2 evaluator with speculative decoding
- [x] 6 real bugs fixed from GitReins semantic evaluation
- [x] consensus-watchdog cron running every 4h
- [x] Admin UI (/ui/ route, 340-line Go HTML)
- [x] DESIGN.md (Google format, WCAG compliant)
- [x] docs/diagrams.md (11 Mermaid diagrams)
- [x] chronicle.html product page on GitHub Pages
