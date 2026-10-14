# opencode protocol — surface diff report at pin `7945de20` (v1.18.33)

**Contract:** `specs/024-shim-support-matrix.md` §A (client-facing opencode protocol),
standing obligation **`SHIM-SOURCE-001`**.
**Procedure that produced it:** `docs/opencode-upstream-refresh-procedure.md`.
**Board row:** `SHIM-SOURCE-001` · **Shim revision compared:** `f4d02b6` (master) ·
**Shim base URL:** `http://127.0.0.1:18242` · **Date:** 2026-10-11.

This is the declared-vs-served surface comparison §A.2/§A.3 requires after a refresh:

* **declared** — the OpenAPI document the upstream project publishes at the pinned commit
  (`packages/sdk/openapi.json`);
* **served** — what the Consensus shim actually publishes and answers, captured live from
  `GET /openapi.json` (identical to `GET /doc`) and from the router (route-level);

with every addition, removal and modification listed, and the compatibility suite
re-measured at the same pin and attached to it.

**Result in one line:** the pin did not move and the *document* did not drift, but the shim
now passes **34/34** upstream assertions (was 32/34), the route-level surface is unchanged
at 73 covered / 107 drift, **two** declared-but-unserved operations still answer `501`, and
**three** of the five recorded `OUTCOME-MISMATCH` rows are stale — our own served-surface
artifact rotted, which is a finding about the artifact, not about the shim.

---

## 1. The pin (canonical, in-repo)

| | Value | Where |
|---|---|---|
| Repository | `https://github.com/anomalyco/opencode.git` (`sst/opencode` redirects here) | `scripts/opencode-upstream/manifest.json` |
| Revision | `7945de208964a49300d7f770d1a71d078db9a4c4` | `manifest.json` `revision` |
| Version | `1.18.33` (`packages/opencode/package.json` at that commit) | `manifest.json` `version` |
| Lock | `bun.lock` sha256 `b68e7ece1128eb383663e55ffca4f9f08e451530cc09fc9cf84c489bf41bdc2a` | `manifest.json` `lock_sha256` |
| Branch / tag | `dev` tip; latest tag `v1.18.33` = `51ef4be1d3c122f18fefb510dca8d778571f4f18` | `specs/openapi/opencode-pin.yaml` |
| Previous pin | `16747470f976aca3d362ad730bcd3fe82ecc2c9a` (v1.18.29) | `opencode-pin.yaml` `previous_pin` |

The pin is recorded twice on purpose — the **suite pin** (`manifest.json`, which checkout the
upstream tests run against) and the **protocol pin** (`opencode-pin.yaml`, which published
document our declared surface is compared against) — and both are enforced in code, not by
convention:

```text
$ sh scripts/test-opencode-upstream.sh --self-test
1..7   # 7 pass 0 fail
self-test: ok            # runner constants == manifest; adapter touches no upstream assertion

$ for f in $(jq -r '.suites[].path' scripts/opencode-upstream/manifest.json); do
    git -C ~/.cache/consensus/opencode-7945de208964a49300d7f770d1a71d078db9a4c4 show HEAD:$f | sha256sum; done
40b0b2a4…  packages/opencode/test/server/httpapi-instance.test.ts     # == manifest suites[0].sha256
5c77c490…  packages/opencode/test/server/httpapi-sdk.test.ts          # == manifest suites[1].sha256
fdd44f6f…  packages/opencode/test/server/sdk-error-shape.test.ts      # == manifest suites[2].sha256
0365ea4f…  packages/client/test/promise.test.ts                       # == manifest suites[3].sha256
```

**`scripts/opencode-upstream/manifest.json` is canonical and was verified, not assumed:**
its four suite hashes were re-derived from `git show HEAD:<path>` in a pristine checkout of
the pinned revision and all four match. The file was extended in this pass with the fields
that were previously implied — `purpose`, `procedure`, `protocol_pin`, the `transport`
adapter hashes, the `document` block, the `comparison` artifact paths and a `consistency`
block naming what enforces the pin. The four hashes the runner reads (`revision`, `version`,
`repository`, `lock_sha256`) and the `suites[]` array are unchanged, and the runner's
`--self-test` plus `internal/chronicle/opencode_upstream_runner_test.go` confirm it.

### 1.1 Document history — the document has not moved

| Ref | Commit | `packages/sdk/openapi.json` blob |
|---|---|---|
| `v1.18.29` (previous pin) | `16747470…` | `e66d1405…` |
| `v1.18.30` … `v1.18.33` | `3104c142…` … `51ef4be1…` | `e66d1405…` |
| `dev` tip (this pin) | `7945de20…` | `e66d1405…` |

Re-verified here from the in-repo copy:

```text
$ sha256sum specs/openapi/upstream/openapi-1.18.33.json
00502bd13e9c86f3ca9e765e99a57e06fa9f434ca16f2a714766d1444f8d37f3   # == opencode-pin.yaml document.sha256
$ git hash-object specs/openapi/upstream/openapi-1.18.33.json
e66d140505585ab0bab62640360eb08f249f35c0                          # == document.git_blob_sha1 (upstream's own identity)
```

**0 paths added, 0 removed, 0 changed; 0 schemas added, 0 removed, 0 changed since the
previous pin.** The byte-identity chain above is the one recorded in
`specs/openapi/opencode-pin.yaml` (`document_history_check`) and in
`docs/reports/shim-source-002-baseline-diff-2026-09-29.md` §3, which fetched the document at
each ref and compared blob SHAs; what this refresh re-verified is that the in-repo copy's
`sha256` and blob still equal the pin's recorded values (commands above), and that the pin
did not move. The useful output of a refresh with an unchanged document is §3–§6, not a
document delta — on an unchanged document a shim-side regression looks exactly like "no
upstream news".

---

## 2. Declared surface (upstream document at the pin)

| | Value |
|---|---|
| In-repo copy | `specs/openapi/upstream/openapi-1.18.33.json` (1,061,286 B) |
| sha256 | `00502bd13e9c86f3ca9e765e99a57e06fa9f434ca16f2a714766d1444f8d37f3` |
| Declares | `openapi: 3.1.0`, `info.title: opencode`, `info.version: 1.0.0` |
| Size | **162 paths · 188 operations · 472 schemas · 38 tags** |
| Extracted listing | `openapi-1.18.33.surface.json`, `openapi-1.18.33.surface.txt` (`scripts/extract-opencode-surface.sh`) |

---

## 3. Served surface (live, at `f4d02b6`)

Captured from a shim built from this tree (`CGO_ENABLED=0 go build ./cmd/consensus`) and
served on `127.0.0.1:18242` with a scratch SQLite database:

```text
$ curl -sS http://127.0.0.1:18242/openapi.json -o served-openapi.json    # 174,354 B
$ curl -sS http://127.0.0.1:18242/doc         -o served-doc.json
$ cmp <(jq -S . served-openapi.json) <(jq -S . served-doc.json) && echo STRUCTURALLY_IDENTICAL
STRUCTURALLY_IDENTICAL
$ sha256sum served-openapi.json
eb8243a9c1c8bbeef4910c003f858b9c6f76481443efa042c0d4142cef211a21
```

| | Value |
|---|---|
| Served path | `GET /openapi.json` (== `GET /doc`, same embedded document) |
| sha256 (compact JSON) | `eb8243a9c1c8bbeef4910c003f858b9c6f76481443efa042c0d4142cef211a21` |
| Committed snapshot | `docs/evidence/opencode-upstream-v1.18.33/served-openapi.json` |
| Declares | `openapi: 3.1.0`, `info.title: Consensus Agent Runtime`, `info.version: 0.1.0` |
| Size | **69 paths · 82 operations · 48 schemas** |

**The served document is the embedded bundle, not the upstream document.** Verified against
the build input:

```text
$ # specs/openapi/bundled.yaml (embedded via specs/embed.go) vs the live /openapi.json
bundled paths: 69  served paths: 69     paths identical: True
bundled schemas: 48 served schemas: 48  components identical: True
```

So `GET /doc` answers *Consensus's own* contract (the shim surface **plus** the native REST
API), which is what §A calls "the served spec" and what `DF-CONSENSUS-36` pinned as "the
document, not a Swagger UI page". It is **not** a copy of the upstream document, and §4.4
shows that this matters.

Route-level served side (the other half): `specs/openapi/upstream/consensus-shim-served-surface.yaml`
— **102 servable route entries**, derived from `internal/shim/opencode/server.go` with
`file:line` evidence on every row.

---

## 4. Document-level diff — declared vs served

Generated with `scripts/opencode-surface-document-diff.py` (placeholders normalized so
`{sessionID}` and `{sessionId}` compare equal; the raw spelling of both sides is kept on
every row). Machine-readable result and the full row listing:

* `document-diff.json` — every row with an id (`DOC-OP-nnn`, `DOC-SCH-nnn`) and schema-level pointers
* `document-diff.md` — the same rows as markdown tables

### 4.1 Counts

| Measure | Declared (upstream) | Served (shim) | Common | Declared only | Served only |
|---|---:|---:|---:|---:|---:|
| paths | 162 | 69 | **28** | 134 | 41 |
| operations | 188 | 82 | **24 exact path+method** | 155 | 49 |
| schemas | 472 | 48 | **4 by name** | 468 | 44 |

| Operation classes | count |
|---|---:|
| identical (same path+method, same operationId/params/responses) | **0** |
| `PATH-REMOVED` (path declared upstream, absent when served) | 152 |
| `PATH-ADDED` (path served, not declared upstream) | 48 |
| `OPERATION-MODIFIED` (same path+method, different declaration) | 33 |
| `OPERATION-REMOVED` / `OPERATION-ADDED` (method-level) | 3 / 1 |

| Schema classes | count |
|---|---:|
| identical | **0** |
| `SCHEMA-MODIFIED` (same name, different shape) | 4 |
| declared-only | 468 |
| served-only | 44 |

### 4.2 The 28 common paths — and why all 24 shared operations still differ

24 path+method operations are declared by **both** documents:

```text
GET /agent            GET /config            GET /config/providers   GET /event
GET /experimental/tool                      GET /experimental/tool/ids
GET /file/content     GET /file/status      GET /find               GET /find/file
GET /find/symbol      GET /global/event     GET /global/health      GET /lsp
GET /mcp              GET /permission       GET /project            GET /provider
GET /session          GET /vcs              GET /vcs/diff
PATCH /config         POST /mcp             POST /session
```

Not one of them is declared **identically**. Every one differs in at least
`operationId` (`session.list` vs `shimListSessions`, `global.health` vs `shimGlobalHealth`, …),
`tags` (`session` vs `Opencode Shim`), and the declared **response set** — typically the
served document adds `401`/`500` and drops the `400` upstream declares on nearly every
operation, and drops the `directory`/`workspace` query parameters that upstream declares
everywhere. Examples verbatim from `document-diff.json`:

```text
GET /session            operationId declared=session.list  served=shimListSessions
                        responses   declared=[200,400]     served=[200,401,500]
                        parameters  declared=[(directory,q,false),(limit,q,false),…]  served=[]
GET /global/health      operationId declared=global.health served=shimGlobalHealth
                        responses   declared=[200,400]     served=[200]
GET /session/{sessionID} (session.get)
                        responses   declared=[200,400,404] served=[200,401,404]
PUT /auth/{providerID}  operationId declared=auth.set     served=shimUpdateAuth
                        parameters  declared=[(providerID,path,true)]  served=[(authId,path,true)]
```

### 4.3 Declared-only (134 paths / 152 operations) and served-only (41 paths / 48 operations), by family

| Declared-only family | ops | | Served-only family | ops |
|---|---:|---|---|---:|
| `/api/*` (the document's v2 surface) | 58 | | `/api/v1/*` (Consensus native REST) | 37 |
| `/experimental/*` | 23 | | `/instance`, `/instance/*` | 4 |
| `/session/*` sub-paths | 16 | | `/tui/*`, `/permission/*` | 4 |
| `/tui/*` sub-paths | 13 | | `/doc`, `/webhooks/*`, `/vcs/*` | 3 |
| `/pty`, `/pty/*` | 8 | | | |
| `/mcp/*` | 6 | | | |
| `/sync/*`, `/global/*` | 8 | | | |
| `/vcs/*`, `/provider/*`, `/project/*` | 9 | | | |
| `/question*`, `/skill`, `/formatter`, `/file`, `/path`, `/log`, `/instance/*`, others | 11 | | | |

The declared-only half is dominated by the document's **v2 surface** (`v2.`-prefixed
operationIds under `/api/*`) plus the `/pty` and `/experimental` families, which the shim
serves none of; that classification work already exists as `SHIM-GAP-001`/`SHIM-GAP-007`
(must-serve / never-serve / defer per family) and is not re-litigated here. The served-only
half is `/doc`, the native `/api/v1/*` REST surface (a different protocol deliberately
served from the same document), and the `/instance*` translations already declared as
intentional extensions under `SHIM-GAP-005`. The tag vocabularies differ the same way: 38
tags upstream (`session`, `config`, `find`, …) vs 13 served (`Opencode Shim`, `MCP`, …).

### 4.4 The four same-name schema collisions — schema-level detail

Four schema **names** exist on both sides with **different shapes** — these are the rows a
client generator would silently get wrong:

| Schema | changes | What it is on each side |
|---|---:|---|
| `Config` | 43 | upstream `Config` (opencode's config document: `agent`, `keybinds`, `layout`, `mcp`, `mode`, `permission`, `provider`, `share`, …) vs the shim's `Config` (`database`, `harness`, `hitl`, `llm`, `logging`, `system_settings`, …). Disjoint property sets |
| `Session` | 40 | upstream `Session` (`id` pattern `^ses`, `slug`, `projectID`, `directory`, `title`, `version`, `time`, `tokens`, `revert`, `share`, …) vs the shim's `Session` (`id` **`format: uuid`**, `agent_name`, `status`, `goal`, `model_id`, `context_budget`, `iteration`, `tokens_used_in/out`, `created_at`, …) |
| `VcsFileDiff` | 9 | upstream declares `patch` (string) and `additions`/`deletions` as **number**; the served type omits `patch` (described as "patch omitted (optional upstream)") and types counts as **integer** |
| `VcsInfo` | 4 | same fields, both optional in the served type; descriptions differ (`default_branch` from origin HEAD) |

The `Session` row is the sharp one: the shim's own session ids are UUIDs
(`794fbf2a-8fe8-468b-a2ed-bc19197eb6f8`, observed below), while upstream's declared pattern is
`^ses`. A client that types `Session` from the served document and validates ids against
upstream's document will disagree with itself. **Row filed (§7, F2).**

---

## 5. Route-level diff — every declared operation, is it answered?

`scripts/compare-opencode-declared-served.py` compares the 188 declared operations against
the shim's router (served-surface YAML) operation by operation:

| Measure | Count |
|---|---:|
| Declared operations | 188 |
| Servable route entries in the shim | 102 |
| **Declared and served as declared** | **73** |
| Drift — declared but not served | **107** |
| — `NOT-SERVED` (no shim route at all → net/http 404) | 102 |
| — `OUTCOME-MISMATCH` (declared success code never answered) | 5 |
| Error-contract-only (declared success unreachable, declared error answered) | 6 |
| Semantic collision (same path+method, different operation) | 2 |
| Served but not declared | **0** (17 routes are *declared* shim extensions, `x-shim-extension` + rationale) |

**Reproducibility check (this refresh):**

```text
$ python3 scripts/compare-opencode-declared-served.py \
    specs/openapi/upstream/openapi-1.18.33.surface.json \
    specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/opencode-declared-vs-served
$ diff <(jq -S . /tmp/opencode-declared-vs-served/declared-vs-served.json) \
       <(jq -S . specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json)
# no output — byte-identical to the committed comparison artifact
```

Drift by path family (declared-not-served only) — the same table the script emits as
`family-summary.md`: `/api/*` 58, `/experimental/*` 23, `/pty/*` 6, `/mcp/*` 6,
`/global/*` 4, `/session/*` 2 (all `OUTCOME-MISMATCH`), `/auth/*` 1, `/pty` 1,
`/tui/*` 1, plus singletons (`/command`, `/file`, `/formatter`, `/sync/*`, `/provider/*`).

---

## 6. Live probe of the five `OUTCOME-MISMATCH` rows — three are stale

The served-surface file is a static derivation; a shim route that lands afterwards can leave
a row describing behaviour the handler no longer has. Every `OUTCOME-MISMATCH` row was
therefore probed against the running shim (admin key read from the server log into a shell
variable; a real session was created first, because a path can answer differently when the
resource exists):

```text
$ POST /session {"title":"SHIM-SOURCE-001 probe"}            -> 200 id=794fbf2a-8fe8-468b-a2ed-bc19197eb6f8
```

| Row | Declared | Recorded served | **Live served (f4d02b6)** | Verdict |
|---|---|---|---|---|
| `DELETE /auth/{providerID}` (auth.remove) | 200, 400 | 405 | **200** `true` | **row stale** — covered |
| `POST /pty` (pty.create) | 200, 400 | 501-typed | **501** `{"error":"not_implemented","operation":"pty.create",…}` | drift stands |
| `POST /session/{sessionID}/fork` (session.fork) | 200, 400, 404 | 501 | **200** for a real session; **404** `{"data":{"message":"Session not found"},"name":"NotFoundError"}` for an unknown one | **row stale** — both declared codes answered |
| `POST /session/{sessionID}/init` (session.init) | 200, 400, 404 | 501 | **400** `{"error":{"code":"INVALID_REQUEST","message":"modelID, providerID and messageID are required"}}` | **row stale** → error-contract-only |
| `POST /tui/submit-prompt` (tui.submitPrompt) | 200, 400 | 501 | **501** `{"error":{"code":"NOT_IMPLEMENTED",…}}` | drift stands |

**Surviving drift: 2 of 5** (`POST /pty`, `POST /tui/submit-prompt` — both answer an
undeclared `501` where upstream declares `200`/`400`). The other three are defects in our
own served-surface artifact: it describes handlers that have since been written, so the
comparison reports drift that no longer exists. That is exactly the failure mode
`SHIM-GAP-005`'s note describes ("four served-surface outcomes rotted to 200 … that a fresh
comparison run had silently re-scored covered") — recurring, and now measured.

---

## 7. The upstream compatibility suite, re-measured at the pin

`scripts/test-opencode-upstream.sh` runs four pinned upstream suites (34 assertions) against
a live shim. Every run below carries the pin, the lock hash and the suite file hashes in its
own `summary.md`.

| Run | Pin | Shim revision | Credential armed | Ran | Pass | Fail | Evidence |
|---|---|---|---:|---:|---:|---:|---|
| recorded 2026-09-30 | `7945de20` / 1.18.33 | **not recorded** (measurement committed `a824256`) | yes | 34 | **32** | 2 | `docs/evidence/opencode-upstream-v1.18.33/` |
| this refresh, r2 | `7945de20` / 1.18.33 | `f4d02b6` | **no** (environment probe) | 34 | 32 | 2 | `docs/evidence/opencode-upstream-v1.18.33-r2/` |
| this refresh, r3 | `7945de20` / 1.18.33 | `f4d02b6` | yes | 34 | **34** | **0** | `docs/evidence/opencode-upstream-v1.18.33-r3/` |
| this refresh, r4 (confirmation) | `7945de20` / 1.18.33 | `f4d02b6` | yes | 34 | **34** | **0** | `docs/evidence/opencode-upstream-v1.18.33-r4/` |

The recorded run's `summary.md` carries the pin, the lock hash, the adapter/preload hashes
and the base URL — but **not the shim revision it measured**, which is why its row says
"not recorded". That is a gap in the evidence shape, not in the measurement: a suite number
is only comparable against another when both the upstream pin *and* the shim revision are
attached. The refresh procedure now requires recording the shim commit alongside the base
URL; r3/r4 carry it here, and the attribution below reconstructs the older one from `git log`.

Per suite at r3/r4 (all `PASS`, exit 0):

| # | suite | ran | pass | fail |
|---:|---|---:|---:|---:|
| 1 | `packages/opencode/test/server/httpapi-instance.test.ts` | 7 | 7 | 0 |
| 2 | `packages/opencode/test/server/httpapi-sdk.test.ts` | 18 | 18 | 0 |
| 3 | `packages/opencode/test/server/sdk-error-shape.test.ts` | 2 | 2 | 0 |
| 4 | `packages/client/test/promise.test.ts` | 7 | 7 | 0 |

**The two assertions that failed at the recorded measurement now pass**, and the attribution
is not a guess — the fix names them:

```text
$ git log -1 --format='%h %ad %s' --date=short f27811b
f27811b 2026-10-09 fix(shim): session-not-found 404 answers upstream NamedError body (SHIM-SUITE33-001)
$ cat docs/evidence/opencode-upstream-v1.18.33-r3/packages_opencode_test_server_sdk-error-shape_test_ts.log
(pass) v2 SDK error shape > 404 with NamedError body throws a real Error carrying the server message [236.89ms]
(pass) v2 SDK error shape > 400 schema rejection: SDK extracts the field-level reason from the NamedError body [4.61ms]
  2 pass / 0 fail
```

`SHIM-SUITE33-001` (board row, `complete`) landed 2026-10-09, nine days **after** the
32/34 measurement; the shim's `/session/{id}` 404 now answers the upstream `NamedError`
envelope, which is precisely the body shape those two assertions assert. The 32/34 figure in
`docs/evidence/opencode-upstream-v1.18.33/summary.md` is therefore a **historical**
measurement at the same pin, superseded by this one; both are kept.

### 7.1 The r2 run is kept on purpose: it measures the arming requirement

r2 was run without `CONSENSUS_OPENCODE_PASSWORD`, so the runner's fetch preload sent no
credential and every shim request answered `401`:

```text
Received: "GET http://test/session/ses_no_such?directory=…  → 401 Unauthorized"
```

The count is identical (32/34) but the *cause* is an auth artifact, not the compatibility
gap — which is why "same number" is not evidence of "same result". The refresh procedure now
carries this as a failure mode; the directory is kept so the next reader can see the
difference rather than re-learn it.

---

## 8. Findings, and the rows they imply

| # | Finding | Evidence | Row |
|---|---|---|---|
| **F1** | The served document declares the same 24 operations under different `operationId`s (`shim*`), different tags, and different response sets (adds 401/500, drops upstream's declared 400, drops `directory`/`workspace` parameters). A client generated from `/doc` is not interchangeable with one generated from upstream's document. §A calls `/doc` "the served spec" — today that is the *Consensus* contract, not the upstream one. | §2 vs §3, `document-diff.json` `DOC-OP-*` | **SHIM-DOC-001** (file) |
| **F2** | Four schema names collide across the two documents with different shapes — `Session` (uuid vs `^ses`, disjoint property sets), `Config`, `VcsFileDiff` (`patch` omitted, number→integer), `VcsInfo`. | §4.4, `document-diff.json` `DOC-SCH-513…516` | **SHIM-DOC-002** (file) |
| **F3** | Our served-surface artifact rotted: 3 of the 5 `OUTCOME-MISMATCH` rows describe handlers that no longer behave that way (auth DELETE 405→200, session fork 501→200/404, session init 501→400). The comparison still runs green against it. | §6 | **SHIM-DOC-003** (file) |
| **F4** | Surviving declared-vs-served drift, 2 operations: `POST /pty` (pty.create) and `POST /tui/submit-prompt` (tui.submitPrompt) answer an undeclared `501` where upstream declares `200`/`400`. | §6, `SHIM-DRIFT-097`, `SHIM-DRIFT-107` | **SHIM-DRIFT-PTY-001**, **SHIM-DRIFT-TUI-001** (file) |
| **F5** | 102 declared operations are not served at all, dominated by the document's `/api/*` v2 surface (58), `/experimental/*` (23) and `/pty/*` (6). Classification already exists (`SHIM-GAP-001` triage, `SHIM-GAP-007` re-triage) — cited, not re-filed. | §5 | existing |
| **F6** | 2 semantic collisions survive on `/mcp` (`mcp.status`/`mcp.add` vs the MCP streamable-HTTP endpoint the shim serves at the same path+method). | §5, `SHIM-GAP-006` | existing |
| **F7** | The compatibility suite is now **34/34** at this pin, reproduced twice. The obligation's number is therefore "34 ran / 34 pass / 0 fail", and any future figure below it is a regression. | §7 | evidence r3/r4 |

F1–F4 are **not** fixed here: this row's deliverable is the refresh procedure, the diff
report and the re-measured number. They are stated as row-ready findings for the foreman to
file (the procedure's §7.3 documents the shape), in the same spirit as `SHIM-SOURCE-002`,
which closed with its findings filed as `SHIM-GAP-001…008` rather than silently backfilled.

---

## 9. Reproduce this report

```bash
# declared
sha256sum specs/openapi/upstream/openapi-1.18.33.json
bash scripts/extract-opencode-surface.sh specs/openapi/upstream/openapi-1.18.33.json /tmp/declared

# served (needs a running shim; see the procedure §6)
CGO_ENABLED=0 go build -o /tmp/consensus-refresh ./cmd/consensus
CONSENSUS_PORT=18242 CONSENSUS_HOSTNAME=127.0.0.1 \
CONSENSUS_DB_URL="sqlite:///tmp/consensus-refresh/scratch.db" \
DEEPSEEK_API_KEY="$(cat ~/.hermes/DEEPSEEK_API_KEY)" /tmp/consensus-refresh serve &
curl -sS http://127.0.0.1:18242/openapi.json -o /tmp/served-openapi.json

# document-level diff (this report's §4; commit the output next to this file)
python3 scripts/opencode-surface-document-diff.py \
  specs/openapi/upstream/openapi-1.18.33.json /tmp/served-openapi.json /tmp/opencode-document-diff

# route-level diff (this report's §5)
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-1.18.33.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/opencode-declared-vs-served

# the compatibility suite, with the credential the preload needs
export CONSENSUS_OPENCODE_PASSWORD=$(grep -oE 'key=cs_ak_[0-9a-f]+' /tmp/consensus-refresh/serve.log | head -1 | cut -d= -f2)
sh scripts/test-opencode-upstream.sh --base-url http://127.0.0.1:18242 \
  --evidence-dir docs/evidence/opencode-upstream-v1.18.33-r5
```

**Artifacts committed with this report:**
`served-openapi.json` (the served document as captured),
`document-diff.json` (machine-readable diff, ids on every row),
`document-diff.md` (the same rows as tables),
`surface-diff-report.md` (this file).
Route-level output stays canonical at
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` (reproduced byte-identical
in §5).

## 10. ROUTE-ADD-104 follow-up — `GET /pty/shells`

The route-level comparison was rerun after ROUTE-ADD-104 added the declared
`pty.shells` operation. The implementation discovers executable regular files
listed by the host's `/etc/shells` (with conventional fallbacks), validates the
optional directory/workspace selectors, and keeps the standard API-key auth.
It does not fabricate a PTY session registry.

The stable declared operation key `/pty/shells` + `GET` + `pty.shells` moved from
`drift_declared_not_served` to `covered`; the canonical route artifact uses the
repository's positional `SHIM-DRIFT-NNN` ids for the remaining drift rows.
The comparator's fresh output was cross-checked against the targeted artifact
flip by stable path/method/operation rows:

```text
{"declared_paths":162,"declared_operations":188,"served_route_entries":103,"covered_operations":74,"narrowed_error_contract_only":6,"intentional_narrowing":1,"semantic_different":2,"drift_declared_not_served":105,"drift_by_class":{"NOT-SERVED":101,"OUTCOME-MISMATCH":4},"drift_served_not_declared":0,"served_drift_by_class":{},"shim_extension_operations":17}
```

> **SUPERSEDED — annotated by SHIM-GAP-005 (2026-10-14), numbers not rewritten.**
> The counts in the block above are the pre-SHIM-GAP-004 snapshot of the
> comparator's output (this report's own capture, kept as evidence of that run).
> `narrowed_error_contract_only` reads `6` here only because the detection rule
> at that revision still relied on a hand-maintained list of error-contract
> rows; SHIM-GAP-004 replaced it with a rule derived from the declared response
> set and the served outcome, which surfaced the remaining rows. The
> authoritative, regenerated artifact is
> `specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`
> (`narrowed_error_contract_only: 14`, `narrowed_unexplained: 0`,
> `covered_operations: 83`, `served_route_entries: 121`); re-derive it with the
> command in `specs/018-openapi-contract.md` §12.4 rather than citing the block
> above as a live count. The narrowing ledger itself lives in
> `specs/018-openapi-contract.md` §12.3.

Live HTTP evidence is recorded in `docs/evidence/ROUTE-ADD-104-live-probe.md`.
