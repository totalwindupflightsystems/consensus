---
name: temporal-shellops
description: >
  Temporal workflow orchestration patterns: workflow and activity authoring in Python,
  worker configuration, schedules, retry policies, search attributes, multi-cluster
  federation, Aurora PostgreSQL backend setup, Helm chart operations (infra-charts/temporal),
  and Axiom traceability integration. General-purpose workflow platform alongside Flyte.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/00-PRD.md
  related_skills:
    - flyte-shellops
    - kubernetes-shellops
    - helm-shellops
    - external-secrets-shellops
    - aws-cli-shellops
tags:
  vertical: [devops, workflows, temporal, orchestration]
  category: workflow-orchestration
  core: false
---

# Temporal — Axiom Integration Skill

> **"Workflows are durable functions. A crash mid-execution replays from the last checkpoint."**
> **"Activities are the side effects. Workflows are the coordination. Keep them separate."**
> **"Use schedules, not cron jobs. Temporal schedules are observable, pausable, and backfillable."**

This skill covers Temporal as deployed at Dexdat — Aurora PostgreSQL backend
(`aurora-pg` cluster, `temporal` + `temporal_visibility` databases), Helm chart via
`infra-charts/temporal`, multiple worker deployments, and Python SDK workflows for
lab automation, orchestration, and service coordination.

Temporal is the **general-purpose** workflow platform. Use Flyte (`flyte-shellops`)
for ML/scientific pipelines with data lineage. Use Temporal for service orchestration,
lab automation sequences, and long-running business processes.

---


## Scope & Prerequisites

> **Scope**: These patterns are tuned for the Dexdat infrastructure. Values in `<angle brackets>` are placeholders — replace with your environment. Values in `${VAR}` require the named environment variable to be set.

```bash
# Verify required tools
command -v jq      || echo 'MISSING: brew install jq'
command -v kubectl || echo 'MISSING: https://kubernetes.io/docs/tasks/tools/'
command -v aws     || echo 'MISSING: https://awscli.amazonaws.com'
# Set required environment variables
export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text 2>/dev/null)
export WORK_ITEM_ID="${WORK_ITEM_ID:-$(date +%Y%m%d)-ops}"   # Axiom work item ID
export CLUSTER_NAME="${CLUSTER_NAME:-}"   # aws eks list-clusters --region $AWS_REGION
```

---

## Activation

Load this skill when:
- Writing a new Temporal workflow or activity in Python
- Setting up a new Temporal worker deployment
- Configuring retry policies, timeouts, or heartbeating for activities
- Using Temporal schedules (replacing cron jobs)
- Debugging a stuck or failed workflow execution
- Deploying or upgrading the Temporal server via `infra-charts/temporal`
- Setting up a new Temporal namespace
- Writing workflow search attributes for query/filtering

---

## Non-Negotiables

1. **Activities must heartbeat for long-running operations.** Any activity that takes
   more than 30 seconds must call `activity.heartbeat()` periodically. Without heartbeats,
   Temporal assumes the worker crashed and reschedules.

2. **Workflow code must be deterministic.** No `datetime.now()`, `random()`, `uuid4()`,
   or I/O in workflow functions. Side effects belong in activities. Use
   `workflow.now()` and `workflow.uuid4()` instead.

3. **Set `start_to_close_timeout` on every activity.** Never rely on the default
   (infinite). An activity without a timeout can block a workflow forever.

4. **Use `workflow.execute_activity` not direct function calls.** Direct calls bypass
   Temporal's retry, timeout, and durability guarantees entirely.

5. **Namespace per environment.** Don't use the `default` namespace for production.
   Use `production`, `staging`, `development` namespaces with separate retention policies.

---

## Workflow and Activity Authoring (Python SDK)

### Basic Pattern

```python
# workflows/lab_automation/sample_processing.py
import asyncio
from datetime import timedelta
from temporalio import activity, workflow
from temporalio.common import RetryPolicy

# ── Activities (side effects — I/O, external calls) ─────────────────────────

@activity.defn
async def fetch_sample_metadata(sample_id: str) -> dict:
    """Fetch sample data from LIMS API. Has I/O — must be an activity."""
    activity.heartbeat(f"Fetching sample {sample_id}")  # Required for long ops

    async with httpx.AsyncClient() as client:
        resp = await client.get(
            f"https://lims.dexdat.ai/api/samples/{sample_id}",
            headers={"Authorization": f"Bearer {get_token()}"},
            timeout=30.0,
        )
        resp.raise_for_status()
        return resp.json()

@activity.defn
async def run_instrument_protocol(
    sample_id: str,
    protocol_id: str,
    workcell: str,
) -> str:
    """Submit protocol to instrument workcell. Long-running — heartbeat required."""
    # Heartbeat every 30s so Temporal knows we're alive
    for i in range(0, 3600, 30):   # Up to 1 hour
        activity.heartbeat(f"Protocol running — elapsed: {i}s")
        result = await check_protocol_status(protocol_id)
        if result["status"] in ("completed", "failed"):
            return result["status"]
        await asyncio.sleep(30)

    raise TimeoutError("Protocol did not complete within 1 hour")

# ── Workflow (coordination — no I/O, must be deterministic) ─────────────────

@workflow.defn
class SampleProcessingWorkflow:
    """End-to-end sample processing workflow.

    axiom:trace work_item=${WORK_ITEM_ID} impl=workflows/lab_automation/sample_processing.py
    """

    @workflow.run
    async def run(self, sample_id: str, protocol_id: str) -> dict:
        # Fetch metadata — retries 3 times with exponential backoff
        metadata = await workflow.execute_activity(
            fetch_sample_metadata,
            sample_id,
            start_to_close_timeout=timedelta(minutes=2),
            retry_policy=RetryPolicy(
                maximum_attempts=3,
                initial_interval=timedelta(seconds=5),
                backoff_coefficient=2.0,
                non_retryable_error_types=["ValueError"],
            ),
        )

        # Run instrument protocol — long-running, needs heartbeat
        status = await workflow.execute_activity(
            run_instrument_protocol,
            args=[sample_id, protocol_id, metadata["assigned_workcell"]],
            start_to_close_timeout=timedelta(hours=2),   # Max 2 hours
            heartbeat_timeout=timedelta(minutes=2),       # Must heartbeat every 2 min
            retry_policy=RetryPolicy(maximum_attempts=2),
        )

        return {"sample_id": sample_id, "status": status, "metadata": metadata}
```

---

## Worker Setup

```python
# workers/lab_automation/worker.py
import asyncio
from temporalio.client import Client
from temporalio.worker import Worker
from workflows.lab_automation.sample_processing import (
    SampleProcessingWorkflow,
    fetch_sample_metadata,
    run_instrument_protocol,
)

async def main():
    client = await Client.connect(
        "temporal.dexdat.ai:7233",       # Temporal frontend service
        namespace="production",
        tls=True,                       # Always TLS in production
    )

    worker = Worker(
        client,
        task_queue="lab-automation",    # Task queue name — must match workflow registration
        workflows=[SampleProcessingWorkflow],
        activities=[fetch_sample_metadata, run_instrument_protocol],
        max_concurrent_activities=10,
        max_concurrent_workflow_tasks=50,
    )

    await worker.run()

if __name__ == "__main__":
    asyncio.run(main())
```

### Worker Kubernetes Deployment

```yaml
# Helm chart template for worker deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: lab-automation-worker
  namespace: lab-automation
  labels:
    app: lab-automation-worker
    version: "1.0.0"
  annotations:
    axiom.io/work-item: "{{ .Values.codeopsWorkItem }}"
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      app: lab-automation-worker
  template:
    spec:
      serviceAccountName: lab-automation-worker
      containers:
        - name: worker
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          command: ["python", "-m", "workers.lab_automation.worker"]
          resources:
            requests:
              cpu: "500m"
              memory: "512Mi"
            limits:
              cpu: "2000m"
              memory: "2Gi"
          env:
            - name: TEMPORAL_HOST
              value: "temporal.dexdat.ai:7233"
            - name: TEMPORAL_NAMESPACE
              value: production
          envFrom:
            - secretRef:
                name: lab-automation-secrets  # Via ExternalSecret
```

---

## Schedules (replace cron jobs)

```python
# Create a schedule (run once at startup / from CI)
from temporalio.client import Client, Schedule, ScheduleActionStartWorkflow, ScheduleSpec, ScheduleIntervalSpec

async def create_daily_report_schedule():
    client = await Client.connect("temporal.dexdat.ai:7233", namespace="production")

    await client.create_schedule(
        "daily-sample-report",
        Schedule(
            action=ScheduleActionStartWorkflow(
                DailyReportWorkflow.run,
                id="daily-report-{scheduledTime}",
                task_queue="reporting",
            ),
            spec=ScheduleSpec(
                cron_expressions=["0 8 * * 1-5"],  # Monday-Friday 8am UTC
            ),
        ),
    )
```

```bash
# Temporal CLI — manage schedules
temporal schedule list --namespace production

# Trigger schedule immediately (backfill / manual run)
temporal schedule trigger daily-sample-report --namespace production

# Pause a schedule
temporal schedule pause daily-sample-report \
  --reason "Maintenance window" \
  --namespace production
```

---

## CLI Operations (Evidence-Producing)

```bash
# Check cluster health
temporal operator cluster health --address temporal.dexdat.ai:7233

# List workflows by status
temporal workflow list \
  --namespace production \
  --query 'ExecutionStatus="Failed"' \
  --limit 20

# Show workflow details
temporal workflow show \
  --workflow-id my-workflow-id \
  --namespace production \
  --output json | tee .memory-bank/work-items/${WORK_ITEM_ID}/temporal-workflow.json

# Reset a failed workflow (replay from specific event)
temporal workflow reset \
  --workflow-id my-workflow-id \
  --run-id <run-id> \
  --event-id <event-id> \
  --reason "Retrying after fix" \
  --namespace production

# Terminate a stuck workflow
temporal workflow terminate \
  --workflow-id my-workflow-id \
  --reason "Manually terminated — $WORK_ITEM_ID" \
  --namespace production
```

---

## Debugging Stuck Workflows

```bash
# Check Temporal server pods
kubectl get pods -n temporal

# Frontend service logs (connection issues)
kubectl logs -n temporal \
  $(kubectl get pods -n temporal -l app.kubernetes.io/component=frontend -o name | head -1) \
  | grep -E "ERROR|WARN" | tail -30

# Worker pod logs
kubectl logs -l app=lab-automation-worker -n lab-automation --tail=50

# Check Aurora DB connectivity (Temporal uses postgres)
# temporal + temporal_visibility databases on aurora-pg cluster

# Common issues:
# "sticky queue" errors: worker restarted; workflow replays from history
# "activity heartbeat timeout": heartbeat interval too long; reduce sleep
# "workflow execution timeout": increase WorkflowExecutionTimeout
# "namespace not found": create namespace first with temporal operator namespace create
```

---

## Namespace Management

```bash
# Create a namespace (run once per environment)
temporal operator namespace create production \
  --retention 30d \
  --description "Production workflows" \
  --address temporal.dexdat.ai:7233

temporal operator namespace create staging \
  --retention 14d \
  --address temporal.dexdat.ai:7233

temporal operator namespace create development \
  --retention 3d \
  --address temporal.dexdat.ai:7233

# List namespaces
temporal operator namespace list --address temporal.dexdat.ai:7233
```

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| I/O or `datetime.now()` in workflow code | Non-deterministic; breaks replay | Move to activity; use `workflow.now()` |
| Activity with no `start_to_close_timeout` | Blocks workflow forever on hang | Always set explicit timeout |
| Long activity with no heartbeat | Temporal assumes crash; reschedules | `activity.heartbeat()` every <`heartbeat_timeout/2` |
| Using `default` namespace in production | No retention control; noisy from dev workflows | Named namespace per environment |
| Direct function calls instead of `execute_activity` | Bypasses retry/timeout/durability | Always use `workflow.execute_activity()` |
| Worker with `max_concurrent_activities=1` | Single worker bottleneck | Tune based on activity I/O vs CPU profile |
| Cron jobs via Kubernetes CronJob | No visibility, no backfill, no pause | Temporal Schedules instead |
| Storing large data in workflow state | Temporal history size limit (~50MB); slow replay | Pass S3 paths or IDs; load in activities |

---

## axiom:trace

`axiom:trace work_item=devops-skills-01 spec=specs/00-PRD.md plan= prompt=.opencode/skills/temporal-shellops/SKILL.md evidence= doc= ops= commit=`
