// CLI: `consensus tool register` (DF-CONSENSUS-62).
//
// The tool verb used to be inspect-only, so a fresh install had no CLI path to
// create the tools_registry row the HITL approval gate fires on. These tests
// pin the register verb's request payload and its local validation.
//
// test=internal/cli/tool_register_test.go
package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newToolRegisterAPI serves POST /api/v1/tools and records the decoded body.
func newToolRegisterAPI(t *testing.T) (*httptest.Server, *[]map[string]any, *[]string) {
	t.Helper()
	bodies := &[]map[string]any{}
	paths := &[]string{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.Method+" "+r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		*bodies = append(*bodies, payload)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                "tool-1",
			"name":              payload["name"],
			"description":       payload["description"],
			"hemisphere":        "internal",
			"handler_type":      "go_native",
			"status":            "active",
			"enabled":           true,
			"requires_approval": payload["requires_approval"],
		})
	}))
	return srv, bodies, paths
}

func TestToolRegister_Payload(t *testing.T) {
	srv, bodies, paths := newToolRegisterAPI(t)
	defer srv.Close()
	defer overrideGlobals(srv.URL, "test-key", "json", false)()

	cmd := newToolRegisterCmd()
	cmd.SetArgs([]string{
		"gated_cleanup",
		"--description", "Flush the response cache",
		"--handler-type", "subprocess",
		"--handler-ref", "purge_cache --all",
		"--requires-approval",
		"--rate-limit", "5",
		"--parameter-schema", `{"type":"object"}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("register: %v", err)
	}

	if len(*paths) != 1 || (*paths)[0] != "POST /api/v1/tools" {
		t.Fatalf("requests = %v, want exactly one POST /api/v1/tools", *paths)
	}

	payload := (*bodies)[0]
	if payload["name"] != "gated_cleanup" {
		t.Errorf("name = %v", payload["name"])
	}
	if payload["description"] != "Flush the response cache" {
		t.Errorf("description = %v", payload["description"])
	}
	if payload["handler_type"] != "subprocess" {
		t.Errorf("handler_type = %v", payload["handler_type"])
	}
	if payload["handler_ref"] != "purge_cache --all" {
		t.Errorf("handler_ref = %v", payload["handler_ref"])
	}
	if payload["requires_approval"] != true {
		t.Errorf("requires_approval = %v, want true", payload["requires_approval"])
	}
	if payload["rate_limit_per_min"] != float64(5) {
		t.Errorf("rate_limit_per_min = %v, want 5", payload["rate_limit_per_min"])
	}
	if schema, ok := payload["parameter_schema"].(map[string]any); !ok || schema["type"] != "object" {
		t.Errorf("parameter_schema = %v, want the decoded object", payload["parameter_schema"])
	}
}

func TestToolRegister_DefaultsOmitted(t *testing.T) {
	srv, bodies, _ := newToolRegisterAPI(t)
	defer srv.Close()
	defer overrideGlobals(srv.URL, "test-key", "json", false)()

	cmd := newToolRegisterCmd()
	cmd.SetArgs([]string{"plain_tool", "--description", "no extras", "--handler-ref", "list_sessions"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("register: %v", err)
	}

	payload := (*bodies)[0]
	for _, key := range []string{"handler_type", "hemisphere", "status", "parameter_schema", "rate_limit_per_min"} {
		if _, present := payload[key]; present {
			t.Errorf("%s should be omitted when unset (server/defaults decide), payload: %v", key, payload)
		}
	}
	if payload["requires_approval"] != false {
		t.Errorf("requires_approval = %v, want false", payload["requires_approval"])
	}
}

func TestToolRegister_RequiresFlagValues(t *testing.T) {
	// No server round trip: these fail locally before any request is made.
	defer overrideGlobals("http://127.0.0.1:1", "test-key", "json", false)()

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no description", []string{"t1", "--handler-ref", "fn_x"}, "--description is required"},
		{"no handler ref", []string{"t2", "--description", "d"}, "--handler-ref is required"},
		{"bad schema", []string{"t3", "--description", "d", "--handler-ref", "fn_x", "--parameter-schema", "nope"}, "must be a JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newToolRegisterCmd()
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}
