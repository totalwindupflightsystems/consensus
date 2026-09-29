# Debugging Consensus — the pprof debug listener

Consensus ships a dedicated [net/http/pprof](https://pkg.go.dev/net/http/pprof) debug listener
so a 3am operator can capture goroutine dumps, heap and CPU profiles from a running
`consensus serve` process — without restarting it, without an API key, and without touching
the public API router. It was added for PERF-CONSENSUS-11 after the 2026-09-09 dogfood
wedge, where the absence of `/debug/pprof/goroutine` (404) made the diagnosis 10× slower
(see [docs/dogfood/diagnostics-2026-09-09.md](dogfood/diagnostics-2026-09-09.md), probe
step 4).

> **Security first:** pprof handlers are **unauthenticated**. The listener is
> loopback-only by design and code-enforced: `internal/api/pprof_debug.go` refuses to bind
> any non-loopback address (logs an error, disables the listener). Never try to route
> around it. See [Security](#security) below.

## Where it is configured

| What | Value |
|---|---|
| Config key | `server.pprof_addr` (`internal/config/config.go:49`) |
| Default | `127.0.0.1:8095` (asserted in `internal/config/config_test.go:78-80`) |
| Disable | set `server.pprof_addr: ""` (or omit → default applies) |
| Override paths | config file only: `--config <file>` (or `CONSENSUS_CONFIG`), `./consensus.yaml`, `~/.consensus/config.yaml`, `/etc/consensus/config.yaml` |
| Env override | **none** — `applyEnvOverrides` (`internal/config/config.go:350-452`) handles hostname/port/db/llm/crier only; no `CONSENSUS_PPROF_*` variable exists. Documented after source inspection. |

Mount point: `cmd/consensus/main.go:188-194` calls
`api.StartPprofListener(cfg.Server.PprofAddr)` inside `runServer`. The public API router
never mounts `/debug/pprof` — on the API port it returns **404** (verified live; the unit
test `internal/api/pprof_debug_test.go` pins this).

YAML shape (add to `consensus.yaml` or a `--config` file):

```yaml
server:
  pprof_addr: 127.0.0.1:8095   # default; "" disables the listener
```

Behavioral notes (from `internal/api/pprof_debug.go`, verified live):

- Empty address → listener disabled (server logs nothing about pprof).
- Non-loopback address (e.g. `0.0.0.0:8095`) → **refused**: logged loudly, listener stays
  off, server keeps running. A config mistake cannot expose pprof.
- Busy port → warning, server keeps running **without** pprof. A wedged debug port must
  not take the API down. If you see `pprof: debug listener failed to start` in the log,
  the API is fine but profiling is unavailable — free the port and restart.
- On success the server logs
  `pprof: debug listener started (loopback only) addr=...:8095`.

## Capturing profiles

`consensus serve` below runs on its default API port `8090` with pprof on the default
`127.0.0.1:8095`. pprof needs **no API key and no auth of any kind** — curl straight at
it (that is also exactly why it must stay on loopback; see
[Security](#security)).

All examples fetch from a live server. `go` needs to be installed to post-process; the
binary path must match the binary the server is actually running (pprof matches by build
ID — a mismatched local binary gives misleading symbolization).

### Goroutine dump — the first thing to run

```bash
# Human-readable summary: one block per goroutine-stack group with counts.
curl -s http://127.0.0.1:8095/debug/pprof/goroutine?debug=1 > goroutines.txt

# Verbose: every goroutine individually, with its full stack and creation site.
curl -s http://127.0.0.1:8095/debug/pprof/goroutine?debug=2 > goroutines-verbose.txt
```

Both return HTTP 200 with plain text (`Content-Type: text/plain`). `debug=1` groups
identical stacks with a leading count; `debug=2` lists each goroutine separately — use
`debug=2` when you need per-goroutine wait reasons and lifetimes.

`?debug=1` on the index page lists every profile type with HTTP codes:

```bash
curl -s http://127.0.0.1:8095/debug/pprof/
```

### Heap profile

```bash
# Fetch the profile, then analyze.
curl -s http://127.0.0.1:8095/debug/pprof/heap > heap.pb.gz
go tool pprof -top -nodecount=25 bin/consensus heap.pb.gz
```

Or skip the file entirely — pprof fetches it itself:

```bash
go tool pprof -top -nodecount=25 bin/consensus http://127.0.0.1:8095/debug/pprof/heap
```

Default heap sample is in-use space (`-sample_index=inuse_space` is the default for this
endpoint). For allocation pressure over the process lifetime, use the `allocs`
profile instead — it records cumulative allocations:

```bash
curl -s http://127.0.0.1:8095/debug/pprof/allocs > allocs.pb.gz
go tool pprof -sample_index=alloc_space -top bin/consensus allocs.pb.gz
```

### CPU profile (30-second window)

```bash
curl -s "http://127.0.0.1:8095/debug/pprof/profile?seconds=30" > cpu.pb.gz
# (the request blocks for the full window, then returns the profile)

go tool pprof -top -nodecount=25 bin/consensus cpu.pb.gz
```

`seconds=` accepts any duration; 30 is the usual balance between signal and operator
patience. On an idle server the profile will legitimately be near-empty (few samples) —
that is the answer, not a failure.

### Block and mutex profiles

```bash
curl -s http://127.0.0.1:8095/debug/pprof/block > block.pb.gz
curl -s http://127.0.0.1:8095/debug/pprof/mutex > mutex.pb.gz

go tool pprof -top bin/consensus block.pb.gz
go tool pprof -top bin/consensus mutex.pb.gz
```

**Both are empty in current builds.** Consensus never calls
`runtime.SetBlockProfileRate` / `runtime.SetMutexProfileFraction`, so contention sampling
is off and these endpoints return HTTP 200 with zero samples (verified live against this
repo: 200, "Showing nodes accounting for 0"). They are documented here for completeness
and future enablement — do not treat an empty block/mutex profile as evidence that no
contention exists.

### Interactive mode

```bash
go tool pprof -http=127.0.0.1:18080 bin/consensus http://127.0.0.1:8095/debug/pprof/heap
```

Opens the web UI (flame graph, graph view, source-annotated listings) in a browser.
On a headless box the browser-open step fails (`Missing X server or $DISPLAY`) — the
pprof HTTP server still listens on the `-http` address, so port-forward or open
`http://127.0.0.1:18080` from your workstation. Use `-http=127.0.0.1:0` to let pprof
pick a free port.

## Wedge playbook

When `consensus serve` appears hung (API times out, sessions stop progressing, logs show
a planning window that never closes):

1. **Goroutine dump first — `?debug=1`:**

   ```bash
   curl -s --max-time 10 http://127.0.0.1:8095/debug/pprof/goroutine?debug=1 > goroutines.txt
   ```

   The pprof listener runs on its own listener and goroutine — it answers even when the
   API handler pool is wedged (it answered with a 404-missing profile back when pprof
   wasn't mounted; today it is mounted and answers 200). `--max-time` keeps your shell
   honest if the whole process is truly dead (gone if curl itself times out).

2. **Read the header, then look for groups:**

   ```
   goroutine profile: total 14
   1 @ 0x4214e9 ...
   ```

   The first line is your total goroutine count. `debug=1` groups identical stacks with
   a leading count — a **large count on one stack is your suspect**.

3. **What to look for:**

   - **Same mutex, many waiters** — a group of goroutines whose stacks end in
     `sync.(*Mutex).Lock` (or `RWMutex`) all converging on the same acquisition site.
     The goroutine *holding* the lock is elsewhere in the dump, parked in the critical
     section's next blocking call.
   - **Same channel, many senders/receivers** — stacks containing
     `chan send`/`chan receive` (runtime text: `gopark` → `chanrecv`/`chansend`), N
     goroutines parked on one channel that nobody services.
   - **`select` with no default** parked forever — goroutines parked in
     `runtime.gopark` via `selectgo` with no timeout arm.
   - **HTTP handlers all in flight** — many goroutines in `net/http.(*conn).serve`
     waiting on the same downstream call (in Consensus's documented wedge this was the
     SQLite planning transaction: `begin tx: context deadline exceeded`).
   - **Heartbeat loop state** — grep for the heartbeat/consensus-specific frames
     (`harness` package) to see whether the heartbeat goroutine is alive or itself
     parked. A dead heartbeat goroutine means the harness will not recover on its own.

4. **Escalate with `?debug=2`** for per-goroutine wait reasons and creation sites when
   the grouped view is ambiguous (e.g. which of the parked goroutines would have held
   the mutex).

5. **Then** heap (`/debug/pprof/heap`) if memory pressure is suspected, and the
   30-second CPU profile for the post-restart busy loop. Keep every artifact with
   timestamps — they are the evidence chain for the post-mortem.

## Security

- pprof handlers are **unauthenticated**: no API key, no auth header, nothing. The
  config-file comment (`internal/config/config.go:46-48`) is the rationale:

  > PprofAddr is the loopback-only address for the pprof debug listener
  > (PERF-CONSENSUS-11). Empty disables the listener. It must never point
  > at a public interface — pprof handlers are unauthenticated.

- The public API router **never** mounts `/debug/pprof` (it 404s there — verified live
  and pinned by `internal/api/pprof_debug_test.go`). Unauthenticated debug surface is
  exactly the DF-CONSENSUS-19 exposure class the loopback-only rule exists to prevent.
- The listener is **code-enforced loopback-only** (`internal/api/pprof_debug.go`): a
  config value of `0.0.0.0:8095`, `:8095`, or any hostname that is not
  `127.0.0.1`/`::1`/`localhost` is refused — logged as an error, listener disabled,
  server continues. There is no supported way to bind pprof past loopback; do not
  work around it with an external port-forward without adding your own auth in front.
- If you need profiles from a remote host, reach loopback safely: SSH port-forward
  (`ssh -N -L 8095:127.0.0.1:8095 user@host`) or an authenticated proxy — never a
  rebind.
- Goroutine dumps can include function arguments and goroutine-local state; heap
  profiles can include type names and sizes. On a host that serves real users, treat
  captured profile artifacts like any other operational data with privacy-sensitive
  content: keep them off public surfaces.

## Verified live

Verified live 2026-09-29 against HEAD `3242d33` (branch `wt/DOC-7`) in this worktree.
Build: `go build -o /tmp/doc7probe ./cmd/consensus` (go1.26.5). Run:
`/tmp/doc7probe serve --db-url sqlite:///tmp/doc7-1790689360.db --port 18443 --config <scratch yaml with server.pprof_addr: 127.0.0.1:18915>` (scratch ports: host's 8095 was
occupied by an unrelated process). Server log confirmed
`pprof: debug listener started (loopback only) addr=127.0.0.1:18915`.

| Probe | Result |
|---|---|
| `GET 127.0.0.1:18915/debug/pprof/` | **200** (index) |
| `GET /debug/pprof/goroutine?debug=1` | **200**, text dump ("goroutine profile: total 14") |
| `GET /debug/pprof/goroutine?debug=2` | **200**, per-goroutine verbose dump |
| `GET /debug/pprof/heap` | **200**, 3110 bytes; `go tool pprof -top` symbolized against the probe binary |
| `GET /debug/pprof/profile?seconds=30` | **200**, 468 bytes, `Duration: 30s` samples symbolized via `go tool pprof` |
| `GET /debug/pprof/block` | **200**, empty profile (sampling off by default — documented above) |
| `GET /debug/pprof/mutex` | **200**, empty profile (sampling off by default — documented above) |
| `go tool pprof -http=127.0.0.1:0` against fetched heap | web UI served; browser auto-open failed on headless host (expected, noted above) |
| `GET 127.0.0.1:18443/api/v1/health` | **200** (server alive during the whole battery) |
| `GET 127.0.0.1:18443/debug/pprof/` | **404** — public router does not mount pprof |

Server was killed after the battery; both ports released.

## Corrections to the task brief (source is decisive)

1. **No env override exists for pprof_addr.** The brief asked to document the
   `CONSENSUS_*` env equivalent "if one exists". It does not:
   `applyEnvOverrides` (`internal/config/config.go:350-452`) overrides hostname, port,
   db URL, LLM keys/provider/base-URL, log level and crier settings — never
   `server.pprof_addr`. Override paths are the config file (via `--config`,
   `CONSENSUS_CONFIG`, or the default file chain) only.
2. **Block/mutex profiles are empty by default.** The brief listed them as capturable
   profiles; they are capturable (HTTP 200) but carry zero samples because Consensus
   never enables `runtime.SetBlockProfileRate`/`runtime.SetMutexProfileFraction`.
   Documented as such rather than implying they yield useful output on a wedged server.
