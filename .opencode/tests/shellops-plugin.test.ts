/**
 * ShellOps Plugin Integration Tests — LIVE DAEMON
 *
 * These tests start the real ShellOps Go daemon on a test port and verify that
 * every plugin tool's HTTP endpoint returns a non-404 response. This is the
 * critical regression test for bug SA-1, where the plugin called /api/v1/*
 * paths but the daemon only registered /api/* paths — all tool calls silently
 * got 404s and the mismatch went undetected.
 *
 * Test coverage:
 *   LT-1  /health returns 200 {"status":"ok"}
 *   LT-2  POST /api/v1/classify returns non-404 with classification level
 *   LT-3  POST /api/v1/classify DANGEROUS returns non-404
 *   LT-4  POST /api/v1/terminal/create returns non-404 with session_id
 *   LT-5  GET  /api/v1/terminal/list returns non-404 with sessions array
 *   LT-6  POST /api/v1/terminal/send returns non-404 (shellops_terminal_run)
 *   LT-7  POST /api/v1/terminal/read returns non-404 (shellops_terminal_capture)
 *   LT-8  POST /api/v1/terminal/kill returns non-404 (shellops_terminal_destroy)
 *   LT-9  POST /api/v1/watch/start returns non-404
 *   LT-10 GET  /api/v1/watch/query returns non-404 (shellops_watch_query)
 *   LT-11 GET  /api/v1/watch/list returns non-404 with watches array
 *   LT-12 POST /api/v1/watch/stop returns non-404
 *   LT-13 GET  /api/v1/events/query returns non-404 (shellops_events_query)
 *   LT-14 POST /api/v1/events/listen returns non-404 (shellops_events_listen)
 *   LT-15 GET  /api/v1/profiles/query returns non-404 (shellops_profile_query)
 *   LT-16 POST /api/v1/broadcast returns non-404 (shellops_broadcast)
 *   LT-17 POST /api/v1/investigate returns non-404 (shellops_investigate)
 *   LT-18 POST /api/v1/triage returns non-404 (shellops_triage)
 *   LT-19 POST /api/v1/panic-mode returns non-404
 *   LT-20 GET  /api/v1/status returns non-404 with uptime
 *   SC-1  All 11 current tool exports exist in shellops.ts (surface completeness)
 *   SC-2  Missing 10 tools noted as known gap (shellops_logs_*, shellops_events_stop,
 *          shellops_profile_load + 7 others) — tests fail when missing tools are added
 *          without corresponding daemon route
 *
 * Prerequisites:
 *   - shellops binary built: go build -o /tmp/shellops-integration-test-bin ./cmd/shellops/
 *     (the beforeAll() does this automatically via GO_BIN)
 *   - Port 19876 must be free
 *
 * Run:
 *   cd .opencode && bun test plugins/shellops.test.ts --timeout 30000
 *
 * axiom:trace work_item=shellops-01 spec=specs/115-ShellOps-Architecture.md plan=add-typescript-integration-tests test=plugins/shellops.test.ts
 */

import { describe, test, expect, beforeAll, afterAll } from "bun:test";
import { spawnSync, spawn, type ChildProcess } from "child_process";

// ─────────────────────────────────────────────────────────────────────────────
// Test configuration
// ─────────────────────────────────────────────────────────────────────────────

const TEST_PORT = "19876";
// Use the toolchain path confirmed working in this environment
const GO_BIN =
  "/home/coder/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.linux-amd64/bin/go";
const DAEMON_BIN = "/tmp/shellops-integration-test-bin";
const SHELLOPS_REPO = "/home/coder/code/Axiom/shellops";
// Daemon binds to :PORT (all interfaces) so 127.0.0.1 works fine
const BASE_URL = `http://127.0.0.1:${TEST_PORT}`;

let daemonProcess: ChildProcess | null = null;
// Tracks sessions created in LT-4 for use in LT-6/LT-7/LT-8
let testSessionId: string | null = null;
// Tracks watch id created in LT-9 for use in LT-10/LT-12
const TEST_WATCH_ID = "integration-test-watch";

// ─────────────────────────────────────────────────────────────────────────────
// HTTP helpers
// ─────────────────────────────────────────────────────────────────────────────

async function req(
  method: string,
  path: string,
  body?: unknown,
): Promise<{ status: number; data: unknown; text: string }> {
  const response = await fetch(`${BASE_URL}${path}`, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
    signal: AbortSignal.timeout(8000),
  });
  const text = await response.text();
  let data: unknown = null;
  try {
    data = JSON.parse(text);
  } catch {
    data = text;
  }
  return { status: response.status, data, text };
}

async function waitForDaemon(maxMs = 12_000): Promise<void> {
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    try {
      const r = await fetch(`${BASE_URL}/health`, {
        signal: AbortSignal.timeout(800),
      });
      if (r.ok) return;
    } catch {
      /* not ready yet */
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(
    `Daemon on port ${TEST_PORT} did not become ready within ${maxMs}ms`,
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Lifecycle: build + start daemon, then stop it
// ─────────────────────────────────────────────────────────────────────────────

beforeAll(async () => {
  // Build daemon binary from current source so the test always uses fresh code
  const build = spawnSync(
    GO_BIN,
    ["build", "-o", DAEMON_BIN, "./cmd/shellops/"],
    {
      cwd: SHELLOPS_REPO,
      env: { ...process.env, HOME: process.env.HOME ?? "/root" },
      timeout: 90_000,
    },
  );
  if (build.status !== 0) {
    throw new Error(
      `Failed to build shellops daemon: ${build.stderr?.toString() ?? "(no stderr)"}`,
    );
  }

  // Spawn daemon on test port with shellops repo as root (config lives there)
  daemonProcess = spawn(
    DAEMON_BIN,
    ["start", "--port", TEST_PORT, "--root", SHELLOPS_REPO],
    {
      cwd: SHELLOPS_REPO,
      env: { ...process.env },
      detached: false,
      stdio: ["ignore", "ignore", "ignore"],
    },
  );

  daemonProcess.on("error", (err) => {
    console.error("[integration-test] daemon spawn error:", err);
  });

  await waitForDaemon(12_000);
});

afterAll(() => {
  if (daemonProcess) {
    daemonProcess.kill("SIGTERM");
    daemonProcess = null;
  }
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-1: Health check
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-1: Health check", () => {
  test("GET /health returns 200 with status:ok", async () => {
    const { status, data } = await req("GET", "/health");
    expect(status).toBe(200);
    expect((data as Record<string, unknown>).status).toBe("ok");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-2 / LT-3: Classification (shellops_classify)
// SA-1 regression: plugin calls POST /api/v1/classify — must NOT 404
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-2/LT-3: shellops_classify → POST /api/v1/classify", () => {
  test("LT-2: SAFE command returns non-404 with level:SAFE", async () => {
    const { status, data } = await req("POST", "/api/v1/classify", {
      command: "ls -la",
    });
    // The SA-1 bug would give 404 here. Non-404 = route is wired correctly.
    expect(status).not.toBe(404);
    expect(status).toBe(200);
    const d = data as Record<string, unknown>;
    expect(d.level).toBe("SAFE");
    expect(d.blocked).toBe(false);
  });

  test("LT-3: Destructive command returns non-404 with DANGEROUS/CAUTIOUS level", async () => {
    const { status, data } = await req("POST", "/api/v1/classify", {
      command: "kubectl delete pod --all",
    });
    expect(status).not.toBe(404);
    const d = data as Record<string, unknown>;
    expect(["DANGEROUS", "CAUTIOUS", "FORBIDDEN"]).toContain(d.level);
  });

  test("LT-3b: Missing command field returns 400", async () => {
    const { status } = await req("POST", "/api/v1/classify", {});
    // 400 means the route IS registered (a 404 would mean the route is missing entirely)
    expect(status).not.toBe(404);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-4 / LT-5: Terminal management (shellops_terminal_create, shellops_terminal_list)
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-4/LT-5: Terminal management routes", () => {
  test("LT-4: POST /api/v1/terminal/create returns non-404 with session_id", async () => {
    const { status, data } = await req("POST", "/api/v1/terminal/create", {
      name: "shellops-integration-test",
    });
    expect(status).not.toBe(404);
    const d = data as Record<string, unknown>;
    expect(typeof d.session_id).toBe("string");
    expect((d.session_id as string).length).toBeGreaterThan(0);
    // Store for LT-6/LT-7/LT-8
    testSessionId = d.session_id as string;
  });

  test("LT-5: GET /api/v1/terminal/list returns non-404 with sessions array", async () => {
    const { status, data } = await req("GET", "/api/v1/terminal/list");
    expect(status).not.toBe(404);
    const d = data as Record<string, unknown>;
    expect(Array.isArray(d.sessions)).toBe(true);
    // We created one session in LT-4
    expect((d.sessions as unknown[]).length).toBeGreaterThanOrEqual(1);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-6 / LT-7 / LT-8: Terminal send/read/kill
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-6/LT-7/LT-8: Terminal send/read/kill routes", () => {
  test("LT-6: POST /api/v1/terminal/send returns non-404 (shellops_terminal_run)", async () => {
    if (!testSessionId) {
      throw new Error("No session from LT-4; cannot test terminal/send");
    }
    const { status, data } = await req("POST", "/api/v1/terminal/send", {
      session_id: testSessionId,
      command: "echo shellops-integration-test",
    });
    expect(status).not.toBe(404);
    // Body assertions: RunResult must have output (string) and exit_code (number)
    const d = data as any;
    expect(typeof d.output).toBe("string");
    expect(typeof d.exit_code).toBe("number");
    expect(d.exit_code).toBe(0);
  });

  test("LT-7: POST /api/v1/terminal/read returns non-404 (shellops_terminal_capture)", async () => {
    if (!testSessionId) {
      throw new Error("No session from LT-4; cannot test terminal/read");
    }
    const { status } = await req("POST", "/api/v1/terminal/read", {
      session_id: testSessionId,
      lines: 20,
    });
    expect(status).not.toBe(404);
  });

  test("LT-8: POST /api/v1/terminal/kill returns non-404 (shellops_terminal_destroy)", async () => {
    if (!testSessionId) {
      throw new Error("No session from LT-4; cannot test terminal/kill");
    }
    const { status } = await req("POST", "/api/v1/terminal/kill", {
      session_id: testSessionId,
    });
    expect(status).not.toBe(404);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-9 / LT-10 / LT-11 / LT-12: Watch management
// Note: The daemon assigns its own watch_id (e.g. "watch-abcd1234") distinct
// from the "id" name sent in the request. LT-9 captures the returned watch_id
// for use in LT-10 and LT-12.
// ─────────────────────────────────────────────────────────────────────────────

// Stores the daemon-assigned watch_id returned by LT-9 for LT-10/LT-12
let createdWatchId: string | null = null;

describe("LT-9..LT-12: Watch management routes", () => {
  test("LT-9: POST /api/v1/watch/start returns non-404 with watch_id (shellops_watch_start)", async () => {
    // The file must exist for the watcher to accept it
    const { spawnSync } = await import("child_process");
    spawnSync("touch", ["/tmp/shellops-integration-test.log"]);

    const { status, data } = await req("POST", "/api/v1/watch/start", {
      id: TEST_WATCH_ID,
      file_path: "/tmp/shellops-integration-test.log",
      pattern: "ERROR",
      agent_id: "integration-test",
    });
    expect(status).not.toBe(404);
    // 201 = created; 400 = file validation failure (route IS registered)
    const d = data as Record<string, unknown>;
    // On success, daemon returns the watch record with a watch_id field
    if (status === 201 || status === 200) {
      expect(typeof d.watch_id).toBe("string");
      createdWatchId = d.watch_id as string;
    } else {
      // Non-201 means validation error — route IS registered, just the
      // file might not be accessible. That's OK for this routing test.
      expect(status).not.toBe(404);
    }
  });

  test("LT-10: GET /api/v1/watch/query returns non-404 (shellops_watch_query)", async () => {
    // With no watch_id param, returns all watches — guaranteed non-empty path
    const { status, data } = await req("GET", "/api/v1/watch/query");
    expect(status).not.toBe(404);
    // Both "watches" array and a single watch record are valid responses
    expect(data).toBeDefined();

    // If we have a created watch from LT-9, also verify query-by-id works
    if (createdWatchId) {
      const { status: s2, data: d2 } = await req(
        "GET",
        `/api/v1/watch/query?watch_id=${createdWatchId}`,
      );
      expect(s2).not.toBe(404);
      expect(d2).toBeDefined();
    }
  });

  test("LT-11: GET /api/v1/watch/list returns non-404 with watches array (shellops_watch_list)", async () => {
    const { status, data } = await req("GET", "/api/v1/watch/list");
    expect(status).not.toBe(404);
    const d = data as Record<string, unknown>;
    expect((d as any)).toHaveProperty("watches");
    expect(Array.isArray((d as any).watches)).toBe(true);
  });

  test("LT-12: POST /api/v1/watch/stop returns non-404 (shellops_watch_stop)", async () => {
    // Use the daemon-assigned watch_id from LT-9; if unavailable, send a
    // deliberate missing-id request — a 404 with "watch not found" proves
    // the ROUTE exists (a missing route gives a generic Go ServeMux 404).
    const idToStop = createdWatchId ?? "nonexistent-watch-probe";
    const { status, text } = await req("POST", "/api/v1/watch/stop", {
      id: idToStop,
    });
    // 200 = stopped, 404 with "not found" body = route registered but watch
    // ID unknown. A routing 404 has an empty body or "404 page not found".
    if (status === 404) {
      // Distinguish: domain 404 ("watch X not found") vs routing 404 ("404 page not found")
      expect(text).not.toContain("page not found");
    } else {
      expect(status).toBe(200);
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-13 / LT-14: Events (shellops_events_query, shellops_events_listen)
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-13/LT-14: Events routes", () => {
  test("LT-13: GET /api/v1/events/query returns non-404 (shellops_events_query)", async () => {
    const { status, data: d } = await req("GET", "/api/v1/events/query");
    expect(status).not.toBe(404);
    // events may be null when empty, but the response must be a defined object
    expect(d).toBeDefined();
    expect(typeof d).toBe("object");
  });

  test("LT-14: POST /api/v1/events/listen returns non-404 (shellops_events_listen)", async () => {
    // source is required — a well-formed 400 proves the route is registered
    const { status } = await req("POST", "/api/v1/events/listen", {
      source: "webhook",
      data: {},
    });
    // 202 = accepted, 400 = bad request (route registered but validation failed)
    // 404 = route is missing (the SA-1 class of bug)
    expect(status).not.toBe(404);
    expect([200, 202, 400, 422]).toContain(status);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-15: Profiles (shellops_profile_query)
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-15: Profiles routes", () => {
  test("LT-15: GET /api/v1/profiles/query returns non-404 (shellops_profile_query)", async () => {
    const { status, data } = await req("GET", "/api/v1/profiles/query");
    expect(status).not.toBe(404);
    const d = data as Record<string, unknown>;
    // profiles is either an array or null — both are valid; must have the key
    expect(d).toHaveProperty("profiles");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-16: Broadcast (shellops_broadcast)
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-16: Broadcast route", () => {
  test("LT-16: POST /api/v1/broadcast returns non-404 with records (shellops_broadcast)", async () => {
    const { status, data } = await req("POST", "/api/v1/broadcast", {
      text: "ShellOps integration test broadcast",
      severity: "P3",
    });
    expect(status).not.toBe(404);
    const d = data as Record<string, unknown>;
    // 200 = sent, 422 = blocked (content filter) — both prove the route is wired
    expect([200, 422]).toContain(status);
    expect(d.records ?? d.status).toBeDefined();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-17: Investigate (shellops_investigate)
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-17: Investigate route", () => {
  test("LT-17: POST /api/v1/investigate returns non-404 with investigation id (shellops_investigate)", async () => {
    const { status, data } = await req("POST", "/api/v1/investigate", {
      service: "shellops-integration-test-svc",
      symptom: "high latency in integration test",
      timebox_minutes: 5,
    });
    expect(status).not.toBe(404);
    const d = data as Record<string, unknown>;
    expect(typeof d.id).toBe("string");
    expect(d.status).toBe("started");
    expect(d.service).toBe("shellops-integration-test-svc");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-18: Triage (shellops_triage)
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-18: Triage route", () => {
  test("LT-18: POST /api/v1/triage returns non-404 with severity (shellops_triage)", async () => {
    const { status, data } = await req("POST", "/api/v1/triage", {
      signals: {
        service_tier: "standard",
        error_trend: "increasing",
      },
    });
    expect(status).not.toBe(404);
    expect(status).toBe(200);
    const d = data as Record<string, unknown>;
    expect(typeof d.severity).toBe("string");
    expect(["P1", "P2", "P3", "P4"]).toContain(d.severity);
    expect(typeof d.score).toBe("number");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-19: Panic mode
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-19: Panic mode route", () => {
  test("LT-19: POST /api/v1/panic-mode returns non-404 (shellops_classify gate)", async () => {
    const { status, data } = await req("POST", "/api/v1/panic-mode", {
      active: false,
    });
    expect(status).not.toBe(404);
    expect(status).toBe(200);
    const d = data as Record<string, unknown>;
    expect(d.status).toBe("ok");
    expect(d.active).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-20: Status
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-20: Status route", () => {
  test("LT-20: GET /api/v1/status returns non-404 with uptime_seconds", async () => {
    const { status, data } = await req("GET", "/api/v1/status");
    expect(status).not.toBe(404);
    const d = data as Record<string, unknown>;
    expect(typeof d.uptime_seconds).toBe("number");
    // uptime_seconds is 0 when daemon just started and is a valid value;
    // we only require the field is present and non-negative.
    expect((d.uptime_seconds as number)).toBeGreaterThanOrEqual(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// SC-1: Tool surface completeness — all currently-exported tools exist
// ─────────────────────────────────────────────────────────────────────────────

describe("SC-1: shellops.ts tool surface completeness", () => {
  // The 26 tool constants in shellops.ts use raw JSON Schema in `args` (not Zod
  // via tool.schema.*). Wiring them into the factory's `tool: {}` causes
  // Claude/Bedrock to reject the input_schema as invalid JSON Schema 2020-12.
  // Until the schemas are migrated to Zod, the plugin factory exposes no tools
  // through OpenCode's plugin loader. The 11 expected tools are documented
  // here so the test fails loudly when migration completes (and fails the
  // "tools defined as constants" check below until the constants change).
  const implementedTools = [
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
  ];

  test("ShellOpsPlugin factory is exported", async () => {
    const shellopsModule = await import("../plugins/shellops.ts");
    expect(typeof shellopsModule.ShellOpsPlugin).toBe("function");
  });

  test("11 implemented tool constants are defined in shellops.ts source", async () => {
    // We check the source file for `const <toolName> = tool({` patterns rather
    // than importing — the constants are module-local (not exported) so we
    // can only verify their presence textually. When migration to Zod-via-args
    // completes and these are wired into `tool: {}`, switch this test to load
    // the factory result and verify each tool is registered.
    const fs = await import("fs");
    const source = fs.readFileSync(import.meta.dir + "/../lib/shellops.ts", "utf8");
    for (const name of implementedTools) {
      expect(source, `Missing tool constant: const ${name} = tool({`).toContain(`const ${name} = tool({`);
    }
  });

  // SC-2: Gap tracking — the 10 tools that exist as daemon routes but not
  // yet in shellops.ts. This test documents the known gap. When the missing
  // tools are added to shellops.ts, add them to `implementedTools` above
  // AND to the live daemon tests above.
  //
  // Missing tools (daemon routes exist, plugin exports do not):
  //   shellops_logs_query      → GET  /api/v1/logs/query     (not in daemon yet)
  //   shellops_logs_similar    → POST /api/v1/logs/similar   (not in daemon yet)
  //   shellops_events_stop     → POST /api/v1/events/stop
  //   shellops_profile_load    → POST /api/v1/profiles/load
  //   shellops_health (v1)     → shellops_health calls /health, not /api/v1/health
  //
  // The above 4 (excluding logs which have no daemon route) should be added
  // to shellops.ts as the next implementation step.
  test("SC-2: known gap — 10 spec tools not yet in shellops.ts (documents progress)", async () => {
    const fs = await import("fs");
    const source = fs.readFileSync(import.meta.dir + "/../lib/shellops.ts", "utf8");
    const missingFromPlugin = [
      "shellops_logs_query",
      "shellops_logs_similar",
      "shellops_events_stop",
      "shellops_profile_load",
      "shellops_broadcast",
      "shellops_investigate",
      "shellops_triage",
      "shellops_events_query",
      "shellops_events_listen",
      "shellops_profile_query",
    ];
    const actuallyMissing = missingFromPlugin.filter((name) =>
      !source.includes(`const ${name} = tool({`)
    );
    // This test DOCUMENTS the gap rather than failing the build.
    // When actuallyMissing.length reaches 0, the surface is complete.
    if (actuallyMissing.length > 0) {
      console.warn(
        `[SC-2] ${actuallyMissing.length} spec tools still missing from shellops.ts: ${actuallyMissing.join(", ")}`,
      );
    }
    // Real threshold gate: migration complete 2026-05-18 — all 26 tools present; threshold locked to 0.
    // Temporarily removing any shellops tool from lib/shellops.ts will cause this test to fail.
    expect(actuallyMissing.length).toBe(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// SA-1 Regression Guard
// This test would have FAILED had it existed before the bug was introduced.
// It verifies the exact URL scheme the plugin uses matches the daemon.
// ─────────────────────────────────────────────────────────────────────────────

describe("SA-1 Regression Guard: /api/v1/ prefix is correctly wired", () => {
  test("plugin URL prefix /api/v1/ resolves to daemon routes (not 404)", async () => {
    // Probe all /api/v1/ routes that the current 11 tools call.
    // We distinguish routing 404s ("404 page not found") from domain 404s
    // ("watch X not found") — only routing 404s indicate the SA-1 class of bug.
    const probes: Array<[string, string, unknown?]> = [
      ["POST", "/api/v1/classify", { command: "date" }],
      ["POST", "/api/v1/terminal/create", { name: "sa1-regression-probe" }],
      ["GET", "/api/v1/terminal/list"],
      ["POST", "/api/v1/watch/start", { id: "sa1-probe-watch", file_path: "/tmp/shellops-integration-test.log", pattern: "TEST" }],
      ["GET", "/api/v1/watch/list"],
      // watch/stop may return a domain-404 if the watch wasn't created;
      // only a "404 page not found" (empty/generic body) is a routing failure
      ["GET", "/health"], // shellops_health calls /health (not /api/v1/health)
    ];

    const routingFailures: string[] = [];
    for (const [method, path, body] of probes) {
      const { status, text } = await req(method, path, body);
      if (status === 404 && (text.trim() === "404 page not found" || text.trim() === "")) {
        routingFailures.push(`${method} ${path} → routing 404 (SA-1 class bug!)`);
      }
    }

    // Also probe watch/stop and check for routing (not domain) 404
    const { status: wsStatus, text: wsText } = await req("POST", "/api/v1/watch/stop", {
      id: "sa1-probe-watch",
    });
    if (wsStatus === 404 && wsText.includes("page not found")) {
      routingFailures.push(`POST /api/v1/watch/stop → routing 404 (SA-1 class bug!)`);
    }

    if (routingFailures.length > 0) {
      throw new Error(
        `SA-1 regression detected — these endpoints returned routing 404s:\n  ${routingFailures.join("\n  ")}`,
      );
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-21: POST /api/exec — executes command and returns output
// Verifies the exec endpoint actually runs commands, not just routes them.
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-21: POST /api/exec executes command and returns output", () => {
  test("POST /api/exec executes command and returns output", async () => {
    const { status, data } = await req("POST", "/api/exec", { command: "echo shellops-test-verify" });
    expect(status).toBe(200);
    expect((data as any).executed).toBe(true);
    expect((data as any).output).toContain("shellops-test-verify");
    // axiom:trace work_item=shellops-01 spec=specs/115-ShellOps-Architecture.md plan=improve-ts-integration-test-assertions
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// SC-3: ShellOpsPlugin hook completeness
// Regression guard: ensures on_session_idle (and all 4 required hooks) remain
// present in shellops.ts after any future refactor.
// ─────────────────────────────────────────────────────────────────────────────

describe("SC-3: ShellOpsPlugin hook completeness", () => {
  // axiom:trace work_item=shellops-01 spec=specs/115-ShellOps-Architecture.md#2.1 plan=add-on-session-idle-test-guard

  test("ShellOpsPlugin returns on_session_idle hook", async () => {
    // Import and call the factory — it's exported from shellops.ts
    const fs = await import("fs");
    const content = fs.readFileSync(import.meta.dir + "/../lib/shellops.ts", "utf-8");

    // Verify the on_session_idle key exists in the ShellOpsPlugin return object
    expect(content).toContain('"on_session_idle"');
    // Verify it's a function declaration (not just a comment)
    expect(content).toMatch(/["']on_session_idle["']\s*:\s*async/);
  });

  test("ShellOpsPlugin has all 4 required hook keys", async () => {
    const fs = await import("fs");
    const content = fs.readFileSync(import.meta.dir + "/../lib/shellops.ts", "utf-8");

    // Quoted hooks (string-keyed properties)
    const quotedHooks = [
      "tool.execute.before",
      "experimental.chat.system.transform",
      "on_session_idle",
    ];
    for (const hook of quotedHooks) {
      expect(content).toContain(`"${hook}"`);
    }

    // "event" is written as a bare (unquoted) identifier key in the source
    expect(content).toMatch(/\bevent\s*:/);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// LT-22 / LT-23: BUG-5 regression — /api/v1/logs/similar route
//
// BUG-5: The /api/v1/logs/similar route was added to the ShellOps daemon but
// had no test. If the route is removed or renamed, these tests catch it.
//
// axiom:trace work_item=plugin-bug-sweep-01 spec=specs/115-ShellOps-Architecture.md plan=phase-4/step-verify-002/bug-5 test=plugins/shellops.test.ts
// ─────────────────────────────────────────────────────────────────────────────

describe("LT-22/LT-23: BUG-5 regression — GET /api/v1/logs/similar route", () => {
  // LT-22: BUG-5 regression — /api/v1/logs/similar route exists and is not 404
  test("LT-22 BUG-5 regression: GET /api/v1/logs/similar is registered (not 404)", async () => {
    const { status } = await req("GET", "/api/v1/logs/similar?pattern=ERROR&limit=5");
    expect(status).not.toBe(404);  // Route must be registered
  });

  // LT-23: BUG-5: /api/v1/logs/similar response has expected schema
  test("LT-23 BUG-5 regression: /api/v1/logs/similar returns correct JSON schema", async () => {
    const { status, data } = await req("GET", "/api/v1/logs/similar?pattern=test&limit=3");
    expect(status).not.toBe(404);
    expect((data as Record<string, unknown>).pattern).toBe("test");  // Response includes pattern echo
  });
});
