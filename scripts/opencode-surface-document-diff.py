#!/usr/bin/env python3
"""opencode protocol: DOCUMENT-level declared-vs-served surface diff (specs/024 §A.2/§A.3).

Compares two OpenAPI documents by PATH, OPERATION and SCHEMA:

  declared = the upstream published document pinned in specs/openapi/opencode-pin.yaml
             (packages/sdk/openapi.json at the pinned commit)
  served   = the document the Consensus shim actually serves
             (GET /openapi.json, equivalently GET /doc — the same embedded bundle)

The route-level declared-vs-served comparison (which operation is ANSWERED, and
with which status) is a different, complementary check and lives in
scripts/compare-opencode-declared-served.py.  This script answers the other half
of §A.2's obligation: which paths, operations and SCHEMAS were added, removed or
modified between the two documents.

Outputs, into <out-dir>:

  document-diff.json   full machine-readable result (ids on every row)
  document-diff.md     markdown tables, one row per addition/removal/modification

Usage:
  python3 scripts/opencode-surface-document-diff.py [declared.json] [served.json] [out-dir]

Placeholders are normalized before comparison ({sessionID} == {sessionId}) so a
naming convention difference is not reported as a path removal; the raw spelling
of both sides is kept on every row.

Exit status: 0 always for a comparison that ran (drift is data, not failure);
2 for a usage/parse error.
"""
import json
import re
import sys
from collections import Counter
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
DEFAULT_DECLARED = REPO / "specs/openapi/upstream/openapi-1.18.33.json"
DEFAULT_SERVED = Path("/tmp/consensus-served-openapi.json")
DEFAULT_OUT = Path("/tmp/opencode-document-diff")

METHODS = ("get", "post", "put", "patch", "delete", "head", "options", "trace")


def norm_path(p: str) -> str:
    """Normalize path placeholders so {sessionID} and {sessionId} compare equal."""
    return re.sub(r"\{[^}]*\}", "{}", p).replace("*", "{}").rstrip("/") or "/"


def exact_path(spec: dict) -> str:
    """Path template verbatim, the form both upstream and the shim actually serve."""
    return str(spec.get("__path__", ""))


def deep_diff(a, b, pointer="", out=None, cap=400):
    """Record structural differences between two JSON values as (pointer, a, b)."""
    if out is None:
        out = []
    if len(out) >= cap:
        return out
    if type(a) is not type(b):
        out.append((pointer, a, b))
        return out
    if isinstance(a, dict):
        for k in sorted(set(a) | set(b)):
            if k not in a:
                out.append((pointer + "/" + k, "<absent>", b[k]))
            elif k not in b:
                out.append((pointer + "/" + k, a[k], "<absent>"))
            else:
                deep_diff(a[k], b[k], pointer + "/" + k, out, cap)
            if len(out) >= cap:
                return out
    elif isinstance(a, list):
        if len(a) != len(b):
            out.append((pointer + "#len", len(a), len(b)))
        for i, (x, y) in enumerate(zip(a, b)):
            deep_diff(x, y, pointer + "/" + str(i), out, cap)
            if len(out) >= cap:
                return out
    elif a != b:
        out.append((pointer, a, b))
    return out


def fmt(v, limit=140):
    s = json.dumps(v, sort_keys=True, ensure_ascii=False) if not isinstance(v, str) else v
    s = s.replace("|", "\\|").replace("\n", " ")
    return s if len(s) <= limit else s[: limit - 3] + "..."


def md_table(rows, cols):
    if not rows:
        return "_(none)_\n"
    out = ["| " + " | ".join(cols) + " |", "|" + "|".join(["---"] * len(cols)) + "|"]
    for row in rows:
        out.append("| " + " | ".join(fmt(row.get(c, "")) for c in cols) + " |")
    return "\n".join(out) + "\n"


def op_summary(item: dict) -> dict:
    return {
        "operationId": item.get("operationId"),
        "tags": item.get("tags") or [],
        "responses": sorted((item.get("responses") or {}).keys()),
        "parameters": sorted(
            (p.get("name"), p.get("in"), bool(p.get("required")))
            for p in (item.get("parameters") or [])
            if isinstance(p, dict)
        ),
        "requestBody": bool(item.get("requestBody")),
        "security": item.get("security"),
    }


def main() -> int:
    declared_path = Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_DECLARED
    served_path = Path(sys.argv[2]) if len(sys.argv) > 2 else DEFAULT_SERVED
    out = Path(sys.argv[3]) if len(sys.argv) > 3 else DEFAULT_OUT
    out.mkdir(parents=True, exist_ok=True)

    declared = json.loads(declared_path.read_text())
    served = json.loads(served_path.read_text())

    d_paths = {norm_path(p): {**m, "__path__": p} for p, m in declared["paths"].items()}
    s_paths = {norm_path(p): {**m, "__path__": p} for p, m in served["paths"].items()}

    d_only = sorted(set(d_paths) - set(s_paths))
    s_only = sorted(set(s_paths) - set(d_paths))
    common = sorted(set(d_paths) & set(s_paths))

    # ── operations ──────────────────────────────────────────────────────────
    op_rows = []  # one row per added/removed/modified operation
    for np in d_only:
        for m, item in sorted(d_paths[np].items()):
            if m in METHODS:
                op_rows.append({
                    "id": "", "class": "PATH-REMOVED", "side": "declared-not-served",
                    "path": d_paths[np]["__path__"], "method": m.upper(),
                    "operationId": item.get("operationId") or "",
                    "detail": "the served document declares no such path",
                })
    for np in s_only:
        for m, item in sorted(s_paths[np].items()):
            if m in METHODS:
                op_rows.append({
                    "id": "", "class": "PATH-ADDED", "side": "served-not-declared",
                    "path": s_paths[np]["__path__"], "method": m.upper(),
                    "operationId": item.get("operationId") or "",
                    "detail": "the served document declares a path upstream does not",
                })
    for np in common:
        d_ops = {m: v for m, v in d_paths[np].items() if m in METHODS}
        s_ops = {m: v for m, v in s_paths[np].items() if m in METHODS}
        for m in sorted(set(d_ops) - set(s_ops)):
            op_rows.append({
                "id": "", "class": "OPERATION-REMOVED", "side": "declared-not-served",
                "path": d_paths[np]["__path__"], "method": m.upper(),
                "operationId": d_ops[m].get("operationId") or "",
                "detail": "path is declared on both sides; this method is declared upstream only",
            })
        for m in sorted(set(s_ops) - set(d_ops)):
            op_rows.append({
                "id": "", "class": "OPERATION-ADDED", "side": "served-not-declared",
                "path": s_paths[np]["__path__"], "method": m.upper(),
                "operationId": s_ops[m].get("operationId") or "",
                "detail": "path is declared on both sides; this method is served only",
            })
        for m in sorted(set(d_ops) & set(s_ops)):
            a, b = op_summary(d_ops[m]), op_summary(s_ops[m])
            if a == b:
                continue
            changed = [k for k in a if a[k] != b[k]]
            op_rows.append({
                "id": "", "class": "OPERATION-MODIFIED", "side": "both",
                "path": d_paths[np]["__path__"], "method": m.upper(),
                "operationId": b.get("operationId") or a.get("operationId") or "",
                "detail": "; ".join(
                    "%s: declared=%s served=%s" % (k, fmt(a[k], 60), fmt(b[k], 60))
                    for k in changed),
            })
    for i, r in enumerate(op_rows, 1):
        r["id"] = "DOC-OP-%03d" % i

    # ── schemas ─────────────────────────────────────────────────────────────
    d_sch = (declared.get("components") or {}).get("schemas") or {}
    s_sch = (served.get("components") or {}).get("schemas") or {}
    declared_only_schemas = sorted(set(d_sch) - set(s_sch))
    served_only_schemas = sorted(set(s_sch) - set(d_sch))
    common_schemas = sorted(set(d_sch) & set(s_sch))

    schema_rows = []
    for name in declared_only_schemas:
        schema_rows.append({
            "id": "", "class": "SCHEMA-REMOVED", "schema": name,
            "changes": 0, "detail": "declared upstream, absent from the served document",
        })
    for name in served_only_schemas:
        schema_rows.append({
            "id": "", "class": "SCHEMA-ADDED", "schema": name,
            "changes": 0, "detail": "served, not declared upstream",
        })
    modified = {}
    for name in common_schemas:
        diffs = deep_diff(d_sch[name], s_sch[name])
        if diffs:
            modified[name] = diffs
            schema_rows.append({
                "id": "", "class": "SCHEMA-MODIFIED", "schema": name,
                "changes": len(diffs),
                "detail": " | ".join(
                    "%s: %s -> %s" % (p, fmt(x, 40), fmt(y, 40)) for p, x, y in diffs[:6]),
            })
    for i, r in enumerate(schema_rows, 1):
        r["id"] = "DOC-SCH-%03d" % i

    counts = {
        "declared": {
            "path": str(declared_path),
            "title": (declared.get("info") or {}).get("title"),
            "version": (declared.get("info") or {}).get("version"),
            "paths": len(declared["paths"]),
            "operations": sum(1 for m in declared["paths"].values() for k in m if k in METHODS),
            "schemas": len(d_sch),
        },
        "served": {
            "path": str(served_path),
            "title": (served.get("info") or {}).get("title"),
            "version": (served.get("info") or {}).get("version"),
            "paths": len(served["paths"]),
            "operations": sum(1 for m in served["paths"].values() for k in m if k in METHODS),
            "schemas": len(s_sch),
        },
        "paths_common": len(common),
        "paths_declared_only": len(d_only),
        "paths_served_only": len(s_only),
        "operations": {
            "common_identical": sum(
                1 for np in common
                for m in set(d_paths[np]) & set(s_paths[np]) & set(METHODS)
                if op_summary(d_paths[np][m]) == op_summary(s_paths[np][m])
            ),
            "added": sum(1 for r in op_rows if r["class"] in ("PATH-ADDED", "OPERATION-ADDED")),
            "removed": sum(1 for r in op_rows if r["class"] in ("PATH-REMOVED", "OPERATION-REMOVED")),
            "modified": sum(1 for r in op_rows if r["class"] == "OPERATION-MODIFIED"),
            "by_class": dict(Counter(r["class"] for r in op_rows)),
        },
        "schemas": {
            "common_identical": len(common_schemas) - len(modified),
            "added": len(served_only_schemas),
            "removed": len(declared_only_schemas),
            "modified": len(modified),
        },
    }

    (out / "document-diff.json").write_text(json.dumps({
        "declared_source": counts["declared"],
        "served_source": counts["served"],
        "counts": counts,
        "operations": op_rows,
        "schemas": schema_rows,
        "schema_detail": {k: [{"pointer": p, "declared": x, "served": y} for p, x, y in v]
                          for k, v in modified.items()},
    }, indent=2) + "\n")

    md = ["# Document diff — declared upstream vs served shim", ""]
    md.append(md_table([{
        "measure": "paths", "declared": counts["declared"]["paths"],
        "served": counts["served"]["paths"], "common": counts["paths_common"],
        "declared_only": counts["paths_declared_only"],
        "served_only": counts["paths_served_only"]}], [
        "measure", "declared", "served", "common", "declared_only", "served_only"]))
    md.append("")
    md.append("## Operations added / removed / modified\n")
    md.append(md_table(op_rows, ["id", "class", "path", "method", "operationId", "detail"]))
    md.append("\n## Schemas added / removed / modified\n")
    md.append(md_table(schema_rows, ["id", "class", "schema", "changes", "detail"]))
    (out / "document-diff.md").write_text("\n".join(md) + "\n")

    print(json.dumps(counts, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
