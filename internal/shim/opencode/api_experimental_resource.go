package opencode

import (
	"fmt"
	"net/http"
	"strings"
)

// ROUTE-ADD-073 — GET /experimental/resource (experimental.resource.list)
//
// The pinned upstream operation returns "all available MCP resources from
// connected servers": opencode acts as an MCP CLIENT toward its configured
// MCP servers and aggregates the resources they advertise. This runtime
// operates no MCP client connections — the shim's /mcp endpoint is the
// SERVER side of the protocol (internal/mcp via SetMCPHandler), and
// internal/db keeps no McpResource store — so the declared 200 (a JSON
// OBJECT mapping resource names to McpResource) is truthfully the empty
// object: any entry would assert a connected MCP server this runtime does
// not have (the ROUTE-ADD-076 workspace-list truthfulness precedent,
// object-shaped).
//
// The declared directory/workspace query selectors are plain optional
// strings with no pattern, so the only malformed spelling is
// present-but-blank, answered with the declared 400 BadRequestError (the
// /experimental/capabilities convention). Undeclared methods keep the
// sibling 405 METHOD_NOT_ALLOWED envelope.
func (s *Server) handleExperimentalResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	q := r.URL.Query()
	for _, param := range []string{"directory", "workspace"} {
		if values, present := q[param]; present && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeOpencodeNamedError(w, r, http.StatusBadRequest, "BadRequest",
				fmt.Sprintf("query parameter %q must not be blank", param))
			return
		}
	}
	writeJSON(w, map[string]any{})
}
