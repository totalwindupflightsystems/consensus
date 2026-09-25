package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// --- shipped consensus.yaml vs the DeepSeek compression guard (DF-CONSENSUS-22) ---
//
// C-GAP-002 taught ApplyStartupValidations to disable compression and warn
// when the LLM backend has no embeddings endpoint, and C-GAP-002's follow-up
// fixed the compiled default — but the shipped consensus.yaml kept
// compression.enabled=true against the DeepSeek base_url it also pins, so a
// fresh install logged "Compression worker DISABLED" on every boot. The
// shipped file must already satisfy the guard, not lean on the runtime
// correction. This test loads the repository's shipped consensus.yaml through
// the same load/validate path serve uses (LoadWithPath is what config.Load()
// delegates to, and cmd/consensus/main.go calls ApplyStartupValidations on
// the result), so a regression in the file fails here before it ships.

// TestLoad_ShippedConfigNoCompressionWarning loads the shipped consensus.yaml
// from the repo root and asserts the startup validation neither warns about
// the compression worker nor leaves compression enabled for the DeepSeek
// base URL case.
func TestLoad_ShippedConfigNoCompressionWarning(t *testing.T) {
	hermeticEnv(t)

	// Test CWD is the package dir (internal/config); the shipped file is
	// two levels up at the repo root.
	cfg, err := LoadWithPath(filepath.Join("..", "..", "consensus.yaml"))
	if err != nil {
		t.Fatalf("LoadWithPath(shipped consensus.yaml): %v", err)
	}

	// Guard precondition: this test only pins something while the shipped
	// LLM block still targets DeepSeek. If that changes, revisit the test.
	if cfg.LLM.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("shipped consensus.yaml no longer targets DeepSeek (base_url=%q); revisit this test", cfg.LLM.BaseURL)
	}

	warns := cfg.ApplyStartupValidations()

	for _, w := range warns {
		if strings.Contains(w, "Compression worker DISABLED") {
			t.Errorf("shipped consensus.yaml still triggers the compression warning: %q", w)
		}
	}
	if cfg.Compression.Enabled {
		t.Error("expected compression disabled after validation for the DeepSeek base URL")
	}
}
