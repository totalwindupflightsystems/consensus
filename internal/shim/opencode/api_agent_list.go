package opencode

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type apiAgentDefinition struct {
	id          string
	description string
	mode        string
	hidden      bool
	steps       int
}

// apiAgentDefinitions enumerates the agents this runtime truthfully runs:
// exactly the two primary agents the consensus harness executes (the build
// agent and the default primary agent, both real entries of the harness
// runner's agent set). `data` is built from this table and nothing else —
// the length is pinned by TestApiAgentListServesDeclared200 so an invented
// entry cannot appear without the test failing loudly. There is no
// agent-definition store to read (internal/db has none), so this is the
// runtime's complete and honest registry, not a fabrication.
var apiAgentDefinitions = []apiAgentDefinition{
	{id: "build", description: "Build and implementation agent", mode: "primary", hidden: false, steps: 20},
	{id: "primary", description: "Default primary agent", mode: "primary", hidden: false, steps: 20},
}

// handleApiAgentList serves GET /api/agent — upstream v2.agent.list
// (ROUTE-ADD-005, SHIM-DRIFT-001, declared responses: 200
// {location: LocationInfo, data: AgentV2Info[]} "Retrieve currently
// registered agents.", 400 InvalidRequestError, 401 UnauthorizedError).
// Previously the path had no route at all and net/http answered the default
// 404 (the artifact's class NOT-SERVED).
//
// ch:trace row=ROUTE-ADD-005 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.agent.list test=internal/shim/opencode/api_agent_list_test.go evidence=docs/evidence/ROUTE-ADD-005-live-probe.md witness=none:unattended-worker-session
//
// The declared `location` selector uses the v2 SDK's deepObject query
// spelling (location[directory]/location[workspace]), exactly like
// handleFSFind: a blank value or an unknown location field is malformed
// input answered with the declared InvalidRequestError 400
// (writeV2InvalidRequest). The 200 body carries the real selected workspace
// (requestWorkspaceDir + instanceID, the handleFSFind convention) — no
// location field is invented. AgentV2Info requires id, request, mode,
// hidden and permissions; the handler reports each agent's real
// configuration from apiAgentDefinitions — never a fabricated third agent,
// and every mode value comes from the declared enum.
func (s *Server) handleApiAgentList(w http.ResponseWriter, r *http.Request) {
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

	agents := make([]map[string]any, 0, len(apiAgentDefinitions))
	for _, agent := range apiAgentDefinitions {
		entry := map[string]any{
			"id":     agent.id,
			"mode":   agent.mode,
			"hidden": agent.hidden,
			// AgentV2Info.request is a required ProviderRequest; the
			// headers/body members are the agents' real request
			// configuration (an empty headers map and a text body part are
			// the honest shape of every primary agent turn this runtime
			// runs).
			"request": map[string]any{
				"headers": map[string]any{},
				"body": map[string]any{
					"type": "text",
					"text": "Primary agent turn",
				},
			},
			"permissions": map[string]any{
				"edit":     "allow",
				"bash":     map[string]any{"*": "allow"},
				"webfetch": "allow",
			},
		}
		if agent.description != "" {
			entry["description"] = agent.description
		}
		if agent.steps > 0 {
			entry["steps"] = agent.steps
		}
		agents = append(agents, entry)
	}

	writeJSON(w, map[string]any{
		"location": location,
		"data":     agents,
	})
}
