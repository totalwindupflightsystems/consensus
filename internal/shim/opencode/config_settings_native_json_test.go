package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/wojons/consensus/internal/db"
)

// DF-CONSENSUS-54 (P2) — GET /config returned settings values as the raw TEXT
// of their system_settings row. The mcp.<name> rows PATCH /config writes are
// JSON documents, so a client received JSON *text* it had to re-parse, and a
// row written before DF-CONSENSUS-53 flattened MCP blocks received the raw Go
// %v rendering ("map[websearch:map[type:remote url:...]]") — a programmatic
// client could not round-trip its own configuration.
//
// The contract pinned here:
//
//	1. NATIVE JSON — a row whose text is a JSON object/array is emitted as that
//	   JSON value, so settings["mcp.websearch"].url is readable without a second
//	   parse.
//	2. DOCUMENTED ACCESS — the reconstructed mcp block is exposed alongside the
//	   flat rows, so the documented settings.mcp.<name>.url path resolves.
//	3. NOTHING INVENTED — a scalar setting stays a string ("5" is not 5) and
//	   non-JSON text (the legacy %v rows) is returned unchanged, never guessed
//	   into structure.
//	4. FIXED POINT — PATCH -> GET -> PATCH is stable, and the derived block is
//	   DERIVED per response: the store keeps exactly the flat rows the
//	   /mcp/{name}/* resolver reads (re-PATCHing what GET returned must not
//	   write a literal mcp row back — the DF-CONSENSUS-53 regression).

var df54WebsearchConfig = map[string]any{
	"type": "remote",
	"url":  "https://mcp.example.com",
}

// df54GetSettings issues GET /config and returns the decoded settings object.
func df54GetSettings(t *testing.T, baseURL string) map[string]any {
	t.Helper()

	status, _, body := doShimRequest(t, baseURL, http.MethodGet, "/config")
	if status != http.StatusOK {
		t.Fatalf("GET /config: got %d, want 200. Body: %s", status, body)
	}
	var got struct {
		Settings        map[string]any    `json:"settings"`
		ProviderDefault map[string]string `json:"provider_default"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode GET /config: %v (%s)", err, body)
	}
	if got.Settings == nil {
		t.Fatalf("GET /config settings = null, want an object. Body: %s", body)
	}
	return got.Settings
}

// df54SettingRows reads the settings store the way the /mcp/{name}/* resolver
// does, so a test can prove the derived block is not persisted.
func df54SettingRows(t *testing.T, conn db.DB) []db.Row {
	t.Helper()

	rows, err := conn.Query(context.Background(), `SELECT key, value FROM system_settings ORDER BY key`)
	if err != nil {
		t.Fatalf("read system_settings rows: %v", err)
	}
	return rows
}

func df54InsertSetting(t *testing.T, conn db.DB, key, value string) {
	t.Helper()

	if err := conn.Exec(context.Background(),
		`INSERT INTO system_settings (key, value) VALUES ($1, $2)
		 ON CONFLICT (key) DO UPDATE SET value = $2`, key, value); err != nil {
		t.Fatalf("insert system_settings %q: %v", key, err)
	}
}

// TestGetConfigSettingsAreNativeJSON is the headline DF-CONSENSUS-54 contract:
// a JSON-valued settings row reaches the client as native JSON (not JSON text,
// not Go %v text), the documented settings.mcp.<name>.url path resolves, and
// scalar settings are untouched.
func TestGetConfigSettingsAreNativeJSON(t *testing.T) {
	srv, conn := newMCPConfigTestServer(t)

	patchConfig(t, srv.URL, "/config", map[string]any{
		"mcp":                       map[string]any{"websearch": df54WebsearchConfig},
		"llm.default_model":         "gpt-4o",
		"harness.heartbeat_seconds": "5",
	})

	settings := df54GetSettings(t, srv.URL)

	// 1. The flat row is native JSON, not the text of that JSON.
	if text, isText := settings["mcp.websearch"].(string); isText {
		t.Fatalf("settings[mcp.websearch] = %#v (a string), want a native JSON object", text)
	}
	flat, ok := settings["mcp.websearch"].(map[string]any)
	if !ok {
		t.Fatalf("settings[mcp.websearch] = %#v, want a native JSON object", settings["mcp.websearch"])
	}
	if !reflect.DeepEqual(flat, df54WebsearchConfig) {
		t.Errorf("settings[mcp.websearch] = %#v, want %#v", flat, df54WebsearchConfig)
	}

	// 2. jq-style nested access: settings.mcp.websearch.url.
	mcp, ok := settings["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("settings.mcp = %#v, want the documented mcp block as a JSON object", settings["mcp"])
	}
	server, ok := mcp["websearch"].(map[string]any)
	if !ok {
		t.Fatalf("settings.mcp.websearch = %#v, want a JSON object", mcp["websearch"])
	}
	if got := server["url"]; got != "https://mcp.example.com" {
		t.Errorf("settings.mcp.websearch.url = %#v, want the configured URL as a string", got)
	}
	if got := server["type"]; got != "remote" {
		t.Errorf("settings.mcp.websearch.type = %#v, want %q", got, "remote")
	}

	// 3. Scalar settings keep their text: a numeric-looking value is not a JSON
	// number and a model id is not a JSON string literal.
	if got := settings["harness.heartbeat_seconds"]; got != "5" {
		t.Errorf("settings[harness.heartbeat_seconds] = %#v, want the string \"5\"", got)
	}
	if got := settings["llm.default_model"]; got != "gpt-4o" {
		t.Errorf("settings[llm.default_model] = %#v, want the string \"gpt-4o\"", got)
	}

	// 4. The derived block is never persisted and the store format is unchanged:
	// the settings rows are the ones PATCH upserted (no derived mcp row), and
	// the server row still holds JSON text.
	rows := df54SettingRows(t, conn)
	if len(rows) != 3 {
		t.Fatalf("system_settings rows = %v, want the three rows PATCH /config upserted", rows)
	}
	foundServerRow := false
	for _, row := range rows {
		key := toString(row["key"])
		if key == "mcp" {
			t.Errorf("system_settings holds a literal mcp row (%v) — the block is derived per response, never persisted", rows)
		}
		if key != "mcp.websearch" {
			continue
		}
		foundServerRow = true
		assertMCPConfigSetting(t, toString(row["value"]), df54WebsearchConfig)
	}
	if !foundServerRow {
		t.Errorf("system_settings rows = %v, want the flattened mcp.websearch server row", rows)
	}
}

// TestConfigRoundTripPatchGetPatchEquality is acceptance criterion 2's
// round-trip: what GET /config returns can be PATCHed straight back and is a
// fixed point, and doing so does not resurrect the literal mcp row that
// DF-CONSENSUS-53 removed (the derived block must stay derived).
func TestConfigRoundTripPatchGetPatchEquality(t *testing.T) {
	srv, conn := newMCPConfigTestServer(t)

	patchConfig(t, srv.URL, "/config", map[string]any{
		"mcp": map[string]any{"websearch": df54WebsearchConfig},
	})
	first := df54GetSettings(t, srv.URL)

	// The round trip is defined over NATIVE JSON: the client reads
	// settings.mcp.<name>.url as a structure before it PATCHes anything back, so
	// a response that only carried JSON text could not pass this as a stable
	// fixed point.
	assertMCPConfigObject(t, first["mcp.websearch"], df54WebsearchConfig)
	mcp, ok := first["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("first settings.mcp = %#v, want the documented mcp block as a JSON object", first["mcp"])
	}
	server, ok := mcp["websearch"].(map[string]any)
	if !ok {
		t.Fatalf("first settings.mcp.websearch = %#v, want a JSON object", mcp["websearch"])
	}
	if got := server["url"]; got != "https://mcp.example.com" {
		t.Errorf("first settings.mcp.websearch.url = %#v, want the configured URL", got)
	}

	// Re-PATCH exactly what GET returned.
	patchConfig(t, srv.URL, "/config", first)
	second := df54GetSettings(t, srv.URL)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("PATCH -> GET -> PATCH is not a fixed point:\n first: %#v\nsecond: %#v", first, second)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first settings: %v", err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second settings: %v", err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Errorf("round trip changed the wire shape:\n first: %s\nsecond: %s", firstJSON, secondJSON)
	}

	// Non-regression on DF-CONSENSUS-53: re-PATCHing the derived mcp block must
	// not persist a literal mcp row (the dotted mcp.<name> row wins), and the
	// /mcp/{name}/* resolver must still find the server.
	rows := df54SettingRows(t, conn)
	if len(rows) != 1 {
		t.Fatalf("system_settings rows after round trip = %v, want exactly one flat server row", rows)
	}
	if key := toString(rows[0]["key"]); key != "mcp.websearch" {
		t.Errorf("system_settings key after round trip = %q, want mcp.websearch (a derived block must never be persisted)", key)
	}
	assertMCPNameOperationsResolve(t, srv.URL, "websearch")
}

// TestGetConfigNonJSONSettingsStayStrings is the "nothing invented" arm: text
// that is not JSON (the legacy Go %v rows written before DF-CONSENSUS-53) is
// returned unchanged instead of being guessed into a structure.
func TestGetConfigNonJSONSettingsStayStrings(t *testing.T) {
	srv, conn := newMCPConfigTestServer(t)

	const legacyMCP = "map[websearch:map[type:remote url:https://mcp.example.com]]"
	const legacyServer = "map[type:remote url:https://mcp.example.com]"
	df54InsertSetting(t, conn, "mcp", legacyMCP)
	df54InsertSetting(t, conn, "mcp.websearch", legacyServer)

	settings := df54GetSettings(t, srv.URL)
	if got := settings["mcp"]; got != legacyMCP {
		t.Errorf("settings.mcp = %#v, want the raw text %q unchanged (an unparseable value is not structure)", got, legacyMCP)
	}
	if got := settings["mcp.websearch"]; got != legacyServer {
		t.Errorf("settings[mcp.websearch] = %#v, want the raw text %q unchanged", got, legacyServer)
	}

	// A JSON array row IS JSON, so it is emitted as an array.
	df54InsertSetting(t, conn, "list.value", `["a","b"]`)
	settings = df54GetSettings(t, srv.URL)
	if got := settings["list.value"]; !reflect.DeepEqual(got, []any{"a", "b"}) {
		t.Errorf("settings[list.value] = %#v, want the JSON array [a b]", got)
	}
}

// TestGetConfigWithoutMCPRowsHasNoDerivedBlock is the non-vacuity control for
// the derived block: a store with no mcp.<name> server rows must not grow an
// mcp key (the block is derived from real rows, never synthesized).
func TestGetConfigWithoutMCPRowsHasNoDerivedBlock(t *testing.T) {
	mdb := &mockDB{
		queryResults: []db.Row{
			rowOf(map[string]any{"key": "llm.default_model", "value": "gpt-4o"}),
		},
	}
	_, srv := newTestServer(mdb)
	defer srv.Close()

	settings := df54GetSettings(t, srv.URL)
	if _, exists := settings["mcp"]; exists {
		t.Errorf("settings = %#v, want no mcp block when the store holds no mcp.<name> server row", settings)
	}
	if got := settings["llm.default_model"]; got != "gpt-4o" {
		t.Errorf("settings[llm.default_model] = %#v, want the string \"gpt-4o\"", got)
	}
}
