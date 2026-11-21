#!/usr/bin/env bash
# install-hooks.sh — install this repository's GitReins pre-commit guard into the
# COMMON hooks directory so it fires identically from the primary checkout and
# from every linked worktree.
#
# Why not `cp .gitreins/pre-commit .git/hooks/pre-commit`:
#   In a linked worktree `.git` is a FILE (a gitdir pointer), not a directory.
#   With `core.hooksPath` set to the relative value `.git/hooks`, git resolves
#   the hooks dir against the WORKTREE root — where `.git/hooks` cannot exist —
#   so it finds no hook, prints nothing, and the guard is silently disarmed
#   exactly where fleet workers commit. Verified failure mode:
#       $ git -C <worktree> rev-parse --git-path hooks   -> .git/hooks
#       $ ls .git/hooks                                  -> Not a directory
#       $ git hook run pre-commit                        -> cannot find a hook named pre-commit
#   `git rev-parse --git-path hooks` already resolves to the common hooks dir
#   when `core.hooksPath` is UNSET (absolute inside a linked worktree), so this
#   installer keeps it unset and writes there.
#
# Properties:
#   * installs from the tracked source `.gitreins/pre-commit`
#   * chmod 0755 on the installed hook
#   * unsets a relative `core.hooksPath`, and refuses to continue if it survives
#   * refuses an absolute `core.hooksPath` that shadows the common hooks dir
#   * idempotent: a second run writes nothing; the hook stays byte-identical
#
# Usage: scripts/install-hooks.sh [--quiet]
set -euo pipefail

quiet=0
[ "${1:-}" = "--quiet" ] && quiet=1
say() { [ "$quiet" -eq 1 ] || printf '%s\n' "$*"; }

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

src="$repo_root/.gitreins/pre-commit"
[ -f "$src" ] || { echo "error: missing hook source $src" >&2; exit 1; }

# ── 1. A relative core.hooksPath is the defect this installer exists to prevent.
#      Repair it (unset at every level where it is set) and prove the repair.
hooks_cfg="$(git config --get core.hooksPath || true)"
if [ -n "$hooks_cfg" ]; then
  case "$hooks_cfg" in
    /*) : ;;  # absolute — reconciled against the common hooks dir below
    *)
      say "==> core.hooksPath=$hooks_cfg is RELATIVE"
      say "    a linked worktree resolves it under a .git that is a file, so the hook never runs."
      for level in local worktree; do
        if git config --"$level" --get core.hooksPath >/dev/null 2>&1; then
          git config --"$level" --unset-all core.hooksPath
          say "    unset core.hooksPath (scope: $level)"
        fi
      done
      hooks_cfg="$(git config --get core.hooksPath || true)"
      if [ -n "$hooks_cfg" ]; then
        echo "error: refusing to install — relative core.hooksPath=$hooks_cfg is still set." >&2
        echo "       The installed hook would never run in a linked worktree." >&2
        echo "       Inspect with: git config --show-origin --get-all core.hooksPath" >&2
        echo "       Then:         git config --unset-all core.hooksPath" >&2
        exit 1
      fi
      say "    core.hooksPath is now unset — git uses the common hooks dir"
      ;;
  esac
fi

# ── 2. Resolve the COMMON hooks dir (absolute; shared by every worktree).
#      Derived from --git-common-dir, NOT from --git-path hooks: the latter
#      follows core.hooksPath, so comparing a hooksPath against it can never
#      detect a shadowing path (it compares the value with itself).
common_dir="$(git rev-parse --git-common-dir)"
case "$common_dir" in
  /*) : ;;
  *)  common_dir="$repo_root/$common_dir" ;;
esac
hooks_dir="$common_dir/hooks"

if [ -n "$hooks_cfg" ]; then
  cfg_real="$(cd "$(dirname "$hooks_cfg")" 2>/dev/null && pwd)/$(basename "$hooks_cfg")" || cfg_real="$hooks_cfg"
  dir_real="$(cd "$hooks_dir" 2>/dev/null && pwd)" || dir_real="$hooks_dir"
  if [ "$cfg_real" != "$dir_real" ]; then
    echo "error: refusing to install — absolute core.hooksPath=$hooks_cfg shadows the" >&2
    echo "       common hooks dir $dir_real, so a hook installed there would never run." >&2
    echo "       Unset it (git config --unset-all core.hooksPath) and re-run." >&2
    exit 1
  fi
fi

if [ ! -d "$hooks_dir" ]; then
  mkdir -p "$hooks_dir" 2>/dev/null || {
    echo "error: hooks dir $hooks_dir does not exist and cannot be created (is .git a file here?)" >&2
    exit 1
  }
fi

# ── 3. Install (idempotent: identical content + executable mode => no write).
dst="$hooks_dir/pre-commit"
src_sha="$(sha256sum "$src" | awk '{print $1}')"
if [ -f "$dst" ] && cmp -s "$src" "$dst" && [ -x "$dst" ]; then
  say "✓ hook already current — no write: $dst"
else
  cp "$src" "$dst"
  chmod 0755 "$dst"
  say "✓ hook installed: $dst"
fi
dst_sha="$(sha256sum "$dst" | awk '{print $1}')"
say "  src sha256: $src_sha"
say "  dst sha256: $dst_sha"
[ "$src_sha" = "$dst_sha" ] || { echo "error: installed hook does not match $src" >&2; exit 1; }

# ── 4. Report the resolution a linked worktree will see (acceptance evidence).
abs_hooks="$(git rev-parse --path-format=absolute --git-path hooks 2>/dev/null || echo "$hooks_dir")"
say "  rev-parse --git-path hooks:            $(git rev-parse --git-path hooks)"
say "  rev-parse --path-format=absolute:      $abs_hooks"
say "  git dir:                               $(git rev-parse --git-dir)"
case "$(git rev-parse --git-path hooks)" in
  /*) : ;;
  *) [ "$(git rev-parse --git-dir)" = ".git" ] || {
        echo "error: hooks path resolved relative inside a linked worktree — refusing." >&2
        exit 1
      } ;;
esac
say "✓ GitReins pre-commit armed (shared by every worktree of this repository)"
