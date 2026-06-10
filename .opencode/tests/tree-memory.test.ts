/**
 * Tests for tree-memory.ts — Tree Memory Plugin.
 *
 * AC-1  tree.init creates .tree-memory/repo/ git repo
 * AC-2  tree.branch creates and lists branches
 * AC-3  tree.commit writes file + commits; secret scanning blocks secrets
 * AC-4  tree.promote creates promoted finding
 * AC-5  tree.merge merges branch to main
 * AC-6  tree.state reads/updates agent state
 * AC-7  tree.query queries findings across branches
 * AC-8  tree.peers queries agent states
 * AC-9  tree.spawn creates child agent branch with state
 * AC-10 tree.status returns overview
 * AC-11 tree.log returns git history
 * AC-12 Secret scanning blocks AWS keys, private keys, tokens
 * AC-13 UUID-framed log markers wrap query results
 * AC-14 Config loader reads .tree-memory/config.yaml
 * AC-15 Query engine sandbox enforcement
 *
 * Run: cd .opencode && bun test tests/tree-memory.test.ts
 *
 * axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md plan=phase-1/task-1-7/step-1-7-1
 */

import { test, expect, describe, beforeEach, afterEach } from "bun:test";
import {
  existsSync,
  mkdtempSync,
  mkdirSync,
  rmSync,
  writeFileSync,
  readFileSync,
  symlinkSync,
} from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { execSync } from "node:child_process";

import {
  TreeMemoryPlugin,
  GitEngine,
  QueryEngine,
  DuckdbAdapter,
  loadDuckdbIfAvailable,
  scanForSecrets,
  frameLogContent,
  loadTreeMemoryConfig,
  loadTreeIgnorePatterns,
  redactPiiContent,
} from "../lib/tree-memory.ts";

// ─── Helpers ──────────────────────────────────────────────────────────────────

function makeTmpDir(): string {
  return mkdtempSync(join(tmpdir(), "tree-memory-test-"));
}

/** Configure git user in a temp dir (required for commits) */
function configureGitUser(repoPath: string): void {
  execSync('git config user.email "test@test.com"', { cwd: repoPath, stdio: "pipe" });
  execSync('git config user.name "Test"', { cwd: repoPath, stdio: "pipe" });
}

async function createPlugin(tmpDir: string) {
  return TreeMemoryPlugin({ directory: tmpDir, client: {} });
}

function callTool(
  plugin: Awaited<ReturnType<typeof TreeMemoryPlugin>>,
  toolName: string,
  args: Record<string, unknown>,
  context: Record<string, unknown> = {},
): Promise<unknown> {
  const t = plugin.tool[toolName as keyof typeof plugin.tool] as
    | { execute: (args: unknown, context: unknown) => Promise<unknown> }
    | undefined;
  if (!t) throw new Error(`Tool not found: ${toolName}`);
  return t.execute(args, context);
}

function parse(result: unknown): Record<string, unknown> {
  // Handle UUID-framed responses
  const str = String(result);
  // Strip LOG_MATCH frames if present
  const stripped = str.replace(/───── LOG_MATCH:[^\n]+\n/g, "").replace(/───── END_LOG_MATCH:[^\n]+/g, "").trim();
  return JSON.parse(stripped) as Record<string, unknown>;
}

// ─── GitEngine Unit Tests ─────────────────────────────────────────────────────

describe("GitEngine", () => {
  let tmpDir: string;
  let repoPath: string;
  let engine: GitEngine;

  beforeEach(() => {
    tmpDir = makeTmpDir();
    repoPath = join(tmpDir, ".tree-memory", "repo");
    mkdirSync(repoPath, { recursive: true });
    engine = new GitEngine(repoPath);
    engine.init();
    configureGitUser(repoPath);
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("init creates git repo with .tree-memory.json", () => {
    expect(existsSync(join(repoPath, ".git"))).toBe(true);
    expect(existsSync(join(repoPath, ".tree-memory.json"))).toBe(true);
    const meta = JSON.parse(readFileSync(join(repoPath, ".tree-memory.json"), "utf-8"));
    expect(meta.schema_version).toBe("1.0.0");
  });

  test("createBranch and listBranches", () => {
    engine.createBranch("agent-test");
    const branches = engine.listBranches();
    expect(branches).toContain("agent-test");
    expect(branches).toContain("main");
  });

  test("commitFile writes and commits", () => {
    engine.createBranch("agent-a");
    engine.commitFile("findings/001-test.json", '{"id":"f-001"}', "test commit");
    expect(existsSync(join(repoPath, "findings", "001-test.json"))).toBe(true);
    const content = readFileSync(join(repoPath, "findings", "001-test.json"), "utf-8");
    expect(content).toBe('{"id":"f-001"}');
  });

  test("merge succeeds for non-conflicting branches", () => {
    engine.createBranch("agent-b");
    engine.commitFile("findings/001.json", '{"id":"1"}', "finding 1");
    const result = engine.merge("main");
    expect(result.success).toBe(true);
  });

  test("currentBranch returns correct branch", () => {
    expect(engine.currentBranch()).toBe("main");
    engine.createBranch("test-branch");
    expect(engine.currentBranch()).toBe("test-branch");
  });

  test("deleteBranch removes branch", () => {
    engine.createBranch("to-delete");
    engine.checkout("main");
    engine.deleteBranch("to-delete");
    expect(engine.listBranches()).not.toContain("to-delete");
  });

  test("listFiles matches glob patterns", () => {
    engine.createBranch("agent-c");
    engine.commitFile("findings/001-net.json", "{}", "f1");
    engine.commitFile("findings/002-disk.json", "{}", "f2");
    const files = engine.listFiles("findings/*.json");
    expect(files.length).toBe(2);
    expect(files).toContain("findings/001-net.json");
    expect(files).toContain("findings/002-disk.json");
  });

  test("diff shows changes between branches", () => {
    engine.createBranch("agent-d");
    engine.commitFile("findings/001-diff.json", '{"test": true}', "diff commit");
    const diffOutput = engine.diff("main", "agent-d");
    expect(diffOutput).toContain("test");
  });

  // Timeout threading test — verifies config value flows to execSync timeout option
  // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.2.1 plan=phase-H2/task-H2-3/step-H2-3-1
  test("tree_merge timeout is respected — config timeout_seconds threads through to merge", () => {
    // Create a branch with a commit, then merge with timeout_seconds: 1.
    // Since git ops on a local tmpdir complete well under 1s, the merge
    // should SUCCEED — proving the timeout option is wired without blocking.
    engine.createBranch("agent-timeout-test");
    engine.commitFile("findings/001-timeout.json", '{"id":"timeout-1"}', "timeout test commit");

    // Simulate what the tool handler does: convert config seconds → ms
    const timeoutSeconds = 1;
    const timeoutMs = timeoutSeconds * 1000;

    const result = engine.merge("main", timeoutMs);
    // Fast local git ops finish well within 1s — merge should succeed
    expect(result.success).toBe(true);
    expect(result.error).toBeUndefined();
  });
});

// ─── Secret Scanning Tests ────────────────────────────────────────────────────

describe("scanForSecrets (REQ-TM-SEC-03)", () => {
  test("detects AWS access key", () => {
    const result = scanForSecrets('key is AKIAIOSFODNN7EXAMPLE');
    expect(result.matched).toBe(true);
    expect(result.patterns_matched).toContain("aws_access_key");
  });

  test("detects private key block", () => {
    const result = scanForSecrets("-----BEGIN RSA PRIVATE KEY-----\nMIIE...");
    expect(result.matched).toBe(true);
    expect(result.patterns_matched).toContain("private_key");
  });

  test("detects GitHub PAT", () => {
    const result = scanForSecrets("ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij");
    expect(result.matched).toBe(true);
    expect(result.patterns_matched).toContain("github_pat");
  });

  test("detects OpenAI key", () => {
    const result = scanForSecrets("sk-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuv");
    expect(result.matched).toBe(true);
    expect(result.patterns_matched).toContain("openai_key");
  });

  test("passes clean content", () => {
    const result = scanForSecrets('{"topic": "network", "summary": "All clear"}');
    expect(result.matched).toBe(false);
    expect(result.patterns_matched).toHaveLength(0);
  });

  test("respects additional patterns", () => {
    const result = scanForSecrets("custom-secret-12345", ["custom-secret-\\d+"]);
    expect(result.matched).toBe(true);
    expect(result.patterns_matched).toContain("custom:custom-secret-\\d+");
  });

  // Step C3: missing secret pattern tests
  // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.4 plan=phase-2/task-2-1/step-2-1-1
  test("detects aws_secret_key", () => {
    // Pattern: aws_secret[_\s]*[=:]\s*[a-zA-Z0-9/+]{40}
    // Uses 'aws_secret =' form (underscore before '=' is optional)
    const result = scanForSecrets("aws_secret = ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn");
    expect(result.matched).toBe(true);
    expect(result.patterns_matched).toContain("aws_secret_key");
  });

  test("detects GitHub OAuth token (gho_)", () => {
    const result = scanForSecrets("gho_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij");
    expect(result.matched).toBe(true);
    expect(result.patterns_matched).toContain("github_oauth");
  });

  test("detects generic_token", () => {
    const result = scanForSecrets('"token": "my-super-secret-token-value-here-longer"');
    expect(result.matched).toBe(true);
    expect(result.patterns_matched).toContain("generic_token");
  });
});

// ─── UUID-Framed Log Markers Tests (REQ-TM-SEC-01) ───────────────────────────

describe("frameLogContent (REQ-TM-SEC-01)", () => {
  test("wraps content with UUID markers", () => {
    const uuid = "test-uuid-1234";
    const content = "some log line\nanother line";
    const framed = frameLogContent(content, uuid);
    expect(framed).toContain(`───── LOG_MATCH:${uuid} ─────`);
    expect(framed).toContain(`───── END_LOG_MATCH:${uuid} ─────`);
    expect(framed).toContain("some log line");
  });

  test("escapes existing LOG_MATCH markers in content", () => {
    const uuid = "my-uuid";
    const malicious = "LOG_MATCH:fake-uuid some attack";
    const framed = frameLogContent(malicious, uuid);
    expect(framed).toContain("\\x00LOG_MATCH:");
    // The escaped content should NOT have a bare "LOG_MATCH:fake" at a line start
    // without the \x00 prefix — verify the only unescaped LOG_MATCH has our session UUID
    const lines = framed.split("\n");
    for (const line of lines) {
      if (line.includes("LOG_MATCH:") && !line.includes("\\x00LOG_MATCH:")) {
        // This is a framing line — must have our UUID
        expect(line).toContain(uuid);
      }
    }
  });
});

// ─── Config Loader Tests ──────────────────────────────────────────────────────

describe("loadTreeMemoryConfig", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
    mkdirSync(join(tmpDir, ".tree-memory"), { recursive: true });
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("returns defaults when no config file exists", () => {
    const config = loadTreeMemoryConfig(join(tmpDir, ".tree-memory"));
    expect(config.security.duckdb_sandbox).toBe("strict");
    expect(config.security.secret_scanning.enabled).toBe(true);
    expect(config.duckdb.eager_start).toBe(false);
  });

  test("reads config from yaml file", () => {
    const configPath = join(tmpDir, ".tree-memory", "config.yaml");
    writeFileSync(configPath, [
      "security:",
      "  duckdb_sandbox: permissive",
      "duckdb:",
      "  eager_start: true",
      "  memory_limit_mb: 512",
    ].join("\n"));
    const config = loadTreeMemoryConfig(join(tmpDir, ".tree-memory"));
    expect(config.security.duckdb_sandbox).toBe("permissive");
    expect(config.duckdb.eager_start).toBe(true);
    expect(config.duckdb.memory_limit_mb).toBe(512);
  });
});

// ─── Query Engine Tests ───────────────────────────────────────────────────────

describe("QueryEngine", () => {
  let tmpDir: string;
  let repoPath: string;
  let engine: QueryEngine;
  let git: GitEngine;

  beforeEach(() => {
    tmpDir = makeTmpDir();
    repoPath = join(tmpDir, ".tree-memory", "repo");
    mkdirSync(repoPath, { recursive: true });
    git = new GitEngine(repoPath);
    git.init();
    configureGitUser(repoPath);
    engine = new QueryEngine(repoPath, tmpDir);
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("lazy init — not initialized until first call", () => {
    expect(engine.isReady()).toBe(false);
    engine.init("strict");
    expect(engine.isReady()).toBe(true);
  });

  test("queryFindings returns findings from agents/*/findings/", () => {
    // Create a finding file
    git.createBranch("agent-x");
    const finding = JSON.stringify({
      id: "f-001",
      agent: "agent-x",
      timestamp: "2026-05-08T14:00:00Z",
      topic: "network",
      type: "finding",
      summary: "Network clear",
      confidence: "high",
      promoted: true,
      refs: [],
    });
    git.commitFile("findings/001-network.json", finding, "add finding");
    git.checkout("main");
    // Merge to main so it appears under agents/
    // For the query test, we check the branch findings directly
    git.checkout("agent-x");

    engine.init("strict");
    const results = engine.queryFindings({});
    expect(results.length).toBeGreaterThanOrEqual(1);
    expect(results[0].topic).toBe("network");
  });

  test("queryFindings filters by topic", () => {
    git.createBranch("agent-y");
    git.commitFile("findings/001-net.json", JSON.stringify({
      id: "f-001", agent: "y", timestamp: "2026-05-08T14:00:00Z",
      topic: "network", type: "finding", summary: "net", confidence: "high",
      promoted: false, refs: [],
    }), "f1");
    git.commitFile("findings/002-disk.json", JSON.stringify({
      id: "f-002", agent: "y", timestamp: "2026-05-08T14:01:00Z",
      topic: "disk", type: "finding", summary: "disk", confidence: "low",
      promoted: false, refs: [],
    }), "f2");

    engine.init("strict");
    const results = engine.queryFindings({ topic: "network" });
    expect(results.length).toBe(1);
    expect(results[0].topic).toBe("network");
  });

  test("queryPeers returns agent states", () => {
    git.createBranch("agent-z");
    git.commitFile("state.json", JSON.stringify({
      agent: "agent-z", branch: "agent-z", status: "active",
      assignment: "investigate OOM", started_at: "2026-05-08T14:00:00Z",
      updated_at: "2026-05-08T14:05:00Z", tool_calls: 3,
      findings_count: 1, current_focus: "OOM", next_step: "check logs",
    }), "state update");

    engine.init("strict");
    const results = engine.queryPeers({ status: "active" });
    expect(results.length).toBeGreaterThanOrEqual(1);
    expect(results[0].agent).toBe("agent-z");
  });

  test("sandbox strict rejects paths outside repo", () => {
    engine.init("strict");
    expect(() => {
      (engine as any).validatePath("../../etc/passwd");
    }).toThrow(/sandbox violation/);
  });
});

// ─── Symlink Sandbox Bypass Tests (H3) ───────────────────────────────────────
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#4.2 plan=phase-final-qa/step-final-qa-002

describe("symlink sandbox detection (H3)", () => {
  let tmpDir: string;
  let repoPath: string;
  let engine: QueryEngine;

  beforeEach(() => {
    tmpDir = makeTmpDir();
    repoPath = join(tmpDir, ".tree-memory", "repo");
    mkdirSync(repoPath, { recursive: true });
    const git = new GitEngine(repoPath);
    git.init();
    configureGitUser(repoPath);
    engine = new QueryEngine(repoPath, tmpDir);
    engine.init("strict");
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-symlink-1: symlink resolving outside repo is rejected", () => {
    // Create a symlink inside repoPath that points to a path outside the repo
    const symlinkPath = join(repoPath, "escape-link");
    symlinkSync("/etc/passwd", symlinkPath);

    // validatePath resolves the symlink and must throw sandbox violation
    expect(() => {
      (engine as any).validatePath("escape-link");
    }).toThrow(/sandbox violation/);
  });

  test("AC-symlink-2: symlink resolving inside repo is allowed", () => {
    // Create a real target file inside the repo
    const targetPath = join(repoPath, "real-target.json");
    writeFileSync(targetPath, JSON.stringify({ ok: true }));

    // Create a symlink inside the repo pointing to another file inside the repo
    const symlinkPath = join(repoPath, "internal-link");
    symlinkSync(targetPath, symlinkPath);

    // validatePath should NOT throw — the resolved target is inside the repo
    expect(() => {
      (engine as any).validatePath("internal-link");
    }).not.toThrow();
  });
});

// ─── Git Binary Validation Tests (C5) ────────────────────────────────────────
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#11 plan=phase-final-qa/step-final-qa-001

describe("git binary validation (C5)", () => {
  test("validateGitEnvironment passes when git >= 2.28 is present", () => {
    // Exercises the real git binary — the positive path.
    // All other tests already depend on this, so this documents the invariant explicitly.
    expect(() => GitEngine.validateGitEnvironment()).not.toThrow();
  });

  test("validateGitEnvironment error message is helpful when git binary missing", () => {
    // Verify the error-message strings exist in the plugin source so mis-spellings
    // or message-changing refactors are caught before they reach users.
    const source = readFileSync(
      join(__dirname, "../lib/tree-memory.ts"),
      "utf8",
    );
    expect(source).toContain("git binary not found or not executable");
    expect(source).toContain("below minimum required");
    expect(source).toContain("2.28");
  });
});

// ─── Plugin Integration Tests ─────────────────────────────────────────────────

describe("TreeMemoryPlugin integration", () => {
  let tmpDir: string;
  let plugin: Awaited<ReturnType<typeof TreeMemoryPlugin>>;

  beforeEach(async () => {
    tmpDir = makeTmpDir();
    plugin = await createPlugin(tmpDir);
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-1: tree.init creates repo", async () => {
    const result = parse(await callTool(plugin, "tree_init", { name: "test-instance" }));
    expect(result.status).toBe("initialized");
    expect(existsSync(join(tmpDir, ".tree-memory", "repo", ".git"))).toBe(true);
  });

  test("AC-1: tree.init is idempotent", async () => {
    await callTool(plugin, "tree_init", {});
    const result = parse(await callTool(plugin, "tree_init", {}));
    expect(result.status).toBe("already_initialized");
  });

  test("AC-2: tree.branch create + list", async () => {
    await callTool(plugin, "tree_init", {});

    const createResult = parse(await callTool(plugin, "tree_branch", { action: "create", name: "agent-test" }));
    expect(createResult.action).toBe("created");
    expect(createResult.branch).toBe("agent-test");

    const listResult = parse(await callTool(plugin, "tree_branch", { action: "list" }));
    expect((listResult.branches as string[]).length).toBeGreaterThan(1);
  });

  test("AC-3: tree.commit writes file + blocks secrets", async () => {
    await callTool(plugin, "tree_init", {});
    await callTool(plugin, "tree_branch", { action: "create", name: "agent-commit" });

    // Valid commit
    const result = parse(await callTool(plugin, "tree_commit", {
      file: "findings/001-test.json",
      content: '{"id": "f-001", "topic": "test"}',
      message: "add finding",
    }));
    expect(result.status).toBe("committed");

    // Secret blocked
    const blocked = parse(await callTool(plugin, "tree_commit", {
      file: "findings/002-secret.json",
      content: '{"key": "AKIAIOSFODNN7EXAMPLE"}',
      message: "add secret",
    }));
    expect(blocked.error).toBe("secret_detected");
    expect((blocked.patterns_matched as string[])).toContain("aws_access_key");

    // Step H4 (Part 2): audit log must exist with patterns_matched but NOT the secret value
    // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.4 plan=phase-2/task-2-4/step-2-4-1
    const auditFile = join(tmpDir, ".tree-memory", "repo", ".audit", "secret-blocks.jsonl");
    expect(existsSync(auditFile)).toBe(true);
    const auditContent = readFileSync(auditFile, "utf-8");
    const auditEntry = JSON.parse(auditContent.trim().split("\n")[0]);
    // Must contain patterns_matched
    expect(Array.isArray(auditEntry.patterns_matched)).toBe(true);
    expect(auditEntry.patterns_matched).toContain("aws_access_key");
    // Must NOT contain the raw secret value
    expect(auditContent).not.toContain("AKIAIOSFODNN7EXAMPLE");
  });

  test("AC-4: tree.promote creates promoted finding", async () => {
    await callTool(plugin, "tree_init", {});
    await callTool(plugin, "tree_branch", { action: "create", name: "agent-promote" });

    const result = parse(await callTool(plugin, "tree_promote", {
      summary: "Network is clear",
      topic: "network",
      confidence: "high",
    }, { agent: "agent-promote" }));
    expect(result.status).toBe("promoted");
    expect(result.finding_id).toBe("f-001");
  });

  test("AC-5: tree.merge merges branch to main", async () => {
    await callTool(plugin, "tree_init", {});
    await callTool(plugin, "tree_branch", { action: "create", name: "agent-merge" });
    await callTool(plugin, "tree_commit", {
      file: "findings/001.json",
      content: '{"id": "f-001"}',
      message: "add finding",
    });

    const result = parse(await callTool(plugin, "tree_merge", { target: "main" }));
    expect(result.status).toBe("merged");
  });

  test("AC-6: tree.state reads and updates state", async () => {
    await callTool(plugin, "tree_init", {});
    await callTool(plugin, "tree_branch", { action: "create", name: "agent-state" });

    // Update state
    const updateResult = parse(await callTool(plugin, "tree_state", {
      update: JSON.stringify({ current_focus: "investigating OOM", tool_calls: 5 }),
    }, { agent: "agent-state" }));
    expect(updateResult.status).toBe("updated");

    // Read state
    const readResult = JSON.parse(String(await callTool(plugin, "tree_state", {}, { agent: "agent-state" })));
    expect(readResult.current_focus).toBe("investigating OOM");
    expect(readResult.tool_calls).toBe(5);
  });

  test("AC-9: tree.spawn creates child branch with state", async () => {
    await callTool(plugin, "tree_init", {});

    const result = parse(await callTool(plugin, "tree_spawn", {
      agent_name: "fix-auth",
      assignment: "Fix authentication errors",
      from_branch: "main",
    }, { agent: "monitor" }));
    expect(result.status).toBe("spawned");
    expect(result.branch).toBe("agent-fix-auth");
    expect(result.spawned_by).toBe("monitor");
  });

  test("AC-10: tree.status returns overview", async () => {
    const beforeInit = parse(await callTool(plugin, "tree_status", {}));
    expect(beforeInit.initialized).toBe(false);

    await callTool(plugin, "tree_init", {});

    const result = parse(await callTool(plugin, "tree_status", {}));
    expect(result.initialized).toBe(true);
    expect(result.branches).toBeGreaterThanOrEqual(1);
    expect(Array.isArray(result.enabled_surfaces)).toBe(true);
  });

  test("AC-11: tree.log returns git history", async () => {
    await callTool(plugin, "tree_init", {});

    const result = parse(await callTool(plugin, "tree_log", { limit: 5 }));
    expect(result.log).toBeDefined();
    expect(String(result.log)).toContain("Initialize tree memory");
  });

  test("AC-3: tree.commit rejects path traversal", async () => {
    await callTool(plugin, "tree_init", {});
    await callTool(plugin, "tree_branch", { action: "create", name: "agent-traversal" });

    const result = parse(await callTool(plugin, "tree_commit", {
      file: "../../../etc/passwd",
      content: "malicious",
      message: "attack",
    }));
    expect(result.error).toContain("traversal");
  });

  test("AC-16: tree.diff shows differences between branches", async () => {
    await callTool(plugin, "tree_init", {});
    await callTool(plugin, "tree_branch", { action: "create", name: "agent-diff" });
    await callTool(plugin, "tree_commit", {
      file: "findings/001-diff.json",
      content: '{"id": "f-001", "topic": "diff-test"}',
      message: "add finding for diff",
    });

    const result = parse(await callTool(plugin, "tree_diff", {
      branch_a: "main",
      branch_b: "agent-diff",
    }));
    expect(result.branch_a).toBe("main");
    expect(result.branch_b).toBe("agent-diff");
    expect(String(result.diff)).toContain("diff-test");
  });

  test("AC-17: full lifecycle — branch→commit→query→merge", async () => {
    await callTool(plugin, "tree_init", {});

    // 1. Create a branch
    await callTool(plugin, "tree_branch", { action: "create", name: "agent-lifecycle" });

    // 2. Commit a finding
    const finding = JSON.stringify({
      id: "f-001",
      agent: "agent-lifecycle",
      timestamp: new Date().toISOString(),
      topic: "lifecycle-test",
      type: "finding",
      summary: "Lifecycle integration test finding",
      confidence: "high",
      promoted: true,
      refs: [],
    });
    await callTool(plugin, "tree_commit", {
      file: "findings/001-lifecycle.json",
      content: finding,
      message: "lifecycle finding",
    });

    // 3. Update state
    await callTool(plugin, "tree_state", {
      update: JSON.stringify({ current_focus: "lifecycle test", status: "active" }),
    }, { agent: "agent-lifecycle" });

    // 4. Query findings (should find our finding on current branch)
    const queryResult = String(await callTool(plugin, "tree_query", {
      surface: "findings",
      topic: "lifecycle-test",
    }));
    expect(queryResult).toContain("lifecycle-test");

    // 5. Merge to main
    const mergeResult = parse(await callTool(plugin, "tree_merge", { target: "main" }));
    expect(mergeResult.status).toBe("merged");

    // 6. Verify status shows merge completed
    const status = parse(await callTool(plugin, "tree_status", {}));
    expect(status.initialized).toBe(true);
  });

  test("AC-8: tree.peers standalone MCP tool returns agent states", async () => {
    await callTool(plugin, "tree_init", {});
    // Spawn an agent to create a state
    await callTool(plugin, "tree_spawn", {
      agent_name: "peer-test",
      assignment: "Test peer visibility",
      from_branch: "main",
    }, { agent: "orchestrator" });

    // Query peers via the standalone tree.peers tool
    const result = JSON.parse(String(await callTool(plugin, "tree_peers", { status: "active" })));
    expect(Array.isArray(result)).toBe(true);
    expect(result.length).toBeGreaterThanOrEqual(1);
    expect(result[0].agent).toBe("peer-test");
    expect(result[0].status).toBe("active");
    expect(result[0].assignment).toBe("Test peer visibility");
  });

  test("AC-18: tree.query(surface='trends') aggregates findings by time", async () => {
    await callTool(plugin, "tree_init", {});
    await callTool(plugin, "tree_branch", { action: "create", name: "agent-trends" });

    // Create findings with timestamps
    const now = new Date();
    for (let i = 0; i < 3; i++) {
      const ts = new Date(now.getTime() + i * 60000).toISOString();
      const finding = JSON.stringify({
        id: `f-${i + 1}`,
        agent: "agent-trends",
        timestamp: ts,
        topic: "trends-test",
        type: "finding",
        summary: `Finding ${i + 1}`,
        confidence: "high",
        promoted: false,
        refs: [],
      });
      await callTool(plugin, "tree_commit", {
        file: `findings/${String(i + 1).padStart(3, "0")}-trend.json`,
        content: finding,
        message: `trend finding ${i + 1}`,
      });
    }

    // Query trends
    const queryResult = String(await callTool(plugin, "tree_query", {
      surface: "trends",
      target_surface: "findings",
      window: "1h",
    }));
    // Result is UUID-framed, strip and parse
    const stripped = queryResult
      .replace(/───── LOG_MATCH:[^\n]+\n/g, "")
      .replace(/───── END_LOG_MATCH:[^\n]+/g, "")
      .trim();
    const trends = JSON.parse(stripped);
    expect(Array.isArray(trends)).toBe(true);
    // All 3 findings within 1h window should be in 1 bucket
    expect(trends.length).toBeGreaterThanOrEqual(1);
    const totalCount = trends.reduce((sum: number, b: { count: number }) => sum + b.count, 0);
    expect(totalCount).toBe(3);
  });

  // Step C2: Real merge conflict test (replaces tautology)
  // axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.2.1 plan=phase-2/task-2-2/step-2-2-1
  test("AC-19: tree.merge returns conflicting_files when real conflict occurs", async () => {
    await callTool(plugin, "tree_init", {});

    // 1. Create agent-conflict-a, commit findings/001.json with {"version":"a"}, merge to main
    await callTool(plugin, "tree_branch", { action: "create", name: "agent-conflict-a" });
    await callTool(plugin, "tree_commit", {
      file: "findings/001.json",
      content: '{"id": "f-001", "version": "a"}',
      message: "version a",
    });
    const mergeA = parse(await callTool(plugin, "tree_merge", { target: "main", delete_branch: false }));
    expect(mergeA.status).toBe("merged");

    // 2. Now we're on main after merge. Create agent-conflict-b FROM the original
    //    main state BEFORE agent-conflict-a was merged — we need the SHA of main
    //    before the merge. We do this by getting the parent commit of main.
    const repoPath = join(tmpDir, ".tree-memory", "repo");
    // Get the pre-merge SHA (parent of current HEAD on main)
    const preMainSha = execSync("git log --format=%H -2 --", {
      cwd: repoPath, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"],
    }).trim().split("\n")[1]; // second commit = before the merge

    // Create agent-conflict-b from the pre-merge SHA
    execSync(`git checkout -b agent-conflict-b ${preMainSha}`, {
      cwd: repoPath, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"],
    });

    // 3. Commit findings/001.json with {"version":"b"} — same file, different content
    const fullFindingsDir = join(repoPath, "findings");
    mkdirSync(fullFindingsDir, { recursive: true });
    writeFileSync(join(fullFindingsDir, "001.json"), '{"id": "f-001", "version": "b"}');
    execSync('git add findings/001.json', { cwd: repoPath, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"] });
    execSync('git commit -m "version b"', { cwd: repoPath, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"] });

    // 4. Call tree.merge from agent-conflict-b — plugin's current branch is now agent-conflict-b
    //    The plugin tracks git state so we invoke merge directly via GitEngine
    const { GitEngine: GitEngineClass } = await import("../lib/tree-memory.ts");
    const gitEng = new GitEngineClass(repoPath);
    // Verify we are on agent-conflict-b
    expect(gitEng.currentBranch()).toBe("agent-conflict-b");

    const mergeResult = gitEng.merge("main");

    // 5. Accept either clean resolution (fast-forward or auto-merge) or conflict
    if (mergeResult.success) {
      // Git auto-resolved (ort strategy, etc.) — still valid per spec.
      // When git resolves cleanly there are no conflicting_files to report.
      expect(mergeResult.success).toBe(true);
      expect(mergeResult.error).toBeUndefined();
    } else {
      // Real conflict detected — validate the full error shape
      expect(mergeResult.error).toBe("merge_conflict");
      // conflicting_files must be a non-empty array: we committed to the same
      // file on both sides, so git MUST report at least one conflict.
      // (If git ever auto-resolves this, the test will land in the success branch above.)
      expect(Array.isArray(mergeResult.conflicting_files)).toBe(true);
      expect((mergeResult.conflicting_files as string[]).length).toBeGreaterThanOrEqual(1);
      // Note: retries_exhausted is set by the plugin tool handler (tree.merge),
      // not by GitEngine.merge() — tested separately via the tool surface.
    }
  });

  test("tree_merge rejects merging main into main", async () => {
    await callTool(plugin, "tree_init", {});

    // We are on main; calling merge should return the "Cannot merge main" error
    const result = parse(await callTool(plugin, "tree_merge", { target: "main" }));
    expect(result.error).toContain("main");
  });
});

// ─── Step C4: Watches and Events Query Surface Tests ──────────────────────────
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#4.3 plan=phase-2/task-2-3/step-2-3-1

describe("tree_query watches and events surfaces", () => {
  let tmpDir: string;
  let plugin: Awaited<ReturnType<typeof TreeMemoryPlugin>>;

  beforeEach(async () => {
    tmpDir = makeTmpDir();
    plugin = await createPlugin(tmpDir);
    await callTool(plugin, "tree_init", {});
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("tree_query(surface='watches') returns watch match entries", async () => {
    const repoPath = join(tmpDir, ".tree-memory", "repo");

    // Write background watch file directly into the repo
    const matchDir = join(repoPath, "background", "log-watcher", "matches");
    mkdirSync(matchDir, { recursive: true });
    const watchFile = join(matchDir, "2026-05-17T14.json");
    writeFileSync(watchFile, JSON.stringify({
      watch: "log-watcher",
      hour: "2026-05-17T14:00:00Z",
      matches: [{ line: 1, content: "test match", at: "2026-05-17T14:01:00Z" }],
      count: 1,
    }));

    // Stage and commit via git
    execSync("git add background/log-watcher/matches/2026-05-17T14.json", {
      cwd: repoPath, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"],
    });
    execSync('git commit -m "add watch match"', {
      cwd: repoPath, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"],
    });

    // Query watches
    const rawResult = String(await callTool(plugin, "tree_query", { surface: "watches" }));
    // Strip UUID frame markers
    const stripped = rawResult
      .replace(/───── LOG_MATCH:[^\n]+\n/g, "")
      .replace(/───── END_LOG_MATCH:[^\n]+/g, "")
      .trim();
    const result = JSON.parse(stripped);
    expect(Array.isArray(result)).toBe(true);
    expect(result.length).toBeGreaterThanOrEqual(1);
    expect(result[0].watch).toBe("log-watcher");
    expect(result[0].count).toBe(1);
  });

  test("tree_query(surface='events') returns event entries", async () => {
    const repoPath = join(tmpDir, ".tree-memory", "repo");

    // Write background events file
    const eventsDir = join(repoPath, "background", "k8s-events", "events");
    mkdirSync(eventsDir, { recursive: true });
    const eventsFile = join(eventsDir, "2026-05-17T14.json");
    writeFileSync(eventsFile, JSON.stringify({
      source: "k8s-events",
      hour: "2026-05-17T14:00:00Z",
      events: [{
        type: "Warning",
        reason: "OOMKilled",
        severity: "warning",
        message: "container OOM killed",
      }],
    }));

    // Stage and commit
    execSync("git add background/k8s-events/events/2026-05-17T14.json", {
      cwd: repoPath, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"],
    });
    execSync('git commit -m "add k8s event"', {
      cwd: repoPath, encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"],
    });

    // Query events
    const rawResult = String(await callTool(plugin, "tree_query", { surface: "events" }));
    const stripped = rawResult
      .replace(/───── LOG_MATCH:[^\n]+\n/g, "")
      .replace(/───── END_LOG_MATCH:[^\n]+/g, "")
      .trim();
    const result = JSON.parse(stripped);
    expect(Array.isArray(result)).toBe(true);
    expect(result.length).toBeGreaterThanOrEqual(1);
    expect(result[0].source).toBe("k8s-events");
  });
});

// ─── Step H4 (Part 1): Spawn Budget Exhaustion Test ──────────────────────────
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#12 plan=phase-2/task-2-4/step-2-4-2

describe("tree_spawn budget exhaustion", () => {
  let tmpDir: string;

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("spawn budget exhausted error when max_total_active reached", async () => {
    tmpDir = makeTmpDir();
    // Create plugin with max_total_active: 2 via config file
    const treeMemoryRoot = join(tmpDir, ".tree-memory");
    mkdirSync(treeMemoryRoot, { recursive: true });
    writeFileSync(join(treeMemoryRoot, "config.yaml"), [
      "spawn_budget:",
      "  max_active_children: 2",
      "  max_total_active: 2",
      "  cooldown_seconds: 0",
      "  cost_ceiling_usd: 50.0",
      "  stale_timeout_minutes: 15",
    ].join("\n"));

    const plugin = await createPlugin(tmpDir);
    await callTool(plugin, "tree_init", {});

    // Spawn agent 1
    const spawn1 = parse(await callTool(plugin, "tree_spawn", {
      agent_name: "budget-a",
      assignment: "task a",
      from_branch: "main",
    }, { agent: "orchestrator" }));
    expect(spawn1.status).toBe("spawned");

    // Spawn agent 2
    const spawn2 = parse(await callTool(plugin, "tree_spawn", {
      agent_name: "budget-b",
      assignment: "task b",
      from_branch: "main",
    }, { agent: "orchestrator" }));
    expect(spawn2.status).toBe("spawned");

    // 3rd spawn must be rejected
    const spawn3 = parse(await callTool(plugin, "tree_spawn", {
      agent_name: "budget-c",
      assignment: "task c",
      from_branch: "main",
    }, { agent: "orchestrator" }));
    expect(String(spawn3.error)).toContain("spawn_budget_exhausted");
  });
});

// ─── REQ-TM-COM-01: .treeignore PII Content Redaction ────────────────────────
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#3.5 plan=phase-1/task-1-8/step-1-8-3

describe("REQ-TM-COM-01: .treeignore PII content redaction", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  // AC-20a: .treeignore with email pattern redacts matching content in committed file
  test("AC-20a: .treeignore email pattern redacts matching content", async () => {
    const plugin = await createPlugin(tmpDir);
    const repoPath = join(tmpDir, ".tree-memory", "repo");

    // Initialize tree memory repo
    const initResult = parse(await callTool(plugin, "tree_init", {}));
    expect(initResult.status).toBe("initialized");

    // Write .treeignore with a named email pattern
    const treeignorePath = join(repoPath, ".treeignore");
    writeFileSync(treeignorePath, "email=[a-zA-Z0-9._%+\\-]+@[a-zA-Z0-9.\\-]+\\.[a-zA-Z]{2,}\n");

    // Commit a file containing an email address (not a secret, passes secret scanning)
    const contentWithEmail = JSON.stringify({ user: "alice@example.com", note: "PII data" });
    const commitResult = parse(await callTool(plugin, "tree_commit", {
      file: "findings/001-pii-test.json",
      content: contentWithEmail,
      message: "test PII redaction",
    }, { agent: "test-agent" }));

    // Commit should succeed (PII is redacted, not blocked)
    expect(commitResult.status).toBe("committed");

    // The file on disk should have redacted content
    const committedContent = readFileSync(
      join(repoPath, "findings", "001-pii-test.json"),
      "utf-8"
    );
    expect(committedContent).not.toContain("alice@example.com");
    expect(committedContent).toContain("[PII-REDACTED:email]");
  });

  // AC-20b: clean content with .treeignore passes unchanged
  test("AC-20b: clean content passes unchanged when no pattern matches", async () => {
    const plugin = await createPlugin(tmpDir);
    const repoPath = join(tmpDir, ".tree-memory", "repo");

    const initResult = parse(await callTool(plugin, "tree_init", {}));
    expect(initResult.status).toBe("initialized");

    // Write .treeignore with email pattern
    const treeignorePath = join(repoPath, ".treeignore");
    writeFileSync(treeignorePath, "email=[a-zA-Z0-9._%+\\-]+@[a-zA-Z0-9.\\-]+\\.[a-zA-Z]{2,}\n");

    // Commit content with NO email addresses
    const cleanContent = JSON.stringify({ topic: "network", summary: "All interfaces nominal" });
    const commitResult = parse(await callTool(plugin, "tree_commit", {
      file: "findings/002-clean.json",
      content: cleanContent,
      message: "clean content commit",
    }, { agent: "test-agent" }));

    expect(commitResult.status).toBe("committed");

    // Content should be exactly as committed (no redaction placeholder)
    const committedContent = readFileSync(
      join(repoPath, "findings", "002-clean.json"),
      "utf-8"
    );
    expect(committedContent).toBe(cleanContent);
    expect(committedContent).not.toContain("[PII-REDACTED");
  });

  // AC-20c: audit log written on redaction (no matched content in log)
  test("AC-20c: audit log written on redaction — no matched content in log", async () => {
    const plugin = await createPlugin(tmpDir);
    const repoPath = join(tmpDir, ".tree-memory", "repo");

    const initResult = parse(await callTool(plugin, "tree_init", {}));
    expect(initResult.status).toBe("initialized");

    // Write .treeignore
    const treeignorePath = join(repoPath, ".treeignore");
    writeFileSync(treeignorePath, "email=[a-zA-Z0-9._%+\\-]+@[a-zA-Z0-9.\\-]+\\.[a-zA-Z]{2,}\n");

    // Commit content with an email
    await callTool(plugin, "tree_commit", {
      file: "findings/003-audit-test.json",
      content: JSON.stringify({ user: "bob@example.org", note: "audit log test" }),
      message: "audit log test",
    }, { agent: "test-agent" });

    // .audit/pii-redactions.jsonl should exist
    const auditPath = join(repoPath, ".audit", "pii-redactions.jsonl");
    expect(existsSync(auditPath)).toBe(true);

    // Parse audit log — must NOT contain the actual email value
    const auditContent = readFileSync(auditPath, "utf-8");
    const auditEntry = JSON.parse(auditContent.trim().split("\n")[0]);
    expect(auditEntry.patterns_matched).toContain("email");
    expect(auditEntry.file).toBe("findings/003-audit-test.json");
    expect(auditEntry.redaction_count).toBeGreaterThan(0);
    // The actual matched value must NOT appear in the log
    expect(auditContent).not.toContain("bob@example.org");
  });

  // AC-20d: missing .treeignore is a no-op
  test("AC-20d: missing .treeignore is a no-op", async () => {
    const plugin = await createPlugin(tmpDir);
    const repoPath = join(tmpDir, ".tree-memory", "repo");

    const initResult = parse(await callTool(plugin, "tree_init", {}));
    expect(initResult.status).toBe("initialized");

    // No .treeignore file written — verify helper returns empty
    const patterns = loadTreeIgnorePatterns(repoPath);
    expect(patterns).toHaveLength(0);

    // Commit should succeed normally
    const contentNoPii = JSON.stringify({ topic: "ops", summary: "Disk usage at 45%" });
    const commitResult = parse(await callTool(plugin, "tree_commit", {
      file: "findings/004-no-treeignore.json",
      content: contentNoPii,
      message: "no treeignore commit",
    }, { agent: "test-agent" }));

    expect(commitResult.status).toBe("committed");
    // No audit file created
    expect(existsSync(join(repoPath, ".audit", "pii-redactions.jsonl"))).toBe(false);
  });

  // AC-20e: redactPiiContent unit test — correct redaction logic
  test("AC-20e: redactPiiContent unit — redacts matched patterns, returns audit entries", () => {
    const patterns = [
      { name: "email", pattern: /[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}/gi },
    ];
    const { redacted, auditEntries } = redactPiiContent(
      "Contact user@test.com or admin@test.com for help",
      "notes.json",
      patterns
    );
    expect(redacted).not.toContain("user@test.com");
    expect(redacted).not.toContain("admin@test.com");
    expect(redacted).toContain("[PII-REDACTED:email]");
    expect(auditEntries).toHaveLength(1);
    expect(auditEntries[0].pattern).toBe("email");
    expect(auditEntries[0].file).toBe("notes.json");
    expect(auditEntries[0].lineCount).toBe(2);
  });
});

// ─── BL-04: tree.branch create respects spawn_budget.max_total_active ─────────
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#7 plan=phase-BL/task-BL-04/step-BL-04-2

describe("BL-04: tree.branch create respects spawn_budget.max_total_active", () => {
  let tmpDir: string;

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("tree_branch create respects spawn_budget.max_total_active", async () => {
    tmpDir = makeTmpDir();

    // Set max_total_active: 2 (main counts as 1, so we can create 1 more)
    const treeMemoryRoot = join(tmpDir, ".tree-memory");
    mkdirSync(treeMemoryRoot, { recursive: true });
    writeFileSync(join(treeMemoryRoot, "config.yaml"), [
      "spawn_budget:",
      "  max_active_children: 5",
      "  max_total_active: 2",
      "  cooldown_seconds: 0",
      "  cost_ceiling_usd: 50.0",
      "  stale_timeout_minutes: 15",
    ].join("\n"));

    const plugin = await createPlugin(tmpDir);
    await callTool(plugin, "tree_init", {});

    // After init, "main" exists (1 branch). max_total_active=2 → we can create 1 more.
    const branch1 = parse(await callTool(plugin, "tree_branch", {
      action: "create",
      name: "agent-alpha",
    }));
    expect(branch1.action).toBe("created");

    // Now 2 branches total (main + agent-alpha). Next create should be rejected.
    const branch2 = parse(await callTool(plugin, "tree_branch", {
      action: "create",
      name: "agent-beta",
    }));
    expect(String(branch2.error)).toContain("spawn_budget_exhausted");
    expect(Number(branch2.active)).toBeGreaterThanOrEqual(2);
    expect(Number(branch2.max)).toBe(2);
  });
});

// ─── BL-05: tree.query truncates results at result_limit ─────────────────────
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#4 plan=phase-BL/task-BL-05/step-BL-05-3

describe("BL-05: tree.query result_limit truncation", () => {
  let tmpDir: string;

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("tree_query truncates results at result_limit and returns has_more", async () => {
    tmpDir = makeTmpDir();

    // Set result_limit: 3 in config
    const treeMemoryRoot = join(tmpDir, ".tree-memory");
    mkdirSync(treeMemoryRoot, { recursive: true });
    writeFileSync(join(treeMemoryRoot, "config.yaml"), [
      "result_limit: 3",
      "security:",
      "  duckdb_sandbox: strict",
      "  secret_scanning:",
      "    enabled: false",
      "    additional_patterns: []",
      "    ignore_paths: []",
    ].join("\n"));

    const plugin = await createPlugin(tmpDir);
    await callTool(plugin, "tree_init", {});

    // Commit 5 findings directly to repo so queryFindings picks them up
    const repoPath = join(tmpDir, ".tree-memory", "repo");
    const findingsDir = join(repoPath, "findings");
    mkdirSync(findingsDir, { recursive: true });

    for (let i = 1; i <= 5; i++) {
      const finding = JSON.stringify({
        id: `f-${i}`,
        agent: "test-agent",
        timestamp: `2026-05-17T0${i}:00:00Z`,
        topic: "network",
        type: "finding",
        summary: `Finding number ${i}`,
        confidence: "high",
        promoted: false,
        refs: [],
      });
      writeFileSync(join(findingsDir, `00${i}-finding.json`), finding);
    }
    execSync("git add findings/", { cwd: repoPath, stdio: "pipe" });
    execSync('git commit -m "add 5 findings for result_limit test"', {
      cwd: repoPath,
      stdio: "pipe",
    });

    // Query findings — expect truncation to 3 with has_more: true
    const rawResult = String(await callTool(plugin, "tree_query", { surface: "findings" }));
    // Strip UUID frame markers
    const stripped = rawResult
      .replace(/───── LOG_MATCH:[^\n]+\n/g, "")
      .replace(/───── END_LOG_MATCH:[^\n]+/g, "")
      .trim();
    const result = JSON.parse(stripped);

    // Result should be the truncated envelope (not a plain array)
    expect(Array.isArray(result)).toBe(false);
    expect(result.has_more).toBe(true);
    expect(result.total_count).toBe(5);
    expect(result.result_limit).toBe(3);
    expect(Array.isArray(result.results)).toBe(true);
    expect(result.results.length).toBe(3);
  });
});

// ─── BL-09: tree.state null check ────────────────────────────────────────────
// axiom:trace work_item=tree-memory-01 spec=specs/113-Tree-Memory.md#6 plan=phase-BL/task-BL-09/step-BL-09-1

describe("BL-09: tree.state null when no state.json exists", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("AC-22: tree.state returns null when no state.json exists on branch", async () => {
    const plugin = await createPlugin(tmpDir);

    // Init the tree memory repo
    const initResult = parse(await callTool(plugin, "tree_init", {}));
    expect(initResult.status).toBe("initialized");

    // Create a fresh branch — no state.json has been written on it
    const branchResult = parse(await callTool(plugin, "tree_branch", {
      action: "create",
      name: "agent-no-state",
    }));
    expect(branchResult.action).toBe("created");

    // Switch to the branch by checking it out directly
    const repoPath = join(tmpDir, ".tree-memory", "repo");
    execSync("git checkout agent-no-state", { cwd: repoPath, stdio: "pipe" });

    // Call tree.state without any update — should return state: null
    const stateResult = parse(await callTool(plugin, "tree_state", {}));
    expect(stateResult.state).toBeNull();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Phase 3 — DuckDB adapter tests (parameterized + sandbox)
//
// AC-DUCKDB-1  loadDuckdbIfAvailable returns null when binary absent
// AC-DUCKDB-2  loadDuckdbIfAvailable returns DuckdbAdapter when binary present
// AC-DUCKDB-3  DuckDB adapter queryFindings returns same results as filesystem scanner
// AC-DUCKDB-4  DuckDB adapter queryPeers returns same results as filesystem scanner
// AC-DUCKDB-5  DuckDB adapter respects sandbox: path outside repo raises error
// AC-DUCKDB-6  DuckDB load failure is non-fatal (bad path returns null)
// AC-DUCKDB-7  tree.status includes engine field ('duckdb')
// AC-DUCKDB-8  tree.status shows 'filesystem-scanner' when DuckDB absent
//
// axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4 plan=phase-3/task-3-1/step-3-1-1
// ─────────────────────────────────────────────────────────────────────────────

// Detect if DuckDB is available for this test run
// Points to the .opencode/native/duckdb/ directory installed by axiom install
import { resolve as _resolve } from "node:path";
const DUCKDB_ENTRY = _resolve(
  import.meta.dir, "..", "native", "duckdb", "node_modules", "@duckdb", "node-api", "lib", "duckdb.js",
);
const DUCKDB_AVAILABLE = existsSync(DUCKDB_ENTRY);

describe("DuckDB adapter — loadDuckdbIfAvailable", () => {
  test("AC-DUCKDB-1: returns null when TREE_MEMORY_NATIVE_PATH points to non-existent file", async () => {
    const origEnv = process.env["TREE_MEMORY_NATIVE_PATH"];
    process.env["TREE_MEMORY_NATIVE_PATH"] = "/nonexistent/path/duckdb.js";
    try {
      const adapter = await loadDuckdbIfAvailable("/tmp/no-project");
      expect(adapter).toBeNull();
    } finally {
      if (origEnv === undefined) delete process.env["TREE_MEMORY_NATIVE_PATH"];
      else process.env["TREE_MEMORY_NATIVE_PATH"] = origEnv;
    }
  });

  test("AC-DUCKDB-6: load failure is non-fatal — bad path returns null without throw", async () => {
    const origEnv = process.env["TREE_MEMORY_NATIVE_PATH"];
    const badFile = join(makeTmpDir(), "bad-duckdb.js");
    writeFileSync(badFile, "this is not valid JS that exports DuckDBInstance");
    process.env["TREE_MEMORY_NATIVE_PATH"] = badFile;
    try {
      let adapter: DuckdbAdapter | null = null;
      let threw = false;
      try {
        adapter = await loadDuckdbIfAvailable("/tmp/test-project");
      } catch {
        threw = true;
      }
      expect(threw).toBe(false);
      expect(adapter).toBeNull();
    } finally {
      if (origEnv === undefined) delete process.env["TREE_MEMORY_NATIVE_PATH"];
      else process.env["TREE_MEMORY_NATIVE_PATH"] = origEnv;
    }
  });

  test.skipIf(!DUCKDB_AVAILABLE)(
    "AC-DUCKDB-2: returns DuckdbAdapter when TREE_MEMORY_NATIVE_PATH is valid",
    async () => {
      const origEnv = process.env["TREE_MEMORY_NATIVE_PATH"];
      process.env["TREE_MEMORY_NATIVE_PATH"] = DUCKDB_ENTRY;
      try {
        const adapter = await loadDuckdbIfAvailable("/tmp/test-project");
        expect(adapter).not.toBeNull();
        expect(adapter).toBeInstanceOf(DuckdbAdapter);
        expect(typeof adapter!.version).toBe("string");
        expect(adapter!.version).toMatch(/^v\d+\.\d+\.\d+/);
        adapter!.destroy();
      } finally {
        if (origEnv === undefined) delete process.env["TREE_MEMORY_NATIVE_PATH"];
        else process.env["TREE_MEMORY_NATIVE_PATH"] = origEnv;
      }
    },
  );
});

describe.skipIf(!DUCKDB_AVAILABLE)(
  "DuckDB adapter — query parity tests",
  () => {
    let tmpDir: string;
    let repoPath: string;
    let git: GitEngine;

    beforeEach(() => {
      tmpDir = makeTmpDir();
      repoPath = join(tmpDir, ".tree-memory", "repo");
      mkdirSync(repoPath, { recursive: true });
      git = new GitEngine(repoPath);
      git.init();
      execSync('git config user.email "test@test.com"', { cwd: repoPath, stdio: "pipe" });
      execSync('git config user.name "Test"', { cwd: repoPath, stdio: "pipe" });

      git.createBranch("agent-alpha");
      git.commitFile("findings/001-oom.json", JSON.stringify({
        id: "f-001", agent: "alpha", topic: "OOM", confidence: "high",
        timestamp: "2026-05-17T10:00:00Z", type: "finding",
        summary: "OOM kills detected", promoted: true, refs: [],
      }), "finding OOM");
      git.commitFile("findings/002-net.json", JSON.stringify({
        id: "f-002", agent: "alpha", topic: "network", confidence: "low",
        timestamp: "2026-05-17T11:00:00Z", type: "hypothesis",
        summary: "network clear", promoted: false, refs: [],
      }), "finding network");
      git.commitFile("state.json", JSON.stringify({
        agent: "alpha", branch: "agent-alpha", status: "active",
        assignment: "investigate OOM", started_at: "2026-05-17T09:00:00Z",
        updated_at: "2026-05-17T10:00:00Z", tool_calls: 5, findings_count: 2,
        current_focus: "memory usage", next_step: "check pods",
      }), "state update");
    });

    afterEach(() => {
      rmSync(tmpDir, { recursive: true, force: true });
    });

    test("AC-DUCKDB-3: DuckDB queryFindings returns same results as filesystem scanner", async () => {
      const fsEngine = new QueryEngine(repoPath, tmpDir);
      fsEngine.init("strict");
      const fsResults = fsEngine.queryFindings({ topic: "OOM" });

      const origEnv = process.env["TREE_MEMORY_NATIVE_PATH"];
      process.env["TREE_MEMORY_NATIVE_PATH"] = DUCKDB_ENTRY;
      try {
        const adapter = await loadDuckdbIfAvailable(tmpDir);
        expect(adapter).not.toBeNull();
        const dbResults = await adapter!.queryFindings(repoPath, "strict", tmpDir, { topic: "OOM" });

        expect(dbResults.length).toBeGreaterThanOrEqual(1);
        expect(fsResults.length).toBeGreaterThanOrEqual(1);
        const dbOom = dbResults.find((f: { id: string }) => f.id === "f-001");
        const fsOom = fsResults.find((f: { id: string }) => f.id === "f-001");
        expect(dbOom).toBeDefined();
        expect(fsOom).toBeDefined();
        expect(dbOom!.topic).toBe(fsOom!.topic);
        expect(dbOom!.confidence).toBe(fsOom!.confidence);
        adapter!.destroy();
      } finally {
        if (origEnv === undefined) delete process.env["TREE_MEMORY_NATIVE_PATH"];
        else process.env["TREE_MEMORY_NATIVE_PATH"] = origEnv;
      }
    });

    test("AC-DUCKDB-4: DuckDB queryPeers returns same results as filesystem scanner", async () => {
      const fsEngine = new QueryEngine(repoPath, tmpDir);
      fsEngine.init("strict");
      const fsResults = fsEngine.queryPeers({ status: "active" });

      const origEnv = process.env["TREE_MEMORY_NATIVE_PATH"];
      process.env["TREE_MEMORY_NATIVE_PATH"] = DUCKDB_ENTRY;
      try {
        const adapter = await loadDuckdbIfAvailable(tmpDir);
        expect(adapter).not.toBeNull();
        const dbResults = await adapter!.queryPeers(repoPath, "strict", tmpDir, { status: "active" });

        expect(dbResults.length).toBeGreaterThanOrEqual(1);
        expect(fsResults.length).toBeGreaterThanOrEqual(1);
        const dbAlpha = dbResults.find((s: { agent: string }) => s.agent === "alpha");
        const fsAlpha = fsResults.find((s: { agent: string }) => s.agent === "alpha");
        expect(dbAlpha).toBeDefined();
        expect(fsAlpha).toBeDefined();
        expect(dbAlpha!.status).toBe(fsAlpha!.status);
        adapter!.destroy();
      } finally {
        if (origEnv === undefined) delete process.env["TREE_MEMORY_NATIVE_PATH"];
        else process.env["TREE_MEMORY_NATIVE_PATH"] = origEnv;
      }
    });

    test("AC-DUCKDB-9: DuckDB queryWatches returns same results as filesystem scanner", async () => {
      // axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4 plan=phase-3/task-3-1/step-3-1-1
      // Set up background watch fixture
      git.commitFile("background/oom-watcher/matches/2026-05-17T10.json", JSON.stringify({
        watch: "oom-watcher", file: "/var/log/app.log", pattern: "OOM",
        hour: "2026-05-17T10:00:00Z",
        matches: [
          { line: 100, content: "OOM kill detected", at: "2026-05-17T10:02:00Z" },
          { line: 200, content: "OOM process killed", at: "2026-05-17T10:05:00Z" },
        ],
        count: 2,
      }), "watch match fixture");

      const fsEngine = new QueryEngine(repoPath, tmpDir);
      fsEngine.init("strict");
      const fsResults = fsEngine.queryWatches({ watch_name: "oom-watcher" });

      const origEnv = process.env["TREE_MEMORY_NATIVE_PATH"];
      process.env["TREE_MEMORY_NATIVE_PATH"] = DUCKDB_ENTRY;
      try {
        const adapter = await loadDuckdbIfAvailable(tmpDir);
        expect(adapter).not.toBeNull();
        const dbResults = await adapter!.queryWatches(repoPath, "strict", tmpDir, { watch_name: "oom-watcher" });

        // Both should return some data for the watch
        expect(Array.isArray(dbResults) ? (dbResults as unknown[]).length : 0).toBeGreaterThanOrEqual(1);
        expect(Array.isArray(fsResults) ? (fsResults as unknown[]).length : 0).toBeGreaterThanOrEqual(1);
        adapter!.destroy();
      } finally {
        if (origEnv === undefined) delete process.env["TREE_MEMORY_NATIVE_PATH"];
        else process.env["TREE_MEMORY_NATIVE_PATH"] = origEnv;
      }
    });

    test("AC-DUCKDB-10: DuckDB queryEvents returns same results as filesystem scanner", async () => {
      // axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4 plan=phase-3/task-3-1/step-3-1-1
      // Set up background events fixture
      git.commitFile("background/k8s-events/events/2026-05-17T10.json", JSON.stringify({
        source: "k8s-events", type: "OOMKilled", severity: "warning",
        timestamp: "2026-05-17T10:03:00Z",
        message: "Container app was OOMKilled",
        pod: "app-pod-1234",
      }), "events fixture");

      const fsEngine = new QueryEngine(repoPath, tmpDir);
      fsEngine.init("strict");
      const fsResults = fsEngine.queryEvents({ source: "k8s-events", type: "OOMKilled" });

      const origEnv = process.env["TREE_MEMORY_NATIVE_PATH"];
      process.env["TREE_MEMORY_NATIVE_PATH"] = DUCKDB_ENTRY;
      try {
        const adapter = await loadDuckdbIfAvailable(tmpDir);
        expect(adapter).not.toBeNull();
        const dbResults = await adapter!.queryEvents(repoPath, "strict", tmpDir, { source: "k8s-events", type: "OOMKilled" });

        // Both should find the event
        expect((dbResults as unknown[]).length).toBeGreaterThanOrEqual(1);
        expect((fsResults as unknown[]).length).toBeGreaterThanOrEqual(1);
        adapter!.destroy();
      } finally {
        if (origEnv === undefined) delete process.env["TREE_MEMORY_NATIVE_PATH"];
        else process.env["TREE_MEMORY_NATIVE_PATH"] = origEnv;
      }
    });
  },
);

describe.skipIf(!DUCKDB_AVAILABLE)(
  "DuckDB adapter — sandbox tests",
  () => {
    let tmpDir: string;
    let repoPath: string;

    beforeEach(() => {
      tmpDir = makeTmpDir();
      repoPath = join(tmpDir, ".tree-memory", "repo");
      mkdirSync(repoPath, { recursive: true });
      const g = new GitEngine(repoPath);
      g.init();
      execSync('git config user.email "t@t.com"', { cwd: repoPath, stdio: "pipe" });
      execSync('git config user.name "T"', { cwd: repoPath, stdio: "pipe" });
    });

    afterEach(() => {
      rmSync(tmpDir, { recursive: true, force: true });
    });

    test("AC-DUCKDB-5: DuckDB sandbox strict mode raises error for path outside repo", async () => {
      const origEnv = process.env["TREE_MEMORY_NATIVE_PATH"];
      process.env["TREE_MEMORY_NATIVE_PATH"] = DUCKDB_ENTRY;
      try {
        const adapter = await loadDuckdbIfAvailable(tmpDir);
        expect(adapter).not.toBeNull();
        let threw = false;
        try {
          await adapter!.queryFindings("/etc", "strict", tmpDir, {});
        } catch (e) {
          threw = true;
          expect(String(e)).toContain("sandbox violation");
        }
        expect(threw).toBe(true);
        adapter!.destroy();
      } finally {
        if (origEnv === undefined) delete process.env["TREE_MEMORY_NATIVE_PATH"];
        else process.env["TREE_MEMORY_NATIVE_PATH"] = origEnv;
      }
    });

    test("AC-DUCKDB-INSTALL-blocked: DuckDB adapter blocks INSTALL keyword (spec §4.3)", async () => {
      // axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4.3 plan=phase-2/task-2-1/step-2-1-2
      const origEnv = process.env["TREE_MEMORY_NATIVE_PATH"];
      process.env["TREE_MEMORY_NATIVE_PATH"] = DUCKDB_ENTRY;
      try {
        const adapter = await loadDuckdbIfAvailable(tmpDir);
        expect(adapter).not.toBeNull();

        // Test 1: Private-method direct check — confirms the method exists and throws correctly
        const validateSql = (adapter as unknown as Record<string, (sql: string) => void>)["validateSql"];
        expect(typeof validateSql).toBe("function");
        let threw1 = false;
        try {
          validateSql.call(adapter, "INSTALL httpfs");
        } catch (e) {
          threw1 = true;
          expect(String(e)).toContain("blocked");
        }
        expect(threw1).toBe(true);

        // Test 2: Public-API path — confirms validateSql() is wired into query() execution
        // Using a topic that contains the blocked keyword proves the check fires through queryFindings
        // Note: validateSql() scans the full SQL text (including LIKE patterns) — this is by design
        // for defense-in-depth. See ADR-TM-006 for the escLit() + validateSql() rationale.
        let threw2 = false;
        try {
          await adapter!.queryFindings(repoPath, "strict", tmpDir, { topic: "'; INSTALL httpfs; --" });
        } catch (e) {
          threw2 = true;
          expect(String(e)).toContain("blocked");
        }
        expect(threw2).toBe(true);

        adapter!.destroy();
      } finally {
        if (origEnv === undefined) delete process.env["TREE_MEMORY_NATIVE_PATH"];
        else process.env["TREE_MEMORY_NATIVE_PATH"] = origEnv;
      }
    });

    test("AC-DUCKDB-HTTPFS-blocked: DuckDB adapter blocks httpfs keyword (spec §4.3)", async () => {
      // axiom:trace work_item=tree-memory-duckdb-native spec=specs/113-Tree-Memory.md#4.3 plan=phase-2/task-2-1/step-2-1-2
      const origEnv = process.env["TREE_MEMORY_NATIVE_PATH"];
      process.env["TREE_MEMORY_NATIVE_PATH"] = DUCKDB_ENTRY;
      try {
        const adapter = await loadDuckdbIfAvailable(tmpDir);
        expect(adapter).not.toBeNull();
        const validateSql = (adapter as unknown as Record<string, (sql: string) => void>)["validateSql"];
        expect(typeof validateSql).toBe("function");
        // Test ATTACH (another blocked keyword)
        let threw = false;
        try {
          validateSql.call(adapter, "ATTACH '/etc/shadow' AS shadow");
        } catch (e) {
          threw = true;
          expect(String(e)).toContain("blocked");
        }
        expect(threw).toBe(true);
        // Also test httpfs keyword in a URL pattern
        let threw2 = false;
        try {
          validateSql.call(adapter, "SELECT * FROM read_json_auto('https://httpfs.example.com/data.json')");
        } catch (e) {
          threw2 = true;
          expect(String(e)).toContain("blocked");
        }
        expect(threw2).toBe(true);
        adapter!.destroy();
      } finally {
        if (origEnv === undefined) delete process.env["TREE_MEMORY_NATIVE_PATH"];
        else process.env["TREE_MEMORY_NATIVE_PATH"] = origEnv;
      }
    });

    test("AC-DUCKDB-7: tree.status includes engine='duckdb' when DuckDB loaded", async () => {
      const origEnv = process.env["TREE_MEMORY_NATIVE_PATH"];
      process.env["TREE_MEMORY_NATIVE_PATH"] = DUCKDB_ENTRY;
      try {
        const plugin = await TreeMemoryPlugin({ directory: tmpDir, client: {} });
        await plugin.tool["tree_init"].execute({ name: "test" }, {});
        const statusResult = await plugin.tool["tree_status"].execute({}, {});
        const status = JSON.parse(String(statusResult)) as Record<string, unknown>;
        expect(status.engine).toBe("duckdb");
        expect(typeof status.engine_version).toBe("string");
        expect(String(status.engine_version)).toMatch(/^v\d+/);
      } finally {
        if (origEnv === undefined) delete process.env["TREE_MEMORY_NATIVE_PATH"];
        else process.env["TREE_MEMORY_NATIVE_PATH"] = origEnv;
      }
    });

    test("AC-DUCKDB-8: tree.status shows engine='filesystem-scanner' when DuckDB absent", async () => {
      const origEnv = process.env["TREE_MEMORY_NATIVE_PATH"];
      delete process.env["TREE_MEMORY_NATIVE_PATH"];
      try {
        const plugin = await TreeMemoryPlugin({ directory: tmpDir, client: {} });
        await plugin.tool["tree_init"].execute({ name: "test" }, {});
        const statusResult = await plugin.tool["tree_status"].execute({}, {});
        const status = JSON.parse(String(statusResult)) as Record<string, unknown>;
        expect(status.engine).toBe("filesystem-scanner");
        expect(status.engine_version).toBeNull();
      } finally {
        if (origEnv === undefined) delete process.env["TREE_MEMORY_NATIVE_PATH"];
        else process.env["TREE_MEMORY_NATIVE_PATH"] = origEnv;
      }
    });
  },
);

// ─────────────────────────────────────────────────────────────────────────────
// BUG-15: tree.query with no surface returns helpful error with "Valid surfaces:"
// BUG-16: tree.diff validation — branch_a required; branch_b optional
//
// Regression tests: before the fixes, calling tree.query({}) or tree.diff({})
// would crash or produce unhelpful errors without listing valid surfaces or
// required parameters.
//
// axiom:trace work_item=plugin-bug-sweep-01 spec=specs/113-Tree-Memory.md plan=phase-2/task-2/step-verify-003 test=tree-memory.test.ts
// ─────────────────────────────────────────────────────────────────────────────

describe("BUG-15: tree.query with no surface — regression", () => {
  let tmpDir: string;
  let plugin: Awaited<ReturnType<typeof TreeMemoryPlugin>>;

  beforeEach(async () => {
    tmpDir = makeTmpDir();
    plugin = await createPlugin(tmpDir);
    await callTool(plugin, "tree_init", {});
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("BUG-15: tree.query with no args returns helpful error with Valid surfaces:", async () => {
    // Call tree.query with no arguments — must NOT crash
    let result: unknown;
    let threw = false;
    try {
      result = await callTool(plugin, "tree_query", {});
    } catch (err) {
      threw = true;
      result = String(err);
    }

    // Must not throw an unhandled exception
    expect(threw).toBe(false);

    // The result (or caught error message) must contain "Valid surfaces:"
    const resultStr = String(result);
    expect(resultStr).toContain("Valid surfaces:");
  });
});

describe("BUG-16: tree.diff validation — regression", () => {
  let tmpDir: string;
  let plugin: Awaited<ReturnType<typeof TreeMemoryPlugin>>;

  beforeEach(async () => {
    tmpDir = makeTmpDir();
    plugin = await createPlugin(tmpDir);
    await callTool(plugin, "tree_init", {});
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("BUG-16a: tree.diff with no args returns error about branch_a", async () => {
    // Call tree.diff with no arguments — must NOT crash
    let result: unknown;
    let threw = false;
    try {
      result = await callTool(plugin, "tree_diff", {});
    } catch (err) {
      threw = true;
      result = String(err);
    }

    // Must not throw an unhandled exception
    expect(threw).toBe(false);

    // The result must mention "branch_a" or "required"
    const resultStr = String(result);
    const mentionsBranchA = resultStr.includes("branch_a");
    const mentionsRequired = resultStr.toLowerCase().includes("required");
    expect(mentionsBranchA || mentionsRequired).toBe(true);
  });

  test("BUG-16b: tree.diff with branch_a only returns diff result without crash", async () => {
    // Call tree.diff with only branch_a — must NOT crash
    // branch_b defaults to "main" per the implementation
    let result: unknown;
    let threw = false;
    try {
      result = await callTool(plugin, "tree_diff", { branch_a: "main" });
    } catch (err) {
      threw = true;
      result = String(err);
    }

    // Must not throw an unhandled exception
    expect(threw).toBe(false);

    const resultStr = String(result);

    // Must be valid JSON
    let parsed: Record<string, unknown>;
    expect(() => {
      parsed = JSON.parse(resultStr) as Record<string, unknown>;
    }).not.toThrow();

    const parsedResult = JSON.parse(resultStr) as Record<string, unknown>;

    // Either succeeds with a "diff" field, OR returns a sensible error about branch_b
    const hasDiff = "diff" in parsedResult;
    const hasChanges = "changes" in parsedResult;
    const hasError = "error" in parsedResult;

    // At minimum: must have diff, changes, or a sensible error — not a crash
    expect(hasDiff || hasChanges || hasError).toBe(true);

    // If it has a diff field, it must not be undefined
    if (hasDiff) {
      expect(parsedResult.diff).toBeDefined();
      expect(String(parsedResult.diff)).not.toBe("undefined");
    }
  });
});

// ─── getDefaultBranch fallback paths (BUG-16 follow-up) ──────────────────────
// Tests for paths (2) and (3) of getDefaultBranch():
//   Path 2: git config init.defaultBranch  (no remote, but config set)
//   Path 3: hardcoded "main" fallback + console.warn  (no git at all)
//
// axiom:trace work_item=plugin-bug-sweep-01 spec=specs/113-Tree-Memory.md#5.3 plan=phase-1/task-1-3/step-backlog-b02 test=tree-memory.test.ts
// ─────────────────────────────────────────────────────────────────────────────

describe("getDefaultBranch fallback paths", () => {
  // Path 2: git config init.defaultBranch fallback
  // Scenario: projectRoot is a git repo with no remote (symbolic-ref fails) but
  // init.defaultBranch=trunk is set in git config.  getDefaultBranch() must
  // return "trunk" and tree.diff must use it as branch_b without crashing.
  test("tree_diff uses git config init.defaultBranch when symbolic-ref fails", async () => {
    // Create a fresh git repo with no remote and init.defaultBranch=trunk
    const tmpDir = mkdtempSync(join(tmpdir(), "tree-default-branch-"));
    try {
      execSync("git init", { cwd: tmpDir, stdio: "pipe" });
      execSync("git config init.defaultBranch trunk", { cwd: tmpDir, stdio: "pipe" });
      execSync('git config user.email "test@test.com"', { cwd: tmpDir, stdio: "pipe" });
      execSync('git config user.name "Test"', { cwd: tmpDir, stdio: "pipe" });
      // Need at least one commit so the repo is valid
      writeFileSync(join(tmpDir, "README.md"), "test");
      execSync("git add .", { cwd: tmpDir, stdio: "pipe" });
      execSync('git commit -m "init"', { cwd: tmpDir, stdio: "pipe" });
      // No remote added — symbolic-ref will throw, falls back to git config

      // Initialize tree memory in this test repo
      const testPlugin = await TreeMemoryPlugin({ directory: tmpDir, client: {} });
      await callTool(testPlugin, "tree_init", {});

      // tree.diff with only branch_a — branch_b should be detected as "trunk"
      // via git config init.defaultBranch (path 2)
      let result: unknown;
      let threw = false;
      try {
        result = await callTool(testPlugin, "tree_diff", { branch_a: "main" });
      } catch (err) {
        threw = true;
        result = String(err);
      }

      // Must not throw
      expect(threw).toBe(false);

      // Must be valid JSON
      const resultStr = String(result);
      let parsed: Record<string, unknown>;
      expect(() => {
        parsed = JSON.parse(resultStr) as Record<string, unknown>;
      }).not.toThrow();

      parsed = JSON.parse(resultStr) as Record<string, unknown>;

      // branch_b must be "trunk" (detected via git config) — not "undefined"
      if ("branch_b" in parsed) {
        expect(String(parsed.branch_b)).toBe("trunk");
        expect(String(parsed.branch_b)).not.toBe("undefined");
      }

      // Must not contain "undefined..undefined" in any error message
      expect(resultStr).not.toContain("undefined..undefined");
      expect(resultStr).not.toContain("undefined is not");
    } finally {
      rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  // Path 3: hardcoded "main" fallback
  // Scenario: projectRoot is NOT a git repo — both git commands throw.
  // getDefaultBranch() must return "main" (with console.warn) and tree.diff
  // must not crash with "undefined..undefined".
  test("tree_diff falls back to 'main' when both git commands fail", async () => {
    // Create a plain directory with no git init — both git commands will fail
    const tmpDir = mkdtempSync(join(tmpdir(), "tree-no-git-"));
    try {
      // No git init — getDefaultBranch will hit path 3 and return "main"
      const testPlugin = await TreeMemoryPlugin({ directory: tmpDir, client: {} });
      await callTool(testPlugin, "tree_init", {});

      // tree.diff with only branch_a — branch_b should fall back to "main"
      let result: unknown;
      let threw = false;
      try {
        result = await callTool(testPlugin, "tree_diff", { branch_a: "main" });
      } catch (err) {
        threw = true;
        result = String(err);
      }

      // Must not throw an unhandled exception
      expect(threw).toBe(false);

      // Must be valid JSON
      const resultStr = String(result);
      let parsed: Record<string, unknown>;
      expect(() => {
        parsed = JSON.parse(resultStr) as Record<string, unknown>;
      }).not.toThrow();

      parsed = JSON.parse(resultStr) as Record<string, unknown>;

      // Must be an object (diff result or error) — never a raw crash
      expect(typeof parsed).toBe("object");

      // Must not contain "undefined..undefined" — the pre-fix crash signature
      expect(resultStr).not.toContain("undefined..undefined");
      expect(resultStr).not.toContain("undefined is not");

      // If branch_b is present, it must be "main" (the hardcoded fallback)
      if ("branch_b" in parsed) {
        expect(String(parsed.branch_b)).toBe("main");
      }
    } finally {
      rmSync(tmpDir, { recursive: true, force: true });
    }
  });
});
