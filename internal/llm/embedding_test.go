// Package llm: tests for embedding client and cosine similarity.
//
// axiom:trace work_item=vector-compression-01 spec=specs/002-memory.md,specs/011-canonical-definitions.md plan=phase-1/task-1-1 test=internal/llm/embedding_test.go
package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ============================================================================
// Embedding Client Tests
// ============================================================================

func TestEmbeddingClient_Embed_Success(t *testing.T) {
	// Mock server that returns a fixed embedding
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/embeddings" {
			t.Errorf("expected /embeddings, got %s", r.URL.Path)
		}

		// Verify auth header
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}

		// Parse request body to verify model
		var req openaiEmbeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "text-embedding-3-small" {
			t.Errorf("expected model text-embedding-3-small, got %s", req.Model)
		}

		// Return fixed embedding
		resp := openaiEmbeddingResponse{
			Object: "list",
			Data: []openaiEmbeddingData{
				{
					Object:    "embedding",
					Index:     0,
					Embedding: json.RawMessage(`[0.1, 0.2, 0.3, 0.4, 0.5]`),
				},
			},
			Model: "text-embedding-3-small",
			Usage: openaiEmbeddingUsage{
				PromptTokens: 5,
				TotalTokens:  5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewEmbeddingClient(EmbeddingConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "text-embedding-3-small",
	})

	vec, err := client.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	expected := []float64{0.1, 0.2, 0.3, 0.4, 0.5}
	if len(vec) != len(expected) {
		t.Fatalf("expected %d dims, got %d", len(expected), len(vec))
	}
	for i, v := range expected {
		if vec[i] != v {
			t.Errorf("dim %d: expected %f, got %f", i, v, vec[i])
		}
	}
}

func TestEmbeddingClient_EmbedBatch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openaiEmbeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		// Should be an array for batch
		inputs, ok := req.Input.([]any)
		if !ok {
			t.Fatalf("expected array input for batch, got %T", req.Input)
		}
		if len(inputs) != 3 {
			t.Fatalf("expected 3 inputs, got %d", len(inputs))
		}

		resp := openaiEmbeddingResponse{
			Object: "list",
			Data: []openaiEmbeddingData{
				{Object: "embedding", Index: 0, Embedding: json.RawMessage(`[0.1, 0.2]`)},
				{Object: "embedding", Index: 1, Embedding: json.RawMessage(`[0.3, 0.4]`)},
				{Object: "embedding", Index: 2, Embedding: json.RawMessage(`[0.5, 0.6]`)},
			},
			Model: "text-embedding-3-small",
			Usage: openaiEmbeddingUsage{PromptTokens: 15, TotalTokens: 15},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewEmbeddingClient(EmbeddingConfig{BaseURL: server.URL})
	vectors, err := client.EmbedBatch(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}
	if len(vectors) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(vectors))
	}
}

func TestEmbeddingClient_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(openaiEmbeddingResponse{
			Error: &openaiError{
				Message: "Invalid API key",
				Type:    "invalid_request_error",
				Code:    "invalid_api_key",
			},
		})
	}))
	defer server.Close()

	client := NewEmbeddingClient(EmbeddingConfig{BaseURL: server.URL, APIKey: "bad-key"})
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for bad API key, got nil")
	}
	if err.Error() != "embedding: api error (status 400): Invalid API key (type=invalid_request_error, code=invalid_api_key)" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEmbeddingClient_EmptyInput(t *testing.T) {
	client := NewEmbeddingClient(EmbeddingConfig{})
	_, err := client.EmbedBatch(context.Background(), []string{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestEmbeddingClient_MissingData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openaiEmbeddingResponse{
			Object: "list",
			Data:   []openaiEmbeddingData{},
			Model:  "text-embedding-3-small",
		})
	}))
	defer server.Close()

	client := NewEmbeddingClient(EmbeddingConfig{BaseURL: server.URL})
	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

// ============================================================================
// Cosine Similarity Tests
// ============================================================================

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float64{1.0, 0.0, 0.0}
	b := []float64{1.0, 0.0, 0.0}
	sim, err := CosineSimilarity(a, b)
	if err != nil {
		t.Fatalf("CosineSimilarity failed: %v", err)
	}
	if sim != 1.0 {
		t.Errorf("expected 1.0 for identical vectors, got %f", sim)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float64{1.0, 0.0}
	b := []float64{0.0, 1.0}
	sim, err := CosineSimilarity(a, b)
	if err != nil {
		t.Fatalf("CosineSimilarity failed: %v", err)
	}
	if sim != 0.0 {
		t.Errorf("expected 0.0 for orthogonal vectors, got %f", sim)
	}
}

func TestCosineSimilarity_Parallel(t *testing.T) {
	a := []float64{1.0, 2.0, 3.0}
	b := []float64{2.0, 4.0, 6.0} // 2x a
	sim, err := CosineSimilarity(a, b)
	if err != nil {
		t.Fatalf("CosineSimilarity failed: %v", err)
	}
	if sim != 1.0 {
		t.Errorf("expected 1.0 for parallel vectors, got %f", sim)
	}
}

func TestCosineSimilarity_KnownValue(t *testing.T) {
	// cos([1,2,3], [4,5,6]) = (4+10+18)/(sqrt(14)*sqrt(77)) = 32/(3.742*8.775) = 32/32.833 = 0.9746...
	a := []float64{1, 2, 3}
	b := []float64{4, 5, 6}
	sim, err := CosineSimilarity(a, b)
	if err != nil {
		t.Fatalf("CosineSimilarity failed: %v", err)
	}
	expected := 32.0 / (sqrt(14.0) * sqrt(77.0))
	if abs(sim-expected) > 0.0001 {
		t.Errorf("expected ~%f, got %f", expected, sim)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float64{0, 0, 0}
	b := []float64{1, 2, 3}
	sim, err := CosineSimilarity(a, b)
	if err != nil {
		t.Fatalf("CosineSimilarity failed: %v", err)
	}
	if sim != 0.0 {
		t.Errorf("expected 0.0 for zero vector, got %f", sim)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	_, err := CosineSimilarity([]float64{1, 2}, []float64{1, 2, 3})
	if err == nil {
		t.Fatal("expected error for different length vectors")
	}
}

func TestCosineSimilarity_Empty(t *testing.T) {
	_, err := CosineSimilarity([]float64{}, []float64{})
	if err == nil {
		t.Fatal("expected error for empty vectors")
	}
}

// ============================================================================
// Vector Conversion Tests
// ============================================================================

func TestVectorToString(t *testing.T) {
	v := []float64{0.123456789, -1.5, 3.14159}
	s := VectorToString(v)
	if s == "" {
		t.Fatal("expected non-empty string")
	}
	// Verify it's valid JSON
	parsed, err := StringToVector(s)
	if err != nil {
		t.Fatalf("StringToVector failed: %v", err)
	}
	if len(parsed) != len(v) {
		t.Fatalf("length mismatch: %d vs %d", len(parsed), len(v))
	}
}

func TestStringToVector_Invalid(t *testing.T) {
	_, err := StringToVector("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestStringToVector_Empty(t *testing.T) {
	vec, err := StringToVector("[]")
	if err != nil {
		t.Fatalf("StringToVector failed: %v", err)
	}
	if len(vec) != 0 {
		t.Errorf("expected empty vector, got %d elements", len(vec))
	}
}

// ============================================================================
// Helpers
// ============================================================================

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Compile-time check: openaiEmbeddingClient implements EmbeddingClient.
var _ EmbeddingClient = (*openaiEmbeddingClient)(nil)
