// Package specs embeds the bundled OpenAPI specification into the binary.
//
// The bundle lives at specs/openapi/bundled.yaml at the repository root.
// internal/api cannot go:embed it directly (embed patterns cannot cross
// package directories), so this tiny root-level package is the single
// compile-time holder. The API server prefers the on-disk file when it
// exists (dev workflow — live edits after `make bundle-spec`) and falls
// back to this embedded copy otherwise, which makes /openapi.json and
// /openapi.yaml work from any working directory and in the Docker image
// (which does not copy specs/). See DOGFOOD-103.
package specs

import _ "embed"

// BundledYAML is the single-file bundled OpenAPI contract
// (specs/openapi/bundled.yaml), embedded at build time.
//
//go:embed openapi/bundled.yaml
var BundledYAML []byte
