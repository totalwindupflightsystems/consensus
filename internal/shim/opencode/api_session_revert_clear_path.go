package opencode

// isApiSessionRevertClearPath admits exactly the declared concrete route shape
// to the v2 UnauthorizedError envelope.
func isApiSessionRevertClearPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	return ok && sessionID != "" && tail == "revert/clear"
}
