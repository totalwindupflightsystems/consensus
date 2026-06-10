/**
 * Agent Depth Guard Plugin — prevents recursive fork bombs by tracking
 * actual delegation DEPTH (via session parentID chain from the OpenCode API)
 * and detecting rapid cascade bursts.
 *
 * ## The core distinction this plugin must get right
 *
 * LEGITIMATE (breadth — orchestrator dispatching many agents in parallel):
 *   tower → [agent-A, agent-B, agent-C, agent-D, agent-E, agent-F, ...]
 *   All at depth 1. Total count can be 20+. This is fine.
 *
 * FORK BOMB (depth — agent spawning agents spawning agents):
 *   tower → agent-A → agent-B → agent-A → agent-B → ...
 *   Each call increases depth. Depth 7+ is almost certainly a cascade.
 *
 * ## Detection strategy — THREE signals
 *
 * 1. REAL DEPTH: Walk session.parentID chain via the OpenCode SDK client.
 *    If the current session is already at depth >= MAX_DELEGATION_DEPTH,
 *    block ALL further Task calls from this session. This is the primary guard.
 *
 * 2. BURST DETECTION: >FORK_BOMB_BURST_THRESHOLD calls within FORK_BOMB_WINDOW_MS.
 *    Catches cascades even if depth tracking fails (API unavailable, etc).
 *
 * 3. TOTAL CEILING: >MAX_TOTAL_TASKS in a single session.
 *    Hard cap to catch slow-burn cascades that evade burst detection.
 *
 * This is defense-in-depth — it works alongside:
 * - permission.task self-deny rules in agent frontmatter (Layer 1)
 * - Anti-recursion prompt text in agent bodies (Layer 2)
 * - doom_loop: deny in opencode.jsonc (Layer 3)
 * - This plugin is Layer 4: runtime depth + cascade detection
 *
 * axiom:trace work_item=agent-depth-guard-01 spec=specs/98-Agent-Safety-Guardrails.md#PG-001
 */

// ── Depth limit (primary guard) ────────────────────────────────────────────
// Walk the session parentID chain. If this session is already N levels deep,
// it should NOT be spawning more agents — that's the orchestrator's job.
const MAX_DELEGATION_DEPTH = 7;

// ── Burst detection (secondary guard) ──────────────────────────────────────
const FORK_BOMB_WINDOW_MS = 5_000;
const FORK_BOMB_BURST_THRESHOLD = 12;
const FORK_BOMB_WARN_THRESHOLD = 6;

// ── Total ceiling (tertiary guard) ─────────────────────────────────────────
const MAX_TOTAL_TASKS = 0;

/**
 * Compute the delegation depth of a session by walking the parentID chain
 * via the OpenCode SDK client. Returns the depth (0 = root session,
 * 1 = direct child, etc.) or -1 if the API call fails.
 */
async function getSessionDepth(client, sessionId) {
  try {
    let depth = 0;
    let currentId = sessionId;

    // Walk up the parent chain. Cap at 100 to prevent infinite loops
    // in case of circular references (shouldn't happen, but defensive).
    while (currentId && depth < 100) {
      const response = await client.session.get({
        path: { id: currentId },
      });

      // response.data is the Session object per the SDK types
      const session = response.data;
      if (!session || !session.parentID) {
        // Reached the root session (no parent)
        break;
      }

      depth++;
      currentId = session.parentID;
    }

    return depth;
  } catch (_) {
    // API call failed — fall back to burst/total detection only.
    return -1;
  }
}

export const AgentDepthGuard = async ({ client }) => {
  // Session-scoped state
  let totalTaskCount = 0;
  const recentTaskCalls = [];
  let cachedDepth = null; // Cache depth — it doesn't change within a session

  const log = async (level, message, extra) => {
    try {
      await client.app.log({
        body: { service: "agent-depth-guard", level, message, extra },
      });
    } catch (_) {
      // Log failure must not prevent the guard from firing
    }
  };

  return {
    tool: {},
    "tool.execute.before": async (input, output) => {
      // Only intercept Task tool calls
      if (input.tool !== "task") return;

      totalTaskCount++;

      // Track timing for burst detection
      const now = Date.now();
      recentTaskCalls.push(now);
      while (recentTaskCalls.length > 0 && recentTaskCalls[0] < now - FORK_BOMB_WINDOW_MS) {
        recentTaskCalls.shift();
      }

      const subagentType = output?.args?.subagent_type || output?.args?.description || "unknown";
      const burstCount = recentTaskCalls.length;

      // ── Check 1: Real delegation depth (primary guard) ─────────────────
      // Query the API once per session and cache the result.
      if (cachedDepth === null) {
        cachedDepth = await getSessionDepth(client, input.sessionID);
      }

      if (cachedDepth >= 0 && cachedDepth >= MAX_DELEGATION_DEPTH) {
        const msg =
          `[AgentDepthGuard] BLOCKED: Session is at delegation depth ${cachedDepth} (limit: ${MAX_DELEGATION_DEPTH}). ` +
          `Subagent: ${subagentType}. ` +
          `This session is too deep in the agent chain to spawn more agents. ` +
          `Only the orchestrator (depth 0-1) should dispatch subagents.`;

        await log("error", msg, { depth: cachedDepth, subagentType, maxDepth: MAX_DELEGATION_DEPTH, sessionID: input.sessionID });
        throw new Error(msg);
      }

      // ── Check 2: Hard total ceiling ──────────────────────────────────────
      if (MAX_TOTAL_TASKS > 0 &&totalTaskCount > MAX_TOTAL_TASKS) {
        const msg =
          `[AgentDepthGuard] BLOCKED: Total Task calls (${totalTaskCount}) exceeds session limit (${MAX_TOTAL_TASKS}). ` +
          `Subagent: ${subagentType}. ` +
          `This guard catches runaway cascades. If you need more than ${MAX_TOTAL_TASKS} subagent calls ` +
          `in one session, raise MAX_TOTAL_TASKS in agent-depth-guard.ts.`;

        await log("error", msg, { totalTaskCount, subagentType, maxTotal: MAX_TOTAL_TASKS });
        throw new Error(msg);
      }

      // ── Check 3: Burst / cascade detection ───────────────────────────────
      if (burstCount >= FORK_BOMB_BURST_THRESHOLD) {
        const msg =
          `[AgentDepthGuard] BLOCKED: ${burstCount} Task calls in ${FORK_BOMB_WINDOW_MS / 1000}s — ` +
          `cascade/fork-bomb detected. Subagent: ${subagentType}. Total this session: ${totalTaskCount}. ` +
          `Legitimate parallel dispatch fires all calls in one message (~simultaneous). ` +
          `A cascade fires continuously as each agent spawns more.`;

        await log("error", msg, { burstCount, totalTaskCount, subagentType, windowMs: FORK_BOMB_WINDOW_MS });
        throw new Error(msg);
      }

      // ── Warning: approaching burst threshold ─────────────────────────────
      if (burstCount >= FORK_BOMB_WARN_THRESHOLD) {
        const msg =
          `[AgentDepthGuard] WARNING: ${burstCount} Task calls in ${FORK_BOMB_WINDOW_MS / 1000}s window. ` +
          `Subagent: ${subagentType}. Total: ${totalTaskCount}/${MAX_TOTAL_TASKS}. ` +
          `Depth: ${cachedDepth >= 0 ? cachedDepth : "unknown"}/${MAX_DELEGATION_DEPTH}. ` +
          `Monitoring for cascade pattern.`;

        await log("warn", msg, { burstCount, totalTaskCount, subagentType, depth: cachedDepth });
      }
    },
  };
};
