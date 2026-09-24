// Package mcp: tests for DOGFOOD-007 (MCP auth error messages point at the
// real configuration surface: --api-key / CONSENSUS_API_KEY, not _meta).
//
// axiom:trace work_item=dogfood-007 spec=specs/015-api-and-mcp.md test=internal/mcp/dogfood007_test.go
package mcp

import (
	"fmt"
	"strings"
	"testing"
)

// dataStr renders err.Data as a string for assertion messages.
func dataStr(err *JSONRPCErrObj) string {
	return fmt.Sprintf("%v", err.Data)
}

// TestValidateAuth_MissingKey_MessagePointsAtConfig verifies the missing-key
// error tells the user to configure the key via --api-key / CONSENSUS_API_KEY
// rather than implying they must hand-craft _meta.authorization (DOGFOOD-007).
func TestValidateAuth_MissingKey_MessagePointsAtConfig(t *testing.T) {
	sess := &mcpSession{}

	// No _meta at all.
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  []byte(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"1"}}`),
	}

	srv := NewServer(&mockMCPDB{})
	err := srv.validateAuth(req, sess)
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	d := dataStr(err)
	if !strings.Contains(d, "--api-key") && !strings.Contains(d, "CONSENSUS_API_KEY") {
		t.Errorf("error should point at --api-key / CONSENSUS_API_KEY config, got Data=%q", d)
	}
	if strings.Contains(d, "missing _meta.authorization") {
		t.Errorf("error should not tell users to set _meta.authorization manually, got Data=%q", d)
	}
}

// TestValidateAuth_TooShortKey_MessagePointsAtConfig verifies the too-short
// key error also references the configuration surface (DOGFOOD-007).
func TestValidateAuth_TooShortKey_MessagePointsAtConfig(t *testing.T) {
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params: []byte(`{
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "t", "version": "1"},
			"_meta": {"authorization": "short"}
		}`),
	}

	srv := NewServer(&mockMCPDB{})
	err := srv.validateAuth(req, sess)
	if err == nil {
		t.Fatal("expected error for too-short key")
	}
	if err.Code != -32001 {
		t.Errorf("expected -32001, got %d", err.Code)
	}
	if !strings.Contains(dataStr(err), "--api-key") && !strings.Contains(dataStr(err), "CONSENSUS_API_KEY") {
		t.Errorf("error should point at --api-key / CONSENSUS_API_KEY config, got Data=%v", err.Data)
	}
}

// TestValidateAuth_NonStringAuthorization_MessagePointsAtConfig verifies the
// non-string authorization error explains the transport field is populated
// from the client's configured key (DOGFOOD-007).
func TestValidateAuth_NonStringAuthorization_MessagePointsAtConfig(t *testing.T) {
	sess := &mcpSession{}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params: []byte(`{
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "t", "version": "1"},
			"_meta": {"authorization": 42}
		}`),
	}

	srv := NewServer(&mockMCPDB{})
	err := srv.validateAuth(req, sess)
	if err == nil {
		t.Fatal("expected error for non-string authorization")
	}
	if !strings.Contains(dataStr(err), "--api-key") && !strings.Contains(dataStr(err), "CONSENSUS_API_KEY") {
		t.Errorf("error should point at --api-key / CONSENSUS_API_KEY config, got Data=%v", err.Data)
	}
}
