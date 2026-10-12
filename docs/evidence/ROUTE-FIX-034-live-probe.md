# ROUTE-FIX-034 live probe

Commit: 952d560

The scratch HTTP compatibility server was exercised through the focused Go
HTTP battery (`go test ./internal/shim/opencode -run 'TestProjectUpdate' -count=1`),
which uses a real `httptest.Server` and real filesystem-backed workspace.

Observed arms:

- PATCH /project/{resolvable workspace id}: 200 Project; name, icon, and commands persisted to consensus.json.
- PATCH /project/current: 200 Project; the subsequent GET /project/current observed the persisted metadata.
- malformed, null, array, undeclared-field, wrong-type, blank-selector, and failed-write inputs: 400.
- PATCH /project/project_missing and GET /project/project_missing: typed ProjectNotFoundError 404.
- unsupported POST and PATCH /project/{id}/directories: unchanged typed behavior.

Comparator command:

`python3 scripts/compare-opencode-declared-served.py specs/openapi/upstream/openapi-1.18.33.surface.json specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/consensus-route-fix-034-compare`

Comparator result: covered_operations=75 (previously 74), narrowed_error_contract_only=5 (previously 6), and the project.update operation is covered. Stable IDs preserve SHIM-NARROWED-002 for the project row's historical identity while it is removed from the current narrowed list.
