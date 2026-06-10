---
name: docker-shellops
description: >
  Docker and container patterns: Dockerfile authoring (multi-stage, UV Python builds,
  AL2023-based), ECR push/pull workflows, BuildKit distributed builds via the
  infra-charts/buildkit-cluster, image tagging conventions (SHA-first), security
  scanning, and GitHub Actions CI integration. Tuned for Dexdat ECR + EKS stack.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/00-PRD.md
  related_skills:
    - github-actions-shellops
    - kubernetes-shellops
    - aws-cli-shellops
    - karpenter-shellops
    - external-secrets-shellops
tags:
  vertical: [devops, containers, docker, ecr]
  category: containers
  core: false
---

# Docker — Axiom Integration Skill

> **"Multi-stage builds. Every time. Dev deps never ship to production."**
> **"Tag with the git SHA. Never with `:latest`."**
> **"The image is the deployment unit. Build it once; promote it through environments."**

This skill covers Docker container patterns for Dexdat: Python service images
(UV-based fast builds), ECR as the registry, BuildKit cluster for distributed CI builds,
and GitHub Actions for the build → push → deploy pipeline.

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
- Writing or reviewing a `Dockerfile` for a Python or FastAPI service
- Setting up ECR push in a GitHub Actions workflow
- Configuring BuildKit cluster for faster parallel builds
- Debugging image pull failures in EKS
- Implementing multi-stage builds to reduce image size
- Setting up image scanning (ECR native or Trivy)
- Adding a new ECR repository via `infra-charts/ecr`

---

## Non-Negotiables

1. **Multi-stage builds always.** Stage 1 builds dependencies and compiles assets.
   Stage 2 is the runtime image — only what the application needs. Dev tools, test
   dependencies, and build artifacts must never appear in the final image.

2. **SHA tag on every push.** The CI pipeline tags images with the full git SHA.
   `latest` is banned in production manifests. Semantic version tags are additive
   aliases, not replacements for the SHA tag.

3. **Non-root user in the final stage.** Production containers must not run as root.
   Create a dedicated user (`RUN useradd -r -u 10001 app`) and `USER app`.

4. **Pin base image digests for production.** `FROM python:3.12-slim` can silently change.
   Use `FROM python:3.12-slim@sha256:<digest>` for production images, or pin via
   Dependabot automation.

5. **Never store secrets in layers.** `ENV SECRET=value` bakes the secret into every
   layer. Use build args only for non-sensitive config; inject secrets at runtime via
   ExternalSecrets / Kubernetes Secret env refs.

6. **BuildKit remote connections require mTLS.** Plain `tcp://` exposes the BuildKit
   daemon to any pod in the cluster. Use `--driver kubernetes` (preferred — no exposed port)
   or pass `--driver-opt "cacert=/ca.pem,cert=/cert.pem,key=/key.pem"` for remote.

---

## Standard Python Dockerfile (UV + multi-stage)

Dexdat uses [UV](https://github.com/astral-sh/uv) for fast Python dependency
management. The pattern below is the standard for all Python services.

```dockerfile
# syntax=docker/dockerfile:1.7
# ── Stage 1: dependency builder ──────────────────────────────────────────────
FROM python:3.12-slim AS builder

# Install UV (fast Python package manager)
COPY --from=ghcr.io/astral-sh/uv:0.5.11 /uv /uvx /usr/local/bin/

WORKDIR /app

# Copy dependency files first (layer cache — only re-runs if deps change)
COPY pyproject.toml uv.lock ./

# Install dependencies into a virtual environment
# --frozen: fail if uv.lock is out of date
# --no-dev: exclude dev/test dependencies from production image
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-install-project

# Copy application source
COPY src/ ./src/

# Install the project itself
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev

# ── Stage 2: runtime image ────────────────────────────────────────────────────
FROM python:3.12-slim AS runtime

# Security: non-root user
RUN useradd -r -u 10001 -g root app

WORKDIR /app

# Copy only the venv and app from builder (no UV, no pip, no build tools)
COPY --from=builder --chown=app:root /app/.venv ./.venv
COPY --from=builder --chown=app:root /app/src ./src

# Activate venv in PATH
ENV PATH="/app/.venv/bin:$PATH"
ENV PYTHONUNBUFFERED=1
ENV PYTHONDONTWRITEBYTECODE=1

# Run as non-root
USER app

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')"

CMD ["uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8000"]
```

---

## FastAPI + UV Template

```dockerfile
# syntax=docker/dockerfile:1.7
FROM python:3.12-slim AS builder
COPY --from=ghcr.io/astral-sh/uv:0.5.11 /uv /usr/local/bin/

WORKDIR /app
COPY pyproject.toml uv.lock ./
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-install-project
COPY . .
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev

FROM python:3.12-slim AS runtime
RUN useradd -r -u 10001 -g root app
WORKDIR /app
COPY --from=builder --chown=app:root /app/.venv ./.venv
COPY --from=builder --chown=app:root /app/src ./src
ENV PATH="/app/.venv/bin:$PATH" PYTHONUNBUFFERED=1
USER app
EXPOSE 8080
CMD ["uvicorn", "src.app:app", "--host", "0.0.0.0", "--port", "8080", "--workers", "2"]
```

---

## ECR Workflow

### Repository Setup (infra-charts/ecr)

ECR repositories are managed via Helm chart — not created manually:

```yaml
# infra-charts/ecr/values.yaml addition
repositories:
  my-service:
    name: my-service
    imageScanOnPush: true          # Auto-scan every pushed image
    lifecyclePolicy:
      rules:
        - rulePriority: 1
          description: "Keep last 20 tagged images"
          selection:
            tagStatus: tagged
            tagPrefixList: ["v", "sha-"]
            countType: imageCountMoreThan
            countNumber: 20
          action: { type: expire }
        - rulePriority: 2
          description: "Expire untagged after 7 days"
          selection:
            tagStatus: untagged
            countType: sinceImagePushed
            countUnit: days
            countNumber: 7
          action: { type: expire }
```

### Build and Push (GitHub Actions — SHA-tagged)

```bash
# Local build + push pattern (mirrors what CI does)
ACCOUNT="${AWS_ACCOUNT_ID}"
REGION="us-east-1"
REPO="my-service"
SHA=$(git rev-parse HEAD)
TAG="sha-${SHA:0:12}"

# Authenticate to ECR (OIDC in CI; profile locally)
aws ecr get-login-password --region $REGION \
  | docker login --username AWS --password-stdin \
    "${ACCOUNT}.dkr.ecr.${REGION}.amazonaws.com"

# Build with BuildKit
DOCKER_BUILDKIT=1 docker build \
  --target runtime \
  --tag "${ACCOUNT}.dkr.ecr.${REGION}.amazonaws.com/${REPO}:${TAG}" \
  --cache-from "type=registry,ref=${ACCOUNT}.dkr.ecr.${REGION}.amazonaws.com/${REPO}:cache" \
  --cache-to "type=registry,ref=${ACCOUNT}.dkr.ecr.${REGION}.amazonaws.com/${REPO}:cache,mode=max" \
  .

docker push "${ACCOUNT}.dkr.ecr.${REGION}.amazonaws.com/${REPO}:${TAG}"
```

### GitHub Actions — Full Build + ECR Push

```yaml
# .github/workflows/build.yaml
jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      id-token: write
      contents: read
    outputs:
      image-tag: ${{ steps.meta.outputs.version }}

    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683

      - name: Configure AWS (OIDC — no static keys)
        uses: aws-actions/configure-aws-credentials@ececc8c814d2f5765dfb4ca9a42a2e30a3c42d00
        with:
          role-to-assume: arn:aws:iam::${AWS_ACCOUNT_ID}:role/github-actions-ecr-push
          aws-region: us-east-1

      - name: Login to ECR
        id: ecr
        uses: aws-actions/amazon-ecr-login@062b18b96a7aff071d4dc91bc00c4c1a7945b076

      - name: Docker meta (SHA + branch tags)
        id: meta
        uses: docker/metadata-action@902fa8ec7d6ecbae3b421b0ea9e9e80f978f4e3b
        with:
          images: ${{ steps.ecr.outputs.registry }}/my-service
          tags: |
            type=sha,prefix=sha-,format=short    # sha-abc1234 — primary tag
            type=ref,event=branch                # main, release/v2
            type=semver,pattern={{version}}       # v2.1.5 if tagged

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@b5730b58c35571f454d92bc89da60e6f3be64d94

      - name: Build and push
        uses: docker/build-push-action@263435318d21b6f5a68d84b62af4a08b09b68a12
        with:
          context: .
          target: runtime                        # Final stage only
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          cache-from: type=gha                   # GitHub Actions cache
          cache-to: type=gha,mode=max
          provenance: true                       # SBOM attestation
          sbom: true
```

---

## BuildKit Cluster (infra-charts/buildkit-cluster)

For large images or parallel builds, Dexdat uses a BuildKit cluster running on dedicated
Karpenter spot nodes (`buildkit` nodepool). CI routes to this cluster via `--builder`:

```bash
# Point Docker to the in-cluster BuildKit daemon
docker buildx create \
  --name dexdat-buildkit \
  --driver remote \
  tcp://buildkit.buildkit.svc.cluster.local:1234

docker buildx use dexdat-buildkit

# Build using the cluster (much faster for large Python images)
docker buildx build \
  --builder dexdat-buildkit \
  --platform linux/amd64 \
  --push \
  --tag "${AWS_ACCOUNT_ID}.dkr.ecr.us-east-1.amazonaws.com/my-service:sha-abc1234" \
  .
```

---

## Image Scanning

```bash
# ECR native scan results (imageScanOnPush: true in ecr chart)
aws ecr describe-image-scan-findings \
  --repository-name my-service \
  --image-id imageTag=sha-abc1234 \
  --region us-east-1 \
  --output json | tee .memory-bank/work-items/${WORK_ITEM_ID}/ecr-scan.json

# Check for CRITICAL/HIGH findings
aws ecr describe-image-scan-findings \
  --repository-name my-service \
  --image-id imageTag=sha-abc1234 \
  --region us-east-1 \
  --query 'imageScanFindings.findings[?severity==`CRITICAL` || severity==`HIGH`]' \
  --output json

# Local scan with Trivy (faster feedback in dev)
trivy image \
  --severity CRITICAL,HIGH \
  --exit-code 1 \
  "${AWS_ACCOUNT_ID}.dkr.ecr.us-east-1.amazonaws.com/my-service:sha-abc1234"
```

---

## Debugging Image Pull Failures in EKS

```bash
# ImagePullBackOff — get the actual error
kubectl describe pod <pod> -n <namespace> | grep -A 10 "Events:"

# Common causes:
# 1. ECR auth expired — nodes use instance profile but token needs refresh
kubectl get secret ecr-registry-credentials -n <namespace>
# Check ExternalSecret is syncing (refreshInterval: 4h for ECR tokens)

# 2. Image tag doesn't exist in ECR
aws ecr describe-images \
  --repository-name my-service \
  --image-ids imageTag=sha-abc1234 \
  --region us-east-1

# 3. Wrong region in image URI
# Should be: ${AWS_ACCOUNT_ID}.dkr.ecr.us-east-1.amazonaws.com/...
# NOT: ${AWS_ACCOUNT_ID}.dkr.ecr.eu-west-1.amazonaws.com/...

# 4. Cross-account ECR pull (console cluster pulling from another account)
# Check ecr/templates/eks_pull/ for cross-account pull permissions
kubectl get pod <pod> -n <namespace> \
  -o jsonpath='{.spec.serviceAccountName}'
# Verify SA has correct IRSA role with ecr:GetAuthorizationToken
```

---

## .dockerignore (always present)

```
# .dockerignore — keeps build context small and avoids leaking secrets
.git
.gitignore
.env*
*.env
**/.env
**/__pycache__/
**/*.pyc
**/*.pyo
.pytest_cache/
.mypy_cache/
.ruff_cache/
htmlcov/
.coverage
dist/
*.egg-info/
node_modules/
.memory-bank/
_tmp/
docs/
*.md
!README.md
tests/
```

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| `FROM python:3.12` (full image) | 1.2GB base; slow pulls; large attack surface | `python:3.12-slim` or `python:3.12-alpine` |
| Single-stage build | Dev deps in prod; large image; slow pulls | Multi-stage: builder + runtime |
| `COPY . .` before dependency install | Invalidates dep cache on every code change | Copy `pyproject.toml`/`requirements.txt` first |
| `:latest` tag in K8s manifests | Non-deterministic; can't rollback | SHA tag: `sha-abc1234` |
| `ENV SECRET_KEY=supersecret` | Baked into image layer; leaked in `docker history` | Inject at runtime via K8s Secret |
| Running as root (`USER root` or no USER) | Container escape has root on host | `RUN useradd -r app && USER app` |
| No `.dockerignore` | `.git`, `.env`, `__pycache__` in build context; slow + leaky | Always add `.dockerignore` |
| `pip install` without `--no-cache-dir` | pip cache bloats image by ~100MB | `pip install --no-cache-dir` or use UV |
| `imageScanOnPush: false` | Vulnerable images deployed silently | Enable scan; block on CRITICAL in CI |

---

## axiom:trace

`axiom:trace work_item=devops-skills-01 spec=specs/00-PRD.md plan= prompt=.opencode/skills/docker-shellops/SKILL.md evidence= doc= ops= commit=`
