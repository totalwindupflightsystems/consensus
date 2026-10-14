package opencode

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// This file serves the two remaining declared v2 filesystem read operations on
// the /api/fs family (ROUTE-ADD-011): v2.fs.list (GET /api/fs/list) and
// v2.fs.read (GET /api/fs/read/*). Both were NOT-SERVED drift rows — no shim
// route at all, so net/http answered the default 404 — and both now answer the
// documented contract from the real host filesystem, because this runtime runs
// on the same filesystem as its workspaces (service_adapter.go) and
// requestWorkspaceDir genuinely resolves one.
//
// The declared contract for both operations
// (specs/openapi/upstream/openapi-1.18.33.json, pinned v1.18.33):
//
//	v2.fs.list  — "List direct children of one directory relative to the
//	               requested location."
//	               query: deepObject `location` ({directory?, workspace?},
//	               additionalProperties false), optional string `path`
//	               200 {location: LocationInfo, data: FileSystemEntry[]}
//	               400 InvalidRequestError, 401 UnauthorizedError
//	v2.fs.read  — "Serve one file relative to the requested location."
//	               query: deepObject `location` (same shape)
//	               200 application/octet-stream (binary string)
//	               400 InvalidRequestError, 401 UnauthorizedError
//
// Neither operation declares a 404: a path that does not exist, is not the
// expected kind, or cannot be read is reported through the declared
// InvalidRequestError 400 — never an undeclared 404 or 5xx (the
// handleFileList/handleProviderAuth doctrine).
//
// Location-relative paths: the requested location IS the workspace root, so
// every path this file accepts (the `path` query of fs.list, the URL suffix of
// fs.read) is interpreted relative to it and every path it returns is spelled
// relative to it — the fs.find convention, which keeps fs.list output directly
// usable as fs.read input. Directories carry fs.find's trailing-slash
// spelling; fs.read accepts a name with or without it.

// fsReadRoutePrefix is the declared v2.fs.read path family (upstream
// `/api/fs/read/*`): the file to serve is everything the request addresses
// after it, relative to the requested location. It is the single spelling used
// by both the route registration and the handler.
const fsReadRoutePrefix = "/api/fs/read/"

// v2Location is a resolved deepObject `location` selector — the absolute
// workspace directory a v2 filesystem operation is rooted at, plus the
// caller-named workspace id when the request carried one.
type v2Location struct {
	directory   string
	workspaceID string
}

// locationInfo renders the declared LocationInfo object for this location:
// `directory` plus the required `project` {id, directory}. The project id is
// derived deterministically from the selected directory (instanceID) — the
// same singleton-instance translation handleFSFind and handleApiAgentList use,
// so no id is invented. `workspaceID` is emitted only when the caller named
// one (the declared ^wrk pattern was already enforced by the resolver).
func (l v2Location) locationInfo() map[string]any {
	info := map[string]any{
		"directory": l.directory,
		"project": map[string]any{
			"id":        instanceID(l.directory),
			"directory": l.directory,
		},
	}
	if l.workspaceID != "" {
		info["workspaceID"] = l.workspaceID
	}
	return info
}

// errPathEscapesLocation marks a caller-supplied path that does not stay under
// the requested location. It is the traversal class the declared 400 answers.
var errPathEscapesLocation = errors.New("path escapes the requested location")

// resolveV2Location validates the declared deepObject `location` query selector
// shared by v2.fs.find / v2.fs.list / v2.fs.read (the v2 SDK's
// location[directory] / location[workspace] spelling) and resolves it to the
// absolute workspace directory the operation is rooted at. It is the
// handleFSFind resolution, factored out for this family's later operations.
//
// Malformed input is answered with the declared InvalidRequestError 400 and the
// bool result is false, so the caller returns without writing a second body:
// an unknown location[...] field (the declared object sets
// additionalProperties: false), a present-but-blank value, a directory that
// cannot be read or is not a directory, and a workspace id that violates the
// declared ^wrk pattern.
func (s *Server) resolveV2Location(w http.ResponseWriter, r *http.Request) (v2Location, bool) {
	q := r.URL.Query()
	for key := range q {
		if strings.HasPrefix(key, "location[") && key != "location[directory]" && key != "location[workspace]" {
			writeV2InvalidRequest(w, "Query", key, fmt.Sprintf("unknown location field %q", key))
			return v2Location{}, false
		}
	}
	for _, key := range []string{"location[directory]", "location[workspace]"} {
		if values, ok := q[key]; ok && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeV2InvalidRequest(w, "Query", key, fmt.Sprintf("query parameter %q must not be blank", key))
			return v2Location{}, false
		}
	}

	root := strings.TrimSpace(q.Get("location[directory]"))
	if root == "" {
		root = s.requestWorkspaceDir(r)
	}
	absoluteRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		writeV2InvalidRequest(w, "Query", "location[directory]", "location directory is invalid")
		return v2Location{}, false
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		writeV2InvalidRequest(w, "Query", "location[directory]",
			fmt.Sprintf("location directory %q cannot be read: %v", absoluteRoot, err))
		return v2Location{}, false
	}
	if !info.IsDir() {
		writeV2InvalidRequest(w, "Query", "location[directory]",
			fmt.Sprintf("location directory %q is not a directory", absoluteRoot))
		return v2Location{}, false
	}

	workspaceID := strings.TrimSpace(q.Get("location[workspace]"))
	if workspaceID != "" && !strings.HasPrefix(workspaceID, "wrk") {
		writeV2InvalidRequest(w, "Query", "location[workspace]", "location[workspace] must start with wrk")
		return v2Location{}, false
	}

	return v2Location{directory: absoluteRoot, workspaceID: workspaceID}, true
}

// resolveWorkspacePath resolves a caller-supplied path against the workspace
// root and refuses anything that is not a plain relative path under it. It
// returns the absolute target plus its location-relative slash spelling.
//
// Refused (all reported to the caller as the declared InvalidRequestError 400,
// never served): an empty path, a NUL byte, an absolute path (the operations
// address paths RELATIVE to the requested location — an absolute path names
// something outside it), and any path carrying a `..` element. The `..` rule is
// deliberately stricter than "normalizes to somewhere inside": a path spelled
// with a parent reference is not a relative path under the location, and
// rejecting the spelling outright means no caller-supplied input can ever walk
// upward out of the workspace root, whatever the filesystem does with it. The
// joined result is additionally checked for containment (filepath.Rel) as a
// second, independent layer.
func resolveWorkspacePath(root, requested string) (string, string, error) {
	raw := strings.TrimSpace(requested)
	if raw == "" {
		return "", "", fmt.Errorf("path is empty")
	}
	if strings.ContainsRune(raw, 0) {
		return "", "", fmt.Errorf("path contains a NUL byte")
	}
	slashed := filepath.ToSlash(raw)
	if filepath.IsAbs(raw) || strings.HasPrefix(slashed, "/") {
		return "", "", fmt.Errorf(
			"path %q is absolute; the v2 filesystem operations address paths relative to the requested location",
			requested)
	}
	for _, element := range strings.Split(slashed, "/") {
		if element == ".." {
			return "", "", fmt.Errorf("%w: %q contains a parent reference", errPathEscapesLocation, requested)
		}
	}
	cleaned := filepath.Clean(filepath.FromSlash(slashed))
	if cleaned == ".." {
		return "", "", fmt.Errorf("%w: %q contains a parent reference", errPathEscapesLocation, requested)
	}
	absolute := filepath.Join(root, cleaned)
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("%w: %q", errPathEscapesLocation, requested)
	}
	return absolute, filepath.ToSlash(relative), nil
}

// ensureNoSymlinkEscape refuses a path whose REAL location — every symlink in
// the chain resolved — leaves the workspace root. The lexical containment
// check above cannot see a symlink stored inside the workspace that points out
// of it, and this operation must never serve content from outside the requested
// location, so the resolved target is checked against the resolved root. Both
// sides are resolved, so a workspace that is itself reached through a symlink
// still compares consistently.
func ensureNoSymlinkEscape(root, absolute string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("path %q cannot be read: %v", absolute, err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return fmt.Errorf("path %q cannot be read: %v", absolute, err)
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedTarget)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q resolves to %q, outside %q",
			errPathEscapesLocation, absolute, resolvedTarget, resolvedRoot)
	}
	return nil
}

// dispatchWithV2FSTraversalGuard dispatches a request to the shim's ServeMux
// with one interception in front of it: a v2.fs.read request whose URL path
// carries a `..` element.
//
// net/http's ServeMux canonicalizes the request path BEFORE matching and
// answers a redirect (308) to the cleaned path when it is not canonical, so a
// traversing path would never reach handleFSRead — the operation's declared
// InvalidRequestError arm would be unreachable for exactly the input the arm
// exists for, and a caller would receive a status the operation does not
// declare. The traversal is therefore refused here, with the declared 400, and
// the path is never walked or opened. Both spellings are covered because the
// check reads the DECODED URL path: `/api/fs/read/../x` and its percent-encoded
// form are refused identically. Every other request — including the canonical
// ones — is dispatched to the mux unchanged.
//
// The guard runs behind the auth wall (Server.Handler), so an unauthenticated
// traversal request is still answered with the declared 401, like every other
// arm of this surface.
func (s *Server) dispatchWithV2FSTraversalGuard(w http.ResponseWriter, r *http.Request) {
	if fsReadRequestCarriesParentReference(r) {
		writeV2InvalidRequest(w, "Path", "",
			fmt.Sprintf("%v: %q contains a parent reference", errPathEscapesLocation, r.URL.Path))
		return
	}
	s.mux.ServeHTTP(w, r)
}

// fsReadRequestCarriesParentReference reports whether the request addresses the
// v2.fs.read path family (everything after fsReadRoutePrefix is the file to
// serve) with a `..` element.
func fsReadRequestCarriesParentReference(r *http.Request) bool {
	if r == nil || r.URL == nil || !strings.HasPrefix(r.URL.Path, fsReadRoutePrefix) {
		return false
	}
	for _, element := range strings.Split(strings.TrimPrefix(r.URL.Path, fsReadRoutePrefix), "/") {
		if element == ".." {
			return true
		}
	}
	return false
}

// handleFSList serves GET /api/fs/list — upstream v2.fs.list (ROUTE-ADD-011,
// source item SHIM-DRIFT-007 of
// docs/reports/shim-source-002-baseline-diff-2026-09-29.md; declared
// responses: 200 {location: LocationInfo, data: FileSystemEntry[]}, 400
// InvalidRequestError, 401 UnauthorizedError). Before this handler the path had
// no route at all and net/http answered the default 404 (the artifact's
// NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-011 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.fs.list test=internal/shim/opencode/fs_list_test.go evidence=docs/evidence/ROUTE-ADD-011-live-probe.md witness=none:unattended-worker-session
//
// The declared `location` selector is validated by resolveV2Location (the
// v2 deepObject spelling, handleFSFind convention). The declared optional
// `path` selects "one directory relative to the requested location"; absent or
// empty it is the location itself. The 200 payload is a REAL os.ReadDir of that
// directory — this runtime runs on the same host filesystem as its workspaces,
// so the honest answer is the directory's actual direct children, one declared
// FileSystemEntry {path, type} per entry, with no field invented and no entry
// fabricated. Paths are location-relative (the fs.find convention, so a list
// result feeds fs.read directly) and directories carry the family's
// trailing-slash spelling. An empty directory answers `[]` — never null (the
// document declares an array); ReadDir already returns entries sorted by name,
// so repeated requests are deterministic.
func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}

	location, ok := s.resolveV2Location(w, r)
	if !ok {
		return
	}

	target := location.directory
	relative := ""
	if values, present := r.URL.Query()["path"]; present {
		if len(values) == 0 || strings.TrimSpace(values[0]) == "" {
			writeV2InvalidRequest(w, "Query", "path", `query parameter "path" must not be blank`)
			return
		}
		resolved, rel, err := resolveWorkspacePath(location.directory, values[0])
		if err != nil {
			writeV2InvalidRequest(w, "Query", "path", fmt.Sprintf("query parameter %q is invalid: %v", "path", err))
			return
		}
		info, err := os.Stat(resolved)
		if err != nil {
			writeV2InvalidRequest(w, "Query", "path",
				fmt.Sprintf("path %q cannot be listed: %v", resolved, err))
			return
		}
		if !info.IsDir() {
			writeV2InvalidRequest(w, "Query", "path",
				fmt.Sprintf("path %q is not a directory", resolved))
			return
		}
		if err := ensureNoSymlinkEscape(location.directory, resolved); err != nil {
			writeV2InvalidRequest(w, "Query", "path", err.Error())
			return
		}
		target, relative = resolved, rel
		if relative == "." {
			relative = ""
		}
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		writeV2InvalidRequest(w, "Query", "path",
			fmt.Sprintf("directory %q cannot be listed: %v", target, err))
		return
	}

	data := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		entryPath := entry.Name()
		if relative != "" {
			entryPath = relative + "/" + entryPath
		}
		entryType := "file"
		if entry.IsDir() {
			entryType = "directory"
			entryPath += "/"
		}
		data = append(data, map[string]any{
			"path": entryPath,
			"type": entryType,
		})
	}

	writeJSON(w, map[string]any{
		"location": location.locationInfo(),
		"data":     data,
	})
}

// handleFSRead serves GET /api/fs/read/* — upstream v2.fs.read (ROUTE-ADD-011,
// source item SHIM-DRIFT-008 of
// docs/reports/shim-source-002-baseline-diff-2026-09-29.md; declared
// responses: 200 application/octet-stream binary string, 400
// InvalidRequestError, 401 UnauthorizedError). Before this handler the path had
// no route at all and net/http answered the default 404 (the artifact's
// NOT-SERVED class).
//
// ch:trace row=ROUTE-ADD-011 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.fs.read test=internal/shim/opencode/fs_read_test.go evidence=docs/evidence/ROUTE-ADD-011-live-probe.md witness=none:unattended-worker-session
//
// The file is addressed by the URL suffix after /api/fs/read/ ("Serve one file
// relative to the requested location") and the declared `location` selector —
// resolved by resolveV2Location — is the workspace it must stay inside. The 200
// body is the file's REAL bytes served as application/octet-stream, the
// declared media type; the shim does not wrap a binary read in a JSON envelope
// (the declared schema is a binary string, not a JSON object) and does not
// decode or re-encode the content.
//
// Path safety is the point of the `/api/fs/read/*` surface: a path that escapes
// the workspace root is NEVER served. resolveWorkspacePath refuses blank,
// absolute and parent-referencing paths lexically and re-checks containment
// after joining; ensureNoSymlinkEscape then refuses a path whose symlink-
// resolved location leaves the root. Every refusal — traversal, empty suffix,
// absent file, a directory, an unreadable file — is answered with the declared
// InvalidRequestError 400, because the operation declares no other error and
// this shim never answers an undeclared status.
func (s *Server) handleFSRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}

	location, ok := s.resolveV2Location(w, r)
	if !ok {
		return
	}

	requested := strings.TrimPrefix(r.URL.Path, fsReadRoutePrefix)
	if requested == r.URL.Path {
		// Reached without the declared sub-path (a bare /api/fs/read hit the
		// ServeMux subtree root, or a test dispatched the handler directly):
		// there is no file to read, so this is malformed input.
		writeV2InvalidRequest(w, "Path", "",
			"the v2.fs.read path carries the file to serve after "+fsReadRoutePrefix)
		return
	}

	absolute, _, err := resolveWorkspacePath(location.directory, requested)
	if err != nil {
		writeV2InvalidRequest(w, "Path", "", err.Error())
		return
	}

	info, err := os.Stat(absolute)
	if err != nil {
		writeV2InvalidRequest(w, "Path", "",
			fmt.Sprintf("file %q cannot be read: %v", absolute, err))
		return
	}
	if info.IsDir() {
		writeV2InvalidRequest(w, "Path", "",
			fmt.Sprintf("path %q is a directory, not a file", absolute))
		return
	}
	if err := ensureNoSymlinkEscape(location.directory, absolute); err != nil {
		writeV2InvalidRequest(w, "Path", "", err.Error())
		return
	}

	data, err := os.ReadFile(absolute)
	if err != nil {
		writeV2InvalidRequest(w, "Path", "",
			fmt.Sprintf("file %q cannot be read: %v", absolute, err))
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
