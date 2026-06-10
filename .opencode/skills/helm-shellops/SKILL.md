---
name: helm-shellops
description: >
  Helm chart authoring and operations: chart structure, values management, templating
  patterns, release lifecycle (install/upgrade/rollback), environment overlays, secrets
  handling (External Secrets, Sealed Secrets, Vault Agent), and Axiom traceability
  integration. Load this skill when writing, reviewing, or deploying Helm charts.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/00-PRD.md
  related_skills:
    - kubernetes-shellops
    - argocd-shellops
    - vault-shellops
    - version-pinning-axiom
    - sre-ops-axiom
tags:
  vertical: [devops, kubernetes, helm]
  category: cloud-operations
  core: false
---

# Helm — Axiom Integration Skill

> **"Chart versioning is a contract. Bump it when the interface changes."**
> **"Values files are environment configs, not secrets stores."**
> **"Never `helm upgrade --force` your way out of a bad state."**

This skill covers Helm chart authoring, release operations, and deployment patterns
for Axiom workflows. It includes the critical patterns for multi-environment values,
secrets handling, CI/CD integration, and traceable evidence capture.

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
- Writing a new Helm chart from scratch
- Adding or modifying templates in an existing chart
- Managing multi-environment values files (dev/staging/production)
- Running `helm install`, `helm upgrade`, or `helm rollback`
- Debugging Helm template rendering errors
- Setting up Helm in ArgoCD or GitHub Actions
- Handling secrets in Helm deployments
- Performing chart linting and testing (`helm lint`, `helm test`)

---

## Non-Negotiables

1. **Pin chart versions in dependencies.** `Chart.lock` is the equivalent of
   `package-lock.json`. Commit it. Never rely on floating `*` versions.

2. **Values files are not secrets stores.** Never put passwords, API keys, or tokens
   in `values.yaml` or any `values-*.yaml` committed to git. Use External Secrets,
   Sealed Secrets, or Vault Agent Injector.

3. **`helm diff` before every upgrade in production.** Use the `helm-diff` plugin to
   see exactly what will change. Capture the diff as evidence.

4. **`helm rollback` must be tested.** Every chart should have a documented rollback
   procedure. Untested rollbacks are not rollbacks — they are guesses.

5. **Lint before push.** `helm lint` + `helm template | kubectl apply --dry-run=server`
   catches most template errors before they hit a cluster.

---

## Chart Structure

### Standard Layout

```
charts/my-app/
├── Chart.yaml              # Chart metadata (name, version, appVersion, dependencies)
├── Chart.lock              # Pinned dependency versions — COMMIT THIS
├── values.yaml             # Default values (documented, no secrets)
├── values-dev.yaml         # Dev overrides
├── values-staging.yaml     # Staging overrides
├── values-production.yaml  # Production overrides
├── templates/
│   ├── _helpers.tpl        # Named templates / partials
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── ingress.yaml
│   ├── configmap.yaml
│   ├── serviceaccount.yaml
│   ├── hpa.yaml
│   ├── pdb.yaml            # PodDisruptionBudget
│   ├── NOTES.txt           # Post-install instructions
│   └── tests/
│       └── test-connection.yaml
└── .helmignore
```

### Chart.yaml

```yaml
apiVersion: v2
name: my-app
description: Application serving the MyApp API
type: application
version: 1.3.0           # Chart version — bump on any chart change
appVersion: "2.1.5"      # Application version — the image tag being deployed

maintainers:
  - name: Platform Team
    email: platform@dexdat.ai

dependencies:
  - name: postgresql
    version: "~14.0"       # Pin minor; allow patch updates
    repository: https://charts.bitnami.com/bitnami
    condition: postgresql.enabled   # Toggle via values
```

---

## Values File Patterns

### values.yaml (defaults — fully documented)

```yaml
# values.yaml — default values, all documented

replicaCount: 1

image:
  repository: my-registry/my-app
  tag: ""                  # Overridden by CI with the actual git SHA
  pullPolicy: IfNotPresent

# Resource requests and limits — always set, never omit
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi

service:
  type: ClusterIP
  port: 8080

ingress:
  enabled: false
  className: nginx
  annotations: {}
  hosts: []
  tls: []

autoscaling:
  enabled: false
  minReplicas: 1
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

podDisruptionBudget:
  enabled: true
  minAvailable: 1

serviceAccount:
  create: true
  annotations: {}
  name: ""

# Secrets injected via External Secrets Operator — not stored in values
# externalSecrets:
#   secretStoreRef: my-secret-store
#   data: ...

# Database (disabled by default, enabled per environment)
postgresql:
  enabled: false
```

### values-production.yaml (environment override — no secrets)

```yaml
# values-production.yaml

replicaCount: 3

image:
  pullPolicy: Always

resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 2000m
    memory: 2Gi

autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 20

ingress:
  enabled: true
  hosts:
    - host: api.dexdat.ai
      paths:
        - path: /
          pathType: Prefix
```

---

## Template Best Practices

### _helpers.tpl (named templates)

```
{{/*
Expand the name of the chart.
*/}}
{{- define "my-app.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "my-app.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels — applied to every resource
*/}}
{{- define "my-app.labels" -}}
helm.sh/chart: {{ include "my-app.chart" . }}
{{ include "my-app.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
axiom.io/work-item: {{ .Values.codeopsWorkItem | default "unknown" | quote }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "my-app.selectorLabels" -}}
app.kubernetes.io/name: {{ include "my-app.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
```

### Deployment template (correct patterns)

```yaml
# templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "my-app.fullname" . }}
  labels:
    {{- include "my-app.labels" . | nindent 4 }}
  annotations:
    # Checksum forces pod restart when ConfigMap changes
    checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
spec:
  {{- if not .Values.autoscaling.enabled }}
  replicas: {{ .Values.replicaCount }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "my-app.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "my-app.selectorLabels" . | nindent 8 }}
    spec:
      serviceAccountName: {{ include "my-app.serviceAccountName" . }}
      securityContext:
        runAsNonRoot: true
        runAsUser: 10001
      containers:
        - name: {{ .Chart.Name }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: http
              containerPort: 8080
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
          readinessProbe:
            httpGet:
              path: /ready
              port: http
          livenessProbe:
            httpGet:
              path: /health
              port: http
```

---

## Release Operations

### Install

```bash
# Add repo (if using external chart)
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

# Install with environment values
helm install my-app ./charts/my-app \
  -n my-app \
  --create-namespace \
  -f charts/my-app/values-production.yaml \
  --set image.tag="v2.1.5" \
  --set codeopsWorkItem="${WORK_ITEM_ID}" \
  --wait \
  --timeout 5m \
  --output json | tee .memory-bank/work-items/${WORK_ITEM_ID}/helm-install.json
```

### Upgrade (with diff evidence)

```bash
# Install diff plugin if not present
helm plugin install https://github.com/databus23/helm-diff

# 1. Show diff (evidence)
helm diff upgrade my-app ./charts/my-app \
  -n my-app \
  -f charts/my-app/values-production.yaml \
  --set image.tag="v2.1.6" \
  2>&1 | tee .memory-bank/work-items/${WORK_ITEM_ID}/helm-diff.txt

# 2. Review diff output — check for unexpected changes
# Abort if there are deletions you didn't expect

# 3. Apply upgrade
helm upgrade my-app ./charts/my-app \
  -n my-app \
  -f charts/my-app/values-production.yaml \
  --set image.tag="v2.1.6" \
  --set codeopsWorkItem="${WORK_ITEM_ID}" \
  --wait \
  --timeout 5m \
  --output json | tee .memory-bank/work-items/${WORK_ITEM_ID}/helm-upgrade.json

# 4. Verify rollout
kubectl rollout status deployment/my-app -n my-app
```

### Rollback

```bash
# View release history
helm history my-app -n my-app --output table

# Roll back to previous revision
helm rollback my-app -n my-app --wait --timeout 5m

# Roll back to specific revision
helm rollback my-app 3 -n my-app --wait --timeout 5m

# Verify rollback
helm status my-app -n my-app
kubectl get pods -n my-app
```

---

## Secrets Handling

### External Secrets Operator (preferred)

```yaml
# templates/externalsecret.yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: {{ include "my-app.fullname" . }}-secrets
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: {{ .Values.externalSecrets.secretStoreRef }}
    kind: SecretStore
  target:
    name: {{ include "my-app.fullname" . }}-secrets
    creationPolicy: Owner
  data:
    - secretKey: db-password
      remoteRef:
        key: my-app/production
        property: db_password
    - secretKey: api-key
      remoteRef:
        key: my-app/production
        property: api_key
```

### Vault Agent Injector (alternative)

```yaml
# Annotation-based secret injection
podAnnotations:
  vault.hashicorp.com/agent-inject: "true"
  vault.hashicorp.com/role: "my-app"
  vault.hashicorp.com/agent-inject-secret-config: "secret/data/my-app/production"
  vault.hashicorp.com/agent-inject-template-config: |
    {{`{{- with secret "secret/data/my-app/production" -}}
    export DB_PASSWORD="{{ .Data.data.db_password }}"
    {{- end }}`}}
```

---

## Linting and Testing

```bash
# Lint chart
helm lint ./charts/my-app \
  -f charts/my-app/values-production.yaml \
  --strict

# Render templates (dry-run — catches template errors)
helm template my-app ./charts/my-app \
  -f charts/my-app/values-production.yaml \
  --set image.tag="v2.1.5" \
  | kubectl apply --dry-run=server -f -

# Run chart tests (requires `templates/tests/`)
helm test my-app -n my-app
```

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| `version: "*"` in Chart.yaml dependencies | Breaks deterministic builds | Exact or `~>` pinned versions + `Chart.lock` |
| Passwords in `values.yaml` | Git history exposes secrets | External Secrets / Vault Agent |
| `helm upgrade --force` | Corrupts state; causes data loss risk | Fix root cause; `helm rollback` first |
| No `Chart.lock` committed | Different deps on each install | `helm dependency update` + commit lock |
| `{{ .Values.foo }}` without `default` | Null reference errors | `{{ .Values.foo | default "fallback" }}` |
| No `readinessProbe` in template | Traffic to not-ready pods | Always include probe templates |
| No PodDisruptionBudget | Zero-disruption upgrades fail | `pdb.yaml` with `minAvailable: 1` |
| `helm upgrade` without `--wait` | "Done" before pods are ready | Always `--wait --timeout Nm` |

---

## axiom:trace

`axiom:trace work_item=devops-skills-01 spec=specs/00-PRD.md plan= prompt=.opencode/skills/helm-shellops/SKILL.md evidence= doc= ops= commit=`
