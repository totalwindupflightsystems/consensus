# RELEASE-CONSENSUS-009 — verification evidence

Row: **RELEASE-CONSENSUS-009** (P1, CI-integrity) — "Guard is disarmed in every
linked worktree: `core.hooksPath=.git/hooks` is relative, and in a worktree `.git`
is a file, so the pre-commit hook never resolves."

Branch: `rel009-worktree-hooks`. Worktree:
`/home/kara/.hermes/cache/scratch/wt-rel009-1337`.
Base: `origin/main` @ `154fb7695dac203761e507becc9e0836ea6c8fba`.
Captured 2026-11-21 ~15:00–15:10Z, host load 10.5–13.5 on 16 cores, go1.26.11.

Changes under test:

| file | change |
|---|---|
| `.gitreins/pre-commit` | every path derived at runtime; no checkout path anywhere |
| `scripts/install-hooks.sh` | new installer: common hooks dir, repairs/refuses relative `core.hooksPath`, idempotent |
| `Makefile` | `gitreins-install` delegates to the installer; `gitreins-eval` no longer reads `~/consensus` |
| `docs/release.md` | "Pre-commit guard: install once per clone, never relative" |

---

## 1. Defect reproduced (before)

Reproduced on a throwaway clone of the product checkout in scratch cache, with the
product's historical local config, plus a linked worktree — the exact fleet-worker
configuration:

```
$ git config core.hooksPath
.git/hooks
$ git rev-parse --git-path hooks
.git/hooks
$ ls .git/hooks
ls: cannot access '.git/hooks': Not a directory     # .git is a FILE in a worktree
$ git hook run pre-commit
error: cannot find a hook named pre-commit           # exit 1
```

A real commit is worse than that error, because nothing prints at all: the hook
simply never runs and the commit succeeds unguarded. Proven with a marker-writing
hook installed in the common dir, committing the same file in the same worktree
twice:

```
$ git config core.hooksPath .git/hooks
$ git commit -m "probe: relative hooksPath"
commit_rc=0
marker with relative hooksPath: (absent — hook never ran)

$ git config --unset-all core.hooksPath
$ git commit -m "probe: hooksPath unset"
commit_rc=0
marker with hooksPath unset: 2026-11-21T15:08:27Z
```

That is the disarm this row fixes: green commits, no guard, in every linked
worktree.

The stale checkout (measured fact 2 and 3), from the hook installed before this
row (`pre-commit.pre-rel009.bak`):

```
$ grep -n 'board_root=\|~/consensus\|~/gitreins-poc' "$(git rev-parse --git-path hooks)/pre-commit.pre-rel009.bak"
4:board_root="/home/kara/consensus"
120:  if [ ! -d ~/gitreins-poc ]; then
121:    echo "⚠ GitReins: Tier 2 — SKIP (gitreins-poc not found at ~/gitreins-poc)"
126:    PYTHONPATH=. python3 ~/consensus/.gitreins/eval-runner.py build-gate || {
```

So a commit made from any worktree of `/home/kara/totalwindupflightsystems/consensus`
had its board-lint run against `/home/kara/consensus` — a different, stale
checkout — and its Tier 2 leg against `~/consensus/.gitreins/eval-runner.py`.

Product-checkout state after the releng lane's manual repair (the behaviour this
row makes the installed default):

```
$ git -C /home/kara/totalwindupflightsystems/consensus config --get core.hooksPath
                        -> exit 1 (unset at every scope)
$ git -C /home/kara/totalwindupflightsystems/consensus rev-parse --git-path hooks
.git/hooks              # benign there: in the primary checkout .git IS a directory
```

## 2. Hook resolution (after)

Primary checkout vs. a fresh linked worktree, with `core.hooksPath` unset:

```
$ git -C /home/kara/totalwindupflightsystems/consensus rev-parse --git-path hooks
.git/hooks

$ cd /home/kara/.hermes/cache/scratch/wt-rel009-1337            # linked worktree
$ git rev-parse --git-path hooks
/home/kara/totalwindupflightsystems/consensus/.git/hooks        # absolute, common dir
$ git rev-parse --path-format=absolute --git-path hooks
/home/kara/totalwindupflightsystems/consensus/.git/hooks
$ git rev-parse --git-dir
/home/kara/totalwindupflightsystems/consensus/.git/worktrees/wt-rel009-1337
```

Acceptance 1 holds: inside a linked worktree the resolution is absolute and is the
**common** hooks directory — never the literal `.git/hooks`.

## 3. Hook executes and resolves THIS repository (acceptance 2)

From the linked worktree, against the freshly installed hook:

```
$ cd /home/kara/.hermes/cache/scratch/wt-rel009-1337
$ git hook run pre-commit
▸ GitReins pre-commit — repo: /home/kara/.hermes/cache/scratch/wt-rel009-1337
board-lint: ok (/home/kara/.hermes/cache/scratch/wt-rel009-1337/.coding-hermes/board)
```

The `repo:` line and the `board-lint` path are the committing worktree's own
board — no `/home/kara/consensus` anywhere. Same run from a *fresh clone* and its
linked worktree (target directory `/home/kara/.hermes/cache/scratch/rel009-repro-wt`):

```
$ ./scripts/install-hooks.sh
==> core.hooksPath=.git/hooks is RELATIVE
    a linked worktree resolves it under a .git that is a file, so the hook never runs.
    unset core.hooksPath (scope: local)
    core.hooksPath is now unset — git uses the common hooks dir
✓ hook installed: /home/kara/.hermes/cache/scratch/rel009-repro/.git/hooks/pre-commit
  src sha256: 24c0f5f115ae1dfbbe14fb2d7d78f923dae48f8810825bf99ed8ffa54edb88bc
  dst sha256: 24c0f5f115ae1dfbbe14fb2d7d78f923dae48f8810825bf99ed8ffa54edb88bc
  rev-parse --git-path hooks:            /home/kara/.hermes/cache/scratch/rel009-repro/.git/hooks
  rev-parse --path-format=absolute:      /home/kara/.hermes/cache/scratch/rel009-repro/.git/hooks
  git dir:                               /home/kara/.hermes/cache/scratch/rel009-repro/.git/worktrees/rel009-repro-wt
✓ GitReins pre-commit armed (shared by every worktree of this repository)
rc=0
$ git config --get core.hooksPath   -> exit 1 (unset)
$ git hook run pre-commit
▸ GitReins pre-commit — repo: /home/kara/.hermes/cache/scratch/rel009-repro-wt
board-lint: ok (/home/kara/.hermes/cache/scratch/rel009-repro-wt/.coding-hermes/board)
rc=0
```

Tier 2 — explicit SKIP instead of silent success, one arm per missing prerequisite
(all with a clean file staged so the Tier-1 gate is actually reached):

```
$ GITREINS_EVAL=1 GITREINS_EVAL_PKG=/nonexistent-rel009 git hook run pre-commit
━━━ GitReins Tier 2: Agentic Evaluator ━━━
⚠ GitReins: Tier 2 — SKIP (missing /nonexistent-rel009)

$ mv .gitreins/eval-runner.py /tmp/… ; GITREINS_EVAL=1 git hook run pre-commit
⚠ GitReins: Tier 2 — SKIP (missing /home/kara/.hermes/cache/scratch/wt-rel009-1337/.gitreins/eval-runner.py)

$ env -u GITREINS_LLM_API_KEY GITREINS_EVAL=1 git hook run pre-commit
⚠ GitReins: Tier 2 — SKIP (GITREINS_LLM_API_KEY not set)
```

## 4. Installer (acceptance 3)

Executable, idempotent, installs into the common hooks dir:

```
$ ls -l scripts/install-hooks.sh
-rwxrwxr-x 1 kara kara 5596 Nov 21 10:03 scripts/install-hooks.sh
$ ./scripts/install-hooks.sh            # run 1
✓ hook installed: /home/kara/totalwindupflightsystems/consensus/.git/hooks/pre-commit
  src sha256: 24c0f5f115ae1dfbbe14fb2d7d78f923dae48f8810825bf99ed8ffa54edb88bc
  dst sha256: 24c0f5f115ae1dfbbe14fb2d7d78f923dae48f8810825bf99ed8ffa54edb88bc
$ ./scripts/install-hooks.sh            # run 2
✓ hook already current — no write: /home/kara/totalwindupflightsystems/consensus/.git/hooks/pre-commit
  src sha256: 24c0f5f115ae1dfbbe14fb2d7d78f923dae48f8810825bf99ed8ffa54edb88bc
  dst sha256: 24c0f5f115ae1dfbbe14fb2d7d78f923dae48f8810825bf99ed8ffa54edb88bc
```

Byte-identical after the second run (`24c0f5f1…` both times) and the mtime did not
change (the installer `cmp`s and skips the write). Relative `core.hooksPath` is
repaired; an absolute one that shadows the common dir is refused:

```
$ git config core.hooksPath /tmp/rel009-elsewhere && ./scripts/install-hooks.sh
error: refusing to install — absolute core.hooksPath=/tmp/rel009-elsewhere shadows the
       common hooks dir /home/kara/.hermes/cache/scratch/rel009-repro/.git/hooks, so a hook installed there would never run.
       Unset it (git config --unset-all core.hooksPath) and re-run.
rc=1
```

Comparison is made against `git rev-parse --git-common-dir` + `/hooks`, never
against `--git-path hooks` (which itself follows `core.hooksPath` and would always
compare equal to the value being tested — an earlier draft of the installer had
exactly that hole; it was found and fixed before this evidence run).

## 5. Guard strength preserved (no weakening)

Nothing that blocked before is warn-only now. Red-proof, on a staged probe file:

```
$ printf 'DEEPSEEK_API_KEY=deadbeefdeadbeefdeadbeefdeadbeef\n' > zz-rel009-hook-probe.txt
$ git add -- zz-rel009-hook-probe.txt
$ git hook run pre-commit
▸ GitReins pre-commit — repo: /home/kara/.hermes/cache/scratch/wt-rel009-1337
1:DEEPSEEK_API_KEY=deadbeefdeadbeefdeadbeefdeadbeef
╔══════════════════════════════════════════════════════════╗
║  SECRET DETECTED in zz-rel009-hook-probe.txt
║  Remove secrets before committing.
╚══════════════════════════════════════════════════════════╝

COMMIT BLOCKED: secrets found in staged files.
Remove secrets or add files to .gitignore, then try again.
board-lint: ok (/home/kara/.hermes/cache/scratch/wt-rel009-1337/.coding-hermes/board)
rc=1
$ bash "$(git rev-parse --git-path hooks)/pre-commit.pre-rel009.bak"   # same input, old hook
old_hook_rc=1
```

Board-lint keeps its warn-only/skip semantics: a missing, slow or wedged
`boardctl` prints a SKIP line and the commit proceeds — the block is wrapped in a
timeout subshell and never returns non-zero to the caller.

## 6. Gates (run once, sequentially, this worktree)

| gate | command | exit |
|---|---|---|
| 1 | `go build ./...` | `0` |
| 2 | `go vet ./...` | `0` |
| 3 | `go test ./... -count=1` | `0` (see below) |
| 4 | `python3 scripts/audit/wirecheck.py .` | `0` — `RESULT: PASS` |

`wirecheck` printed 24 `!! … fixture supplies one` notes (production NOT NULL
columns populated by the fixture) and finished `RESULT: PASS` with exit 0.

`go test ./... -count=1`: **37 packages `ok`**, 3 `[no test files]`, no `FAIL`,
`gate3_test_rc=0`, completed in well under the 15-minute limit; the tail:

```
ok   github.com/wojons/consensus/internal/tools      0.286s
ok   github.com/wojons/consensus/internal/web        0.010s
ok   github.com/wojons/consensus/internal/webhook    0.480s
ok   github.com/wojons/consensus/pkg/client          0.022s
ok   github.com/wojons/consensus/scripts/loadprobe   0.018s
?    github.com/wojons/consensus/specs              [no test files]
gate3_test_rc=0
```

The guards also fired on the commits made *from this worktree* while the row was
being closed — the arm/dismount pair proving acceptance 1 in one transcript:

```
$ git commit -F /tmp/rel009-msg-20261121b.txt
▸ GitReins pre-commit — repo: /home/kara/.hermes/cache/scratch/wt-rel009-1337
✓ GitReins: Secrets scan — PASS
✓ GitReins: Go vet — SKIP (no Go changes)
✓ GitReins: Go test -short — SKIP (no Go changes)
board-lint: ok (/home/kara/.hermes/cache/scratch/wt-rel009-1337/.coding-hermes/board)
```

## 7. Scope honored

- No tag, no GitHub Release, `VERSION` and `CHANGELOG.md` untouched.
- No board rows rewritten; row `RELEASE-CONSENSUS-009` left `pending` with
  `commit_hash: null` for the foreman to merge and re-run.
- No `gitreins task complete` (Tier-2 judge deferred at host load).
- The product checkout was read for measurement only; the one piece of named state
  changed is `.git/hooks/pre-commit` itself (that *is* this row's fix landing), with
  the pre-row hook preserved alongside it as `pre-commit.pre-rel009.bak`.

## 8. Disclosure: automation committed part of this work

A fleet harvester committed the two source files from this worktree mid-task as
`a0a3727d` ("fix(hooks): derive hook + board roots at runtime, add shared-hook
installer", author `totalwindupflightsystems <totalwindupflightsystems@gmail.com>`)
and pushed `rel009-worktree-hooks`. Its tree is byte-identical to the files
verified above (`sha256 9cbd2a39…` for `scripts/install-hooks.sh` in that commit and
on disk). The documentation and this evidence file land in the follow-up commit
with the row's required message and `Co-authored-by` trailer; history was not
rewritten (no force-push).
