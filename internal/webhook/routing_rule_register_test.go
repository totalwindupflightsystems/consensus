package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/consensus/internal/api"
	"github.com/wojons/consensus/internal/db"
	"github.com/wojons/consensus/internal/db/driver"
	"github.com/wojons/consensus/internal/migrate"
)

// DF-CONSENSUS-67: a routing rule registered through REST must affect the next
// real event processed by the existing webhook router. No routing_rules row is
// seeded by the test.
func TestRoutingRuleRegistrationRoutesEventEndToEnd(t *testing.T) {
	ctx := context.Background()
	conn, err := driver.Open(ctx, db.Config{URL: "sqlite://" + filepath.Join(t.TempDir(), "df67-routing.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := migrate.New(conn).AutoMigrate(ctx); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	const adminKey = "cs_sk_admin_df67_routing_test_key"
	hash := sha256.Sum256([]byte(adminKey))
	if err := conn.Exec(ctx, `INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at)
		VALUES ('key-df67-routing', $1, 'cs_sk_ad', 'admin', datetime('now'))`, hex.EncodeToString(hash[:])); err != nil {
		t.Fatalf("create test admin credential: %v", err)
	}
	ts := httptest.NewServer(api.NewServer(api.ServerConfig{DB: conn}).Handler())
	defer ts.Close()

	post := func(path, body string) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, ts.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminKey)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		var decoded map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&decoded)
		return resp.StatusCode, decoded
	}

	status, session := post("/api/v1/sessions", `{"agent_name":"df67-router","goal":"receive routed events"}`)
	if status != http.StatusCreated {
		t.Fatalf("create session = %d %v", status, session)
	}
	sessionID, _ := session["id"].(string)
	if sessionID == "" {
		t.Fatalf("session response has no id: %v", session)
	}

	status, rule := post("/api/v1/routing-rules", fmt.Sprintf(`{"name":"df67-route","source_pattern":"df67","event_type_pattern":"push","target_session_id":%q,"priority":10}`, sessionID))
	if status != http.StatusCreated {
		t.Fatalf("register routing rule = %d %v, want 201", status, rule)
	}

	store := New(conn)
	result, _, err := store.IngestEvent(ctx, Event{
		Source: "df67", SourceID: "delivery-df67", EventType: "webhook.push",
		Payload: []byte(`{"path":"repos/df67"}`), SignatureValid: true,
	}, nil)
	if err != nil || result != IngestOK {
		t.Fatalf("ingest event = %s, %v", result, err)
	}
	if err := store.routePendingEvents(ctx); err != nil {
		t.Fatalf("process pending events: %v", err)
	}

	rows, err := conn.Query(ctx, `SELECT session_id, status FROM external_events WHERE source_id = 'delivery-df67'`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("read processed event: rows=%d err=%v", len(rows), err)
	}
	if got := fmt.Sprint(rows[0]["session_id"]); got != sessionID {
		t.Fatalf("event session_id = %q, want routing-rule target %q", got, sessionID)
	}
	if got := fmt.Sprint(rows[0]["status"]); got != "routed" {
		t.Fatalf("event status = %q, want routed", got)
	}
}
