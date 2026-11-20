#!/usr/bin/env python3
"""Unit tests for scripts/audit/wirecheck.py (WIRE-CONSENSUS-3).

Run: python3 scripts/audit/test_wirecheck.py

Stdlib only (the repository carries no pytest suite). Every case builds a
throwaway repository tree and drives the REAL command line, so the thing under
test is the exit code CI gates on — not a helper re-implementation of it.

Coverage:
  * an orphan read (a table some production file reads, nobody writes) fails
    the run and names both the table and its reader;
  * giving that same table a production writer turns the run green (the check
    measures pairing, not the existence of a read);
  * --only=table-pair makes the exit code depend on the table-pair section
    alone, so the pre-existing STUB debt cannot mask a regression either way;
  * an unknown section name is a usage error (rc 2), never a silent pass;
  * on THIS tree, memory_pages / projects / routing_rules are not reported as
    orphan reads — the regression guard for the DF-CONSENSUS-67 writers.
    Deleting a writer (e.g. internal/api/registries.go) without replacing it
    puts the table back in this test's failure message.
"""
import os
import subprocess
import sys
import tempfile
import textwrap
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SCRIPT = os.path.join(HERE, "wirecheck.py")
REPO_ROOT = os.path.dirname(os.path.dirname(HERE))

# The three tables WIRE-CONSENSUS-3 closed. Each one must keep a production
# writer; the row is only satisfied while none of them is an orphan read.
RESOLVED_TABLES = ("memory_pages", "projects", "routing_rules")


def run_wirecheck(root, *extra):
    """Run the real script and return (exit_code, combined_output)."""
    proc = subprocess.run(
        [sys.executable, SCRIPT, root] + list(extra),
        capture_output=True,
        text=True,
    )
    return proc.returncode, proc.stdout + proc.stderr


def write(path, content):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(textwrap.dedent(content).lstrip("\n"))


def build_fixture_repo(root, with_writer):
    """A minimal repo whose only table is read by production code."""
    write(
        os.path.join(root, "internal", "migrate", "migrations", "001_widgets.sql"),
        """
        CREATE TABLE widgets (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL
        );
        """,
    )
    write(
        os.path.join(root, "internal", "store", "read.go"),
        """
        package store

        func widgetNames() string {
        \treturn `SELECT name FROM widgets ORDER BY name`
        }
        """,
    )
    if with_writer:
        write(
            os.path.join(root, "internal", "store", "write.go"),
            """
            package store

            func insertWidget(name string) string {
            \treturn `INSERT INTO widgets (name) VALUES ('` + name + `')`
            }
            """,
        )


def build_stub_fixture_repo(root):
    """A healthy table pair plus an unrelated dispatch-path STUB finding."""
    build_fixture_repo(root, with_writer=True)
    write(
        os.path.join(root, "internal", "store", "stub.go"),
        """
        package store

        func placeholderDispatch() string {
        \treturn "not implemented"
        }
        """,
    )


class WirecheckTablePairTest(unittest.TestCase):
    def test_orphan_read_fails_and_names_table_and_reader(self):
        with tempfile.TemporaryDirectory() as root:
            build_fixture_repo(root, with_writer=False)
            code, out = run_wirecheck(root, "--only=table-pair")
            self.assertEqual(code, 1, out)
            self.assertIn("!! widgets", out)
            self.assertIn("reader: " + os.path.join("internal", "store", "read.go"), out)
            self.assertIn("RESULT: FAIL", out)

    def test_production_writer_turns_the_pair_green(self):
        with tempfile.TemporaryDirectory() as root:
            build_fixture_repo(root, with_writer=True)
            code, out = run_wirecheck(root, "--only=table-pair")
            self.assertEqual(code, 0, out)
            self.assertNotIn("!! widgets", out)
            self.assertIn("RESULT: PASS", out)

    def test_only_selects_the_sections_that_judge_the_exit_code(self):
        with tempfile.TemporaryDirectory() as root:
            build_stub_fixture_repo(root)
            # Default run: the STUB finding fails it (unchanged behaviour).
            code_all, out_all = run_wirecheck(root)
            self.assertEqual(code_all, 1, out_all)
            self.assertIn("== 2 STUB", out_all)
            # table-pair only: the stub is not even inspected, so a healthy
            # table pair passes. The opposite direction (an orphan read under
            # --only=stub) must pass too, otherwise --only would be cosmetic.
            code_only, out_only = run_wirecheck(root, "--only=table-pair")
            self.assertEqual(code_only, 0, out_only)
            self.assertNotIn("== 2 STUB", out_only)

    def test_orphan_read_is_invisible_to_stub_only_runs(self):
        with tempfile.TemporaryDirectory() as root:
            build_fixture_repo(root, with_writer=False)
            code, out = run_wirecheck(root, "--only=stub")
            self.assertEqual(code, 0, out)
            self.assertNotIn("!! widgets", out)

    def test_unknown_section_is_a_usage_error(self):
        with tempfile.TemporaryDirectory() as root:
            build_fixture_repo(root, with_writer=True)
            code, out = run_wirecheck(root, "--only=tablepair")
            self.assertEqual(code, 2, out)
            self.assertIn("unknown section name", out)

    def test_missing_root_is_a_usage_error(self):
        code, out = run_wirecheck("--json")
        self.assertEqual(code, 2, out)


class ResolvedTablesRegressionTest(unittest.TestCase):
    """The three WIRE-CONSENSUS-3 tables must never become orphan reads again."""

    def test_resolved_tables_have_production_writers_on_this_tree(self):
        code, out = run_wirecheck(REPO_ROOT, "--only=table-pair")
        self.assertEqual(code, 0, out)
        offender_lines = [ln for ln in out.splitlines() if ln.strip().startswith("!!")]
        for table in RESOLVED_TABLES:
            self.assertFalse(
                any(ln.startswith("   !! " + table + " ") for ln in offender_lines),
                "table %r is an orphan read again:\n%s" % (table, out),
            )


if __name__ == "__main__":
    unittest.main(verbosity=2)
