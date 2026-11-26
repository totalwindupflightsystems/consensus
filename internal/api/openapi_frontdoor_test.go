// Front-door parity guard (WIRE-CONSENSUS-5).
//
// The in-process guard (openapi_parity_test.go, SHIM-SOURCE-003) proves the
// authored spec, the served document, and the handler tree agree. This test
// proves the same surface on the REAL front door: it builds ./cmd/consensus,
// boots the actual binary on a free loopback port against a throwaway SQLite
// database, and then
//
//  1. fetches GET /openapi.json from the running server and takes the
//     declared-operation set from what the binary ITSELF serves (never from
//     specs/), and
//  2. exercises every declared operation with one real HTTP request carrying
//     an admin bearer and a minimal body. An operation counts as exercised
//     when a handler answers — any status except 405 or chi's bare
//     "404 page not found" (a mux miss). A declared-but-unimplemented route
//     (documented 501) and an unmounted one (mux 404) are thereby
//     distinguished, which static route-list comparisons cannot do.
//  3. An operation that cannot safely be exercised must be listed in the
//     committed exclusion file internal/api/testdata/frontdoor-parity-exclusions.json
//     naming the operation and the reason. Silent skipping is forbidden: the
//     test fails on any declared operation that is neither exercised nor
//     excluded, and on any stale exclusion entry (an op that is not declared
//     or was exercised anyway).
//
// Counts reported (board row WIRE-CONSENSUS-5):
//
//	N  declared operations in the served /openapi.json
//	M  operations exercised by a real HTTP request reaching a handler
//	K  operations excluded from exercise (each must be in the exclusion
//	   file with a reason)
//	E  exclusion-file entries consumed for this run (must equal K; a
//	   stale entry fails the run)
//
// PASS requires N == M + K == M + E.
//
// The same run re-probes the five T1 contract defects from
// docs/evidence/t1-openapi-sweep-2026-09-30.md against the real binary, so a
// regression of any of them fails this test:
//
//	T1-D1  POST /api/v1/auth/keys must return the declared 201 (was 200)
//	T1-D2  POST /api/v1/sessions/{id}/tasks must return the declared 201 (was 200)
//	T1-D3  GET /project/{projectId} must answer a declared, typed status
//	       (sweep observed a bare 404 outside the then-declared set; the
//	       contract now documents 200/401/404 and the handler must stay
//	       inside it)
//	T1-D4  GET /permission/{id} must return the documented 200 for an
//	       existing pending approval row (was an unreachable 200)
//	T1-D5  POST /permission/{id}/resolve must return 200 on a well-formed
//	       body (was 500 "no such column: decision_reason")
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

const frontDoorTitle = "Consensus Agent Runtime"

// frontDoorExclusionEntry is one committed exclusion: an operation that this
// guard does not exercise, and why. Silent skipping is forbidden, so every
// unexercised operation must appear here with a reason.
type frontDoorExclusionEntry struct {
	Operation string `json:"operation"` // "<METHOD> <path>" as declared
	Reason    string `json:"reason"`
}

type frontDoorExclusions struct {
	Description string                    `json:"description"`
	Exclusions  []frontDoorExclusionEntry `json:"exclusions"`
}

func loadFrontDoorExclusions(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(parityRepoRoot(t), "internal", "api", "testdata", "frontdoor-parity-exclusions.json"))
	if err != nil {
		t.Fatalf("read exclusion file: %v", err)
	}
	var doc frontDoorExclusions
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse exclusion file: %v", err)
	}
	out := make(map[string]string, len(doc.Exclusions))
	for _, e := range doc.Exclusions {
		if e.Operation == "" || strings.TrimSpace(e.Reason) == "" {
			t.Fatalf("exclusion file: every entry needs a non-empty operation and reason, got %+v", e)
		}
		if _, dup := out[e.Operation]; dup {
			t.Fatalf("exclusion file: duplicate entry for %s", e.Operation)
		}
		out[e.Operation] = e.Reason
	}
	return out
}

// TestFrontDoorParityRealBinary is the whole guard: boot the real binary,
// take the operation set from its own /openapi.json, exercise every operation
// against the live front door, enforce the exclusion ledger, and re-probe the
// five T1 defects.
func TestFrontDoorParityRealBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-binary front-door parity in short mode")
	}

	repoRoot := parityRepoRoot(t)
	bin := frontDoorBuildBinary(t, repoRoot)

	// Seeded rows the exercise and the T1-D4/D5 reprobes address. The binary
	// auto-migrates on boot, so migrate first, seed, then boot.
	dbPath := filepath.Join(t.TempDir(), "frontdoor.db")
	dbURL := "sqlite://" + dbPath
	adminKey := frontDoorInitDB(t, bin, dbURL)

	const sessionID = "68ed0699-367d-4206-9268-25ebab753dcb"  // API session row (sessions FK)
	const approvalID = "00000000-0000-4000-8000-00000000d405" // pending approval row (T1-D4/D5)
	frontDoorSeed(t, dbURL, sessionID, approvalID)

	port := frontDoorReservePort(t)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	serve, logPath := frontDoorBoot(t, bin, repoRoot, port, dbPath)
	client := &http.Client{Timeout: 20 * time.Second}
	frontDoorWaitReady(t, client, baseURL, logPath, serve)

	// The declared surface is what the RUNNING BINARY serves — not specs/.
	served := frontDoorFetchServed(t, client, baseURL)

	// Param values: the seeded session/approval rows give the read paths a
	// real row to answer 200 with; DELETEs always use a synthetic id so the
	// sweep can never destroy its own fixtures mid-run.
	fixtures := map[string]string{
		"sessionId":    sessionID,
		"permissionId": approvalID,
		"approvalId":   approvalID,
		"skillName":    "frontdoor-no-such-skill",
		"toolName":     "frontdoor-no-such-tool",
		"source":       "frontdoor-probe",
		"action":       "frontdoor-probe",
		"projectId":    "frontdoor-probe",
	}
	for _, name := range []string{"memoryId", "messageId", "taskId", "keyId", "qID", "requestId", "authId", "vcsId"} {
		fixtures[name] = "00000000-0000-4000-8000-000000009999"
	}

	exclusions := loadFrontDoorExclusions(t)
	exercised := map[string]int{} // op -> observed status
	var unmounted []string

	for _, op := range paritySortedKeysString(served) {
		method, pattern, _ := strings.Cut(op, " ")
		if _, excluded := exclusions[op]; excluded {
			continue // excluded from exercise; ledger-checked below
		}
		status, body := frontDoorProbe(t, client, baseURL, adminKey, method, pattern, fixtures)
		if status == http.StatusMethodNotAllowed ||
			(status == http.StatusNotFound && strings.Contains(string(body), "page not found")) {
			unmounted = append(unmounted, fmt.Sprintf("%s — mux miss (status %d)", op, status))
			continue
		}
		exercised[op] = status
		if status >= 500 {
			// Visibility, not a finding: a 5xx proves the handler is mounted,
			// but a NEW undocumented 5xx on a minimal request is exactly the
			// T1-D5 defect class, so name it in the run log.
			t.Logf("front-door parity: NOTE %s answered %d on the minimal probe", op, status)
		}
	}

	// Ledger accounting: N == M + K == M + E, no silent skips, no stale
	// exclusion entries.
	nDeclared := len(served)
	mExercised := len(exercised)
	eEntries := 0
	for _, op := range paritySortedKeysString(exclusions) {
		reason := exclusions[op]
		if _, declared := served[op]; !declared {
			t.Errorf("front-door parity: stale exclusion entry %q (not declared by the served /openapi.json) — remove it", op)
			continue
		}
		if _, wasExercised := exercised[op]; wasExercised {
			t.Errorf("front-door parity: stale exclusion entry %q (%s) was exercised anyway — remove it", op, reason)
			continue
		}
		eEntries++
		t.Logf("front-door parity: excluded %q — %s", op, reason)
	}
	kExcluded := nDeclared - mExercised
	if len(unmounted) > 0 {
		for _, f := range unmounted {
			t.Errorf("front-door parity: declared operation is NOT mounted on the real front door: %s", f)
		}
	}
	if kExcluded != eEntries {
		for _, op := range paritySortedKeysString(served) {
			if _, ok := exercised[op]; !ok {
				if _, ok := exclusions[op]; !ok {
					t.Errorf("front-door parity: declared operation %s was neither exercised nor listed in "+
						"internal/api/testdata/frontdoor-parity-exclusions.json — exercise it or exclude it with a named reason", op)
				}
			}
		}
	}

	t.Logf("front-door parity: N=%d declared ops (served /openapi.json), M=%d exercised, K=%d excluded, E=%d exclusion entries",
		nDeclared, mExercised, kExcluded, eEntries)
	histogram := map[string]int{}
	for _, status := range exercised {
		histogram[fmt.Sprintf("%dxx", status/100)]++
	}
	for _, bucket := range frontDoorSortedIntKeys(histogram) {
		t.Logf("front-door parity: exercised status histogram %s: %d", bucket, histogram[bucket])
	}
	if nDeclared == 0 {
		t.Fatal("served document declares no operations — the guard would be vacuous")
	}
	if nDeclared != mExercised+kExcluded || kExcluded != eEntries {
		t.Fatalf("front-door parity FAIL: N=%d != M=%d + K=%d (E=%d) — every declared operation must be exercised or excluded with a named reason",
			nDeclared, mExercised, kExcluded, eEntries)
	}

	// ------------------------------------------------------------------
	// T1 defect reprobes (docs/evidence/t1-openapi-sweep-2026-09-30.md).
	// Each arm asserts BOTH directions: the served document must declare
	// the code, and the real binary must answer it.
	// ------------------------------------------------------------------
	declaredCodes := frontDoorDeclaredResponses(t, served)
	expectDeclared := func(op string, want ...string) {
		t.Helper()
		have := declaredCodes[op]
		for _, w := range want {
			if !have[w] {
				t.Errorf("T1 arm: served /openapi.json does not declare %s for %s (declares %v) — contract regressed", w, op, sortedSet(have))
			}
		}
	}

	// T1-D1 — POST /api/v1/auth/keys: 201 declared and served (was 200).
	expectDeclared("POST /api/v1/auth/keys", "201")
	status, body := frontDoorCall(t, client, baseURL, adminKey, http.MethodPost, "/api/v1/auth/keys", `{"scope":"readonly"}`)
	if status != http.StatusCreated {
		t.Errorf("T1-D1: POST /api/v1/auth/keys = %d, want the declared 201. Body: %s", status, body)
	}

	// T1-D2 — POST /api/v1/sessions/{id}/tasks: 201 declared and served (was 200).
	expectDeclared("POST /api/v1/sessions/{sessionId}/tasks", "201")
	status, body = frontDoorCall(t, client, baseURL, adminKey, http.MethodPost,
		"/api/v1/sessions/"+sessionID+"/tasks", `{"title":"front-door-t1-d2","priority":5}`)
	if status != http.StatusCreated {
		t.Errorf("T1-D2: POST /api/v1/sessions/{id}/tasks = %d, want the declared 201. Body: %s", status, body)
	}

	// T1-D3 — GET /project/{projectId}: the answer must stay inside the
	// declared set and be a typed envelope, not a bare mux 404 (the sweep's
	// defect class: an undeclared status + foreign error body).
	expectDeclared("GET /project/{projectId}", "404")
	status, body = frontDoorCall(t, client, baseURL, adminKey, http.MethodGet, "/project/frontdoor-probe", "")
	if status == http.StatusMethodNotAllowed || strings.Contains(string(body), "page not found") {
		t.Errorf("T1-D3: GET /project/{projectId} is a mux miss (%d %q) — route not mounted", status, body)
	}
	if have := declaredCodes["GET /project/{projectId}"]; !have[fmt.Sprint(status)] {
		t.Errorf("T1-D3: GET /project/{projectId} = %d, outside the declared set %v. Body: %s", status, sortedSet(have), body)
	}

	// T1-D4 — GET /permission/{id}: the documented 200 must be reachable for
	// an existing pending row (was silently masked as 404).
	expectDeclared("GET /permission/{permissionId}", "200")
	status, body = frontDoorCall(t, client, baseURL, adminKey, http.MethodGet, "/permission/"+approvalID, "")
	if status != http.StatusOK {
		t.Errorf("T1-D4: GET /permission/{id} = %d, want the documented 200 for the seeded pending row. Body: %s", status, body)
	} else {
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("T1-D4: 200 body is not JSON: %v (%s)", err, body)
		} else if got["id"] != approvalID || got["status"] != "pending" {
			t.Errorf("T1-D4: 200 body = %v, want the seeded pending row %s", got, approvalID)
		}
	}

	// T1-D5 — POST /permission/{id}/resolve: 200 on a well-formed body (was
	// 500 "no such column: decision_reason").
	expectDeclared("POST /permission/{permissionId}/resolve", "200")
	status, body = frontDoorCall(t, client, baseURL, adminKey, http.MethodPost,
		"/permission/"+approvalID+"/resolve", `{"decision":"approved","reason":"front-door T1-D5 reprobe"}`)
	if status != http.StatusOK {
		t.Errorf("T1-D5: POST /permission/{id}/resolve = %d, want the documented 200 (a 500 here is the schema-drift defect). Body: %s", status, body)
	}

	t.Logf("front-door parity: T1-D1..D5 reprobes pass against the real binary (commit %s)", parityCommit(repoRoot))
}

// frontDoorProbe issues the sweep's minimal request for one declared
// operation: admin bearer, "{}" body for writes, no body otherwise.
func frontDoorProbe(t *testing.T, client *http.Client, baseURL, adminKey, method, pattern string, fixtures map[string]string) (int, []byte) {
	t.Helper()

	segs := strings.Split(pattern, "/")
	for i, seg := range segs {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			name := strings.Trim(seg, "{}")
			if method == http.MethodDelete {
				segs[i] = "00000000-0000-4000-8000-00000000dead" // never destroy a fixture mid-sweep
			} else if v, ok := fixtures[name]; ok {
				segs[i] = v
			} else {
				segs[i] = "00000000-0000-4000-8000-000000009999"
			}
		}
	}
	var body string
	if method == http.MethodPost || method == http.MethodPatch || method == http.MethodPut {
		body = "{}"
	}
	return frontDoorCall(t, client, baseURL, adminKey, method, strings.Join(segs, "/"), body)
}

func frontDoorCall(t *testing.T, client *http.Client, baseURL, adminKey, method, target, body string) (int, []byte) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, baseURL+target, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, target, err)
	}
	req.Header.Set("Authorization", "Bearer "+adminKey)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	// SSE handlers (/api/v1/events, /global/event, /event, GET /mcp) keep the
	// stream open; status + headers are the exercise, the body is never drained.
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, raw
}

func frontDoorFetchServed(t *testing.T, client *http.Client, baseURL string) map[string]string {
	t.Helper()

	resp, err := client.Get(baseURL + "/openapi.json")
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
	if doc.Info.Title != frontDoorTitle {
		t.Fatalf("served document title is %q, not the Consensus contract — refusing to measure a foreign server", doc.Info.Title)
	}
	served := map[string]string{}
	for p, item := range doc.Paths {
		for _, m := range parityMethods(item) {
			served[strings.ToUpper(m)+" "+p] = baseURL + "/openapi.json"
		}
	}
	return served
}

// frontDoorDeclaredResponses extracts each declared operation's response-code
// set from the served document, so the T1 arms can assert both directions
// (declared AND served) against the running binary.
func frontDoorDeclaredResponses(t *testing.T, served map[string]string) map[string]map[string]bool {
	t.Helper()

	resp, err := http.Get(servedURL(served))
	if err != nil {
		t.Fatalf("GET /openapi.json: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// Path items carry non-operation keys (summary, description, parameters),
	// so decode loosely and pick only the HTTP-method keys.
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode /openapi.json responses: %v", err)
	}
	out := map[string]map[string]bool{}
	for p, item := range doc.Paths {
		for m, raw := range item {
			if !isHTTPMethod(m) {
				continue
			}
			var operation struct {
				Responses map[string]json.RawMessage `json:"responses"`
			}
			if err := json.Unmarshal(raw, &operation); err != nil {
				t.Fatalf("decode responses for %s %s: %v", strings.ToUpper(m), p, err)
			}
			codes := map[string]bool{}
			for code := range operation.Responses {
				codes[code] = true
			}
			out[strings.ToUpper(m)+" "+p] = codes
		}
	}
	return out
}

func isHTTPMethod(s string) bool {
	switch strings.ToLower(s) {
	case "get", "post", "put", "patch", "delete", "head", "options", "trace":
		return true
	}
	return false
}

func servedURL(served map[string]string) string {
	for _, v := range served {
		return v
	}
	return ""
}

func sortedSet(set map[string]bool) []string {
	var out []string
	for k := range set {
		out = append(out, k)
	}
	if out == nil {
		out = []string{}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// --- real-binary harness (same shape as df65_tool_handoff_integration_test.go,
// duplicated because that helper set is unexported in package api) ---

func frontDoorBuildBinary(t *testing.T, repoRoot string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "consensus")
	build := exec.Command("go", "build", "-o", bin, "./cmd/consensus")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real consensus binary: %v\n%s", err, out)
	}
	return bin
}

func frontDoorInitDB(t *testing.T, bin, dbURL string) string {
	t.Helper()
	// `consensus init` applies the embedded migrations offline and prints the
	// bootstrap admin key (SPEC-016 §5.2) — that key is what every probe
	// authenticates with. init also bootstraps a .memory-bank/ skeleton in
	// its CWD, so run it in a scratch dir.
	initCmd := exec.Command(bin, "init", "--db-url", dbURL)
	initCmd.Dir = t.TempDir()
	out, err := initCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init scratch database: %v\n%s", err, out)
	}
	keyPattern := regexp.MustCompile(`cs_ak_[A-Za-z0-9_-]{10,}`)
	if key := keyPattern.FindString(string(out)); key != "" {
		return key
	}
	t.Fatalf("init printed no bootstrap admin key:\n%s", out)
	return ""
}

func frontDoorSeed(t *testing.T, dbURL, sessionID, approvalID string) {
	t.Helper()
	ctx := context.Background()
	conn, err := driver.Open(ctx, db.Config{URL: dbURL, MaxOpenConns: 1})
	if err != nil {
		t.Fatalf("open scratch database for seeding: %v", err)
	}
	defer func() { _ = conn.Close() }()
	for _, stmt := range []string{
		// sessions (001) needs agent_name + model_id NOT NULL.
		fmt.Sprintf(`INSERT INTO sessions (id, agent_name, model_id, status, goal)
			VALUES ('%s', 'frontdoor-parity', 'mock', 'idle', 'WIRE-CONSENSUS-5 front-door parity fixture')`, sessionID),
		// approval_requests (008): the T1-D4/D5 schema-drift surface. Real
		// columns only (target_sql/review_notes/reviewed_at/reviewer_id).
		fmt.Sprintf(`INSERT INTO approval_requests (id, session_id, iteration, request_type, description, risk_level, target_sql, status)
			VALUES ('%s', '%s', 2, 'destructive_action', 'WIRE-CONSENSUS-5 pending approval', 'high', 'DROP TABLE temp_cache', 'pending')`,
			approvalID, sessionID),
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("seed scratch database: %v", err)
		}
	}
}

func frontDoorReservePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve scratch port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release scratch port: %v", err)
	}
	return port
}

func frontDoorBoot(t *testing.T, bin, repoRoot string, port int, dbPath string) (*exec.Cmd, string) {
	t.Helper()

	// Hermetic fake OpenAI-compatible upstream: session-scoped routes that
	// reach the harness round-trip against this, never the network.
	var llmCalls atomic.Int64
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		llmCalls.Add(1)
		_, _ = io.WriteString(w, `{"id":"frontdoor-llm","model":"frontdoor-model","choices":[{"message":{"role":"assistant","content":"{\"internal_monologue\":\"idle\",\"memory_state_changes\":[],\"system_actions\":[\"respond\"],\"message_to_user\":\"ok\",\"tool_requests\":[],\"sub_agent_spawns\":[]}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(llm.Close)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "consensus.yaml")
	config := fmt.Sprintf(`server:
  hostname: 127.0.0.1
  port: %d
  pprof_addr: ""
llm:
  default_model: frontdoor-model
  provider: openai
  base_url: %s/v1
  api_key: frontdoor-dummy-key
  max_context_tokens: 128000
  max_output_tokens: 16384
harness:
  heartbeat_interval_seconds: 1
  max_iterations: 3
  max_consecutive_errors: 2
  budget_limit_cents: 100
database:
  url: "sqlite://%s"
  max_open_conns: 4
logging:
  level: warn
compression:
  enabled: false
`, port, llm.URL, dbPath)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write scratch config: %v", err)
	}

	logPath := filepath.Join(tmpDir, "serve.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create serve log: %v", err)
	}
	serve := exec.Command(bin, "--config", configPath, "serve")
	serve.Dir = tmpDir
	serve.Stdout = logFile
	serve.Stderr = logFile
	serve.Env = frontDoorCleanEnv()
	if err := serve.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start consensus serve: %v", err)
	}
	t.Cleanup(func() {
		_ = serve.Process.Kill()
		_ = serve.Wait()
		_ = logFile.Close()
	})
	return serve, logPath
}

// frontDoorWaitReady polls /openapi.json until the real binary answers 200.
func frontDoorWaitReady(t *testing.T, client *http.Client, baseURL, logPath string, serve *exec.Cmd) {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if serve.ProcessState != nil && serve.ProcessState.Exited() {
			t.Fatalf("consensus serve exited before /openapi.json answered\n%s", frontDoorReadLog(logPath))
		}
		resp, err := client.Get(baseURL + "/openapi.json")
		if err == nil {
			code := resp.StatusCode
			_ = resp.Body.Close()
			if code == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("serve readiness timeout\n%s", frontDoorReadLog(logPath))
}

func frontDoorReadLog(path string) string {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "read serve log: " + err.Error()
	}
	return string(contents)
}

func frontDoorSortedIntKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func frontDoorCleanEnv() []string {
	blocked := []string{
		"CONSENSUS_", "OPENROUTER_BASE_URL", "OPENAI_API_KEY", "DEEPSEEK_API_KEY", "LM_STUDIO_BASE_URL",
	}
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		drop := false
		for _, prefix := range blocked {
			if strings.HasPrefix(entry, prefix) {
				drop = true
				break
			}
		}
		if !drop {
			env = append(env, entry)
		}
	}
	return env
}
