package opencode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// ROUTE-ADD-035 — the committed declared-vs-served artifact must be a faithful,
// SELF-CONSISTENT record of the comparison.
//
// The artifact is a generated file (`scripts/compare-opencode-declared-served.py`,
// see docs/opencode-upstream-refresh-procedure.md §"regenerate"): every
// declared operation is classified into exactly ONE of covered /
// drift_declared_not_served / narrowed_error_contract_only /
// intentional_narrowing / semantic_different, and the `counts` block restates
// those classes. Nothing enforced that until this test, and the artifact was in
// fact inconsistent: `POST /api/integration/{integrationID}/connect/key`
// (v2.integration.connect.key) was listed as `covered` — ROUTE-ADD-019 serves it
// — while a stale `NOT-SERVED` drift row for the same operation ("no shim route
// at all", id SHIM-DRIFT-001) survived the merge, and `counts` still carried the
// pre-merge numbers (covered_operations 102, drift_declared_not_served 67).
//
// A double-classified operation is what makes a stale row look like live work:
// reading only `drift_declared_not_served` re-files an already-served operation
// as a missing route. The gate therefore keys on the operation identity
// (method + path + operationId) rather than the generated ids — the ids are
// POSITIONAL and are renumbered by every regeneration, which is exactly how the
// board rows citing "SHIM-DRIFT-031" for /api/question/request ended up naming
// v2.reference.list in the current artifact.
type artifactOperation struct {
	ID            string `json:"id"`
	Path          string `json:"path"`
	Method        string `json:"method"`
	OperationID   string `json:"operationId"`
	Class         string `json:"class"`
	ServedOutcome string `json:"served_outcome"`
}

// artifactKey is the identity a classification must be unique within: the
// generated `id` is positional and NOT part of it.
func artifactKey(op artifactOperation) string {
	return op.Method + " " + op.Path + " " + op.OperationID
}

func artifactPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "specs", "openapi", "upstream",
		"opencode-declared-vs-served-1.18.33.json")
}

func readArtifact(t *testing.T) []byte {
	t.Helper()
	path := artifactPath(t)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("declared-vs-served artifact not present at %s", path)
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

// artifactDoc is the classification view of the artifact this file needs.
type artifactDoc struct {
	Counts struct {
		DeclaredOperations        int            `json:"declared_operations"`
		CoveredOperations         int            `json:"covered_operations"`
		NarrowedErrorContractOnly int            `json:"narrowed_error_contract_only"`
		NarrowedUnexplained       int            `json:"narrowed_unexplained"`
		IntentionalNarrowing      int            `json:"intentional_narrowing"`
		SemanticDifferent         int            `json:"semantic_different"`
		DriftDeclaredNotServed    int            `json:"drift_declared_not_served"`
		DriftByClass              map[string]int `json:"drift_by_class"`
	} `json:"counts"`
	Covered     []artifactOperation `json:"covered"`
	Drift       []artifactOperation `json:"drift_declared_not_served"`
	Narrowed    []artifactOperation `json:"narrowed_error_contract_only"`
	Intentional []artifactOperation `json:"intentional_narrowing"`
	Semantic    []artifactOperation `json:"semantic_different"`
}

func decodeArtifact(t *testing.T) artifactDoc {
	t.Helper()
	raw := readArtifact(t)
	var doc artifactDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", artifactPath(t), err)
	}
	return doc
}

// TestDeclaredVsServedArtifactHasNoDuplicateObjectKeys guards the shape a
// merge can corrupt: `encoding/json` (and jq) silently keep the LAST of two
// same-named keys in one object, so a duplicated `"path"`/`"operationId"` pair
// makes an object decode as an operation it only half describes. The committed
// artifact carried exactly that — the drift row for
// `POST /api/integration/{integrationID}/connect/oauth` (ROUTE-ADD-020) and its
// sibling `/connect/key` (ROUTE-ADD-019) were textually glued into ONE object
// repeating five keys, so the oauth operation vanished into the key operation's
// row. The comparator emits one `json.dumps` per object, so a generated artifact
// can never contain a repeated key: any occurrence is merge damage.
func TestDeclaredVsServedArtifactHasNoDuplicateObjectKeys(t *testing.T) {
	raw := readArtifact(t)

	type objectFrame struct {
		seen      map[string]bool
		expectKey bool
	}
	var stack []objectFrame
	var dupes []string

	dec := json.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tokenize %s: %v", artifactPath(t), err)
		}
		if delim, ok := tok.(json.Delim); ok {
			switch delim {
			case '{':
				stack = append(stack, objectFrame{seen: map[string]bool{}, expectKey: true})
			case '[':
				stack = append(stack, objectFrame{})
			case '}', ']':
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				// The container was a value: its parent continues with the next
				// key.
				if len(stack) > 0 && stack[len(stack)-1].seen != nil {
					stack[len(stack)-1].expectKey = true
				}
			}
			continue
		}

		if len(stack) == 0 {
			continue
		}
		top := &stack[len(stack)-1]
		if top.seen != nil && top.expectKey {
			key := fmt.Sprint(tok)
			if top.seen[key] {
				dupes = append(dupes, key)
			}
			top.seen[key] = true
			top.expectKey = false
			continue
		}
		if top.seen != nil {
			top.expectKey = true
		}
	}

	if len(dupes) > 0 {
		sort.Strings(dupes)
		t.Errorf("the artifact repeats %d object key(s): %s — a repeated key silently loses the earlier value when decoded",
			len(dupes), strings.Join(dupes, ", "))
	}
}

// TestDeclaredVsServedArtifactIsInternallyConsistent is the invariant gate: the
// artifact's own classification must be well-formed before any consumer (a
// board row, a report, a client) reads a single row out of it.
func TestDeclaredVsServedArtifactIsInternallyConsistent(t *testing.T) {
	doc := decodeArtifact(t)

	classes := []struct {
		name string
		rows []artifactOperation
		want int
	}{
		{"covered", doc.Covered, doc.Counts.CoveredOperations},
		{"drift_declared_not_served", doc.Drift, doc.Counts.DriftDeclaredNotServed},
		{"narrowed_error_contract_only", doc.Narrowed, doc.Counts.NarrowedErrorContractOnly},
		{"intentional_narrowing", doc.Intentional, doc.Counts.IntentionalNarrowing},
		{"semantic_different", doc.Semantic, doc.Counts.SemanticDifferent},
	}

	// Refuse to pass vacuously: an empty or truncated artifact must fail loudly
	// rather than satisfy every set-comparison below.
	if doc.Counts.DeclaredOperations == 0 {
		t.Fatal("artifact declares 0 operations — refusing to pass vacuously")
	}
	for _, c := range classes {
		if len(c.rows) == 0 {
			t.Fatalf("artifact carries 0 %s rows — refusing to pass vacuously", c.name)
		}
	}

	// 1. Each class's array length must equal the count the artifact states for
	//    it. A stale `counts` block is how the double-classified row hid.
	total := 0
	for _, c := range classes {
		if len(c.rows) != c.want {
			t.Errorf("counts.%s = %d but the artifact lists %d rows",
				c.name, c.want, len(c.rows))
		}
		total += len(c.rows)
	}
	if total != doc.Counts.DeclaredOperations {
		t.Errorf("the five classes hold %d operations but counts.declared_operations = %d",
			total, doc.Counts.DeclaredOperations)
	}

	// 2. The drift class breakdown must describe the drift rows themselves.
	if got := len(doc.Drift); got != doc.Counts.DriftDeclaredNotServed {
		t.Errorf("drift rows = %d, want counts.drift_declared_not_served = %d",
			got, doc.Counts.DriftDeclaredNotServed)
	}
	byClass := map[string]int{}
	for _, row := range doc.Drift {
		if row.Class == "" {
			t.Errorf("drift row %s %s (%s) carries no class", row.Method, row.Path, row.ID)
			continue
		}
		byClass[row.Class]++
	}
	classSum := 0
	for class, n := range doc.Counts.DriftByClass {
		classSum += n
		if byClass[class] != n {
			t.Errorf("counts.drift_by_class[%s] = %d but %d drift rows carry that class",
				class, n, byClass[class])
		}
	}
	for class, n := range byClass {
		if _, recorded := doc.Counts.DriftByClass[class]; !recorded {
			t.Errorf("%d drift rows carry class %s, which counts.drift_by_class does not record",
				n, class)
		}
	}
	if classSum != len(doc.Drift) {
		t.Errorf("counts.drift_by_class sums to %d, want %d drift rows", classSum, len(doc.Drift))
	}

	// 3. THE gate: every declared operation is classified exactly ONCE. A row
	//    present in two classes is a leftover of a merge that served an
	//    operation without deleting its drift row.
	owner := map[string]string{}
	var dupes []string
	for _, c := range classes {
		for _, row := range c.rows {
			key := artifactKey(row)
			if prev, seen := owner[key]; seen {
				dupes = append(dupes, fmt.Sprintf("%s — recorded as both %s and %s (id %s)",
					key, prev, c.name, row.ID))
				continue
			}
			owner[key] = c.name
		}
	}
	if len(dupes) > 0 {
		sort.Strings(dupes)
		t.Errorf("the artifact classifies an operation in more than one class (a stale row after a merge):\n  %s",
			strings.Join(dupes, "\n  "))
	}

	// 4. `narrowed_unexplained` is 0 by construction (the comparator aborts on an
	//    unexplained narrowing); a non-zero value means the fail-closed rule was
	//    bypassed in a hand-edit.
	if doc.Counts.NarrowedUnexplained != 0 {
		t.Errorf("counts.narrowed_unexplained = %d, want 0 (the comparator is fail-closed)",
			doc.Counts.NarrowedUnexplained)
	}
}

// TestApiV2ListsRecordedCoveredInArtifact ties the seven ROUTE-ADD-021 read-only
// list operations to the artifact: each must be recorded `covered` with the
// declared 200 served, and none may carry a drift row. This is the check that
// makes the comparator result machine-verifiable instead of hand-read — the
// board rows that re-file these paths as "missing, returns 404" are contradicted
// by the served surface AND by this artifact, and the test fails if the artifact
// ever regresses to calling one of them NOT-SERVED.
func TestApiV2ListsRecordedCoveredInArtifact(t *testing.T) {
	doc := decodeArtifact(t)

	want := map[string]struct{ path, operation string }{
		"/api/location":                     {"/api/location", "v2.location.get"},
		"/api/model":                        {"/api/model", "v2.model.list"},
		"/api/reference":                    {"/api/reference", "v2.reference.list"},
		"/api/skill":                        {"/api/skill", "v2.skill.list"},
		"/api/permission/request":           {"/api/permission/request", "v2.permission.request.list"},
		"/api/permission/saved":             {"/api/permission/saved", "v2.permission.saved.list"},
		"/api/question/request":             {"/api/question/request", "v2.question.request.list"},
		"/api/session/{sessionID}/question": {"/api/session/{sessionID}/question", "v2.session.question.list"},
	}

	covered := map[string]artifactOperation{}
	for _, row := range doc.Covered {
		covered[artifactKey(row)] = row
	}
	for _, row := range doc.Drift {
		if _, isServed := want[row.Path]; isServed {
			t.Errorf("%s %s (%s) is recorded as drift (%s) although ROUTE-ADD-021 serves it",
				row.Method, row.Path, row.OperationID, row.Class)
		}
	}

	for path, op := range want {
		key := "GET " + path + " " + op.operation
		row, ok := covered[key]
		if !ok {
			t.Errorf("GET %s (%s) is not recorded covered in the artifact", path, op.operation)
			continue
		}
		if row.ServedOutcome != "200" {
			t.Errorf("GET %s (%s) recorded covered with served_outcome %q, want 200",
				path, op.operation, row.ServedOutcome)
		}
	}
}
