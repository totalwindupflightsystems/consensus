---
name: github-actions-shellops
description: >
  GitHub Actions workflow patterns: CI/CD pipeline design, reusable workflows, composite
  actions, matrix builds, secrets and variables management, environment gates, OIDC for
  AWS/cloud auth (no long-lived keys), caching strategies, and Axiom traceability
  integration. Load this skill when writing or reviewing GitHub Actions workflows.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/00-PRD.md
  related_skills:
    - docker-shellops
    - aws-cli-shellops
    - terraform-shellops
    - argocd-shellops
    - helm-shellops
    - version-pinning-axiom
tags:
  vertical: [devops, ci-cd, github]
  category: ci-cd
  core: false
---

# GitHub Actions — Axiom Integration Skill

> **"Pin action versions with full SHA. A tag can be moved; a SHA cannot."**
> **"OIDC instead of long-lived AWS keys. No exceptions in production CI."**
> **"Every workflow that deploys must have a matching rollback workflow."**

This skill covers GitHub Actions CI/CD patterns for Axiom workflows. It covers secure
OIDC-based cloud auth, workflow structure, reusable components, and evidence capture
that satisfies Axiom verification gates.

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
- Writing new CI/CD workflows for application builds or deployments
- Pinning action versions for security (SHA-pinning)
- Setting up OIDC-based AWS authentication (replacing static keys)
- Creating reusable workflows or composite actions
- Configuring environment protection gates (required approvals)
- Implementing matrix builds for multi-platform or multi-version testing
- Setting up caching for dependencies (npm, pip, go, docker layers)
- Wiring GitHub Actions to ArgoCD, Helm, or Terraform

---

## Non-Negotiables

1. **Pin actions to SHA, not tag.** `uses: actions/checkout@v4` can be changed by
   the action author. `uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683`
   cannot. Pin all third-party actions to a full commit SHA.

2. **Use OIDC for AWS auth.** Never store `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`
   as GitHub Secrets for production CI. Use IAM OIDC + `aws-actions/configure-aws-credentials`.

3. **Secrets never in logs.** Never `echo $SECRET` or print env vars. Use
   `::add-mask::` for dynamic values that must be masked.

4. **Environment gates for production deploys.** Production deploy jobs must have
   `environment: production` with required reviewers configured in GitHub settings.

5. **Minimum permissions.** Set `permissions:` at the workflow or job level.
   Never use the implicit default (write-all).

---

## Workflow Permissions (always explicit)

```yaml
# Set at workflow level (applies to all jobs unless overridden)
permissions:
  contents: read          # Read repo contents
  id-token: write         # Required for OIDC
  packages: read          # Read GitHub Packages

# OR set per-job (preferred for strict control)
jobs:
  deploy:
    permissions:
      id-token: write
      contents: read
```

---

## Standard CI Pipeline

```yaml
# .github/workflows/ci.yaml
name: CI

on:
  push:
    branches: [main, 'release/**']
  pull_request:
    branches: [main]

permissions:
  contents: read
  id-token: write         # OIDC for AWS ECR push

env:
  PYTHON_VERSION: "3.12"
  AWS_REGION: us-east-1
  ECR_REPOSITORY: my-app

jobs:
  # ── Lint and Test ───────────────────────────────────────────────────
  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683  # v4 SHA-pinned

      - uses: actions/setup-python@0b93645e9fea7318ecaed2b359559ac225c90a2b  # v5
        with:
          python-version: ${{ env.PYTHON_VERSION }}

      - name: Cache pip packages
        uses: actions/cache@5a3ec84eff668545956fd18022155c47e93e2684  # v4
        with:
          path: ~/.cache/pip
          key: ${{ runner.os }}-pip-${{ hashFiles('**/requirements*.txt') }}
          restore-keys: |
            ${{ runner.os }}-pip-

      - name: Install dependencies
        run: pip install -r requirements.txt -r requirements-dev.txt

      - name: Lint
        run: |
          ruff check .
          mypy src/

      - name: Test
        run: pytest tests/ -v --tb=short --junit-xml=test-results.xml

      - name: Upload test results
        if: always()
        uses: actions/upload-artifact@ea165f8d65b6e75b540449f83ef25dc3f6a28cd6  # v4
        with:
          name: test-results
          path: test-results.xml

  # ── Build and Push Docker Image ─────────────────────────────────────
  build:
    name: Build
    runs-on: ubuntu-latest
    needs: test
    if: github.ref == 'refs/heads/main' || startsWith(github.ref, 'refs/heads/release/')
    outputs:
      image-tag: ${{ steps.meta.outputs.tags }}
      image-digest: ${{ steps.build.outputs.digest }}

    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683

      # OIDC AWS auth — no static keys
      - name: Configure AWS credentials (OIDC)
        uses: aws-actions/configure-aws-credentials@ececc8c814d2f5765dfb4ca9a42a2e30a3c42d00  # v4
        with:
          role-to-assume: arn:aws:iam::${{ secrets.AWS_ACCOUNT_ID }}:role/github-actions-ecr-push
          aws-region: ${{ env.AWS_REGION }}

      - name: Login to Amazon ECR
        id: login-ecr
        uses: aws-actions/amazon-ecr-login@062b18b96a7aff071d4dc91bc00c4c1a7945b076  # v2

      - name: Docker meta
        id: meta
        uses: docker/metadata-action@902fa8ec7d6ecbae3b421b0ea9e9e80f978f4e3b  # v5
        with:
          images: ${{ steps.login-ecr.outputs.registry }}/${{ env.ECR_REPOSITORY }}
          tags: |
            type=sha,format=long
            type=ref,event=branch
            type=semver,pattern={{version}}

      - name: Build and push
        id: build
        uses: docker/build-push-action@263435318d21b6f5a68d84b62af4a08b09b68a12  # v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
          provenance: true        # SBOM and provenance attestation

      - name: Output image digest for tracing
        run: |
          echo "Image digest: ${{ steps.build.outputs.digest }}"
          echo "image_digest=${{ steps.build.outputs.digest }}" >> $GITHUB_STEP_SUMMARY
```

---

## Environment Gates for Production

```yaml
# .github/workflows/deploy-production.yaml
name: Deploy to Production

on:
  workflow_dispatch:
    inputs:
      image-tag:
        description: 'Image tag to deploy'
        required: true

jobs:
  deploy:
    name: Deploy Production
    runs-on: ubuntu-latest
    environment: production    # ← Requires manual approval in GitHub settings

    permissions:
      id-token: write
      contents: read

    steps:
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@ececc8c814d2f5765dfb4ca9a42a2e30a3c42d00
        with:
          role-to-assume: arn:aws:iam::${{ secrets.AWS_ACCOUNT_ID }}:role/github-actions-deploy
          aws-region: us-east-1

      - name: Update kubeconfig
        run: |
          aws eks update-kubeconfig \
            --name my-prod-cluster \
            --region us-east-1

      - name: Deploy with Helm
        run: |
          helm upgrade my-app ./charts/my-app \
            -n my-app \
            -f charts/my-app/values-production.yaml \
            --set image.tag="${{ github.event.inputs.image-tag }}" \
            --set codeopsWorkItem="${{ github.run_id }}" \
            --wait \
            --timeout 5m

      - name: Smoke test
        run: |
          kubectl rollout status deployment/my-app -n my-app
          curl -sf https://api.dexdat.ai/health

      - name: Summary
        run: |
          echo "## Deploy Summary" >> $GITHUB_STEP_SUMMARY
          echo "- **Image**: ${{ github.event.inputs.image-tag }}" >> $GITHUB_STEP_SUMMARY
          echo "- **Environment**: production" >> $GITHUB_STEP_SUMMARY
          echo "- **Triggered by**: ${{ github.actor }}" >> $GITHUB_STEP_SUMMARY
          echo "- **Run ID**: ${{ github.run_id }}" >> $GITHUB_STEP_SUMMARY
```

---

## Reusable Workflows

```yaml
# .github/workflows/_deploy.yaml (reusable workflow — starts with _)
name: Deploy (Reusable)

on:
  workflow_call:
    inputs:
      environment:
        required: true
        type: string
      image-tag:
        required: true
        type: string
      helm-values-file:
        required: true
        type: string
    secrets:
      AWS_ACCOUNT_ID:
        required: true

jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: ${{ inputs.environment }}
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@ececc8c814d2f5765dfb4ca9a42a2e30a3c42d00
        with:
          role-to-assume: arn:aws:iam::${{ secrets.AWS_ACCOUNT_ID }}:role/github-actions-deploy-${{ inputs.environment }}
          aws-region: us-east-1
      # ... rest of deploy steps
```

```yaml
# Calling the reusable workflow
jobs:
  deploy-staging:
    uses: ./.github/workflows/_deploy.yaml
    with:
      environment: staging
      image-tag: ${{ needs.build.outputs.image-tag }}
      helm-values-file: values-staging.yaml
    secrets:
      AWS_ACCOUNT_ID: ${{ secrets.AWS_ACCOUNT_ID }}
```

---

## OIDC Trust Policy (AWS side)

```json
// IAM Role Trust Policy — allows GitHub Actions to assume this role
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::123456789012:oidc-provider/token.actions.githubusercontent.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
        },
        "StringLike": {
          "token.actions.githubusercontent.com:sub": "repo:<YOUR_GITHUB_ORG>/<REPO_NAME>:environment:production"
        }
      }
    }
  ]
}
```

---

## Matrix Builds (multi-version / multi-arch testing)

```yaml
jobs:
  test:
    strategy:
      fail-fast: false
      matrix:
        python-version: ["3.10", "3.11", "3.12"]
        os: [ubuntu-latest, macos-latest]

    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683
      - uses: actions/setup-python@0b93645e9fea7318ecaed2b359559ac225c90a2b
        with:
          python-version: ${{ matrix.python-version }}
      - run: pytest tests/
```

---

## Dependabot for Action Version Updates

```yaml
# .github/dependabot.yaml
version: 2
updates:
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
    groups:
      actions:
        patterns:
          - "actions/*"
          - "aws-actions/*"
          - "docker/*"
```

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| `uses: actions/checkout@v4` (tag not SHA) | Tag can be re-pointed to malicious commit | Pin to full SHA; use Dependabot to update |
| Static `AWS_ACCESS_KEY_ID` in secrets | Long-lived keys; key rotation burden | OIDC with IAM role assumption |
| `permissions: write-all` (implicit default) | Over-privileged; supply chain attack surface | Explicit minimal permissions per job |
| `echo ${{ secrets.MY_SECRET }}` | Prints secret in logs | GitHub auto-masks but avoid printing secrets |
| No environment gate on production deploys | Any push merges can auto-deploy prod | `environment: production` with required reviewers |
| Running `npm install` without cache | Slow CI; package download on every run | `actions/cache` keyed on lockfile hash |
| `continue-on-error: true` on security gates | Silently passes security failures | Never on security/lint/test steps |
| Self-hosted runners without security hardening | Shared state between runs; secret exposure | Ephemeral self-hosted runners; no persistent state |

---

## axiom:trace

`axiom:trace work_item=devops-skills-01 spec=specs/00-PRD.md plan= prompt=.opencode/skills/github-actions-shellops/SKILL.md evidence= doc= ops= commit=`
