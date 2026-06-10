---
name: kubernetes-shellops
description: >
  Kubernetes (kubectl) operations patterns: pod/deployment management, resource debugging,
  RBAC, ConfigMaps/Secrets, namespaces, network policies, resource quotas, HPA/VPA,
  health checks, and Axiom traceability integration. Load this skill when writing K8s
  manifests, debugging cluster issues, operating workloads, or reviewing Kubernetes configs.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/00-PRD.md
  related_skills:
    - helm-shellops
    - argocd-shellops
    - aws-cli-shellops
    - prometheus-shellops
    - sre-ops-axiom
    - hardening-sre-axiom
tags:
  vertical: [devops, sre, kubernetes, containers]
  category: cloud-operations
  core: false
---

# Kubernetes — Axiom Integration Skill

> **"Always namespace. Never assume default."**
> **"No kubectl apply in prod without a plan, rollback, and evidence capture."**
> **"Labels and annotations are your observability contract — make them consistent."**

This skill provides production-grade Kubernetes operations patterns for Axiom workflows.
It covers day-1 and day-2 operations, debugging playbooks, RBAC, resource management,
and how to produce traceable evidence for every cluster state change.

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
- Writing Kubernetes manifests (Deployments, Services, ConfigMaps, etc.)
- Debugging pod failures, CrashLoopBackOffs, or OOMKilled events
- Configuring RBAC (Roles, ClusterRoles, ServiceAccounts)
- Managing namespaces, resource quotas, and limit ranges
- Setting up HPA/VPA for autoscaling
- Writing network policies
- Running pre/post deploy verification in runbooks
- Reviewing existing manifests for production readiness

---

## Non-Negotiables

1. **Always specify namespace.** Never rely on the current context namespace.
   Pass `-n <namespace>` or `--namespace <namespace>` explicitly.

2. **`kubectl diff` before apply in production.** Run `kubectl diff -f manifest.yaml`
   before applying any change to production. Capture diff as evidence.

3. **Avoid `kubectl apply -f https://...` in production.** Always download, review,
   and pin manifests before applying. Never pipe untrusted URLs directly to `kubectl apply`.

4. **Resource requests/limits are required.** Every container in prod must have
   `resources.requests` and `resources.limits`. No unlimited containers.

5. **Labels as contracts.** Every resource must have `app`, `version`, and `env` labels.
   These are your selectors, your Prometheus targets, and your runbook handles.

6. **Never `kubectl exec` into production pods for debugging** without documenting
   the session in the work item evidence. Prefer ephemeral debug containers.

---

## Context and Namespace Safety

```bash
# ALWAYS check context before prod operations
kubectl config current-context
kubectl config get-contexts

# Switch context
kubectl config use-context my-prod-cluster

# Set default namespace for session (not a substitute for -n in scripts)
kubectl config set-context --current --namespace=my-app

# Verify you are where you think you are
kubectl cluster-info
kubectl get nodes --output wide
```

---

## Pod Operations

### Inspect and Debug

```bash
# List pods with wide output (shows node placement)
kubectl get pods -n my-app --output wide

# Describe pod (events are the most useful section)
kubectl describe pod my-pod-xyz -n my-app

# Get pod logs
kubectl logs my-pod-xyz -n my-app
kubectl logs my-pod-xyz -n my-app --previous  # crashed container
kubectl logs my-pod-xyz -n my-app -c sidecar  # specific container

# Follow logs in real time
kubectl logs -f deployment/my-app -n my-app

# Get events sorted by time (best first debugging step)
kubectl get events -n my-app --sort-by='.lastTimestamp'
```

### Ephemeral Debug Containers (preferred over exec)

```bash
# Attach ephemeral debug container (K8s 1.23+)
kubectl debug -it my-pod-xyz -n my-app \
  --image=busybox:stable \
  --target=app-container

# Debug a node
kubectl debug node/my-node -it --image=ubuntu
```

### Resource Usage

```bash
# Pod CPU/memory usage
kubectl top pods -n my-app --sort-by=memory

# Node CPU/memory usage
kubectl top nodes
```

---

## Deployment Patterns

### Apply with Diff and Evidence

```bash
# Standard evidence-producing apply
WORK_ITEM_ID="my-work-item"
EVIDENCE_DIR=".memory-bank/work-items/${WORK_ITEM_ID}"
mkdir -p "$EVIDENCE_DIR"

# 1. Capture pre-state
kubectl get deployment my-app -n my-app -o json > "${EVIDENCE_DIR}/pre-deploy.json"

# 2. Show diff
kubectl diff -f k8s/deployment.yaml > "${EVIDENCE_DIR}/diff.txt" 2>&1 || true
cat "${EVIDENCE_DIR}/diff.txt"

# 3. Apply
kubectl apply -f k8s/deployment.yaml --output json | tee "${EVIDENCE_DIR}/apply-result.json"

# 4. Wait for rollout
kubectl rollout status deployment/my-app -n my-app --timeout=300s

# 5. Capture post-state
kubectl get deployment my-app -n my-app -o json > "${EVIDENCE_DIR}/post-deploy.json"
```

### Rollback

```bash
# Check rollout history
kubectl rollout history deployment/my-app -n my-app

# Rollback to previous
kubectl rollout undo deployment/my-app -n my-app

# Rollback to specific revision
kubectl rollout undo deployment/my-app -n my-app --to-revision=3

# Watch rollback
kubectl rollout status deployment/my-app -n my-app
```

### Scaling

```bash
# Scale manually
kubectl scale deployment my-app --replicas=3 -n my-app

# Check HPA status
kubectl get hpa -n my-app --output wide
kubectl describe hpa my-app-hpa -n my-app
```

---

## Resource Manifest Patterns

### Deployment with Best Practices

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: my-app                # Always explicit namespace
  labels:
    app: my-app                    # Required: selector label
    version: "1.2.3"               # Required: version label
    env: production                # Required: environment label
  annotations:
    axiom.io/work-item: "PROJ-123"  # Trace to work item
    axiom.io/trace: "axiom:trace work_item=PROJ-123 impl=k8s/deployment.yaml"
spec:
  replicas: 3
  selector:
    matchLabels:
      app: my-app
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0            # Zero-downtime
  template:
    metadata:
      labels:
        app: my-app
        version: "1.2.3"
        env: production
    spec:
      serviceAccountName: my-app  # Explicit SA, not default
      securityContext:
        runAsNonRoot: true         # Never run as root
        runAsUser: 10001
        fsGroup: 10001
        seccompProfile:
          type: RuntimeDefault     # Required by PSS Restricted (K8s 1.25+)
      containers:
        - name: app
          image: my-registry/my-app:1.2.3  # Always pinned tag, never :latest
          ports:
            - containerPort: 8080
          securityContext:         # Container-level (separate from pod-level above)
            allowPrivilegeEscalation: false  # Required: no sudo/setuid escalation
            readOnlyRootFilesystem: true     # Required: no runtime filesystem writes
            capabilities:
              drop: ["ALL"]        # Required: drop all Linux capabilities
          resources:
            requests:
              cpu: "100m"          # Required: resource requests
              memory: "128Mi"
            limits:
              cpu: "500m"          # Required: resource limits
              memory: "512Mi"
          readinessProbe:          # Required: readiness probe
            httpGet:
              path: /ready
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:           # Required: liveness probe
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 15
            periodSeconds: 20
          env:
            - name: APP_ENV
              value: production
            - name: DB_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: my-app-secrets
                  key: db-password
      affinity:                    # Spread across nodes
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchLabels:
                    app: my-app
                topologyKey: kubernetes.io/hostname
```

---

## ConfigMaps and Secrets

### ConfigMaps

```bash
# Create from literal
kubectl create configmap my-app-config \
  --from-literal=DB_HOST=my-db.svc.cluster.local \
  --from-literal=APP_LOG_LEVEL=info \
  -n my-app

# Create from file
kubectl create configmap my-app-config --from-file=config.yaml -n my-app

# View
kubectl get configmap my-app-config -n my-app -o yaml
```

### Secrets (External Secret Managers are preferred)

```bash
# Create secret (prefer ExternalSecrets + Vault/AWS SM over manual creation)
kubectl create secret generic my-app-secrets \
  --from-literal=db-password='[REDACTED]' \
  -n my-app

# Check secret exists (never print values)
kubectl get secret my-app-secrets -n my-app -o jsonpath='{.metadata.name}'

# Verify secret keys (not values)
kubectl get secret my-app-secrets -n my-app -o jsonpath='{.data}' | jq 'keys'
```

> **Note**: Prefer ExternalSecrets Operator + AWS Secrets Manager or Vault over
> `kubectl create secret`. Native K8s secrets are base64-encoded, not encrypted at rest
> unless you configure KMS encryption.

---

## RBAC Patterns

### ServiceAccount + Role + RoleBinding

```yaml
# ServiceAccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-app
  namespace: my-app

---
# Role (namespace-scoped)
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: my-app-role
  namespace: my-app
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch"]
    resourceNames: ["my-app-config"]    # Scope to specific resource names

---
# RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-app-rolebinding
  namespace: my-app
subjects:
  - kind: ServiceAccount
    name: my-app
    namespace: my-app
roleRef:
  kind: Role
  apiGroup: rbac.authorization.k8s.io
  name: my-app-role
```

```bash
# Check RBAC for a service account
kubectl auth can-i list pods \
  --as=system:serviceaccount:my-app:my-app \
  -n my-app

# Audit what a SA can do
kubectl auth can-i --list \
  --as=system:serviceaccount:my-app:my-app \
  -n my-app
```

---

## Resource Quotas and Limit Ranges

```yaml
# Namespace ResourceQuota
apiVersion: v1
kind: ResourceQuota
metadata:
  name: my-app-quota
  namespace: my-app
spec:
  hard:
    requests.cpu: "4"
    requests.memory: 8Gi
    limits.cpu: "8"
    limits.memory: 16Gi
    pods: "20"

---
# LimitRange (default limits for containers without explicit limits)
apiVersion: v1
kind: LimitRange
metadata:
  name: my-app-limits
  namespace: my-app
spec:
  limits:
    - default:
        cpu: "500m"
        memory: "512Mi"
      defaultRequest:
        cpu: "100m"
        memory: "128Mi"
      type: Container
```

---

## Network Policies

```yaml
# Default deny-all (start here, then allow what's needed)
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-all
  namespace: my-app
spec:
  podSelector: {}
  policyTypes:
    - Ingress
    - Egress

---
# Allow ingress from load balancer and specific namespaces only
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-frontend-ingress
  namespace: my-app
spec:
  podSelector:
    matchLabels:
      app: my-app
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ingress-nginx
```

---

## Debugging Playbook

### CrashLoopBackOff

```bash
# Step 1: Get pod status
kubectl get pod my-pod -n my-app -o wide

# Step 2: Get events (first thing to check)
kubectl get events -n my-app --field-selector involvedObject.name=my-pod

# Step 3: Check logs (including previous container)
kubectl logs my-pod -n my-app --previous 2>&1 | tail -50

# Step 4: Describe for full picture
kubectl describe pod my-pod -n my-app

# Step 5: If OOMKilled
kubectl get pod my-pod -n my-app -o jsonpath='{.status.containerStatuses[0].lastState.terminated.reason}'
```

### Pending Pod (scheduling issues)

```bash
# Check why pod is pending
kubectl describe pod my-pod -n my-app | grep -A 20 "Events:"

# Check node capacity
kubectl describe nodes | grep -A 5 "Allocated resources"

# Check if there are taints blocking scheduling
kubectl get nodes -o custom-columns=NAME:.metadata.name,TAINTS:.spec.taints
```

### Service Connectivity

```bash
# Test DNS resolution from within cluster
kubectl run debug-pod --rm -it --image=busybox:stable --restart=Never -n my-app -- \
  nslookup my-service.my-app.svc.cluster.local

# Test port connectivity
kubectl run debug-pod --rm -it --image=busybox:stable --restart=Never -n my-app -- \
  wget -qO- http://my-service:8080/health

# Check endpoints (no endpoints = pod selector mismatch)
kubectl get endpoints my-service -n my-app
```

---

## Production Readiness Checklist

Before declaring a K8s workload production-ready:

```markdown
## K8s Production Readiness: <deployment-name>

### Required
- [ ] Namespace-scoped (not default)
- [ ] Image tag pinned (not :latest)
- [ ] Resources requests + limits set
- [ ] Readiness probe configured
- [ ] Liveness probe configured
- [ ] runAsNonRoot: true
- [ ] Dedicated ServiceAccount (not default)
- [ ] Secrets from external source (not hardcoded)
- [ ] Pod disruption budget configured
- [ ] Horizontal Pod Autoscaler configured
- [ ] Required labels: app, version, env
- [ ] Network policy restricts egress/ingress

### Evidence
- [ ] `kubectl diff` output captured before apply
- [ ] `kubectl rollout status` shows successful rollout
- [ ] Health checks passing post-deploy
- [ ] Resource usage within limits (kubectl top)
```

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| `image: my-app:latest` | Non-deterministic; breaks rollbacks | Pinned semantic version or SHA |
| No resource limits | Node OOM kills all pods on node | Always set requests + limits |
| Running as root | Security blast radius; container escape | `runAsNonRoot: true` + explicit UID |
| Using `default` namespace | No isolation; accidental cross-app access | Dedicated namespace per service |
| No readiness probe | Traffic to not-ready pods; failed rollouts | Always add readiness probe |
| `kubectl apply -f https://...` | Unreviewed code execution | Download, review, pin first |
| Secrets in ConfigMaps | Secrets are plain text | Use K8s Secrets + KMS encryption |
| No PodDisruptionBudget | Rolling updates kill all pods simultaneously | PDB with `minAvailable: 1` |
| `kubectl delete pod` to force redeploy | Bypasses deployment strategy | `kubectl rollout restart deployment/...` |

---

## axiom:trace

`axiom:trace work_item=kubernetes-shellops-01 spec=specs/00-PRD.md plan= prompt=.opencode/skills/kubernetes-shellops/SKILL.md evidence= doc= ops= commit=`
