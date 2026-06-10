/**
 * Context Pipeline Hook — OpenCode plugin that runs the 7-stage context
 * pipeline during session lifecycle and emits structured observability events.
 *
 * Hook: experimental.chat.system.transform
 * REQ-HLU-001: runs all 7 pipeline stages as observable events
 * REQ-HLU-002: respects token budget from harness config
 *
 * This is a proper OpenCode plugin (all exports are functions, no top-level
 * side effects, no top-level async). The plugin loader calls each exported
 * function and expects it to return a hook map.
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-2
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-2
 */

import { existsSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import {
  runContextPipeline,
  type ContextItem,
  type PipelineConfig,
} from "./context-pipeline.ts";

// ─────────────────────────────────────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────────────────────────────────────

/** Maximum file size (bytes) to read — skip larger files to respect token budget. */
const MAX_FILE_BYTES = 8192;

/** Default token budget for context packing. Override via AXIOM_CONTEXT_BUDGET. */
const DEFAULT_TOKEN_BUDGET = 4000;

/**
 * Candidate file paths relative to the repo root (directory passed to the plugin).
 * Files that don't exist are silently skipped.
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-2
 */
const CANDIDATE_PATHS = [
  "specs/00-PRD.md",
  "specs/101-Harness-Engineering.md",
  ".memory-bank/_index.md",
  ".memory-bank/work-items/harness-levelup-01/verification.md",
] as const;

// ─────────────────────────────────────────────────────────────────────────────
// Per-session cache: avoids redundant disk reads across multiple LLM calls
// within the same session. Keyed by directory; invalidated when any candidate
// file's mtime changes.
//
// axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-3/step-5-3-1
// ─────────────────────────────────────────────────────────────────────────────

/** Module-level cache: maps directory → { items: ContextItem[], maxMtime: number } */
// _candidateCache: NOT exported as a value — OpenCode's plugin loader
// iterates Object.values(module) and crashes on non-function exports.
// Tests access it via the getter function below.
const _candidateCache = new Map<string, { items: ContextItem[]; maxMtime: number }>();

// Test-only access (must be a function to satisfy plugin loader)
export const _getCandidateCache = () => _candidateCache;

/**
 * Returns the maximum mtime (in milliseconds) across all CANDIDATE_PATHS under
 * the given repoRoot. Missing files are silently skipped (mtime 0 contribution).
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-3/step-5-3-1
 */
function getCandidateMaxMtime(repoRoot: string): number {
  const paths = CANDIDATE_PATHS.map((p) => join(repoRoot, p));
  let maxMtime = 0;
  for (const p of paths) {
    try {
      const stat = statSync(p);
      if (stat.mtimeMs > maxMtime) maxMtime = stat.mtimeMs;
    } catch {
      // file missing — ignore
    }
  }
  return maxMtime;
}

// _candidateCache not exported — plugin loader crashes on non-function exports.
// getCandidateMaxMtime is a function so it's safe to export for tests.
export { getCandidateMaxMtime };

// ─────────────────────────────────────────────────────────────────────────────
// Helper: read candidate files from disk → ContextItem[]
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Reads candidate files from the repo root directory, filtering out files
 * that exceed MAX_FILE_BYTES. Returns a ContextItem[] ready for the pipeline.
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-1/step-5-1-2
 */
export function readCandidateFiles(repoRoot: string): ContextItem[] {
  const items: ContextItem[] = [];

  for (const relPath of CANDIDATE_PATHS) {
    const absPath = join(repoRoot, relPath);

    if (!existsSync(absPath)) {
      continue;
    }

    try {
      const stat = statSync(absPath);
      if (stat.size > MAX_FILE_BYTES) {
        // Skip oversized files — silently. The pipeline event emitter (when
        // AXIOM_CONTEXT_PIPELINE_DEBUG=1) records the final item count;
        // per-file skip messages are noise in normal operation.
        continue;
      }

      const content = readFileSync(absPath, "utf-8");
      const tokenCount = Math.ceil(content.length / 4);

      items.push({
        id: relPath,
        path: relPath,
        content,
        tokenCount,
        // Assign a modest relevance score; all spec/index files are equally relevant
        relevanceScore: 0.8,
      });
    } catch {
      // Non-blocking: skip unreadable files silently.
      // (Read errors during context collection are common — symlinks, permissions,
      // race conditions during git operations. Logging each one floods the UI.)
    }
  }

  return items;
}

/**
 * Format the packed pipeline result into a compact context summary string.
 * Takes the first 200 chars of each item's content and joins with separators.
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-2
 */
export function formatContextSummary(items: ContextItem[]): string {
  if (!items || !Array.isArray(items)) return "";
  return items
    .map((item) => {
      const snippet = item.content.slice(0, 200);
      return `[${item.path}]\n${snippet}`;
    })
    .join("\n---\n");
}

// ─────────────────────────────────────────────────────────────────────────────
// OpenCode Plugin Export
// ─────────────────────────────────────────────────────────────────────────────

/**
 * ContextPipelineHook — OpenCode plugin.
 *
 * Wires the 7-stage context pipeline into the session lifecycle via the
 * `experimental.chat.system.transform` hook. On each LLM call, the plugin:
 *   1. Reads candidate files from the repo root
 *   2. Runs all 7 pipeline stages (collection → evidence_capture)
 *   3. Injects a compact summary of selected items into output.context
 *   4. Emits NDJSON events to stderr (via runContextPipeline)
 *
 * Errors are caught and logged — the pipeline never blocks the session.
 *
 * All exports must be functions (OpenCode plugin loader requirement).
 * No top-level side effects at import time.
 *
 * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-2
 */
export const ContextPipelineHook = async ({ directory, client }: { directory: string; client?: { app?: { log?: (args: { body: { service: string; level: string; message: string; extra?: unknown } }) => Promise<unknown> } } }) => {
  // Structured logger — uses OpenCode's client.app.log instead of stderr.
  // Per https://opencode.ai/docs/plugins/#logging — stderr from plugin hooks
  // is captured by OpenCode's TUI and pollutes the conversation pane.
  const log = async (level: "debug" | "info" | "warn" | "error", message: string, extra?: unknown) => {
    try {
      if (client?.app?.log) {
        await client.app.log({ body: { service: "context-pipeline-hook", level, message, extra } });
      }
      // If client.app.log is unavailable (e.g., tests), drop silently.
    } catch {
      // Log failure must not break the hook
    }
  };

  return {
    // No custom tools — this is a hook-only plugin. tool: {} prevents
    // OpenCode's ToolRegistry from crashing on Object.entries(undefined).
    tool: {},
    /**
     * experimental.chat.system.transform hook.
     * Fires before each LLM call. We read candidates, run pipeline, inject context.
     *
     * axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-001 plan=phase-5/task-5-1/step-5-1-2
     */
    "experimental.chat.system.transform": async (
      _input: unknown,
      output: { context: string[] }
    ) => {
      try {
        // Resolve config from environment — fail safely with defaults
        const workItemId =
          process.env["AXIOM_WORK_ITEM_ID"] ?? "harness-levelup-01";
        const tokenBudget = process.env["AXIOM_CONTEXT_BUDGET"]
          ? parseInt(process.env["AXIOM_CONTEXT_BUDGET"], 10)
          : DEFAULT_TOKEN_BUDGET;

        const config: PipelineConfig = {
          runId: `hook-${Date.now()}`,
          workItemId,
          tokenBudget: isNaN(tokenBudget) ? DEFAULT_TOKEN_BUDGET : tokenBudget,
        };

        // Read candidate files from the repo root — use per-session cache to
        // avoid redundant disk reads when the hook fires multiple times in a session.
        // Cache is invalidated when any candidate file's mtime changes.
        // axiom:trace work_item=harness-levelup-01 spec=specs/101-Harness-Engineering.md#REQ-HLU-002 plan=phase-5/task-5-3/step-5-3-1
        const currentMtime = getCandidateMaxMtime(directory);
        const cached = _candidateCache.get(directory);
        let candidates: ContextItem[];
        if (cached && cached.maxMtime === currentMtime) {
          candidates = cached.items;
        } else {
          candidates = readCandidateFiles(directory);
          _candidateCache.set(directory, { items: candidates, maxMtime: currentMtime });
        }

        // Run all 7 pipeline stages; events are emitted to stderr by runContextPipeline
        const result = runContextPipeline(config, candidates);

        // Inject compact summary into context if items were packed
        if (result.items.length > 0) {
          const summary = formatContextSummary(result.items);
          output.context.push(summary);
        }

        // Log outcome via OpenCode SDK (debug level — visible only with --print-logs)
        await log("debug", "context pipeline completed", {
          items_selected: result.items.length,
          token_budget: config.tokenBudget,
        });
      } catch (err) {
        // Non-blocking: log via SDK and continue — never break the session.
        await log("error", "context pipeline hook error", {
          message: err instanceof Error ? err.message : String(err),
        });
      }
    },
  };
};
