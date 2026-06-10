---
name: axiom-cost-tracking
description: Portable cost tracking and session analytics for Axiom — token/cost collection from OpenCode sessions, pricing tables, Jira integration, steering logs, and budget governance.
version: "1.0"
synopsis: |
  Defines how Axiom collects, calculates, aggregates, and surfaces LLM cost data from OpenCode session
  storage. Covers the 5-field pricing schema, session-to-run correlation, cost formulas, Jira cost comments,
  evidence bundle integration, steering logs for quality signals, metrics, and configuration.
when-to-use: |
  Load this skill when implementing cost collection, building pricing tables, posting cost summaries to Jira,
  integrating cost data into evidence bundles, designing steering logs for model quality signals, or
  configuring cost budgets and alert thresholds.
tags:
  vertical: [ops, sre, benchmarking]
  category: observability
  core: false
---

# Axiom Cost Tracking and Session Analytics (Portable)

This skill defines how Axiom collects, parses, aggregates, and surfaces LLM cost data.

Source spec: `specs/47-Cost-Tracking-And-Session-Analytics.md`

---

## Overview

Every Axiom run spawns one or more OpenCode sessions. Each session makes multiple LLM calls. Token and cost data is written to disk by OpenCode in per-message JSON files. Axiom must:

1. **Collect** cost data from OpenCode session storage after each run.
2. **Aggregate** cost per run, per work item, and per Jira ticket.
3. **Attach** cost summaries to Jira tickets and evidence bundles.
4. **Expose** cost data via structured log events and metrics.
5. **Detect** errors/failures and record them as quality signals for the steering log.

### Non-Goals (v1)
- Real-time cost streaming during execution.
- Automatic model switching based on cost thresholds.
- Billing integration with cloud providers.
- Cross-repo cost rollups.

---

## OpenCode Session Storage

### Base Directory

| Platform | Default Path | Override |
|---|---|---|
| macOS | `~/Library/Application Support/opencode/` | `OPENCODE_DATA_DIR` |
| Linux | `~/.local/share/opencode/` | `OPENCODE_DATA_DIR` |
| Windows | `%LOCALAPPDATA%\opencode\` | `OPENCODE_DATA_DIR` |

Precedence: `OPENCODE_DATA_DIR` env var → platform default.

Path MUST be validated against allowlist (no path traversal).

### Storage Structure

```
<base_dir>/storage/
├── session/<projectHash>/
│   └── <sessionID>.json          # Session metadata + accumulated cost
└── message/<sessionID>/
    └── <messageID>.json          # Per-LLM-call cost + token data
```

### Key Rules
- Session-level `promptTokens`/`completionTokens` reset after compaction — do NOT use as sole source.
- Message files are the authoritative per-call records.
- `cost: 0` on disk — always recalculate from tokens + pricing table.
- Compaction summary messages (`summary: true`) MUST be excluded from aggregation.

---

## Cost Calculation

### 5-Field Pricing Schema

Each model entry in the pricing table has:

```yaml
- id: "model-id"
  provider: "provider-name"
  aliases: ["alias-1", "alias-2"]
  price_per_1m_input: 3.00
  price_per_1m_output: 15.00
  price_per_1m_cache_read: 0.30
  price_per_1m_cache_write: 3.75
  price_per_1m_reasoning: 0.00
```

### Cost Formula

```
cost_usd = (price_per_1m_cache_write / 1M × tokens.cache.write)
         + (price_per_1m_cache_read  / 1M × tokens.cache.read)
         + (price_per_1m_input       / 1M × tokens.input)
         + (price_per_1m_output      / 1M × tokens.output)
         + (price_per_1m_reasoning   / 1M × tokens.reasoning)
```

### Model Alias Resolution
If `modelID` not found directly, check `aliases` map before treating as unknown.

### Unknown Models
If model not in pricing table: cost = `null`, emit WARN log. Run MUST NOT fail.

---

## Token Breakdown

```python
@dataclass
class TokenBreakdown:
    input: int
    output: int
    reasoning: int
    cache_read: int
    cache_write: int

    @property
    def total(self) -> int:
        return self.input + self.output + self.reasoning + self.cache_read + self.cache_write
```

---

## Data Schemas

### MessageCostRecord (per-LLM-call)

| Field | Type | Notes |
|---|---|---|
| `session_id` | string | Parent session |
| `message_id` | string | Message UUID |
| `model_id` | string | Model identifier |
| `provider_id` | string | Provider (anthropic, amazon-bedrock, etc.) |
| `agent` | string | Agent name (dev-axiom, qa-axiom, etc.) |
| `tokens` | TokenBreakdown | Token counts |
| `cost_usd` | float \| null | null if model unpriced |
| `duration_ms` | int | LLM call duration |
| `finish` | string | "stop", "tool-calls", "length" |
| `error` | dict \| null | Error details if call failed |

### RunCostRecord (aggregated per run)

| Field | Type | Notes |
|---|---|---|
| `run_id` | string | Run identifier |
| `work_item_id` | string | Work item |
| `repo` | string | Repository |
| `session_ids` | list[str] | All sessions in run |
| `total_cost_usd` | float \| null | null if any model unpriced |
| `total_cost_usd_lower_bound` | float | Sum of priced messages only |
| `total_tokens` | TokenBreakdown | Sum across all messages |
| `message_count` | int | Total LLM calls |
| `error_count` | int | Messages with errors |
| `finish_length_count` | int | Context overflow count |
| `by_agent` | dict | Per-agent cost summary |
| `by_model` | dict | Per-model cost summary |
| `collected_at` | string | ISO 8601 timestamp |
| `pricing_table_version` | string | Version of pricing table used |

### Storage Path

```
.memory-bank/work-items/<WORK_ITEM_ID>/runs/<RUN_ID>/cost.json
```

---

## Session-to-Run Correlation

- Runner records every OpenCode session ID in the run checkpoint (`opencode_session_ids`).
- Cost collection triggers on `run_completed`, `run_failed`, or `run_blocked`.
- Cost collection MUST be attempted even on failed/blocked runs.

---

## Jira Integration

### Cost Comment
- Posted as a separate comment (not merged into progress comment).
- Idempotency key: `axiom:cost:<run_id>`.
- Includes: total cost, tokens, LLM calls, duration, models, per-agent breakdown, quality signals.
- If `total_cost_usd` is null: display `$<lower_bound>+ USD (partial — N model(s) unpriced)`.

### Evidence Bundle Integration

```yaml
cost:
  total_usd: 0.4821
  total_tokens: 157400
  message_count: 47
  error_count: 2
  finish_length_count: 1
  cost_json_path: .memory-bank/work-items/<ID>/runs/<RUN_ID>/cost.json
  pricing_table_version: "2026-02-01"
```

---

## Steering Log (Quality Signals)

The steering log captures error/quality signals for human-reviewed model selection decisions. **Not automatic model switching** — human-reviewed artifact in v1.

### Signal Types

| Signal | Source | Description |
|---|---|---|
| `xml_parse_failure` | XML parser | Required tags missing/malformed after recovery |
| `xml_partial_recovery` | V2 variant | Tags recovered via v2 (original was incomplete) |
| `llm_call_error` | Message `error` field | LLM call returned error |
| `context_overflow` | `finish == "length"` | Response truncated |
| `session_compaction` | `summaryMessageID` non-empty | Session was compacted |
| `high_cost_step` | Cost calculation | Step cost > threshold |
| `high_cost_run` | Cost calculation | Run cost > threshold |

### Storage
- Append-only JSONL: `.memory-bank/work-items/<ID>/steering-log.jsonl`
- Cross-run summary: `.memory-bank/work-items/<ID>/steering-summary.json`

### Trend Calculation
- `"increasing"`: last run count > 1.5× per-run average.
- `"decreasing"`: last run count < 0.5× per-run average.
- `"stable"`: otherwise.

---

## Structured Log Events

| Event | Level | When |
|---|---|---|
| `cost_collection_started` | INFO | Collection begins |
| `cost_collection_completed` | INFO | Collection succeeds |
| `cost_collection_failed` | WARN | Collection fails (non-fatal) |
| `cost_model_unpriced` | WARN | Model not in pricing table |
| `cost_comment_posted` | INFO | Cost comment posted to Jira |
| `cost_steering_signal` | INFO | Quality signal recorded |

---

## Metrics

| Metric | Type | Labels |
|---|---|---|
| `codeops_run_cost_usd` | Gauge | `work_item_id`, `repo`, `run_id` |
| `codeops_run_tokens_total` | Counter | `work_item_id`, `repo`, `token_type` |
| `codeops_llm_call_cost_usd` | Histogram | `agent`, `model_id`, `provider_id` |
| `codeops_llm_call_duration_seconds` | Histogram | `agent`, `model_id`, `provider_id` |
| `codeops_steering_signal_total` | Counter | `work_item_id`, `repo`, `signal_type`, `agent` |
| `codeops_cost_model_unpriced_total` | Counter | `model_id`, `provider_id` |

---

## Configuration

### Pricing Table (`.axiom/cost-pricing.yaml`)

```yaml
version: "2026-02-01"
models:
  - id: "claude-3-5-sonnet-20241022"
    provider: "anthropic"
    aliases: ["claude-sonnet-4-20250514"]
    price_per_1m_input: 3.00
    price_per_1m_output: 15.00
    price_per_1m_cache_read: 0.30
    price_per_1m_cache_write: 3.75
    price_per_1m_reasoning: 0.00
```

Rules:
- Loaded at cost collection time (not startup).
- Version recorded in every `RunCostRecord` and Jira comment.
- Updatable without code change.

### Cost Config (`.axiom/axiom.config.yaml`)

```yaml
cost:
  enabled: true
  pricing_table_path: ".axiom/cost-pricing.yaml"
  post_to_jira: true
  attach_to_evidence_bundle: true
  alert_threshold_usd_per_step: 1.00
  alert_threshold_usd_per_run: 10.00
  steering_log_enabled: true
  opencode_data_dir: null
```

---

## Security and Privacy

- MUST NOT read files outside OpenCode storage directory.
- MUST NOT log message content at INFO level (only metadata: session ID, tokens, cost).
- Pricing table MUST NOT contain API keys or secrets.
- Jira cost comments MUST NOT include message content or repo file paths beyond evidence bundle refs.

---

## Negative Cases

| Case | Behavior |
|---|---|
| Missing OpenCode storage dir | WARN log; empty/partial record; run continues |
| Corrupt message JSON | Skip with WARN; other messages still processed |
| Missing pricing table | All costs = null with WARN; run continues |
| Path traversal in data dir | Reject per input sanitization rules |
| Jira API failure on comment | Log error; cost data still written locally |
| Duplicate model IDs in pricing | Use first entry; WARN log |
| Negative token counts | Treat as 0; WARN log |

---

## Implementation Location

| Component | Path |
|---|---|
| Cost collector | `.axiom/src/axiom/shared/cost/collector.py` |
| Cost parser | `.axiom/src/axiom/shared/cost/parser.py` |
| Pricing loader | `.axiom/src/axiom/shared/cost/pricing.py` |
| Steering log writer | `.axiom/src/axiom/shared/cost/steering.py` |
| Jira comment formatter | `.axiom/src/axiom/shared/cost/jira_comment.py` |
| Pricing table | `.axiom/cost-pricing.yaml` |
| Unit tests | `.axiom/tests/shared/cost/` |
