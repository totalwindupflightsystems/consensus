---
name: hardening-spof
description: Audit this codebase for single points of failure — missing timeouts, no circuit breakers, no replicas, no fallback paths, shared resources that stop unrelated features. Produces HARDEN-SPOF-* findings prioritized by blast radius.
---

# Hardening: Single Points of Failure Audit

Load `hardening-anti-patterns-axiom` for the shared audit header and finding format.
Load `hardening-spof-axiom` for the full SPOF detection checklist, blast radius calculation, and remediation patterns.

Run the SPOF audit prompt against this codebase. Prioritize findings by blast radius: how many user-facing features break if this one thing goes down?

Key areas to check:
- HTTP calls without timeout, retry, or circuit breaker
- Single database/cache/queue with no replica or failover
- Third-party dependencies with no fallback path
- Shared resources whose failure stops unrelated features
- Configuration or secrets sources with no redundancy

axiom:trace work_item=hardening-skills-01 jira_ref=SWDE-7 plan=phase-1/task-10/step-2
