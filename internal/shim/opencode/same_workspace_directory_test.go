package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSameWorkspaceDirectoryAcceptsResolvedSpellingAndRejectsStrangers pins the
// comparison the path assertions in this package use. The shim reports
// canonical paths (the handlers call filepath.EvalSymlinks before answering),
// while a test fixture built from t.TempDir can retain the host's symlinked
// TMPDIR spelling, so the two spellings must compare EQUAL. The comparison must
// still reject a genuinely different directory, otherwise the relaxed
// assertions could never fail.
func TestSameWorkspaceDirectoryAcceptsResolvedSpellingAndRejectsStrangers(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("create %q: %v", real, err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink %q -> %q: %v", link, real, err)
	}
	sibling := filepath.Join(root, "sibling")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("create %q: %v", sibling, err)
	}

	for _, tc := range []struct {
		name string
		a, b string
		want bool
	}{
		{"identical spelling", real, real, true},
		{"same directory, trailing separator", real, real + string(os.PathSeparator), true},
		{"symlink spelling names the same directory", link, real, true},
		{"symlink spelling, both sides", link, link, true},
		{"different directories", real, sibling, false},
		{"nested directory is not the parent", root, real, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameWorkspaceDirectory(tc.a, tc.b); got != tc.want {
				t.Fatalf("sameWorkspaceDirectory(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
