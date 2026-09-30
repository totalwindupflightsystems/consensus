# SG-6 report — T4 graded non-vacuously, and it is GREEN over a real conversation

**Subgoal.** *T4 — non-vacuous grading over a real conversation.* (Merged tree §4, SG-6;
baseline `tier-sweep-v2.json` graded `types={}` / `('failed', 0, 0)` / `0 rows` — a sweep over
**zero conversations**.)

## done-when → measured

| requirement | result |
|---|---|
| one real round trip yields session status **idle** with **tokens > 0** | **MET** — `idle`, `tokens_used_in=1695`, `tokens_used_out=128` (session `a9c1aa6c-73ea-43f1-8899-81b886646c87`, run `20260930T092901Z`) |
| **>= 1 `agent_billing` row with cost > 0** | **MET** — 1 row, `model_id=deepseek-flash`, 1695 prompt + 128 completion tokens, **`cost_usd=0.010395`** |
| the sweep driver prints **audited_targets** | **MET** — `scripts/battery/t4_sweep.sh` prints each audited target (route + table + why) before the verdict; also carried as the `audited_targets` field of `t4.json` |
| the driver **FAILs on a non-idle session** | **MET** — red control `badkey`: the LLM 401s, the session settles `paused`, `session-idle` check FAILs, verdict **FAIL** (`t4-control-badkey.json`) |
| evidence `docs/testing/evidence/t4-rerun/t4.json`, same schema as tier-sweep-v2.json | **MET** — `schema: "consensus-tier-sweep-v2"`, plus `audited_targets`, `round_trip`, `checks`, `falsifier` fields |

The falsifier **did not trigger**: `agent_billing=1` and `memory_events=3` after the round trip —
rows are no longer reported over zero conversations, so this is a product verdict, not a runner row.

## What actually happens in one round trip (the wiring the numbers prove)

`POST /api/v1/sessions` → 201 (session `booting`, session-scoped key minted) →
`POST /api/v1/sessions/{id}/message` (user_message row written; session wakes `thinking`, harness
wake signalled) → harness claims `thinking→planning`, calls the configured LLM
(`https://api.deepseek.com/v1`, `deepseek-flash`) → on every terminal the pending-usage flush
commits one transaction that both accumulates `sessions.tokens_used_in/out` AND inserts the
`agent_billing` row with `cost_usd` → session settles **idle**. The audit then reads exactly those
surfaces: `GET /api/v1/sessions/{id}` (status + tokens), `GET /api/v1/sessions/{id}/memory`
(the conversation ledger), `GET /api/v1/sessions/{id}/billing` (cost rows).

## Non-vacuity is proven, not asserted

The 2026-09-24 baseline passed `session-tokens` while observing `('failed', 0, 0)` — a check that
cannot fail is decoration. Both halves of this grading are red-proven by committed control runs:

| control | mechanism | observed | verdict |
|---|---|---|---|
| `CONTROL_MODE=mock` | `CONSENSUS_MOCK_LLM=1`: the session still idles, but with zero tokens and 10 zero-cost billing rows | `session-tokens` FAIL, `billing-rows` FAIL | **FAIL** (expected FAIL) |
| `CONTROL_MODE=badkey` | invalid provider key: every LLM call fails, the circuit breaker pauses the session, it never settles idle | `session-idle` FAIL, `billing-rows` FAIL | **FAIL** (expected FAIL) |

So a green verdict requires a session that genuinely reached idle **and** moved real token/cost
state — either alone is not enough. The driver exits 1 on any mandated-check failure; it also
emits an explicit `verdict:"BLOCKED"` artifact (exit 2) when no live LLM key is configured.

## Runner defects found while doing this

The historical T4 defects were **runner defects**, per the SG-6 falsifier's own logic:

1. **The sweep had no committed driver at all.** `tier-sweep-v2.json` was produced ad hoc on the
   `consensus-dev` bunker agent (expired 09-27+); nothing in the repo could re-run or audit it.
   Fixed here: `scripts/battery/t4_sweep.sh` is the committed, re-runnable driver
   (`bash scripts/battery/t4_sweep.sh` from the repo root, `DEEPSEEK_API_KEY` set; fresh scratch
   DB + port 18492 + fresh session each run).
2. **The baseline audited zero conversations and graded the rows anyway** (`0 rows` → FAIL with no
   conversation behind it). The driver now refuses that shape: `audited_targets` names every
   source, and the falsifier block fires when rows are reported over zero conversations.
3. Minor: a `t4.json` may now exist with `verdict:"BLOCKED"` (key missing) — an honest state the
   old binary PASS/FAIL vocabulary could not express.

## What did not move

- **T3's agent-initiated tool calls, T2 multi-turn, T5 MCP, T10 task-create** — separate subgoals
  (SG-7, SG-8..14); untouched here.
- **The model id mismatch is cosmetic but real**: the session row carries `model_id="default"`
  (config default resolved at call time) while the billing row carries the resolved
  `deepseek-flash`. The billing row is the truthful one.
- The DeepSeek endpoint serves model ids `deepseek-flash` / `deepseek-v4-pro`; the shipped
  `consensus.yaml` still names `deepseek-v4-flash` as `default_model`. The API accepts the shipped
  id (it serves and bills as V4.1-Flash), so this is a naming drift, not a failure — noted for the
  SG-8..14 re-measurement, not changed here (out of SG-6 scope).

## Recompute

```bash
cd /home/kara/consensus
export DEEPSEEK_API_KEY=...        # live key; or T4_DEEPSEEK_KEY on this host
bash scripts/battery/t4_sweep.sh   # fresh scratch DB, fresh session each run
jq '.checks, .verdict' docs/testing/evidence/t4-rerun/t4.json
```

Every number in `t4.json` is parsed from a live HTTP response of that run; `MANIFEST.txt` pins the
sha256 of the three committed artifacts.
