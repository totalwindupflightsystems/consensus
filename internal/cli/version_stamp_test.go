// RELEASE-CONSENSUS-1: goreleaser build-stamp adoption tests.
//
// The stamp path is: goreleaser ldflags → main.version/commit/date
// (cmd/consensus) → SetBuildInfo → the shared version var in root.go that
// --version and `consensus version` read. These tests
// pin the adoption contract: stamped values land, inert defaults do not
// clobber existing values, and the version subcommand surfaces commit/date
// once stamped.
package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

// saveVersionGlobals preserves the package-level version surface across a
// test that mutates it (tests run sequentially in this package; sibling
// tests assert on the VERSION-file default).
func saveVersionGlobals(t *testing.T) (restore func()) {
	t.Helper()
	ov, oc, od := version, buildCommit, buildDate
	t.Cleanup(func() { version, buildCommit, buildDate = ov, oc, od })
	return func() {}
}

// captureVersionSubcommand runs `consensus version` and returns its stdout
// (the formatter is hardwired to os.Stdout; see TestRootCommandVersionSubcommand
// for the os.Stdout swap precedent).
func captureVersionSubcommand(t *testing.T) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		os.Stdout = oldStdout
		t.Fatalf("consensus version failed: %v", err)
	}
	os.Stdout = oldStdout
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured output: %v", err)
	}
	return string(out)
}

// TestSetBuildInfo_StampedValuesAdopted proves the adoption seam lands all
// three goreleaser stamps (RED before SetBuildInfo existed: the function did
// not compile, and the binary ignored main.* ldflags entirely).
func TestSetBuildInfo_StampedValuesAdopted(t *testing.T) {
	saveVersionGlobals(t)

	SetBuildInfo("v0.1.0-test", "abc1234", "2026-09-29T00:00:00Z")

	if version != "v0.1.0-test" {
		t.Errorf("version = %q, want stamped v0.1.0-test", version)
	}
	if buildCommit != "abc1234" {
		t.Errorf("buildCommit = %q, want stamped abc1234", buildCommit)
	}
	if buildDate != "2026-09-29T00:00:00Z" {
		t.Errorf("buildDate = %q, want stamped 2026-09-29T00:00:00Z", buildDate)
	}
}

// TestSetBuildInfo_DefaultsAreInert proves the uninstantiated defaults
// (dev/none/unknown — what main's vars hold on any build that did NOT receive
// the goreleaser ldflags) never clobber a live value. Guards the mis-stamped
// scenario: a partial stamp must not blank the surfaces.
func TestSetBuildInfo_DefaultsAreInert(t *testing.T) {
	saveVersionGlobals(t)
	version = "0.1.0"
	buildCommit, buildDate = "", ""

	SetBuildInfo("dev", "none", "unknown")

	if version != "0.1.0" {
		t.Errorf("version = %q, want VERSION-file default preserved", version)
	}
	if buildCommit != "" || buildDate != "" {
		t.Errorf("commit/date = %q/%q, want empty (defaults inert)", buildCommit, buildDate)
	}
}

// TestSetBuildInfo_EmptyArgsNeverBlank proves empty strings are refused too:
// a stray caller cannot blank the version surface.
func TestSetBuildInfo_EmptyArgsNeverBlank(t *testing.T) {
	saveVersionGlobals(t)
	version = "0.1.0"

	SetBuildInfo("", "", "")

	if version != "0.1.0" {
		t.Errorf("version = %q, want preserved on empty args", version)
	}
}

// TestVersionSubcommandShowsStamps proves the `consensus version` output
// carries commit and date once a stamped build adopts them.
func TestVersionSubcommandShowsStamps(t *testing.T) {
	saveVersionGlobals(t)

	SetBuildInfo("v0.1.0-test", "abc1234", "2026-09-29T00:00:00Z")
	out := captureVersionSubcommand(t)

	for _, want := range []string{"consensus version", "v0.1.0-test", "abc1234", "2026-09-29T00:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("version output missing %q, got: %q", want, out)
		}
	}
}

// TestVersionSubcommandUnstampedHasNoStamps proves an unstamped build prints
// only the version — no commit/date fields leaking as empty values.
func TestVersionSubcommandUnstampedHasNoStamps(t *testing.T) {
	saveVersionGlobals(t)
	buildCommit, buildDate = "", ""

	out := captureVersionSubcommand(t)

	if !strings.Contains(out, "consensus version") {
		t.Errorf("version output missing banner, got: %q", out)
	}
	if strings.Contains(out, `"commit"`) || strings.Contains(out, `"date"`) {
		t.Errorf("unstamped version output should omit commit/date, got: %q", out)
	}
}
