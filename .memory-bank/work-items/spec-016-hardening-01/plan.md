---
work_item_id: spec-016-hardening-01
status: complete
spec: specs/016-cli-interface.md
source_sweep: sweep-019
created: 2026-05-05
completed: 2026-05-06
---

# Plan — SPEC-016 CLI Hardening

Remediate CLI gaps found during idle sweep-019 of `specs/016-cli-interface.md`.

axiom:trace work_item=spec-016-hardening-01 spec=specs/016-cli-interface.md sweep=sweep-019

## Findings Remediated

| ID | Severity | Description | Outcome |
|----|----------|-------------|---------|
| HARDEN-CLI-01 | HIGH | Config file priority chain incomplete; `--config` flag was dead | ✅ Wired `SetConfigPath` → `LoadWithPath` in config.Load(); serve pushes flag |
| HARDEN-CLI-02 | HIGH | Interactive approval mode not implemented | ✅ Already implemented (`approveInteractive` in approve.go) |
| HARDEN-CLI-03 | HIGH | 5 subcommands were non-functional stubs | ✅ ServerFunc/InitFunc wired in main.go; migrate create writes real files; config edit added |
| HARDEN-CLI-04 | HIGH | `status` command bypasses output formatter | ✅ Already uses `fm.Print()` with structured map (table/json/yaml all work) |
| HARDEN-CLI-05 | HIGH | 21 command-specific flags missing | ✅ Added --adapter, --migrations to serve; most other flags already present |
| HARDEN-CLI-06 | MEDIUM | Approve command naming convention mismatch | ✅ Already aligned (approve accept/reject subcommands + top-level reject) |
| HARDEN-CLI-07 | MEDIUM | Migrate bare-command structure mismatch | ✅ Already aliases to migrate run |
| HARDEN-CLI-08 | LOW | Nested config key lookups not supported | ✅ nestedGet already works in config get; config set now uses buildNestedMap |

## Changes Made

1. **internal/config/config.go** — Added `LoadWithPath()`, `resolveConfigPath()` with ~/.conscience/ and /etc/ fallbacks, `SetConfigPath()` for --config flag, `Path()` method
2. **cmd/conscience/main.go** — Wired `cli.InitFunc = runInit`, `cli.ServerFunc = runServer`; added `runInit()` function
3. **internal/cli/serve.go** — Wired --config flag to `config.SetConfigPath()` + env var flag passthrough; added --adapter and --migrations flags
4. **internal/cli/migrate.go** — `migrate create` now actually writes a .sql file to migrations/ instead of printing
5. **internal/cli/config.go** — Added `config edit` command; `config set` now uses `buildNestedMap` for dot-notation; added proper imports

## Verification

- `go build ./...` succeeds ✅
- `go test ./...` passes (23 packages, 0 failures) ✅
- `go test ./internal/cli/...` passes ✅
- `go test ./internal/config/...` passes ✅
