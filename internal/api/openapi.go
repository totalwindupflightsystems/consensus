// Package api: OpenAPI spec serving (SPEC-018 §9).
//
// The specs directory contains split OpenAPI files (openapi.yaml + components/ + paths/),
// but for serving we use a single-file bundled spec. The bundle step can be run via
// `npx @redocly/cli bundle specs/openapi/openapi.yaml -o specs/openapi/bundled.yaml`
// or the server reads the root spec directly and serves it via Swagger UI CDN.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/018-openapi-contract.md plan=phase-5/task-5-1/step-5-1-3 impl=internal/api/openapi.go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

// ============================================================================
// OpenAPI Spec Serving (SPEC-018 §9)
// ============================================================================

// registerOpenAPIRoutes adds the /doc, /openapi.yaml, and /openapi.json routes.
func (s *Server) registerOpenAPIRoutes(r chi.Router) {
	r.Get("/openapi.yaml", s.handleOpenAPIYAML)
	r.Get("/openapi.json", s.handleOpenAPIJSON)
	r.Get("/doc", s.handleSwaggerUI)
	r.Get("/doc/*", s.handleSwaggerUI)
}

// resolveSpecPath finds the OpenAPI spec file, checking multiple locations.
func resolveSpecPath() string {
	// Check for bundled.yaml first (generated single-file spec)
	candidates := []string{
		"specs/openapi/bundled.yaml",
		"specs/openapi/openapi.yaml",
		filepath.Join("..", "..", "specs", "openapi", "bundled.yaml"),
		filepath.Join("..", "..", "specs", "openapi", "openapi.yaml"),
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Fallback: try from current directory
	if _, err := os.Stat("openapi.yaml"); err == nil {
		return "openapi.yaml"
	}

	return ""
}

// loadSpec reads and parses the OpenAPI YAML spec into a generic map.
func (s *Server) loadSpec() ([]byte, map[string]any, error) {
	path := resolveSpecPath()
	if path == "" {
		return nil, nil, os.ErrNotExist
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return data, nil, err // return raw data even if parse fails (for YAML serving)
	}

	return data, doc, nil
}

// handleOpenAPIYAML serves the raw YAML OpenAPI spec.
func (s *Server) handleOpenAPIYAML(w http.ResponseWriter, r *http.Request) {
	data, _, err := s.loadSpec()
	if err != nil {
		slog.Error("api: failed to load OpenAPI spec", "error", err)
		writeError(w, r, http.StatusNotFound, "NOT_FOUND",
			"OpenAPI spec not found. Run 'make bundle-spec' to generate it, or place specs/openapi/openapi.yaml.")
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

// handleSwaggerUI serves the interactive Swagger UI documentation page.
func (s *Server) handleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	specURL := "/openapi.yaml"
	html := strings.Replace(swaggerUITemplate, "{{SPEC_URL}}", specURL, 1)
	w.Write([]byte(html))
}

// swaggerUITemplate is the Swagger UI HTML page.
const swaggerUITemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <meta name="description" content="Conscience API Documentation" />
  <title>Conscience API — Swagger UI</title>
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
