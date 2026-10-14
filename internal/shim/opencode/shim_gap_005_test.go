// SHIM-GAP-005 — every recorded error-contract narrowing, re-derived with code
// evidence.
//
// ch:trace row=SHIM-GAP-005 spec=specs/018-openapi-contract.md#12.3
//
// specs/018-openapi-contract.md §12.3 is the ledger: one block per narrowed
// operation, each carrying a rationale, `file:line symbol` code evidence and the
// engine a widening would require. Three properties are pinned here so the
// ledger cannot become a claim nobody can check:
//
//  1. CENSUS — the set of operations the comparator
//     (scripts/compare-opencode-declared-served.py) DETECTS as narrowed is
//     exactly the set §12.3 records, and the count §12.3 prints agrees with the
//     artifact. A newly detected narrowing with no ledger block fails this test,
//     and so does a ledger block for an operation that is no longer narrowed.
//  2. EVIDENCE — every `file:line symbol` citation in §12.3 resolves: the cited
//     file exists, the symbol is declared in it, and the recorded line is inside
//     the file. This is the SHIM-SUPPORT-001 lesson applied to this row:
//     documented evidence must be CHECKABLE, not merely asserted. The recorded
//     line is additionally compared against the symbol's current declaration
//     line and reported when it has drifted BECAUSE this package's route files
//     are edited by sibling workers — the hard anchor is the symbol, so a
//     documentation ledger cannot gate an unrelated route commit.
//  3. BEHAVIOUR — every narrowed operation, driven live against a real store,
//     answers a DECLARED ERROR code and never its declared success. That is what
//     makes "recorded narrowing" a fact about the runtime instead of a note.
package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

const (
	shimGap005SpecPath     = "../../../specs/018-openapi-contract.md"
	shimGap005ArtifactPath = "../../../specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json"
)

// shimGap005Narrowing is one row of the artifact's narrowed_error_contract_only.
type shimGap005Narrowing struct {
	ID                string `json:"id"`
	OperationID       string `json:"operationId"`
	Method            string `json:"method"`
	Path              string `json:"path"`
	DeclaredResponses string `json:"declared_responses"`
	ServedOutcome     string `json:"served_outcome"`
	Rationale         string `json:"rationale"`
	Detail            string `json:"detail"`
}

// shimGap005Artifact loads the narrowed rows the comparator detected, plus the
// count the artifact claims for them.
func shimGap005Artifact(t *testing.T) ([]shimGap005Narrowing, int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.FromSlash(shimGap005ArtifactPath))
	if os.IsNotExist(err) {
		t.Skipf("declared-vs-served artifact not present at %s", shimGap005ArtifactPath)
	}
	if err != nil {
		t.Fatalf("read %s: %v", shimGap005ArtifactPath, err)
	}
	var doc struct {
		Counts struct {
			Narrowed int `json:"narrowed_error_contract_only"`
		} `json:"counts"`
		Narrowed []shimGap005Narrowing `json:"narrowed_error_contract_only"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", shimGap005ArtifactPath, err)
	}
	if len(doc.Narrowed) == 0 {
		t.Fatalf("%s enumerates no narrowed rows — refusing to pass vacuously", shimGap005ArtifactPath)
	}
	return doc.Narrowed, doc.Counts.Narrowed
}

// shimGap005Citation is one `file:line symbol` citation from the ledger.
type shimGap005Citation struct {
	File   string
	Line   int
	Symbol string
}

func (c shimGap005Citation) String() string {
	return fmt.Sprintf("%s:%d %s", c.File, c.Line, c.Symbol)
}

// shimGap005LedgerRow is one `#### SHIM-NARROWED-NNN — op (METHOD /path)` block.
type shimGap005LedgerRow struct {
	ID          string
	OperationID string
	Method      string
	Path        string
	Rationale   string
	Widening    string
	Evidence    string
	Citations   []shimGap005Citation
}

var (
	// The block header, e.g. "#### SHIM-NARROWED-012 — pty.get (GET /pty/{ptyID})"
	// with the method+path optionally wrapped in backticks. The pattern tolerates
	// the backticks; the parser trims whatever it captures.
	shimGap005BlockRe = regexp.MustCompile("^#### (SHIM-NARROWED-\\d+) — ([A-Za-z0-9_.]+) \\(`?([A-Z]+) (\\S+?)`?\\)$")
	// `path/to/file.go:123 symbol` — the required evidence shape.
	shimGap005CiteRe = regexp.MustCompile(`([A-Za-z0-9_./-]+\.go):(\d+) ([A-Za-z0-9_]+)`)
	// The count §12.3 prints, so the printed number cannot drift from the rows.
	shimGap005CountRe = regexp.MustCompile(`narrowed_error_contract_only: (\d+)`)
)

// shimGap005LedgerSection returns the §12.3 section text (from its heading to
// the start of §12.4).
func shimGap005LedgerSection(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.FromSlash(shimGap005SpecPath))
	if os.IsNotExist(err) {
		t.Skipf("spec not present at %s", shimGap005SpecPath)
	}
	if err != nil {
		t.Fatalf("read %s: %v", shimGap005SpecPath, err)
	}
	text := string(raw)
	start := strings.Index(text, "### 12.3 ")
	if start < 0 {
		t.Fatalf("%s: §12.3 heading not found — the narrowing ledger is the deliverable of SHIM-GAP-005", shimGap005SpecPath)
	}
	rest := text[start:]
	end := strings.Index(rest, "\n### 12.4")
	if end < 0 {
		t.Fatalf("%s: §12.4 heading not found after §12.3 — cannot bound the ledger", shimGap005SpecPath)
	}
	return rest[:end]
}

// shimGap005ParseLedger parses the §12.3 blocks out of a ledger section.
func shimGap005ParseLedger(section string) []shimGap005LedgerRow {
	var rows []shimGap005LedgerRow
	var cur *shimGap005LedgerRow
	flush := func() {
		if cur != nil {
			rows = append(rows, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := shimGap005BlockRe.FindStringSubmatch(line); m != nil {
			flush()
			cur = &shimGap005LedgerRow{
				ID:          m[1],
				OperationID: strings.Trim(m[2], "`"),
				Method:      strings.Trim(m[3], "`"),
				Path:        strings.Trim(m[4], "`"),
			}
			continue
		}
		if cur == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "- rationale: "):
			cur.Rationale = strings.TrimSpace(strings.TrimPrefix(line, "- rationale: "))
		case strings.HasPrefix(line, "- widening requires: "):
			cur.Widening = strings.TrimSpace(strings.TrimPrefix(line, "- widening requires: "))
		case strings.HasPrefix(line, "- evidence: "):
			cur.Evidence = strings.TrimSpace(strings.TrimPrefix(line, "- evidence: "))
			for _, m := range shimGap005CiteRe.FindAllStringSubmatch(line, -1) {
				n, err := strconv.Atoi(m[2])
				if err != nil {
					continue
				}
				cur.Citations = append(cur.Citations, shimGap005Citation{File: m[1], Line: n, Symbol: m[3]})
			}
		}
	}
	flush()
	return rows
}

// shimGap005VerifyLedger resolves every citation in the ledger against the repo
// tree rooted at root. It returns the problems that must fail the test: a
// missing rationale/widening-requirement, a block with no citation, a citation
// whose file does not resolve, a line outside the file, or a symbol that is not
// declared in the cited file (the fabricated-evidence class).
func shimGap005VerifyLedger(root string, rows []shimGap005LedgerRow) []string {
	var problems []string
	for _, r := range rows {
		label := r.ID + " " + r.OperationID
		if strings.TrimSpace(r.Rationale) == "" {
			problems = append(problems, label+": the block carries no `- rationale:` line")
		}
		if strings.TrimSpace(r.Widening) == "" {
			problems = append(problems, label+": the block carries no `- widening requires:` line — a recorded narrowing must name what would have to be built")
		}
		if len(r.Citations) == 0 {
			problems = append(problems, label+": the block carries no `file:line symbol` citation")
			continue
		}
		for _, c := range r.Citations {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(c.File)))
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: cited file %s does not resolve: %v", label, c.File, err))
				continue
			}
			lines := strings.Split(string(data), "\n")
			if c.Line < 1 || c.Line > len(lines) {
				problems = append(problems, fmt.Sprintf("%s: cited line %s:%d is outside the file (%d lines)", label, c.File, c.Line, len(lines)))
				continue
			}
			word := regexp.MustCompile(`\b` + regexp.QuoteMeta(c.Symbol) + `\b`)
			if !word.Match(data) {
				problems = append(problems, fmt.Sprintf("%s: symbol %q is not declared anywhere in %s — the citation is fabricated or stale", label, c.Symbol, c.File))
			}
		}
	}
	return problems
}

// shimGap005LineDrift reports citations whose recorded line no longer mentions
// the cited symbol — i.e. the code moved and the ledger's line needs re-pointing.
// Reported, never failed: sibling route work shifts these lines and a
// documentation ledger must not gate it (see the file comment).
func shimGap005LineDrift(root string, rows []shimGap005LedgerRow) []string {
	var notes []string
	for _, r := range rows {
		for _, c := range r.Citations {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(c.File)))
			if err != nil {
				continue
			}
			lines := strings.Split(string(data), "\n")
			if c.Line < 1 || c.Line > len(lines) {
				continue
			}
			if strings.Contains(lines[c.Line-1], c.Symbol) {
				continue
			}
			where := "not found in the file"
			word := regexp.MustCompile(`\b` + regexp.QuoteMeta(c.Symbol) + `\b`)
			for i, line := range lines {
				if word.MatchString(line) {
					where = fmt.Sprintf("now at %s:%d", c.File, i+1)
					break
				}
			}
			notes = append(notes, fmt.Sprintf("%s: %s:%d no longer names %s (%s)",
				r.ID, c.File, c.Line, c.Symbol, where))
		}
	}
	return notes
}

// ---------------------------------------------------------------------------
// 1 + 2. The ledger census and its evidence.
// ---------------------------------------------------------------------------

// shimGap005CensusProblems compares the operations the comparator detected as
// narrowed against the operations §12.3 records, in both directions, and returns
// the disagreements. Keying is on the operation identity (not the positional
// SHIM-NARROWED id) so a regeneration that renumbers rows does not produce noise.
func shimGap005CensusProblems(narrowed []shimGap005Narrowing, rows []shimGap005LedgerRow) []string {
	var problems []string
	ledger := map[string]string{}
	for _, r := range rows {
		key := r.OperationID + " " + r.Method + " " + r.Path
		if first, dup := ledger[key]; dup {
			problems = append(problems, fmt.Sprintf("§12.3 records %s twice (%s and %s)", key, first, r.ID))
			continue
		}
		ledger[key] = r.ID
	}
	detected := map[string]bool{}
	for _, r := range narrowed {
		key := r.OperationID + " " + r.Method + " " + r.Path
		detected[key] = true
		if _, ok := ledger[key]; !ok {
			problems = append(problems, fmt.Sprintf("the comparator detects %s as narrowed but specs/018 §12.3 records nothing for it — file the rationale there or widen the operation", key))
		}
	}
	for key, id := range ledger {
		if !detected[key] {
			problems = append(problems, fmt.Sprintf("specs/018 §12.3 (%s) records %s as narrowed but the comparator no longer detects a narrowing there — delete the block or re-point its evidence", id, key))
		}
	}
	return problems
}

func TestShimGap005LedgerRecordsEveryDetectedNarrowing(t *testing.T) {
	narrowed, artifactCount := shimGap005Artifact(t)
	section := shimGap005LedgerSection(t)
	rows := shimGap005ParseLedger(section)

	if len(rows) == 0 {
		t.Fatalf("§12.3 parses to zero ledger blocks — the SHIM-GAP-005 ledger is missing or its block headings changed shape")
	}

	// The count §12.3 prints must agree with the artifact and with the blocks.
	if m := shimGap005CountRe.FindStringSubmatch(section); m != nil {
		if printed, err := strconv.Atoi(m[1]); err == nil {
			if printed != artifactCount || printed != len(narrowed) {
				t.Errorf("§12.3 prints narrowed_error_contract_only: %d but the artifact counts %d and enumerates %d rows",
					printed, artifactCount, len(narrowed))
			}
			if printed != len(rows) {
				t.Errorf("§12.3 prints narrowed_error_contract_only: %d but records %d ledger blocks", printed, len(rows))
			}
		}
	} else {
		t.Errorf("§12.3 carries no `narrowed_error_contract_only: N` count line")
	}

	// Census, both directions.
	if problems := shimGap005CensusProblems(narrowed, rows); len(problems) > 0 {
		t.Errorf("the §12.3 ledger and the comparator disagree:\n  - %s", strings.Join(problems, "\n  - "))
	}

	// Evidence: every citation must resolve to real, declared code.
	if problems := shimGap005VerifyLedger(repoRoot(), rows); len(problems) > 0 {
		t.Errorf("§12.3 documents evidence that does not resolve:\n  - %s", strings.Join(problems, "\n  - "))
	}
	for _, r := range rows {
		t.Logf("ledger %s %s %s %s — %d citation(s): %s", r.ID, r.Method, r.Path, r.OperationID, len(r.Citations), r.Evidence)
		for _, note := range shimGap005LineDrift(repoRoot(), []shimGap005LedgerRow{r}) {
			t.Logf("WARNING (line drift, re-point the ledger): %s", note)
		}
	}
}

// repoRoot is the repository root as seen from this package directory.
func repoRoot() string {
	return filepath.Join("..", "..", "..")
}

// TestShimGap005LedgerGuardRejectsUnbackedBlocks proves the census/evidence
// guard above is not vacuous: a ledger block with a fabricated citation, or with
// no rationale and no widening requirement, must be reported as a problem.
func TestShimGap005LedgerGuardRejectsUnbackedBlocks(t *testing.T) {
	fabricated := `### 12.3 The recorded error-contract narrowings, with code evidence

#### SHIM-NARROWED-900 — made.up.operation (` + "`" + `POST /made/up/{id}` + "`" + `)
- declared: 200 Thing, 400, 404
- served: 404
- rationale: a fabricated entry used to prove the guard can fail.
- evidence: ` + "`" + `internal/shim/opencode/server.go:4242 handleNothingAtAll` + "`" + `
- widening requires: nothing.

#### SHIM-NARROWED-901 — made.up.again (` + "`" + `GET /made/up/{id}` + "`" + `)
- declared: 200 Thing, 400
- served: 400
- evidence: ` + "`" + `internal/shim/opencode/does_not_exist.go:1 whatever` + "`" + `

### 12.4 Reproducing and checking this section
`
	rows := shimGap005ParseLedger(fabricated)
	if len(rows) != 2 {
		t.Fatalf("synthetic ledger parsed %d blocks, want 2 — the parser no longer matches the documented block shape", len(rows))
	}
	problems := shimGap005VerifyLedger(repoRoot(), rows)
	if len(problems) == 0 {
		t.Fatalf("the ledger guard passed a ledger with a fabricated symbol, a missing file, and a block with no rationale/widening line — the guard is vacuous")
	}
	joined := strings.Join(problems, "\n")
	for _, want := range []string{"handleNothingAtAll", "does_not_exist.go", "no `- rationale:` line"} {
		if !strings.Contains(joined, want) {
			t.Errorf("guard problems do not name %q:\n%s", want, joined)
		}
	}

	// The census half must fail in BOTH directions too: a detected narrowing
	// with no ledger block, and a ledger block the comparator no longer detects.
	detected := []shimGap005Narrowing{
		{OperationID: "made.up.operation", Method: "POST", Path: "/made/up/{id}"},
		{OperationID: "not.recorded.anywhere", Method: "GET", Path: "/not/recorded"},
	}
	census := strings.Join(shimGap005CensusProblems(detected, rows), "\n")
	if !strings.Contains(census, "not.recorded.anywhere") {
		t.Errorf("census guard did not report a detected narrowing §12.3 does not record:\n%s", census)
	}
	if !strings.Contains(census, "made.up.again") {
		t.Errorf("census guard did not report a ledger block (SHIM-NARROWED-901) the comparator no longer detects:\n%s", census)
	}
	if got := shimGap005CensusProblems(detected[:1], rows[:1]); len(got) != 0 {
		t.Errorf("census guard flagged an agreeing pair: %v", got)
	}
}

// ---------------------------------------------------------------------------
// 3. The behaviour: no narrowed operation answers its declared success.
// ---------------------------------------------------------------------------

// shimGap005Request is the live request that reaches one narrowed operation with
// a fixture every declared input of that operation accepts.
type shimGap005Request struct {
	Method string
	Path   string
	Body   string
}

// shimGap005Requests maps each detected narrowing to a live request. The battery
// fails if the map and the artifact disagree in either direction, so it cannot
// silently stop covering an operation the comparator reports.
func shimGap005Requests() map[string]shimGap005Request {
	return map[string]shimGap005Request{
		// The settings store resolves mcp.demo, so the resolved-server arm
		// (the declared 400) is the one exercised.
		"mcp.auth.start":        {http.MethodPost, "/mcp/demo/auth", ""},
		"mcp.auth.authenticate": {http.MethodPost, "/mcp/demo/auth/authenticate", ""},
		"mcp.auth.callback":     {http.MethodPost, "/mcp/demo/auth/callback", `{"code":"gap5"}`},
		"pty.get":               {http.MethodGet, "/pty/pty_gap5", ""},
		"pty.update":            {http.MethodPut, "/pty/pty_gap5", ""},
		"pty.remove":            {http.MethodDelete, "/pty/pty_gap5", ""},
		"pty.connect":           {http.MethodGet, "/pty/pty_gap5/connect", ""},
		"pty.connectToken":      {http.MethodPost, "/pty/pty_gap5/connect-token", ""},
		"question.reject":       {http.MethodPost, "/question/que_gap5/reject", ""},
		"question.reply":        {http.MethodPost, "/question/que_gap5/reply", `{"answers":[]}`},
		"part.delete":           {http.MethodDelete, "/session/s_gap5/message/msg-42/part/prt_gap5", ""},
		"part.update":           {http.MethodPatch, "/session/s_gap5/message/msg-42/part/prt_gap5", `{"type":"text","text":"gap5"}`},
		"session.share":         {http.MethodPost, "/session/s_gap5/share", ""},
		"session.unshare":       {http.MethodDelete, "/session/s_gap5/share", ""},
		// ROUTE-ADD-009: no declared parameters and no requestBody, so an
		// empty GET is every declared input the operation accepts.
		"v2.event.subscribe": {http.MethodGet, "/api/event", ""},
	}
}

func TestShimGap005NarrowedOperationsNeverAnswerTheirDeclaredSuccess(t *testing.T) {
	narrowed, _ := shimGap005Artifact(t)
	_, srv := shimGap005Server(t)

	requests := shimGap005Requests()
	covered := map[string]bool{}
	for _, r := range narrowed {
		key := r.OperationID + " " + r.Method + " " + r.Path
		req, ok := requests[r.OperationID]
		if !ok {
			t.Errorf("the comparator detects %s as narrowed but this battery has no live request for it (%s) — add one; an uncovered narrowing is exactly what the ledger must not hide", key, r.ServedOutcome)
			continue
		}
		if covered[r.OperationID] {
			t.Errorf("duplicate artifact row for %s", r.OperationID)
			continue
		}
		covered[r.OperationID] = true

		declared := shimGap005DeclaredCodes(r.DeclaredResponses)
		if len(declared) == 0 {
			t.Errorf("%s: artifact declares no response codes (%q)", key, r.DeclaredResponses)
			continue
		}

		status, _, raw := doShimRequestBody(t, srv.URL, req.Method, req.Path, req.Body)
		if !declared[status] {
			t.Errorf("%s: %s %s answered %d, which is NOT in the declared set %v (body %s)",
				key, req.Method, req.Path, status, r.DeclaredResponses, truncateForError(raw))
			continue
		}
		if status < 300 {
			t.Errorf("%s: %s %s answered the DECLARED SUCCESS %d — specs/018 §12.3 records a narrowing that is not real (body %s)",
				key, req.Method, req.Path, status, truncateForError(raw))
		}
	}

	for op := range requests {
		if !covered[op] {
			t.Errorf("the battery carries a request for %s, which the comparator no longer detects as narrowed — delete it or re-derive the artifact", op)
		}
	}
}

// shimGap005DeclaredCodes parses the artifact's comma-separated declared set.
func shimGap005DeclaredCodes(declared string) map[int]bool {
	codes := map[int]bool{}
	for _, part := range strings.Split(declared, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			codes[n] = true
		}
	}
	return codes
}

func truncateForError(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

// ---------------------------------------------------------------------------
// Fixture
// ---------------------------------------------------------------------------

// shimGap005Server builds the smallest real store the narrowed operations read:
// one idle session, one held message, and the settings rows that resolve the
// MCP server name. Every family therefore answers from real rows rather than
// from a missing table.
func shimGap005Server(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	ctx := context.Background()
	conn, err := driver.Open(ctx, db.Config{
		URL:          "sqlite://" + filepath.Join(t.TempDir(), "shim-gap-005.db"),
		MaxOpenConns: 4,
	})
	if err != nil {
		t.Fatalf("open shim-gap-005 test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, stmt := range []string{
		`CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			iteration INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE memory_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			content TEXT NOT NULL,
			session_id TEXT NOT NULL,
			iteration_created INTEGER NOT NULL
		)`,
		`CREATE TABLE system_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			description TEXT,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO sessions (id, status, iteration) VALUES ('s_gap5', 'idle', 0)`,
		`INSERT INTO memory_events (id, type, content, session_id, iteration_created)
		 VALUES (42, 'user_instruction', 'shim-gap-005 message', 's_gap5', 0)`,
		`INSERT INTO system_settings (key, value) VALUES
			('mcp.demo.type', 'remote'),
			('mcp.demo.url', 'https://example.invalid/mcp')`,
	} {
		if err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare shim-gap-005 test database: %v", err)
		}
	}

	s := NewServer(conn, "test-key", nil, nil)
	s.skipAuth = true
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv
}
