// axiom:trace work_item=code-intelligence-native-01 spec=specs/70-OpenCode-Plugin.md plan=phase-4/task-4-2/step-4-2-1

/**
 * code-intel OpenCode tool
 *
 * Provides code-intelligence discovery and actions backed by the native
 * axiom-code-intel Go binary. Operations:
 *
 *   status    — index the repo and return file_count, symbol_count, schema_version
 *   query     — index the repo then query by symbol name or path filter
 *   changes   — detect changed-code impact against a git base ref
 *   run_path  — capture run-path report for a command/entry point
 *
 * If the binary and `go run` are both unavailable, returns an "available:false"
 * state with build instructions.
 *
 * Spec: specs/70-OpenCode-Plugin.md
 * AC-11: Plugin action or test returns the same core fields as CLI/API or a
 *        clear unavailable state.
 */

import { tool } from "@opencode-ai/plugin"
import * as fs from "node:fs"
import * as path from "node:path"
import * as os from "node:os"
import { spawnSync } from "node:child_process"

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const BUILD_INSTRUCTIONS =
  "Run: cd code-intel && go build -o ../_tmp/axiom-code-intel ./cmd/axiom-code-intel && export PATH=\"$PATH:$PWD/_tmp\""

/** Relative path of the Go main package from the repo root. */
const GO_PKG = "code-intel/cmd/axiom-code-intel"

/** Well-known binary names to try via PATH. */
const BINARY_NAMES = ["axiom-code-intel"]

// ---------------------------------------------------------------------------
// Binary / go-run discovery helpers
// ---------------------------------------------------------------------------

type RunnerKind = "binary" | "go_run" | "none"

interface Runner {
  kind: RunnerKind
  /** Resolved binary path (for kind=binary) or "go" (for kind=go_run). */
  cmd: string
  /** Extra prefix args (empty for binary; ["run", "./cmd/axiom-code-intel"] for go_run). */
  prefix: string[]
  /**
   * Working directory for the runner subprocess.
   * For kind=go_run this is the code-intel/ module root so `go run` can find go.mod.
   * For kind=binary this is the caller-supplied cwd (repo root or repoRoot).
   */
  cwd?: string
}

/**
 * Attempt to locate axiom-code-intel.
 * Priority: (1) binary on PATH, (2) `go run ./cmd/axiom-code-intel` from code-intel/ module root
 */
function discoverRunner(worktree: string): Runner {
  // Try binary names on PATH first
  for (const name of BINARY_NAMES) {
    const result = spawnSync("which", [name], { encoding: "utf8", timeout: 3000 })
    if (result.status === 0 && result.stdout.trim().length > 0) {
      return { kind: "binary", cmd: result.stdout.trim(), prefix: [] }
    }
  }

  // Axiom convention: pre-built binary at <worktree>/_tmp/axiom-code-intel.
  // The axiom install scripts and developer workflows put the build output here.
  // Checking this means the tool works without requiring users to manually edit PATH.
  for (const name of BINARY_NAMES) {
    const candidate = path.join(worktree, "_tmp", name)
    if (fs.existsSync(candidate)) {
      try {
        const stat = fs.statSync(candidate)
        // Verify it's an executable file (mode bits set on owner/group/other)
        if (stat.isFile() && (stat.mode & 0o111) !== 0) {
          return { kind: "binary", cmd: candidate, prefix: [] }
        }
      } catch {
        // ignore stat errors and fall through to go_run
      }
    }
  }

  // Fall back to `go run ./cmd/axiom-code-intel` from the module root (code-intel/).
  // IMPORTANT: go run requires cwd to be the module root (where go.mod lives).
  const moduleRoot = path.join(worktree, "code-intel")
  const goModPath = path.join(moduleRoot, "go.mod")
  const pkgDir = path.join(moduleRoot, "cmd", "axiom-code-intel")

  if (fs.existsSync(goModPath) && fs.existsSync(pkgDir)) {
    const goResult = spawnSync("go", ["version"], { encoding: "utf8", timeout: 5000, cwd: moduleRoot })
    if (goResult.status === 0) {
      return {
        kind: "go_run",
        cmd: "go",
        prefix: ["run", "./cmd/axiom-code-intel"],
        cwd: moduleRoot,
      }
    }
  }

  return { kind: "none", cmd: "", prefix: [] }
}

/**
 * Run axiom-code-intel with the given args. Returns { ok, stdout, stderr, code }.
 * Uses runner.cwd when set (needed for go_run so go.mod is found), otherwise falls back to cwd param.
 */
function runCLI(
  runner: Runner,
  cliArgs: string[],
  cwd: string,
): { ok: boolean; stdout: string; stderr: string; code: number | null } {
  const args = [...runner.prefix, ...cliArgs]
  const effectiveCwd = runner.cwd ?? cwd
  const result = spawnSync(runner.cmd, args, {
    encoding: "utf8",
    timeout: 60_000,
    cwd: effectiveCwd,
    // Inherit env so PATH, GOPATH, etc. are available
    env: { ...process.env },
  })
  return {
    ok: result.status === 0,
    stdout: result.stdout ?? "",
    stderr: result.stderr ?? "",
    code: result.status,
  }
}

/**
 * Parse the first JSON object or array from a string.
 * The CLI may emit log lines before/after JSON when using `go run`.
 */
function parseFirstJSON(text: string): unknown {
  const start = text.search(/[\[{]/)
  if (start === -1) {
    throw new Error(`No JSON found in output: ${text.slice(0, 200)}`)
  }
  return JSON.parse(text.slice(start))
}

// ---------------------------------------------------------------------------
// Tool definition
// ---------------------------------------------------------------------------

export default tool({
  description:
    "Query the native Axiom code-intelligence index for symbol context, changed-code impact, and run-path reports. " +
    "Operations: status (index summary), query (symbol/path search), changes (impact of git diff), run_path (command execution path). " +
    "If the axiom-code-intel binary is not built, returns an unavailable state with build instructions.",
  args: {
    operation: tool.schema
      .enum(["status", "query", "changes", "run_path"])
      .describe(
        "status: index summary (file_count, symbol_count). " +
          "query: search symbols by name or path. " +
          "changes: detect impacted symbols from git diff. " +
          "run_path: capture run-path for a command/entry point.",
      ),
    symbol: tool.schema
      .string()
      .optional()
      .describe("Symbol name filter (for query operation)"),
    path: tool.schema
      .string()
      .optional()
      .describe("Repo-relative path filter (for query operation)"),
    base: tool.schema
      .string()
      .optional()
      .describe("Git base ref to diff against (for changes operation, default: HEAD)"),
    expand_imports: tool.schema
      .boolean()
      .optional()
      .describe(
        "Expand impact via import graph for low-confidence import_dependent symbols (for changes operation)",
      ),
    entry: tool.schema
      .string()
      .optional()
      .describe("Command or test name to look up (required for run_path operation)"),
    repo: tool.schema
      .string()
      .optional()
      .describe("Repo root path (default: worktree root)"),
  },

  async execute(args, context) {
    const repoRoot = args.repo
      ? path.resolve(context.worktree, args.repo)
      : context.worktree

    // Discover runner
    const runner = discoverRunner(context.worktree)

    if (runner.kind === "none") {
      return JSON.stringify({
        available: false,
        reason:
          "axiom-code-intel binary not found on PATH and `go run` unavailable or package directory missing. " +
          "Go 1.22+ is required. Install from https://go.dev/dl/ then run: cd code-intel && go build -o ../_tmp/axiom-code-intel ./cmd/axiom-code-intel",
        instructions: BUILD_INSTRUCTIONS,
        go_pkg: GO_PKG,
        worktree: context.worktree,
      })
    }

    // Route to operation-specific CLI call
    switch (args.operation) {
      case "status":
        return handleStatus(runner, repoRoot, context.worktree)

      case "query":
        return handleQuery(runner, repoRoot, context.worktree, args.symbol, args.path)

      case "changes":
        return handleChanges(
          runner,
          repoRoot,
          context.worktree,
          args.base ?? "HEAD",
          args.expand_imports ?? false,
        )

      case "run_path": {
        if (!args.entry || args.entry.trim().length === 0) {
          return JSON.stringify({
            available: true,
            error: "entry parameter is required for run_path operation",
          })
        }
        return handleRunPath(runner, repoRoot, context.worktree, args.entry)
      }

      default:
        return JSON.stringify({
          available: true,
          error: `Unknown operation: ${args.operation}`,
        })
    }
  },
})

// ---------------------------------------------------------------------------
// Operation handlers
// ---------------------------------------------------------------------------

/**
 * status: run `index --repo <repo>` and return summary.
 * Returns: { available, schema_version, file_count, symbol_count, generated_at, limitations }
 */
async function handleStatus(runner: Runner, repoRoot: string, cwd: string): Promise<string> {
  const result = runCLI(runner, ["index", "--repo", repoRoot], cwd)

  if (!result.ok) {
    return JSON.stringify({
      available: true,
      operation: "status",
      success: false,
      error: `index command failed (exit ${result.code})`,
      stderr: result.stderr.slice(0, 500),
    })
  }

  try {
    const idx = parseFirstJSON(result.stdout) as {
      schema_version?: string
      files?: unknown[]
      symbols?: unknown[]
      generated_at?: string
      limitations?: string[]
    }
    return JSON.stringify({
      available: true,
      operation: "status",
      success: true,
      schema_version: idx.schema_version ?? "",
      file_count: Array.isArray(idx.files) ? idx.files.length : 0,
      symbol_count: Array.isArray(idx.symbols) ? idx.symbols.length : 0,
      generated_at: idx.generated_at ?? "",
      limitations: idx.limitations ?? [],
    })
  } catch (err) {
    return JSON.stringify({
      available: true,
      operation: "status",
      success: false,
      error: `Failed to parse index JSON: ${(err as Error).message}`,
      raw_output: result.stdout.slice(0, 500),
    })
  }
}

/**
 * query: build an index into a temp file, then query it.
 * Returns: { available, schema_version, matches, limitations }
 */
async function handleQuery(
  runner: Runner,
  repoRoot: string,
  cwd: string,
  symbol?: string,
  filterPath?: string,
): Promise<string> {
  // Step 1: build index to a temp file
  const tmpDir = os.tmpdir()
  const tmpIndex = path.join(tmpDir, `axiom-code-intel-${Date.now()}.json`)

  const idxResult = runCLI(
    runner,
    ["index", "--repo", repoRoot, "--out", tmpIndex],
    cwd,
  )

  if (!idxResult.ok) {
    return JSON.stringify({
      available: true,
      operation: "query",
      success: false,
      error: `index command failed (exit ${idxResult.code})`,
      stderr: idxResult.stderr.slice(0, 500),
    })
  }

  // Verify index file was actually written
  if (!fs.existsSync(tmpIndex)) {
    return JSON.stringify({
      available: true,
      operation: "query",
      success: false,
      error: "Index file was not created by the index command",
    })
  }

  try {
    // Step 2: run query
    const queryArgs = ["query", "--index", tmpIndex]
    if (symbol && symbol.trim().length > 0) {
      queryArgs.push("--symbol", symbol)
    }
    if (filterPath && filterPath.trim().length > 0) {
      queryArgs.push("--path", filterPath)
    }

    const qResult = runCLI(runner, queryArgs, cwd)

    if (!qResult.ok) {
      return JSON.stringify({
        available: true,
        operation: "query",
        success: false,
        error: `query command failed (exit ${qResult.code})`,
        stderr: qResult.stderr.slice(0, 500),
      })
    }

    const qData = parseFirstJSON(qResult.stdout) as {
      schema_version?: string
      matches?: unknown[]
      limitations?: string[]
    }
    return JSON.stringify({
      available: true,
      operation: "query",
      success: true,
      schema_version: qData.schema_version ?? "",
      matches: qData.matches ?? [],
      match_count: Array.isArray(qData.matches) ? qData.matches.length : 0,
      limitations: qData.limitations ?? [],
    })
  } finally {
    // Clean up temp index
    try {
      fs.unlinkSync(tmpIndex)
    } catch {
      // Best-effort cleanup; ignore errors
    }
  }
}

/**
 * changes: run `detect-changes --repo <repo> --base <base>`.
 * Returns: { available, schema_version, changed_files, affected_symbols, freshness, limitations }
 */
async function handleChanges(
  runner: Runner,
  repoRoot: string,
  cwd: string,
  base: string,
  expandImports: boolean,
): Promise<string> {
  const cliArgs = ["detect-changes", "--repo", repoRoot, "--base", base]
  if (expandImports) {
    cliArgs.push("--expand-imports")
  }

  const result = runCLI(runner, cliArgs, cwd)

  if (!result.ok) {
    return JSON.stringify({
      available: true,
      operation: "changes",
      success: false,
      error: `detect-changes command failed (exit ${result.code})`,
      stderr: result.stderr.slice(0, 500),
    })
  }

  try {
    const cm = parseFirstJSON(result.stdout) as {
      schema_version?: string
      changed_files?: unknown[]
      affected_symbols?: unknown[]
      freshness?: unknown
      limitations?: string[]
    }
    return JSON.stringify({
      available: true,
      operation: "changes",
      success: true,
      schema_version: cm.schema_version ?? "",
      changed_files: cm.changed_files ?? [],
      changed_file_count: Array.isArray(cm.changed_files) ? cm.changed_files.length : 0,
      affected_symbols: cm.affected_symbols ?? [],
      affected_symbol_count: Array.isArray(cm.affected_symbols)
        ? cm.affected_symbols.length
        : 0,
      freshness: cm.freshness ?? null,
      limitations: cm.limitations ?? [],
    })
  } catch (err) {
    return JSON.stringify({
      available: true,
      operation: "changes",
      success: false,
      error: `Failed to parse detect-changes JSON: ${(err as Error).message}`,
      raw_output: result.stdout.slice(0, 500),
    })
  }
}

/**
 * run_path: run `run-path --repo <repo> --entry <entry>`.
 * Returns: { available, schema_version, entry_point, matched_files, trace_markers, evidence_links, limitations }
 */
async function handleRunPath(
  runner: Runner,
  repoRoot: string,
  cwd: string,
  entry: string,
): Promise<string> {
  const result = runCLI(
    runner,
    ["run-path", "--repo", repoRoot, "--entry", entry],
    cwd,
  )

  if (!result.ok) {
    return JSON.stringify({
      available: true,
      operation: "run_path",
      success: false,
      error: `run-path command failed (exit ${result.code})`,
      stderr: result.stderr.slice(0, 500),
    })
  }

  try {
    const rp = parseFirstJSON(result.stdout) as {
      schema_version?: string
      entry_point?: string
      matched_files?: unknown[]
      trace_markers?: unknown[]
      evidence_links?: unknown[]
      limitations?: string[]
      captured_at?: string
    }
    return JSON.stringify({
      available: true,
      operation: "run_path",
      success: true,
      schema_version: rp.schema_version ?? "",
      entry_point: rp.entry_point ?? entry,
      matched_file_count: Array.isArray(rp.matched_files) ? rp.matched_files.length : 0,
      matched_files: rp.matched_files ?? [],
      trace_marker_count: Array.isArray(rp.trace_markers) ? rp.trace_markers.length : 0,
      trace_markers: rp.trace_markers ?? [],
      evidence_links: rp.evidence_links ?? [],
      limitations: rp.limitations ?? [],
      captured_at: rp.captured_at ?? "",
    })
  } catch (err) {
    return JSON.stringify({
      available: true,
      operation: "run_path",
      success: false,
      error: `Failed to parse run-path JSON: ${(err as Error).message}`,
      raw_output: result.stdout.slice(0, 500),
    })
  }
}
