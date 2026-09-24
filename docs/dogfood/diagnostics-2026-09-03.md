# Diagnostics Addendum — 2026-09-03 dogfood run

Third dogfood pass (after 2026-08-04 and 2026-08-15). This addendum explains
what the 2026-09-03 run established about *why* the system behaves the way it
does, for whoever picks up DF-CONSENSUS-6..9. Read the main trail above for
architecture; only deltas are recorded here.

## What was proven working this run (do not re-litigate)

- **Bootstrap:** fresh scratch DB → `init` prints the one-time admin key with
  90-day expiry exactly as documented; restart prints `created=false` +
  prefix only (key is hashed, unrecoverable by design).
- **Goal-execution loop:** session whose `goal` describes an executable task,
  woken with `{"type":"user_instruction"}` → planning loop stages SQL, the
  transaction executes ACID-committed, the memory event lands, session goes
  `idle`. This is the DEMO code path (`demo/demo_test.go`) and it works with
  real DeepSeek calls.
- **Crash recovery:** SIGKILL mid-planning-run → restart on same sqlite file
  → health 200, session row intact, heartbeat auto-resumes planning. The
  append-only ledger held everything written before the kill.
- **Infra surface:** Bearer auth on every route, `/mcp/sse` handshake,
  `/doc/api` Swagger UI, embedded `/openapi.json` (CWD-independent now),
  Chronicle at `/chronicle/`.

## Why the conversational path is dead (DF-CONSENSUS-6) — mechanism

Evidence triangulated from four independent angles:

1. **Server log:** every LLM call in every run logs
   `messages=2` — the request body sent to DeepSeek is the same size on turn
   1 and turn 10. Session memory (where the user message IS stored, verifiably,
   via `GET /memory`) is never assembled into the prompt.
2. **The model's own words:** during the A/B run the model staged and
   committed a `thinking` memory event reading
   `"No user-supplied text is present in this turn - the transaction window is
   empty."` It is not refusing; it genuinely received nothing.
3. **Error black hole:** `planning: staged command failed ... no such column:
   id` at turn 1, then the model repeats structurally identical SQL for 10
   turns — staged-command errors are never appended to the next prompt, so
   from the model's perspective nothing ever fails.
4. **Zero durable output:** `tokens_used_in/out` stay 0 on the session row,
   `agent_billing`/`audit_logs`/`iteration_commits` stay at 0 rows after ~30
   real API calls (the provider metered them; consensus didn't). After
   `max turns reached, auto-committing`, the session transitions to `idle`
   and the API caller has no signal that anything failed.

Consequence: the A/B split — identical infrastructure, only the payload
differs — localizes the defect to **prompt assembly for the interactive
message path** (and the turn-result feedback loop), not to the LLM layer,
the ledger, or the transactional staging machinery, all of which work in the
goal-driven path.

## Why the quickstart fails a fresh user (DF-CONSENSUS-7)

- `ghcr.io/v2/wojons/consensus/manifests/latest` returns 401 to anonymous
  token requests; an authenticated-less `docker pull` gets `denied` while
  `alpine:latest` pulls fine from the same agent — the image exists but is
  private (or the package was never published).
- `git clone https://github.com/wojons/consensus` prompts for credentials
  from a clean agent — the repo is private. The goreleaser config exists
  (`.goreleaser.yaml`) but nothing public ships from it today.
- Honest fixes, in order of preference: publish the image from CI on tag;
  or say in the README quickstart that both paths need auth and show the
  `docker login` / git-auth steps. **Never** widen repo/package visibility
  as a workaround (hard rule).

## Fresh-machine install mechanics (what the docs don't say)

- A bare Debian agent has no Go and no sudo. Bootstrap order that worked:
  `curl -sSL go1.26.5.linux-amd64.tar.gz | tar -C $HOME -xzf -` → set
  `GOROOT=$HOME/go` (warning: default `GOPATH=$HOME/go` collides with the
  extracted GOROOT — point `GOPATH` elsewhere) → `go build` = 61 s cold.
- bunker-managed agents: bunkerd starts its own rootless dockerd with
  `--state-dir=/run/user/<uid>/dockerd-rootless` and socket
  `/run/bunker/<agent>/docker.sock`. The shipped systemd *user* unit then
  cannot start (`failed to lock ... another RootlessKit is running with the
  same state directory`) and enters a restart loop — harmless if you use the
  bunkerd socket, confusing as hell if you follow `systemctl --user start
  docker` habit. Either bake DOCKER_HOST into the agent env or document the
  socket location.
- bunker-las-03 was unusable this run: `port range allocation: no free port
  ranges available (pool exhausted: 10 ranges)` with zero agents listed =
  orphaned in-memory ranges (bunkerd restart clears them; skipped —
  interactive approval unavailable in cron). Same class as
  `qa-foreman-ops/references/spawn-pool-exhaustion-2026-09-03.md`.

## Operational notes for the next runner

- `pkill -f "consensus serve"` self-matches the invoking shell (the pattern
  appears in the shell's own command line) — kill by binary path or PID file;
  same trap as recorded in the 2026-08-04 trail.
- Heartbeat auto-resume means a session you leave in `planning` will keep
  making LLM calls (~10 turns × ~1.7k prompt tokens each) until it exhausts
  max_turns or the 3-minute window. For scratch runs: PATCH status to
  `pause`/`cancel` before walking away, or it burns tokens in the background.
- sqlite health probe: `python3 -c "import sqlite3; ..."` with
  `file:...?mode=ro` — never open the live DB read-write while a server owns
  it (state.db corruption lesson from the wider fleet applies here too).
- `staging_buffer` accumulates stranded rows across abnormal exits (74 at
  audit time). It is the first place to look when reconstructing what a run
  *tried* to do; nothing prunes it.
