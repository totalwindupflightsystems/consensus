// Package memory: tests for semantic retrieval.
package memory

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
)

// ============================================================================
// Mock Embedder — deterministic keyword-influenced vectors
// ============================================================================

// mockEmbedder returns vectors where each dimension corresponds to a keyword
// from a known vocabulary. Co-occurring keywords produce similar vectors,
// unrelated keywords produce orthogonal vectors.
type mockEmbedder struct {
	vocab map[string]int // word → dimension index
}

func newMockEmbedder() *mockEmbedder {
	return &mockEmbedder{
		vocab: map[string]int{
			"postgresql":    0,
			"database":      0,
			"migration":     1,
			"schema":        1,
			"security":      2,
			"vulnerability": 2,
			"python":        3,
			"pytest":        3,
			"docker":        4,
			"container":     4,
			"api":           5,
			"endpoint":      5,
			"performance":   6,
			"slow":          6,
		},
	}
}

func (m *mockEmbedder) Embed(ctx context.Context, input string) ([]float64, error) {
	// Create a 1536-dim vector with non-zero values only at keyword dimensions
	vec := make([]float64, 1536)
	lower := strings.ToLower(input)
	words := strings.Fields(lower)
	for _, word := range words {
		word = strings.Trim(word, ".,;:!?\"'()")
		if dim, ok := m.vocab[word]; ok {
			vec[dim] = 1.0
		}
	}
	return vec, nil
}

// ============================================================================
// Vector Math Tests (inlined)
// ============================================================================

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{1, 0, 0}
	sim, err := cosineSimilarity(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if sim < 0.99 {
		t.Errorf("expected ~1.0 for identical vectors, got %.4f", sim)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{0, 1, 0}
	sim, err := cosineSimilarity(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(sim) > 0.01 {
		t.Errorf("expected ~0.0 for orthogonal vectors, got %.4f", sim)
	}
}

func TestCosineSimilarity_MismatchedSizes(t *testing.T) {
	_, err := cosineSimilarity([]float64{1, 2}, []float64{1, 2, 3})
	if err == nil {
		t.Error("expected error for mismatched sizes")
	}
}

func TestParseVector(t *testing.T) {
	vec, err := parseVector("[1.0, 2.0, 3.0]")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3 dims, got %d", len(vec))
	}
	if vec[0] != 1.0 || vec[1] != 2.0 || vec[2] != 3.0 {
		t.Errorf("wrong values: %v", vec)
	}
}

func TestVectorToString_Roundtrip(t *testing.T) {
	original := []float64{0.1234567890, 0.9876543210}
	encoded := vectorToString(original)
	decoded, err := parseVector(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 dims, got %d", len(decoded))
	}
	if math.Abs(decoded[0]-0.1234567890) > 1e-9 {
		t.Errorf("dim 0 mismatch: %.10f", decoded[0])
	}
	if math.Abs(decoded[1]-0.9876543210) > 1e-9 {
		t.Errorf("dim 1 mismatch: %.10f", decoded[1])
	}
}

// ============================================================================
// Mock Embedder Tests
// ============================================================================

func TestMockEmbedder_KeywordMatch(t *testing.T) {
	m := newMockEmbedder()
	vec, err := m.Embed(context.Background(), "Fix the PostgreSQL migration for security")
	if err != nil {
		t.Fatal(err)
	}
	// PostgreSQL → dim 0, migration → dim 1, security → dim 2
	if vec[0] != 1.0 {
		t.Error("expected PostgreSQL keyword in dim 0")
	}
	if vec[1] != 1.0 {
		t.Error("expected migration keyword in dim 1")
	}
	if vec[2] != 1.0 {
		t.Error("expected security keyword in dim 2")
	}
}

func TestMockEmbedder_SimilarQueries(t *testing.T) {
	m := newMockEmbedder()
	v1, _ := m.Embed(context.Background(), "postgresql database migration")
	v2, _ := m.Embed(context.Background(), "database schema postgresql")
	sim, _ := cosineSimilarity(v1, v2)
	// Both hit dim 0 (postgresql/database) → should be similar
	if sim < 0.70 {
		t.Errorf("expected similar vectors (≥0.7), got %.4f", sim)
	}
}

func TestMockEmbedder_DifferentTopics(t *testing.T) {
	m := newMockEmbedder()
	v1, _ := m.Embed(context.Background(), "postgresql migration security")
	v2, _ := m.Embed(context.Background(), "python pytest performance")
	sim, _ := cosineSimilarity(v1, v2)
	// No shared keywords → should be 0 or near-0
	if sim > 0.01 {
		t.Errorf("expected dissimilar vectors (≈0), got %.4f", sim)
	}
}

// ============================================================================
// Retriever Tests (require DB — tested in harness package E2E test)
// ============================================================================

func TestRetriever_NoEmbedder(t *testing.T) {
	r := NewRetriever(nil, nil)
	_, err := r.FindSimilar(context.Background(), "s1", "query", 5)
	if err == nil {
		t.Error("expected error for nil embedder")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestNewRetriever(t *testing.T) {
	m := newMockEmbedder()
	r := NewRetriever(nil, m)
	if r == nil {
		t.Fatal("expected non-nil retriever")
	}
	if r.embedder == nil {
		t.Error("expected embedder to be set")
	}
}

func TestRetrievalResult_String(t *testing.T) {
	r := RetrievalResult{
		Event:      MemoryEvent{ID: 42, Content: "test content"},
		Similarity: 0.95,
	}
	if r.Event.ID != 42 {
		t.Errorf("expected ID 42, got %d", r.Event.ID)
	}
	if r.Similarity != 0.95 {
		t.Errorf("expected similarity 0.95, got %.2f", r.Similarity)
	}
	_ = fmt.Sprintf("%+v", r) // ensure it formats
}

// ============================================================================
// Benchmarks - PERF-001 consensus hot paths
// ============================================================================
//
// These benchmarks measure the per-query cost of semantic retrieval over
// the memory ledger. FindSimilar is the hot path every agent call uses to
// surface relevant prior context; cosineSimilarity / vectorToString /
// parseVector are the inner-loop primitives that compound across thousands
// of similarity computations per retrieval.

// retrievalBenchSchema is the minimum DDL required for the FindSimilar
// benchmark. Mirrors the subset of internal/harness/testdata/migration_test.sql
// that FindSimilar actually queries.
const retrievalBenchSchema = `
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
CREATE TABLE event_embeddings (
    event_id INTEGER NOT NULL,
    model TEXT NOT NULL,
    embedding TEXT NOT NULL,
    dimensions INTEGER NOT NULL DEFAULT 1536,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (event_id)
);
`

// openRetrievalBenchDB opens a fresh SQLite-backed db.DB backed by a temp
// file (so the database/sql pool can share state across connections) and
// applies the minimum DDL.
func openRetrievalBenchDB(b *testing.B) (db.DB, func()) {
	b.Helper()
	tmpFile, err := os.CreateTemp("", "consensus-memory-bench-*.db")
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
	for _, stmt := range strings.Split(retrievalBenchSchema, ";") {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			continue
		}
		if err := conn.Exec(ctx, trimmed); err != nil {
			conn.Close()
			os.Remove(tmpPath)
			b.Fatalf("bench: apply schema: %v", err)
		}
	}
	cleanup := func() {
		conn.Close()
		os.Remove(tmpPath)
	}
	return conn, cleanup
}

// deterministicVector produces a vector of length `dim` whose values are
// derived from the input string. Used to populate event_embeddings rows
// with realistic non-zero vectors and to build inputs for the pure-math
// cosineSimilarity / vectorToString / parseVector benchmarks.
func deterministicVector(input string, dim int) []float64 {
	v := make([]float64, dim)
	hash := uint64(1469598103934665603)
	for i := 0; i < len(input); i++ {
		hash ^= uint64(input[i])
		hash *= 1099511628211
	}
	for i := 0; i < dim; i++ {
		hash = hash*6364136223846793005 + 1442695040888963407
		v[i] = float64(int64(hash)%2000-1000) / 1000.0 // range [-1, 1)
	}
	return v
}

// BenchmarkFindSimilar measures the cost of semantic retrieval over the
// memory ledger with 50 embedded events for the session. This is the
// per-call cost on a session with moderate activity.
func BenchmarkFindSimilar(b *testing.B) {
	conn, cleanup := openRetrievalBenchDB(b)
	defer cleanup()

	ctx := context.Background()
	sessionID := "bench-findsimilar-session"
	if err := conn.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, model_id, status, trust_level, goal) VALUES ($1, 'bench-agent', 'test-model', 'idle', 'high', 'bench')`,
		sessionID); err != nil {
		b.Fatalf("bench: insert session: %v", err)
	}

	const eventCount = 50
	const vectorDim = 1536
	for i := 0; i < eventCount; i++ {
		content := fmt.Sprintf("Event %d: this is a synthetic memory event about postgresql schema design and migration safety.", i)
		if err := conn.Exec(ctx,
			`INSERT INTO memory_events (id, type, content, session_id, iteration_created) VALUES ($1, 'text_block', $2, $3, 1)`,
			i+1, content, sessionID); err != nil {
			b.Fatalf("bench: insert memory_events: %v", err)
		}
		vec := deterministicVector(content, vectorDim)
		if err := conn.Exec(ctx,
			`INSERT INTO event_embeddings (event_id, model, embedding, dimensions) VALUES ($1, 'bench-model', $2, $3)`,
			i+1, vectorToString(vec), vectorDim); err != nil {
			b.Fatalf("bench: insert event_embeddings: %v", err)
		}
	}

	r := NewRetriever(conn, newMockEmbedder())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.FindSimilar(ctx, sessionID, "postgresql migration", 5); err != nil {
			b.Fatalf("FindSimilar: %v", err)
		}
	}
}

// makeBenchVector is a thin alias for deterministicVector kept under the
// more discoverable benchmark name.
func makeBenchVector(seed string, dim int) []float64 {
	return deterministicVector(seed, dim)
}

// BenchmarkCosineSimilarity_384 measures cosine similarity at the small
// embedding dimension used by MiniLM-style models.
func BenchmarkCosineSimilarity_384(b *testing.B) {
	a := makeBenchVector("cosine-384-a", 384)
	c := makeBenchVector("cosine-384-b", 384)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := cosineSimilarity(a, c); err != nil {
			b.Fatalf("cosineSimilarity: %v", err)
		}
	}
}

// BenchmarkCosineSimilarity_768 measures cosine similarity at the
// mid-range dimension used by many open-source embedding models.
func BenchmarkCosineSimilarity_768(b *testing.B) {
	a := makeBenchVector("cosine-768-a", 768)
	c := makeBenchVector("cosine-768-b", 768)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := cosineSimilarity(a, c); err != nil {
			b.Fatalf("cosineSimilarity: %v", err)
		}
	}
}

// BenchmarkCosineSimilarity_1536 measures cosine similarity at the
// production dimension used by OpenAI text-embedding-3-small and friends.
func BenchmarkCosineSimilarity_1536(b *testing.B) {
	a := makeBenchVector("cosine-1536-a", 1536)
	c := makeBenchVector("cosine-1536-b", 1536)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := cosineSimilarity(a, c); err != nil {
			b.Fatalf("cosineSimilarity: %v", err)
		}
	}
}

// BenchmarkVectorToString measures the cost of serializing an embedding
// vector to its JSON string form (the format stored in event_embeddings).
func BenchmarkVectorToString(b *testing.B) {
	v := makeBenchVector("vector-to-string", 1536)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vectorToString(v)
	}
}

// BenchmarkParseVector measures the cost of deserializing an embedding
// vector from its stored JSON string form.
func BenchmarkParseVector(b *testing.B) {
	v := makeBenchVector("parse-vector", 1536)
	s := vectorToString(v)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := parseVector(s); err != nil {
			b.Fatalf("parseVector: %v", err)
		}
	}
}

// BenchmarkVectorRoundtrip measures the full serialize-then-parse cycle
// that runs every time we read an embedding back out of the database.
// This is the combined cost FindSimilar incurs per row.
func BenchmarkVectorRoundtrip(b *testing.B) {
	v := makeBenchVector("vector-roundtrip", 1536)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := vectorToString(v)
		parsed, err := parseVector(s)
		if err != nil {
			b.Fatalf("parseVector: %v", err)
		}
		// Touch parsed to defeat dead-code elimination.
		if len(parsed) != len(v) {
			b.Fatalf("roundtrip length mismatch: %d vs %d", len(parsed), len(v))
		}
	}
}
