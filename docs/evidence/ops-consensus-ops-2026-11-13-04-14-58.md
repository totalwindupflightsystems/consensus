# consensus-ops tick evidence

Tick: consensus-ops-2026-11-13-04-14-58
Selected row: QA-CONSENSUS-8
Project root: /home/kara/totalwindupflightsystems/consensus
Published ref measured: origin/main at 2423bb34b89ed0298565cad69344fd46deb81210

## Board and ownership

The published row `QA-CONSENSUS-8` was present exactly once and remained `pending`. Its acceptance requires the production Compose file to render `postgres` and `consensus`, publish host port 8090 once, include the `/api/v1/health` healthcheck, and have `deploy/README.md` name `docker-compose.prod.yml` as the battery surface.

The remote board had no event containing either the scheduler tick slug `consensus-ops-2026-11-13-04-14-58` or its ISO start timestamp `2026-11-13T04:14:58Z` before this tick's write leg.

## Read-only live probes

Command:
`sqlite3 -readonly ~/.hermes/coding-hermes/scheduler.db "select name,repo_url,workdir,enabled,worker_model,worker_provider,last_tick_started,last_tick_completed,last_tick_status from projects where name='consensus-ops';"`

Output:
`consensus-ops|local:/home/kara/totalwindupflightsystems/consensus|/home/kara/.hermes/stand-in/pm-lane/consensus|1|openai/gpt-6.1-terra|xkiro|2026-11-12T23:14:59-05:00|2026-11-11T18:01:17-05:00|failed`

The registered path was a stand-in, so probes and the change ran in the resolved canonical checkout `/home/kara/totalwindupflightsystems/consensus`. Remote: `origin git@github.com:totalwindupflightsystems/consensus.git`. The isolated worktree was based on `origin/main` at `2423bb34b89ed0298565cad69344fd46deb81210`.

Command:
`sqlite3 -readonly ~/.hermes/coding-hermes/scheduler.db "select id,project_name,status,spawned_at from ticks where status='running';"`

Output included:
`consensus-ops-2026-11-13-04-14-58|consensus-ops|running|2026-11-12T23:14:58-05:00`

The current ops tick was in flight; no service restart or container mutation was attempted.

Command:
`curl -sS --max-time 10 http://127.0.0.1:9090/api/v1/projects/consensus-ops`

Output summary:
`latest_tick.status=running`, `latest_tick.id=consensus-ops-2026-11-13-04-14-58`, `project.enabled=true`, `project.worker_model=openai/gpt-6.1-terra`, `project.worker_provider=xkiro`, `project.last_tick_status=failed`.

Command:
`ss -ltnp '( sport = :8090 or sport = :5432 )'`

Output:
`LISTEN 127.0.0.1:5432`; no listener was present on port 8090.

Command:
`curl -sS -o /tmp/consensus-health-ops-20261113.txt -w 'http_code=%{http_code} time_total=%{time_total}\\n' --max-time 5 http://127.0.0.1:8090/api/v1/health`

Output:
`curl: (7) Failed to connect to 127.0.0.1 port 8090 after 0 ms: Could not connect to server`; `http_code=000`; `curl_rc=7`.

This control host had no running production Consensus stack. No `docker compose up`, restart, stop, delete, or cleanup was attempted.

## Acceptance re-check and bounded change

Before the change, the exact row arm failed:

Command:
`grep -n 'docker-compose.prod.yml' deploy/README.md`

Output:
no output; `rc=1`.

The production Compose contract already passed its static arms:

Command:
`docker compose -f docker-compose.prod.yml config --services`

Output:
`postgres` and `consensus`; `rc=0`.

Command:
`docker compose -f docker-compose.prod.yml config`, followed by `grep -c 'published: "8090"'` and inspection of the healthcheck.

Output:
`config rc=0`; `published_8090=1`; healthcheck line was `curl -fsS --connect-timeout 5 --max-time 8 http://localhost:8090/api/v1/health || exit 1`.

Bounded change applied in this isolated project worktree: `deploy/README.md` now names `docker-compose.prod.yml` as the production Docker Compose surface, documents the `up -d --wait` and health probe, records the static config checks, and warns not to use `down -v` unless data deletion is intentional. No scheduler or host state was changed.

Post-change exact arms:

Command:
`docker compose -f docker-compose.prod.yml config --services`

Output:
`postgres` and `consensus`; `rc=0`.

Command:
`docker compose -f docker-compose.prod.yml config`, followed by `grep -c 'published: "8090"'` and healthcheck inspection.

Output:
`config rc=0`; `published_8090=1`; healthcheck contains `/api/v1/health`.

Command:
`grep -n 'docker-compose.prod.yml' deploy/README.md`

Output:
multiple matching lines including the new production-surface instructions; `rc=0`.

These static acceptance arms pass. Live health remains unproven because no production stack is running on the control host and the current ops tick was in flight; the row remains pending rather than being closed.

## Deliberate non-actions

No service was restarted because the active scheduler tick and other project ticks were in flight. No container stack was started because that would be an external deployment mutation and no failure diagnosis authorized it. No volumes, images, scheduler database, lanes, or unrelated repositories were modified.
