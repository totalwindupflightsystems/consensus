// Package memory: tests for semantic retrieval.
package memory

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
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
			"postgresql":  0,
			"database":    0,
			"migration":   1,
			"schema":      1,
			"security":    2,
			"vulnerability": 2,
			"python":      3,
			"pytest":      3,
			"docker":      4,
			"container":   4,
			"api":         5,
			"endpoint":    5,
			"performance": 6,
			"slow":        6,
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
