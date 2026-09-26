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
//
// The opencode shim serves the same embedded contract at /doc as the
// machine-readable document the upstream opencode suite pins there
// (DF-CONSENSUS-36); the interactive REST Swagger UI lives at /doc/api.
package specs

import (
	_ "embed"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// BundledYAML is the single-file bundled OpenAPI contract
// (specs/openapi/bundled.yaml), embedded at build time.
//
//go:embed openapi/bundled.yaml
var BundledYAML []byte

var (
	specOnce  sync.Once
	parsedDoc map[string]any
	parseErr  error
)

// ParsedDocument returns the embedded OpenAPI bundle parsed into a generic
// document, ready to be re-marshaled as JSON. The result is computed once
// per process and shared; the returned map must be treated as read-only.
// The error is os.ErrNotExist when the embedded bundle is empty (the binary
// was built without the embed) and a yaml parse error only when the embedded
// bundle fails to parse — a build-time contract bug (the bundle is
// regenerated from specs/openapi/ by `make bundle-spec`).
func ParsedDocument() (map[string]any, error) {
	specOnce.Do(func() {
		if len(BundledYAML) == 0 {
			parseErr = os.ErrNotExist
			return
		}
		var doc map[string]any
		if err := yaml.Unmarshal(BundledYAML, &doc); err != nil {
			parseErr = err
			return
		}
		parsedDoc = doc
	})
	return parsedDoc, parseErr
}
