// Package llm: OpenAI-compatible embedding client.
//
// Provides the EmbeddingClient interface for generating text embeddings
// via OpenAI-compatible APIs. Used by the compression pipeline for
// cosine similarity validation (SPEC-002 §8, SPEC-011 §10).
//
// axiom:trace work_item=vector-compression-01 spec=specs/002-memory.md,specs/011-canonical-definitions.md plan=phase-1/task-1-1 impl=internal/llm/embedding.go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ============================================================================
// EmbeddingClient Interface
// ============================================================================

// EmbeddingClient generates text embeddings for vector similarity comparison.
// The embedding model is fixed system-wide (SPEC-011 §10) to ensure cosine
// similarity comparisons are mathematically valid.
type EmbeddingClient interface {
	// Embed generates an embedding vector for a single text input.
	// Returns a float64 slice of dimension 1536 (text-embedding-3-small).
	Embed(ctx context.Context, input string) ([]float64, error)

	// EmbedBatch generates embeddings for multiple text inputs in a single API call.
	// Returns vectors in the same order as inputs.
	EmbedBatch(ctx context.Context, inputs []string) ([][]float64, error)
}

// ============================================================================
// OpenAI-Compatible Embedding Client
// ============================================================================

// openaiEmbeddingClient implements EmbeddingClient via OpenAI's /v1/embeddings endpoint.
// It works with OpenAI, OpenRouter, and any API that implements the same contract.
type openaiEmbeddingClient struct {
	httpClient *http.Client
	baseURL    string // e.g. https://api.openai.com/v1
	apiKey     string
	model      string // e.g. "text-embedding-3-small"
}

// openaiEmbeddingRequest is the request body for POST /v1/embeddings.
type openaiEmbeddingRequest struct {
	Model          string `json:"model"`
	Input          any    `json:"input"` // string or []string
	EncodingFormat string `json:"encoding_format,omitempty"` // "float" (default)
}

// openaiEmbeddingResponse is the response from POST /v1/embeddings.
type openaiEmbeddingResponse struct {
	Object string                `json:"object"`
	Data   []openaiEmbeddingData `json:"data"`
	Model  string                `json:"model"`
	Usage  openaiEmbeddingUsage  `json:"usage"`
	Error  *openaiError          `json:"error,omitempty"`
}

// openaiEmbeddingData is a single embedding result in the response.
type openaiEmbeddingData struct {
	Object    string          `json:"object"`
	Index     int             `json:"index"`
	Embedding json.RawMessage `json:"embedding"` // float array as JSON
}

// openaiEmbeddingUsage holds token usage from the embedding API.
type openaiEmbeddingUsage struct {
	PromptTokens int64 `json:"prompt_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

// EmbeddingConfig holds configuration for the embedding client.
type EmbeddingConfig struct {
	// BaseURL overrides the API base URL. Defaults to OpenAI.
	BaseURL string

	// APIKey is the provider API key (not logged, not stored in evidence).
	APIKey string

	// Model is the embedding model identifier.
	// Default: "text-embedding-3-small"
	Model string

	// Timeout is the HTTP client timeout.
	Timeout time.Duration
}

// DefaultEmbeddingModel is the default embedding model per SPEC-011 §10.
const DefaultEmbeddingModel = "text-embedding-3-small"

// DefaultEmbeddingDimensions is the output dimension of the default model.
const DefaultEmbeddingDimensions = 1536

// NewEmbeddingClient creates an OpenAI-compatible embedding client.
// The base URL defaults to OpenAI but can be overridden for OpenRouter, etc.
func NewEmbeddingClient(cfg EmbeddingConfig) EmbeddingClient {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	model := cfg.Model
	if model == "" {
		model = DefaultEmbeddingModel
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &openaiEmbeddingClient{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		model:      model,
	}
}

// Embed generates an embedding vector for a single text input.
func (c *openaiEmbeddingClient) Embed(ctx context.Context, input string) ([]float64, error) {
	vectors, err := c.EmbedBatch(ctx, []string{input})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embedding: empty response for single input")
	}
	return vectors[0], nil
}

// EmbedBatch generates embeddings for multiple text inputs in a single API call.
func (c *openaiEmbeddingClient) EmbedBatch(ctx context.Context, inputs []string) ([][]float64, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("embedding: no inputs provided")
	}

	startTime := time.Now()

	// Build request — use single string for one input, array for multiple
	var reqInput any
	if len(inputs) == 1 {
		reqInput = inputs[0]
	} else {
		reqInput = inputs
	}

	reqBody := openaiEmbeddingRequest{
		Model:          c.model,
		Input:          reqInput,
		EncodingFormat: "float",
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("embedding: marshal request: %w", err)
	}

	// Build HTTP request
	url := c.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("embedding: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	slog.Debug("embedding: calling API",
		"model", c.model,
		"inputs", len(inputs),
		"url", url,
	)

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		elapsed := time.Since(startTime).Milliseconds()
		return nil, fmt.Errorf("embedding: http request failed after %dms: %w", elapsed, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embedding: read response: %w", err)
	}

	// Parse response
	var embResp openaiEmbeddingResponse
	if err := json.Unmarshal(respBytes, &embResp); err != nil {
		return nil, fmt.Errorf("embedding: parse response (status %d): %w", resp.StatusCode, err)
	}

	// Handle API errors
	if embResp.Error != nil {
		return nil, fmt.Errorf("embedding: api error (status %d): %s (type=%s, code=%s)",
			resp.StatusCode, embResp.Error.Message, embResp.Error.Type, embResp.Error.Code)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("embedding: http %d: %s", resp.StatusCode, string(respBytes))
	}

	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("embedding: no data in response")
	}

	// Parse embedding vectors
	vectors := make([][]float64, len(embResp.Data))
	for i, d := range embResp.Data {
		vec, err := parseEmbeddingVector(d.Embedding)
		if err != nil {
			return nil, fmt.Errorf("embedding: parse vector %d: %w", i, err)
		}
		vectors[d.Index] = vec
	}

	elapsed := time.Since(startTime).Milliseconds()
	slog.Debug("embedding: response received",
		"model", embResp.Model,
		"vectors", len(vectors),
		"elapsed_ms", elapsed,
		"prompt_tokens", embResp.Usage.PromptTokens,
	)

	return vectors, nil
}

// ============================================================================
// Embedding Vector Helpers
// ============================================================================

// parseEmbeddingVector parses a JSON float array into a Go float64 slice.
func parseEmbeddingVector(raw json.RawMessage) ([]float64, error) {
	// Handle both [0.1, 0.2, ...] and [1, 2, ...] (int) formats
	var vec []float64
	if err := json.Unmarshal(raw, &vec); err != nil {
		// Try int64 array (some providers return integers)
		var intVec []int64
		if err2 := json.Unmarshal(raw, &intVec); err2 != nil {
			return nil, fmt.Errorf("cannot parse embedding vector: %w", err)
		}
		vec = make([]float64, len(intVec))
		for i, v := range intVec {
			vec[i] = float64(v)
		}
	}
	return vec, nil
}

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns a value in [0, 1] where 1 = identical direction, 0 = orthogonal.
// SPEC-002 §8.2, SPEC-011 §10.
func CosineSimilarity(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("cosine similarity: vector size mismatch: %d vs %d", len(a), len(b))
	}
	if len(a) == 0 {
		return 0, fmt.Errorf("cosine similarity: empty vectors")
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0, nil // Zero vectors have no direction; similarity is 0
	}

	return dot / (sqrt(normA) * sqrt(normB)), nil
}

// sqrt computes the square root of a float64 using Newton's method.
// Used instead of math.Sqrt to avoid importing the math package for a single call.
// For production code, prefer math.Sqrt — this is a standalone helper.
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton's method: z = z - (z*z - x) / (2*z)
	z := x
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// VectorToString converts an embedding vector to its JSON string representation
// for storage as TEXT or JSONB in the database.
func VectorToString(v []float64) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, val := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(fmt.Sprintf("%.10f", val))
	}
	b.WriteByte(']')
	return b.String()
}

// StringToVector parses a JSON-formatted embedding vector from a string.
func StringToVector(s string) ([]float64, error) {
	var vec []float64
	if err := json.Unmarshal([]byte(s), &vec); err != nil {
		return nil, fmt.Errorf("parse embedding string: %w", err)
	}
	return vec, nil
}
