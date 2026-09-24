# Dogfood Diagnostic Trail — 2026-09-09 run

Not raw logs — this is how the run was built, what each probe ruled out, and
the right way to reproduce or refute every claim.

## Why this run existed

Consensus was dogfooded 2026-08-04, 08-15, 09-03, and 09-04. Nine
`DF-CONSENSUS-*` tasks landed on the board, including two P0s
(DF-CONSENSUS-6: conversational path never reaches the LLM; DF-CONSENSUS-7:
no install path works from zero). All nine were still `pending` on 09-09 —
the foreman had not fixed them. The cron re-picked the project, so this run
was a **fix-verification pass**: re-test the P0s at HEAD `8e3e7e6`, confirm
or clear them, and bisect anything new.

## Run architecture

- Build: `go build -o /tmp/dogfood-consensus/consensus ./cmd/consensus` (2 s).
  Never tested against the repo's `bin/` — a fresh build is the honest surface.
- Isolation: every instance got its own SQLite file (`scratch*.db`) and port
  8124 (8090/8095 are occupied on this host by other fleet services — a
  standing landmine from the 09-01 run, still true).
- LLM key: sourced from `/home/kara/deepseek-harness/.env`. Gotcha discovered
  mid-run: this host's cron shell exports `DEEPSEEK_API_KEY` as **empty** —
  the first server boot warned "No LLM API key configured" despite the env
  var being "set". Verify the key inside the server process
  (`/proc/<pid>/environ`) before believing any LLM-path verdict. The 09-04
  "invalid key wedges the API" finding has a cousin here: a silent empty key
  produces a confusing WARN, not a hard fail, at startup.
- Each leg ran in a background process with logs to `server*.log`; every
  instance was destroyed after its leg (kill -9 + pgrep sweep + port check).

## Probe log — what each step ruled out

1. **Health/init/serve** (no auth): PASS. 37 tables, schema 23, sqlite,
   correct scratch DB path. Rules out "the project doesn't build/boot".
2. **Documented session create** (verbatim docs/API.md body): 400
   `agent_name is required`. Third consecutive dogfood hitting
   DF-CONSENSUS-2. Not re-litigated; re-confirmed and moved on.
3. **Documented message POST** on env-var instance: 200
   `message_received` in 9 ms → then the wedge (below). This single
   observation motivated the whole bisect.
4. **Wedge forensics**: server log shows the 3-minute planning window, then
   `begin tx: context deadline exceeded`, then heartbeat re-pick. API
   behavior during wedge: valid-key GET → 401, no-auth health → timeout.
   `/debug/pprof/goroutine` → 404 (no pprof mounted) — worth adding in the
   product; it would have made this diagnosis 10× faster.
5. **Crash-recovery A/B**: kill -9 during wedge → `PRAGMA integrity_check`
   ok → restart → heartbeat resumed the session and **re-wedged**. This
   kills the "restart clears it" recovery story: the poison-pill session
   persists in the ledger (by design — ACID) and the resume logic (by
   design — heartbeat) re-enters the wedging path every boot. Two correct
   features compose into an unbootable instance.
6. **Environment isolation**: fresh `scratch2.db`, same wedge → rules out
   dirty state from leg 1. One leftover `consensus serve` from a QA lane
   (port 8103, started 09-07) was present the whole time but never shared
   my DB file or port — checked and ruled out as a factor.
7. **Config-file leg** (`max_open_conns: 4`, fresh `scratch3.db`): the exact
   same goal-driven workflow ran clean end-to-end with real LLM calls.
   This is the A/B that pins the default-pool as the DF-CONSENSUS-10 root
   cause. It also retroactively explains why the 09-03 recipe "worked":
   it shipped its own config file with the pool pinned — the pin was doing
   the work, not the env-var path the README documents.
8. **Conversational A/B** (`scratch4.db`, pool pinned): `role:user` message
   → stored in memory_events → LLM call `messages=2` → model replies "no
   user request present". Clean isolation: the prompt builder ignores user
   turns, independent of the pool bug. This upgrades DF-CONSENSUS-6 from
   "verified broken twice" to "root-caused with a controlled experiment".

## How the diagnosis chain worked (transferable method)

- Never trust an env var — verify inside the target process.
- One variable per leg; every leg gets a fresh DB and its own log file.
- When "it worked last time", diff the *recipe*, not just the code: the
  09-03 recipe carried a hidden fix (pool pin) that its author didn't
  flag as load-bearing.
- A feature composed of two individually-correct mechanisms (durable
  ledger + auto-resume) needs a poison-pill test: poison a state, restart,
  ask "does it ever come back?".
- pprof 404 during a live wedge is itself a finding — observability for
  the wedge class is absent.

## History cross-reference

- 09-04 finding "invalid LLM key wedges the entire API" — same observable
  (401-to-valid-key, health freeze), different trigger (bad key vs none at
  all vs plain happy path). One underlying pool/tx defect likely explains
  all three; DF-CONSENSUS-10 detail lists it as the parent bug.
- 09-03 DF-CONSENSUS-8 (crash recovery recovers little durable output) —
  unchanged; tokens/iterations/commits all 0 after wedged runs here too.
- Anonymous clone worked at HEAD (half of DF-CONSENSUS-7 fixed sometime
  between 09-04 and 09-09 — repo visibility was NOT changed by this run;
  verified with all credential helpers stripped).

## Evidence file map (scratch, transient)

`/tmp/dogfood-consensus/` — `server.log`/`server2.log` (wedged legs),
`server3.log` (clean pool-pinned leg), `server4.log` (conversational A/B),
`scratch*.db` (state snapshots incl. `memory_events` rows quoted in the
integration report). Ephemeral by design; the durable copy of every claim
lives in this directory and on the board rows.
