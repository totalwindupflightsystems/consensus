// Package memory: semantic retrieval over the append-only memory ledger.
//
// Retriever provides FindSimilar, which embeds a natural-language query and
// returns the top-K memory events ranked by cosine similarity. This is the
// "finding the data that is needed" layer — the #1 missing capability in
// agentic harnesses where data exists but can't be discovered.
//
// The retrieval pipeline:
//  1. Embed the natural-language query via the Embedder interface
//  2. Query event_embeddings JOIN memory_events filtered by session
//  3. Compute cosine similarity between query vector and each event vector
//  4. Sort by similarity descending, return top-K

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/wojons/conscientiousness/internal/db"
)

// Embedder is the minimal embedding interface needed for retrieval.
// This avoids an import cycle with the llm package while remaining
// compatible with llm.EmbeddingClient.
type Embedder interface {
	// Embed generates an embedding vector for a single text input.
	Embed(ctx context.Context, input string) ([]float64, error)
}

// RetrievalResult pairs a MemoryEvent with its similarity score.
type RetrievalResult struct {
	Event      MemoryEvent
	Similarity float64 // cosine similarity [0, 1], 1 = identical
}

// Retriever performs semantic search over memory events using vector embeddings.
// It requires a database connection and an embedding client.
type Retriever struct {
	db     db.DB
	embedder Embedder
}

// NewRetriever creates a new semantic retriever.
func NewRetriever(database db.DB, embedder Embedder) *Retriever {
	return &Retriever{
		db:     database,
		embedder: embedder,
	}
}

// FindSimilar searches the memory ledger for events semantically similar to
// the query, scoped to the given session. Returns results sorted by cosine
// similarity descending, capped at topK.
//
// Only events that have an entry in event_embeddings are considered. Events
// without embeddings are silently skipped.
func (r *Retriever) FindSimilar(ctx context.Context, sessionID string, query string, topK int) ([]RetrievalResult, error) {
	if r.embedder == nil {
		return nil, fmt.Errorf("retrieve: no embedding client configured")
	}
	if topK <= 0 {
		topK = 5
	}

	// 1. Embed the query
	queryVec, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("retrieve: embed query: %w", err)
	}

	// 2. Fetch all embedded events for this session
	rows, err := r.db.Query(ctx, `
		SELECT
			me.id,
			me.type,
			me.content,
			COALESCE(me.summary_text, '') as summary_text,
			COALESCE(dm.mode, 'full') as display_mode,
			me.iteration_created,
			me.session_id,
			COALESCE(me.created_at, datetime('now')) as created_at,
			ee.embedding
		FROM memory_events me
		JOIN event_embeddings ee ON ee.event_id = me.id
		LEFT JOIN display_modes dm ON dm.memory_id = me.id
		WHERE me.session_id = $1
		ORDER BY me.id
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("retrieve: query events: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil // no embedded events for this session
	}

	// 3. Compute cosine similarity for each event
	results := make([]RetrievalResult, 0, len(rows))
	for _, row := range rows {
		event := MemoryEvent{
			ID:               toInt64Safe(row["id"]),
			Type:             toStringSafe(row["type"]),
			Content:          toStringSafe(row["content"]),
			SummaryText:      toStringSafe(row["summary_text"]),
			DisplayMode:      toStringSafe(row["display_mode"]),
			IterationCreated: toInt64Safe(row["iteration_created"]),
			SessionID:        toStringSafe(row["session_id"]),
		}
		// Parse embedding vector from stored JSON text
		embStr := toStringSafe(row["embedding"])
		if embStr == "" {
			continue
		}
		eventVec, err := parseVector(embStr)
		if err != nil {
			continue // skip malformed embeddings
		}

		sim, err := cosineSimilarity(queryVec, eventVec)
		if err != nil {
			continue
		}

		results = append(results, RetrievalResult{
			Event:      event,
			Similarity: sim,
		})
	}

	// 4. Sort by similarity descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	// Cap at topK
	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// ============================================================================
// Vector Math (inlined to avoid import cycle with llm package)
// ============================================================================

// DefaultEmbeddingDimensions is the output dimension of the default model.
const DefaultEmbeddingDimensions = 1536

// vectorToString converts an embedding vector to its JSON string representation.
func vectorToString(v []float64) string {
	var b []byte
	b = append(b, '[')
	for i, val := range v {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, []byte(fmt.Sprintf("%.10f", val))...)
	}
	b = append(b, ']')
	return string(b)
}

// parseVector parses a JSON-formatted embedding vector from a string.
func parseVector(s string) ([]float64, error) {
	var vec []float64
	if err := json.Unmarshal([]byte(s), &vec); err != nil {
		return nil, fmt.Errorf("parse embedding vector: %w", err)
	}
	return vec, nil
}

// cosineSimilarity computes the cosine similarity between two vectors.
// Returns a value in [0, 1] where 1 = identical direction, 0 = orthogonal.
func cosineSimilarity(a, b []float64) (float64, error) {
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
		return 0, nil
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB)), nil
}

// ============================================================================
// Helpers
// ============================================================================

func toInt64Safe(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	}
	return 0
}

func toStringSafe(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
