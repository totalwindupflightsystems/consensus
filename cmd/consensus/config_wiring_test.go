// Package main: CONFIG-INERT-1 wiring regression — the YAML `hitl` section
// now reaches the HITL manager instead of being overwritten by the package
// defaults at every start.
//
// runServer previously called hitlMgr.SetConfiguration(ctx,
// hitl.DefaultConfiguration()) unconditionally, so all four hitl.* keys were
// parsed, documented and ignored. The startup write now goes through
// hitlConfigurationFromConfig, which is what these tests pin: the resolved
// config is the source of the four keys, and the two settings with no YAML key
// keep their shipped defaults.
//
// axiom:trace work_item=config-inert-1 spec=internal/config/config.go test=cmd/consensus/config_wiring_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wojons/consensus/internal/config"
	"github.com/wojons/consensus/internal/hitl"
)

// TestHITLConfigDefaultsUnchanged: with no hitl.* overrides, the startup write
// is byte-identical to the previous hitl.DefaultConfiguration() write, so the
// wiring changes no shipped behavior.
func TestHITLConfigDefaultsUnchanged(t *testing.T) {
	want := hitl.DefaultConfiguration() // Scope is already global
	got := hitlConfigurationFromConfig(config.Defaults().HITL)

	if got != want {
		t.Fatalf("startup HITL configuration drifted from the shipped default:\n got: %+v\nwant: %+v", got, want)
	}
}

// TestHITLConfigReachesManagerConfiguration drives a real config file through
// the loader and the mapping used by runServer, proving the operator's values
// are the ones persisted — and that settings without a YAML key are not
// collateral damage.
func TestHITLConfigReachesManagerConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "consensus.yaml")
	body := `hitl:
  auto_pause_on_error_threshold: 9
  require_approval_for_destructive: false
  require_approval_for_schema_changes: false
  approval_timeout_minutes: 5
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.LoadWithPath(path)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	got := hitlConfigurationFromConfig(cfg.HITL)

	if got.Scope != hitl.ScopeGlobal {
		t.Errorf("Scope = %q, want global", got.Scope)
	}
	if got.AutoPauseOnErrorThreshold != 9 {
		t.Errorf("AutoPauseOnErrorThreshold = %d, want 9", got.AutoPauseOnErrorThreshold)
	}
	if got.RequireApprovalForDestructive {
		t.Error("RequireApprovalForDestructive = true, want false from the file")
	}
	if got.RequireApprovalForSchemaChanges {
		t.Error("RequireApprovalForSchemaChanges = true, want false from the file")
	}
	if got.ApprovalTimeoutMinutes != 5 {
		t.Errorf("ApprovalTimeoutMinutes = %d, want 5", got.ApprovalTimeoutMinutes)
	}

	// The two settings with no YAML key must keep the shipped defaults.
	defaults := hitl.DefaultConfiguration()
	if got.RequireApprovalForExternalTools != defaults.RequireApprovalForExternalTools {
		t.Errorf("RequireApprovalForExternalTools = %v, want the shipped %v",
			got.RequireApprovalForExternalTools, defaults.RequireApprovalForExternalTools)
	}
	if got.NotifyOnPause != defaults.NotifyOnPause {
		t.Errorf("NotifyOnPause = %v, want the shipped %v", got.NotifyOnPause, defaults.NotifyOnPause)
	}
}

// TestHITLConfigZeroValuesKeepProtection: a numeric key written as 0 must not
// disable the protection it configures; the shipped default stands.
func TestHITLConfigZeroValuesKeepProtection(t *testing.T) {
	got := hitlConfigurationFromConfig(config.HITLConfig{
		AutoPauseOnErrorThreshold:       0,
		RequireApprovalForDestructive:   true,
		RequireApprovalForSchemaChanges: true,
		ApprovalTimeoutMinutes:          0,
	})
	defaults := hitl.DefaultConfiguration()

	if got.AutoPauseOnErrorThreshold != defaults.AutoPauseOnErrorThreshold {
		t.Errorf("threshold = %d, want the default %d for an unset value",
			got.AutoPauseOnErrorThreshold, defaults.AutoPauseOnErrorThreshold)
	}
	if got.ApprovalTimeoutMinutes != defaults.ApprovalTimeoutMinutes {
		t.Errorf("approval timeout = %d, want the default %d for an unset value",
			got.ApprovalTimeoutMinutes, defaults.ApprovalTimeoutMinutes)
	}
}
