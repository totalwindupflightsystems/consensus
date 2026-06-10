---
name: hardening-observability
description: Observability audit — unstructured logging, missing correlation IDs, high-cardinality metric labels, PII in logs, alerts without runbooks, missing SLIs/SLOs. Answers the "3am debugging" question: would an on-call engineer have enough information to debug without reading source code?
---

# Hardening: Observability Audit

Load `hardening-anti-patterns-axiom` for the shared audit header and finding format.
Load `hardening-observability-axiom` for the full observability audit checklist and remediation patterns.

Run the observability audit prompt against this codebase.

Also flag: what symptoms would an on-call engineer see if this service broke at 3am, and would they have enough information to debug without reading source code?

Key areas to check:

**Logging:**
- Critical paths with no structured logging (auth, writes, external calls)
- Unstructured log messages that can't be queried
- Sensitive data (PII, secrets, tokens) logged in cleartext
- Missing correlation IDs across service boundaries

**Metrics:**
- User-facing features with no SLI (latency, error rate, throughput)
- Resource metrics missing (queue depth, connection pool, cache hit rate)
- High-cardinality labels (user_id, request_id in metric labels)
- Metrics with no alerts, or alerts with no runbooks

**Tracing:**
- Request flows that can't be traced end-to-end
- External calls not instrumented
- Missing context propagation (request ID, user ID, tenant ID)

Per the operating doctrine, observability is mandatory: events must correlate by ticket id, repo id, PR id, and run id. Any finding where these correlations are missing is at minimum "high."

axiom:trace work_item=hardening-skills-01 jira_ref=SWDE-7 plan=phase-1/task-10/step-7
