# Consensus — Full Test Plan v1

**Provenance.** Built by a quorum round (定足数) on 2026-09-24: a shared claim-checklist brief
(`/tmp/quorum-consensus-plan/brief.txt`) fanned to 5 candidate lanes; 4 landed and were merged, one per
model family, plus one still running (`qwen3.7-max@opencode-go`). Each seat worked the WHOLE checklist
independently; nothing was divided between seats.

| seat | lane | output |
|---|---|---|
| seat-1 | gpt-5.6-sol@openai-codex | 76590 B, detector line present |
| seat-4 | deepseek-v4.1-flash@synthetic | 94286 B, detector line present |
| seat-7 | glm-5.3-flash@zai-glm-default | 50877 B, detector line present |
| seat-8 | kimi-k3@neuralwatt | 60710 B, detector line present |

Merge seat: hermes (this document). The merge **invents nothing** — every test below carries
`proposed_by`, and every contradiction was re-verified by the merge seat against raw data before it
was allowed into this document (see the ledger in §1).


Machine-readable manifest: `consensus-test-manifest-v1.json` — **223 canonical tests** (from 225 seat entries), `proposed_by` attribution per test, `converged: true` where 2+ independent seats proposed it.


Test counts per tier: **T0**: 20, **T1**: 27, **T2**: 24, **T3**: 19, **T4**: 22, **T5**: 14, **T6**: 17, **T7**: 20, **T8**: 19, **T9**: 15, **T10**: 15, **GATE**: 3, **S1**: 2, **S4**: 3


---

## 1. Verified contradiction ledger (merge seat re-check)

Judge claims are leads, not facts. Each item below was re-run against the repo before being accepted.

| # | Claim (seat) | Verdict | Raw evidence |
|---|---|---|---|
| 1 | SQLite pool is "not 8" and the shipped config pins 5 (seat-1) | **CONFIRMED, and it corrects the brief** | `consensus.yaml:29 max_open_conns: 5`; `internal/config/config.go:156 MaxOpenConns: 8` (default); `README.md:143` documents the 5 pin, `README.md:158-161` documents the 8 default. So the pool IN EFFECT under the shipped config is **5**, and 8 is only the no-config default. Any pool-pressure test must state which one it exercised. |
| 2 | The pinned upstream runner does NOT target sst/opencode (seat-7) | **CONFIRMED** | `scripts/test-opencode-upstream.sh:12 REPOSITORY=https://github.com/anomalyco/opencode.git`, VERSION=1.18.29; `scripts/opencode-upstream/manifest.json:2` agrees; but `internal/chronicle/opencode_full_contract_test.go:3` and `opencode_contract_test.go:4,28` say the expectations come from **sst/opencode**, and `specs/017-ui-adapter-layer.md:463` names `sst/opencode` paths. The board row C-GAP-032-ALPHA also says sst/opencode. **One of the two names is wrong, and the plan must test the PINNED artifact (anomalyco/opencode v1.18.29) while the naming is reconciled.** |
| 3 | `docs/API.md` is not the authoritative path list (seat-8) | **CONFIRMED** | `grep -cE "^(GET|POST|PUT|PATCH|DELETE) " docs/API.md` = **0**. The path authority is `specs/openapi/bundled.yaml` (**60** path keys, counted). |
| 4 | Crier docs carry no endpoint list either (seat-8) | **CONFIRMED** | `/home/kara/crier/docs/openapi.yaml` = **14** path keys; `docs/integration-guide.md` GET/POST lines = **0**. T7 must be written against the OpenAPI file, not the prose. |
| 5 | `internal/db/db.go:76` comment says MaxOpenConns is "Postgres only" (seats 7 and 8) | **CONFIRMED (comment drift, not behaviour drift)** | `internal/db/sqlite/sqlite.go:90 maxOpen := cfg.MaxOpenConns` / `:94 conn.SetMaxOpenConns(maxOpen)` — SQLite consumes it. A plan step asserting pool size must not read that comment as truth. |
| 6 | `make contract-test` is NOT full API coverage (seat-7) | **PARTLY VERIFIED** | `specs/openapi/bundled.yaml` has 60 paths while the contract target delegates to `bin/contract-test.sh` with a handful of flows. T1 therefore derives its path list from the spec, not from `make contract-test`. |
| 7 | The brief's dead-key census was stale/wrong (seats 1 and 8) | **CONFIRMED — brief corrected** | The brief said "5 of 11 are DEAD"; the measured truth is **5 LIVE / 6 DEAD of 11** (board row ENV-CONSENSUS-1, corrected at commit `77b990b`). |

**Not confirmed / dropped:** nothing was dropped; every CONTRADICTED lead above reproduced.


---

## 2. S0 — Claims under test

A test plan is a proof obligation against claims. Each claim cites its source; the tier that discharges it is named.

| claim | source | discharged by |
|---|---|---|
| "The database IS the agent runtime" — sessions, typed append-only ledger, planning loop | README, specs/ | T2, T4 |
| A real LLM round trip persists assistant output and token/cost accounting | README, `agent_billing` | T2, T4 |
| The HTTP API matches its published OpenAPI contract | `specs/openapi/bundled.yaml` | T1 |
| Agents can call tools, and the RESULTS are visible to the agent | specs, tool registry | T3 |
| An MCP client can attach and drive consensus | docs/INTEGRATION.md, `MCP-DIRECT-001` | T5 |
| The shim is compatible with the pinned upstream opencode server | SPEC-017, `C-GAP-032-ALPHA` | T6 |
| Failed staged SQL is surfaced to the agent instead of being retried blindly | `DF-CONSENSUS-6` fix | T3, T8 |
| Resilience: circuit breaker, pool behaviour, restart recovery | README, `DF-CONSENSUS-10/11` | T8 |
| Committed progress survives a crash; uncommitted work is NOT claimed as recovered | crash demo | T8 |
| An external agent can receive messages through crier | crier README, `CR-IN-001` | T7 |
| muster can drive consensus from its OpenAPI spec | `MUSTER-INT-001` | T6/T10 (new; see §5) |

---

## 3. S1 — Bunker topology & provisioning (authored by the merge seat, from measured facts)

Target: **bunker-mvp** (`78.46.173.180:18080`), measured ONLINE 2026-09-24 with **0/50 agents**, 150 GB disk (12% used), 15.2 GB RAM, egress to `api.deepseek.com` reachable. Fresh-box prerequisites from the bunker project: uidmap, fuse-overlayfs, slirp4netns, dbus-user-session.

Five measured constraints decide the shape — none of them are optional:

1. **A fresh agent has NO Go toolchain** (measured class: `bunker agent-tools` reports present/MISSING by name; the toolchain probe is `bunker exec <id> -- sh -c 'for t in go python3 gcc make; do printf "%-8s %s\n" "$t" "$(command -v $t || echo ABSENT)"; done'`). Provisioning is therefore a first-class step, not an assumption.
2. **`bunker exec` starts a FRESH container per call** — only the workspace bind survives. `GOMODCACHE`/`GOCACHE` must be exported INTO the workspace on every exec, and a build must never be backgrounded inside an exec (it dies with its exec).
3. **`--timeout` defaults to 30s** and a real build exceeds it; every build/test exec must pass an explicit `--timeout`.
4. **The agent's own credentials are not yours.** Git over the SSHFS mount executes on YOUR side (your identity); exec runs as root with `HOME=/root` in the agent. Clone/build inside the agent, commit/push from the mount side or by shipping a token — state which, and never assume the mount's identity carries into exec.
5. **Bulk transfer goes over `bunker cp`, not the mount** (~150x; large writes over the mount can wedge in `D` state).
Deployment shape: spawn with an explicit long TTL (`bunker spawn consensus-dev --ttl 7d`), extend while live, deploy the repo + toolchain into the agent, run the server INSIDE the agent with its own DB and a distinct port, and drive the batteries from the control host against the agent. Teardown must be scripted and idempotent (agent destroyed, mounts unmounted, no orphan ports, no 25 MB scratch dirs left behind — see `QA-CONSENSUS-6`).
DB choice: run BOTH shapes at least once — the shipped `sqlite://` config (pool **5**) and a PostgreSQL instance — because the persistence story and the pool story differ per backend, and `DF-CONSENSUS-10` was SQLite-specific.

---

## 4. S2 — Tiers (index; per-test detail + commands live in the manifest)


### T0 — Build / unit — compile, vet, env-clean full Go suite.  (20 tests)
| id | title | proposed by |
|---|---|---|
| C-002 | Build all Go packages | gpt-5.6-sol |
| C-171 | Env-clean short suite | kimi-k3 |
| C-004 | Environment-clean fresh tests | gpt-5.6-sol |
| C-126 | Fresh build, no stale root binary | glm-5.3-flash |
| C-170 | Full build | kimi-k3 |
| C-174 | Guard + fmt drift | kimi-k3 |
| C-005 | Keyless mock smoke | gpt-5.6-sol |
| C-129 | Pool fix commit in tree | glm-5.3-flash |
| C-125 | Record commit identity under test | glm-5.3-flash |
| C-001 | Source and binary provenance | gpt-5.6-sol |
| C-172 | TVF probe (generate_series) | kimi-k3 |
| C-173 | Tool registry handler tests | kimi-k3 |
| C-003 | Vet all Go packages | gpt-5.6-sol |
| C-063 | env-clean keyless short suite | deepseek-v4.1-flash |
| … | *6 more in the manifest* | |

### T1 — HTTP API contract — every path in specs/openapi/bundled.yaml (60 path objects), auth negatives, error shapes.  (27 tests)
| id | title | proposed by |
|---|---|---|
| C-013 | API lifecycle and revocation | gpt-5.6-sol |
| C-011 | Admin-key acceptance | gpt-5.6-sol |
| C-177 | Auth ladder | kimi-k3 |
| C-132 | Auth ladder on all authenticated paths | glm-5.3-flash |
| C-181 | DF-2 probe: documented-only fields | kimi-k3 |
| C-133 | Error body shape on 4xx | glm-5.3-flash |
| C-179 | Error shape consistency | kimi-k3 |
| C-131 | Every REST path exercised with expected status | glm-5.3-flash |
| C-008 | Every declared operation exercised | gpt-5.6-sol |
| C-010 | Fake-key rejection | gpt-5.6-sol |
| C-178 | Full 60-path sweep | kimi-k3 |
| C-012 | Invalid UUID error contract | gpt-5.6-sol |
| C-007 | OpenAPI operation census is nonempty | gpt-5.6-sol |
| C-130 | OpenAPI path census = 60 | glm-5.3-flash |
| … | *13 more in the manifest* | |

### T2 — Live-LLM agent loop INCLUDING MULTI-TURN (the DF-CONSENSUS-15 path) with token + billing assertions.  (24 tests)
| id | title | proposed by |
|---|---|---|
| C-183 | Baseline round trip | kimi-k3 |
| C-140 | Billing row model stamp matches served model | glm-5.3-flash |
| C-186 | Billing sanity band | kimi-k3 |
| C-187 | DF-14 model override probe | kimi-k3 |
| C-184 | DF-15 multi-call planning | kimi-k3 |
| C-080 | ENV-ONLY path (no consensus.yaml in cwd) | deepseek-v4.1-flash |
| C-139 | Env-only model override regression (DF-14) | glm-5.3-flash |
| C-019 | Environment-only LLM configuration | gpt-5.6-sol |
| C-014 | Exact live-key preflight | gpt-5.6-sol |
| C-135 | Live LLM key validated pre-run | glm-5.3-flash |
| C-018 | Live multi-call database question | gpt-5.6-sol |
| C-077 | MULTI-TURN: user turn survives the 2nd LLM call in the same iteration | deepseek-v4.1-flash |
| C-185 | Multi-iteration user-turn visibility | kimi-k3 |
| C-137 | Multi-turn cell A: next-iteration continuity | glm-5.3-flash |
| … | *10 more in the manifest* | |

### T3 — Tool calls end to end: registry -> execute -> result -> AGENT-VISIBLE -> the agent actually uses it.  (19 tests)
| id | title | proposed by |
|---|---|---|
| C-082 | API-level tool execute (documented path) | deepseek-v4.1-flash |
| C-144 | Agent USES the tool end-to-end (DF-15 dependent) | glm-5.3-flash |
| C-022 | Agent requests tool | gpt-5.6-sol |
| C-190 | Agent uses tool end-to-end | kimi-k3 |
| C-024 | Agent uses tool result | gpt-5.6-sol |
| C-021 | Direct deterministic query-tool execution | gpt-5.6-sol |
| C-189 | Direct tool execute | kimi-k3 |
| C-142 | Direct tool execute returns real DB count | glm-5.3-flash |
| C-188 | Registry non-empty | kimi-k3 |
| C-191 | Tool error path | kimi-k3 |
| C-020 | Tool registry discovery | gpt-5.6-sol |
| C-141 | Tool registry register + list | glm-5.3-flash |
| C-023 | Tool result reinjected into agent context | gpt-5.6-sol |
| C-143 | Tool sandbox negatives rejected | glm-5.3-flash |
| … | *5 more in the manifest* | |

### T4 — Sessions / ledger / billing integrity (append-only memory_events, tokens_used_in/out, agent_billing rows).  (22 tests)
| id | title | proposed by |
|---|---|---|
| C-026 | Append-only enforcement | gpt-5.6-sol |
| C-193 | Append-only ledger triggers | kimi-k3 |
| C-145 | Append-only triggers enforce the ledger | glm-5.3-flash |
| C-029 | Atomic rollback | gpt-5.6-sol |
| C-198 | Billing 1:1 with LLM calls | kimi-k3 |
| C-196 | Cost sanity + money cap | kimi-k3 |
| C-148 | Cost sanity vs sticker price | glm-5.3-flash |
| C-197 | DB session isolation | kimi-k3 |
| C-149 | Session isolation at DB layer (C5) | glm-5.3-flash |
| C-028 | Session-key isolation | gpt-5.6-sol |
| C-027 | Token and billing reconciliation | gpt-5.6-sol |
| C-147 | Token reconciliation billing<->sessions | glm-5.3-flash |
| C-194 | Token sums equality | kimi-k3 |
| C-195 | Typed events census | kimi-k3 |
| … | *8 more in the manifest* | |

### T5 — MCP surface — /mcp/message plus the discoverability requirement in MCP-DIRECT-001.  (14 tests)
| id | title | proposed by |
|---|---|---|
| C-202 | External MCP attach (2 clients) | kimi-k3 |
| C-094 | MCP auth enforced on non-initialize methods | deepseek-v4.1-flash |
| C-201 | MCP auth ladder | kimi-k3 |
| C-032 | MCP authentication negatives | gpt-5.6-sol |
| C-200 | MCP handshake + tools | kimi-k3 |
| C-030 | MCP initialize | gpt-5.6-sol |
| C-092 | MCP initialize over /mcp/message | deepseek-v4.1-flash,glm-5.3-flash |
| C-199 | MCP route truth | kimi-k3 |
| C-150 | MCP tools/list + tools/call real execution | glm-5.3-flash |
| C-093 | MCP tools/list with auth, count and uniqueness | deepseek-v4.1-flash |
| C-031 | MCP typed tool discovery | gpt-5.6-sol |
| C-151 | MCP unknown method shaped error | glm-5.3-flash |
| C-095 | MCP unknown tool returns a JSON-RPC error | deepseek-v4.1-flash |
| C-033 | MCP-only session lifecycle | gpt-5.6-sol |

### T6 — OpenCode / shim compatibility — including the LITERAL pinned upstream TypeScript suite and a real-TUI case.  (17 tests)
| id | title | proposed by |
|---|---|---|
| C-034 | Go OpenCode compatibility port | gpt-5.6-sol |
| C-203 | Go port suite count | kimi-k3 |
| C-152 | Go-port chronicle suite baseline | glm-5.3-flash |
| C-100 | Go-port suite retained for the record, excluded from the gate | deepseek-v4.1-flash |
| C-154 | LITERAL upstream TS suites vs live shim (first-ever run) | glm-5.3-flash |
| C-204 | Literal upstream TS suite vs live shim | kimi-k3 |
| C-035 | Pinned literal upstream suites | gpt-5.6-sol |
| C-037 | Real OpenCode TUI attach and chat | gpt-5.6-sol |
| C-205 | Real TUI smoke | kimi-k3 |
| C-155 | Real-TUI / CLI proof | glm-5.3-flash |
| C-153 | Runner adapter self-test (gate only) | glm-5.3-flash |
| C-038 | Shim SSE event mapping | gpt-5.6-sol |
| C-036 | Upstream source and lock integrity | gpt-5.6-sol |
| C-098 | literal upstream TS suites against the live shim | deepseek-v4.1-flash |
| … | *3 more in the manifest* | |

### T7 — CRIER message transport — consensus RECEIVES a message through crier and it becomes an agent-visible turn.  (20 tests)
| id | title | proposed by |
|---|---|---|
| C-044 | Ack, restart and idempotent redelivery | gpt-5.6-sol |
| C-039 | Consensus Crier wiring exists | gpt-5.6-sol |
| C-206 | Crier control round trip | kimi-k3 |
| C-042 | Crier message becomes Consensus user input | gpt-5.6-sol |
| C-208 | Crier message becomes agent-visible input | kimi-k3 |
| C-043 | Different-agent message isolation | gpt-5.6-sol |
| C-209 | Lease crash safety | kimi-k3 |
| C-041 | Register Consensus Crier identity | gpt-5.6-sol |
| C-210 | Signature negatives | kimi-k3 |
| C-040 | Standalone signed Crier inbox lifecycle | gpt-5.6-sol |
| C-207 | Wiring existence census | kimi-k3 |
| C-107 | ack drains the queue | deepseek-v4.1-flash |
| C-108 | consensus-side crier intake (XFAIL, build gate) | deepseek-v4.1-flash |
| C-101 | crier builds and serves | deepseek-v4.1-flash |
| … | *6 more in the manifest* | |

### T8 — Resilience — dead-key circuit breaker, sqlite pool pressure under polling + held-open SSE, restart/recovery, crash demo.  (19 tests)
| id | title | proposed by |
|---|---|---|
| C-211 | Circuit breaker on verified-dead key | kimi-k3 |
| C-048 | Clean process restart durability | gpt-5.6-sol |
| C-050 | Corrupt-copy refusal | gpt-5.6-sol |
| C-214 | Corrupted DB boot | kimi-k3 |
| C-049 | Crash demo | gpt-5.6-sol |
| C-213 | Crash demo (keyed E2E) | kimi-k3 |
| C-161 | Crash recovery: kill -9 mid-flight (C4) | glm-5.3-flash |
| C-045 | Dead-key circuit breaker | gpt-5.6-sol |
| C-159 | Dead-key circuit breaker (C6) | glm-5.3-flash |
| C-047 | Environment-only pool default | gpt-5.6-sol |
| C-046 | Held SSE plus polling pressure | gpt-5.6-sol |
| C-162 | Keyless crash demo reproduces (C14) | glm-5.3-flash |
| C-212 | Pool pressure regression | kimi-k3 |
| C-113 | SIGTERM restart preserves committed ledger | deepseek-v4.1-flash |
| … | *5 more in the manifest* | |

### T9 — Bounded load / concurrency (host load gate respected; never an unbounded burn loop).  (15 tests)
| id | title | proposed by |
|---|---|---|
| C-216 | 8 concurrent sessions | kimi-k3 |
| C-164 | Bounded 4-session concurrency cell | glm-5.3-flash |
| C-052 | Bounded four-worker load | gpt-5.6-sol |
| C-217 | Concurrent billing join | kimi-k3 |
| C-053 | Concurrent session isolation | gpt-5.6-sol |
| C-051 | Host load gate | gpt-5.6-sol |
| C-215 | Load gate | kimi-k3 |
| C-163 | Load-gate pre-flight | glm-5.3-flash |
| C-054 | Post-load health and cleanup | gpt-5.6-sol |
| C-218 | Postgres spot check (optional) | kimi-k3 |
| C-165 | Scratch-dir leak census (QA-CONSENSUS-6 regression) | glm-5.3-flash |
| C-116 | bounded concurrency: exactly 200 sessions over 8 workers | deepseek-v4.1-flash |
| C-118 | host load bounded; no SQLITE_BUSY | deepseek-v4.1-flash |
| C-115 | load gate respected before the cell | deepseek-v4.1-flash |
| … | *1 more in the manifest* | |

### T10 — End-to-end "baby project" — fresh fake project + goal driven through sessions/tasks/tools, verified on disk.  (15 tests)
| id | title | proposed by |
|---|---|---|
| C-057 | Agent file and command tools | gpt-5.6-sol |
| C-167 | Baby project billing closure | glm-5.3-flash |
| C-219 | Baby project setup | kimi-k3 |
| C-060 | Baby project survives Consensus restart | gpt-5.6-sol |
| C-058 | Baby project verified on disk | gpt-5.6-sol |
| C-166 | Baby project: goal -> disk artifact | glm-5.3-flash |
| C-221 | Disk verification | kimi-k3 |
| C-059 | Disk-to-ledger fidelity | gpt-5.6-sol |
| C-055 | Fresh baby-project precondition | gpt-5.6-sol |
| C-056 | Goal and task creation | gpt-5.6-sol |
| C-220 | Goal-driven session | kimi-k3 |
| C-222 | Independent ground truth | kimi-k3 |
| C-120 | baby project: agent used a tool and produced numbers | deepseek-v4.1-flash |
| C-119 | baby project: goal -> tasks created | deepseek-v4.1-flash |
| … | *1 more in the manifest* | |

### GATE — Defect gate and false-green guards (run-blocking).  (3 tests)
| id | title | proposed by |
|---|---|---|
| C-123 | VOID detection (any tier with zero observables) | deepseek-v4.1-flash |
| C-122 | defect gate evaluation | deepseek-v4.1-flash |
| C-124 | secret hygiene of the evidence bundle | deepseek-v4.1-flash |

### S1 — Bunker topology / provisioning assertions.  (2 tests)
| id | title | proposed by |
|---|---|---|
| C-176 | Bunker provision | kimi-k3 |
| C-175 | Key health probe | kimi-k3 |

### S4 — Evidence + reporting contract assertions.  (3 tests)
| id | title | proposed by |
|---|---|---|
| C-168 | Evidence tree assembled + third-party rederivation | glm-5.3-flash |
| C-223 | HTML report + re-derivation | kimi-k3 |
| C-169 | Secret hygiene on artifacts | glm-5.3-flash |

---

## 5. S3 — Defect gate (run-blocking)

This run may be declared **GREEN only when every item below is closed or explicitly xfail-pinned with its board id**.

| id | what | effect on the run |
|---|---|---|
| DF-CONSENSUS-14 | model not env-overridable (`gpt-4o` hardcoded, no `CONSENSUS_LLM_MODEL`) | any env-only start fails 400 x3 and pauses; T2 cannot pass env-only until fixed |
| DF-CONSENSUS-15 | user turn hidden after the first LLM call of an iteration | **T2 multi-turn and T3 tool-use cannot pass**; this is the single most load-bearing defect for the plan |
| C-GAP-032-ALPHA | pinned upstream TS suite never executed against a live shim | T6 stays partial; the plan may not claim "upstream-verified" |
| DF-CONSENSUS-2 / -3 | undocumented required session fields; async message contract vs docs | T1/T2 doc-truth assertions |
| ENV-CONSENSUS-1 | 5 live / 6 dead key candidates in the host env | every live tier must probe its key first and fail loudly |
| QA-CONSENSUS-2 | bunker-las-02 runs an 08-27 binary (leak fix absent) | any battery run on that host tests the wrong binary |
| QA-CONSENSUS-7 / QA-BUNKER-B20 | battery can report `4 passed / 0 failed` while auditing nothing | every tier asserts `audited_targets > 0` |
| MCP-DIRECT-001 | MCP attach surface undiscoverable | T5 discovery assertions |

---

## 6. S4 — Evidence & reporting contract

- One directory per run: `~/consensus-reports/runs/<tick-id>/` containing raw logs, the DB dump used, per-tier JSON results, and the client transcripts. HTML report generated FROM those files, never from memory.
- Every tier writes `{tier, id, command, expected, observed, verdict, artifact_path}` JSON. A tier with no artifact is a FAIL, not a skip.
- The runner must record the commit under test (`git rev-parse HEAD`), the binary hash, the DB path, and the pool size in effect.
- Re-derivation rule: a third party must be able to reproduce every number without trusting the runner — so each number names the command that produced it.

---

## 7. S5 — False-green analysis (merged from all seats)

The seats independently produced the failure classes below; the merge keeps them all because a plan is only as good as its guard against passing wrongly.


---

## 8. S6 — Unknowns (merged)


---

## 9. Top risks (per seat)


### from gpt-5.6-sol@openai-codex
1. Crier intake is still absent.
   Countermeasure: CR-IN-001 is a hard defect gate; require message-ID provenance, isolation, ack and idempotency.

2. DF-CONSENSUS-15 causes multi-call sessions to forget the user request.
   Countermeasure: direct mock-client message-array regression plus a live multi-call database question.

3. Environment-only configuration sends gpt-4o to DeepSeek.
   Countermeasure: no-config live cell with explicit model/provider assertions; DF-CONSENSUS-14 must close.

4. SQLite pool documentation and code have regressed from 8 to 4.
   Countermeasure: explicit primary pool of 8, separate default-value regression test and README/code parity gate.

5. Literal upstream OpenCode tests may fail despite the 46-test Go port passing.
   Countermeasure: run the pinned upstream suites and a real TUI; no substitution.

6. A dead key can mimic a product failure.
   Countermeasure: preflight the exact key, fail immediately on 401, and never log the secret.

7. Another process can answer the chosen port and generate convincing evidence.
   Countermeasure: loopback unique ports, socket preflight, container/image identity and product-specific health checks.

8. The acceptance harness can pass while exercising zero targets.
   Countermeasure: nonzero path, operation, session, tool, billing and load-target assertions; empty-target falsifier.

9. Live SQLite evidence can be incomplete if copied without WAL.
   Countermeasure: SQLite backup API, integrity check and count reconciliation.

10. Remote evidence may be destroyed with the Bunker agent.
    Countermeasure: harvest and verify the archive checksum before service teardown or `bunker destroy`.

PLAN REVIEW COMPLETE

session_id: 20260924_034404_5a2c41

### from deepseek-v4.1-flash@synthetic
1. **DF-CONSENSUS-15 makes T2/T10 self-fulfillingly red, and a rushed run reports it as "environment".** Root cause is one missing predicate at `context.go:585`. Countermeasure: fix or XFAIL-pin *before* the run; T2-04 is a count-based assertion (`messages=N` per call), not a judgement call.
2. **A dead `sk-` key is misread as a product defect** (ENV-CONSENSUS-1, already happened twice and leaked into a judge run). Countermeasure: mandatory probe-first in S1; the report prints last-4 + live/dead beside every `paused`.
3. **`make lint` / `make contract-test` certify a red run** (Makefile:58, :122/:126, no `exit 1`). Countermeasure: banned as evidence; direct commands only; command rcs are recorded so "rc=0 over its own ✗ log" is itself a filed finding.
4. **Phantom wiring: crier intake (C-7) or the shim paths (C-5).** Zero crier references in the tree; 26 bundled shim paths with no visible native registration. Countermeasure: T7-08 is a pinned XFAIL build gate; T1-03 fails on any 404 for a declared path.
5. **The literal upstream suite has never run** (C-GAP-032-ALPHA) and the pin is `anomalyco/opencode`, not `sst/opencode` (C-4). Countermeasure: T6-01/02/03 with an assertion count and revision check; the Go port is explicitly excluded.
6. **False green with zero targets** — the historic QA battery failure (QA-CONSENSUS-7). Countermeasure: every PASS carries a counter > 0; `VOID` is a separate verdict; host pre-flight fails closed.
7. **Stale binary / wrong checkout / cwd-relative DB.** Three independent ways to test the wrong artifact. Countermeasure: `make fresh` + sha256 + a board-marker grep + absolute DSN + t0 freshness assertions.
8. **Billing asserted by the wrong price table** (C-2): the `unknown` fallback overstates DeepSeek flash by **31.9x**, so `cost_usd > 0` is meaningless. Countermeasure: exact recomputation with the `model_registry` premise recorded in the same artifact.
9. **Pool wedge returns under the SSE + polling cell** (DF-CONSENSUS-10). Countermeasure: T8-02 counts `begin tx: context deadline exceeded` as a hard zero and counts stuck `planning` sessions.
10. **The plan itself burns the host or collides with the fleet** (port-pool exhaustion on las-02, load gate). Countermeasure: dedicated agent, explicit pre-flighted port, `/proc/loadavg` gate before T9, bounded 8×25 cell, teardown that kills by pidfile (never `pkill -f`) and destroys the agent after the evidence bundle is copied back.

PLAN REVIEW COMPLETE

session_id: 20260924_034404_ebe67c

### from glm-5.3-flash@zai-glm-default
1. **DF-CONSENSUS-15 stays open → T2-04/T3-04/T10-01 all xfail** and the headline agent capability (tools) is unproven. Countermeasure: fix DF-15 BEFORE the run (iteration-scoped hide in planning.go/context.go); it is the single highest-leverage pre-run fix.
2. **T6 upstream suites have never run — they may fail wholesale** (shim drift vs pinned v1.18.29). Countermeasure: run early in the window, budget a triage slot, pin failures to C-GAP-032-ALPHA rather than waiving.
3. **Dead key confound (5/11 dead).** Countermeasure: T2-01 gate; a dead key aborts T2 before any session is created.
4. **Wrong/stale host or binary** (las-03 ghost, 08-27 binary). Countermeasure: bunker-info-derived host + T0-02 version gate; redeploy on mismatch.
5. **Port 8091 shadowed by a stale sidecar → 404s read as API contract failures.** Countermeasure: pre-flight `ss -tlnp` empty-or-owned check recorded in evidence.
6. **T7b blocks on CR-IN-001 implementation** (zero crier code today). Countermeasure: T7a is independently passable; T7b is a pinned xfail, not a plan blocker; schedule CR-IN-001 work in parallel.
7. **Pool pressure cell under-applied** (no concurrent planning → vacuous pass). Countermeasure: T8-02 asserts the concurrent session completed (billing rows exist) during the pressure window.
8. **Report built by hand, numbers not re-derivable.** Countermeasure: S4-01 rederive.sh gate; HTML generated only from results.json.
9. **Secret leakage into evidence** (serve.log contains the key). Countermeasure: S4-02 secret scan as a hard run gate; key delivered to the bunker via scp'd .env, never argv.
10. **Bunker port-pool/capacity class (QA-CONSENSUS-1/3) kills mid-run cells.** Countermeasure: single dedicated agent for the run, port allocated up front, capacity check in provisioning step; teardown releases everything.

PLAN REVIEW COMPLETE

session_id: 20260924_034559_67a6d7

### from kimi-k3@neuralwatt
1. DF-CONSENSUS-15 contaminates T2/T3/T10: agent cannot see the user turn on multi-call iterations, so tool-using tests degrade into hallucinated correct answers. Countermeasure: tool_requests>0 as a HARD assertion everywhere a tool is expected — zero-tool correct answers are RED, and DF-15's presence is itself detected by T2-2's message-count-shrink check rather than inferred.
2. False GREEN over open P1s (the named #1 failure class). Countermeasure: S3 disposition table is a gate — report builder refuses GREEN while any mapped P1/P2 row lacks a probe outcome line; EXPECTED-FAIL lines print board ids.
3. Dead-key landmine (ENV-CONSENSUS-1, census already drifted once). Countermeasure: fresh probe on the bunker gates the run; T8-1 uses verified-dead keys by measurement, never by board memory.
4. Zero-target battery recurrence (QA-CONSENSUS-7/9). Countermeasure: inventory counts derived twice (yaml parse vs grep) and asserted equal-60 before the T1 sweep executes; executed-count>0 asserted in T6-2; INVALID (not PASS) is the verdict for any tier whose executed volume is zero.
5. Stale/wrong binary or repo (README's own warning; QA-CONSENSUS-2 class). Countermeasure: sha256 + `git rev-parse HEAD` + remote URL in env.txt, compared at report build; `make fresh` first; never run repo-root `./consensus`.
6. Crier capability assumed but absent (CR-IN-001 has zero code at HEAD — grep-proven). Countermeasure: T7-1 census stated as fact; T7-2/3/4 exist as the post-fix acceptance contract and are EXPECTED-FAIL until CR-IN-001 lands; the report cannot silently drop T7 rows.
7. Bunker provisioning/capacity traps (QA-CONSENSUS-1 port pool, las-03 dropped, fresh-box packages). Countermeasure: S1 package list explicit; ports outside fleet pools; `bunker info` resolution; teardown verified by empty ss/pgrep.
8. Unbounded burn violating fleet load doctrine. Countermeasure: every concurrency step bounded (timeouts, worker caps), load gate checked before T9, PIDs journaled and killed by exact pid — no self-matching pkill.
9. Tier tests the harness, not the product (T6 Go port passing while TS suite never ran; demo testing a fixture). Countermeasure: T6-2 is the literal upstream suite against the live shim with BASE_URL identity asserted; T8-3 adds the manual kill -9 repro against the deployed service, not only the Go test.
10. Evidence un-re-derivable (runner-trust). Countermeasure: S4 mandates raw dumps + per-claim recompute commands printed in the report; any number without an artifact hyperlink is marked non-evidence in the report header.

PLAN REVIEW COMPLETE

session_id

---

## Appendix A — per-seat contradiction sections (verbatim)


### gpt-5.6-sol@openai-codex
1. SQLite pool default is not 8 at observed HEAD.

   Evidence:
   - internal/db/sqlite/sqlite.go:90-94:
     `maxOpen := cfg.MaxOpenConns`
     `if maxOpen <= 0 { maxOpen = 4 }`
   - consensus.yaml:27-29 explicitly pins `max_open_conns: 5`.
   - README.md:158-166 claims the environment-only path defaults to 8.

   Consequence: fact 4 and the README’s environment-only pool claim are false at HEAD 2c51cee3b92167aeaacddfe54fe56f6e861120bf. This is a release-blocking contract drift until code, tests and README agree.

2. Dead-key census is 6 dead, not 5 dead.

   Evidence:
   - Last-wins board row ENV-CONSENSUS-1 records 11 candidates, 5 LIVE and 6 DEAD.
   - The row explicitly says its earlier “5 dead” statement was corrected.

   Consequence: fact 8 is stale. The acceptance runner must never select a key by position; it must probe the exact candidate it will use.

3. README’s “environment-only path is also safe now” is false for two independent reasons.

   Evidence:
   - README.md:158-166 claims the path is safe.
   - cmd/consensus/main.go:140-142 defaults an unset model to gpt-4o.
   - DF-CONSENSUS-14 records the resulting DeepSeek 400/circuit-breaker path.
   - internal/db/sqlite/sqlite.go:90-94 defaults the pool to 4 rather than the documented 8.

4. Crier intake is not a shipped Consensus capability at the observed HEAD.

   Evidence:
   - CR-IN-001 is pending.
   - No product source path inspected under internal/api, internal/harness or internal/shim/opencode exposes Crier intake configuration or a Crier consumer.
   - The board acceptance contract still requires a Crier message to become a provenance-bearing user_message.

   Consequence: T7 is an intentional hard failure until CR-IN-001 lands. Treating the standalone Crier demo as proof of Consensus intake would be phantom wiring.

5. The literal upstream OpenCode gate exists but has not earned a PASS.

   Evidence:
   - scripts/test-opencode-upstream.sh pins v1.18.29 at commit 16747470f976aca3d362ad730bcd3fe82ecc2c9a and executes four upstream suites.
   - C-GAP-032-ALPHA remains pending because the live shim run was not completed.

### deepseek-v4.1-flash@synthetic
**C-1 — DF-CONSENSUS-15's mechanism is real and I can name the unused column.**
`internal/harness/context.go:580-585`:
```
COALESCE(dm.mode, 'full') as display_mode,
LEFT JOIN display_modes dm ON dm.memory_id = me.id
  AND COALESCE(dm.mode, 'full') != 'hidden'
```
There is **no iteration predicate in the join** — yet `planning.go:758-761` writes `display_modes(..., set_by_iteration = $2, ...)`. The scoping column exists and the reader ignores it. That is the defect, verbatim: the fix is one line in the join (`AND dm.set_by_iteration = $iteration`) or moving the hide to iteration rollover. Consequence for this plan: any T2 multi-turn assertion that does not count `messages=N` on **each** LLM call in the serve log will pass while the bug is live.

**C-2 — The "verified" billing value is a pricing-table artifact, and the table has no DeepSeek row.**
`internal/harness/harness.go:281-291` prices only `gpt-4o, gpt-4o-mini, claude-*, gemini-flash, mock-model, default-model, unknown`. There is **no `deepseek-flash` and no `deepseek-v4-flash` key**, and `readModelPricing` (harness.go:296-318) returns `unknown = {5.00, 15.00}` when `model_registry` has no `enabled` row. Re-derived arithmetic for the coordinator's own numbers (prompt 1682 / completion 91):

```
1682 * 5.00/1e6 = 0.008410
  91 * 15.00/1e6 = 0.001365
              sum = 0.009775   ← exactly the "verified" cost_usd
DeepSeek public flash rate  = 0.000307  (0.15 / 0.60 per M)
0.009775 / 0.000307 = 31.9x overstatement
```
So pre-run fact 1 is *internally* reproducible but proves **the billing table, not the bill**. Two consequences: (a) `model_registry` is evidently empty/disabled for this model on the SQLite path — itself a claim worth a test; (b) a T4 assertion of the shape `cost_usd > 0` is a false-green generator. T4 must assert the **exact** value **and** reconcile it to the price source, and the 31.9x must be filed as a finding.

**C-3 — The repo's own "contract test" cannot pass and cannot fail.**
`Makefile:102-137`: step 3 and step 4 authenticate with the literal string `$$CONS..._KEY` (lines 122, 126) — an empty shell expansion after make processing, so both requests are unauthenticated; and every step prints `✗` via `echo` with **no `exit 1`**, so `make contract-test` exits 0 over a red run. Combined with `Makefile:58` (`lint:` = `golangci-lint … || echo "skipping"`, always exit 0), two advertised gates are structurally incapable of failing. **Neither may appear as pass evidence in this plan.** (Related: `make smoke` on `Makefile:52-53` is explicitly "real server + **mocked LLM**" — a T0-only check that must never be promoted into T2.)

**C-4 — The upstream suite is pinned to a repository that is not the one the plan's brief names.**
`scripts/test-opencode-upstream.sh:12-14`: `REPOSITORY=https://github.com/anomalyco/opencode.git`, `REVISION=16747470f976aca3d362ad730bcd3fe82ecc2c9a`, `VERSION=1.18.29`. The brief (and board row `C-GAP-032-ALPHA`) says "upstream **sst/opencode**". A plan that fetches `sst/opencode` at the pinned revision will either 404 or fetch a different tree, and the resulting log would be a green over the wrong artifact. T6 must use the in-repo pinned script and assert the revision string appears in the evidence summary.

**C-5 — The API surface is drifted in BOTH directions.**
`specs/openapi/bundled.yaml` declares exactly 60 paths. `internal/api/server.go` registers native routes that are **not** in that set — `server.go:121 GET /api/v1/events` (SSE), `server.go:172-174 POST /api/v1/quarantine`, `/api/v1/quarantine/{qID}/approve|reject`, plus `PATCH`/`DEL

### glm-5.3-flash@zai-glm-default
1. **"upstream sst/opencode TypeScript suite" — the pinned runner does not target sst/opencode.** `scripts/test-opencode-upstream.sh:23-24` pins `REPOSITORY=https://github.com/anomalyco/opencode.git`, `REVISION=16747470f976aca3d362ad730bcd3fe82ecc2c9a`, `VERSION=1.18.29`. Either the brief's "sst/opencode" label is wrong or the pin is. Evidence: file cited above. Impact: T6 tests the pinned anomalyco suites; the plan records the pin, not the brand.
2. **C-GAP-032-ALPHA is still OPEN** (board, last occurrence: `pending`, "[P1] Align shim API with upstream sst/opencode structure: run upstream TS server test suite directly"). The pre-run fact said the runner adapter landed; the alignment itself has not. T6 must expect real failures, not a green.
3. **README's env-only path claim is misleading while DF-CONSENSUS-14 is open.** README.md:159-167 says the environment-only path "is also safe now"; but with env-only config the model stays hardcoded `gpt-4o` and DeepSeek 400s (DF-CONSENSUS-14, pending). The README sentence is true only for the pool, false for the LLM config. T2 includes a regression cell for exactly this.
4. **`internal/db/db.go:76` comment says MaxOpenConns is "Postgres only"** but `internal/db/sqlite/sqlite.go:90-94` applies it to SQLite too. Comment drift, not behavior drift — noted, no test impact beyond documentation honesty.
5. **`make contract-test` covers 5 endpoints; the API has 60 paths in `specs/openapi/bundled.yaml`** (counted: 60 top-level `  /` path keys). Any plan leaning on `make contract-test` for "full API contract" tests ~8% of the surface. T1 replaces it with generated full-path coverage.
6. **Board census is larger than the brief stated.** Last-wins board read (334 lines, deduped): 24 open unique ids, including P1s the brief didn't name: DF-CONSENSUS-2, DF-CONSENSUS-3, QA-CONSENSUS-2 (stale binary on las-02), QA-CONSENSUS-6, QA-CONSENSUS-7, ENV-CONSENSUS-1, MCP-DIRECT-001, MUSTER-INT-001, C-GAP-032-ALPHA. The defect gate (S3) covers them; a gate limited to DF-14/15 would green a run with four other open P1s.
7. **No crier wiring exists in consensus.** `grep -ril crier internal/ cmd/ pkg/` → empty. The brief's T7 premise ("consensus must be able to GET MESSAGES VIA CRIER") is a requirement, not a capability. Confirmed with the board: CR-IN-001 `pending`.

### kimi-k3@neuralwatt
1. Brief fact 8 vs HEAD board: brief states "5 of the 11 sk- keys are DEAD"; commit 77b990b (HEAD) corrects ENV-CONSENSUS-1 to "5 LIVE / 6 DEAD of 11 probed". Evidence: `git log --oneline -1` = `77b990b board: correct ENV-CONSENSUS-1 census — 5 LIVE / 6 DEAD of 11 probed (prior text misreported the split)`. The plan must re-probe all 11 keys on the bunker rather than trust either number — key health is time-dependent, not a constant.
2. docs/API.md carries zero `^(GET|POST|…) /`-prefixed endpoint lines (`grep -E '^(GET|POST|PUT|PATCH|DELETE) ' docs/API.md | wc -l` = 0) — i.e., docs/API.md is NOT the authoritative path list; `specs/openapi/bundled.yaml` (60 paths, verified) is. Any plan section citing API.md as the contract source would test the wrong source. The plan below drives T1 from bundled.yaml only.
3. crier docs: `README.md`/`integration-guide.md` contain no `^GET /`-style endpoint lines either (grep = 0); the crier HTTP surface is authoritative in `docs/openapi.yaml` (14 paths, enumerated below). T7 is written against those 14 paths, not the prose guides.
4. `internal/db/db.go:76` comment claims MaxOpenConns is "Postgres only" while `internal/db/sqlite/sqlite.go:90` consumes it — stale comment, real behavior. A plan step that asserts pool size via the comment would falsify itself; assertion must run against `PRAGMA`/`sqlite3_db` behavior under load, not the comment.
