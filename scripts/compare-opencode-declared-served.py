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

# Operations where the shim answers the declared ERROR code but never the
# declared SUCCESS code (Consensus has no project/question/permission registry).
#
# ROUTE-FIX-035 REMOVED ("/session/{sessionID}/message/{messageID}", "DELETE")
# from this set when session.deleteMessage started serving its declared 200.
# ROUTE-FIX-037 registers the sibling PATCH route on the message-part path
# (part.update, served/covered) and therefore has to record the path's DELETE
# explicitly: without an entry the served-path set would absorb the path and
# part.delete would be reclassified METHOD-MISSING ("path is routed and serves
# other methods") even though DELETE on the sub-path is answered — the
# message-DELETE case matches the longer "message/..." prefix, hands
# sessionDeleteMessage a garbled "msg-<id>/part/<id>" id that never resolves,
# and answers the declared 404 NOT_FOUND. The declared 200 boolean
# ("Successfully deleted part") is still unreachable, so the row stays
# error-contract-only here until ROUTE-FIX-036 gives part.delete its own
# handler.
ERROR_CONTRACT_ONLY = {
    ("/project/{projectID}", "PATCH"),
    ("/permission/{requestID}/reply", "POST"),
    ("/question/{requestID}/reply", "POST"),
    ("/question/{requestID}/reject", "POST"),
    ("/session/{sessionID}/message/{messageID}/part/{partID}", "DELETE"),
}
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

    covered, drift, narrowed, semantic = [], [], [], []
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
                if (path, method) in SEMANTIC_DIFFERENT:
                    row["class"] = "SEMANTIC-DIFFERENT"
                    row["detail"] = SEMANTIC_DIFFERENT[(path, method)]
                    semantic.append(row)
                    continue
                if served_code(s["outcome"]) in declared_codes:
                    if (path, method) in ERROR_CONTRACT_ONLY:
                        row["detail"] = ("shim answers the declared %s but never the "
                                         "declared success code" % served_code(s["outcome"]))
                        narrowed.append(row)
                    else:
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
                        row["detail"] = ("router catch-all answers the declared %s "
                                         "(no success path)" % code)
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
            extensions.append(row)
        else:
            served_only.append(row)

    for i, r in enumerate(drift, 1):
        r["id"] = "SHIM-DRIFT-%03d" % i
    for i, r in enumerate(served_only, 1):
        r["id"] = "SHIM-DRIFT-S%03d" % i
    for i, r in enumerate(extensions, 1):
        r["id"] = "SHIM-EXT-%03d" % i
    for i, r in enumerate(narrowed, 1):
        r["id"] = "SHIM-NARROWED-%03d" % i
    for i, r in enumerate(semantic, 1):
        r["id"] = "SHIM-SEMANTIC-%03d" % i

    counts = {
        "declared_paths": len(declared),
        "declared_operations": sum(len(m) for m in declared.values()),
        "served_route_entries": len(served_doc["routes"]),
        "covered_operations": len(covered),
        "narrowed_error_contract_only": len(narrowed),
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
        "id", "path", "method", "outcome", "auth", "side", "rationale"]))
    (out / "appendix-covered.md").write_text(md_table(covered, [
        "path", "method", "operationId", "declared_responses", "served_outcome"]))
    (out / "appendix-narrowed.md").write_text(md_table(narrowed, [
        "id", "path", "method", "operationId", "declared_responses",
        "served_outcome", "detail"]))
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
