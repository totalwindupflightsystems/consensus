---
name: version-pinning-axiom
description: >
  Dependency version pinning strategy for Helm charts, npm, pip, Go modules, and Docker images. Covers exact pin vs range strategies, upgrade workflows, and anti-patterns.
version: "1.0"
tags:
  vertical: ['operations', 'security']
  category: operations
  core: false
---
# version-pinning-axiom — Dependency Version Pinning Strategy

## When to Load This Skill

Load this skill when:
- Setting up Helm charts, npm packages, pip packages, or Go modules for a service
- Reviewing a PR that bumps dependency versions
- Designing a CI/CD pipeline that consumes external packages
- Deciding between pinning strategies for a new dependency
- Onboarding a new third-party service (Langfuse, Keycloak, Redis, etc.)

---

## The Core Decision

Every external dependency has the same question: **how tightly do you pin the version?**

| Strategy | Format | What flows in | Risk | Effort |
|---|---|---|---|---|
| **Exact pin** | `1.2.3` | Nothing — you control every upgrade | Lowest risk, highest staleness | Manual bump PRs |
| **Patch range** | `~1.2.3` or `>=1.2.3, <1.3.0` | Patch fixes (1.2.4, 1.2.5) | Low risk — patches are bug fixes | Occasional review |
| **Minor range** | `^1.2.3` or `>=1.2.0, <2.0.0` | Minor features + patches (1.3.0, 1.4.0) | Medium risk — new features may change behavior | Monthly review |
| **Major range** | `>=1.0.0` or `*` | Everything including breaking changes | High risk — anything can break | Constant vigilance |
| **Latest/floating** | `latest`, `main`, no pin | Whatever is newest right now | Maximum risk — you don't know what you're running | None (that's the problem) |

---

## Recommended Default: Exact Pin + Monthly Bump PR

For most services, **pin to an exact version and upgrade deliberately**:

```yaml
# Helm chart
helm install langfuse langfuse/langfuse-k8s --version 1.2.3

# npm
"langfuse": "3.42.0"

# pip
langfuse==3.42.0

# Go
require github.com/langfuse/langfuse-go v1.2.3
```

Then create a monthly PR that bumps the version:
1. Check the changelog for breaking changes
2. Bump the version in the lock file / chart values
3. Run tests in staging
4. Promote to production after soak

This matches the ArgoCD pattern: **staging auto-syncs, production has a manual gate**.

---

## When to Use Each Strategy

### Exact Pin (`1.2.3`) — Default for production services

Use when:
- The dependency is in your critical path (database, auth, observability)
- The dependency releases frequently (multiple times per week)
- Breaking changes have bitten you before
- You have a staging environment to test upgrades

Examples: Langfuse, Keycloak, PostgreSQL, Redis, Kafka

### Patch Range (`~1.2.3`) — Good for stable libraries

Use when:
- The dependency follows strict semver
- Patches are genuinely bug-fix-only (no behavior changes)
- You want security patches to flow automatically
- The dependency is well-tested and mature

Examples: lodash, express, pytest, boto3

### Minor Range (`^1.2.3`) — Good for dev tools

Use when:
- The dependency is a dev tool, not a production runtime dependency
- New features are additive and don't break existing usage
- You want to stay current without manual effort
- Breaking changes are rare and well-documented

Examples: ruff, mypy, eslint, prettier, jest

### Never Use: Floating / No Pin

Never use `latest`, `main`, or unpinned versions in:
- Production deployments
- CI pipelines (builds should be reproducible)
- Helm charts (ArgoCD will drift)
- Docker base images (use digest pins: `python:3.13@sha256:abc...`)

---

## Helm Chart Pinning (Kubernetes / ArgoCD)

### The Pattern

```yaml
# argocd/apps/langfuse.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
spec:
  source:
    repoURL: https://langfuse.github.io/langfuse-k8s
    chart: langfuse
    targetRevision: "1.2.3"  # ← EXACT PIN
    helm:
      values: |
        # your values here
```

### Upgrade Workflow

1. **Check releases**: `helm search repo langfuse/langfuse-k8s --versions | head -10`
2. **Read changelog**: Check the GitHub releases page for breaking changes
3. **Bump in staging**: Update `targetRevision` in the staging app
4. **ArgoCD auto-syncs staging** — watch for errors
5. **Soak 24-48h** in staging
6. **Promote to production**: Update `targetRevision` in the production app
7. **Manual sync in ArgoCD** (production should NOT auto-sync)

### Values Pinning

Pin the application image version separately from the chart version:

```yaml
helm:
  values: |
    image:
      tag: "3.42.0"  # ← Application version (may differ from chart version)
```

Chart version and app version are independent. A chart version bump may not change the app version, and vice versa.

---

## npm Pinning

### package.json

```json
{
  "dependencies": {
    "langfuse": "3.42.0",
    "express": "~4.18.2",
    "lodash": "~4.17.21"
  },
  "devDependencies": {
    "typescript": "^5.4.0",
    "eslint": "^9.0.0",
    "vitest": "^3.0.0"
  }
}
```

**Production deps**: exact pin or patch range.  
**Dev deps**: minor range is fine — they don't ship to production.

### Lock Files

**Always commit lock files** (`package-lock.json`, `bun.lockb`, `yarn.lock`). The lock file is the real pin — `package.json` ranges are just constraints. Without the lock file, `npm install` on a different machine may resolve to different versions.

### Renovate / Dependabot

Use automated dependency update tools to create bump PRs:

```json
// renovate.json
{
  "extends": ["config:base"],
  "schedule": ["every month"],
  "packageRules": [
    {
      "matchDepTypes": ["dependencies"],
      "automerge": false,
      "labels": ["dependency-update"]
    },
    {
      "matchDepTypes": ["devDependencies"],
      "automerge": true,
      "labels": ["dev-dependency-update"]
    }
  ]
}
```

Production deps get a PR for review. Dev deps auto-merge.

---

## pip Pinning (Python)

### pyproject.toml

```toml
[project]
dependencies = [
    "langfuse==3.42.0",
    "fastapi>=0.100,<1.0",
    "pydantic>=2.0,<3.0",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.0,<9.0",
    "ruff>=0.8",
    "mypy>=1.20",
]
```

### pip-compile (recommended)

Use `pip-compile` from `pip-tools` to generate a fully-pinned `requirements.txt` from your `pyproject.toml`:

```bash
pip-compile pyproject.toml -o requirements.txt
# Produces: langfuse==3.42.0, fastapi==0.136.1, pydantic==2.13.3, ...
```

This gives you the best of both worlds: loose constraints in `pyproject.toml` (for compatibility) and exact pins in `requirements.txt` (for reproducibility).

---

## Go Module Pinning

Go modules are automatically pinned by `go.sum`. The `go.mod` file specifies minimum versions, and `go.sum` records exact checksums.

```go
// go.mod
require (
    github.com/langfuse/langfuse-go v1.2.3
    github.com/gin-gonic/gin v1.9.1
)
```

Go's MVS (Minimum Version Selection) means you always get the exact version specified, not "the latest that satisfies the constraint." This is the most deterministic pinning model of any package manager.

**Upgrade**: `go get github.com/langfuse/langfuse-go@v1.3.0`

---

## Docker Image Pinning

### Tags vs Digests

```dockerfile
# BAD — tag can be repointed to a different image
FROM python:3.13

# BETTER — specific tag
FROM python:3.13.1-slim

# BEST — digest pin (immutable)
FROM python:3.13.1-slim@sha256:abc123def456...
```

Digest pins are immutable — even if the tag is repointed (supply chain attack), the digest won't match and the build fails.

### When to Use Digests

- **Production Dockerfiles**: always use digest pins
- **CI base images**: use digest pins
- **Dev Dockerfiles**: tag pins are fine (convenience > security for local dev)

### Getting the Digest

```bash
docker pull python:3.13.1-slim
docker inspect --format='{{index .RepoDigests 0}}' python:3.13.1-slim
# → python@sha256:abc123def456...
```

---

## Anti-Patterns

### 1. Pinning in code but floating in CI

```yaml
# BAD: CI installs latest, code expects 3.42.0
- run: pip install langfuse
```

```yaml
# GOOD: CI installs the same version as code
- run: pip install -r requirements.txt
```

### 2. Pinning the chart but not the image

```yaml
# BAD: chart is pinned but image floats
targetRevision: "1.2.3"
helm:
  values: |
    image:
      tag: "latest"  # ← FLOATING
```

### 3. Lock file not committed

If `package-lock.json` or `bun.lockb` is in `.gitignore`, every `npm install` may resolve to different versions. **Always commit lock files.**

### 4. Upgrading without reading the changelog

```bash
# BAD: blind upgrade
helm upgrade langfuse langfuse/langfuse-k8s --version 2.0.0

# GOOD: read first, upgrade second
# 1. Check: https://github.com/langfuse/langfuse-k8s/releases
# 2. Look for BREAKING CHANGES
# 3. Test in staging
# 4. Upgrade production
```

### 5. Pinning to a git commit hash without a tag

```
# BAD: commit hash with no context
github.com/org/repo@abc123def

# GOOD: tagged release
github.com/org/repo@v1.2.3
```

Commit hashes are immutable (good) but have no semantic meaning (bad). You can't tell if `abc123` → `def456` is a patch fix or a breaking change.

---

## Langfuse Specific Guidance

Langfuse releases multiple times per week and uses semver. Recommended approach:

```yaml
# Helm chart: exact pin
targetRevision: "1.2.3"

# Application image: exact pin (may differ from chart version)
image:
  tag: "3.42.0"

# Upgrade cadence: monthly
# Process:
#   1. Check https://github.com/langfuse/langfuse/releases
#   2. Bump chart + image versions in staging values
#   3. ArgoCD auto-syncs staging
#   4. Soak 48h
#   5. Bump production values
#   6. Manual sync in ArgoCD
```

If you need Langfuse patches faster (e.g., security fix), do an out-of-band bump:
1. Read the specific release notes
2. Bump only the affected version
3. Fast-track through staging (4h soak instead of 48h)
4. Promote to production

---

## Traceability

```
axiom:trace spec=specs/06-Project-Configuration.md
```
