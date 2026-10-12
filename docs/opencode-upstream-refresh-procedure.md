# opencode upstream refresh procedure

**Contract:** `specs/024-shim-support-matrix.md` §A — "Client-facing: the opencode protocol".
**Standing obligation:** board row `SHIM-SOURCE-001`.
**Owner of the pin:** the Consensus shim (`internal/shim/opencode/`); the compatibility
suite that proves it (`scripts/opencode-upstream/`); the evidence that records it
(`docs/evidence/opencode-upstream-*`).

This file is the procedure, not a memory. It exists so "which opencode are we compatible
with, and how do we know?" is answerable with commands instead of archaeology. Run it
top-to-bottom on every refresh; each step states what it produces and what "done" means.

---

## 0. What is pinned, and where

There are **two pins**, and they are deliberately separate because they answer two
different questions. A refresh updates both or it is not a refresh.

| Pin | File | Question it answers | Fields |
|---|---|---|---|
| **Suite pin** | `scripts/opencode-upstream/manifest.json` | *which upstream checkout do we execute their tests against?* | `repository`, `revision`, `version`, `lock_sha256`, `suites[].{path,sha256,transport}` |
| **Protocol pin** | `specs/openapi/opencode-pin.yaml` | *which published protocol document is our declared surface compared against?* | `pin.commit`, `pin.version`, `document.sha256`, `document.git_blob_sha1`, `document.path_count`, `previous_pin`, `other_lineages` |

Supporting, non-pin artifacts (regenerated/compared, never the source of truth):

| Artifact | Role |
|---|---|
| `specs/openapi/upstream/openapi-<version>.json` | the published document, verbatim, at the pinned commit |
| `specs/openapi/upstream/openapi-<version>.surface.json` / `.surface.txt` | the extracted declared surface (paths + methods + schema names) |
| `specs/openapi/upstream/consensus-shim-served-surface.yaml` | the **served** side of the route-level comparison, derived from `internal/shim/opencode/server.go` with `file:line` evidence on every row |
| `specs/openapi/upstream/opencode-declared-vs-served-<version>.json` | the machine-readable route-level comparison |
| `docs/evidence/opencode-upstream-v<version>/` | the suite measurement at that pin (summary + per-suite logs + `results.tsv`), and the surface-diff report |

**Invariant:** the pin is never implied. If it cannot be read out of the two files above,
the refresh has not happened.

---

## 1. Preflight

```bash
cd <consensus worktree>
git pull --rebase                # or confirm you are on the intended revision
command -v git curl jq node bun python3 sha256sum   # all required
python3 -c 'import yaml'                            # PyYAML, for the document comparison
git status --short                                  # expect clean; the shim tree must be yours
```

Ports used by this procedure: the shim's own port (this document uses `18242`, the port
the recorded evidence was produced on). The `8199` in some board rows is a placeholder —
substitute whatever port your server actually binds, and record it in the evidence.

---

## 2. Resolve the upstream ref — never guess it

The upstream project is `sst/opencode`, which **redirects** to `anomalyco/opencode`; the
document path is `packages/sdk/openapi.json`.

```bash
# 2.1 the redirect (recorded so the two repo names in our files are not a mystery)
curl -sS -o /dev/null -w '%{redirect_url}\n' https://github.com/sst/opencode

# 2.2 the default branch and the tip commit on it
curl -sS https://api.github.com/repos/anomalyco/opencode | jq -r .default_branch
git ls-remote https://github.com/sst/opencode HEAD

# 2.3 the version the checkout carries (the pin's `version` field is THIS, not the tip tag)
curl -sS https://raw.githubusercontent.com/sst/opencode/<COMMIT>/packages/opencode/package.json | jq -r .version

# 2.4 the release tag nearest that branch, for the record
git ls-remote --tags https://github.com/sst/opencode | grep -E "v1\.[0-9]+\.[0-9]+$" | sort -V | tail -5
```

**2.5 The second lineage.** Upstream also publishes a `2.0` branch. At the time of the
last refresh it did **not** carry `packages/sdk/openapi.json` (404 on
`raw.githubusercontent.com`, and the contents API for `packages/sdk` lists an
effect-based `src/` tree instead), so it cannot be pinned the same way. Re-check it every
refresh and record the result under `other_lineages` in `opencode-pin.yaml`; do not let
the next tick rediscover it as a surprise:

```bash
curl -sS -o /dev/null -w '%{http_code}\n' \
  https://raw.githubusercontent.com/sst/opencode/1fd016ef32286de9489b7b24f1029f52c49a27b3/packages/sdk/openapi.json
```

**Done when:** `COMMIT`, `VERSION`, `BRANCH`, and the tag list are captured verbatim in the
refresh note. A commit resolved from a wiki page, a memory, or `latest` is not a resolution.

---

## 3. Fetch the protocol document at the pin

```bash
V=1.18.33                     # <VERSION from step 2>
C=<COMMIT from step 2>
curl -sS -o specs/openapi/upstream/openapi-$V.json \
  https://raw.githubusercontent.com/sst/opencode/$C/packages/sdk/openapi.json

sha256sum specs/openapi/upstream/openapi-$V.json          # -> document.sha256
git hash-object specs/openapi/upstream/openapi-$V.json    # -> document.git_blob_sha1
stat -c %s specs/openapi/upstream/openapi-$V.json         # -> document.size_bytes
jq -r '"\(.openapi) \(.info.title) \(.info.version) paths=\(.paths|length) ops=\([.paths[]|to_entries[]|select(.key|IN("get","post","put","patch","delete","head","options"))]|length) schemas=\(.components.schemas|length)"' \
  specs/openapi/upstream/openapi-$V.json
```

**Why the blob SHA matters:** it is the identity GitHub itself uses. Re-fetching the same
path at a different ref and comparing `git hash-object` is how the last refresh proved the
document had *not* moved between v1.18.29 and the dev tip without diffing a megabyte of
JSON. A `sha256sum` match plus a blob match is the proof that the committed file *is* the
upstream file.

---

## 4. Diff the document against the previous pin

Put the previous document next to the new one (it is in the repo at
`specs/openapi/upstream/openapi-<previous-version>.json`; if it is not, fetch it at
`previous_pin.commit` from `opencode-pin.yaml`), then:

```bash
jq -S '.paths|keys' specs/openapi/upstream/openapi-<old>.json | sort > /tmp/old.paths
jq -S '.paths|keys' specs/openapi/upstream/openapi-$V.json   | sort > /tmp/new.paths
diff /tmp/old.paths /tmp/new.paths            # paths added / removed
jq -S '.components.schemas|keys' …old… | sort > /tmp/old.schemas
jq -S '.components.schemas|keys' …new… | sort > /tmp/new.schemas
diff /tmp/old.schemas /tmp/new.schemas        # schemas added / removed
diff <(jq -S '.paths' …old…) <(jq -S '.paths' …new…)              # per-operation changes
diff <(jq -S '.components.schemas' …old…) <(jq -S '.components.schemas' …new…)
```

**Report this even when it is empty.** "0 paths added, 0 removed, 0 changed" is a
result: on an unchanged document the useful output of a refresh is §7 (declared vs
served) and §8 (the suite number), because a shim-side regression looks exactly like
"no upstream news".

---

## 5. Update the pins

### 5.1 `scripts/opencode-upstream/manifest.json`

| Field | How to derive it |
|---|---|
| `repository` | `https://github.com/anomalyco/opencode.git` (the redirect target; the runner clones this) |
| `revision` | `COMMIT` from step 2 |
| `version` | `packages/opencode/package.json` `.version` at that commit |
| `lock_sha256` | `curl -sS https://raw.githubusercontent.com/sst/opencode/$C/bun.lock \| sha256sum` (the runner verifies this before anything else) |
| `suites[].path` | leave as-is unless upstream moved a suite; the four paths are the suite definition |
| `suites[].sha256` | **pristine** hash of the file at the pinned commit — verify with `git show HEAD:<path> \| sha256sum` in a clean checkout, or `curl …/<path> \| sha256sum`. Never hash a patched working tree |
| `document` | keep in step with `opencode-pin.yaml` §3 (path, commit, sha256, blob, counts) |

Verify the hashes you just wrote against a **pristine** checkout, not your working tree:

```bash
CACHE=~/.cache/consensus/opencode-<COMMIT>
for f in $(jq -r '.suites[].path' scripts/opencode-upstream/manifest.json); do
  printf '%s  %s\n' "$(git -C $CACHE show HEAD:$f | sha256sum | cut -d' ' -f1)" "$f"
done
```

### 5.2 The runner and the test that makes the pin un-silent

Two places hard-code the pin on purpose. Update them in the same commit:

* `scripts/test-opencode-upstream.sh` — the `REVISION` / `VERSION` / `REPOSITORY`
  constants near the top (`--self-test` fails the run if they disagree with the manifest).
* `internal/chronicle/opencode_upstream_runner_test.go` — the pinned revision / version /
  lock strings. This test exists so a pin cannot change silently; it is expected to fail
  until you update it, and that failure is the point.

### 5.3 `specs/openapi/opencode-pin.yaml`

Update `pin.*`, move the outgoing pin to `previous_pin` (with its `evidence_dir`), and
refresh `document.*` (fetch URL, hashes, size, counts, `surface_listing`) and
`document_history_check` (fetch the path at every ref between the old and new pin and
compare blob SHAs — the cheap proof that "no drift" is measured, not assumed).

Then regenerate the surface listing:

```bash
bash scripts/extract-opencode-surface.sh \
  specs/openapi/upstream/openapi-$V.json specs/openapi/upstream/openapi-$V
# -> openapi-$V.surface.json + openapi-$V.surface.txt (jq only, no network)
```

---

## 6. Serve the shim and capture the *served* surface

The **declared** side is the upstream document. The **served** side is the running shim.
Two different things are compared, and each has its own script:

| Comparison | Declared | Served | Script | Answers |
|---|---|---|---|---|
| **Document-level** | upstream `openapi-$V.json` | live `GET /openapi.json` (= `GET /doc`; the embedded `specs/openapi/bundled.yaml`) | `scripts/opencode-surface-document-diff.py` | which paths, operations and **schemas** were added / removed / modified |
| **Route-level** | upstream `openapi-$V.surface.json` | `specs/openapi/upstream/consensus-shim-served-surface.yaml` (static derivation from `server.go`) | `scripts/compare-opencode-declared-served.py` | for each declared operation, is it **answered**, and with a declared status |

```bash
# 6.1 build and serve from the tree you intend to compare
CGO_ENABLED=0 go build -o /tmp/consensus-refresh ./cmd/consensus
CONSENSUS_PORT=18242 CONSENSUS_HOSTNAME=127.0.0.1 \
CONSENSUS_DB_URL="sqlite:///tmp/consensus-refresh/scratch.db" \
DEEPSEEK_API_KEY="$(cat ~/.hermes/DEEPSEEK_API_KEY)" \
  /tmp/consensus-refresh serve > /tmp/consensus-refresh/serve.log 2>&1 &
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18242/global/health   # expect 200

# 6.2 capture BOTH served documents and prove they are the same document
curl -sS http://127.0.0.1:18242/openapi.json -o /tmp/served-openapi.json
curl -sS http://127.0.0.1:18242/doc         -o /tmp/served-doc.json
cmp <(jq -S . /tmp/served-openapi.json) <(jq -S . /tmp/served-doc.json) && echo STRUCTURALLY_IDENTICAL
sha256sum /tmp/served-openapi.json

# 6.3 document-level diff (schema-level detail)
python3 scripts/opencode-surface-document-diff.py \
  specs/openapi/upstream/openapi-$V.json /tmp/served-openapi.json /tmp/opencode-document-diff

# 6.4 route-level diff (which declared operation is answered)
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-$V.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml \
  /tmp/opencode-declared-vs-served
```

**The served-surface YAML is the one artifact that rots.** It is a static derivation of
the route table; nothing regenerates it automatically. After a shim route lands, its
outcome can be stale even though the comparison still runs green. Three defences, all
mandatory in a refresh:

1. `grep -n` the routes whose rows you touch against `internal/shim/opencode/server.go`
   (`file:line` evidence is on every row) and update the row when the handler moved.
2. Re-run the comparison and **compare it byte-for-byte** with the committed artifact
   (`diff <(jq -S . /tmp/…/declared-vs-served.json) specs/openapi/upstream/opencode-declared-vs-served-$V.json`).
   A clean diff means the comparison is reproducible; it does **not** mean the served
   side is current.
3. **Live-probe every OUTCOME-MISMATCH / STUB row against the running shim** (step 7.2).
   That is the only measurement that catches a rotted row.

---

## 7. Compare, and turn drift into rows

### 7.1 Classify

The route-level comparison emits, with an id on every row:
`appendix-drift-declared.md` (declared, not served), `appendix-drift-served.md` (served,
not declared), `appendix-narrowed.md` (declared success unreachable),
`appendix-intentional-narrowing.md` (spec-owned exclusions answered by typed `501`),
`appendix-semantic.md` (same path+method, different operation),
`appendix-covered.md`, `appendix-shim-extensions.md`, `family-summary.md`.

A served route that upstream does not declare is *drift* unless the served-surface file
declares it as an intentional extension (`x-shim-extension: true` **plus** an
`extension-rationale`). The comparison **aborts** on an extension without a rationale —
an unexplained extension must never pass as accounted divergence.

A declared operation may be intentionally narrowed only when its served row sets
`x-shim-intentional-narrowing: true`, supplies a non-empty
`narrowing-rationale`, and reports `outcome: 501-typed`. The comparison aborts
otherwise. These rows are accounted separately from both full upstream coverage
and drift; the marker does not turn an unsupported operation into a fake success.

### 7.2 Live-probe the ambiguous rows

Static derivation says what the router *should* answer. Probe the ones that matter:

```bash
KEY=$(grep -oE 'key=cs_ak_[0-9a-f]+' /tmp/consensus-refresh/serve.log | head -1 | cut -d= -f2)
curl -s -o /dev/null -w '%{http_code}\n' -X POST -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' -d '{}' http://127.0.0.1:18242<path>
```

Two traps learned the hard way:

* **A path can answer differently depending on whether the resource exists.** `POST
  /session/{id}/fork` answers a typed `404` for an unknown session and `200` for a real
  one. Create a real session first (`POST /session`) and probe both, or you will file a
  drift row that does not exist.
* **An unauthenticated probe returns `401`, not the outcome.** Load the admin key from the
  server log into a shell variable (never paste it into a report or a commit).

### 7.3 File the rows

Drift that is *work* becomes a board row in `.coding-hermes/board/tasks.jsonl`. That file
has concurrent writers (foremen, stand-in PM, sibling ticks), so file through the
fleet's board appender / the foreman rather than a hand-rolled `>>`: one JSON object per
line, one row per drifted path+method.

```
{
  "id": "SHIM-DRIFT-<family>-<n>", "status": "pending", "priority": "P2",
  "complexity": 2, "attempts": 0, "branch": null, "commit_hash": null,
  "ci_result": null, "guard_result": null, "review_notes": null,
  "sessions": null, "worker_status": null,
  "source": "shim-source-refresh-<yyyy-mm-dd>",
  "capability_tags": ["opencode", "protocol", "spec-drift"],
  "title": "[P2] <path> <METHOD>: declared <codes>, served <outcome>",
  "detail": "<class> per <evidence dir>/surface-diff-report.md <row id>. Declared: <codes>. Served: <outcome> at <commit>. Evidence: <file:line or probe line>.",
  "reasoning": "WHY: <what a client loses while this is wrong>.",
  "created_at": "<yyyy-mm-dd HH:MM:SS>", "updated_at": "<yyyy-mm-dd HH:MM:SS>"
}
```

Rules that keep the register honest:

* one row per drifted **path+method**, with the drift class and the evidence id in the
  text — a row that says "the surface drifted" cannot be closed;
* the row's evidence must be re-findable: `file:line`, or a probe command plus its output;
* a drift row is filed even when the decision is "we will never serve this" — the decision
  *is* the deliverable, recorded in the row (`SHIM-GAP-001` exists for exactly this);
* when a comparison row's finding is that **our own served-surface file is stale**, file
  that too: it is a defect in the artifact, not in the shim.

---

## 8. Re-run the upstream compatibility suite at the pin

The suite is 34 upstream-owned assertions in four files, run against a live shim. The
number is only meaningful with the pin attached.

```bash
sh scripts/test-opencode-upstream.sh \
  --base-url http://127.0.0.1:18242 \
  --evidence-dir docs/evidence/opencode-upstream-v$V
```

* Use a **fresh evidence directory per run** (`…-v1.18.33`, then `…-v1.18.33-r2`, `-r3`, …).
  Never overwrite the measurement a report cites; a re-run at the same pin is a new
  measurement and gets a new directory.
* The runner refuses to proceed if the checkout revision does not match the pin, if
  `bun.lock` or any suite file has drifted, or if `packages/opencode/package.json` version
  disagrees with the manifest. Those refusals are the pin's teeth — do not work around them.
* `--self-test` runs the offline runner/adapter contract check (the adapter may not touch
  any upstream assertion or test registration).
* Classification is per suite: `PASS`, `DIVERGENCE` (upstream assertion executed and
  failed — a compatibility gap, **not** a skip), `SETUP_FAILURE`, `RUN_ERROR`. A
  `SETUP_FAILURE` is never reported as a compatibility number.
* The suite needs a shim started with `DEEPSEEK_API_KEY` present (the shim's LLM calls);
  without it, classifications are not comparable with the recorded series.
* The runner's fetch preload authenticates with `Basic opencode:<admin key>` **only when
  `CONSENSUS_OPENCODE_PASSWORD` is set** in the runner's environment. Run it unarmed and
  every shim request answers `401`: the count can still come out the same while the
  *cause* of each failure is an auth artifact instead of the compatibility gap being
  measured. Export the admin key the server printed at startup:

  ```bash
  export CONSENSUS_OPENCODE_PASSWORD=$(grep -oE 'key=cs_ak_[0-9a-f]+' /tmp/consensus-refresh/serve.log | head -1 | cut -d= -f2)
  ```
* The admin key is a credential: read it from the server log into an environment variable,
  never paste it into a report, an evidence file, or a commit.

Record in the report: pin (revision + version + lock sha + suite file shas), the shim
commit and base URL, per-suite ran/pass/fail, the named failing assertions with their
verbatim output, and the previous measurement for comparison.

**If a re-run is not possible** (no bun, no network, no shim), say exactly that and cite
the last recorded measurement with its pin — an unverified number is never presented as
current.

---

## 9. Land it

```bash
git add scripts/opencode-upstream/manifest.json \
        scripts/test-opencode-upstream.sh \
        scripts/opencode-surface-document-diff.py \
        specs/openapi/opencode-pin.yaml \
        specs/openapi/upstream/… \
        internal/chronicle/opencode_upstream_runner_test.go \
        docs/opencode-upstream-refresh-procedure.md \
        docs/evidence/opencode-upstream-v$V/…
git commit -F /tmp/msg.txt -- <the same paths>     # path-limited if siblings are live
git push origin master
gitreins task complete SHIM-SOURCE-001
```

Commit message shape: `shim: refresh opencode pin to <version> (<commit>), re-measure suite
and diff the surface`. The message must name the pin — the pin in the diff is the record.

**Then re-open the obligation.** `SHIM-SOURCE-001` is a *standing* obligation (specs/024
§A): closing it does not retire it. A refresh that lands is a refresh performed; the next
one is due when upstream moves, when a shim route lands, or on the fleet's cadence.

---

## 10. Failure modes seen in practice

| Symptom | Cause | Fix |
|---|---|---|
| Runner exits 2 `pinned source drift: <suite>` | the working tree holds a **patched** file (a prior run was SIGKILLed before its `trap … EXIT` restore — the ~420 s command cap can do this) | `git -C ~/.cache/consensus/opencode-<COMMIT> checkout -- <patched paths>`, then re-run; verify with `git show HEAD:<path> \| sha256sum` |
| `git apply --check` fails | checkout left patched, or the adapter no longer applies to a moved suite | restore the checkout; a moved suite is a pin change — re-derive the manifest `sha256` from the new revision |
| Comparison prints a count you cannot reproduce | the served-surface YAML is stale for the shim revision you built | re-derive the row from `server.go`, then re-run; byte-compare with the committed artifact |
| `/doc` returns HTML or a non-parseable body | the shim is serving the Swagger UI path (DF-CONSENSUS-36 regression) | `/doc` must serve the **document**; the interactive UI lives at `/doc/api` |
| Suite classifies `SETUP_FAILURE` for a whole file | bun collection/launcher failure — not a compatibility result | fix the environment; never record it as a pass/fail number |
| `bind: address already in use` on the shim port | another server (or a docker-published one) already owns it | probe the port before starting and pick a free one; record the chosen port in the evidence |
| Evidence directory already exists with results | you are about to overwrite a measurement a report cites | use the next `-rN` suffix |

---

## 11. Transcript of the last refresh (2026-10-11, pin v1.18.33)

Kept here so the commands above are known-good in this repository, not aspirational.

```text
$ git log --oneline -1                          # f4d02b6
$ jq -r '.revision, .version' scripts/opencode-upstream/manifest.json
7945de208964a49300d7f770d1a71d078db9a4c4
1.18.33
$ sha256sum specs/openapi/upstream/openapi-1.18.33.json
00502bd13e9c86f3ca9e765e99a57e06fa9f434ca16f2a714766d1444f8d37f3   # == opencode-pin.yaml document.sha256
$ git hash-object specs/openapi/upstream/openapi-1.18.33.json
e66d140505585ab0bab62640360eb08f249f35c0                          # == document.git_blob_sha1
$ for f in $(jq -r '.suites[].path' scripts/opencode-upstream/manifest.json); do
    git -C ~/.cache/consensus/opencode-7945de20… show HEAD:$f | sha256sum; done
40b0b2a4… httpapi-instance.test.ts   5c77c490… httpapi-sdk.test.ts
fdd44f6f… sdk-error-shape.test.ts    0365ea4f… promise.test.ts        # == manifest suite hashes
$ curl -sS http://127.0.0.1:18242/openapi.json -o /tmp/served-openapi.json   # 174,354 B
$ python3 scripts/opencode-surface-document-diff.py …                        # 69 served paths / 48 schemas
$ python3 scripts/compare-opencode-declared-served.py …                      # 73 covered / 107 drift / 17 extensions
$ diff <(jq -S . /tmp/opencode-declared-vs-served/declared-vs-served.json) \
       specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json       # no output = reproducible
$ sh scripts/test-opencode-upstream.sh --base-url http://127.0.0.1:18242 \
      --evidence-dir docs/evidence/opencode-upstream-v1.18.33-r2
```

The result of that refresh — counts, the four same-name schema collisions, the live probe
of the ambiguous drift rows, and the suite re-measurement — is recorded in
`docs/evidence/opencode-upstream-v1.18.33/surface-diff-report.md`.
