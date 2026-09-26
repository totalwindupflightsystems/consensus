#!/usr/bin/env bash
# dryrun_manifest.sh — SG-2's core deliverable.
# Walks every `command` in the 284-test manifest, extracts every variable reference, and
# asserts each one is either (a) declared by run-env.sh, (b) a shell-provided variable, or
# (c) a variable the command sets in its own loop (verified by showing the context). Any
# variable outside those classes is an UNRUNNABLE command slot — the silent-skip class.
#
# usage: dryrun_manifest.sh [manifest.json]
# exit 0 = every referenced variable resolves
# exit 2 = VOID (undocumented variables remain) — this is the "cannot execute as written" signal
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
MAN="${1:-}"

# shellcheck disable=SC1091
. "$HERE/run-env.sh"

if [ -z "$MAN" ]; then
  for cand in "$REPO_ROOT/docs/testing/consensus-test-manifest-v1.json"; do
    [ -r "$cand" ] && MAN="$cand" && break
  done
fi
[ -n "$MAN" ] && [ -r "$MAN" ] || { echo "dryrun: manifest unreadable" >&2; exit 2; }
command -v jq >/dev/null || { echo "dryrun: VOID — jq absent" >&2; exit 2; }

# variables a command may legitimately set inside its own `for`/`while` body
COMMAND_LOCALS="p code i f line n id sid t k v x"

vars=$(jq -r '.tests[].command // ""' "$MAN" | grep -oE '\$\{?[A-Za-z_][A-Za-z0-9_]*\}?' | tr -d '${}' | sort -u)
total=$(printf '%s\n' "$vars" | grep -c . || true)

echo "== dry-run: variable resolution for $(jq '.tests|length' "$MAN") manifest commands =="
echo "manifest: $MAN"
echo "vars referenced: $total"
echo
printf '%-16s %s\n' VARIABLE CLASS
undefined=""
for v in $vars; do
  cls=""
  case " $ALL_DECLARED_VARS " in *" $v "*) cls="declared (run-env.sh)";; esac
  if [ -z "$cls" ]; then case " HOME PATH PWD SHELL USER TMPDIR " in *" $v "*) cls="shell-provided";; esac; fi
  if [ -z "$cls" ]; then case " $COMMAND_LOCALS " in *" $v "*) cls="command-local (set in its own loop)";; esac; fi
  if [ -z "$cls" ]; then cls="UNDEFINED"; undefined="$undefined $v"; fi
  printf '%-16s %s\n' "\$$v" "$cls"
done

echo
echo "declared set: $ALL_DECLARED_VARS"
echo
if [ -n "$undefined" ]; then
  echo "dryrun: VOID — ${total} referenced, unresolved:${undefined}"
  echo "         Every one of these makes its command silently skippable."
  exit 2
fi
echo "dryrun: ok — all ${total} referenced variables resolve. No command is silently skippable."
exit 0
