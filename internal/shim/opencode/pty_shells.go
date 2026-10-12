package opencode

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ptyShell is the upstream pty.shells response item. The shim discovers
// executable shell files from the host rather than maintaining a fabricated
// shell registry.
type ptyShell struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Acceptable bool   `json:"acceptable"`
}

// handlePtyShells serves GET /pty/shells — upstream pty.shells
// (ROUTE-ADD-104, source item SHIM-DRIFT-107; declared responses: 200
// ShellItem[], 400 BadRequestError).
//
// The host's /etc/shells file is the backing capability for this operation.
// Only entries that still resolve to executable regular files are returned;
// when the file is unavailable, a small set of conventional absolute paths is
// checked. This makes the 200 response a report of real host executables, not a
// claimed PTY registry. The optional directory/workspace selectors are only
// validated because shell availability is host-wide in this shim.
//
// ch:trace row=ROUTE-ADD-104 spec=specs/024-shim-support-matrix.md#intentional-narrowing test=internal/shim/opencode/pty_shells_test.go doc=docs/evidence/opencode-upstream-v1.18.33/surface-diff-report.md evidence=docs/evidence/opencode-upstream-v1.18.33/ witness=none:pending-worker
func (s *Server) handlePtyShells(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}
	if !validatePtyShellSelectors(w, r) {
		return
	}
	writeJSON(w, discoverPtyShells())
}

func validatePtyShellSelectors(w http.ResponseWriter, r *http.Request) bool {
	for _, param := range []string{"directory", "workspace"} {
		values, present := r.URL.Query()[param]
		if !present || len(values) == 0 || strings.TrimSpace(values[0]) == "" {
			if present {
				writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
					fmt.Sprintf("query parameter %q must not be blank", param))
				return false
			}
			continue
		}
		value := strings.TrimSpace(values[0])
		info, err := os.Stat(value)
		if err != nil || !info.IsDir() {
			writeOpencodeError(w, r, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("%s %q does not exist or is not a directory", param, value))
			return false
		}
	}
	return true
}

func discoverPtyShells() []ptyShell {
	paths := shellPaths()
	seen := make(map[string]struct{}, len(paths))
	items := make([]ptyShell, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "." || path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}

		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			continue
		}
		name := strings.ToLower(filepath.Base(path))
		items = append(items, ptyShell{
			Path:       path,
			Name:       name,
			Acceptable: name != "fish" && name != "nu",
		})
	}
	return items
}

func shellPaths() []string {
	if data, err := os.ReadFile("/etc/shells"); err == nil {
		paths := make([]string, 0)
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			paths = append(paths, line)
		}
		if len(paths) > 0 {
			return paths
		}
	}
	return []string{"/bin/bash", "/bin/sh", "/usr/bin/bash", "/usr/bin/sh"}
}
