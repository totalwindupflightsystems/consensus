---
name: prometheus-shellops
description: >
  Prometheus and Amazon Managed Prometheus (AMP) patterns: PromQL query authoring,
  recording rules, alerting rules with AlertmanagerConfig, scrape config, DCGM GPU
  metrics, CloudWatch exporter, Pushgateway, Victoria Metrics long-term storage,
  and infra-charts/prometheus Helm chart operations. Tuned for the Dexdat
  observability stack across multi-cluster EKS with AMP as the primary backend.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/34-Observability-And-Metrics.md
  related_skills:
    - grafana-shellops
    - sre-ops-axiom
    - kubernetes-shellops
    - alert-engineering-axiom
    - aws-cli-shellops
tags:
  vertical: [devops, sre, observability, prometheus]
  category: observability
  core: false
---

# Prometheus — Axiom Integration Skill

> **"Every metric needs a unit in its name. `http_requests` is ambiguous. `http_requests_total` is a counter."**
> **"High cardinality kills Prometheus. Labels must have bounded value sets."**
> **"AMP is managed Prometheus — no capacity planning, no storage ops. Use it."**

This skill covers Prometheus and Amazon Managed Prometheus (AMP) as operated at Dexdat.
The stack uses AMP as the primary metrics backend (per-cluster remote_write), Victoria Metrics
for long-term retention, self-hosted Prometheus-Operator for scrape config and alerting rules,
and Grafana for visualization. All deployed via `infra-charts/prometheus` and
`infra-charts/amazon-managed-prometheus`.

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
- Writing new PromQL queries for dashboards or alerts
- Adding `PrometheusRule` CRDs for new alerting or recording rules
- Configuring scrape targets (`ServiceMonitor`, `PodMonitor`)
- Debugging "No data" for a metric in Grafana
- Setting up remote_write to AMP for a new cluster
- Interpreting GPU metrics (DCGM exporter, NVSentinel)
- Configuring the Pushgateway for batch job metrics
- Tuning cardinality or reducing metric volume

---

## Non-Negotiables

1. **Labels must have bounded cardinality.** Never use `user_id`, `request_id`, `trace_id`,
   or any unbounded value as a label. These create millions of time series and crash Prometheus.

2. **Metric names must follow the naming convention.** Format:
   `{namespace}_{subsystem}_{name}_{unit}`. End counters with `_total`, gauges with the
   unit (e.g. `_bytes`, `_seconds`), histograms with `_seconds` or `_bytes`.

3. **`rate()` over `increase()` in alerting rules.** `increase()` has edge cases with
   resets and scrape interval mismatches. Use `rate()` and multiply by the window.

4. **Recording rules for expensive queries.** Any query used in multiple dashboards or
   alerts that takes >1s should be a recording rule. Pre-compute at ingestion time.

5. **Alert `for:` duration minimum 5 minutes.** Zero-duration alerts fire on transient
   spikes and cause alert fatigue. Minimum `for: 5m` except for critical binary-state
   alerts (e.g. `up == 0`).

---

## Prometheus Stack at Dexdat

```
Per-cluster Prometheus-Operator
        │
        │ remote_write (sigv4 auth)
        ▼
Amazon Managed Prometheus (AMP)      ← Primary query backend for Grafana
        │
        │ remote_read (long-term)
        ▼
Victoria Metrics                     ← Long-term retention (90d+)

Grafana datasource → AMP workspace
        ┌─────────────────────────────┐
        │ Also scraped per-cluster:   │
        │ - DCGM exporter (GPU)       │
        │ - NVSentinel (NVIDIA)       │
        │ - CloudWatch exporter       │
        │ - Pushgateway               │
        │ - Node exporter             │
        │ - kube-state-metrics        │
        └─────────────────────────────┘
```

---

## PromQL Patterns

### Golden Signals

```promql
# Request rate (per second, 5m window)
sum(rate(http_requests_total{namespace="$namespace", job="$job"}[5m]))

# Error rate (fraction)
sum(rate(http_requests_total{namespace="$namespace", job="$job", status=~"5.."}[5m]))
/
sum(rate(http_requests_total{namespace="$namespace", job="$job"}[5m]))

# p50/p95/p99 latency
histogram_quantile(0.95,
  sum by (le) (
    rate(http_request_duration_seconds_bucket{namespace="$namespace", job="$job"}[5m])
  )
)

# Saturation — CPU
sum(rate(container_cpu_usage_seconds_total{namespace="$namespace", container!=""}[5m]))
  by (pod)
/
sum(kube_pod_container_resource_limits{namespace="$namespace", resource="cpu"})
  by (pod)
```

### GPU Metrics (DCGM Exporter)

```promql
# GPU utilization per device
avg by (pod, gpu) (
  DCGM_FI_DEV_GPU_UTIL{namespace="$namespace"}
)

# GPU memory used
DCGM_FI_DEV_FB_USED{namespace="$namespace"} * 1024 * 1024   # bytes

# GPU memory free
DCGM_FI_DEV_FB_FREE{namespace="$namespace"} * 1024 * 1024

# GPU power draw
DCGM_FI_DEV_POWER_USAGE{namespace="$namespace"}

# GPU temperature
DCGM_FI_DEV_GPU_TEMP{namespace="$namespace"}

# Alert: GPU idle (utilization < 5% for 15 min — wasted $12/hr)
avg by (node) (DCGM_FI_DEV_GPU_UTIL) < 5
```

### Kubernetes Cluster Health

```promql
# Node not ready
kube_node_status_condition{condition="Ready", status="true"} == 0

# Pod restarts in last hour (potential crash loops)
increase(kube_pod_container_status_restarts_total[1h]) > 5

# PVC almost full (>85%)
(
  kubelet_volume_stats_used_bytes
  / kubelet_volume_stats_capacity_bytes
) * 100 > 85

# Karpenter nodes launching (autoscaling activity)
increase(karpenter_nodes_created_total[5m]) > 0

# Kueue queue depth (ML job backlog)
kueue_pending_workloads_total{namespace="$namespace"}
```

### Temporal Worker Metrics

```promql
# Workflow task schedule-to-start latency p95
histogram_quantile(0.95,
  sum by (le, namespace, task_queue) (
    rate(temporal_workflow_task_schedule_to_start_latency_bucket[5m])
  )
)

# Activity failures
sum by (activity_type) (
  rate(temporal_activity_execution_failed_total[5m])
)

# Worker poll success rate
rate(temporal_worker_task_slots_available[5m])
```

---

## PrometheusRule (alert + recording rules)

```yaml
# infra-charts/prometheus/templates/prometheusrule.yaml pattern
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: my-service-rules
  namespace: monitoring
  labels:
    prometheus: kube-prometheus       # Must match Prometheus CR selector
    role: alert-rules
spec:
  groups:
    # ── Recording rules (pre-compute expensive queries) ──────────────────────
    - name: my_service.recording
      interval: 30s
      rules:
        - record: job:http_requests:rate5m
          expr: |
            sum by (job, namespace, status) (
              rate(http_requests_total[5m])
            )
          labels:
            aggregation: job

    # ── Alert rules ───────────────────────────────────────────────────────────
    - name: my_service.alerts
      rules:
        - alert: MyServiceHighErrorRate
          expr: |
            (
              sum(rate(http_requests_total{job="my-service", status=~"5.."}[5m]))
              /
              sum(rate(http_requests_total{job="my-service"}[5m]))
            ) > 0.05
          for: 5m
          labels:
            severity: warning
            team: platform
          annotations:
            summary: "{{ $labels.namespace }} error rate {{ $value | humanizePercentage }}"
            description: "Error rate has been above 5% for 5 minutes."
            runbook_url: "https://github.com/<YOUR_ORG>/engineering-docs/blob/main/runbooks/my-service.md"

        - alert: MyServiceDown
          expr: up{job="my-service"} == 0
          for: 2m                    # Shorter for binary up/down
          labels:
            severity: critical
            team: platform
          annotations:
            summary: "{{ $labels.instance }} is down"
            runbook_url: "https://github.com/<YOUR_ORG>/engineering-docs/blob/main/runbooks/my-service.md"

        - alert: GPUIdleWaste
          expr: |
            avg by (node) (DCGM_FI_DEV_GPU_UTIL) < 5
          for: 15m
          labels:
            severity: warning
            team: mlops
          annotations:
            summary: "GPU node {{ $labels.node }} is idle — check for stuck jobs"
            runbook_url: "https://github.com/<YOUR_ORG>/engineering-docs/blob/main/runbooks/gpu-idle.md"
```

---

## ServiceMonitor (scrape config)

```yaml
# Add a scrape target for a new service
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: my-service
  namespace: monitoring     # ServiceMonitor lives in monitoring namespace
  labels:
    prometheus: kube-prometheus
spec:
  namespaceSelector:
    matchNames:
      - my-service           # Watch services in this namespace
  selector:
    matchLabels:
      app: my-service        # Match services with this label
  endpoints:
    - port: metrics          # Port name on the Service
      interval: 30s
      path: /metrics
      honorLabels: true
```

---

## AMP Remote Write Setup (Terraform)

```hcl
# modules/observability/main.tf — AMP workspace + remote_write IAM
resource "aws_prometheus_workspace" "main" {
  alias = "${var.name_prefix}-metrics"
  tags  = local.effective_tags
}

# IAM role for Prometheus remote_write (IRSA)
resource "aws_iam_role" "amp_ingest" {
  name = "${var.cluster_name}-amp-ingest"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Federated = var.oidc_provider_arn }
      Action    = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${var.oidc_provider_url}:sub" = "system:serviceaccount:monitoring:prometheus"
        }
      }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "amp_ingest" {
  role       = aws_iam_role.amp_ingest.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonPrometheusRemoteWriteAccess"
}
```

```yaml
# Prometheus remote_write config (in values.yaml)
prometheus:
  prometheusSpec:
    remoteWrite:
      - url: "https://aps-workspaces.us-east-1.amazonaws.com/workspaces/${AMP_WORKSPACE_ID}/api/v1/remote_write"
        sigv4:
          region: us-east-1
          roleArn: "arn:aws:iam::${AWS_ACCOUNT_ID}:role/${CLUSTER_NAME}-amp-ingest"
        queueConfig:
          maxSamplesPerSend: 1000
          maxShards: 200
          capacity: 2500
```

---

## Pushgateway (batch job metrics)

```python
# Python — push metrics from a batch job (Flyte task, cron, lab script)
from prometheus_client import CollectorRegistry, Gauge, push_to_gateway

registry = CollectorRegistry()

job_duration = Gauge(
    'batch_job_duration_seconds',
    'Duration of batch job',
    registry=registry,
)
records_processed = Gauge(
    'batch_job_records_processed_total',
    'Total records processed',
    registry=registry,
)

# ... do work ...
job_duration.set(elapsed_seconds)
records_processed.set(count)

push_to_gateway(
    'pushgateway.monitoring.svc.cluster.local:9091',
    job='my-batch-job',
    registry=registry,
)
```

---

## Debugging "No Data" in Grafana

```bash
# 1. Check Prometheus targets (is your service being scraped?)
kubectl port-forward -n monitoring svc/prometheus-operated 9090:9090 &
# Open http://localhost:9090/targets — find your service

# 2. Check ServiceMonitor exists and selector matches
kubectl get servicemonitor -n monitoring
kubectl describe servicemonitor my-service -n monitoring

# 3. Check the Service has a 'metrics' port matching the ServiceMonitor
kubectl get svc my-service -n my-service -o yaml | grep -A5 'ports:'

# 4. Check if metric exists at all
curl http://my-service.my-service.svc.cluster.local/metrics | grep my_metric_name

# 5. Check AMP remote_write errors
kubectl logs -n monitoring \
  $(kubectl get pods -n monitoring -l app.kubernetes.io/name=prometheus -o name | head -1) \
  | grep -i "remote_write\|error" | tail -20

# 6. Check cardinality (too many series = dropped)
# http://localhost:9090/tsdb-status → Top series by labelname
```

---

## Cardinality Audit

```promql
# Top 10 metrics by series count
topk(10, count by (__name__)({__name__=~".+"}))

# Labels with high cardinality (>1000 unique values = danger)
count by (label_name) (group by (label_name, label_value) ({__name__=~".+"}))
```

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| `user_id` or `pod_name` as metric label | Unbounded cardinality; OOM/crash | Use aggregations; keep labels bounded |
| `increase()` in alert rules | Edge cases with counter resets | Use `rate()` × window |
| `for: 0s` on alert rules | Fires on every scrape spike | Minimum `for: 5m` |
| No unit in metric name | Ambiguous (`requests` vs `requests_total`?) | Follow `{ns}_{subsystem}_{name}_{unit}` |
| Recording rule interval < scrape interval | Creates gaps; wastes CPU | Match recording to scrape interval (30s) |
| Scraping `/metrics` from every pod directly | No service discovery; breaks on pod restarts | Use `ServiceMonitor` / `PodMonitor` |
| AMP without `sigV4` in Grafana datasource | Unauthenticated; 403 errors | Always configure `sigV4Auth: true` + roleArn |
| Pushing to Pushgateway from long-running jobs | Stale metrics after job ends | Pushgateway for batch only; delete after push |

---

## axiom:trace

`axiom:trace work_item=devops-skills-01 spec=specs/34-Observability-And-Metrics.md plan= prompt=.opencode/skills/prometheus-shellops/SKILL.md evidence= doc= ops= commit=`
