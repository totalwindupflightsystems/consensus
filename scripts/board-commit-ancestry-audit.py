#!/usr/bin/env python3
"""Board commit-ancestry audit (INT-BOARD-1).

ch:trace row=INT-BOARD-1 spec=.coding-hermes/board/tasks.jsonl#acceptance wave=.coding-hermes/waves/consensus-foreman-2026-10-27-12-38-19.json#INT-BOARD-1 test=scripts/board-commit-ancestry-audit.py doc=scripts/board-commit-ancestry-audit.py prompt=/tmp/consensus-foreman-2026-10-27-12-38-19-INT-BOARD-1-brief.txt evidence=docs/evidence/INT-BOARD-1/ witness=live:git-object-store-queries verdict=none:foreman-will-run-after-merge commit=none:self-referential-single-commit memory=/project/consensus/foreman/INT-BOARD-1

Classifies every CLOSED row in .coding-hermes/board/tasks.jsonl that carries a
present commit_hash into exactly one of:

  landed              the hash resolves to a commit object AND is an ancestor
                      of the delivery ref (origin/main)
  landed-but-unmerged the hash resolves to a commit object but is NOT an
                      ancestor of the delivery ref — the claimed work is not
                      on the delivery branch
  missing-object      the hash is SHA-shaped but no commit object with that id
                      exists in the repository (after recovery searches across
                      every ref and the reflog)
  non-sha-field       the commit_hash field is present but is not a bare SHA
                      (prose, "a + b" multi-commit strings, "HEAD", branch
                      names, ...) — never treated as a valid SHA

Closed means status in {complete, done, closed}. Rows with a null/absent
commit_hash are counted (closed_rows_without_commit_hash) but not classified —
no evidence is invented for them.

Determinism: output contains no timestamps and sorts rows by their board line
number, so two runs over the same board and refs produce byte-identical
output, including from a clean clone (object existence and ancestry are
properties of the fetched object database, not of local state).

Usage:
  python3 scripts/board-commit-ancestry-audit.py [--board PATH] [--repo PATH]
      [--ref REF] [--json]

Exit codes: 0 the audit ran (anomalies are its OUTPUT, not failures);
2 usage/IO error (board missing, unreadable, or git unusable).
"""
import argparse
import json
import os
import re
import subprocess
import sys

SHA_RE = re.compile(r"^[0-9a-f]{7,40}$")
CLOSED_STATUSES = {"complete", "done", "closed"}
CLASSES = ("landed", "landed-but-unmerged", "missing-object", "non-sha-field")


def git(repo, args):
    """Run one git command; return (returncode, stdout, stderr)."""
    return subprocess.run(
        ["git", "-C", repo] + args, capture_output=True, text=True
    )


def delivery_ref(repo, requested):
    """Resolve the delivery ref, falling back origin/main -> main -> HEAD.

    Returns (name, commit_sha_or_None). A clean clone always has origin/main;
    the fallbacks keep the tool usable in a bare checkout.
    """
    candidates = [requested] if requested else ["origin/main", "main", "HEAD"]
    for name in candidates:
        proc = git(repo, ["rev-parse", "--verify", "--quiet", name + "^{commit}"])
        if proc.returncode == 0:
            return name, proc.stdout.strip()
    return (requested or "origin/main"), None


def read_board(path):
    """Parse the JSONL board line by line.

    Returns (rows, torn) where rows is a list of (line_number, dict) and torn
    lists the line numbers that failed to parse. Byte-exactness of untouched
    lines is the writer's concern; the auditor only reads.
    """
    rows, torn = [], []
    with open(path, "r", encoding="utf-8") as f:
        for lineno, line in enumerate(f, 1):
            s = line.strip()
            if not s:
                continue
            try:
                row = json.loads(s)
            except Exception:
                torn.append(lineno)
                continue
            if not isinstance(row, dict):
                torn.append(lineno)
                continue
            rows.append((lineno, row))
    return rows, torn


def classify(repo, ref_commit, raw_hash):
    """Classify one present commit_hash field. Never fabricates a hash."""
    h = raw_hash.strip() if isinstance(raw_hash, str) else ""
    if not h or not SHA_RE.match(h):
        return "non-sha-field", "not a bare SHA-1 (prose, multi-commit, ref name or symbolic)"
    proc = git(repo, ["cat-file", "-e", h + "^{commit}"])
    if proc.returncode != 0:
        # Object existence is a property of the fetched object database, not
        # of local refs — no search over refs or the reflog can add an object
        # that is not in the store, so this verdict is stable on a clean clone
        # that has fetched the same remote.
        return "missing-object", "no commit object with this id in the repository"
    proc = git(repo, ["merge-base", "--is-ancestor", h, ref_commit])
    if proc.returncode == 0:
        return "landed", "ancestor of delivery ref"
    return "landed-but-unmerged", "commit exists but is not an ancestor of the delivery ref"


def audit(board_path, repo, requested_ref):
    rows, torn = read_board(board_path)
    ref_name, ref_commit = delivery_ref(repo, requested_ref)
    if ref_commit is None:
        raise ValueError(
            "board-commit-ancestry-audit: no delivery ref resolved (tried %s)"
            % (requested_ref or "origin/main, main, HEAD")
        )

    result = {
        "board": os.path.relpath(board_path, repo),
        "repo": os.path.abspath(repo),
        "ref": ref_name,
        "ref_commit": ref_commit,
        "jsonl_rows": len(rows),
        "torn_lines": torn,
        "closed_rows": 0,
        "closed_rows_with_commit_hash": 0,
        "closed_rows_without_commit_hash": 0,
        "counts": {c: 0 for c in CLASSES},
        "rows": {c: [] for c in CLASSES},
    }

    for lineno, row in rows:
        if row.get("status") not in CLOSED_STATUSES:
            continue
        result["closed_rows"] += 1
        h = row.get("commit_hash")
        if h is None or (isinstance(h, str) and not h.strip()):
            result["closed_rows_without_commit_hash"] += 1
            continue
        result["closed_rows_with_commit_hash"] += 1
        cls, detail = classify(repo, ref_commit, h)
        result["counts"][cls] += 1
        entry = {"id": row.get("id"), "line": lineno, "commit_hash": h, "detail": detail}
        result["rows"][cls].append(entry)

    return result


def render_text(result):
    lines = []
    lines.append("board-commit-ancestry-audit")
    lines.append("board: %s" % result["board"])
    lines.append("delivery ref: %s (%s)" % (result["ref"], result["ref_commit"]))
    lines.append(
        "closed rows: %d (with commit_hash: %d, without: %d)"
        % (
            result["closed_rows"],
            result["closed_rows_with_commit_hash"],
            result["closed_rows_without_commit_hash"],
        )
    )
    if result["torn_lines"]:
        lines.append("torn JSONL lines: %s" % result["torn_lines"])
    for cls in CLASSES:
        entries = result["rows"][cls]
        lines.append("")
        lines.append("%s (%d)" % (cls, len(entries)))
        if not entries:
            lines.append("  (none)")
        for e in entries:
            lines.append("  line %-4d %-22s %s" % (e["line"], e["id"], e["commit_hash"]))
    return "\n".join(lines)


def main(argv=None):
    ap = argparse.ArgumentParser(
        description="Classify closed board rows by commit ancestry (landed / "
        "landed-but-unmerged / missing-object / non-sha-field)."
    )
    ap.add_argument("--board", default=None, help="path to tasks.jsonl (default: <repo>/.coding-hermes/board/tasks.jsonl)")
    ap.add_argument("--repo", default=None, help="repository root (default: git toplevel of the board or CWD)")
    ap.add_argument("--ref", default=None, help="delivery ref (default: origin/main, then main, then HEAD)")
    ap.add_argument("--json", action="store_true", help="machine-readable JSON output (stable key order)")
    args = ap.parse_args(argv)

    repo = args.repo
    if not repo:
        # Probe from the board's own directory (or the CWD when --board is
        # used with the default layout) so a linked worktree resolves to the
        # worktree toplevel — walking up from a parent dir can cross into an
        # outer repository and silently audit the wrong tree.
        probe = os.path.dirname(os.path.abspath(args.board)) if args.board else os.getcwd()
        proc = git(probe, ["rev-parse", "--show-toplevel"])
        if proc.returncode != 0:
            print(
                "board-commit-ancestry-audit: cannot locate the repository from %s "
                "(pass --repo)" % probe,
                file=sys.stderr,
            )
            return 2
        repo = proc.stdout.strip()
    board = args.board or os.path.join(repo, ".coding-hermes", "board", "tasks.jsonl")
    if not os.path.isfile(board):
        print("board-commit-ancestry-audit: board not found: %s" % board, file=sys.stderr)
        return 2

    try:
        result = audit(board, repo, args.ref)
    except ValueError as exc:
        print(str(exc), file=sys.stderr)
        return 2
    if args.json:
        print(json.dumps(result, indent=2, sort_keys=True))
    else:
        print(render_text(result))
    return 0


if __name__ == "__main__":
    sys.exit(main())
