# Ops Runbooks

Operational runbooks for the Conscience agent runtime. Each runbook follows the symptom → triage → recovery → verification pattern.

| Runbook | Purpose | Severity |
|---------|---------|----------|
| [deployment.md](deployment.md) | Binary build, DB migration, health check | Normal |
| [troubleshooting.md](troubleshooting.md) | Log locations, startup failures, common config errors | Varies |
| [backup-restore.md](backup-restore.md) | Postgres/SQLite backup and restore | Critical |
| [admin-key-rotation.md](admin-key-rotation.md) | Rotate bootstrap admin key | High |
| [failure-modes.md](failure-modes.md) | LLM errors, DB loss, disk full, OOM | P1-P3 |

> **Trace**: `axiom:trace work_item=WI-019 spec=specs/009-deployment.md doc=docs/runbooks/`
