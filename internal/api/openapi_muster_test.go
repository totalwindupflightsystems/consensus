// Package api: muster-compatibility contract tests (MUSTER-INT-001).
//
// muster (github.com/wojons/muster) generates a CLI, an MCP server, and a Go
// library from the spec served at /openapi.json — no hand-written glue. These
// tests pin the properties its parser and generators rely on:
//
//  1. every operation carries a non-empty, unique operationId (command names);
//  2. every operation declares at least one success (2xx) response, unless its
//     path item is an explicitly declared stub (x-not-implemented: true);
//  3. every declared response resolves to a body schema (clients must be able
//     to decode the wire, including the error arms);
//  4. every servers[] URL is absolute — a relative server makes generated
//     commands die with "unsupported protocol scheme" (muster DF-015);
//  5. the served surface stays muster-scale: at least 60 paths.
//
// Every check parses the SERVED spec (GET /openapi.json off an httptest
// server, i.e. the embedded bundle, C-GAP-039) exactly the way a muster
// client consumes it — not an on-disk file that could drift from the binary.
//
// axiom:trace work_item=muster-int-001 spec=specs/018-openapi-contract.md test=internal/api/openapi_muster_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// servedSpecDoc fetches /openapi.json the way a generator client would and
// decodes it.
func servedSpecDoc(t *testing.T) map[string]any {
	t.Helper()
	s := NewServer(ServerConfig{DB: &mockAPIDB{}, Addr: ":0"})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("GET /openapi.json: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /openapi.json: got %d, want 200", resp.StatusCode)
	}
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode /openapi.json: %v", err)
	}
	return doc
}

// specOperations iterates every path item's HTTP operations, handing each to
// fn with its method, path, operation object, and whether the path item is a
// declared not-implemented stub.
func specOperations(t *testing.T, doc map[string]any, fn func(method, path string, op map[string]any, notImplemented bool)) {
	t.Helper()
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("served spec has no paths object")
	}
	for pattern, itemAny := range paths {
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		notImplemented := false
		if v, ok := item["x-not-implemented"].(bool); ok && v {
			notImplemented = true
		}
		for method := range specHTTPMethodsSet {
			op, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			fn(method, pattern, op, notImplemented)
		}
	}
}

// specHTTPMethodsSet mirrors openapi_paths_test.go's specHTTPMethods (unexported
// there — this file is in-package, so a local copy keeps both readable).
var specHTTPMethodsSet = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true, "delete": true,
}

// resolveResponse follows a response-level $ref into components.responses and
// returns the resolved response object (nil when the ref dangles or the
// response is inline).
func resolveResponse(doc map[string]any, resp map[string]any) map[string]any {
	ref, ok := resp["$ref"].(string)
	if !ok {
		return resp
	}
	// Served (bundled) refs look like #/components/responses/<Name> with an
	// optional deeper pointer (…/content/application~1json/schema).
	parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
	if len(parts) < 3 || parts[0] != "components" || parts[1] != "responses" {
		return nil
	}
	components, _ := doc["components"].(map[string]any)
	responses, _ := components["responses"].(map[string]any)
	node := responses[parts[2]]
	resolved, _ := node.(map[string]any)
	// Walk any remaining pointer segments (~1 escapes "/" per RFC 6901).
	for _, seg := range parts[3:] {
		seg = strings.ReplaceAll(seg, "~1", "/")
		seg = strings.ReplaceAll(seg, "~0", "~")
		next, ok := resolved[seg].(map[string]any)
		if !ok {
			return nil
		}
		resolved = next
	}
	return resolved
}

// TestServedSpecOperationsHaveOperationIds pins the muster command-name
// contract: every operation declares a non-empty operationId, and no two
// operations share one (duplicate ids would collide into one command).
func TestServedSpecOperationsHaveOperationIds(t *testing.T) {
	doc := servedSpecDoc(t)
	seen := map[string]string{}
	ops := 0
	specOperations(t, doc, func(method, path string, op map[string]any, _ bool) {
		ops++
		id, _ := op["operationId"].(string)
		if strings.TrimSpace(id) == "" {
			t.Errorf("%s %s has no operationId — muster cannot name the generated command", strings.ToUpper(method), path)
			return
		}
		if prev, dup := seen[id]; dup {
			t.Errorf("operationId %q is declared twice (%s and %s %s) — generated command names would collide", id, prev, strings.ToUpper(method), path)
		}
		seen[id] = strings.ToUpper(method) + " " + path
	})
	if ops == 0 {
		t.Fatal("served spec declares no operations")
	}
	if len(seen) != ops {
		t.Errorf("expected %d unique operationIds, counted %d", ops, len(seen))
	}
}

// TestServedSpecOperationsDeclareSuccessResponse pins the muster codegen
// contract: every operation declares at least one 2xx response — unless the
// operation genuinely has no success answer, in which case its declaration
// must be honest about it. Two exemptions hold (both pre-fix stub lies, both
// corrected to honest declarations by MUSTER-INT-001):
//
//   - x-not-implemented: true path items are declared stubs — the spec
//     promises no contract, so no success arm is required (POST /mcp on bare
//     deployments, GET /vcs/{vcsId});
//   - a fail-only LOOKUP operation (every declared response is 4xx/5xx) whose
//     path item also declares the real served operation under another method
//     — GET /tui/{action} documents 404/405/401 only, truthfully: the actual
//     TUI execution lives on POST /tui/{action}, which declares the 200.
//
// A fail-only operation with NO served sibling remains a defect: generators
// emit a command that can never succeed.
func TestServedSpecOperationsDeclareSuccessResponse(t *testing.T) {
	doc := servedSpecDoc(t)
	paths, _ := doc["paths"].(map[string]any)
	hasSuccess := func(item map[string]any) bool {
		for method := range specHTTPMethodsSet {
			op, _ := item[method].(map[string]any)
			if op == nil {
				continue
			}
			responses, _ := op["responses"].(map[string]any)
			for code := range responses {
				if strings.HasPrefix(code, "2") {
					return true
				}
			}
		}
		return false
	}
	checked, stubbed := 0, 0
	specOperations(t, doc, func(method, path string, op map[string]any, notImplemented bool) {
		checked++
		if notImplemented {
			stubbed++
			return
		}
		responses, _ := op["responses"].(map[string]any)
		for code := range responses {
			if strings.HasPrefix(code, "2") {
				return
			}
		}
		// Fail-only operation: exempt only when a sibling method on the same
		// path item serves the real success contract.
		if item, _ := paths[path].(map[string]any); item != nil && hasSuccess(item) {
			t.Logf("%s %s is a fail-only lookup (no 2xx) with a served sibling method on the same path — honest declaration, exempt", strings.ToUpper(method), path)
			return
		}
		codes := make([]string, 0, len(responses))
		for code := range responses {
			codes = append(codes, code)
		}
		t.Errorf("%s %s declares no 2xx response (only %v) — generators need a success arm; if this path is a declared stub, mark the path item x-not-implemented: true", strings.ToUpper(method), path, codes)
	})
	if checked == 0 {
		t.Fatal("served spec declares no operations")
	}
	t.Logf("%d operations checked; %d on x-not-implemented stub path items (success arm not required)", checked, stubbed)
}

// TestServedSpecDeclaredResponsesCarryBodySchema pins the muster decode
// contract: every declared response that carries a body resolves to a body
// schema, so generated clients can decode the wire. A description-only
// response (no content at all) is a defect whenever the server writes a body
// the spec claims does not exist. Pre-fix this failed for 8 responses
// (PUT /auth/{authId} 405, POST /mcp 405+501, POST /webhooks/{source}
// 403+404+413+429+500).
//
// 202 and 204 are the explicit exception: their semantics are a zero-byte
// body, and OpenAPI's correct representation of that is NO content key —
// a description-only 202/204 is honest, not a defect (POST /mcp 202 answers
// zero bytes for notifications).
func TestServedSpecDeclaredResponsesCarryBodySchema(t *testing.T) {
	doc := servedSpecDoc(t)
	bad := 0
	bodyless := map[string]bool{"202": true, "204": true}
	specOperations(t, doc, func(method, path string, op map[string]any, _ bool) {
		responses, _ := op["responses"].(map[string]any)
		for code, respAny := range responses {
			resp, ok := respAny.(map[string]any)
			if !ok {
				t.Errorf("%s %s %s: response is not an object (%T)", strings.ToUpper(method), path, code, respAny)
				bad++
				continue
			}
			resolved := resolveResponse(doc, resp)
			if resolved == nil {
				t.Errorf("%s %s %s: $ref does not resolve into components.responses", strings.ToUpper(method), path, code)
				bad++
				continue
			}
			content, _ := resolved["content"].(map[string]any)
			if len(content) == 0 {
				if bodyless[code] {
					continue // zero-byte body: no content key is the honest declaration
				}
				t.Errorf("%s %s %s has a description-only response (no content schema) — the server answers a JSON body the spec does not document; muster clients cannot decode it", strings.ToUpper(method), path, code)
				bad++
				continue
			}
			for ct, mediaAny := range content {
				media, ok := mediaAny.(map[string]any)
				if !ok || media["schema"] == nil {
					t.Errorf("%s %s %s %s: media type carries no schema", strings.ToUpper(method), path, code, ct)
					bad++
				}
			}
		}
	})
	if bad > 0 {
		t.Errorf("%d declared responses lack a body schema", bad)
	}
}

// TestServedSpecServersAreAbsoluteURLs pins the muster DF-015 contract: every
// servers[].url is an absolute http(s) URL. A relative server makes generated
// commands die with "unsupported protocol scheme" the first time one runs.
func TestServedSpecServersAreAbsoluteURLs(t *testing.T) {
	doc := servedSpecDoc(t)
	servers, ok := doc["servers"].([]any)
	if !ok || len(servers) == 0 {
		t.Fatal("served spec declares no servers array — muster cannot pick a base URL")
	}
	for i, sAny := range servers {
		s, _ := sAny.(map[string]any)
		u, _ := s["url"].(string)
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			t.Errorf("servers[%d].url %q is relative — generated commands fail with 'unsupported protocol scheme' (muster DF-015)", i, u)
		}
	}
}

// TestServedSpecMusterScaleFloor keeps the served surface at muster scale: a
// spec that silently loses paths breaks generated command inventories.
func TestServedSpecMusterScaleFloor(t *testing.T) {
	doc := servedSpecDoc(t)
	paths, _ := doc["paths"].(map[string]any)
	if len(paths) < 60 {
		t.Errorf("served spec has %d paths, want >= 60 (muster inventory floor)", len(paths))
	}
}

// TestServedSpecShimStubMarkerHonesty: x-not-implemented is an ALLOWLIST
// escape hatch from the success-arm rule — it must only mark path items whose
// operations genuinely serve no declared contract. The shim's real served
// surfaces (find/symbol, project/{projectId}, tui, vcs item) were wrongly
// marked before MUSTER-INT-001; pin the marker to the exact set that remains.
func TestServedSpecShimStubMarkerHonesty(t *testing.T) {
	doc := servedSpecDoc(t)
	allowed := map[string]bool{
		// /mcp management stub on bare deployments: MCP-DIRECT-001 owns the
		// discoverable surface; the marker documents the shim's 501 answer.
		"/mcp": true,
		// GET /vcs/{vcsId} IS the declared not-implemented surface (vcs.apply,
		// vcs.diff.raw answer the typed 501 envelope). MUSTER-INT-001 made the
		// marker honest rather than removing it: the 501 bodies now carry
		// schemas, and the description no longer claims the no-auth behavior
		// the live server does not have.
		"/vcs/{vcsId}": true,
	}
	paths, _ := doc["paths"].(map[string]any)
	for pattern, itemAny := range paths {
		item, _ := itemAny.(map[string]any)
		if item == nil {
			continue
		}
		if v, ok := item["x-not-implemented"].(bool); ok && v && !allowed[pattern] {
			t.Errorf("path %s carries x-not-implemented but is not on the known-stub allowlist %v — either the surface is now real (declare its served responses and drop the marker) or it belongs on the list with a reason", pattern, allowed)
		}
	}
}

// TestServedSpecResponseRefsResolve guards the ~1-escaped pointer style the
// spec sources use (../components/responses.yaml#/X/content/application~1json/
// schema): after bundling every response-level $ref must resolve deep enough
// to reach a schema node.
func TestServedSpecResponseRefsResolve(t *testing.T) {
	doc := servedSpecDoc(t)
	checked := 0
	specOperations(t, doc, func(method, path string, op map[string]any, _ bool) {
		responses, _ := op["responses"].(map[string]any)
		for code, respAny := range responses {
			resp, _ := respAny.(map[string]any)
			if resp == nil {
				continue
			}
			if _, isRef := resp["$ref"]; !isRef {
				continue
			}
			resolved := resolveResponse(doc, resp)
			if resolved == nil {
				t.Errorf("%s %s %s: response $ref does not resolve", strings.ToUpper(method), path, code)
				continue
			}
			content, _ := resolved["content"].(map[string]any)
			for _, mediaAny := range content {
				media, _ := mediaAny.(map[string]any)
				if schema, ok := media["schema"].(map[string]any); ok {
					if ref, isRef := schema["$ref"].(string); isRef {
						// Schema-level refs point into components.schemas —
						// verify the target exists so muster never meets a
						// dangling pointer.
						name := strings.TrimPrefix(ref, "#/components/schemas/")
						components, _ := doc["components"].(map[string]any)
						schemas, _ := components["schemas"].(map[string]any)
						if _, ok := schemas[name]; !ok {
							t.Errorf("%s %s %s: schema $ref %q does not resolve", strings.ToUpper(method), path, code, ref)
						}
						checked++
					}
				}
			}
		}
	})
	if checked == 0 {
		t.Fatal("no response schema refs checked — spec responses carry no $ref schemas?")
	}
}
