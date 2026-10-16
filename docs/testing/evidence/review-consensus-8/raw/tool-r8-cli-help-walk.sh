#!/usr/bin/env bash
# REVIEW-CONSENSUS-8 — recursive `--help` enumeration of the consensus CLI tree.
# Walks cobra subcommands by parsing "Available Commands:" from each --help.
set -u
BIN="${BIN:-/tmp/consensus-r8-bin}"
OUT="${OUT:-/tmp/r8-cli-help}"
rm -rf "$OUT"; mkdir -p "$OUT"

walk() {
  local path="$1"   # space separated command path, may be empty for root
  local slug="$2"   # filename slug
  # shellcheck disable=SC2086
  "$BIN" $path --help > "$OUT/$slug.txt" 2>&1
  local rc=$?
  echo "=== $slug (exit $rc)" >> "$OUT/_index.txt"
  # extract subcommands from the "Available Commands:" block
  awk '/^Available Commands:/{f=1;next} /^$/{if(f)exit} f{print $1}' "$OUT/$slug.txt" \
    | grep -v '^$' | grep -v '^help$' | while read -r sub; do
      walk "$path $sub" "${slug}__${sub}"
    done
}

walk "" "root"
echo "done"
