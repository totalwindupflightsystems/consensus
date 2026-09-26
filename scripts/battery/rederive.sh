#!/usr/bin/env bash
# rederive.sh — C-230, made executable.
# Third-party re-derivation: recompute the run's headline numbers FROM THE ARTIFACTS
# so a reviewer with only the evidence tree gets the same values. Prints a sha256 per
# artifact so the recomputation is pinned to bytes, not to a narrative.
#
# usage: rederive.sh <run-dir>
# exit 0 = recomputed >=1 artifact
# exit 2 = VOID: no artifacts found (nothing to re-derive -> not a pass)
set -uo pipefail

RUN="${1:-}"
[ -n "$RUN" ] && [ -d "$RUN" ] || { echo "rederive: run dir not found: '${RUN}'" >&2; exit 2; }

found=0
echo "== re-derivation over ${RUN} =="
printf '%-52s %-10s %-20s %s\n' 'ARTIFACT' 'SHA256-12' 'ROWS' 'PASS/FAIL/TOTAL'

while IFS= read -r f; do
  [ -z "$f" ] && continue
  found=$((found+1))
  sha=$(sha256sum "$f" 2>/dev/null | cut -c1-12)
  case "$f" in
    *.json)
      if jq -e 'type=="array"' "$f" >/dev/null 2>&1; then
        rows=$(jq 'length' "$f")
        p=$(jq '[.[]|select(.verdict=="PASS")]|length' "$f")
        fl=$(jq '[.[]|select(.verdict=="FAIL")]|length' "$f")
        printf '%-52s %-10s %-20s %s\n' "$(basename "$f")" "$sha" "$rows" "${p}/${fl}/$(jq '[.[]|select(.verdict!=null)]|length' "$f")"
      else
        printf '%-52s %-10s %-20s %s\n' "$(basename "$f")" "$sha" "-" "not a results array"
      fi;;
    *.jsonl)
      rows=$(wc -l < "$f")
      printf '%-52s %-10s %-20s %s\n' "$(basename "$f")" "$sha" "$rows" "-";;
    *)
      printf '%-52s %-10s %-20s %s\n' "$(basename "$f")" "$sha" "-" "-";;
  esac
done < <(find "$RUN" -maxdepth 3 -type f \( -name '*.json' -o -name '*.jsonl' -o -name '*.tsv' -o -name '*.log' \) | sort)

echo
if [ "$found" -eq 0 ]; then
  echo "rederive: VOID — found no artifacts under ${RUN}; rc must not be read as success"; exit 2
fi
echo "rederive: ok — re-derived ${found} artifact(s). Copy the commands above to reproduce."
exit 0
