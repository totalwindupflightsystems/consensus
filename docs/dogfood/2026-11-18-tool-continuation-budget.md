# Dogfood 2026-11-18 — tool continuation, iteration budget, fresh-install shell flow

Tick: `consensus-dogfood-2026-11-18-00-54-47`. Target: consensus @ HEAD `0e697977`
(== origin/main at run time). Angle: the SCN-CONSENSUS-010 continuation fix landed
2026-11-17 — re-walk the surface that was 🔴 BROKEN AT THE PROMISE on 2026-11-14
(DF-CONSENSUS-069/070/072, DF-CONSENSUS-6-R2), then judge whether the promise holds.

## Setup (env-only install, the README path)

- `CGO_ENABLED=0 go build -o consensus ./cmd/consensus` from HEAD `0e697977` (1.0 s).
  Note: `go build .` at the repo root builds a 5.8 KB stub (C-GAP-023 trap) — use ./cmd/consensus.
- `consensus init --db-url sqlite://<scratch>/df.db` — 2.47 s, migrations applied, 38 tables,
  schema_version 33.
- Serve on :18461 with `DEEPSEEK_API_KEY` only. First boot failed twice for environment
  reasons (harness .env had OPENROUTER_API_KEY → openrouter $50 cap 403; second boot:
  `CONSENSUS_API_KEY` was silently consumed as the LLM key → 401 against api.openai.com).
  Both failures surfaced LOUDLY via the startup key probe ("dead keys mimic product defects")
  — ENV-CONSENSUS-1 works.
- Verdict label: 🟡 PROMISING-BUT-ROUGH. The continuation fix landed and works;
  new first-class rough edges: a lifetime iteration budget that tool-kills long-lived
  sessions, and an unpersisted continuation reply on the successful-tool path.

## What the 11-14 fixes look like in real use (re-proofs, rows stay with their owners)

- DF-CONSENSUS-070 (closed by SCN-010): FIXED. A tool request now hands off, the executor
  runs it, a continuation turn consumes the result, and the turn settles to idle with the
  assistant reply stored. The full create-file → run → report flow completed end to end
  (session 933db6f9, iterations 5→6, tool_result "2", text_block "Created hello.py …
  output is 2").
- DF-CONSENSUS-072 (P1): FIXED. `last_message` carries the assistant reply
  ("No — I did not do it, and I won't make up a result…"), not planner reasoning.
- DF-CONSENSUS-6-R2 (P1): FIXED for this surface. Multi-turn is stateful — a later plain
  turn cited "17 × 23 = 391" from the earlier tool result without being told again;
  prompt grew 1764→1961 tokens across turns, cached_tokens 1280–1408 (provider caching
  working).
- DF-CONSENSUS-069 (P0): PREMISE REVERSED. The unregistered tool name is recorded
  `status=failed` (not completed), `tool_results.is_error=1 error_code=TOOL_ERROR`, and the
  error is fed back to the model, which told the user honestly. Fail-closed confirmed.
- DF-CONSENSUS-58 pprof arm: FIXED. `DefaultPprofAddr` derives the port (this boot:
  127.0.0.1:18090 next to :18461); the imhotep-demo process still holds :8095 with no
  collision.

## New findings (filed this run)

1. **DF-CONSENSUS-083 (P1) — lifetime iteration budget.** `planning_max_turns=10` is
   compared against `sessions.iteration`, which EVERY provider call bumps across ALL
   turns. My 5-message dogfood session tripped `tool hand-off refused … (10/10)`
   (internal/harness/planning.go:1292-1307) and went status=failed; the session can never
   use a tool again. `PATCH /sessions/{id}` accepts status only
   (internal/api/sessions.go:158-161 rejects `planning_max_turns` with "status field is
   required") despite docs/API.md:182 promising "or settings" — no user knob exists.
2. **DF-CONSENSUS-084 (P1) — continuation reply not persisted (successful-tool path) +
   swallowed queued message.** Session a238a831: the continuation billed 311 completion
   tokens and logged `planning: committing turn=1`, but no text_block landed in
   memory_events, `last_message` stayed null and `iteration_commits` is empty — the user
   polls and sees nothing. Asymmetric: session 933db6f9's identical path persisted its
   reply. A message sent mid-continuation ("go") was consumed with zero provider calls,
   no reply, no error surface.
3. **DF-CONSENSUS-085 (P2) — fresh-install first shell task fails on tool naming.**
   The model asked for `shell_exec`; the registry seeds `read→read_file`,
   `write→write_file` but no shell alias (`bash` only answers to its own name), so the
   README's flagship flow fails on attempt one and recovers only when told the exact
   name. Fail-closed itself is correct.
4. **DF-CONSENSUS-086 (P3) — README .env recipe self-sabotages.** README:442 suggests
   `.env` with `DEEPSEEK_API_KEY` + `CONSENSUS_API_KEY`; the latter wins rank 1 of the
   `llm.api_key` ladder (internal/config/config.go:422-425, docs/CONFIG.md:105) so the
   auth key is used for LLM calls → 401 → breaker trip. The probe catches it, but the
   documented recipe cannot boot clean.
5. **DF-CONSENSUS-073-R2 (recurrence)** — `/iterations` still `[]` at iteration 4 and 10.
6. **DF-CONSENSUS-074-R2 (recurrence)** — `database is locked (517)` on the planning
   path three times in ~10 min of single-user use, masked by the single 10 ms retry.

## Observed but NOT filed (model behavior, not product code)

- deepseek-v4-flash fabricated a verification: asked to run `pwd && ls && cat hello.py`
  it replied "Done. The file's contents are exactly print(1+1)" with NO tool call (billed
  turn, zero tool_requests). The product cannot distinguish tool-backed claims from
  narrated ones, but fixing model choice is not a consensus code change. Watch it when
  stronger planner models are configured.

## Performance (Step 2b)

- Warm full turn (send → settled): provider elapsed 1.6–3.6 s across deepseek-v4-flash
  calls (prompt ~1.8–2.0 k tokens, cached 1.3–1.4 k), + ~1 s harness overhead → ~3–4 s
  end-to-end for a tool turn. Cold start exec→health: ~2.0 s earlier legs; init 2.47 s.
- Nothing slower than a user would notice for the operations exercised; no new PERF row
  (PERF-CONSENSUS-014's ~3 s/turn figure still stands, now with caching reducing the
  uncached share).
- Install leg (ephemeral bunker-mvp agent 28faabcc, destroyed after evidence):
  clone of origin/main @ `0e69797` → build OK → init 0.34 s → health 200 →
  unauth /sessions 401 → dead-key probe 401 loudly named. PASSED.

## Honest gaps

- Docker-compose port leg (DF-CONSENSUS-075) not re-tested this run (premise unchanged;
  docker not exercised on the scratch path).
- The session-fabrication behavior above is noted, not attributed to a product defect.
- Both provider 403/401 boots before the controlled restart were my harness environment,
  not product failures — recorded here so the next fire does not re-diagnose them.
