#!/usr/bin/env python3
"""Unit tests for scripts/board-commit-ancestry-audit.py (INT-BOARD-1).

Run: python3 scripts/test_board_commit_ancestry_audit.py
Stdlib only (the repository carries no pytest suite); each test builds a
throwaway git repository so object existence and ancestry are exercised for
real, never asserted from fixtures alone.
"""
import importlib.util
import io
import json
import os
import contextlib
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location(
    "board_commit_ancestry_audit",
    os.path.join(HERE, "board-commit-ancestry-audit.py"),
)
bcaa = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(bcaa)


def git(repo, *args, check=True):
    proc = subprocess.run(
        ["git", "-C", repo] + list(args), capture_output=True, text=True
    )
    if check and proc.returncode != 0:
        raise AssertionError("git %s failed: %s" % (args, proc.stderr))
    return proc


def commit_file(repo, relpath, content, message):
    path = os.path.join(repo, relpath)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)
    git(repo, "add", relpath)
    git(repo, "commit", "-q", "-m", message)
    return git(repo, "rev-parse", "HEAD").stdout.strip()


def write_board(repo, rows):
    """rows: list of dicts (or raw strings for torn lines)."""
    lines = []
    for r in rows:
        lines.append(r if isinstance(r, str) else json.dumps(r))
    board = os.path.join(repo, ".coding-hermes", "board", "tasks.jsonl")
    with open(board, "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")
    return board


def run_audit(board, repo, extra=None):
    out, err = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
        rc = bcaa.main(["--board", board, "--repo", repo] + (extra or []))
    return rc, out.getvalue(), err.getvalue()


class AuditorTests(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        repo = os.path.join(tmp.name, "repo")
        os.makedirs(os.path.join(repo, ".coding-hermes", "board"))
        git(repo, "init", "-q", "-b", "main")
        git(repo, "config", "user.email", "audit@example.invalid")
        git(repo, "config", "user.name", "audit")
        self.landed_sha = commit_file(repo, "f.txt", "one\n", "base commit")
        # A commit on a side branch that main does not contain.
        git(repo, "checkout", "-q", "-b", "side")
        self.unmerged_sha = commit_file(repo, "g.txt", "two\n", "side commit")
        git(repo, "checkout", "-q", "main")
        self.repo = repo

    def test_four_classes_and_open_rows_excluded(self):
        board = write_board(self.repo, [
            {"id": "R-LANDED", "status": "complete", "commit_hash": self.landed_sha[:7]},
            {"id": "R-UNMERGED", "status": "done", "commit_hash": self.unmerged_sha},
            {"id": "R-MISSING", "status": "closed", "commit_hash": "d" * 40},
            {"id": "R-PROSE", "status": "complete", "commit_hash": "abc1234 + def5678"},
            {"id": "R-NOHASH", "status": "complete", "commit_hash": None},
            {"id": "R-OPEN", "status": "pending", "commit_hash": self.unmerged_sha},
        ])
        rc, out, err = run_audit(board, self.repo, ["--ref", "main"])
        self.assertEqual(rc, 0, err)
        self.assertIn("landed (1)", out)
        self.assertIn("landed-but-unmerged (1)", out)
        self.assertIn("missing-object (1)", out)
        self.assertIn("non-sha-field (1)", out)
        self.assertIn("R-LANDED", out)
        self.assertIn("R-UNMERGED", out)
        self.assertIn("R-MISSING", out)
        self.assertIn("R-PROSE", out)
        # The pending row is not classified anywhere, and the hash-less closed
        # row is counted, not classified.
        self.assertNotIn("R-OPEN", out)
        self.assertNotIn("R-NOHASH", out)
        self.assertIn("closed rows: 5 (with commit_hash: 4, without: 1)", out)

    def test_prose_and_symbolic_fields_are_never_shas(self):
        board = write_board(self.repo, [
            {"id": "P1", "status": "complete", "commit_hash": "5d48ab5 + 1223ff4"},
            {"id": "P2", "status": "complete", "commit_hash": "HEAD"},
            {"id": "P3", "status": "complete", "commit_hash": "origin/master"},
            {"id": "P4", "status": "complete", "commit_hash": "FOREMAN-DIRECT"},
            {"id": "P5", "status": "complete", "commit_hash": "abc123"},  # 6 hex: too short
            {"id": "P6", "status": "complete", "commit_hash": 12345},     # not a string
            {"id": "P7", "status": "complete", "commit_hash": self.landed_sha[:7] + " (merged deadbee)"},
        ])
        rc, out, err = run_audit(board, self.repo, ["--ref", "main"])
        self.assertEqual(rc, 0, err)
        self.assertIn("non-sha-field (7)", out)
        self.assertIn("missing-object (0)", out)
        self.assertIn("landed (0)", out)

    def test_missing_object_reported_without_fabrication(self):
        ghost = "e" * 40
        board = write_board(self.repo, [
            {"id": "R-GHOST", "status": "complete", "commit_hash": ghost},
        ])
        rc, out, err = run_audit(board, self.repo, ["--ref", "main"])
        self.assertEqual(rc, 0, err)
        self.assertIn("missing-object (1)", out)
        # The input hash is echoed verbatim; nothing else takes its place.
        self.assertIn(ghost, out)

    def test_json_output_is_deterministic(self):
        write_board(self.repo, [
            {"id": "R-A", "status": "complete", "commit_hash": self.landed_sha[:7]},
            {"id": "R-B", "status": "complete", "commit_hash": "zz prose"},
        ])
        board = os.path.join(self.repo, ".coding-hermes", "board", "tasks.jsonl")
        rc1, out1, _ = run_audit(board, self.repo, ["--ref", "main", "--json"])
        rc2, out2, _ = run_audit(board, self.repo, ["--ref", "main", "--json"])
        self.assertEqual(rc1, 0)
        self.assertEqual(out1, out2, "two runs over the same board must be byte-identical")
        doc = json.loads(out1)
        self.assertEqual(doc["counts"], {
            "landed": 1, "landed-but-unmerged": 0, "missing-object": 0, "non-sha-field": 1,
        })
        self.assertEqual([e["id"] for e in doc["rows"]["landed"]], ["R-A"])

    def test_torn_lines_listed_and_valid_rows_still_classified(self):
        board = write_board(self.repo, [
            {"id": "R-GOOD", "status": "complete", "commit_hash": self.landed_sha[:7]},
            '{"id": "R-TORN", "status": "complete", "commit_hash": ',  # torn JSON
        ])
        rc, out, err = run_audit(board, self.repo, ["--ref", "main"])
        self.assertEqual(rc, 0, err)
        self.assertIn("R-GOOD", out)
        self.assertNotIn("R-TORN", out)
        doc = json.loads(run_audit(board, self.repo, ["--ref", "main", "--json"])[1])
        self.assertEqual(doc["torn_lines"], [2])  # the torn line is line 2 (1-based)

    def test_missing_board_file_exits_2(self):
        rc, out, err = run_audit(
            os.path.join(self.repo, "no", "such", "tasks.jsonl"), self.repo
        )
        self.assertEqual(rc, 2)
        self.assertIn("board not found", err)

    def test_no_delivery_ref_exits_2(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        empty = os.path.join(tmp.name, "empty")
        os.makedirs(os.path.join(empty, ".coding-hermes", "board"))
        git(empty, "init", "-q", "-b", "main")
        board = write_board(empty, [{"id": "R", "status": "complete", "commit_hash": "abc1234"}])
        rc, out, err = run_audit(board, empty)
        self.assertEqual(rc, 2)
        self.assertIn("no delivery ref", err)

    def test_ref_override_reclassifies_ancestry(self):
        board = write_board(self.repo, [
            {"id": "R-SIDE", "status": "complete", "commit_hash": self.unmerged_sha[:7]},
        ])
        rc, out, _ = run_audit(board, self.repo, ["--ref", "side"])
        self.assertEqual(rc, 0)
        self.assertIn("landed (1)", out)
        rc, out, _ = run_audit(board, self.repo, ["--ref", "main"])
        self.assertIn("landed-but-unmerged (1)", out)


if __name__ == "__main__":
    unittest.main(verbosity=2)
