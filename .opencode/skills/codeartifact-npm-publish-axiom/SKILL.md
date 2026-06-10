---
tags:
  vertical: [ops, coding]
  category: operations
  core: false
---

# Skill: CodeArtifact npm Package Publishing (TypeScript/Bun)

**Skill ID**: `codeartifact-npm-publish-axiom`  
**Load when**: Setting up a new TypeScript package publish pipeline to the org's CodeArtifact registry, or debugging a failing publish workflow.

---

## Quick Reference

| Item | Value |
|------|-------|
| Shared action | `fl97inc/dexdat-actions/deploy-typescript-lib/codeartifact@v2` |
| IAM role | `arn:aws:iam::904233114280:role/houston-github-code-artifact-push` |
| Domain | `dexdat-prod` |
| Repository | `npm-prod` |
| Region | `us-east-1` |
| Registry URL | `https://dexdat-prod-904233114280.d.codeartifact.us-east-1.amazonaws.com/npm/npm-prod/` |
| Auth | OIDC — `id-token: write` permission, no stored secrets |
| Reference repo | `fl97inc/dexdat-flyte-execution-utils-ts` |

---

## Load-Trigger Table

Load this skill when any of these conditions are true:

| Condition | Action |
|-----------|--------|
| Setting up a new TypeScript package publish to CodeArtifact | Load and follow §Workflow Template |
| `bun install` fails with 401 in CI | Load and see §Gotcha 1 |
| `npm publish` fails with auth error | Load and see §Gotcha 3 |
| Asking "how do we publish to the internal registry?" | Load and share §Quick Reference + §Workflow Template |
| Adding `publishConfig` to `package.json` | Load and see §package.json Setup |
| Debugging a CodeArtifact publish failure | Load and see §Troubleshooting |

---

## Workflow Template (copy-paste ready)

Save as `.github/workflows/publish-codeartifact.yml`:

```yaml
# Publish [PACKAGE_NAME] to CodeArtifact (internal npm-prod registry)
# Uses fl97inc/dexdat-actions/deploy-typescript-lib/codeartifact@v2
# Auth: OIDC → IAM role (no stored secrets; id-token: write required)
#
# Triggers: GitHub release published, or manual workflow_dispatch with dry-run option.

name: publish-codeartifact

on:
  release:
    types: [published]
  workflow_dispatch:
    inputs:
      dry-run:
        description: "Dry run (build + pack only, skip publish)"
        required: false
        default: "false"
        type: choice
        options: ["true", "false"]

permissions:
  id-token: write   # Required for OIDC → IAM role assumption
  contents: read

jobs:
  build-and-publish:
    name: Build & Publish to CodeArtifact
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: [PACKAGE_DIR]   # e.g. .axiom/plugin or .

    steps:
      - uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7

      - name: Set up Bun
        uses: oven-sh/setup-bun@735343b943e7a5450a09e42a3f29b5ffccc5392a # v2.0.2
        with:
          bun-version: "1.1.45"

      # IMPORTANT: BUN_CONFIG_REGISTRY override prevents 401 from private registry
      # during bun install. See Gotcha 1 below.
      - name: Install dependencies
        run: BUN_CONFIG_REGISTRY=https://registry.npmjs.org bun install

      - name: Typecheck
        run: bun run typecheck

      - name: Run tests
        run: bun test

      # IMPORTANT: dist/ is gitignored — must rebuild on every CI run.
      - name: Build
        run: bun run build

      - name: Verify package name and version
        run: |
          PKG_NAME=$(node -e "console.log(require('./package.json').name)")
          PKG_VER=$(node -e "console.log(require('./package.json').version)")
          echo "✅ Publishing: $PKG_NAME@$PKG_VER"

      - name: Dry run (pack only)
        if: ${{ github.event.inputs.dry-run == 'true' }}
        run: |
          npm pack --dry-run
          echo "Dry run complete — skipping publish"

      - name: Publish to CodeArtifact
        if: ${{ github.event_name == 'release' || (github.event_name == 'workflow_dispatch' && github.event.inputs.dry-run == 'false') }}
        uses: fl97inc/dexdat-actions/deploy-typescript-lib/codeartifact@v2
        with:
          package-path: "[PACKAGE_DIR]"   # same as working-directory above
          aws-role: arn:aws:iam::904233114280:role/houston-github-code-artifact-push
```

**Replace**:
- `[PACKAGE_NAME]` — your package name (e.g., `opencode-axiom`)
- `[PACKAGE_DIR]` — path to the package directory (e.g., `.axiom/plugin` or `.`)

---

## package.json Setup

Add `publishConfig` to prevent accidental public npm publishes:

```json
{
  "name": "your-package-name",
  "version": "1.0.0",
  "publishConfig": {
    "registry": "https://dexdat-prod-904233114280.d.codeartifact.us-east-1.amazonaws.com/npm/npm-prod/"
  }
}
```

---

## How the Shared Action Works

The `fl97inc/dexdat-actions/deploy-typescript-lib/codeartifact@v2` action does:

```bash
# 1. Get a 12-hour auth token
NPM_PASSWORD=$(aws codeartifact get-authorization-token \
  --domain dexdat-prod --region us-east-1 \
  --query authorizationToken --output text)

# 2. Get the registry endpoint
NPM_REPOSITORY_URL=$(aws codeartifact get-repository-endpoint \
  --domain dexdat-prod --region us-east-1 \
  --repository npm-prod --format npm --output text)

# 3. Configure npm
npm config set registry=${NPM_REPOSITORY_URL}
npm config set ${NPM_REPOSITORY_URL:6}:_authToken=${NPM_PASSWORD}

# 4. Publish
npm publish
```

The IAM role assumption happens via OIDC — GitHub's OIDC provider issues a JWT, which AWS exchanges for temporary credentials. No stored secrets needed.

---

## Gotchas

### Gotcha 1: `bun install` 401 in CI

**Symptom**: `bun install` fails with `401` on `@opencode-ai/plugin` or another private package.

**Cause**: The `.opencode/` directory contains a `package.json` that references the private CodeArtifact registry. When `bun install` runs, it tries to resolve all packages from that registry, including ones that need a fresh auth token.

**Fix**: Always prefix `bun install` with the public registry override:
```bash
BUN_CONFIG_REGISTRY=https://registry.npmjs.org bun install
```

### Gotcha 2: `dist/` not found

**Symptom**: `npm publish` succeeds but the published package is empty, or `ERR_MODULE_NOT_FOUND` when installing.

**Cause**: `dist/` is in `.gitignore`. A fresh checkout has no `dist/` directory.

**Fix**: Always run `bun run build` before the publish step. Never skip the build step.

### Gotcha 3: `npm publish` auth error

**Symptom**: `npm publish` fails with `403 Forbidden` or `401 Unauthorized`.

**Cause**: Usually one of:
- The `package-path` input is wrong (action is running `npm publish` from the wrong directory)
- The `publishConfig.registry` in `package.json` doesn't match the CodeArtifact URL
- The IAM role doesn't have `codeartifact:PublishPackageVersion` permission

**Fix**:
1. Verify `package-path` matches the directory containing `package.json`
2. Check `publishConfig.registry` matches the CodeArtifact endpoint exactly
3. Verify the IAM role has the required CodeArtifact permissions

### Gotcha 4: Version already exists

**Symptom**: `npm publish` fails with `409 Conflict` — version already published.

**Cause**: CodeArtifact (like npm) does not allow republishing the same version.

**Fix**: Bump the version in `package.json` before tagging a new release. Use semantic versioning.

### Gotcha 5: `package-path` vs `working-directory`

The `package-path` input to the shared action is the path **from the repo root** to the package directory. It is NOT relative to the workflow's `defaults.run.working-directory`. Always use the repo-root-relative path.

```yaml
# ✅ CORRECT — repo-root-relative
- uses: fl97inc/dexdat-actions/deploy-typescript-lib/codeartifact@v2
  with:
    package-path: ".axiom/plugin"

# ❌ WRONG — relative to working-directory (if working-directory is .axiom/plugin)
- uses: fl97inc/dexdat-actions/deploy-typescript-lib/codeartifact@v2
  with:
    package-path: "."
```

---

## Installing from CodeArtifact (Consumer Side)

When another repo needs to install a package from CodeArtifact:

### With npm

```bash
# Get token and configure
export NPM_TOKEN=$(aws codeartifact get-authorization-token \
  --domain dexdat-prod --region us-east-1 \
  --query authorizationToken --output text)
npm config set registry https://dexdat-prod-904233114280.d.codeartifact.us-east-1.amazonaws.com/npm/npm-prod/
npm config set //dexdat-prod-904233114280.d.codeartifact.us-east-1.amazonaws.com/npm/npm-prod/:_authToken=$NPM_TOKEN
npm install opencode-axiom
```

### With Bun

```bash
BUN_CONFIG_REGISTRY=https://dexdat-prod-904233114280.d.codeartifact.us-east-1.amazonaws.com/npm/npm-prod/ \
  bun install opencode-axiom
```

### In `.npmrc` (for repos that always use CodeArtifact)

```ini
registry=https://dexdat-prod-904233114280.d.codeartifact.us-east-1.amazonaws.com/npm/npm-prod/
//dexdat-prod-904233114280.d.codeartifact.us-east-1.amazonaws.com/npm/npm-prod/:_authToken=${CODEARTIFACT_TOKEN}
```

---

## Troubleshooting Checklist

- [ ] `permissions: id-token: write` is set in the workflow
- [ ] `BUN_CONFIG_REGISTRY=https://registry.npmjs.org bun install` (not bare `bun install`)
- [ ] `bun run build` runs before the publish step
- [ ] `package-path` is repo-root-relative (e.g., `.axiom/plugin`, not `.`)
- [ ] `package.json` version is bumped (no 409 conflict)
- [ ] `publishConfig.registry` matches the CodeArtifact URL exactly
- [ ] The IAM role is `arn:aws:iam::904233114280:role/houston-github-code-artifact-push`

## GitHub Packages (npm.pkg.github.com)

GitHub Packages is a second publish target alongside CodeArtifact and public npm. It requires no extra secrets — `GITHUB_TOKEN` is built-in to every Actions run.

### Key differences from public npm

| Item | Public npm | GitHub Packages |
|------|-----------|-----------------|
| Registry | `registry.npmjs.org` | `npm.pkg.github.com` |
| Package name | `opencode-axiom` | `@fl97inc/opencode-axiom` (scoped) |
| Auth | `NPM_TOKEN` secret | `GITHUB_TOKEN` (built-in) |
| Provenance | `--provenance` flag | Not supported |
| Access | Public | Org members + PAT for external |

### Scoped name requirement

GitHub Packages **requires** a scoped package name matching the org: `@fl97inc/opencode-axiom`. Since the public npm name must stay unscoped (`opencode-axiom`), we patch `package.json` in-place at publish time using a Node.js one-liner — the repo file is never changed:

```yaml
- name: Scope package name for GitHub Packages
  run: |
    node -e "
      const fs = require('fs');
      const pkg = JSON.parse(fs.readFileSync('package.json', 'utf8'));
      pkg.name = '@fl97inc/opencode-axiom';
      pkg.publishConfig = { registry: 'https://npm.pkg.github.com' };
      fs.writeFileSync('package.json', JSON.stringify(pkg, null, 2) + '\n');
      console.log('Scoped: opencode-axiom → @fl97inc/opencode-axiom');
    "
  working-directory: .axiom/plugin

- name: Publish to GitHub Packages
  run: npm publish --access public
  working-directory: .axiom/plugin
  env:
    NODE_AUTH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Installing from GitHub Packages

```bash
# One-time: authenticate
npm login --registry=https://npm.pkg.github.com --scope=@fl97inc

# Install
npm install @fl97inc/opencode-axiom --registry https://npm.pkg.github.com
```

Or add to `.npmrc`:
```ini
@fl97inc:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}
```

### Build-once pattern

When publishing to multiple registries, build once and share the artifact:

```yaml
jobs:
  build:
    steps:
      - name: Build
        run: bun run build
      - uses: actions/upload-artifact@v4
        with:
          name: plugin-dist
          path: .axiom/plugin/dist/

  publish-npm:
    needs: build
    steps:
      - uses: actions/download-artifact@v4
        with: { name: plugin-dist, path: .axiom/plugin }
      - run: npm publish --provenance --access public
        env: { NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }} }

  publish-github-packages:
    needs: build
    steps:
      - uses: actions/download-artifact@v4
        with: { name: plugin-dist, path: .axiom/plugin }
      - run: npm publish --access public
        env: { NODE_AUTH_TOKEN: ${{ secrets.GITHUB_TOKEN }} }
```

---

## References

- [Shared action source](https://github.com/dexdat/dexdat-actions/blob/main/deploy-typescript-lib/codeartifact/action.yml)
- [Reference workflow: dexdat-flyte-execution-utils-ts](https://github.com/dexdat/dexdat-flyte-execution-utils-ts/blob/main/.github/workflows/publish_lib.yml)
- [Best Practice: codeartifact-npm-publish](../../../.memory-bank/best-practices/codeartifact-npm-publish.md)
- [Finding: 2026-04-04-codeartifact-npm-publish-pattern](../../../.memory-bank/findings/2026-04-04-codeartifact-npm-publish-pattern.md)
- [Spec: REQ-PLG-001](../../../specs/70-OpenCode-Plugin.md)
