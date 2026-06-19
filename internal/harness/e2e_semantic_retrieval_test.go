// Package harness: E2E test for semantic memory retrieval.
//
// Verifies that the Retriever can search memory events by semantic similarity
// — the #1 missing agentic harness capability: finding data you know is there.
package harness

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/memory"
)

// ============================================================================
// Deterministic mock embedder (keyword-influenced vectors)
// ============================================================================

// keywordEmbedder creates 1536-dim vectors where specific keyword dimensions
// are set to 1.0. Shared keywords → high cosine similarity.
type keywordEmbedder struct {
	vocab map[string]int
}

func newKeywordEmbedder() *keywordEmbedder {
	return &keywordEmbedder{
		vocab: map[string]int{
			"postgresql":      0,
			"database":        0,
			"sqlite":          0,
			"migration":       1,
			"schema":          1,
			"ddl":             1,
			"security":        2,
			"vulnerability":   2,
			"auth":            2,
			"encryption":      2,
			"api":             3,
			"endpoint":        3,
			"rest":            3,
			"http":            3,
			"performance":     4,
			"slow":            4,
			"latency":         4,
			"optimization":    4,
			"testing":         5,
			"pytest":          5,
			"coverage":        5,
			"mock":            5,
			"docker":          6,
			"container":       6,
			"deployment":      6,
		},
	}
}

func (k *keywordEmbedder) Embed(ctx context.Context, input string) ([]float64, error) {
	vec := make([]float64, 1536)
	lower := strings.ToLower(input)
	for word, dim := range k.vocab {
		if strings.Contains(lower, word) {
			vec[dim] = 1.0
		}
	}
	return vec, nil
}

// ============================================================================
// Test: Semantic Retrieval — 20 events, 4 topic clusters
// ============================================================================

func TestE2E_SemanticRetrieval_FindSimilarByTopic(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Insert 20 memory events across 4 topic clusters:
	//   1. Database/migration (IDs ~1-5)
	//   2. Security (IDs ~6-10)
	//   3. API/endpoints (IDs ~11-15)
	//   4. Performance (IDs ~16-20)
	topics := []struct {
		content string
		topic   string
	}{
		// Database cluster
		{"PostgreSQL migration 013 adds the trust_level column to sessions", "database"},
		{"SQLite WAL checkpoint causes latency spikes under heavy writes", "database"},
		{"Database schema version mismatch between SQLite and PostgreSQL backends", "database"},
		{"The migration filterForSQLite function strips PostgreSQL-specific triggers", "database"},
		{"CREATE TABLE IF NOT EXISTS avoids DDL auto-commit issues in SQLite", "database"},

		// Security cluster
		{"Security audit reveals XSS vulnerability in the admin dashboard", "security"},
		{"API key rotation policy requires 90-day expiration for all service accounts", "security"},
		{"Encryption at rest implemented using AES-256-GCM for all memory_events", "security"},
		{"Row-level security prevents session A from reading session B data", "security"},
		{"Authentication bypass discovered in the OAuth callback handler", "security"},

		// API cluster
		{"REST API endpoint POST /api/v1/sessions returns 201 on successful creation", "api"},
		{"HTTP 429 rate limiting applied per API key with sliding window counters", "api"},
		{"OpenAPI spec documents all 47 endpoints with request/response schemas", "api"},
		{"Webhook callback delivers session completion events to registered URLs", "api"},
		{"API versioning strategy uses URL path prefix /api/v1/ with deprecation headers", "api"},

		// Performance cluster
		{"Query performance degrades after 100,000 memory_events due to missing index", "performance"},
		{"Context formatting shows 40% overhead on sessions with 10K+ memory events", "performance"},
		{"Optimization reduced planning latency from 8.2s to 2.6s per iteration", "performance"},
		{"Slow LLM responses cause heartbeat timeouts on sessions with budget exhaustion", "performance"},
		{"Memory pooling reduced allocation rate by 60% in the iteration executor", "performance"},
	}

	embedder := newKeywordEmbedder()
	retriever := memory.NewRetriever(th.conn, embedder)

	// Insert events + compute and store embeddings
	for i, topic := range topics {
		// Insert memory event
		err = th.conn.Exec(th.ctx, `
			INSERT INTO memory_events (type, content, session_id, iteration_created)
			VALUES ('text_block', $1, $2, $3)
		`, topic.content, sessionID, (i/5)+1)
		if err != nil {
			t.Fatalf("insert event %d: %v", i, err)
		}

		// Get the auto-increment ID
		rows, err := th.conn.Query(th.ctx, `SELECT last_insert_rowid() as id`)
		if err != nil || len(rows) == 0 {
			t.Fatalf("get event id %d: %v", i, err)
		}
		eventID := toInt64(rows[0]["id"])

		// Embed and store
		vec, err := embedder.Embed(th.ctx, topic.content)
		if err != nil {
			t.Fatalf("embed event %d: %v", i, err)
		}
		vecJSON := vectorToJSON(vec)
		err = th.conn.Exec(th.ctx, `
			INSERT INTO event_embeddings (event_id, model, embedding, dimensions)
			VALUES ($1, 'mock-keyword-embedder', $2, 1536)
		`, eventID, vecJSON)
		if err != nil {
			t.Fatalf("store embedding %d: %v", i, err)
		}
	}

	t.Logf("✓ inserted %d events with embeddings across 4 topic clusters", len(topics))

	// --- Test 1: Search for database topics ---
	results, err := retriever.FindSimilar(th.ctx, sessionID, "how to fix the PostgreSQL migration schema", 5)
	if err != nil {
		t.Fatalf("search database: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for database query")
	}
	t.Logf("database query → %d results:", len(results))
	for i, r := range results {
		t.Logf("  #%d [sim=%.3f] %s", i+1, r.Similarity, truncate(r.Event.Content, 60))
	}

	// Top result should be database-related
	topContent := results[0].Event.Content
	if !strings.Contains(strings.ToLower(topContent), "postgresql") &&
		!strings.Contains(strings.ToLower(topContent), "migration") &&
		!strings.Contains(strings.ToLower(topContent), "schema") &&
		!strings.Contains(strings.ToLower(topContent), "database") &&
		!strings.Contains(strings.ToLower(topContent), "sqlite") {
		t.Errorf("top result should be database-related, got: %s", truncate(topContent, 80))
	}
	if results[0].Similarity < 0.5 {
		t.Errorf("top database result similarity too low: %.3f", results[0].Similarity)
	}

	// --- Test 2: Search for security topics ---
	results2, err := retriever.FindSimilar(th.ctx, sessionID, "find security vulnerabilities and authentication issues", 5)
	if err != nil {
		t.Fatalf("search security: %v", err)
	}
	if len(results2) == 0 {
		t.Fatal("expected results for security query")
	}
	t.Logf("security query → %d results:", len(results2))
	for i, r := range results2 {
		t.Logf("  #%d [sim=%.3f] %s", i+1, r.Similarity, truncate(r.Event.Content, 60))
	}

	topSecContent := strings.ToLower(results2[0].Event.Content)
	if !strings.Contains(topSecContent, "security") &&
		!strings.Contains(topSecContent, "vulnerab") &&
		!strings.Contains(topSecContent, "auth") &&
		!strings.Contains(topSecContent, "encrypt") {
		t.Errorf("top security result should be security-related, got: %s", truncate(results2[0].Event.Content, 80))
	}

	// --- Test 3: Orthogonal query should return low/no similarity ---
	results3, err := retriever.FindSimilar(th.ctx, sessionID, "quantum computing breakthrough changes everything", 5)
	if err != nil {
		t.Fatalf("search orthogonal: %v", err)
	}
	t.Logf("orthogonal query → %d results (expect low similarity):", len(results3))
	for i, r := range results3 {
		t.Logf("  #%d [sim=%.3f] %s", i+1, r.Similarity, truncate(r.Event.Content, 60))
		if r.Similarity > 0.1 {
			t.Errorf("orthogonal query: expected sim ≤0.1 for unrelated topic, got %.3f on %s",
				r.Similarity, truncate(r.Event.Content, 40))
		}
	}

	// --- Test 4: Session isolation — different session returns nothing ---
	otherSession := "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"
	results4, err := retriever.FindSimilar(th.ctx, otherSession, "postgresql migration", 5)
	if err != nil {
		t.Fatalf("search other session: %v", err)
	}
	if len(results4) != 0 {
		t.Errorf("expected 0 results for different session, got %d", len(results4))
	}

	t.Log("✓ Semantic retrieval E2E test PASSED — topic clusters correctly identified")
}

// ============================================================================
// Test: Empty event_embeddings table returns clean nil
// ============================================================================

func TestE2E_SemanticRetrieval_EmptyTable(t *testing.T) {
	th, err := newTestHarness(newMockLLM(minimalOutput()))
	if err != nil {
		t.Fatalf("create harness: %v", err)
	}
	defer th.close()

	sessionID, err := th.createTestSession()
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	retriever := memory.NewRetriever(th.conn, newKeywordEmbedder())

	// No embeddings stored at all
	results, err := retriever.FindSimilar(th.ctx, sessionID, "anything", 5)
	if err != nil {
		t.Fatalf("search empty: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty table, got %d results", len(results))
	}
	t.Log("✓ Empty table returns nil (not error)")
}

// ============================================================================
// Helpers
// ============================================================================

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func vectorToJSON(v []float64) string {
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
