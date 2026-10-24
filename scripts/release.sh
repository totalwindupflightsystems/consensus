#!/usr/bin/env bash
# scripts/release.sh — release preflight for totalwindupflightsystems/consensus.
#
# Verifies, in order, that the working tree can produce a clean, reproducible
# v${VERSION} cut BEFORE anything is tagged:
#
#   1. tree clean                  — no uncommitted changes, staged or not
#   2. remote parity 0/0           — HEAD == origin/main (no unpushed/unpulled work)
#   3. VERSION == newest numeric
#      CHANGELOG heading           — the cut tag must match the promoted section
#   4. CI green on HEAD            — a real successful GitHub Actions run for the
#                                    exact HEAD SHA (branch labels are not trusted)
#
# `--dry-run` (the default for verification) stops after the checks and prints
# the exact annotated-tag command; it never creates tags or pushes. Run without
# the flag to also create the local annotated tag (release.yml does the rest on
# `git push origin v<VERSION>`).
#
# Fails closed: every refusal names the failing check and exits non-zero.
# Requires: git, gh (authenticated), awk, and coreutils. See docs/release.md
# for the full cut procedure and rollback.
#
# Usage:
#   bash scripts/release.sh --dry-run   # preflight only (safe, no tag)
#   bash scripts/release.sh             # preflight + create the annotated tag

set -euo pipefail

# --- Resolve repository identity from the git remote (no hardcoding). -------
REMOTE_URL="$(git remote get-url origin)"
case "$REMOTE_URL" in
  git@github.com:*) REPO_SLUG="${REMOTE_URL#git@github.com:}" ;;
  https://github.com/*) REPO_SLUG="${REMOTE_URL#https://github.com/}" ;;
  *)
    echo "REFUSED: check 'remote origin' — unsupported URL: $REMOTE_URL (expected a github.com remote)" >&2
    exit 1
    ;;
esac
REPO_SLUG="${REPO_SLUG%.git}"

# --- Reject options we do not implement (fail closed on typos). -------------
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    *)
      echo "REFUSED: check 'arguments' — unknown option: $arg (supported: --dry-run)" >&2
      exit 1
      ;;
  esac
done

# --- Required tools ----------------------------------------------------------
for cmd in git gh awk; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "REFUSED: check 'required tooling' — '$cmd' is not installed or not on PATH" >&2
    exit 1
  fi
done

# --- Repo root (script must run inside a checkout) ---------------------------
if ! REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)"; then
  echo "REFUSED: check 'repository' — not inside a git work tree" >&2
  exit 1
fi
cd "$REPO_ROOT"

# --- Preflight 1: clean tree --------------------------------------------------
if ! git status --porcelain --untracked-files=normal | grep -q .; then
  echo "OK  [1/4] clean tree"
else
  echo "REFUSED: check 'clean tree' failed — working tree has uncommitted changes:" >&2
  git status --porcelain --untracked-files=normal | sed 's/^/    /' >&2
  echo "    Commit or stash, then re-run. A release tag must name a committed state." >&2
  exit 1
fi

# --- Preflight 2: remote parity 0/0 ------------------------------------------
git fetch origin main >/dev/null 2>&1 || {
  echo "REFUSED: check 'remote parity' failed — 'git fetch origin main' errored (network or auth?)" >&2
  exit 1
}
HEAD_SHA="$(git rev-parse HEAD)"
MAIN_SHA="$(git rev-parse origin/main 2>/dev/null || echo 'unresolvable')"
if [ "$HEAD_SHA" = "$MAIN_SHA" ]; then
  echo "OK  [2/4] remote parity 0/0 (HEAD == origin/main at ${HEAD_SHA:0:12})"
else
  AHEAD="$(git rev-list --count "origin/main..HEAD" 2>/dev/null || echo '?')"
  BEHIND="$(git rev-list --count "HEAD..origin/main" 2>/dev/null || echo '?')"
  echo "REFUSED: check 'remote parity' failed — HEAD $HEAD_SHA != origin/main $MAIN_SHA (ahead: $AHEAD, behind: $BEHIND)" >&2
  echo "    Push your commits or pull main until HEAD == origin/main, then re-run." >&2
  exit 1
fi

# --- Preflight 3: VERSION == newest numeric CHANGELOG heading -----------------
VERSION="$(tr -d '[:space:]' < VERSION)"
if [ -z "$VERSION" ]; then
  echo "REFUSED: check 'version consistency' failed — VERSION file is empty" >&2
  exit 1
fi
# Numeric release headings look like "## [0.1.0] — 2026-09-29"; the first one
# after [Unreleased] is the newest. Any non-numeric heading (none today) is not
# a release and is skipped.
NEWEST_HEADING="$(awk '/^## \[/ { if ($2 != "[Unreleased]") { print $2; exit } }' CHANGELOG.md)"
NEWEST_VERSION="${NEWEST_HEADING#[}"   # strip leading  [
NEWEST_VERSION="${NEWEST_VERSION%]}"   # strip trailing ]
if [ "$VERSION" = "$NEWEST_VERSION" ]; then
  echo "OK  [3/4] VERSION (${VERSION}) matches newest CHANGELOG heading (${NEWEST_HEADING})"
else
  echo "REFUSED: check 'version consistency' failed — VERSION is ${VERSION} but the newest numeric CHANGELOG heading is ${NEWEST_HEADING}" >&2
  echo "    Promote [Unreleased] into a [${VERSION}] section (or bump VERSION), commit, and re-run. See docs/release.md." >&2
  exit 1
fi

TAG="v${VERSION}"

# --- Preflight 4: a successful CI run exists for the exact HEAD SHA -----------
# Trust the commit SHA, never a local branch label: ask GitHub for runs whose
# head_sha equals HEAD and require at least one successful CI run.
if CI_RUN_ID="$(gh run list --repo "$REPO_SLUG" --commit "$HEAD_SHA" \
      --workflow ci.yml --status success --limit 1 --json databaseId --jq '.[0].databaseId' 2>/dev/null)" \
   && [ -n "$CI_RUN_ID" ] && [ "$CI_RUN_ID" != "null" ]; then
  echo "OK  [4/4] CI green on HEAD (run $CI_RUN_ID succeeded for ${HEAD_SHA:0:12})"
else
  echo "REFUSED: check 'CI green on HEAD' failed — no successful ci.yml run found for commit ${HEAD_SHA:0:12} on ${REPO_SLUG}" >&2
  echo "    Wait for CI to finish on this exact SHA (push it first if needed), then re-run." >&2
  exit 1
fi

# --- Result -------------------------------------------------------------------
echo ""
echo "Preflight passed for ${REPO_SLUG} @ ${HEAD_SHA:0:12} — release ${TAG} is ready to cut."
echo "Exact tag command:"
echo ""
echo "    git tag -a \"${TAG}\" -m \"Release ${TAG}\" && git push origin \"${TAG}\""
echo ""

if [ "${DRY_RUN:-0}" = "1" ]; then
  echo "Dry run: no tag was created and nothing was pushed."
  exit 0
fi

if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
  echo "REFUSED: check 'tag collision' failed — tag ${TAG} already exists locally." >&2
  exit 1
fi
git tag -a "$TAG" -m "Release ${TAG}"
echo "Created annotated tag ${TAG}. Now push it to trigger the release workflow:"
echo "    git push origin ${TAG}"
