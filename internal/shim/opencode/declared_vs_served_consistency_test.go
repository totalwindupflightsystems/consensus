package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// The declared-vs-served comparison (specs/024 §A.3) is a GENERATED artifact:
// scripts/compare-opencode-declared-served.py rewrites
// specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json from the
// pinned declared surface plus the hand-maintained served surface. Nothing in
// the generator is checked into CI, so the artifact is only ever as consistent
// as the last person who regenerated it — and it is regenerated inside ROUTE
// worker branches, which is exactly where a merge can reintroduce a row.
//
// That is not hypothetical. The ROUTE-ADD-020 merge (7ca0e79, conflicts on this
// very file) re-added the drift row for POST
// /api/integration/{integrationID}/connect/key that ROUTE-ADD-019 had removed
// in 2c13465, while keeping its `covered` row — leaving one operation recorded
// as BOTH served-as-declared AND not-served-at-all, and shifting every
// positional SHIM-DRIFT-NNN id after it by one (a stale id that later board
// rows cite, e.g. ROUTE-ADD-023's SHIM-DRIFT-019). The package tests could not
// see it: they key on the stable method+path+operationId tuple, not on the id.
//
// This test pins the artifact's own consistency so that class cannot land
// silently again.
const (
	consistencyArtifactPath = "../../../specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json"
	consistencyServedPath   = "../../../specs/openapi/upstream/consensus-shim-served-surface.yaml"
)

// consistencyOp is one operation row of the artifact, across every bucket that
// enumerates operations.
type consistencyOp struct {
	ID            string `json:"id"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	OperationID   string `json:"operationId"`
	Class         string `json:"class"`
	ServedOutcome string `json:"served_outcome"`
}

// consistencyKey is the semantically stable identity of an operation. The
// generated SHIM-DRIFT-NNN ids are POSITIONAL (assigned in row order), so they
// must never be used to identify an operation.
func consistencyKey(op consistencyOp) string {
	return op.Method + " " + op.Path
}

func consistencyLabel(op consistencyOp) string {
	return fmt.Sprintf("%s %s (%s)", op.Method, op.Path, op.OperationID)
}

func loadConsistencyArtifact(t *testing.T) struct {
	Counts struct {
		ServedRouteEntries int            `json:"served_route_entries"`
		Covered            int            `json:"covered_operations"`
		Drift              int            `json:"drift_declared_not_served"`
		Narrowed           int            `json:"narrowed_error_contract_only"`
		Intentional        int            `json:"intentional_narrowing"`
		Semantic           int            `json:"semantic_different"`
		ServedNotDeclared  int            `json:"drift_served_not_declared"`
		DriftByClass       map[string]int `json:"drift_by_class"`
	} `json:"counts"`
	Covered              []consistencyOp `json:"covered"`
	Drift                []consistencyOp `json:"drift_declared_not_served"`
	ServedNotDeclared    []consistencyOp `json:"drift_served_not_declared"`
	Narrowed             []consistencyOp `json:"narrowed_error_contract_only"`
	IntentionalNarrowing []consistencyOp `json:"intentional_narrowing"`
	SemanticDifferent    []consistencyOp `json:"semantic_different"`
} {
	t.Helper()
	path := filepath.FromSlash(consistencyArtifactPath)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("declared-vs-served artifact not present at %s", consistencyArtifactPath)
	}
	if err != nil {
		t.Fatalf("read %s: %v", consistencyArtifactPath, err)
	}
	var doc struct {
		Counts struct {
			ServedRouteEntries int            `json:"served_route_entries"`
			Covered            int            `json:"covered_operations"`
			Drift              int            `json:"drift_declared_not_served"`
			Narrowed           int            `json:"narrowed_error_contract_only"`
			Intentional        int            `json:"intentional_narrowing"`
			Semantic           int            `json:"semantic_different"`
			ServedNotDeclared  int            `json:"drift_served_not_declared"`
			DriftByClass       map[string]int `json:"drift_by_class"`
		} `json:"counts"`
		Covered              []consistencyOp `json:"covered"`
		Drift                []consistencyOp `json:"drift_declared_not_served"`
		ServedNotDeclared    []consistencyOp `json:"drift_served_not_declared"`
		Narrowed             []consistencyOp `json:"narrowed_error_contract_only"`
		IntentionalNarrowing []consistencyOp `json:"intentional_narrowing"`
		SemanticDifferent    []consistencyOp `json:"semantic_different"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", consistencyArtifactPath, err)
	}
	return doc
}

// TestDeclaredVsServedArtifactIsSelfConsistent asserts the artifact cannot
// contradict itself: its declared counts must match the rows it actually
// enumerates, no operation may be recorded in two mutually exclusive buckets,
// and its positional drift ids must stay a contiguous unique run.
func TestDeclaredVsServedArtifactIsSelfConsistent(t *testing.T) {
	doc := loadConsistencyArtifact(t)

	// Refuse to pass vacuously: an artifact with no rows would satisfy every
	// "no contradictions" assertion below.
	if len(doc.Covered) == 0 || len(doc.Drift) == 0 {
		t.Fatalf("artifact enumerates %d covered and %d drift rows — refusing to pass vacuously",
			len(doc.Covered), len(doc.Drift))
	}

	// --- counts must match the rows actually enumerated ---------------------
	// A hand-edited or merge-resolved artifact leaves the two disagreeing: the
	// stale artifact claimed covered_operations 102 while enumerating 103.
	for _, c := range []struct {
		name string
		want int
		got  int
	}{
		{"counts.covered_operations", len(doc.Covered), doc.Counts.Covered},
		{"counts.drift_declared_not_served", len(doc.Drift), doc.Counts.Drift},
		{"counts.drift_served_not_declared", len(doc.ServedNotDeclared), doc.Counts.ServedNotDeclared},
		{"counts.narrowed_error_contract_only", len(doc.Narrowed), doc.Counts.Narrowed},
		{"counts.intentional_narrowing", len(doc.IntentionalNarrowing), doc.Counts.Intentional},
		{"counts.semantic_different", len(doc.SemanticDifferent), doc.Counts.Semantic},
	} {
		if c.want != c.got {
			t.Errorf("%s = %d but the artifact enumerates %d row(s) — the artifact is stale; "+
				"regenerate with scripts/compare-opencode-declared-served.py and copy the result "+
				"to specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json",
				c.name, c.got, c.want)
		}
	}

	// drift_by_class must account for every drift row (the comparator derives
	// both, so a mismatch means the artifact was edited rather than generated).
	byClass := 0
	for _, n := range doc.Counts.DriftByClass {
		byClass += n
	}
	if byClass != len(doc.Drift) {
		t.Errorf("counts.drift_by_class sums to %d but drift_declared_not_served enumerates %d row(s) (%v)",
			byClass, len(doc.Drift), doc.Counts.DriftByClass)
	}

	// --- no operation may sit in two mutually exclusive buckets -------------
	// This is the merge signature: one operation listed as covered (served as
	// declared) AND as drift (not served at all).
	covered := map[string]consistencyOp{}
	for _, op := range doc.Covered {
		if _, dup := covered[consistencyKey(op)]; dup {
			t.Errorf("covered enumerates %s twice — a duplicated row shifts every positional drift id",
				consistencyLabel(op))
		}
		covered[consistencyKey(op)] = op
	}
	for _, op := range doc.Drift {
		if cov, both := covered[consistencyKey(op)]; both {
			t.Errorf("%s is recorded as BOTH covered (served outcome %q) and drift_declared_not_served "+
				"(class %q): one operation cannot be served-as-declared and not served at all. "+
				"This is the merge-resolved-artifact signature — regenerate the artifact rather than "+
				"hand-editing it.", consistencyLabel(op), cov.ServedOutcome, op.Class)
		}
	}

	// Drift classes carry their own served-outcome contract, and the two must
	// agree: NOT-SERVED means the path has no shim route ("net/http default
	// 404"), so it can name no served code, while OUTCOME-MISMATCH is DEFINED by
	// the code the shim answers that the document does not declare. A row that
	// contradicts its own class is a hand-edited or merge-resolved artifact.
	for _, op := range doc.Drift {
		switch op.Class {
		case "NOT-SERVED":
			if op.ServedOutcome != "" {
				t.Errorf("%s is classified NOT-SERVED (no shim route at all) but names served_outcome %q — "+
					"a route that answers a code is not unserved", consistencyLabel(op), op.ServedOutcome)
			}
		case "OUTCOME-MISMATCH":
			if op.ServedOutcome == "" {
				t.Errorf("%s is classified OUTCOME-MISMATCH but names no served_outcome — that class is "+
					"defined by the served code", consistencyLabel(op))
			}
		}
	}

	// --- positional drift ids must stay a contiguous unique run -------------
	// The comparator numbers drift rows SHIM-DRIFT-001..N in row order. A merge
	// that re-adds a row inserts a duplicate id and pushes the rest down, which
	// is how downstream board rows end up citing an id that means a different
	// operation.
	seen := map[string]string{}
	for i, op := range doc.Drift {
		if op.ID == "" {
			t.Errorf("drift row %d (%s) carries no id", i, consistencyLabel(op))
			continue
		}
		if prev, dup := seen[op.ID]; dup {
			t.Errorf("drift id %s is assigned to two operations: %s and %s — ids are positional, "+
				"so a duplicated id shifts every later row", op.ID, prev, consistencyLabel(op))
		}
		seen[op.ID] = consistencyLabel(op)
		want := fmt.Sprintf("SHIM-DRIFT-%03d", i+1)
		if op.ID != want {
			t.Errorf("drift row %d has id %s, want %s — the positional sequence is broken, "+
				"so every board row citing a SHIM-DRIFT id now points at the wrong operation",
				i, op.ID, want)
		}
	}

	// --- served_route_entries must match the served-surface file ------------
	servedPath := filepath.FromSlash(consistencyServedPath)
	rawServed, err := os.ReadFile(servedPath)
	if os.IsNotExist(err) {
		t.Skipf("served surface not present at %s", consistencyServedPath)
	}
	if err != nil {
		t.Fatalf("read %s: %v", consistencyServedPath, err)
	}
	var served struct {
		Routes []struct {
			Path   string `yaml:"path"`
			Method string `yaml:"method"`
		} `yaml:"routes"`
	}
	if err := yaml.Unmarshal(rawServed, &served); err != nil {
		t.Fatalf("decode %s: %v", consistencyServedPath, err)
	}
	if len(served.Routes) == 0 {
		t.Fatalf("%s enumerates no routes — refusing to pass vacuously", consistencyServedPath)
	}
	if doc.Counts.ServedRouteEntries != len(served.Routes) {
		t.Errorf("counts.served_route_entries = %d but %s enumerates %d route(s) — the artifact was not "+
			"regenerated after the served surface changed", doc.Counts.ServedRouteEntries,
			consistencyServedPath, len(served.Routes))
	}
}
