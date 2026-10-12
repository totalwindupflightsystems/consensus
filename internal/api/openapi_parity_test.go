// Package api_test: declared-vs-served OpenAPI path parity guard
// (SHIM-SOURCE-003, specs/024 §A.3 "the shim's served surface is compared
// against the declared surface after every refresh — declared == served, per
// path").
//
// This is the in-process half of `make check-spec-parity` (the Makefile target
// runs scripts/check-spec-parity.sh against a scratch server). Both entry
// points compare the same two surfaces:
//
//	declared = the authored split spec, `specs/openapi/openapi.yaml` plus the
//	           `paths/*.yaml` path items it references. `bundled.yaml` is a
//	           GENERATED artifact, so it is deliberately not the declared side:
//	           comparing a bundle against itself proves nothing.
//	served   = `GET /openapi.json` of a running server (its embedded
//	           `specs/openapi/bundled.yaml`) plus the runtime route table.
//
// Drift arms, each named in the failure text:
//
//	MISSING    declared operation absent from the served document
//	           (edited ids in paths/ without re-bundling/rebuilding)
//	EXTRA      served operation the source does not declare
//	           (the published contract promises a path the source dropped)
//	ORPHAN     path item in paths/*.yaml that openapi.yaml never references
//	           (declared in a file, invisible to the built contract)
//	UNANSWERED declared non-stub operation the runtime does not serve
//	           (404 "page not found" / 405 — declared spec, no handler)
//
// A path item marked `x-not-implemented: true` is a deliberately
// declared-but-unserved stub: its operations are excluded from MISSING and
// UNANSWERED (any answer is allowed) and counted as exempt in the summary.
//
// The server under test is the package's full-deployment fixture
// (newFullDeployServer in openapi_paths_test.go): most declared paths live on
// the opencode shim surface, which the bare API router does not mount, so a
// registration check against the API router alone would report false drift.
//
// CONSENSUS_SPEC_PARITY_ROOT overrides the declared spec root so a drift drill
// can point the guard at a mutated COPY of specs/openapi (see the SHIM-SOURCE-003
// report); it is a test-only seam and defaults to the repo's own spec.
//
// axiom:trace work_item=SHIM-SOURCE-003 spec=specs/024-shim-support-matrix.md impl=internal/api/openapi_parity_test.go test=internal/api/openapi_parity_test.go
package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// parityDeclared is the authored (split) surface plus the two classes of drift
// that only exist on the source side.
type parityDeclared struct {
	ops     map[string]string // "<METHOD> <path>" -> "<file>#<anchor>"
	stubs   map[string]bool   // path items marked x-not-implemented
	orphans []string          // path items no openapi.yaml $ref reaches
}

func parityRepoRoot(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve openapi_parity_test.go location")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
}

// paritySetSpecRoot is the declared-spec root: the repo's own split spec, or
// the drift-drill override when CONSENSUS_SPEC_PARITY_ROOT is set.
func paritySetSpecRoot(t *testing.T, repoRoot string) string {
	t.Helper()
	root := os.Getenv("CONSENSUS_SPEC_PARITY_ROOT")
	if root == "" {
		return filepath.Join(repoRoot, "specs", "openapi")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("resolve CONSENSUS_SPEC_PARITY_ROOT %q: %v", root, err)
	}
	t.Logf("spec-parity: declared spec root overridden to %s (drift drill)", abs)
	return abs
}

// parityAsMap tolerates both YAML map shapes (yaml.v3 yields map[string]any;
// some readers hand back map[any]any) so the loader cannot silently see zero
// keys on a decoder change.
func parityAsMap(t *testing.T, what string, v any) map[string]any {
	t.Helper()
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			s, ok := k.(string)
			if !ok {
				t.Fatalf("%s: non-string map key %v", what, k)
			}
			out[s] = val
		}
		return out
	default:
		t.Fatalf("%s: expected a YAML mapping, got %T", what, v)
		return nil
	}
}

// loadDeclaredParity resolves the declared surface out of the split source:
// every `paths:` entry in openapi.yaml, its $ref target, and the HTTP methods
// the target path item declares.
func loadDeclaredParity(t *testing.T, specRoot string) parityDeclared {
	t.Helper()

	rootPath := filepath.Join(specRoot, "openapi.yaml")
	index := parityLoadYAML(t, rootPath)
	paths := parityAsMap(t, rootPath+" paths", index["paths"])
	if len(paths) == 0 {
		t.Fatalf("%s declares no paths object — the declared surface cannot be empty", rootPath)
	}

	declared := parityDeclared{ops: map[string]string{}, stubs: map[string]bool{}}
	// spec-relative file -> anchors openapi.yaml references ("" == whole file)
	referenced := map[string]map[string]bool{}
	cache := map[string]map[string]any{}

	load := func(rel string) map[string]any {
		if doc, ok := cache[rel]; ok {
			return doc
		}
		full := filepath.Join(specRoot, rel)
		if _, err := os.Stat(full); err != nil {
			t.Fatalf("openapi.yaml references missing file %s: %v", rel, err)
		}
		doc := parityLoadYAML(t, full)
		cache[rel] = doc
		return doc
	}

	for _, p := range paritySortedKeys(paths) {
		itemRef := parityAsMap(t, rootPath+" paths."+p, paths[p])
		raw, ok := itemRef["$ref"].(string)
		if !ok {
			t.Fatalf("%s path %s is not a $ref (got %v)", rootPath, p, itemRef)
		}
		filePart, anchor, _ := strings.Cut(raw, "#")
		rel := path.Clean(strings.TrimPrefix(filePart, "./"))
		anchor = strings.TrimPrefix(anchor, "/")
		if referenced[rel] == nil {
			referenced[rel] = map[string]bool{}
		}
		referenced[rel][anchor] = true

		doc := load(rel)
		item := doc
		if anchor != "" {
			ast, ok := doc[anchor]
			if !ok {
				t.Fatalf("%s -> %s: anchor %q not found in %s", p, raw, anchor, rel)
			}
			item = parityAsMap(t, rel+"#"+anchor, ast)
		}

		methods := parityMethods(item)
		if len(methods) == 0 {
			t.Fatalf("%s -> %s: path item declares no HTTP method", p, raw)
		}
		if stub, ok := item["x-not-implemented"].(bool); ok && stub {
			declared.stubs[p] = true
		}
		source := rel
		if anchor != "" {
			source = rel + "#" + anchor
		}
		for _, m := range methods {
			declared.ops[strings.ToUpper(m)+" "+p] = source
		}
	}

	// ORPHAN: a path-item-shaped key in paths/*.yaml that no openapi.yaml $ref
	// reaches. It is declared in a file yet invisible in the built contract.
	entries, err := os.ReadDir(filepath.Join(specRoot, "paths"))
	if err != nil {
		t.Fatalf("read %s/paths: %v", specRoot, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !(strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
			continue
		}
		rel := "paths/" + name
		doc := load(rel)
		used := referenced[rel]
		for _, key := range paritySortedKeys(doc) {
			if used[key] || key == "" {
				continue
			}
			value, ok := doc[key]
			if !ok {
				continue
			}
			if m, ok := value.(map[string]any); ok && len(parityMethods(m)) > 0 {
				declared.orphans = append(declared.orphans, rel+"#"+key)
			}
		}
	}
	sort.Strings(declared.orphans)
	return declared
}

func parityLoadYAML(t *testing.T, file string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	return parityAsMap(t, file, doc)
}

// parityMethods returns the HTTP verbs a path-item / operation object declares.
func parityMethods(item map[string]any) []string {
	var out []string
	for _, m := range []string{"get", "post", "put", "patch", "delete", "head", "options", "trace"} {
		if _, ok := item[m]; ok {
			out = append(out, m)
		}
	}
	return out
}

func paritySortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// parityFetchServed reads the served document and returns "<METHOD> <path>" ->
// source. The title is asserted so a foreign server can never be measured.
func parityFetchServed(t *testing.T, baseURL string) map[string]string {
	t.Helper()

	resp, err := http.Get(baseURL + "/openapi.json")
	if err != nil {
		t.Fatalf("GET /openapi.json: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /openapi.json: status %d", resp.StatusCode)
	}

	var doc struct {
		Info struct {
			Title string `json:"title"`
		} `json:"info"`
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode /openapi.json: %v", err)
	}
	if doc.Info.Title != "Consensus Agent Runtime" {
		t.Fatalf("served document title is %q, not the Consensus contract — refusing to compare", doc.Info.Title)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("served document declares no paths")
	}

	served := map[string]string{}
	for p, item := range doc.Paths {
		for _, m := range parityMethods(item) {
			served[strings.ToUpper(m)+" "+p] = baseURL + "/openapi.json"
		}
	}
	return served
}

// parityDiff is the pure comparison, split out so the detection logic is
// unit-testable without a server (TestSpecParityGuardDiffDetectsDrift).
func parityDiff(declared map[string]string, served map[string]string, stubs map[string]bool, orphans []string) []string {
	var findings []string

	for _, key := range paritySortedKeysString(declared) {
		if _, ok := served[key]; ok {
			continue
		}
		if stubs[parityPathOf(key)] {
			continue // deliberately declared-but-unserved stub
		}
		findings = append(findings, fmt.Sprintf(
			"MISSING %s — declared in %s but absent from the served document", key, declared[key]))
	}
	for _, key := range paritySortedKeysString(served) {
		if _, ok := declared[key]; ok {
			continue
		}
		findings = append(findings, fmt.Sprintf(
			"EXTRA %s — served (%s) but not declared by specs/openapi", key, served[key]))
	}
	for _, orphan := range orphans {
		findings = append(findings, fmt.Sprintf(
			"ORPHAN %s — path item not referenced by openapi.yaml (invisible to the built contract)", orphan))
	}
	return findings
}

// parityProbeRuntime round-trips every declared non-stub operation against the
// running handler tree. A declared operation is ANSWERED unless the router
// replies 405 (method not registered) or chi's bare "404 page not found" (no
// route at all); any other answer — 2xx, 400, 401, a typed 404 body, 5xx —
// proves the route is registered. Auth is not what this arm measures.
func parityProbeRuntime(t *testing.T, baseURL string, declared map[string]string, stubs map[string]bool) []string {
	t.Helper()

	client := &http.Client{Timeout: 10 * time.Second}
	var findings []string
	probed, exempt := 0, 0

	for _, key := range paritySortedKeysString(declared) {
		method, p, _ := strings.Cut(key, " ")
		if stubs[p] {
			exempt++
			continue
		}
		probed++

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		req, err := http.NewRequestWithContext(ctx, method, baseURL+parityInstantiate(p), nil)
		if err != nil {
			cancel()
			t.Fatalf("build request %s: %v", key, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			findings = append(findings, fmt.Sprintf(
				"UNANSWERED %s — no answer from the runtime: %v", key, err))
			continue
		}
		// Only a 404 body is needed to tell chi's unregistered-route page from a
		// typed handler 404. Never drain a 200: GET /mcp is an SSE stream that
		// would hold the read until the deadline.
		var body []byte
		if resp.StatusCode == http.StatusNotFound {
			body, _ = io.ReadAll(io.LimitReader(resp.Body, 4096))
		}
		_ = resp.Body.Close()
		cancel()

		switch {
		case resp.StatusCode == http.StatusMethodNotAllowed:
			findings = append(findings, fmt.Sprintf(
				"UNANSWERED %s — 405 Method Not Allowed (route not registered for %s)", key, method))
		case resp.StatusCode == http.StatusNotFound && strings.Contains(string(body), "page not found"):
			findings = append(findings, fmt.Sprintf(
				"UNANSWERED %s — 404 page not found (route not registered)", key))
		}
	}
	t.Logf("spec-parity: runtime arm probed %d declared operations, %d exempt (x-not-implemented)",
		probed, exempt)
	return findings
}

func parityInstantiate(pattern string) string {
	segs := strings.Split(pattern, "/")
	for i, seg := range segs {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			segs[i] = "00000000-0000-0000-0000-000000000000"
		}
	}
	return strings.Join(segs, "/")
}

func parityPathOf(key string) string {
	_, p, _ := strings.Cut(key, " ")
	return p
}

func paritySortedKeysString(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// parityCommit records the revision the guard ran at (SHIM-SOURCE-003
// requirement: the guard states its commit). Best effort — a source tarball
// without git logs "unknown" rather than failing the check.
func parityCommit(repoRoot string) string {
	out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// TestSpecParityGuard is the machine check: the authored split spec and the
// document the server serves must declare the same path+method set, and every
// declared non-stub operation must be routed (specs/024 §A.3).
func TestSpecParityGuard(t *testing.T) {
	repoRoot := parityRepoRoot(t)
	commit := parityCommit(repoRoot)
	specRoot := paritySetSpecRoot(t, repoRoot)

	declared := loadDeclaredParity(t, specRoot)
	if len(declared.ops) == 0 {
		t.Fatal("declared surface is empty — the guard would be vacuous")
	}

	srv := httptest.NewServer(newFullDeployServer())
	defer srv.Close()
	served := parityFetchServed(t, srv.URL)
	if len(served) == 0 {
		t.Fatal("served surface is empty — the guard would be vacuous")
	}

	findings := parityDiff(declared.ops, served, declared.stubs, declared.orphans)
	findings = append(findings, parityProbeRuntime(t, srv.URL, declared.ops, declared.stubs)...)

	declaredPaths, servedPaths := map[string]bool{}, map[string]bool{}
	for k := range declared.ops {
		declaredPaths[parityPathOf(k)] = true
	}
	for k := range served {
		servedPaths[parityPathOf(k)] = true
	}
	stubList := make([]string, 0, len(declared.stubs))
	for p := range declared.stubs {
		stubList = append(stubList, p)
	}
	sort.Strings(stubList)

	t.Logf("spec-parity: commit=%s spec=%s", commit, specRoot)
	t.Logf("spec-parity: declared=%d paths / %d ops, served=%d paths / %d ops, x-not-implemented path items=%v (exempt from MISSING/UNANSWERED)",
		len(declaredPaths), len(declared.ops), len(servedPaths), len(served), stubList)

	if len(findings) == 0 {
		t.Logf("spec-parity: PASS — declared == served at commit %s (%d paths / %d ops)",
			commit, len(declaredPaths), len(declared.ops))
		return
	}
	for _, finding := range findings {
		t.Errorf("spec-parity drift at commit %s: %s", commit, finding)
	}
}

// TestSpecParityGuardDiffDetectsDrift pins the comparison itself: a guard whose
// diff silently returns nothing would pass TestSpecParityGuard against any
// tree, so the drift arms are exercised here on synthetic surfaces (no server,
// no spec files). The stub exemption is asserted too — that is the one input
// that must NOT produce a MISSING finding.
func TestSpecParityGuardDiffDetectsDrift(t *testing.T) {
	declared := map[string]string{
		"GET /a":    "paths/a.yaml#a",
		"POST /a":   "paths/a.yaml#a",
		"GET /stub": "paths/stub.yaml#stub",
	}
	served := map[string]string{
		"GET /a":    "served",
		"DELETE /b": "served",
		"GET /stub": "served",
	}
	stubs := map[string]bool{"/stub": true}

	findings := parityDiff(declared, served, stubs, []string{"paths/orphan.yaml#/orphan"})

	wantContains := []string{
		"MISSING POST /a — declared in paths/a.yaml#a but absent from the served document",
		"EXTRA DELETE /b — served (served) but not declared by specs/openapi",
		"ORPHAN paths/orphan.yaml#/orphan",
	}
	joined := strings.Join(findings, "\n")
	for _, want := range wantContains {
		if !strings.Contains(joined, want) {
			t.Errorf("diff missed %q; got:\n%s", want, joined)
		}
	}
	if len(findings) != len(wantContains) {
		t.Errorf("expected exactly %d findings, got %d:\n%s", len(wantContains), len(findings), joined)
	}

	// A clean pair must produce nothing, or the guard would fail every tree.
	if clean := parityDiff(declared, declared, stubs, nil); len(clean) != 0 {
		t.Errorf("identical surfaces must not report drift, got %v", clean)
	}
	// The stub exemption is load-bearing: drop the marker and /stub's absence
	// from nothing changes, but a stub missing on the SERVED side must be
	// exempted, not reported.
	missingStub := parityDiff(map[string]string{"GET /stub": "paths/stub.yaml#stub"}, map[string]string{}, stubs, nil)
	if len(missingStub) != 0 {
		t.Errorf("a declared x-not-implemented path item must be exempt from MISSING, got %v", missingStub)
	}
}
