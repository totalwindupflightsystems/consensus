#!/usr/bin/env bash
# require_tools.sh — SG-2. Declare the run host's prerequisites and refuse to proceed
# when one is missing. A missing tool used to mean "the step silently did nothing";
# here it means VOID (exit 2), which can never be mistaken for a pass.
#
# usage: require_tools.sh            # required set
#        require_tools.sh --optional # report optional tools without failing
set -uo pipefail
OPTIONAL=0
[ "${1:-}" = "--optional" ] && OPTIONAL=1

REQUIRED="bash jq curl sqlite3 ss sha256sum awk grep sed sort timeout tee git go"
OPTIONAL_T="yq(node/loadgen shims) bun node make docker ssh bunker hey ab"

missing=""
printf '%-14s %s\n' TOOL STATUS
for t in $REQUIRED; do
  if command -v "$t" >/dev/null 2>&1; then printf '%-14s %s\n' "$t" "ok ($(command -v "$t"))"
  else printf '%-14s %s\n' "$t" "MISSING"; missing="$missing $t"; fi
done
for t in bun node make docker ssh bunker; do
  if command -v "$t" >/dev/null 2>&1; then printf '%-14s %s\n' "$t" "ok (optional)"; else printf '%-14s %s\n' "$t" "absent (optional)"; fi
done
echo
echo "declared substitutes in scripts/battery/bin/: yq (PyYAML shim), loadgen (curl load loop)"
echo "note: hey and ab are ABSENT by design — T9 uses loadgen instead of an undeclared tool."
echo
if [ -n "$missing" ]; then
  echo "require_tools: VOID — required tool(s) missing:${missing}"
  [ "$OPTIONAL" -eq 1 ] && { echo "  (continuing anyway: --optional)"; exit 0; }
  exit 2
fi
echo "require_tools: ok — every required tool resolves."
exit 0
