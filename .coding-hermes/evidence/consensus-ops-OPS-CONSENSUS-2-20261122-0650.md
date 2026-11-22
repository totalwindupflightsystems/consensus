# consensus-ops — OPS-CONSENSUS-2 — 2026-11-22T11:50Z (tick consensus-ops-2026-11-22-11-45-40)

- tick: `consensus-ops-2026-11-22-11-45-40` (lane consensus-ops)
- row: `OPS-CONSENSUS-2` (stays pending — blocked on an owner-gated LLM credential, not on code)
- workdir resolved: `/home/kara/.hermes/stand-in/pm-lane/consensus` → symlink → `/home/kara/totalwindupflightsystems/consensus` (`ls -ld` → `consensus -> /home/kara/totalwindupflightsystems/consensus`)
- head at measurement: `5765cafd9a91f9352d515564b6c53cdf05ccba6e`; `origin/main` identical after `git fetch`

## What changed since the last audit (2026-11-21 fire 5)

1. The comparison tip moved: `bc00da72` → `5765cafd`, so the drift figure the row title carries
   ("194 commits behind origin/main f160ba7") is stale twice over.
2. **NEW FAULT, fixed this tick — the main checkout could not build.** The working tree carried a
   conflicted `internal/harness/planning.go` (10 conflict markers, unmerged index stages 1/2/3, no
   `MERGE_HEAD`), i.e. an abandoned `git stash pop` of `stash@{0}`. Cause proven by blob identity.
3. Planning is still 100 % dead, now measured at request level on a fresh 3 h window: 10379/10379.

## Probe 1 — checkout state at open (the fault)

```
$ cd /home/kara/consensus && git status -sb
## main...origin/main
UU internal/harness/planning.go

$ git ls-files -u internal/harness/planning.go
100644 e8864c43c7ac46dc690e07193ecdc0886ad6b6d9 1	internal/harness/planning.go
100644 e9096ff21f03d4dd1ada386d05a349250ed32a8b 2	internal/harness/planning.go
100644 f24f27d9554b59fd5f62472d16fffd945784ef65 3	internal/harness/planning.go

$ ls .git/ | grep -Ei 'MERGE|REBASE|CHERRY|BISECT|sequencer'   # -> no output (no merge in progress)
$ grep -c '^<<<<<<<\|^>>>>>>>' internal/harness/planning.go    # -> 10
$ grep -n '^<<<<<<<\|^=======$\|^>>>>>>>' internal/harness/planning.go
268:<<<<<<< Updated upstream   306:=======   307:>>>>>>> Stashed changes
327:<<<<<<< Updated upstream   329:=======   332:>>>>>>> Stashed changes
1235:<<<<<<< Updated upstream  1263:=======  1267:>>>>>>> Stashed changes
1272:<<<<<<< Updated upstream  1313:=======  1314:>>>>>>> Stashed changes
1563:<<<<<<< Updated upstream  1680:=======  1681:>>>>>>> Stashed changes
```

Cause pinned to one stash entry, not inferred:

```
$ git rev-parse 'stash@{0}:internal/harness/planning.go'
f24f27d9554b59fd5f62472d16fffd945784ef65        # == stage 3 (theirs)
$ git rev-parse HEAD:internal/harness/planning.go
e9096ff21f03d4dd1ada386d05a349250ed32a8b        # == stage 2 (ours) == committed content
$ git stash list | head -1
stash@{0}: On main: tick 22-07-51 quarantine: uncommitted revert of DF-65/DF-70 planning.go fixes
```

Impact, measured before the fix:

```
$ go build ./internal/harness/
# github.com/wojons/consensus/internal/harness
internal/harness/planning.go:268:1: syntax error: unexpected <<, expected }
internal/harness/planning.go:345:4: syntax error: non-declaration statement outside function body
internal/harness/planning.go:566:15: method has no receiver
... (rc=1, "too many errors")
```

`internal/harness/planning.go` mtime `2026-11-22 01:47:22 -0500`, `.git/index` `2026-11-22 02:30:36
-0500`; no `git`/`go build`/`consensus` compile process was running (`ps -eo pid,etime,cmd`), and the
tree carried no other change. So the checkout was broken for ~9 h with nothing in flight in it.

Arm table at this point: **every repo-side gate and every battery run from this checkout was
impossible**, including the redeploy this row's own acceptance criteria require.

## Fix applied (bounded, reversible, inside the project worktree)

The two sides were fully recoverable before the edit: stage 2 == the committed blob, stage 3 ==
`stash@{0}`'s blob (still in the stash list), and a byte copy of the conflicted file was taken first.

```
$ cp internal/harness/planning.go ~/.hermes/cache/scratch/cons-1122/planning.go.conflicted-20261122
$ git status --porcelain > ~/.hermes/cache/scratch/cons-1122/porcelain-before.txt
$ cat porcelain-before.txt
UU internal/harness/planning.go                      # that is the WHOLE blast radius

$ git checkout HEAD -- internal/harness/planning.go ; echo checkout_rc=$?
checkout_rc=0
$ grep -c '^<<<<<<<' internal/harness/planning.go ; echo    # -> 0
$ git ls-files -u internal/harness/planning.go | wc -l      # -> 0
$ git hash-object internal/harness/planning.go              # -> e9096ff2...  (== HEAD blob)
$ git status -sb                                            # -> ## main...origin/main   (clean)
$ git stash list | head -1                                  # -> stash@{0} quarantine entry STILL PRESENT
```

Verification after the fix:

```
$ go build ./internal/harness/   # rc=0
$ go build ./...                 # rc=0  (whole module)
```

No commit was needed: the restored content is byte-identical to `HEAD` == `origin/main`, so the
repair returns the checkout to its published state instead of creating a repo delta. The stash entry
that caused it is deliberately NOT dropped — that is a DF-CONSENSUS-065/070 data decision, not an ops
one.

## Probe 2 — deployed artifact gate (arm 2)

```
$ bash scripts/check-deployed-artifact.sh ; echo real_gate_rc=$?
deployed stamp        : bcf2338a07a25c8af1332d45d964689123da1833  [image-label:org.opencontainers.image.revision]
recorded deployment   : bcf2338a07a25c8af1332d45d964689123da1833
master                : 5765cafd9a91f9352d515564b6c53cdf05ccba6e  (policy: equal, ref: @{upstream})
commits behind master : 224
drift repair in image : present
verdict               : RED
reason                : drift: deployed bcf2338... is 224 commits behind master 5765cafd...
real_gate_rc=1
```

Identity arm GREEN, drift 198 → **224**. Drift is the only failing arm.

Arm 4 re-run at this tip (it could not be run while the tree was conflicted; the tip moved since the
last run, so it was re-measured rather than inherited):

```
$ bash scripts/battery/deploy_artifact_check_selfproof.sh ; echo selfproof_rc=$?
PASS  green: --json machine-readable verdict       rc=0 verdict=GREEN
self-proof: 14 passed, 0 failed
selfproof_rc=0
```

## Probe 3 — planning liveness (arm 5 premise), 3 h window, request level

```
$ for c in dexdat-core-consensus-1 df1120-consensus-1; do docker logs --since 3h "$c" > "$c.3h.log"; done
dexdat-core-consensus-1 lines=62274
df1120-consensus-1      lines=0        # idle: serves no traffic in this window
window: time=2026-11-22T08:48:42.831Z .. 2026-11-22T11:48:38.553Z

$ grep -o 'msg="[^"]*' dexdat-core-consensus-1.3h.log | cut -c1-50 | sort | uniq -c | sort -rn
10379 planning: starting interactive session
10379 planning: LLM error
10379 planning: LLM call failed
10379 llm: calling provider
10379 llm: HTTP 401 — LLM auth failed: check your API ke
10379 harness: found active session

$ grep 'msg="llm: HTTP 401' ... | grep -o 'T[0-9][0-9]' | sort | uniq -c
 670 08h    3534 09h    3498 10h    2677 11h
```

Only six message types exist in the window — all six are parts of one failure chain, so there is no
success message type at all: **0 successes in 10379 provider calls**. Error text names the frozen
credential: `Your api key: ****6e23 is invalid`. Level counts `31137 ERROR / 31137 INFO` reconcile
exactly with `10379 × 6`, so no line in the window is unaccounted for.

Unit note for the next reader: each failed request emits THREE `msg` lines containing `HTTP 401`
(`llm: HTTP 401`, `planning: LLM call failed`, `planning: LLM error`); the request-level count is
`grep -c 'msg="llm: HTTP 401'`, which is the figure above.

## Probe 4 — container state (arms 1 and 3)

```
$ docker ps --format '{{.Names}} | {{.Ports}} | {{.Status}}' | grep -i consensus
df1120-consensus-1       | 0.0.0.0:18380->8080, 18381->8081 | Up 35 hours (healthy)
dexdat-core-consensus-1  | 0.0.0.0:8380->8080, 8381->8081   | Up 35 hours (healthy)

$ docker inspect <c> --format '{{.Created}} | restarts={{.RestartCount}} | health={{.State.Health.Status}}'
dexdat-core-consensus-1 | 2026-11-18T02:40:14Z | restarts=1 | health=healthy
df1120-consensus-1      | 2026-11-20T11:50:14Z | restarts=0 | health=healthy
```

Both stacks answer `healthy` while every planning call 401s — a healthcheck that cannot see the fault.
That is why acceptance arm 5 is graded below as **UNMET, not met** despite `health=healthy`.

`docker ps -a` still lists `consensus-opsprobe-postgres` / `consensus-opsprobe-runtime`
`Exited (255) 4 days ago` — probe residue, no destructive cleanup (pool regenerability unproven).

## Acceptance arms, graded today

| # | arm | state |
|---|-----|-------|
| 1 | rebuild at current main + recreate stack (Created newer than 2026-11-21) | **UNMET** — image created 2026-11-18, runs bcf2338 |
| 2 | gate exits 0 / GREEN / 0 behind | **UNMET** — rc=1, RED, 224 behind 5765cafd |
| 3 | `deploy/deployed-artifact.json` re-recorded | **UNMET** — still records bcf2338 |
| 4 | self-proof still 14 passed / 0 failed | **MET at this tip** — 14 passed, 0 failed, rc=0 |
| 5 | healthy after redeploy, no SQLSTATE 42703 / restart loop | **UNMET** — no redeploy happened; and `healthy` is a false signal here (0/10379 planning successes) |

Prerequisite this tick added: the checkout builds again (`go build ./...` rc=0), which arms 1–4
needed and did not have.

## Not done this tick, and why

- **The redeploy itself.** A recreate at current main would turn arms 1–3 GREEN and leave planning
  100 % broken: the dead `DEEPSEEK_API_KEY` (…`6e23`) is frozen into container env at creation and the
  host `.env` still carries it. Missing input is an owner-gated credential, not code. It is also
  cross-project mutation (the compose projects `dexdat-core` / `df1120` own both containers).
- **Sourcing a replacement key** — owner-gated; not read, not copied, not echoed.
- **Gate policy `equal`** — intentional; greening it is a redeploy, not a policy edit.
- **`stash@{0}` drop** — a DF-CONSENSUS-065/070 content decision; the stamp stays for its owner.
- **Service restarts** — nothing was in an in-flight tick and no fault named a restart as the remedy.
- **`consensus-opsprobe-*` exited containers** — 4-day-old residue; cleanup withheld (regenerability
  unproven), and it destroys nothing to leave them.
