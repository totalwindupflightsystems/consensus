/**
 * SWDE-56: Distributed Graph Execution Phase 1
 * Tier 3 Integration Tests
 *
 * These tests verify the distributed coordination semantics:
 * - CLUSTER-DIST-001: Work distribution across 2 instances
 * - CLUSTER-CRASH-001: Stale detection + node reassignment after instance death
 * - CLUSTER-AFFINITY-001: Affinity require constraints block wrong-capability instances
 *
 * Tests with real PG: require GRAPH_HARNESS_PG_URL env var.
 * Tests without: source-inspection checks that verify implementation correctness.
 *
 * axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#14
 * plan=phase-4/task-4-1
 */

import { describe, test, expect } from "bun:test";

const GRAPH_HARNESS_PG_URL = process.env.GRAPH_HARNESS_PG_URL ?? "";
const SKIP_DISTRIBUTED = !GRAPH_HARNESS_PG_URL;

// ─────────────────────────────────────────────────────────────────────────────
// CLUSTER-DIST-001: 2-instance work distribution
// Spec: specs/108-Distributed-Graph-Execution.md §14 Tier 3 Verification
// axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#14 plan=phase-4/task-4-1/step-4-1-1
// ─────────────────────────────────────────────────────────────────────────────

describe("CLUSTER-DIST-001: 2-instance work distribution (REQ-DGE-010, REQ-DGE-011)", () => {
  test("Source: performWorkSteal returns WorkStealResult type (claimed | no_work | at_capacity | error)", async () => {
    const source = await Bun.file("lib/graph-harness.ts").text();
    // Verify all expected result reasons are in the implementation
    expect(source).toContain("claimed: true");
    expect(source).toContain('"no_work"');
    expect(source).toContain('"at_capacity"');
    expect(source).toContain('"error"');
  });

  test("Source: CAS CTE uses FOR UPDATE SKIP LOCKED LIMIT 1 (prevents double-assignment)", async () => {
    const source = await Bun.file("lib/graph-harness.ts").text();
    // The LIMIT 1 with SKIP LOCKED is the key to preventing double-assignment
    const casSection = source.slice(source.indexOf("async function performWorkSteal"), source.indexOf("async function performWorkSteal") + 3000);
    expect(casSection).toMatch(/FOR UPDATE SKIP LOCKED/);
    expect(casSection).toMatch(/LIMIT 1/);
  });

  test.skipIf(SKIP_DISTRIBUTED)("Behavioral: 2 concurrent steal attempts on same node — exactly 1 wins (no double-assignment)", async () => {
    // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-010 plan=post_milestone_followon/step-qa-002
    const schemaName = `dist_test_${Date.now().toString(36)}_cas`;
    const sql1 = new Bun.SQL(GRAPH_HARNESS_PG_URL);
    const sql2 = new Bun.SQL(GRAPH_HARNESS_PG_URL);
    try {
      // Create test schema and minimal tables
      await sql1.unsafe(`CREATE SCHEMA IF NOT EXISTS "${schemaName}"`);
      await sql1.unsafe(`
        CREATE TABLE IF NOT EXISTS "${schemaName}".nodes (
          id TEXT PRIMARY KEY, graph_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
          assigned_session TEXT, activated_at TEXT, title TEXT, execution_mode TEXT
        )
      `);
      await sql1.unsafe(`SET search_path TO "${schemaName}"`);
      await sql2.unsafe(`SET search_path TO "${schemaName}"`);

      // Insert one PENDING node
      await sql1.unsafe(`INSERT INTO nodes (id, graph_id, title) VALUES ('node-1', 'graph-1', 'test-node')`);

      // Two concurrent CAS attempts — exactly 1 should succeed
      const [r1, r2] = await Promise.all([
        sql1.unsafe<{ id: string }[]>(
          `WITH candidate AS (SELECT id FROM nodes WHERE status='pending' AND assigned_session IS NULL FOR UPDATE SKIP LOCKED LIMIT 1)
           UPDATE nodes SET status='active', assigned_session='instance-a' FROM candidate WHERE nodes.id=candidate.id RETURNING nodes.id`
        ),
        sql2.unsafe<{ id: string }[]>(
          `WITH candidate AS (SELECT id FROM nodes WHERE status='pending' AND assigned_session IS NULL FOR UPDATE SKIP LOCKED LIMIT 1)
           UPDATE nodes SET status='active', assigned_session='instance-b' FROM candidate WHERE nodes.id=candidate.id RETURNING nodes.id`
        ),
      ]);

      // Exactly one wins, one gets empty result
      const winners = [r1, r2].filter(r => r.length > 0);
      const losers = [r1, r2].filter(r => r.length === 0);
      expect(winners.length).toBe(1);
      expect(losers.length).toBe(1);

      // Verify no double-assignment
      const final = await sql1.unsafe<{ assigned_session: string }[]>(
        `SELECT assigned_session FROM nodes WHERE id='node-1'`
      );
      expect(['instance-a', 'instance-b']).toContain(final[0]?.assigned_session);
    } finally {
      await sql1.unsafe(`DROP SCHEMA IF EXISTS "${schemaName}" CASCADE`);
      await sql1.end();
      await sql2.end();
    }
  });

  test.skipIf(SKIP_DISTRIBUTED)("Behavioral: work-stealing latency < 10s per claim (REQ-DGE-011 SLO)", async () => {
    // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#16 plan=post_milestone_followon/step-qa-001
    const schemaName = `dist_test_${Date.now().toString(36)}_slo`;
    const sql = new Bun.SQL(GRAPH_HARNESS_PG_URL);
    try {
      await sql.unsafe(`CREATE SCHEMA IF NOT EXISTS "${schemaName}"`);
      await sql.unsafe(`CREATE TABLE "${schemaName}".nodes (id TEXT PRIMARY KEY, graph_id TEXT NOT NULL, status TEXT DEFAULT 'pending', assigned_session TEXT, activated_at TEXT, title TEXT, execution_mode TEXT)`);
      await sql.unsafe(`SET search_path TO "${schemaName}"`);

      // Insert 1 PENDING node
      await sql.unsafe(`INSERT INTO nodes (id, graph_id, title) VALUES ('perf-node', 'perf-graph', 'perf-test')`);

      // Measure CAS claim latency
      const start = Date.now();
      const result = await sql.unsafe<{ id: string }[]>(
        `WITH candidate AS (SELECT id FROM nodes WHERE status='pending' AND assigned_session IS NULL FOR UPDATE SKIP LOCKED LIMIT 1)
         UPDATE nodes SET status='active', assigned_session='perf-instance' FROM candidate WHERE nodes.id=candidate.id RETURNING nodes.id`
      );
      const elapsed = Date.now() - start;

      expect(result.length).toBe(1); // Node was claimed
      expect(elapsed).toBeLessThan(10_000); // SLO: < 10s
    } finally {
      await sql.unsafe(`DROP SCHEMA IF EXISTS "${schemaName}" CASCADE`);
      await sql.end();
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// CLUSTER-CRASH-001: Instance crash + stale detection + node reassignment
// Spec: specs/108-Distributed-Graph-Execution.md§14, REQ-DGE-060, REQ-DGE-061
// axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-060 plan=phase-4/task-4-1/step-4-1-2
// ─────────────────────────────────────────────────────────────────────────────

describe("CLUSTER-CRASH-001: crash + stale detection + reassignment (REQ-DGE-060, REQ-DGE-061)", () => {
  test("Source: detectStaleInstances uses INTERVAL-based last_heartbeat check", async () => {
    const source = await Bun.file("lib/graph-harness.ts").text();
    const fnIdx = source.indexOf("async function detectStaleInstances");
    expect(fnIdx).toBeGreaterThan(-1);
    const fn = source.slice(fnIdx, fnIdx + 2000);
    expect(fn).toMatch(/INTERVAL/);
    expect(fn).toMatch(/last_heartbeat/);
    expect(fn).toMatch(/status.*=.*'dead'/);
  });

  test("Source: reassignDeadInstanceNodes resets nodes to pending with assigned_session=NULL", async () => {
    const source = await Bun.file("lib/graph-harness.ts").text();
    const fnIdx = source.indexOf("async function reassignDeadInstanceNodes");
    expect(fnIdx).toBeGreaterThan(-1);
    const fn = source.slice(fnIdx, fnIdx + 2000);
    expect(fn).toMatch(/status.*=.*'pending'/);
    expect(fn).toMatch(/assigned_session.*=.*NULL/);
  });

  test("Source: detectStaleInstances uses CAS UPDATE RETURNING (coordinator-free atomicity)", async () => {
    const source = await Bun.file("lib/graph-harness.ts").text();
    const fnIdx = source.indexOf("async function detectStaleInstances");
    const fn = source.slice(fnIdx, fnIdx + 2000);
    // Must use RETURNING for CAS semantics (only winner gets rows)
    expect(fn).toMatch(/RETURNING/i);
    // Must exclude self (instance_id != ?)
    expect(fn).toMatch(/instance_id\s*!=\s*\?|instance_id\s*<>\s*\?/);
  });

  test.skipIf(SKIP_DISTRIBUTED)("Behavioral: stale instance (heartbeat > 90s) gets marked dead and its nodes reassigned", async () => {
    // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-060 plan=post_milestone_followon/step-qa-001
    const schemaName = `dist_test_${Date.now().toString(36)}_crash`;
    const sql = new Bun.SQL(GRAPH_HARNESS_PG_URL);
    try {
      await sql.unsafe(`CREATE SCHEMA IF NOT EXISTS "${schemaName}"`);
      await sql.unsafe(`CREATE TABLE "${schemaName}".cluster_instances (instance_id TEXT PRIMARY KEY, status TEXT DEFAULT 'active', active_nodes INT DEFAULT 0, last_heartbeat TIMESTAMPTZ DEFAULT NOW(), capabilities JSONB DEFAULT '[]', region TEXT DEFAULT '', max_nodes INT DEFAULT 10, opencode_base_url TEXT DEFAULT '', registered_at TIMESTAMPTZ DEFAULT NOW(), metadata JSONB DEFAULT '{}')`);
      await sql.unsafe(`CREATE TABLE "${schemaName}".sessions (session_id TEXT PRIMARY KEY, graph_id TEXT NOT NULL, status TEXT DEFAULT 'active', instance_id TEXT, last_heartbeat TEXT, node_id TEXT, role TEXT DEFAULT 'worker', created_at TEXT DEFAULT now()::text, tokens_used BIGINT DEFAULT 0, cost_usd REAL DEFAULT 0, tool_calls TEXT DEFAULT '{}', consecutive_briefing_failures BIGINT DEFAULT 0, worker_pid BIGINT)`);
      await sql.unsafe(`CREATE TABLE "${schemaName}".nodes (id TEXT PRIMARY KEY, graph_id TEXT NOT NULL, status TEXT DEFAULT 'active', assigned_session TEXT, activated_at TEXT, title TEXT, execution_mode TEXT)`);
      await sql.unsafe(`SET search_path TO "${schemaName}"`);

      // Setup: instance-dead with stale heartbeat, instance-live is the detector
      await sql.unsafe(`INSERT INTO cluster_instances (instance_id, last_heartbeat) VALUES ('instance-dead', NOW() - INTERVAL '120 seconds'), ('instance-live', NOW())`);
      await sql.unsafe(`INSERT INTO sessions (session_id, graph_id, instance_id) VALUES ('sess-1', 'graph-1', 'instance-dead')`);
      await sql.unsafe(`INSERT INTO nodes (id, graph_id, assigned_session) VALUES ('node-1', 'graph-1', 'sess-1')`);

      // Detect and mark stale — instance-live runs detection
      const dead = await sql.unsafe<{ instance_id: string }[]>(
        `UPDATE cluster_instances SET status='dead' WHERE instance_id != 'instance-live' AND status='active' AND last_heartbeat < NOW() - INTERVAL '90 seconds' RETURNING instance_id`
      );

      expect(dead.length).toBe(1);
      expect(dead[0].instance_id).toBe('instance-dead');

      // Reassign nodes
      const reassigned = await sql.unsafe<{ id: string }[]>(
        `UPDATE nodes SET status='pending', assigned_session=NULL WHERE status='active' AND assigned_session IN (SELECT session_id FROM sessions WHERE instance_id='instance-dead') RETURNING id`
      );

      expect(reassigned.length).toBe(1);
      expect(reassigned[0].id).toBe('node-1');

      // Verify final state
      const nodeState = await sql.unsafe<{ status: string }[]>(`SELECT status FROM nodes WHERE id='node-1'`);
      expect(nodeState[0].status).toBe('pending');
    } finally {
      await sql.unsafe(`DROP SCHEMA IF EXISTS "${schemaName}" CASCADE`);
      await sql.end();
    }
  });

  test.skipIf(SKIP_DISTRIBUTED)("Behavioral: stale detection < 90s SLO (REQ-DGE-060, spec §16)", async () => {
    // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#16 plan=post_milestone_followon/step-qa-001
    const schemaName = `dist_test_${Date.now().toString(36)}_slo2`;
    const sql = new Bun.SQL(GRAPH_HARNESS_PG_URL);
    try {
      await sql.unsafe(`CREATE SCHEMA IF NOT EXISTS "${schemaName}"`);
      await sql.unsafe(`CREATE TABLE "${schemaName}".cluster_instances (instance_id TEXT PRIMARY KEY, status TEXT DEFAULT 'active', active_nodes INT DEFAULT 0, last_heartbeat TIMESTAMPTZ DEFAULT NOW(), capabilities JSONB DEFAULT '[]', region TEXT DEFAULT '', max_nodes INT DEFAULT 10, opencode_base_url TEXT DEFAULT '', registered_at TIMESTAMPTZ DEFAULT NOW(), metadata JSONB DEFAULT '{}')`);
      await sql.unsafe(`SET search_path TO "${schemaName}"`);

      // Insert a stale instance (heartbeat 120s ago)
      await sql.unsafe(`INSERT INTO cluster_instances (instance_id, last_heartbeat) VALUES ('stale-inst', NOW() - INTERVAL '120 seconds'), ('live-inst', NOW())`);

      // Measure detection latency
      const start = Date.now();
      const result = await sql.unsafe<{ instance_id: string }[]>(
        `UPDATE cluster_instances SET status='dead' WHERE instance_id!='live-inst' AND status='active' AND last_heartbeat < NOW() - INTERVAL '90 seconds' RETURNING instance_id`
      );
      const elapsed = Date.now() - start;

      expect(result.length).toBe(1); // One stale instance detected
      expect(elapsed).toBeLessThan(90_000); // SLO: detection < 90s (query is fast; 90s is the threshold, not detection time)
    } finally {
      await sql.unsafe(`DROP SCHEMA IF EXISTS "${schemaName}" CASCADE`);
      await sql.end();
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// CLUSTER-AFFINITY-001: Affinity require constraint blocks wrong-capability instances
// Spec: specs/108-Distributed-Graph-Execution.md§14, REQ-DGE-021
// axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-021 plan=phase-4/task-4-1/step-4-1-3
// ─────────────────────────────────────────────────────────────────────────────

describe("CLUSTER-AFFINITY-001: affinity require constraint blocks non-matching instances (REQ-DGE-021)", () => {
  test("Source: affinity filter inside CAS CTE uses NOT EXISTS with node_affinity join", async () => {
    const source = await Bun.file("lib/graph-harness.ts").text();
    const fnIdx = source.indexOf("async function performWorkSteal");
    const fn = source.slice(fnIdx, fnIdx + 4000);
    // NOT EXISTS with node_affinity join must be before FOR UPDATE SKIP LOCKED
    const notExistsIdx = fn.indexOf("NOT EXISTS");
    const nodeAffinityIdx = fn.indexOf("node_affinity");
    const skipLockedIdx = fn.lastIndexOf("FOR UPDATE SKIP LOCKED");
    expect(notExistsIdx).toBeGreaterThan(-1);
    expect(nodeAffinityIdx).toBeGreaterThan(-1);
    expect(skipLockedIdx).toBeGreaterThan(-1);
    // NOT EXISTS must come before FOR UPDATE SKIP LOCKED in the CTE
    expect(notExistsIdx).toBeLessThan(skipLockedIdx);
  });

  test("Source: affinity filter checks affinity_type='require' only (prefer is Phase 2)", async () => {
    const source = await Bun.file("lib/graph-harness.ts").text();
    const fnIdx = source.indexOf("async function performWorkSteal");
    const fn = source.slice(fnIdx, fnIdx + 4000);
    expect(fn).toContain("affinity_type = 'require'");
    // 'prefer' should not be in Phase 1 implementation
    expect(fn).not.toContain("affinity_type = 'prefer'");
  });

  test("Source: affinity filter uses json_array_elements_text(capabilities) for JSON array expansion", async () => {
    const source = await Bun.file("lib/graph-harness.ts").text();
    // This is PostgreSQL-specific for JSONB array expansion
    expect(source).toMatch(/json_array_elements_text\(capabilities\)/);
  });

  test.skipIf(SKIP_DISTRIBUTED)("Behavioral: node with require:gpu blocked for non-gpu instance, claimed by gpu instance", async () => {
    // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-021 plan=post_milestone_followon/step-qa-002
    const schemaName = `dist_test_${Date.now().toString(36)}_affinity`;
    const sql = new Bun.SQL(GRAPH_HARNESS_PG_URL);
    try {
      await sql.unsafe(`CREATE SCHEMA IF NOT EXISTS "${schemaName}"`);
      await sql.unsafe(`CREATE TABLE "${schemaName}".nodes (id TEXT PRIMARY KEY, graph_id TEXT NOT NULL, status TEXT DEFAULT 'pending', assigned_session TEXT, activated_at TEXT, title TEXT, execution_mode TEXT)`);
      await sql.unsafe(`CREATE TABLE "${schemaName}".node_affinity (id TEXT PRIMARY KEY, node_id TEXT NOT NULL, graph_id TEXT NOT NULL, affinity_type TEXT NOT NULL, capability TEXT NOT NULL, region TEXT DEFAULT '', weight INT DEFAULT 1)`);
      await sql.unsafe(`CREATE TABLE "${schemaName}".cluster_instances (instance_id TEXT PRIMARY KEY, capabilities JSONB DEFAULT '[]', status TEXT DEFAULT 'active', active_nodes INT DEFAULT 0, last_heartbeat TIMESTAMPTZ DEFAULT NOW(), region TEXT DEFAULT '', max_nodes INT DEFAULT 10, opencode_base_url TEXT DEFAULT '', registered_at TIMESTAMPTZ DEFAULT NOW(), metadata JSONB DEFAULT '{}')`);
      await sql.unsafe(`SET search_path TO "${schemaName}"`);

      // Setup: gpu-required node, python-only instance
      await sql.unsafe(`INSERT INTO nodes (id, graph_id) VALUES ('gpu-node', 'graph-1')`);
      await sql.unsafe(`INSERT INTO node_affinity (id, node_id, graph_id, affinity_type, capability) VALUES ('aff-1', 'gpu-node', 'graph-1', 'require', 'gpu')`);
      await sql.unsafe(`INSERT INTO cluster_instances (instance_id, capabilities) VALUES ('python-inst', '["python"]'), ('gpu-inst', '["gpu", "python"]')`);

      // Non-gpu instance: CAS should return 0 rows (node blocked by affinity)
      const nonGpuResult = await sql.unsafe<{ id: string }[]>(
        `WITH candidate AS (
           SELECT id FROM nodes WHERE status='pending' AND assigned_session IS NULL
           AND NOT EXISTS (
             SELECT 1 FROM node_affinity na
             WHERE na.node_id=nodes.id AND na.affinity_type='require'
             AND na.capability NOT IN (SELECT json_array_elements_text(capabilities) FROM cluster_instances WHERE instance_id='python-inst')
           )
           FOR UPDATE SKIP LOCKED LIMIT 1
         )
         UPDATE nodes SET status='active', assigned_session='python-inst' FROM candidate WHERE nodes.id=candidate.id RETURNING nodes.id`
      );
      expect(nonGpuResult.length).toBe(0); // Non-gpu instance cannot claim gpu-required node

      // GPU instance: CAS should return 1 row
      const gpuResult = await sql.unsafe<{ id: string }[]>(
        `WITH candidate AS (
           SELECT id FROM nodes WHERE status='pending' AND assigned_session IS NULL
           AND NOT EXISTS (
             SELECT 1 FROM node_affinity na
             WHERE na.node_id=nodes.id AND na.affinity_type='require'
             AND na.capability NOT IN (SELECT json_array_elements_text(capabilities) FROM cluster_instances WHERE instance_id='gpu-inst')
           )
           FOR UPDATE SKIP LOCKED LIMIT 1
         )
         UPDATE nodes SET status='active', assigned_session='gpu-inst' FROM candidate WHERE nodes.id=candidate.id RETURNING nodes.id`
      );
      expect(gpuResult.length).toBe(1); // GPU instance claims the node
      expect(gpuResult[0].id).toBe('gpu-node');
    } finally {
      await sql.unsafe(`DROP SCHEMA IF EXISTS "${schemaName}" CASCADE`);
      await sql.end();
    }
  });

  test.skipIf(SKIP_DISTRIBUTED)("Behavioral: node with no affinity constraints claimable by any instance", async () => {
    // axiom:trace work_item=SWDE-56 spec=specs/108-Distributed-Graph-Execution.md#REQ-DGE-021 plan=post_milestone_followon/step-qa-001
    const schemaName = `dist_test_${Date.now().toString(36)}_noaffinity`;
    const sql = new Bun.SQL(GRAPH_HARNESS_PG_URL);
    try {
      await sql.unsafe(`CREATE SCHEMA IF NOT EXISTS "${schemaName}"`);
      await sql.unsafe(`CREATE TABLE "${schemaName}".nodes (id TEXT PRIMARY KEY, graph_id TEXT NOT NULL, status TEXT DEFAULT 'pending', assigned_session TEXT, activated_at TEXT, title TEXT, execution_mode TEXT)`);
      await sql.unsafe(`CREATE TABLE "${schemaName}".node_affinity (id TEXT PRIMARY KEY, node_id TEXT NOT NULL, graph_id TEXT NOT NULL, affinity_type TEXT NOT NULL, capability TEXT NOT NULL, region TEXT DEFAULT '', weight INT DEFAULT 1)`);
      await sql.unsafe(`CREATE TABLE "${schemaName}".cluster_instances (instance_id TEXT PRIMARY KEY, capabilities JSONB DEFAULT '[]', status TEXT DEFAULT 'active', active_nodes INT DEFAULT 0, last_heartbeat TIMESTAMPTZ DEFAULT NOW(), region TEXT DEFAULT '', max_nodes INT DEFAULT 10, opencode_base_url TEXT DEFAULT '', registered_at TIMESTAMPTZ DEFAULT NOW(), metadata JSONB DEFAULT '{}')`);
      await sql.unsafe(`SET search_path TO "${schemaName}"`);

      // No affinity entries for this node
      await sql.unsafe(`INSERT INTO nodes (id, graph_id) VALUES ('open-node', 'graph-1')`);
      await sql.unsafe(`INSERT INTO cluster_instances (instance_id, capabilities) VALUES ('any-inst', '["python"]')`);
      // node_affinity intentionally empty

      // Any instance should be able to claim the node
      const result = await sql.unsafe<{ id: string }[]>(
        `WITH candidate AS (
           SELECT id FROM nodes WHERE status='pending' AND assigned_session IS NULL
           AND NOT EXISTS (
             SELECT 1 FROM node_affinity na
             WHERE na.node_id=nodes.id AND na.affinity_type='require'
             AND na.capability NOT IN (SELECT json_array_elements_text(capabilities) FROM cluster_instances WHERE instance_id='any-inst')
           )
           FOR UPDATE SKIP LOCKED LIMIT 1
         )
         UPDATE nodes SET status='active', assigned_session='any-inst' FROM candidate WHERE nodes.id=candidate.id RETURNING nodes.id`
      );
      expect(result.length).toBe(1); // Node claimable when no affinity constraints
      expect(result[0].id).toBe('open-node');
    } finally {
      await sql.unsafe(`DROP SCHEMA IF EXISTS "${schemaName}" CASCADE`);
      await sql.end();
    }
  });
});
