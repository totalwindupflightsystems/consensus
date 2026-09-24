package harness

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

type codedSQLiteError int

func (e codedSQLiteError) Error() string { return fmt.Sprintf("sqlite code %d", e) }
func (e codedSQLiteError) Code() int     { return int(e) }

func TestIsSQLiteBusyErrorRecognizesPrimaryAndSnapshotCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "busy", err: codedSQLiteError(5), want: true},
		{name: "busy snapshot", err: codedSQLiteError(517), want: true},
		{name: "wrapped busy snapshot", err: fmt.Errorf("consume: %w", codedSQLiteError(517)), want: true},
		{name: "locked protocol", err: codedSQLiteError(6), want: false},
		{name: "text without typed code", err: fmt.Errorf("database is locked (517)"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSQLiteBusyError(tt.err); got != tt.want {
				t.Errorf("isSQLiteBusyError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

type busyRetryDB struct {
	beginCalls int
	execCalls  int
}

func (d *busyRetryDB) BeginTx(context.Context) (db.Tx, error) {
	d.beginCalls++
	return &busyRetryTx{owner: d, active: true}, nil
}
func (*busyRetryDB) Exec(context.Context, string, ...any) error { return nil }
func (*busyRetryDB) Query(context.Context, string, ...any) ([]db.Row, error) {
	return nil, nil
}
func (*busyRetryDB) QueryRow(context.Context, string, ...any) (db.Row, error) {
	return nil, nil
}
func (*busyRetryDB) Backend() db.Backend { return db.BackendSQLite }
func (*busyRetryDB) Close() error        { return nil }

type busyRetryTx struct {
	owner  *busyRetryDB
	active bool
}

func (tx *busyRetryTx) Exec(context.Context, string, ...any) error {
	tx.owner.execCalls++
	return codedSQLiteError(517)
}
func (*busyRetryTx) Query(context.Context, string, ...any) ([]db.Row, error) {
	return nil, nil
}
func (*busyRetryTx) QueryRow(context.Context, string, ...any) (db.Row, error) {
	return nil, nil
}
func (*busyRetryTx) SetSessionContext(context.Context, string) error { return nil }
func (tx *busyRetryTx) Commit() error {
	tx.active = false
	return nil
}
func (tx *busyRetryTx) Rollback() error {
	tx.active = false
	return nil
}
func (tx *busyRetryTx) IsActive() bool { return tx.active }

func TestConsumeUserMessagesBusyRetriesAreBounded(t *testing.T) {
	database := &busyRetryDB{}
	h := &Harness{db: database}
	initial := &busyRetryTx{owner: database, active: true}

	finalTx, err := h.markUserMessagesReadWithRetry(context.Background(), initial, "session-1", 1, []PendingUserMessage{{ID: 1}})
	if err == nil || !isSQLiteBusyError(err) {
		t.Fatalf("retry result error = %v, want SQLITE_BUSY_SNAPSHOT", err)
	}
	if got, want := database.execCalls, consumeUserMessagesMaxRetries+1; got != want {
		t.Errorf("consume attempts = %d, want %d", got, want)
	}
	if got, want := database.beginCalls, consumeUserMessagesMaxRetries; got != want {
		t.Errorf("replacement transactions = %d, want %d", got, want)
	}
	if finalTx.IsActive() {
		_ = finalTx.Rollback()
	}
}

type snapshotRaceLLM struct {
	onCall func() error
	calls  int
}

func (l *snapshotRaceLLM) Call(_ context.Context, _ []Message) (*LLMResponse, error) {
	l.calls++
	if l.onCall != nil {
		if err := l.onCall(); err != nil {
			return nil, fmt.Errorf("inject concurrent user message: %w", err)
		}
	}
	return &LLMResponse{
		Output: &AgentOutput{
			InternalMonologue: "Reply after the concurrent message arrived.",
			SystemActions:     []string{"respond"},
			MessageToUser:     "acknowledged",
		},
		ModelID: "test-model",
		Usage: LLMUsage{
			PromptTokens:     11,
			CompletionTokens: 2,
		},
	}, nil
}

func TestInteractivePlanningRetriesBusySnapshotWithoutSecondLLMCall(t *testing.T) {
	llm := &snapshotRaceLLM{}
	th, err := newTestHarness(llm)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `
		UPDATE sessions SET status = 'thinking', iteration = 1 WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("prime session: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `
		INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		VALUES ('user_message', 'first message', $1, 1, CURRENT_TIMESTAMP)`, sessionID); err != nil {
		t.Fatalf("insert initial message: %v", err)
	}

	llm.onCall = func() error {
		llm.onCall = nil
		return th.conn.Exec(th.ctx, `
			INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
			VALUES ('user_message', 'message during planning', $1, 2, CURRENT_TIMESTAMP)`, sessionID)
	}

	cfg := DefaultPlanningConfig()
	cfg.MaxTurns = 1
	result, err := th.RunInteractivePlanning(th.ctx, sessionID, cfg)
	if err != nil {
		t.Fatalf("run planning: %v", err)
	}
	if result == nil || result.Status != "success" {
		t.Fatalf("planning result = %+v, want success", result)
	}
	if llm.calls != 1 {
		t.Fatalf("LLM calls = %d, want exactly one paid call", llm.calls)
	}

	row, err := th.conn.QueryRow(th.ctx, `
		SELECT status, tokens_used_in, tokens_used_out FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	if got := toString(row["status"]); got != "idle" {
		t.Errorf("session status = %q, want idle", got)
	}
	if got := toInt64(row["tokens_used_in"]); got != 11 {
		t.Errorf("input tokens = %d, want 11 (recorded once)", got)
	}
	if got := toInt64(row["tokens_used_out"]); got != 2 {
		t.Errorf("output tokens = %d, want 2 (recorded once)", got)
	}

	rows, err := th.conn.Query(th.ctx, `
		SELECT error_message FROM audit_logs
		WHERE session_id = $1 AND error_message IS NOT NULL`, sessionID)
	if err != nil {
		t.Fatalf("query failure audits: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("failure audit rows = %#v, want none after recovered busy snapshot", rows)
	}
}

func TestHandlePlanningErrorPersistsAuditError(t *testing.T) {
	th, err := newTestHarness(nil)
	if err != nil {
		t.Fatalf("create test harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := th.conn.Exec(th.ctx, `UPDATE sessions SET status = 'planning', iteration = 7 WHERE id = $1`, sessionID); err != nil {
		t.Fatalf("prime session: %v", err)
	}
	tx, err := th.db.BeginTx(th.ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	const failure = "consume user messages: sqlite tx: exec: database is locked (517)"
	result, err := th.handlePlanningError(th.ctx, tx, sessionID, fmt.Errorf("%s", failure))
	if err != nil {
		t.Fatalf("handle planning error: %v", err)
	}
	if result == nil || result.Status != "error" {
		t.Fatalf("planning result = %+v, want error", result)
	}

	row, err := th.conn.QueryRow(th.ctx, `
		SELECT iteration, result, error_message FROM audit_logs
		WHERE session_id = $1 ORDER BY id DESC LIMIT 1`, sessionID)
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if got := toInt64(row["iteration"]); got != 7 {
		t.Errorf("audit iteration = %d, want 7", got)
	}
	if got := toString(row["result"]); got != "rolled_back" {
		t.Errorf("audit result = %q, want rolled_back", got)
	}
	if got := toString(row["error_message"]); !strings.Contains(got, failure) {
		t.Errorf("audit error = %q, want %q", got, failure)
	}
}
