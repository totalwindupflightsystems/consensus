#!/usr/bin/env python3
# axiom:trace work_item=SHIM-SOURCE-003 spec=specs/024-shim-support-matrix.md impl=scripts/spec-parity-check.py test=internal/api/openapi_parity_test.go
"""OpenAPI declared-vs-served path parity check (specs/024 §A.3, SHIM-SOURCE-003).

specs/024 §A.3 requires the shim/REST surface to be compared mechanically after
every refresh: **declared == served, per path**. This script is that comparison
for the Consensus OpenAPI contract, and it is the engine behind
`make check-spec-parity` (scripts/check-spec-parity.sh owns the scratch server).

  declared = the authored split spec: `specs/openapi/openapi.yaml` plus the
             `paths/*.yaml` path items it references (the source redocly
             bundles). `specs/openapi/bundled.yaml` is a GENERATED artifact, so
             it is deliberately not the declared side: comparing the bundle
             against the bundle would be a tautology.
  served   = `GET /openapi.json` of a running server (its embedded
             `specs/openapi/bundled.yaml`), plus the runtime route table
             exercised operation-by-operation.

Three drift arms, each named in the output:

  MISSING     declared (source) operation absent from the served document
              -> someone edited ids in `paths/` without re-bundling/rebuilding
  EXTRA       served document operation not declared by the source
              -> the published contract promises a path the source no longer has
  ORPHAN      path item in `paths/*.yaml` that `openapi.yaml` does not reference
              -> declared in a file, invisible to the contract
  UNANSWERED  declared (non-stub) operation the runtime does not route
              -> declared spec, no handler (404 "page not found" / 405)

Drift in ANY arm exits non-zero with one named line per finding. A path item
marked `x-not-implemented: true` is a deliberately declared-but-unserved stub:
its operations are excluded from MISSING and UNANSWERED (they may answer
anything) and counted as exempt in the summary.

Usage:
  python3 scripts/spec-parity-check.py --base-url http://127.0.0.1:18242 [options]

Options:
  --base-url URL      server root to compare against (required for the served
                      document and the runtime arm)
  --served-file FILE  read the served document from a saved /openapi.json
                      instead of fetching it (offline comparison)
  --no-runtime        skip the runtime arm (document comparison only)
  --spec-root DIR     override the declared spec root (default specs/openapi);
                      also settable via CONSENSUS_SPEC_PARITY_ROOT
  --commit SHA        the revision this run is recorded at (default: git HEAD)
  --json              also print one JSON object with the full result
"""

import argparse
import http.client
import json
import os
import subprocess
import sys
import time
from pathlib import Path
from urllib.parse import urlsplit

try:
    import yaml
except ImportError:  # pragma: no cover - environment prerequisite
    sys.stderr.write(
        "spec-parity: PyYAML is required to read the split spec "
        "(python3 -m pip install pyyaml)\n"
    )
    sys.exit(2)

REPO = Path(__file__).resolve().parent.parent
HTTP_METHODS = ("get", "post", "put", "patch", "delete", "head", "options", "trace")
# The published document is ours, not some stranger's on the same port: a
# running server we did not start must never be measured (see the refresh
# procedure §10 "bind: address already in use").
EXPECTED_TITLE = "Consensus Agent Runtime"
RUNTIME_BUDGET_SECONDS = 180
RUNTIME_PROBE_TIMEOUT = 10.0


class Drift(Exception):
    """A structural problem that stops the comparison (not a drift finding)."""


def operation_methods(item):
    """HTTP methods of a path-item object, in spec order."""
    if not isinstance(item, dict):
        return []
    return [m for m in HTTP_METHODS if m in item]


def is_stub(item):
    return isinstance(item, dict) and item.get("x-not-implemented") is True


def load_declared(spec_root):
    """Resolve `declared` from the split source.

    Returns (declared, stubs, orphans):
      declared: {"<METHOD> <path>": "<source file>"}
      stubs:    set of path items marked x-not-implemented
      orphans:  ["<file>#<key> (/path)"] path items no openapi.yaml ref reaches
    """
    root_yaml = spec_root / "openapi.yaml"
    if not root_yaml.is_file():
        raise Drift(f"declared spec root has no openapi.yaml: {root_yaml}")
    index = yaml.safe_load(root_yaml.read_text())

    paths = index.get("paths")
    if not isinstance(paths, dict) or not paths:
        raise Drift(f"{root_yaml} declares no paths object")

    declared = {}
    stubs = set()
    # spec-root-relative file -> anchors openapi.yaml references
    # ("" == the whole file is the path item)
    referenced = {}
    loaded = {}

    def load(rel_file):
        if rel_file not in loaded:
            full = spec_root / rel_file
            if not full.is_file():
                raise Drift(f"openapi.yaml references missing file: {rel_file}")
            loaded[rel_file] = yaml.safe_load(full.read_text())
        return loaded[rel_file]

    for path, ref in paths.items():
        if not isinstance(ref, dict) or "$ref" not in ref:
            raise Drift(f"{root_yaml} path {path} is not a $ref (found {ref!r})")
        target = str(ref["$ref"])
        file_part, _, anchor = target.partition("#")
        # $refs are relative to openapi.yaml's own directory ("./paths/x.yaml").
        rel_file = os.path.normpath(file_part.lstrip("/")) or "openapi.yaml"
        anchor = anchor.lstrip("/")
        referenced.setdefault(rel_file, set()).add(anchor)

        doc = load(rel_file)
        item = doc.get(anchor) if anchor else doc
        if not isinstance(item, dict):
            raise Drift(f"{path} -> {target}: resolved path item is not an object")
        methods = operation_methods(item)
        if not methods:
            raise Drift(f"{path} -> {target}: path item declares no HTTP method")
        if is_stub(item):
            stubs.add(path)
        for method in methods:
            declared[f"{method.upper()} {path}"] = f"{rel_file}#{anchor}" if anchor else rel_file

    orphans = []
    paths_dir = spec_root / "paths"
    for path_file in sorted(os.listdir(paths_dir)) if paths_dir.is_dir() else []:
        if not path_file.endswith((".yaml", ".yml")):
            continue
        rel_file = f"paths/{path_file}"
        doc = load(rel_file)
        if not isinstance(doc, dict):
            continue
        used = referenced.get(rel_file, set())
        for key, value in doc.items():
            if key in used or key == "" or not operation_methods(value):
                continue
            orphans.append(f"{rel_file}#{key}")

    return declared, stubs, orphans


def load_served_document(args):
    """Fetch (or read) the served OpenAPI document and return its operations."""
    if args.served_file:
        raw = Path(args.served_file).read_bytes()
        source = args.served_file
    else:
        if not args.base_url:
            raise Drift("--base-url (or --served-file) is required")
        raw = fetch_document(args.base_url)
        source = f"{args.base_url.rstrip('/')}/openapi.json"

    try:
        doc = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise Drift(f"served document at {source} is not JSON: {exc}")

    title = (doc.get("info") or {}).get("title")
    if title != EXPECTED_TITLE:
        raise Drift(
            f"served document at {source} has title {title!r}, expected "
            f"{EXPECTED_TITLE!r} — refusing to compare against a foreign server"
        )

    served, stubs = {}, set()
    for path, item in (doc.get("paths") or {}).items():
        methods = operation_methods(item)
        if not methods:
            continue
        if is_stub(item):
            stubs.add(path)
        for method in methods:
            served[f"{method.upper()} {path}"] = source

    if not served:
        raise Drift(f"served document at {source} declares no operations")
    return served, stubs, source


def fetch_document(base_url):
    parts = urlsplit(base_url)
    conn = http.client.HTTPConnection(parts.hostname or "127.0.0.1", parts.port or 80, timeout=15)
    try:
        conn.request("GET", "/openapi.json")
        resp = conn.getresponse()
        body = resp.read()
        if resp.status != 200:
            raise Drift(
                f"GET {base_url.rstrip('/')}/openapi.json answered {resp.status} "
                f"(expected 200): {body[:200]!r}"
            )
        return body
    except OSError as exc:
        raise Drift(f"GET {base_url.rstrip('/')}/openapi.json failed: {exc}")
    finally:
        conn.close()


def probe_runtime(base_url, declared, stubs):
    """Round-trip every declared non-stub operation against the live router.

    A declared operation is ANSWERED unless the router replies 405 (method not
    registered) or chi's bare `404 page not found` (no route at all). Any other
    answer — 2xx, 400, 401, 403, a typed 404 body, 5xx — proves the route is
    registered; auth is not what this arm measures.
    """
    parts = urlsplit(base_url)
    host = parts.hostname or "127.0.0.1"
    port = parts.port or 80
    unanswered, exempt, probed = [], 0, 0
    started = time.monotonic()

    for key in sorted(declared):
        method, path = key.split(" ", 1)
        if path in stubs:
            exempt += 1
            continue
        if time.monotonic() - started > RUNTIME_BUDGET_SECONDS:
            raise Drift(
                f"runtime arm exceeded its {RUNTIME_BUDGET_SECONDS}s budget after "
                f"{probed} operations — is a route hanging before its headers?"
            )
        probed += 1
        target = instantiate(path)
        conn = http.client.HTTPConnection(host, port, timeout=RUNTIME_PROBE_TIMEOUT)
        try:
            conn.request(method, target)
            resp = conn.getresponse()
            status = resp.status
            body = resp.read(4096) if status == 404 else b""
        except OSError as exc:
            unanswered.append(f"{key} — no answer from the runtime: {exc}")
            continue
        finally:
            conn.close()

        if status == 405:
            unanswered.append(f"{key} — 405 Method Not Allowed (route not registered for {method})")
        elif status == 404 and b"page not found" in body:
            unanswered.append(f"{key} — 404 page not found (route not registered)")
    return unanswered, exempt, probed


def instantiate(pattern):
    """Replace every {param} segment with a plausible id (u-u-i-d shaped)."""
    return "/".join(
        "00000000-0000-0000-0000-000000000000"
        if seg.startswith("{") and seg.endswith("}")
        else seg
        for seg in pattern.split("/")
    )


def diff_parity(declared, served, stubs, orphans, unanswered):
    """Pure comparison: returns (findings, exempt_path_count)."""
    findings = []
    for key in sorted(set(declared) - set(served)):
        path = key.split(" ", 1)[1]
        if path in stubs:
            continue  # deliberately declared-but-unserved
        findings.append(("MISSING", f"{key} — declared in {declared[key]} but absent from the served document"))
    for key in sorted(set(served) - set(declared)):
        findings.append(("EXTRA", f"{key} — served but not declared ({served[key]})"))
    for orphan in sorted(orphans):
        findings.append(("ORPHAN", f"{orphan} — path item not referenced by openapi.yaml (invisible to the built contract)"))
    for line in unanswered:
        findings.append(("UNANSWERED", line))
    return findings


def git_commit():
    try:
        out = subprocess.run(
            ["git", "-C", str(REPO), "rev-parse", "HEAD"],
            capture_output=True, text=True, check=True,
        )
        return out.stdout.strip()
    except (OSError, subprocess.CalledProcessError):
        return "unknown"


def git_dirty():
    try:
        out = subprocess.run(
            ["git", "-C", str(REPO), "status", "--porcelain"],
            capture_output=True, text=True, check=True,
        )
        return len([line for line in out.stdout.splitlines() if line.strip()])
    except (OSError, subprocess.CalledProcessError):
        return -1


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--base-url")
    parser.add_argument("--served-file")
    parser.add_argument("--spec-root")
    parser.add_argument("--commit")
    parser.add_argument("--no-runtime", action="store_true")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args(argv)

    spec_root = Path(
        args.spec_root or os.environ.get("CONSENSUS_SPEC_PARITY_ROOT") or (REPO / "specs" / "openapi")
    ).resolve()
    commit = args.commit or git_commit()
    dirty = git_dirty()

    try:
        declared, declared_stubs, orphans = load_declared(spec_root)
        served, _, served_source = load_served_document(args)
    except Drift as exc:
        print(f"spec-parity: ERROR — {exc}", file=sys.stderr)
        return 2

    unanswered, exempt_ops, probed = [], 0, 0
    runtime_skipped = None
    if args.no_runtime or not args.base_url:
        runtime_skipped = "--no-runtime" if args.no_runtime else "no --base-url"
    else:
        try:
            unanswered, exempt_ops, probed = probe_runtime(
                args.base_url, declared, declared_stubs
            )
        except Drift as exc:
            print(f"spec-parity: ERROR — {exc}", file=sys.stderr)
            return 2

    findings = diff_parity(declared, served, declared_stubs, orphans, unanswered)
    declared_paths = {k.split(" ", 1)[1] for k in declared}
    served_paths = {k.split(" ", 1)[1] for k in served}

    print(f"spec-parity: commit={commit}" + (f" (working tree dirty: {dirty} paths)" if dirty > 0 else " (working tree clean)"))
    print(f"spec-parity: declared={len(declared_paths)} paths / {len(declared)} ops from {spec_root}/openapi.yaml")
    print(f"spec-parity: served={len(served_paths)} paths / {len(served)} ops from {served_source}")
    print(f"spec-parity: declared x-not-implemented path items={len(declared_stubs)} "
          f"({', '.join(sorted(declared_stubs)) or 'none'}) — excluded from MISSING/UNANSWERED")
    if runtime_skipped:
        print(f"spec-parity: runtime arm skipped ({runtime_skipped}) — document comparison only")
    else:
        print(f"spec-parity: runtime {probed} ops probed, {len(unanswered)} unanswered, {exempt_ops} exempt")

    if findings:
        print(f"spec-parity: FAIL — {len(findings)} drift finding(s) at commit {commit}")
        for kind, detail in findings:
            print(f"  {kind:11s} {detail}")
        print(
            f"spec-parity: FAIL — declared != served at commit {commit} "
            f"({len(findings)} finding(s); rerun `make bundle-spec` + rebuild, or file the drift)"
        )
    else:
        print(f"spec-parity: PASS — declared == served at commit {commit} "
              f"({len(declared_paths)} paths / {len(declared)} ops)")

    if args.json:
        print(json.dumps({
            "commit": commit,
            "dirty_paths": dirty,
            "spec_root": str(spec_root),
            "served_source": served_source,
            "declared_paths": len(declared_paths),
            "declared_ops": len(declared),
            "served_paths": len(served_paths),
            "served_ops": len(served),
            "exempt_path_items": sorted(declared_stubs),
            "findings": [{"kind": k, "detail": d} for k, d in findings],
            "pass": not findings,
        }, sort_keys=True))

    return 1 if findings else 0


if __name__ == "__main__":
    sys.exit(main())
