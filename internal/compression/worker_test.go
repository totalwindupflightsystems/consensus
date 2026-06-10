// Package compression: tests for the background compression worker.
//
// axiom:trace work_item=vector-compression-01 spec=specs/002-memory.md plan=phase-2/task-2-1 test=internal/compression/worker_test.go
package compression

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wojons/conscientiousness/internal/db"
	"github.com/wojons/conscientiousness/internal/llm"
)

// ============================================================================
// Mock Implementations
// ============================================================================

// mockDB implements db.DB for testing.
type mockDB struct {
	backend  db.Backend
	queryFn  func(ctx context.Context, query string, args ...any) ([]db.Row, error)
	queryRowFn func(ctx context.Context, query string, args ...any) (db.Row, error)
	execFn   func(ctx context.Context, query string, args ...any) error
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
