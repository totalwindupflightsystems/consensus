// Package compression: background worker for memory compression pipeline.
//
// The compression worker runs as a goroutine, polling the compression_queue
// table for pending events, generating summaries with model cascade,
// validating via cosine similarity, and escalating through display tiers.
//
// Flow per event:
//  1. SELECT content FROM memory_events
//  2. Generate embedding₁ = embed(content)
//  3. Select tier model from model_registry
//  4. Generate summary via LLM
//  5. Generate embedding₂ = embed(summary)
//  6. Cosine similarity check
//  7. Accept (write summary_text + display_modes) or escalate/fail
//
// axiom:trace work_item=vector-compression-01 spec=specs/002-memory.md,specs/008-harness.md plan=phase-2/task-2-1 impl=internal/compression/worker.go
package compression

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/llm"
)

// ============================================================================
// Compression Queue Schema (from migration 001)
// ============================================================================
//
// CREATE TABLE compression_queue (
//     id                BIGSERIAL PRIMARY KEY,
//     event_id          BIGINT NOT NULL REFERENCES memory_events(id),
//     current_tier      INT NOT NULL DEFAULT 1,
//     next_tier         INT NOT NULL DEFAULT 2,
//     status            TEXT NOT NULL DEFAULT 'pending'
//                       CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
//     attempts          INT NOT NULL DEFAULT 0,
//     max_attempts      INT NOT NULL DEFAULT 3,
//     created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
//     processed_at      TIMESTAMPTZ
// );

// ============================================================================
// Compression Queue Item
// ============================================================================

// QueueItem represents a row from the compression_queue table.
type QueueItem struct {
	ID          int64
	EventID     int64
	CurrentTier int
	NextTier    int
	Status      string
	Attempts    int
	MaxAttempts int
	CreatedAt   time.Time
}

// MemoryEventContent holds content and session info from memory_events.
type MemoryEventContent struct {
	ID        int64
	Content   string
	SessionID string
}

// ============================================================================
// Summarizer Interface
// ============================================================================

// Summarizer generates text summaries via an LLM.
// This is a minimal interface to avoid coupling with harness.AgentOutput parsing.
type Summarizer interface {
	// Summarize sends a system prompt and content to an LLM and returns the
	// generated summary text.
	Summarize(ctx context.Context, systemPrompt, content string, modelID string) (string, error)
}

// ============================================================================
// OpenAI Summarizer Implementation
// ============================================================================

// openaiSummarizer implements Summarizer via the OpenAI chat completions API.
type openaiSummarizer struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

// chatCompletionRequest is the request body for POST /v1/chat/completions (summarization).
type chatCompletionRequest struct {
	Model       string                `json:"model"`
	Messages    []chatMessage         `json:"messages"`
	MaxTokens   int                   `json:"max_tokens,omitempty"`
	Temperature float64               `json:"temperature,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse is the response from POST /v1/chat/completions.
type chatCompletionResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
	Error   *chatError   `json:"error,omitempty"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type chatError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// NewOpenAISummarizer creates a Summarizer backed by OpenAI chat completions.
func NewOpenAISummarizer(baseURL, apiKey string) Summarizer {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &openaiSummarizer{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    baseURL,
		apiKey:     apiKey,
	}
}

// Summarize sends messages to the chat completions API and returns the response text.
func (s *openaiSummarizer) Summarize(ctx context.Context, systemPrompt, content string, modelID string) (string, error) {
	reqBody := chatCompletionRequest{
		Model: modelID,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: content},
		},
		MaxTokens:   1024,
		Temperature: 0.3,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("summarizer: marshal request: %w", err)
	}

	url := s.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("summarizer: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	slog.Debug("summarizer: calling LLM", "model", modelID, "content_len", len(content))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("summarizer: http request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("summarizer: read response: %w", err)
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("summarizer: parse response (status %d): %w", resp.StatusCode, err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("summarizer: api error (status %d): %s (type=%s, code=%s)",
			resp.StatusCode, chatResp.Error.Message, chatResp.Error.Type, chatResp.Error.Code)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("summarizer: http %d: %s", resp.StatusCode, string(respBytes))
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("summarizer: no choices in response")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// ============================================================================
// Compression Worker
// ============================================================================

// WorkerConfig holds configuration for the compression worker.
type WorkerConfig struct {
	// PollInterval is how often to poll the compression_queue.
	PollInterval time.Duration

	// BatchSize is the max number of events to process per poll cycle.
	BatchSize int

	// CosineThreshold is the minimum similarity for accepting a summary.
	// Default: 0.85 (from SPEC-002 §8.2)
	CosineThreshold float64

	// EmbeddingModel is the model to use for embedding generation.
	EmbeddingModel string

	// LLMBaseURL overrides the chat completions base URL.
	LLMBaseURL string

	// LLMAPIKey is the API key for LLM calls.
	LLMAPIKey string

	// EmbeddingBaseURL overrides the embeddings base URL.
	EmbeddingBaseURL string

	// EmbeddingAPIKey is the API key for embedding calls.
	EmbeddingAPIKey string
}

// DefaultWorkerConfig returns a WorkerConfig with sensible defaults.
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		PollInterval:    5 * time.Second,
		BatchSize:       5,
		CosineThreshold: DefaultCosineThreshold,
		EmbeddingModel:  llm.DefaultEmbeddingModel,
	}
}

// Worker is the background compression goroutine.
// It polls the compression_queue, processes events, and writes results.
type Worker struct {
	db             db.DB
	embedClient    llm.EmbeddingClient
	summarizer     Summarizer
	cfg            WorkerConfig

	// billingTracker is an optional callback for recording billing rows.
	// Signature: (sessionID, iteration, modelID, category, promptTokens, completionTokens, costUSD)
	billingTracker BillingFunc

	mu      sync.Mutex
	stopped chan struct{}
	running bool
}

// BillingFunc records a billing entry for compression or embedding operations.
type BillingFunc func(ctx context.Context, sessionID string, iteration int64, modelID string, category string, promptTokens, completionTokens int64, costUSD float64)

// NewWorker creates a new compression worker.
func NewWorker(database db.DB, embedClient llm.EmbeddingClient, summarizer Summarizer, cfg WorkerConfig) *Worker {
	return &Worker{
		db:             database,
		embedClient:    embedClient,
		summarizer:     summarizer,
		cfg:            cfg,
		stopped:        make(chan struct{}),
	}
}

// SetBillingTracker sets the optional billing callback.
func (w *Worker) SetBillingTracker(tracker BillingFunc) {
	w.billingTracker = tracker
}

// Start begins the compression worker loop in a background goroutine.
func (w *Worker) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	go w.runLoop(ctx)
	slog.Info("compression: worker started",
		"interval", w.cfg.PollInterval,
		"batch_size", w.cfg.BatchSize,
		"threshold", w.cfg.CosineThreshold,
	)
}

// Stop signals the worker to shut down and waits for it to finish.
func (w *Worker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.mu.Unlock()

	// Signal the loop to stop
	close(w.stopped)
}

// runLoop is the main worker loop, running in its own goroutine.
func (w *Worker) runLoop(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("compression: worker stopped via context")
			return
		case <-w.stopped:
			slog.Info("compression: worker stopped via signal")
			return
		case <-ticker.C:
			w.pollAndProcess(ctx)
		}
	}
}

// pollAndProcess polls the queue and processes pending events.
func (w *Worker) pollAndProcess(ctx context.Context) {
	items, err := w.fetchPending(ctx)
	if err != nil {
		slog.Error("compression: fetch pending failed", "error", err)
		return
	}

	if len(items) == 0 {
		return
	}

	slog.Info("compression: processing batch", "count", len(items))

	for _, item := range items {
		if err := w.processOne(ctx, &item); err != nil {
			slog.Error("compression: process event failed",
				"queue_id", item.ID,
				"event_id", item.EventID,
				"error", err,
			)
			// Mark as failed so we don't retry forever
			w.markFailed(ctx, item.ID, err.Error())
		}
	}
}

// fetchPending retrieves pending compression queue items.
func (w *Worker) fetchPending(ctx context.Context) ([]QueueItem, error) {
	// Use FOR UPDATE SKIP LOCKED on Postgres for concurrent safety
	lockClause := ""
	if w.db.Backend() == db.BackendPostgres {
		lockClause = " FOR UPDATE SKIP LOCKED"
	}

	query := fmt.Sprintf(`
		SELECT id, event_id, current_tier, next_tier, status, attempts, max_attempts, created_at
		FROM compression_queue
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1%s
	`, lockClause)

	rows, err := w.db.Query(ctx, query, w.cfg.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("compression: query pending: %w", err)
	}

	items := make([]QueueItem, 0, len(rows))
	for _, row := range rows {
		item := QueueItem{
			ID:          toInt64(row["id"]),
			EventID:     toInt64(row["event_id"]),
			Status:      toString(row["status"]),
			Attempts:    int(toInt64(row["attempts"])),
			MaxAttempts: int(toInt64(row["max_attempts"])),
		}
		// Handle nullable tier fields
		if v, ok := row["current_tier"]; ok && v != nil {
			item.CurrentTier = int(toInt64(v))
		}
		if v, ok := row["next_tier"]; ok && v != nil {
			item.NextTier = int(toInt64(v))
		}
		items = append(items, item)
	}

	return items, nil
}

// processOne handles a single compression queue item.
func (w *Worker) processOne(ctx context.Context, item *QueueItem) error {
	slog.Info("compression: processing event",
		"queue_id", item.ID,
		"event_id", item.EventID,
		"tier", item.CurrentTier,
	)

	// Mark as processing
	if err := w.updateStatus(ctx, item.ID, "processing"); err != nil {
		return fmt.Errorf("mark processing: %w", err)
	}

	// Step 1: Fetch original content from memory_events
	me, err := w.fetchMemoryEvent(ctx, item.EventID)
	if err != nil {
		return fmt.Errorf("fetch memory event: %w", err)
	}

	// Step 2: Determine target tier
	currentTier := TierFromInt(item.CurrentTier)
	targetTier := NextTier(currentTier)

	// Step 3: Select summarization model from model_registry
	// Try the current tier level model first, escalate if needed
	summaryModel, err := w.selectModelForTier(ctx, item.CurrentTier)
	if err != nil {
		// Fallback: use a sensible default
		summaryModel = "gpt-4o-mini"
		slog.Warn("compression: model_registry query failed, using default", "error", err)
	}

	// Step 4: If content has already been compressed (has summary_text), use the
	// existing summary_text as the source for further compression.
	sourceContent := me.Content
	// Check if the event already has a summary (re-compression)
	// In this case, the source for abstract/canonical tiers is the summary
	if item.CurrentTier > 1 && me.Content == "" {
		// Fall back to empty content — error out
		return fmt.Errorf("memory event %d has no content", item.EventID)
	}

	// Step 5: Generate embedding of the original content
	slog.Debug("compression: generating embedding of original", "event_id", item.EventID)
	originalEmbedding, err := w.embedClient.Embed(ctx, sourceContent)
	if err != nil {
		return fmt.Errorf("embed original: %w", err)
	}
	w.recordEmbeddingBilling(ctx, me.SessionID, len(sourceContent))

	// Step 6: Generate summary using the selected LLM model
	systemPrompt := CompressionSummaryPrompt(targetTier)
	slog.Debug("compression: generating summary",
		"event_id", item.EventID,
		"model", summaryModel,
		"tier", targetTier.String(),
	)
	summary, err := w.summarizer.Summarize(ctx, systemPrompt, sourceContent, summaryModel)
	if err != nil {
		return fmt.Errorf("summarize: %w", err)
	}
	w.recordSummarizationBilling(ctx, me.SessionID, summaryModel, len(sourceContent), len(summary))

	// Step 7: Generate embedding of the summary
	slog.Debug("compression: generating embedding of summary", "event_id", item.EventID)
	summaryEmbedding, err := w.embedClient.Embed(ctx, summary)
	if err != nil {
		return fmt.Errorf("embed summary: %w", err)
	}
	w.recordEmbeddingBilling(ctx, me.SessionID, len(summary))

	// Step 8: Compute cosine similarity
	threshold := w.cfg.CosineThreshold
	cosineScore, err := llm.CosineSimilarity(originalEmbedding, summaryEmbedding)
	if err != nil {
		return fmt.Errorf("cosine similarity: %w", err)
	}

	slog.Info("compression: validation result",
		"event_id", item.EventID,
		"cosine_score", cosineScore,
		"threshold", threshold,
		"tier", targetTier.String(),
	)

	// Step 9: Accept or reject based on cosine similarity
	if !ShouldEscalate(cosineScore, threshold) {
		// ACCEPTED — write summary back
		return w.acceptSummary(ctx, item, me.SessionID, summary, targetTier)
	}

	// REJECTED — escalate or fail
	return w.rejectSummary(ctx, item, targetTier, cosineScore)
}

// acceptSummary writes the accepted summary to memory_events and display_modes.
func (w *Worker) acceptSummary(ctx context.Context, item *QueueItem, sessionID, summary string, tier DisplayTier) error {
	// Update memory_events.summary_text
	// This requires the compression_worker role (GRANT UPDATE summary_text ON memory_events TO compression_worker)
	err := w.db.Exec(ctx,
		`UPDATE memory_events SET summary_text = $1 WHERE id = $2`,
		summary, item.EventID,
	)
	if err != nil {
		return fmt.Errorf("update summary_text: %w", err)
	}

	// Update display_modes to compressed
	err = w.db.Exec(ctx, `
		INSERT INTO display_modes (memory_id, mode, set_at, set_by_iteration, session_id)
		VALUES ($1, 'compressed', datetime('now'), 0, $2)
		ON CONFLICT (memory_id) DO UPDATE SET mode = 'compressed', set_at = datetime('now')
	`, item.EventID, sessionID)
	if err != nil {
		return fmt.Errorf("update display_modes: %w", err)
	}

	// Mark queue item as completed
	err = w.db.Exec(ctx, `
		UPDATE compression_queue
		SET status = 'completed', processed_at = datetime('now'), current_tier = $2
		WHERE id = $1
	`, item.ID, int(tier))
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}

	slog.Info("compression: accepted",
		"event_id", item.EventID,
		"queue_id", item.ID,
		"tier", tier.String(),
	)
	return nil
}

// rejectSummary handles a failed compression attempt — escalate or fail.
func (w *Worker) rejectSummary(ctx context.Context, item *QueueItem, currentTier DisplayTier, cosineScore float64) error {
	item.Attempts++
	nextTier := NextTier(currentTier)

	if item.Attempts >= item.MaxAttempts || currentTier >= MaxTier {
		// Exhausted all attempts or tiers — mark as failed
		err := w.db.Exec(ctx, `
			UPDATE compression_queue
			SET status = 'failed', attempts = $2, processed_at = datetime('now')
			WHERE id = $1
		`, item.ID, item.Attempts)
		if err != nil {
			return fmt.Errorf("mark failed: %w", err)
		}

		slog.Warn("compression: failed — max attempts/tiers exhausted",
			"event_id", item.EventID,
			"cosine_score", cosineScore,
			"attempts", item.Attempts,
			"tier", currentTier.String(),
		)
		return nil
	}

	// Escalate to next tier — update the queue item for retry
	err := w.db.Exec(ctx, `
		UPDATE compression_queue
		SET status = 'pending',
		    current_tier = $2,
		    next_tier = $3,
		    attempts = $4
		WHERE id = $1
	`, item.ID, int(nextTier), int(NextTier(nextTier)), item.Attempts)
	if err != nil {
		return fmt.Errorf("escalate tier: %w", err)
	}

	slog.Info("compression: escalating",
		"event_id", item.EventID,
		"cosine_score", cosineScore,
		"attempts", item.Attempts,
		"next_tier", nextTier.String(),
	)
	return nil
}

// markFailed sets a compression queue item to failed with an error message.
func (w *Worker) markFailed(ctx context.Context, queueID int64, errMsg string) {
	werr := w.db.Exec(ctx, `
		UPDATE compression_queue
		SET status = 'failed', processed_at = datetime('now')
		WHERE id = $1 AND status != 'completed'
	`, queueID)
	if werr != nil {
		slog.Error("compression: mark failed", "queue_id", queueID, "error", werr)
	}
}

// updateStatus updates the status of a queue item.
func (w *Worker) updateStatus(ctx context.Context, queueID int64, status string) error {
	return w.db.Exec(ctx,
		`UPDATE compression_queue SET status = $2 WHERE id = $1`,
		queueID, status,
	)
}

// fetchMemoryEvent retrieves content and session info from memory_events.
func (w *Worker) fetchMemoryEvent(ctx context.Context, eventID int64) (*MemoryEventContent, error) {
	row, err := w.db.QueryRow(ctx, `
		SELECT id, COALESCE(content, '') as content, COALESCE(session_id::TEXT, '') as session_id
		FROM memory_events
		WHERE id = $1
	`, eventID)
	if err != nil {
		return nil, fmt.Errorf("query memory_events: %w", err)
	}
	if row == nil {
		return nil, fmt.Errorf("memory event %d not found", eventID)
	}

	return &MemoryEventContent{
		ID:        toInt64(row["id"]),
		Content:   toString(row["content"]),
		SessionID: toString(row["session_id"]),
	}, nil
}

// selectModelForTier queries model_registry for the best model at the given tier.
func (w *Worker) selectModelForTier(ctx context.Context, tier int) (string, error) {
	// Use the tier's summarization model: query for the cheapest enabled model
	// at the event's current tier that fits our context needs
	query := `
		SELECT model_id
		FROM model_registry
		WHERE tier = $1 AND enabled = true
		ORDER BY cost_per_m_out ASC
		LIMIT 1
	`

	rows, err := w.db.Query(ctx, query, tier)
	if err != nil {
		return "", fmt.Errorf("query model_registry: %w", err)
	}

	if len(rows) == 0 {
		// Tier 0 is the embedding tier — use Tier 1 for summarization
		if tier <= 0 {
			rows, err = w.db.Query(ctx, `
				SELECT model_id
				FROM model_registry
				WHERE tier = 1 AND enabled = true
				ORDER BY cost_per_m_out ASC
				LIMIT 1
			`)
			if err != nil {
				return "", fmt.Errorf("query model_registry tier 1: %w", err)
			}
		}
		if len(rows) == 0 {
			return "", fmt.Errorf("no enabled models found for tier %d", tier)
		}
	}

	return toString(rows[0]["model_id"]), nil
}

// ============================================================================
// Billing Recording
// ============================================================================

// recordEmbeddingBilling records a billing entry for an embedding API call.
func (w *Worker) recordEmbeddingBilling(ctx context.Context, sessionID string, charCount int) {
	if w.billingTracker == nil || sessionID == "" {
		return
	}
	// Estimate tokens from characters (rough: ~4 chars per token)
	estimatedTokens := int64(charCount / 4)
	if estimatedTokens < 1 {
		estimatedTokens = 1
	}
	// Embedding cost is tiny; use a flat rate
	costUSD := float64(estimatedTokens) * 0.0000001 // ~$0.10/1M tokens
	w.billingTracker(ctx, sessionID, 0, w.cfg.EmbeddingModel, "embedding",
		estimatedTokens, 0, costUSD)
}

// recordSummarizationBilling records a billing entry for a summarization LLM call.
func (w *Worker) recordSummarizationBilling(ctx context.Context, sessionID, modelID string, inputChars, outputChars int) {
	if w.billingTracker == nil || sessionID == "" {
		return
	}
	promptTokens := int64(inputChars / 4)
	completionTokens := int64(outputChars / 4)
	if promptTokens < 1 {
		promptTokens = 1
	}
	if completionTokens < 1 {
		completionTokens = 1
	}
	// Rough cost estimate; real pricing comes from model_registry
	costUSD := float64(promptTokens)*0.000002 + float64(completionTokens)*0.000008 // ~$2/$8 per M
	w.billingTracker(ctx, sessionID, 0, modelID, "compression",
		promptTokens, completionTokens, costUSD)
}

// ============================================================================
// Helpers
// ============================================================================

func toInt64(v any) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case uint64:
		return int64(val)
	case []byte:
		var i int64
		fmt.Sscanf(string(val), "%d", &i)
		return i
	default:
		return 0
	}
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}
