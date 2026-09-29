# CLI Exit Codes

Every `consensus` subcommand is scriptable, and scripts can only branch on
exit codes. This document is the measured exit-code contract for the CLI
(`consensus version` 0.1.0 tree), per verb and per failure class.

**Rationale (SPEC-016).** The CLI spec's third design principle is:

> 3. **Scriptable** — All output available as JSON (`--format json`). Designed
>    for pipes, cron jobs, and automation.
>    — `specs/016-cli-interface.md`, §1 (Design Principles)

Pipe-and-cron automation is only sound if a failed verb *reports failure*.
That is what the spec's exit-code table (`specs/016-cli-interface.md`, §8)
is for, and what this document records as measured behavior:

| Code | Meaning |
|---|---|
| 0 | Success (including a legitimately empty result — see below) |
| 1 | General error |
| 2 | Invalid arguments / usage error |
| 3 | Server unreachable |
| 4 | Authentication failed |
| 5 | Not found |
| 6 | Conflict (wrong state) |
| 7 | Rate limited |

## How it works

`cmd/consensus/main.go` dispatches subcommands through `cli.Execute()`
(`internal/cli/root.go`), which maps a returned error to a §8 code by
matching the error text (`UNAUTHENTICATED` → 4, `NOT_FOUND`/`not found` → 5,
`connection refused`/`no such host`/`i/o timeout` → 3, argument-shaped
messages → 2, everything else → 1). Cobra usage errors (unknown command,
unknown flag, wrong arg count, unknown `--format`) are exit 2.

Two verbs bypass the server entirely and are exempt from the pre-flight
identity check: `serve` (is the server) and `init` (first-time setup).
`version`, `completion`, and `migrate create <name>` are offline as well.

## Exit codes by verb

Measured against a live `consensus serve` (scratch SQLite DB) and a dead
server (`http://127.0.0.1:1`, connection refused). "—" means the class does
not apply to that verb.

| Verb | Success | Conn. failure | Auth rejected | Not found | Usage error |
|---|---|---|---|---|---|
| `status` | 0 | 3 | 4 *(fixed, DOC-10: was 0)* | — | 2 |
| `session create` | 0 | 3 | 4 | — | 2 (missing `--goal`) |
| `session list` | 0 (0 rows = still 0) | 3 | 4 | — | 2 |
| `session show` / `logs` / `cost` / `pause` / `resume` / `cancel` / `message` | 0 | 3 | 4 | 5 | 2 |
| `approve list` | 0 (empty = still 0) | 3 | 4 | — | 2 |
| `approve show` / `approve <id>` / `reject <id>` | 0 | 3 | 4 | 5 | 2 |
| `config list` | 0 | 3 | 4 | — | 2 |
| `config get <key>` | 0 | 3 | 4 | 5 *(fixed, DOC-10: was 0)* | 2 |
| `config set <k> <v>` | 0 | 3 | 4 | — | 2 |
| `config edit` | 0 | — (offline) | — | — | 2; 1 if `$EDITOR` exits non-zero |
| `tool list` | 0 (empty registry = still 0) | 3 | 4 | — | 2 |
| `tool show <name>` | 0 | 3 | 4 | 5 *(fixed, DOC-10: was 0)* | 2 |
| `skill list` | 0 (empty = still 0) | 3 | 4 | — | 2 |
| `skill show <name>` | 0 | 3 | 4 | 5 | 2 |
| `memory list` / `iterations` | 0 (200-empty = still 0)¹ | 3 | 4 | — | 2 |
| `memory show` / `pages` | 0 | 3 | 4 | 5 | 2 |
| `migrate version` | 0 | 3 | — | — | 2 |
| `migrate run` / `rollback` (`--db-url`) | 0 | 3² | — | — | 2 |
| `models sync` (`--db-url`) | 0 | 3² | — | — | 2 |
| `init` (`--db-url`) | 0 | 3 (unreachable DB) | — | — | 2 |
| `serve` | blocks; 0 on graceful SIGINT/SIGTERM drain | 1 (bad DB URL, occupied port) | — | — | 2 |
| `mcp-stdio` | 0 on clean stdin EOF | 1 (startup failure) | — | — | 2 |
| `version` | 0 | — (offline) | — | — | — |
| `completion <shell>` | 0 | — (offline) | — | — | 2 (unsupported shell) |
| unknown verb / flag (any level) | — | — | — | — | 2 |

Notes:

1. `memory list` / `memory iterations` / `session logs` against a
   *nonexistent* session return HTTP 200 with an empty list (the server
   treats a missing session as zero rows for these endpoints), so the CLI
   prints "(no results)" and exits 0. This is an empty-result success, not a
   swallowed failure — `session show <id>` and `session cost <id>` do return
   NOT_FOUND (5) for the same id. If you need strict existence checking in a
   script, use `session show` first.
2. `migrate run|version|rollback --db-url` and `models sync --db-url` are
   *offline* verbs, but when no server is listening on the default URL the
   root pre-flight identity check still fires first and exits 3 with
   "cannot connect to Consensus server at http://localhost:8090". The exit
   code is correct (non-zero); the message points at the server rather than
   the `--db-url`. `init` is exempt from the pre-check and reports the real
   DB error (also exit 3). Known quirk, not fixed in DOC-10.

## Fixed in DOC-10 (before → after)

| Verb + case | Before | After |
|---|---|---|
| `config get <missing-key>` | printed `Key not found: <key>`, **exit 0** | `NOT_FOUND: config key "<key>" not found`, **exit 5** |
| `tool show <missing-tool>` | printed `Tool not found: <name>`, **exit 0** | `NOT_FOUND: tool "<name>" not found`, **exit 5** |
| `status` when `/api/v1/health` succeeds but `/api/v1/metrics` fails (e.g. wrong API key → 401) | printed `metrics: unavailable`, **exit 0** | error propagated: `UNAUTHENTICATED: ...`, **exit 4** (any §8 code per the metrics error) |

Everything else was probed as already correct and is unchanged.

## Deliberate exit-0 empty results

These exit 0 *by design* — an empty result is a success for a listing verb.
Do not "fix" them:

- `session list`, `approve list`, `tool list`, `skill list`,
  `memory list`, `memory iterations` with zero rows
- `session logs` for a session with no committed iterations (prints an
  explanatory hint; see note 1 for the missing-session nuance)
- `mcp-stdio` on clean stdin EOF (a client that closes the session is not
  an error)

## `set -e`-safe usage

```bash
#!/usr/bin/env bash
set -euo pipefail

# 1. Gate on server reachability (exit 3 tells you it's the connection).
consensus status >/dev/null

# 2. Fail loudly on a wrong config key (exit 5, fixed in DOC-10).
MODEL=$(consensus config get llm.default_model --format json)
echo "model: $MODEL"

# 3. Branch on specific codes where it matters.
if ! consensus session show "$SID" >/dev/null 2>&1; then
  rc=$?
  if [ "$rc" -eq 5 ]; then
    echo "session $SID does not exist" >&2
  elif [ "$rc" -eq 3 ]; then
    echo "server unreachable" >&2
  else
    echo "session show failed (rc=$rc)" >&2
  fi
  exit "$rc"
fi

# 4. Machine-readable failure reason on stderr, non-zero on stdout:
consensus tool show nosuchtool 2>err.txt || rc=$?
# rc=5, err.txt: consensus: NOT_FOUND: tool "nosuchtool" not found
```

With `set -e`, any verb failure now stops the script. Before DOC-10,
`config get`/`tool show` on a missing resource and `status` with a rejected
key exited 0 and silently continued — the exact failure mode the
Scriptable principle exists to prevent.

## Verification

Reprobed on the fixed tree (see the DOC-10 work item for the raw probe
logs): `go test -short -count=1 ./internal/cli/...` covers the contract with
`TestExitCode_ConnectionFailure_Is3`, `TestStatus_MetricsAuthRejected`,
`TestStatus_ConnectionFailure`, `TestSessionList_ConnectionFailure`,
`TestConfigGet_ConnectionFailure`, and the updated
`TestConfigGet_NotFound` / `TestToolShow_NotFound`.
