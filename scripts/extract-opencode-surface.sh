#!/usr/bin/env bash
# SHIM-SOURCE-002 surface extraction.
# Derives the protocol surface listing (paths + schemas) from the recorded
# upstream document. Re-runnable: jq only, no network.
set -euo pipefail
DOC="${1:?usage: extract-surface.sh <document.json> <out-prefix>}"
OUT="${2:?usage: extract-surface.sh <document.json> <out-prefix>}"
jq '{
  source: {
    upstream_repo: "sst/opencode (redirects to anomalyco/opencode)",
    document_path: "packages/sdk/openapi.json",
    extracted_by: "scripts/extract-opencode-surface.sh"
  },
  document: {
    openapi: .openapi,
    info_title: .info.title,
    info_version: .info.version,
    security: .security,
    tag_count: (.tags | length),
    path_count: (.paths | length),
    operation_count: ([.paths[] | to_entries[] | select(.key | IN("get","post","put","patch","delete","head","options"))] | length),
    schema_count: (.components.schemas | length)
  },
  paths: (.paths | to_entries | map({
    key: .key,
    value: (.value | to_entries
      | map(select(.key | IN("get","post","put","patch","delete","head","options")))
      | map({key: (.key | ascii_upcase), value: {
          operationId: .value.operationId,
          tags: (.value.tags // []),
          responses: ((.value.responses // {}) | keys),
          security: .value.security
        }})
      | from_entries)
  }) | from_entries),
  schemas: (.components.schemas | keys)
}' "$DOC" > "${OUT}.surface.json"
{
  jq -r '.paths | to_entries | sort_by(.key) | .[] | .key as $p | (.value | to_entries | sort_by(.key))[] | "\(.key)\t\($p)\t\(.value.operationId)"' "$DOC"
} > "${OUT}.surface.txt"
{
  echo "# schemas (\(jq -r '.components.schemas | length' "$DOC"))"
  jq -r '.components.schemas | keys[]' "$DOC"
} >> "${OUT}.surface.txt"
echo "wrote ${OUT}.surface.json and ${OUT}.surface.txt"
