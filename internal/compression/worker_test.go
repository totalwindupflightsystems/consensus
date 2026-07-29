// Package compression: tests for the background compression worker.
//
// axiom:trace work_item=vector-compression-01 spec=specs/002-memory.md plan=phase-2/task-2-1 test=internal/compression/worker_test.go
package compression

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	"github.com/wojons/consensus/internal/llm"
)

// ============================================================================
// Mock Implementations
// ============================================================================

// mockDB implements db.DB for testing.
type mockDB struct {
	backend    db.Backend
	queryFn    func(ctx context.Context, query string, args ...any) ([]db.Row, error)
	queryRowFn func(ctx context.Context, query string, args ...any) (db.Row, error)
	execFn     func(ctx context.Context, query string, args ...any) error
}

func (m *mockDB) BeginTx(ctx context.Context) (db.Tx, error) { return nil, nil }
func (m *mockDB) Exec(ctx context.Context, query string, args ...any) error {
	if m.execFn != nil {
		return m.execFn(ctx, query, args...)
	}
	return nil
}
func (m *mockDB) Query(ctx context.Context, query string, args ...any) ([]db.Row, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, query, args...)
	}
	return nil, nil
}
func (m *mockDB) QueryRow(ctx context.Context, query string, args ...any) (db.Row, error) {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, query, args...)
	}
	return nil, nil
}
func (m *mockDB) Backend() db.Backend { return m.backend }
func (m *mockDB) Close() error        { return nil }

// mockEmbedClient implements llm.EmbeddingClient for testing.
type mockEmbedClient struct {
	vectors map[string][]float64
	err     error
}

func (m *mockEmbedClient) Embed(ctx context.Context, input string) ([]float64, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Return a deterministic vector based on input hash
	if v, ok := m.vectors[input]; ok {
		return v, nil
	}
	// Default vector (1536 dims of 0.01)
	v := make([]float64, 1536)
	for i := range v {
		v[i] = 0.01
	}
	// Make it slightly different based on content length for test stability
	v[0] = float64(len(input)) / 1000.0
	return v, nil
}

func (m *mockEmbedClient) EmbedBatch(ctx context.Context, inputs []string) ([][]float64, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([][]float64, len(inputs))
	for i, input := range inputs {
		v, err := m.Embed(ctx, input)
		if err != nil {
			return nil, err
		}
		result[i] = v
	}
	return result, nil
}

// mockSummarizer implements Summarizer for testing.
type mockSummarizer struct {
	summary string
	err     error
}

func (m *mockSummarizer) Summarize(ctx context.Context, systemPrompt, content, modelID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.summary != "" {
		return m.summary, nil
	}
	// Generate a deterministic summary
	return "Summary of: " + truncateStr(content, 100), nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ============================================================================
// Worker Tests
// ============================================================================

func TestNewWorker(t *testing.T) {
	w := NewWorker(&mockDB{}, &mockEmbedClient{}, &mockSummarizer{}, DefaultWorkerConfig())
	if w == nil {
		t.Fatal("expected non-nil worker")
	}
}

func TestWorkerStartStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	w := NewWorker(&mockDB{}, &mockEmbedClient{}, &mockSummarizer{}, DefaultWorkerConfig())
	w.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	w.Stop()
	// Should not panic
}

func TestWorkerFetchPending(t *testing.T) {
	rows := []db.Row{
		{
			"id":           int64(1),
			"event_id":     int64(100),
			"current_tier": int64(1),
			"next_tier":    int64(2),
			"status":       "pending",
			"attempts":     int64(0),
			"max_attempts": int64(3),
		},
	}

	mock := &mockDB{
		backend: db.BackendSQLite,
		queryFn: func(ctx context.Context, query string, args ...any) ([]db.Row, error) {
			return rows, nil
		},
	}

	w := NewWorker(mock, &mockEmbedClient{}, &mockSummarizer{}, DefaultWorkerConfig())
	items, err := w.fetchPending(context.Background())
	if err != nil {
		t.Fatalf("fetchPending failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].EventID != 100 {
		t.Errorf("expected event_id 100, got %d", items[0].EventID)
	}
}

func TestWorkerFetchMemoryEvent(t *testing.T) {
	row := db.Row{
		"id":         int64(100),
		"content":    "This is a test memory event with content to compress.",
		"session_id": "test-session-uuid",
	}

	mock := &mockDB{
		queryRowFn: func(ctx context.Context, query string, args ...any) (db.Row, error) {
			return row, nil
		},
	}

	w := NewWorker(mock, &mockEmbedClient{}, &mockSummarizer{}, DefaultWorkerConfig())
	me, err := w.fetchMemoryEvent(context.Background(), 100)
	if err != nil {
		t.Fatalf("fetchMemoryEvent failed: %v", err)
	}
	if me.Content != "This is a test memory event with content to compress." {
		t.Errorf("unexpected content: %q", me.Content)
	}
	if me.SessionID != "test-session-uuid" {
		t.Errorf("unexpected session_id: %q", me.SessionID)
	}
}

func TestWorkerProcessOne_Accept(t *testing.T) {
	// Set up a scenario where cosine similarity passes
	// Both original and summary produce similar vectors
	ctx := context.Background()

	var execQueries []string
	mock := &mockDB{
		backend: db.BackendSQLite,
		queryFn: func(ctx context.Context, query string, args ...any) ([]db.Row, error) {
			// Return a mock model_registry result
			if strings.Contains(query, "model_registry") {
				return []db.Row{
					{"model_id": "gpt-4o-mini"},
				}, nil
			}
			return nil, nil
		},
		queryRowFn: func(ctx context.Context, query string, args ...any) (db.Row, error) {
			return db.Row{
				"id":         int64(100),
				"content":    "Test content that will be compressed.",
				"session_id": "session-1",
			}, nil
		},
		execFn: func(ctx context.Context, query string, args ...any) error {
			execQueries = append(execQueries, query)
			return nil
		},
	}

	w := NewWorker(mock, &mockEmbedClient{}, &mockSummarizer{}, DefaultWorkerConfig())

	item := &QueueItem{
		ID:          1,
		EventID:     100,
		CurrentTier: 1,
		Attempts:    0,
		MaxAttempts: 3,
	}

	err := w.processOne(ctx, item)
	if err != nil {
		t.Fatalf("processOne failed: %v", err)
	}

	// Should have executed 4 queries (mark processing, update summary_text, update display_modes, mark completed)
	if len(execQueries) < 4 {
		t.Errorf("expected at least 4 exec queries, got %d", len(execQueries))
	}
}

func TestWorkerProcessOne_EmbedError(t *testing.T) {
	ctx := context.Background()

	mock := &mockDB{
		queryRowFn: func(ctx context.Context, query string, args ...any) (db.Row, error) {
			return db.Row{
				"id":         int64(100),
				"content":    "Test content.",
				"session_id": "session-1",
			}, nil
		},
		execFn: func(ctx context.Context, query string, args ...any) error {
			return nil
		},
	}

	embedClient := &mockEmbedClient{err: assertError{"embedding failed"}}
	w := NewWorker(mock, embedClient, &mockSummarizer{}, DefaultWorkerConfig())

	item := &QueueItem{ID: 1, EventID: 100, CurrentTier: 1}

	err := w.processOne(ctx, item)
	if err == nil {
		t.Fatal("expected error from embed failure")
	}
	if !strings.Contains(err.Error(), "embedding failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// assertError is a simple error type for testing.
type assertError struct{ msg string }

func (e assertError) Error() string { return e.msg }

// ============================================================================
// OpenAI Summarizer Tests
// ============================================================================

func TestOpenAISummarizer_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Role: "assistant", Content: "This is the summary."}},
			},
			Usage: chatUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := NewOpenAISummarizer(server.URL, "test-key")
	summary, err := s.Summarize(context.Background(), "system prompt", "content to summarize", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if summary != "This is the summary." {
		t.Errorf("expected 'This is the summary.', got %q", summary)
	}
}

func TestOpenAISummarizer_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(chatCompletionResponse{
			Error: &chatError{Message: "Invalid model", Type: "invalid_request_error"},
		})
	}))
	defer server.Close()

	s := NewOpenAISummarizer(server.URL, "bad-key")
	_, err := s.Summarize(context.Background(), "system prompt", "content", "bad-model")
	if err == nil {
		t.Fatal("expected error for bad model")
	}
}

// ============================================================================
// Integration: ProcessOne with Mock Server
// ============================================================================

func TestWorkerProcessOneWithMockServer(t *testing.T) {
	// Create a mock OpenAI embeddings and chat completions server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "embeddings") {
			// Return a fixed embedding vector
			resp := map[string]any{
				"object": "list",
				"data": []map[string]any{
					{
						"object": "embedding",
						"index":  0,
						"embedding": []any{
							0.5, 0.25,
						},
					},
				},
				"model": "text-embedding-3-small",
				"usage": map[string]any{
					"prompt_tokens": 5,
					"total_tokens":  5,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Chat completions endpoint
		resp := chatCompletionResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Role: "assistant", Content: "This is a compressed summary of the original content."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create a real embedding client pointing at the mock server
	embedClient := llm.NewEmbeddingClient(llm.EmbeddingConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "text-embedding-3-small",
	})

	// Create a real summarizer pointing at the mock server
	summarizer := NewOpenAISummarizer(server.URL, "test-key")

	// Create mock DB
	var execQueries []string
	mock := &mockDB{
		backend: db.BackendSQLite,
		queryFn: func(ctx context.Context, query string, args ...any) ([]db.Row, error) {
			if strings.Contains(query, "model_registry") {
				return []db.Row{
					{"model_id": "gpt-4o-mini"},
				}, nil
			}
			return nil, nil
		},
		queryRowFn: func(ctx context.Context, query string, args ...any) (db.Row, error) {
			return db.Row{
				"id":         int64(100),
				"content":    "Original test content that needs compression.",
				"session_id": "session-1",
			}, nil
		},
		execFn: func(ctx context.Context, query string, args ...any) error {
			execQueries = append(execQueries, query)
			return nil
		},
	}

	cfg := DefaultWorkerConfig()
	cfg.CosineThreshold = 0.0 // Always accept (since mock embeddings are identical)
	w := NewWorker(mock, embedClient, summarizer, cfg)

	item := &QueueItem{
		ID:          1,
		EventID:     100,
		CurrentTier: 1,
		Attempts:    0,
		MaxAttempts: 3,
	}

	err := w.processOne(context.Background(), item)
	if err != nil {
		t.Fatalf("processOne with mock server failed: %v", err)
	}

	// Should have executed queries
	if len(execQueries) < 4 {
		t.Errorf("expected at least 4 exec queries, got %d", len(execQueries))
	}
}

// ============================================================================
// Helpers
// ============================================================================

// Compile-time interface checks
var (
	_ llm.EmbeddingClient = (*mockEmbedClient)(nil)
	_ Summarizer          = (*mockSummarizer)(nil)
)

// ============================================================================
// Benchmarks - PERF-001 consensus hot paths
// ============================================================================
//
// These benchmarks measure the per-event cost of the background compression
// pipeline. Summarize runs against an httptest server so we measure the
// real HTTP path (marshal request, send, parse response). fetchPending and
// processOne use a real SQLite database (backed by a temp file so the
// database/sql pool can share state across connections) seeded with the
// minimum tables the worker touches.

// compressionBenchSchema is the minimum DDL required for the compression
// worker benchmarks. It mirrors the subset of the production migration
// that the worker actually queries.
const compressionBenchSchema = `
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    agent_name TEXT NOT NULL DEFAULT '',
    model_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'idle',
    trust_level TEXT NOT NULL DEFAULT 'low',
    goal TEXT NOT NULL DEFAULT '',
    heartbeat_at TEXT,
    iteration INTEGER NOT NULL DEFAULT 0,
    parent_id TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE model_registry (
    model_id TEXT PRIMARY KEY,
    tier INTEGER NOT NULL DEFAULT 1,
    max_context INTEGER NOT NULL DEFAULT 8192,
    cost_per_m_in REAL NOT NULL DEFAULT 0,
    cost_per_m_out REAL NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE memory_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    summary_text TEXT,
    session_id TEXT NOT NULL,
    iteration_created INTEGER NOT NULL DEFAULT 0,
    linked_memory_pages TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE display_modes (
    memory_id INTEGER NOT NULL,
    mode TEXT NOT NULL DEFAULT 'full',
    set_at TEXT NOT NULL DEFAULT (datetime('now')),
    set_by_iteration INTEGER NOT NULL DEFAULT 0,
    session_id TEXT NOT NULL,
    PRIMARY KEY (memory_id)
);
CREATE TABLE compression_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER NOT NULL,
    current_tier INTEGER NOT NULL DEFAULT 1,
    next_tier INTEGER NOT NULL DEFAULT 2,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    processed_at TEXT
);
`

// openCompressionBenchDB opens a fresh SQLite-backed db.DB backed by a
// temp file (so the database/sql pool can share state across connections)
// and applies the minimum DDL plus a seed model registry row.
func openCompressionBenchDB(b *testing.B) (db.DB, func()) {
	b.Helper()
	tmpFile, err := os.CreateTemp("", "consensus-compression-bench-*.db")
	if err != nil {
		b.Fatalf("bench: create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	ctx := context.Background()
	conn, err := driver.Open(ctx, db.Config{URL: "sqlite://" + tmpPath})
	if err != nil {
		os.Remove(tmpPath)
		b.Fatalf("bench: open sqlite: %v", err)
	}

	for _, stmt := range strings.Split(compressionBenchSchema, ";") {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			continue
		}
		if err := conn.Exec(ctx, trimmed); err != nil {
			conn.Close()
			os.Remove(tmpPath)
			b.Fatalf("bench: apply schema (%s): %v", trimmed, err)
		}
	}

	if err := conn.Exec(ctx, `INSERT INTO model_registry (model_id, tier, max_context, cost_per_m_in, cost_per_m_out) VALUES ('gpt-4o-mini', 1, 128000, 0.15, 0.60)`); err != nil {
		conn.Close()
		os.Remove(tmpPath)
		b.Fatalf("bench: seed model_registry: %v", err)
	}

	cleanup := func() {
		conn.Close()
		os.Remove(tmpPath)
	}
	return conn, cleanup
}

// seedCompressionQueue populates the compression_queue table with `count`
// pending rows so fetchPending has something to return.
func seedCompressionQueue(b *testing.B, conn db.DB, count int) {
	b.Helper()
	ctx := context.Background()
	for i := 0; i < count; i++ {
		if err := conn.Exec(ctx,
			`INSERT INTO compression_queue (event_id, current_tier, next_tier, status, attempts, max_attempts)
			 VALUES ($1, 1, 2, 'pending', 0, 3)`, int64(i+1)); err != nil {
			b.Fatalf("bench: insert queue row: %v", err)
		}
	}
}

// seedMemoryEvent inserts a single memory_events row used by processOne
// benchmarks so the SELECT inside fetchMemoryEvent returns data.
func seedMemoryEvent(b *testing.B, conn db.DB, sessionID string, eventID int64) {
	b.Helper()
	ctx := context.Background()
	if err := conn.Exec(ctx,
		`INSERT INTO memory_events (id, type, content, session_id, iteration_created)
		 VALUES ($1, 'text_block', $2, $3, 1)`,
		eventID, "Benchmark content: this is the original memory event body that will be compressed by the worker. "+
			"It has enough length to be a realistic input for the summarization path - roughly 200 characters.",
		sessionID); err != nil {
		b.Fatalf("bench: insert memory_events: %v", err)
	}
	if err := conn.Exec(ctx,
		`INSERT INTO display_modes (memory_id, mode, session_id) VALUES ($1, 'full', $2)`,
		eventID, sessionID); err != nil {
		b.Fatalf("bench: insert display_modes: %v", err)
	}
}

// BenchmarkSummarize measures the HTTP round-trip cost of the OpenAI-
// compatible chat completions endpoint used by the compression worker.
// The httptest server returns a deterministic summary so the benchmark
// measures Go-side overhead (marshal request, send, read, unmarshal).
func BenchmarkSummarize(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Role: "assistant", Content: "Compressed summary of the input content."}},
			},
			Usage: chatUsage{PromptTokens: 50, CompletionTokens: 10, TotalTokens: 60},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := NewOpenAISummarizer(server.URL, "bench-key")
	content := "This is the original memory event content that the compression worker will summarize. " +
		"It spans multiple sentences to approximate a realistic input length."
	systemPrompt := CompressionSummaryPrompt(TierCompressed)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Summarize(context.Background(), systemPrompt, content, "gpt-4o-mini"); err != nil {
			b.Fatalf("Summarize: %v", err)
		}
	}
}

// BenchmarkFetchPending measures the SQL load path that runs at the top
// of every poll cycle. The benchmark seeds 10 pending rows so the SELECT
// exercises a realistic workload.
func BenchmarkFetchPending(b *testing.B) {
	conn, cleanup := openCompressionBenchDB(b)
	defer cleanup()

	if err := conn.Exec(context.Background(),
		`INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal) VALUES ('bench-session', 'bench-agent', 'gpt-4o-mini', 'idle', 'high', 'bench')`); err != nil {
		b.Fatalf("bench: insert session: %v", err)
	}

	cfg := DefaultWorkerConfig()
	cfg.BatchSize = 10
	w := NewWorker(conn, &mockEmbedClient{}, &mockSummarizer{}, cfg)
	ctx := context.Background()

	// Seed the queue outside the timed region.
	seedCompressionQueue(b, conn, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := w.fetchPending(ctx); err != nil {
			b.Fatalf("fetchPending: %v", err)
		}
	}
}

// BenchmarkProcessOne measures the end-to-end cost of processing a single
// compression queue item with a mock summarizer (no HTTP overhead) and a
// mock DB that returns the data the worker expects. This isolates the
// Go-side overhead (status updates, embedding math, queue/escalation logic)
// from SQL latency. The worker uses session_id::TEXT casts that are
// Postgres-specific, so we exercise the same code path via the in-memory
// mockDB the existing TestWorkerProcessOneWithMockServer uses.
func BenchmarkProcessOne(b *testing.B) {
	var execQueries []string
	mock := &mockDB{
		backend: db.BackendSQLite,
		queryFn: func(ctx context.Context, query string, args ...any) ([]db.Row, error) {
			if strings.Contains(query, "model_registry") {
				return []db.Row{{"model_id": "gpt-4o-mini"}}, nil
			}
			return nil, nil
		},
		queryRowFn: func(ctx context.Context, query string, args ...any) (db.Row, error) {
			return db.Row{
				"id":         int64(100),
				"content":    "Benchmark content: this is the original memory event body that will be compressed by the worker. It has enough length to be a realistic input for the summarization path - roughly 200 characters.",
				"session_id": "bench-session",
			}, nil
		},
		execFn: func(ctx context.Context, query string, args ...any) error {
			execQueries = append(execQueries, query)
			return nil
		},
	}

	cfg := DefaultWorkerConfig()
	cfg.CosineThreshold = 0.0 // accept every summary (mock embeddings are identical)
	w := NewWorker(mock, &mockEmbedClient{}, &mockSummarizer{}, cfg)

	item := &QueueItem{
		ID:          1,
		EventID:     100,
		CurrentTier: 1,
		Attempts:    0,
		MaxAttempts: 3,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset state so the accept path runs every iteration.
		item.Attempts = 0
		execQueries = execQueries[:0]
		if err := w.processOne(context.Background(), item); err != nil {
			b.Fatalf("processOne: %v", err)
		}
	}
}
