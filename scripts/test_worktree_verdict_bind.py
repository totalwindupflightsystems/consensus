#!/usr/bin/env python3
"""Unit tests for scripts/worktree-verdict-bind.py (INT-CI-CONSENSUS-002).

Run: python3 scripts/test_worktree_verdict_bind.py
Stdlib only (the repository carries no pytest suite). Each test builds a
throwaway git repository and a throwaway GitReins history layout so verdict
binding is exercised for real: exact task_id, graded commit ancestry and
byte-identical promotion into a canonical history store. The failure cases
are the red arms — each asserts the binder REFUSES and does not write.

Red/green proof: the red cases assert refusal classes (mismatched task_id,
mismatched commit, missing verdict.json, a printed verdict id without a
history directory, incomplete/ungraded verdicts, failed verdicts, refused
overwrites); the green case proves one successful, byte-identical promotion
plus post-copy verification.
"""
import hashlib
import importlib.util
import io
import json
import os
import contextlib
import subprocess
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location(
    "worktree_verdict_bind",
    os.path.join(HERE, "worktree-verdict-bind.py"),
)
wvb = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(wvb)

TASK_ID = "SCN-CONSENSUS-010"


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


def verdict_dir(tmp, run_id, task_id, commit, passed=True, items=None,
                ungraded=None, evaluated_at="2026-11-28T05:00:00", files=None,
                evidence_items=None):
    """Build a run directory shaped like a real GitReins history run."""
    run = os.path.join(tmp, "history", "2026-11-28", run_id)
    os.makedirs(run, exist_ok=True)
    if items is None:
        items = [{"criterion": "c1", "status": "PASS", "detail": "ok"}]
    v = {
        "task_id": task_id,
        "passed": passed,
        "worktree": tmp,
        "branch": "wt/x",
        "commit": commit,
        "pass_class": "judged" if passed else "judged",
        "tier2_skipped": False,
        "items": items,
        "evaluated_at": evaluated_at,
        "evidence": {
            "collected_at": evaluated_at,
            "task_id": task_id,
            "items": evidence_items if evidence_items is not None else [
                {"name": "patch", "label": "Landed commit patch",
                 "file": "commit.patch", "bytes": 0, "source": "git show x"},
            ],
        },
    }
    if ungraded is not None:
        v["ungraded_criteria"] = ungraded
    # The evidence byte count must match the artifact on disk (the binder
    # checks recorded vs actual size) — patch before the dump, not after.
    patch_content = "patch\n"
    for entry in v["evidence"]["items"]:
        if entry.get("file") == "commit.patch":
            entry["bytes"] = len(patch_content)
    with open(os.path.join(run, "verdict.json"), "w", encoding="utf-8") as f:
        json.dump(v, f, indent=2)
    with open(os.path.join(run, "commit.patch"), "w", encoding="utf-8") as f:
        f.write(patch_content)
    for name, content in (files or {}).items():
        with open(os.path.join(run, name), "w", encoding="utf-8") as f:
            f.write(content)
    return run


def run_main(argv):
    out, err = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
        rc = wvb.main(argv)
    return rc, out.getvalue(), err.getvalue()


def sha256(path):
    with open(path, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()


class BinderTests(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = tmp.name
        self.repo = os.path.join(self.root, "repo")
        os.makedirs(self.repo)
        git(self.repo, "init", "-q", "-b", "main")
        git(self.repo, "config", "user.email", "b@example.invalid")
        git(self.repo, "config", "user.name", "binder")
        self.landed_sha = commit_file(self.repo, "f.txt", "one\n", "base commit")
        # A side commit on an unmerged branch — exists as an object but is
        # not an ancestor of main.
        git(self.repo, "checkout", "-q", "-b", "side")
        self.unmerged_sha = commit_file(self.repo, "g.txt", "two\n", "side commit")
        git(self.repo, "checkout", "-q", "main")

    # ------------------------------------------------------------- green

    def test_verify_accepts_valid_verdict(self):
        run = verdict_dir(self.root, "8ba94a92", TASK_ID, self.landed_sha)
        rc, out, err = run_main([
            "verify", "--verdict-dir", run, "--task-id", TASK_ID,
            "--repo", self.repo,
        ])
        self.assertEqual(rc, 0, err)
        self.assertIn("task_id:", out)
        self.assertIn(TASK_ID, out)
        self.assertIn("passed:       True", out)

    def test_promote_copies_byte_identical_and_verifies(self):
        run = verdict_dir(self.root, "8ba94a92", TASK_ID, self.landed_sha)
        canon = os.path.join(self.root, "canonical")
        os.makedirs(canon)
        rc, out, err = run_main([
            "promote", "--source", run, "--run-id", "8ba94a92",
            "--task-id", TASK_ID, "--expect-commit", self.landed_sha,
            "--dest", canon, "--repo", self.repo,
        ])
        self.assertEqual(rc, 0, err)
        self.assertIn("evidence-run-id: 8ba94a92", out)
        dest = os.path.join(canon, ".gitreins", "history", "2026-11-28", "8ba94a92")
        self.assertTrue(os.path.isfile(os.path.join(dest, "verdict.json")))
        self.assertEqual(
            sha256(os.path.join(dest, "verdict.json")),
            sha256(os.path.join(run, "verdict.json")),
        )
        # The promoted copy verifies against the canonical repo.
        rc2, out2, err2 = run_main([
            "verify", "--verdict-dir", dest, "--task-id", TASK_ID,
            "--repo", self.repo,
        ])
        self.assertEqual(rc2, 0, err2)

    def test_verify_ancestor_check_passes_and_fails(self):
        run = verdict_dir(self.root, "8ba94a92", TASK_ID, self.landed_sha)
        rc, _, err = run_main([
            "verify", "--verdict-dir", run, "--task-id", TASK_ID,
            "--repo", self.repo, "--expect-ancestor-of", "main",
        ])
        self.assertEqual(rc, 0, err)
        run2 = verdict_dir(self.root, "aaaa1111", TASK_ID, self.unmerged_sha)
        rc2, out2, err2 = run_main([
            "verify", "--verdict-dir", run2, "--task-id", TASK_ID,
            "--repo", self.repo, "--expect-ancestor-of", "main",
        ])
        self.assertEqual(rc2, 1)
        self.assertIn("not an ancestor", err2)

    # --------------------------------------------------------------- red

    def test_verify_rejects_task_id_mismatch(self):
        run = verdict_dir(self.root, "8ba94a92", "DF-CONSENSUS-081", self.landed_sha)
        rc, out, err = run_main([
            "verify", "--verdict-dir", run, "--task-id", TASK_ID,
            "--repo", self.repo,
        ])
        self.assertEqual(rc, 1)
        self.assertIn("task_id", err)
        self.assertIn("DF-CONSENSUS-081", err)
        self.assertIn(TASK_ID, err)

    def test_verify_rejects_commit_mismatch(self):
        run = verdict_dir(self.root, "8ba94a92", TASK_ID, self.unmerged_sha)
        rc, out, err = run_main([
            "verify", "--verdict-dir", run, "--task-id", TASK_ID,
            "--expect-commit", self.landed_sha, "--repo", self.repo,
        ])
        self.assertEqual(rc, 1)
        self.assertIn("commit", err)

    def test_verify_rejects_missing_verdict_dir(self):
        missing = os.path.join(self.root, "history", "2026-11-28", "deadbeef")
        rc, out, err = run_main([
            "verify", "--verdict-dir", missing, "--task-id", TASK_ID,
            "--repo", self.repo,
        ])
        self.assertEqual(rc, 1)
        self.assertIn("no history directory", err)

    def test_verify_rejects_missing_verdict_json(self):
        run = verdict_dir(self.root, "8ba94a92", TASK_ID, self.landed_sha)
        os.remove(os.path.join(run, "verdict.json"))
        rc, out, err = run_main([
            "verify", "--verdict-dir", run, "--task-id", TASK_ID,
            "--repo", self.repo,
        ])
        self.assertEqual(rc, 1)
        self.assertIn("verdict.json", err)

    def test_verify_rejects_incomplete_verdict(self):
        run = verdict_dir(
            self.root, "8ba94a92", TASK_ID, self.landed_sha,
            passed=False, items=[],
            ungraded=["criterion never graded"],
        )
        rc, out, err = run_main([
            "verify", "--verdict-dir", run, "--task-id", TASK_ID,
            "--repo", self.repo,
        ])
        self.assertEqual(rc, 1)
        self.assertIn("ungraded", err)

    def test_verify_rejects_failed_verdict_by_default(self):
        run = verdict_dir(
            self.root, "8ba94a92", TASK_ID, self.landed_sha,
            passed=False,
            items=[{"criterion": "c1", "status": "FAIL", "detail": "no"}],
        )
        rc, out, err = run_main([
            "verify", "--verdict-dir", run, "--task-id", TASK_ID,
            "--repo", self.repo,
        ])
        self.assertEqual(rc, 1)
        self.assertIn("passed", err)

    def test_verify_rejects_tier2_skipped(self):
        run = verdict_dir(self.root, "8ba94a92", TASK_ID, self.landed_sha)
        with open(os.path.join(run, "verdict.json"), "r+", encoding="utf-8") as f:
            v = json.load(f)
            v["tier2_skipped"] = True
            f.seek(0)
            json.dump(v, f, indent=2)
            f.truncate()
        rc, out, err = run_main([
            "verify", "--verdict-dir", run, "--task-id", TASK_ID,
            "--repo", self.repo,
        ])
        self.assertEqual(rc, 1)
        self.assertIn("tier2", err)

    def test_verify_rejects_missing_evidence_artifact(self):
        run = verdict_dir(self.root, "8ba94a92", TASK_ID, self.landed_sha)
        os.remove(os.path.join(run, "commit.patch"))
        rc, out, err = run_main([
            "verify", "--verdict-dir", run, "--task-id", TASK_ID,
            "--repo", self.repo,
        ])
        self.assertEqual(rc, 1)
        self.assertIn("commit.patch", err)

    def test_promote_refuses_existing_target_without_clobbering(self):
        run = verdict_dir(self.root, "8ba94a92", TASK_ID, self.landed_sha)
        canon = os.path.join(self.root, "canonical")
        os.makedirs(canon)
        target = os.path.join(canon, ".gitreins", "history", "2026-11-28", "8ba94a92")
        os.makedirs(target)
        with open(os.path.join(target, "verdict.json"), "w", encoding="utf-8") as f:
            json.dump({"task_id": "EXISTING", "passed": False}, f)
        before = sha256(os.path.join(target, "verdict.json"))
        rc, out, err = run_main([
            "promote", "--source", run, "--run-id", "8ba94a92",
            "--task-id", TASK_ID, "--expect-commit", self.landed_sha,
            "--dest", canon, "--repo", self.repo,
        ])
        self.assertEqual(rc, 1)
        self.assertIn("already exists", err)
        self.assertEqual(sha256(os.path.join(target, "verdict.json")), before)

    def test_promote_rejects_run_id_without_history_directory(self):
        # A printed verdict id in place of a history directory is refused:
        # the source path does not exist, and nothing is fabricated.
        canon = os.path.join(self.root, "canonical")
        os.makedirs(canon)
        rc, out, err = run_main([
            "promote", "--source", os.path.join(self.root, "history", "nope"),
            "--run-id", "8ba94a92", "--task-id", TASK_ID,
            "--expect-commit", self.landed_sha, "--dest", canon,
            "--repo", self.repo,
        ])
        self.assertEqual(rc, 1)
        self.assertIn("no history directory", err)
        self.assertFalse(
            os.path.exists(
                os.path.join(canon, ".gitreins", "history", "2026-11-28", "8ba94a92")
            )
        )

    def test_promote_ambiguous_run_id_across_dates_is_refused(self):
        run = verdict_dir(self.root, "8ba94a92", TASK_ID, self.landed_sha)
        canon = os.path.join(self.root, "canonical")
        os.makedirs(canon)
        # A second history root containing the same run id under another date.
        other = os.path.join(self.root, "history", "2026-11-27", "8ba94a92")
        os.makedirs(other)
        rc, out, err = run_main([
            "promote", "--source", os.path.join(self.root, "history"),
            "--run-id", "8ba94a92", "--task-id", TASK_ID,
            "--expect-commit", self.landed_sha, "--dest", canon,
            "--repo", self.repo,
        ])
        self.assertEqual(rc, 2)
        self.assertIn("ambiguous", err)


if __name__ == "__main__":
    unittest.main()
