// Memory-bank skeleton bootstrapping for `consensus init`.
//
// Fresh checkouts have no .memory-bank/ (it is gitignored), yet AGENTS.md
// directs agents to read .memory-bank/_index.md and write findings to
// .memory-bank/findings/ at task start. EnsureMemoryBank closes that gap by
// creating the skeleton during init (C-GAP-013).
//
// axiom:trace work_item=c-gap-013 spec=specs/016-cli-interface.md impl=internal/bootstrap/memory_bank.go test=internal/bootstrap/memory_bank_test.go
package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
)

// MemoryBankDirName is the agent memory-bank directory bootstrapped by init
// and referenced throughout AGENTS.md.
const MemoryBankDirName = ".memory-bank"

// MemoryBankResult describes what EnsureMemoryBank did.
type MemoryBankResult struct {
	// Created lists the .memory-bank-relative paths of files written on
	// this call (empty when the skeleton was already present).
	Created []string
	// Dir is the absolute path of the memory-bank root.
	Dir string
}

// EnsureMemoryBank creates the .memory-bank/ skeleton under baseDir:
// _index.md, _prompt.md, findings/_index.md, and the work-items/ directory.
//
// It is idempotent: existing files are never overwritten (O_EXCL write),
// and existing directories are reused. Only the files written on this call
// are reported in the result.
//
// axiom:trace work_item=c-gap-013 spec=specs/016-cli-interface.md impl=internal/bootstrap/memory_bank.go test=internal/bootstrap/memory_bank_test.go
func EnsureMemoryBank(baseDir string) (MemoryBankResult, error) {
	root := filepath.Join(baseDir, MemoryBankDirName)
	for _, dir := range []string{
		root,
		filepath.Join(root, "findings"),
		filepath.Join(root, "work-items"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return MemoryBankResult{}, fmt.Errorf("memory bank: create %s: %w", dir, err)
		}
	}

	files := []struct {
		rel     string
		content string
	}{
		{"_index.md", memoryBankIndexTemplate},
		{"_prompt.md", memoryBankPromptTemplate},
		{filepath.Join("findings", "_index.md"), memoryBankFindingsIndexTemplate},
	}

	result := MemoryBankResult{Dir: root}
	for _, f := range files {
		path := filepath.Join(root, f.rel)
		written, err := writeFileIfAbsent(path, f.content)
		if err != nil {
			return MemoryBankResult{}, fmt.Errorf("memory bank: %w", err)
		}
		if written {
			result.Created = append(result.Created, filepath.ToSlash(filepath.Join(MemoryBankDirName, f.rel)))
		}
	}
	return result, nil
}

// writeFileIfAbsent writes content to path only when the file does not
// already exist (O_CREATE|O_EXCL). It reports whether a new file was written.
func writeFileIfAbsent(path, content string) (bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
}
