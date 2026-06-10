---
name: grafana-shellops
description: >
  Grafana operations and dashboard-as-code patterns: provisioning dashboards via Helm/ConfigMap,
  dashboard JSON authoring, alert rule groups, Loki/Prometheus/Tempo/AMP datasources,
  folder organization, contact points, and Axiom traceability integration. Tuned for
  Grafana deployed via the infra-charts Helm chart pattern with Amazon Managed Prometheus.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/34-Observability-And-Metrics.md
  related_skills:
    - prometheus-shellops
    - sre-ops-axiom
    - alert-engineering-axiom
    - dashboard-design-axiom
    - kubernetes-shellops
tags:
  vertical: [devops, sre, observability, grafana]
  category: observability
  core: false
---

# Grafana — Axiom Integration Skill

> **"Dashboards are code. They live in Git, they deploy via Helm, they have reviews."**
> **"Every alert rule must have a contact point. Alerts firing into the void are noise."**
> **"Golden signals first: latency, traffic, errors, saturation. Everything else is secondary."**

This skill covers Grafana operations as practiced at Dexdat: dashboards provisioned
as Helm chart ConfigMaps (in `infra-charts/grafana/`), alert rules as Helm templates,
Amazon Managed Prometheus (AMP) and Loki as primary datasources, and ArgoCD for delivery.

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
- Adding a new Grafana dashboard to `infra-charts/grafana/dashboards/`
- Writing new alert rule groups in `infra-charts/grafana/templates/alerts/`
- Configuring a new datasource (AMP workspace, Loki, Tempo)
- Debugging a dashboard that shows "No data" or stale panels
- Adding Grafana contact points or notification policies
- Exporting a dashboard from the UI to commit it as code
- Provisioning Grafana in a new cluster via ArgoCD

---

## Non-Negotiables

1. **Dashboards live in Git.** Never create a permanent dashboard in the Grafana UI
   without exporting the JSON and committing it to `infra-charts/grafana/dashboards/`.
   UI-only dashboards are lost on the next Helm upgrade.

2. **Every alert rule group needs a contact point.** An alert firing with no receiver
   is silent failure. Every `alert-rule-group.yaml` must reference a named contact point.

3. **Use templating variables.** Hardcoded cluster names, namespaces, or job labels in
   dashboard JSON make dashboards non-portable. Use `$cluster`, `$namespace`, `$job`
   template variables.

4. **Test alert rules before deploying.** Use `mimirtool rules check` or the Grafana
   UI alerting preview before merging alert rule PRs.

5. **Dashboard UID is the stable identifier.** Set `uid` explicitly in dashboard JSON
   (e.g. `"uid": "golden-signals-v1"`). Let Grafana auto-generate UIDs and you'll get
   duplicates on every deploy.

---

## Repository Structure (infra-charts pattern)

```
infra-charts/grafana/
├── Chart.yaml
├── values.yaml                  # Grafana Helm values (datasources, auth, etc.)
├── values-test.yaml             # Test environment overrides
├── dashboards/                  # Raw dashboard JSON files
│   ├── golden_signals.json
│   ├── temporal_workflow_dashboard.json
│   ├── nvidia_dcgm_exporter.json
│   └── ...                      # 30+ dashboards
├── templates/
│   ├── _helpers.tpl
│   ├── grafana.yaml             # Main Grafana deployment (via upstream chart)
│   ├── datasources.yaml         # Datasource ConfigMaps
│   ├── folders.yaml             # Dashboard folder definitions
│   ├── ingress.yaml
│   ├── iam.yaml                 # IAM role for AMP query
│   ├── external-secret.yaml     # Auth secrets via ESO
│   ├── dashboards/              # ConfigMaps wrapping dashboard JSONs
│   │   ├── golden-signals-dashboard.yaml
│   │   └── ...
│   └── alerts/                  # Alert rule groups
│       ├── contact-points.yaml
│       ├── cpu-alert-rule-group.yaml
│       ├── memory-alert-rule-group.yaml
│       ├── node-alert-rule-group.yaml
│       ├── ml-alert-rules.yaml
│       └── ...
```

---

## Adding a Dashboard

### Step 1: Build in Grafana UI, then export

```
Grafana UI → Dashboard → Share → Export → Export for sharing externally
                                              → Download JSON
```

Critical: before exporting, set a stable UID in Dashboard Settings → General.

### Step 2: Add JSON to dashboards/

```bash
# Place the exported JSON
cp ~/Downloads/my-service-dashboard.json \
  infra-charts/grafana/dashboards/my_service_dashboard.json
```

### Step 3: Create the dashboard ConfigMap template

```yaml
# infra-charts/grafana/templates/dashboards/my-service-dashboard.yaml
{{- if .Values.myServiceDashboard.enabled }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "grafana.fullname" . }}-my-service-dashboard
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "grafana.labels" . | nindent 4 }}
    grafana_dashboard: "1"          # This label triggers Grafana sidecar to load it
  annotations:
    grafana_folder: {{ .Values.myServiceDashboard.folderRef | quote }}
data:
  my-service-dashboard.json: |
    {{ .Files.Get "dashboards/my_service_dashboard.json" | nindent 4 }}
{{- end }}
```

### Step 4: Add values entry

```yaml
# values.yaml addition
myServiceDashboard:
  enabled: true
  folderRef: provisioned-services   # Must match a folder defined in folders.yaml
```

---

## Dashboard JSON Best Practices

### Required fields in every dashboard

```json
{
  "uid": "my-service-v1",           // Stable — never auto-generated
  "title": "My Service",
  "tags": ["service", "production"],
  "timezone": "browser",
  "refresh": "30s",
  "time": { "from": "now-3h", "to": "now" },
  "templating": {
    "list": [
      {
        "name": "cluster",
        "type": "query",
        "datasource": { "type": "prometheus", "uid": "${datasource}" },
        "query": "label_values(up, cluster)",
        "refresh": 2,
        "includeAll": false
      },
      {
        "name": "namespace",
        "type": "query",
        "datasource": { "type": "prometheus", "uid": "${datasource}" },
        "query": "label_values(kube_pod_info{cluster=\"$cluster\"}, namespace)",
        "refresh": 2
      }
    ]
  }
}
```

### Golden Signals panel queries (Prometheus / AMP)

```promql
# Request rate (Traffic)
sum(rate(http_requests_total{cluster="$cluster", namespace="$namespace", job="$job"}[5m]))

# Error rate (Errors)
sum(rate(http_requests_total{cluster="$cluster", namespace="$namespace", job="$job", status=~"5.."}[5m]))
/ sum(rate(http_requests_total{cluster="$cluster", namespace="$namespace", job="$job"}[5m]))

# Latency p95 (Latency)
histogram_quantile(0.95,
  sum by (le) (rate(http_request_duration_seconds_bucket{
    cluster="$cluster", namespace="$namespace", job="$job"
  }[5m]))
)

# CPU saturation
sum(rate(container_cpu_usage_seconds_total{
  cluster="$cluster", namespace="$namespace", container!=""
}[5m]))
/ sum(kube_pod_container_resource_limits{
  cluster="$cluster", namespace="$namespace", resource="cpu"
})
```

---

## Alert Rule Groups (Helm template pattern)

```yaml
# templates/alerts/my-service-alert-rule-group.yaml
{{- if .Values.myServiceAlerts.enabled }}
apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaAlertRuleGroup
metadata:
  name: my-service-alerts
  namespace: {{ .Release.Namespace }}
spec:
  instanceSelector:
    matchLabels:
      dashboards: grafana
  folderRef: provisioned-alerts
  rules:
    - uid: my-service-high-error-rate
      title: "My Service — High Error Rate"
      condition: C
      for: "5m"
      labels:
        severity: warning
        team: platform
      annotations:
        summary: "Error rate above 5% for {{ $labels.namespace }}"
        runbook_url: "https://github.com/<YOUR_ORG>/engineering-docs/runbooks/my-service.md"
        # ^^^ Every alert MUST have a runbook_url
      data:
        - refId: A
          relativeTimeRange: { from: 300, to: 0 }
          datasourceUid: "${datasource_uid}"
          model:
            expr: |
              sum(rate(http_requests_total{namespace="{{ $labels.namespace }}", status=~"5.."}[5m]))
              / sum(rate(http_requests_total{namespace="{{ $labels.namespace }}"}[5m]))
            intervalMs: 1000
            maxDataPoints: 43200
        - refId: C
          relativeTimeRange: { from: 300, to: 0 }
          datasourceUid: "-100"
          model:
            conditions:
              - evaluator: { params: [0.05], type: gt }
                operator: { type: and }
                query: { params: [A] }
                reducer: { type: last }
{{- end }}
```

---

## Datasource Configuration (AMP + Loki)

```yaml
# templates/datasources.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "grafana.fullname" . }}-datasources
  labels:
    grafana_datasource: "1"
data:
  datasources.yaml: |
    apiVersion: 1
    datasources:
      # Amazon Managed Prometheus (primary metrics)
      - name: AMP
        type: prometheus
        uid: amp-primary
        url: {{ .Values.grafanaInstance.metrics.url }}
        access: proxy
        isDefault: true
        jsonData:
          sigV4Auth: true
          sigV4Region: us-east-1
          sigV4AssumeRoleArn: {{ .Values.grafanaInstance.metrics.roleArn }}
          httpMethod: POST

      # Loki (logs)
      - name: Loki
        type: loki
        uid: loki-primary
        url: http://loki-gateway.monitoring.svc.cluster.local
        access: proxy
        jsonData:
          maxLines: 5000

      # Tempo (traces)
      - name: Tempo
        type: tempo
        uid: tempo-primary
        url: http://tempo.monitoring.svc.cluster.local:3100
        access: proxy
        jsonData:
          tracesToLogsV2:
            datasourceUid: loki-primary
          serviceMap:
            datasourceUid: amp-primary
```

---

## Common Debugging

```bash
# Dashboard not loading — check sidecar logs
kubectl logs -n monitoring \
  $(kubectl get pods -n monitoring -l app.kubernetes.io/name=grafana -o name | head -1) \
  -c grafana-sc-dashboard

# Alert not firing — check alert rule evaluation
# Grafana UI: Alerting → Alert Rules → [rule] → Preview

# Check Grafana pod for datasource errors
kubectl logs -n monitoring \
  $(kubectl get pods -n monitoring -l app.kubernetes.io/name=grafana -o name | head -1) \
  | grep -i "datasource\|error" | tail -20

# Export all dashboards via API (for backup)
GRAFANA_URL="https://grafana.houston.dexdat.ai"
curl -s "$GRAFANA_URL/api/search?type=dash-db" \
  -H "Authorization: Bearer $GRAFANA_TOKEN" \
  | jq '.[].uid' -r | while read uid; do
    curl -s "$GRAFANA_URL/api/dashboards/uid/$uid" \
      -H "Authorization: Bearer $GRAFANA_TOKEN" \
      > "backup/$uid.json"
  done
```

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| Dashboards created only in UI | Lost on next Helm upgrade | Export JSON → commit to `infra-charts/grafana/dashboards/` |
| Hardcoded cluster/namespace in queries | Dashboard breaks on other clusters | Template variables: `$cluster`, `$namespace` |
| Alert with no `runbook_url` annotation | On-call has no guidance | Always add `runbook_url` pointing to engineering-docs |
| Auto-generated dashboard UIDs | Duplicates on redeploy | Set explicit `uid` in dashboard JSON |
| Alert rule `for: 0s` | Fires on transient spikes | Minimum `for: 5m` for non-critical alerts |
| Loki query without index filters | Full scan; slow + expensive | Always include `{namespace="$namespace", cluster="$cluster"}` |

---

## axiom:trace

`axiom:trace work_item=devops-skills-01 spec=specs/34-Observability-And-Metrics.md plan= prompt=.opencode/skills/grafana-shellops/SKILL.md evidence= doc= ops= commit=`
