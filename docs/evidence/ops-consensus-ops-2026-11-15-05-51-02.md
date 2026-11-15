# Ops tick — consensus-ops-2026-11-15-05-51-02

Row: **DF-CONSENSUS-075** (P3) — *docker compose up fails hard on any host where 5432 is
already bound: docker-compose.yml publishes the postgres port on a fixed host port with no
override.*

Base: `main` @ `b2057132` (local `HEAD` == `origin/main` at tick start, working tree clean).
All measurements below were taken on this control host on 2026-11-15.

---

## 1. Premise is live on this host, not hypothetical

`ss -ltn 'sport = :5432'`:

```
State  Recv-Q Send-Q Local Address:Port Peer Address:Port
LISTEN 0      200        127.0.0.1:5432      0.0.0.0:*
```

`systemctl is-active postgresql@18-main.service` → `active`
`systemctl show -p ActiveEnterTimestamp --value postgresql@18-main.service` →
`Mon 2026-10-19 15:42:47 -05`

A host-level PostgreSQL has held `127.0.0.1:5432` continuously since 2026-10-19, so on this
machine the documented compose path cannot bind its port at all.

## 2. RED proof — the pre-fix file really fails (not just "it existed")

`git show HEAD:docker-compose.yml` written to a scratch path and started as its own compose
project (`cons075red`):

```
 Volume "cons075red_pgdata"  Creating
 Volume "cons075red_pgdata"  Created
 Container cons075red-postgres-1  Creating
 Container cons075red-postgres-1  Created
 Container cons075red-postgres-1  Starting
Error response from daemon: failed to set up container networking: driver failed programming
external connectivity on endpoint cons075red-postgres-1
(a0a7802fee7677b94d198d4f0d11b128c0f98586278a55274a1be7d04d83c516):
failed to bind host port 0.0.0.0:5432/tcp: address already in use
```

Note on the rc of that call: it was printed through a pipe (`| tail -6`), so the visible
`red_rc=0` belongs to `tail`, not to compose. The RED verdict rests on the daemon error text
above, not on that exit status. The GREEN arm below captures compose's own rc with
`PIPESTATUS[0]`.

## 3. Change (bounded, reversible, config-only)

Three files, no behaviour change when the host port is free:

- `docker-compose.yml` — `- "5432:5432"` → `- "${CONSENSUS_PG_PORT:-5432}:5432"`
- `docker-compose.prod.yml` — `- "127.0.0.1:5432:5432"` → `- "127.0.0.1:${CONSENSUS_PG_PORT:-5432}:5432"`
- `README.md` — the embedded prod-compose snippet carries the same line, plus a paragraph and a
  copy-paste example naming `CONSENSUS_PG_PORT`.

The container port stays 5432; only the host side moves. Default is unchanged (5432), so
nothing that already worked changes behaviour.

## 4. GREEN proof — the documented path starts with 5432 occupied

Resolution, `docker compose config`:

```
--- default ---
    ports:
      - mode: ingress
        target: 5432
        published: "5432"
--- CONSENSUS_PG_PORT=15433 ---
    ports:
      - mode: ingress
        target: 5432
        published: "15433"
```

Boot, literal acceptance command, on the same host that owns 127.0.0.1:5432:

```
CONSENSUS_PG_PORT=15433 docker compose -p cons075probe -f docker-compose.yml up -d --build --wait
 Container cons075probe-postgres-1  Running
 Container cons075probe-postgres-1  Waiting
 Container cons075probe-postgres-1  Healthy
compose_up_build_rc=0
```

And the published mapping / listener:

```
cons075probe-postgres-1  Up 5 minutes (healthy)  0.0.0.0:15433->5432/tcp, [::]:15433->5432/tcp

LISTEN 0 4096 0.0.0.0:15433 0.0.0.0:*
LISTEN 0 4096    [::]:15433    [::]:*
```

The dev `docker-compose.yml` declares no `build:` sections, so `--build` is a no-op there; the
arm is graded on "the documented command returns 0 and the container becomes healthy with the
host port moved", which is what the run above shows.

---

## Acceptance arms

| Arm | Verdict | Evidence |
|---|---|---|
| `docker compose up -d --build` succeeds on a host with 5432 occupied | **PASS** | §4 — `compose_up_build_rc=0`, container `(healthy)`, host 5432 owned by `postgresql@18-main` |
| host port configurable via env with a sane default | **PASS** | §4 — default resolves `published: "5432"`, `CONSENSUS_PG_PORT=15433` resolves `published: "15433"`, container port fixed at 5432 |
| README/quickstart names the variable if a port is published | **PASS** | `README.md` — the compose section names `CONSENSUS_PG_PORT` with a `CONSENSUS_PG_PORT=15432 docker compose -f docker-compose.prod.yml up -d --wait` example, and the embedded snippet carries the variable |

## NOT DONE in this tick (stated, not hidden)

1. **Probe-stack teardown was refused by the destructive-action gate.** The cleanup leg
   (`docker compose -p <project> ... down -v`) returned `BLOCKED: ... no user response` after the
   gate's full approval timeout. It is not retried here, per the gate contract. Residue left on
   this host:
   - `cons075probe-postgres-1` — running, healthy, `0.0.0.0:15433->5432/tcp`
   - `cons075red-postgres-1` — created, never started (bind failed)
   Owner-run cleanup (each is disposable; both volumes belong to the throwaway probe projects):
   ```
   docker compose -p cons075probe -f docker-compose.yml down -v
   docker compose -p cons075red -f <scratch>/head-compose.yml down -v
   ```
   These are the only containers this tick created.
2. **Pre-existing residue from the 2026-11-14 ops fire is still up:** `consensus-opsprobe-runtime`
   and `consensus-opsprobe-postgres` (created 2026-11-14T04:25, `restart: no`), publishing
   `0.0.0.0:8090` for ~25h after the row they verified was closed. No cron job or script on this
   host references `8090` or the stack (`jq` over `~/.hermes/cron/jobs.json` → no match), so
   nothing depends on it. Left alone for the same gate reason; owner-run:
   ```
   docker compose -p consensus-opsprobe \
     -f docker-compose.prod.yml \
     -f ~/.hermes/cache/scratch/consensus-ops-tick-20261114/probe-override.yml down
   ```
   (No `-v`: its `consensus-opsprobe-pgdata` volume may still hold the evidence DB.)
3. **`docker-compose.prod.yml` is changed but not boot-proven.** Its `container_name` values are
   fixed (`consensus-postgres`, `consensus-runtime`), so a parallel proof stack would collide with
   any real deployment on the host; only its port resolution was verified
   (`published: "15434"` under `CONSENSUS_PG_PORT`). Boot of the production shape belongs to
   QA-CONSENSUS-35, which is the same one-line class and stays pending.
