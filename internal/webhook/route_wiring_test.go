package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/wojons/consensus/internal/db"
)

// newWiredServer builds the production webhook wiring seam the way
// cmd/consensus/main.go does: a chi router with the store mounted on the
// source-carrying wildcard route, so HandleWebhook receives
// /webhooks/{source} (SPEC-013 §4, WEBHOOK-1).
func newWiredServer(t *testing.T) (http.Handler, db.DB, *Store) {
	t.Helper()
	database, cleanup := setupTestDB(t)
	t.Cleanup(cleanup)
	store := New(database)
	mux := chi.NewRouter()
	mux.Handle("/webhooks/*", store)
	return mux, database, store
}

// signedRequest builds a POST with a valid HMAC-SHA256 signature over the
// exact payload bytes, the way a real sender does (docs/API.md webhooks).
func signedRequest(t *testing.T, path, secret, payload string) *http.Request {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("X-Event-Type", "ping")
	req.Header.Set("X-Delivery-ID", "delivery-wired-1")
	return req
}

// TestWebhookSourceRouteServesOnChiRouter proves the signed-delivery flow end
// to end at the real wiring seam: a registered source POSTed to
// /webhooks/{source} through the chi mount is ingested (202, event persisted
// with a valid signature), an empty source is still the handler's 400, an
// unknown source is the handler's 404 (not the router's "page not found"),
// and the bare /webhooks path remains unrouted.
func TestWebhookSourceRouteServesOnChiRouter(t *testing.T) {
	ctx := context.Background()
	handler, database, store := newWiredServer(t)

	if _, err := store.CreateRegistration(ctx, Registration{
		Name:    "docs-demo",
		Source:  "docs-demo",
		URLPath: "/webhooks/docs-demo",
		Secret:  "whsec_wiring_test",
	}); err != nil {
		t.Fatalf("create registration: %v", err)
	}

	// Source-bearing path: must NOT 404 at the router; the handler ingests.
	payload := `{"event":"ping","message":"wiring"}`
	req := signedRequest(t, "/webhooks/docs-demo", "whsec_wiring_test", payload)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("POST /webhooks/docs-demo = %d (%s), want 202 Accepted", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "accepted") {
		t.Errorf("response body %q does not report acceptance", w.Body.String())
	}

	// The event was persisted with the verified signature.
	rows, err := database.Query(ctx, `SELECT source, source_id, event_type, signature_valid, status FROM external_events`)
	if err != nil {
		t.Fatalf("query external_events: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("external_events has %d rows, want 1", len(rows))
	}
	if got := rows[0]["signature_valid"]; got != int64(1) && got != "1" && got != true {
		t.Errorf("persisted event signature_valid = %v, want verified", got)
	}
	if got := rows[0]["status"]; got != "pending" {
		t.Errorf("persisted event status = %v, want pending", got)
	}

	// Empty source: chi's wildcard also matches "/webhooks/" itself; the
	// handler must keep rejecting it with 400 INVALID_REQUEST.
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/webhooks/", strings.NewReader(payload)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /webhooks/ = %d, want 400 for empty source", w.Code)
	}
	if !strings.Contains(w.Body.String(), "INVALID_REQUEST") {
		t.Errorf("empty-source body %q missing INVALID_REQUEST", w.Body.String())
	}

	// Unknown source: handler-level 404 with the NOT_FOUND error shape —
	// reachable route, not chi's "page not found".
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, signedRequest(t, "/webhooks/unknown-source", "whsec_wiring_test", payload))
	if w.Code != http.StatusNotFound {
		t.Errorf("POST /webhooks/unknown-source = %d, want handler 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), "NOT_FOUND") {
		t.Errorf("unknown-source body %q missing NOT_FOUND (router fell through?)", w.Body.String())
	}

	// Bare /webhooks (no trailing slash): the wildcard does not match it and
	// the spec does not declare it — router 404 as before.
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, signedRequest(t, "/webhooks", "whsec_wiring_test", payload))
	if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "NOT_FOUND") {
		t.Errorf("POST /webhooks = %d (%s), want router-level 404", w.Code, w.Body.String())
	}
}

// TestProductionWiringMountsWebhookWildcard pins the fixture above (and the
// internal/api full-deploy fixture) to the production tree the same way
// TestProductionWiringMountsBareMCPAlongsideWildcard does for MCP: main.go
// must serve the webhook store on the source-carrying wildcard route, and the
// old exact /webhooks/ mount (which starved HandleWebhook of its source and
// 404'd every source-bearing delivery) must be gone.
func TestProductionWiringMountsWebhookWildcard(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve route_wiring_test.go location")
	}
	mainPath := filepath.Join(filepath.Dir(testFile), "..", "..", "cmd", "consensus", "main.go")
	source, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read production wiring %s: %v", mainPath, err)
	}
	src := string(source)

	const wildcardMount = `apiMux.Handle("/webhooks/*", whStore)`
	if !strings.Contains(src, wildcardMount) {
		t.Fatal("production wiring must mount the webhook store on the source-carrying /webhooks/* route so HandleWebhook receives /webhooks/{source}")
	}
	const exactMount = `apiMux.Handle("/webhooks/", whStore)`
	if strings.Contains(src, exactMount) {
		t.Fatal("production wiring still mounts the exact /webhooks/ route, which does not match /webhooks/{source} (WEBHOOK-1)")
	}
}
