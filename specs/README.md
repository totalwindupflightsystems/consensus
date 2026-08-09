# Specs

This directory holds the living specifications for Consensus — a database-native
cognitive architecture. The spec set is the contract surface for the project:
AGENTS.md directs agents to read this inventory first, and each spec is the
authoritative reference for its subsystem.

## Reading Order

New agents should start with `specs/000-north-star.md` (project North Star),
then `specs/00-PRD.md` (product requirements) and `specs/001-architecture.md`
(system architecture) before diving into subsystem specs.

## Inventory

- `specs/000-north-star.md` — 000 — North Star: Consensus
- `specs/00-PRD.md` — PRD — Consensus: Database-Native Cognitive Architecture
- `specs/001-architecture.md` — SPEC-001: System Architecture & Core Philosophy
- `specs/002-memory.md` — SPEC-002: Cognitive Memory Engine
- `specs/003-database.md` — SPEC-003: Database Schema & Parity Layer
- `specs/004-subagents.md` — SPEC-004: Subagent Spawning, Process Control & Communication
- `specs/005-security.md` — SPEC-005: Security — RLS, Alt-Mode, Cognitive Firewall & Secrets
- `specs/006-transactions.md` — SPEC-006: Atomic Cognition — Transactions, Rollback & Error Handling
- `specs/007-json-schema.md` — SPEC-007: LLM JSON Output Schema
- `specs/008-harness.md` — SPEC-008: The Harness — Execution Loop & Runtime
- `specs/009-deployment.md` — SPEC-009: Deployment
- `specs/010-tools.md` — SPEC-010: Tools
- `specs/011-canonical-definitions.md` — SPEC-011: Canonical Definitions — Cross-Spec Reconciliation
- `specs/012-system-prompt-and-discovery.md` — SPEC-012: System Prompt & Schema Discovery Protocol
- `specs/013-webhooks-and-events.md` — SPEC-013: Webhook & External Event Ingestion
- `specs/014-hitl-interrupt-state.md` — SPEC-014: Human-in-the-Loop (HITL) Interrupt State
- `specs/015-api-and-mcp.md` — SPEC-015: API & External Interface Layer
- `specs/016-cli-interface.md` — SPEC-016: CLI Interface
- `specs/017-ui-adapter-layer.md` — SPEC-017: TUI Protocol Shims — Borrow the Frontend
- `specs/018-openapi-contract.md` — SPEC-018: OpenAPI Contract
- `specs/019-user-interaction-flows.md` — SPEC-019: User Interaction Flows
- `specs/020-multi-turn-planning.md` — SPEC-020: Interactive Transaction Staging
- `specs/021-repository-layout.md` — SPEC-021: Repository Layout & Go Project Structure
- `specs/022-library-research.md` — SPEC-022: Library Research & Dependency Decisions
- `specs/023-adr-bootstrap-key-expiry.md` — ADR-023: Bootstrap Admin Key Expiry — 90-Day Default TTL
- `specs/026-dashboard-ui.md` — SPEC-026: Dashboard UI

## Supporting Files

- `specs/README.md` — This file (spec inventory)
- `specs/_index.md` — Spec inventory index
- `specs/_prompt.md` — Rules for writing/updating specs

> **Note:** keep this inventory in sync with `ls specs/*.md` — when adding a
> spec, add its entry here in the same commit.
