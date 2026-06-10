---
name: hardening-sre
description: Reliability/SRE audit — missing timeouts, retry without backoff, no circuit breakers, rate limiting gaps, goroutine/thread/connection leaks, missing graceful shutdown, swallowed errors. Acceptance criteria must be runtime-testable via failure injection.
---

# Hardening: Reliability & SRE Audit

Load `hardening-anti-patterns-axiom` for the shared audit header and finding format.
Load `hardening-sre-axiom` for the full SRE audit checklist and remediation patterns.

Run the SRE audit prompt against this codebase.

For each finding, acceptance_criteria should be runtime-testable: "outbound call X has a Y-second timeout and returns a fallback response when exceeded, as demonstrated by a test that injects delay."

Key areas to check:
- Timeouts: missing on outbound HTTP calls, DB queries, queue ops
- Retries: missing where they'd help, retry without backoff, no cap (thundering herd)
- Circuit breakers: missing on flaky external dependencies
- Rate limiting: missing on expensive or external-facing endpoints
- Graceful degradation: crashes instead of degraded-but-useful response
- Resource management: goroutine/thread/connection leaks, unbounded queues
- Startup and shutdown: no drain on SIGTERM, no distinct readiness probe
- Error handling: errors swallowed silently, panic where retry would do

axiom:trace work_item=hardening-skills-01 jira_ref=SWDE-7 plan=phase-1/task-10/step-5
