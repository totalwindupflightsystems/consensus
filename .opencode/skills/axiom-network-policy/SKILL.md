---
name: axiom-network-policy
description: Portable network policy and egress control for Axiom workspace containers — permissive v1 posture, deny-by-default post-v1 with allowlists, enforcement mechanisms, audit mode, and posture transition guidance.
version: "1.0"
synopsis: |
  Defines how egress traffic is controlled for Axiom PR workspace containers. Covers the v1
  permissive posture, post-v1 deny-by-default with allowlist model, allowlist schema and validation,
  enforcement boundaries (Kubernetes NetworkPolicy, Cilium, Calico), platform default allowlist,
  audit mode for safe transition, monitoring metrics/alerts, and operational guidance for updates.
when-to-use: |
  Load this skill when configuring network egress policies for Axiom containers, designing
  allowlist schemas, implementing deny-by-default enforcement, transitioning from permissive to
  restricted posture, or setting up egress monitoring and alerting.
tags:
  vertical: [ops, security]
  category: security
  core: false
---

# Axiom Network Policy and Egress Control (Portable)

This skill defines egress traffic control for Axiom workspace containers.

Source spec: `specs/33-Network-Policy-And-Egress.md`

---

## Posture Overview

| Phase | Posture | Enforcement |
|---|---|---|
| v1 | **Permissive** | No NetworkPolicy applied; all outbound allowed |
| Post-v1 | **Deny-by-default** | Allowlist-based; operator-activated |

---

## v1 Posture: Permissive

All egress traffic from PR workspace containers is allowed. No Kubernetes NetworkPolicy or equivalent is applied.

| Concern | v1 Behavior |
|---|---|
| Outbound HTTPS | Allowed to any destination |
| Outbound DNS | Allowed via cluster DNS |
| Outbound non-HTTPS | Allowed (no port restrictions) |
| Logging | No network-policy-level logging; app-level HTTP logging applies |
| Monitoring | No egress-specific alerts |

### v1 Constraints (still enforced)

- Containers run as non-root (UID 1000)
- Application code never sends secrets to non-allowlisted destinations
- OpenCode server binds to localhost only

---

## Post-v1 Posture: Deny-by-Default

When activated, all egress is denied unless explicitly allowed by the allowlist.

### Allowlist Schema

Defined in `.axiom/axiom.config.yaml` under `network`:

```yaml
network:
  egress_policy: "deny_by_default"   # "permissive" | "deny_by_default"
  egress_allowlist:
    - destination: "pypi.org"
      ports: [443]
      protocol: "TCP"
      description: "Python package registry"
    - destination: "*.atlassian.net"
      ports: [443]
      protocol: "TCP"
      description: "Jira API (wildcard)"
```

### Allowlist Entry Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `destination` | string | Yes | Hostname, IP, CIDR, or wildcard (`*.example.com`) |
| `ports` | list[int] | Yes | Allowed destination ports (1-65535) |
| `protocol` | string | Yes | `"TCP"` or `"UDP"` |
| `description` | string | Yes | Human-readable purpose |

### Validation Rules

- `destination`: non-empty string
- `ports`: non-empty list of integers in [1, 65535]
- `protocol`: `"TCP"` or `"UDP"`
- `description`: non-empty string
- Duplicates allowed but discouraged; control plane deduplicates

---

## Platform Default Allowlist

The control plane maintains a platform default allowlist (not in per-repo config):

**MUST include**:
- Cluster DNS (UDP port 53)
- Kubernetes API server (if applicable)

**SHOULD include**:
- Common package registries (PyPI, npm, Docker Hub)
- Jira and GitHub API endpoints (when integrations enabled)
- S3 endpoints (when snapshot storage configured for S3)

### Merge Strategy

Per-repo entries are **merged** with platform defaults. Per-repo can only **add** destinations; cannot remove platform defaults.

---

## Enforcement Boundaries

| Mechanism | Domain Support | When to Use |
|---|---|---|
| Kubernetes NetworkPolicy | No (IP/CIDR only) | Default; works with any CNI |
| Cilium CiliumNetworkPolicy | Yes (FQDN-based) | When Cilium CNI available |
| Calico NetworkPolicy | Yes (DNS-based) | When Calico CNI available |

### Selection Rule

Control plane auto-detects available enforcement mechanism. If no domain-capable CNI and allowlist contains hostnames: resolve to IPs at policy generation time, log warning about DNS drift.

### Generated NetworkPolicy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: axiom-work-egress
  namespace: "{namespace}"
  labels:
    app.kubernetes.io/managed-by: axiom
spec:
  podSelector:
    matchLabels:
      app: axiom-work
  policyTypes: [Egress]
  egress:
    - ports:
        - protocol: UDP
          port: 53          # DNS always allowed
    # + generated rules from merged allowlist
```

### Invariants

- DNS egress (UDP 53) always allowed; cannot be removed
- Targets pods with label `app: axiom-work`
- Sync frequency: on pod creation (default), operator-configurable

---

## Posture Transition

Transitioning from permissive to deny-by-default is a **deliberate operator action**.

### Transition Steps

| Step | Action | Verification |
|---|---|---|
| 1 | Set `network.egress_policy: "deny_by_default"` | Config loads without error |
| 2 | Deploy with `audit` mode (`egress_audit_only: true`) | Denials logged but traffic not blocked |
| 3 | Analyze audit logs for 1-2 weeks | No unexpected denials for normal operations |
| 4 | Update allowlist with missing entries | Allowlist covers all required destinations |
| 5 | Switch from audit to enforce (`egress_audit_only: false`) | Denied connections blocked and logged |

### Audit Mode

When `network.egress_audit_only: true`:
- Cilium: native audit mode
- Standard NetworkPolicy: log intended denials without applying policy

---

## Monitoring

### Metrics

| Metric | Type | Labels | Description |
|---|---|---|---|
| `codeops_egress_denied_total` | Counter | work_item_id, repo, destination, port | Denied egress connections |
| `codeops_egress_allowed_total` | Counter | work_item_id, repo, destination, port | Allowed connections (audit mode) |

### Alerts

| Alert | Condition | Severity |
|---|---|---|
| `EgressDeniedRepeated` | `rate(codeops_egress_denied_total[5m]) > 0` for single work item | Warning |
| `EgressDeniedBurst` | `rate(codeops_egress_denied_total[1m]) > 10` across all work items | Critical |

### Structured Log Events

```json
{
  "level": "WARN",
  "event_type": "egress_denied",
  "component": "network_policy",
  "work_item_id": "ABC-123",
  "destination": "api.example.com",
  "port": 443,
  "protocol": "TCP",
  "action": "denied"
}
```

---

## Operational Guidance

### Updating the Allowlist

1. Edit `network.egress_allowlist` in `.axiom/axiom.config.yaml`
2. Commit and push
3. Control plane detects on next pod creation (or sync interval)
4. New pods get updated policy; existing pods updated on next sync

**Safe rollout**: Test in staging namespace first. Use audit mode to verify.

---

## Resolved Decisions

| Decision | Resolution |
|---|---|
| v1 enforcement | None (permissive) |
| Post-v1 default enforcement | Kubernetes NetworkPolicy |
| Allowlist merge strategy | Additive only (per-repo adds to platform defaults) |
| Wildcard support | Enforcement-mechanism-dependent (requires Cilium/Calico) |
| Audit mode | Supported |
| Config location | `.axiom/axiom.config.yaml` |

## Open Decisions

| Decision | Options |
|---|---|
| Sync frequency for existing pods | On-demand vs periodic |
| Audit mode for standard NetworkPolicy | Log-only vs shadow policy |
