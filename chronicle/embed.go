// Package chronicle provides the Chronicle Investigation Workbench UI.
//
// Serves an embedded dark-theme operational dashboard for AI-powered
// investigations — dense data, transparent reasoning, operator-first workflows.
//
// Design system: specs/026-dashboard-ui.md, DESIGN.md, docs/diagrams.md
package chronicle

import "embed"

//go:embed css/* index.html
var Assets embed.FS
