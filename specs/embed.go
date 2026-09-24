// Package specs embeds the bundled OpenAPI specification into the binary.
//
// The bundle lives at specs/openapi/bundled.yaml at the repository root.
// internal/api cannot go:embed it directly (embed patterns cannot cross
// package directories), so this tiny root-level package is the single
// compile-time holder. The embedded copy is the single source of truth for
// the served contract (C-GAP-039): the API server serves specs.BundledYAML
// unconditionally, so /openapi.yaml and /openapi.json are identical from any
// working directory and in the Docker image (which does not copy specs/).
// To publish a new contract, update specs/openapi/, re-run `make bundle-spec`,
// and rebuild. See DOGFOOD-103 and SPEC-018 §9.
package specs

import _ "embed"

// BundledYAML is the single-file bundled OpenAPI contract
// (specs/openapi/bundled.yaml), embedded at build time.
//
//go:embed openapi/bundled.yaml
var BundledYAML []byte
