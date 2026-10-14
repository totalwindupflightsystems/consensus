#!/usr/bin/env python3
"""opencode protocol: declared-vs-served comparison (specs/024 §A.3).

declared = the upstream published document surface
           (specs/openapi/upstream/openapi-<version>.surface.json)
served   = the Consensus shim's served surface
           (specs/openapi/upstream/consensus-shim-served-surface.yaml)

Emits a machine-readable comparison plus markdown tables (one row per drift):

  <out>/declared-vs-served.json      full result, ids on every drift row
  <out>/appendix-drift-declared.md   declared operations the shim does not serve
  <out>/appendix-drift-served.md     served operations the document does not declare
  <out>/appendix-narrowed.md         error-contract-only (success code unreachable)
  <out>/appendix-intentional-narrowing.md
                                     explicitly unsupported operations (typed 501)
  <out>/appendix-semantic.md         same path+method, different operation
  <out>/appendix-covered.md          declared operations served as declared
  <out>/family-summary.md            drift counts per path family

Usage:
  python3 scripts/compare-opencode-declared-served.py [declared.surface.json] \
      [served-surface.yaml] [out-dir]

Drift classes:
  NOT-SERVED       no shim route at all -> net/http default 404
  ROUTED-404       router subtree matches, handler answers 404 NOT_FOUND
  STUB-501         routed subtree answers 501 NOT_IMPLEMENTED
  METHOD-MISSING   handler answers 405 METHOD_NOT_ALLOWED
  OUTCOME-MISMATCH served code is not one the document declares for that operation

Served-but-not-declared routes (the reverse direction) that carry
x-shim-extension: true + extension-rationale in the served-surface file are
NOT drift: they are the shim's own declared extension surface (SHIM-GAP-005)
and land in shim_extensions / appendix-shim-extensions.md instead. An
x-shim-extension row without a rationale aborts the comparison — an
unexplained extension must not silently pass as accounted divergence.

A declared operation may be classified as an intentional narrowing only when
the served-surface row carries x-shim-intentional-narrowing: true, a non-empty
narrowing-rationale, and outcome 501-typed. This records an explicit exclusion
without pretending that the shim serves the upstream success contract or
leaving the routed operation in undeclared drift.

ERROR-CONTRACT NARROWING (SHIM-GAP-004). A second, separate narrowing class:
an operation that answers a DECLARED error code but can never answer the
declared success code (its served row's outcome is a declared non-2xx code
while the declared set contains a 2xx). This is detected, not hand-listed: the
comparison derives the class from the declared response set and the served
outcome, so a newly-narrowed route cannot hide inside `covered` just because
the status it answers happens to be IN the declared set. The detection is
fail-closed: such a row MUST carry x-shim-error-contract-narrowing: true plus a
non-empty narrowing-rationale, otherwise the comparison aborts naming the row.
`counts.narrowed_unexplained` is the number of narrowed rows without a
rationale and is 0 by construction — an unexplained narrowing cannot be
produced by this script.
"""
import json
import re
import sys
from collections import Counter, defaultdict
from pathlib import Path

import yaml

REPO = Path(__file__).resolve().parent.parent
DEFAULT_SURFACE = REPO / "specs/openapi/upstream/openapi-1.18.33.surface.json"
DEFAULT_SERVED = REPO / "specs/openapi/upstream/consensus-shim-served-surface.yaml"

# Error-contract-only operations are DETECTED from the declared response set and
# the served outcome (a declared non-2xx code answered while the declared set
# contains a 2xx), never hand-listed — a hand list is exactly what let a
# narrowing hide inside `covered`. Each detected row must carry
# x-shim-error-contract-narrowing: true plus a non-empty narrowing-rationale in
# the served-surface file, or the comparison aborts (see ERROR-CONTRACT
# NARROWING in the module docstring). SHIM-GAP-004 widened permission.reply
# (POST /permission/{requestID}/reply) and tui.selectSession
# (POST /tui/select-session) to their full declared contracts, and recorded the
# remaining 14 rows as explicit narrowings with reasons.
NARROWING_FLAG = "x-shim-error-contract-narrowing"
# Same path+method in both surfaces, different operation (checked against the
# upstream document's own summary/description for those operations).
SEMANTIC_DIFFERENT = {
    ("/mcp", "GET"): "upstream declares mcp.status (list MCP server statuses); "
                     "the shim serves the MCP streamable-HTTP protocol endpoint",
    ("/mcp", "POST"): "upstream declares mcp.add (add an MCP server); "
                      "the shim serves the MCP JSON-RPC endpoint",
}


def norm(path: str) -> str:
    """Normalize path placeholders so {sessionID} and {id} compare equal."""
    return re.sub(r"\{[^}]*\}", "{}", path).replace("*", "{}").rstrip("/")


def served_code(outcome: str) -> str:
    o = str(outcome)
    return "200" if o.startswith("200") else o.split("-")[0]


def family(path: str) -> str:
    segs = [s for s in path.split("/") if s]
    if not segs:
        return "/"
    if segs[0] == "api":
        return "/api/*  (v2 surface)"
    if segs[0] == "experimental":
        return "/experimental/*"
    return "/" + segs[0] + ("/*" if len(segs) > 1 else "")


def is_error_contract_narrowing(outcome, declared_codes) -> bool:
    """A declared operation whose served row answers a DECLARED non-2xx code
    while the declared set contains a 2xx: the declared success is unreachable.

    Detected, never hand-listed (SHIM-GAP-004): the served-surface row's outcome
    is the code the handler answers, so a row that can only answer an error can
    no longer be scored `covered` just because that error is IN the declared
    set. A row whose outcome is a 2xx (even a summary like "200" for an
    operation with several arms) is never flagged.
    """
    sc = served_code(outcome)
    return (sc in declared_codes
            and not sc.startswith("2")
            and any(c.startswith("2") for c in declared_codes))


def md_table(rows, cols):
    out = ["| " + " | ".join(cols) + " |", "|" + "|".join(["---"] * len(cols)) + "|"]
    for row in rows:
        out.append("| " + " | ".join(str(row.get(c, "")) for c in cols) + " |")
    return "\n".join(out) + "\n"


def main():
    declared_path = Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_SURFACE
    served_path = Path(sys.argv[2]) if len(sys.argv) > 2 else DEFAULT_SERVED
    out = Path(sys.argv[3]) if len(sys.argv) > 3 else Path("/tmp/opencode-declared-vs-served")
    out.mkdir(parents=True, exist_ok=True)

    surface = json.loads(declared_path.read_text())
    served_doc = yaml.safe_load(served_path.read_text())

    prefix_rules = sorted(served_doc.get("prefix_rules") or [],
                          key=lambda r: len(r["prefix"]), reverse=True)

    def longest_prefix_rule(path):
        for rule in prefix_rules:
            if path.startswith(rule["prefix"]):
                return rule
        return None

    served_by_key = {(norm(r["path"]), r["method"].upper()): r for r in served_doc["routes"]}
    served_paths = {norm(r["path"]) for r in served_doc["routes"] if
                    str(r.get("other_methods_outcome", "")) != "default-404"}
    exact_method_paths = {norm(r["path"]) for r in served_doc["routes"]
                          if str(r.get("other_methods_outcome", "")) == "default-404"}
    declared = surface["paths"]
    declared_norm_paths = {norm(p) for p in declared}
    declared_keys = {(norm(p), m) for p, ms in declared.items() for m in ms}

    covered, drift, narrowed, intentional_narrowing, semantic = [], [], [], [], []
    extensions = []  # SHIM-GAP-005: served routes declared as shim extensions
    for path, methods in sorted(declared.items()):
        np = norm(path)
        for method, spec in sorted(methods.items()):
            declared_codes = spec.get("responses") or []
            row = {
                "path": path, "method": method,
                "operationId": spec.get("operationId"),
                "tags": ",".join(spec.get("tags") or []),
                "declared_responses": ",".join(declared_codes),
                "family": family(path),
            }
            s = served_by_key.get((np, method))
            if s is not None:
                row.update({"served_outcome": str(s["outcome"]), "evidence": s["evidence"]})
                if s.get("x-shim-intentional-narrowing") is True:
                    rationale = str(s.get("narrowing-rationale") or "").strip()
                    if not rationale:
                        raise SystemExit(
                            "served-surface route %s %s sets "
                            "x-shim-intentional-narrowing: true without a "
                            "narrowing-rationale — refusing to hide an "
                            "unexplained contract narrowing"
                            % (method, path))
                    if str(s["outcome"]) != "501-typed":
                        raise SystemExit(
                            "served-surface route %s %s marks an intentional "
                            "narrowing with outcome %s; only the typed 501 "
                            "not_implemented envelope is accepted"
                            % (method, path, s["outcome"]))
                    row["rationale"] = rationale
                    row["detail"] = (
                        "explicitly unsupported by the Consensus shim; "
                        "answers typed 501 instead of fabricating the declared success"
                    )
                    intentional_narrowing.append(row)
                    continue
                if (path, method) in SEMANTIC_DIFFERENT:
                    row["class"] = "SEMANTIC-DIFFERENT"
                    row["detail"] = SEMANTIC_DIFFERENT[(path, method)]
                    semantic.append(row)
                    continue
                flagged = is_error_contract_narrowing(s["outcome"], declared_codes)
                if s.get(NARROWING_FLAG) is True and not flagged:
                    raise SystemExit(
                        "served-surface route %s %s sets %s: true but the "
                        "runtime answers the declared success code (%s) — the "
                        "marker would understate the served contract; remove it"
                        % (method, path, NARROWING_FLAG, s["outcome"]))
                if flagged:
                    # Fail closed: an error-contract narrowing must record WHY.
                    # This is what makes counts.narrowed_unexplained 0 by
                    # construction instead of by hand-maintenance.
                    rationale = str(s.get("narrowing-rationale") or "").strip()
                    if s.get(NARROWING_FLAG) is not True or not rationale:
                        raise SystemExit(
                            "served-surface route %s %s answers declared %s "
                            "but can never answer the declared success code "
                            "(%s) without recording the narrowing: add "
                            "%s: true plus a non-empty narrowing-rationale to "
                            "its served-surface row, or serve the declared "
                            "contract"
                            % (method, path, served_code(s["outcome"]),
                               ",".join(c for c in declared_codes if c.startswith("2")),
                               NARROWING_FLAG))
                    row["rationale"] = rationale
                    row["detail"] = (
                        "shim answers the declared %s but never the declared "
                        "success code %s"
                        % (served_code(s["outcome"]),
                           "/".join(c for c in declared_codes if c.startswith("2"))))
                    narrowed.append(row)
                elif served_code(s["outcome"]) in declared_codes:
                    covered.append(row)
                else:
                    row["class"] = "OUTCOME-MISMATCH"
                    row["detail"] = "declared %s, served %s" % (
                        row["declared_responses"], served_code(s["outcome"]))
                    drift.append(row)
                continue
            if np in exact_method_paths:
                # Exact-pattern route registered for a single method; the
                # unregistered methods on this path fall through to the
                # net/http default 404 (route entry: other_methods_outcome).
                klass, detail = "NOT-SERVED", "exact-pattern route serves a single method; other methods answer the net/http default 404"
            elif np in served_paths:
                klass = "METHOD-MISSING"
                detail = "path is routed and serves other methods, not this one"
            else:
                rule = longest_prefix_rule(path)
                if rule is None:
                    klass = "NOT-SERVED"
                    detail = "no shim route at all; net/http default 404"
                else:
                    second = path[len(rule["prefix"]):].split("/")[0]
                    outcome = (rule.get("stub_outcome")
                               if second in (rule.get("stub_second_segments") or [])
                               else rule["outcome"])
                    code = served_code(outcome)
                    row["served_outcome"] = str(outcome)
                    row["evidence"] = rule["evidence"]
                    if code in declared_codes:
                        if any(c.startswith("2") for c in declared_codes):
                            # SHIM-GAP-004 fail-closed: a router catch-all that
                            # answers a declared error code while the operation
                            # declares a 2xx leaves the declared success
                            # unreachable, and a prefix rule cannot record a
                            # rationale — so it must not be classified silently.
                            raise SystemExit(
                                "router catch-all answers the declared %s for "
                                "%s %s but the operation also declares a "
                                "success code (%s) and no served-surface route "
                                "row records the narrowing; register a served "
                                "route (with %s: true plus a "
                                "narrowing-rationale if the declared contract "
                                "is unreachable) or serve the declared contract"
                                % (code, method, path,
                                   ",".join(c for c in declared_codes if c.startswith("2")),
                                   NARROWING_FLAG))
                        row["detail"] = ("router catch-all answers the declared %s "
                                         "(the operation declares no success code)" % code)
                        narrowed.append(row)
                        continue
                    if code == "501":
                        klass, detail = "STUB-501", "routed subtree answers 501 NOT_IMPLEMENTED"
                    elif code == "404":
                        klass, detail = "ROUTED-404", "router subtree matches; handler answers 404 NOT_FOUND"
                    elif code == "405":
                        klass, detail = "METHOD-MISSING", "handler answers 405 METHOD_NOT_ALLOWED"
                    else:
                        klass, detail = "OUTCOME-MISMATCH", "served %s" % code
            row.update({"class": klass, "detail": detail})
            drift.append(row)

    served_only = []
    for r in served_doc["routes"]:
        key = (norm(r["path"]), r["method"].upper())
        if key in declared_keys:
            continue
        row = {
            "path": r["path"], "method": r["method"].upper(),
            "outcome": str(r["outcome"]), "auth": r["auth"],
            "side": "METHOD-ABSENT" if key[0] in declared_norm_paths else "PATH-ABSENT",
            "evidence": r["evidence"], "family": family(r["path"]),
        }
        # SHIM-GAP-005: a route the shim's spec contribution declares as an
        # intentional extension (x-shim-extension: true + extension-rationale
        # naming why the surface exists outside upstream v1.18.33) is not
        # undeclared drift — it is documented divergence. It moves to its own
        # appendix and count so drift_served_not_declared keeps meaning "the
        # shim serves something neither the upstream document nor its own
        # spec contribution accounts for".
        if r.get("x-shim-extension") is True:
            row["rationale"] = str(r.get("extension-rationale") or "").strip()
            if not row["rationale"]:
                raise SystemExit(
                    "served-surface route %s %s sets x-shim-extension: true "
                    "without an extension-rationale — refusing to classify an "
                    "unexplained extension as declared divergence"
                    % (row["method"], row["path"]))
            # SHIM-GAP-005 recorded a per-entry decision for every extension
            # (declared in the shipped spec bundle, or justified-intentional).
            # It is carried on the served-surface row so a regeneration
            # reproduces the artifact instead of dropping the decision.
            decision = str(r.get("extension-decision") or "").strip()
            if not decision:
                raise SystemExit(
                    "served-surface route %s %s sets x-shim-extension: true "
                    "without an extension-decision — record whether the route "
                    "is declared in the shipped spec bundle or justified as an "
                    "intentional extension" % (row["method"], row["path"]))
            row["decision"] = decision
            row["decision_reason"] = str(r.get("extension-decision-reason") or "").strip()
            extensions.append(row)
        else:
            served_only.append(row)

    for i, r in enumerate(drift, 1):
        r["id"] = "SHIM-DRIFT-%03d" % i
    for i, r in enumerate(served_only, 1):
        r["id"] = "SHIM-DRIFT-S%03d" % i
    for i, r in enumerate(extensions, 1):
        r["id"] = "SHIM-EXT-%03d" % i
    # Stable ids for the error-contract narrowings. SHIM-NARROWED-001..011 are
    # the positional ids of the 2026-09-29 baseline report
    # (docs/reports/shim-source-002-baseline-diff-2026-09-29.md); the ones still
    # in the class keep their baseline id, and the rows SHIM-GAP-004 surfaced
    # (the /pty/{ptyID} family, the three mcp.auth.* operations and the two
    # /session/{id}/share operations, all of which the comparison previously
    # scored `covered`) take SHIM-NARROWED-012..021 in this fixed order.
    # Baseline ids that LEFT the class: 001 permission.reply and 011
    # tui.selectSession (widened to their full declared contracts by
    # SHIM-GAP-004); 002 project.update, 005 session.deleteMessage, 008
    # permission.respond, 009 session.todo, 010 session.unrevert (widened
    # earlier by the ROUTE-FIX wave).
    stable_narrowed_ids = {
        ("/api/credential/{credentialID}", "PATCH"): "SHIM-NARROWED-022",
        ("/question/{requestID}/reject", "POST"): "SHIM-NARROWED-003",
        ("/question/{requestID}/reply", "POST"): "SHIM-NARROWED-004",
        ("/session/{sessionID}/message/{messageID}/part/{partID}", "DELETE"): "SHIM-NARROWED-006",
        ("/session/{sessionID}/message/{messageID}/part/{partID}", "PATCH"): "SHIM-NARROWED-007",
        ("/pty/{ptyID}", "GET"): "SHIM-NARROWED-012",
        ("/pty/{ptyID}", "PUT"): "SHIM-NARROWED-013",
        ("/pty/{ptyID}", "DELETE"): "SHIM-NARROWED-014",
        ("/pty/{ptyID}/connect", "GET"): "SHIM-NARROWED-015",
        ("/pty/{ptyID}/connect-token", "POST"): "SHIM-NARROWED-016",
        ("/mcp/{name}/auth", "POST"): "SHIM-NARROWED-017",
        ("/mcp/{name}/auth/authenticate", "POST"): "SHIM-NARROWED-018",
        ("/mcp/{name}/auth/callback", "POST"): "SHIM-NARROWED-019",
        ("/session/{sessionID}/share", "POST"): "SHIM-NARROWED-020",
        ("/session/{sessionID}/share", "DELETE"): "SHIM-NARROWED-021",
    }
    for i, r in enumerate(narrowed):
        rid = stable_narrowed_ids.get((r.get("path"), r.get("method")))
        if rid is None:
            rid = "SHIM-NARROWED-%03d" % (100 + i)
            print("note: %s %s has no stable narrowing id; using %s — add it to "
                  "stable_narrowed_ids in this script"
                  % (r.get("method"), r.get("path"), rid), file=sys.stderr)
        r["id"] = rid
    for i, r in enumerate(intentional_narrowing, 1):
        r["id"] = "SHIM-NARROWED-U%03d" % i
    for i, r in enumerate(semantic, 1):
        r["id"] = "SHIM-SEMANTIC-%03d" % i

    # SHIM-GAP-004: a narrowed row without a recorded rationale is an UNEXPLAINED
    # narrowing. The classification above already aborts on one; this count is
    # computed from the rows themselves so the artifact states the number the
    # acceptance criterion asks for instead of implying it.
    narrowed_unexplained = sum(1 for r in narrowed if not str(r.get("rationale") or "").strip())
    if narrowed_unexplained:
        raise SystemExit("%d narrowed row(s) carry no rationale — refusing to "
                         "emit an artifact with unexplained narrowings"
                         % narrowed_unexplained)

    counts = {
        "declared_paths": len(declared),
        "declared_operations": sum(len(m) for m in declared.values()),
        "served_route_entries": len(served_doc["routes"]),
        "covered_operations": len(covered),
        "narrowed_error_contract_only": len(narrowed),
        # 0 = every narrowing above records WHY (SHIM-GAP-004 acceptance).
        "narrowed_unexplained": narrowed_unexplained,
        "intentional_narrowing": len(intentional_narrowing),
        "semantic_different": len(semantic),
        "drift_declared_not_served": len(drift),
        "drift_by_class": dict(Counter(d["class"] for d in drift)),
        "drift_served_not_declared": len(served_only),
        "served_drift_by_class": dict(Counter(d["side"] for d in served_only)),
        # SHIM-GAP-005: served routes outside the upstream document that the
        # shim's spec contribution declares as intentional extensions, each
        # with an extension-rationale. Not drift — accounted divergence.
        "shim_extension_operations": len(extensions),
    }
    (out / "declared-vs-served.json").write_text(json.dumps({
        "declared_source": surface.get("source"),
        "declared_document": surface.get("document"),
        "served_source": served_doc.get("meta"),
        "counts": counts,
        "covered": covered,
        "narrowed_error_contract_only": narrowed,
        "intentional_narrowing": intentional_narrowing,
        "semantic_different": semantic,
        "drift_declared_not_served": drift,
        "drift_served_not_declared": served_only,
        "shim_extensions": extensions,
    }, indent=2) + "\n")

    (out / "appendix-drift-declared.md").write_text(md_table(drift, [
        "id", "path", "method", "class", "operationId", "declared_responses",
        "served_outcome", "detail"]))
    (out / "appendix-drift-served.md").write_text(md_table(served_only, [
        "id", "path", "method", "outcome", "auth", "side", "evidence"]))
    # SHIM-GAP-005: the extension rows, one per accounted served-only operation.
    (out / "appendix-shim-extensions.md").write_text(md_table(extensions, [
        "id", "path", "method", "outcome", "auth", "side", "decision",
        "decision_reason", "rationale"]))
    (out / "appendix-covered.md").write_text(md_table(covered, [
        "path", "method", "operationId", "declared_responses", "served_outcome"]))
    # SHIM-GAP-004: one row per error-contract narrowing, each with the reason it
    # is narrowed (counts.narrowed_unexplained is 0 by construction).
    (out / "appendix-narrowed.md").write_text(md_table(narrowed, [
        "id", "path", "method", "operationId", "declared_responses",
        "served_outcome", "detail", "rationale"]))
    (out / "appendix-intentional-narrowing.md").write_text(md_table(
        intentional_narrowing, [
            "id", "path", "method", "operationId", "declared_responses",
            "served_outcome", "detail", "rationale"]))
    (out / "appendix-semantic.md").write_text(md_table(semantic, [
        "id", "path", "method", "operationId", "declared_responses",
        "served_outcome", "detail"]))

    fam = defaultdict(Counter)
    for d in drift:
        fam[d["family"]][d["class"]] += 1
    rows = [{"family": f, "total": sum(fam[f].values()),
             "NOT-SERVED": fam[f].get("NOT-SERVED", 0),
             "ROUTED-404": fam[f].get("ROUTED-404", 0),
             "STUB-501": fam[f].get("STUB-501", 0),
             "OUTCOME-MISMATCH": fam[f].get("OUTCOME-MISMATCH", 0),
             "METHOD-MISSING": fam[f].get("METHOD-MISSING", 0)}
            for f in sorted(fam)]
    (out / "family-summary.md").write_text(md_table(rows, [
        "family", "total", "NOT-SERVED", "ROUTED-404", "STUB-501",
        "OUTCOME-MISMATCH", "METHOD-MISSING"]))

    print(json.dumps(counts, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
