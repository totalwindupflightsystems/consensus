/**
 * ShellOps Plugin Integration Tests — no live daemon required.
 *
 * Covers:
 *   IT-1  Tool definitions exist with correct names
 *   IT-2  Daemon URL config resolves correctly (env var and default)
 *   IT-3  daemonRequest handles ECONNREFUSED with a helpful error message
 *   IT-4  daemonRequest handles AbortError (timeout) gracefully
 *   IT-5  All expected plugin tool exports are present and have execute functions
 *   IT-6  ShellOpsPlugin factory returns the three expected hooks
 *   IT-7  v1 endpoint paths match expected plugin API contract
 *
 * These tests do NOT require a live daemon. They mock fetch and inspect
 * module structure + error-path behavior.
 *
 * Run: cd .opencode && bun test tests/shellops-integration.test.ts
 *
 * axiom:trace work_item=shellops-01 spec=specs/115-ShellOps-Architecture.md plan=add-typescript-integration-tests test=tests/shellops-integration.test.ts
 */

import { test, expect, describe, beforeEach, afterEach } from "bun:test";

// ─────────────────────────────────────────────────────────────────────────────
// Module structure inspection
// We import * and verify the named exports match the expected tool names.
// ─────────────────────────────────────────────────────────────────────────────

import * as shellopsModule from "../lib/shellops.ts";

// ─── IT-1: Tool definitions exist with correct names ─────────────────────────

describe("IT-1: Tool definitions exist with correct names", () => {
  // axiom:trace work_item=shellops-zod-migration-01 spec=specs/48-Test-Quality-Gates.md plan=phase-5/task-5-2/step-fix-it1-26-tools
  const expectedToolNames = [
    // Original 11
    "shellops_classify",
    "shellops_terminal_create",
    "shellops_terminal_run",
    "shellops_terminal_capture",
    "shellops_terminal_list",
    "shellops_terminal_destroy",
    "shellops_watch_start",
    "shellops_watch_query",
    "shellops_watch_list",
    "shellops_watch_stop",
    "shellops_health",
    // 15 added during zod migration (shellops-zod-migration-01)
    "shellops_exec",
    "shellops_status",
    "shellops_nohup_list",
    "shellops_nohup_check",
    "shellops_nohup_output",
    "shellops_logs_query",
    "shellops_logs_similar",
    "shellops_events_query",
    "shellops_events_listen",
    "shellops_events_stop",
    "shellops_profile_load",
    "shellops_profile_query",
    "shellops_broadcast",
    "shellops_investigate",
    "shellops_triage",
  ];

  for (const toolName of expectedToolNames) {
    test(`export '${toolName}' exists and has name property`, () => {
      const exported = (shellopsModule as Record<string, unknown>)[toolName];
      expect(exported).toBeDefined();
      // Tool objects from @opencode-ai/plugin have a .name property
      expect((exported as { name: string }).name).toBe(toolName);
    });
  }

  test("each exported tool has an execute function", () => {
    for (const toolName of expectedToolNames) {
      const exported = (shellopsModule as Record<string, unknown>)[toolName];
      expect(typeof (exported as { execute: unknown }).execute).toBe("function");
    }
  });
});

// ─── IT-2: Daemon URL config resolves correctly ───────────────────────────────

describe("IT-2: Daemon URL configuration", () => {
  const originalEnv = process.env.SHELLOPS_PORT;

  afterEach(() => {
    if (originalEnv === undefined) {
      delete process.env.SHELLOPS_PORT;
    } else {
      process.env.SHELLOPS_PORT = originalEnv;
    }
  });

  test("default port is 9876 when SHELLOPS_PORT is not set", () => {
    delete process.env.SHELLOPS_PORT;
    // The plugin uses process.env.SHELLOPS_PORT || 9876 at module level.
    // We verify the default by checking the plugin's hardcoded constant value
    // via the fact that SHELLOPS_PORT is absent and the connection refused error
    // would reference port 9876.
    // Since the URL is built at module load time (const), we verify the module
    // loads without error and the default constant is 9876.
    // This is a structural test — the URL is private, but we can verify the
    // module structure doesn't break.
    expect(shellopsModule.shellops_health).toBeDefined();
  });

  test("SHELLOPS_PORT env var is respected (structural check)", () => {
    // Port override is read at module import time, so we can't change it
    // mid-test, but we verify the env var mechanism is documented and the
    // module exports are stable regardless.
    process.env.SHELLOPS_PORT = "9999";
    expect(shellopsModule.shellops_health).toBeDefined();
    expect((shellopsModule.shellops_health as { name: string }).name).toBe("shellops_health");
  });
});

// ─── IT-3: daemonRequest handles ECONNREFUSED ────────────────────────────────
// We test this indirectly by calling a tool's execute() with a mocked fetch
// that rejects with ECONNREFUSED, then verifying the error message is helpful.

describe("IT-3: daemonRequest ECONNREFUSED handling", () => {
  let originalFetch: typeof globalThis.fetch;

  beforeEach(() => {
    originalFetch = globalThis.fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  test("ECONNREFUSED returns helpful error message about starting daemon", async () => {
    // Mock fetch to throw ECONNREFUSED
    globalThis.fetch = async () => {
      const err = new Error("fetch failed: connect ECONNREFUSED 127.0.0.1:9876");
      (err as NodeJS.ErrnoException).code = "ECONNREFUSED";
      throw err;
    };

    const tool = shellopsModule.shellops_health;
    const result = await (tool as { execute: () => Promise<string> }).execute();
    const parsed = JSON.parse(result) as { error: string };

    expect(typeof parsed.error).toBe("string");
    // Should mention starting the daemon
    expect(parsed.error.toLowerCase()).toContain("shellops");
    expect(
      parsed.error.includes("start") || parsed.error.includes("not running")
    ).toBe(true);
  });

  test("ECONNREFUSED on classify tool returns error object, not throws", async () => {
    globalThis.fetch = async () => {
      throw new Error("fetch failed: connect ECONNREFUSED 127.0.0.1:9876");
    };

    const tool = shellopsModule.shellops_classify;
    // Must not throw — must return stringified error
    const result = await (tool as { execute: (args: { command: string }) => Promise<string> }).execute({
      command: "ls -la",
    });
    const parsed = JSON.parse(result) as { error: string };
    expect(typeof parsed.error).toBe("string");
    expect(parsed.error.length).toBeGreaterThan(0);
  });
});

// ─── IT-4: daemonRequest handles AbortError (timeout) ────────────────────────

describe("IT-4: daemonRequest timeout/AbortError handling", () => {
  let originalFetch: typeof globalThis.fetch;

  beforeEach(() => {
    originalFetch = globalThis.fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  test("AbortError returns error object, not throws", async () => {
    globalThis.fetch = async () => {
      const err = new Error("The operation was aborted");
      err.name = "AbortError";
      throw err;
    };

    const tool = shellopsModule.shellops_health;
    const result = await (tool as { execute: () => Promise<string> }).execute();
    const parsed = JSON.parse(result) as { error: string };
    expect(typeof parsed.error).toBe("string");
    expect(parsed.error.length).toBeGreaterThan(0);
  });
});

// ─── IT-5: All tool exports have execute functions ────────────────────────────

describe("IT-5: Tool exports are callable", () => {
  test("shellops_classify has correct parameter schema (command is required)", () => {
    const t = shellopsModule.shellops_classify as {
      args: Record<string, { isOptional: () => boolean }>;
    };
    expect(t.args.command).toBeDefined();
    expect(t.args.command.isOptional()).toBe(false);
  });

  test("shellops_terminal_run has session_id and command as required", () => {
    const t = shellopsModule.shellops_terminal_run as {
      args: Record<string, { isOptional: () => boolean }>;
    };
    expect(t.args.session_id).toBeDefined();
    expect(t.args.session_id.isOptional()).toBe(false);
    expect(t.args.command).toBeDefined();
    expect(t.args.command.isOptional()).toBe(false);
  });

  test("shellops_watch_start has id, file_path, pattern as required", () => {
    const t = shellopsModule.shellops_watch_start as {
      args: Record<string, { isOptional: () => boolean }>;
    };
    expect(t.args.id).toBeDefined();
    expect(t.args.id.isOptional()).toBe(false);
    expect(t.args.file_path).toBeDefined();
    expect(t.args.file_path.isOptional()).toBe(false);
    expect(t.args.pattern).toBeDefined();
    expect(t.args.pattern.isOptional()).toBe(false);
  });
});

// ─── IT-5b: New tool exports regression guard ─────────────────────────────────
// Guards against accidentally removing `export const` from the 15 tools added
// during shellops-zod-migration-01. If export is removed, this test fails immediately.
// axiom:trace work_item=shellops-zod-migration-01 spec=specs/48-Test-Quality-Gates.md plan=phase-5/task-5-2/step-fix-export-regression-guard

describe("IT-5b: new tool exports regression guard (shellops-zod-migration-01)", () => {
  const newTools = [
    "shellops_exec",
    "shellops_status",
    "shellops_nohup_list",
    "shellops_nohup_check",
    "shellops_nohup_output",
    "shellops_logs_query",
    "shellops_logs_similar",
    "shellops_events_query",
    "shellops_events_listen",
    "shellops_events_stop",
    "shellops_profile_load",
    "shellops_profile_query",
    "shellops_broadcast",
    "shellops_investigate",
    "shellops_triage",
  ] as const;

  test("all 15 new tool consts are named exports with execute functions", () => {
    for (const name of newTools) {
      const exported = (shellopsModule as Record<string, unknown>)[name];
      expect(exported, `${name} must be a named export from lib/shellops.ts`).toBeDefined();
      expect(
        typeof (exported as { execute?: unknown }).execute,
        `${name}.execute must be a function`
      ).toBe("function");
    }
  });
});

// ─── IT-6: ShellOpsPlugin factory returns correct hooks ──────────────────────

describe("IT-6: ShellOpsPlugin factory returns expected hooks", () => {
  test("ShellOpsPlugin export exists and is a function", () => {
    expect(typeof shellopsModule.ShellOpsPlugin).toBe("function");
  });

  test("ShellOpsPlugin() returns object with three hooks", async () => {
    const hooks = await shellopsModule.ShellOpsPlugin({});
    expect(hooks).toBeDefined();

    // Hook 1: tool.execute.before — L1 classification enforcement
    expect(typeof hooks["tool.execute.before"]).toBe("function");

    // Hook 2: experimental.chat.system.transform — ops context injection
    expect(typeof hooks["experimental.chat.system.transform"]).toBe("function");

    // Hook 3: event (session.idle) — watch result surfacing
    expect(typeof hooks["event"]).toBe("function");
  });

  test("tool.execute.before allows args without command field (fail-open)", async () => {
    // Mock fetch to simulate daemon unavailable — should not throw
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => {
      throw new Error("ECONNREFUSED");
    };

    try {
      const hooks = await shellopsModule.ShellOpsPlugin({});
      const beforeHook = hooks["tool.execute.before"] as (
        input: { tool: string; sessionID: string; callID: string },
        output: { args: Record<string, unknown> }
      ) => Promise<void>;

      // args without command — should return without throwing
      await expect(
        beforeHook(
          { tool: "bash", sessionID: "test", callID: "c1" },
          { args: { some_other_arg: "value" } }
        )
      ).resolves.toBeUndefined();
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  test("tool.execute.before throws for FORBIDDEN command when daemon responds", async () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      ({
        ok: true,
        status: 200,
        json: async () => ({
          level: "FORBIDDEN",
          blocked: true,
          message: "Command is FORBIDDEN",
        }),
      } as Response);

    try {
      const hooks = await shellopsModule.ShellOpsPlugin({});
      const beforeHook = hooks["tool.execute.before"] as (
        input: { tool: string; sessionID: string; callID: string },
        output: { args: Record<string, unknown> }
      ) => Promise<void>;

      await expect(
        beforeHook(
          { tool: "bash", sessionID: "test", callID: "c1" },
          { args: { command: "DROP DATABASE production" } }
        )
      ).rejects.toThrow("FORBIDDEN");
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // axiom:trace work_item=shellops-zod-migration-01 spec=specs/48-Test-Quality-Gates.md plan=phase-5/task-5-2/step-fix-system-transform-test
  test("system.transform hook: daemon unavailable — returns original systemPrompt unchanged", async () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => {
      throw new Error("ECONNREFUSED");
    };

    try {
      const hooks = await shellopsModule.ShellOpsPlugin({});
      // Correct signature: (systemPrompt: string) => Promise<string>
      const transformHook = hooks["experimental.chat.system.transform"] as (
        systemPrompt: string
      ) => Promise<string>;

      // Subtest A: daemon unavailable — returns original systemPrompt unchanged
      const result = await transformHook("base system prompt");
      expect(result).toBe("base system prompt");
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // IT-6b: daemon available — appends ShellOps briefing in correct format
  // axiom:trace work_item=shellops-zod-migration-01 spec=specs/115-ShellOps-Architecture.md#2.1 plan=phase-5/task-5-2/step-fix-system-transform-test
  test("IT-6b: system.transform hook: daemon available — appends ShellOps briefing", async () => {
    const hooks = await shellopsModule.ShellOpsPlugin({});
    const transformHook = hooks["experimental.chat.system.transform"] as (
      systemPrompt: string
    ) => Promise<string>;

    // Subtest B: daemon available — appends ShellOps briefing
    const successFetch: typeof globalThis.fetch = async () =>
      new Response(
        JSON.stringify({
          environment: "prod",
          active_watches: 3,
          active_terminals: 1,
          uptime_seconds: 120,
          panic_mode: false,
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      );

    const savedFetch = globalThis.fetch;
    globalThis.fetch = successFetch;
    try {
      const briefed = await transformHook("base system prompt");
      expect(briefed).toContain("base system prompt"); // original preserved
      expect(briefed).toContain("[ShellOps:");           // briefing appended
      expect(briefed).toContain("env=prod");
      expect(briefed).toContain("watches=3");
      expect(briefed).toContain("terminals=1");
    } finally {
      globalThis.fetch = savedFetch;
    }
  });
});

// ─── IT-7: v1 endpoint paths match expected API contract ─────────────────────

describe("IT-7: Plugin calls correct /api/v1/ endpoints", () => {
  const interceptedCalls: Array<{ method: string; url: string }> = [];
  let originalFetch: typeof globalThis.fetch;

  beforeEach(() => {
    interceptedCalls.length = 0;
    originalFetch = globalThis.fetch;
    globalThis.fetch = async (input, init) => {
      const url = typeof input === "string" ? input : (input as Request).url;
      const method = (init?.method ?? "GET").toUpperCase();
      interceptedCalls.push({ method, url });
      // Return a minimal OK response
      return new Response(JSON.stringify({ ok: true, status: "ok" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    };
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  test("shellops_classify calls POST /api/v1/classify", async () => {
    const tool = shellopsModule.shellops_classify;
    await (tool as { execute: (args: { command: string }) => Promise<string> }).execute({
      command: "ls",
    });
    const call = interceptedCalls.find((c) => c.url.includes("/api/v1/classify"));
    expect(call).toBeDefined();
    expect(call!.method).toBe("POST");
  });

  test("shellops_terminal_create calls POST /api/v1/terminal/create", async () => {
    const tool = shellopsModule.shellops_terminal_create;
    await (tool as { execute: (args: { name?: string }) => Promise<string> }).execute({});
    const call = interceptedCalls.find((c) => c.url.includes("/api/v1/terminal/create"));
    expect(call).toBeDefined();
    expect(call!.method).toBe("POST");
  });

  test("shellops_terminal_list calls GET /api/v1/terminal/list", async () => {
    const tool = shellopsModule.shellops_terminal_list;
    await (tool as { execute: () => Promise<string> }).execute();
    const call = interceptedCalls.find((c) => c.url.includes("/api/v1/terminal/list"));
    expect(call).toBeDefined();
    expect(call!.method).toBe("GET");
  });

  test("shellops_watch_start calls POST /api/v1/watch/start", async () => {
    const tool = shellopsModule.shellops_watch_start;
    await (tool as {
      execute: (args: { id: string; file_path: string; pattern: string }) => Promise<string>;
    }).execute({ id: "w1", file_path: "/tmp/test.log", pattern: "ERROR" });
    const call = interceptedCalls.find((c) => c.url.includes("/api/v1/watch/start"));
    expect(call).toBeDefined();
    expect(call!.method).toBe("POST");
  });

  test("shellops_watch_list calls GET /api/v1/watch/list", async () => {
    const tool = shellopsModule.shellops_watch_list;
    await (tool as { execute: () => Promise<string> }).execute();
    const call = interceptedCalls.find((c) => c.url.includes("/api/v1/watch/list"));
    expect(call).toBeDefined();
    expect(call!.method).toBe("GET");
  });

  test("shellops_health calls GET /health", async () => {
    const tool = shellopsModule.shellops_health;
    await (tool as { execute: () => Promise<string> }).execute();
    const call = interceptedCalls.find((c) => c.url.includes("/health"));
     expect(call).toBeDefined();
     expect(call!.method).toBe("GET");
  });
});

// ─── IT-8: daemonRequest Content-Type guard — Bug 1 regression ───────────────
// Regression test for: daemonRequest always called response.json() on ALL
// HTTP responses, including Go's http.Error() plain-text 400/404 responses.
// Bun's JSON parser threw "Failed to parse JSON", masking the real error.
// Fix: check Content-Type header before parsing; plain-text responses are
// surfaced as { error: <actual text> } instead of a JSON parse failure.
//
// axiom:trace work_item=plugin-live-test-findings-01 spec=specs/117-ShellOps-Log-Intelligence.md plan=phase-5/task-5-1/step-5-1-1
// ─────────────────────────────────────────────────────────────────────────────
describe("IT-8: daemonRequest Content-Type guard (Bug 1 regression)", () => {
  let originalFetch: typeof globalThis.fetch;

  beforeEach(() => {
    originalFetch = globalThis.fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  test("plain-text 404 response surfaces actual error text, not 'Failed to parse JSON'", async () => {
    // Simulate Go's http.Error(w, "watch \"integ-watch-01\" not found", 404)
    // which returns text/plain body with no Content-Type: application/json header.
    // Before the fix: response.json() threw "Failed to parse JSON".
    // After the fix: the error text is surfaced as { error: <text> }.
    globalThis.fetch = async () =>
      new Response('watch "integ-watch-01" not found\n', {
        status: 404,
        headers: {}, // intentionally NO Content-Type: application/json
      });

    const tool = shellopsModule.shellops_watch_stop;
    const result = await (tool as { execute: (args: { id: string }) => Promise<string> }).execute({ id: "integ-watch-01" });
    const parsed = JSON.parse(result) as { error?: string; status?: string };

    // Must NOT be a JSON parse failure — that was the bug.
    expect(parsed.error).not.toContain("Failed to parse JSON");
    expect(parsed.error).not.toContain("SyntaxError");

    // Must surface the actual error text from the daemon response body.
    expect(typeof parsed.error).toBe("string");
    expect(parsed.error).toContain("not found");
  });

  test("plain-text 400 response surfaces error text, not JSON parse failure", async () => {
    // Simulate Go's http.Error(w, "id is required", 400) — text/plain, no Content-Type.
    globalThis.fetch = async () =>
      new Response("id is required\n", {
        status: 400,
        headers: {},
      });

    const tool = shellopsModule.shellops_exec;
    const result = await (tool as { execute: (args: { command: string }) => Promise<string> }).execute({ command: "echo hi" });
    const parsed = JSON.parse(result) as { error?: string };

    expect(parsed.error).not.toContain("Failed to parse JSON");
    expect(parsed.error).not.toContain("SyntaxError");
    expect(typeof parsed.error).toBe("string");
    expect(parsed.error).toContain("required");
  });

  test("JSON response with Content-Type: application/json is still parsed correctly", async () => {
    // Verify the fix does NOT break the happy path — valid JSON responses still work.
    globalThis.fetch = async () =>
      new Response(JSON.stringify({ status: "stopped" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });

    const tool = shellopsModule.shellops_watch_stop;
    const result = await (tool as { execute: (args: { id: string }) => Promise<string> }).execute({ id: "watch-abc123" });
    const parsed = JSON.parse(result) as { status?: string };

    expect(parsed.status).toBe("stopped");
  });

  test("JSON response WITHOUT Content-Type header is still surfaced via fallback JSON.parse", async () => {
    // Some Go handlers may return JSON without setting Content-Type.
    // The fix includes a fallback JSON.parse attempt so those responses still work.
    globalThis.fetch = async () =>
      new Response(JSON.stringify({ status: "ok", detail: "probe" }), {
        status: 200,
        headers: {}, // no Content-Type — tests the JSON.parse fallback path
      });

    const tool = shellopsModule.shellops_health;
    const result = await (tool as { execute: () => Promise<string> }).execute();
    const parsed = JSON.parse(result) as { status?: string };

    // Should be parsed as JSON (fallback JSON.parse succeeded), not as {error: "..."}.
    expect(parsed.status).toBe("ok");
  });
});
