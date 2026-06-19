package config

import (
	"os"
	"testing"
)

// axiom:trace work_item=repo-bootstrap-01 spec=specs/016-cli-interface.md,specs/021-repository-layout.md plan=phase-1/task-1/step-2 test=internal/config/config_test.go

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Server.Port != 8090 {
		t.Errorf("expected port 8090, got %d", cfg.Server.Port)
	}
	if cfg.Server.Hostname != "127.0.0.1" {
		t.Errorf("expected hostname 127.0.0.1, got %s", cfg.Server.Hostname)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("expected openai provider, got %s", cfg.LLM.Provider)
	}
	if cfg.Harness.HeartbeatIntervalSec != 5 {
		t.Errorf("expected heartbeat 5s, got %d", cfg.Harness.HeartbeatIntervalSec)
	}
	if cfg.Harness.MaxIterations != 100 {
		t.Errorf("expected max iterations 100, got %d", cfg.Harness.MaxIterations)
	}
	if cfg.Harness.MaxConsecutiveErrors != 3 {
		t.Errorf("expected max consecutive errors 3, got %d", cfg.Harness.MaxConsecutiveErrors)
	}
	if cfg.Database.URL != "sqlite://dev.db" {
		t.Errorf("expected sqlite://dev.db, got %s", cfg.Database.URL)
	}
	if cfg.HITL.RequireApprovalForDestructive != true {
		t.Errorf("expected require_approval_for_destructive=true")
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected info log level, got %s", cfg.Logging.Level)
	}
}

func TestLoadNoFileUsesDefaults(t *testing.T) {
	// Ensure no config file is found.
	os.Setenv("CONSENSUS_CONFIG", "/nonexistent/path")
	defer os.Unsetenv("CONSENSUS_CONFIG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Port != 8090 {
		t.Errorf("expected default port 8090, got %d", cfg.Server.Port)
	}
}

func TestEnvOverride(t *testing.T) {
	os.Setenv("CONSENSUS_DB_URL", "postgres://override:5432/db")
	os.Setenv("CONSENSUS_CONFIG", "/nonexistent/path")
	defer os.Unsetenv("CONSENSUS_DB_URL")
	defer os.Unsetenv("CONSENSUS_CONFIG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Database.URL != "postgres://override:5432/db" {
		t.Errorf("expected env override URL, got %s", cfg.Database.URL)
	}
}

func TestHITLDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.HITL.AutoPauseOnErrorThreshold != 3 {
		t.Errorf("expected auto_pause threshold 3, got %d", cfg.HITL.AutoPauseOnErrorThreshold)
	}
	if cfg.HITL.RequireApprovalForSchemaChanges != true {
		t.Errorf("expected require_approval_for_schema_changes=true")
	}
	if cfg.HITL.ApprovalTimeoutMinutes != 60 {
		t.Errorf("expected approval timeout 60, got %d", cfg.HITL.ApprovalTimeoutMinutes)
	}
}

func TestAPIRateDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.APIRate.AdminLimit != 1000 {
		t.Errorf("expected admin limit 1000, got %d", cfg.APIRate.AdminLimit)
	}
	if cfg.APIRate.SessionLimit != 100 {
		t.Errorf("expected session limit 100, got %d", cfg.APIRate.SessionLimit)
	}
}
