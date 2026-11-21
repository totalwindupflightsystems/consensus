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


def build_refusal_fixture_repo(root):
    """The two shapes WIRE-CONSENSUS-2 established as legal, and nothing else.

    (1) a marker handed to an explicit failure writer — the caller receives 501;
    (2) a marker that is provably not a dispatch return — the served UI's HTML
        placeholder attribute, allowlisted by exact file + exact string.
    """
    build_fixture_repo(root, with_writer=True)
    write(
        os.path.join(root, "internal", "shim", "opencode", "server.go"),
        """
        package opencode

        import (
        \t"encoding/json"
        \t"net/http"
        )

        func writeNotImplemented(w http.ResponseWriter, operation string) {
        \tw.WriteHeader(http.StatusNotImplemented)
        \tdata, _ := json.Marshal(map[string]string{"error": "not_implemented", "operation": operation})
        \t_, _ = w.Write(data)
        }

        func handlePty(w http.ResponseWriter, r *http.Request) {
        \twriteNotImplemented(w, "pty.create is not implemented")
        \t_, _ = w, r
        }
        """,
    )
    write(
        os.path.join(root, "internal", "web", "server.go"),
        """
        package web

        const sessionsPage = `<input id="ca" placeholder="researcher"/>`
        """,
    )


def build_allowlist_scope_fixture_repo(root):
    """The allowlisted STRING in a file the allowlist does not name.

    Proves the exemption is scoped to (file, string): moving the same HTML
    attribute into a sibling file must fail the run, otherwise the allowlist
    would be a blanket word-ban rather than a narrow, auditable exemption.
    """
    build_fixture_repo(root, with_writer=True)
    write(
        os.path.join(root, "internal", "web", "other.go"),
        """
        package web

        const memoryPage = `<input id="m" placeholder="search memory"/>`
        """,
    )


def build_unclassified_text_fixture_repo(root):
    """A marker in plain text nobody classifies — neither refusal nor return.

    The section must not pass it: an unclassifiable marker is a finding until a
    human proves what it is (refusal) or allowlists it with a reason.
    """
    build_fixture_repo(root, with_writer=True)
    write(
        os.path.join(root, "internal", "store", "codes.go"),
        """
        package store

        const envelopeCode = "not_implemented"
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


class WirecheckStubSectionTest(unittest.TestCase):
    """WIRE-CONSENSUS-2: the law is 'never a success for work not done'.

    The STUB section must pass a marker that hands the caller an error status
    (a refusal) and fail one that can reach a caller as a return, while the
    allowlist stays narrow enough that the same string in another file still
    fails.
    """

    def test_refusal_and_allowlisted_marker_pass(self):
        with tempfile.TemporaryDirectory() as root:
            build_refusal_fixture_repo(root)
            code, out = run_wirecheck(root, "--only=stub")
            self.assertEqual(code, 0, out)
            self.assertIn("refusals: 2", out)
            self.assertIn("allowlisted: 1", out)
            self.assertIn(os.path.join("internal", "web", "server.go"), out)
            self.assertIn("RESULT: PASS", out)

    def test_returned_marker_fails(self):
        with tempfile.TemporaryDirectory() as root:
            build_stub_fixture_repo(root)
            code, out = run_wirecheck(root, "--only=stub")
            self.assertEqual(code, 1, out)
            self.assertIn("!! placeholderDispatch", out)
            self.assertIn("RESULT: FAIL", out)

    def test_unclassified_marker_text_fails(self):
        with tempfile.TemporaryDirectory() as root:
            build_unclassified_text_fixture_repo(root)
            code, out = run_wirecheck(root, "--only=stub")
            self.assertEqual(code, 1, out)
            self.assertIn('"not_implemented"', out)
            self.assertIn("RESULT: FAIL", out)

    def test_allowlist_is_scoped_to_the_named_file(self):
        # The exact allowlisted string, one file over: still a finding. This is
        # what stops the allowlist from decaying into a blanket word-ban.
        with tempfile.TemporaryDirectory() as root:
            build_allowlist_scope_fixture_repo(root)
            code, out = run_wirecheck(root, "--only=stub")
            self.assertEqual(code, 1, out)
            self.assertIn(os.path.join("internal", "web", "other.go"), out)
            self.assertIn("RESULT: FAIL", out)

    def test_this_tree_has_no_returned_stub_marker(self):
        # Regression guard on the real repository: every surviving marker is a
        # refusal or a justified allowlist entry. A new handler that returns a
        # marker string puts its file and line in this failure message.
        code, out = run_wirecheck(REPO_ROOT, "--only=stub")
        self.assertEqual(code, 0, out)
        self.assertNotIn("!!", out)


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
