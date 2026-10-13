package opencode

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type fsFindCandidate struct {
	path  string
	kind  string
	rank  int
	label string
}

// handleFSFind serves GET /api/fs/find — upstream v2.fs.find
// (ROUTE-ADD-010, source item SHIM-DRIFT-006, declared responses: 200, 400,
// 401). It walks the selected workspace, ranks case-insensitive fuzzy path
// matches, and returns workspace-relative FileSystemEntry objects. The
// location selector uses the v2 SDK's deepObject query spelling
// location[directory]/location[workspace]; x-opencode-directory and the
// configured workdir remain the fixed-workspace fallbacks.
//
// ch:trace row=ROUTE-ADD-010 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.fs.find test=internal/shim/opencode/fs_find_test.go evidence=docs/evidence/ROUTE-ADD-010-live-probe.md witness=none:unattended-worker-session
func (s *Server) handleFSFind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}

	q := r.URL.Query()
	for key := range q {
		if strings.HasPrefix(key, "location[") && key != "location[directory]" && key != "location[workspace]" {
			writeV2InvalidRequest(w, "Query", key, fmt.Sprintf("unknown location field %q", key))
			return
		}
	}
	for _, key := range []string{"location[directory]", "location[workspace]"} {
		if values, ok := q[key]; ok && (len(values) == 0 || strings.TrimSpace(values[0]) == "") {
			writeV2InvalidRequest(w, "Query", key, fmt.Sprintf("query parameter %q must not be blank", key))
			return
		}
	}

	query := strings.TrimSpace(q.Get("query"))
	if query == "" {
		writeV2InvalidRequest(w, "Query", "query", "?query= is required")
		return
	}

	entryType := q.Get("type")
	if entryType != "" && entryType != "file" && entryType != "directory" {
		writeV2InvalidRequest(w, "Query", "type", "?type= must be file or directory")
		return
	}

	limit := 50
	if raw := q.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeV2InvalidRequest(w, "Query", "limit", "?limit= must be a positive integer")
			return
		}
		limit = parsed
	}

	workspaceID := strings.TrimSpace(q.Get("location[workspace]"))
	if workspaceID != "" && !strings.HasPrefix(workspaceID, "wrk") {
		writeV2InvalidRequest(w, "Query", "location[workspace]", "location[workspace] must start with wrk")
		return
	}

	root := strings.TrimSpace(q.Get("location[directory]"))
	if root == "" {
		root = s.requestWorkspaceDir(r)
	}
	absoluteRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		writeV2InvalidRequest(w, "Query", "location[directory]", "location directory is invalid")
		return
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		writeV2InvalidRequest(w, "Query", "location[directory]",
			fmt.Sprintf("location directory %q cannot be read: %v", absoluteRoot, err))
		return
	}
	if !info.IsDir() {
		writeV2InvalidRequest(w, "Query", "location[directory]",
			fmt.Sprintf("location directory %q is not a directory", absoluteRoot))
		return
	}

	candidates := make([]fsFindCandidate, 0)
	err = filepath.WalkDir(absoluteRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := r.Context().Err(); err != nil {
			return err
		}
		if path == absoluteRoot {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return fs.SkipDir
		}
		kind := "file"
		if entry.IsDir() {
			kind = "directory"
		}
		if entryType != "" && entryType != kind {
			return nil
		}
		relative, relErr := filepath.Rel(absoluteRoot, path)
		if relErr != nil {
			return relErr
		}
		relative = filepath.ToSlash(relative)
		if kind == "directory" {
			relative += "/"
		}
		rank, matched := rankFSFindMatch(query, relative)
		if !matched {
			return nil
		}
		candidates = append(candidates, fsFindCandidate{
			path:  relative,
			kind:  kind,
			rank:  rank,
			label: strings.TrimSuffix(filepath.Base(filepath.Clean(relative)), string(filepath.Separator)),
		})
		return nil
	})
	if err != nil {
		writeV2InvalidRequest(w, "Query", "location[directory]",
			fmt.Sprintf("location directory %q cannot be searched: %v", absoluteRoot, err))
		return
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].rank != candidates[j].rank {
			return candidates[i].rank < candidates[j].rank
		}
		if len(candidates[i].label) != len(candidates[j].label) {
			return len(candidates[i].label) < len(candidates[j].label)
		}
		return candidates[i].path < candidates[j].path
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	entries := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		entries = append(entries, map[string]any{
			"path": candidate.path,
			"type": candidate.kind,
		})
	}
	location := map[string]any{
		"directory": absoluteRoot,
		"project": map[string]any{
			"id":        instanceID(absoluteRoot),
			"directory": absoluteRoot,
		},
	}
	if workspaceID != "" {
		location["workspaceID"] = workspaceID
	}
	writeJSON(w, map[string]any{
		"location": location,
		"data":     entries,
	})
}

// rankFSFindMatch provides deterministic fuzzy ranking without fabricating an
// index: basename matches rank ahead of full-path matches, and contiguous
// matches rank ahead of subsequences. Lower scores are better.
func rankFSFindMatch(query, candidate string) (int, bool) {
	needle := strings.ToLower(strings.TrimSpace(query))
	target := strings.ToLower(strings.TrimSuffix(filepath.ToSlash(candidate), "/"))
	base := strings.ToLower(filepath.Base(filepath.Clean(target)))
	if base == needle {
		return 0, true
	}
	if strings.HasPrefix(base, needle) {
		return 1, true
	}
	if idx := strings.Index(base, needle); idx >= 0 {
		return 2_000 + idx, true
	}
	if strings.HasPrefix(target, needle) {
		return 3_000, true
	}
	if idx := strings.Index(target, needle); idx >= 0 {
		return 4_000 + idx, true
	}
	gap, ok := fsFindSubsequenceGap(needle, target)
	if !ok {
		return 0, false
	}
	return 5_000 + gap, true
}

func fsFindSubsequenceGap(needle, target string) (int, bool) {
	needleRunes := []rune(needle)
	if len(needleRunes) == 0 {
		return 0, true
	}
	targetRunes := []rune(target)
	matched := 0
	first := -1
	last := -1
	for idx, char := range targetRunes {
		if matched >= len(needleRunes) || char != needleRunes[matched] {
			continue
		}
		if first < 0 {
			first = idx
		}
		last = idx
		matched++
	}
	if matched != len(needleRunes) {
		return 0, false
	}
	return last - first - len(needleRunes) + 1, true
}

func writeV2InvalidRequest(w http.ResponseWriter, kind, field, message string) {
	body := map[string]any{
		"_tag":    "InvalidRequestError",
		"message": message,
	}
	if kind != "" {
		body["kind"] = kind
	}
	if field != "" {
		body["field"] = field
	}
	writeJSONStatus(w, http.StatusBadRequest, body)
}

func writeV2Unauthorized(w http.ResponseWriter, message string) {
	writeJSONStatus(w, http.StatusUnauthorized, map[string]any{
		"_tag":    "UnauthorizedError",
		"message": message,
	})
}
