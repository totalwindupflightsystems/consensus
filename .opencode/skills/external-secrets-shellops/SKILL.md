---
name: external-secrets-shellops
description: >
  External Secrets Operator (ESO) patterns for Kubernetes: SecretStore and ClusterSecretStore
  setup with AWS Secrets Manager, ExternalSecret and PushSecret resources, ECR token rotation,
  on-prem SecretStore via PrivateLink, and Axiom traceability integration. This is the
  primary secrets management pattern — not Vault. Load this skill for any work involving
  secrets in Kubernetes at Dexdat.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/00-PRD.md
  related_skills:
    - aws-cli-shellops
    - kubernetes-shellops
    - helm-shellops
    - argocd-shellops
    - hardening-security-axiom
tags:
  vertical: [devops, sre, security, secrets]
  category: security
  core: false
---

# External Secrets Operator — Axiom Integration Skill

> **"Secrets live in AWS Secrets Manager. ESO syncs them to Kubernetes. Never the reverse."**
> **"A secret that exists only in a values.yaml file is not a secret — it's a plaintext credential."**
> **"Every namespace that needs secrets gets its own SecretStore binding, scoped minimally."**

This skill covers the External Secrets Operator (ESO) as deployed at Dexdat via
`infra-charts/external-secrets/`, `external-secrets-store/`, and `external-secrets-onprem/`.
ESO is the **primary secrets management pattern** — AWS Secrets Manager is the source of truth,
and ESO syncs secrets into K8s on a configurable refresh interval.

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
- Adding a new secret to a Kubernetes workload
- Setting up a new SecretStore for a namespace or cluster
- Debugging an ExternalSecret stuck in `SecretSyncedError` state
- Rotating a secret in AWS Secrets Manager and verifying K8s sync
- Configuring ECR token auto-refresh for image pulls
- Setting up on-prem cluster secret access via PrivateLink SecretStore
- Reviewing a Helm chart that manages ExternalSecret resources

---

## Non-Negotiables

1. **Never put secret values in `values.yaml`, `values-*.yaml`, or any Git-committed file.**
   These files are in Git. Secrets go into AWS Secrets Manager; an `ExternalSecret`
   resource references them by name.

2. **SecretStore IAM role follows least-privilege.** The IAM role used by the SecretStore
   must only have `secretsmanager:GetSecretValue` on the specific secrets it needs.
   No `secretsmanager:*` or `*` resources.

3. **`refreshInterval` must be set.** Never use `refreshInterval: 0` in production.
   Use `1h` for stable secrets, `15m` for rotating credentials (e.g. DB passwords),
   `5m` for tokens (e.g. ECR).

4. **Verify sync before deploy.** Check `kubectl get externalsecret -n <ns>` shows
   `SYNCED: True` before deploying workloads that depend on the secret.

5. **ExternalSecrets are owned by the Helm release.** Define ExternalSecrets in Helm
   chart templates, not as raw `kubectl apply`. This ensures they are lifecycle-managed.

---

## Architecture (Dexdat pattern)

```
AWS Secrets Manager (source of truth)
        │
        │  secretsmanager:GetSecretValue
        │  (via IAM Role for Service Account — IRSA)
        ▼
External Secrets Operator (runs in monitoring/external-secrets namespace)
        │
        │  Creates/updates K8s Secrets
        ▼
ClusterSecretStore / SecretStore (defines connection + IAM binding)
        │
        ▼
ExternalSecret (maps ASM secret keys → K8s Secret keys)
        │
        ▼
K8s Secret (consumed by Pod via env/volume)
```

---

## ClusterSecretStore Setup

The `external-secrets` Helm chart in `infra-charts` creates the ClusterSecretStore
with Pod Identity Association (newer, preferred over annotation-based IRSA).

```yaml
# infra-charts/external-secrets/templates/secretstore.yaml
apiVersion: external-secrets.io/v1beta1
kind: ClusterSecretStore
metadata:
  name: aws-secrets-manager
  annotations:
    axiom.io/work-item: "{{ .Values.codeopsWorkItem }}"
spec:
  provider:
    aws:
      service: SecretsManager
      region: us-east-1
      auth:
        jwt:
          serviceAccountRef:
            name: external-secrets-sa
            namespace: external-secrets
```

---

## ExternalSecret (the core resource)

### Basic pattern — map ASM secret to K8s Secret

```yaml
# In your service's Helm chart templates/externalsecret.yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: {{ include "my-service.fullname" . }}-secrets
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "my-service.labels" . | nindent 4 }}
  annotations:
    axiom.io/work-item: {{ .Values.codeopsWorkItem | default "unknown" | quote }}
spec:
  refreshInterval: 1h           # How often to re-sync from ASM

  secretStoreRef:
    name: aws-secrets-manager   # The ClusterSecretStore name
    kind: ClusterSecretStore

  target:
    name: {{ include "my-service.fullname" . }}-secrets   # K8s Secret name to create
    creationPolicy: Owner        # ESO owns the Secret lifecycle

  data:
    # Each entry maps one ASM secret key to one K8s Secret key
    - secretKey: db-password     # Key in the K8s Secret
      remoteRef:
        key: my-service/production   # ASM secret name (path)
        property: db_password        # JSON key within the ASM secret value

    - secretKey: api-key
      remoteRef:
        key: my-service/production
        property: api_key

    - secretKey: jwt-secret
      remoteRef:
        key: my-service/production
        property: jwt_secret
```

### Whole ASM secret as K8s Secret (dataFrom)

```yaml
spec:
  # ...
  dataFrom:
    - extract:
        key: my-service/production   # All JSON keys in ASM secret → K8s Secret keys
```

---

## AWS Secrets Manager Conventions

Dexdat uses a path-based naming convention for ASM secrets:

```
{service}/{environment}         → e.g. my-service/production
{cluster}/{component}           → e.g. houston/temporal
{team}/{service}/{environment}  → e.g. ml/trainer/production
```

```bash
# Create a new secret
aws secretsmanager create-secret \
  --name "my-service/production" \
  --description "Production secrets for my-service" \
  --secret-string '{"db_password":"[REDACTED]","api_key":"[REDACTED]"}' \
  --region us-east-1 \
  --output json | tee .memory-bank/work-items/${WORK_ITEM_ID}/secret-created.json

# Update a secret (rotation)
aws secretsmanager put-secret-value \
  --secret-id "my-service/production" \
  --secret-string '{"db_password":"[REDACTED_NEW]","api_key":"[REDACTED]"}' \
  --region us-east-1

# Force ESO to re-sync immediately after rotation
kubectl annotate externalsecret my-service-secrets \
  -n my-service \
  force-sync=$(date +%s) \
  --overwrite
```

---

## ECR Token Rotation (infra-charts pattern)

The `external-secrets` chart includes automatic ECR token refresh — ESO generates
a short-lived ECR auth token and stores it as a K8s dockerconfigjson Secret.

```yaml
# infra-charts/external-secrets/templates/ecr-token.yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: ecr-registry-credentials
  namespace: {{ .Values.ecrToken.namespace }}
spec:
  refreshInterval: 4h             # ECR tokens expire after 12h; refresh every 4h

  secretStoreRef:
    name: aws-secrets-manager
    kind: ClusterSecretStore

  target:
    name: ecr-registry-credentials
    creationPolicy: Owner
    template:
      type: kubernetes.io/dockerconfigjson
      data:
        .dockerconfigjson: |
          {"auths":{"{{ .Values.ecrRegistry }}":{"auth":"{{ .username | b64enc }}"}}}

  data:
    - secretKey: username
      remoteRef:
        key: ecr-token/{{ .Values.cluster }}
        property: token
```

---

## On-Prem SecretStore (via PrivateLink)

> ⚠️ **Security Exception — requires_human_review: true**
> This pattern uses static long-lived IAM access keys stored in a Kubernetes Secret.
> This is a **last-resort** pattern for on-prem clusters with no IRSA/Pod Identity path.
> If you use this pattern you MUST:
> 1. Store the IAM key in AWS Secrets Manager and rotate via ESO itself (90-day max TTL)
> 2. Set a CloudWatch alarm on the key's last-used date
> 3. Scope the IAM policy to the minimum required secrets only (specific ARN prefixes)
> 4. Document the exception in your work item with `requires_human_review: true`
>
> **Preferred alternatives** (in priority order):
> - [AWS IAM Roles Anywhere](https://docs.aws.amazon.com/rolesanywhere/latest/userguide/introduction.html) — certificate-based, no static keys
> - [EKS Anywhere](https://anywhere.eks.amazonaws.com/) with Pod Identity — if cluster is EKS Anywhere
> - HashiCorp Vault bridge — Vault on-prem with AWS Secrets backend, ESO uses Vault provider

For on-prem K8s clusters that need AWS Secrets Manager access via Direct Connect/PrivateLink,
the `external-secrets-onprem` chart sets up a SecretStore using a VPC endpoint:

```yaml
# infra-charts/external-secrets-onprem/templates/secretstore.yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore                  # Namespace-scoped for on-prem
metadata:
  name: aws-secrets-manager-onprem
  namespace: {{ .Release.Namespace }}
spec:
  provider:
    aws:
      service: SecretsManager
      region: ${AWS_REGION}
      endpoint: https://secretsmanager.${AWS_REGION}.vpce.amazonaws.com  # PrivateLink endpoint
      auth:
        secretRef:
          # ⚠️ Static keys — accepted exception only. See warning above.
          accessKeyIDSecretRef:
            name: aws-credentials-onprem
            key: access-key-id
          secretAccessKeySecretRef:
            name: aws-credentials-onprem
            key: secret-access-key
```

---

## Debugging ExternalSecrets

```bash
# Check sync status across all namespaces
kubectl get externalsecret -A \
  --output custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,SYNCED:.status.conditions[0].status,REASON:.status.conditions[0].reason'

# Describe a failing ExternalSecret
kubectl describe externalsecret my-service-secrets -n my-service

# Common failure: IAM permission denied
# Look for: "AccessDeniedException" in events
kubectl get events -n my-service --field-selector reason=SecretSyncedError

# Force re-sync
kubectl annotate externalsecret my-service-secrets \
  -n my-service \
  force-sync=$(date +%s) --overwrite

# Check the resulting K8s Secret exists (don't print values)
kubectl get secret my-service-secrets -n my-service -o jsonpath='{.data}' | jq 'keys'

# Verify ESO operator is healthy
kubectl get pods -n external-secrets
kubectl logs -n external-secrets \
  $(kubectl get pods -n external-secrets -l app.kubernetes.io/name=external-secrets -o name | head -1) \
  | tail -20
```

---

## IAM Policy for SecretStore (least privilege)

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret"
      ],
      "Resource": [
        "arn:aws:secretsmanager:us-east-1:${AWS_ACCOUNT_ID}:secret:my-service/*"
      ]
    }
  ]
}
```

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| Secret values in `values.yaml` | Plaintext in Git; leaked in PRs | ASM secret + ExternalSecret reference |
| `refreshInterval: 0` | Secret never re-syncs; stale after rotation | Use `1h` minimum |
| `ClusterSecretStore` IAM role with `secretsmanager:*` | Over-privileged; blast radius = all secrets | Scope to specific `arn:...:secret:path/*` |
| `creationPolicy: Merge` without understanding it | Can leave orphaned keys in existing Secrets | Use `Owner` unless you specifically need `Merge` |
| No `force-sync` after rotation | Pods use stale password until next interval | Annotate after rotating: `kubectl annotate ...` |
| ExternalSecret in raw `kubectl apply` | Not lifecycle-managed; leaks on namespace delete | Define in Helm chart templates |
| Storing whole JSON blob as single Secret key | App can't access individual fields | Use `property:` to map individual JSON keys |

---

## axiom:trace

`axiom:trace work_item=devops-skills-01 spec=specs/00-PRD.md plan= prompt=.opencode/skills/external-secrets-shellops/SKILL.md evidence= doc= ops= commit=`
