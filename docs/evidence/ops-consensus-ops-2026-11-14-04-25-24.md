# Ops tick — consensus-ops-2026-11-14-04-25-24

Lane: `consensus-ops` (coding-hermes ops lane). Selected row: **QA-CONSENSUS-8** (P3,
docker-deploy observability). Theme: prove the declared deploy surface can answer the
health probe the QA battery uses.

Date: 2026-11-14 04:25–04:35 local (2026-11-13T23:25–23:35Z). Version on disk: `v0.1.0`.
Repo HEAD at measurement: `c96ff4cd84b785363a7c2aaab677cbd43e6e44f0`, working tree clean.

## 1. Why this row

The board had 9 pending rows. QA-CONSENSUS-8 is the only docker-deploy observability row
and its own text names the open question: "compose up succeeded on a clean machine but the
health probe read 000". Everything else pending is QA/perf/review-lane work (QA-CONSENSUS-9,
QA-CONSENSUS-12, QA-CONSENSUS-25, QA-CONSENSUS-26, QA-CONSENSUS-30, QA-CONSENSUS-31,
QA-CONSENSUS-REVIEW-1) or product follow-ups (BUG-010, BUG-011).

## 2. Static acceptance arms (re-measured, not inherited)

### arm1 — services enumerate

    $ docker compose -f docker-compose.prod.yml config --services
    postgres
    consensus
    rc=0                                 # arm1_postgres=1 arm1_consensus=1

### arm2 — the declared surface publishes the port the battery probes, and checks it

    $ docker compose -f docker-compose.prod.yml config
    rc=0
    arm2_pub8090=1                       # grep -c 'published: "8090"'
    arm2_healthpath=1                    # grep -c 'api/v1/health'

### arm3 — deploy/README.md names that file as the battery surface

    $ grep -n 'docker-compose.prod.yml' deploy/README.md
    7:... The production Docker deployment is `docker-compose.prod.yml`; it is the only
       Compose file in this repository that starts both the `postgres` dependency and the
       `consensus` runtime. From the repository root, prepare a `.env` file containing the
       required `DEEPSEEK_API_KEY` and `CONSENSUS_API_KEY` values, then run:
    10:docker compose -f docker-compose.prod.yml up -d --wait
    17:docker compose -f docker-compose.prod.yml config --services
    18:docker compose -f docker-compose.prod.yml config
    23:To stop this stack without deleting named volumes, run `docker compose -f
       docker-compose.prod.yml down`. Do not use `down -v` unless the persisted database and
       Consensus data are intentionally being discarded.
    rc=0

All three arms pass on the current tree.

## 3. Live arm — the surface actually answers (this is what the row really asked)

Static arms cannot answer "probe reads 000". A live boot was run on the control host with
an **isolation override** (fresh volumes, alternate DB port, probe-specific container names,
`restart: "no"`) so nothing shared with other projects was touched. Note: host port 5432 is
already bound by another project, so the base file's `127.0.0.1:5432:5432` cannot bind here;
the override moves postgres to `127.0.0.1:15432` with `ports: !override` (a plain list in an
override **merges**, and the first attempt failed with
`failed to bind host port 127.0.0.1:5432/tcp: address already in use`).

Environment supplied was exactly the documented one — `DEEPSEEK_API_KEY` and
`CONSENSUS_API_KEY` only, no `POSTGRES_PASSWORD`:

    $ docker compose --env-file probe.env -f docker-compose.prod.yml \
        -f probe-override.yml -p consensus-opsprobe up -d --wait --wait-timeout 150
    Container consensus-opsprobe-postgres  Started
    Container consensus-opsprobe-postgres  Healthy
    Container consensus-opsprobe-runtime   Started
    Container consensus-opsprobe-runtime   Healthy
    boot_rc=0

    $ docker ps --filter name=consensus-opsprobe
    consensus-opsprobe-postgres   Up (healthy)   127.0.0.1:15432->5432/tcp
    consensus-opsprobe-runtime    Up (healthy)   0.0.0.0:8090->8090/tcp, [::]:8090->8090/tcp

    $ curl -s -o health.json -w 'http_code=%{http_code} time=%{time_total}\n' \
        http://127.0.0.1:8090/api/v1/health
    http_code=200 time=0.003034
    curl_rc=0

    {"status":"ok","version":"0.1.0","uptime_seconds":10,"api_latency_ms":0,
     "db_latency_ms":0.27,"llm_latency_ms":0,"error_rate_pct":0,
     "db_backend":"postgres","db_path":"consensus","db_size_mb":11.36,
     "db_tables":40,"db_migrations":28,...}

    $ docker inspect consensus-opsprobe-runtime --format '{{json .State.Health}}'
    {"Status":"healthy","FailingStreak":0,
     "Log":[{"ExitCode":0,"Output":"{\"status\":\"ok\",...}"}]}

The Compose healthcheck defined in the file
(`curl -fsS --connect-timeout 5 --max-time 8 http://localhost:8090/api/v1/health`)
itself exits **0** inside the container — that is the probe the row asked to "prove it".

Runtime log confirms a real postgres backend, not a degraded fallback:

    msg="consensus: postgres LISTEN/NOTIFY event bridge started"
    msg="postgres: notification listener: LISTENing" channel=session_events
    msg="postgres: notification listener: LISTENing" channel=approval_events
    msg="consensus: starting" addr=0.0.0.0:8090 backend=postgres
    $ docker logs ... | grep -ci 'password authentication failed'   -> 0

## 4. Findings

1. **QA-CONSENSUS-8's symptom does not reproduce at `c96ff4c`.** `up -d --wait` returns 0,
   both services reach healthy, and `/api/v1/health` answers **200** in ~3 ms well within the
   healthcheck's `start_period: 30s`. The "probe 000" recorded on the clean machine was a
   QA-harness timing/port question, not a product defect on the declared surface.
2. **The db-url is not a hardcoded credential.** `docker-compose.prod.yml` renders the
   runtime's `--db-url` password as `***` in tool output; that is the harness redaction layer,
   not the file. The file interpolates the same variable the postgres service uses
   (`grep -c 'POSTGRES_PASSWORD:-consensus}@postgres'` = 1), so a documented deploy is
   internally consistent — which the live boot confirms.
3. **Port 5432 is already taken on this host.** Any attempt to run the base file unmodified
   on the control host fails at container start. Host-specific, not a repo defect; worth
   knowing before anyone debugs "compose up won't bind".

## 5. What was changed

Nothing in the product. This was a verification tick: the row's acceptance is satisfied by
measurement, and no bounded defect remained to fix. Only this evidence file was added.

## 6. Deliberately left alone

- **The probe stack is still running** (`consensus-opsprobe-postgres`,
  `consensus-opsprobe-runtime`, volumes `consensus-opsprobe-{data,pgdata}`, network
  `consensus-opsprobe-net`). The teardown command was submitted and **blocked by the
  unattended consent gate** — silence is not consent, so it was not retried or worked around.
  Safe teardown (per deploy/README.md line 23, volumes preserved): 

      docker compose --env-file probe.env -f docker-compose.prod.yml \
        -f probe-override.yml -p consensus-opsprobe down

  Both containers carry `restart: "no"`, so they will not come back after a daemon restart.
  The stack listens on 0.0.0.0:8090 guarded by the probe-only `CONSENSUS_API_KEY`.
- **Pre-existing volumes `consensus-data`, `consensus-pgdata`, `consensus_pgdata`** were not
  touched, inspected destructively, or removed. They are residue from earlier runs
  (explicitly named in the file, so a compose project name does not isolate them); the probe
  used its own volumes instead.
- **No service was restarted.** Nothing was running when the tick started (no consensus
  containers, no `:8090` listener).
- **No image was rebuilt and no deploy was performed.** The probe used the published
  `ghcr.io/totalwindupflightsystems/consensus:latest` (50.1 MB).

## 7. Incidental observations (not filed on this row, for the next tick)

- `docker-compose.prod.yml` warns "Anonymous GHCR pulls currently return 401: run
  `docker login ghcr.io` first". On this host `~/.docker/config.json` has `auths: {}` (no
  credHelpers, no credsStore) and both `docker manifest inspect` and the actual image pull
  **succeeded anonymously**, so that comment no longer matches observed behaviour. Not fixed
  here — it is outside QA-CONSENSUS-8's acceptance.
- On boot the runtime logs `modelsync: initial sync failed error="decode models.dev response:
  invalid character '<' looking for beginning of value"` — models.dev answered with an HTML
  error page instead of JSON. Auto-sync retries every 24h; no gate depends on it, but the
  initial catalog sync currently never lands.
