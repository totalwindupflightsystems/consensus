---
name: karpenter-shellops
description: >
  Karpenter node autoscaler patterns for EKS: NodePool and EC2NodeClass authoring,
  GPU node pools (g6e L40S, H100), spot instance configuration, workload taints/tolerations,
  consolidation policies, interruption handling, and Terraform module integration
  (commercial-infra/modules/karpenter). Tuned for Dexdat multi-pool EKS clusters.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/00-PRD.md
  related_skills:
    - kubernetes-shellops
    - terraform-shellops
    - helm-shellops
    - aws-cli-shellops
tags:
  vertical: [devops, kubernetes, autoscaling, gpu]
  category: cloud-operations
  core: false
---

# Karpenter — Axiom Integration Skill

> **"Karpenter provisions exactly the node the workload needs — not a generic node that mostly fits."**
> **"GPU nodes need taints. Without taints, CPU workloads land on $15/hr GPU nodes."**
> **"Consolidation is free cost savings. Enable it on every non-GPU pool."**

This skill covers Karpenter as deployed at Dexdat via `commercial-infra/modules/karpenter`
and `infra-charts/karpenter`. Multiple specialized NodePools handle CPU workloads, GPU
training (g6e/L40S), GitHub ARC runners, BuildKit, Axiom, and Loki.

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
- Adding a new NodePool for a specialized workload (GPU type, spot, ARC runners)
- Debugging a pod stuck in `Pending` due to no available nodes
- Configuring spot instance interruption handling
- Tuning consolidation policy to reduce idle node costs
- Adding taints/tolerations for workload isolation
- Reviewing Karpenter controller logs for scheduling decisions
- Applying the `commercial-infra/modules/karpenter` Terraform module to a new cluster

---

## Non-Negotiables

1. **GPU NodePools MUST have taints.** Without `dexdat.io/workload=gpu:NoSchedule`,
   CPU pods schedule onto g6e instances (L40S GPU nodes at ~$12/hr). Always taint GPU pools.

2. **Every NodePool needs a weight.** Weight determines which pool Karpenter prefers
   when multiple pools could satisfy a pod. Set explicitly — default is 0 for all pools,
   creating unpredictable behavior.

3. **`consolidationPolicy: WhenEmptyOrUnderutilized` on CPU pools.** This eliminates
   idle nodes within minutes. GPU pools use `WhenEmpty` only — underutilized GPU nodes
   may still have running jobs.

4. **Always set `expireAfter` on NodePools.** Forces node recycling, applies OS patches,
   prevents configuration drift. Use `720h` (30 days) for stable workloads.

5. **Test disruption budgets before enabling consolidation.** Ensure pods have
   `PodDisruptionBudget` and `terminationGracePeriodSeconds` set before Karpenter
   consolidates nodes under them.

---

## Dexdat NodePool Inventory

From `infra-charts/karpenter/templates/` and `commercial-infra/modules/karpenter/`:

| NodePool | Instance Types | Capacity | Purpose | Taint |
|---|---|---|---|---|
| `default` | m7i, m6i, c6i families | On-demand | General CPU workloads | none |
| `g6e` | g6e.* (L40S GPU) | On-demand | ML training, inference | `dexdat.io/workload=gpu` |
| `g6e-placeholder` | g6e.xlarge (placeholder) | On-demand | Reserve g6e capacity | platform |
| `loki` | r6i.2xlarge | On-demand | Loki log aggregation | loki workload |
| `buildkit` | c6i.4xlarge, c7i.4xlarge | Spot | Docker image builds | buildkit |
| `axiom` | m6i.xlarge, m7i.xlarge | On-demand | Axiom agents | platform |
| `code-execution` | m6i.2xlarge, c6i.2xlarge | Spot | Sandboxed code execution | code-execution |
| `github-arc` | m6i.large, m7i.large | Spot | GitHub Actions runners | github-arc |
| `msa-server` | r6i.8xlarge | On-demand | MSA server (high-mem) | msa-server |

---

## EC2NodeClass

```yaml
# infra-charts/karpenter/templates/nodeclass.yaml (default)
apiVersion: karpenter.k8s.aws/v1
kind: EC2NodeClass
metadata:
  name: default
spec:
  # AL2023 — Amazon Linux 2023 (not AL2)
  amiSelectorTerms:
    - alias: al2023@latest       # Karpenter resolves to latest AL2023 AMI

  role: "${cluster_name}-karpenter-node"   # IAM role for EC2 nodes

  subnetSelectorTerms:
    - tags:
        karpenter.sh/discovery: "${cluster_name}"

  securityGroupSelectorTerms:
    - tags:
        karpenter.sh/discovery: "${cluster_name}"

  instanceStorePolicy: RAID0     # NVMe instance store RAID0 for fast local storage

  tags:
    ManagedBy: karpenter
    cluster: "${cluster_name}"
```

```yaml
# GPU EC2NodeClass (g6e instances — L40S)
apiVersion: karpenter.k8s.aws/v1
kind: EC2NodeClass
metadata:
  name: g6e
spec:
  amiSelectorTerms:
    - alias: al2023@latest
  role: "${cluster_name}-karpenter-node"
  subnetSelectorTerms:
    - tags:
        karpenter.sh/discovery: "${cluster_name}"
        tier: gpu                # GPU subnet tier (E-label in commercial-infra)
  securityGroupSelectorTerms:
    - tags:
        karpenter.sh/discovery: "${cluster_name}"
  blockDeviceMappings:
    - deviceName: /dev/xvda
      ebs:
        volumeSize: 500Gi        # Large root volume for ML container images
        volumeType: gp3
        iops: 6000
        throughput: 750
        encrypted: true
  tags:
    ManagedBy: karpenter
    workload: gpu
```

---

## NodePool

```yaml
# Default CPU NodePool
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: default
spec:
  weight: 10                     # Lower weight = less preferred than specialized pools

  template:
    metadata:
      labels:
        dexdat.io/node-pool: default
    spec:
      nodeClassRef:
        group: karpenter.k8s.aws
        kind: EC2NodeClass
        name: default

      requirements:
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["on-demand"]
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64"]
        - key: karpenter.k8s.aws/instance-category
          operator: In
          values: ["m", "c", "r"]
        - key: karpenter.k8s.aws/instance-generation
          operator: Gt
          values: ["5"]

      # Platform components (Karpenter itself, ArgoCD, etc.) must tolerate this
      # so they don't block node provisioning
      taints: []

      expireAfter: 720h           # 30 days — force node recycling

  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 10m
    budgets:
      - nodes: "20%"              # Max 20% of nodes disrupted at once

  limits:
    cpu: "500"
    memory: 2000Gi
```

```yaml
# GPU NodePool (g6e — L40S)
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: g6e
spec:
  weight: 100                    # Highest weight — GPU jobs should land here

  template:
    metadata:
      labels:
        dexdat.io/node-pool: g6e
        nvidia.com/gpu.present: "true"
    spec:
      nodeClassRef:
        group: karpenter.k8s.aws
        kind: EC2NodeClass
        name: g6e

      requirements:
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["on-demand"]  # GPU = on-demand; spot interruptions kill training runs
        - key: karpenter.k8s.aws/instance-family
          operator: In
          values: ["g6e"]

      taints:
        # REQUIRED: prevents CPU workloads from landing on expensive GPU nodes
        - key: dexdat.io/workload
          value: gpu
          effect: NoSchedule
        - key: nvidia.com/gpu
          value: "true"
          effect: NoSchedule

      expireAfter: 168h           # 7 days for GPU nodes (faster AMI rotation)

  disruption:
    consolidationPolicy: WhenEmpty    # Never consolidate non-empty GPU nodes
    consolidateAfter: 30m
    budgets:
      - nodes: "0"               # No disruption during business hours
        schedule: "0 9 * * 1-5"
        duration: 8h
      - nodes: "1"

  limits:
    cpu: "256"                   # Cap total GPU node CPUs
    memory: 4000Gi
```

---

## Spot Instance NodePool (BuildKit / GitHub ARC)

```yaml
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: buildkit
spec:
  weight: 50

  template:
    spec:
      nodeClassRef:
        group: karpenter.k8s.aws
        kind: EC2NodeClass
        name: default
      requirements:
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["spot"]        # BuildKit is spot-safe (stateless builds)
        - key: karpenter.k8s.aws/instance-family
          operator: In
          values: ["c6i", "c7i"]
        - key: karpenter.k8s.aws/instance-size
          operator: In
          values: ["4xlarge"]
      taints:
        - key: dexdat.io/workload
          value: buildkit
          effect: NoSchedule
      expireAfter: 168h

  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 5m         # Aggressive — build nodes idle quickly

  limits:
    cpu: "128"
```

---

## Workload Tolerations (pair with NodePool taints)

```yaml
# In your Deployment/Job spec — required to land on a tainted NodePool
spec:
  tolerations:
    # For GPU nodes
    - key: dexdat.io/workload
      operator: Equal
      value: gpu
      effect: NoSchedule
    - key: nvidia.com/gpu
      operator: Equal
      value: "true"
      effect: NoSchedule

  # Use nodeSelector or nodeAffinity to be explicit
  nodeSelector:
    dexdat.io/node-pool: g6e

  # OR preferredDuringScheduling for soft preference
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: karpenter.k8s.aws/instance-family
                operator: In
                values: ["g6e"]
```

---

## Terraform Module Usage (commercial-infra pattern)

```hcl
# console/modules/post-cilium/main.tf (simplified)
module "karpenter" {
  source = "../../../modules/karpenter"

  cluster_name          = var.cluster_name
  chart_version         = "1.3.1"                # Pin chart version
  ami_selector_alias    = "al2023@latest"
  node_repair_enabled   = true

  # Enable specialized node pools
  loki_node_pool_enabled          = true
  buildkit_node_pool_enabled      = true
  codeops_node_pool_enabled       = true
  code_execution_node_pool_enabled = false
  github_arc_node_pool_enabled    = true
  spot_instances_enabled          = true
}
```

---

## Debugging Pending Pods

```bash
# Check why a pod is Pending (scheduling failure)
kubectl describe pod <pod> -n <namespace> | grep -A 10 "Events:"

# Check Karpenter controller logs for provisioning decisions
kubectl logs -n kube-system \
  $(kubectl get pods -n kube-system -l app.kubernetes.io/name=karpenter -o name | head -1) \
  | grep -E "provisioned|launched|ERROR|cannot" | tail -30

# List all NodeClaims Karpenter created
kubectl get nodeclaims --output wide

# Check NodePool status (capacity limits)
kubectl get nodepools --output wide

# Spot interruption events (check if nodes are being reclaimed)
kubectl get events -A --field-selector reason=SpotInterruption 2>/dev/null || \
  kubectl logs -n kube-system \
    $(kubectl get pods -n kube-system -l app.kubernetes.io/name=karpenter -o name | head -1) \
    | grep -i "interruption\|spot" | tail -20

# Force consolidation (drain a specific underutilized node)
kubectl annotate node <node-name> karpenter.sh/do-not-disrupt-
```

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| GPU NodePool without taints | CPU pods land on $12/hr GPU nodes | `dexdat.io/workload=gpu:NoSchedule` required |
| `consolidationPolicy: WhenEmpty` on CPU pools | Idle CPU nodes cost money for hours | Use `WhenEmptyOrUnderutilized` |
| No `expireAfter` on NodePools | Nodes never recycled; drift, unpatched AMIs | Set `720h` for CPU, `168h` for GPU |
| GPU spot instances for training jobs | Interruption kills multi-hour training runs | `on-demand` for GPU; spot only for stateless |
| No `limits` on NodePool | Runaway autoscaling; unexpected AWS bills | Always set `cpu:` and `memory:` limits |
| No PodDisruptionBudget on services | Consolidation kills all replicas at once | PDB with `minAvailable: 1` |
| Forgetting `nvidia.com/gpu` taint | Other GPU taints exist; some schedulers check both | Always pair `dexdat.io/workload=gpu` with `nvidia.com/gpu` |

---

## axiom:trace

`axiom:trace work_item=devops-skills-01 spec=specs/00-PRD.md plan= prompt=.opencode/skills/karpenter-shellops/SKILL.md evidence= doc= ops= commit=`
