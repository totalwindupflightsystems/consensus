package opencode

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ch:trace row=BUG-013 spec=none:config-surface wave=none:serial test=TestMessageResponseTimeoutParseEnv,TestNewServerMessageResponseTimeoutDefault,TestNewServerWithMessageResponseTimeoutOption,TestNewServerMessageResponseTimeoutEnvOverride,TestNewServerMessageResponseTimeoutInvalidEnvFallsBack,TestMessageResponseTimeoutOptionDrivesRequestDeadline doc=docs/CONFIG.md prompt=none:worker-brief evidence=commit witness=none:local-repo-verification verdict=none:pending commit=self memory=none:pending
//
// BUG-013: the shim's synchronous message-response budget was a hard-coded
// 90s constant, so consumers whose agent turns run longer than that
// (dexdat-core planning sessions against slow models) saw every long turn
// surface as 504 RESPONSE_TIMEOUT while the session stayed healthy. These
// tests pin the production surface: the WithMessageResponseTimeout option and
// the CONSENSUS_MESSAGE_RESPONSE_TIMEOUT environment variable, the 90s
// default, and the invalid-value fallback.

// TestMessageResponseTimeoutParseEnv covers the env value grammar on the
// injected-getenv seam (no process environment mutated, so it is safe next to
// any test ordering): unset, empty, valid durations, and every invalid shape
// resolve to "no override" (0) rather than an error.
func TestMessageResponseTimeoutParseEnv(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want time.Duration
	}{
		{"unset", "", 0},
		{"blank", "   ", 0},
		{"three minutes", "3m", 3 * time.Minute},
		{"seconds", "200s", 200 * time.Second},
		{"sub-second", "250ms", 250 * time.Millisecond},
		{"padded", " 3m ", 3 * time.Minute},
		{"zero", "0s", 0},
		{"negative", "-5m", 0},
		{"missing unit", "180", 0},
		{"garbage", "not-a-duration", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := messageResponseTimeoutFromEnv(func(string) string { return tc.val })
			if got != tc.want {
				t.Fatalf("messageResponseTimeoutFromEnv(%q) = %s, want %s", tc.val, got, tc.want)
			}
		})
	}
}

// TestNewServerMessageResponseTimeoutDefault: construction without any
// surface set keeps the compiled 90s default — the budget an existing
// deployment sees must not move.
func TestNewServerMessageResponseTimeoutDefault(t *testing.T) {
	t.Setenv("CONSENSUS_MESSAGE_RESPONSE_TIMEOUT", "")
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	if s.messageResponseTimeout != defaultMessageResponseTimeout {
		t.Fatalf("default budget = %s, want %s", s.messageResponseTimeout, defaultMessageResponseTimeout)
	}
	if s.messageResponseTimeout != 90*time.Second {
		t.Fatalf("default budget = %s, want 90s", s.messageResponseTimeout)
	}
}

// TestNewServerWithMessageResponseTimeoutOption: the option reaches the
// field at construction, and a non-positive value keeps the default.
func TestNewServerWithMessageResponseTimeoutOption(t *testing.T) {
	t.Setenv("CONSENSUS_MESSAGE_RESPONSE_TIMEOUT", "")

	s := NewServer(&mockDB{}, "test-key", nil, nil, WithMessageResponseTimeout(5*time.Minute))
	if s.messageResponseTimeout != 5*time.Minute {
		t.Fatalf("option budget = %s, want 5m", s.messageResponseTimeout)
	}

	for _, d := range []time.Duration{0, -1 * time.Second} {
		s := NewServer(&mockDB{}, "test-key", nil, nil, WithMessageResponseTimeout(d))
		if s.messageResponseTimeout != defaultMessageResponseTimeout {
			t.Fatalf("WithMessageResponseTimeout(%s) budget = %s, want default %s", d, s.messageResponseTimeout, defaultMessageResponseTimeout)
		}
	}
}

// TestNewServerMessageResponseTimeoutEnvOverride: a valid
// CONSENSUS_MESSAGE_RESPONSE_TIMEOUT is parsed at construction, and the env
// value wins over the WithMessageResponseTimeout option — the env-beats-file
// precedence documented in docs/CONFIG.md §2.
func TestNewServerMessageResponseTimeoutEnvOverride(t *testing.T) {
	t.Setenv("CONSENSUS_MESSAGE_RESPONSE_TIMEOUT", "3m")
	s := NewServer(&mockDB{}, "test-key", nil, nil)
	if s.messageResponseTimeout != 3*time.Minute {
		t.Fatalf("env budget = %s, want 3m", s.messageResponseTimeout)
	}

	s = NewServer(&mockDB{}, "test-key", nil, nil, WithMessageResponseTimeout(time.Minute))
	if s.messageResponseTimeout != 3*time.Minute {
		t.Fatalf("env budget with option present = %s, want env 3m to win over option 1m", s.messageResponseTimeout)
	}
}

// TestNewServerMessageResponseTimeoutInvalidEnvFallsBack: an unparseable or
// non-positive env value must not crash startup nor clobber a configured
// value — the construction falls back to the default (or the option value
// when one was given).
func TestNewServerMessageResponseTimeoutInvalidEnvFallsBack(t *testing.T) {
	for _, v := range []string{"banana", "180", "0s", "-3m", ""} {
		t.Run("value="+v, func(t *testing.T) {
			t.Setenv("CONSENSUS_MESSAGE_RESPONSE_TIMEOUT", v)
			s := NewServer(&mockDB{}, "test-key", nil, nil)
			if s.messageResponseTimeout != defaultMessageResponseTimeout {
				t.Fatalf("env %q budget = %s, want default %s", v, s.messageResponseTimeout, defaultMessageResponseTimeout)
			}

			s = NewServer(&mockDB{}, "test-key", nil, nil, WithMessageResponseTimeout(4*time.Minute))
			if s.messageResponseTimeout != 4*time.Minute {
				t.Fatalf("env %q with option = %s, want option 4m to survive", v, s.messageResponseTimeout)
			}
		})
	}
}

// TestMessageResponseTimeoutOptionDrivesRequestDeadline is the request-level
// proof: a server built with the option answers POST /session/{id}/message
// with the concrete 504 RESPONSE_TIMEOUT error once the OPTION's budget —
// not the 90s default — expires. With a 60ms budget the whole exchange is
// bounded (250x load-aware bound below); under the old hard-coded 90s this
// request would sit for two orders of magnitude past the bound and fail.
// Follows TestSendMessageNoResponseReturnsConcreteError's red-arm shape
// (QA-CONSENSUS-22 load-aware bound), no sleeps.
func TestMessageResponseTimeoutOptionDrivesRequestDeadline(t *testing.T) {
	s, _, _ := newMessageResponseTestServer(t, WithMessageResponseTimeout(60*time.Millisecond))
	body := jsonBody(t, map[string]any{
		"parts": []map[string]any{{"type": "text", "text": "no answer will arrive"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/session/s1/message", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("opencode", "test-key")
	recorder := httptest.NewRecorder()

	started := time.Now()
	s.Handler().ServeHTTP(recorder, req)
	// Load-aware bound: 250x the configured budget (10s here) vs the 90s
	// default the request would take if the option did not reach the field.
	bound := 250 * s.messageResponseTimeout
	if elapsed := time.Since(started); elapsed > bound {
		t.Fatalf("no-response path took %s, want bounded cancellation (bound %s)", elapsed, bound)
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "agent response") ||
		!strings.Contains(recorder.Body.String(), "not produced") {
		t.Fatalf("timeout error is not concrete: %s", recorder.Body.String())
	}
}
