---
name: hardening-database
description: Database and data layer audit — N+1 queries, connection pool exhaustion, transactions spanning HTTP calls, table-locking migrations, dual-write consistency gaps. Migration findings require human review.
---

# Hardening: Database & Data Layer Audit

Load `hardening-anti-patterns-axiom` for the shared audit header and finding format.
Load `hardening-database-axiom` for the full database audit checklist and remediation patterns.

Run the database audit prompt against this codebase.

**Migration findings MUST have `requires_human_review: true`.** Migrations are irreversible operations that can cause data loss or production outages.

Key areas to check:
- Connection handling: unbounded pools, missing timeouts, no retry on transient failures
- Query patterns: N+1 queries, missing indexes, full-table scans
- Transactions: missing transactions on multi-step writes, transactions spanning HTTP calls
- Migrations: destructive migrations with no rollback, table-locking DDL during peak traffic
- Consistency: dual-writes to DB and cache with no invalidation
- Resilience: what happens if the primary DB is unreachable for 30 seconds? 5 minutes?

axiom:trace work_item=hardening-skills-01 jira_ref=SWDE-7 plan=phase-1/task-10/step-4
