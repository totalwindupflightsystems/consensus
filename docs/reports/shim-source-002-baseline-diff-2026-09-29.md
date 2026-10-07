# SHIM-SOURCE-002 — opencode protocol baseline and surface diff

**Date:** 2026-09-29 · **Contract:** `specs/024-shim-support-matrix.md` §A ·
**Board row:** `SHIM-SOURCE-002` (first execution of the standing obligation
`SHIM-SOURCE-001`) · **Branch:** `wt/SHIM-SOURCE-002`

This is the baseline the standing refresh procedure diffs against. It records
(a) the upstream commit + version the shim is built against, (b) the protocol
document fetched at that commit, (c) the document diff against our previous
suite pin, and (d) a per-path comparison of the *declared* document surface
against the *served* shim surface, with every drift given its own row.

**Artifacts committed by this row**

| Artifact | Purpose |
|---|---|
| `specs/openapi/opencode-pin.yaml` | The pin, in-repo and greppable (§A.1) |
| `specs/openapi/upstream/openapi-1.18.33.json` | The published document, verbatim (1,061,286 B) |
| `specs/openapi/upstream/openapi-1.18.33.surface.json` | Extracted surface: paths + methods + schemas |
| `specs/openapi/upstream/openapi-1.18.33.surface.txt` | Same listing, one line per operation + schema names |
| `specs/openapi/upstream/consensus-shim-served-surface.yaml` | The served surface, derived from `server.go` with line evidence |
| `specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json` | Machine-readable comparison (every drift row carries an id) |
| `scripts/extract-opencode-surface.sh` | Regenerates the surface listing (jq only, no network) |
| `scripts/compare-opencode-declared-served.py` | Regenerates the comparison + drift tables |
| `docs/reports/shim-source-002-baseline-diff-2026-09-29.md` | This report |

---

## 1. The pin

Resolved live, not remembered. Commands and their results (run 2026-09-29):

```text
$ curl -sS -o /dev/null -w '%{redirect_url}' https://github.com/sst/opencode
https://github.com/anomalyco/opencode                      # sst/opencode redirects

$ git ls-remote https://github.com/sst/opencode HEAD
7945de208964a49300d7f770d1a71d078db9a4c4   HEAD

$ curl -sS https://api.github.com/repos/anomalyco/opencode | jq -r .default_branch
dev

$ curl -sS https://raw.githubusercontent.com/sst/opencode/<commit>/packages/opencode/package.json
{ "version": "1.18.33", "name": "opencode", ... }          # version evidence
```

| | Value |
|---|---|
| Upstream repo | `sst/opencode` → redirects to `anomalyco/opencode` |
| Default branch | `dev` |
| **Pinned commit** | `7945de208964a49300d7f770d1a71d078db9a4c4` |
| **Version** | `1.18.33` (`packages/opencode/package.json` at that commit) |
| Latest release tag on that branch | `v1.18.33` = `51ef4be1d3c122f18fefb510dca8d778571f4f18` |
| Previous pin (superseded for the diff) | `16747470f976aca3d362ad730bcd3fe82ecc2c9a` = `v1.18.29` |

**A second lineage exists and is *not* pinnable the same way.** The upstream
project also ships a 2.x line on branch `2.0` (latest tag `v2.0.19` =
`1fd016ef32286de9489b7b24f1029f52c49a27b3`). At that commit
`packages/sdk/openapi.json` **does not exist** — `raw.githubusercontent.com`
answers HTTP 404 and the GitHub contents API lists `packages/sdk/` as
`README.md, package.json, script, src, sst-env.d.ts, test, tsconfig*.json`, i.e.
an effect-based `src/` tree instead of a published OpenAPI document. Recorded in
`opencode-pin.yaml` under `other_lineages` so the next refresh does not spend a
tick rediscovering it. The 1.x `dev` line is the pin because it is the line that
still publishes the document §A names as the source of truth.

---

## 2. The protocol document at the pin

| | Value |
|---|---|
| Path | `packages/sdk/openapi.json` |
| Fetched at | `7945de20…` (the pinned commit) |
| In-repo copy | `specs/openapi/upstream/openapi-1.18.33.json` |
| Size | 1,061,286 bytes |
| Git blob SHA-1 | `e66d140505585ab0bab62640360eb08f249f35c0` |
| SHA-256 | `00502bd13e9c86f3ca9e765e99a57e06fa9f434ca16f2a714766d1444f8d37f3` |
| Declares | `openapi: "3.1.0"`, `info.title: opencode`, `info.version: "1.0.0"` |
| Surface size | 162 paths · 188 operations · 472 schemas · 38 tags · top-level `security: []` |

The in-repo copy's `git hash-object` equals the upstream blob SHA-1
(`e66d1405…`), so the committed file *is* the upstream file.

Surface extraction (re-runnable, jq only):

```text
bash scripts/extract-opencode-surface.sh \
    specs/openapi/upstream/openapi-1.18.33.json \
    specs/openapi/upstream/openapi-1.18.33
```

**Spec-vs-source note (drift row SHIM-SPEC-D001):** `specs/024` §A and the board
row text call the document "OpenAPI **3.1.1**". The published document declares
`"openapi": "3.1.0"`. One of the two is wrong and it is not the file.

---

## 3. Document diff against our suite pin `16747470…` (v1.18.29)

**Result: 0 paths added, 0 paths removed, 0 paths changed. 0 schemas added, 0
removed, 0 changed.**

The assertion is byte identity, not a structural coincidence. The document is a
single blob whose SHA-1 is the same at every ref between our old pin and the
current tip:

| Ref | Commit | `packages/sdk/openapi.json` blob |
|---|---|---|
| `v1.18.29` (previous pin) | `16747470f976aca3d362ad730bcd3fe82ecc2c9a` | `e66d140505585ab0bab62640360eb08f249f35c0` |
| `v1.18.30` | `3104c1428ec91f809e5ab86631300de41eb6952e` | `e66d1405…` |
| `v1.18.31` | `014614d35b397775e5d397a490fc72368c894ec2` | `e66d1405…` |
| `v1.18.32` | `545f51d26cc39a907d2867492d498d9607ea5fa4` | `e66d1405…` |
| `v1.18.33` | `51ef4be1d3c122f18fefb510dca8d778571f4f18` | `e66d1405…` |
| `dev` tip (current pin) | `7945de208964a49300d7f770d1a71d078db9a4c4` | `e66d1405…` |

Independently confirmed by fetching the file at both ends and comparing:

```text
$ sha256sum openapi-1.18.29.json openapi-HEAD.json
00502bd13e9c86f3ca9e765e99a57e06fa9f434ca16f2a714766d1444f8d37f3  openapi-1.18.29.json
00502bd13e9c86f3ca9e765e99a57e06fa9f434ca16f2a714766d1444f8d37f3  openapi-HEAD.json
$ cmp openapi-1.18.29.json openapi-HEAD.json && echo IDENTICAL
IDENTICAL
```

**What that means.** The protocol document did not drift while the shim's suite
was failing 10 assertions at r2 — so those failures were *shim-side*, not
document-side, and a refresh that only diffs the document would have reported
"no news" while the compatibility gap sat there. That is exactly why §A.3
(declared vs served, per path) and §A.4 (re-measure the suite number at the pin)
are part of the same obligation. On an unchanged document the useful output of a
refresh is the comparison in section 4, not section 3.

---

## 4. Declared vs served, per path, at the pinned commit

* Declared: the 162 paths / 188 operations of the document above.
* Served: derived statically from the shim's route table and sub-path switches
  (`internal/shim/opencode/server.go`, function/line evidence on every row in
  `consensus-shim-served-surface.yaml`), including the router's catch-all
  semantics for registered prefixes.
* Every drift below has its own id and row: Appendix A (declared, not served),
  Appendix B (served, not declared), Appendix C (error-contract-only),
  Appendix D (same path+method, different operation).

| Measure | Count |
|---|---|
| Declared operations | 188 |
| Servable route entries in the shim | 71 |
| **Declared and served as declared** | **31** |
| Drift — declared but not served (Appendix A) | 144 |
| Drift — served but not declared (Appendix B) | 17 |
| Error-contract-only — declared success unreachable (Appendix C) | 11 |
| Semantic collision — same path+method, different operation (Appendix D) | 2 |

Drift classes on the declared side: `NOT-SERVED` 111 (no shim route at all),
`ROUTED-404` 13, `STUB-501` 3, `OUTCOME-MISMATCH` 16 (declared success answered
with 501/405/404), `METHOD-MISSING` 1.

*Post-fix (2026-09-29):* the `ROUTED-404` 13, `STUB-501` 3 and `METHOD-MISSING`
1 rows are emptied by SHIM-GAP-002 — see §9 for the updated counts, the named
survivor rows and the re-run upstream suite.

Drift by path family (declared-not-served only):

| family | total | NOT-SERVED | ROUTED-404 | STUB-501 | OUTCOME-MISMATCH | METHOD-MISSING |
|---|---|---|---|---|---|---|
| /api/*  (v2 surface) | 58 | 58 | 0 | 0 | 0 | 0 |
| /experimental/* | 23 | 23 | 0 | 0 | 0 | 0 |
| /session/* | 11 | 0 | 2 | 0 | 8 | 1 |
| /tui/* | 12 | 0 | 8 | 0 | 4 | 0 |
| /pty, /pty/* | 8 | 8 | 0 | 0 | 0 | 0 |
| /mcp/* | 6 | 6 | 0 | 0 | 0 | 0 |
| /global/* | 4 | 4 | 0 | 0 | 0 | 0 |
| /sync/* | 4 | 4 | 0 | 0 | 0 | 0 |
| /project, /project/* | 4 | 0 | 3 | 0 | 1 | 0 |
| /vcs/* | 3 | 0 | 0 | 3 | 0 | 0 |
| /provider/* | 3 | 3 | 0 | 0 | 0 | 0 |
| /auth/* | 1 | 0 | 0 | 0 | 1 | 0 |
| /find/* | 1 | 0 | 0 | 0 | 1 | 0 |
| /instance/* | 1 | 0 | 0 | 0 | 1 | 0 |
| /question | 1 | 1 | 0 | 0 | 0 | 0 |
| /command, /skill, /formatter, /file | 4 | 4 | 0 | 0 | 0 | 0 |

### 4.1 The four findings that matter

**F1 — the document carries a second, larger surface we serve none of.**
`/api/*` is 58 operations with `v2.`-prefixed operationIds and its own tags
(`opencode HttpApi`), alongside the legacy v1 paths in the same document. The
shim registers no `/api/*` route (`server.go:173-239`, `MountPatterns`
`server.go:272-295`); Consensus's own REST API is `/api/v1/*`. Either the shim
targets v1 and the document's v2 half is out of scope (say so, in a spec), or it
is in scope and 58 operations are missing. Rows `SHIM-DRIFT-001…058`.

**F2 — the whole `/instance/*` translation surface we serve is undeclared.**
12 of the 17 served-not-declared rows are `/instance`, `/instance/path`,
`/instance/vcs`, `/instance/vcs/diff` (real translations, served 200, public)
and the nine `/instance/*` 501 stubs (Appendix B); the remaining four are
`/doc`, `/permission/{id}`, `/permission/{id}/resolve` and `/vcs/{id}`. The
document declares only `/instance/dispose`. The pinned upstream suite *requires*
these paths (`serves path and VCS read endpoints`, r3 PASS), so the served
surface is ahead of the published document — the reverse-direction drift §A.3
also asks about.
Rows `SHIM-DRIFT-S001`, `SHIM-DRIFT-S002`, `SHIM-DRIFT-S003`,
`SHIM-DRIFT-S005`, `SHIM-DRIFT-S006…S017`.

**F3 — 16 operations are answered with a status the document does not declare.**
`/tui/append-prompt|submit-prompt|execute-command|show-toast` declare 200/400
and answer 501; `/project` GET, `/session/{id}/share|shell|command|summarize|
init|fork|revert|prompt_async` and `/find/symbol`, `/instance/dispose` likewise;
`/auth/{providerID}` DELETE declares 200/400 and answers 405. These are
deliberate shim stubs, but "deliberate" is not a documented contract until it is
written down: today a client generated from the document will read 501/405 where
the contract promised a success path. Rows `SHIM-DRIFT-*` (class
`OUTCOME-MISMATCH`) plus `SHIM-NARROWED-001…011` where the answer happens to be
the declared *error* code but the declared success is unreachable
(`/session/{id}/todo`, `/session/{id}/message/{id}/part/{partID}`,
`/project/{projectID}` PATCH, `/question/{requestID}/reply|reject`,
`/permission/{requestID}/reply`, …).

**F4 — `/mcp` is the same path+method in two different protocols.**
The document declares `GET /mcp` = `mcp.status` ("Get the status of all Model
Context Protocol (MCP) servers") and `POST /mcp` = `mcp.add` ("Dynamically add a
new MCP server"). The shim serves `GET /mcp` and `POST /mcp` as the MCP
*streamable-HTTP protocol endpoint* itself (JSON-RPC + SSE), delegating to the
real MCP handler when the request looks like an MCP client and answering 501
otherwise. Same path, same method, different operation — the kind of collision a
path-level comparison alone would score as "covered". Rows `SHIM-SEMANTIC-001`,
`SHIM-SEMANTIC-002`.

### 4.2 Document vs its own suite

`GET /doc` is probed by the pinned upstream suite (`httpapi-instance.test.ts`
"serves the OpenAPI document") and **the document does not declare `/doc`**
(verified: `jq '.paths | has("/doc")'` → `false`). The shim serves `/doc` with the
*Consensus* bundle (`specs/openapi/bundled.yaml`), not the opencode document. If
an opencode client reads `/doc` expecting the upstream contract, it gets a
different protocol's schema — a real interoperability trap, and the reason
`SHIM-DRIFT-S001` is not just bookkeeping.

---

## 5. Known open items at the old pin — status

| Item | Old-pin state | State in the r2→r3 evidence | Verdict |
|---|---|---|---|
| `DF-CONSENSUS-47` (missing-project returns 501 instead of typed 404) | open | `httpapi-instance.test.ts` **7 pass / 0 fail** at r3 (`returns typed not found bodies for missing projects` passes) | **fixed**; the served row is `SHIM-DRIFT-S004`/`/project/{projectID}` → typed `ProjectNotFoundError` 404, consistent with the suite |
| `DF-CONSENSUS-39` (project skills in REST API prompt context) | open | `httpapi-sdk.test.ts` **17 pass / 1 fail**, the failing case is still `includes project skills in REST API prompt context` | **still open** |
| `DF-CONSENSUS-40` (400-class validation answered as 404) | open | `sdk-error-shape.test.ts` **0 pass / 2 fail** — `400 schema rejection … Expected 400 Received 404`, and the 404 case still observes `401 Unauthorized` | **still open** |

Evidence: `docs/evidence/opencode-upstream-v1.18.29-r2/` (0/7 on the instance
suite) versus `…-r3/` (7/7). Note the suite numbers above are **at the old pin
`16747470…`**; re-measuring at the new pin is §A.4 and is a residual (section 6).

---

## 6. Residuals — what this row did not do

1. **The suite has not been re-run at the new pin.** §A.4 requires the
   compatibility number re-measured *with the new pin attached*. The number in
   section 5 is pinned to `16747470…`, not to `7945de20…`. Given the document is
   byte-identical this is expected to be a no-op, but "expected" is not
   "measured" — it stays an open item for the next refresh tick.
2. **The served surface is derived statically, not probed live.** It comes from
   the route table, the sub-path switches and the auth middleware; a live
   instance was not driven in this row. `SHIM-SOURCE-003` is the mechanical,
   live form of this check and should be treated as the authority once it lands.
3. **The served-surface file is pinned to a commit.** Its `derived_at_commit`
   is `f012e36684f27b22ee432efc9fbd75abb1bd0945`; the line references stop being
   exact the moment `server.go` changes, so any route change must re-derive it.
4. **The 2.x lineage is recorded, not resolved.** Whether Consensus tracks
   `v2.0.x` (which no longer publishes `packages/sdk/openapi.json`) is a product
   decision, not a measurement.
5. **No new `DF-CONSENSUS-*` ids were allocated.** Drift rows are reported under
   their own `SHIM-DRIFT-*` / `SHIM-NARROWED-*` / `SHIM-SEMANTIC-*` /
   `SHIM-SPEC-D*` ids so this report cannot collide with the existing finding
   series; the finder should carry them over when filing.

---

## 7. Board rows to file from this report

The 174 drift items are enumerated one-per-row in the appendices. Filed as
board rows they collapse into five decisions (each row lists the ids it covers;
the appendix remains the evidence):

| Proposed row | Priority | Covers | Decision needed |
|---|---|---|---|
| `SHIM-SURFACE-V2-001` | P2 | `SHIM-DRIFT-001…058` | Is the document's `/api/*` v2 surface (58 operations) in scope for the shim? If no, record the exclusion in `specs/017`/`024`; if yes, it is a project-sized build. |
| `SHIM-SURFACE-INSTANCE-001` | P2 | `SHIM-DRIFT-S001…S017` | Our served-but-undeclared surface (`/instance/*`, `/permission/{id}`, `/doc`, `/permission/{id}/resolve`, `/vcs/{id}`) needs a declared home: either our own `/doc` documents the shim's extensions, or the upstream document is behind and the divergence is recorded as intentional. |
| `SHIM-SURFACE-STUBS-001` | P2 | 16 `OUTCOME-MISMATCH` + `SHIM-NARROWED-001…011` + 3 `STUB-501` + 111 `NOT-SERVED` | Per family: implement, or declare the gap in-repo (a served-vs-declared intent table). Today a document-generated client sees 501/405/404 on declared success paths with no in-repo statement saying so. |
| `SHIM-SURFACE-DOC-001` | P3 | `SHIM-DRIFT-S001`, `SHIM-SEMANTIC-001…002` | `/doc` returns the Consensus bundle (suite-required, document-undeclared); `/mcp` GET+POST are two different operations in the two protocols. Both need an explicit interoperability note. |
| `SHIM-SPEC-D001` | P3 | spec text | `specs/024` §A says "OpenAPI 3.1.1"; the document declares 3.1.0. Fix the spec (or the claim's source). |

`SHIM-SOURCE-003` (automate declared-vs-served in a gate) is the mechanism that
keeps all of this true; `scripts/compare-opencode-declared-served.py` is already
the comparison half of it.

---

## 8. Reproduce

### 8.1 Re-run the 46-check Go-port grade

The compatibility number is now backed by one fresh-clone entry point:

```bash
scripts/opencode-compat.sh
```

The keyless run takes about one minute (56.07 seconds in this worktree). It
runs 44 checks without a live credential: 39 graded checks plus five named
C19/C20 exclusions. It does not pretend that the two LLM-path checks ran.
C09 and `TestShimRealLLMSessionLifecycle` require `DEEPSEEK_API_KEY` and can be
included, at API cost, with `scripts/opencode-compat.sh --with-live-llm`.
C19/C20 remain excluded because they are pre-existing tracked compatibility
work; this report does not close or fix them. This is the secondary Go-port
score, not a substitute for `scripts/test-opencode-upstream.sh` and its literal
upstream TypeScript suites.

Actual keyless output recorded on 2026-09-29:

```text
KNOWN EXCLUSIONS C19/C20: 5 (pass=5 fail=0 skip=0)
SUMMARY pass=39 fail=0 skip=0 excluded=5 live_key_not_run=2 inventory=46
GRADE 39/39 graded checks passed
```

### 8.2 Re-run the source/surface comparison

```bash
# 1. resolve the pin
git ls-remote https://github.com/sst/opencode HEAD
curl -sS https://api.github.com/repos/anomalyco/opencode | jq -r .default_branch
curl -sS https://raw.githubusercontent.com/sst/opencode/<commit>/packages/opencode/package.json | jq -r .version

# 2. fetch the document at that commit and record it
curl -sS -o specs/openapi/upstream/openapi-<version>.json \
  https://raw.githubusercontent.com/sst/opencode/<commit>/packages/sdk/openapi.json
sha256sum specs/openapi/upstream/openapi-<version>.json
git hash-object specs/openapi/upstream/openapi-<version>.json   # == upstream blob

# 3. derive the surface listing
bash scripts/extract-opencode-surface.sh \
  specs/openapi/upstream/openapi-<version>.json specs/openapi/upstream/openapi-<version>

# 4. diff the document against the previous pin (blob identity at both refs)

# 5. compare declared vs served, one drift row per item
python3 scripts/compare-opencode-declared-served.py \
  specs/openapi/upstream/openapi-<version>.surface.json \
  specs/openapi/upstream/consensus-shim-served-surface.yaml /tmp/shim-source-refresh
```

Honest limits of the comparison: the served side is a static derivation (no live
probe), the shim's auth policy is recorded per route but the comparison is about
path/method/outcome only, and `served_code()` maps a conditional outcome
(`200-mcp-delegate`) to its success code. None of these change a drift verdict;
all three are inputs to `SHIM-SOURCE-003`.

---

## Appendix A — declared operations the shim does not serve (144 rows)

One row per drift. `class` is the router's actual answer, `operationId` and
`declared_responses` come from the document, `served_outcome` from the
served surface.

| id | path | method | class | operationId | declared_responses | served_outcome | detail |
|---|---|---|---|---|---|---|---|
| SHIM-DRIFT-001 | /api/agent | GET | NOT-SERVED | v2.agent.list | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-002 | /api/command | GET | NOT-SERVED | v2.command.list | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-003 | /api/credential/{credentialID} | DELETE | NOT-SERVED | v2.credential.remove | 204,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-004 | /api/credential/{credentialID} | PATCH | NOT-SERVED | v2.credential.update | 204,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-005 | /api/event | GET | NOT-SERVED | v2.event.subscribe | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-006 | /api/fs/find | GET | NOT-SERVED | v2.fs.find | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-007 | /api/fs/list | GET | NOT-SERVED | v2.fs.list | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-008 | /api/fs/read/* | GET | NOT-SERVED | v2.fs.read | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-009 | /api/health | GET | NOT-SERVED | v2.health.get | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-010 | /api/integration | GET | NOT-SERVED | v2.integration.list | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-011 | /api/integration/attempt/{attemptID} | DELETE | NOT-SERVED | v2.integration.attempt.cancel | 204,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-012 | /api/integration/attempt/{attemptID} | GET | NOT-SERVED | v2.integration.attempt.status | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-013 | /api/integration/attempt/{attemptID}/complete | POST | NOT-SERVED | v2.integration.attempt.complete | 204,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-014 | /api/integration/{integrationID} | GET | NOT-SERVED | v2.integration.get | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-015 | /api/integration/{integrationID}/connect/key | POST | NOT-SERVED | v2.integration.connect.key | 204,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-016 | /api/integration/{integrationID}/connect/oauth | POST | NOT-SERVED | v2.integration.connect.oauth | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-017 | /api/location | GET | NOT-SERVED | v2.location.get | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-018 | /api/model | GET | NOT-SERVED | v2.model.list | 200,400,401,503 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-019 | /api/permission/request | GET | NOT-SERVED | v2.permission.request.list | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-020 | /api/permission/saved | GET | NOT-SERVED | v2.permission.saved.list | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-021 | /api/permission/saved/{id} | DELETE | NOT-SERVED | v2.permission.saved.remove | 204,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-022 | /api/provider | GET | NOT-SERVED | v2.provider.list | 200,400,401,503 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-023 | /api/provider/{providerID} | GET | NOT-SERVED | v2.provider.get | 200,400,401,404,503 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-024 | /api/pty | GET | NOT-SERVED | v2.pty.list | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-025 | /api/pty | POST | NOT-SERVED | v2.pty.create | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-026 | /api/pty/{ptyID} | DELETE | NOT-SERVED | v2.pty.remove | 204,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-027 | /api/pty/{ptyID} | GET | NOT-SERVED | v2.pty.get | 200,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-028 | /api/pty/{ptyID} | PUT | NOT-SERVED | v2.pty.update | 200,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-029 | /api/pty/{ptyID}/connect | GET | NOT-SERVED | v2.pty.connect | 200,400,401,403,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-030 | /api/pty/{ptyID}/connect-token | POST | NOT-SERVED | v2.pty.connectToken | 200,400,401,403,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-031 | /api/question/request | GET | NOT-SERVED | v2.question.request.list | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-032 | /api/reference | GET | NOT-SERVED | v2.reference.list | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-033 | /api/session | GET | NOT-SERVED | v2.session.list | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-034 | /api/session | POST | NOT-SERVED | v2.session.create | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-035 | /api/session/active | GET | NOT-SERVED | v2.session.active | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-036 | /api/session/{sessionID} | GET | NOT-SERVED | v2.session.get | 200,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-037 | /api/session/{sessionID}/agent | POST | NOT-SERVED | v2.session.switchAgent | 204,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-038 | /api/session/{sessionID}/compact | POST | NOT-SERVED | v2.session.compact | 204,400,401,404,503 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-039 | /api/session/{sessionID}/context | GET | NOT-SERVED | v2.session.context | 200,400,401,404,500 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-040 | /api/session/{sessionID}/event | GET | NOT-SERVED | v2.session.events | 200,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-041 | /api/session/{sessionID}/history | GET | NOT-SERVED | v2.session.history | 200,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-042 | /api/session/{sessionID}/interrupt | POST | NOT-SERVED | v2.session.interrupt | 204,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-043 | /api/session/{sessionID}/message | GET | NOT-SERVED | v2.session.messages | 200,400,401,404,500 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-044 | /api/session/{sessionID}/message/{messageID} | GET | NOT-SERVED | v2.session.message | 200,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-045 | /api/session/{sessionID}/model | POST | NOT-SERVED | v2.session.switchModel | 204,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-046 | /api/session/{sessionID}/permission | GET | NOT-SERVED | v2.session.permission.list | 200,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-047 | /api/session/{sessionID}/permission | POST | NOT-SERVED | v2.session.permission.create | 200,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-048 | /api/session/{sessionID}/permission/{requestID} | GET | NOT-SERVED | v2.session.permission.get | 200,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-049 | /api/session/{sessionID}/permission/{requestID}/reply | POST | NOT-SERVED | v2.session.permission.reply | 204,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-050 | /api/session/{sessionID}/prompt | POST | NOT-SERVED | v2.session.prompt | 200,400,401,404,409 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-051 | /api/session/{sessionID}/question | GET | NOT-SERVED | v2.session.question.list | 200,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-052 | /api/session/{sessionID}/question/{requestID}/reject | POST | NOT-SERVED | v2.session.question.reject | 204,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-053 | /api/session/{sessionID}/question/{requestID}/reply | POST | NOT-SERVED | v2.session.question.reply | 204,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-054 | /api/session/{sessionID}/revert/clear | POST | NOT-SERVED | v2.session.revert.clear | 204,400,401,404,500 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-055 | /api/session/{sessionID}/revert/commit | POST | NOT-SERVED | v2.session.revert.commit | 204,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-056 | /api/session/{sessionID}/revert/stage | POST | NOT-SERVED | v2.session.revert.stage | 200,400,401,404,500 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-057 | /api/session/{sessionID}/wait | POST | NOT-SERVED | v2.session.wait | 204,400,401,404,503 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-058 | /api/skill | GET | NOT-SERVED | v2.skill.list | 200,400,401 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-059 | /auth/{providerID} | DELETE | OUTCOME-MISMATCH | auth.remove | 200,400 | 405 | declared 200,400, served 405 |
| SHIM-DRIFT-060 | /command | GET | NOT-SERVED | command.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-061 | /experimental/capabilities | GET | NOT-SERVED | experimental.capabilities.get | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-062 | /experimental/console | GET | NOT-SERVED | experimental.console.get | 200,400,500 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-063 | /experimental/console/orgs | GET | NOT-SERVED | experimental.console.listOrgs | 200,400,500 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-064 | /experimental/console/switch | POST | NOT-SERVED | experimental.console.switchOrg | 200 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-065 | /experimental/control-plane/move-session | POST | NOT-SERVED | experimental.controlPlane.moveSession | 204,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-066 | /experimental/project/{projectID}/copy | DELETE | NOT-SERVED | v2.projectCopy.remove | 204,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-067 | /experimental/project/{projectID}/copy | POST | NOT-SERVED | v2.projectCopy.create | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-068 | /experimental/project/{projectID}/copy/generate-name | POST | NOT-SERVED | experimental.projectCopy.generateName | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-069 | /experimental/project/{projectID}/copy/refresh | POST | NOT-SERVED | v2.projectCopy.refresh | 204,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-070 | /experimental/resource | GET | NOT-SERVED | experimental.resource.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-071 | /experimental/session | GET | NOT-SERVED | experimental.session.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-072 | /experimental/session/{sessionID}/background | POST | NOT-SERVED | experimental.session.background | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-073 | /experimental/workspace | GET | NOT-SERVED | experimental.workspace.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-074 | /experimental/workspace | POST | NOT-SERVED | experimental.workspace.create | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-075 | /experimental/workspace/adapter | GET | NOT-SERVED | experimental.workspace.adapter.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-076 | /experimental/workspace/status | GET | NOT-SERVED | experimental.workspace.status | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-077 | /experimental/workspace/sync-list | POST | NOT-SERVED | experimental.workspace.syncList | 204,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-078 | /experimental/workspace/warp | POST | NOT-SERVED | experimental.workspace.warp | 204,400,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-079 | /experimental/workspace/{id} | DELETE | NOT-SERVED | experimental.workspace.remove | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-080 | /experimental/worktree | DELETE | NOT-SERVED | worktree.remove | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-081 | /experimental/worktree | GET | NOT-SERVED | worktree.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-082 | /experimental/worktree | POST | NOT-SERVED | worktree.create | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-083 | /experimental/worktree/reset | POST | NOT-SERVED | worktree.reset | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-084 | /file | GET | NOT-SERVED | file.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-085 | /find/symbol | GET | OUTCOME-MISMATCH | find.symbols | 200,400 | 501 | declared 200,400, served 501 |
| SHIM-DRIFT-086 | /formatter | GET | NOT-SERVED | formatter.status | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-087 | /global/config | GET | NOT-SERVED | global.config.get | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-088 | /global/config | PATCH | NOT-SERVED | global.config.update | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-089 | /global/dispose | POST | NOT-SERVED | global.dispose | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-090 | /global/upgrade | POST | NOT-SERVED | global.upgrade | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-091 | /instance/dispose | POST | OUTCOME-MISMATCH | instance.dispose | 200,400 | 501 | declared 200,400, served 501 |
| SHIM-DRIFT-092 | /mcp/{name}/auth | DELETE | NOT-SERVED | mcp.auth.remove | 200,400,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-093 | /mcp/{name}/auth | POST | NOT-SERVED | mcp.auth.start | 200,400,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-094 | /mcp/{name}/auth/authenticate | POST | NOT-SERVED | mcp.auth.authenticate | 200,400,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-095 | /mcp/{name}/auth/callback | POST | NOT-SERVED | mcp.auth.callback | 200,400,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-096 | /mcp/{name}/connect | POST | NOT-SERVED | mcp.connect | 200,400,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-097 | /mcp/{name}/disconnect | POST | NOT-SERVED | mcp.disconnect | 200,400,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-098 | /project | GET | OUTCOME-MISMATCH | project.list | 200,400 | 501 | declared 200,400, served 501 |
| SHIM-DRIFT-099 | /project/current | GET | ROUTED-404 | project.current | 200,400 | 404-typed | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-100 | /project/git/init | POST | ROUTED-404 | project.initGit | 200,400 | 404-typed | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-101 | /project/{projectID}/directories | GET | ROUTED-404 | project.directories | 200,400 | 404-typed | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-102 | /provider/auth | GET | NOT-SERVED | provider.auth | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-103 | /provider/{providerID}/oauth/authorize | POST | NOT-SERVED | provider.oauth.authorize | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-104 | /provider/{providerID}/oauth/callback | POST | NOT-SERVED | provider.oauth.callback | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-105 | /pty | GET | NOT-SERVED | pty.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-106 | /pty | POST | NOT-SERVED | pty.create | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-107 | /pty/shells | GET | NOT-SERVED | pty.shells | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-108 | /pty/{ptyID} | DELETE | NOT-SERVED | pty.remove | 200,400,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-109 | /pty/{ptyID} | GET | NOT-SERVED | pty.get | 200,400,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-110 | /pty/{ptyID} | PUT | NOT-SERVED | pty.update | 200,400,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-111 | /pty/{ptyID}/connect | GET | NOT-SERVED | pty.connect | 200,403,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-112 | /pty/{ptyID}/connect-token | POST | NOT-SERVED | pty.connectToken | 200,400,403,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-113 | /question | GET | NOT-SERVED | question.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-114 | /session/status | GET | ROUTED-404 | session.status | 200,400 | 404 | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-115 | /session/{sessionID}/command | POST | OUTCOME-MISMATCH | session.command | 200,400,404 | 501 | declared 200,400,404, served 501 |
| SHIM-DRIFT-116 | /session/{sessionID}/diff | GET | ROUTED-404 | session.diff | 200,400 | 404 | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-117 | /session/{sessionID}/fork | POST | OUTCOME-MISMATCH | session.fork | 200,400,404 | 501 | declared 200,400,404, served 501 |
| SHIM-DRIFT-118 | /session/{sessionID}/init | POST | OUTCOME-MISMATCH | session.init | 200,400,404 | 501 | declared 200,400,404, served 501 |
| SHIM-DRIFT-119 | /session/{sessionID}/prompt_async | POST | OUTCOME-MISMATCH | session.prompt_async | 204,400,404 | 501 | declared 204,400,404, served 501 |
| SHIM-DRIFT-120 | /session/{sessionID}/revert | POST | OUTCOME-MISMATCH | session.revert | 200,400,404,409 | 501 | declared 200,400,404,409, served 501 |
| SHIM-DRIFT-121 | /session/{sessionID}/share | DELETE | METHOD-MISSING | session.unshare | 200,400,404,500 |  | path is routed and serves other methods, not this one |
| SHIM-DRIFT-122 | /session/{sessionID}/share | POST | OUTCOME-MISMATCH | session.share | 200,400,404,500 | 501 | declared 200,400,404,500, served 501 |
| SHIM-DRIFT-123 | /session/{sessionID}/shell | POST | OUTCOME-MISMATCH | session.shell | 200,400,404,409 | 501 | declared 200,400,404,409, served 501 |
| SHIM-DRIFT-124 | /session/{sessionID}/summarize | POST | OUTCOME-MISMATCH | session.summarize | 200,400,404 | 501 | declared 200,400,404, served 501 |
| SHIM-DRIFT-125 | /skill | GET | NOT-SERVED | app.skills | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-126 | /sync/history | POST | NOT-SERVED | sync.history.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-127 | /sync/replay | POST | NOT-SERVED | sync.replay | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-128 | /sync/start | POST | NOT-SERVED | sync.start | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-129 | /sync/steal | POST | NOT-SERVED | sync.steal | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-130 | /tui/append-prompt | POST | OUTCOME-MISMATCH | tui.appendPrompt | 200,400 | 501 | declared 200,400, served 501 |
| SHIM-DRIFT-131 | /tui/clear-prompt | POST | ROUTED-404 | tui.clearPrompt | 200,400 | 404 | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-132 | /tui/control/next | GET | ROUTED-404 | tui.control.next | 200,400 | 404 | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-133 | /tui/control/response | POST | ROUTED-404 | tui.control.response | 200,400 | 404 | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-134 | /tui/execute-command | POST | OUTCOME-MISMATCH | tui.executeCommand | 200,400 | 501 | declared 200,400, served 501 |
| SHIM-DRIFT-135 | /tui/open-help | POST | ROUTED-404 | tui.openHelp | 200,400 | 404 | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-136 | /tui/open-models | POST | ROUTED-404 | tui.openModels | 200,400 | 404 | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-137 | /tui/open-sessions | POST | ROUTED-404 | tui.openSessions | 200,400 | 404 | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-138 | /tui/open-themes | POST | ROUTED-404 | tui.openThemes | 200,400 | 404 | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-139 | /tui/publish | POST | ROUTED-404 | tui.publish | 200,400 | 404 | router subtree matches; handler answers 404 NOT_FOUND |
| SHIM-DRIFT-140 | /tui/show-toast | POST | OUTCOME-MISMATCH | tui.showToast | 200,400 | 501 | declared 200,400, served 501 |
| SHIM-DRIFT-141 | /tui/submit-prompt | POST | OUTCOME-MISMATCH | tui.submitPrompt | 200,400 | 501 | declared 200,400, served 501 |
| SHIM-DRIFT-142 | /vcs/apply | POST | STUB-501 | vcs.apply | 200,400 | 501 | routed subtree answers 501 NOT_IMPLEMENTED |
| SHIM-DRIFT-143 | /vcs/diff/raw | GET | STUB-501 | vcs.diff.raw | 200,400 | 501 | routed subtree answers 501 NOT_IMPLEMENTED |
| SHIM-DRIFT-144 | /vcs/status | GET | STUB-501 | vcs.status | 200,400 | 501 | routed subtree answers 501 NOT_IMPLEMENTED |

## Appendix B — served operations the document does not declare (17 rows)

`PATH-ABSENT` = the document has no such path at all; `METHOD-ABSENT` = the
path is declared but not this method.

| id | path | method | outcome | auth | side | evidence |
|---|---|---|---|---|---|---|
| SHIM-DRIFT-S001 | /doc | GET | 200 | public | PATH-ABSENT | server.go:186 handleDoc; serves the CONSENSUS bundle (specs/openapi/bundled.yaml), not the upstream document |
| SHIM-DRIFT-S002 | /permission/{id} | GET | 200 | api-key | PATH-ABSENT | server.go:206, 2148 |
| SHIM-DRIFT-S003 | /permission/{id}/resolve | POST | 200 | api-key | PATH-ABSENT | server.go:2150 resolvePermission (shim-only verb; upstream document declares reply) |
| SHIM-DRIFT-S004 | /project/{projectID} | GET | 404-typed | stub-skip-non-GET | METHOD-ABSENT | server.go:234 handleProjectByID |
| SHIM-DRIFT-S005 | /vcs/{id} | GET | 501 | api-key | PATH-ABSENT | server.go:237 handleProjectVCSSStub; GET keeps auth so unauthenticated is 401 (isStubPath server.go:1159) |
| SHIM-DRIFT-S006 | /instance | GET | 200 | public | PATH-ABSENT | server.go:238 handleInstance; authMiddleware server.go:381 |
| SHIM-DRIFT-S007 | /instance/path | GET | 200 | public | PATH-ABSENT | server.go:239 handleInstanceSub (server.go:1351) |
| SHIM-DRIFT-S008 | /instance/vcs | GET | 200 | public | PATH-ABSENT | server.go:1353 |
| SHIM-DRIFT-S009 | /instance/vcs/diff | GET | 200 | public | PATH-ABSENT | server.go:1355 |
| SHIM-DRIFT-S010 | /instance/vcs/status | GET | 501 | public | PATH-ABSENT | instanceKnownSubpaths server.go:1333 |
| SHIM-DRIFT-S011 | /instance/vcs/diff/raw | GET | 501 | public | PATH-ABSENT | instanceKnownSubpaths server.go:1334 |
| SHIM-DRIFT-S012 | /instance/vcs/apply | POST | 501 | public | PATH-ABSENT | instanceKnownSubpaths server.go:1335 |
| SHIM-DRIFT-S013 | /instance/command | GET | 501 | public | PATH-ABSENT | instanceKnownSubpaths server.go:1336 |
| SHIM-DRIFT-S014 | /instance/agent | GET | 501 | public | PATH-ABSENT | instanceKnownSubpaths server.go:1337 |
| SHIM-DRIFT-S015 | /instance/skill | GET | 501 | public | PATH-ABSENT | instanceKnownSubpaths server.go:1338 |
| SHIM-DRIFT-S016 | /instance/lsp | GET | 501 | public | PATH-ABSENT | instanceKnownSubpaths server.go:1339 |
| SHIM-DRIFT-S017 | /instance/formatter | GET | 501 | public | PATH-ABSENT | instanceKnownSubpaths server.go:1340 |

## Appendix C — error-contract-only (11 rows)

The shim answers a code the document declares for that operation, but never
the declared success code. Not counted as drift because the error contract
holds; listed because the success path is nonetheless absent.

| id | path | method | operationId | declared_responses | served_outcome | detail |
|---|---|---|---|---|---|---|
| SHIM-NARROWED-001 | /permission/{requestID}/reply | POST | permission.reply | 200,400,404 | 404-typed | shim answers the declared 404 but never the declared success code |
| SHIM-NARROWED-002 | /project/{projectID} | PATCH | project.update | 200,400,404 | 404-typed | shim answers the declared 404 but never the declared success code |
| SHIM-NARROWED-003 | /question/{requestID}/reject | POST | question.reject | 200,400,404 | 404-typed | shim answers the declared 404 but never the declared success code |
| SHIM-NARROWED-004 | /question/{requestID}/reply | POST | question.reply | 200,400,404 | 404-typed | shim answers the declared 404 but never the declared success code |
| SHIM-NARROWED-005 | /session/{sessionID}/message/{messageID} | DELETE | session.deleteMessage | 200,400,404,409 | 404 | shim answers the declared 404 but never the declared success code |
| SHIM-NARROWED-006 | /session/{sessionID}/message/{messageID}/part/{partID} | DELETE | part.delete | 200,400,404 | 404 | router catch-all answers the declared 404 (no success path) |
| SHIM-NARROWED-007 | /session/{sessionID}/message/{messageID}/part/{partID} | PATCH | part.update | 200,400,404 | 404 | router catch-all answers the declared 404 (no success path) |
| SHIM-NARROWED-008 | /session/{sessionID}/permissions/{permissionID} | POST | permission.respond | 200,400,404 | 404 | router catch-all answers the declared 404 (no success path) |
| SHIM-NARROWED-009 | /session/{sessionID}/todo | GET | session.todo | 200,400,404 | 404 | router catch-all answers the declared 404 (no success path) |
| SHIM-NARROWED-010 | /session/{sessionID}/unrevert | POST | session.unrevert | 200,400,404,409 | 404 | router catch-all answers the declared 404 (no success path) |
| SHIM-NARROWED-011 | /tui/select-session | POST | tui.selectSession | 200,400,404 | 404 | router catch-all answers the declared 404 (no success path) |

## Appendix D — semantic collisions (2 rows)

Same path + method in both surfaces, different operation.

| id | path | method | operationId | declared_responses | served_outcome | detail |
|---|---|---|---|---|---|---|
| SHIM-SEMANTIC-001 | /mcp | GET | mcp.status | 200,400 | 200-mcp-delegate | upstream declares mcp.status (list MCP server statuses); the shim serves the MCP streamable-HTTP protocol endpoint |
| SHIM-SEMANTIC-002 | /mcp | POST | mcp.add | 200,400 | 200-mcp-delegate | upstream declares mcp.add (add an MCP server); the shim serves the MCP JSON-RPC endpoint |

## Appendix E — declared operations served as declared (31 rows)

Listed for completeness: these are the operations where the served answer is
one the document declares for the same path+method.

| path | method | operationId | declared_responses | served_outcome |
|---|---|---|---|---|
| /agent | GET | app.agents | 200,400 | 200 |
| /auth/{providerID} | PUT | auth.set | 200,400 | 200 |
| /config | GET | config.get | 200,400 | 200 |
| /config | PATCH | config.update | 200,400 | 200 |
| /config/providers | GET | config.providers | 200,400 | 200 |
| /event | GET | event.subscribe | 200 | 200-sse |
| /experimental/tool | GET | tool.list | 200,400 | 200 |
| /experimental/tool/ids | GET | tool.ids | 200,400 | 200 |
| /file/content | GET | file.read | 200,400 | 200 |
| /file/status | GET | file.status | 200,400 | 200 |
| /find | GET | find.text | 200,400 | 200 |
| /find/file | GET | find.files | 200,400 | 200 |
| /global/event | GET | global.event | 200,400 | 200-sse |
| /global/health | GET | global.health | 200,400 | 200 |
| /log | POST | app.log | 200,400 | 200 |
| /lsp | GET | lsp.status | 200,400 | 200 |
| /path | GET | path.get | 200,400 | 200 |
| /permission | GET | permission.list | 200,400 | 200 |
| /provider | GET | provider.list | 200,400 | 200 |
| /session | GET | session.list | 200,400 | 200 |
| /session | POST | session.create | 200,400 | 200 |
| /session/{sessionID} | DELETE | session.delete | 200,400,404 | 200 |
| /session/{sessionID} | GET | session.get | 200,400,404 | 200 |
| /session/{sessionID} | PATCH | session.update | 200,400,404 | 200 |
| /session/{sessionID}/abort | POST | session.abort | 200,400 | 200 |
| /session/{sessionID}/children | GET | session.children | 200,400,404 | 200 |
| /session/{sessionID}/message | GET | session.messages | 200,400,404 | 200 |
| /session/{sessionID}/message | POST | session.prompt | 200,400,404 | 200 |
| /session/{sessionID}/message/{messageID} | GET | session.message | 200,400,404 | 200 |
| /vcs | GET | vcs.get | 200,400 | 200 |
| /vcs/diff | GET | vcs.diff | 200,400 | 200 |

---

## 9. Post-fix update — SHIM-GAP-002 (2026-09-29, commit `0246241`)

This report is the baseline at commit `f012e36`; it is *updated here, not
rewritten* — the tables above remain the pre-fix record.

Board row SHIM-GAP-002 closed the 17 dishonest rows (13 `ROUTED-404`, 1
`METHOD-MISSING`, 3 `STUB-501`) by answering each with HTTP 501 and the typed
envelope

```json
{"error":"not_implemented","operation":"<upstream operationId>","detail":"<what is missing>"}
```

served by one helper (`writeNotImplemented`, `internal/shim/opencode/server.go`;
documented in `specs/017-ui-adapter-layer.md` §3.9).

| Class | Baseline (§4) | After SHIM-GAP-002 |
|---|---:|---:|
| `ROUTED-404` | 13 | 0 |
| `METHOD-MISSING` | 1 | 0 |
| `STUB-501` | 3 | 0 |
| `OUTCOME-MISMATCH` | 16 | 33 |
| `NOT-SERVED` | 111 | 111 |
| **declared-not-served total** | 144 | 144 |

What changed, and what did not:

* The 17 rows keep their ids (`SHIM-DRIFT-099…144`) and remain declared-vs-served
  drift — upstream still declares `200` for them — but each row now carries
  `served_outcome: 501-typed` in
  `specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`: the
  survivors are named, and the classes that meant "a registered route lied" are
  empty.
* Served surface: 71 → 88 route entries — the 17 are now explicit rows in
  `specs/openapi/upstream/consensus-shim-served-surface.yaml`, whose outcome
  vocabulary gained `501-typed` and whose `meta.derived_at_commit` is now
  `0246241`.
* Unchanged: `NOT-SERVED` 111, covered operations 31, error-contract-only 11,
  semantic collisions 2, served-not-declared 17.
* Gate: `go test -short ./internal/shim/opencode/...` includes
  `TestNotImplementedTableMatchesDriftArtifact`, which fails if the artifact
  classifies any of the 17 as `ROUTED-404`/`METHOD-MISSING`/`STUB-501` or does
  not record it as `501-typed` — this comparison can no longer go stale
  silently for this row.
* Reproduce: `python3 scripts/compare-opencode-declared-served.py` (the
  regenerated result is the committed artifact).
* Upstream suite re-run at the fix
  (`docs/evidence/opencode-upstream-v1.18.29-r4/`): identical to `r3` —
  `httpapi-instance.test.ts` 7/7 PASS, `httpapi-sdk.test.ts` 17/18,
  `sdk-error-shape.test.ts` 0/2, `promise.test.ts` 7/7. The change introduced no
  new divergence; the two divergence suites are the pre-existing durable gaps
  recorded above.

---

## 10. Addendum — 2026-09-30 re-measure at pin `7945de20`

`SHIM-GAP-003`. The four pinned upstream suites were re-run with the current pin
attached. **This section is appended; sections 1–9 above are the historical body
and are not rewritten.** Section 6 residual 1 ("the suite has not been re-run at
the new pin") is closed by this section.

### 10.1 What was re-pinned

* `specs/openapi/opencode-pin.yaml` — unchanged (it already named
  `7945de208964a49300d7f770d1a71d078db9a4c4` / `1.18.33`).
* `scripts/test-opencode-upstream.sh` — `REVISION` and `VERSION` moved from
  `16747470…`/`1.18.29` to `7945de20…`/`1.18.33`; the manifest/checkout/package
  consistency checks are untouched, and the evidence directory now derives as
  `docs/evidence/opencode-upstream-v1.18.33`.
* `scripts/opencode-upstream/manifest.json` — `revision`, `version` and
  `lock_sha256` updated (`e4a33f0d…` → `b68e7ece1128eb383663e55ffca4f9f08e451530cc09fc9cf84c489bf41bdc2a`).
* The four suite files are **byte-identical** across the two pins — the
  manifest's per-suite `sha256` values did not change and the runner still
  verifies them against the fresh checkout before applying the transport-only
  adapter. Only `bun.lock` moved.
* `internal/chronicle/opencode_upstream_runner_test.go` carries the pinned
  revision/version/lock strings and was updated with them (it is the test that
  makes the pin impossible to change silently).

### 10.2 Target and invocation

* Live target: `consensus serve` on `http://127.0.0.1:18242`, scratch SQLite
  (`scratch.db`), binary built from this tree (`make build`).
* `DEEPSEEK_API_KEY` was present in the environment for the run (loaded from the
  secret store, never echoed); the server's start-up probe reported
  `llm key probe ok`. No suite was classified LLM-DIVERGENCE, so nothing in the
  table below rests on a missing credential.
* Evidence directory: `docs/evidence/opencode-upstream-v1.18.33/`
  (`summary.md`, `results.tsv`, four per-suite logs).
* Reproduce: see §10.6.

### 10.3 Suite results at pin `7945de20` (v1.18.33)

34 tests ran, **32 pass, 2 fail**.

| # | Actual upstream suite | Exit | Ran | Pass | Fail | Classification |
|---:|---|---:|---:|---:|---:|---|
| 1 | `packages/opencode/test/server/httpapi-instance.test.ts` | 0 | 7 | 7 | 0 | PASS |
| 2 | `packages/opencode/test/server/httpapi-sdk.test.ts` | 0 | 18 | 18 | 0 | PASS |
| 3 | `packages/opencode/test/server/sdk-error-shape.test.ts` | 1 | 2 | 0 | 2 | DIVERGENCE |
| 4 | `packages/client/test/promise.test.ts` | 0 | 7 | 7 | 0 | PASS |

Against the measured series in `docs/evidence/` (all at the **old** pin):

| Evidence | Ran | Pass | Fail |
|---|---:|---:|---:|
| `opencode-upstream-v1.18.29` | 34 | 30 | 4 |
| `…-r2` | 34 | 24 | 10 |
| `…-r3` | 34 | 31 | 3 |
| `…-r4` | 34 | 32 | 2 |
| `…-v1.18.33` (this run) | 34 | **32** | **2** |

Note for readers: the "34 ran / 24 pass / 10 fail" figure that circulated is the
`r2` measurement, not the latest — `r3`/`r4` had already improved it to 31/3 and
32/2. At the new pin the count is unchanged from `r4`, which is the expected
shape given the suites are byte-identical, but the *mode* of the first remaining
failure changed (below), so "unchanged" is a measured statement and not an
assumption.

### 10.4 The two remaining failing assertions, named

Both live in `packages/opencode/test/server/sdk-error-shape.test.ts`
(`docs/evidence/opencode-upstream-v1.18.33/packages_opencode_test_server_sdk-error-shape_test_ts.log`).

**F1 — `v2 SDK error shape > 404 with NamedError body throws a real Error carrying the server message`** (test line 30; assertion at line 44):

```text
error: expect(received).toContain(expected)
Expected to contain: "Session not found"
Received: "GET http://test/session/ses_no_such?directory=%2Ftmp%2Fopencode-test-684l17w2e1q → 404 Not Found"
```

Live shim, same request:

```
GET /session/ses_no_such?directory=%2Ftmp        →  HTTP 404
{"error":{"code":"NOT_FOUND","message":"session not found"}}
```

The status is now correct — at `r4` this assertion observed `401 Unauthorized`,
so the route/auth half has since been fixed. What remains is the **body shape**:
upstream's SDK requires the NamedError envelope
`{name:"NotFoundError", data:{message:"…Session not found…"}}` (test lines 46–49)
for `wrapClientError` to lift `data.message` into `Error.message`. The shim
answers with its own `{"error":{"code":…,"message":…}}` envelope, so the SDK falls
back to the generic fetch text and both the `.message` assertion (line 44) and the
`cause.body` assertion (lines 46–49) fail.

**F2 — `v2 SDK error shape > 400 schema rejection: SDK extracts the field-level reason from the NamedError body`** (test line 52; assertion at line 71):

```text
error: expect(received).toBe(expected)
Expected: 400
Received: 404
```

Live shim, same request:

```
POST /sync/history/list  {"aggregate":-1}  →  HTTP 404
404 page not found
```

The route does not exist, so the request never reaches validation. This is the
already-classified drift item `SHIM-DRIFT-126` (`POST /sync/history`,
`sync.history.list`, class `NOT-SERVED`, "no shim route at all; net/http default
404", triage `defer`) in
`specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`.

### 10.5 Board rows

* **`SHIM-SUITE33-001`** (new, P2, pending) — F1, the NamedError response-body
  envelope for `session.get`'s 404. Not previously tracked: the declared-vs-served
  artifact classifies `session.get` as *covered* (it compares path/method/status,
  `served_outcome: 200`, not error-body shape), so F1 is invisible to that
  comparison and needed its own row.
* **`DF-CONSENSUS-40`** (existing, P2, pending) — F2, 400-class validation
  answered as 404. Updated in place with this pin's measurement rather than
  duplicated; the same defect also sits in `SHIM-GAP-004` (error-contract
  narrowing) and `SHIM-GAP-007` (must-serve implementation, `/sync/*` family).

### 10.6 Reproduce

```bash
make build
# scratch instance on a free port with its own SQLite file
cd /tmp/scratch && "$OLDPWD/bin/consensus" init --db-url "sqlite://scratch.db?_journal_mode=WAL"
"$OLDPWD/bin/consensus" serve --port 18242 --db-url "sqlite://scratch.db?_journal_mode=WAL" &
# the runner's fetch preload authenticates with Basic opencode:<admin key>
CONSENSUS_OPENCODE_PASSWORD=<admin key printed by init> \
  scripts/test-opencode-upstream.sh --base-url http://127.0.0.1:18242
```

The runner fetches the pinned checkout into `$XDG_CACHE_HOME/consensus/opencode-7945de20…`,
verifies `bun.lock` and all four suite hashes, applies the transport-only adapter,
runs the four suites and writes `docs/evidence/opencode-upstream-v1.18.33/`.
The secondary Go-port grade (`scripts/opencode-compat.sh`, 44 keyless checks)
still exits 0 at this pin.

---

## Addendum — SHIM-GAP-005 (2026-10-07): the 17 served-not-declared rows declared as shim extensions

The `SHIM-SURFACE-INSTANCE-001` question this report posed — "our own `/doc`
documents the shim's extensions, or the divergence is recorded as intentional" —
was answered "recorded as intentional" on the served-surface side:

* All 17 Appendix B rows (`SHIM-DRIFT-S001…S017`) now carry
  `x-shim-extension: true` plus a per-row `extension-rationale` in
  `specs/openapi/upstream/consensus-shim-served-surface.yaml`, truthfully
  describing each served behavior from its handler evidence: the two HITL reads
  (`/permission/{id}` GET, `/permission/{id}/resolve` POST), the method-absent
  sibling `/project/{projectID}` GET (same typed `ProjectNotFoundError` 404 as
  the declared PATCH arm), the `/vcs/{id}` stub keep-alive, `/doc` (serves the
  Consensus bundle, JSON default / YAML on `Accept: application/yaml`), the four
  real `/instance*` translations (200 public), and the eight known-but-untranslated
  501 stubs under `/instance/*`.
* `scripts/compare-opencode-declared-served.py` classifies rationale-carrying
  extension rows as accounted divergence — a new `shim_extensions` block +
  `appendix-shim-extensions.md` + `counts.shim_extension_operations` (17) —
  and aborts if an `x-shim-extension` row lacks its rationale. Fresh run at
  commit 89394b1: `drift_served_not_declared: 0`, `covered_operations: 68`
  (the pre-SHIM-GAP-005 artifact's 72 was inflated: four outcomes had rotted to
  `200` in the served YAML during the 2026-10-06 ROUTE-FIX-020 window while
  their handlers answer typed 501 — `/pty` POST, `/vcs/apply` POST,
  `/vcs/diff/raw` GET, `/vcs/status` GET; SHIM-GAP-005 restored all four, plus
  the `/find/` prefix-rule `stub_outcome`).
* The pinned upstream surface (`openapi-1.18.33.surface.json`) is untouched:
  declaring on our own contribution is the honest alternative to editing a
  pinned third-party document. Appendix B above remains the historical baseline
  (17 rows, 2026-09-29); the live state is the regenerated
  `specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json`.

