// Package harness: ledger coverage for AGENT-INITIATED tool calls.
//
// The API-direct tool_execute endpoint was shown (battery) to leave
// tool_requests empty; these tests prove the AGENT path — an agent
// iteration whose LLM output requests a tool during planning — writes
// both a tool_requests row and an audit_logs row.
//
// axiom:trace work_item=C-GAP-POOL-FOLLOWUP impl=internal/harness/executor.go,internal/harness/audit.go
package harness

import (
	"strings"
	"testing"
)

// TestAgentToolCall_WritesLedgerRows drives a real agent iteration whose
// mock LLM output requests a tool (the agent-initiated execution path,
// executor.go executeInTransaction → tool_requests INSERT + post-commit
// FinalizeIteration → audit_logs INSERT), then asserts both rows land.
func TestAgentToolCall_WritesLedgerRows(t *testing.T) {
	th, err := newTestHarness(newMockLLM(outputWithToolCall()))
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Register the tool the mock output requests (web_scraper) so the
	// executor table state matches a real deployment (execution itself
	// is out of scope here — the ledger writes under test happen during
	// the planning iteration, before any tool runs).
	if err := th.conn.Exec(th.ctx,
		`INSERT INTO tools_registry (id, name, description, hemisphere, handler_type, handler_ref)
		 VALUES ('tool-ws-agt', 'web_scraper', 'Scrapes web pages', 'external', 'subprocess', 'web_scraper_handler')`); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	result, err := th.RunAgentIteration(th.ctx, sessionID)
	if err != nil {
		t.Fatalf("RunAgentIteration: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("iteration status = %q, want success", result.Status)
	}

	// tool_requests: exactly one pending row for the requested tool.
	reqRows, err := th.conn.Query(th.ctx,
		`SELECT id, tool_name, parameters, status FROM tool_requests WHERE session_id = $1`,
		sessionID)
	if err != nil {
		t.Fatalf("query tool_requests: %v", err)
	}
	if len(reqRows) != 1 {
		t.Fatalf("tool_requests rows = %d, want 1 (agent-initiated tool call must write the tool_requests table)", len(reqRows))
	}
	if got := toString(reqRows[0]["tool_name"]); got != "web_scraper" {
		t.Errorf("tool_name = %q, want web_scraper", got)
	}
	if got := toString(reqRows[0]["status"]); got != "pending" {
		t.Errorf("status = %q, want pending", got)
	}
	if params := toString(reqRows[0]["parameters"]); params == "" || params == "{}" {
		t.Errorf("parameters empty, want serialized {\"url\": ...}; got %q", params)
	}

	// audit_logs: the same iteration must record an audit row whose
	// sql_executed includes the tool_requests INSERT.
	auditCount, err := th.assertAuditLogCount(sessionID)
	if err != nil {
		t.Fatalf("query audit_logs: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("audit_logs rows = %d, want 1 (agent-initiated iteration must write the audit_logs table)", auditCount)
	}
	auditRows, err := th.conn.Query(th.ctx,
		`SELECT sql_executed, result FROM audit_logs WHERE session_id = $1`,
		sessionID)
	if err != nil {
		t.Fatalf("query audit_logs rows: %v", err)
	}
	sqlExec := toString(auditRows[0]["sql_executed"])
	if !strings.Contains(sqlExec, "INSERT INTO tool_requests") {
		t.Errorf("audit sql_executed does not mention the tool_requests INSERT; got: %s", sqlExec)
	}
	if got := toString(auditRows[0]["result"]); got != "committed" {
		t.Errorf("audit result = %q, want committed", got)
	}
}
