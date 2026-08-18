// Package consensus is the module-root build-info holder.
//
// It exists for one reason: go:embed patterns cannot cross package
// directories, so only a package in this directory can embed the
// repo-root VERSION file. This mirrors the specs/ package pattern
// (specs/embed.go, DOGFOOD-103): a tiny root-level package acting as
// the single compile-time holder for an otherwise non-Go artifact.
package consensus

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var versionFile string

// Version is the build version reported by `consensus --version` and the
// /api/v1/health endpoint. It defaults to the repo-root VERSION file
// content (whitespace-trimmed) so plain `go build` output matches the
// documented version (C-GAP-027). Overridable at build time via
// -ldflags -X (e.g. -X github.com/wojons/consensus/internal/cli.version=…).
var Version = strings.TrimSpace(versionFile)
