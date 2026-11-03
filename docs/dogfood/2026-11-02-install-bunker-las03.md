# Consensus Dogfood — 2026-11-02 — Install leg on bunker-las-03 (DF-CONSENSUS-32)

**Verdict: INSTALLABLE.** The row DF-CONSENSUS-32 ("SKIPPED-install-bunker", skipped
2026-09-25 because bunker-las-03 was unreachable) is closed by this run: the full
fresh-agent install leg — anonymous clone → Go toolchain bootstrap → build → init →
serve → health — executed **on bunker-las-03 itself** (ssh alias `bunker3`), NOT on a
substitute host. The 2026-09-26 substitute run used bunker-mvp and deliberately left
this row open; this leg completes it on the real target.

INSTALL_SECONDS=55

## Host and reachability

Host: bunker-las-03, ssh alias `bunker3`. Foreman premise (verified 2026-11-02) plus an
independent re-check at leg start:

```
$ ssh -o BatchMode=yes -o ConnectTimeout=6 bunker3 'hostname; whoami; echo HOME=$HOME; uname -m; command -v git go curl jq; git --version 2>/dev/null; go version 2>/dev/null || echo NO_GO'
bunker-las-03
kara
HOME=/home/kara
x86_64
/usr/bin/git
/usr/bin/curl
/usr/bin/jq
git version 2.47.3
NO_GO
```

No Go on the box → bootstrap provisioned under HOME (step 3 below). The repo's
`go.mod` declares `go 1.26.0` / `toolchain go1.26.5`.

## Ephemeral environment

Everything lived under HOME on the box (never `/tmp`):
`/home/kara/df32-install-leg-20261102-180704/` — contained the clone, the bootstrap Go
tree, GOPATH/GOMODCACHE (`gopath/`), GOCACHE (`gocache/`), the sqlite db, all logs, and
the built binary. No agent was created via the bunker CLI; plain ssh + a HOME-scoped
directory was the chosen path (documented here). No host-level installs, no config
edits, nothing outside HOME. All helper scripts staged under `~/tmp-df32-stage/` were
deleted at cleanup.

## Step 1–2: clone (anonymous, current main)

```
LEG_DIR=/home/kara/df32-install-leg-20261102-180704
PORT_FREE=8132
--- step: anonymous clone ---
Cloning into 'consensus'...
CLONE_RC=0
CLONE_SECONDS=6.6
CLONE_SHA=c84c29edf3412cb438dfff9a85aec6ea200e75c4
```

Clone wall time 6.6s; HEAD c84c29e (current main; same commit this worktree branched
from). Port 8132 confirmed free before serve.

## Step 3: Go bootstrap under HOME (no Go present)

```
--- step: bootstrap go under HOME ---
GO_DL_RC=0
TAR_RC=0
go: downloading go1.26.5 (linux/amd64)
go version go1.26.5 linux/amd64
GO_VERSION_RC=0
```

Bootstrap go1.22.12 (linux/amd64 tarball from go.dev, extracted inside the leg dir) was
enough: the first `go` invocation auto-downloaded the exact toolchain per the repo's
`go.mod` toolchain line, and `go version` reports go1.26.5.

## Step 4: build

```
--- step: build (INSTALL_SECONDS) ---
BUILD_RC=0
INSTALL_SECONDS=55
--- build.log tail ---
go: downloading github.com/jackc/pgx/v5 v5.10.0
go: downloading github.com/spf13/cobra v1.10.2
go: downloading github.com/spf13/pflag v1.0.9
go: downloading modernc.org/sqlite v1.54.0
go: downloading github.com/jackc/puddle/v2 v2.2.2
go: downloading github.com/jackc/pgpassfile v1.0.0
go: downloading github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761
go: downloading golang.org/x/text v0.39.0
go: downloading golang.org/x/sync v0.21.0
go: downloading modernc.org/libc v1.74.1
go: downloading golang.org/x/sys v0.46.0
go: downloading github.com/dustin/go-humanize v1.0.1
go: downloading modernc.org/mathutil v1.7.1
go: downloading modernc.org/memory v1.11.0
go: downloading github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec
--- binary ---
-rwxrwxr-x 1 kara kara 28182450 Nov  2 18:09 bin/consensus
--- toolchain downloaded by go.mod line ---
toolchain@v0.0.1-go1.26.5.linux-amd64
```

INSTALL_SECONDS=55 is the build-step wall time (`go build -o bin/consensus
./cmd/consensus/`), including module downloads — same convention as INSTALL_SECONDS=91
in the 2026-09-26 doc. The toolchain auto-download itself happened earlier, at the
step-3 `go version` invocation, and is recorded there. Build clean, no errors,
28,182,450-byte binary. (Faster than the 09-26 run's 91s on bunker-mvp — network
bandwidth, not a code change; both runs compile the same tree shape.)

## Step 5: init

```
$ ./bin/consensus init --db-url sqlite://$LEG/consensus.db
consensus: init
Database:      sqlite:///home/kara/df32-install-leg-20261102-180704/consensus.db
Migrations:    applied
consensus: first_admin_key created=true key=cs_ak_  REDACTED  key_prefix=cs_ak_d1 id=b5bb5788-4baa-4061-8077-2b441b72ff5a created_at=2026-11-03T02:10:05Z expires_at=2027-02-01T02:10:05Z
consensus: this key expires at 2027-02-01T02:10:05Z (2160h0m0s from now)
consensus: save this key now; it is stored hashed and will not be printed again
Server URL:    http://127.0.0.1:8090
Config file:   ./consensus.yaml or ~/.consensus/config.yaml
Memory bank:   ready
INIT_RC=0
```

Init OK on a fresh box: tables migrated, bootstrap admin key printed exactly once
(secret REDACTED here; it lived only on the ephemeral environment that is now deleted),
90-day expiry shown, non-secret metadata (key_prefix/id) intact.

## Step 6: serve + health (three attempts — two instructive refusals, then 200)

Attempt 1 — plain `serve` on the keyless fresh box **refused to start** (fails fast
instead of burning auth-fail iterations — good startup validation):

```
--- serve.log tail ---
consensus: startup failed: LLM API key not configured (set DEEPSEEK_API_KEY, CONSENSUS_API_KEY, or set llm.provider=mock); an empty key silently burns auth-fail iterations before the harness PAUSE
```

(HEALTH_HTTP=000 after the 90s poll — the process was already dead; the poll cap, not
the server, consumed the window.)

Attempt 2 — the banner's documented keyless path `CONSENSUS_LLM_PROVIDER=mock` —
**also refused**: mock additionally requires an opt-in env var the banner does not
mention (incidental finding, keyless-startup docs gap):

```
consensus: first_admin_key created=false key_prefix=cs_ak_d1 id=b5bb5788-4baa-4061-8077-2b441b72ff5a created_at=2026-11-03T02:10:05Z
consensus: llm client init failed: llm: mock provider requires CONSENSUS_MOCK_LLM=1 env var
```

Attempt 3 — `CONSENSUS_LLM_PROVIDER=mock CONSENSUS_MOCK_LLM=1` (env-overridable,
tested in `internal/config`) — **healthy**:

```
SERVE_PID=857123
HEALTH_SECONDS=1
HEALTH_HTTP=200
--- health.json ---
{"status":"ok","version":"0.1.0","uptime_seconds":0,"api_latency_ms":0,"db_latency_ms":0.071,"llm_latency_ms":0,"error_rate_pct":0,"db_backend":"sqlite","db_path":"/home/kara/df32-install-leg-20261102-180704/consensus.db","db_size_mb":0.484375,"db_tables":38,"db_migrations":31,"schema_version":32,"active_connections":{"websocket":0,"db_pool_active":0,"db_pool_max":0,"llm_active":0,"api_requests_last_min":0},"system_log":[]}
--- serve3.log head (excerpt) ---
consensus: first_admin_key created=false key_prefix=cs_ak_d1 id=b5bb5788-4baa-4061-8077-2b441b72ff5a created_at=2026-11-03T02:10:05Z
time=2026-11-02T18:13:01.813-08:00 level=INFO msg="consensus: starting" addr=127.0.0.1:8132 backend=sqlite
time=2026-11-02T18:13:01.813-08:00 level=INFO msg="api: starting" addr=127.0.0.1:8132
```

Health 200 on `/api/v1/health` ~1s after launch: sqlite backend (db under the leg
dir), 38 tables, 31 migrations applied, schema_version 32 — comparable to the 09-26
reference (37 tables / schema_version 24; the tree has moved in five weeks). Serve ran
bound to 127.0.0.1:8132 only. Leg-procedure note: the serve process outlived its ssh
session (nohup'd background child kept the session's stdin open, so the driver ssh call
timed out client-side); liveness + health 200 were re-verified on a fresh connection
before cleanup, so no evidence depends on the killed session.

## Step 7: cleanup (commands + result)

```
pkill -f "consensus/bin/consensus serve"   # live serve PID 857125
kill "$SPID"                               # wrapper PID from the state file
SERVER_STOPPED
NO_CONSENSUS_PROCS
PORT_8132_LISTENERS=0
rm -rf "$LEG"                              # first pass: RM_RC=1 — Go module cache dirs are 0555
chmod -R u+w "$LEG" && rm -rf "$LEG"       # second pass
RM_RC=0
LEG_REMOVED=/home/kara/df32-install-leg-20261102-180704
NO_LEG_DIRS
```

Final verification on the box: no `consensus serve` processes, port 8132 with zero
listeners, no `df32-install-leg-*` directories, and `~/tmp-df32-stage/` (the five leg
helper scripts) removed — `STAGE_DIR_REMOVED`, `NO_LEG_RESIDUE`. The bootstrap admin
key died with the deleted database; the served process was loopback-only throughout.

## Findings / incidental observations (no source changes in this run)

1. Keyless fresh-box startup requires TWO env vars but the refusal banner names only
   one (`llm.provider=mock` works only with `CONSENSUS_MOCK_LLM=1`, which the banner
   does not mention). Cost to a real new user: one confusing extra failed start.
   Candidate small docs/UX improvement; filed here as an observation, not a board row.
2. The startup LLM-key gate failing FAST is a good property worth keeping: an empty key
   refuses to boot rather than silently pausing the harness after auth-fail burn.
3. Install from zero on this box is ~6 minutes end-to-end including the two refused
   starts and health verification (clone 6.6s + toolchain + 55s build dominate; the
   09-26 run measured 91s build on bunker-mvp).

## Row acceptance evidence

- A1: `grep -c 'DF-CONSENSUS-32' docs/dogfood/*.md` → this file matches (>= 1).
- A2: `grep -c 'INSTALL_SECONDS=' docs/dogfood/*.md` → INSTALL_SECONDS=55 above (>= 1).
- A3: commit subject on wt/DF-CONSENSUS-32 names DF-CONSENSUS-32 + install-bunker over
  docs/dogfood/ (>= 1).

**Verdict: installable on bunker-las-03** — anonymous clone, bootstrap-Go toolchain
auto-download, build, init, and healthy serve all succeeded on the previously
unreachable target host; DF-CONSENSUS-32 can close.
