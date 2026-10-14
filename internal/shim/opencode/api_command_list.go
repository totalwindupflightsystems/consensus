package opencode

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// handleApiCommandList serves GET /api/command — upstream v2.command.list
// (ROUTE-ADD-006, source item SHIM-DRIFT-002, declared responses: 200
// {location: LocationInfo, data: CommandV2Info[]} "Retrieve currently
// registered commands.", 400 InvalidRequestError, 401 UnauthorizedError).
// The exact path previously had no route at all, so net/http answered the
// default 404 (the artifact's NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-006 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.command.list test=internal/shim/opencode/api_command_list_test.go witness=none:unattended-worker-session
//
// TRUTHFUL payload: the Consensus runtime keeps no v2 command registry —
// internal/db has no commands store and no migration creates one (the v2 SDK
// surface is served by this shim alone) — so `data` is an EMPTY
// CommandV2Info[] array: never null, and never a fabricated entry. That is the
// same honest answer the sibling v1 route gives for its own absent registry
// (handleCommandList, GET /command, upstream command.list) and the
// question.list precedent (empty list 200). The declared `location` member is
// the REAL selected workspace (requestWorkspaceDir + instanceID, the
// handleApiAgentList / handleFSFind convention) — no location field is
// invented.
//
// The declared `location` selector uses the v2 SDK's deepObject query spelling
// (location[directory]/location[workspace]), exactly like handleApiAgentList:
// a blank value, an unknown location field, an unreadable or non-directory
// location[directory], or a non-^wrk workspace id is malformed input answered
// with the declared InvalidRequestError 400 (writeV2InvalidRequest).
// Unauthenticated requests answer the declared UnauthorizedError 401 (the
// middleware /api/command branch, mirroring /api/agent and /api/fs/find), and
// undeclared methods keep the sibling METHOD_NOT_ALLOWED envelope.
func (s *Server) handleApiCommandList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}

	q := r.URL.Query()
	for key := range q {
		if strings.HasPrefix(key, "location[") && key != "location[directory]" && key != "location[workspace]" {
			writeV2InvalidRequest(w, "Query", key, fmt.Sprintf("unknown location field %q", key))
			return
		}
	}
	for _, key := range []string{"location[directory]", "location[workspace]"} {
		if values, ok := q[key]; ok && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeV2InvalidRequest(w, "Query", key, fmt.Sprintf("query parameter %q must not be blank", key))
			return
		}
	}

	root := strings.TrimSpace(q.Get("location[directory]"))
	if root == "" {
		root = s.requestWorkspaceDir(r)
	}
	absoluteRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		writeV2InvalidRequest(w, "Query", "location[directory]", "location directory is invalid")
		return
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		writeV2InvalidRequest(w, "Query", "location[directory]",
			fmt.Sprintf("location directory %q cannot be read: %v", absoluteRoot, err))
		return
	}
	if !info.IsDir() {
		writeV2InvalidRequest(w, "Query", "location[directory]",
			fmt.Sprintf("location directory %q is not a directory", absoluteRoot))
		return
	}

	workspaceID := strings.TrimSpace(q.Get("location[workspace]"))
	if workspaceID != "" && !strings.HasPrefix(workspaceID, "wrk") {
		writeV2InvalidRequest(w, "Query", "location[workspace]", "location[workspace] must start with wrk")
		return
	}

	location := map[string]any{
		"directory": absoluteRoot,
		"project": map[string]any{
			"id":        instanceID(absoluteRoot),
			"directory": absoluteRoot,
		},
	}
	if workspaceID != "" {
		location["workspaceID"] = workspaceID
	}

	// data is a non-nil empty slice, so the declared CommandV2Info[] array is
	// emitted as [] and never as null; it has zero entries because this
	// runtime registers no commands.
	writeJSON(w, map[string]any{
		"location": location,
		"data":     []map[string]any{},
	})
}
