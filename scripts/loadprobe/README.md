# consensus-loadprobe

Postgres backend load probe for the Consensus runtime (REVIEW-CONSENSUS-3).

Drives N concurrent agent sessions through the real REST API and measures the
per-turn write path (auth lookup + rate-limit accounting + `memory_events`
INSERT + `sessions` UPDATE) that every production agent turn exercises.

## Build

```bash
go build -o /tmp/loadprobe ./scripts/loadprobe
```

The probe is a standalone `package main` in the Consensus module and imports
only the standard library, so `go build ./...` and `go vet ./...` cover it.

## Run

```bash
/tmp/loadprobe \
  --base-url http://127.0.0.1:8090 \
  --admin-key cs_ak_... \
  --concurrency 1,4,16,32 \
  --turns 200 \
  --backend postgres \
  --out /tmp/loadprobe.json
```

For each `--concurrency` level the probe:

1. Creates N sessions concurrently via the admin key (`POST /api/v1/sessions`).
2. Has each session send `--turns` messages concurrently with its own
   session-scoped key (`POST /api/v1/sessions/{id}/message`).
3. Records every turn's latency and HTTP status.

Output is a single JSON document: per-level aggregate metrics (sessions
created, total/successful/error turns, error rate, throughput in turns/sec,
p50/p95/p99 latency) plus the full raw per-turn latency sample set.

## What it measures

| Metric | Definition |
|---|---|
| `throughput_turns_per_sec` | total turns / wall-clock duration of the turn phase |
| `latency_ms.p95` | 95th percentile of per-turn end-to-end latency |
| `error_rate` | turns with HTTP status >= 400 / total turns |
| `session_create_errors` | sessions that failed to create (recorded separately) |

Session creation failures are recorded distinctly from turn errors: if zero
sessions can be created, the level reports `error_rate: 1` with
`session_create_error` carrying the backend's exact error, so a backend that
rejects the write path is visible in the raw output rather than silently
reading as "zero throughput".

## Backend notes

- **Postgres**: run `docker compose up -d` (or the pgvector image — migration
  001 requires the `vector` extension) and boot the server with
  `--config` pointing at a config whose `database.url` is a `postgres://` DSN.
- **SQLite control**: point `--base-url` at a server booted on a scratch
  `sqlite://` DB to prove the probe itself produces real metrics on a healthy
  backend.
