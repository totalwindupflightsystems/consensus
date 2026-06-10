/**
 * ShellOps Plugin — Production Ops Domain Layer for Axiom.
 *
 * Provides ops-specific tools that integrate with the ShellOps Go backend daemon:
 * - Action classification enforcement (L1 safety boundary)
 * - Terminal management (tmux abstraction)
 * - Log intelligence (file watching + reactive grep)
 * - Broadcast (outbound message delivery with content scanning)
 *
 * The plugin communicates with the ShellOps daemon via localhost HTTP.
 * If the daemon is not running, tools that require it return clear errors
 * instructing the agent to start it first.
 *
 * Plugin load order: Axiom base → Graph Harness → Context Stash → Conductor → ShellOps
 *
 * axiom:trace work_item=shellops-01 spec=specs/115-ShellOps-Architecture.md plan=phase-1/task-1-3/step-1-3-1
 */

import { tool } from "@opencode-ai/plugin";
import { pluginWarn } from "./config-utils.ts";

// ─────────────────────────────────────────────────────────────────────────────
// Export Safety Note (REF: .opencode/plugins/_prompt.md §2.1)
// ─────────────────────────────────────────────────────────────────────────────
// The 26 tool constants below (shellops_exec, shellops_status, etc.) are
// exported with `export const` to enable direct import in tests
// (see .opencode/tests/shellops-integration.test.ts IT-5b).
//
// This is safe because OpenCode's getLegacyPlugins() iterates Object.values(module)
// and calls only function-typed values as plugin factories. The tool constants
// are objects (created by tool()), not functions, so they are NOT called as
// factories and do NOT cause double-registration.
//
// DO NOT remove export keywords from tool constants — tests depend on them.
// DO NOT add export to raw objects/arrays — only tool({...}) results are safe to export.
// ─────────────────────────────────────────────────────────────────────────────

// ─────────────────────────────────────────────────────────────────────────────
// Configuration
// ─────────────────────────────────────────────────────────────────────────────

const DEFAULT_DAEMON_PORT = 9876;
const DAEMON_BASE_URL = `http://127.0.0.1:${process.env.SHELLOPS_PORT || DEFAULT_DAEMON_PORT}`;
const REQUEST_TIMEOUT_MS = 10_000;

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

async function daemonRequest(
  method: "GET" | "POST",
  path: string,
  body?: unknown,
): Promise<{ ok: boolean; status: number; data: unknown }> {
  const url = `${DAEMON_BASE_URL}${path}`;
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);

  try {
    const response = await fetch(url, {
      method,
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body: body ? JSON.stringify(body) : undefined,
      signal: controller.signal,
    });

    // axiom:trace work_item=plugin-live-test-findings-01 spec=specs/117-ShellOps-Log-Intelligence.md plan=phase-2/task-2-1/step-2-1-1
    // Bug fix: check Content-Type before calling .json(). Go's http.Error() returns plain
    // text (no Content-Type: application/json), so calling .json() on those responses
    // caused Bun to throw "Failed to parse JSON", masking the real error message.
    const contentType = response.headers.get("Content-Type") ?? "";
    let data: unknown;
    if (contentType.includes("application/json")) {
      data = await response.json();
    } else {
      const text = (await response.text()).trim();
      // Attempt JSON parse as fallback (some responses may omit Content-Type header).
      try {
        data = JSON.parse(text);
      } catch {
        // Plain text error from daemon (e.g. http.Error responses) — surface as structured error.
        data = { error: text || `HTTP ${response.status}` };
      }
    }
    return { ok: response.ok, status: response.status, data };
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    if (msg.includes("ECONNREFUSED") || msg.includes("fetch failed")) {
      return {
        ok: false,
        status: 0,
        data: {
          error: "ShellOps daemon not running. Start it with: shellops start (or: cd shellops && go run ./cmd/shellops/)",
        },
      };
    }
    return { ok: false, status: 0, data: { error: msg } };
  } finally {
    clearTimeout(timeout);
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_exec
// Primary "filtered shell" tool — classify then execute.
// axiom:trace work_item=shellops-01 spec=specs/119-ShellOps-Operating-Modes.md plan=implement-missing-mcp-tools
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_exec = tool({
  name: "shellops_exec",
  description:
    "Execute a shell command after action classification. " +
    "SAFE/CAUTIOUS auto-execute; DANGEROUS requires human approval (returns pending_approval); " +
    "FORBIDDEN is blocked.",
  args: {
    command: tool.schema.string().describe("Shell command to execute"),
    timeout_ms: tool.schema.number().optional().describe("Timeout in milliseconds (default: 30000)"),
  },
  async execute({ command, timeout_ms = 30000 }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/exec", {
      command,
      timeout_ms,
    });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_status
// axiom:trace work_item=shellops-01 spec=specs/119-ShellOps-Operating-Modes.md plan=implement-missing-mcp-tools
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_status = tool({
  name: "shellops_status",
  description:
    "Get ShellOps daemon status including connected subsystems and metrics.",
  args: {},
  async execute() {
    const { ok, data } = await daemonRequest("GET", "/api/v1/status");
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_classify
// axiom:trace work_item=shellops-01 spec=specs/118-ShellOps-Action-Classification.md plan=phase-1/task-1-3/step-1-3-2
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_classify = tool({
  name: "shellops_classify",
  description:
    "Classify a shell command according to ShellOps action classification rules. " +
    "Returns SAFE, CAUTIOUS, DANGEROUS, or FORBIDDEN with reasoning. " +
    "Use this to check what classification a command would get before executing it.",
  args: {
    command: tool.schema.string().describe("The shell command to classify"),
  },
  async execute({ command }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/classify", {
      command,
    });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_terminal_create
// axiom:trace work_item=shellops-01 spec=specs/116-ShellOps-Terminal-Management.md plan=phase-2/task-2-2/step-2-2-1
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_terminal_create = tool({
  name: "shellops_terminal_create",
  description:
    "Create a managed terminal session (tmux). Returns the session ID " +
    "to use with other terminal tools. Sessions persist across agent restarts.",
  args: {
    name: tool.schema.string().optional().describe("Human-readable name for the session (optional)"),
  },
  async execute({ name }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/terminal/create", {
      name: name || "",
    });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_terminal_run
// axiom:trace work_item=shellops-zod-migration-01 spec=specs/116-ShellOps-Terminal-Management.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_terminal_run = tool({
  name: "shellops_terminal_run",
  description:
    "Execute a command in a managed terminal session. The command is sent to the " +
    "tmux session and executed. Use shellops_terminal_read to capture output afterward.",
  args: {
    session_id: tool.schema.string().describe("The session ID from shellops_terminal_create"),
    command: tool.schema.string().describe("Command to execute in the terminal"),
  },
  async execute({ session_id, command }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/terminal/send", {
      session_id,
      command,
    });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_terminal_capture
// axiom:trace work_item=shellops-zod-migration-01 spec=specs/116-ShellOps-Terminal-Management.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_terminal_capture = tool({
  name: "shellops_terminal_capture",
  description:
    "Capture the current visible output from a terminal session. " +
    "Returns the last N lines of terminal content.",
  args: {
    session_id: tool.schema.string().describe("The session ID to read from"),
    lines: tool.schema.number().optional().describe("Number of lines to capture (default: 100)"),
  },
  async execute({ session_id, lines }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/terminal/read", {
      session_id,
      lines: lines || 100,
    });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_terminal_list
// axiom:trace work_item=shellops-zod-migration-01 spec=specs/116-ShellOps-Terminal-Management.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_terminal_list = tool({
  name: "shellops_terminal_list",
  description: "List all active managed terminal sessions with metadata.",
  args: {},
  async execute() {
    const { ok, data } = await daemonRequest("GET", "/api/v1/terminal/list");
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_terminal_destroy
// axiom:trace work_item=shellops-zod-migration-01 spec=specs/116-ShellOps-Terminal-Management.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_terminal_destroy = tool({
  name: "shellops_terminal_destroy",
  description: "Tear down a terminal session and release its resources.",
  args: {
    session_id: tool.schema.string().describe("The session ID to destroy"),
  },
  async execute({ session_id }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/terminal/kill", {
      session_id,
    });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_watch_start
// axiom:trace work_item=shellops-01 spec=specs/117-ShellOps-Log-Intelligence.md plan=phase-3/task-3-2/step-3-2-1
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_watch_start = tool({
  name: "shellops_watch_start",
  description:
    "Start watching a file for a regex pattern (reactive grep). Matches are " +
    "stored and queryable. Watches persist until explicitly stopped.",
  args: {
    id: tool.schema.string().describe("Unique watch identifier"),
    file_path: tool.schema.string().describe("Absolute path to the file to watch"),
    pattern: tool.schema.string().describe("Regex pattern to match against each line"),
    agent_id: tool.schema.string().optional().describe("Agent creating this watch (for attribution)"),
  },
  async execute({ id, file_path, pattern, agent_id }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/watch/start", {
      id,
      file_path,
      pattern,
      agent_id: agent_id || "",
    });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_watch_query
// axiom:trace work_item=shellops-zod-migration-01 spec=specs/117-ShellOps-Log-Intelligence.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_watch_query = tool({
  name: "shellops_watch_query",
  description:
    "Query watch results: recent matches, count, or time-based filters. " +
    "Returns the most recent matches for the specified watch.",
  args: {
    watch_id: tool.schema.string().optional().describe("Watch ID to query (omit for all watches)"),
  },
  async execute({ watch_id }) {
    const params = watch_id ? `?watch_id=${encodeURIComponent(watch_id)}` : "";
    const { ok, data } = await daemonRequest("GET", `/api/v1/watch/query${params}`);
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_watch_list
// axiom:trace work_item=shellops-zod-migration-01 spec=specs/117-ShellOps-Log-Intelligence.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_watch_list = tool({
  name: "shellops_watch_list",
  description:
    "List all active file watches with match counts and last-match timestamps.",
  args: {},
  async execute() {
    const { ok, data } = await daemonRequest("GET", "/api/v1/watch/list");
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_watch_stop
// axiom:trace work_item=plugin-live-test-findings-01 spec=specs/117-ShellOps-Log-Intelligence.md plan=phase-2/task-2-1/step-2-1-1
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_watch_stop = tool({
  name: "shellops_watch_stop",
  description:
    "Stop an active file watch. Pass the watch_id returned by shellops_watch_start " +
    "(e.g. 'watch-43e60b98'). The id/name you provided at creation time is a label " +
    "only — use the watch_id field from the start response, not the creation-time id.",
  args: {
    id: tool.schema.string().describe(
      "The watch_id returned by shellops_watch_start (e.g. 'watch-43e60b98'), " +
      "NOT the creation-time id/name parameter.",
    ),
  },
  async execute({ id }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/watch/stop", { id });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tool: shellops_health
// axiom:trace work_item=shellops-zod-migration-01 spec=specs/115-ShellOps-Architecture.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_health = tool({
  name: "shellops_health",
  description:
    "Check ShellOps daemon health. Returns running status, uptime, and " +
    "connected subsystems. Use to verify the daemon is available before " +
    "other shellops_* tool calls.",
  args: {},
  async execute() {
    const { ok, data } = await daemonRequest("GET", "/health");
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tools: Log Intelligence (spec/117 §4)
// axiom:trace work_item=shellops-01 spec=specs/117-ShellOps-Log-Intelligence.md plan=implement-missing-mcp-tools
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_logs_query = tool({
  name: "shellops_logs_query",
  description:
    "Query log matches from a file watch. Returns recent matches with UUID framing " +
    "for injection-safe display per spec/117 §9.",
  args: {
    watch_id: tool.schema.string().describe("Watch ID to query"),
    limit: tool.schema.number().optional().describe("Max results to return (default: 20)"),
  },
  async execute({ watch_id, limit }: { watch_id: string; limit?: number }) {
    const params = `?watch_id=${encodeURIComponent(watch_id)}${limit ? `&limit=${limit}` : ""}`;
    const { ok, data } = await daemonRequest("GET", `/api/v1/watch/query${params}`);
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// axiom:trace work_item=shellops-zod-migration-01 spec=specs/117-ShellOps-Log-Intelligence.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
export const shellops_logs_similar = tool({
  name: "shellops_logs_similar",
  description: "List all watches (similarity analysis is v1.1; returns full watch list as context).",
  args: {
    reference_watch_id: tool.schema.string().describe("Reference watch ID for similarity context"),
  },
  async execute({ reference_watch_id }: { reference_watch_id: string }) {
    const { ok, data } = await daemonRequest("GET", "/api/v1/watch/list");
    if (!ok) return JSON.stringify(data);
    return JSON.stringify({ reference: reference_watch_id, watches: (data as any).watches ?? [] }, null, 2);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tools: Sensory Events (spec/120 §2)
// axiom:trace work_item=shellops-01 spec=specs/120-ShellOps-Sensory-Model.md plan=implement-missing-mcp-tools
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_events_query = tool({
  name: "shellops_events_query",
  description: "Query recent sensory events (K8s, webhook, system).",
  args: {
    limit: tool.schema.number().optional().describe("Max events to return"),
  },
  async execute(_params: { limit?: number }) {
    const { ok, data } = await daemonRequest("GET", "/api/v1/events/query");
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// axiom:trace work_item=shellops-zod-migration-01 spec=specs/116-ShellOps.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
export const shellops_events_listen = tool({
  name: "shellops_events_listen",
  description: "Register a new event listener (webhook or source). Returns listener status.",
  // Decision (BL-007): data is a pass-through payload to the daemon for configuring event sources
  // (webhook URL, k8s namespace, etc.). Fields vary by source type and are not pre-specified.
  // Using JSON-encoded string (Option B) to eliminate unknown() and pass Bedrock JSON Schema
  // validation, while keeping the open-ended config contract. The execute body parses the
  // JSON string before forwarding to the daemon.
  args: {
    source: tool.schema.string().describe("Event source type (webhook, k8s)"),
    data: tool.schema.string().optional().describe("JSON-encoded listener configuration (e.g., '{\"url\":\"https://...\"}')"),
  },
  async execute({ source, data }: { source: string; data?: string }) {
    // data: accepts JSON-encoded string (e.g., '{"url":"https://example.com"}')
    // The plugin parses this to an object before forwarding to the daemon.
    // This contract is intentional: Bedrock can validate string type; the daemon receives a parsed object.
    // Invalid JSON is forwarded as-is (best-effort passthrough).
    const parsedData = data ? (() => { try { return JSON.parse(data); } catch { pluginWarn("shellops", `events_listen: data is not valid JSON, forwarding as-is`); return data; } })() : undefined;
    const { ok, result } = (await daemonRequest("POST", "/api/v1/events/listen", { source, data: parsedData })) as any;
    if (!ok) return JSON.stringify(result);
    return JSON.stringify(result, null, 2);
  },
});

// axiom:trace work_item=shellops-zod-migration-01 spec=specs/116-ShellOps.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
export const shellops_events_stop = tool({
  name: "shellops_events_stop",
  description: "Stop an active event listener.",
  args: {
    listener_id: tool.schema.string().describe("The listener ID to stop"),
  },
  async execute({ listener_id }: { listener_id: string }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/events/stop", { listener_id });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tools: Service Profiles (spec/120 §3)
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=shellops-zod-migration-01 spec=specs/120-ShellOps-Sensory-Model.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
export const shellops_profile_load = tool({
  name: "shellops_profile_load",
  description: "Load a service profile into the daemon's profile store.",
  args: {
    service: tool.schema.string().describe("Service name to load profile for"),
  },
  async execute({ service }: { service: string }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/profiles/load", { service });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// axiom:trace work_item=shellops-zod-migration-01 spec=specs/120-ShellOps-Sensory-Model.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
export const shellops_profile_query = tool({
  name: "shellops_profile_query",
  description: "Query a service profile for operational heuristics (safe/dangerous ops, quirks).",
  args: {
    service: tool.schema.string().describe("Service name to query profile for"),
  },
  async execute({ service }: { service: string }) {
    const { ok, data } = await daemonRequest("GET", `/api/v1/profiles/query?service=${encodeURIComponent(service)}`);
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tools: Broadcast, Investigate, Triage (spec/115 §4, spec/120)
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=shellops-zod-migration-01 spec=specs/116-ShellOps.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
export const shellops_broadcast = tool({
  name: "shellops_broadcast",
  description: "Send an outbound broadcast message (Slack, PagerDuty, Jira).",
  args: {
    text: tool.schema.string().describe("Alert text"),
    severity: tool.schema.string().describe("Alert severity (P1, P2, P3, P4)"),
    channel: tool.schema.string().optional().describe("Target channel (slack, pagerduty, jira)"),
  },
  async execute({ text, severity, channel }: { text: string; severity: string; channel?: string }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/broadcast", { text, severity, channel });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data);
  },
});

// axiom:trace work_item=shellops-zod-migration-01 spec=specs/120-ShellOps-Sensory-Model.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
export const shellops_investigate = tool({
  name: "shellops_investigate",
  description:
    "Start an incident investigation for a service. Gathers logs, events, and service profile " +
    "to build an investigation context.",
  args: {
    service: tool.schema.string().describe("Service to investigate"),
    symptom: tool.schema.string().describe("Observed symptom or error message"),
    timebox_minutes: tool.schema.number().optional().describe("Investigation time box (default: 30)"),
  },
  async execute({ service, symptom, timebox_minutes }: { service: string; symptom: string; timebox_minutes?: number }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/investigate", {
      service,
      symptom,
      timebox_minutes: timebox_minutes || 30,
    });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// axiom:trace work_item=shellops-zod-migration-01 spec=specs/120-ShellOps-Sensory-Model.md plan=phase-5/task-5-1/step-fix-triage-signals-schema
export const shellops_triage = tool({
  name: "shellops_triage",
  description:
    "Run a triage assessment to determine incident severity (P1-P4) and recommended response mode. " +
    "Returns severity, mode, score, and reasoning.",
  args: {
    signals: tool.schema.object({
      service_tier: tool.schema.string().optional().describe("Service priority tier (P1-P4)"),
      error_rate: tool.schema.number().optional().describe("Current error rate (0.0-1.0)"),
      error_trend: tool.schema.string().optional().describe("increasing | stable | decreasing"),
      users_affected: tool.schema.number().optional().describe("Estimated number of impacted users"),
      known_pattern: tool.schema.boolean().optional().describe("Matches a known failure pattern"),
      time_of_day: tool.schema.string().optional().describe("peak | off-peak"),
      concurrent_alerts: tool.schema.number().optional().describe("Number of other active alerts"),
    }).describe("Structured operational signals for triage severity assessment"),
  },
  async execute({ signals }) {
    const { ok, data } = await daemonRequest("POST", "/api/v1/triage", { signals });
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Tools: Nohup Tracking (spec/116 §4)
// axiom:trace work_item=shellops-01 spec=specs/116-ShellOps-Terminal-Management.md#4
// ─────────────────────────────────────────────────────────────────────────────

export const shellops_nohup_list = tool({
  name: "shellops_nohup_list",
  description:
    "List tracked detached (nohup) processes. Returns processes started via " +
    "shellops_terminal_run with detach:true. Filter by status (running/completed/failed).",
  args: {
    status: tool.schema.string().optional().describe("Filter by process status (omit for all): running, completed, failed, unknown"),
  },
  async execute({ status }: { status?: string }) {
    const params = status ? `?status=${encodeURIComponent(status)}` : "";
    const { ok, data } = await daemonRequest("GET", `/api/v1/nohup/list${params}`);
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// axiom:trace work_item=shellops-zod-migration-01 spec=specs/116-ShellOps.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
export const shellops_nohup_check = tool({
  name: "shellops_nohup_check",
  description:
    "Check the status of a tracked detached process. Returns current PID status, " +
    "command, start time, and whether the process is still alive.",
  args: {
    id: tool.schema.number().describe("Process tracking ID from shellops_nohup_list"),
  },
  async execute({ id }: { id: number }) {
    const { ok, data } = await daemonRequest("GET", `/api/v1/nohup/check?id=${id}`);
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// axiom:trace work_item=shellops-zod-migration-01 spec=specs/116-ShellOps.md plan=phase-5/task-5-9/step-fix-missing-trace-markers
export const shellops_nohup_output = tool({
  name: "shellops_nohup_output",
  description:
    "Read the tail of a tracked process's nohup output file. " +
    "Returns the last 4KB of output from the nohup.out file.",
  args: {
    id: tool.schema.number().describe("Process tracking ID from shellops_nohup_list"),
  },
  async execute({ id }: { id: number }) {
    const { ok, data } = await daemonRequest("GET", `/api/v1/nohup/output?id=${id}`);
    if (!ok) return JSON.stringify(data);
    return JSON.stringify(data, null, 2);
  },
});

// ─────────────────────────────────────────────────────────────────────────────
// Plugin Factory — ShellOpsPlugin
// Registers OpenCode hooks for tool.execute.before classification gate
// and experimental.chat.system.transform briefing injection.
// axiom:trace work_item=shellops-01 spec=specs/118-ShellOps-Action-Classification.md plan=implement-plugin-hooks
// ─────────────────────────────────────────────────────────────────────────────

export function ShellOpsPlugin(_opts?: Record<string, unknown>) {
  return {
    tool: {
      shellops_exec,
      shellops_status,
      shellops_classify,
      shellops_terminal_create,
      shellops_terminal_run,
      shellops_terminal_capture,
      shellops_terminal_list,
      shellops_terminal_destroy,
      shellops_watch_start,
      shellops_watch_query,
      shellops_watch_list,
      shellops_watch_stop,
      shellops_logs_query,
      shellops_logs_similar,
      shellops_events_listen,
      shellops_events_query,
      shellops_events_stop,
      shellops_nohup_list,
      shellops_nohup_check,
      shellops_nohup_output,
      shellops_profile_load,
      shellops_profile_query,
      shellops_health,
      shellops_investigate,
      shellops_triage,
      shellops_broadcast,
    },
    "tool.execute.before": async (
      _input: { tool?: string; sessionID?: string; callID?: string },
      output: { args?: Record<string, unknown> }
    ) => {
      const command = output?.args?.command as string | undefined;
      if (!command) return; // fail-open: no command to classify

      try {
        const { ok, data } = await daemonRequest("POST", "/api/v1/classify", { command });
        if (!ok) return; // fail-open: daemon unavailable
        if (data?.level === "FORBIDDEN") {
          throw new Error(`Command blocked: FORBIDDEN — ${data.message ?? data.reason ?? "action classification rejected this command"}`);
        }
      } catch (err) {
        if (err instanceof Error && err.message.startsWith("Command blocked:")) throw err;
        // fail-open for connection errors
      }
    },

    "experimental.chat.system.transform": async (systemPrompt: string): Promise<string> => {
      // Inject concise ShellOps status into system prompt (capped ~150 chars).
      // axiom:trace work_item=shellops-01 spec=specs/115-ShellOps-Architecture.md#2.1 plan=fix-system-transform-briefing-truncation
      try {
        const { ok, data } = await daemonRequest("GET", "/api/v1/status");
        if (!ok || !data) return systemPrompt;
        const s = data as { active_watches?: number; active_terminals?: number; environment?: string; panic_mode?: boolean; uptime_seconds?: number };
        const briefing = `\n\n[ShellOps: env=${s.environment ?? "?"}, watches=${s.active_watches ?? 0}, terminals=${s.active_terminals ?? 0}, uptime=${s.uptime_seconds ?? 0}s${s.panic_mode ? ", PANIC_MODE" : ""}]`;
        return systemPrompt + briefing;
      } catch {
        return systemPrompt; // fail-open
      }
    },

    event: async (event: { type: string }) => {
      if (event.type === "session.start") {
        // Check daemon health on session start
        const { ok } = await daemonRequest("GET", "/api/v1/health");
        if (!ok) {
          pluginWarn("shellops", "Daemon not running. Start with: shellops start");
        }
      }
    },

    // axiom:trace work_item=shellops-01 spec=specs/115-ShellOps-Architecture.md#2.1 plan=fix-on-session-idle-hook
    "on_session_idle": async (): Promise<string | undefined> => {
      // Surface pending watch matches and proprioception signals when agent idles.
      try {
        const { ok, data } = await daemonRequest("GET", "/api/v1/watch/list");
        if (!ok) return undefined;
        const watches = (data as any)?.watches as Array<{ id: string; name: string; match_count: number }> | undefined;
        if (!watches || watches.length === 0) return undefined;
        const withMatches = watches.filter((w) => w.match_count > 0);
        if (withMatches.length === 0) return undefined;
        const lines = withMatches.map((w) => `  • Watch "${w.name || w.id}": ${w.match_count} match${w.match_count > 1 ? "es" : ""}`);
        let proprioLine = "";
        try {
          const { ok: pOk, data: pData } = await daemonRequest("GET", "/api/v1/proprio/status");
          if (pOk && pData) {
            const s = pData as { status?: string; stuck_detected?: boolean; budget_remaining_usd?: number };
            if (s.stuck_detected) proprioLine = "\n  ⚠️ Stuck-loop detected — consider changing approach.";
            if (s.budget_remaining_usd !== undefined && s.budget_remaining_usd < 0.5)
              proprioLine += `\n  ⚠️ Budget: $${s.budget_remaining_usd.toFixed(2)} remaining.`;
          }
        } catch { /* proprioception is best-effort */ }
        return `[ShellOps Watch Activity]\n${lines.join("\n")}${proprioLine}\n  → Use shellops_watch_query to retrieve match details.`;
      } catch {
        return undefined; // fail-open
      }
    },
  };
}
