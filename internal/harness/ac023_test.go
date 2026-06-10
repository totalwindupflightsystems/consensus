// AC-023: Iteration commits — snapshot and rollback
// Canonical from SPEC-002 §6
//
// Verifies that:
//   1. Each iteration saves a snapshot to iteration_commits
//   2. Multiple iterations produce multiple commit rows
//   3. Querying an older commit returns the correct data

package harness

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestAC023_IterationCommits_MultipleIterations(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	// Run 3 iterations, each with different data
	iterations := 3
	for i := 1; i <= iterations; i++ {
		is := strconv.Itoa(i)
		llmResp := []byte(`{"internal_monologue":"iter ` + is + `","memory_state_changes":[]}`)
		sqlExec := []string{"SELECT 1", "INSERT INTO memory_events (type, content, session_id, iteration_created) VALUES ('text_block', 'iter." + is + "', '" + sessionID + "', " + is + ")"}

		// Call FinalizeIteration which writes both audit log and iteration snapshot
		errs := th.FinalizeIteration(th.ctx, sessionID, int64(i),
			"internal monologue for iter "+is,
			sqlExec, llmResp, 1, "committed", "")
		if len(errs) > 0 {
			t.Fatalf("FinalizeIteration iter %d: %v", i, errs)
		}
	}

	// Verify 3 rows exist in iteration_commits
	rows, err := th.conn.Query(th.ctx,
		`SELECT iteration_id, llm_response, sql_executed, rows_affected
		 FROM iteration_commits
		 WHERE session_id = $1
		 ORDER BY iteration_id`, sessionID)
	if err != nil {
		t.Fatalf("query iteration_commits: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("AC-023: expected 3 iteration_commit rows, got %d", len(rows))
	}
	t.Logf("AC-023: 3 iteration_commit rows created as expected")

	// Verify each iteration's data
	for i, r := range rows {
		iterID := toInt64(r["iteration_id"])
		rowsAffected := toInt(r["rows_affected"])
		llmRespRaw, _ := r["llm_response"].(string)

		// Parse the LLM response to verify it matches
		var respData map[string]interface{}
		if err := json.Unmarshal([]byte(llmRespRaw), &respData); err == nil {
			if mono, ok := respData["internal_monologue"]; ok {
				expected := "iter " + strconv.Itoa(i+1)
				if mono != expected {
					t.Errorf("iter %d: expected monologue %q, got %v", iterID, expected, mono)
				}
			}
		}

		if rowsAffected != 1 {
			t.Errorf("iter %d: expected rows_affected=1, got %d", iterID, rowsAffected)
		}
		t.Logf("  iter %d: rows_affected=%d llm_response=%s", iterID, rowsAffected, truncateStr(llmRespRaw, 60))
	}

	// Verify SQL executed was stored correctly — query the second iteration
	row2, err := th.conn.Query(th.ctx,
		`SELECT sql_executed FROM iteration_commits
		 WHERE session_id = $1 AND iteration_id = 2`, sessionID)
	if err != nil || len(row2) != 1 {
		t.Fatalf("query iter 2: err=%v count=%d", err, len(row2))
	}
	sqlRaw, ok := row2[0]["sql_executed"].(string)
	if !ok || sqlRaw == "" {
		t.Fatalf("iter 2: sql_executed is empty or not a string, got %T", row2[0]["sql_executed"])
	}
	// Verify it's valid JSON array and contains the right statement
	var sqlArr []string
	if err := json.Unmarshal([]byte(sqlRaw), &sqlArr); err != nil {
		t.Fatalf("iter 2: sql_executed not valid JSON: %v", err)
	}
	if len(sqlArr) < 2 || sqlArr[0] != "SELECT 1" {
		t.Errorf("iter 2: expected SQL to contain 'SELECT 1', got %v", sqlArr)
	}
	t.Logf("AC-023 PASS: iter 2 sql_executed=%v", sqlArr)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
