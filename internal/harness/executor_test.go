// Package harness: fixLLMSQLDefaults regression tests (BUG-011).
//
// BUG-011: the harness executor used SQLite-only datetime('now') in session
// claim/heartbeat SQL, breaking PG deployments (SQLSTATE 42883). The fix
// swapped all SQL literals to backend-neutral CURRENT_TIMESTAMP and taught
// fixLLMSQLDefaults to emit CURRENT_TIMESTAMP when stripping DEFAULT(...)
// wrappers from LLM-generated INSERT VALUES clauses.
package harness

import "testing"

func TestFixLLMSQLDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "datetime now wrapped",
			in:   `INSERT INTO t (a, created_at) VALUES (1, DEFAULT (datetime('now')))`,
			want: `INSERT INTO t (a, created_at) VALUES (1, CURRENT_TIMESTAMP)`,
		},
		{
			name: "current timestamp wrapped",
			in:   `INSERT INTO t (a, created_at) VALUES (1, DEFAULT (CURRENT_TIMESTAMP))`,
			want: `INSERT INTO t (a, created_at) VALUES (1, CURRENT_TIMESTAMP)`,
		},
		{
			name: "current timestamp wrapped lowercase",
			in:   `INSERT INTO t (a, created_at) VALUES (1, DEFAULT (current_timestamp))`,
			want: `INSERT INTO t (a, created_at) VALUES (1, CURRENT_TIMESTAMP)`,
		},
		{
			name: "args variant keeps bare datetime",
			in:   `INSERT INTO t (a, expires_at) VALUES (1, DEFAULT (datetime('now', '+1 hour')))`,
			want: `INSERT INTO t (a, expires_at) VALUES (1, datetime('now', '+1 hour'))`,
		},
		{
			name: "no default wrapper untouched",
			in:   `INSERT INTO t (a, created_at) VALUES (1, CURRENT_TIMESTAMP)`,
			want: `INSERT INTO t (a, created_at) VALUES (1, CURRENT_TIMESTAMP)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixLLMSQLDefaults(tt.in)
			if got != tt.want {
				t.Errorf("fixLLMSQLDefaults(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}
