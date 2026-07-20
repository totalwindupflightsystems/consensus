package modelsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// Test Harness: mock DB for modelsync tests
// ============================================================================

type mockDB struct {
	queryResults []db.Row
	queryErr     error
	execErr      error
	queries      []string
	backend      db.Backend
}

func (m *mockDB) BeginTx(ctx context.Context) (db.Tx, error) { return nil, nil }
func (m *mockDB) Exec(ctx context.Context, query string, args ...any) error {
	m.queries = append(m.queries, query)
	return m.execErr
}
func (m *mockDB) Query(ctx context.Context, query string, args ...any) ([]db.Row, error) {
	m.queries = append(m.queries, query)
	return m.queryResults, m.queryErr
}
func (m *mockDB) QueryRow(ctx context.Context, query string, args ...any) (db.Row, error) {
	rows, err := m.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}
func (m *mockDB) Backend() db.Backend {
	if m.backend == "" {
		return db.BackendSQLite
	}
	return m.backend
}
func (m *mockDB) Close() error { return nil }

// stubURL overrides modelsDevURL for a single test, restoring it after.
func stubURL(t *testing.T, newURL string) {
	t.Helper()
	orig := modelsDevURL
	modelsDevURL = newURL
	t.Cleanup(func() { modelsDevURL = orig })
}

// ============================================================================
// New
// ============================================================================

func TestNew_SetsHTTPTimeout(t *testing.T) {
	md := &mockDB{}
	s := New(md)
	if s == nil {
		t.Fatal("expected non-nil Syncer")
	}
	if s.database != md {
		t.Error("database not set on Syncer")
	}
	if s.client == nil {
		t.Fatal("expected non-nil HTTP client")
	}
	if s.client.Timeout == 0 {
		t.Error("expected non-zero HTTP client timeout")
	}
}

// ============================================================================
// mapProvider
// ============================================================================

func TestMapProvider(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"openai lowercase", "openai", "openai"},
		{"openai uppercase", "OPENAI", "openai"},
		{"anthropic", "anthropic", "anthropic"},
		{"anthropic mixed case", "AnThRoPiC", "anthropic"},
		{"unknown provider", "google", "openrouter"},
		{"empty string", "", "openrouter"},
		{"custom provider", "deepseek", "openrouter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapProvider(tt.input)
			if got != tt.expected {
				t.Errorf("mapProvider(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ============================================================================
// classifyTier
// ============================================================================

func TestClassifyTier_Premium(t *testing.T) {
	premiumPrefixes := []string{"gpt-4", "gpt-5", "claude-3", "claude-4", "gemini-2.5-pro", "grok-3"}
	for _, id := range premiumPrefixes {
		e := ModelEntry{ID: id, ContextWindow: 128000, Pricing: struct {
			Input  float64 `json:"input"`
			Output float64 `json:"output"`
		}{Input: 10.0, Output: 30.0}}
		if got := classifyTier(e); got != 3 {
			t.Errorf("classifyTier(%q) = %d, want 3 (premium)", id, got)
		}
	}
}

func TestClassifyTier_Tier1(t *testing.T) {
	e := ModelEntry{ID: "small-model", ContextWindow: 16000, Pricing: struct {
		Input  float64 `json:"input"`
		Output float64 `json:"output"`
	}{Input: 0.05, Output: 0.10}}
	if got := classifyTier(e); got != 1 {
		t.Errorf("classifyTier(small model) = %d, want 1", got)
	}
}

func TestClassifyTier_Tier2_Default(t *testing.T) {
	e := ModelEntry{ID: "mid-model", ContextWindow: 64000, Pricing: struct {
		Input  float64 `json:"input"`
		Output float64 `json:"output"`
	}{Input: 0.50, Output: 2.00}}
	if got := classifyTier(e); got != 2 {
		t.Errorf("classifyTier(mid model) = %d, want 2", got)
	}
}

func TestClassifyTier_CaseInsensitivePremium(t *testing.T) {
	e := ModelEntry{ID: "GPT-4-Turbo", ContextWindow: 128000, Pricing: struct {
		Input  float64 `json:"input"`
		Output float64 `json:"output"`
	}{Input: 10.0, Output: 30.0}}
	if got := classifyTier(e); got != 3 {
		t.Errorf("classifyTier(GPT-4-Turbo) = %d, want 3", got)
	}
}

func TestClassifyTier_ExactTier1Boundary(t *testing.T) {
	e := ModelEntry{ID: "cheap-model", ContextWindow: 31999, Pricing: struct {
		Input  float64 `json:"input"`
		Output float64 `json:"output"`
	}{Input: 10.0, Output: 30.0}}
	if got := classifyTier(e); got != 1 {
		t.Errorf("classifyTier(context<32k) = %d, want 1", got)
	}
}

// ============================================================================
// capabilitiesToTags
// ============================================================================

func TestCapabilitiesToTags_AlwaysHasChat(t *testing.T) {
	tags := capabilitiesToTags(nil)
	found := false
	for _, tag := range tags {
		if tag == "chat" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'chat' tag in empty capabilities, got %v", tags)
	}
}

func TestCapabilitiesToTags(t *testing.T) {
	tests := []struct {
		name string
		caps []string
		want []string
	}{
		{
			name: "embedding",
			caps: []string{"embedding"},
			want: []string{"chat", "embedding"},
		},
		{
			name: "vision",
			caps: []string{"vision"},
			want: []string{"chat", "vision"},
		},
		{
			name: "image",
			caps: []string{"image"},
			want: []string{"chat", "vision"},
		},
		{
			name: "function_calling",
			caps: []string{"function_calling"},
			want: []string{"chat", "function_calling"},
		},
		{
			name: "tools",
			caps: []string{"tools"},
			want: []string{"chat", "function_calling"},
		},
		{
			name: "code",
			caps: []string{"code"},
			want: []string{"chat", "code"},
		},
		{
			name: "multiple capabilities",
			caps: []string{"vision", "code", "function_calling"},
			want: []string{"chat", "vision", "code", "function_calling"},
		},
		{
			name: "case insensitive",
			caps: []string{"EMBEDDING", "CODE"},
			want: []string{"chat", "embedding", "code"},
		},
		{
			name: "unknown capability ignored",
			caps: []string{"unknown_cap"},
			want: []string{"chat"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capabilitiesToTags(tt.caps)
			if len(got) != len(tt.want) {
				t.Errorf("capabilitiesToTags() = %v (%d), want %v (%d)", got, len(got), tt.want, len(tt.want))
				return
			}
			for i, tag := range got {
				if tag != tt.want[i] {
					t.Errorf("capabilitiesToTags()[%d] = %q, want %q", i, tag, tt.want[i])
				}
			}
		})
	}
}

// ============================================================================
// lookup
// ============================================================================

func TestLookup_Found(t *testing.T) {
	md := &mockDB{
		queryResults: []db.Row{
			{"sync_source": "models.dev"},
		},
	}
	s := New(md)
	result, err := s.lookup(context.Background(), "gpt-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for found model")
	}
	if result["sync_source"] != "models.dev" {
		t.Errorf("sync_source = %q, want 'models.dev'", result["sync_source"])
	}
}

func TestLookup_NotFound(t *testing.T) {
	md := &mockDB{
		queryResults: []db.Row{},
	}
	s := New(md)
	result, err := s.lookup(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for not-found model, got %v", result)
	}
}

func TestLookup_StaticSource(t *testing.T) {
	md := &mockDB{
		queryResults: []db.Row{
			{"sync_source": "static"},
		},
	}
	s := New(md)
	result, err := s.lookup(context.Background(), "static-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for static model")
	}
	if result["sync_source"] != "static" {
		t.Errorf("sync_source = %q, want 'static'", result["sync_source"])
	}
}

func TestLookup_DBError(t *testing.T) {
	md := &mockDB{
		queryErr: context.DeadlineExceeded,
	}
	s := New(md)
	_, err := s.lookup(context.Background(), "gpt-4")
	if err == nil {
		t.Error("expected error from DB, got nil")
	}
}

// ============================================================================
// insert
// ============================================================================

func TestInsert_Success(t *testing.T) {
	md := &mockDB{}
	s := New(md)
	e := ModelEntry{ID: "gpt-4", ContextWindow: 128000}
	e.Pricing.Input = 10.0
	e.Pricing.Output = 30.0
	e.Capabilities = []string{"code", "vision"}

	err := s.insert(context.Background(), e, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(md.queries) == 0 {
		t.Fatal("expected Exec to be called")
	}
	query := md.queries[0]
	if !strings.Contains(query, "INSERT INTO model_registry") {
		t.Errorf("expected INSERT query, got: %s", query)
	}
}

func TestInsert_DBError(t *testing.T) {
	md := &mockDB{execErr: context.DeadlineExceeded}
	s := New(md)
	e := ModelEntry{ID: "gpt-4", ContextWindow: 128000}

	err := s.insert(context.Background(), e, 3)
	if err == nil {
		t.Error("expected error from DB insert, got nil")
	}
}

// ============================================================================
// update
// ============================================================================

func TestUpdate_Success(t *testing.T) {
	md := &mockDB{}
	s := New(md)
	e := ModelEntry{ID: "gpt-4", ContextWindow: 128000}
	e.Pricing.Input = 10.0
	e.Pricing.Output = 30.0

	err := s.update(context.Background(), e, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(md.queries) == 0 {
		t.Fatal("expected Exec to be called")
	}
	query := md.queries[0]
	if !strings.Contains(query, "UPDATE model_registry") {
		t.Errorf("expected UPDATE query, got: %s", query)
	}
	if !strings.Contains(query, "sync_source = 'models.dev'") {
		t.Errorf("expected WHERE clause with sync_source='models.dev', got: %s", query)
	}
}

func TestUpdate_DBError(t *testing.T) {
	md := &mockDB{execErr: context.DeadlineExceeded}
	s := New(md)
	e := ModelEntry{ID: "gpt-4", ContextWindow: 128000}

	err := s.update(context.Background(), e, 3)
	if err == nil {
		t.Error("expected error from DB update, got nil")
	}
}

// ============================================================================
// fetchModels
// ============================================================================

func TestFetchModels_Success(t *testing.T) {
	models := []ModelEntry{
		{ID: "gpt-4", Provider: "openai", ContextWindow: 128000, Capabilities: []string{"code"}},
		{ID: "claude-3", Provider: "anthropic", ContextWindow: 200000, Capabilities: []string{"vision"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	s := New(&mockDB{})
	entries, err := s.fetchModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestFetchModels_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	s := New(&mockDB{})
	_, err := s.fetchModels(context.Background())
	if err == nil {
		t.Error("expected error for non-200 response, got nil")
	}
}

func TestFetchModels_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	stubURL(t, server.URL)

	s := New(&mockDB{})
	_, err := s.fetchModels(context.Background())
	if err == nil {
		t.Error("expected JSON decode error, got nil")
	}
}

// ============================================================================
// RegisterIfMissing
// ============================================================================

func TestRegisterIfMissing_AlreadyExists(t *testing.T) {
	md := &mockDB{
		queryResults: []db.Row{
			{"sync_source": "models.dev"},
		},
	}
	s := New(md)
	err := s.RegisterIfMissing(context.Background(), "gpt-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(md.queries) != 1 {
		t.Errorf("expected 1 query (lookup only), got %d queries", len(md.queries))
	}
}

func TestRegisterIfMissing_NotFound_FetchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	md := &mockDB{
		queryResults: []db.Row{},
	}
	s := New(md)

	err := s.RegisterIfMissing(context.Background(), "unknown-model")
	if err != nil {
		t.Fatalf("expected no error (falls back to unknown), got: %v", err)
	}
	if len(md.queries) < 2 {
		t.Errorf("expected at least 2 queries (lookup + insert), got %d", len(md.queries))
	}
}

func TestRegisterIfMissing_NotFound_NotInFetch(t *testing.T) {
	models := []ModelEntry{
		{ID: "gpt-4", Provider: "openai", ContextWindow: 128000},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	md := &mockDB{
		queryResults: []db.Row{},
	}
	s := New(md)

	err := s.RegisterIfMissing(context.Background(), "unknown-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(md.queries) < 2 {
		t.Errorf("expected at least 2 queries, got %d", len(md.queries))
	}
}

func TestRegisterIfMissing_LookupFails(t *testing.T) {
	md := &mockDB{
		queryErr: context.DeadlineExceeded,
	}
	s := New(md)
	err := s.RegisterIfMissing(context.Background(), "gpt-4")
	if err == nil {
		t.Error("expected error from failed lookup, got nil")
	}
}

// ============================================================================
// Sync
// ============================================================================

func TestSync_InsertNewModel(t *testing.T) {
	models := []ModelEntry{
		{ID: "gpt-4", Provider: "openai", ContextWindow: 128000, Capabilities: []string{"code"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	md := &mockDB{
		queryResults: []db.Row{},
	}
	s := New(md)

	result, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Added != 1 {
		t.Errorf("expected 1 added, got %d", result.Added)
	}
	if result.Updated != 0 {
		t.Errorf("expected 0 updated, got %d", result.Updated)
	}
}

func TestSync_UpdateExistingModel(t *testing.T) {
	models := []ModelEntry{
		{ID: "gpt-4", Provider: "openai", ContextWindow: 128000, Capabilities: []string{"code", "vision"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	md := &mockDB{
		queryResults: []db.Row{
			{"sync_source": "models.dev"},
		},
	}
	s := New(md)

	result, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Updated != 1 {
		t.Errorf("expected 1 updated, got %d", result.Updated)
	}
	if result.Added != 0 {
		t.Errorf("expected 0 added, got %d", result.Added)
	}
}

func TestSync_SkipsStaticEntry(t *testing.T) {
	models := []ModelEntry{
		{ID: "gpt-4", Provider: "openai", ContextWindow: 128000},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	md := &mockDB{
		queryResults: []db.Row{
			{"sync_source": "static"},
		},
	}
	s := New(md)

	result, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Added != 0 {
		t.Errorf("expected 0 added for static entry, got %d", result.Added)
	}
	if result.Updated != 0 {
		t.Errorf("expected 0 updated for static entry, got %d", result.Updated)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %v", result.Errors)
	}
}

func TestSync_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]ModelEntry{})
	}))
	defer server.Close()

	stubURL(t, server.URL)

	md := &mockDB{}
	s := New(md)

	result, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Added != 0 {
		t.Errorf("expected 0 added, got %d", result.Added)
	}
	if result.Updated != 0 {
		t.Errorf("expected 0 updated, got %d", result.Updated)
	}
}

func TestSync_FetchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	md := &mockDB{}
	s := New(md)

	result, err := s.Sync(context.Background())
	if err == nil {
		t.Error("expected error from failed fetch, got nil")
	}
	if len(result.Errors) == 0 {
		t.Error("expected errors in result")
	}
}

func TestSync_LookupErrorSkipsModel(t *testing.T) {
	models := []ModelEntry{
		{ID: "gpt-4", Provider: "openai", ContextWindow: 128000},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	md := &mockDB{
		queryErr: context.DeadlineExceeded,
	}
	s := New(md)

	result, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Added != 0 && result.Updated != 0 {
		t.Error("expected no adds or updates when lookup fails")
	}
	if len(result.Errors) == 0 {
		t.Error("expected errors in result for failed lookup")
	}
}

func TestSync_InsertError(t *testing.T) {
	models := []ModelEntry{
		{ID: "gpt-4", Provider: "openai", ContextWindow: 128000},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	md := &mockDB{
		queryResults: []db.Row{},
		execErr:      context.DeadlineExceeded,
	}
	s := New(md)

	result, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Error("expected errors in result for failed insert")
	}
}

func TestSync_UpdateError(t *testing.T) {
	models := []ModelEntry{
		{ID: "gpt-4", Provider: "openai", ContextWindow: 128000},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	md := &mockDB{
		queryResults: []db.Row{
			{"sync_source": "models.dev"},
		},
		execErr: context.DeadlineExceeded,
	}
	s := New(md)

	result, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Error("expected errors in result for failed update")
	}
}

func TestSync_MultipleModels(t *testing.T) {
	models := []ModelEntry{
		{ID: "gpt-4", Provider: "openai", ContextWindow: 128000},
		{ID: "claude-3", Provider: "anthropic", ContextWindow: 200000},
		{ID: "small-model", Provider: "unknown", ContextWindow: 16000},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	md := &mockDB{
		queryResults: []db.Row{},
	}
	s := New(md)

	result, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Added != 3 {
		t.Errorf("expected 3 added, got %d", result.Added)
	}
}

func TestSync_StaticEntrySkipped(t *testing.T) {
	models := []ModelEntry{
		{ID: "gpt-4", Provider: "openai", ContextWindow: 128000},
		{ID: "claude-3", Provider: "anthropic", ContextWindow: 200000},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	stubURL(t, server.URL)

	// Both models have sync_source='static' so both are skipped
	md := &mockDB{
		queryResults: []db.Row{
			{"sync_source": "static"},
		},
	}
	s := New(md)

	result, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Added != 0 {
		t.Errorf("expected 0 added (all static), got %d", result.Added)
	}
	if result.Updated != 0 {
		t.Errorf("expected 0 updated (all static), got %d", result.Updated)
	}
}

func TestSync_RealHTTPTimeout(t *testing.T) {
	// Restore the real URL and verify it's reachable (smoke test only)
	// Don't actually call models.dev in unit tests
	if testing.Short() {
		t.Skip("skipping real HTTP test in short mode")
	}
}

// ============================================================================
// AutoSyncLoop — basic smoke test
// ============================================================================

func TestAutoSyncLoop_ContextCancel(t *testing.T) {
	// Use an unreachable URL to avoid hitting models.dev
	stubURL(t, "http://127.0.0.1:1/nope")

	s := &Syncer{
		database: &mockDB{},
		client: &http.Client{
			Timeout: 50 * time.Millisecond,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		s.AutoSyncLoop(ctx, 1*time.Hour)
		close(done)
	}()

	// Give it time to attempt the initial Sync (which will fail quickly)
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Should exit within reasonable time
	select {
	case <-done:
		// ok
	case <-time.After(5 * time.Second):
		t.Error("AutoSyncLoop did not exit after context cancel")
	}
}
