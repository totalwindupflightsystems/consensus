---
name: argocd-shellops
description: >
  ArgoCD GitOps patterns: application definitions, App of Apps, sync policies, health
  checks, RBAC, multi-cluster management, progressive delivery with Argo Rollouts,
  image updater, and Axiom traceability integration. Load this skill when configuring
  or operating ArgoCD for GitOps-style Kubernetes deployments.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/00-PRD.md
  related_skills:
    - helm-shellops
    - kubernetes-shellops
    - github-actions-shellops
    - sre-ops-axiom
tags:
  vertical: [devops, gitops, kubernetes, argocd]
  category: cloud-operations
  core: false
---

# ArgoCD — Axiom Integration Skill

> **"Git is the single source of truth. ArgoCD enforces it."**
> **"AutoSync without SyncWindows in production is an incident waiting to happen."**
> **"Every Application needs a health check — OutOfSync is not the same as Degraded."**

This skill covers GitOps workflows with ArgoCD for Axiom-managed Kubernetes infrastructure.
It covers the full application lifecycle from App of Apps bootstrapping through progressive
delivery with Argo Rollouts, with traceability at every step.

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
- Setting up a new ArgoCD Application or ApplicationSet
- Configuring sync policies (auto-sync, self-heal, prune)
- Debugging OutOfSync, Degraded, or Progressing application states
- Implementing App of Apps pattern for cluster bootstrapping
- Configuring RBAC and Projects for multi-team access
- Setting up SyncWindows to prevent off-hours auto-deploy
- Implementing progressive delivery with Argo Rollouts
- Wiring GitHub Actions or other CI to ArgoCD image updates

---

## Non-Negotiables

1. **AutoSync + AutoPrune requires SyncWindows in production.** Never enable
   `automated: {prune: true, selfHeal: true}` without SyncWindows that restrict
   auto-sync to business hours. An accidental Git commit must not auto-destroy prod.

2. **Every Application lives in an ArgoCD Project.** Never use the `default` project
   in production. Projects enforce source repo allowlists, destination cluster/namespace
   restrictions, and RBAC boundaries.

3. **Health check required for every custom resource.** ArgoCD doesn't know how healthy
   your CRDs are unless you define `resource.customizations.health.*`. Missing = always Healthy.

4. **`argocd app sync` — not `kubectl apply`.** When you need to force a sync, use
   the ArgoCD CLI/UI. Direct `kubectl apply` creates drift that ArgoCD will fight.

5. **Capture sync history for evidence.** `argocd app history` and `argocd app get`
   are your audit trail. Pipe to JSON for Axiom evidence bundles.

---

## Application Definition

### Single Application

```yaml
# apps/my-app.yaml — stored in the GitOps repo
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
  namespace: argocd
  labels:
    app.kubernetes.io/part-of: platform
    env: production
  annotations:
    # Axiom trace
    axiom.io/work-item: "devops-skills-01"
    notifications.argoproj.io/subscribe.on-sync-failed.slack: ops-alerts
    notifications.argoproj.io/subscribe.on-degraded.slack: ops-alerts
  finalizers:
    - resources-finalizer.argocd.argoproj.io  # Ensures cleanup on delete
spec:
  project: platform                            # Never 'default'

  source:
    repoURL: https://github.com/<YOUR_ORG>/infra
    targetRevision: HEAD
    path: charts/my-app
    helm:
      releaseName: my-app
      valueFiles:
        - values.yaml
        - values-production.yaml
      parameters:
        - name: image.tag
          value: "2.1.5"             # Pinned; updated by Image Updater or CI

  destination:
    server: https://kubernetes.default.svc   # In-cluster
    namespace: my-app

  syncPolicy:
    automated:
      prune: true                    # Remove resources deleted from Git
      selfHeal: true                 # Revert manual kubectl edits
    syncOptions:
      - CreateNamespace=true
      - PrunePropagationPolicy=foreground
      - ApplyOutOfSyncOnly=true      # Only sync changed resources
    retry:
      limit: 3
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m

  revisionHistoryLimit: 10           # Keep last 10 syncs
```

---

## App of Apps Pattern (cluster bootstrapping)

The App of Apps pattern lets a single ArgoCD Application manage all other Applications.
It is the standard way to bootstrap a cluster from a Git repo.

```
gitops-repo/
├── apps/
│   ├── root-app.yaml           # The bootstrap App of Apps
│   ├── my-app.yaml
│   ├── monitoring.yaml
│   ├── cert-manager.yaml
│   └── ingress-nginx.yaml
├── charts/
│   ├── my-app/
│   └── monitoring/
└── cluster-config/
    └── namespaces.yaml
```

```yaml
# apps/root-app.yaml — applied ONCE manually to bootstrap the cluster
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app
  namespace: argocd
spec:
  project: default                   # Bootstrap only; other apps use Projects
  source:
    repoURL: https://github.com/<YOUR_ORG>/infra
    targetRevision: HEAD
    path: apps                       # Points to the apps/ directory
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

```bash
# One-time manual bootstrap
kubectl apply -f apps/root-app.yaml

# After this, all other apps are managed by ArgoCD
argocd app list
```

---

## ApplicationSet (dynamic app generation)

ApplicationSet generates multiple Applications from a single template — perfect for
multi-cluster or multi-environment deployments.

```yaml
# appsets/my-app-environments.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: my-app-environments
  namespace: argocd
spec:
  generators:
    - list:
        elements:
          - cluster: dev
            url: https://k8s-dev.internal
            values_file: values-dev.yaml
          - cluster: staging
            url: https://k8s-staging.internal
            values_file: values-staging.yaml
          - cluster: production
            url: https://k8s-prod.internal
            values_file: values-production.yaml

  template:
    metadata:
      name: "my-app-{{cluster}}"
      namespace: argocd
      labels:
        env: "{{cluster}}"
    spec:
      project: platform
      source:
        repoURL: https://github.com/<YOUR_ORG>/infra
        targetRevision: HEAD
        path: charts/my-app
        helm:
          valueFiles:
            - values.yaml
            - "{{values_file}}"
      destination:
        server: "{{url}}"
        namespace: my-app
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
```

---

## Projects and RBAC

### Project Definition

```yaml
# projects/platform.yaml
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: platform
  namespace: argocd
spec:
  description: Platform infrastructure applications

  # Restrict to specific source repos
  sourceRepos:
    - https://github.com/<YOUR_ORG>/infra
    - https://charts.bitnami.com/bitnami
    - https://helm.nginx.org/stable

  # Restrict to specific destinations
  destinations:
    - namespace: "my-app"
      server: https://kubernetes.default.svc
    - namespace: "monitoring"
      server: https://kubernetes.default.svc

  # Restrict which resource types can be deployed
  clusterResourceWhitelist:
    - group: ''
      kind: Namespace
    - group: 'rbac.authorization.k8s.io'
      kind: ClusterRole
    - group: 'rbac.authorization.k8s.io'
      kind: ClusterRoleBinding

  # Orphaned resources (not managed by Argo) trigger warnings
  orphanedResources:
    warn: true
```

### RBAC Config

```yaml
# argocd-rbac-cm ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-rbac-cm
  namespace: argocd
data:
  policy.default: role:readonly          # Default is read-only
  policy.csv: |
    # Platform team can sync platform project
    p, role:platform-team, applications, sync, platform/*, allow
    p, role:platform-team, applications, get, platform/*, allow

    # Ops can sync all
    p, role:ops, applications, *, */*, allow

    # Bind groups to roles (using SSO/OIDC groups)
    g, fl97inc:platform-team, role:platform-team
    g, fl97inc:ops, role:ops
```

---

## SyncWindows (prevent off-hours auto-sync in production)

```yaml
# In AppProject spec:
syncWindows:
  - kind: allow
    schedule: "0 9 * * 1-5"     # Monday-Friday 9am UTC
    duration: 8h
    applications:
      - "my-app-production"
    clusters:
      - https://k8s-prod.internal
    manualSync: true             # Allow manual sync any time

  - kind: deny
    schedule: "0 17 * * 1-5"    # Block weeknights
    duration: 16h
    applications:
      - "*-production"
    manualSync: false            # Require explicit override for emergency deploys
```

---

## CLI Operations (Evidence-Producing)

```bash
# Current app state
argocd app get my-app --output json \
  | tee .memory-bank/work-items/${WORK_ITEM_ID}/argocd-app-state.json

# Sync history (audit trail)
argocd app history my-app --output json \
  | tee .memory-bank/work-items/${WORK_ITEM_ID}/argocd-history.json

# Manual sync (when AutoSync is off or SyncWindow blocks)
argocd app sync my-app \
  --prune \
  --timeout 300 \
  --output json | tee .memory-bank/work-items/${WORK_ITEM_ID}/argocd-sync.json

# Force a hard refresh (re-fetches from Git, re-evaluates diff)
argocd app get my-app --refresh --hard-refresh

# Get diff (what would change if synced now)
argocd app diff my-app

# List all apps and their sync status
argocd app list --output json \
  | jq '.[] | {name:.metadata.name, sync:.status.sync.status, health:.status.health.status}'

# Watch sync progress
argocd app wait my-app \
  --sync \
  --health \
  --timeout 300
```

---

## Image Updater (CI → ArgoCD image tag updates)

```yaml
# Annotation-driven image update (writes tag back to Git)
annotations:
  argocd-image-updater.argoproj.io/image-list: "my-app=my-registry/my-app"
  argocd-image-updater.argoproj.io/my-app.update-strategy: digest
  argocd-image-updater.argoproj.io/my-app.helm.image-name: image.repository
  argocd-image-updater.argoproj.io/my-app.helm.image-tag: image.tag
  argocd-image-updater.argoproj.io/write-back-method: git
```

---

## Debugging Common Issues

### OutOfSync After Correct Deploy

```bash
# Check what ArgoCD thinks is different
argocd app diff my-app

# Common cause: labels injected by Helm that ArgoCD ignores by default
# Fix in argocd-cm ConfigMap:
# resource.compareoptions: |
#   ignoreAggregatedRoles: true
```

### Progressing (Never Becomes Healthy)

```bash
# Check sync status
argocd app get my-app

# Check pod events directly
kubectl get events -n my-app --sort-by='.lastTimestamp' | tail -20

# Check if health check is defined for CRD
argocd app get my-app -o yaml | grep -A5 health
```

### Sync Stuck (Webhook Trigger)

```bash
# Force refresh
argocd app get my-app --refresh

# Check webhook is configured in GitHub repo settings
# ArgoCD URL: https://<argocd-host>/api/webhook
```

---

## Axiom Integration

Every deploy to production via ArgoCD produces an evidence trail:

```bash
# Pre-sync state
argocd app get my-app --output json \
  > .memory-bank/work-items/${WORK_ITEM_ID}/pre-sync-state.json

# Sync
argocd app sync my-app --prune --timeout 300

# Post-sync state
argocd app get my-app --output json \
  > .memory-bank/work-items/${WORK_ITEM_ID}/post-sync-state.json

# Add trace marker
echo "axiom:trace work_item=${WORK_ITEM_ID} ops=.axiom/runbooks/argocd-deploy.md evidence=.memory-bank/work-items/${WORK_ITEM_ID}/post-sync-state.json" \
  >> .memory-bank/work-items/${WORK_ITEM_ID}/verification.md
```

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| AutoSync + AutoPrune without SyncWindows in prod | Any bad commit auto-destroys prod | Add SyncWindows; restrict to business hours |
| Using `default` project | No source/destination restrictions | Dedicated Project per team/domain |
| `argocd app delete --cascade` without review | Deletes all K8s resources | Check resources first; use dry-run |
| Direct `kubectl apply` on Argo-managed resources | Creates drift; Argo reverts it | Always route changes through Git |
| No notifications configured | Silent failures | Configure Slack/PagerDuty on sync-failed + degraded |
| Storing secrets in the GitOps repo | Plaintext secrets in git history | External Secrets + Sealed Secrets |
| No `revisionHistoryLimit` | Unlimited history; ArgoCD DB bloat | Set `revisionHistoryLimit: 10` |
| Missing health checks for CRDs | CRDs always appear "Healthy" | Define `resource.customizations.health` |

---

## axiom:trace

`axiom:trace work_item=devops-skills-01 spec=specs/00-PRD.md plan= prompt=.opencode/skills/argocd-shellops/SKILL.md evidence= doc= ops= commit=`
