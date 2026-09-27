package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeScopedConfig(t *testing.T, port int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "consensus.yaml")
	if err := os.WriteFile(path, []byte(fmt.Sprintf("server:\n  port: %d\n", port)), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestSetConfigPathRestore(t *testing.T) {
	t.Setenv("CONSENSUS_CONFIG", "")
	t.Setenv("CONSENSUS_PORT", "")

	firstPath := writeScopedConfig(t, 4101)
	secondPath := writeScopedConfig(t, 4202)

	restoreFirst := SetConfigPath(firstPath)
	t.Cleanup(restoreFirst)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load first override: %v", err)
	}
	if cfg.Server.Port != 4101 {
		t.Fatalf("first override port=%d, want 4101", cfg.Server.Port)
	}

	restoreSecond := SetConfigPath(secondPath)
	cfg, err = Load()
	if err != nil {
		t.Fatalf("load second override: %v", err)
	}
	if cfg.Server.Port != 4202 {
		t.Fatalf("second override port=%d, want 4202", cfg.Server.Port)
	}

	restoreSecond()
	cfg, err = Load()
	if err != nil {
		t.Fatalf("load restored first override: %v", err)
	}
	if cfg.Server.Port != 4101 {
		t.Fatalf("restored override port=%d, want 4101", cfg.Server.Port)
	}
}
