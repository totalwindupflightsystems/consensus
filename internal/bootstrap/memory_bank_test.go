// Package bootstrap: tests for .memory-bank/ skeleton bootstrapping (C-GAP-013).
//
// axiom:trace work_item=c-gap-013 spec=specs/016-cli-interface.md test=internal/bootstrap/memory_bank_test.go
package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureMemoryBank_CreatesSkeleton(t *testing.T) {
	base := t.TempDir()

	result, err := EnsureMemoryBank(base)
	if err != nil {
		t.Fatalf("ensure memory bank: %v", err)
	}

	// All three template files exist with placeholder content.
	wantFiles := []string{
		".memory-bank/_index.md",
		".memory-bank/_prompt.md",
		".memory-bank/findings/_index.md",
	}
	for _, rel := range wantFiles {
		p := filepath.Join(base, rel)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("expected non-empty content in %s", rel)
		}
	}
	// The work-items directory exists.
	if fi, err := os.Stat(filepath.Join(base, ".memory-bank", "work-items")); err != nil || !fi.IsDir() {
		t.Errorf("expected .memory-bank/work-items/ directory, got err=%v", err)
	}

	// Every created path is reported.
	if len(result.Created) != len(wantFiles) {
		t.Fatalf("expected %d created files, got %d: %v", len(wantFiles), len(result.Created), result.Created)
	}
	for _, rel := range wantFiles {
		found := false
		for _, c := range result.Created {
			if c == rel {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s in Created, got %v", rel, result.Created)
		}
	}
	if !filepath.IsAbs(result.Dir) {
		t.Errorf("expected absolute Dir, got %q", result.Dir)
	}
}

func TestEnsureMemoryBank_TemplatesMatchAgentsPromises(t *testing.T) {
	base := t.TempDir()
	if _, err := EnsureMemoryBank(base); err != nil {
		t.Fatalf("ensure memory bank: %v", err)
	}

	// AGENTS.md promises: _index.md navigates the bank, _prompt.md gives
	// rules, findings/_index.md is the finding index, work-items/ holds items.
	index, err := os.ReadFile(filepath.Join(base, ".memory-bank", "_index.md"))
	if err != nil {
		t.Fatalf("read _index.md: %v", err)
	}
	for _, want := range []string{"findings/_index.md", "work-items/<WORK_ITEM_ID>/", "_prompt.md", "AGENTS.md"} {
		if !strings.Contains(string(index), want) {
			t.Errorf("_index.md missing %q", want)
		}
	}

	prompt, err := os.ReadFile(filepath.Join(base, ".memory-bank", "_prompt.md"))
	if err != nil {
		t.Fatalf("read _prompt.md: %v", err)
	}
	for _, want := range []string{"_index.md", "findings/", "work-items/<WORK_ITEM_ID>/"} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("_prompt.md missing %q", want)
		}
	}

	findings, err := os.ReadFile(filepath.Join(base, ".memory-bank", "findings", "_index.md"))
	if err != nil {
		t.Fatalf("read findings/_index.md: %v", err)
	}
	if !strings.Contains(string(findings), "Findings") {
		t.Errorf("findings/_index.md missing 'Findings' heading")
	}
}

func TestEnsureMemoryBank_Idempotent_DoesNotOverwrite(t *testing.T) {
	base := t.TempDir()
	if _, err := EnsureMemoryBank(base); err != nil {
		t.Fatalf("ensure memory bank: %v", err)
	}

	// Plant custom content in an existing file, then re-run: it must not
	// be overwritten, and nothing must be reported as created.
	custom := "# custom agent notes\nkeep me\n"
	if err := os.WriteFile(filepath.Join(base, ".memory-bank", "_index.md"), []byte(custom), 0o644); err != nil {
		t.Fatalf("plant custom _index.md: %v", err)
	}

	result, err := EnsureMemoryBank(base)
	if err != nil {
		t.Fatalf("ensure memory bank second call: %v", err)
	}
	if len(result.Created) != 0 {
		t.Fatalf("expected no files created on second call, got %v", result.Created)
	}
	data, err := os.ReadFile(filepath.Join(base, ".memory-bank", "_index.md"))
	if err != nil {
		t.Fatalf("read _index.md: %v", err)
	}
	if string(data) != custom {
		t.Errorf("existing _index.md was overwritten:\n got: %q\nwant: %q", string(data), custom)
	}
}

func TestEnsureMemoryBank_ErrorWhenBaseIsAFile(t *testing.T) {
	base := t.TempDir()
	filePath := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("create base file: %v", err)
	}

	if _, err := EnsureMemoryBank(filePath); err == nil {
		t.Fatal("expected error when baseDir is a file, got nil")
	}
}
