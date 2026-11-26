#!/usr/bin/env python3
"""wirecheck — the semantic wiring layer a file-import graph cannot provide.

Hilo answers "what depends on this file". This answers:
  1 TABLE-PAIR   does every table the code READS have a production WRITER?
  2 STUB         does a dispatch path return a marker instead of executing?
  3 TEST-SEAM    does a test CREATE/INSERT what production is supposed to produce?
  4 DDL-DRIFT    does a test fixture declare a kinder schema than the migration?

Usage: wirecheck.py <repo-root> [--json] [--only=table-pair,stub,test-seam,ddl-drift]
                    [--baseline PATH | --write-baseline PATH]

--only selects the sections to run AND to judge: with --only=table-pair the
exit code reflects orphan reads only, so a CI gate can enforce the table-pair
class while the separately-tracked STUB/TEST-SEAM debt is still printed.
Without --only every section runs and the run fails on any TABLE-PAIR or STUB
finding (unchanged behaviour).

Ratchet mode (WIRE-CONSENSUS-4) — fail on NEW findings, not on known debt:
  --write-baseline PATH  helper: record the current fail-able findings of the
                         selected sections into PATH (human-readable text, one
                         finding per line, sorted, with a per-line annotation)
                         and exit 0. Re-run it whenever a finding is fixed to
                         tighten the committed baseline.
  --baseline PATH        ratchet gate: compare the current findings against the
                         baseline file. Current findings == baseline or a
                         subset -> exit 0 (a strict subset prints a note that
                         the baseline can be tightened). Any NEW finding not
                         in the baseline -> exit 1, printed clearly as a
                         ratchet violation. A missing/unreadable baseline file
                         is a usage error.

Exit 1 if any selected TABLE-PAIR or STUB finding is present (gate-able; in
--baseline mode only NEW findings fail), 2 on usage errors.
"""
import json
import os
import re
import sys
import time

RE_INSERT = re.compile(r"(?i)\bINSERT\s+INTO\s+([a-z_][a-z0-9_]*)")
RE_UPDATE = re.compile(r"(?i)\bUPDATE\s+([a-z_][a-z0-9_]*)\s+SET")
RE_DELETE = re.compile(r"(?i)\bDELETE\s+FROM\s+([a-z_][a-z0-9_]*)")
RE_FROM   = re.compile(r"(?i)\bFROM\s+([a-z_][a-z0-9_]*)")
RE_CREATE = re.compile(r"(?i)\bCREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-z_][a-z0-9_]*)")
RE_STUB   = re.compile(r'(?i)"[^"\n]*\b(stub|not[_ ]implemented|unimplemented|placeholder)\b[^"\n]*"')
RE_FUNC   = re.compile(r"^func\s+(?:\([^)]*\)\s*)?([A-Za-z0-9_]+)\s*\(")

# ── STUB classification (WIRE-CONSENSUS-2) ──────────────────────────────────
# RE_STUB only proves a quoted marker exists. The law this section enforces is
# narrower: a caller must never receive a SUCCESS status for work that did not
# happen. A marker therefore has exactly two legal shapes, and one illegal one:
#
#   refusal   — the marker is handed to an explicit failure writer, so the
#               caller receives an error status (501 for a declared narrowing,
#               a documented 4xx for a rejected input) instead of a success
#               payload describing work that never happened. The 501 narrowings
#               are declared in the served OpenAPI document (paths/shim.yaml),
#               so the contract and the wire agree.
#   allowlist — the marker provably is not a dispatch return at all. Every
#               entry is an exact (file, marker-prefix) pair carrying a reason
#               and the board row that justified it, so the list can never
#               silently rot into a blanket exemption.
#   returned  — anything else: a marker returned as a handler's outcome, or
#               text nobody can classify. THIS is the defect.
#
# Adding an allowlist entry to silence a real returned stub is exactly the
# failure this section exists to catch.
RE_FAILURE_WRITER = re.compile(
    r"writeNotImplemented\("
    r"|StatusNotImplemented"
    r"|writeOpencodeNamedError\("
    r"|writeOpencodeBadRequest\("
    r"|writeOpencodeError\("
    r"|writeV2InvalidRequest\("
    r"|writeJSONStatus\("
    r"|http\.Error\("
)
# A refusal writer call can wrap the marker several lines below it (the call's
# argument list spans lines); look back generously and forward a little.
STUB_WINDOW_BEFORE = 6
STUB_WINDOW_AFTER = 3

# Narrow, reviewer-visible exemptions. `marker_prefix` must be a PREFIX of the
# exact quoted text RE_STUB matched in that file — not a regex, not a substring
# elsewhere — so an exemption cannot drift onto a different string.
STUB_ALLOWLIST = (
    {
        "file": "internal/llm/shim_matrix.go",
        "marker_prefix": '"SHIM-SUPPORT-004',
        "reason": "evidence note quoting a board/test id, not a dispatch return",
        "row": "WIRE-CONSENSUS-2",
    },
    {
        "file": "internal/web/server.go",
        "marker_prefix": '" placeholder="',
        "reason": "HTML <input placeholder=...> attribute in the served UI, not a stub return",
        "row": "WIRE-CONSENSUS-2",
    },
)


def stub_allowlist_entry(rel_path, text):
    """Return the STUB_ALLOWLIST entry covering this exact marker, or None.

    Matching is (file, exact string prefix): an entry for one file never
    excuses the same text in another file, and a prefix that is not at the
    very start of the matched literal never matches.
    """
    for entry in STUB_ALLOWLIST:
        if entry["file"] == rel_path and text.startswith(entry["marker_prefix"]):
            return entry
    return None


# Functions whose whole job is to answer a failure status. A marker INSIDE one
# of these is a refusal by construction — including the envelope constants and
# the log line that name the refusal shape (writeNotImplemented's own body:
# w.WriteHeader(http.StatusNotImplemented) then the typed body then a WARN).
# Kept as an explicit set so the exemption is auditable, not inferred from a
# naming convention that a future helper could accidentally match.
REFUSAL_WRITER_FUNCS = frozenset({
    "writeNotImplemented",
    "writeOpencodeError",
    "writeOpencodeNamedError",
    "writeOpencodeBadRequest",
    "writeV2InvalidRequest",
    "writeJSONStatus",
})


def stub_is_refusal(func_name, window):
    """A marker is a refusal when the caller receives an error status."""
    if func_name in REFUSAL_WRITER_FUNCS:
        return True
    return bool(RE_FAILURE_WRITER.search(window))

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
    violations, safe = [], []
    for f in prod:
        try:
            lines = open(f, encoding="utf-8", errors="ignore").read().splitlines()
        except OSError:
            continue
        rel = os.path.relpath(f, root)
        for i, ln in enumerate(lines):
            m = RE_STUB.search(ln)
            if not m:
                continue
            text = m.group(0)
            fn = "?"
            for j in range(i, -1, -1):
                fm = RE_FUNC.match(lines[j])
                if fm:
                    fn = fm.group(1)
                    break
            hit = {"func": fn, "file": rel, "line": i + 1, "marker": text[:60]}
            allowed = stub_allowlist_entry(rel, text)
            if allowed is not None:
                hit["kind"] = "allowlisted"
                hit["reason"] = allowed["reason"]
                hit["row"] = allowed["row"]
                safe.append(hit)
                continue
            window = "\n".join(lines[max(0, i - STUB_WINDOW_BEFORE):i + STUB_WINDOW_AFTER])
            if stub_is_refusal(fn, window):
                # The marker is handed to an explicit failure writer: the
                # caller receives an error status (501 / documented 4xx), never
                # a success payload describing work that did not happen.
                hit["kind"] = "refusal"
                safe.append(hit)
                continue
            hit["kind"] = "returned"
            violations.append(hit)

    # report["stubs"] is the DEFECT list — main() fails the run on it and
    # --json consumers have always read that key as "markers to fix".
    report["stubs"] = violations
    report["stub_safe"] = safe
    refusals = [h for h in safe if h["kind"] == "refusal"]
    allowed = [h for h in safe if h["kind"] == "allowlisted"]
    print(f"   refusals: {len(refusals)} marker(s) hand the caller a declared failure status")
    print(f"   allowlisted: {len(allowed)} marker(s) are provably not dispatch returns")
    for h in allowed:
        print(f"      ok {h['file']}:{h['line']} — {h['reason']} ({h['row']})")
    if violations:
        for h in violations:
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


# ── Ratchet (baseline) mode (WIRE-CONSENSUS-4) ───────────────────────────────
# Value-taking flags besides --only. Both `--flag VALUE` and `--flag=VALUE`
# spellings are accepted, mirroring parse_only's convention.
VALUE_FLAGS = ("--baseline", "--write-baseline")

BASELINE_HEADER = """\
# wirecheck ratchet baseline
# generated: {generated}  root={root}
#
# The ratchet gate (`wirecheck.py <repo> --baseline <this file>`) fails only on
# NEW findings: a finding present in a run but absent from this file is a
# ratchet violation (exit 1). A finding that disappears must NOT fail —
# tighten the baseline by re-running
#   python3 scripts/audit/wirecheck.py <repo> --write-baseline <this file>
# and committing the smaller file.
#
# format: one finding per line, sorted. Lines starting with '#' are comments;
# a trailing ' # ...' annotation is informational only and is never part of
# the comparison.
"""


def collect_gate_findings(report, selected):
    """The fail-able findings of the selected sections, as sorted (key, note).

    These are exactly the findings main() exits 1 on: TABLE-PAIR orphan reads
    and returned STUB markers. TEST-SEAM and DDL-DRIFT are informational and
    never enter the gate set. The key (before the annotation) is what a
    baseline records and compares on; the note is context for a human reading
    the file.
    """
    findings = []
    if "table-pair" in selected:
        for table, _readers in report.get("table_pair", {}).get("orphan_reads", []):
            findings.append(
                ("table-pair:%s" % table, "orphan read: no production writer")
            )
    if "stub" in selected:
        for hit in report.get("stubs", []):
            findings.append(
                (
                    "stub:%s:%d" % (hit["file"], hit["line"]),
                    "returned marker: %s (in %s)" % (hit["marker"], hit["func"]),
                )
            )
    return sorted(findings)


def write_baseline_file(path, findings, root):
    """Write the baseline for `findings` to PATH (sorted, annotated)."""
    lines = BASELINE_HEADER.format(
        generated=time.strftime("%Y-%m-%dT%H:%M:%S"), root=root
    ).splitlines()
    for key, note in findings:
        # No owning board row is known to the audit itself; when a finding is
        # accepted as debt, name its row here by hand.
        lines.append(
            "%s  # TODO row: name the board row accepting this debt (%s)" % (key, note)
        )
    with open(path, "w", encoding="utf-8") as handle:
        handle.write("\n".join(lines) + "\n")


def read_baseline(path):
    """Return the set of gate keys recorded in a baseline file.

    Comment and blank lines are ignored; the comparison key is the text before
    the first '#', stripped — so the per-line annotations carry no weight.
    Raises ValueError when the file cannot be read (a usage error, not a
    silent "everything is new").
    """
    try:
        with open(path, encoding="utf-8") as handle:
            text = handle.read()
    except OSError as exc:
        raise ValueError("baseline file unreadable: %s" % exc)
    keys = set()
    for ln in text.splitlines():
        key = ln.split("#", 1)[0].strip()
        if key:
            keys.add(key)
    return keys


def main():
    args = list(sys.argv[1:])
    try:
        only = parse_only(args)
    except ValueError as exc:
        print(f"wirecheck: {exc}", file=sys.stderr)
        return 2
    # A value-taking flag consumes a positional-looking token; drop each flag
    # (and its value, when it was passed space-separated) before picking the
    # root and the flag values.
    cleaned, skip_next = [], False
    values = {}
    i = 0
    while i < len(args):
        arg = args[i]
        if skip_next:
            skip_next = False
            i += 1
            continue
        matched = False
        for flag in VALUE_FLAGS:
            if arg.startswith(flag + "="):
                values[flag] = arg.split("=", 1)[1]
                matched = True
                break
            if arg == flag:
                if i + 1 >= len(args):
                    print(f"wirecheck: {flag} requires a path", file=sys.stderr)
                    return 2
                values[flag] = args[i + 1]
                skip_next = True
                matched = True
                break
        if matched:
            i += 1
            continue
        if arg == "--only":
            skip_next = True
        elif not arg.startswith("--only="):
            cleaned.append(arg)
        i += 1
    baseline = values.get("--baseline")
    write_baseline = values.get("--write-baseline")
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

    gate_findings = collect_gate_findings(report, selected)
    report["gate_findings"] = [key for key, _note in gate_findings]

    if write_baseline is not None:
        write_baseline_file(write_baseline, gate_findings, root)
        if not as_json:
            print(f"baseline written: {write_baseline} ({len(gate_findings)} finding(s))")
        if baseline is None:
            if as_json:
                print(json.dumps(report, indent=2))
            return 0

    if baseline is not None:
        try:
            baseline_keys = read_baseline(baseline)
        except ValueError as exc:
            print(f"wirecheck: {exc}", file=sys.stderr)
            return 2
        gate_keys = {key for key, _note in gate_findings}
        new = sorted(key for key, _note in gate_findings if key not in baseline_keys)
        resolved = sorted(baseline_keys - gate_keys)
        report["ratchet"] = {
            "baseline": baseline,
            "baseline_findings": sorted(baseline_keys),
            "new": new,
            "resolved": resolved,
        }
        if new:
            if not as_json:
                print(f"== RATCHET — fail only on NEW findings (baseline: {baseline}) ==")
                print(f"   baseline accepts: {len(baseline_keys)} finding(s)")
                print(f"   current findings: {len(gate_findings)}")
                print("   !! RATCHET VIOLATION — NEW finding(s) not in the baseline:")
                for key in new:
                    print(f"      {key}")
                print("RESULT: FAIL (ratchet violations present)")
            if as_json:
                print(json.dumps(report, indent=2))
            return 1
        if not as_json:
            print(f"== RATCHET — fail only on NEW findings (baseline: {baseline}) ==")
            print(f"   baseline accepts: {len(baseline_keys)} finding(s)")
            print(f"   current findings: {len(gate_findings)}")
            if resolved:
                print(
                    f"   note: {len(resolved)} finding(s) in the baseline no longer occur —"
                    " the ratchet can be tightened:"
                )
                for key in resolved:
                    print(f"      resolved: {key}")
                print(
                    "   tighten with: python3 scripts/audit/wirecheck.py <repo>"
                    f" --write-baseline {baseline}"
                )
            print("RESULT: PASS")
        if as_json:
            print(json.dumps(report, indent=2))
        return 0

    fail = False
    if "table-pair" in selected:
        fail = fail or bool(report.get("table_pair", {}).get("orphan_reads"))
    if "stub" in selected:
        fail = fail or bool(report.get("stubs"))
    if as_json:
        print(json.dumps(report, indent=2))
    if not as_json:
        print("RESULT:", "FAIL (wiring gaps present)" if fail else "PASS")
    return 1 if fail else 0


if __name__ == "__main__":
    sys.exit(main())
