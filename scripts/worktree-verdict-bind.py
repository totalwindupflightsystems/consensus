#!/usr/bin/env python3
"""Bind a GitReins Tier-2 verdict produced inside a worker worktree to its
board row (INT-CI-CONSENSUS-002).

ch:trace row=INT-CI-CONSENSUS-002 spec=.coding-hermes/board/tasks.jsonl#INT-CI-CONSENSUS-002 test=scripts/test_worktree_verdict_bind.py doc=scripts/worktree-verdict-bind.py evidence=scripts/worktree-verdict-bind.py witness=none:worktree-verdict-binding-captured-in-test-output verdict=none:foreman-runs-after-merge commit=none:self-referential-single-commit memory=none:not-written

Two subcommands, one contract: **no fabrication, no clobbering, no binding by
printed verdict id.**

  verify   Validate one history run directory against its expected task id
           and graded commit. Checks, each with a named failure:
             - the directory exists and contains a parseable verdict.json
               (a printed verdict id without a history directory is refused);
             - verdict.task_id equals the expected task id EXACTLY;
             - verdict.commit is a full SHA that resolves to a commit object
               in --repo, matches --expect-commit when given, and is an
               ancestor of --expect-ancestor-of when given;
             - the verdict was actually graded: tier2_skipped is false,
               items is non-empty and ungraded_criteria is empty;
             - verdict.passed is true (unless --allow-failed, for
               evidence-only binding of judged-but-failed runs);
             - every evidence artifact named in verdict.evidence.items
               exists in the run directory with its recorded byte count.

  promote  Verify (all of the above, with --expect-commit required) then
           copy the run directory — verdict.json plus every evidence
           artifact it names — into the canonical checkout's
           .gitreins/history/<date>/<run-id>/ so boardctl's
           --evidence-run-id resolves there. The copy is byte-identical
           (sha256-asserted) and staged atomically; an existing target run
           directory is refused, never overwritten.

Usage:
  python3 scripts/worktree-verdict-bind.py verify \
      --verdict-dir <worktree>/.gitreins/history/<date>/<run-id> \
      --task-id <ROW-ID> [--expect-commit SHA] [--expect-ancestor-of REF]
      [--repo PATH] [--allow-failed] [--json]

  python3 scripts/worktree-verdict-bind.py promote \
      --source <worktree>/.gitreins/history[/...] --run-id <run-id> \
      --task-id <ROW-ID> --expect-commit SHA \
      --dest <canonical-repo-root> [--repo PATH] [--date YYYY-MM-DD]

Exit codes: 0 verified/promoted; 1 binding refused (the named check);
2 usage/IO error (ambiguous run id, unreadable paths).
"""
import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

RUN_ID_RE = re.compile(r"^[0-9a-f]{8}$")
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
DATE_RE = re.compile(r"^\d{4}-\d{2}-\d{2}$")


class BindError(Exception):
    """A binding check failed — carries the exact refusal reason."""


class BindUsageError(BindError):
    """Ambiguous or invalid invocation — exit 2, not a refused binding."""


def git(repo, args):
    return subprocess.run(["git", "-C", repo] + args, capture_output=True, text=True)


def resolve_commit(repo, value):
    """Resolve a SHA or ref to a full commit SHA; None when it will not resolve."""
    proc = git(repo, ["rev-parse", "--verify", "--quiet", value + "^{commit}"])
    if proc.returncode != 0:
        return None
    return proc.stdout.strip()


def check_verdict(run_dir, task_id, repo, expect_commit=None,
                  expect_ancestor_of=None, allow_failed=False):
    """Run every binding check; return the parsed verdict or raise BindError."""
    if not os.path.isdir(run_dir):
        raise BindError(
            "no history directory at %s (a printed verdict id is not evidence)"
            % run_dir
        )
    vpath = os.path.join(run_dir, "verdict.json")
    if not os.path.isfile(vpath):
        raise BindError(
            "verdict.json missing from history directory %s — the run never "
            "wrote a verdict, so there is nothing to bind" % run_dir
        )
    try:
        with open(vpath, "r", encoding="utf-8") as f:
            verdict = json.load(f)
    except (ValueError, OSError) as exc:
        raise BindError("verdict.json unreadable at %s: %s" % (vpath, exc))
    if not isinstance(verdict, dict):
        raise BindError("verdict.json at %s is not a JSON object" % vpath)

    vtask = verdict.get("task_id")
    if not isinstance(vtask, str) or vtask != task_id:
        raise BindError(
            "task_id mismatch: verdict carries %r, row is %r"
            % (vtask, task_id)
        )

    vcommit = verdict.get("commit")
    if not isinstance(vcommit, str) or not SHA_RE.match(vcommit):
        raise BindError(
            "verdict.commit is not a full 40-hex SHA: %r" % (vcommit,)
        )
    if not os.path.isdir(repo) or git(repo, ["rev-parse", "--is-inside-work-tree"]).returncode != 0:
        raise BindError("repository not usable at %s (pass --repo)" % repo)
    if resolve_commit(repo, vcommit) != vcommit:
        raise BindError(
            "graded commit %s does not resolve to a commit object in %s — "
            "fetch the worktree's objects before binding" % (vcommit, repo)
        )
    if expect_commit is not None:
        expected = resolve_commit(repo, expect_commit)
        if expected is None:
            raise BindError("--expect-commit %r resolves to no commit object" % expect_commit)
        if expected != vcommit:
            raise BindError(
                "graded commit mismatch: verdict graded %s, expected %s"
                % (vcommit, expected)
            )
    if expect_ancestor_of is not None:
        proc = git(repo, ["merge-base", "--is-ancestor", vcommit, expect_ancestor_of])
        if proc.returncode != 0:
            raise BindError(
                "graded commit %s is not an ancestor of %s"
                % (vcommit, expect_ancestor_of)
            )

    if verdict.get("tier2_skipped"):
        raise BindError(
            "verdict has tier2_skipped=true — the tier-2 judge never ran"
        )
    items = verdict.get("items")
    if not isinstance(items, list) or not items:
        raise BindError(
            "verdict is incomplete: items is empty and ungraded_criteria=%r — "
            "the judge produced no graded criteria" % (verdict.get("ungraded_criteria"),)
        )
    ungraded = verdict.get("ungraded_criteria")
    if ungraded:
        raise BindError(
            "verdict is incomplete: %d ungraded criteria remain (%r...)"
            % (len(ungraded), ungraded[0][:80])
        )
    if not verdict.get("passed") and not allow_failed:
        raise BindError(
            "verdict passed=false — refusing to bind a failed run as "
            "completion evidence (pass --allow-failed for evidence-only binding)"
        )

    evidence = verdict.get("evidence")
    if isinstance(evidence, dict):
        for item in evidence.get("items") or []:
            name = item.get("file")
            if not name:
                continue
            epath = os.path.join(run_dir, name)
            if not os.path.isfile(epath):
                raise BindError(
                    "evidence artifact %s named by verdict.evidence is missing "
                    "from the run directory" % name
                )
            recorded = item.get("bytes")
            if isinstance(recorded, int) and recorded >= 0:
                actual = os.path.getsize(epath)
                if actual != recorded:
                    raise BindError(
                        "evidence artifact %s byte count mismatch: verdict "
                        "records %d, run directory holds %d" % (name, recorded, actual)
                    )
    return verdict


def resolve_run_dir(source, run_id):
    """Locate <source>/.../<run-id>. Direct run-dir path or a history root scan."""
    if not os.path.isdir(source):
        raise BindError(
            "no history directory at %s for run-id %s (a printed verdict id is "
            "not evidence)" % (source, run_id)
        )
    if os.path.basename(os.path.normpath(source)) == run_id:
        return source
    if os.path.isdir(os.path.join(source, run_id)):
        return os.path.join(source, run_id)
    matches = []
    try:
        entries = sorted(os.listdir(source))
    except OSError as exc:
        raise BindError("cannot list history root %s: %s" % (source, exc))
    for date in entries:
        if DATE_RE.match(date):
            candidate = os.path.join(source, date, run_id)
            if os.path.isdir(candidate):
                matches.append(candidate)
    if not matches:
        raise BindError(
            "no history directory at %s for run-id %s (a printed verdict id is "
            "not evidence)" % (source, run_id)
        )
    if len(matches) > 1:
        raise BindUsageError(
            "ambiguous run-id %s under %s: found in %s — pass --source pointed "
            "at the exact run directory" % (run_id, source, ", ".join(matches))
        )
    return matches[0]


def copy_run_atomically(src_run, dest_history, date, run_id):
    """Stage-copy verdict.json + named evidence files; sha256-asserted."""
    target = os.path.join(dest_history, date, run_id)
    if os.path.exists(target):
        raise BindError("target run directory already exists: %s" % target)
    wanted = ["verdict.json"]
    try:
        with open(os.path.join(src_run, "verdict.json"), "r", encoding="utf-8") as f:
            verdict = json.load(f)
    except (ValueError, OSError) as exc:
        raise BindError("cannot re-read verdict.json before copy: %s" % exc)
    evidence = verdict.get("evidence")
    if isinstance(evidence, dict):
        for item in evidence.get("items") or []:
            name = item.get("file")
            if name and name not in wanted:
                wanted.append(name)
    for name in ("summary.md",):
        if os.path.isfile(os.path.join(src_run, name)) and name not in wanted:
            wanted.append(name)

    stage = tempfile.mkdtemp(prefix=".%s.stage-" % run_id, dir=dest_history)
    try:
        dest_run = os.path.join(stage, run_id)
        os.makedirs(dest_run)
        for name in wanted:
            spath = os.path.join(src_run, name)
            if not os.path.isfile(spath):
                raise BindError("artifact %s vanished before copy" % name)
            shutil.copy2(spath, os.path.join(dest_run, name))
        for name in wanted:
            s, d = os.path.join(src_run, name), os.path.join(dest_run, name)
            if sha256_of(s) != sha256_of(d):
                raise BindError("copy verification failed for %s" % name)
        os.makedirs(os.path.dirname(target), exist_ok=True)
        os.rename(dest_run, target)
    except BindError:
        shutil.rmtree(stage, ignore_errors=True)
        raise
    finally:
        shutil.rmtree(stage, ignore_errors=True)
    return target, wanted


def sha256_of(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def cmd_verify(args):
    repo = args.repo
    verdict = check_verdict(
        args.verdict_dir, args.task_id, repo,
        expect_commit=args.expect_commit,
        expect_ancestor_of=args.expect_ancestor_of,
        allow_failed=args.allow_failed,
    )
    if args.json:
        print(json.dumps({
            "status": "verified",
            "run_dir": os.path.abspath(args.verdict_dir),
            "task_id": args.task_id,
            "commit": verdict.get("commit"),
            "passed": verdict.get("passed"),
            "items": len(verdict.get("items") or []),
        }, indent=2, sort_keys=True))
    else:
        print("worktree-verdict-bind: VERIFIED")
        print("  run dir:      %s" % args.verdict_dir)
        print("  task_id:      %s" % args.task_id)
        print("  graded commit: %s" % verdict.get("commit"))
        print("  passed:       %s (%d criteria graded)"
              % (verdict.get("passed"), len(verdict.get("items") or [])))
    return 0


def cmd_promote(args):
    if not RUN_ID_RE.match(args.run_id):
        print("worktree-verdict-bind: --run-id must be an 8-hex history run id, got %r"
              % args.run_id, file=sys.stderr)
        return 2
    repo = args.repo
    src_run = resolve_run_dir(args.source, args.run_id)
    verdict = check_verdict(
        src_run, args.task_id, repo,
        expect_commit=args.expect_commit,
        expect_ancestor_of=args.expect_ancestor_of,
        allow_failed=args.allow_failed,
    )
    dest_history = os.path.join(args.dest, ".gitreins", "history")
    if not os.path.isdir(dest_history):
        os.makedirs(dest_history, exist_ok=True)
    date = args.date
    if date is None:
        evaluated = str(verdict.get("evaluated_at") or "")
        date = evaluated[:10]
    if not DATE_RE.match(date):
        raise BindError(
            "cannot derive the history date for the target (evaluated_at=%r, "
            "--date=%r)" % (verdict.get("evaluated_at"), args.date)
        )
    target, copied = copy_run_atomically(src_run, dest_history, date, args.run_id)
    print("worktree-verdict-bind: PROMOTED (byte-identical copy)")
    print("  source run dir: %s" % src_run)
    print("  destination:    %s" % target)
    print("  copied:         %s" % ", ".join(copied))
    print("  task_id:        %s" % args.task_id)
    print("  graded commit:  %s" % verdict.get("commit"))
    print("  passed:         %s" % verdict.get("passed"))
    print("  evidence-run-id: %s" % args.run_id)
    # Re-verify the promoted copy against the canonical repo: the copy IS the
    # thing the row will now cite, so it is graded on its own.
    check_verdict(target, args.task_id, repo,
                  expect_commit=args.expect_commit,
                  expect_ancestor_of=args.expect_ancestor_of,
                  allow_failed=args.allow_failed)
    print("  post-copy verify: OK")
    return 0


def main(argv=None):
    ap = argparse.ArgumentParser(
        description="Verify and bind worktree-produced GitReins verdicts to "
        "their board row (no fabrication, no clobbering)."
    )
    sub = ap.add_subparsers(dest="cmd", required=True)

    v = sub.add_parser("verify", help="validate one history run directory")
    v.add_argument("--verdict-dir", required=True)
    v.add_argument("--task-id", required=True)
    v.add_argument("--expect-commit", default=None)
    v.add_argument("--expect-ancestor-of", default=None)
    v.add_argument("--repo", default=None)
    v.add_argument("--allow-failed", action="store_true")
    v.add_argument("--json", action="store_true")

    p = sub.add_parser(
        "promote",
        help="verify then byte-identically copy a run dir into a canonical "
        "history store",
    )
    p.add_argument("--source", required=True,
                   help="run directory or history root holding <date>/<run-id>")
    p.add_argument("--run-id", required=True)
    p.add_argument("--task-id", required=True)
    p.add_argument("--expect-commit", required=True)
    p.add_argument("--expect-ancestor-of", default=None)
    p.add_argument("--dest", required=True,
                   help="canonical repository root receiving .gitreins/history")
    p.add_argument("--repo", default=None,
                   help="repository the graded commit must resolve in "
                   "(default: --dest)")
    p.add_argument("--date", default=None)
    p.add_argument("--allow-failed", action="store_true")
    p.add_argument("--json", action="store_true")

    args = ap.parse_args(argv)
    if args.cmd == "promote" and not args.repo:
        args.repo = args.dest

    if args.cmd == "verify" and not args.repo:
        probe = os.path.dirname(os.path.abspath(args.verdict_dir))
        proc = git(probe, ["rev-parse", "--show-toplevel"])
        if proc.returncode != 0:
            print("worktree-verdict-bind: cannot locate a repository from %s "
                  "(pass --repo)" % probe, file=sys.stderr)
            return 2
        args.repo = proc.stdout.strip()

    try:
        if args.cmd == "verify":
            return cmd_verify(args)
        return cmd_promote(args)
    except BindUsageError as exc:
        print("worktree-verdict-bind: USAGE: %s" % exc, file=sys.stderr)
        return 2
    except BindError as exc:
        print("worktree-verdict-bind: REFUSED: %s" % exc, file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
