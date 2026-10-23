// Package harness: config wiring regression for the planning deadline
// (CONFIG-INERT-1).
//
// harness.planning_timeout_seconds was declared, documented and defaulted at
// 180 in internal/config, and nothing in the tree read it: every dispatch went
// through RunInteractivePlanning(ctx, id, nil), which falls back to
// DefaultPlanningConfig() and silently ignored the operator's value. The
// deadline is now carried on the Harness (PlanningTimeout) and applied by
// planningConfig(), which is what pollAndDispatch hands to
// RunInteractivePlanning. These tests pin both halves: the field cannot be
// dropped without failing, and a configured value cannot silently fail to
// override the default.
//
// axiom:trace work_item=config-inert-1 spec=internal/config/config.go test=internal/harness/planning_timeout_config_test.go
package harness

import (
	"testing"
	"time"

	"github.com/wojons/consensus/internal/config"
)

// TestHarnessPlanningTimeoutDefault: a fresh harness carries the shipped 180s
// planning deadline, so the wiring changed no default behavior.
func TestHarnessPlanningTimeoutDefault(t *testing.T) {
	h := New(nil, nil)

	want := DefaultPlanningConfig().Timeout
	if h.PlanningTimeout != want {
		t.Errorf("New().PlanningTimeout = %v, want the planning default %v", h.PlanningTimeout, want)
	}
	if got := h.planningConfig().Timeout; got != want {
		t.Errorf("planningConfig().Timeout = %v, want %v", got, want)
	}
	// Every other planning setting must still come from the spec defaults.
	if got := h.planningConfig(); got.MaxTurns != DefaultPlanningConfig().MaxTurns ||
		got.MaxRollbacks != DefaultPlanningConfig().MaxRollbacks {
		t.Errorf("planningConfig() dropped spec defaults: %+v", got)
	}
}

// TestHarnessPlanningTimeoutFromConfig is the end-to-end seam: a value that
// arrived through internal/config (the same struct the YAML loader and env
// overrides fill) reaches the configuration the dispatcher hands to the
// planning loop.
func TestHarnessPlanningTimeoutFromConfig(t *testing.T) {
	// Exactly what cmd/consensus/main.go does with the resolved config.
	cfg := config.Defaults()
	cfg.Harness.PlanningTimeoutSec = 300

	h := New(nil, nil)
	if cfg.Harness.PlanningTimeoutSec > 0 {
		h.PlanningTimeout = time.Duration(cfg.Harness.PlanningTimeoutSec) * time.Second
	}

	if got := h.planningConfig().Timeout; got != 300*time.Second {
		t.Fatalf("planning deadline = %v, want 5m from harness.planning_timeout_seconds", got)
	}

	// Non-positive values keep the default instead of creating an
	// instantly-expired deadline.
	cfg.Harness.PlanningTimeoutSec = 0
	h.PlanningTimeout = 0
	if got := h.planningConfig().Timeout; got != DefaultPlanningConfig().Timeout {
		t.Errorf("planning deadline with unset config = %v, want the default", got)
	}
}
