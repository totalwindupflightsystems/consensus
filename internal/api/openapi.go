// Package api: OpenAPI spec serving (SPEC-018 §9).
//
// The specs directory contains split OpenAPI files (openapi.yaml + components/ + paths/),
// which are bundled into a single file (specs/openapi/bundled.yaml) and embedded into
// the binary at build time (see specs/embed.go). The served contract is ALWAYS the
// embedded copy — an on-disk specs/openapi/bundled.yaml can no longer shadow it
// (C-GAP-039). Regenerate the bundle with `make bundle-spec` and rebuild to publish
// a new contract.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/018-openapi-contract.md plan=phase-5/task-5-1/step-5-1-3 impl=internal/api/openapi.go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/wojons/consensus/specs"
)

// ============================================================================
// OpenAPI Spec Serving (SPEC-018 §9)
// ============================================================================

// registerOpenAPIRoutes adds the /doc/api, /openapi.yaml, and /openapi.json
// routes. The REST API Swagger UI lives at /doc/api (NOT /doc): the opencode
// shim (SPEC-017) mounts its own Swagger UI at /doc, and the bare /doc path
// belongs to that shim surface in full deployments (DOGFOOD-103).
func (s *Server) registerOpenAPIRoutes(r chi.Router) {
	r.Get("/openapi.yaml", s.handleOpenAPIYAML)
	r.Get("/openapi.json", s.handleOpenAPIJSON)
	r.Get("/doc/api", s.handleSwaggerUI)
	r.Get("/doc/api/*", s.handleSwaggerUI)
}

// loadSpec returns the embedded OpenAPI spec bytes and its parsed document.
//
// The embedded copy (specs.BundledYAML) is the single source of truth for
// the served contract (C-GAP-039): /openapi.yaml and /openapi.json serve
// byte-identical content from any working directory and in the Docker image
// (which does not copy specs/), regardless of what files exist on disk.
// Previously the server preferred an on-disk specs/openapi/bundled.yaml so
// `make bundle-spec` changes were picked up without a rebuild — that let a
// stale or divergent local bundle shadow the repo spec, so the on-disk
// resolution was removed. Publish a new contract by regenerating the bundle
// and rebuilding.
func (s *Server) loadSpec() ([]byte, map[string]any, error) {
	if len(specs.BundledYAML) == 0 {
		return nil, nil, os.ErrNotExist
	}

	var doc map[string]any
	if err := yaml.Unmarshal(specs.BundledYAML, &doc); err != nil {
		return specs.BundledYAML, nil, err // return raw data even if parse fails (for YAML serving)
	}

	return specs.BundledYAML, doc, nil
}

// handleOpenAPIYAML serves the raw YAML OpenAPI spec.
func (s *Server) handleOpenAPIYAML(w http.ResponseWriter, r *http.Request) {
	data, _, err := s.loadSpec()
	if err != nil {
		slog.Error("api: failed to load OpenAPI spec", "error", err)
		writeError(w, r, http.StatusNotFound, "NOT_FOUND",
			"OpenAPI spec not found. The spec is embedded at build time from specs/openapi/bundled.yaml; rebuild the binary.")
		return
	}

	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(data)
}

// handleOpenAPIJSON serves the JSON-equivalent OpenAPI spec.
func (s *Server) handleOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	_, doc, err := s.loadSpec()
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "OpenAPI spec not found")
		return
	}

	if doc == nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to parse OpenAPI spec to JSON")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to marshal OpenAPI spec to JSON")
		return
	}
	w.Write(data)
}

// handleSwaggerUI serves the interactive Swagger UI documentation page for
// the REST API. The servers URL is derived from the request Host so the UI
// points at the port the server is actually listening on rather than a
// hardcoded default (DOGFOOD-103).
func (s *Server) handleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	specURL := "/openapi.yaml"
	serversURL := "http://" + r.Host
	if r.TLS != nil {
		serversURL = "https://" + r.Host
	}
	html := strings.Replace(swaggerUITemplate, "{{SPEC_URL}}", specURL, 1)
	html = strings.Replace(html, "{{SERVERS_URL}}", serversURL, 1)
	w.Write([]byte(html))
}

// swaggerUITemplate is the Swagger UI HTML page.
const swaggerUITemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <meta name="description" content="Consensus API Documentation" />
  <title>Consensus API — Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
  <style>
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #fafafa; }
    .topbar { display: none; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js" crossorigin></script>
  <script>
    window.onload = function() {
      window.ui = SwaggerUIBundle({
        url: "{{SPEC_URL}}",
        servers: [{ url: "{{SERVERS_URL}}", description: "Consensus REST API" }],
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
        plugins: [SwaggerUIBundle.plugins.DownloadUrl],
        layout: "StandaloneLayout",
        defaultModelsExpandDepth: 1,
        defaultModelExpandDepth: 1,
        docExpansion: "list",
        filter: true,
        showExtensions: true,
        showCommonExtensions: true,
      });
    };
  </script>
</body>
</html>
`
