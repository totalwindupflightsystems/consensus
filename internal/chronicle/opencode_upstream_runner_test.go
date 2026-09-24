package chronicle

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type upstreamManifest struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	Version    string `json:"version"`
	LockSHA256 string `json:"lock_sha256"`
	Suites     []struct {
		Path      string `json:"path"`
		SHA256    string `json:"sha256"`
		Transport string `json:"transport"`
	} `json:"suites"`
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func TestOpenCodeUpstreamRunnerManifestIsPinned(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "scripts", "opencode-upstream", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest upstreamManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Repository != "https://github.com/anomalyco/opencode.git" {
		t.Fatalf("repository = %q", manifest.Repository)
	}
	if manifest.Revision != "16747470f976aca3d362ad730bcd3fe82ecc2c9a" {
		t.Fatalf("revision = %q", manifest.Revision)
	}
	if manifest.Version != "1.18.29" {
		t.Fatalf("version = %q", manifest.Version)
	}
	if manifest.LockSHA256 != "e4a33f0dce76bd625ceef88b2fe44cfa75d0653a3ccba0c7066ac558896ff239" {
		t.Fatalf("lock hash = %q", manifest.LockSHA256)
	}

	want := []struct {
		path      string
		sha256    string
		transport string
	}{
		{
			"packages/opencode/test/server/httpapi-instance.test.ts",
			"40b0b2a4161c8cc71fb213553c1c70d8cfd5ac443fb4e76e6e06a790be918ce1",
			"adapter.patch + fetch preload",
		},
		{
			"packages/opencode/test/server/httpapi-sdk.test.ts",
			"5c77c49091e2ba83d8e3735570f9913ddb63b0d0c7e849761e9d3a89fc1c3562",
			"fetch preload",
		},
		{
			"packages/opencode/test/server/sdk-error-shape.test.ts",
			"fdd44f6fc14268546fc70841a8da718c4cedb79c652b4685269da25c8a68d386",
			"adapter.patch + fetch preload",
		},
		{
			"packages/client/test/promise.test.ts",
			"0365ea4fbba26d8c5d4d61eb539e485f3f46b49a43df851c1f1c27a58bcd89ab",
			"self-contained SDK contract; no live HTTP transport",
		},
	}
	if len(manifest.Suites) != len(want) {
		t.Fatalf("suite count = %d, want %d", len(manifest.Suites), len(want))
	}
	for i := range want {
		if manifest.Suites[i].Path != want[i].path {
			t.Errorf("suite[%d] path = %q, want %q", i, manifest.Suites[i].Path, want[i].path)
		}
		if manifest.Suites[i].SHA256 != want[i].sha256 {
			t.Errorf("suite[%d] hash = %q, want %q", i, manifest.Suites[i].SHA256, want[i].sha256)
		}
		if manifest.Suites[i].Transport != want[i].transport {
			t.Errorf("suite[%d] transport = %q, want %q", i, manifest.Suites[i].Transport, want[i].transport)
		}
	}
}

func TestOpenCodeUpstreamAdapterDoesNotRewriteAssertions(t *testing.T) {
	root := repositoryRoot(t)
	f, err := os.Open(filepath.Join(root, "scripts", "opencode-upstream", "adapter.patch"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	seenMarker := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "CONSENSUS_ADAPTER") {
			seenMarker = true
		}
		if !strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "-") {
			continue
		}
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		for _, assertion := range []string{"expect(", "describe(", "test(", "it.live(", "it.instance("} {
			if strings.Contains(line, assertion) {
				t.Errorf("adapter mutates upstream assertion/registration: %s", line)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !seenMarker {
		t.Fatal("adapter patch has no CONSENSUS_ADAPTER marker")
	}
}

func TestOpenCodeUpstreamRunnerWiring(t *testing.T) {
	root := repositoryRoot(t)
	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	runner := read("scripts/test-opencode-upstream.sh")
	for _, required := range []string{
		`git -C "$SCRIPT_DIR/.." rev-parse --show-toplevel`,
		`done <"$SUITE_LIST"`,
		`docs/evidence/opencode-upstream-v$VERSION`,
		`CONSENSUS_OPENCODE_BASE_URL`,
	} {
		if !strings.Contains(runner, required) {
			t.Errorf("runner missing %q", required)
		}
	}
	for _, forbidden := range []string{"| while ", "head -n", "tail -n"} {
		if strings.Contains(runner, forbidden) {
			t.Errorf("runner contains fragile/forbidden construct %q", forbidden)
		}
	}

	makefile := read("Makefile")
	if !strings.Contains(makefile, "test-opencode-upstream") ||
		!strings.Contains(makefile, "test-opencode-upstream:\n") ||
		!strings.Contains(makefile, "scripts/test-opencode-upstream.sh") ||
		!strings.Contains(makefile, "CONSENSUS_OPENCODE_BASE_URL") {
		t.Fatal("Makefile does not expose the opt-in upstream compatibility target")
	}
	if !strings.Contains(makefile, "test-short:\n	$(CGO_FLAGS) $(GO) test") {
		t.Fatal("ordinary test-short target must remain an offline Go-only target")
	}

	proof := read("scripts/prove-opencode-cli.sh")
	for _, required := range []string{
		"opencode run --attach",
		"--pure",
		"CONSENSUS_LLM_BASE_URL",
		"docs/evidence/opencode-cli-v1.18.29",
		"[REDACTED]",
	} {
		if !strings.Contains(proof, required) {
			t.Errorf("CLI proof script missing %q", required)
		}
	}
	for _, name := range []string{"scripts/test-opencode-upstream.sh", "scripts/prove-opencode-cli.sh"} {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&0o111 == 0 {
			t.Errorf("%s is not executable", name)
		}
	}
}

func TestOpenCodeUpstreamRunnerSelfTest(t *testing.T) {
	root := repositoryRoot(t)
	runner := filepath.Join(root, "scripts", "test-opencode-upstream.sh")
	cmd := exec.Command("sh", runner, "--self-test")
	cmd.Dir = t.TempDir()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("runner self-test: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "self-test: ok") {
		t.Fatalf("runner self-test output missing success marker:\n%s", out)
	}
}
