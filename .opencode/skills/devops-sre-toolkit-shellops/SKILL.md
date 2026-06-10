---
name: devops-sre-toolkit-shellops
description: >
  Parent routing skill for the Dexdat DevOps/SRE toolkit. Routes to the right
  child skill based on the task at hand: Terraform IaC, Kubernetes operations, Helm
  charts, ArgoCD GitOps, GitHub Actions CI/CD, Grafana dashboards, Prometheus alerting,
  Docker containers, Karpenter autoscaling, Temporal workflows, Flyte ML pipelines,
  External Secrets, and AWS CLI operations. Load this skill first when the task spans
  multiple DevOps domains or when you are unsure which specific skill to use.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/00-PRD.md
  related_skills:
    - aws-cli-shellops
    - kubernetes-shellops
    - terraform-shellops
    - helm-shellops
    - argocd-shellops
    - github-actions-shellops
    - grafana-shellops
    - prometheus-shellops
    - docker-shellops
    - karpenter-shellops
    - temporal-shellops
    - flyte-shellops
    - external-secrets-shellops
tags:
  vertical: [devops, sre, routing]
  category: meta
  core: false
---

# DevOps/SRE Toolkit — Parent Routing Skill

This skill is the entry point for all DevOps and SRE work at Dexdat. Load it
when you need a skill but aren't sure which one applies, or when a task spans multiple
infrastructure domains. It will route you to the right child skill.

All child skills use the `-shellops` suffix and are tuned to the actual Dexdat
infrastructure: **4-account AWS org** (portal / console / lab / inference), **EKS + Cilium
+ Karpenter**, **ArgoCD App of Apps**, **Flyte + Temporal** for workflows, **AMP + Grafana**
for observability, and **External Secrets + AWS SM** for secrets.

---

## Routing Table

| Task | Load This Skill |
|---|---|
| AWS CLI commands, IAM, EC2, S3, EKS cluster auth, RDS snapshots | `aws-cli-shellops` |
| kubectl, K8s manifests, RBAC, debugging pods, network policies | `kubernetes-shellops` |
| Terraform modules, `terraform plan/apply`, state, `commercial-infra` | `terraform-shellops` |
| Helm chart authoring, `helm upgrade`, values files, `infra-charts` | `helm-shellops` |
| ArgoCD Applications, App of Apps, sync policies, multi-cluster | `argocd-shellops` |
| GitHub Actions workflows, OIDC, ECR push, environment gates | `github-actions-shellops` |
| Grafana dashboards, alert rule groups, datasources, `infra-charts/grafana` | `grafana-shellops` |
| PromQL, PrometheusRule, AMP remote_write, recording rules, DCGM | `prometheus-shellops` |
| Dockerfile, multi-stage builds, ECR, BuildKit cluster | `docker-shellops` |
| Karpenter NodePools, GPU nodes, spot instances, consolidation | `karpenter-shellops` |
| Temporal workflows, activities, workers, schedules | `temporal-shellops` |
| Flyte workflows, tasks, GPU tasks, FlyteRemote, scientific pipelines | `flyte-shellops` |
| ExternalSecret, SecretStore, AWS Secrets Manager, ESO | `external-secrets-shellops` |

---

## Infrastructure Overview

### 4-Account AWS Topology

```
dexdat-portal-prod    → Customer onboarding portal (EKS + Aurora)
dexdat-console-prod   → Main platform (EKS + Envoy + 4 Aurora clusters + PrivateLink consumers)
dexdat-labs-prod      → Lab instruments (EKS + Direct Connect + PrivateLink providers)
dexdat-inference-prod → GPU inference (EKS + p4d/p5 + PrivateLink provider)
```

Cross-account: PrivateLink only. Console → Lab (NATS, Fila, ISS, SampleTracker).
Console → Inference (LLM model service). No internet traversal between accounts.

### EKS Cluster Standard Stack

Every cluster runs this stack, provisioned via `commercial-infra/` Terraform modules:

| Layer | Tool | Terraform Module |
|---|---|---|
| Network | VPC (5-tier subnets: public/endpoints/app/db/gpu) | `modules/vpc` |
| Cluster | EKS (KMS secrets encryption, all log types) | `modules/eks` |
| CNI | Cilium (eBPF, replaces vpc-cni) | `modules/cilium` |
| Node autoscaling | Karpenter + NodePools | `modules/karpenter` (post-cilium) |
| GitOps | ArgoCD | `modules/argocd` |
| Observability | AMP workspace + IAM | `modules/observability` |
| Secrets | External Secrets Operator | `infra-charts/external-secrets` |

### ArgoCD App of Apps Clusters

| Cluster | Account | Purpose |
|---|---|---|
| `houston` | internal | Main internal platform |
| `buzz`, `chewie`, `luke`, `ripley`, `solo`, `ride` | internal | Internal services |
| `lambda-128x`, `lambda-512x`, `lambda-512x-south` | Lambda Labs | GPU training (H100) |
| `playground` | internal | Dev/test |
| console cluster | dexdat-console-prod | Commercial platform |
| lab cluster | dexdat-labs-prod | Commercial lab |
| inference cluster | dexdat-inference-prod | Commercial GPU inference |
| portal cluster | dexdat-portal-prod | Commercial onboarding |

### Key Helm Charts (`infra-charts`)

```
grafana/           → 30+ dashboards + alert rules (golden signals, GPU, Temporal, Flyte)
prometheus/        → PrometheusRule + AlertmanagerConfig + remote_write to AMP
loki/              → Log aggregation (S3 backend)
tempo/             → Distributed tracing (S3 backend)
temporal/          → Workflow server (Aurora aurora-pg backend)
flyte-core/        → ML workflow platform (S3 + Aurora backend)
karpenter/         → NodePool + EC2NodeClass config
external-secrets/  → ESO + ClusterSecretStore
external-secrets-store/ → Per-namespace SecretStore + IAM
kong/              → API gateway (OPA/Lua policies)
keycloak/          → SSO/OIDC identity provider
crossplane/        → K8s-native infra provisioning
coder/             → Coder.com workspace platform
github-arc/        → GitHub Actions self-hosted runners on K8s
buildkit-cluster/  → Distributed Docker image builds
nats/              → NATS JetStream messaging
```

---

## Common Workflow Patterns

### Add a new service to a cluster

1. `docker-shellops` — write Dockerfile, build + push to ECR
2. `helm-shellops` — create Helm chart in the service's repo (or `infra-charts`)
3. `external-secrets-shellops` — add ExternalSecret for service secrets
4. `argocd-shellops` — add Application template to `infra-argocd-apps/templates/`
5. `prometheus-shellops` — add ServiceMonitor + PrometheusRule
6. `grafana-shellops` — add dashboard JSON + alert rule group
7. `github-actions-shellops` — add CI workflow for build + deploy

### Provision a new EKS cluster

1. `terraform-shellops` — copy closest `commercial-infra/{account}/modules/` pattern, apply phases:
   - Phase 1: `vpc` → `eks` → `cilium`
   - Phase 2: `post-cilium` (Karpenter, GitHub OIDC, node groups)
   - Phase 3: ArgoCD bootstrap (`modules/argocd`)
2. `argocd-shellops` — add cluster to `infra-argocd-apps/clusters/`
3. `karpenter-shellops` — configure NodePools in `infra-charts/karpenter/values-<cluster>.yaml`

### Debug a production incident

1. `kubernetes-shellops` — `kubectl get events`, pod logs, describe
2. `prometheus-shellops` — PromQL to quantify impact (error rate, latency)
3. `grafana-shellops` — check golden signals dashboard
4. `temporal-shellops` / `flyte-shellops` — if workflow-related
5. `aws-cli-shellops` — check CloudWatch alarms, RDS, EKS nodegroups

### Rotate a secret

1. `aws-cli-shellops` — `aws secretsmanager put-secret-value`
2. `external-secrets-shellops` — `kubectl annotate externalsecret force-sync=$(date +%s)`
3. Verify: `kubectl get secret <name> -o jsonpath='{.data}' | jq 'keys'`

---

## axiom:trace

`axiom:trace work_item=devops-skills-01 spec=specs/00-PRD.md plan= prompt=.opencode/skills/devops-sre-toolkit-shellops/SKILL.md evidence= doc= ops= commit=`
