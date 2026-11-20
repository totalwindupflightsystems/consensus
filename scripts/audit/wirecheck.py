#!/usr/bin/env python3
"""wirecheck — the semantic wiring layer a file-import graph cannot provide.

Hilo answers "what depends on this file". This answers:
  1 TABLE-PAIR   does every table the code READS have a production WRITER?
  2 STUB         does a dispatch path return a marker instead of executing?
  3 TEST-SEAM    does a test CREATE/INSERT what production is supposed to produce?
  4 DDL-DRIFT    does a test fixture declare a kinder schema than the migration?

Usage: wirecheck.py <repo-root> [--json] [--only=table-pair,stub,test-seam,ddl-drift]

--only selects the sections to run AND to judge: with --only=table-pair the
exit code reflects orphan reads only, so a CI gate can enforce the table-pair
class while the separately-tracked STUB/TEST-SEAM debt is still printed.
Without --only every section runs and the run fails on any TABLE-PAIR or STUB
finding (unchanged behaviour).

Exit 1 if any selected TABLE-PAIR or STUB finding is present (gate-able), 2 on
usage errors.
"""
import json
import os
import re
import sys

RE_INSERT = re.compile(r"(?i)\bINSERT\s+INTO\s+([a-z_][a-z0-9_]*)")
RE_UPDATE = re.compile(r"(?i)\bUPDATE\s+([a-z_][a-z0-9_]*)\s+SET")
RE_DELETE = re.compile(r"(?i)\bDELETE\s+FROM\s+([a-z_][a-z0-9_]*)")
RE_FROM   = re.compile(r"(?i)\bFROM\s+([a-z_][a-z0-9_]*)")
RE_CREATE = re.compile(r"(?i)\bCREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-z_][a-z0-9_]*)")
RE_STUB   = re.compile(r'(?i)"[^"\n]*\b(stub|not[_ ]implemented|unimplemented|placeholder)\b[^"\n]*"')
RE_FUNC   = re.compile(r"^func\s+(?:\([^)]*\)\s*)?([A-Za-z0-9_]+)\s*\(")

SKIP_DIRS = {"wt", ".git", "vendor", "node_modules", ".vfs", "dist", "build"}

# Every section the script can run. --only picks a subset; the exit code is
# computed from the selected sections only (see main()).
ALL_CHECKS = ("table-pair", "stub", "test-seam", "ddl-drift")


def go_files(root):
    prod, test = [], []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        for fn in filenames:
            if not fn.endswith(".go"):
                continue
            p = os.path.join(dirpath, fn)
            (test if fn.endswith("_test.go") else prod).append(p)
    return prod, test


def scan(root, files):
    """table -> {'write': set(files), 'read': set(files)}

    File paths are reported relative to the scanned ROOT (not to the caller's
    CWD) so a report is meaningful no matter where the script was invoked from.
    """
    uses = {}

    def ent(t):
        return uses.setdefault(t, {"write": set(), "read": set()})

    for f in files:
        try:
            s = open(f, encoding="utf-8", errors="ignore").read()
        except OSError:
            continue
        rel = os.path.relpath(f, root)
        for rx in (RE_INSERT, RE_UPDATE, RE_DELETE):
            for m in rx.findall(s):
                ent(m.lower())["write"].add(rel)
        for m in RE_FROM.findall(s):
            ent(m.lower())["read"].add(rel)
    return uses


def schema_tables(root):
    d = os.path.join(root, "internal", "migrate", "migrations")
    tabs = {}
    if not os.path.isdir(d):
        return tabs
    for fn in sorted(os.listdir(d)):
        if not fn.endswith(".sql"):
            continue
        s = open(os.path.join(d, fn), encoding="utf-8", errors="ignore").read()
        for m in RE_CREATE.findall(s):
            tabs.setdefault(m.lower(), fn)
    return tabs


def check_table_pair(root, prod, schema, report):
    uses = scan(root, prod)
    orphans, sinks = [], []
    for t in sorted(uses):
        if t not in schema:
            continue
        w, r = len(uses[t]["write"]), len(uses[t]["read"])
        if r > 0 and w == 0:
            orphans.append((t, r))
        elif w > 0 and r == 0:
            sinks.append((t, w))
    report["table_pair"] = {"tables_in_schema": len(schema), "orphan_reads": orphans, "write_only": sinks}
    print("== 1 TABLE-PAIR — read with no production writer ==")
    print(f"   schema tables: {len(schema)}")
    if orphans:
        for t, r in orphans:
            print(f"   !! {t:<26} read by {r} file(s), written by 0 production file(s)")
            # Name the readers: "which file reads it" is the first thing a
            # resolver needs, and the report must be actionable on its own.
            for reader in sorted(uses[t]["read"])[:3]:
                print(f"        reader: {reader}")
            extra = len(uses[t]["read"]) - 3
            if extra > 0:
                print(f"        ... and {extra} more reader(s)")
    else:
        print("   none")
    if sinks:
        print(f"   ~ write-only (nobody reads): {', '.join(t for t,_ in sinks)}")
    print()


def check_stubs(root, prod, report):
    print("== 2 STUB — dispatch paths returning a marker ==")
    hits = []
    for f in prod:
        try:
            lines = open(f, encoding="utf-8", errors="ignore").read().splitlines()
        except OSError:
            continue
        for i, ln in enumerate(lines):
            m = RE_STUB.search(ln)
            if not m:
                continue
            fn = "?"
            for j in range(i, -1, -1):
                fm = RE_FUNC.match(lines[j])
                if fm:
                    fn = fm.group(1)
                    break
            hits.append({"func": fn, "file": os.path.relpath(f), "line": i + 1, "marker": m.group(0)[:60]})
    report["stubs"] = hits
    if hits:
        for h in hits:
            print(f"   !! {h['func']:<42} {h['file']}:{h['line']}")
            print(f"        returns {h['marker']}")
    else:
        print("   none")
    print()


def check_test_seam(root, test, prod_uses, report):
    print("== 3 TEST-SEAM — tests building state production should produce ==")
    seams = []
    for f in test:
        try:
            s = open(f, encoding="utf-8", errors="ignore").read()
        except OSError:
            continue
        kinds = []
        for t in {m.lower() for m in RE_CREATE.findall(s)}:
            kinds.append("CREATE TABLE " + t)
        for t in {m.lower() for m in RE_INSERT.findall(s)}:
            if t in prod_uses and not prod_uses[t]["write"]:
                kinds.append(f"INSERTs {t} (production never writes it)")
        if kinds:
            seams.append({"file": os.path.relpath(f), "kinds": sorted(kinds)})
    seams.sort(key=lambda x: x["file"])
    report["test_seam"] = seams
    print(f"   test files owning their own schema/state: {len(seams)}")
    for s in seams[:10]:
        print(f"   {s['file']}")
        for k in s["kinds"][:4]:
            print(f"       - {k}")
    if len(seams) > 10:
        print(f"   ... and {len(seams)-10} more")
    print()


def check_ddl_drift(root, test, schema, report):
    """Compare NOT NULL / DEFAULT between migration and test fixture per column."""
    print("== 4 DDL-DRIFT — fixture kinder than production ==")
    mig = {}
    d = os.path.join(root, "internal", "migrate", "migrations")
    for fn in (sorted(os.listdir(d)) if os.path.isdir(d) else []):
        if not fn.endswith(".sql"):
            continue
        s = open(os.path.join(d, fn), encoding="utf-8", errors="ignore").read()
        for m in re.finditer(r"(?is)CREATE TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-z_][a-z0-9_]*)\s*\((.*?)\n\s*\);", s):
            t = m.group(1).lower()
            cols = {}
            for line in m.group(2).split("\n"):
                cm = re.match(r"\s*([a-z_][a-z0-9_]*)\s+([A-Z]+)", line, re.I)
                if cm and cm.group(1).lower() not in ("primary", "foreign", "unique", "check", "constraint"):
                    cols[cm.group(1).lower()] = "DEFAULT" in line.upper()
            if cols:
                mig[t] = cols
    drifts = []
    for f in test:
        try:
            s = open(f, encoding="utf-8", errors="ignore").read()
        except OSError:
            continue
        for m in re.finditer(r"(?is)CREATE TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-z_][a-z0-9_]*)\s*\((.*?)\n\s*\)`", s):
            t = m.group(1).lower()
            if t not in mig:
                continue
            for line in m.group(2).split("\n"):
                cm = re.match(r"\s*([a-z_][a-z0-9_]*)\s+", line)
                if not cm:
                    continue
                c = cm.group(1).lower()
                if c in mig[t] and not mig[t][c] and "DEFAULT" in line.upper():
                    drifts.append(f"{t}.{c}: production NOT NULL w/o default, fixture supplies one")
    report["ddl_drift"] = drifts
    if drifts:
        for x in sorted(set(drifts)):
            print(f"   !! {x}")
    else:
        print("   none detected")
    print()


def parse_only(argv):
    """Return the selected check names, or None for "run everything".

    Accepts --only=a,b and --only a,b (the second form keeps the pre-existing
    bare-root calling convention comfortable).
    """
    raw = None
    i = 0
    while i < len(argv):
        arg = argv[i]
        if arg.startswith("--only="):
            raw = arg.split("=", 1)[1]
            i += 1
        elif arg == "--only":
            if i + 1 >= len(argv):
                raise ValueError("--only requires a comma-separated list of sections")
            raw = argv[i + 1]
            i += 2
        else:
            i += 1
    if raw is None:
        return None
    selected = [s.strip() for s in raw.split(",") if s.strip()]
    unknown = [s for s in selected if s not in ALL_CHECKS]
    if unknown or not selected:
        raise ValueError(
            "unknown section name(s): %s (valid: %s)"
            % (", ".join(unknown) or "(empty)", ", ".join(ALL_CHECKS))
        )
    return selected


def main():
    args = list(sys.argv[1:])
    try:
        only = parse_only(args)
    except ValueError as exc:
        print(f"wirecheck: {exc}", file=sys.stderr)
        return 2
    # --only VALUE consumes a positional-looking token; drop the flag (and its
    # value, when it was passed space-separated) before picking the root.
    cleaned, skip_next = [], False
    for arg in args:
        if skip_next:
            skip_next = False
            continue
        if arg == "--only":
            skip_next = True
            continue
        if arg.startswith("--only="):
            continue
        cleaned.append(arg)
    positional = [a for a in cleaned if not a.startswith("--")]
    if not positional:
        print(__doc__)
        return 2
    root = os.path.abspath(positional[0])
    as_json = "--json" in args
    prod, test = go_files(root)
    schema = schema_tables(root)
    prod_uses = scan(root, prod)
    selected = set(only or ALL_CHECKS)
    report = {"root": root, "prod_files": len(prod), "test_files": len(test)}
    if only is not None:
        report["only"] = sorted(selected)

    if not as_json:
        print(f"wirecheck  root={root}")
        print(f"           prod={len(prod)} go files, test={len(test)} go files")
        if only is not None:
            print(f"           sections: {', '.join(sorted(selected))}")
        print()
    if "table-pair" in selected:
        check_table_pair(root, prod, schema, report)
    if "stub" in selected:
        check_stubs(root, prod, report)
    if "test-seam" in selected:
        check_test_seam(root, test, prod_uses, report)
    if "ddl-drift" in selected:
        check_ddl_drift(root, test, schema, report)

    if as_json:
        print(json.dumps(report, indent=2))

    fail = False
    if "table-pair" in selected:
        fail = fail or bool(report.get("table_pair", {}).get("orphan_reads"))
    if "stub" in selected:
        fail = fail or bool(report.get("stubs"))
    if not as_json:
        print("RESULT:", "FAIL (wiring gaps present)" if fail else "PASS")
    return 1 if fail else 0


if __name__ == "__main__":
    sys.exit(main())
