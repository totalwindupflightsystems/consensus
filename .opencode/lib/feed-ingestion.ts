/**
 * Feed Ingestion Plugin — ALL PHASES COMPLETE (SWDE-52)
 *
 * RSS/Atom, API, Slack, email, iCal feed polling with LLM relevance evaluation,
 * expert routing, health monitoring, and analytics.
 *
 * Tools (8 total):
 *   feed.list      — list registered feeds with health/budget status
 *   feed.poll      — poll one or all enabled feeds (RSS/Atom/API/Slack/iCal)
 *   feed.status    — detailed status for a specific feed
 *   feed.webhook   — accept push payloads (HMAC + schema validation)
 *   feed.email     — accept email digest payloads (HTML strip + subject regex)
 *   feed.health    — comprehensive health metrics + store-rate flagging
 *   feed.analytics — feed effectiveness analytics (cost, store rate, rankings)
 *   feed.subscribe — associate/disassociate feeds with expert agents
 *
 * Storage layout:
 *   .axiom/feeds.yaml             — global config (REQ-FEED-009)
 *   .axiom/feeds/<id>.yaml        — per-feed definitions (REQ-FEED-001)
 *   .memory-bank/feed-state/        — runtime dedup + budget state (gitignored)
 *   .memory-bank/signals/           — default stored relevant items (REQ-FEED-070)
 *   .memory-bank/{agent}/signals/   — expert-routed items (Phase 4)
 *
 * Security:
 *   - Path boundary enforced for all memory writes (safeFeedPath)
 *   - No secrets stored in memory notes
 *   - Feed ID validated against FEED_ID_REGEX before filesystem access
 *   - Webhook HMAC-SHA256 via timingSafeEqual
 *   - Expert name validated against FEED_ID_REGEX before routing
 *
 * Phase 1: RSS/Atom + relevance evaluation + memory write + budget controls
 * Phase 2: Webhook + API polling + staggering + retry queue + cost accounting
 * Phase 3: Slack + email + iCal + health dashboard + store-rate analytics
 * Phase 4: run_when enforcement + expert routing + analytics + subscribe tool
 *
 */
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-4/COMPLETE jira_ref=SWDE-52

import {
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  realpathSync,
} from "node:fs";
import * as fsPromises from "node:fs/promises";
import { join, resolve, dirname } from "node:path";
import { parse as yamlParse, stringify as yamlStringify } from "yaml";
import { tool } from "@opencode-ai/plugin";
import { pluginError } from "./config-utils.ts";
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-020 plan=phase-1-fix/step-52-003 jira_ref=SWDE-52
import { XMLParser } from "fast-xml-parser";
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-027 plan=phase-2/webhook-tool/step-52-p2-005 jira_ref=SWDE-52
import { createHmac, timingSafeEqual } from "node:crypto";

// ─────────────────────────────────────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────────────────────────────────────

/** Regex for valid feed IDs — prevents path traversal via feed ID (REQ-FEED-005) */
// FEED_ID_REGEX: NOT exported as a value — OpenCode's plugin loader iterates
// Object.values(module) and crashes on non-function exports.
const FEED_ID_REGEX = /^[a-z0-9][a-z0-9-]{0,63}$/;
// Test-only access (must be a function)
export const getFeedIdRegex = () => FEED_ID_REGEX;

/** Default budget caps (REQ-FEED-080, REQ-FEED-081) */
const DEFAULT_MAX_ITEMS_PER_DAY = 100;
const DEFAULT_MAX_COST_PER_DAY_USD = 1.0;
const DEFAULT_GLOBAL_MAX_COST_PER_DAY_USD = 10.0;
// DEFAULT_DEDUP_WINDOW_DAYS: configurable dedup window (days) for feed item seen_ids.
// Used by parseDedupWindow() — operators can override per-feed via deduplication.window config.
// Internal fallback for parseDedupWindow() when the window string is unparseable.
// Note: parseDedupWindow IS exported (see below) and is the correct public API for tests
// that need to verify dedup window behavior. DEFAULT_DEDUP_WINDOW_DAYS is not exported
// because it is an implementation detail — the public contract is parseDedupWindow("7d").
// This asymmetry with the exported SEVEN_DAYS_MS is intentional — SEVEN_DAYS_MS is
// a fixed public contract (REQ-FEED-084); DEFAULT_DEDUP_WINDOW_DAYS is a mutable internal default.
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-021 plan=phase-24/step-v18-002 jira_ref=SWDE-52
const DEFAULT_DEDUP_WINDOW_DAYS = 7;
const DEFAULT_UNHEALTHY_AFTER_FAILURES = 3;
// Note: DEFAULT_DEDUP_WINDOW_DAYS (above) and SEVEN_DAYS_MS (below) both equal 7 days
// but serve DIFFERENT purposes. Dedup window is configurable per-feed; store rate window
// is FIXED per REQ-FEED-084. Do not conflate them.
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-22/step-v16-002 jira_ref=SWDE-52
/** Fixed 7-day store rate window per REQ-FEED-084. DESIGN DECISION: not configurable.
 *  This is the single source of truth — update this constant only to change the window
 *  duration. All 4 usages (isStaleWindow + 3 pollFeed reset blocks) pick up the change
 *  automatically. Do NOT update the 4 call sites separately.
 *  axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-22/step-v16-001 jira_ref=SWDE-52
 */
export const SEVEN_DAYS_MS = 7 * 24 * 60 * 60 * 1000;
const DEFAULT_TTL_DAYS = 30;

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

/** A normalized feed item from any source type. REQ-FEED-022 */
export interface FeedItem {
  feed_id: string;
  item_id: string;
  title: string;
  content: string;
  url: string;
  published_at: string;
  author: string;
  tags: string[];
  raw: Record<string, unknown>;
}

/** Structured relevance decision. REQ-FEED-061 */
export interface RelevanceDecision {
  store: boolean;
  reason: string;
  priority: "high" | "medium" | "low";
  tags: string[];
  summary: string;
  /** Optional cost of this evaluation in USD. Accumulated into pollFeed cost tracking. REQ-FEED-082. */
  cost_usd?: number;
}

/** Per-feed relevance configuration */
export interface FeedRelevanceConfig {
  prompt: string;
  model?: string;
  max_items_per_day?: number;
  max_cost_per_day_usd?: number;
  /** Maximum milliseconds to wait for the evaluator before treating as a failure. Default: 30000 (30s). REQ-FEED-065. */
  timeout_ms?: number;
}

/** Deduplication configuration */
export interface FeedDeduplication {
  key?: "guid" | "url" | "title_hash";
  window?: string; // e.g. "7d"
}

/** Source configuration for a feed */
export interface FeedSource {
  url?: string;
  /** Slack channel ID (e.g., "C0123ALERTS") for slack feeds. REQ-FEED-035. */
  channel?: string;
  imap_folder?: string;
  /** HTTP headers for api feeds. Supports ${VAR} env var interpolation. REQ-FEED-031. */
  headers?: Record<string, string>;
  /** jq-style extraction path for api feeds. E.g. ".[]" or ".items[].releases". REQ-FEED-031. */
  jq_extract?: string;
  /** Lookahead window in days for ical feeds (default: 7). REQ-FEED-047. */
  lookahead_days?: number;
  /** Env var name that holds the Slack bot token (default: "SLACK_BOT_TOKEN"). REQ-FEED-035. */
  slack_token_env?: string;
  /** Filter: only return messages from this user (display name or user ID). REQ-FEED-036. */
  slack_filter_user?: string;
  /** Filter: only return messages containing this keyword. REQ-FEED-036. */
  slack_filter_keyword?: string;
  /** Filter: only return thread replies (not top-level messages). REQ-FEED-036. */
  slack_threads_only?: boolean;
  /** Subject-line regex pattern for email feeds. REQ-FEED-041. */
  subject_regex?: string;
}

/** Target where relevant items are stored */
export interface FeedTarget {
  agent?: string;
  memory_path?: string;
  pandora?: boolean;
}

/**
 * Webhook configuration for push-based feeds. REQ-FEED-025/026/027.
 */
export interface FeedWebhookConfig {
  /** HMAC-SHA256 signing secret. If set, the signature header is required. REQ-FEED-027. */
  secret?: string;
  /** Header name that carries the signature (default: "x-hub-signature-256"). */
  signature_header?: string;
  /** Optional schema for basic payload validation. REQ-FEED-026. */
  schema?: {
    required?: string[]; // required top-level fields
  };
}

/**
 * A complete feed definition. REQ-FEED-002
 * Loaded from .axiom/feeds/<id>.yaml
 */
export interface FeedConfig {
  id: string;
  name: string;
  type: "rss" | "atom" | "webhook" | "email" | "slack" | "api" | "ical";
  source: FeedSource;
  poll_interval: string;
  target: FeedTarget;
  relevance: FeedRelevanceConfig;
  deduplication: FeedDeduplication;
  tags: string[];
  enabled: boolean;
  /** Webhook-specific configuration. Only used when type === "webhook". REQ-FEED-025/026/027. */
  webhook?: FeedWebhookConfig;
}

/** Runtime state per feed — persisted as JSON. REQ-FEED-006 */
 export interface FeedState {
  last_poll_at: string; // ISO8601
  last_success_at: string; // ISO8601 or ""
  last_error: string | null;
  consecutive_failures: number;
  items_today: number; // resets at midnight UTC (REQ-FEED-080)
  cost_today_usd: number; // resets at midnight UTC (REQ-FEED-081)
  budget_date: string; // "YYYY-MM-DD" UTC — for reset detection
  seen_ids: Record<string, string>; // id -> ISO8601 first-seen (dedup window)
  /** Items queued for retry after evaluator failure. REQ-FEED-065. */
  pending_retry: string[]; // array of item_id strings
  /** Per-item retry attempt counter. After 5 failures, item is permanently failed. REQ-FEED-065. */
  retry_attempts: Record<string, number>;
  /** 7-day rolling window for store-rate analytics. REQ-FEED-084/085. */
  store_rate_7d?: {
    items_evaluated: number;
    items_stored: number;
    window_start: string; // ISO8601 — reset when >7 days old
  };
}

/**
 * Injectable relevance evaluator for testability.
 * Default implementation uses the OpenCode HTTP API.
 * Tests inject a mock.
 */
export type RelevanceEvaluator = (
  item: FeedItem,
  relevanceConfig: FeedRelevanceConfig,
  serverUrl?: string
) => Promise<RelevanceDecision>;

/**
 * Required fields for a complete RelevanceDecision. REQ-FEED-061.
 * Single source of truth — used by validateRelevanceDecision() across all 6 feed paths.
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-001 jira_ref=SWDE-52
 */
export const REQUIRED_DECISION_FIELDS = ['store', 'reason', 'priority', 'tags', 'summary'] as const;

/**
 * Validate a RelevanceDecision returned by an evaluator. REQ-FEED-061.
 * Returns an array of invalid/missing field names (empty = valid).
 *
 * Guards:
 * - All required fields must be non-null and non-undefined (val == null catches both)
 * - `reason` must be a non-empty string after trimming (catches null, number, boolean, empty, whitespace)
 * - `summary` must be a non-empty string after trimming when store === true
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-001 jira_ref=SWDE-52
 */
export function validateRelevanceDecision(decision: RelevanceDecision): string[] {
  return REQUIRED_DECISION_FIELDS.filter(f => {
    const val = decision[f as keyof typeof decision];
    if (val == null) return true; // catches undefined and null
    if (f === 'reason' && (typeof val !== 'string' || val.trim().length === 0)) return true;
    if (f === 'summary' && decision.store && (typeof val !== 'string' || val.trim().length === 0)) return true;
    if (f === 'priority' && !['high', 'medium', 'low'].includes(val as string)) return true;
    if (f === 'tags' && (!Array.isArray(val) || !(val as unknown[]).every(t => typeof t === 'string'))) return true;
    return false;
  });
}

/**
 * Call the relevance evaluator with a configurable timeout. REQ-FEED-065.
 * If the evaluator does not resolve within timeoutMs, rejects with an Error('evaluator timeout').
 * This makes the B-04 mitigation advice ("set poll_interval > evaluator latency") enforceable.
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#14.4 plan=phase-17/step-v11-001 jira_ref=SWDE-52
 */
export async function callEvaluatorWithTimeout(
  evaluator: RelevanceEvaluator,
  item: FeedItem,
  relevanceConfig: FeedRelevanceConfig,
  serverUrl?: string,
  timeoutMs = 30_000
): Promise<RelevanceDecision> {
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#14.4 plan=phase-19/step-v13-002 jira_ref=SWDE-52
  let timerId: ReturnType<typeof setTimeout> | undefined;
  const timeoutPromise = new Promise<never>((_, reject) => {
    timerId = setTimeout(() => reject(new Error('evaluator timeout')), timeoutMs);
  });
  try {
    return await Promise.race([
      evaluator(item, relevanceConfig, serverUrl),
      timeoutPromise,
    ]);
  } finally {
    // timerId is always assigned — the setTimeout executor inside the Promise constructor
    // runs synchronously before Promise.race settles, so timerId is never undefined here.
    // clearTimeout is safe to call unconditionally. Verified: Bun resolves bare clearTimeout
    // through globalThis at call time (spy count=1 in tests).
    clearTimeout(timerId);
  }
}

/**
 * Returns true if the store_rate_7d window_start is strictly older than 7 days.
 * Computed at call time using Date.now() — not a persisted property.
 *
 * DESIGN DECISION: The 7-day window is fixed per REQ-FEED-084. Custom window
 * durations require a code change to this function.
 *
 * @internal — not part of the public plugin API; exported for testing only.
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-19/step-v13-003 plan=phase-20/step-v14-007 jira_ref=SWDE-52
 */
export function isStaleWindow(windowStart: string): boolean {
  return Date.now() - new Date(windowStart).getTime() > SEVEN_DAYS_MS;
}

/** Global feed configuration (from .axiom/feeds.yaml) */
interface GlobalFeedsConfig {
  enabled: boolean;
  global_max_cost_per_day_usd: number;
  run_when: "always" | "idle" | "scheduled";
  schedule?: string;
  default_relevance_model: string;
  storage: { memory_bank: boolean; pandora: boolean };
  health: { unhealthy_after_failures: number; alert_on_unhealthy: boolean };
}

// ─────────────────────────────────────────────────────────────────────────────
// Validation helpers
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Validate a feed ID. REQ-FEED-005
 * Throws on invalid ID to prevent path traversal.
 */
export function validateFeedId(id: string): void {
  if (!FEED_ID_REGEX.test(id)) {
    throw new Error(
      `Invalid feed ID "${id}". Must match ^[a-z0-9][a-z0-9-]{0,63}$`
    );
  }
}

/**
 * Validate a feed config object has all required fields. REQ-FEED-004
 * Returns the config cast to FeedConfig or throws with a descriptive error.
 */
export function validateFeedConfig(raw: unknown): FeedConfig {
  if (typeof raw !== "object" || raw === null) {
    throw new Error("Feed config must be an object");
  }
  const c = raw as Record<string, unknown>;
  const required: string[] = ["id", "name", "type", "source", "poll_interval", "relevance"];
  for (const field of required) {
    if (!(field in c)) {
      throw new Error(`Feed config missing required field: "${field}"`);
    }
  }
  if (typeof c.id !== "string") throw new Error("Feed id must be a string");
  validateFeedId(c.id);
  const validTypes = ["rss", "atom", "webhook", "email", "slack", "api", "ical"];
  if (!validTypes.includes(c.type as string)) {
    throw new Error(
      `Feed type "${c.type}" is not valid. Must be one of: ${validTypes.join(", ")}`
    );
  }
  if (typeof c.relevance !== "object" || c.relevance === null) {
    throw new Error("Feed relevance must be an object");
  }
  const rel = c.relevance as Record<string, unknown>;
  if (typeof rel.prompt !== "string" || rel.prompt.trim() === "") {
    throw new Error("Feed relevance.prompt must be a non-empty string");
  }
  // REQ-FEED-065: timeout_ms must be a positive number if provided
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-19/step-v13-004 jira_ref=SWDE-52
  if (rel.timeout_ms !== undefined && typeof rel.timeout_ms !== "number") {
    throw new Error(
      `Feed relevance.timeout_ms must be a number (milliseconds); received ${typeof rel.timeout_ms} — did you quote the value in YAML?`
    );
  }
  if (rel.timeout_ms !== undefined && (rel.timeout_ms as number) <= 0) {
    throw new Error(
      `Feed relevance.timeout_ms must be a positive number (milliseconds); received ${rel.timeout_ms}`
    );
  }
  // REQ-FEED-027: webhook.secret must be a non-empty string if provided.
  // An empty string secret passes the truthiness check at runtime but produces a trivially
  // forgeable HMAC signature (any payload signed with "" would be accepted). Reject at
  // config load time to prevent silent security misconfiguration.
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-027 plan=phase-30/step-v24-hmac-secret jira_ref=SWDE-52
  if (c.webhook !== undefined && c.webhook !== null) {
    const wh = c.webhook as Record<string, unknown>;
    if (wh.secret !== undefined && (typeof wh.secret !== "string" || wh.secret.trim() === "")) {
      throw new Error(
        `Feed webhook.secret must be a non-empty string when provided. ` +
        `An empty string secret would accept any payload without real HMAC verification.`
      );
    }
  }
  return {
    id: c.id as string,
    name: (c.name as string) ?? c.id,
    type: c.type as FeedConfig["type"],
    source: (c.source as FeedSource) ?? {},
    poll_interval: (c.poll_interval as string) ?? "1h",
    target: (c.target as FeedTarget) ?? { memory_path: "signals/" },
    relevance: {
      prompt: rel.prompt as string,
      model: rel.model as string | undefined,
      max_items_per_day: (rel.max_items_per_day as number) ?? DEFAULT_MAX_ITEMS_PER_DAY,
      max_cost_per_day_usd: (rel.max_cost_per_day_usd as number) ?? DEFAULT_MAX_COST_PER_DAY_USD,
      timeout_ms: rel.timeout_ms as number | undefined,
    },
    deduplication: (c.deduplication as FeedDeduplication) ?? { key: "guid", window: "7d" },
    tags: (c.tags as string[]) ?? [],
    enabled: c.enabled !== false, // default true
    webhook: (c.webhook as FeedWebhookConfig) ?? undefined,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// Path safety
// ─────────────────────────────────────────────────────────────────────────────

// Process-lifetime cache for realpathSync(root) results.
// Assumption: root directories are not relocated during process lifetime.
// No invalidation mechanism — cache entries persist until process exit.
// Test safety: each test uses a unique tmpDir path, so cache entries
// from one test do not affect another. If tests reuse paths, clear
// the cache with _realRootCache.clear() in afterEach.
// Avoids 200+ redundant synchronous FS calls per poll cycle when 50 feeds × 4+ calls each.
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-005 plan=step-v1-007/realpath-cache jira_ref=SWDE-52
const _realRootCache = new Map<string, string>();

/**
 * Resolve a path and verify it stays within root.
 * Prevents path traversal via malicious content/IDs.
 */
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-005 plan=phase-b03/symlink-fix test=feed-ingestion.test.ts#safeFeedPath-symlink jira_ref=SWDE-52
export function safeFeedPath(root: string, ...segments: string[]): string {
  const candidate = join(root, ...segments);
  const resolved = resolve(candidate);
  const rootResolved = resolve(root);
  // For existing paths, resolve symlinks to prevent traversal via symlinks inside root
  const realResolved = existsSync(resolved) ? realpathSync(resolved) : resolved;
  // Cache the resolved root — root paths don't change within a process lifetime,
  // so we avoid a realpathSync syscall on every safeFeedPath() invocation.
  const realRoot = _realRootCache.get(rootResolved) ?? (() => {
    const r = existsSync(rootResolved) ? realpathSync(rootResolved) : rootResolved;
    _realRootCache.set(rootResolved, r);
    return r;
  })();
  if (!realResolved.startsWith(realRoot + "/") && realResolved !== realRoot) {
    throw new Error(
      `Path traversal detected: "${realResolved}" is outside root "${realRoot}"`
    );
  }
  return resolved; // return the original resolved path (not the realpath) for consistency
}

// ─────────────────────────────────────────────────────────────────────────────
// Feed config loading
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Load all feed configs from .axiom/feeds/*.yaml.
 * Invalid configs are skipped with a warning; the valid ones are returned.
 * REQ-FEED-009
 */
export function loadFeedConfigs(feedsDir: string): FeedConfig[] {
  if (!existsSync(feedsDir)) return [];
  const files = readdirSync(feedsDir).filter((f) => f.endsWith(".yaml") || f.endsWith(".yml")).sort();
  const configs: FeedConfig[] = [];
  for (const file of files) {
    const filePath = safeFeedPath(feedsDir, file);
    try {
      const content = readFileSync(filePath, "utf-8");
      const raw = yamlParse(content);
      const config = validateFeedConfig(raw);
      configs.push(config);
    } catch (err) {
      // Skip invalid configs; do not crash the plugin
      pluginError("feed-ingestion", `Skipping invalid feed config "${file}"`, { error: String(err) });
    }
  }
  // REQ-FEED-005: Duplicate IDs MUST be rejected — keep first occurrence, skip rest
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-005 plan=phase-1/task-1.2/step-b07 test=feed-ingestion.test.ts#AC-B07 jira_ref=SWDE-52
  const seen = new Set<string>();
  const deduped: FeedConfig[] = [];
  for (const config of configs) {
    if (seen.has(config.id)) {
      pluginError("feed-ingestion", `Duplicate feed ID "${config.id}" — skipping second occurrence`, { id: config.id });
      continue;
    }
    seen.add(config.id);
    deduped.push(config);
  }
  return deduped;
}

/**
 * Load global feeds config from .axiom/feeds.yaml.
 * Returns defaults if not found.
 */
export function loadGlobalFeedsConfig(configPath: string): GlobalFeedsConfig {
  const defaults: GlobalFeedsConfig = {
    enabled: true,
    global_max_cost_per_day_usd: DEFAULT_GLOBAL_MAX_COST_PER_DAY_USD,
    run_when: "always",
    default_relevance_model: "anthropic.claude-haiku",
    storage: { memory_bank: true, pandora: false },
    health: {
      unhealthy_after_failures: DEFAULT_UNHEALTHY_AFTER_FAILURES,
      alert_on_unhealthy: true,
    },
  };
  if (!existsSync(configPath)) return defaults;
  try {
    const raw = yamlParse(readFileSync(configPath, "utf-8"));
    const feeds = raw?.feeds ?? {};
    return {
      enabled: feeds.enabled ?? defaults.enabled,
      global_max_cost_per_day_usd:
        feeds.global_max_cost_per_day_usd ?? defaults.global_max_cost_per_day_usd,
      run_when: feeds.run_when ?? defaults.run_when,
      schedule: feeds.schedule,
      default_relevance_model:
        feeds.default_relevance_model ?? defaults.default_relevance_model,
      storage: {
        memory_bank: feeds.storage?.memory_bank ?? true,
        pandora: feeds.storage?.pandora ?? false,
      },
      health: {
        unhealthy_after_failures:
          feeds.health?.unhealthy_after_failures ?? DEFAULT_UNHEALTHY_AFTER_FAILURES,
        alert_on_unhealthy: feeds.health?.alert_on_unhealthy ?? true,
      },
    };
  } catch {
    return defaults;
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// RSS / Atom XML parsing
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Extract the text content of the first matching XML tag.
 * Handles CDATA sections. Case-insensitive tag match.
 * REQ-FEED-020
 */
export function extractTagContent(xml: string, tag: string): string {
  // Match <tag ...>content</tag> or <tag>content</tag>
  const regex = new RegExp(`<${tag}(?:\\s[^>]*)?>([\\s\\S]*?)<\/${tag}>`, "i");
  const match = xml.match(regex);
  if (!match) return "";
  return stripCDATA(match[1]).trim();
}

/**
 * Extract all occurrences of a tag and return their full XML strings.
 * Used to split an RSS feed into items.
 */
export function extractAllTagBlocks(xml: string, tag: string): string[] {
  const results: string[] = [];
  const regex = new RegExp(`<${tag}(?:\\s[^>]*)?>([\\s\\S]*?)<\/${tag}>`, "gi");
  let match: RegExpExecArray | null;
  while ((match = regex.exec(xml)) !== null) {
    results.push(match[0]);
  }
  return results;
}

/**
 * Strip CDATA wrapper if present.
 * REQ-FEED-020
 */
export function stripCDATA(s: string): string {
  const m = s.match(/^\s*<!\[CDATA\[([\s\S]*?)\]\]>\s*$/);
  return m ? m[1] : s;
}

/**
 * Extract an XML attribute value from a tag string.
 * e.g. extractAttr('<link href="https://..." rel="alternate"/>', 'href') → 'https://...'
 */
export function extractAttr(tagStr: string, attr: string): string {
  const regex = new RegExp(`\\b${attr}\\s*=\\s*(?:"([^"]*?)"|'([^']*?)')`, "i");
  const match = tagStr.match(regex);
  if (!match) return "";
  return match[1] ?? match[2] ?? "";
}

/**
 * Generate a stable hash for a string (for title-based dedup).
 * Simple djb2-style hash — sufficient for dedup, not cryptographic.
 */
export function hashString(s: string): string {
  let h = 5381;
  for (let i = 0; i < s.length; i++) {
    h = ((h << 5) + h + s.charCodeAt(i)) >>> 0;
  }
  return h.toString(16).padStart(8, "0");
}

/**
 * Verify HMAC-SHA256 signature of payload.
 * Uses timing-safe comparison. REQ-FEED-027.
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-027 plan=phase-2/webhook-tool/step-52-p2-005 jira_ref=SWDE-52
 */
export function verifyWebhookSignature(
  payload: string,
  signature: string,
  secret: string
): boolean {
  const expected = "sha256=" + createHmac("sha256", secret).update(payload).digest("hex");
  const expectedBuf = Buffer.from(expected, "utf-8");
  const actualBuf = Buffer.from(signature, "utf-8");
  if (expectedBuf.length !== actualBuf.length) return false;
  return timingSafeEqual(expectedBuf, actualBuf);
}

/**
 * Normalize a webhook payload to the common FeedItem schema. REQ-FEED-028.
 * Supports common webhook patterns (GitHub, Jira, generic).
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-028 plan=phase-2/webhook-tool/step-52-p2-005 jira_ref=SWDE-52
 */
export function normalizeWebhookPayload(
  payload: Record<string, unknown>,
  feedId: string
): FeedItem {
  // Try various common field names from webhook providers
  const title = String(
    payload.title ?? payload.name ?? payload.summary ?? payload.subject ??
    (payload.action ? `${payload.action}: ${payload.repository ?? payload.project ?? "event"}` : "(webhook event)")
  );
  const content = String(
    payload.body ?? payload.description ?? payload.message ?? payload.text ??
    payload.content ?? JSON.stringify(payload).slice(0, 500)
  );
  const url = String(
    payload.url ?? payload.html_url ?? payload.link ??
    (payload.repository as Record<string, unknown>)?.html_url ?? ""
  );
  const publishedRaw = payload.timestamp ?? payload.created_at ?? payload.published_at ??
    payload.updated_at ?? "";
  const published_at = publishedRaw
    ? new Date(String(publishedRaw)).toISOString()
    : new Date().toISOString();
  const author = String(
    (payload.sender as Record<string, unknown>)?.login ??
    payload.author ?? payload.creator ?? payload.user ?? ""
  );
  const tags: string[] = Array.isArray(payload.labels)
    ? (payload.labels as unknown[]).map(l =>
        typeof l === "string" ? l : String((l as Record<string, unknown>)?.name ?? l)
      )
    : [];

  const id = String(
    payload.id ?? payload.guid ?? payload.uuid ??
    hashString(title + published_at)
  );
  const item_id = hashString(feedId + ":" + id);

  return {
    feed_id: feedId,
    item_id,
    title,
    content,
    url,
    published_at,
    author,
    tags,
    raw: payload,
  };
}

/**
 * Replace ${VAR_NAME} placeholders with environment variable values.
 * Returns null if any referenced variable is missing. REQ-FEED-032.
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-032 plan=phase-2/api-polling/step-52-p2-004 jira_ref=SWDE-52
 */
export function interpolateEnvVars(s: string): string | null {
  const pattern = /\$\{([^}]+)\}/g;
  let result = s;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(s)) !== null) {
    const varName = match[1];
    const value = process.env[varName];
    if (value === undefined) {
      return null; // missing env var — caller marks feed unhealthy
    }
    result = result.replace(match[0], value);
  }
  return result;
}

/**
 * Parse a generic JSON API response and return normalized FeedItems.
 * Supports simple jq_extract path: "." (root), ".[]" (root array), ".key" (object key),
 * ".key[]" (array at key), ".key[].subkey" (not needed for Phase 2).
 * REQ-FEED-030, REQ-FEED-031.
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-030 plan=phase-2/api-polling/step-52-p2-004 jira_ref=SWDE-52
 */
export function parseApiItems(
  json: unknown,
  jq_extract: string,
  feedId: string
): FeedItem[] {
  let items: unknown[];

  // Navigate the JSON based on jq_extract path
  try {
    let node: unknown = json;
    // Strip leading "." and trailing "[]" then navigate dot-path
    const path = jq_extract.replace(/\[\]$/, "").replace(/^\.\s*/, "");
    if (path) {
      for (const key of path.split(".")) {
        if (key && node != null && typeof node === "object") {
          node = (node as Record<string, unknown>)[key];
        }
      }
    }
    items = Array.isArray(node) ? node : node != null ? [node] : [];
  } catch {
    return [];
  }

  return items.map((raw, idx) => {
    const r = (typeof raw === "object" && raw !== null ? raw : {}) as Record<string, unknown>;
    const title = String(r.title ?? r.name ?? r.subject ?? "(no title)");
    const content = String(r.body ?? r.content ?? r.description ?? r.text ?? r.summary ?? "");
    const url = String(r.url ?? r.html_url ?? r.link ?? r.href ?? "");
    const publishedRaw = r.published_at ?? r.created_at ?? r.timestamp ?? r.date ?? "";
    const published_at = publishedRaw ? new Date(String(publishedRaw)).toISOString() : new Date().toISOString();
    const author = String(r.author ?? r.creator ?? r.user ?? r.username ?? "");
    const tags = Array.isArray(r.tags) ? r.tags.map(String) : [];
    const id = String(r.id ?? r.guid ?? r.uuid ?? (url || hashString(title + published_at)));
    const item_id = hashString(feedId + ":" + id);

    // suppress unused variable warning for idx
    void idx;

    return {
      feed_id: feedId,
      item_id,
      title,
      content,
      url,
      published_at,
      author,
      tags,
      raw: r,
    } satisfies FeedItem;
  });
}

// ─────────────────────────────────────────────────────────────────────────────
// fast-xml-parser instance (shared, stateless) — REQ-FEED-020
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-020 plan=phase-1-fix/step-52-003 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

const _xmlParser = new XMLParser({
  ignoreAttributes: false,
  attributeNamePrefix: "@_",
  textNodeName: "#text",
  cdataPropName: "__cdata",
  parseAttributeValue: false,
  allowBooleanAttributes: true,
  removeNSPrefix: true, // strips dc:, content:, atom: namespace prefixes
});

/**
 * Extract a plain text value from a fast-xml-parser node.
 * Handles: string, CDATA object (__cdata), mixed text (#text), or coerced value.
 */
function getText(v: unknown): string {
  if (v == null) return "";
  if (typeof v === "string") return v;
  if (typeof v === "number" || typeof v === "boolean") return String(v);
  if (typeof v === "object") {
    const o = v as Record<string, unknown>;
    // CDATA content: <description><![CDATA[...]]></description>
    if (o.__cdata != null) return String(o.__cdata);
    // Mixed text + attributes: <title>text</title> with attrs
    if (o["#text"] != null) return String(o["#text"]);
  }
  return "";
}

/**
 * Extract the href from an Atom <link> field.
 * Handles: string, single object with @_href, or array of link objects
 * (finds rel=alternate first, falls back to first element).
 */
function extractAtomLink(linkField: unknown): string {
  if (!linkField) return "";
  if (typeof linkField === "string") return linkField;
  if (Array.isArray(linkField)) {
    const links = linkField as Record<string, string>[];
    const alt = links.find((l) => l["@_rel"] === "alternate");
    return alt?.["@_href"] ?? links[0]?.["@_href"] ?? "";
  }
  if (typeof linkField === "object") {
    return (linkField as Record<string, string>)["@_href"] ?? "";
  }
  return "";
}

/**
 * Parse an RSS 2.0 feed and return normalized FeedItems.
 * Uses fast-xml-parser to correctly handle CDATA sections that contain
 * </item> tags (WordPress, GitHub releases, Medium, Substack).
 * REQ-FEED-020, REQ-FEED-021, REQ-FEED-022
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-020 plan=phase-1-fix/step-52-003 jira_ref=SWDE-52
 */
export function parseRssItems(xml: string, feedId: string): FeedItem[] {
  let parsed: Record<string, unknown>;
  try {
    parsed = _xmlParser.parse(xml) as Record<string, unknown>;
  } catch {
    return [];
  }

  const rss = parsed?.rss as Record<string, unknown> | undefined;
  const channel = (rss?.channel ?? parsed?.channel ?? {}) as Record<string, unknown>;
  const rawItems = Array.isArray(channel.item)
    ? (channel.item as Record<string, unknown>[])
    : channel.item != null
      ? [channel.item as Record<string, unknown>]
      : [];

  return rawItems.map((raw) => {
    const title = getText(raw.title) || "(no title)";
    const link = getText(raw.link);

    // description first; fall back to content:encoded (becomes "encoded" after removeNSPrefix)
    const description =
      getText(raw.description) ||
      getText(raw.encoded) || // content:encoded → encoded via removeNSPrefix
      "";
    const pubDate = getText(raw.pubDate);

    // guid may be an object with @_isPermaLink attr: unwrap with getText
    const guid = getText(raw.guid) || link || hashString(title + pubDate);
    // dc:creator → creator via removeNSPrefix
    const author = getText(raw.author) || getText(raw.creator) || "";

    // categories: single value or array
    const categories: string[] = raw.category != null
      ? (Array.isArray(raw.category)
          ? (raw.category as unknown[]).map(getText)
          : [getText(raw.category)])
      : [];

    // item_id is the stable dedup key for guid mode (default).
    // For url/title_hash dedup modes, deduplicateItems() uses item.url / item.title directly.
    const item_id = hashString(feedId + ":" + guid);

    return {
      feed_id: feedId,
      item_id,
      title,
      content: description,
      url: link,
      published_at: pubDate ? new Date(pubDate).toISOString() : new Date().toISOString(),
      author,
      tags: categories,
      raw: { guid, pubDate, link, description, author },
    } satisfies FeedItem;
  });
}

/**
 * Parse an Atom 1.0 feed and return normalized FeedItems.
 * Uses fast-xml-parser to correctly handle CDATA, namespaces, and
 * multiple <link> elements per entry.
 * REQ-FEED-020, REQ-FEED-021, REQ-FEED-022
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-020 plan=phase-1-fix/step-52-003 jira_ref=SWDE-52
 */
export function parseAtomItems(xml: string, feedId: string): FeedItem[] {
  let parsed: Record<string, unknown>;
  try {
    parsed = _xmlParser.parse(xml) as Record<string, unknown>;
  } catch {
    return [];
  }

  const feed = (parsed?.feed ?? {}) as Record<string, unknown>;
  const rawEntries = Array.isArray(feed.entry)
    ? (feed.entry as Record<string, unknown>[])
    : feed.entry != null
      ? [feed.entry as Record<string, unknown>]
      : [];

  return rawEntries.map((raw) => {
    const title = getText(raw.title) || "(no title)";

    // Atom <link> is an object with @_href (or array when multiple links present)
    const link = extractAtomLink(raw.link);

    const content = getText(raw.content) || getText(raw.summary) || "";
    const published = getText(raw.published) || getText(raw.updated) || "";
    const id = getText(raw.id) || link || hashString(title + published);

    // <author><name>Bob</name></author> — parsed as { name: "Bob" }
    let author = "";
    if (raw.author != null) {
      if (typeof raw.author === "object" && !Array.isArray(raw.author)) {
        author = getText((raw.author as Record<string, unknown>).name) || getText(raw.author);
      } else {
        author = getText(raw.author);
      }
    }

    // Atom categories: <category term="go"/> → { @_term: "go" }
    const categories: string[] = raw.category != null
      ? (Array.isArray(raw.category)
          ? (raw.category as Record<string, string>[]).map((c) =>
              typeof c === "object" ? (c["@_term"] ?? "") : getText(c)
            )
          : [typeof raw.category === "object"
              ? ((raw.category as Record<string, string>)["@_term"] ?? "")
              : getText(raw.category)])
      : [];

    // item_id is the stable dedup key for guid mode (default).
    // For url/title_hash dedup modes, deduplicateItems() uses item.url / item.title directly.
    const item_id = hashString(feedId + ":" + id);

    return {
      feed_id: feedId,
      item_id,
      title,
      content,
      url: link,
      published_at: published ? new Date(published).toISOString() : new Date().toISOString(),
      author,
      tags: categories,
      raw: { id, published, link, content, author },
    } satisfies FeedItem;
  });
}

/**
 * Detect feed type (RSS vs Atom) from XML content and parse items.
 * REQ-FEED-020
 */
export function parseFeed(xml: string, feedId: string): FeedItem[] {
  // Atom feeds have <feed xmlns="http://www.w3.org/2005/Atom"> or just <feed>
  if (/<feed[\s>]/i.test(xml)) {
    return parseAtomItems(xml, feedId);
  }
  // RSS feeds have <rss> or <channel>
  return parseRssItems(xml, feedId);
}

// ─────────────────────────────────────────────────────────────────────────────
// Feed state — deduplication + budget tracking
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Return today's UTC date string "YYYY-MM-DD" for budget reset detection.
 */
export function utcDateString(date: Date = new Date()): string {
  return date.toISOString().slice(0, 10);
}

/**
 * Parse a dedup window string like "7d", "24h" to milliseconds.
 */
export function parseDedupWindow(window: string): number {
  const m = window.match(/^(\d+)([hdw])$/);
  if (!m) return DEFAULT_DEDUP_WINDOW_DAYS * 24 * 60 * 60 * 1000;
  const n = parseInt(m[1], 10);
  if (m[2] === "h") return n * 60 * 60 * 1000;
  if (m[2] === "d") return n * 24 * 60 * 60 * 1000;
  if (m[2] === "w") return n * 7 * 24 * 60 * 60 * 1000;
  return DEFAULT_DEDUP_WINDOW_DAYS * 24 * 60 * 60 * 1000;
}

/** Build a fresh empty FeedState */
export function emptyFeedState(): FeedState {
  return {
    last_poll_at: "",
    last_success_at: "",
    last_error: null,
    consecutive_failures: 0,
    items_today: 0,
    cost_today_usd: 0,
    budget_date: utcDateString(),
    seen_ids: {},
    pending_retry: [],
    retry_attempts: {},
    store_rate_7d: { items_evaluated: 0, items_stored: 0, window_start: new Date().toISOString() },
  };
}

/**
 * Compute the 7-day store rate for a feed (0–1). Returns null if insufficient data (< 5 evaluations).
 * REQ-FEED-084/085.
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-3/health-dashboard/step-52-p3-004 jira_ref=SWDE-52
 */
export function getStoreRate(state: FeedState): number | null {
  const sr = state.store_rate_7d;
  if (!sr || sr.items_evaluated < 5) return null; // insufficient data
  return sr.items_stored / sr.items_evaluated;
}

/**
 * Load feed state from .memory-bank/feed-state/<feed-id>.json.
 * Returns empty state if not found.
 */
export function loadFeedState(stateDir: string, feedId: string): FeedState {
  validateFeedId(feedId);
  const statePath = safeFeedPath(stateDir, `${feedId}.json`);
  if (!existsSync(statePath)) return emptyFeedState();
  try {
    return JSON.parse(readFileSync(statePath, "utf-8")) as FeedState;
  } catch {
    return emptyFeedState();
  }
}

/**
 * Save feed state atomically (write to temp + rename). REQ-FEED-006
 */
export async function saveFeedState(
  stateDir: string,
  feedId: string,
  state: FeedState
): Promise<void> {
  validateFeedId(feedId);
  mkdirSync(stateDir, { recursive: true });
  const statePath = safeFeedPath(stateDir, `${feedId}.json`);
  const tmp = `${statePath}.tmp.${Date.now()}`;
  await fsPromises.writeFile(tmp, JSON.stringify(state, null, 2), "utf-8");
  await fsPromises.rename(tmp, statePath);
}

/**
 * Reset daily budget counters if the date has rolled over.
 * REQ-FEED-080, REQ-FEED-081
 */
export function resetBudgetIfNewDay(state: FeedState): FeedState {
  const today = utcDateString();
  if (state.budget_date !== today) {
    return {
      ...state,
      items_today: 0,
      cost_today_usd: 0,
      budget_date: today,
    };
  }
  return state;
}

/**
 * Prune seen_ids older than the dedup window to prevent unbounded growth.
 */
export function pruneSeenIds(
  seenIds: Record<string, string>,
  windowMs: number,
  now: Date = new Date()
): Record<string, string> {
  const cutoff = now.getTime() - windowMs;
  const pruned: Record<string, string> = {};
  for (const [id, seenAt] of Object.entries(seenIds)) {
    if (new Date(seenAt).getTime() >= cutoff) {
      pruned[id] = seenAt;
    }
  }
  return pruned;
}

/**
 * Separate new items from already-seen duplicates. REQ-FEED-021
 * Returns the new items and the count of duplicates filtered.
 */
export function deduplicateItems(
  items: FeedItem[],
  state: FeedState,
  dedupConfig: FeedDeduplication
): { newItems: FeedItem[]; duplicateCount: number } {
  const windowMs = parseDedupWindow(dedupConfig.window ?? "7d");
  const seen = pruneSeenIds(state.seen_ids, windowMs);
  const newItems: FeedItem[] = [];
  let duplicateCount = 0;

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-021 plan=phase-1-fix/step-52-002 jira_ref=SWDE-52
  for (const item of items) {
    let key: string;
    switch (dedupConfig.key) {
      case "url":
        key = item.url || item.item_id; // fallback to item_id if url empty
        break;
      case "title_hash":
        key = hashString(item.title || item.item_id);
        break;
      case "guid":
      default:
        key = item.item_id; // default: guid-derived hash (existing behavior)
        break;
    }
    if (key in seen) {
      duplicateCount++;
    } else {
      newItems.push(item);
    }
  }

  return { newItems, duplicateCount };
}

// ─────────────────────────────────────────────────────────────────────────────
// Budget enforcement
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Check whether the per-feed daily item cap has been reached. REQ-FEED-053
 * Returns true if budget OK (can proceed), false if cap reached.
 */
export function checkItemBudget(state: FeedState, config: FeedConfig): boolean {
  const cap = config.relevance.max_items_per_day ?? DEFAULT_MAX_ITEMS_PER_DAY;
  return state.items_today < cap;
}

/**
 * Check whether the per-feed daily cost cap has been reached. REQ-FEED-053
 * Returns true if budget OK (can proceed), false if cap reached.
 */
export function checkCostBudget(state: FeedState, config: FeedConfig): boolean {
  const cap = config.relevance.max_cost_per_day_usd ?? DEFAULT_MAX_COST_PER_DAY_USD;
  return state.cost_today_usd < cap;
}

/**
 * Check whether the global daily cost cap has been reached. REQ-FEED-082
 * Returns true if global budget OK.
 */
export function checkGlobalCostBudget(
  totalCostToday: number,
  globalMaxCost: number
): boolean {
  return totalCostToday < globalMaxCost;
}

// ─────────────────────────────────────────────────────────────────────────────
// Schedule helpers (REQ-FEED-083)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Check if a simple cron-like schedule matches the current time.
 * Supports basic patterns: every-15-min ("* /15 * * * *"), hourly ("0 * * * *"),
 * daily-at-8am ("0 8 * * *"). Only checks minute and hour fields.
 * REQ-FEED-083: run_when=scheduled support.
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-083 plan=phase-4/run-when/step-52-p4-001 jira_ref=SWDE-52
 */
export function isScheduledTimeMatch(schedule: string, now: Date = new Date()): boolean {
  if (!schedule) return true; // no schedule = always match
  const parts = schedule.trim().split(/\s+/);
  if (parts.length < 5) return true; // invalid = always run

  const [minuteExpr, hourExpr] = parts;
  const minute = now.getUTCMinutes();
  const hour = now.getUTCHours();

  return matchCronField(minuteExpr, minute, 0, 59) &&
         matchCronField(hourExpr, hour, 0, 23);
}

/**
 * Match a single cron field expression against a value.
 * Supports: wildcard (*), step (every-N), exact (N), list (N,M), range (N-M).
 */
export function matchCronField(expr: string, value: number, min: number, max: number): boolean {
  if (expr === "*") return true;
  if (expr.startsWith("*/")) {
    const step = parseInt(expr.slice(2), 10);
    return step > 0 && value % step === 0;
  }
  if (expr.includes(",")) {
    return expr.split(",").some(part => parseInt(part, 10) === value);
  }
  if (expr.includes("-")) {
    const [lo, hi] = expr.split("-").map(Number);
    return value >= lo && value <= hi;
  }
  return parseInt(expr, 10) === value;
}

// ─────────────────────────────────────────────────────────────────────────────
// Expert Platform routing (Phase 4 — REQ-FEED-001)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Determine the memory path for a feed item based on target.agent routing.
 * Expert Platform integration: items for a specific agent go to their own subdirectory.
 * If target.agent is set, writes to {memoryRoot}/{target.agent}/signals/
 * Otherwise defaults to {memoryRoot}/signals/ (standard path).
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-4/expert-routing/step-52-p4-002 jira_ref=SWDE-52
 */
export function getExpertMemoryPath(config: FeedConfig, memoryRoot: string): string {
  const agent = config.target?.agent;
  if (agent && agent.trim()) {
    // Route to expert-specific subdirectory
    return join(memoryRoot, agent.trim());
  }
  return memoryRoot;
}

// ─────────────────────────────────────────────────────────────────────────────
// Memory write
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Build a YAML frontmatter + Markdown memory note for a relevant feed item.
 * REQ-FEED-071, REQ-FEED-072
 */
export function buildSignalNote(
  item: FeedItem,
  decision: RelevanceDecision,
  config: FeedConfig,
  ttlDays: number = DEFAULT_TTL_DAYS
): string {
  const now = new Date().toISOString();
  const allTags = [...new Set([...(item.tags ?? []), ...(config.tags ?? []), ...(decision.tags ?? [])])];
  const frontmatter = {
    mb: {
      type: "signal",
      title: item.title.slice(0, 120),
      created: now,
      updated: now,
      tags: allTags,
      source: {
        type: "feed",
        feed_id: item.feed_id,
        item_url: item.url,
        ingested_at: now,
        relevance_score: decision.priority,
        relevance_reason: decision.reason,
      },
      target_agent: config.target?.agent ?? undefined,
      ttl_days: ttlDays,
    },
  };

  const body = [
    `# ${item.title}`,
    "",
    `**Summary**: ${decision.summary}`,
    "",
    `**Source**: ${config.name} (${item.feed_id}) — [Original](${item.url})`,
    "",
    `**Ingested**: ${now}`,
    `**Priority**: ${decision.priority}`,
    item.author ? `**Author**: ${item.author}` : null,
    item.published_at ? `**Published**: ${item.published_at}` : null,
    "",
    "## Content",
    "",
    item.content || "(no content)",
    "",
    "---",
    "",
    `<!-- axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md jira_ref=SWDE-52 -->`,
  ]
    .filter((l) => l !== null)
    .join("\n");

  return `---\n${yamlStringify(frontmatter).trimEnd()}\n---\n\n${body}\n`;
}

/**
 * Write a relevant feed item to .memory-bank/signals/.
 * REQ-FEED-070, REQ-FEED-072
 * File name: feed-<feed-id>-<date>-<item-id>.md
 */
export async function writeSignalNote(
  item: FeedItem,
  decision: RelevanceDecision,
  config: FeedConfig,
  memoryRoot: string,
  ttlDays: number = DEFAULT_TTL_DAYS
): Promise<string> {
  const signalsDir = safeFeedPath(memoryRoot, "signals");
  mkdirSync(signalsDir, { recursive: true });

  const dateStr = utcDateString();
  const fileName = `feed-${item.feed_id}-${dateStr}-${item.item_id}.md`;
  const filePath = safeFeedPath(signalsDir, fileName);

  const content = buildSignalNote(item, decision, config, ttlDays);
  const tmp = `${filePath}.tmp.${Date.now()}`;
  await fsPromises.writeFile(tmp, content, "utf-8");
  await fsPromises.rename(tmp, filePath);

  return filePath;
}

// ─────────────────────────────────────────────────────────────────────────────
// Default relevance evaluator
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Default relevance evaluator — stores NOTHING and warns loudly.
 * Production MUST inject a real evaluator via FeedIngestionPlugin({ evaluator: ... }).
 * REQ-FEED-060: relevance evaluation is required; storing everything defeats the purpose.
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-060 plan=phase-1-fix/step-52-006 jira_ref=SWDE-52
 */
export const defaultRelevanceEvaluator: RelevanceEvaluator = async (
  item,
  _relevanceConfig,
  _serverUrl
): Promise<RelevanceDecision> => {
  // DEFAULT: store nothing and warn. Production MUST inject a real evaluator.
  // Do NOT change this to store: true — that's a silent data accumulation trap.
  // REQ-FEED-060: relevance evaluation is required; storing everything defeats the purpose.
  return {
    store: false,
    reason:
      "WARNING: No evaluator configured. Items are discarded until a real evaluator is injected " +
      "via FeedIngestionPlugin({ evaluator: yourEvaluator }). " +
      "See specs/105-Feed-Ingestion.md#REQ-FEED-060.",
    priority: "low",
    tags: [],
    summary: "",
  };
};

// ─────────────────────────────────────────────────────────────────────────────
// Structured logging
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Emit a structured log event to stdout.
 * REQ-FEED-054: structured log events for poll lifecycle.
 * Event schema follows specs/25-Structured-Logging-Events.md.
 * All events have: event, timestamp, feed_id (when applicable).
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-054 plan=phase-1-fix/step-52-004 jira_ref=SWDE-52
 */
export function logEvent(
  event_type: string,
  fields: Record<string, unknown> = {}
): void {
  // Gate event emission behind AXIOM_FEED_INGESTION_DEBUG=1 — without this,
  // OpenCode's TUI captures stdout from plugin code paths and floods the
  // conversation pane with NDJSON lines on every feed poll cycle.
  // For production logging, plugins should use client.app.log() — but this
  // helper is a pure function used in many places, including from sync code
  // paths where the SDK client isn't accessible. Env-gated stderr is the
  // simplest correct fallback.
  // See .memory-bank/best-practices/opencode-plugin-tools-sdk.md (Bug 10).
  if (process.env.AXIOM_FEED_INGESTION_DEBUG !== "1") return;
  process.stderr.write(
    JSON.stringify({
      event: event_type,
      timestamp: new Date().toISOString(),
      ...fields,
    }) + "\n"
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTP fetch helpers
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Fetch an RSS/Atom feed URL and return the XML string.
 * Handles HTTP 304 Not Modified (returns empty string → 0 new items). REQ-FEED-023
 * Marks the feed unhealthy on persistent fetch errors. REQ-FEED-024
 * Enforces a fetch timeout via AbortSignal.timeout(). REQ-FEED-024
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-024 plan=phase-1-fix/step-52-005 jira_ref=SWDE-52
 */
export async function fetchFeed(
  url: string,
  etag?: string,
  lastModified?: string,
  timeoutMs: number = 30000,
  extraHeaders?: Record<string, string> // REQ-FEED-031: custom headers for api feeds (env-var interpolated)
): Promise<{ xml: string; notModified: boolean; etag?: string; lastModified?: string }> {
  const headers: Record<string, string> = {
    "User-Agent": "Axiom-FeedIngestion/1.0",
    Accept: "application/rss+xml, application/atom+xml, application/xml, text/xml, */*",
  };
  if (etag) headers["If-None-Match"] = etag;
  if (lastModified) headers["If-Modified-Since"] = lastModified;
  // Merge extra headers AFTER default headers so they can override Accept/User-Agent if needed
  if (extraHeaders) Object.assign(headers, extraHeaders);

  // REQ-FEED-024: abort hanging fetches after timeoutMs to avoid blocking the poll loop
  const signal = AbortSignal.timeout(timeoutMs);

  let response: Response;
  try {
    response = await fetch(url, { headers, signal });
  } catch (err) {
    if (err instanceof Error && err.name === "AbortError") {
      throw new Error(`Feed fetch timed out after ${timeoutMs}ms`);
    }
    throw err;
  }

  if (response.status === 304) {
    return { xml: "", notModified: true };
  }
  if (!response.ok) {
    throw new Error(`HTTP ${response.status} ${response.statusText} fetching ${url}`);
  }
  const xml = await response.text();
  return {
    xml,
    notModified: false,
    etag: response.headers.get("ETag") ?? undefined,
    lastModified: response.headers.get("Last-Modified") ?? undefined,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 3: iCal calendar feed parsing (REQ-FEED-045/046/047)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Parse iCal (RFC 5545) text content and return normalized FeedItems.
 * Filters events to those starting within lookaheadDays from now. REQ-FEED-045/046/047.
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-045 plan=phase-3/ical-feed/step-52-p3-001 jira_ref=SWDE-52
 */
export function parseICalItems(
  text: string,
  feedId: string,
  lookaheadDays: number = 7,
  _now: Date = new Date()
): FeedItem[] {
  const items: FeedItem[] = [];
  const now = _now;
  const cutoffEnd = new Date(now.getTime() + lookaheadDays * 24 * 60 * 60 * 1000);

  // Split into VEVENT blocks
  const eventBlocks = text.split(/BEGIN:VEVENT/).slice(1);

  for (const block of eventBlocks) {
    const endIdx = block.indexOf("END:VEVENT");
    const content = endIdx >= 0 ? block.slice(0, endIdx) : block;

    // Parse key:value lines (handle folded lines — lines starting with space/tab are continuations)
    const unfolded = content.replace(/\r?\n[ \t]/g, "");
    const lines = unfolded.split(/\r?\n/);
    const props: Record<string, string> = {};
    for (const line of lines) {
      const colonIdx = line.indexOf(":");
      if (colonIdx < 0) continue;
      const key = line.slice(0, colonIdx).split(";")[0].toUpperCase(); // strip params like ;TZID=...
      const value = line.slice(colonIdx + 1).trim();
      if (key && value) props[key] = value;
    }

    // Parse DTSTART
    const dtStartRaw = props["DTSTART"] ?? "";
    if (!dtStartRaw) continue; // skip events without a start time

    let dtStart: Date;
    try {
      dtStart = parseICalDate(dtStartRaw);
    } catch {
      continue;
    }

    // Apply lookahead filter: only events starting within [now - 1day, now + lookaheadDays]
    // Allow slightly past events (within 1 day) so events in progress are included
    const pastCutoff = new Date(now.getTime() - 24 * 60 * 60 * 1000);
    if (dtStart < pastCutoff || dtStart > cutoffEnd) continue;

    const dtEndRaw = props["DTEND"] ?? "";
    let dtEnd = "";
    try { dtEnd = dtEndRaw ? parseICalDate(dtEndRaw).toISOString() : ""; } catch { dtEnd = ""; }

    const title = unescapeICal(props["SUMMARY"] ?? "(no title)");
    const description = unescapeICal(props["DESCRIPTION"] ?? "");
    const location = unescapeICal(props["LOCATION"] ?? "");
    const uid = props["UID"] ?? hashString(title + dtStartRaw);
    const item_id = hashString(feedId + ":" + uid);

    // Collect attendees for author field (first attendee's CN)
    const attendees: string[] = [];
    for (const line of lines) {
      if (line.toUpperCase().startsWith("ATTENDEE")) {
        const cnMatch = line.match(/CN=([^;:]+)/i);
        if (cnMatch) attendees.push(cnMatch[1].trim());
      }
    }

    const itemContent = [
      description,
      location ? `Location: ${location}` : "",
      dtEnd ? `End: ${dtEnd}` : "",
      attendees.length > 0 ? `Attendees: ${attendees.join(", ")}` : "",
    ].filter(Boolean).join("\n");

    const tags: string[] = [];
    if (location) tags.push("calendar");
    if (attendees.length > 0) tags.push("meeting");

    items.push({
      feed_id: feedId,
      item_id,
      title,
      content: itemContent,
      url: props["URL"] ?? props["ORGANIZER"] ?? "",
      published_at: dtStart.toISOString(),
      author: attendees[0] ?? "",
      tags,
      raw: { ...props, dtEnd, attendees },
    });
  }

  return items;
}

/**
 * Parse iCal date/datetime string (e.g. "20260508T100000Z", "20260508") to Date.
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-046 plan=phase-3/ical-feed/step-52-p3-001 jira_ref=SWDE-52
 */
export function parseICalDate(s: string): Date {
  const clean = s.replace(/Z$/, "");
  if (clean.length === 8) {
    // All-day date: YYYYMMDD
    return new Date(`${clean.slice(0,4)}-${clean.slice(4,6)}-${clean.slice(6,8)}T00:00:00Z`);
  }
  // DateTime: YYYYMMDDTHHmmss
  return new Date(
    `${clean.slice(0,4)}-${clean.slice(4,6)}-${clean.slice(6,8)}T${clean.slice(9,11)}:${clean.slice(11,13)}:${clean.slice(13,15)}Z`
  );
}

/**
 * Unescape iCal text values (RFC 5545: \\n → \n, \, → ,, \; → ;).
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-046 plan=phase-3/ical-feed/step-52-p3-001 jira_ref=SWDE-52
 */
export function unescapeICal(s: string): string {
  return s.replace(/\\n/gi, "\n").replace(/\\,/g, ",").replace(/\\;/g, ";").replace(/\\\\/g, "\\");
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 3: Email feed HTML stripping (REQ-FEED-042)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Strip HTML tags from content and normalize whitespace to plain text.
 * REQ-FEED-042: email content MUST be extracted as plain text (HTML stripped).
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-042 plan=phase-3/email-feed/step-52-p3-003 jira_ref=SWDE-52
 */
export function stripHtml(html: string): string {
  return html
    // Remove script and style blocks (and their content)
    .replace(/<(script|style)[^>]*>[\s\S]*?<\/\1>/gi, "")
    // Replace block-level elements with newlines
    .replace(/<\/(div|p|br|h[1-6]|li|tr|td|th|blockquote|article|section)[^>]*>/gi, "\n")
    .replace(/<(br|hr)[^>]*\/?>/gi, "\n")
    // Strip all remaining tags
    .replace(/<[^>]+>/g, "")
    // Decode common HTML entities
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&nbsp;/g, " ")
    // Collapse multiple newlines/spaces
    .replace(/\n{3,}/g, "\n\n")
    .replace(/ {2,}/g, " ")
    .trim();
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 3: Slack channel polling (REQ-FEED-035/036/037)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Parse Slack conversations.history API response and return normalized FeedItems.
 * Supports filtering by user, keyword, and threads-only. REQ-FEED-035/036/037.
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-035 plan=phase-3/slack-feed/step-52-p3-002 jira_ref=SWDE-52
 */
export function parseSlackMessages(
  json: unknown,
  feedId: string,
  options?: {
    filterUser?: string;
    filterKeyword?: string;
    threadsOnly?: boolean;
  }
): FeedItem[] {
  const data = json as { ok?: boolean; messages?: unknown[] };
  if (!data.ok || !Array.isArray(data.messages)) return [];

  const items: FeedItem[] = [];

  for (const raw of data.messages) {
    const msg = raw as Record<string, unknown>;
    const text = String(msg.text ?? "");
    const user = String(msg.user ?? msg.username ?? "");
    const ts = String(msg.ts ?? "");
    const displayName = String(
      (msg.user_profile as Record<string, unknown>)?.display_name ??
      (msg.user_profile as Record<string, unknown>)?.real_name ??
      user
    );
    const threadTs = String(msg.thread_ts ?? "");
    const isReply = Boolean(threadTs && threadTs !== ts);

    // REQ-FEED-036 filters
    if (options?.threadsOnly && !isReply) continue;
    if (options?.filterUser && displayName !== options.filterUser && user !== options.filterUser) continue;
    if (options?.filterKeyword && !text.toLowerCase().includes(options.filterKeyword.toLowerCase())) continue;

    // Convert Slack ts (Unix timestamp with decimal) to ISO8601
    const publishedAt = ts
      ? new Date(parseFloat(ts) * 1000).toISOString()
      : new Date().toISOString();

    const item_id = hashString(feedId + ":" + ts);
    const url = String(msg.permalink ?? "");

    // Tags from feed type and thread status
    const tags: string[] = ["slack"];
    if (isReply) tags.push("thread-reply");

    items.push({
      feed_id: feedId,
      item_id,
      title: text.slice(0, 100) || "(slack message)",
      content: text,
      url,
      published_at: publishedAt,
      author: displayName, // REQ-FEED-037: author = display name
      tags,
      raw: msg,
    });
  }

  return items;
}

// ─────────────────────────────────────────────────────────────────────────────
// Poll engine — core Phase 1 pipeline
// ─────────────────────────────────────────────────────────────────────────────

export interface PollResult {
  feed_id: string;
  fetched: boolean;
  not_modified: boolean;
  new_items: number;
  duplicate_items: number;
  evaluated: number;
  stored: number;
  budget_skipped: number;
  cost_incurred_usd: number;  // REQ-FEED-082: global cap accumulation
  errors: string[];
}

/**
 * Poll a single feed: fetch → parse → dedup → evaluate → write.
 * REQ-FEED-050 through REQ-FEED-055
 * REQ-FEED-060 through REQ-FEED-065
 * REQ-FEED-070 through REQ-FEED-073
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-024 plan=phase-1-fix/step-52-005 jira_ref=SWDE-52
 */
export async function pollFeed(
  config: FeedConfig,
  stateDir: string,
  memoryRoot: string,
  evaluator: RelevanceEvaluator,
  serverUrl?: string,
  options: { dryRun?: boolean; _now?: Date } = {},
  fetchTimeoutMs: number = 30000
): Promise<{ result: PollResult; state: FeedState }> {
  let state = loadFeedState(stateDir, config.id);
  state = resetBudgetIfNewDay(state);
  state.last_poll_at = new Date().toISOString();

  // Ensure pending_retry is initialized for states loaded from older persisted JSON
  if (!Array.isArray(state.pending_retry)) {
    state.pending_retry = [];
  }
  // Ensure retry_attempts is initialized for states loaded from older persisted JSON
  if (typeof state.retry_attempts !== 'object' || state.retry_attempts === null) {
    state.retry_attempts = {};
  }

  logEvent("poll_started", { feed_id: config.id, feed_type: config.type });

  // REQ-FEED-065: re-evaluate items that were queued due to previous evaluator failures
  if (state.pending_retry.length > 0 && !options.dryRun) {
    logEvent("retry_started", { feed_id: config.id, count: state.pending_retry.length });
    for (const retryItemId of state.pending_retry) {
      // We don't have the original item content — retry items are re-fetched next poll.
      // Mark as "needs re-evaluation" — on next fetch they'll be seen as new
      // because pending_retry IDs are excluded from seen_ids dedup
      if (retryItemId in state.seen_ids) {
        delete state.seen_ids[retryItemId]; // un-mark as seen so it's re-evaluated
      }
    }
    // Clear the queue — items will be re-fetched and re-evaluated naturally
    state.pending_retry = [];
    logEvent("retry_cleared", { feed_id: config.id });
  }

  const result: PollResult = {
    feed_id: config.id,
    fetched: false,
    not_modified: false,
    new_items: 0,
    duplicate_items: 0,
    evaluated: 0,
    stored: 0,
    budget_skipped: 0,
    // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-082 plan=phase-2/cost-accounting/step-52-p2-003 jira_ref=SWDE-52
    cost_incurred_usd: 0,
    errors: [],
  };

  // Only RSS/Atom supported in Phase 1
  if (!["rss", "atom"].includes(config.type) && config.source.url) {
    // For Phase 1, treat unknown types as RSS if URL is provided
    // Phase 2 will add dedicated handlers
  }

  // ─── Phase 3: Slack channel feed type (REQ-FEED-035/036/037) ─────────────
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-035 plan=phase-3/slack-feed/step-52-p3-002 jira_ref=SWDE-52
  if (config.type === "slack") {
    // REQ-FEED-035: Slack channel ingestion via REST API
    const tokenEnvName = config.source.slack_token_env ?? "SLACK_BOT_TOKEN";
    const token = process.env[tokenEnvName];
    if (!token) {
      result.errors.push(`Feed ${config.id}: missing Slack bot token env var "${tokenEnvName}"`);
      state.consecutive_failures++;
      state.last_error = result.errors[0];
      logEvent("poll_failed", { feed_id: config.id, error: result.errors[0] });
      if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
      return { result, state };
    }
    if (!config.source.channel) {
      result.errors.push(`Feed ${config.id}: missing source.channel for slack feed`);
      state.consecutive_failures++;
      state.last_error = result.errors[0];
      if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
      return { result, state };
    }
    // Build Slack API URL (conversations.history)
    const slackUrl = `https://slack.com/api/conversations.history?channel=${encodeURIComponent(config.source.channel)}&limit=100`;
    const etag = (state as FeedState & { etag?: string }).etag;
    const lastModified = (state as FeedState & { last_modified?: string }).last_modified;
    try {
      const fetchResult = await fetchFeed(
        slackUrl,
        etag as string | undefined,
        lastModified as string | undefined,
        fetchTimeoutMs,
        { Authorization: `Bearer ${token}` }
      );
      result.fetched = true;
      if (fetchResult.notModified) {
        result.not_modified = true;
        state.last_success_at = new Date().toISOString();
        state.consecutive_failures = 0;
        state.last_error = null;
        if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
        return { result, state };
      }
      let slackJson: unknown;
      try { slackJson = JSON.parse(fetchResult.xml); } catch { slackJson = { ok: false, messages: [] }; }
      const slackItems = parseSlackMessages(slackJson, config.id, {
        filterUser: config.source.slack_filter_user,
        filterKeyword: config.source.slack_filter_keyword,
        threadsOnly: config.source.slack_threads_only,
      });
      state.last_success_at = new Date().toISOString();
      state.consecutive_failures = 0;
      state.last_error = null;

      logEvent("items_found", { feed_id: config.id, count: slackItems.length });

      // Deduplicate
      const { newItems: slackNewItems, duplicateCount: slackDupCount } = deduplicateItems(
        slackItems, state, config.deduplication
      );
      result.new_items = slackNewItems.length;
      result.duplicate_items = slackDupCount;

      // Update seen_ids for new items
      const slackWindowMs = parseDedupWindow(config.deduplication.window ?? "7d");
      const slackNowStr = new Date().toISOString();
      for (const item of slackNewItems) {
        state.seen_ids[item.item_id] = slackNowStr;
      }
      state.seen_ids = pruneSeenIds(state.seen_ids, slackWindowMs);

      // Process new items through relevance evaluation + memory write
      for (const item of slackNewItems) {
        if (!checkItemBudget(state, config)) { result.budget_skipped++; continue; }
        if (!checkCostBudget(state, config)) { result.budget_skipped++; continue; }
        result.evaluated++;
        let decision: RelevanceDecision;
        try {
          decision = await callEvaluatorWithTimeout(evaluator, item, config.relevance, serverUrl, config.relevance.timeout_ms ?? 30_000);
          const itemCost = decision.cost_usd ?? 0;
          result.cost_incurred_usd += itemCost;
          state.cost_today_usd += itemCost;
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err);
          result.errors.push(`Relevance eval error for "${item.title}": ${msg}`);
          if (!state.pending_retry.includes(item.item_id)) {
            if (state.pending_retry.length >= 100) state.pending_retry.shift();
            state.pending_retry.push(item.item_id);
            logEvent("eval_retry_queued", { feed_id: config.id, item_id: item.item_id });
          }
          continue;
        }
        // REQ-FEED-061: Validate required RelevanceDecision fields via shared helper
        // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-001 jira_ref=SWDE-52
        const slackMissingFields = validateRelevanceDecision(decision);
        if (slackMissingFields.length > 0) {
          result.errors.push(`Evaluator returned incomplete RelevanceDecision: missing fields [${slackMissingFields.join(', ')}]`);
          const attempts = (state.retry_attempts[item.item_id] ?? 0) + 1;
          if (attempts >= 5) {
            logEvent("permanent_eval_failure", { feed_id: config.id, item_id: item.item_id, attempts });
            delete state.retry_attempts[item.item_id];
          } else {
            state.retry_attempts[item.item_id] = attempts;
            if (!state.pending_retry.includes(item.item_id)) {
              if (state.pending_retry.length >= 100) state.pending_retry.shift();
              state.pending_retry.push(item.item_id);
            }
          }
          continue;
        }
        if (!decision.store) {
          logEvent("item_discarded", {
            feed_id: config.id, item_id: item.item_id,
            title: item.title.slice(0, 80), reason: decision.reason.slice(0, 200),
          });
          continue;
        }
        if (!options.dryRun) {
          try {
            await writeSignalNote(item, decision, config, getExpertMemoryPath(config, memoryRoot));
            result.stored++;
            state.items_today++;
          } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            result.errors.push(`Memory write error for "${item.title}": ${msg}`);
          }
        } else {
          result.stored++;
          state.items_today++;
        }
      }

      // REQ-FEED-084/085: update store_rate_7d for health dashboard analytics
      // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-3/health-dashboard/fix-store-rate jira_ref=SWDE-52
      if (result.evaluated > 0) {
        if (!state.store_rate_7d) {
          state.store_rate_7d = { items_evaluated: 0, items_stored: 0, window_start: new Date().toISOString() };
        }
        const srWindowAge = Date.now() - new Date(state.store_rate_7d.window_start).getTime();
        if (srWindowAge > SEVEN_DAYS_MS) {
          state.store_rate_7d = { items_evaluated: 0, items_stored: 0, window_start: new Date().toISOString() };
        }
        state.store_rate_7d.items_evaluated += result.evaluated;
        state.store_rate_7d.items_stored += result.stored;
      }
      if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
      logEvent("items_evaluated", {
        feed_id: config.id, new_items: result.new_items, evaluated: result.evaluated,
        stored: result.stored, budget_skipped: result.budget_skipped,
        discarded: result.evaluated - result.stored,
      });
      logEvent("poll_completed", {
        feed_id: config.id, fetched: result.fetched, not_modified: result.not_modified,
        new_items: result.new_items, stored: result.stored, errors: result.errors.length,
      });
      return { result, state };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      result.errors.push(`Fetch error: ${msg}`);
      state.consecutive_failures++;
      state.last_error = msg;
      logEvent("poll_failed", { feed_id: config.id, error: msg });
      if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
      return { result, state };
    }
  }
  // ─── End Slack branch ────────────────────────────────────────────────────

  if (!config.source.url) {
    result.errors.push(`Feed ${config.id} has no source URL configured`);
    state.consecutive_failures++;
    state.last_error = result.errors[0];
    if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
    return { result, state };
  }

  // KNOWN TYPE GAP: etag/last_modified are stored as extra fields on FeedState via type cast.
  // They are not in the FeedState interface to keep the core schema stable.
  // This is safe because state is serialized/deserialized as JSON (extra fields are preserved).
  // Phase 2 action: add etag?: string and last_modified?: string to FeedState interface.
  const etag = (state as FeedState & { etag?: string }).etag;
  const lastModified = (state as FeedState & { last_modified?: string }).last_modified;

  // ─── Phase 2: API feed type (REQ-FEED-030/031/032/033) ───────────────────
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-030 plan=phase-2/api-polling/step-52-p2-004 jira_ref=SWDE-52
  if (config.type === "api") {
    // REQ-FEED-032: interpolate env vars in URL
    const interpolatedUrl = interpolateEnvVars(config.source.url!);
    if (interpolatedUrl === null) {
      result.errors.push(`Feed ${config.id}: missing environment variable in URL`);
      state.consecutive_failures++;
      state.last_error = result.errors[0];
      logEvent("poll_failed", { feed_id: config.id, error: "Missing env var in URL" });
      if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
      return { result, state };
    }
    // REQ-FEED-032: interpolate env vars in headers
    const interpolatedHeaders: Record<string, string> = {};
    let headerError = false;
    for (const [key, value] of Object.entries(config.source.headers ?? {})) {
      const interpolated = interpolateEnvVars(value);
      if (interpolated === null) {
        result.errors.push(`Feed ${config.id}: missing environment variable in header "${key}"`);
        headerError = true;
        break;
      }
      interpolatedHeaders[key] = interpolated;
    }
    if (headerError) {
      state.consecutive_failures++;
      state.last_error = result.errors[result.errors.length - 1];
      logEvent("poll_failed", { feed_id: config.id, error: "Missing env var in headers" });
      if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
      return { result, state };
    }
    // Fetch using the existing fetchFeed (ETag caching works for any HTTP URL — REQ-FEED-033)
     // fetchFeed sends the body back as the `xml` field (it's just a string — works for JSON too)
     try {
       const fetchResult = await fetchFeed(
         interpolatedUrl,
         etag as string | undefined,
         lastModified as string | undefined,
         fetchTimeoutMs,
         Object.keys(interpolatedHeaders).length > 0 ? interpolatedHeaders : undefined // REQ-FEED-031
       );
      // Update ETag/Last-Modified for next poll
      if (fetchResult.etag) (state as FeedState & { etag?: string }).etag = fetchResult.etag;
      if (fetchResult.lastModified)
        (state as FeedState & { last_modified?: string }).last_modified = fetchResult.lastModified;
      result.fetched = true;
      if (fetchResult.notModified) {
        result.not_modified = true;
        state.last_success_at = new Date().toISOString();
        state.consecutive_failures = 0;
        state.last_error = null;
        if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
        return { result, state };
      }
      let json: unknown;
      try { json = JSON.parse(fetchResult.xml); } catch { json = []; }
      const apiItems = parseApiItems(json, config.source.jq_extract ?? ".", config.id);
      state.last_success_at = new Date().toISOString();
      state.consecutive_failures = 0;
      state.last_error = null;

      logEvent("items_found", { feed_id: config.id, count: apiItems.length });

      // Deduplicate
      const { newItems: apiNewItems, duplicateCount: apiDupCount } = deduplicateItems(
        apiItems, state, config.deduplication
      );
      result.new_items = apiNewItems.length;
      result.duplicate_items = apiDupCount;

      // Update seen_ids for new items
      const apiWindowMs = parseDedupWindow(config.deduplication.window ?? "7d");
      const apiNowStr = new Date().toISOString();
      for (const item of apiNewItems) {
        state.seen_ids[item.item_id] = apiNowStr;
      }
      state.seen_ids = pruneSeenIds(state.seen_ids, apiWindowMs);

      // Process new items through relevance evaluation + memory write
      for (const item of apiNewItems) {
        if (!checkItemBudget(state, config)) { result.budget_skipped++; continue; }
        if (!checkCostBudget(state, config)) { result.budget_skipped++; continue; }
        result.evaluated++;
        let decision: RelevanceDecision;
        try {
          decision = await callEvaluatorWithTimeout(evaluator, item, config.relevance, serverUrl, config.relevance.timeout_ms ?? 30_000);
          const itemCost = decision.cost_usd ?? 0;
          result.cost_incurred_usd += itemCost;
          state.cost_today_usd += itemCost;
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err);
          result.errors.push(`Relevance eval error for "${item.title}": ${msg}`);
          if (!state.pending_retry.includes(item.item_id)) {
            if (state.pending_retry.length >= 100) state.pending_retry.shift();
            state.pending_retry.push(item.item_id);
            logEvent("eval_retry_queued", { feed_id: config.id, item_id: item.item_id });
          }
          continue;
        }
        // REQ-FEED-061: Validate required RelevanceDecision fields via shared helper
        // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-001 jira_ref=SWDE-52
        const apiMissingFields = validateRelevanceDecision(decision);
        if (apiMissingFields.length > 0) {
          result.errors.push(`Evaluator returned incomplete RelevanceDecision: missing fields [${apiMissingFields.join(', ')}]`);
          const attempts = (state.retry_attempts[item.item_id] ?? 0) + 1;
          if (attempts >= 5) {
            logEvent("permanent_eval_failure", { feed_id: config.id, item_id: item.item_id, attempts });
            delete state.retry_attempts[item.item_id];
          } else {
            state.retry_attempts[item.item_id] = attempts;
            if (!state.pending_retry.includes(item.item_id)) {
              if (state.pending_retry.length >= 100) state.pending_retry.shift();
              state.pending_retry.push(item.item_id);
            }
          }
          continue;
        }
        if (!decision.store) {
          logEvent("item_discarded", {
            feed_id: config.id, item_id: item.item_id,
            title: item.title.slice(0, 80), reason: decision.reason.slice(0, 200),
          });
          continue;
        }
        if (!options.dryRun) {
          try {
            await writeSignalNote(item, decision, config, getExpertMemoryPath(config, memoryRoot));
            result.stored++;
            state.items_today++;
          } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            result.errors.push(`Memory write error for "${item.title}": ${msg}`);
          }
        } else {
          result.stored++;
          state.items_today++;
        }
      }

      if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
      logEvent("items_evaluated", {
        feed_id: config.id, new_items: result.new_items, evaluated: result.evaluated,
        stored: result.stored, budget_skipped: result.budget_skipped,
        discarded: result.evaluated - result.stored,
      });
      logEvent("poll_completed", {
        feed_id: config.id, fetched: result.fetched, not_modified: result.not_modified,
        new_items: result.new_items, stored: result.stored, errors: result.errors.length,
      });
      return { result, state };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      result.errors.push(`Fetch error: ${msg}`);
      state.consecutive_failures++;
      state.last_error = msg;
      logEvent("poll_failed", { feed_id: config.id, error: msg });
      if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
      return { result, state };
    }
  }
  // ─── End API branch ───────────────────────────────────────────────────────

  // ─── Phase 3: iCal calendar feed type (REQ-FEED-045/046/047) ─────────────
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-045 plan=phase-3/ical-feed/step-52-p3-001 jira_ref=SWDE-52
  if (config.type === "ical") {
    try {
      const fetchResult = await fetchFeed(
        config.source.url,
        etag as string | undefined,
        lastModified as string | undefined,
        fetchTimeoutMs
      );
      if (fetchResult.etag) (state as FeedState & { etag?: string }).etag = fetchResult.etag;
      if (fetchResult.lastModified) (state as FeedState & { last_modified?: string }).last_modified = fetchResult.lastModified;
      result.fetched = true;
      if (fetchResult.notModified) {
        result.not_modified = true;
        state.last_success_at = new Date().toISOString();
        state.consecutive_failures = 0;
        state.last_error = null;
        if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
        return { result, state };
      }
      const lookaheadDays = config.source.lookahead_days ?? 7;
      const icalItems = parseICalItems(fetchResult.xml, config.id, lookaheadDays, options._now);
      state.last_success_at = new Date().toISOString();
      state.consecutive_failures = 0;
      state.last_error = null;

      logEvent("items_found", { feed_id: config.id, count: icalItems.length });

      // Deduplicate
      const { newItems: icalNewItems, duplicateCount: icalDupCount } = deduplicateItems(
        icalItems, state, config.deduplication
      );
      result.new_items = icalNewItems.length;
      result.duplicate_items = icalDupCount;

      // Update seen_ids for new items
      const icalWindowMs = parseDedupWindow(config.deduplication.window ?? "7d");
      const icalNowStr = new Date().toISOString();
      for (const item of icalNewItems) {
        state.seen_ids[item.item_id] = icalNowStr;
      }
      state.seen_ids = pruneSeenIds(state.seen_ids, icalWindowMs);

      // Process new items through relevance evaluation + memory write
      for (const item of icalNewItems) {
        if (!checkItemBudget(state, config)) { result.budget_skipped++; continue; }
        if (!checkCostBudget(state, config)) { result.budget_skipped++; continue; }
        result.evaluated++;
        let decision: RelevanceDecision;
        try {
          decision = await callEvaluatorWithTimeout(evaluator, item, config.relevance, serverUrl, config.relevance.timeout_ms ?? 30_000);
          const itemCost = decision.cost_usd ?? 0;
          result.cost_incurred_usd += itemCost;
          state.cost_today_usd += itemCost;
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err);
          result.errors.push(`Relevance eval error for "${item.title}": ${msg}`);
          if (!state.pending_retry.includes(item.item_id)) {
            if (state.pending_retry.length >= 100) state.pending_retry.shift();
            state.pending_retry.push(item.item_id);
            logEvent("eval_retry_queued", { feed_id: config.id, item_id: item.item_id });
          }
          continue;
        }
        // REQ-FEED-061: Validate required RelevanceDecision fields via shared helper
        // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-001 jira_ref=SWDE-52
        const icalMissingFields = validateRelevanceDecision(decision);
        if (icalMissingFields.length > 0) {
          result.errors.push(`Evaluator returned incomplete RelevanceDecision: missing fields [${icalMissingFields.join(', ')}]`);
          const attempts = (state.retry_attempts[item.item_id] ?? 0) + 1;
          if (attempts >= 5) {
            logEvent("permanent_eval_failure", { feed_id: config.id, item_id: item.item_id, attempts });
            delete state.retry_attempts[item.item_id];
          } else {
            state.retry_attempts[item.item_id] = attempts;
            if (!state.pending_retry.includes(item.item_id)) {
              if (state.pending_retry.length >= 100) state.pending_retry.shift();
              state.pending_retry.push(item.item_id);
            }
          }
          continue;
        }
        if (!decision.store) {
          logEvent("item_discarded", {
            feed_id: config.id, item_id: item.item_id,
            title: item.title.slice(0, 80), reason: decision.reason.slice(0, 200),
          });
          continue;
        }
        if (!options.dryRun) {
          try {
            await writeSignalNote(item, decision, config, getExpertMemoryPath(config, memoryRoot));
            result.stored++;
            state.items_today++;
          } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            result.errors.push(`Memory write error for "${item.title}": ${msg}`);
          }
        } else {
          result.stored++;
          state.items_today++;
        }
      }

      // REQ-FEED-084/085: update store_rate_7d for health dashboard analytics
      // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-3/health-dashboard/fix-store-rate jira_ref=SWDE-52
      if (result.evaluated > 0) {
        if (!state.store_rate_7d) {
          state.store_rate_7d = { items_evaluated: 0, items_stored: 0, window_start: new Date().toISOString() };
        }
        const srWindowAge = Date.now() - new Date(state.store_rate_7d.window_start).getTime();
        if (srWindowAge > SEVEN_DAYS_MS) {
          state.store_rate_7d = { items_evaluated: 0, items_stored: 0, window_start: new Date().toISOString() };
        }
        state.store_rate_7d.items_evaluated += result.evaluated;
        state.store_rate_7d.items_stored += result.stored;
      }
      if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
      logEvent("items_evaluated", {
        feed_id: config.id, new_items: result.new_items, evaluated: result.evaluated,
        stored: result.stored, budget_skipped: result.budget_skipped,
        discarded: result.evaluated - result.stored,
      });
      logEvent("poll_completed", {
        feed_id: config.id, fetched: result.fetched, not_modified: result.not_modified,
        new_items: result.new_items, stored: result.stored, errors: result.errors.length,
      });
      return { result, state };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      result.errors.push(`Fetch error: ${msg}`);
      state.consecutive_failures++;
      state.last_error = msg;
      logEvent("poll_failed", { feed_id: config.id, error: msg });
      if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
      return { result, state };
    }
  }
  // ─── End iCal branch ─────────────────────────────────────────────────────

  // Fetch the feed (RSS/Atom path)
  let xml: string;
  let notModified = false;

  try {
    const fetchResult = await fetchFeed(
      config.source.url,
      etag as string | undefined,
      lastModified as string | undefined,
      fetchTimeoutMs
    );
    xml = fetchResult.xml;
    notModified = fetchResult.notModified;
    result.fetched = true;
    result.not_modified = notModified;

    // Update ETag/Last-Modified in state for next poll
    if (fetchResult.etag) (state as FeedState & { etag?: string }).etag = fetchResult.etag;
    if (fetchResult.lastModified)
      (state as FeedState & { last_modified?: string }).last_modified = fetchResult.lastModified;

    state.last_success_at = new Date().toISOString();
    state.last_error = null;
    state.consecutive_failures = 0;
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    result.errors.push(`Fetch error: ${msg}`);
    state.consecutive_failures++;
    state.last_error = msg;
    logEvent("poll_failed", { feed_id: config.id, error: msg });
    if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
    return { result, state };
  }

  if (notModified) {
    if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
    return { result, state };
  }

  // Parse items
  const items = parseFeed(xml, config.id);
  logEvent("items_found", { feed_id: config.id, count: items.length });

  // Deduplicate
  const { newItems, duplicateCount } = deduplicateItems(items, state, config.deduplication);
  result.new_items = newItems.length;
  result.duplicate_items = duplicateCount;

  // Update seen_ids for new items
  const windowMs = parseDedupWindow(config.deduplication.window ?? "7d");
  const nowStr = new Date().toISOString();
  for (const item of newItems) {
    state.seen_ids[item.item_id] = nowStr;
  }
  state.seen_ids = pruneSeenIds(state.seen_ids, windowMs);

  // Process new items through relevance evaluation + memory write
  for (const item of newItems) {
    // Budget check (REQ-FEED-053)
    if (!checkItemBudget(state, config)) {
      result.budget_skipped++;
      continue;
    }
    if (!checkCostBudget(state, config)) {
      result.budget_skipped++;
      continue;
    }

    result.evaluated++;

    let decision: RelevanceDecision;
    try {
      decision = await callEvaluatorWithTimeout(evaluator, item, config.relevance, serverUrl, config.relevance.timeout_ms ?? 30_000);
      // REQ-FEED-082: accumulate evaluator cost for intra-run global cap enforcement
      const itemCost = decision.cost_usd ?? 0;
      result.cost_incurred_usd += itemCost;
      state.cost_today_usd += itemCost;
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      result.errors.push(`Relevance eval error for "${item.title}": ${msg}`);
      // REQ-FEED-065: queue item for retry on next poll (cap at 100)
      if (!state.pending_retry.includes(item.item_id)) {
        if (state.pending_retry.length >= 100) {
          state.pending_retry.shift(); // drop oldest
        }
        state.pending_retry.push(item.item_id);
        logEvent("eval_retry_queued", { feed_id: config.id, item_id: item.item_id });
      }
      continue;
    }

    // REQ-FEED-061: validation for non-throwing malformed returns only.
    // The catch block above exits via continue — this block is only reached
    // when the evaluator returns successfully but with an invalid decision shape.
    // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-001 jira_ref=SWDE-52
    const missingFields = validateRelevanceDecision(decision);
    if (missingFields.length > 0) {
      result.errors.push(`Evaluator returned incomplete RelevanceDecision: missing fields [${missingFields.join(', ')}]`);
      const attempts = (state.retry_attempts[item.item_id] ?? 0) + 1;
      if (attempts >= 5) {
        logEvent("permanent_eval_failure", { feed_id: config.id, item_id: item.item_id, attempts });
        delete state.retry_attempts[item.item_id];
      } else {
        state.retry_attempts[item.item_id] = attempts;
        if (!state.pending_retry.includes(item.item_id)) {
          if (state.pending_retry.length >= 100) state.pending_retry.shift();
          state.pending_retry.push(item.item_id);
        }
      }
      continue;
    }

    if (!decision.store) {
      // REQ-FEED-062: discard decision logged for audit
      logEvent("item_discarded", {
        feed_id: config.id,
        item_id: item.item_id,
        title: item.title.slice(0, 80),
        reason: decision.reason.slice(0, 200),
      });
      continue;
    }

    // Write to memory (REQ-FEED-070)
    if (!options.dryRun) {
      try {
        await writeSignalNote(item, decision, config, getExpertMemoryPath(config, memoryRoot));
        result.stored++;
        state.items_today++;
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        result.errors.push(`Memory write error for "${item.title}": ${msg}`);
      }
    } else {
      // dry-run: count as stored without writing
      result.stored++;
      state.items_today++;
    }
  }

  // REQ-FEED-084/085: update store_rate_7d for health dashboard analytics
  if (result.evaluated > 0) {
    if (!state.store_rate_7d) {
      state.store_rate_7d = { items_evaluated: 0, items_stored: 0, window_start: new Date().toISOString() };
    }
    // Reset window if > 7 days old
    const windowAge = Date.now() - new Date(state.store_rate_7d.window_start).getTime();
    if (windowAge > SEVEN_DAYS_MS) {
      state.store_rate_7d = { items_evaluated: 0, items_stored: 0, window_start: new Date().toISOString() };
    }
    state.store_rate_7d.items_evaluated += result.evaluated;
    state.store_rate_7d.items_stored += result.stored;
  }

  if (!options.dryRun) await saveFeedState(stateDir, config.id, state);
  logEvent("items_evaluated", {
    feed_id: config.id,
    new_items: result.new_items,
    evaluated: result.evaluated,
    stored: result.stored,
    budget_skipped: result.budget_skipped,
    discarded: result.evaluated - result.stored,
  });
  logEvent("poll_completed", {
    feed_id: config.id,
    fetched: result.fetched,
    not_modified: result.not_modified,
    new_items: result.new_items,
    stored: result.stored,
    errors: result.errors.length,
  });
  return { result, state };
}

// ─────────────────────────────────────────────────────────────────────────────
// Plugin factory
// ─────────────────────────────────────────────────────────────────────────────

export interface FeedIngestionPluginInput {
  /** Repository/workspace root directory */
  directory: string;
  /** URL of the OpenCode HTTP server for LLM-based relevance evaluation */
  serverUrl?: string;
  /** Injectable evaluator — defaults to defaultRelevanceEvaluator */
  evaluator?: RelevanceEvaluator;
  /** Override for feeds directory (default: <directory>/.axiom/feeds) */
  feedsDir?: string;
  /** Override for feeds global config path (default: <directory>/.axiom/feeds.yaml) */
  feedsConfigPath?: string;
  /** Override for feed state directory (default: <directory>/.memory-bank/feed-state) */
  stateDir?: string;
  /** Override for memory bank root (default: <directory>/.memory-bank) */
  memoryRoot?: string;
  /** Timeout in ms for HTTP feed fetches (default: 30000) */
  fetch_timeout_ms?: number;
  /** Milliseconds to wait between feed polls (default: 0 = no stagger). REQ-FEED-051. */
  stagger_ms?: number;
  /** Additional random jitter in ms added to stagger_ms (default: 0). */
  stagger_jitter_ms?: number;
}

/**
 * Create the Feed Ingestion plugin.
 * Returns a `{ tool }` object compatible with the existing plugin pattern.
 *
 * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#4
 *   plan=phase-1/task-1.1 jira_ref=SWDE-52
 */
export const FeedIngestionPlugin = (input: FeedIngestionPluginInput) => {
  const dir = input.directory;
  const feedsDir = input.feedsDir ?? join(dir, ".axiom", "feeds");
  const feedsConfigPath = input.feedsConfigPath ?? join(dir, ".axiom", "feeds.yaml");
  const stateDir = input.stateDir ?? join(dir, ".memory-bank", "feed-state");
  const memoryRoot = input.memoryRoot ?? join(dir, ".memory-bank");
  const evaluator = input.evaluator ?? defaultRelevanceEvaluator;
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-060 plan=phase-1-fix/step-52-006 jira_ref=SWDE-52
  const usingDefaultEvaluator = input.evaluator == null || input.evaluator === defaultRelevanceEvaluator;
  const serverUrl = input.serverUrl;
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-024 plan=phase-1-fix/step-52-005 jira_ref=SWDE-52
  const fetchTimeoutMs = input.fetch_timeout_ms ?? 30000;
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-051 plan=phase-2/staggering/step-52-p2-001 jira_ref=SWDE-52
  const staggerMs = input.stagger_ms ?? 0;
  const staggerJitterMs = input.stagger_jitter_ms ?? 0;

  return {
    tool: {
      /**
       * feed.list — list all registered feeds with health and budget status.
       * REQ-FEED-007
       */
      "feed_list": tool({
        description:
          "List all registered feed sources with health status and today's budget usage. " +
          "Use this to see which feeds are configured, enabled, and their current health.",
        args: {
          include_disabled: tool.schema.boolean().optional()
            .describe("Include disabled feeds in the list (default: false)"),
        },
        async execute(args: Record<string, unknown>) {
          try {
            const includeDisabled = (args.include_disabled as boolean) ?? false;
            const globalConfig = loadGlobalFeedsConfig(feedsConfigPath);
            const configs = loadFeedConfigs(feedsDir);
            const visible = includeDisabled
              ? configs
              : configs.filter((c) => c.enabled);

            const feedList = visible.map((config) => {
              const state = loadFeedState(stateDir, config.id);
              const s = resetBudgetIfNewDay(state);
              const unhealthyAfter =
                globalConfig.health.unhealthy_after_failures;
              const isUnhealthy =
                s.consecutive_failures >= unhealthyAfter;
              return {
                id: config.id,
                name: config.name,
                type: config.type,
                enabled: config.enabled,
                url: config.source.url ?? null,
                tags: config.tags,
                last_poll_at: s.last_poll_at || null,
                last_success_at: s.last_success_at || null,
                last_error: s.last_error,
                consecutive_failures: s.consecutive_failures,
                healthy: !isUnhealthy,
                items_today: s.items_today,
                cost_today_usd: s.cost_today_usd,
                budget: {
                  max_items_per_day:
                    config.relevance.max_items_per_day ?? DEFAULT_MAX_ITEMS_PER_DAY,
                  max_cost_per_day_usd:
                    config.relevance.max_cost_per_day_usd ?? DEFAULT_MAX_COST_PER_DAY_USD,
                },
              };
            });

            return JSON.stringify({
              total: feedList.length,
              global_enabled: globalConfig.enabled,
              feeds: feedList,
            });
          } catch (err) {
            return JSON.stringify({
              error: err instanceof Error ? err.message : String(err),
            });
          }
        },
      }),

      /**
       * feed.status — detailed status for a single feed.
       * REQ-FEED-006, REQ-FEED-007
       */
      "feed_status": tool({
        description:
          "Get detailed health and budget status for a specific feed. " +
          "Includes last poll time, error history, and today's usage.",
        args: {
          feed_id: tool.schema.string()
            .describe("The ID of the feed to inspect"),
        },
        async execute(args: Record<string, unknown>) {
          try {
            const feedId = args.feed_id as string;
            validateFeedId(feedId);

            const configs = loadFeedConfigs(feedsDir);
            const config = configs.find((c) => c.id === feedId);
            if (!config) {
              return JSON.stringify({ error: `Feed "${feedId}" not found` });
            }

            const globalConfig = loadGlobalFeedsConfig(feedsConfigPath);
            const state = resetBudgetIfNewDay(loadFeedState(stateDir, feedId));
            const unhealthyAfter =
              globalConfig.health.unhealthy_after_failures;

            return JSON.stringify({
              id: config.id,
              name: config.name,
              type: config.type,
              enabled: config.enabled,
              source: config.source,
              poll_interval: config.poll_interval,
              target: config.target,
              tags: config.tags,
              health: {
                healthy: state.consecutive_failures < unhealthyAfter,
                consecutive_failures: state.consecutive_failures,
                last_error: state.last_error,
                last_poll_at: state.last_poll_at || null,
                last_success_at: state.last_success_at || null,
              },
              budget_today: {
                items: state.items_today,
                cost_usd: state.cost_today_usd,
                max_items: config.relevance.max_items_per_day ?? DEFAULT_MAX_ITEMS_PER_DAY,
                max_cost_usd:
                  config.relevance.max_cost_per_day_usd ?? DEFAULT_MAX_COST_PER_DAY_USD,
                items_remaining: Math.max(
                  0,
                  (config.relevance.max_items_per_day ?? DEFAULT_MAX_ITEMS_PER_DAY) -
                    state.items_today
                ),
              },
              pending_retry_count: state.pending_retry?.length ?? 0,
            });
          } catch (err) {
            return JSON.stringify({
              error: err instanceof Error ? err.message : String(err),
            });
          }
        },
      }),

      /**
       * feed.poll — poll one or all enabled feeds for new items.
       * Fetches, parses, deduplicates, evaluates relevance, and writes to memory.
       * REQ-FEED-050 through REQ-FEED-055
       */
      "feed_poll": tool({
        description:
          "Poll one or all enabled feeds for new items. " +
          "Fetches feed content, deduplicates against history, evaluates relevance " +
          "using the configured model, and writes relevant items to .memory-bank/signals/. " +
          "Use feed_id to poll a single feed or omit for all enabled feeds.",
        args: {
          feed_id: tool.schema.string().optional()
            .describe("Specific feed to poll. Omit to poll all enabled feeds."),
          dry_run: tool.schema.boolean().optional()
            .describe("If true, evaluate items and log decisions without writing to disk (default: false)."),
          force: tool.schema.boolean().optional()
            .describe("If true, re-poll even if the budget cap has been reached (default: false)."),
        },
        async execute(args: Record<string, unknown>) {
          try {
            const feedIdFilter = args.feed_id as string | undefined;
            const dryRun = (args.dry_run as boolean) ?? false;
            const force = (args.force as boolean) ?? false;

            if (feedIdFilter) validateFeedId(feedIdFilter);

            const globalConfig = loadGlobalFeedsConfig(feedsConfigPath);
            if (!globalConfig.enabled && !force) {
              return JSON.stringify({
                status: "disabled",
                message: "Feed ingestion is globally disabled. Use force=true to override.",
              });
            }

            // REQ-FEED-083: respect run_when configuration
            const runWhen = globalConfig.run_when ?? "always";
            if (runWhen === "idle") {
              // Skip when an active expert session is detected
              const sessionActive = process.env.OPENCODE_SESSION_ACTIVE === "1" ||
                                    process.env.OPENCODE_SESSION_ACTIVE === "true";
              if (sessionActive && !force) {
                return JSON.stringify({
                  status: "skipped",
                  reason: "run_when=idle: active expert session detected (OPENCODE_SESSION_ACTIVE)",
                  run_when: "idle",
                });
              }
            } else if (runWhen === "scheduled") {
              const schedule = globalConfig.schedule ?? "";
              if (schedule && !isScheduledTimeMatch(schedule) && !force) {
                return JSON.stringify({
                  status: "skipped",
                  reason: `run_when=scheduled: current time does not match schedule "${schedule}"`,
                  run_when: "scheduled",
                });
              }
            }
            // run_when=always: always proceed

            const configs = loadFeedConfigs(feedsDir).filter(
              (c) =>
                c.enabled &&
                (!feedIdFilter || c.id === feedIdFilter) &&
                ["rss", "atom", "api", "ical", "slack"].includes(c.type) // Phase 1: RSS/Atom; Phase 2: +api; Phase 3: +ical, +slack
            );

            if (configs.length === 0) {
              return JSON.stringify({
                status: "no_feeds",
                message: feedIdFilter
                  ? `No enabled RSS/Atom/API/iCal/Slack feed found with id "${feedIdFilter}"`
                  : "No enabled RSS/Atom/API/iCal/Slack feeds configured",
              });
            }

            const results: PollResult[] = [];

            // REQ-FEED-082: sum today's cost across all feeds before iterating
            // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-082 plan=phase-1-fix/step-52-001 jira_ref=SWDE-52
            let totalCostToday = configs.reduce((sum, c) => {
              const s = resetBudgetIfNewDay(loadFeedState(stateDir, c.id));
              return sum + s.cost_today_usd;
            }, 0);

            for (let i = 0; i < configs.length; i++) {
              const config = configs[i];
              // REQ-FEED-082: enforce global daily cost cap before each feed poll
              if (!checkGlobalCostBudget(totalCostToday, globalConfig.global_max_cost_per_day_usd)) {
                results.push({
                  feed_id: config.id,
                  fetched: false,
                  not_modified: false,
                  new_items: 0,
                  duplicate_items: 0,
                  evaluated: 0,
                  stored: 0,
                  budget_skipped: 0,
                  cost_incurred_usd: 0,
                  errors: ["Global daily cost cap reached"],
                });
                continue;
              }

              // REQ-FEED-052: each feed is independent — one failure must NOT block others
              try {
                const { result } = await pollFeed(
                  config,
                  stateDir,
                  memoryRoot,
                  evaluator,
                  serverUrl,
                  { dryRun },
                  fetchTimeoutMs
                );
                totalCostToday += result.cost_incurred_usd;
                results.push(result);

                // REQ-FEED-051: stagger between feeds to avoid burst load
                if (i < configs.length - 1 && staggerMs > 0) {
                  const jitter = staggerJitterMs > 0
                    ? Math.floor(Math.random() * staggerJitterMs)
                    : 0;
                  await Bun.sleep(staggerMs + jitter);
                }
              } catch (err) {
                results.push({
                  feed_id: config.id,
                  fetched: false,
                  not_modified: false,
                  new_items: 0,
                  duplicate_items: 0,
                  evaluated: 0,
                  stored: 0,
                  budget_skipped: 0,
                  cost_incurred_usd: 0,
                  errors: [err instanceof Error ? err.message : String(err)],
                });
              }
            }

            const summary = {
              dry_run: dryRun,
              feeds_polled: results.length,
              total_new_items: results.reduce((s, r) => s + r.new_items, 0),
              total_stored: results.reduce((s, r) => s + r.stored, 0),
              total_budget_skipped: results.reduce((s, r) => s + r.budget_skipped, 0),
              total_errors: results.reduce((s, r) => s + r.errors.length, 0),
              global_budget_capped: totalCostToday >= globalConfig.global_max_cost_per_day_usd,
              stagger_ms: staggerMs,
              run_when: runWhen,
              // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-060 plan=phase-1-fix/step-52-006 jira_ref=SWDE-52
              warnings: (() => {
                const warnings: string[] = [];
                if (usingDefaultEvaluator) {
                  warnings.push(
                    "Default evaluator active: all items are being DISCARDED (not stored). " +
                    "Configure a real evaluator via FeedIngestionPlugin({ evaluator: ... }) to enable relevance filtering."
                  );
                }
                return warnings;
              })(),
              results,
            };

            return JSON.stringify(summary);
          } catch (err) {
            return JSON.stringify({
              error: err instanceof Error ? err.message : String(err),
            });
          }
        },
      }),

      /**
       * feed.webhook — accept a push webhook payload for a configured webhook feed.
       * Validates HMAC signature (REQ-FEED-027), validates payload schema (REQ-FEED-026),
       * normalizes to FeedItem (REQ-FEED-028), evaluates relevance, and writes to memory.
       * REQ-FEED-025/026/027/028.
       * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-025 plan=phase-2/webhook-tool/step-52-p2-005 jira_ref=SWDE-52
       */
      "feed_webhook": tool({
        description:
          "Accept a push webhook payload for a configured webhook feed. " +
          "Validates HMAC signature (if configured), validates payload schema (if configured), " +
          "normalizes to FeedItem, evaluates relevance, and writes to memory. " +
          "REQ-FEED-025/026/027/028.",
        args: {
          feed_id: tool.schema.string()
            .describe("ID of the webhook feed to push to"),
          payload: tool.schema.string()
            .describe("The webhook payload as a JSON string (will be parsed internally)"),
          signature: tool.schema.string().optional()
            .describe("HMAC-SHA256 signature header value (format: sha256=<hex>)"),
        },
        async execute(args: Record<string, unknown>) {
          try {
            const feedId = args.feed_id as string;
            // Parse JSON string payload — supports both pre-stringified and (for backward compat) object input
            let payload: Record<string, unknown>;
            const rawPayload = args.payload;
            if (typeof rawPayload === "string") {
              try {
                payload = JSON.parse(rawPayload) as Record<string, unknown>;
              } catch (err) {
                return JSON.stringify({
                  error: "payload is not valid JSON",
                  details: err instanceof Error ? err.message : String(err),
                });
              }
            } else if (typeof rawPayload === "object" && rawPayload !== null) {
              payload = rawPayload as Record<string, unknown>;
            } else {
              return JSON.stringify({ error: "payload must be a JSON string or object" });
            }
            const signature = args.signature as string | undefined;

            validateFeedId(feedId);

            const configs = loadFeedConfigs(feedsDir);
            const config = configs.find((c) => c.id === feedId);
            if (!config) {
              return JSON.stringify({ error: `Feed "${feedId}" not found` });
            }
            if (config.type !== "webhook") {
              return JSON.stringify({
                error: `Feed "${feedId}" is not a webhook feed (type: ${config.type})`,
              });
            }
            if (!config.enabled) {
              return JSON.stringify({ error: `Feed "${feedId}" is disabled` });
            }

            // REQ-FEED-027: HMAC verification
            if (config.webhook?.secret) {
              if (!signature) {
                return JSON.stringify({
                  error: "Signature required but not provided",
                  status: 401,
                });
              }
              const payloadStr = JSON.stringify(payload);
              if (!verifyWebhookSignature(payloadStr, signature, config.webhook.secret)) {
                return JSON.stringify({ error: "Invalid signature", status: 401 });
              }
            }

            // REQ-FEED-026: payload schema validation
            if (config.webhook?.schema?.required) {
              const missing = config.webhook.schema.required.filter(
                (field) => !(field in payload)
              );
              if (missing.length > 0) {
                return JSON.stringify({
                  error: `Payload missing required fields: ${missing.join(", ")}`,
                  status: 400,
                });
              }
            }

            // REQ-FEED-028: normalize to FeedItem
            const item = normalizeWebhookPayload(payload, feedId);

            // Run through relevance evaluation + memory write
            const state = resetBudgetIfNewDay(loadFeedState(stateDir, feedId));

            // Budget checks (REQ-FEED-080, REQ-FEED-081)
            if (!checkItemBudget(state, config)) {
              return JSON.stringify({
                status: "budget_exceeded",
                message: "Daily item cap reached for this feed",
              });
            }
            if (!checkCostBudget(state, config)) {
              return JSON.stringify({
                status: "budget_exceeded",
                message: "Daily cost cap reached for this feed",
              });
            }

            let decision: RelevanceDecision;
            try {
              decision = await callEvaluatorWithTimeout(evaluator, item, config.relevance, serverUrl, config.relevance.timeout_ms ?? 30_000);
            } catch (err) {
              const msg = err instanceof Error ? err.message : String(err);
              return JSON.stringify({ error: `Relevance evaluation failed: ${msg}` });
            }

            // REQ-FEED-061: Validate required RelevanceDecision fields via shared helper
            // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-001 jira_ref=SWDE-52
            const webhookMissingFields = validateRelevanceDecision(decision);
            if (webhookMissingFields.length > 0) {
              return JSON.stringify({ error: `Evaluator returned incomplete RelevanceDecision: missing fields [${webhookMissingFields.join(', ')}]` });
            }

            if (decision.store) {
              try {
                await writeSignalNote(item, decision, config, getExpertMemoryPath(config, memoryRoot));
                state.items_today++;
                const itemCost = decision.cost_usd ?? 0;
                state.cost_today_usd += itemCost;
                await saveFeedState(stateDir, feedId, state);
                logEvent("webhook_stored", { feed_id: feedId, item_id: item.item_id });
                return JSON.stringify({
                  status: "stored",
                  item_id: item.item_id,
                  feed_id: feedId,
                });
              } catch (err) {
                const msg = err instanceof Error ? err.message : String(err);
                return JSON.stringify({ error: `Write failed: ${msg}` });
              }
            } else {
              logEvent("item_discarded", {
                feed_id: feedId,
                item_id: item.item_id,
                title: item.title.slice(0, 80),
                reason: decision.reason.slice(0, 200),
              });
              return JSON.stringify({
                status: "discarded",
                reason: decision.reason,
                feed_id: feedId,
              });
            }
          } catch (err) {
            return JSON.stringify({
              error: err instanceof Error ? err.message : String(err),
            });
          }
        },
      }),

      /**
       * feed.email — accept an email digest payload for a configured email feed.
       * Strips HTML from body (REQ-FEED-042), validates subject regex (REQ-FEED-041),
       * normalizes to FeedItem, evaluates relevance, and writes to memory.
       * Compatible with email webhook providers (Mailgun, SendGrid, Postmark).
       * REQ-FEED-040/041/042.
       * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-040 plan=phase-3/email-feed/step-52-p3-003 jira_ref=SWDE-52
       */
      "feed_email": tool({
        description:
          "Accept an email digest payload for a configured email feed. " +
          "Strips HTML from body (REQ-FEED-042), validates subject regex (REQ-FEED-041), " +
          "normalizes to FeedItem, evaluates relevance, and writes to memory. " +
          "Compatible with email webhook providers (Mailgun, SendGrid, Postmark).",
        args: {
          feed_id: tool.schema.string()
            .describe("ID of the email feed to push to"),
          subject: tool.schema.string()
            .describe("Email subject line"),
          body: tool.schema.string()
            .describe("Email body (HTML or plain text)"),
          from_email: tool.schema.string()
            .describe("Sender email address"),
          message_id: tool.schema.string().optional()
            .describe("Optional email Message-ID header for deduplication"),
        },
        async execute(args: Record<string, unknown>) {
          try {
            const feedId = args.feed_id as string;
            const subject = args.subject as string;
            const body = args.body as string;
            const fromEmail = args.from_email as string;
            const messageId = args.message_id as string | undefined;

            validateFeedId(feedId);

            const configs = loadFeedConfigs(feedsDir);
            const config = configs.find((c) => c.id === feedId);
            if (!config) {
              return JSON.stringify({ error: `Feed "${feedId}" not found` });
            }
            if (config.type !== "email") {
              return JSON.stringify({
                error: `Feed "${feedId}" is not an email feed (type: ${config.type})`,
              });
            }
            if (!config.enabled) {
              return JSON.stringify({ error: `Feed "${feedId}" is disabled` });
            }

            // REQ-FEED-041: subject regex filtering
            if (config.source.subject_regex) {
              const regex = new RegExp(config.source.subject_regex, "i");
              if (!regex.test(subject)) {
                return JSON.stringify({
                  status: "filtered",
                  reason: `Subject "${subject}" does not match regex "${config.source.subject_regex}"`,
                });
              }
            }

            // REQ-FEED-042: strip HTML from body
            const plainText = stripHtml(body);

            // Normalize to FeedItem
            const id = messageId ?? hashString(subject + fromEmail + new Date().toISOString());
            const item: FeedItem = {
              feed_id: feedId,
              item_id: hashString(feedId + ":" + id),
              title: subject,
              content: plainText,
              url: "",
              published_at: new Date().toISOString(),
              author: fromEmail,
              tags: ["email"],
              raw: { subject, from_email: fromEmail, message_id: messageId, body_length: body.length },
            };

            // Budget checks (REQ-FEED-080, REQ-FEED-081, REQ-FEED-082)
            const state = resetBudgetIfNewDay(loadFeedState(stateDir, feedId));
            if (!checkItemBudget(state, config)) {
              return JSON.stringify({ status: "budget_exceeded", message: "Daily item cap reached" });
            }
            if (!checkCostBudget(state, config)) {
              return JSON.stringify({ status: "budget_exceeded", message: "Daily cost cap reached" });
            }
            // REQ-FEED-082: global daily cost cap (load global config for cross-feed enforcement)
            const globalCfg = loadGlobalFeedsConfig(feedsConfigPath);
            const allStates = loadFeedConfigs(feedsDir);
            const globalCostToday = allStates.reduce((sum, c) => {
              const s = resetBudgetIfNewDay(loadFeedState(stateDir, c.id));
              return sum + s.cost_today_usd;
            }, 0);
            if (!checkGlobalCostBudget(globalCostToday, globalCfg.global_max_cost_per_day_usd)) {
              return JSON.stringify({ status: "budget_exceeded", message: "Global daily cost cap reached" });
            }

            // Relevance evaluation
            let decision: RelevanceDecision;
            try {
              decision = await callEvaluatorWithTimeout(evaluator, item, config.relevance, serverUrl, config.relevance.timeout_ms ?? 30_000);
            } catch (err) {
              return JSON.stringify({ error: `Evaluation failed: ${err instanceof Error ? err.message : String(err)}` });
            }

            // REQ-FEED-061: Validate required RelevanceDecision fields via shared helper
            // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-001 jira_ref=SWDE-52
            const emailMissingFields = validateRelevanceDecision(decision);
            if (emailMissingFields.length > 0) {
              return JSON.stringify({ error: `Evaluator returned incomplete RelevanceDecision: missing fields [${emailMissingFields.join(', ')}]` });
            }

            if (decision.store) {
              await writeSignalNote(item, decision, config, getExpertMemoryPath(config, memoryRoot));
              state.items_today++;
              state.cost_today_usd += decision.cost_usd ?? 0;
              await saveFeedState(stateDir, feedId, state);
              logEvent("email_stored", { feed_id: feedId, item_id: item.item_id, subject });
              return JSON.stringify({ status: "stored", item_id: item.item_id, feed_id: feedId });
            } else {
              logEvent("item_discarded", { feed_id: feedId, item_id: item.item_id, title: subject.slice(0, 80), reason: decision.reason.slice(0, 200) });
              return JSON.stringify({ status: "discarded", reason: decision.reason });
            }
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      /**
       * feed.analytics — feed effectiveness analytics (per-feed cost, store rate, value metrics).
       * Identifies the most and least effective feeds based on 7-day store rate data.
       * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-4/analytics/step-52-p4-003 jira_ref=SWDE-52
       */
      "feed_analytics": tool({
        description:
          "Feed effectiveness analytics — per-feed cost, store rate, and value metrics. " +
          "Identifies the most and least effective feeds based on 7-day store rate data. " +
          "Use this to tune relevance prompts and budget allocations.",
        args: {
          min_evaluations: tool.schema.number().optional()
            .describe("Minimum evaluations required to include a feed in effectiveness rankings (default: 5)"),
        },
        async execute(args: Record<string, unknown>) {
          try {
            const minEvaluations = (args.min_evaluations as number) ?? 5;
            const configs = loadFeedConfigs(feedsDir);
            const globalConfig = loadGlobalFeedsConfig(feedsConfigPath);

            let globalTotalEvaluated = 0;
            let globalTotalStored = 0;
            let globalCostToday = 0;

            const feedAnalytics = configs.map((config) => {
              const state = resetBudgetIfNewDay(loadFeedState(stateDir, config.id));
              globalCostToday += state.cost_today_usd;

              const sr = state.store_rate_7d;
              const totalEvaluated = sr?.items_evaluated ?? 0;
              const totalStored = sr?.items_stored ?? 0;
              globalTotalEvaluated += totalEvaluated;
              globalTotalStored += totalStored;

              const storeRate = getStoreRate(state);
              const costPerItemStored =
                totalStored > 0 && state.cost_today_usd > 0
                  ? Math.round((state.cost_today_usd / totalStored) * 10000) / 10000
                  : null;
              const maxItems = config.relevance.max_items_per_day ?? 100;
              const budgetUtilizationPct =
                maxItems > 0 ? Math.round((state.items_today / maxItems) * 100) : 0;

              const healthFlags: string[] = [];
              if (storeRate !== null && storeRate > 0.9) healthFlags.push("high_store_rate");
              if (storeRate !== null && storeRate < 0.05) healthFlags.push("low_store_rate");
              if (state.consecutive_failures >= (globalConfig.health.unhealthy_after_failures)) {
                healthFlags.push("unhealthy");
              }

              return {
                id: config.id,
                name: config.name,
                type: config.type,
                enabled: config.enabled,
                target_agent: config.target?.agent ?? null,
                store_rate_7d: storeRate,
                store_rate_window_start: sr?.window_start ?? null,
                 stale_window: sr
                   ? isStaleWindow(sr.window_start)
                   : false,
                total_evaluated_7d: totalEvaluated,
                total_stored_7d: totalStored,
                cost_today_usd: state.cost_today_usd,
                cost_per_item_stored: costPerItemStored,
                budget_utilization_pct: budgetUtilizationPct,
                pending_retry_count: state.pending_retry.length,
                health_flags: healthFlags,
                has_sufficient_data: totalEvaluated >= minEvaluations,
              };
            });

            // Effectiveness rankings (only feeds with sufficient data)
            const ranked = feedAnalytics
              .filter((f) => f.has_sufficient_data && f.store_rate_7d !== null)
              .sort((a, b) => (b.store_rate_7d ?? 0) - (a.store_rate_7d ?? 0));

            const mostEffective = ranked[0] ?? null;
            const leastEffective = ranked[ranked.length - 1] ?? null;

            const globalStoreRate =
              globalTotalEvaluated > 0
                ? Math.round((globalTotalStored / globalTotalEvaluated) * 100) / 100
                : null;

            return JSON.stringify({
              summary: {
                total_feeds: configs.length,
                feeds_with_data: ranked.length,
                global_store_rate: globalStoreRate,
                global_total_evaluated_7d: globalTotalEvaluated,
                global_total_stored_7d: globalTotalStored,
                global_cost_today_usd: Math.round(globalCostToday * 10000) / 10000,
                global_max_cost_usd: globalConfig.global_max_cost_per_day_usd,
                most_effective_feed: mostEffective
                  ? { id: mostEffective.id, store_rate_7d: mostEffective.store_rate_7d }
                  : null,
                least_effective_feed: leastEffective && leastEffective.id !== mostEffective?.id
                  ? { id: leastEffective.id, store_rate_7d: leastEffective.store_rate_7d }
                  : null,
              },
              feeds: feedAnalytics,
              // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-4/analytics/step-52-p4-003 jira_ref=SWDE-52
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      /**
       * feed.subscribe — associate or disassociate a feed with a target expert agent.
       * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-4/subscribe/step-52-p4-004 jira_ref=SWDE-52
       */
      "feed_subscribe": tool({
        description:
          "Associate or disassociate a feed with a target expert agent. " +
          "subscribe: sets target.agent on the feed config, routing future items to " +
          "the expert's memory bank ({memoryRoot}/{expert_name}/signals/). " +
          "unsubscribe: removes the target.agent assignment, reverting to default routing.",
        args: {
          feed_id: tool.schema.string()
            .describe("ID of the feed to update"),
          expert_name: tool.schema.string()
            .describe("Target expert agent name (e.g., 'security-review-axiom')"),
          action: tool.schema.enum(["subscribe", "unsubscribe"])
            .describe("subscribe: link feed to expert; unsubscribe: remove link"),
        },
        async execute(args: Record<string, unknown>) {
          try {
            const feedId = args.feed_id as string;
            const expertName = args.expert_name as string;
            const action = args.action as "subscribe" | "unsubscribe";

            validateFeedId(feedId);
            // Validate expert name — must be safe for filesystem path
            if (!/^[a-z0-9][a-z0-9-]{0,63}$/.test(expertName)) {
              return JSON.stringify({
                error: `Invalid expert_name "${expertName}". Must match ^[a-z0-9][a-z0-9-]{0,63}$`,
              });
            }

            // Find the feed config file
            const configPath = safeFeedPath(feedsDir, `${feedId}.yaml`);
            if (!existsSync(configPath)) {
              return JSON.stringify({ error: `Feed config file not found: ${feedId}.yaml` });
            }

            // Load and parse current YAML
            const rawContent = readFileSync(configPath, "utf-8");
            const parsed = yamlParse(rawContent) as Record<string, unknown>;

            // Ensure target section exists
            if (!parsed.target || typeof parsed.target !== "object") {
              parsed.target = {};
            }
            const target = parsed.target as Record<string, unknown>;

            const previousAgent = target.agent ?? null;

            if (action === "subscribe") {
              target.agent = expertName;
            } else {
              delete target.agent;
            }
            parsed.target = target;

            // Write updated config back atomically
            const { stringify: yamlStringifyFull } = await import("yaml");
            const updated = yamlStringifyFull(parsed);
            const tmp = `${configPath}.tmp.${Date.now()}`;
            await fsPromises.writeFile(tmp, updated, "utf-8");
            await fsPromises.rename(tmp, configPath);

            return JSON.stringify({
              status: "ok",
              feed_id: feedId,
              action,
              expert_name: expertName,
              previous_agent: previousAgent,
              target_agent: action === "subscribe" ? expertName : null,
              message:
                action === "subscribe"
                  ? `Feed "${feedId}" now routes to expert "${expertName}"`
                  : `Feed "${feedId}" expert routing removed`,
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),

      /**
       * feed.health — comprehensive health metrics for all configured feeds.
       * REQ-FEED-083: run_when idle behaviour (config-driven).
       * REQ-FEED-084: flags feeds with >90% store rate.
       * REQ-FEED-085: flags feeds with <5% store rate.
       * axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-3/health-dashboard/step-52-p3-004 jira_ref=SWDE-52
       */
      "feed_health": tool({
        description:
          "Get comprehensive health metrics for all configured feeds. " +
          "Shows per-feed health, budget usage, store rate analytics, and flagged feeds. " +
          "REQ-FEED-084: flags feeds with >90% store rate. REQ-FEED-085: flags feeds with <5% store rate.",
        args: {
          include_disabled: tool.schema.boolean().optional()
            .describe("Include disabled feeds (default: false)"),
        },
        async execute(args: Record<string, unknown>) {
          try {
            const includeDisabled = (args.include_disabled as boolean) ?? false;
            const globalConfig = loadGlobalFeedsConfig(feedsConfigPath);
            const configs = loadFeedConfigs(feedsDir);
            const visible = includeDisabled ? configs : configs.filter((c) => c.enabled);
            const unhealthyAfter = globalConfig.health.unhealthy_after_failures;

            let globalCostToday = 0;
            const feedHealthList = visible.map((config) => {
              const state = resetBudgetIfNewDay(loadFeedState(stateDir, config.id));
              globalCostToday += state.cost_today_usd;

              const storeRate = getStoreRate(state);
              let flagged: "high_store_rate" | "low_store_rate" | null = null;
              // REQ-FEED-084: >90% store rate flagged
              if (storeRate !== null && storeRate > 0.9) flagged = "high_store_rate";
              // REQ-FEED-085: <5% store rate flagged
              else if (storeRate !== null && storeRate < 0.05) flagged = "low_store_rate";

              return {
                id: config.id,
                name: config.name,
                type: config.type,
                enabled: config.enabled,
                healthy: state.consecutive_failures < unhealthyAfter,
                consecutive_failures: state.consecutive_failures,
                last_poll_at: state.last_poll_at || null,
                last_error: state.last_error,
                budget_today: {
                  items: state.items_today,
                  cost_usd: state.cost_today_usd,
                  max_items: config.relevance.max_items_per_day ?? 100,
                  max_cost_usd: config.relevance.max_cost_per_day_usd ?? 1.0,
                },
                pending_retry_count: state.pending_retry.length,
                store_rate_7d: storeRate !== null ? Math.round(storeRate * 100) / 100 : null,
                store_rate_window_start: state.store_rate_7d?.window_start ?? null,
                 stale_window: state.store_rate_7d
                   ? isStaleWindow(state.store_rate_7d.window_start)
                   : false,
                store_rate_7d_raw: state.store_rate_7d ?? null,
                flagged,
              };
            });

            const flaggedFeeds = feedHealthList.filter((f) => f.flagged !== null);

            return JSON.stringify({
              total_feeds: feedHealthList.length,
              healthy_feeds: feedHealthList.filter((f) => f.healthy).length,
              unhealthy_feeds: feedHealthList.filter((f) => !f.healthy).length,
              flagged_feeds: flaggedFeeds.length,
              global_cost_today_usd: Math.round(globalCostToday * 10000) / 10000,
              global_max_cost_usd: globalConfig.global_max_cost_per_day_usd,
              feeds: feedHealthList,
              flagged: flaggedFeeds.map((f) => ({ id: f.id, flagged: f.flagged, store_rate_7d: f.store_rate_7d })),
            });
          } catch (err) {
            return JSON.stringify({ error: err instanceof Error ? err.message : String(err) });
          }
        },
      }),
    },
  };
};
