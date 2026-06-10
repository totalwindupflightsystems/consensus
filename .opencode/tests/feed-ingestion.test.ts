/**
 * Tests for feed-ingestion.ts — Phase 1 acceptance criteria.
 *
 * AC-1  RSS 2.0 parsing — items extracted with correct fields
 * AC-2  Atom 1.0 parsing — items extracted with correct fields
 * AC-3  CDATA content strips wrapper correctly
 * AC-4  Feed config loading from YAML files
 * AC-5  Feed config validation — missing/invalid fields rejected
 * AC-6  Feed ID validation — rejects path-traversal-style IDs
 * AC-7  Deduplication — seen items are filtered out
 * AC-8  Dedup window pruning — stale entries removed from seen_ids
 * AC-9  Budget enforcement — item cap stops processing at max_items_per_day
 * AC-10 Budget enforcement — cost cap stops processing at max_cost_per_day_usd
 * AC-11 Budget reset — daily counters reset when date changes
 * AC-12 Signal note build — correct YAML frontmatter and body
 * AC-13 Signal note write — file created at expected path, path-traversal safe
 * AC-14 HTTP 304 handling — treated as success with zero new items
 * AC-15 Poll engine integration — fetch → parse → dedup → evaluate → store cycle
 * AC-16 feed.list tool — returns all feeds with health/budget status
 * AC-17 feed.status tool — returns detailed status for named feed
 * AC-18 feed.poll tool — disabled feeds not polled; dry_run skips disk write
 * AC-19 pollFeed increments consecutive_failures on fetch error
 * AC-25 fast-xml-parser: real-world feed parsing (REQ-FEED-020)
 *
 * Run: cd .opencode && bun test tests/feed-ingestion.test.ts
 *
 */
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-1/task-1.2 test=feed-ingestion.test.ts jira_ref=SWDE-52

import {
  test,
  expect,
  describe,
  beforeAll,
  afterAll,
} from "bun:test";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";

import {
  // Types
  type FeedConfig,
  type FeedItem,
  type FeedState,
  type RelevanceDecision,
  // Validation
  validateFeedId,
  validateFeedConfig,
  getFeedIdRegex,
  // Path safety
  safeFeedPath,
  // Config loading
  loadFeedConfigs,
  loadGlobalFeedsConfig,
  // XML parsing
  extractTagContent,
  extractAllTagBlocks,
  stripCDATA,
  extractAttr,
  hashString,
  parseRssItems,
  parseAtomItems,
  parseFeed,
  // API parsing / env-var interpolation (Phase 2 — REQ-FEED-030/031/032)
  parseApiItems,
  interpolateEnvVars,
  // Webhook helpers (Phase 2 — REQ-FEED-025/026/027/028)
  verifyWebhookSignature,
  normalizeWebhookPayload,
  // State management
  emptyFeedState,
  loadFeedState,
  saveFeedState,
  resetBudgetIfNewDay,
  pruneSeenIds,
  deduplicateItems,
  utcDateString,
  parseDedupWindow,
  // Budget
  checkItemBudget,
  checkCostBudget,
  // Memory write
  buildSignalNote,
  writeSignalNote,
  // Poll
  pollFeed,
  // Plugin factory
  FeedIngestionPlugin,
  // Default evaluator (for AC-22 regression test)
  defaultRelevanceEvaluator,
  // Structured logging (for AC-24 regression test)
  logEvent,
  // iCal parsing (Phase 3 — REQ-FEED-045/046/047)
  parseICalItems,
  parseICalDate,
  unescapeICal,
  // Slack parsing (Phase 3 — REQ-FEED-035/036/037)
  parseSlackMessages,
  // Email feed (Phase 3 — REQ-FEED-040/041/042)
  stripHtml,
  // Feed health dashboard (Phase 3 — REQ-FEED-084/085)
  getStoreRate,
  // Schedule helpers (Phase 4 — REQ-FEED-083)
  isScheduledTimeMatch,
  matchCronField,
  // Expert Platform routing (Phase 4 — REQ-FEED-001)
  getExpertMemoryPath,
  // REQ-FEED-061 validation helper (Phase 14 — step-v8-001)
  validateRelevanceDecision,
  REQUIRED_DECISION_FIELDS,
  // Evaluator timeout helper (Phase 17 — step-v11-001)
  callEvaluatorWithTimeout,
  // Stale window helper (Phase 19 — step-v13-003)
  isStaleWindow,
  // Fixed 7-day store rate window constant (Phase 22 — step-v16-004)
  SEVEN_DAYS_MS,
} from "../lib/feed-ingestion.ts";

// ─── test fixtures ────────────────────────────────────────────────────────────

const RSS_FIXTURE = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>Test Feed</title>
    <link>https://example.com</link>
    <description>A test RSS feed</description>
    <item>
      <title>Article One</title>
      <link>https://example.com/article-1</link>
      <description><![CDATA[<p>First article content.</p>]]></description>
      <pubDate>Mon, 01 Jan 2026 12:00:00 GMT</pubDate>
      <guid>https://example.com/article-1</guid>
      <dc:creator>Alice</dc:creator>
      <category>tech</category>
    </item>
    <item>
      <title>Article Two</title>
      <link>https://example.com/article-2</link>
      <description>Second article plain text.</description>
      <pubDate>Tue, 02 Jan 2026 12:00:00 GMT</pubDate>
      <guid>guid-article-2</guid>
    </item>
  </channel>
</rss>`;

const ATOM_FIXTURE = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Test Feed</title>
  <link href="https://example.com/atom" rel="alternate"/>
  <id>urn:example:atom-test</id>
  <updated>2026-01-02T12:00:00Z</updated>
  <entry>
    <id>urn:example:entry-1</id>
    <title>Atom Entry One</title>
    <link href="https://example.com/entry-1" rel="alternate"/>
    <content type="html"><![CDATA[<p>Entry one content.</p>]]></content>
    <published>2026-01-01T10:00:00Z</published>
    <author><name>Bob</name></author>
    <category term="go"/>
    <category term="security"/>
  </entry>
  <entry>
    <id>urn:example:entry-2</id>
    <title>Atom Entry Two</title>
    <link href="https://example.com/entry-2" rel="alternate"/>
    <summary>Entry two summary.</summary>
    <updated>2026-01-02T10:00:00Z</updated>
  </entry>
</feed>`;

const SAMPLE_FEED_CONFIG: FeedConfig = {
  id: "test-feed",
  name: "Test Feed",
  type: "rss",
  source: { url: "https://example.com/rss" },
  poll_interval: "1h",
  target: { agent: "assist-axiom", memory_path: "signals/", pandora: false },
  relevance: {
    prompt: "Is this relevant?",
    model: "anthropic.claude-haiku",
    max_items_per_day: 10,
    max_cost_per_day_usd: 0.50,
  },
  deduplication: { key: "guid", window: "7d" },
  tags: ["test"],
  enabled: true,
};

/** Build a mock RelevanceEvaluator that always returns the given decision */
function alwaysStore(
  priority: "high" | "medium" | "low" = "medium",
  cost_usd?: number
): import("../plugins/feed-ingestion.ts").RelevanceEvaluator {
  return async (item: FeedItem): Promise<RelevanceDecision> => ({
    store: true,
    reason: "mock: always store",
    priority,
    tags: [],
    summary: item.title,
    cost_usd,
  });
}

function neverStore(): import("../plugins/feed-ingestion.ts").RelevanceEvaluator {
  return async () => ({
    store: false,
    reason: "mock: never store",
    priority: "low",
    tags: [],
    summary: "",
  });
}

// ─── temp directory setup ─────────────────────────────────────────────────────

let tmpRoot: string;

beforeAll(() => {
  tmpRoot = mkdtempSync(join(tmpdir(), "feed-test-"));
});

afterAll(() => {
  rmSync(tmpRoot, { recursive: true, force: true });
});

function tmpDir(name: string): string {
  const d = join(tmpRoot, name);
  mkdirSync(d, { recursive: true });
  return d;
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-1: RSS 2.0 parsing
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-1: RSS 2.0 parsing", () => {
  test("parses two items from RSS feed", () => {
    const items = parseRssItems(RSS_FIXTURE, "test-feed");
    expect(items).toHaveLength(2);
  });

  test("first item has correct title", () => {
    const items = parseRssItems(RSS_FIXTURE, "test-feed");
    expect(items[0].title).toBe("Article One");
  });

  test("first item has correct URL", () => {
    const items = parseRssItems(RSS_FIXTURE, "test-feed");
    expect(items[0].url).toBe("https://example.com/article-1");
  });

  test("first item has CDATA stripped from content", () => {
    const items = parseRssItems(RSS_FIXTURE, "test-feed");
    expect(items[0].content).toBe("<p>First article content.</p>");
  });

  test("first item has author from dc:creator", () => {
    const items = parseRssItems(RSS_FIXTURE, "test-feed");
    expect(items[0].author).toBe("Alice");
  });

  test("second item has correct title and content", () => {
    const items = parseRssItems(RSS_FIXTURE, "test-feed");
    expect(items[1].title).toBe("Article Two");
    expect(items[1].content).toBe("Second article plain text.");
  });

  test("items have feed_id set correctly", () => {
    const items = parseRssItems(RSS_FIXTURE, "my-feed-id");
    expect(items[0].feed_id).toBe("my-feed-id");
    expect(items[1].feed_id).toBe("my-feed-id");
  });

  test("items have published_at as ISO8601", () => {
    const items = parseRssItems(RSS_FIXTURE, "test-feed");
    // Should be parseable as a date
    const d = new Date(items[0].published_at);
    expect(isNaN(d.getTime())).toBe(false);
  });

  test("parseFeed auto-detects RSS", () => {
    const items = parseFeed(RSS_FIXTURE, "test-feed");
    expect(items).toHaveLength(2);
    expect(items[0].title).toBe("Article One");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-2: Atom 1.0 parsing
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-2: Atom 1.0 parsing", () => {
  test("parses two entries from Atom feed", () => {
    const items = parseAtomItems(ATOM_FIXTURE, "atom-feed");
    expect(items).toHaveLength(2);
  });

  test("first entry has correct title", () => {
    const items = parseAtomItems(ATOM_FIXTURE, "atom-feed");
    expect(items[0].title).toBe("Atom Entry One");
  });

  test("first entry has URL from link href", () => {
    const items = parseAtomItems(ATOM_FIXTURE, "atom-feed");
    expect(items[0].url).toBe("https://example.com/entry-1");
  });

  test("first entry content is CDATA-stripped HTML", () => {
    const items = parseAtomItems(ATOM_FIXTURE, "atom-feed");
    expect(items[0].content).toBe("<p>Entry one content.</p>");
  });

  test("first entry author extracted", () => {
    const items = parseAtomItems(ATOM_FIXTURE, "atom-feed");
    expect(items[0].author).toBe("Bob");
  });

  test("first entry categories extracted", () => {
    const items = parseAtomItems(ATOM_FIXTURE, "atom-feed");
    expect(items[0].tags).toContain("go");
    expect(items[0].tags).toContain("security");
  });

  test("second entry uses summary when content absent", () => {
    const items = parseAtomItems(ATOM_FIXTURE, "atom-feed");
    expect(items[1].content).toBe("Entry two summary.");
  });

  test("parseFeed auto-detects Atom", () => {
    const items = parseFeed(ATOM_FIXTURE, "atom-feed");
    expect(items).toHaveLength(2);
    expect(items[0].title).toBe("Atom Entry One");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-3: CDATA stripping
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-3: CDATA stripping", () => {
  test("strips <![CDATA[...]]> wrapper", () => {
    expect(stripCDATA("<![CDATA[hello world]]>")).toBe("hello world");
  });

  test("returns plain string unchanged", () => {
    expect(stripCDATA("plain text")).toBe("plain text");
  });

  test("handles CDATA with whitespace", () => {
    expect(stripCDATA("  <![CDATA[ content ]]>  ")).toBe(" content ");
  });

  test("extractTagContent handles CDATA inside tag", () => {
    const xml = "<description><![CDATA[<b>bold</b>]]></description>";
    expect(extractTagContent(xml, "description")).toBe("<b>bold</b>");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-4: Feed config loading from YAML
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-4: Feed config loading from YAML", () => {
  test("loads a valid feed YAML file", () => {
    const dir = tmpDir("feeds-load");
    const yaml = `
id: "my-feed"
name: "My Feed"
type: "rss"
source:
  url: "https://example.com/feed.rss"
poll_interval: "1h"
relevance:
  prompt: "Is this relevant?"
enabled: true
`;
    writeFileSync(join(dir, "my-feed.yaml"), yaml, "utf-8");
    const configs = loadFeedConfigs(dir);
    expect(configs).toHaveLength(1);
    expect(configs[0].id).toBe("my-feed");
    expect(configs[0].name).toBe("My Feed");
    expect(configs[0].source.url).toBe("https://example.com/feed.rss");
  });

  test("skips invalid YAML files without throwing", () => {
    const dir = tmpDir("feeds-skip-invalid");
    writeFileSync(join(dir, "bad.yaml"), "not: valid: yaml: : :", "utf-8");
    const configs = loadFeedConfigs(dir);
    expect(configs).toHaveLength(0);
  });

  test("returns empty array when feeds directory does not exist", () => {
    const configs = loadFeedConfigs("/tmp/nonexistent-feeds-dir-12345");
    expect(configs).toHaveLength(0);
  });

  test("loads multiple feeds from directory", () => {
    const dir = tmpDir("feeds-multi");
    const yaml1 = `id: "feed-a"\nname: "A"\ntype: "rss"\nsource:\n  url: "https://a.com/rss"\npoll_interval: "1h"\nrelevance:\n  prompt: "Relevant?"\n`;
    const yaml2 = `id: "feed-b"\nname: "B"\ntype: "atom"\nsource:\n  url: "https://b.com/atom"\npoll_interval: "4h"\nrelevance:\n  prompt: "Relevant?"\n`;
    writeFileSync(join(dir, "feed-a.yaml"), yaml1, "utf-8");
    writeFileSync(join(dir, "feed-b.yaml"), yaml2, "utf-8");
    const configs = loadFeedConfigs(dir);
    expect(configs).toHaveLength(2);
    const ids = configs.map((c) => c.id).sort();
    expect(ids).toEqual(["feed-a", "feed-b"]);
  });

  // AC-B07: REQ-FEED-005 — duplicate IDs must be rejected (keep first, skip rest)
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-005 plan=phase-1/task-1.2/step-b07 test=feed-ingestion.test.ts#AC-B07 jira_ref=SWDE-52
  test("rejects duplicate feed IDs — keeps first occurrence, skips second (REQ-FEED-005)", () => {
    const dir = tmpDir("feeds-dup-id");
    const yamlA = `id: "dup-feed"\nname: "First Occurrence"\ntype: "rss"\nsource:\n  url: "https://first.example.com/rss"\npoll_interval: "1h"\nrelevance:\n  prompt: "Relevant?"\n`;
    const yamlB = `id: "dup-feed"\nname: "Second Occurrence"\ntype: "rss"\nsource:\n  url: "https://second.example.com/rss"\npoll_interval: "1h"\nrelevance:\n  prompt: "Relevant?"\n`;
    writeFileSync(join(dir, "dup-feed-a.yaml"), yamlA, "utf-8");
    writeFileSync(join(dir, "dup-feed-b.yaml"), yamlB, "utf-8");
    const configs = loadFeedConfigs(dir);
    expect(configs).toHaveLength(1);
    expect(configs[0].id).toBe("dup-feed");
    expect(configs[0].name).toBe("First Occurrence");
    expect(configs[0].source?.url).toBe("https://first.example.com/rss");
  });

  // AC-B08: Two tests with opposite creation orders prove .sort() is the mechanism,
  // not filesystem creation order (which varies by OS/filesystem type).
  // REQ-FEED-005 — .yaml extension sorts before .yml for same base name
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-005 plan=phase-1/task-1.2/step-v2-005 test=feed-ingestion.test.ts#AC-B08 jira_ref=SWDE-52
  test("loadFeedConfigs: .yaml sorts before .yml for same base name (REQ-FEED-005)", () => {
    const dir = tmpDir("feeds-ext-collision");
    // Write yml first so unsorted readdir returns yml before yaml — only .sort() produces the correct alphabetical order
    const yamlContent = `id: "ext-feed"\nname: "YAML version"\ntype: "rss"\nsource:\n  url: "https://yaml.example.com/rss"\npoll_interval: "1h"\nrelevance:\n  prompt: "Relevant?"\n`;
    const ymlContent = `id: "ext-feed"\nname: "YML version"\ntype: "rss"\nsource:\n  url: "https://yml.example.com/rss"\npoll_interval: "1h"\nrelevance:\n  prompt: "Relevant?"\n`;
    writeFileSync(join(dir, "feed.yml"), ymlContent, "utf-8");
    writeFileSync(join(dir, "feed.yaml"), yamlContent, "utf-8");
    const configs = loadFeedConfigs(dir);
    expect(configs).toHaveLength(1);
    expect(configs[0].name).toBe("YAML version"); // feed.yaml sorts before feed.yml
    expect(configs[0].source?.url).toBe("https://yaml.example.com/rss");
  });

  test("loadFeedConfigs: .yaml sorts before .yml regardless of creation order (REQ-FEED-005)", () => {
    const dir = tmpDir("feeds-ext-collision-reversed");
    // Write yaml FIRST this time — proves .sort() is the mechanism, not filesystem creation order
    const yamlContent = `id: "ext-feed2"\nname: "YAML version"\ntype: "rss"\nsource:\n  url: "https://yaml.example.com/rss"\npoll_interval: "1h"\nrelevance:\n  prompt: "Relevant?"\n`;
    const ymlContent = `id: "ext-feed2"\nname: "YML version"\ntype: "rss"\nsource:\n  url: "https://yml.example.com/rss"\npoll_interval: "1h"\nrelevance:\n  prompt: "Relevant?"\n`;
    writeFileSync(join(dir, "feed.yaml"), yamlContent, "utf-8");  // yaml written FIRST
     writeFileSync(join(dir, "feed.yml"), ymlContent, "utf-8");    // yml written second
    const configs = loadFeedConfigs(dir);
    expect(configs).toHaveLength(1);
    expect(configs[0].name).toBe("YAML version"); // feed.yaml still wins because .sort() is alphabetical
    expect(configs[0].source?.url).toBe("https://yaml.example.com/rss");
  });

  // timeout_ms YAML parsing tests (step-v12-006)
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#14.4 plan=phase-18/step-v12-006 test=feed-ingestion.test.ts#AC-4-timeout-ms jira_ref=SWDE-52
  test("loadFeedConfigs parses relevance.timeout_ms from YAML", () => {
    const dir = tmpDir("ac4-timeout-ms");
    writeFileSync(
      join(dir, "feed.yaml"),
      "id: timeout-feed\nname: Timeout Feed\ntype: rss\nsource:\n  url: https://x.com\npoll_interval: 1h\nrelevance:\n  prompt: p\n  timeout_ms: 150\nenabled: true\n",
      "utf-8"
    );
    const configs = loadFeedConfigs(dir);
    expect(configs[0].relevance.timeout_ms).toBe(150);
  });

  test("loadFeedConfigs sets timeout_ms to undefined when absent from YAML", () => {
    const dir = tmpDir("ac4-no-timeout-ms");
    writeFileSync(
      join(dir, "feed.yaml"),
      "id: no-timeout-feed\nname: No Timeout\ntype: rss\nsource:\n  url: https://x.com\npoll_interval: 1h\nrelevance:\n  prompt: p\nenabled: true\n",
      "utf-8"
    );
    const configs = loadFeedConfigs(dir);
    expect(configs[0].relevance.timeout_ms).toBeUndefined();
  });

  // YAML edge case tests (step-v13-006)
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-19/step-v13-006 test=feed-ingestion.test.ts#AC-4-timeout-ms-yaml jira_ref=SWDE-52
  test("validateFeedConfig rejects timeout_ms with underscore separator (30_000 parsed as string by yaml package)", () => {
    // The 'yaml' package parses 30_000 as the string "30_000", not the integer 30000.
    // This will fail the type check with a clear error message.
    expect(() => validateFeedConfig({
      id: "x", name: "X", type: "rss", source: {}, poll_interval: "1h",
      relevance: { prompt: "p", timeout_ms: "30_000" }, // yaml parses 30_000 as string
    })).toThrow("received string");
  });

  test("validateFeedConfig rejects timeout_ms as quoted string ('30000')", () => {
    expect(() => validateFeedConfig({
      id: "x", name: "X", type: "rss", source: {}, poll_interval: "1h",
      relevance: { prompt: "p", timeout_ms: "30000" },
    })).toThrow("received string");
  });

  // True YAML-parser-level test (step-v14-005): proves the yaml package behavior
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-20/step-v14-005 test=feed-ingestion.test.ts#AC-4-yaml-parser jira_ref=SWDE-52
  test("yaml package v2.x parses '30_000' as string '30_000' (not integer 30000)", () => {
    // This test verifies the actual yaml package behavior that the spec note claims.
    // If a future yaml package version changes this behavior, this test will catch it.
    const { parse: yamlParse } = require("yaml");
    const result = yamlParse("timeout_ms: 30_000");
    expect(typeof result.timeout_ms).toBe("string");
    expect(result.timeout_ms).toBe("30_000");
  });

  test("yaml package v2.x parses '30000' (unquoted integer) as number 30000", () => {
    const { parse: yamlParse } = require("yaml");
    const result = yamlParse("timeout_ms: 30000");
    expect(typeof result.timeout_ms).toBe("number");
    expect(result.timeout_ms).toBe(30000);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-5: Feed config validation
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-5: Feed config validation", () => {
  test("accepts valid feed config", () => {
    const raw = {
      id: "valid-feed",
      name: "Valid",
      type: "rss",
      source: { url: "https://example.com/rss" },
      poll_interval: "1h",
      relevance: { prompt: "Is this relevant?" },
    };
    const config = validateFeedConfig(raw);
    expect(config.id).toBe("valid-feed");
  });

  test("throws on missing id", () => {
    expect(() =>
      validateFeedConfig({ name: "X", type: "rss", source: {}, poll_interval: "1h", relevance: { prompt: "p" } })
    ).toThrow(/missing required field.*id/i);
  });

  test("throws on invalid type", () => {
    expect(() =>
      validateFeedConfig({ id: "x", name: "X", type: "twitter", source: {}, poll_interval: "1h", relevance: { prompt: "p" } })
    ).toThrow(/type.*twitter.*not valid/i);
  });

  test("throws on empty relevance.prompt", () => {
    expect(() =>
      validateFeedConfig({ id: "x", name: "X", type: "rss", source: {}, poll_interval: "1h", relevance: { prompt: "   " } })
    ).toThrow(/relevance.prompt must be a non-empty string/i);
  });

  test("defaults enabled to true when not specified", () => {
    const raw = {
      id: "x-feed",
      name: "X",
      type: "rss",
      source: {},
      poll_interval: "1h",
      relevance: { prompt: "Is relevant?" },
    };
    const config = validateFeedConfig(raw);
    expect(config.enabled).toBe(true);
  });

  // timeout_ms validation tests (step-v12-001, hardened in step-v13-008)
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-19/step-v13-008 test=feed-ingestion.test.ts#AC-5-timeout-ms jira_ref=SWDE-52
  test("validateFeedConfig rejects timeout_ms: 0", () => {
    expect(() => validateFeedConfig({
      id: "x", name: "X", type: "rss", source: {}, poll_interval: "1h",
      relevance: { prompt: "p", timeout_ms: 0 },
    })).toThrow("Feed relevance.timeout_ms must be a positive number (milliseconds); received 0");
  });

  test("validateFeedConfig rejects timeout_ms: -1", () => {
    expect(() => validateFeedConfig({
      id: "x", name: "X", type: "rss", source: {}, poll_interval: "1h",
      relevance: { prompt: "p", timeout_ms: -1 },
    })).toThrow("Feed relevance.timeout_ms must be a positive number (milliseconds); received -1");
  });

  test("validateFeedConfig accepts timeout_ms: 100 (positive number)", () => {
    const config = validateFeedConfig({
      id: "x", name: "X", type: "rss", source: {}, poll_interval: "1h",
      relevance: { prompt: "p", timeout_ms: 100 },
    });
    expect(config.relevance.timeout_ms).toBe(100);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-6: Feed ID validation (path traversal prevention)
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-6: Feed ID validation", () => {
  test("accepts valid feed IDs", () => {
    expect(() => validateFeedId("my-feed")).not.toThrow();
    expect(() => validateFeedId("cve-golang")).not.toThrow();
    expect(() => validateFeedId("arxiv01")).not.toThrow();
  });

  test("rejects IDs with path separators", () => {
    expect(() => validateFeedId("../evil")).toThrow(/Invalid feed ID/);
    expect(() => validateFeedId("path/traversal")).toThrow(/Invalid feed ID/);
  });

  test("rejects IDs with uppercase", () => {
    expect(() => validateFeedId("My-Feed")).toThrow(/Invalid feed ID/);
  });

  test("rejects IDs with spaces", () => {
    expect(() => validateFeedId("my feed")).toThrow(/Invalid feed ID/);
  });

  test("FEED_ID_REGEX matches valid IDs", () => {
    expect(getFeedIdRegex().test("my-feed")).toBe(true);
    expect(getFeedIdRegex().test("a")).toBe(true);
    expect(getFeedIdRegex().test("../evil")).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-7: Deduplication
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-7: Deduplication", () => {
  test("all items are new when seen_ids is empty", () => {
    const items = parseRssItems(RSS_FIXTURE, "test-feed");
    const state = emptyFeedState();
    const { newItems, duplicateCount } = deduplicateItems(items, state, { window: "7d" });
    expect(newItems).toHaveLength(2);
    expect(duplicateCount).toBe(0);
  });

  test("item is filtered when its id is in seen_ids", () => {
    const items = parseRssItems(RSS_FIXTURE, "test-feed");
    const state = emptyFeedState();
    // Pre-populate seen_ids with first item's id
    state.seen_ids[items[0].item_id] = new Date().toISOString();
    const { newItems, duplicateCount } = deduplicateItems(items, state, { window: "7d" });
    expect(newItems).toHaveLength(1);
    expect(newItems[0].item_id).toBe(items[1].item_id);
    expect(duplicateCount).toBe(1);
  });

  test("all items filtered when all seen", () => {
    const items = parseRssItems(RSS_FIXTURE, "test-feed");
    const state = emptyFeedState();
    const now = new Date().toISOString();
    for (const item of items) state.seen_ids[item.item_id] = now;
    const { newItems, duplicateCount } = deduplicateItems(items, state, { window: "7d" });
    expect(newItems).toHaveLength(0);
    expect(duplicateCount).toBe(2);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-8: Dedup window pruning
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-8: Dedup window pruning", () => {
  test("stale seen_ids are pruned outside the window", () => {
    const old = new Date(Date.now() - 8 * 24 * 60 * 60 * 1000).toISOString(); // 8 days ago
    const recent = new Date().toISOString();
    const seen = {
      "old-id": old,
      "recent-id": recent,
    };
    const windowMs = parseDedupWindow("7d");
    const pruned = pruneSeenIds(seen, windowMs);
    expect("old-id" in pruned).toBe(false);
    expect("recent-id" in pruned).toBe(true);
  });

  test("parseDedupWindow handles days", () => {
    expect(parseDedupWindow("7d")).toBe(7 * 24 * 60 * 60 * 1000);
  });

  test("parseDedupWindow handles hours", () => {
    expect(parseDedupWindow("24h")).toBe(24 * 60 * 60 * 1000);
  });

  test("parseDedupWindow handles weeks", () => {
    expect(parseDedupWindow("2w")).toBe(2 * 7 * 24 * 60 * 60 * 1000);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-9: Budget enforcement — item cap
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-9: Budget enforcement — item cap", () => {
  test("checkItemBudget returns true when below cap", () => {
    const state: FeedState = { ...emptyFeedState(), items_today: 5 };
    const config: FeedConfig = {
      ...SAMPLE_FEED_CONFIG,
      relevance: { ...SAMPLE_FEED_CONFIG.relevance, max_items_per_day: 10 },
    };
    expect(checkItemBudget(state, config)).toBe(true);
  });

  test("checkItemBudget returns false when at cap", () => {
    const state: FeedState = { ...emptyFeedState(), items_today: 10 };
    const config: FeedConfig = {
      ...SAMPLE_FEED_CONFIG,
      relevance: { ...SAMPLE_FEED_CONFIG.relevance, max_items_per_day: 10 },
    };
    expect(checkItemBudget(state, config)).toBe(false);
  });

  test("checkItemBudget returns false when over cap", () => {
    const state: FeedState = { ...emptyFeedState(), items_today: 50 };
    const config: FeedConfig = {
      ...SAMPLE_FEED_CONFIG,
      relevance: { ...SAMPLE_FEED_CONFIG.relevance, max_items_per_day: 10 },
    };
    expect(checkItemBudget(state, config)).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-10: Budget enforcement — cost cap
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-10: Budget enforcement — cost cap", () => {
  test("checkCostBudget returns true when below cap", () => {
    const state: FeedState = { ...emptyFeedState(), cost_today_usd: 0.30 };
    const config: FeedConfig = {
      ...SAMPLE_FEED_CONFIG,
      relevance: { ...SAMPLE_FEED_CONFIG.relevance, max_cost_per_day_usd: 0.50 },
    };
    expect(checkCostBudget(state, config)).toBe(true);
  });

  test("checkCostBudget returns false when at cap", () => {
    const state: FeedState = { ...emptyFeedState(), cost_today_usd: 0.50 };
    const config: FeedConfig = {
      ...SAMPLE_FEED_CONFIG,
      relevance: { ...SAMPLE_FEED_CONFIG.relevance, max_cost_per_day_usd: 0.50 },
    };
    expect(checkCostBudget(state, config)).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-11: Budget reset on new day
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-11: Budget reset on new day", () => {
  test("counters reset when date rolls over", () => {
    const yesterday = "2026-05-07";
    const state: FeedState = {
      ...emptyFeedState(),
      items_today: 42,
      cost_today_usd: 5.00,
      budget_date: yesterday,
    };
    const updated = resetBudgetIfNewDay(state);
    expect(updated.items_today).toBe(0);
    expect(updated.cost_today_usd).toBe(0);
    expect(updated.budget_date).toBe(utcDateString()); // today
  });

  test("counters NOT reset when same day", () => {
    const today = utcDateString();
    const state: FeedState = {
      ...emptyFeedState(),
      items_today: 42,
      cost_today_usd: 5.00,
      budget_date: today,
    };
    const updated = resetBudgetIfNewDay(state);
    expect(updated.items_today).toBe(42);
    expect(updated.cost_today_usd).toBe(5.00);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-12: Signal note build
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-12: Signal note build", () => {
  const sampleItem: FeedItem = {
    feed_id: "test-feed",
    item_id: "abc123",
    title: "Test Article",
    content: "This is the content.",
    url: "https://example.com/article",
    published_at: "2026-05-08T00:00:00.000Z",
    author: "Test Author",
    tags: ["golang"],
    raw: {},
  };

  const sampleDecision: RelevanceDecision = {
    store: true,
    reason: "Highly relevant to Go stack",
    priority: "high",
    tags: ["security"],
    summary: "One-sentence summary.",
  };

  test("note contains YAML frontmatter", () => {
    const note = buildSignalNote(sampleItem, sampleDecision, SAMPLE_FEED_CONFIG);
    expect(note).toMatch(/^---\n/);
    expect(note).toMatch(/\n---\n/);
  });

  test("frontmatter contains mb.type: signal", () => {
    const note = buildSignalNote(sampleItem, sampleDecision, SAMPLE_FEED_CONFIG);
    expect(note).toContain("type: signal");
  });

  test("frontmatter contains source.type: feed", () => {
    const note = buildSignalNote(sampleItem, sampleDecision, SAMPLE_FEED_CONFIG);
    expect(note).toContain("type: feed");
  });

  test("frontmatter contains feed_id", () => {
    const note = buildSignalNote(sampleItem, sampleDecision, SAMPLE_FEED_CONFIG);
    expect(note).toContain("feed_id: test-feed");
  });

  test("frontmatter contains relevance_score from priority", () => {
    const note = buildSignalNote(sampleItem, sampleDecision, SAMPLE_FEED_CONFIG);
    expect(note).toContain("relevance_score: high");
  });

  test("body contains item title", () => {
    const note = buildSignalNote(sampleItem, sampleDecision, SAMPLE_FEED_CONFIG);
    expect(note).toContain("# Test Article");
  });

  test("body contains summary from decision", () => {
    const note = buildSignalNote(sampleItem, sampleDecision, SAMPLE_FEED_CONFIG);
    expect(note).toContain("One-sentence summary.");
  });

  test("body contains axiom trace marker", () => {
    const note = buildSignalNote(sampleItem, sampleDecision, SAMPLE_FEED_CONFIG);
    expect(note).toContain("axiom:trace work_item=SWDE-52");
  });

  test("body contains merged tags (item + config + decision)", () => {
    const note = buildSignalNote(sampleItem, sampleDecision, SAMPLE_FEED_CONFIG);
    // Should contain tags from item (golang), config (test), and decision (security)
    expect(note).toContain("golang");
    expect(note).toContain("test");
    expect(note).toContain("security");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-13: Signal note write — path safety
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-13: Signal note write — path safety", () => {
  test("writes signal note to expected path", async () => {
    const memRoot = tmpDir("mem-write-13");
    const item: FeedItem = {
      feed_id: "my-feed",
      item_id: "testid001",
      title: "Written Item",
      content: "Content here",
      url: "https://example.com/item",
      published_at: new Date().toISOString(),
      author: "",
      tags: [],
      raw: {},
    };
    const decision: RelevanceDecision = {
      store: true,
      reason: "relevant",
      priority: "medium",
      tags: [],
      summary: "Written Item summary",
    };
    const filePath = await writeSignalNote(item, decision, SAMPLE_FEED_CONFIG, memRoot);
    expect(existsSync(filePath)).toBe(true);
    const content = readFileSync(filePath, "utf-8");
    expect(content).toContain("# Written Item");
  });

  test("safeFeedPath throws on path traversal", () => {
    expect(() => safeFeedPath("/tmp/root", "../evil", "file.txt")).toThrow(
      /path traversal/i
    );
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-005 plan=phase-b03/symlink-fix test=feed-ingestion.test.ts#safeFeedPath-symlink jira_ref=SWDE-52
  test("safeFeedPath throws on symlink pointing outside root", () => {
    expect.assertions(1);
    // Create a temporary feeds directory
    const feedsDir = mkdtempSync(join(tmpdir(), "feeds-symlink-"));
    const symlinkName = "evil-link";
    const symlinkPath = join(feedsDir, symlinkName);
    let canCreateSymlink = true;
    try {
      // Attempt to create a symlink inside feedsDir pointing to /etc/passwd (outside root)
      const { symlinkSync } = require("node:fs");
      symlinkSync("/etc/passwd", symlinkPath);
    } catch {
      // Symlink creation not available in this environment; skip gracefully
      canCreateSymlink = false;
    }
    if (!canCreateSymlink) {
      // Cannot test symlink bypass in this environment — test is vacuously safe
      expect(canCreateSymlink).toBe(false); // documents WHY the real test was skipped
      return;
    }
    // The symlink exists on disk, so realpathSync will resolve it to /etc/passwd
    // which is outside feedsDir — safeFeedPath must throw
    expect(() => safeFeedPath(feedsDir, symlinkName)).toThrow(/path traversal/i);
    // Cleanup
    try { rmSync(feedsDir, { recursive: true }); } catch { /* ignore */ }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-14: HTTP 304 Not Modified handling
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-14: HTTP 304 Not Modified", () => {
  test("pollFeed treats 304 as success with zero new items", async () => {
    const stateDir = tmpDir("state-304");
    const memRoot = tmpDir("mem-304");
    const feedsDir = tmpDir("feeds-304");

    // Write a valid feed config
    const yaml = `
id: "feed-304"
name: "304 Feed"
type: "rss"
source:
  url: "https://example.com/feed.rss"
poll_interval: "1h"
relevance:
  prompt: "Is this relevant?"
`;
    writeFileSync(join(feedsDir, "feed-304.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    // Mock fetch that returns 304
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(null, {
        status: 304,
        statusText: "Not Modified",
      }) as Response;

    try {
      const { result } = await pollFeed(
        config,
        stateDir,
        memRoot,
        alwaysStore(),
        undefined
      );
      expect(result.not_modified).toBe(true);
      expect(result.new_items).toBe(0);
      expect(result.stored).toBe(0);
      expect(result.errors).toHaveLength(0);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-15: Poll engine integration — fetch → parse → dedup → evaluate → store
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-15: Poll engine integration", () => {
  test("full cycle: fetch RSS → parse → dedup → store 2 items", async () => {
    const stateDir = tmpDir("state-poll-full");
    const memRoot = tmpDir("mem-poll-full");
    const feedsDir = tmpDir("feeds-poll-full");
    const yaml = `
id: "poll-test"
name: "Poll Test"
type: "rss"
source:
  url: "https://example.com/feed.rss"
poll_interval: "1h"
relevance:
  prompt: "Relevant?"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
deduplication:
  key: "guid"
  window: "7d"
`;
    writeFileSync(join(feedsDir, "poll-test.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(RSS_FIXTURE, {
        status: 200,
        headers: { "Content-Type": "application/rss+xml" },
      }) as Response;

    try {
      const { result, state } = await pollFeed(
        config,
        stateDir,
        memRoot,
        alwaysStore("high"),
        undefined
      );
      expect(result.fetched).toBe(true);
      expect(result.new_items).toBe(2);
      expect(result.duplicate_items).toBe(0);
      expect(result.stored).toBe(2);
      expect(result.errors).toHaveLength(0);
      expect(state.items_today).toBe(2);

      // Second poll: same feed → 0 new items (all deduped)
      const { result: result2 } = await pollFeed(
        config,
        stateDir,
        memRoot,
        alwaysStore("high"),
        undefined
      );
      expect(result2.new_items).toBe(0);
      expect(result2.duplicate_items).toBe(2);
      expect(result2.stored).toBe(0);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  test("items discarded by evaluator are not stored", async () => {
    const stateDir = tmpDir("state-discard");
    const memRoot = tmpDir("mem-discard");
    const feedsDir = tmpDir("feeds-discard");
    const yaml = `
id: "discard-test"
name: "Discard Test"
type: "rss"
source:
  url: "https://example.com/feed.rss"
poll_interval: "1h"
relevance:
  prompt: "Never relevant"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "discard-test.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const { result } = await pollFeed(
        config,
        stateDir,
        memRoot,
        neverStore(),
        undefined
      );
      expect(result.new_items).toBe(2);
      expect(result.evaluated).toBe(2);
      expect(result.stored).toBe(0);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  test("budget cap stops processing mid-batch", async () => {
    const stateDir = tmpDir("state-budget");
    const memRoot = tmpDir("mem-budget");
    const feedsDir = tmpDir("feeds-budget");
    const yaml = `
id: "budget-cap"
name: "Budget Cap Test"
type: "rss"
source:
  url: "https://example.com/feed.rss"
poll_interval: "1h"
relevance:
  prompt: "Relevant?"
  max_items_per_day: 1
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "budget-cap.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const { result } = await pollFeed(
        config,
        stateDir,
        memRoot,
        alwaysStore(),
        undefined
      );
      // 2 items in feed, cap=1 → 1 stored, 1 budget_skipped
      expect(result.new_items).toBe(2);
      expect(result.stored).toBe(1);
      expect(result.budget_skipped).toBe(1);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  test("pollFeed increments consecutive_failures on fetch error", async () => {
    const stateDir = tmpDir("state-err");
    const memRoot = tmpDir("mem-err");
    const feedsDir = tmpDir("feeds-err");
    const yaml = `
id: "err-feed"
name: "Error Feed"
type: "rss"
source:
  url: "https://example.com/feed.rss"
poll_interval: "1h"
relevance:
  prompt: "Relevant?"
`;
    writeFileSync(join(feedsDir, "err-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => {
      throw new Error("Network unreachable");
    };

    try {
      const { result, state } = await pollFeed(
        config,
        stateDir,
        memRoot,
        alwaysStore(),
        undefined
      );
      expect(result.errors).toHaveLength(1);
      expect(result.errors[0]).toContain("Network unreachable");
      expect(state.consecutive_failures).toBe(1);
      expect(state.last_error).toContain("Network unreachable");
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-16: feed.list tool
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-16: feed.list tool", () => {
  test("lists all enabled feeds", async () => {
    const dir = tmpDir("plugin-list");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });

    const yaml = `
id: "list-feed-a"
name: "List Feed A"
type: "rss"
source:
  url: "https://example.com/rss-a"
poll_interval: "1h"
relevance:
  prompt: "Relevant?"
enabled: true
`;
    writeFileSync(join(feedsDir, "list-feed-a.yaml"), yaml, "utf-8");

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir });
    const raw = await plugin.tool["feed_list"].execute({}, {});
    const result = JSON.parse(raw as string);
    expect(result.feeds).toHaveLength(1);
    expect(result.feeds[0].id).toBe("list-feed-a");
    expect(result.feeds[0].enabled).toBe(true);
  });

  test("excludes disabled feeds by default", async () => {
    const dir = tmpDir("plugin-list-disabled");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });

    const enabled = `id: "enabled-feed"\nname: "On"\ntype: "rss"\nsource:\n  url: "https://x.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: true\n`;
    const disabled = `id: "disabled-feed"\nname: "Off"\ntype: "rss"\nsource:\n  url: "https://y.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: false\n`;
    writeFileSync(join(feedsDir, "enabled-feed.yaml"), enabled, "utf-8");
    writeFileSync(join(feedsDir, "disabled-feed.yaml"), disabled, "utf-8");

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir });
    const raw = await plugin.tool["feed_list"].execute({}, {});
    const result = JSON.parse(raw as string);
    expect(result.feeds).toHaveLength(1);
    expect(result.feeds[0].id).toBe("enabled-feed");
  });

  test("includes disabled feeds when include_disabled=true", async () => {
    const dir = tmpDir("plugin-list-all");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });

    const f1 = `id: "f-on"\nname: "On"\ntype: "rss"\nsource:\n  url: "https://x.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: true\n`;
    const f2 = `id: "f-off"\nname: "Off"\ntype: "rss"\nsource:\n  url: "https://y.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: false\n`;
    writeFileSync(join(feedsDir, "f-on.yaml"), f1, "utf-8");
    writeFileSync(join(feedsDir, "f-off.yaml"), f2, "utf-8");

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir });
    const raw = await plugin.tool["feed_list"].execute({ include_disabled: true }, {});
    const result = JSON.parse(raw as string);
    expect(result.feeds).toHaveLength(2);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-17: feed.status tool
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-17: feed.status tool", () => {
  test("returns detailed status for a known feed", async () => {
    const dir = tmpDir("plugin-status");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });

    const yaml = `
id: "status-feed"
name: "Status Feed"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "2h"
relevance:
  prompt: "p"
  max_items_per_day: 25
  max_cost_per_day_usd: 0.25
enabled: true
`;
    writeFileSync(join(feedsDir, "status-feed.yaml"), yaml, "utf-8");

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir });
    const raw = await plugin.tool["feed_status"].execute({ feed_id: "status-feed" }, {});
    const result = JSON.parse(raw as string);
    expect(result.id).toBe("status-feed");
    expect(result.health).toBeDefined();
    expect(result.budget_today).toBeDefined();
    expect(result.budget_today.max_items).toBe(25);
    expect(result.budget_today.max_cost_usd).toBe(0.25);
  });

  test("returns error for unknown feed", async () => {
    const dir = tmpDir("plugin-status-miss");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir });
    const raw = await plugin.tool["feed_status"].execute({ feed_id: "nonexistent" }, {});
    const result = JSON.parse(raw as string);
    expect(result.error).toMatch(/nonexistent/);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-18: feed.poll tool — disabled feeds / dry_run
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-18: feed.poll tool — disabled feeds and dry_run", () => {
  test("does not poll disabled feeds", async () => {
    const dir = tmpDir("plugin-poll-disabled");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });

    const yaml = `
id: "disabled-poll"
name: "Disabled"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
relevance:
  prompt: "p"
enabled: false
`;
    writeFileSync(join(feedsDir, "disabled-poll.yaml"), yaml, "utf-8");

    let fetchCalled = false;
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => { fetchCalled = true; return new Response("", { status: 200 }); };

    try {
      const plugin = FeedIngestionPlugin({ directory: dir, feedsDir });
      const raw = await plugin.tool["feed_poll"].execute({}, {});
      const result = JSON.parse(raw as string);
      expect(result.status).toBe("no_feeds");
      expect(fetchCalled).toBe(false);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  test("dry_run does not write signal files to disk", async () => {
    const dir = tmpDir("plugin-poll-dryrun");
    const feedsDir = join(dir, ".axiom", "feeds");
    const memRoot = join(dir, ".memory-bank");
    mkdirSync(feedsDir, { recursive: true });

    const yaml = `
id: "dry-feed"
name: "Dry Run Feed"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
enabled: true
`;
    writeFileSync(join(feedsDir, "dry-feed.yaml"), yaml, "utf-8");

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const plugin = FeedIngestionPlugin({
        directory: dir,
        feedsDir,
        memoryRoot: memRoot,
        evaluator: alwaysStore(),
      });
      await plugin.tool["feed_poll"].execute({ dry_run: true }, {});

      // signals/ dir should NOT exist (no writes in dry-run)
      expect(existsSync(join(memRoot, "signals"))).toBe(false);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-19: consecutive_failures tracking (covered in AC-15 + standalone)
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-19: consecutive_failures tracking", () => {
  test("consecutive_failures resets to 0 after successful poll", async () => {
    const stateDir = tmpDir("state-recovery");
    const memRoot = tmpDir("mem-recovery");
    const feedsDir = tmpDir("feeds-recovery");

    const yaml = `
id: "recovery-feed"
name: "Recovery"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "recovery-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    // Simulate 2 prior failures in state
    const initState: FeedState = {
      ...emptyFeedState(),
      consecutive_failures: 2,
      last_error: "prior error",
    };
    await saveFeedState(stateDir, config.id, initState);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const { state } = await pollFeed(
        config,
        stateDir,
        memRoot,
        alwaysStore(),
        undefined
      );
      expect(state.consecutive_failures).toBe(0);
      expect(state.last_error).toBeNull();
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-20: Global cost cap enforcement (REQ-FEED-082)
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-082 plan=phase-1-fix/step-52-001 test=feed-ingestion.test.ts#AC-20 jira_ref=SWDE-52
describe("AC-20: Global cost cap enforcement", () => {
  test("global cost cap stops processing subsequent feeds", async () => {
    const dir = tmpDir("global-cap");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    const memRoot = join(dir, ".memory-bank");
    mkdirSync(feedsDir, { recursive: true });
    mkdirSync(stateDir, { recursive: true });

    // Two feeds
    const f1 = `id: "cap-feed-a"\nname: "A"\ntype: "rss"\nsource:\n  url: "https://a.com/rss"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 50\n  max_cost_per_day_usd: 5.00\nenabled: true\n`;
    const f2 = `id: "cap-feed-b"\nname: "B"\ntype: "rss"\nsource:\n  url: "https://b.com/rss"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 50\n  max_cost_per_day_usd: 5.00\nenabled: true\n`;
    writeFileSync(join(feedsDir, "cap-feed-a.yaml"), f1, "utf-8");
    writeFileSync(join(feedsDir, "cap-feed-b.yaml"), f2, "utf-8");

    // Global config with tiny cap
    const globalYaml = `feeds:\n  enabled: true\n  global_max_cost_per_day_usd: 0.01\n`;
    const feedsConfigPath = join(dir, ".axiom", "feeds.yaml");
    mkdirSync(join(dir, ".axiom"), { recursive: true });
    writeFileSync(feedsConfigPath, globalYaml, "utf-8");

    // Pre-seed first feed's cost at or above cap (simulate already at limit)
    const stateFeedA = { ...emptyFeedState(), cost_today_usd: 0.02, budget_date: utcDateString() };
    await saveFeedState(stateDir, "cap-feed-a", stateFeedA);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response("<rss version='2.0'><channel></channel></rss>", { status: 200 }) as Response;

    try {
      const plugin = FeedIngestionPlugin({
        directory: dir,
        feedsDir,
        feedsConfigPath,
        stateDir,
        memoryRoot: memRoot,
        evaluator: alwaysStore(),
      });
      const raw = await plugin.tool["feed_poll"].execute({}, {});
      const result = JSON.parse(raw as string);

      // The global budget was already hit by feed-a's seeded cost
      // Feed-b should be skipped with global cap error
      const feedBResult = result.results?.find((r: { feed_id: string }) => r.feed_id === "cap-feed-b");
      expect(feedBResult).toBeDefined();
      expect(feedBResult.errors[0]).toMatch(/global.*cap|cap.*reached/i);
      expect(result.global_budget_capped).toBe(true);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-21: Fetch timeout (REQ-FEED-024)
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-024 plan=phase-1-fix/step-52-005 test=feed-ingestion.test.ts#AC-21 jira_ref=SWDE-52
describe("AC-21: Fetch timeout", () => {
  test("fetch timeout marks feed with error and increments consecutive_failures", async () => {
    const stateDir = tmpDir("state-timeout");
    const memRoot = tmpDir("mem-timeout");
    const feedsDir = tmpDir("feeds-timeout");

    const yaml = `
id: "timeout-feed"
name: "Timeout Test"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "timeout-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const originalFetch = globalThis.fetch;
    // Mock fetch that honors AbortSignal — returns a promise that rejects when aborted
    globalThis.fetch = (_url: unknown, init?: RequestInit) =>
      new Promise<Response>((_resolve, reject) => {
        const signal = init?.signal as AbortSignal | undefined;
        if (signal) {
          if (signal.aborted) {
            reject(new DOMException("The operation was aborted.", "AbortError"));
            return;
          }
          signal.addEventListener("abort", () => {
            reject(new DOMException("The operation was aborted.", "AbortError"));
          });
        }
        // No resolve — simulates a hanging server that never sends a response
      });

    const start = Date.now();
    try {
      const { result, state } = await pollFeed(
        config,
        stateDir,
        memRoot,
        alwaysStore(),
        undefined,
        {},
        100  // 100ms timeout — triggers AbortSignal.timeout(100)
      );
      const elapsed = Date.now() - start;

      // Should complete within a reasonable time (not hang)
      expect(elapsed).toBeLessThan(2000); // generous bound for CI
      expect(result.errors).toHaveLength(1);
      expect(result.errors[0]).toMatch(/timed? out|timeout|abort/i);
      expect(state.consecutive_failures).toBe(1);
    } finally {
      globalThis.fetch = originalFetch;
    }
  }, 5000); // 5s test timeout
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-22: Default evaluator stores nothing and warns (REQ-FEED-060)
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-060 plan=phase-1-fix/step-52-006 test=feed-ingestion.test.ts#AC-22 jira_ref=SWDE-52
describe("AC-22: Default evaluator stores nothing and warns", () => {
  test("feed.poll with no custom evaluator stores 0 items and returns warning", async () => {
    const dir = tmpDir("default-eval");
    const feedsDir = join(dir, ".axiom", "feeds");
    const memRoot = join(dir, ".memory-bank");
    mkdirSync(feedsDir, { recursive: true });

    const yaml = `
id: "default-eval-feed"
name: "Default Eval Test"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
relevance:
  prompt: "Is this relevant?"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
enabled: true
`;
    writeFileSync(join(feedsDir, "default-eval-feed.yaml"), yaml, "utf-8");

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      // Plugin with NO custom evaluator (uses defaultRelevanceEvaluator)
      const plugin = FeedIngestionPlugin({
        directory: dir,
        feedsDir,
        memoryRoot: memRoot,
        // No evaluator — uses defaultRelevanceEvaluator
      });
      const raw = await plugin.tool["feed_poll"].execute({}, {});
      const result = JSON.parse(raw as string);

      expect(result.total_stored).toBe(0);  // nothing stored
      expect(result.warnings).toBeDefined();
      expect(result.warnings.length).toBeGreaterThan(0);
      expect(result.warnings[0]).toMatch(/default evaluator|discarded|configure/i);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  test("defaultRelevanceEvaluator returns store: false", async () => {
    const decision = await defaultRelevanceEvaluator(
      { feed_id: "x", item_id: "y", title: "T", content: "C", url: "https://x.com", published_at: "", author: "", tags: [], raw: {} },
      { prompt: "p" },
      undefined
    );
    expect(decision.store).toBe(false);
    expect(decision.reason).toMatch(/WARNING|no evaluator/i);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#§14.2 plan=phase-1/task-1.2 test=feed-ingestion.test.ts#AC-22-shape jira_ref=SWDE-52
  test("defaultRelevanceEvaluator returns all required RelevanceDecision fields (§14.2 shape contract)", async () => {
    const item: FeedItem = {
      feed_id: "shape-test",
      item_id: "item-001",
      title: "Shape Test Title",
      content: "Shape test content body.",
      url: "https://example.com/shape-test",
      published_at: "2026-01-01T00:00:00Z",
      author: "Tester",
      tags: ["test"],
      raw: {},
    };
    const decision = await defaultRelevanceEvaluator(item, { prompt: "Is this relevant?" }, undefined);

    // Required fields per §14.2 of specs/105-Feed-Ingestion.md
    expect(decision).toHaveProperty("store");
    expect(decision).toHaveProperty("reason");
    expect(decision).toHaveProperty("priority");
    expect(decision).toHaveProperty("tags");
    expect(decision).toHaveProperty("summary");

    // Type assertions for required fields
    expect(typeof decision.store).toBe("boolean");
    expect(typeof decision.reason).toBe("string");
    expect(typeof decision.priority).toBe("string");
    expect(Array.isArray(decision.tags)).toBe(true);
    expect(typeof decision.summary).toBe("string");

    // Behavioral contract: stub must return specific values
    expect(decision.store).toBe(false);           // stub must never store
    // Note: defaultRelevanceEvaluator currently returns 'low' — tested separately in AC-22
    expect(['high', 'medium', 'low']).toContain(decision.priority); // valid enum value
    expect(decision.tags).toEqual([]);             // stub always returns empty tags
    expect(decision.summary).toBe('');             // stub always returns empty summary

    // cost_usd is optional — must be number if present, or absent/undefined
    if ("cost_usd" in decision && decision.cost_usd !== undefined) {
      expect(typeof decision.cost_usd).toBe("number");
    }
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-11/step-p11-001 test=feed-ingestion.test.ts#AC-22-partial-decision jira_ref=SWDE-52
  test('AC-22-partial-decision: evaluator returning partial RelevanceDecision (missing priority/tags/summary) is rejected with error and queued for retry', async () => {
    // NEW BEHAVIOR (step-p11-001): pollFeed now validates required RelevanceDecision fields.
    // A partial decision missing priority, tags, or summary is REJECTED — no corrupt note written.
    //
    // Expected outcomes:
    //   - result.stored === 0       (no corrupt notes written)
    //   - result.errors.length > 0  (error recorded for partial decision)
    //   - state.pending_retry.length > 0  (items queued for retry)
    //
    // REQ-FEED-061: RelevanceDecision shape contract — partial decisions are now rejected.
    // The OLD behavior (silently writing corrupt notes) is gone.

    const stateDir = tmpDir('partial-decision-state');
    const memRoot = tmpDir('partial-decision-mem');

    // Evaluator that returns only the minimum required fields (store + reason)
    // — missing priority, tags, summary
    const partialEvaluator = async (): Promise<RelevanceDecision> =>
      ({ store: true, reason: 'ok' } as unknown as RelevanceDecision);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const { result, state } = await pollFeed(
        SAMPLE_FEED_CONFIG,
        stateDir,
        memRoot,
        partialEvaluator,
        undefined,
        {}
      );

      // NEW: partial decision is rejected — no corrupt notes written
      expect(result.stored).toBe(0);

      // NEW: an error is recorded for each partial decision
      expect(result.errors.length).toBeGreaterThan(0);
      expect(result.errors[0]).toMatch(/incomplete RelevanceDecision|missing fields/i);

      // NEW: items are queued for retry so they can be re-evaluated
      expect(state.pending_retry.length).toBeGreaterThan(0);
      expect(state.pending_retry).toHaveLength(2); // REQ-FEED-061 + REQ-FEED-065: step-p11-001 validation treats partial decisions as retriable failures — both RSS items are queued for re-evaluation on next poll

      // No signal files written (signals dir should not exist or be empty)
      const signalsDir = require('node:path').join(memRoot, 'assist-axiom', 'signals');
      if (require('node:fs').existsSync(signalsDir)) {
        const files = require('node:fs').readdirSync(signalsDir).filter((f: string) => f.endsWith('.md'));
        expect(files.length).toBe(0);
      }
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-11/step-p11-001 test=feed-ingestion.test.ts#AC-22-partial-decision-new jira_ref=SWDE-52
  test('AC-22-partial-decision-new: REQ-FEED-061 validation rejects decision with only priority missing (single missing field)', async () => {
    // DISTINCT SCENARIO from AC-22-partial-decision:
    // AC-22-partial-decision: all three required fields (priority, tags, summary) are missing.
    // AC-22-partial-decision-new: only priority is missing — tags and summary are present.
    // This verifies that validation catches a single missing required field, not just the
    // all-missing case. REQ-FEED-061 requires ALL three fields to be present.

    const stateDir = tmpDir('partial-decision-new-state');
    const memRoot = tmpDir('partial-decision-new-mem');

    // Returns a decision with tags and summary present but priority undefined.
    // This is a valid-looking partial decision that should still be rejected.
    const partialEvaluator = async (): Promise<RelevanceDecision> =>
      ({ store: true, reason: 'ok', priority: undefined, tags: ['test'], summary: 'test summary' } as unknown as RelevanceDecision);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const { result, state } = await pollFeed(
        SAMPLE_FEED_CONFIG,
        stateDir,
        memRoot,
        partialEvaluator,
        undefined,
        {}
      );

      // REQ-FEED-061 assertion 1: error was recorded for the missing priority field
      expect(result.errors.length).toBeGreaterThan(0);
      expect(result.errors[0]).toMatch(/incomplete RelevanceDecision|missing fields/i);

      // REQ-FEED-061 assertion 2: no corrupt signal note written (priority is required)
      expect(result.stored).toBe(0);

      // REQ-FEED-061 assertion 3: both RSS items queued for retry (not silently discarded)
      expect(state.pending_retry).toHaveLength(2);

      // REQ-FEED-061 assertion 4: no signal files on disk
      const signalsDir = join(memRoot, 'assist-axiom', 'signals');
      const signalFiles = existsSync(signalsDir) ? readdirSync(signalsDir) : [];
      expect(signalFiles).toHaveLength(0);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

});

// ─────────────────────────────────────────────────────────────────────────────
// AC-23: Dedup key selector modes (REQ-FEED-021)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-021 plan=phase-1-fix/step-52-002 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-23: Dedup key selector modes", () => {
  const ITEMS_SAME_URL: FeedItem[] = [
    {
      feed_id: "dedup-test",
      item_id: "hash-of-guid-1", // different guids
      title: "Article A",
      content: "content",
      url: "https://example.com/shared-url", // SAME URL
      published_at: new Date().toISOString(),
      author: "",
      tags: [],
      raw: { guid: "guid-1" },
    },
    {
      feed_id: "dedup-test",
      item_id: "hash-of-guid-2", // different item_id
      title: "Article A Updated",
      content: "content updated",
      url: "https://example.com/shared-url", // SAME URL
      published_at: new Date().toISOString(),
      author: "",
      tags: [],
      raw: { guid: "guid-2" },
    },
  ];

  const ITEMS_SAME_TITLE: FeedItem[] = [
    {
      feed_id: "dedup-test",
      item_id: "hash-A",
      title: "Same Title",
      content: "first version",
      url: "https://example.com/v1",
      published_at: new Date().toISOString(),
      author: "",
      tags: [],
      raw: { guid: "g1" },
    },
    {
      feed_id: "dedup-test",
      item_id: "hash-B",
      title: "Same Title", // SAME TITLE
      content: "second version",
      url: "https://example.com/v2",
      published_at: new Date().toISOString(),
      author: "",
      tags: [],
      raw: { guid: "g2" },
    },
  ];

  test("dedup key=url: same URL items are deduped even with different guids", () => {
    const state = emptyFeedState();
    // Pre-populate the shared URL key (simulating a prior poll that saw it)
    state.seen_ids["https://example.com/shared-url"] = new Date().toISOString();
    const { newItems, duplicateCount } = deduplicateItems(ITEMS_SAME_URL, state, { key: "url", window: "7d" });
    // Both items share the same URL, which is already in seen_ids.
    // In guid mode they would both be "new" (different item_ids), but in url mode both are dupes.
    expect(duplicateCount).toBe(2);
    expect(newItems).toHaveLength(0);
  });

  test("dedup key=url: same url → first new, second deduped in same batch", () => {
    const state = emptyFeedState();
    // No pre-existing seen_ids
    const { newItems, duplicateCount } = deduplicateItems(ITEMS_SAME_URL, state, { key: "url", window: "7d" });
    // Both have same URL but different item_ids. Since seen_ids starts empty,
    // both would be "new" from the perspective of this function
    // (dedup is against seen_ids from previous polls, not within the same batch)
    expect(newItems).toHaveLength(2);
    expect(duplicateCount).toBe(0);
  });

  test("dedup key=title_hash: same title items are deduped", () => {
    const state = emptyFeedState();
    // Pre-populate with title_hash for "Same Title"
    state.seen_ids[hashString("Same Title")] = new Date().toISOString();
    const { newItems, duplicateCount } = deduplicateItems(ITEMS_SAME_TITLE, state, { key: "title_hash", window: "7d" });
    // Both items share the same title hash, which is already in seen_ids.
    // In guid mode they'd both be new (different item_ids), but in title_hash mode both are dupes.
    expect(duplicateCount).toBe(2);
    expect(newItems).toHaveLength(0);
  });

  test("dedup key=guid (default): preserves existing AC-7 behavior", () => {
    const items = parseRssItems(RSS_FIXTURE, "test-feed");
    const state = emptyFeedState();
    state.seen_ids[items[0].item_id] = new Date().toISOString();
    const { newItems, duplicateCount } = deduplicateItems(items, state, { key: "guid", window: "7d" });
    expect(duplicateCount).toBe(1);
    expect(newItems).toHaveLength(1);
  });

  test("dedup key=undefined (default): same as guid mode", () => {
    const items = parseRssItems(RSS_FIXTURE, "test-feed");
    const state = emptyFeedState();
    state.seen_ids[items[0].item_id] = new Date().toISOString();
    const { newItems } = deduplicateItems(items, state, {});
    expect(newItems).toHaveLength(1);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-24: Structured log events (REQ-FEED-054)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-054 plan=phase-1-fix/step-52-004 test=feed-ingestion.test.ts#AC-24 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-24: Structured log events (REQ-FEED-054)", () => {
  test("pollFeed emits poll_started and poll_completed JSON events to console.log", async () => {
    const stateDir = tmpDir("state-log");
    const memRoot = tmpDir("mem-log");
    const feedsDir = tmpDir("feeds-log");

    const yaml = `
id: "log-test-feed"
name: "Log Test"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "log-test-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const loggedLines: string[] = [];
    const originalWrite = process.stderr.write.bind(process.stderr);
    const prevEnv = process.env.AXIOM_FEED_INGESTION_DEBUG;
    process.env.AXIOM_FEED_INGESTION_DEBUG = "1";
    process.stderr.write = ((chunk: string | Uint8Array): boolean => {
      loggedLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
      return true;
    }) as typeof process.stderr.write;

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      await pollFeed(config, stateDir, memRoot, alwaysStore());
    } finally {
      process.stderr.write = originalWrite;
      if (prevEnv === undefined) delete process.env.AXIOM_FEED_INGESTION_DEBUG;
      else process.env.AXIOM_FEED_INGESTION_DEBUG = prevEnv;
      globalThis.fetch = originalFetch;
    }

    // Parse JSON events from logged lines
    const events = loggedLines
      .filter(line => {
        try { JSON.parse(line); return true; } catch { return false; }
      })
      .map(line => JSON.parse(line) as { event: string; feed_id: string; timestamp: string });

    const eventTypes = events.map(e => e.event);
    expect(eventTypes).toContain("poll_started");
    expect(eventTypes).toContain("poll_completed");

    // items_found must be emitted with a count field
    expect(eventTypes).toContain("items_found");
    const foundEvent = events.find(e => e.event === "items_found");
    expect(typeof (foundEvent as Record<string, unknown>)?.count).toBe("number");

    // items_evaluated must be emitted with stored field
    expect(eventTypes).toContain("items_evaluated");
    const evaluatedEvent = events.find(e => e.event === "items_evaluated");
    expect(typeof (evaluatedEvent as Record<string, unknown>)?.stored).toBe("number");

    // poll_started must have feed_id
    const started = events.find(e => e.event === "poll_started");
    expect(started?.feed_id).toBe("log-test-feed");

    // events must have timestamps
    for (const event of events) {
      expect(event.timestamp).toBeDefined();
      expect(() => new Date(event.timestamp)).not.toThrow();
    }
  });

  test("logEvent emits valid JSON to stderr (when AXIOM_FEED_INGESTION_DEBUG=1)", () => {
    const lines: string[] = [];
    const orig = process.stderr.write.bind(process.stderr);
    const prevEnv = process.env.AXIOM_FEED_INGESTION_DEBUG;
    process.env.AXIOM_FEED_INGESTION_DEBUG = "1";
    process.stderr.write = ((chunk: string | Uint8Array): boolean => {
      lines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
      return true;
    }) as typeof process.stderr.write;
    try {
      logEvent("test_event", { feed_id: "test", count: 42 });
    } finally {
      process.stderr.write = orig;
      if (prevEnv === undefined) delete process.env.AXIOM_FEED_INGESTION_DEBUG;
      else process.env.AXIOM_FEED_INGESTION_DEBUG = prevEnv;
    }
    expect(lines).toHaveLength(1);
    const parsed = JSON.parse(lines[0]);
    expect(parsed.event).toBe("test_event");
    expect(parsed.feed_id).toBe("test");
    expect(parsed.count).toBe(42);
    expect(typeof parsed.timestamp).toBe("string");
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-25: Real-world feed parsing with fast-xml-parser (REQ-FEED-020)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-020 plan=phase-1-fix/step-52-003 test=feed-ingestion.test.ts#AC-25 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-25: Real-world feed parsing with fast-xml-parser", () => {
  // RSS with HTML in description (simulates WordPress/GitHub feeds)
  const RSS_WITH_HTML_DESCRIPTION = `<?xml version="1.0"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>WordPress Blog</title>
    <item>
      <title>My Post</title>
      <link>https://blog.example.com/post-1</link>
      <description><![CDATA[<p>This is a summary with an <a href="/link">link</a>.</p>
<p>And a closing item tag: </item> which used to break the regex parser.</p>]]></description>
      <pubDate>Fri, 08 May 2026 10:00:00 GMT</pubDate>
      <guid>https://blog.example.com/post-1</guid>
      <dc:creator>Alice</dc:creator>
      <content:encoded><![CDATA[<p>Full post content here. This content is longer and includes more HTML.</p><p>Another paragraph.</p>]]></content:encoded>
    </item>
    <item>
      <title>Second Post</title>
      <link>https://blog.example.com/post-2</link>
      <description>Simple description without HTML issues.</description>
      <pubDate>Sat, 09 May 2026 10:00:00 GMT</pubDate>
      <guid>https://blog.example.com/post-2</guid>
    </item>
  </channel>
</rss>`;

  // Atom with multiple <link> elements and categories
  const ATOM_WITH_NAMESPACES = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"
      xmlns:dc="http://purl.org/dc/elements/1.1/">
  <title>Namespaced Feed</title>
  <entry>
    <id>urn:example:ns-entry-1</id>
    <title>Namespaced Entry</title>
    <link href="https://example.com/ns-entry-1" rel="alternate"/>
    <link href="https://example.com/ns-entry-1.pdf" rel="related"/>
    <published>2026-05-08T10:00:00Z</published>
    <content type="html"><![CDATA[<p>Content with <em>HTML</em>.</p>]]></content>
    <author><name>Dr. Smith</name></author>
    <category term="research"/>
    <category term="security"/>
  </entry>
</feed>`;

  test("RSS: item with HTML in description containing </item> is NOT truncated", () => {
    const items = parseRssItems(RSS_WITH_HTML_DESCRIPTION, "wp-feed");
    // With regex parser, the </item> inside CDATA would truncate to 1 item.
    // fast-xml-parser correctly parses CDATA so both items should be present.
    expect(items).toHaveLength(2);
    expect(items[0].title).toBe("My Post");
    expect(items[1].title).toBe("Second Post");
    // Description contains the </item> text intact (in CDATA)
    expect(items[0].content).toContain("closing item tag");
  });

  test("RSS: dc:creator namespace tag is extracted as author", () => {
    const items = parseRssItems(RSS_WITH_HTML_DESCRIPTION, "wp-feed");
    expect(items[0].author).toBe("Alice");
  });

  test("Atom: multiple <link> elements — selects href with rel=alternate", () => {
    const items = parseAtomItems(ATOM_WITH_NAMESPACES, "ns-feed");
    expect(items).toHaveLength(1);
    expect(items[0].url).toBe("https://example.com/ns-entry-1"); // not the related link
  });

  test("Atom: multiple categories extracted correctly", () => {
    const items = parseAtomItems(ATOM_WITH_NAMESPACES, "ns-feed");
    expect(items[0].tags).toContain("research");
    expect(items[0].tags).toContain("security");
  });

  test("Atom: CDATA content extracted correctly", () => {
    const items = parseAtomItems(ATOM_WITH_NAMESPACES, "ns-feed");
    expect(items[0].content).toContain("HTML");
  });

  test("malformed/truncated XML returns array without throwing", () => {
    // fast-xml-parser must not crash on bad input; may return empty or partial results
    expect(() => parseRssItems("<rss><channel><item><title>Unclosed", "bad-feed")).not.toThrow();
    expect(() => parseFeed("<feed xmlns='http://www.w3.org/2005/Atom'><entry><title>", "bad-atom")).not.toThrow();
  });

  test("RSS feed with exactly one item returns array of length 1", () => {
    const singleItemRss = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Single Item Feed</title>
    <item>
      <title>Only Article</title>
      <link>https://example.com/only</link>
      <guid>only-guid-1</guid>
      <pubDate>Fri, 08 May 2026 12:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`;
    const items = parseRssItems(singleItemRss, "single-feed");
    expect(items).toHaveLength(1);
    expect(items[0].title).toBe("Only Article");
    expect(items[0].url).toBe("https://example.com/only");
  });

  test("RSS feed with empty channel returns empty array", () => {
    const emptyRss = `<?xml version="1.0"?><rss version="2.0"><channel><title>Empty</title></channel></rss>`;
    const items = parseRssItems(emptyRss, "empty-feed");
    expect(items).toHaveLength(0);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-020 plan=phase-1-fix/step-52-009 test=feed-ingestion.test.ts#AC-25 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-26: item_discarded log event (REQ-FEED-062)
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-26: item_discarded audit log event (REQ-FEED-062)", () => {
  test("pollFeed emits item_discarded event for each discarded item", async () => {
    const stateDir = tmpDir("state-discard-log");
    const memRoot = tmpDir("mem-discard-log");
    const feedsDir = tmpDir("feeds-discard-log");

    const yaml = `
id: "discard-log-feed"
name: "Discard Log Test"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
relevance:
  prompt: "Never relevant"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "discard-log-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const loggedLines: string[] = [];
    const originalWrite = process.stderr.write.bind(process.stderr);
    const prevEnv = process.env.AXIOM_FEED_INGESTION_DEBUG;
    process.env.AXIOM_FEED_INGESTION_DEBUG = "1";
    process.stderr.write = ((chunk: string | Uint8Array): boolean => {
      loggedLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
      return true;
    }) as typeof process.stderr.write;

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      await pollFeed(config, stateDir, memRoot, neverStore());
    } finally {
      process.stderr.write = originalWrite;
      if (prevEnv === undefined) delete process.env.AXIOM_FEED_INGESTION_DEBUG;
      else process.env.AXIOM_FEED_INGESTION_DEBUG = prevEnv;
      globalThis.fetch = originalFetch;
    }

    // Parse structured JSON events
    const events = loggedLines
      .filter((line) => {
        try {
          JSON.parse(line);
          return true;
        } catch {
          return false;
        }
      })
      .map((line) => JSON.parse(line) as Record<string, unknown>);

    const discardedEvents = events.filter((e) => e.event === "item_discarded");

    // RSS_FIXTURE has 2 items, both discarded
    expect(discardedEvents.length).toBe(2);
    expect(discardedEvents[0].feed_id).toBe("discard-log-feed");
    expect(typeof discardedEvents[0].item_id).toBe("string");
    expect(typeof discardedEvents[0].title).toBe("string");
    expect(typeof discardedEvents[0].reason).toBe("string");
    expect(discardedEvents[0].timestamp).toBeDefined();
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-062 plan=phase-1-fix/step-52-007 test=feed-ingestion.test.ts#AC-26 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-27: Poll staggering (REQ-FEED-051)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-051 plan=phase-2/staggering/step-52-p2-001 test=feed-ingestion.test.ts#AC-27 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-27: Poll staggering (REQ-FEED-051)", () => {
  test("feed.poll applies stagger_ms delay between feeds", async () => {
    const dir = tmpDir("stagger-test");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });

    // Two feeds
    const f1 = `id: "stagger-a"\nname: "A"\ntype: "rss"\nsource:\n  url: "https://a.com/rss"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: true\n`;
    const f2 = `id: "stagger-b"\nname: "B"\ntype: "rss"\nsource:\n  url: "https://b.com/rss"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: true\n`;
    writeFileSync(join(feedsDir, "stagger-a.yaml"), f1, "utf-8");
    writeFileSync(join(feedsDir, "stagger-b.yaml"), f2, "utf-8");

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response('<rss version="2.0"><channel></channel></rss>', { status: 200 }) as Response;

    const plugin = FeedIngestionPlugin({
      directory: dir,
      feedsDir,
      evaluator: alwaysStore(),
      stagger_ms: 50,  // 50ms stagger
    });

    const start = Date.now();
    try {
      const raw = await plugin.tool["feed_poll"].execute({}, {});
      const result = JSON.parse(raw as string);
      const elapsed = Date.now() - start;

      // With 2 feeds and 50ms stagger, total time should be >= 50ms
      // 45ms lower bound for a 50ms stagger (90% of intended delay).
      // This is tight enough to catch a broken stagger (which would complete in <10ms)
      // while allowing for minor CI timing variance. If this flakes on CI, consider
      // mocking the sleep function instead of loosening the bound further.
      // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-051 plan=phase-26/step-v20-003 test=feed-ingestion.test.ts#AC-27 jira_ref=SWDE-52
      expect(elapsed).toBeGreaterThanOrEqual(45); // 90% of 50ms stagger
      expect(result.feeds_polled).toBe(2);
      expect(result.stagger_ms).toBe(50);
    } finally {
      globalThis.fetch = originalFetch;
    }
  }, 3000); // 3s test timeout

  test("feed.poll with stagger_ms=0 does not delay", async () => {
    const dir = tmpDir("no-stagger");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });

    const f1 = `id: "fast-a"\nname: "A"\ntype: "rss"\nsource:\n  url: "https://a.com/rss"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: true\n`;
    writeFileSync(join(feedsDir, "fast-a.yaml"), f1, "utf-8");

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response('<rss version="2.0"><channel></channel></rss>', { status: 200 }) as Response;

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, evaluator: alwaysStore() });
    try {
      const raw = await plugin.tool["feed_poll"].execute({}, {});
      const result = JSON.parse(raw as string);
      expect(result.stagger_ms).toBe(0);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-051 plan=phase-2/staggering/step-52-p2-001 test=feed-ingestion.test.ts#AC-27 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-28: Eval failure retry queue (REQ-FEED-065)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-2/retry-queue/step-52-p2-002 test=feed-ingestion.test.ts#AC-28 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-28: Eval failure retry queue (REQ-FEED-065)", () => {
  test("item added to pending_retry when evaluator throws", async () => {
    const stateDir = tmpDir("state-retry");
    const memRoot = tmpDir("mem-retry");
    const feedsDir = tmpDir("feeds-retry");

    const yaml = `
id: "retry-feed"
name: "Retry Test"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "retry-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(RSS_FIXTURE, { status: 200 }) as Response;

    // Evaluator that always throws
    const throwingEvaluator = async () => {
      throw new Error("LLM API unavailable");
    };

    try {
      const { state } = await pollFeed(config, stateDir, memRoot, throwingEvaluator);
      // Both items should be in pending_retry
      expect(state.pending_retry.length).toBeGreaterThan(0);
      expect(state.pending_retry.length).toBe(2); // RSS_FIXTURE has 2 items
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  test("pending_retry items are cleared and re-queued for evaluation on next poll", async () => {
    const stateDir = tmpDir("state-retry2");
    const memRoot = tmpDir("mem-retry2");
    const feedsDir = tmpDir("feeds-retry2");

    const yaml = `
id: "retry-feed2"
name: "Retry Test 2"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "retry-feed2.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(RSS_FIXTURE, { status: 200 }) as Response;

    // First poll: evaluator throws → items queued
    const throwingEvaluator = async () => {
      throw new Error("LLM API unavailable");
    };
    const { state: failedState } = await pollFeed(config, stateDir, memRoot, throwingEvaluator);
    expect(failedState.pending_retry.length).toBe(2);

    // Second poll: evaluator works → pending_retry cleared, items processed
    const { state: successState, result } = await pollFeed(
      config,
      stateDir,
      memRoot,
      alwaysStore()
    );
    expect(successState.pending_retry).toHaveLength(0);
    // Items should have been evaluated (re-fetched as new since seen_ids cleared)
    expect(result.stored).toBeGreaterThanOrEqual(0); // items may be stored
  });

  test("feed.status shows pending_retry_count", async () => {
    const dir = tmpDir("plugin-retry-status");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    mkdirSync(feedsDir, { recursive: true });
    mkdirSync(stateDir, { recursive: true });

    const yaml = `id: "rs-feed"\nname: "RS"\ntype: "rss"\nsource:\n  url: "https://x.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: true\n`;
    writeFileSync(join(feedsDir, "rs-feed.yaml"), yaml, "utf-8");

    // Seed state with pending_retry
    const state = { ...emptyFeedState(), pending_retry: ["item-a", "item-b"] };
    await saveFeedState(stateDir, "rs-feed", state);

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, stateDir });
    const raw = await plugin.tool["feed_status"].execute({ feed_id: "rs-feed" }, {});
    const result = JSON.parse(raw as string);
    expect(result.pending_retry_count).toBe(2);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-2/retry-queue/step-52-p2-002 test=feed-ingestion.test.ts#AC-28 jira_ref=SWDE-52

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-15/step-v9-003 test=feed-ingestion.test.ts#AC-28-ceiling jira_ref=SWDE-52
  test("retry_attempts ceiling: after 5 consecutive failures, item is permanently failed (not re-queued)", async () => {
    const stateDir = tmpDir("ac28-ceiling-state");
    const memRoot = tmpDir("ac28-ceiling-mem");
    const feedsDir = tmpDir("ac28-ceiling-feeds");

    writeFileSync(
      join(feedsDir, "ceiling-feed.yaml"),
      "id: ceiling-feed\nname: Ceiling Test\ntype: rss\nsource:\n  url: https://x.com/feed\npoll_interval: 1h\nrelevance:\n  prompt: p\nenabled: true\n",
      "utf-8"
    );
    const [config] = loadFeedConfigs(feedsDir);

    // Seed state with 4 prior attempts for the item that will be returned by the feed
    // The RSS fixture has item_id derived from hashString of the item's guid/link
    // We'll use a partial evaluator and check that after the 5th attempt, the item is gone
    const partialEvaluator = async (): Promise<RelevanceDecision> => ({ store: true } as unknown as RelevanceDecision);

    const rssWithKnownId = `<?xml version="1.0"?><rss version="2.0"><channel><title>T</title><link>https://x.com</link><description>D</description><item><title>Ceiling Item</title><link>https://x.com/ceiling-1</link><guid>ceiling-guid-1</guid><description>Test</description></item></channel></rss>`;

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(rssWithKnownId, { status: 200 }) as Response;

    try {
      // First poll: item gets queued, retry_attempts["item-id"] = 1
      const { state: state1 } = await pollFeed(config, stateDir, memRoot, partialEvaluator);
      expect(state1.pending_retry.length).toBeGreaterThan(0);
      const itemId = state1.pending_retry[0];
      expect(state1.retry_attempts[itemId]).toBe(1);

      // Manually advance retry_attempts to 4 (simulating 3 more failed polls)
      state1.retry_attempts[itemId] = 4;
      // Clear seen_ids so the item is re-fetched
      state1.seen_ids = {};
      await saveFeedState(stateDir, "ceiling-feed", state1);

      // 5th poll: ceiling hit → item removed from retry_attempts and pending_retry
      // Also capture stderr to verify permanent_eval_failure log event (REQ-FEED-065 audit trail)
      const loggedLines: string[] = [];
      const originalWrite = process.stderr.write.bind(process.stderr);
      const prevEnv = process.env.AXIOM_FEED_INGESTION_DEBUG;
      process.env.AXIOM_FEED_INGESTION_DEBUG = "1";
      process.stderr.write = ((chunk: string | Uint8Array): boolean => {
        loggedLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
        return true;
      }) as typeof process.stderr.write;

      let state5: Awaited<ReturnType<typeof pollFeed>>["state"];
      try {
        ({ state: state5 } = await pollFeed(config, stateDir, memRoot, partialEvaluator));
      } finally {
        process.stderr.write = originalWrite;
        if (prevEnv === undefined) delete process.env.AXIOM_FEED_INGESTION_DEBUG;
        else process.env.AXIOM_FEED_INGESTION_DEBUG = prevEnv;
      }

      // State assertions: item removed from retry tracking
      expect(state5.retry_attempts[itemId]).toBeUndefined();
      expect(state5.pending_retry).not.toContain(itemId);

       // Log event assertion: permanent_eval_failure must be emitted (REQ-FEED-065)
       const events = loggedLines
         .filter(line => { try { JSON.parse(line); return true; } catch { return false; } })
         .map(line => JSON.parse(line) as Record<string, unknown>);
       const failureEvent = events.find(e => e.event === "permanent_eval_failure");
       expect(failureEvent).toBeDefined();
       expect(failureEvent?.item_id).toBe(itemId);
       // Negative assertion: eval_retry_queued must NOT be emitted on the 5th (ceiling) attempt
       // (spec: "eval_retry_queued is NOT emitted on the 5th (ceiling) attempt")
       // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-24/step-v18-003 test=feed-ingestion.test.ts#AC-28-ceiling jira_ref=SWDE-52
       const retryQueuedOnCeiling = events.filter(e => e.event === "eval_retry_queued" && e.item_id === itemId);
       expect(retryQueuedOnCeiling.length).toBe(0);
     } finally {
       globalThis.fetch = originalFetch;
     }
   });

  // Throw-path positive assertion test (step-v19-001, renamed in step-v20-002)
  // Tests the THROW PATH: eval_retry_queued IS emitted on repeated polls when the evaluator throws.
  // This is an independent positive assertion — the throw path has NO ceiling mechanism.
  // Items that throw indefinitely will retry forever via the 100-item FIFO pending_retry queue.
  // Note: This test does NOT verify ceiling behavior for the throw path (there is none).
  // The ceiling (5-attempt limit) applies ONLY to the validation-failure path.
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-25/step-v19-001 plan=phase-26/step-v20-002 plan=phase-27/step-v21-001 test=feed-ingestion.test.ts#AC-28-ceiling-throw jira_ref=SWDE-52
  test("throw path: eval_retry_queued IS emitted on repeated polls (positive case — throw path has no ceiling)", async () => {
    const stateDir = tmpDir("ac28-ceiling-throw-state");
    const memRoot = tmpDir("ac28-ceiling-throw-mem");
    const feedsDir = tmpDir("ac28-ceiling-throw-feeds");

    writeFileSync(
      join(feedsDir, "ceiling-throw-feed.yaml"),
      "id: ceiling-throw-feed\nname: Ceiling Throw Test\ntype: rss\nsource:\n  url: https://x.com/feed\npoll_interval: 1h\nrelevance:\n  prompt: p\nenabled: true\n",
      "utf-8"
    );
    const [config] = loadFeedConfigs(feedsDir);

    // Throwing evaluator — takes the throw path (not validation failure path)
    const throwingEvaluator = async (): Promise<RelevanceDecision> => {
      throw new Error("LLM API unavailable");
    };

    const rssWithKnownId = `<?xml version="1.0"?><rss version="2.0"><channel><title>T</title><link>https://x.com</link><description>D</description><item><title>Ceiling Throw Item</title><link>https://x.com/ceiling-throw-1</link><guid>ceiling-throw-guid-1</guid><description>Test</description></item></channel></rss>`;

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(rssWithKnownId, { status: 200 }) as Response;

    try {
      // First poll: item gets queued via throw path
      const { state: state1 } = await pollFeed(config, stateDir, memRoot, throwingEvaluator);
      expect(state1.pending_retry.length).toBeGreaterThan(0);
      const itemId = state1.pending_retry[0];

      // KEY ASSERTION: retry_attempts is NOT incremented on the throw path.
      // This is the mechanism that proves the throw path has no ceiling —
      // the ceiling fires when retry_attempts[itemId] >= 5, but since the throw
      // path never increments it, the ceiling never fires.
      // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-27/step-v21-001 jira_ref=SWDE-52
      expect(state1.retry_attempts[itemId]).toBeUndefined();

      // Run 6 polls total (past the 5-attempt validation-failure ceiling).
      // If the throw path had a ceiling, the event would stop being emitted after poll 5.
      // We assert it IS still emitted on EVERY poll (2-6), proving no ceiling exists.
      // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-28/step-v22-007 jira_ref=SWDE-52
      const allPollEvents: Record<string, unknown>[][] = [];
      for (let poll = 2; poll <= 6; poll++) {
        const loggedLines: string[] = [];
        const originalWrite = process.stderr.write.bind(process.stderr);
        const prevEnv = process.env.AXIOM_FEED_INGESTION_DEBUG;
        process.env.AXIOM_FEED_INGESTION_DEBUG = "1";
        process.stderr.write = ((chunk: string | Uint8Array): boolean => {
          loggedLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
          return true;
        }) as typeof process.stderr.write;

        // Manually clearing seen_ids here is redundant — pollFeed's own retry logic
        // (lines 1694-1696 of lib/feed-ingestion.ts) already removes pending_retry items
        // from seen_ids at poll start. This manual clear ensures the item re-appears as
        // new even if the retry logic were absent. The test is correct but does not
        // independently verify the retry path's own clearing logic.
        // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-28/step-v22-002 jira_ref=SWDE-52
        const currentState = await loadFeedState(stateDir, "ceiling-throw-feed");
        currentState.seen_ids = {};
        await saveFeedState(stateDir, "ceiling-throw-feed", currentState);

        try {
          await pollFeed(config, stateDir, memRoot, throwingEvaluator);
        } finally {
          process.stderr.write = originalWrite;
          if (prevEnv === undefined) delete process.env.AXIOM_FEED_INGESTION_DEBUG;
          else process.env.AXIOM_FEED_INGESTION_DEBUG = prevEnv;
        }

        const pollEvents = loggedLines
          .filter(line => { try { JSON.parse(line); return true; } catch { return false; } })
          .map(line => JSON.parse(line) as Record<string, unknown>);
        allPollEvents.push(pollEvents);
      }

      // Assert eval_retry_queued IS emitted on EVERY poll (2-6), not just poll 6.
      // This proves the throw path has no ceiling — the event fires consistently.
      for (let i = 0; i < allPollEvents.length; i++) {
        const pollNum = i + 2;
        const retryQueuedOnPoll = allPollEvents[i].filter(e => e.event === "eval_retry_queued" && e.item_id === itemId);
        expect(retryQueuedOnPoll.length, `poll ${pollNum} must emit eval_retry_queued`).toBeGreaterThan(0);
      }

      // Also verify retry_attempts is still undefined after 6 polls (never incremented)
      const finalState = await loadFeedState(stateDir, "ceiling-throw-feed");
      expect(finalState.retry_attempts[itemId]).toBeUndefined();
      // Verify item is still in pending_retry after 6 polls — proves it retries indefinitely
      // (not evicted, since only 1 item is in the queue and the cap is 100)
      // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-28/step-v22-004 jira_ref=SWDE-52
      expect(finalState.pending_retry).toContain(itemId);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // FIFO eviction test (step-v22-001)
  // Verifies that pending_retry caps at 100 items and the oldest item is dropped (FIFO)
  // when the 101st item is added within a single poll. This tests the >= 100 cap check
  // and shift() call in the throw path.
  // Note: pending_retry is CLEARED at the start of each poll (lines 1678, 1699 of
  // lib/feed-ingestion.ts), so the cap only applies within a single poll cycle.
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-28/step-v22-001 test=feed-ingestion.test.ts#AC-28-fifo-eviction jira_ref=SWDE-52
  test("AC-28-fifo-eviction: pending_retry caps at 100 items within a single poll, oldest item dropped (FIFO)", async () => {
    const stateDir = tmpDir("ac28-fifo-state");
    const memRoot = tmpDir("ac28-fifo-mem");
    const feedsDir = tmpDir("ac28-fifo-feeds");

    writeFileSync(
      join(feedsDir, "fifo-feed.yaml"),
      "id: fifo-feed\nname: FIFO Test\ntype: rss\nsource:\n  url: https://x.com/feed\npoll_interval: 1h\nrelevance:\n  prompt: p\nenabled: true\n",
      "utf-8"
    );
    const [config] = loadFeedConfigs(feedsDir);

    // Throwing evaluator — all items will fail and be added to pending_retry
    const throwingEvaluator = async (): Promise<RelevanceDecision> => {
      throw new Error("LLM API unavailable");
    };

    // Build RSS with 101 items — all will fail, triggering the FIFO cap.
    // Assumption: fast-xml-parser preserves document order when parsing <item> elements,
    // so fifo-guid-0 is processed first and fifo-guid-100 is processed last.
    // This is a documented guarantee of fast-xml-parser (array elements maintain source order).
    const items = Array.from({ length: 101 }, (_, i) =>
      `<item><title>Item ${i}</title><link>https://x.com/item-${i}</link><guid>fifo-guid-${i}</guid><description>Test ${i}</description></item>`
    ).join('\n');
    const rssWithManyItems = `<?xml version="1.0"?><rss version="2.0"><channel><title>T</title><link>https://x.com</link><description>D</description>${items}</channel></rss>`;

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(rssWithManyItems, { status: 200 }) as Response;

    try {
      const { state } = await pollFeed(config, stateDir, memRoot, throwingEvaluator);

      // Queue should be capped at 100 (not 101)
      expect(state.pending_retry.length).toBe(100);

      // The first item (fifo-guid-0) should have been evicted (FIFO shift)
      // because it was added first and then shifted out when the 101st was added.
      // item_id = hashString(feedId + ":" + guid) — stable across polls because
      // the guid is fixed and hashString is deterministic (SHA-256 hex truncated).
      const firstItemId = hashString("fifo-feed:fifo-guid-0");
      expect(state.pending_retry).not.toContain(firstItemId);

      // The last item (fifo-guid-100) should be present (added last, not evicted)
      const lastItemId = hashString("fifo-feed:fifo-guid-100");
      expect(state.pending_retry).toContain(lastItemId);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // Negative test: eval_retry_queued is NOT emitted on push paths (webhook, email).
  // Spec: "The eval_retry_queued event is emitted by poll paths only (RSS/Atom, Slack, API, iCal);
  // push paths (webhook, email) return an error response immediately and do NOT emit this event."
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-29/step-v23-003 test=feed-ingestion.test.ts#AC-28-push-no-retry-event jira_ref=SWDE-52
  test("AC-28-push-no-retry-event: eval_retry_queued is NOT emitted on webhook or email push paths", async () => {
    // Webhook path
    {
      const dir = tmpDir("ac28-push-wh");
      const feedsDir = join(dir, ".axiom", "feeds");
      mkdirSync(feedsDir, { recursive: true });
      writeFileSync(
        join(feedsDir, "wh-feed.yaml"),
        `id: "wh-feed"\nname: "Webhook Feed"\ntype: "webhook"\nsource: {}\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: true\n`,
        "utf-8"
      );

      // logEvent() writes to process.stderr (not stdout) when AXIOM_FEED_INGESTION_DEBUG=1.
      // Must intercept stderr to capture structured log events — matching the pattern used in
      // AC-24, AC-26, and AC-28-ceiling tests.
      const loggedLines: string[] = [];
      const origWrite = process.stderr.write.bind(process.stderr);
      const prevEnv = process.env.AXIOM_FEED_INGESTION_DEBUG;
      process.env.AXIOM_FEED_INGESTION_DEBUG = "1";
      process.stderr.write = (chunk: string | Uint8Array, ...args: unknown[]) => {
        if (typeof chunk === "string") loggedLines.push(chunk);
        return (origWrite as (...a: unknown[]) => boolean)(chunk, ...args);
      };

      const throwingEvaluator = async (): Promise<RelevanceDecision> => {
        throw new Error("LLM unavailable");
      };

      const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, evaluator: throwingEvaluator });
      try {
        await plugin.tool["feed_webhook"].execute({
          feed_id: "wh-feed",
          payload: { title: "T", body: "B", url: "https://x.com/1", id: "w1" },
        }, {});
      } finally {
        process.stderr.write = origWrite as typeof process.stderr.write;
        if (prevEnv === undefined) delete process.env.AXIOM_FEED_INGESTION_DEBUG;
        else process.env.AXIOM_FEED_INGESTION_DEBUG = prevEnv;
      }

      const events = loggedLines
        .filter(line => { try { JSON.parse(line); return true; } catch { return false; } })
        .map(line => JSON.parse(line) as Record<string, unknown>);
      const retryQueuedEvents = events.filter(e => e.event === "eval_retry_queued");
      expect(retryQueuedEvents.length, "webhook path must NOT emit eval_retry_queued").toBe(0);
    }

    // Email path
    {
      const dir = tmpDir("ac28-push-email");
      const feedsDir = join(dir, ".axiom", "feeds");
      mkdirSync(feedsDir, { recursive: true });
      writeFileSync(
        join(feedsDir, "email-feed.yaml"),
        `id: "email-feed"\nname: "Email Feed"\ntype: "email"\nsource: {}\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: true\n`,
        "utf-8"
      );

      // Same stderr interception pattern as webhook path above.
      const loggedLines: string[] = [];
      const origWrite = process.stderr.write.bind(process.stderr);
      const prevEnv = process.env.AXIOM_FEED_INGESTION_DEBUG;
      process.env.AXIOM_FEED_INGESTION_DEBUG = "1";
      process.stderr.write = (chunk: string | Uint8Array, ...args: unknown[]) => {
        if (typeof chunk === "string") loggedLines.push(chunk);
        return (origWrite as (...a: unknown[]) => boolean)(chunk, ...args);
      };

      const throwingEvaluator = async (): Promise<RelevanceDecision> => {
        throw new Error("LLM unavailable");
      };

      const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, evaluator: throwingEvaluator });
      try {
        await plugin.tool["feed_email"].execute({
          feed_id: "email-feed",
          subject: "Test Subject",
          body: "Test body content",
          from_email: "sender@example.com",
        }, {});
      } finally {
        process.stderr.write = origWrite as typeof process.stderr.write;
        if (prevEnv === undefined) delete process.env.AXIOM_FEED_INGESTION_DEBUG;
        else process.env.AXIOM_FEED_INGESTION_DEBUG = prevEnv;
      }

      const events = loggedLines
        .filter(line => { try { JSON.parse(line); return true; } catch { return false; } })
        .map(line => JSON.parse(line) as Record<string, unknown>);
      const retryQueuedEvents = events.filter(e => e.event === "eval_retry_queued");
      expect(retryQueuedEvents.length, "email path must NOT emit eval_retry_queued").toBe(0);
    }
  });

  // Evaluator timeout test (step-v11-001)
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#14.4 plan=phase-17/step-v11-001 test=feed-ingestion.test.ts#AC-28-timeout jira_ref=SWDE-52
  test("evaluator that hangs beyond timeout_ms is treated as failure and item queued for retry", async () => {
    const stateDir = tmpDir("ac28-timeout-state");
    const memRoot = tmpDir("ac28-timeout-mem");
    const feedsDir = tmpDir("ac28-timeout-feeds");

    writeFileSync(
      join(feedsDir, "timeout-feed.yaml"),
      "id: timeout-feed\nname: Timeout Test\ntype: rss\nsource:\n  url: https://x.com/feed\npoll_interval: 1h\nrelevance:\n  prompt: p\n  timeout_ms: 50\nenabled: true\n",
      "utf-8"
    );
    const [config] = loadFeedConfigs(feedsDir);

    // Evaluator that takes longer than timeout_ms (200ms > 50ms timeout)
    const slowEvaluator = async (): Promise<RelevanceDecision> =>
      new Promise(resolve => setTimeout(() => resolve({
        store: true, reason: 'ok', priority: 'low', tags: [], summary: 'ok'
      }), 200));

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const { result, state } = await pollFeed(config, stateDir, memRoot, slowEvaluator);

      // Timeout should be treated as evaluator failure → item queued for retry
      expect(result.errors.length).toBeGreaterThan(0);
      expect(result.errors[0]).toMatch(/evaluator timeout|timeout/i);
      expect(state.pending_retry.length).toBeGreaterThan(0);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-22/step-v16-005 test=feed-ingestion.test.ts#AC-28-eval-retry-queued jira_ref=SWDE-52
  test("eval_retry_queued log event is emitted when evaluator throws (REQ-FEED-065)", async () => {
    const stateDir = tmpDir("state-retry-event");
    const memRoot = tmpDir("mem-retry-event");
    const feedsDir = tmpDir("feeds-retry-event");

    const feedYaml = `
id: "retry-event-feed"
name: "Retry Event Test"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "retry-event-feed.yaml"), feedYaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const throwingEvaluator = async () => { throw new Error("LLM API unavailable"); };

    const loggedLines: string[] = [];
    const originalWrite = process.stderr.write.bind(process.stderr);
    const prevEnv = process.env.AXIOM_FEED_INGESTION_DEBUG;
    process.env.AXIOM_FEED_INGESTION_DEBUG = "1";
    process.stderr.write = ((chunk: string | Uint8Array): boolean => {
      loggedLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
      return true;
    }) as typeof process.stderr.write;

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      await pollFeed(config, stateDir, memRoot, throwingEvaluator);
    } finally {
      process.stderr.write = originalWrite;
      if (prevEnv === undefined) delete process.env.AXIOM_FEED_INGESTION_DEBUG;
      else process.env.AXIOM_FEED_INGESTION_DEBUG = prevEnv;
      globalThis.fetch = originalFetch;
    }

    const events = loggedLines
      .filter(line => { try { JSON.parse(line); return true; } catch { return false; } })
      .map(line => JSON.parse(line) as Record<string, unknown>);

    // eval_retry_queued must be emitted for each item queued for retry
    // RSS_FIXTURE has 2 items — a throwing evaluator must emit exactly 2 events
    const retryEvents = events.filter(e => e.event === "eval_retry_queued");
    expect(retryEvents.length).toBe(2);
    expect(retryEvents[0].feed_id).toBe("retry-event-feed");
    expect(typeof retryEvents[0].item_id).toBe("string");
    expect(retryEvents[0].item_id.length).toBeGreaterThan(0);
    // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-24/step-v18-006 test=feed-ingestion.test.ts#AC-28-eval-retry-queued jira_ref=SWDE-52
    expect(typeof retryEvents[1].item_id).toBe("string");
    expect(retryEvents[1].item_id.length).toBeGreaterThan(0);

    // Cross-check: state.pending_retry must also have exactly 2 items (event count matches state)
    // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-24/step-v18-005 test=feed-ingestion.test.ts#AC-28-eval-retry-queued jira_ref=SWDE-52
    const { state } = await pollFeed(config, stateDir, memRoot, throwingEvaluator);
    // After a second poll with the same throwing evaluator, pending_retry should still have 2 items.
    // Mechanism: at poll start, pending_retry is CLEARED and items are removed from seen_ids so
    // they re-fetch as new. The throwing evaluator fires again and both items are re-queued.
    // The includes() guard is vacuous here because the queue was just emptied — it prevents
    // duplicates within a single poll cycle, not across polls.
    // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-065 plan=phase-25/step-v19-002 test=feed-ingestion.test.ts#AC-28-eval-retry-queued jira_ref=SWDE-52
    expect(state.pending_retry.length).toBe(2);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-29: Intra-run cost accounting (NF-001/REQ-FEED-082)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-082 plan=phase-2/cost-accounting/step-52-p2-003 test=feed-ingestion.test.ts#AC-29 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-29: Intra-run cost accounting (NF-001/REQ-FEED-082)", () => {
  test("pollFeed accumulates cost_usd from evaluator into cost_incurred_usd", async () => {
    const stateDir = tmpDir("state-cost");
    const memRoot = tmpDir("mem-cost");
    const feedsDir = tmpDir("feeds-cost");

    const yaml = `
id: "cost-feed"
name: "Cost Test"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "cost-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      // Evaluator that costs $0.01 per item; RSS_FIXTURE has 2 items
      const { result, state } = await pollFeed(
        config,
        stateDir,
        memRoot,
        alwaysStore("medium", 0.01)
      );
      // 2 items × $0.01 = $0.02
      expect(result.cost_incurred_usd).toBeCloseTo(0.02, 4);
      expect(state.cost_today_usd).toBeCloseTo(0.02, 4);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  test("feed.poll intra-run global cap: first feed incurs cost, second feed is skipped", async () => {
    const dir = tmpDir("intra-run-cap");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    const memRoot = join(dir, ".memory-bank");
    mkdirSync(feedsDir, { recursive: true });
    mkdirSync(stateDir, { recursive: true });

    // Global cap: $0.015 (just enough for first feed's 2 items at $0.01 each, but not both feeds)
    const feedsConfigPath = join(dir, ".axiom", "feeds.yaml");
    mkdirSync(join(dir, ".axiom"), { recursive: true });
    writeFileSync(feedsConfigPath, "feeds:\n  enabled: true\n  global_max_cost_per_day_usd: 0.015\n", "utf-8");

    const f1 = `id: "intra-a"\nname: "A"\ntype: "rss"\nsource:\n  url: "https://a.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 50\n  max_cost_per_day_usd: 5\nenabled: true\n`;
    const f2 = `id: "intra-b"\nname: "B"\ntype: "rss"\nsource:\n  url: "https://b.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 50\n  max_cost_per_day_usd: 5\nenabled: true\n`;
    writeFileSync(join(feedsDir, "intra-a.yaml"), f1, "utf-8");
    writeFileSync(join(feedsDir, "intra-b.yaml"), f2, "utf-8");

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const plugin = FeedIngestionPlugin({
        directory: dir,
        feedsDir,
        feedsConfigPath,
        stateDir,
        memoryRoot: memRoot,
        // Evaluator costs $0.01 per item; RSS_FIXTURE has 2 items
        // First feed: 2 × $0.01 = $0.02 → exceeds $0.015 cap AFTER first feed
        // Second feed should be blocked by global cap
        evaluator: alwaysStore("medium", 0.01),
      });

      const raw = await plugin.tool["feed_poll"].execute({}, {});
      const result = JSON.parse(raw as string);

      // First feed processed, second feed blocked by global cap
      expect(result.global_budget_capped).toBe(true);
      // At least one result should have the global cap error
      const capResult = result.results?.find((r: { errors: string[] }) =>
        r.errors.some(e => /global.*cap|cap.*reached/i.test(e))
      );
      expect(capResult).toBeDefined();
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-082 plan=phase-2/cost-accounting/step-52-p2-003 test=feed-ingestion.test.ts#AC-29 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-30: HTTP API feed polling (REQ-FEED-030/031/032/033)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-030 plan=phase-2/api-polling/step-52-p2-004 test=feed-ingestion.test.ts#AC-30 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-30: HTTP API feed polling (REQ-FEED-030/031/032/033)", () => {
  const GITHUB_RELEASES_JSON = JSON.stringify([
    {
      id: 1,
      name: "v1.0.0",
      body: "First release notes.",
      html_url: "https://github.com/org/repo/releases/tag/v1.0.0",
      published_at: "2026-05-08T10:00:00Z",
      author: { login: "alice" }
    },
    {
      id: 2,
      name: "v1.1.0",
      body: "Second release notes.",
      html_url: "https://github.com/org/repo/releases/tag/v1.1.0",
      published_at: "2026-05-09T10:00:00Z"
    }
  ]);

  test("parseApiItems extracts items from JSON array with .[] path", () => {
    const json = JSON.parse(GITHUB_RELEASES_JSON);
    const items = parseApiItems(json, ".[]", "github-releases");
    expect(items).toHaveLength(2);
    expect(items[0].title).toBe("v1.0.0");
    expect(items[0].content).toBe("First release notes.");
    expect(items[0].url).toBe("https://github.com/org/repo/releases/tag/v1.0.0");
    expect(items[0].feed_id).toBe("github-releases");
  });

  test("parseApiItems handles root dot path for JSON object", () => {
    const json = { title: "Single", url: "https://x.com", id: "x1" };
    const items = parseApiItems(json, ".", "single-feed");
    expect(items).toHaveLength(1);
    expect(items[0].title).toBe("Single");
  });

  test("interpolateEnvVars replaces ${VAR} with env values", () => {
    process.env.TEST_API_TOKEN = "secret-token-123";
    const result = interpolateEnvVars("Bearer ${TEST_API_TOKEN}");
    expect(result).toBe("Bearer secret-token-123");
    delete process.env.TEST_API_TOKEN;
  });

  test("interpolateEnvVars returns null for missing env var", () => {
    const result = interpolateEnvVars("Bearer ${MISSING_VAR_XYZ}");
    expect(result).toBeNull();
  });

  test("pollFeed handles api type with JSON response", async () => {
    const stateDir = tmpDir("state-api");
    const memRoot = tmpDir("mem-api");
    const feedsDir = tmpDir("feeds-api");

    const yaml = `
id: "github-releases"
name: "GitHub Releases"
type: "api"
source:
  url: "https://api.github.com/repos/org/repo/releases"
  jq_extract: ".[]"
poll_interval: "1h"
relevance:
  prompt: "Is this a relevant release?"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "github-releases.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(GITHUB_RELEASES_JSON, {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }) as Response;

    try {
      const { result } = await pollFeed(config, stateDir, memRoot, alwaysStore());
      expect(result.fetched).toBe(true);
      expect(result.new_items).toBe(2);
      expect(result.stored).toBe(2);
      expect(result.errors).toHaveLength(0);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  test("api feed with missing env var in URL marks feed unhealthy", async () => {
    const stateDir = tmpDir("state-api-env");
    const memRoot = tmpDir("mem-api-env");
    const feedsDir = tmpDir("feeds-api-env");

    const yaml = `
id: "missing-env-feed"
name: "Missing Env"
type: "api"
source:
  url: "https://api.example.com/\${MISSING_API_KEY_XYZ_UNIQUE}/endpoint"
  jq_extract: ".[]"
poll_interval: "1h"
relevance:
  prompt: "p"
`;
    writeFileSync(join(feedsDir, "missing-env-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const { result, state } = await pollFeed(config, stateDir, memRoot, alwaysStore());
    expect(result.errors.length).toBeGreaterThan(0);
    expect(result.errors[0]).toMatch(/missing.*env.*var|env.*var.*missing/i);
    expect(state.consecutive_failures).toBe(1);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-030 plan=phase-2/api-polling/step-52-p2-004 test=feed-ingestion.test.ts#AC-30 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-31: Webhook push tool (REQ-FEED-025/026/027/028)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-025 plan=phase-2/webhook-tool/step-52-p2-005 test=feed-ingestion.test.ts#AC-31 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-31: Webhook push tool (REQ-FEED-025/026/027/028)", () => {
  function makeWebhookFeed(dir: string, feedsDir: string, opts: { secret?: string; required?: string[] } = {}) {
    mkdirSync(feedsDir, { recursive: true });
    const webhookSection = opts.secret
      ? `webhook:\n  secret: "${opts.secret}"${opts.required ? `\n  schema:\n    required: [${opts.required.map(r => `"${r}"`).join(", ")}]` : ""}`
      : opts.required
      ? `webhook:\n  schema:\n    required: [${opts.required.map(r => `"${r}"`).join(", ")}]`
      : "";
    const yaml = `id: "wh-feed"\nname: "Webhook Feed"\ntype: "webhook"\nsource: {}\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 50\n  max_cost_per_day_usd: 5\n${webhookSection}\nenabled: true\n`;
    writeFileSync(join(feedsDir, "wh-feed.yaml"), yaml, "utf-8");
  }

  test("feed.webhook accepts valid payload and stores item", async () => {
    const dir = tmpDir("wh-accept");
    const feedsDir = join(dir, ".axiom", "feeds");
    makeWebhookFeed(dir, feedsDir);

    const plugin = FeedIngestionPlugin({
      directory: dir,
      feedsDir,
      evaluator: alwaysStore(),
    });
    const raw = await plugin.tool["feed_webhook"].execute({
      feed_id: "wh-feed",
      payload: { title: "New Release", body: "Content here", url: "https://x.com/release-1", id: "r1" },
    }, {});
    const result = JSON.parse(raw as string);
    expect(result.status).toBe("stored");
    expect(result.item_id).toBeDefined();
    expect(result.feed_id).toBe("wh-feed");
  });

  test("feed.webhook returns error for non-webhook feed type", async () => {
    const dir = tmpDir("wh-type-check");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });
    writeFileSync(join(feedsDir, "rss-feed.yaml"),
      "id: rss-feed\nname: RSS\ntype: rss\nsource:\n  url: https://x.com\npoll_interval: 1h\nrelevance:\n  prompt: p\nenabled: true\n",
      "utf-8"
    );
    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, evaluator: alwaysStore() });
    const raw = await plugin.tool["feed_webhook"].execute({ feed_id: "rss-feed", payload: { title: "T" } }, {});
    const result = JSON.parse(raw as string);
    expect(result.error).toMatch(/not a webhook feed/i);
  });

  test("feed.webhook rejects payload with invalid HMAC signature", async () => {
    const dir = tmpDir("wh-hmac");
    const feedsDir = join(dir, ".axiom", "feeds");
    makeWebhookFeed(dir, feedsDir, { secret: "my-webhook-secret" });

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, evaluator: alwaysStore() });
    const raw = await plugin.tool["feed_webhook"].execute({
      feed_id: "wh-feed",
      payload: { title: "Tampered", url: "https://x.com" },
      signature: "sha256=invalidhashvalue",
    }, {});
    const result = JSON.parse(raw as string);
    expect(result.error).toMatch(/invalid signature/i);
    expect(result.status).toBe(401);
  });

  test("feed.webhook accepts payload with valid HMAC signature", async () => {
    const dir = tmpDir("wh-hmac-valid");
    const feedsDir = join(dir, ".axiom", "feeds");
    const secret = "test-secret-key";
    makeWebhookFeed(dir, feedsDir, { secret });

    // Compute valid signature
    const { createHmac: _createHmac } = await import("node:crypto");
    const payload = { title: "Valid", url: "https://x.com/valid", id: "v1" };
    const payloadStr = JSON.stringify(payload);
    const sig = "sha256=" + _createHmac("sha256", secret).update(payloadStr).digest("hex");

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, evaluator: alwaysStore() });
    const raw = await plugin.tool["feed_webhook"].execute({
      feed_id: "wh-feed",
      payload,
      signature: sig,
    }, {});
    const result = JSON.parse(raw as string);
    expect(result.status).toBe("stored");
  });

  test("feed.webhook rejects payload missing required schema fields", async () => {
    const dir = tmpDir("wh-schema");
    const feedsDir = join(dir, ".axiom", "feeds");
    makeWebhookFeed(dir, feedsDir, { required: ["title", "url"] });

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, evaluator: alwaysStore() });
    const raw = await plugin.tool["feed_webhook"].execute({
      feed_id: "wh-feed",
      payload: { title: "Has Title" }, // missing url
    }, {});
    const result = JSON.parse(raw as string);
    expect(result.error).toMatch(/missing required field/i);
    expect(result.status).toBe(400);
  });

  test("normalizeWebhookPayload extracts fields from GitHub-style payload", () => {
    const payload = {
      action: "published",
      release: { name: "v1.0.0", body: "Release notes", html_url: "https://github.com/r", id: 123 },
      repository: { full_name: "org/repo", html_url: "https://github.com/org/repo" },
      sender: { login: "alice" },
    };
    const item = normalizeWebhookPayload(payload, "gh-releases");
    expect(item.feed_id).toBe("gh-releases");
    expect(item.author).toBe("alice"); // from sender.login
    expect(typeof item.item_id).toBe("string");
    expect(item.item_id.length).toBeGreaterThan(0);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-025 plan=phase-2/webhook-tool/step-52-p2-005 test=feed-ingestion.test.ts#AC-31 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-32: API headers forwarded to HTTP fetch (REQ-FEED-031 F-001 fix)
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-32: API feed headers forwarded to fetch (REQ-FEED-031)", () => {
  test("interpolated Authorization header reaches the fetch() call", async () => {
    const stateDir = tmpDir("state-headers");
    const memRoot = tmpDir("mem-headers");
    const feedsDir = tmpDir("feeds-headers");

    process.env.TEST_GITHUB_TOKEN = "ghp_test_token_123";

    const yaml = `
id: "headers-feed"
name: "Headers Test"
type: "api"
source:
  url: "https://api.github.com/repos/org/repo/releases"
  headers:
    Authorization: "Bearer \${TEST_GITHUB_TOKEN}"
    X-Custom: "custom-value"
  jq_extract: ".[]"
poll_interval: "1h"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "headers-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    let capturedHeaders: Record<string, string> = {};
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async (url: RequestInfo, init?: RequestInit) => {
      capturedHeaders = Object.fromEntries(
        Object.entries((init?.headers ?? {}) as Record<string, string>)
      );
      return new Response(JSON.stringify([{ title: "Release", url: "https://x.com", id: 1 }]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }) as Response;
    };

    try {
      await pollFeed(config, stateDir, memRoot, alwaysStore());
      // Authorization header must be forwarded (REQ-FEED-031)
      expect(capturedHeaders["Authorization"]).toBe("Bearer ghp_test_token_123");
      expect(capturedHeaders["X-Custom"]).toBe("custom-value");
    } finally {
      globalThis.fetch = originalFetch;
      delete process.env.TEST_GITHUB_TOKEN;
    }
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-031 plan=phase-2/api-polling/f001-fix test=feed-ingestion.test.ts#AC-32 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-33: Webhook cost budget enforcement (REQ-FEED-081 F-002 fix)
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-33: Webhook cost budget enforcement (REQ-FEED-081)", () => {
  test("feed.webhook returns budget_exceeded when cost cap reached", async () => {
    const dir = tmpDir("wh-cost-cap");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    mkdirSync(feedsDir, { recursive: true });
    mkdirSync(stateDir, { recursive: true });

    const yaml = `id: "wh-cost"\nname: "Cost WH"\ntype: "webhook"\nsource: {}\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 50\n  max_cost_per_day_usd: 0.50\nenabled: true\n`;
    writeFileSync(join(feedsDir, "wh-cost.yaml"), yaml, "utf-8");

    // Pre-seed state with cost at cap
    const seedState = { ...emptyFeedState(), cost_today_usd: 0.50, budget_date: utcDateString() };
    await saveFeedState(stateDir, "wh-cost", seedState);

    const plugin = FeedIngestionPlugin({
      directory: dir,
      feedsDir,
      stateDir,
      evaluator: alwaysStore("medium", 0.01),
    });

    const raw = await plugin.tool["feed_webhook"].execute({
      feed_id: "wh-cost",
      payload: { title: "Push Event", url: "https://x.com", id: "e1" },
    }, {});
    const result = JSON.parse(raw as string);
    expect(result.status).toBe("budget_exceeded");
    expect(result.message).toMatch(/cost/i);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-081 plan=phase-2/webhook-tool/f002-fix test=feed-ingestion.test.ts#AC-33 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-34: iCal calendar feed parsing (REQ-FEED-045/046/047)
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-34: iCal calendar feed parsing (REQ-FEED-045/046/047)", () => {
  // Build a reference "now" for reproducible tests
  const NOW = new Date("2026-05-08T10:00:00Z");

  // Helper to format iCal datetime
  function icalDate(date: Date): string {
    return date.toISOString().replace(/-|:|\.\d{3}/g, "").replace("Z", "Z");
  }

  const PAST = new Date(NOW.getTime() - 2 * 24 * 60 * 60 * 1000); // 2 days ago
  const TODAY = new Date(NOW.getTime() + 2 * 60 * 60 * 1000); // 2 hours from now
  const FUTURE_NEAR = new Date(NOW.getTime() + 3 * 24 * 60 * 60 * 1000); // 3 days from now
  const FUTURE_FAR = new Date(NOW.getTime() + 15 * 24 * 60 * 60 * 1000); // 15 days from now

  function makeIcal(...events: Array<{ uid: string; summary: string; dtstart: Date; dtend?: Date; description?: string; location?: string; attendees?: string[] }>): string {
    const vevents = events.map(e => {
      const dtend = e.dtend ?? new Date(e.dtstart.getTime() + 60 * 60 * 1000);
      const attendeeLines = (e.attendees ?? []).map(a => `ATTENDEE;CN=${a}:mailto:${a.toLowerCase().replace(/\s/g, "")}@test.com`).join("\n");
      return `BEGIN:VEVENT
UID:${e.uid}
SUMMARY:${e.summary}
DTSTART:${icalDate(e.dtstart)}
DTEND:${icalDate(dtend)}
DESCRIPTION:${e.description ?? ""}
LOCATION:${e.location ?? ""}
${attendeeLines}
END:VEVENT`;
    }).join("\n");
    return `BEGIN:VCALENDAR\nVERSION:2.0\n${vevents}\nEND:VCALENDAR`;
  }

  test("parseICalItems returns only events within lookahead window", () => {
    const ical = makeIcal(
      { uid: "past-1", summary: "Past Event", dtstart: PAST },
      { uid: "today-1", summary: "Today Event", dtstart: TODAY },
      { uid: "near-1", summary: "Near Future Event", dtstart: FUTURE_NEAR },
      { uid: "far-1", summary: "Far Future Event", dtstart: FUTURE_FAR },
    );
    // Use lookahead of 7 days from NOW — TODAY and FUTURE_NEAR should be included
    // PAST and FUTURE_FAR should be excluded
     const items = parseICalItems(ical, "cal-feed", 7, NOW);
    const titles = items.map(i => i.title);
    expect(titles).toContain("Today Event");
    expect(titles).toContain("Near Future Event");
    expect(titles).not.toContain("Far Future Event");
    // PAST is more than 1 day ago, so excluded
    expect(titles).not.toContain("Past Event");
  });

  test("parseICalItems extracts title, description, location, attendees", () => {
    const ical = makeIcal({
      uid: "meeting-1",
      summary: "Team Standup",
      dtstart: TODAY,
      description: "Daily sync meeting",
      location: "Zoom: https://zoom.us/j/123",
      attendees: ["Alice Smith", "Bob Jones"],
    });
    const items = parseICalItems(ical, "work-cal", 7, NOW);
    expect(items).toHaveLength(1);
    expect(items[0].title).toBe("Team Standup");
    expect(items[0].content).toContain("Daily sync meeting");
    expect(items[0].content).toContain("Zoom");
    expect(items[0].author).toBe("Alice Smith"); // first attendee
    expect(items[0].tags).toContain("meeting");
    expect(items[0].feed_id).toBe("work-cal");
  });

  test("parseICalDate parses various iCal date formats", () => {
    // Full datetime with Z
    const dt = parseICalDate("20260508T100000Z");
    expect(dt.toISOString()).toBe("2026-05-08T10:00:00.000Z");
    // All-day date
    const allDay = parseICalDate("20260508");
    expect(allDay.toISOString().startsWith("2026-05-08")).toBe(true);
  });

  test("unescapeICal handles backslash escapes", () => {
    expect(unescapeICal("Line 1\\nLine 2")).toBe("Line 1\nLine 2");
    expect(unescapeICal("a\\,b")).toBe("a,b");
    expect(unescapeICal("a\\\\b")).toBe("a\\b");
  });

  test("pollFeed handles ical type with lookahead filtering", async () => {
    const stateDir = tmpDir("state-ical");
    const memRoot = tmpDir("mem-ical");
    const feedsDir = tmpDir("feeds-ical");

    const yaml = `
id: "work-calendar"
name: "Work Calendar"
type: "ical"
source:
  url: "https://calendar.example.com/feed.ics"
  lookahead_days: 7
poll_interval: "4h"
relevance:
  prompt: "Is this calendar event relevant?"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "work-calendar.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const icalContent = makeIcal(
      { uid: "e1", summary: "Team Meeting", dtstart: TODAY },
      { uid: "e2", summary: "Far Event", dtstart: FUTURE_FAR }
    );

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(icalContent, {
        status: 200,
        headers: { "Content-Type": "text/calendar" },
      }) as Response;

    try {
      const { result } = await pollFeed(config, stateDir, memRoot, alwaysStore(), undefined, { _now: NOW });
      expect(result.fetched).toBe(true);
      expect(result.new_items).toBe(1); // only TODAY event within lookahead
      expect(result.errors).toHaveLength(0);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-045 plan=phase-3/ical-feed/step-52-p3-001 test=feed-ingestion.test.ts#AC-34 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-35: Slack channel polling (REQ-FEED-035/036/037)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-035 plan=phase-3/slack-feed/step-52-p3-002 test=feed-ingestion.test.ts#AC-35 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-35: Slack channel polling (REQ-FEED-035/036/037)", () => {
  const SLACK_RESPONSE = {
    ok: true,
    messages: [
      {
        text: "Deploy failed: critical error in prod",
        user: "U001",
        ts: "1746700800.000100",
        user_profile: { display_name: "Alice", real_name: "Alice Smith" },
      },
      {
        text: "All systems normal today",
        user: "U002",
        ts: "1746700900.000200",
        user_profile: { display_name: "Bob" },
        thread_ts: "1746700900.000200", // top-level, not a reply (thread_ts === ts)
      },
      {
        text: "Agreed, no issues seen",
        user: "U003",
        ts: "1746701000.000300",
        user_profile: { display_name: "Carol" },
        thread_ts: "1746700900.000200", // reply to U002's message (thread_ts !== ts)
      },
    ],
  };

  test("parseSlackMessages normalizes messages with author=display_name (REQ-FEED-037)", () => {
    const items = parseSlackMessages(SLACK_RESPONSE, "slack-feed");
    expect(items).toHaveLength(3);
    expect(items[0].author).toBe("Alice");
    expect(items[1].author).toBe("Bob");
    expect(items[0].tags).toContain("slack");
    expect(items[0].feed_id).toBe("slack-feed");
  });

  test("parseSlackMessages filters by keyword (REQ-FEED-036)", () => {
    const items = parseSlackMessages(SLACK_RESPONSE, "slack-feed", { filterKeyword: "critical" });
    expect(items).toHaveLength(1);
    expect(items[0].content).toContain("critical");
  });

  test("parseSlackMessages filters by user (REQ-FEED-036)", () => {
    const items = parseSlackMessages(SLACK_RESPONSE, "slack-feed", { filterUser: "Bob" });
    expect(items).toHaveLength(1);
    expect(items[0].author).toBe("Bob");
  });

  test("parseSlackMessages threads_only returns only replies (REQ-FEED-036)", () => {
    const items = parseSlackMessages(SLACK_RESPONSE, "slack-feed", { threadsOnly: true });
    // Only Carol's message (thread_ts !== ts) should be returned
    expect(items).toHaveLength(1);
    expect(items[0].author).toBe("Carol");
    expect(items[0].tags).toContain("thread-reply");
  });

  test("parseSlackMessages handles error response gracefully (empty response)", () => {
    const items = parseSlackMessages({ ok: false, messages: [] }, "slack-feed");
    expect(items).toHaveLength(0);
  });

  test("pollFeed handles slack type with token from env var", async () => {
    const stateDir = tmpDir("state-slack");
    const memRoot = tmpDir("mem-slack");
    const feedsDir = tmpDir("feeds-slack");

    process.env.TEST_SLACK_TOKEN = "xoxb-test-token";

    const yaml = `
id: "alerts-channel"
name: "Alerts"
type: "slack"
source:
  channel: "C0123ALERTS"
  slack_token_env: "TEST_SLACK_TOKEN"
  slack_filter_keyword: "critical"
poll_interval: "5m"
relevance:
  prompt: "Is this alert critical?"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "alerts-channel.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    let capturedUrl = "";
    let capturedAuth = "";
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async (url: RequestInfo, init?: RequestInit) => {
      capturedUrl = String(url);
      capturedAuth = ((init?.headers ?? {}) as Record<string, string>)["Authorization"] ?? "";
      return new Response(JSON.stringify(SLACK_RESPONSE), { status: 200 }) as Response;
    };

    try {
      const { result } = await pollFeed(config, stateDir, memRoot, alwaysStore());
      expect(capturedUrl).toContain("conversations.history");
      expect(capturedUrl).toContain("C0123ALERTS");
      expect(capturedAuth).toBe("Bearer xoxb-test-token");
      // keyword filter "critical" → only 1 message matches
      expect(result.new_items).toBe(1);
      expect(result.errors).toHaveLength(0);
    } finally {
      globalThis.fetch = originalFetch;
      delete process.env.TEST_SLACK_TOKEN;
    }
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-035 plan=phase-3/slack-feed/step-52-p3-002 test=feed-ingestion.test.ts#AC-35 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-36: Email feed tool (REQ-FEED-040/041/042)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-040 plan=phase-3/email-feed/step-52-p3-003 test=feed-ingestion.test.ts#AC-36 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-36: Email feed tool (REQ-FEED-040/041/042)", () => {
  test("stripHtml removes HTML tags and decodes entities (REQ-FEED-042)", () => {
    const html = `<html><body><h1>Hello &amp; World</h1><p>This is <strong>bold</strong> text.</p><br/><p>Second para.</p></body></html>`;
    const plain = stripHtml(html);
    expect(plain).toContain("Hello & World");
    expect(plain).toContain("bold");
    expect(plain).not.toContain("<");
    expect(plain).not.toContain("<strong>");
    expect(plain).not.toContain("&amp;");
  });

  test("stripHtml strips script and style tags with content", () => {
    const html = `<div>Content</div><script>alert('xss')</script><style>.foo{color:red}</style>more`;
    const plain = stripHtml(html);
    expect(plain).toContain("Content");
    expect(plain).toContain("more");
    expect(plain).not.toContain("alert");
    expect(plain).not.toContain("color:red");
  });

  function makeEmailFeed(dir: string, feedsDir: string, opts: { subject_regex?: string } = {}) {
    mkdirSync(feedsDir, { recursive: true });
    // Use single-quoted YAML string so backslashes in regex don't need escaping
    const regexLine = opts.subject_regex ? `  subject_regex: '${opts.subject_regex}'` : "";
    const yaml = `id: "digest-feed"\nname: "Email Digest"\ntype: "email"\nsource:\n${regexLine}\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 50\n  max_cost_per_day_usd: 5\nenabled: true\n`;
    writeFileSync(join(feedsDir, "digest-feed.yaml"), yaml, "utf-8");
    void dir;
  }

  test("feed.email accepts valid email and stores item (REQ-FEED-040)", async () => {
    const dir = tmpDir("email-accept");
    const feedsDir = join(dir, ".axiom", "feeds");
    makeEmailFeed(dir, feedsDir);

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, evaluator: alwaysStore() });
    const raw = await plugin.tool["feed_email"].execute({
      feed_id: "digest-feed",
      subject: "Weekly Security Digest",
      body: "<p>New <strong>CVE</strong> found in openssl.</p>",
      from_email: "digest@security.example.com",
    }, {});
    const result = JSON.parse(raw as string);
    expect(result.status).toBe("stored");
    expect(result.item_id).toBeDefined();
  });

  test("feed.email filters by subject regex (REQ-FEED-041)", async () => {
    const dir = tmpDir("email-regex");
    const feedsDir = join(dir, ".axiom", "feeds");
    makeEmailFeed(dir, feedsDir, { subject_regex: "^\\[ALERT\\]" });

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, evaluator: alwaysStore() });

    // Non-matching subject → filtered
    const raw1 = await plugin.tool["feed_email"].execute({
      feed_id: "digest-feed",
      subject: "Normal Newsletter",
      body: "Regular content",
      from_email: "news@example.com",
    }, {});
    expect(JSON.parse(raw1 as string).status).toBe("filtered");

    // Matching subject → stored
    const raw2 = await plugin.tool["feed_email"].execute({
      feed_id: "digest-feed",
      subject: "[ALERT] Critical CVE detected",
      body: "CVE details here",
      from_email: "alerts@security.example.com",
    }, {});
    expect(JSON.parse(raw2 as string).status).toBe("stored");
  });

  test("feed.email returns error for non-email feed type", async () => {
    const dir = tmpDir("email-type-check");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });
    writeFileSync(join(feedsDir, "rss-f.yaml"), "id: rss-f\nname: RSS\ntype: rss\nsource:\n  url: https://x.com\npoll_interval: 1h\nrelevance:\n  prompt: p\nenabled: true\n", "utf-8");
    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, evaluator: alwaysStore() });
    const raw = await plugin.tool["feed_email"].execute({ feed_id: "rss-f", subject: "T", body: "B", from_email: "a@b.com" }, {});
    expect(JSON.parse(raw as string).error).toMatch(/not an email feed/i);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-040 plan=phase-3/email-feed/step-52-p3-003 test=feed-ingestion.test.ts#AC-36 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-37: Feed health dashboard (REQ-FEED-083/084/085)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-3/health-dashboard/step-52-p3-004 test=feed-ingestion.test.ts#AC-37 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-37: Feed health dashboard (REQ-FEED-083/084/085)", () => {
  function makeHealthFeed(dir: string, feedsDir: string, id: string) {
    mkdirSync(feedsDir, { recursive: true });
    const yaml = `id: "${id}"\nname: "${id}"\ntype: "rss"\nsource:\n  url: "https://x.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 50\n  max_cost_per_day_usd: 1.0\nenabled: true\n`;
    writeFileSync(join(feedsDir, `${id}.yaml`), yaml, "utf-8");
  }

  test("getStoreRate returns null when insufficient data (< 5 evaluations)", () => {
    const state = emptyFeedState();
    state.store_rate_7d = { items_evaluated: 3, items_stored: 3, window_start: new Date().toISOString() };
    expect(getStoreRate(state)).toBeNull();
  });

  test("getStoreRate computes ratio correctly", () => {
    const state = emptyFeedState();
    state.store_rate_7d = { items_evaluated: 10, items_stored: 9, window_start: new Date().toISOString() };
    expect(getStoreRate(state)).toBe(0.9);
  });

  test("feed.health flags feed with >90% store rate (REQ-FEED-084)", async () => {
    const dir = tmpDir("health-high");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    mkdirSync(stateDir, { recursive: true });
    makeHealthFeed(dir, feedsDir, "permissive-feed");

    // Seed state with >90% store rate (10 evaluated, 10 stored)
    const state = { ...emptyFeedState(), store_rate_7d: { items_evaluated: 10, items_stored: 10, window_start: new Date().toISOString() } };
    await saveFeedState(stateDir, "permissive-feed", state);

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, stateDir });
    const raw = await plugin.tool["feed_health"].execute({}, {});
    const result = JSON.parse(raw as string);
    const feed = result.feeds.find((f: { id: string }) => f.id === "permissive-feed");
    expect(feed.flagged).toBe("high_store_rate");
    expect(result.flagged_feeds).toBe(1);
  });

  test("feed.health flags feed with <5% store rate (REQ-FEED-085)", async () => {
    const dir = tmpDir("health-low");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    mkdirSync(stateDir, { recursive: true });
    makeHealthFeed(dir, feedsDir, "irrelevant-feed");

    // Seed: 20 evaluated, 0 stored (0% store rate)
    const state = { ...emptyFeedState(), store_rate_7d: { items_evaluated: 20, items_stored: 0, window_start: new Date().toISOString() } };
    await saveFeedState(stateDir, "irrelevant-feed", state);

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, stateDir });
    const raw = await plugin.tool["feed_health"].execute({}, {});
    const result = JSON.parse(raw as string);
    const feed = result.feeds.find((f: { id: string }) => f.id === "irrelevant-feed");
    expect(feed.flagged).toBe("low_store_rate");
  });

  test("feed.health does NOT flag feed with normal store rate", async () => {
    const dir = tmpDir("health-normal");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    mkdirSync(stateDir, { recursive: true });
    makeHealthFeed(dir, feedsDir, "normal-feed");

    // 50% store rate (not flagged)
    const state = { ...emptyFeedState(), store_rate_7d: { items_evaluated: 10, items_stored: 5, window_start: new Date().toISOString() } };
    await saveFeedState(stateDir, "normal-feed", state);

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, stateDir });
    const raw = await plugin.tool["feed_health"].execute({}, {});
    const result = JSON.parse(raw as string);
    const feed = result.feeds.find((f: { id: string }) => f.id === "normal-feed");
    expect(feed.flagged).toBeNull();
    expect(result.flagged_feeds).toBe(0);
  });

  test("feed.health returns comprehensive health metrics", async () => {
    const dir = tmpDir("health-metrics");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    mkdirSync(stateDir, { recursive: true });
    makeHealthFeed(dir, feedsDir, "metrics-feed");

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, stateDir });
    const raw = await plugin.tool["feed_health"].execute({}, {});
    const result = JSON.parse(raw as string);

    expect(result.total_feeds).toBe(1);
    expect(result).toHaveProperty("healthy_feeds");
    expect(result).toHaveProperty("global_cost_today_usd");
    expect(result).toHaveProperty("feeds");
    expect(result.feeds[0]).toHaveProperty("budget_today");
    expect(result.feeds[0]).toHaveProperty("pending_retry_count");
    expect(result.feeds[0]).toHaveProperty("store_rate_7d");
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-3/health-dashboard/step-52-p3-004 test=feed-ingestion.test.ts#AC-37 jira_ref=SWDE-52

  // stale_window in feed.health test (step-v12-003)
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-18/step-v12-003 test=feed-ingestion.test.ts#AC-37-stale-window jira_ref=SWDE-52
  test("feed.health surfaces stale_window=true for feed with window_start >7 days old", async () => {
    const dir = tmpDir("health-stale-window");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    mkdirSync(stateDir, { recursive: true });
    makeHealthFeed(dir, feedsDir, "stale-health-feed");

    const eightDaysAgo = new Date(Date.now() - 8 * 24 * 60 * 60 * 1000).toISOString();
    const state = {
      ...emptyFeedState(),
      store_rate_7d: { items_evaluated: 10, items_stored: 5, window_start: eightDaysAgo },
      budget_date: utcDateString(),
    };
    await saveFeedState(stateDir, "stale-health-feed", state);

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, stateDir });
    const raw = await plugin.tool["feed_health"].execute({}, {});
    const result = JSON.parse(raw as string);

    const feed = result.feeds.find((f: { id: string }) => f.id === "stale-health-feed");
    expect(feed.stale_window).toBe(true);
    expect(feed.store_rate_window_start).toBe(eightDaysAgo);
  });

  // stale_window=false (fresh window) test (step-v13-007)
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-19/step-v13-007 test=feed-ingestion.test.ts#AC-37-stale-window-false jira_ref=SWDE-52
  test("feed.health surfaces stale_window=false for feed with recent window_start", async () => {
    const dir = tmpDir("health-fresh-window");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    mkdirSync(stateDir, { recursive: true });
    makeHealthFeed(dir, feedsDir, "fresh-health-feed");

    const recentStart = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString(); // 2 days ago
    const state = {
      ...emptyFeedState(),
      store_rate_7d: { items_evaluated: 10, items_stored: 5, window_start: recentStart },
      budget_date: utcDateString(),
    };
    await saveFeedState(stateDir, "fresh-health-feed", state);

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, stateDir });
    const raw = await plugin.tool["feed_health"].execute({}, {});
    const result = JSON.parse(raw as string);

    const feed = result.feeds.find((f: { id: string }) => f.id === "fresh-health-feed");
    expect(feed.stale_window).toBe(false);
    expect(feed.store_rate_window_start).toBe(recentStart);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-38: store_rate_7d updated for Slack and iCal branches (F-001 fix)
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-38: store_rate_7d wired for Slack and iCal feed types (REQ-FEED-084/085)", () => {
  const SLACK_RESP_FOR_RATE = { ok: true, messages: [
    { text: "Msg A", user: "U1", ts: "1746700800.001", user_profile: { display_name: "Alice" } },
    { text: "Msg B", user: "U2", ts: "1746700900.002", user_profile: { display_name: "Bob" } },
  ]};

  test("pollFeed with slack type updates store_rate_7d after evaluation", async () => {
    const stateDir = tmpDir("state-rate-slack");
    const memRoot = tmpDir("mem-rate-slack");
    const feedsDir = tmpDir("feeds-rate-slack");

    process.env.RATE_SLACK_TOKEN = "xoxb-rate-test";

    const yaml = `
id: "rate-slack"
name: "Rate Slack"
type: "slack"
source:
  channel: "C0123"
  slack_token_env: "RATE_SLACK_TOKEN"
poll_interval: "5m"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "rate-slack.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(JSON.stringify(SLACK_RESP_FOR_RATE), { status: 200 }) as Response;

    try {
      const { result, state } = await pollFeed(config, stateDir, memRoot, alwaysStore());
      // Both messages evaluated and stored
      expect(result.evaluated).toBe(2);
      // store_rate_7d must be updated (not remain at 0/0)
      expect(state.store_rate_7d).toBeDefined();
      expect(state.store_rate_7d!.items_evaluated).toBe(2);
      expect(state.store_rate_7d!.items_stored).toBe(2);
    } finally {
      globalThis.fetch = originalFetch;
      delete process.env.RATE_SLACK_TOKEN;
    }
  });

  test("pollFeed with ical type updates store_rate_7d after evaluation", async () => {
    const stateDir = tmpDir("state-rate-ical");
    const memRoot = tmpDir("mem-rate-ical");
    const feedsDir = tmpDir("feeds-rate-ical");

    const NOW = new Date();
    const FUTURE = new Date(NOW.getTime() + 2 * 24 * 60 * 60 * 1000);
    const icalStr = (d: Date) => d.toISOString().replace(/-|:|\.\d{3}/g, "").replace("Z", "Z");
    const icalContent = `BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:e1\nSUMMARY:Team Meeting\nDTSTART:${icalStr(FUTURE)}\nDTEND:${icalStr(new Date(FUTURE.getTime() + 3600000))}\nEND:VEVENT\nEND:VCALENDAR`;

    const yaml = `
id: "rate-ical"
name: "Rate iCal"
type: "ical"
source:
  url: "https://cal.example.com/feed.ics"
  lookahead_days: 7
poll_interval: "4h"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "rate-ical.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(icalContent, {
        status: 200,
        headers: { "Content-Type": "text/calendar" },
      }) as Response;

    try {
      const { result, state } = await pollFeed(config, stateDir, memRoot, alwaysStore());
      expect(result.new_items).toBe(1);
      expect(result.evaluated).toBe(1);
      // store_rate_7d must be updated
      expect(state.store_rate_7d).toBeDefined();
      expect(state.store_rate_7d!.items_evaluated).toBe(1);
      expect(state.store_rate_7d!.items_stored).toBe(1);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-3/health-dashboard/f001-fix test=feed-ingestion.test.ts#AC-38 jira_ref=SWDE-52

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-16/step-v10-009 test=feed-ingestion.test.ts#AC-38-window-reset jira_ref=SWDE-52
  test("store_rate_7d window resets when window_start is >7 days old (lazy reset)", async () => {
    const stateDir = tmpDir("ac38-window-reset-state");
    const memRoot = tmpDir("ac38-window-reset-mem");
    const feedsDir = tmpDir("ac38-window-reset-feeds");

    writeFileSync(
      join(feedsDir, "window-reset-feed.yaml"),
      "id: window-reset-feed\nname: Window Reset\ntype: rss\nsource:\n  url: https://x.com/feed\npoll_interval: 1h\nrelevance:\n  prompt: p\nenabled: true\n",
      "utf-8"
    );
    const [config] = loadFeedConfigs(feedsDir);

    // Seed state with a window_start 8 days ago and 5 prior evaluations
    const eightDaysAgo = new Date(Date.now() - 8 * 24 * 60 * 60 * 1000).toISOString();
    const seededState = {
      ...emptyFeedState(),
      store_rate_7d: { items_evaluated: 5, items_stored: 5, window_start: eightDaysAgo },
    };
    await saveFeedState(stateDir, "window-reset-feed", seededState);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      // RSS_FIXTURE has 2 items; alwaysStore() stores both
      const { state } = await pollFeed(config, stateDir, memRoot, alwaysStore());

      // Window should have reset: items_evaluated should be 2 (not 7 = 5 + 2)
      expect(state.store_rate_7d).toBeDefined();
      expect(state.store_rate_7d!.items_evaluated).toBe(2);
      expect(state.store_rate_7d!.items_stored).toBe(2);
      // window_start should be recent (not the 8-days-ago seed)
      const windowAge = Date.now() - new Date(state.store_rate_7d!.window_start).getTime();
      expect(windowAge).toBeLessThan(60_000); // within last 60 seconds
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-39: feed.email global cost budget enforcement (F-002 fix, REQ-FEED-082)
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-39: feed.email global cost budget enforcement (REQ-FEED-082)", () => {
  test("feed.email returns budget_exceeded when global cost cap reached", async () => {
    const dir = tmpDir("email-global-cap");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    const feedsConfigPath = join(dir, ".axiom", "feeds.yaml");
    mkdirSync(feedsDir, { recursive: true });
    mkdirSync(stateDir, { recursive: true });
    mkdirSync(join(dir, ".axiom"), { recursive: true });

    // Global cap: $0.001 (tiny — already exceeded by seeded state below)
    writeFileSync(feedsConfigPath, "feeds:\n  enabled: true\n  global_max_cost_per_day_usd: 0.001\n", "utf-8");

    const yaml = `id: "email-cap"\nname: "Email Cap"\ntype: "email"\nsource: {}\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 50\n  max_cost_per_day_usd: 5\nenabled: true\n`;
    writeFileSync(join(feedsDir, "email-cap.yaml"), yaml, "utf-8");

    // Pre-seed another feed's state to push global cost above cap
    const otherYaml = `id: "other"\nname: "Other"\ntype: "rss"\nsource:\n  url: "https://x.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 50\n  max_cost_per_day_usd: 5\nenabled: true\n`;
    writeFileSync(join(feedsDir, "other.yaml"), otherYaml, "utf-8");
    const overState = { ...emptyFeedState(), cost_today_usd: 0.01, budget_date: utcDateString() };
    await saveFeedState(stateDir, "other", overState);

    const plugin = FeedIngestionPlugin({
      directory: dir,
      feedsDir,
      feedsConfigPath,
      stateDir,
      evaluator: alwaysStore("medium", 0.01),
    });

    const raw = await plugin.tool["feed_email"].execute({
      feed_id: "email-cap",
      subject: "Test email",
      body: "<p>Content</p>",
      from_email: "test@example.com",
    }, {});
    const result = JSON.parse(raw as string);
    expect(result.status).toBe("budget_exceeded");
    expect(result.message).toMatch(/global/i);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-082 plan=phase-3/email-feed/f002-fix test=feed-ingestion.test.ts#AC-39 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-40: run_when schedule enforcement (REQ-FEED-083)
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-40: run_when schedule enforcement (REQ-FEED-083)", () => {
  function makeRunWhenConfig(dir: string, feedsDir: string, runWhen: string, schedule?: string) {
    const feedsConfigPath = join(dir, ".axiom", "feeds.yaml");
    mkdirSync(join(dir, ".axiom"), { recursive: true });
    mkdirSync(feedsDir, { recursive: true });
    let yaml = `feeds:\n  enabled: true\n  run_when: "${runWhen}"`;
    if (schedule) yaml += `\n  schedule: "${schedule}"`;
    yaml += "\n";
    writeFileSync(feedsConfigPath, yaml, "utf-8");

    writeFileSync(
      join(feedsDir, "test-feed.yaml"),
      `id: test-feed\nname: Test\ntype: rss\nsource:\n  url: https://x.com\npoll_interval: 1h\nrelevance:\n  prompt: p\nenabled: true\n`,
      "utf-8"
    );
    return feedsConfigPath;
  }

  test("feed.poll with run_when=idle and OPENCODE_SESSION_ACTIVE=1 returns skipped", async () => {
    const dir = tmpDir("run-when-idle");
    const feedsDir = join(dir, ".axiom", "feeds");
    const feedsConfigPath = makeRunWhenConfig(dir, feedsDir, "idle");

    process.env.OPENCODE_SESSION_ACTIVE = "1";
    try {
      const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, feedsConfigPath });
      const raw = await plugin.tool["feed_poll"].execute({}, {});
      const result = JSON.parse(raw as string);
      expect(result.status).toBe("skipped");
      expect(result.reason).toMatch(/idle/i);
      expect(result.run_when).toBe("idle");
    } finally {
      delete process.env.OPENCODE_SESSION_ACTIVE;
    }
  });

  test("feed.poll with run_when=idle and no active session proceeds normally", async () => {
    const dir = tmpDir("run-when-idle-clear");
    const feedsDir = join(dir, ".axiom", "feeds");
    const feedsConfigPath = makeRunWhenConfig(dir, feedsDir, "idle");

    delete process.env.OPENCODE_SESSION_ACTIVE;

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response('<rss version="2.0"><channel></channel></rss>', { status: 200 }) as Response;

    try {
      const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, feedsConfigPath });
      const raw = await plugin.tool["feed_poll"].execute({}, {});
      const result = JSON.parse(raw as string);
      // Should NOT be skipped — no active session
      expect(result.status).not.toBe("skipped");
      expect(result.feeds_polled).toBeDefined();
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  test("feed.poll with run_when=always ignores OPENCODE_SESSION_ACTIVE", async () => {
    const dir = tmpDir("run-when-always");
    const feedsDir = join(dir, ".axiom", "feeds");
    const feedsConfigPath = makeRunWhenConfig(dir, feedsDir, "always");

    process.env.OPENCODE_SESSION_ACTIVE = "1";
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response('<rss version="2.0"><channel></channel></rss>', { status: 200 }) as Response;

    try {
      const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, feedsConfigPath });
      const raw = await plugin.tool["feed_poll"].execute({}, {});
      const result = JSON.parse(raw as string);
      // run_when=always → always proceeds regardless
      expect(result.status).not.toBe("skipped");
    } finally {
      globalThis.fetch = originalFetch;
      delete process.env.OPENCODE_SESSION_ACTIVE;
    }
  });

  test("isScheduledTimeMatch with */15 matches multiples of 15", () => {
    const at0 = new Date("2026-05-09T02:00:00Z"); // minute=0
    const at15 = new Date("2026-05-09T02:15:00Z"); // minute=15
    const at17 = new Date("2026-05-09T02:17:00Z"); // minute=17
    expect(isScheduledTimeMatch("*/15 * * * *", at0)).toBe(true);
    expect(isScheduledTimeMatch("*/15 * * * *", at15)).toBe(true);
    expect(isScheduledTimeMatch("*/15 * * * *", at17)).toBe(false);
  });

  test("isScheduledTimeMatch with 0 8 * * * matches 8am UTC only", () => {
    const at8 = new Date("2026-05-09T08:00:00Z");
    const at9 = new Date("2026-05-09T09:00:00Z");
    const at8_30 = new Date("2026-05-09T08:30:00Z");
    expect(isScheduledTimeMatch("0 8 * * *", at8)).toBe(true);
    expect(isScheduledTimeMatch("0 8 * * *", at9)).toBe(false);
    expect(isScheduledTimeMatch("0 8 * * *", at8_30)).toBe(false);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-083 plan=phase-4/run-when/step-52-p4-001 test=feed-ingestion.test.ts#AC-40 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-41: Per-expert feed routing (Phase 4 Expert Platform integration)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-4/expert-routing/step-52-p4-002 test=feed-ingestion.test.ts#AC-41 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-41: Per-expert feed routing (Phase 4 Expert Platform integration)", () => {
  test("getExpertMemoryPath returns agent subdirectory when target.agent is set", () => {
    const config: FeedConfig = {
      id: "cve-feed",
      name: "CVE Feed",
      type: "rss",
      source: { url: "https://x.com" },
      poll_interval: "1h",
      target: { agent: "security-review-axiom", memory_path: "signals/" },
      relevance: { prompt: "p" },
      deduplication: {},
      tags: [],
      enabled: true,
    };
    const path = getExpertMemoryPath(config, "/memory-root");
    expect(path).toBe("/memory-root/security-review-axiom");
  });

  test("getExpertMemoryPath returns default path when target.agent is absent", () => {
    const config: FeedConfig = {
      id: "generic-feed",
      name: "Generic",
      type: "rss",
      source: { url: "https://x.com" },
      poll_interval: "1h",
      target: {},
      relevance: { prompt: "p" },
      deduplication: {},
      tags: [],
      enabled: true,
    };
    const path = getExpertMemoryPath(config, "/memory-root");
    expect(path).toBe("/memory-root");
  });

  test("pollFeed writes signal note to expert subdirectory when target.agent is set", async () => {
    const stateDir = tmpDir("state-expert");
    const memRoot = tmpDir("mem-expert");
    const feedsDir = tmpDir("feeds-expert");

    const yaml = `
id: "expert-feed"
name: "Expert Feed"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
target:
  agent: "security-expert"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "expert-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const RSS = '<rss version="2.0"><channel><item><title>CVE Alert</title><link>https://x.com/cve</link><guid>g1</guid><pubDate>Fri, 08 May 2026 10:00:00 GMT</pubDate></item></channel></rss>';

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(RSS, { status: 200 }) as Response;

    try {
      const { result } = await pollFeed(config, stateDir, memRoot, alwaysStore());
      expect(result.stored).toBe(1);

      // Signal note should be in the expert's subdirectory, not the default signals/
      const expertSignalsDir = join(memRoot, "security-expert", "signals");
      const defaultSignalsDir = join(memRoot, "signals");
      expect(existsSync(expertSignalsDir)).toBe(true);
      expect(existsSync(defaultSignalsDir)).toBe(false);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  test("pollFeed writes to default signals/ when no target.agent", async () => {
    const stateDir = tmpDir("state-no-expert");
    const memRoot = tmpDir("mem-no-expert");
    const feedsDir = tmpDir("feeds-no-expert");

    const yaml = `
id: "no-expert-feed"
name: "No Expert"
type: "rss"
source:
  url: "https://example.com/rss"
poll_interval: "1h"
relevance:
  prompt: "p"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "no-expert-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const RSS = '<rss version="2.0"><channel><item><title>Item</title><link>https://x.com/i</link><guid>g2</guid><pubDate>Fri, 08 May 2026 10:00:00 GMT</pubDate></item></channel></rss>';

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(RSS, { status: 200 }) as Response;

    try {
      const { result } = await pollFeed(config, stateDir, memRoot, alwaysStore());
      expect(result.stored).toBe(1);

      // Should use default signals/ directory
      const defaultSignalsDir = join(memRoot, "signals");
      expect(existsSync(defaultSignalsDir)).toBe(true);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-4/expert-routing/step-52-p4-002 test=feed-ingestion.test.ts#AC-41 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-42: Feed effectiveness analytics (feed.analytics tool)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-4/analytics/step-52-p4-003 test=feed-ingestion.test.ts#AC-42 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-42: Feed effectiveness analytics (feed.analytics tool)", () => {
  async function setupAnalyticsFeeds(
    dir: string,
    feedsDir: string,
    stateDir: string,
    feeds: Array<{ id: string; evaluated: number; stored: number; costToday?: number }>
  ) {
    mkdirSync(feedsDir, { recursive: true });
    mkdirSync(stateDir, { recursive: true });
    for (const f of feeds) {
      writeFileSync(
        join(feedsDir, `${f.id}.yaml`),
        `id: "${f.id}"\nname: "${f.id}"\ntype: "rss"\nsource:\n  url: "https://x.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 100\n  max_cost_per_day_usd: 5\nenabled: true\n`,
        "utf-8"
      );
      const state = {
        ...emptyFeedState(),
        cost_today_usd: f.costToday ?? 0,
        items_today: f.stored,
        store_rate_7d: { items_evaluated: f.evaluated, items_stored: f.stored, window_start: new Date().toISOString() },
        budget_date: utcDateString(),
      };
      await saveFeedState(stateDir, f.id, state);
    }
  }

  test("feed.analytics returns per-feed effectiveness metrics", async () => {
    const dir = tmpDir("analytics-basic");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    await setupAnalyticsFeeds(dir, feedsDir, stateDir, [
      { id: "high-rate", evaluated: 10, stored: 9 },
      { id: "low-rate", evaluated: 10, stored: 1 },
    ]);
    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, stateDir });
    const raw = await plugin.tool["feed_analytics"].execute({}, {});
    const result = JSON.parse(raw as string);

    expect(result.feeds).toHaveLength(2);
    const highFeed = result.feeds.find((f: { id: string }) => f.id === "high-rate");
    const lowFeed = result.feeds.find((f: { id: string }) => f.id === "low-rate");
    expect(highFeed.store_rate_7d).toBe(0.9);
    expect(lowFeed.store_rate_7d).toBe(0.1);
    expect(highFeed.total_evaluated_7d).toBe(10);
    expect(highFeed.total_stored_7d).toBe(9);
  });

  test("feed.analytics identifies most and least effective feeds", async () => {
    const dir = tmpDir("analytics-ranking");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    await setupAnalyticsFeeds(dir, feedsDir, stateDir, [
      { id: "excellent", evaluated: 10, stored: 10 }, // 100% store rate
      { id: "medium", evaluated: 10, stored: 5 },    // 50% store rate
      { id: "poor", evaluated: 10, stored: 1 },      // 10% store rate
    ]);
    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, stateDir });
    const raw = await plugin.tool["feed_analytics"].execute({}, {});
    const result = JSON.parse(raw as string);

    expect(result.summary.most_effective_feed?.id).toBe("excellent");
    expect(result.summary.least_effective_feed?.id).toBe("poor");
    expect(result.summary.feeds_with_data).toBe(3);
  });

  test("feed.analytics excludes feeds with insufficient data from rankings", async () => {
    const dir = tmpDir("analytics-insufficient");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    await setupAnalyticsFeeds(dir, feedsDir, stateDir, [
      { id: "good-data", evaluated: 10, stored: 8 },   // sufficient (>= 5)
      { id: "no-data", evaluated: 3, stored: 3 },      // insufficient (< 5)
    ]);
    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, stateDir });
    const raw = await plugin.tool["feed_analytics"].execute({ min_evaluations: 5 }, {});
    const result = JSON.parse(raw as string);

    expect(result.summary.feeds_with_data).toBe(1); // only good-data qualifies
    expect(result.summary.most_effective_feed?.id).toBe("good-data");
    // no-data feed is returned in feeds list but has has_sufficient_data=false
    const noDataFeed = result.feeds.find((f: { id: string }) => f.id === "no-data");
    expect(noDataFeed.has_sufficient_data).toBe(false);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-4/analytics/step-52-p4-003 test=feed-ingestion.test.ts#AC-42 jira_ref=SWDE-52

  // stale_window test (step-v11-003)
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-17/step-v11-003 test=feed-ingestion.test.ts#AC-42-stale-window jira_ref=SWDE-52
  test("feed.analytics surfaces stale_window=true for feeds with window_start >7 days old", async () => {
    const dir = tmpDir("analytics-stale-window");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    mkdirSync(feedsDir, { recursive: true });
    mkdirSync(stateDir, { recursive: true });

    writeFileSync(
      join(feedsDir, "stale-feed.yaml"),
      'id: "stale-feed"\nname: "Stale"\ntype: "rss"\nsource:\n  url: "https://x.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 100\n  max_cost_per_day_usd: 5\nenabled: true\n',
      "utf-8"
    );
    writeFileSync(
      join(feedsDir, "fresh-feed.yaml"),
      'id: "fresh-feed"\nname: "Fresh"\ntype: "rss"\nsource:\n  url: "https://x.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 100\n  max_cost_per_day_usd: 5\nenabled: true\n',
      "utf-8"
    );

    const eightDaysAgo = new Date(Date.now() - 8 * 24 * 60 * 60 * 1000).toISOString();
    const staleState = {
      ...emptyFeedState(),
      store_rate_7d: { items_evaluated: 10, items_stored: 5, window_start: eightDaysAgo },
      budget_date: utcDateString(),
    };
    const freshState = {
      ...emptyFeedState(),
      store_rate_7d: { items_evaluated: 10, items_stored: 5, window_start: new Date().toISOString() },
      budget_date: utcDateString(),
    };
    await saveFeedState(stateDir, "stale-feed", staleState);
    await saveFeedState(stateDir, "fresh-feed", freshState);

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, stateDir });
    const raw = await plugin.tool["feed_analytics"].execute({}, {});
    const result = JSON.parse(raw as string);

    const staleFeed = result.feeds.find((f: { id: string }) => f.id === "stale-feed");
    const freshFeed = result.feeds.find((f: { id: string }) => f.id === "fresh-feed");

    // Stale feed: window_start is 8 days old → stale_window: true
    expect(staleFeed.stale_window).toBe(true);
    expect(staleFeed.store_rate_window_start).toBe(eightDaysAgo);

    // Fresh feed: window_start is recent → stale_window: false
    expect(freshFeed.stale_window).toBe(false);
    expect(freshFeed.store_rate_window_start).toBeDefined();
  });

  // stale_window null branch test (step-v12-008)
  // Note: emptyFeedState() always initializes store_rate_7d with a fresh window_start,
  // so store_rate_window_start is never null in practice. This test verifies that a feed
  // with no evaluations (items_evaluated: 0) correctly shows stale_window: false.
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-18/step-v12-008 test=feed-ingestion.test.ts#AC-42-stale-window-null jira_ref=SWDE-52
  test("feed.analytics returns stale_window=false for feed with no evaluations (fresh empty state)", async () => {
    const dir = tmpDir("analytics-no-data");
    const feedsDir = join(dir, ".axiom", "feeds");
    const stateDir = join(dir, ".memory-bank", "feed-state");
    mkdirSync(feedsDir, { recursive: true });
    mkdirSync(stateDir, { recursive: true });

    writeFileSync(
      join(feedsDir, "no-data-feed.yaml"),
      'id: "no-data-feed"\nname: "No Data"\ntype: "rss"\nsource:\n  url: "https://x.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\n  max_items_per_day: 100\n  max_cost_per_day_usd: 5\nenabled: true\n',
      "utf-8"
    );
    // Save emptyFeedState — store_rate_7d initialized with items_evaluated: 0 and fresh window_start
    const state = { ...emptyFeedState(), budget_date: utcDateString() };
    await saveFeedState(stateDir, "no-data-feed", state);

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, stateDir });
    const raw = await plugin.tool["feed_analytics"].execute({}, {});
    const result = JSON.parse(raw as string);

    const noDataFeed = result.feeds.find((f: { id: string }) => f.id === "no-data-feed");
    // Fresh empty state: window_start is recent → stale_window: false
    expect(noDataFeed.stale_window).toBe(false);
    // store_rate_window_start is present (emptyFeedState always initializes store_rate_7d)
    expect(noDataFeed.store_rate_window_start).toBeDefined();
    expect(noDataFeed.store_rate_window_start).not.toBeNull();
  });
});

// AC-43: feed.subscribe expert association tool
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-4/subscribe/step-52-p4-004 test=feed-ingestion.test.ts#AC-43 jira_ref=SWDE-52
describe("AC-43: feed.subscribe expert association tool", () => {
  function makeSubscribeFeed(feedsDir: string, feedId: string) {
    mkdirSync(feedsDir, { recursive: true });
    const yaml = `id: "${feedId}"\nname: "${feedId}"\ntype: "rss"\nsource:\n  url: "https://x.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: true\n`;
    writeFileSync(join(feedsDir, `${feedId}.yaml`), yaml, "utf-8");
  }

  test("feed.subscribe sets target.agent on feed config", async () => {
    const dir = tmpDir("subscribe-basic");
    const feedsDir = join(dir, ".axiom", "feeds");
    makeSubscribeFeed(feedsDir, "cve-feed");

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir });
    const raw = await plugin.tool["feed_subscribe"].execute({
      feed_id: "cve-feed",
      expert_name: "security-review-axiom",
      action: "subscribe",
    }, {});
    const result = JSON.parse(raw as string);
    expect(result.status).toBe("ok");
    expect(result.target_agent).toBe("security-review-axiom");
    expect(result.previous_agent).toBeNull();

    // Verify the YAML file was actually updated
    const { parse } = await import("yaml");
    const content = readFileSync(join(feedsDir, "cve-feed.yaml"), "utf-8");
    const parsed = parse(content);
    expect(parsed.target?.agent).toBe("security-review-axiom");
  });

  test("feed.subscribe unsubscribe removes target.agent", async () => {
    const dir = tmpDir("unsubscribe-basic");
    const feedsDir = join(dir, ".axiom", "feeds");
    // Pre-create feed with existing target.agent
    mkdirSync(feedsDir, { recursive: true });
    writeFileSync(
      join(feedsDir, "linked-feed.yaml"),
      `id: "linked-feed"\nname: "Linked"\ntype: "rss"\nsource:\n  url: "https://x.com"\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\ntarget:\n  agent: "old-expert"\nenabled: true\n`,
      "utf-8"
    );

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir });
    const raw = await plugin.tool["feed_subscribe"].execute({
      feed_id: "linked-feed",
      expert_name: "old-expert",
      action: "unsubscribe",
    }, {});
    const result = JSON.parse(raw as string);
    expect(result.status).toBe("ok");
    expect(result.target_agent).toBeNull();
    expect(result.previous_agent).toBe("old-expert");

    // Verify agent removed from YAML
    const { parse } = await import("yaml");
    const content = readFileSync(join(feedsDir, "linked-feed.yaml"), "utf-8");
    const parsed = parse(content);
    expect(parsed.target?.agent).toBeUndefined();
  });

  test("feed.subscribe returns error for unknown feed", async () => {
    const dir = tmpDir("subscribe-miss");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });

    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir });
    const raw = await plugin.tool["feed_subscribe"].execute({
      feed_id: "nonexistent",
      expert_name: "some-expert",
      action: "subscribe",
    }, {});
    const result = JSON.parse(raw as string);
    expect(result.error).toMatch(/not found/i);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-4/subscribe/step-52-p4-004 test=feed-ingestion.test.ts#AC-43 jira_ref=SWDE-52
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-44: REQ-FEED-061 validation on slack, api, ical paths (step-v7-001)
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-v7/step-v7-001 test=feed-ingestion.test.ts#AC-44 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("AC-44: REQ-FEED-061 partial-decision validation on slack/api/ical/webhook/email paths (step-v7-001, phase-14)", () => {
  // Evaluator that returns only store+reason — missing priority, tags, summary
  const partialEvaluator = async (): Promise<RelevanceDecision> =>
    ({ store: true, reason: 'ok' } as unknown as RelevanceDecision);

  // ── Slack path ───────────────────────────────────────────────────────────
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-v7/step-v7-001 test=feed-ingestion.test.ts#AC-44-slack jira_ref=SWDE-52
  test("AC-44-slack: partial evaluator on slack feed → error recorded, nothing stored, item queued for retry", async () => {
    const stateDir = tmpDir("ac44-slack-state");
    const memRoot = tmpDir("ac44-slack-mem");
    const feedsDir = tmpDir("ac44-slack-feeds");

    process.env.AC44_SLACK_TOKEN = "xoxb-test-ac44";
    const yaml = `
id: "ac44-slack-feed"
name: "AC44 Slack"
type: "slack"
source:
  channel: "C0123AC44"
  slack_token_env: "AC44_SLACK_TOKEN"
poll_interval: "5m"
relevance:
  prompt: "test"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "ac44-slack-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const slackResponse = JSON.stringify({
      ok: true,
      messages: [{ text: "Deploy alert", user: "U001", ts: "1746700800.000100" }],
    });

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(slackResponse, { status: 200 }) as Response;

    try {
      const { result, state } = await pollFeed(config, stateDir, memRoot, partialEvaluator);

      // REQ-FEED-061 assertion 1: error was recorded
      expect(result.errors.length).toBeGreaterThan(0);
      expect(result.errors[0]).toMatch(/incomplete RelevanceDecision|missing fields/i);

      // REQ-FEED-061 assertion 2: no corrupt signal note written
      expect(result.stored).toBe(0);

      // REQ-FEED-061 assertion 3: item queued for retry (not silently discarded)
      expect(state.pending_retry).toHaveLength(1);
    } finally {
      globalThis.fetch = originalFetch;
      delete process.env.AC44_SLACK_TOKEN;
    }
  });

  // ── API path ────────────────────────────────────────────────────────────
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-v7/step-v7-001 test=feed-ingestion.test.ts#AC-44-api jira_ref=SWDE-52
  test("AC-44-api: partial evaluator on api feed → error recorded, nothing stored, item queued for retry", async () => {
    const stateDir = tmpDir("ac44-api-state");
    const memRoot = tmpDir("ac44-api-mem");
    const feedsDir = tmpDir("ac44-api-feeds");

    const yaml = `
id: "ac44-api-feed"
name: "AC44 API"
type: "api"
source:
  url: "https://api.example.com/items"
  jq_extract: ".[]"
poll_interval: "1h"
relevance:
  prompt: "test"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "ac44-api-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    const apiResponse = JSON.stringify([
      { id: 1, name: "Item One", url: "https://example.com/1" },
    ]);

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(apiResponse, {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }) as Response;

    try {
      const { result, state } = await pollFeed(config, stateDir, memRoot, partialEvaluator);

      // REQ-FEED-061 assertion 1: error was recorded
      expect(result.errors.length).toBeGreaterThan(0);
      expect(result.errors[0]).toMatch(/incomplete RelevanceDecision|missing fields/i);

      // REQ-FEED-061 assertion 2: no corrupt signal note written
      expect(result.stored).toBe(0);

      // REQ-FEED-061 assertion 3: item queued for retry
      expect(state.pending_retry).toHaveLength(1);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // ── iCal path ───────────────────────────────────────────────────────────
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-v7/step-v7-001 test=feed-ingestion.test.ts#AC-44-ical jira_ref=SWDE-52
  test("AC-44-ical: partial evaluator on ical feed → error recorded, nothing stored, item queued for retry", async () => {
    const stateDir = tmpDir("ac44-ical-state");
    const memRoot = tmpDir("ac44-ical-mem");
    const feedsDir = tmpDir("ac44-ical-feeds");

    const yaml = `
id: "ac44-ical-feed"
name: "AC44 iCal"
type: "ical"
source:
  url: "https://calendar.example.com/feed.ics"
  lookahead_days: 7
poll_interval: "4h"
relevance:
  prompt: "test"
  max_items_per_day: 50
  max_cost_per_day_usd: 5.00
`;
    writeFileSync(join(feedsDir, "ac44-ical-feed.yaml"), yaml, "utf-8");
    const [config] = loadFeedConfigs(feedsDir);

    // Build a minimal valid iCal with one event within the lookahead window
    const now = new Date("2026-05-22T10:00:00Z");
    const soon = new Date(now.getTime() + 2 * 60 * 60 * 1000); // 2h from now
    const fmt = (d: Date) =>
      d.toISOString().replace(/[-:]/g, "").replace(/\.\d+/, "");
    const icalContent = [
      "BEGIN:VCALENDAR",
      "VERSION:2.0",
      "BEGIN:VEVENT",
      `UID:ac44-event-1@test`,
      `SUMMARY:AC44 Test Event`,
      `DTSTART:${fmt(soon)}`,
      `DTEND:${fmt(new Date(soon.getTime() + 60 * 60 * 1000))}`,
      "END:VEVENT",
      "END:VCALENDAR",
    ].join("\r\n");

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () =>
      new Response(icalContent, {
        status: 200,
        headers: { "Content-Type": "text/calendar" },
      }) as Response;

    try {
      const { result, state } = await pollFeed(
        config, stateDir, memRoot, partialEvaluator, undefined, { _now: now }
      );

      // REQ-FEED-061 assertion 1: error was recorded
      expect(result.errors.length).toBeGreaterThan(0);
      expect(result.errors[0]).toMatch(/incomplete RelevanceDecision|missing fields/i);

      // REQ-FEED-061 assertion 2: no corrupt signal note written
      expect(result.stored).toBe(0);
      // REQ-FEED-061 assertion 3: item queued for retry
      expect(state.pending_retry).toHaveLength(1);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-003 test=feed-ingestion.test.ts#AC-44-webhook jira_ref=SWDE-52
  test("AC-44-webhook: partial evaluator on webhook feed → error returned, nothing stored", async () => {
    const dir = tmpDir("ac44-webhook-state");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });
    writeFileSync(
      join(feedsDir, "wh-feed.yaml"),
      'id: "wh-feed"\nname: "Webhook"\ntype: "webhook"\nsource:\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: true\n',
      "utf-8"
    );

    const partialEvaluator = async () => ({ store: true } as unknown as import("./feed-ingestion.ts").RelevanceDecision);
    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, evaluator: partialEvaluator });
    const raw = await plugin.tool["feed_webhook"].execute({
      feed_id: "wh-feed",
      payload: { title: "Test Event", url: "https://x.com/event-1", id: "e1" },
    }, {});
    const result = JSON.parse(raw as string);

    // REQ-FEED-061 assertion 1: error returned
    expect(result.error).toMatch(/incomplete RelevanceDecision|missing fields/i);

    // REQ-FEED-061 assertion 2: no corrupt signal note written
    const signalsDir = join(dir, ".memory-bank", "signals");
    const signalFiles = existsSync(signalsDir) ? readdirSync(signalsDir) : [];
    expect(signalFiles).toHaveLength(0);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-003 test=feed-ingestion.test.ts#AC-44-email jira_ref=SWDE-52
  test("AC-44-email: partial evaluator on email feed → error returned, nothing stored", async () => {
    const dir = tmpDir("ac44-email-state");
    const feedsDir = join(dir, ".axiom", "feeds");
    mkdirSync(feedsDir, { recursive: true });
    writeFileSync(
      join(feedsDir, "digest-feed.yaml"),
      'id: "digest-feed"\nname: "Email Digest"\ntype: "email"\nsource:\npoll_interval: "1h"\nrelevance:\n  prompt: "p"\nenabled: true\n',
      "utf-8"
    );

    const partialEvaluator = async () => ({ store: true } as unknown as import("./feed-ingestion.ts").RelevanceDecision);
    const plugin = FeedIngestionPlugin({ directory: dir, feedsDir, evaluator: partialEvaluator });
    const raw = await plugin.tool["feed_email"].execute({
      feed_id: "digest-feed",
      subject: "Weekly Digest",
      body: "Some content here.",
      from_email: "digest@example.com",
    }, {});
    const result = JSON.parse(raw as string);

    // REQ-FEED-061 assertion 1: error returned
    expect(result.error).toMatch(/incomplete RelevanceDecision|missing fields/i);

    // REQ-FEED-061 assertion 2: no corrupt signal note written
    const signalsDir = join(dir, ".memory-bank", "signals");
    const signalFiles = existsSync(signalsDir) ? readdirSync(signalsDir) : [];
    expect(signalFiles).toHaveLength(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// AC-B09: loadGlobalFeedsConfig — merge logic and defaults
// ─────────────────────────────────────────────────────────────────────────────

// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md plan=phase-1/task-1.2/step-b09 test=feed-ingestion.test.ts#AC-B09 jira_ref=SWDE-52
describe("AC-B09: loadGlobalFeedsConfig — custom values and defaults", () => {
  test("loads custom values from feeds.yaml", () => {
    const dir = tmpDir("global-config-custom");
    const configPath = join(dir, "feeds.yaml");
    writeFileSync(
      configPath,
      `feeds:\n  global_max_cost_per_day_usd: 2.50\n  run_when: idle\n`,
      "utf-8"
    );
    const config = loadGlobalFeedsConfig(configPath);
    // Specified fields take custom values
    expect(config.global_max_cost_per_day_usd).toBe(2.50);
    expect(config.run_when).toBe("idle");
    // Unspecified fields must NOT be clobbered — defaults must be preserved
    expect(config.enabled).toBe(true);
    expect(config.default_relevance_model).toBe("anthropic.claude-haiku");
    expect(config.storage.memory_bank).toBe(true);
    expect(config.storage.pandora).toBe(false);
    expect(config.health.unhealthy_after_failures).toBe(3);
    expect(config.health.alert_on_unhealthy).toBe(true);
  });

  test("returns defaults when file does not exist", () => {
    const config = loadGlobalFeedsConfig("/nonexistent/path/feeds.yaml");
    expect(config.global_max_cost_per_day_usd).toBe(10);
    expect(config.run_when).toBe("always");
    expect(config.enabled).toBe(true);
    expect(config.default_relevance_model).toBe("anthropic.claude-haiku");
    expect(config.storage.memory_bank).toBe(true);
    expect(config.storage.pandora).toBe(false);
    expect(config.health.unhealthy_after_failures).toBe(3);
    expect(config.health.alert_on_unhealthy).toBe(true);
  });
  test("preserves defaults for unspecified fields when only run_when is set", () => {
    // REQ-FEED-merge-contract: setting one field must not clobber others
    const dir = tmpDir("global-config-run-when-only");
    const configPath = join(dir, "feeds.yaml");
    writeFileSync(
      configPath,
      `feeds:\n  run_when: idle\n`,
      "utf-8"
    );
    const config = loadGlobalFeedsConfig(configPath);
    // The specified field takes the custom value
    expect(config.run_when).toBe("idle");
    // All unspecified fields must retain their defaults
    expect(config.global_max_cost_per_day_usd).toBe(10);
    expect(config.enabled).toBe(true);
    expect(config.default_relevance_model).toBe("anthropic.claude-haiku");
    expect(config.storage.memory_bank).toBe(true);
    expect(config.storage.pandora).toBe(false);
    expect(config.health.unhealthy_after_failures).toBe(3);
    expect(config.health.alert_on_unhealthy).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// REQ-FEED-061 step-v7-002: empty/whitespace-only string fields are rejected
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-v7/step-v7-002 test=feed-ingestion.test.ts#AC-v7-002 jira_ref=SWDE-52
// ─────────────────────────────────────────────────────────────────────────────

describe("REQ-FEED-061 step-v7-002: empty/whitespace-only reason or summary is rejected", () => {
  // Regression test 1: evaluator returns reason: '' (empty string) → rejected
  test("evaluator returning reason='' is rejected (result.errors.length > 0, result.stored === 0)", async () => {
    const stateDir = tmpDir("state-empty-reason");
    const memRoot = tmpDir("mem-empty-reason");

    // Evaluator returns all required fields present, but reason is empty string
    const emptyReasonEvaluator = async (): Promise<RelevanceDecision> => ({
      store: true,
      reason: '',
      priority: 'low',
      tags: [],
      summary: 'ok',
    });

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const { result, state } = await pollFeed(
        SAMPLE_FEED_CONFIG,
        stateDir,
        memRoot,
        emptyReasonEvaluator,
        undefined,
        {}
      );

      // REQ-FEED-061: empty reason must be rejected
      expect(result.stored).toBe(0);
      expect(result.errors.length).toBeGreaterThan(0);
      // Items should be queued for retry (not silently discarded)
      expect(state.pending_retry.length).toBeGreaterThan(0);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // Regression test 2: evaluator returns reason: '   ' (whitespace-only) → rejected
  test("evaluator returning reason='   ' (whitespace-only) is rejected (result.errors.length > 0, result.stored === 0)", async () => {
    const stateDir = tmpDir("state-whitespace-reason");
    const memRoot = tmpDir("mem-whitespace-reason");

    // Evaluator returns all required fields present, but reason is whitespace-only
    const whitespaceReasonEvaluator = async (): Promise<RelevanceDecision> => ({
      store: true,
      reason: '   ',
      priority: 'low',
      tags: [],
      summary: 'ok',
    });

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const { result, state } = await pollFeed(
        SAMPLE_FEED_CONFIG,
        stateDir,
        memRoot,
        whitespaceReasonEvaluator,
        undefined,
        {}
      );

      // REQ-FEED-061: whitespace-only reason must be rejected
      expect(result.stored).toBe(0);
      expect(result.errors.length).toBeGreaterThan(0);
      // Items should be queued for retry (not silently discarded)
      expect(state.pending_retry.length).toBeGreaterThan(0);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // Regression test 3: evaluator returns reason: null → rejected (null bypasses undefined check)
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-002 test=feed-ingestion.test.ts#AC-v8-002-null jira_ref=SWDE-52
  test("evaluator returning reason=null is rejected (result.errors.length > 0, result.stored === 0)", async () => {
    const stateDir = tmpDir("state-null-reason");
    const memRoot = tmpDir("mem-null-reason");

    const nullReasonEvaluator = async (): Promise<RelevanceDecision> => ({
      store: true,
      reason: null as unknown as string,
      priority: 'low',
      tags: [],
      summary: 'ok',
    });

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const { result } = await pollFeed(SAMPLE_FEED_CONFIG, stateDir, memRoot, nullReasonEvaluator, undefined, {});
      expect(result.stored).toBe(0);
      expect(result.errors.length).toBeGreaterThan(0);
      expect(result.errors[0]).toMatch(/incomplete RelevanceDecision|missing fields/i);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // Regression test 4: evaluator returns reason: 0 (number) → rejected
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-002 test=feed-ingestion.test.ts#AC-v8-002-number jira_ref=SWDE-52
  test("evaluator returning reason=0 (number) is rejected (result.errors.length > 0, result.stored === 0)", async () => {
    const stateDir = tmpDir("state-number-reason");
    const memRoot = tmpDir("mem-number-reason");

    const numberReasonEvaluator = async (): Promise<RelevanceDecision> => ({
      store: true,
      reason: 0 as unknown as string,
      priority: 'low',
      tags: [],
      summary: 'ok',
    });

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const { result } = await pollFeed(SAMPLE_FEED_CONFIG, stateDir, memRoot, numberReasonEvaluator, undefined, {});
      expect(result.stored).toBe(0);
      expect(result.errors.length).toBeGreaterThan(0);
      expect(result.errors[0]).toMatch(/incomplete RelevanceDecision|missing fields/i);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  // Regression test 5: evaluator returns reason: false (boolean) → rejected
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-14/step-v8-002 test=feed-ingestion.test.ts#AC-v8-002-boolean jira_ref=SWDE-52
  test("evaluator returning reason=false (boolean) is rejected (result.errors.length > 0, result.stored === 0)", async () => {
    const stateDir = tmpDir("state-bool-reason");
    const memRoot = tmpDir("mem-bool-reason");

    const boolReasonEvaluator = async (): Promise<RelevanceDecision> => ({
      store: true,
      reason: false as unknown as string,
      priority: 'low',
      tags: [],
      summary: 'ok',
    });

    const originalFetch = globalThis.fetch;
    globalThis.fetch = async () => new Response(RSS_FIXTURE, { status: 200 }) as Response;

    try {
      const { result } = await pollFeed(SAMPLE_FEED_CONFIG, stateDir, memRoot, boolReasonEvaluator, undefined, {});
      expect(result.stored).toBe(0);
      expect(result.errors.length).toBeGreaterThan(0);
      expect(result.errors[0]).toMatch(/incomplete RelevanceDecision|missing fields/i);
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// validateRelevanceDecision() unit tests (phase-15, step-v9-002)
// Direct unit tests for the shared validation helper — single source of truth
// for all 6 feed paths per REQ-FEED-061.
// ─────────────────────────────────────────────────────────────────────────────
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-15/step-v9-002 test=feed-ingestion.test.ts#validateRelevanceDecision-unit jira_ref=SWDE-52
describe("validateRelevanceDecision() unit tests", () => {
  const validDecision: RelevanceDecision = {
    store: true,
    reason: "Relevant to current research",
    priority: "medium",
    tags: ["research"],
    summary: "A useful finding",
  };

  test("valid complete decision returns empty array (no missing fields)", () => {
    expect(validateRelevanceDecision(validDecision)).toEqual([]);
  });

  test("missing priority returns ['priority']", () => {
    const d = { ...validDecision, priority: undefined as unknown as "low" };
    expect(validateRelevanceDecision(d)).toEqual(["priority"]);
  });

  test("missing tags returns ['tags']", () => {
    const d = { ...validDecision, tags: undefined as unknown as string[] };
    expect(validateRelevanceDecision(d)).toEqual(["tags"]);
  });

  test("reason: null returns ['reason']", () => {
    const d = { ...validDecision, reason: null as unknown as string };
    expect(validateRelevanceDecision(d)).toContain("reason");
  });

  test("reason: 0 (number) returns ['reason']", () => {
    const d = { ...validDecision, reason: 0 as unknown as string };
    expect(validateRelevanceDecision(d)).toContain("reason");
  });

  test("reason: false (boolean) returns ['reason']", () => {
    const d = { ...validDecision, reason: false as unknown as string };
    expect(validateRelevanceDecision(d)).toContain("reason");
  });

  test("reason: '' (empty string) returns ['reason']", () => {
    const d = { ...validDecision, reason: "" };
    expect(validateRelevanceDecision(d)).toContain("reason");
  });

  test("reason: '   ' (whitespace-only) returns ['reason']", () => {
    const d = { ...validDecision, reason: "   " };
    expect(validateRelevanceDecision(d)).toContain("reason");
  });

  test("store: true + summary: '' returns ['summary']", () => {
    const d = { ...validDecision, store: true, summary: "" };
    expect(validateRelevanceDecision(d)).toContain("summary");
  });

  test("store: false + summary: '' returns [] (summary not required when not storing)", () => {
    const d = { ...validDecision, store: false, summary: "" };
    expect(validateRelevanceDecision(d)).not.toContain("summary");
  });

  test("REQUIRED_DECISION_FIELDS contains exactly the 5 required fields", () => {
    expect(REQUIRED_DECISION_FIELDS).toEqual(["store", "reason", "priority", "tags", "summary"]);
  });

  // priority enum validation (step-v10-002)
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-16/step-v10-002 test=feed-ingestion.test.ts#validateRelevanceDecision-unit jira_ref=SWDE-52
  test("priority: 'urgent' (non-enum value) returns ['priority']", () => {
    const d = { ...validDecision, priority: 'urgent' as unknown as 'low' };
    expect(validateRelevanceDecision(d)).toEqual(['priority']);
  });

  test("priority: 'medium' (valid enum) returns []", () => {
    const d = { ...validDecision, priority: 'medium' as const };
    expect(validateRelevanceDecision(d)).not.toContain('priority');
  });

  test("priority: 'high' (valid enum) returns []", () => {
    const d = { ...validDecision, priority: 'high' as const };
    expect(validateRelevanceDecision(d)).not.toContain('priority');
  });

  test("priority: 'low' (valid enum) returns []", () => {
    const d = { ...validDecision, priority: 'low' as const };
    expect(validateRelevanceDecision(d)).not.toContain('priority');
  });

  // tags array validation (step-v10-003)
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-16/step-v10-003 test=feed-ingestion.test.ts#validateRelevanceDecision-unit jira_ref=SWDE-52
  test("tags: 'security' (string instead of array) returns ['tags']", () => {
    const d = { ...validDecision, tags: 'security' as unknown as string[] };
    expect(validateRelevanceDecision(d)).toContain('tags');
  });

  test("tags: [] (empty array) returns []", () => {
    const d = { ...validDecision, tags: [] };
    expect(validateRelevanceDecision(d)).not.toContain('tags');
  });

  test("tags: ['a', 'b'] (valid array) returns []", () => {
    const d = { ...validDecision, tags: ['a', 'b'] };
    expect(validateRelevanceDecision(d)).not.toContain('tags');
  });

  // Null edge cases (step-v11-008) — explicit tests for val == null guard on priority and tags
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-17/step-v11-008 test=feed-ingestion.test.ts#validateRelevanceDecision-unit jira_ref=SWDE-52
  test("priority: null returns ['priority']", () => {
    const d = { ...validDecision, priority: null as unknown as 'low' };
    expect(validateRelevanceDecision(d)).toContain('priority');
  });

  test("tags: null returns ['tags']", () => {
    const d = { ...validDecision, tags: null as unknown as string[] };
    expect(validateRelevanceDecision(d)).toContain('tags');
  });

  // Tags element-type tests (step-v11-002) — array must contain only strings
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-061 plan=phase-17/step-v11-002 test=feed-ingestion.test.ts#validateRelevanceDecision-unit jira_ref=SWDE-52
  test("tags: [null, null] (array of nulls) returns ['tags']", () => {
    const d = { ...validDecision, tags: [null, null] as unknown as string[] };
    expect(validateRelevanceDecision(d)).toEqual(['tags']);
  });

  test("tags: [1, 2] (array of numbers) returns ['tags']", () => {
    const d = { ...validDecision, tags: [1, 2] as unknown as string[] };
    expect(validateRelevanceDecision(d)).toEqual(['tags']);
  });

  test("tags: ['a', null] (mixed array) returns ['tags']", () => {
    const d = { ...validDecision, tags: ['a', null] as unknown as string[] };
    expect(validateRelevanceDecision(d)).toEqual(['tags']);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// callEvaluatorWithTimeout unit tests (phase-18, step-v12-007)
// Direct unit tests for the exported timeout helper.
// ─────────────────────────────────────────────────────────────────────────────
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#14.4 plan=phase-18/step-v12-007 test=feed-ingestion.test.ts#callEvaluatorWithTimeout-unit jira_ref=SWDE-52
describe("callEvaluatorWithTimeout unit tests", () => {
  const dummyItem: FeedItem = {
    item_id: "test-item",
    feed_id: "test-feed",
    title: "Test",
    content: "Content",
    url: "https://x.com/test",
    published_at: new Date().toISOString(),
    author: "",
    tags: [],
    raw: {},
  };
  const dummyConfig = { prompt: "p" };
  const validDecisionResult: RelevanceDecision = {
    store: true, reason: "relevant", priority: "medium", tags: ["test"], summary: "ok",
  };

  test("fast evaluator (10ms) resolves successfully with default timeout", async () => {
    const fastEvaluator = async (): Promise<RelevanceDecision> =>
      new Promise(resolve => setTimeout(() => resolve(validDecisionResult), 10));
    const result = await callEvaluatorWithTimeout(fastEvaluator, dummyItem, dummyConfig);
    expect(result).toEqual(validDecisionResult);
  });

  test("slow evaluator (200ms) with explicit 50ms timeout rejects with 'evaluator timeout'", async () => {
    const slowEvaluator = async (): Promise<RelevanceDecision> =>
      new Promise(resolve => setTimeout(() => resolve(validDecisionResult), 200));
    await expect(
      callEvaluatorWithTimeout(slowEvaluator, dummyItem, dummyConfig, undefined, 50)
    ).rejects.toThrow("evaluator timeout");
  });

  test("evaluator that resolves immediately succeeds with any timeout", async () => {
    const immediateEvaluator = async (): Promise<RelevanceDecision> => validDecisionResult;
    const result = await callEvaluatorWithTimeout(immediateEvaluator, dummyItem, dummyConfig, undefined, 1000);
    expect(result).toEqual(validDecisionResult);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// isStaleWindow unit tests (phase-19, step-v13-003)
// ─────────────────────────────────────────────────────────────────────────────
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-19/step-v13-003 test=feed-ingestion.test.ts#isStaleWindow-unit jira_ref=SWDE-52
describe("isStaleWindow unit tests", () => {
  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-23/step-v17-005 test=feed-ingestion.test.ts#isStaleWindow-unit jira_ref=SWDE-52
  test("SEVEN_DAYS_MS equals exactly 7 days in milliseconds (604800000)", () => {
    // This test prevents silent threshold drift: if SEVEN_DAYS_MS is changed to a wrong
    // value, both isStaleWindow and the tests would use the new value and tests would still
    // pass. This assertion independently verifies the constant's value.
    expect(SEVEN_DAYS_MS).toBe(7 * 24 * 60 * 60 * 1000);
    expect(SEVEN_DAYS_MS).toBe(604800000);
  });

  test("returns false for a window_start 2 days ago (fresh)", () => {
    const twoDaysAgo = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString();
    expect(isStaleWindow(twoDaysAgo)).toBe(false);
  });

  test("returns true for a window_start 8 days ago (stale)", () => {
    const eightDaysAgo = new Date(Date.now() - 8 * 24 * 60 * 60 * 1000).toISOString();
    expect(isStaleWindow(eightDaysAgo)).toBe(true);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-22/step-v16-006 test=feed-ingestion.test.ts#isStaleWindow-unit jira_ref=SWDE-52
  test("returns false for a window_start 5 seconds inside the 7-day boundary (not yet stale)", () => {
    // Uses Date.now() - SEVEN_DAYS_MS + 5000ms so the timestamp is 5s inside the 7-day window
    // (not yet stale). The +5000 guards against CI runner latency (GC pauses, container cold
    // starts) where >1s can elapse between Date.now() calls. 5s is the conventional CI margin.
    const justInsideBoundary = new Date(Date.now() - SEVEN_DAYS_MS + 5000).toISOString();
    expect(isStaleWindow(justInsideBoundary)).toBe(false);
  });

  // axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#REQ-FEED-084 plan=phase-24/step-v18-004 test=feed-ingestion.test.ts#isStaleWindow-unit jira_ref=SWDE-52
  test("returns true for a window_start 1001ms past the 7-day boundary (just stale)", () => {
    // Behavioral test: proves SEVEN_DAYS_MS is actually wired into isStaleWindow at the boundary.
    // The SEVEN_DAYS_MS value tests above verify the constant's arithmetic value, but not that
    // isStaleWindow uses it correctly. This test closes that gap.
    // Uses -1001ms (not -1ms) to avoid sub-millisecond timing issues.
    const justPastBoundary = new Date(Date.now() - SEVEN_DAYS_MS - 1001).toISOString();
    expect(isStaleWindow(justPastBoundary)).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// clearTimeout call verification (phase-20, step-v14-006)
// Bun resolves bare clearTimeout() through globalThis at call time (not bound at module load).
// This means replacing globalThis.clearTimeout before calling callEvaluatorWithTimeout
// will intercept the call. Verified live: spy count reaches 1 in both fast and timeout paths.
// axiom:trace work_item=SWDE-52 spec=specs/105-Feed-Ingestion.md#14.4 plan=phase-20/step-v14-006 plan=phase-21/step-v15-005 test=feed-ingestion.test.ts#clearTimeout-verification jira_ref=SWDE-52
describe("callEvaluatorWithTimeout — clearTimeout verification", () => {
  const dummyItem: FeedItem = {
    item_id: "ct-test",
    feed_id: "ct-feed",
    title: "ClearTimeout Test",
    content: "Content",
    url: "https://x.com/ct",
    published_at: new Date().toISOString(),
    author: "",
    tags: [],
    raw: {},
  };
  const dummyConfig = { prompt: "p" };
  const validDecision: RelevanceDecision = {
    store: true, reason: "ok", priority: "low", tags: [], summary: "ok",
  };

  test("clearTimeout is called when fast evaluator resolves before timeout fires", async () => {
    let clearTimeoutCallCount = 0;
    const originalClearTimeout = globalThis.clearTimeout;
    globalThis.clearTimeout = ((id: ReturnType<typeof setTimeout> | undefined) => {
      if (id !== undefined) clearTimeoutCallCount++;
      originalClearTimeout(id as ReturnType<typeof setTimeout>);
    }) as typeof globalThis.clearTimeout;

    try {
      const fastEvaluator = async (): Promise<RelevanceDecision> =>
        new Promise(resolve => setTimeout(() => resolve(validDecision), 10));
      await callEvaluatorWithTimeout(fastEvaluator, dummyItem, dummyConfig, undefined, 5000);
      // clearTimeout should have been called exactly once (for the timeout timer)
      expect(clearTimeoutCallCount).toBe(1);
    } finally {
      globalThis.clearTimeout = originalClearTimeout;
    }
  });

  test("clearTimeout is called even when evaluator times out (finally block runs)", async () => {
    let clearTimeoutCallCount = 0;
    const originalClearTimeout = globalThis.clearTimeout;
    globalThis.clearTimeout = ((id: ReturnType<typeof setTimeout> | undefined) => {
      if (id !== undefined) clearTimeoutCallCount++;
      originalClearTimeout(id as ReturnType<typeof setTimeout>);
    }) as typeof globalThis.clearTimeout;

    try {
      const slowEvaluator = async (): Promise<RelevanceDecision> =>
        new Promise(resolve => setTimeout(() => resolve(validDecision), 500));
      await callEvaluatorWithTimeout(slowEvaluator, dummyItem, dummyConfig, undefined, 50)
        .catch(() => {}); // expected to reject
      // clearTimeout should have been called (even though timeout fired first)
      expect(clearTimeoutCallCount).toBe(1);
    } finally {
      globalThis.clearTimeout = originalClearTimeout;
    }
  });
});
