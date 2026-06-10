---
name: todo-archive-scripts
description: Archive and query TODO blocks using JSONL format. Use this skill when archiving completed TODO sections or querying historical work from the TODO_ARCHIVE.
version: "1.1"
tags:
  vertical: [planning]
  category: planning
  core: false
---

# TODO Archive Scripts

The Axiom CLI provides native TODO archive operations via the `axiom todo` subcommand group. Standalone Python scripts in `scripts/` remain as fallback.

## When to Use

- After completing a phase or work item, archive the relevant TODO block.
- When you need historical context about completed work (what was done, when, evidence paths).
- When cleaning up TODO.md to keep it focused on active work.

## Primary Interface: CLI Commands

### `axiom todo archive` -- Archiver

Archives a TODO block from a markdown file into `.memory-bank/TODO_ARCHIVE/` as JSONL.

**Usage**:

```bash
# Archive a completed block (dry run first)
axiom todo archive \
  --work-item benchmark-sink-01 \
  --phase "Phase 24.7" \
  --dry-run \
  --markdown-file .memory-bank/TODO.md

# Archive for real
axiom todo archive \
  --work-item benchmark-sink-01 \
  --phase "Phase 24.7" \
  --markdown-file .memory-bank/TODO.md
```

**Key flags**:

| Flag | Required | Description |
|------|----------|-------------|
| `--work-item` | Yes | Work item ID (e.g., `benchmark-sink-01`) |
| `--phase` | Yes | Phase name (e.g., `Phase 24.7`) |
| `--markdown-file` | Yes | Path to the markdown file to archive from |
| `--status` | No | `COMPLETE` (default), `IN_PROGRESS`, or `BLOCKED` |
| `--dry-run` | No | Print JSONL without writing |

### `axiom todo query` -- Query Tool

Queries `.memory-bank/TODO_ARCHIVE/*.jsonl` files with filtering and multiple output formats.

**Usage**:

```bash
# List all archived blocks (summary view)
axiom todo query --list

# Filter by work item
axiom todo query --work-item benchmark-sink-01

# Filter by work item (JSON output)
axiom todo query --work-item benchmark-sink-01 --json

# Filter by phase
axiom todo query --phase "Phase 24.7"

# Count matching blocks
axiom todo query --status COMPLETE --count
```

**Key flags**:

| Flag | Description |
|------|-------------|
| `--list` | List all blocks with summaries |
| `--work-item` | Filter by work item ID |
| `--phase` | Filter by phase name |
| `--status` | Filter by `COMPLETE`, `IN_PROGRESS`, or `BLOCKED` |
| `--from-date` | Filter by start date (YYYY-MM-DD) |
| `--to-date` | Filter by end date (YYYY-MM-DD) |
| `--tag` | Filter by tag |
| `--full` | Show full content |
| `--json` | JSON output |
| `--trace-only` | Show only trace markers |
| `--evidence-only` | Show only evidence paths |
| `--count` | Show count only |
| `--archive-dir` | Custom archive directory |

### `axiom todo stats` -- Statistics

Shows archive statistics: total blocks, date range, phases covered, work items, status breakdown.

**Usage**:

```bash
axiom todo stats
axiom todo stats --archive-dir /custom/path
```

## Fallback: Standalone Scripts

The standalone scripts remain available as fallback when the CLI runtime is not installed:

### `scripts/archive_todo_block.py` -- Archiver (fallback)

Archives a TODO block from `TODO.md` (by line range) or from a standalone markdown file into `.memory-bank/TODO_ARCHIVE/` as JSONL.

**Usage**:

```bash
# Archive lines from TODO.md
python3 scripts/archive_todo_block.py \
  --file .memory-bank/TODO.md \
  --start-line 14 --end-line 77 \
  --work-item benchmark-sink-01 \
  --phase "Phase 24.7"

# Archive a standalone markdown file
python3 scripts/archive_todo_block.py \
  --markdown-file 2026-02-17_phase-24-7-scoring.md \
  --work-item benchmark-sink-01 \
  --phase "Phase 24.7"

# Dry run (print JSONL without writing)
python3 scripts/archive_todo_block.py \
  --file .memory-bank/TODO.md \
  --start-line 14 --end-line 77 \
  --work-item benchmark-sink-01 \
  --phase "Phase 24.7" \
  --dry-run
```

**Key flags**:

| Flag | Required | Description |
|------|----------|-------------|
| `--file` | One of `--file` or `--markdown-file` | Path to TODO.md (use with `--start-line`/`--end-line`) |
| `--markdown-file` | One of `--file` or `--markdown-file` | Path to a standalone markdown file |
| `--start-line` | With `--file` | Start line (1-indexed) |
| `--end-line` | With `--file` | End line (1-indexed) |
| `--work-item` | Yes | Work item ID (e.g., `benchmark-sink-01`) |
| `--phase` | Yes | Phase name (e.g., `Phase 24.7`) |
| `--status` | No | `COMPLETE` (default), `IN_PROGRESS`, or `BLOCKED` |
| `--output` | No | Custom output path (default: auto-generated in `TODO_ARCHIVE/`) |
| `--append` | No | Append to existing JSONL file |
| `--dry-run` | No | Print JSONL without writing |

**JSONL schema** (one JSON object per line):

```json
{
  "archived_date": "2026-02-18",
  "work_item_id": "benchmark-sink-01",
  "status": "COMPLETE",
  "phase": "Phase 24.7",
  "original_todo_lines": [14, 77],
  "content": "... original markdown ...",
  "summary": "auto-extracted summary",
  "evidence_paths": [".memory-bank/work-items/.../runs/.../verification.md"],
  "spec_refs": ["specs/00-PRD.md"],
  "commit_sha": "abc1234",
  "tags": ["phase-24-7", "work-item:benchmark-sink-01", "AC-UT-054"],
  "related_work_items": [],
  "trace_marker": "axiom:trace ..."
}
```

Auto-extraction: the archiver automatically extracts `summary`, `evidence_paths`, `spec_refs`, `commit_sha`, `tags`, and `trace_marker` from the content using regex patterns.

### `scripts/query_todo_archive.py` -- Query Tool (fallback)

Queries `.memory-bank/TODO_ARCHIVE/*.jsonl` files with filtering and multiple output formats.

**Usage**:

```bash
# List all archived blocks (summary view)
python3 scripts/query_todo_archive.py --list

# Filter by work item
python3 scripts/query_todo_archive.py --work-item benchmark-sink-01

# Filter by phase
python3 scripts/query_todo_archive.py --phase "Phase 24.7"

# Filter by date range
python3 scripts/query_todo_archive.py --from-date 2026-02-16 --to-date 2026-02-18

# Filter by status
python3 scripts/query_todo_archive.py --status COMPLETE

# Full content view
python3 scripts/query_todo_archive.py --work-item benchmark-sink-01 --full

# JSON output (for programmatic use)
python3 scripts/query_todo_archive.py --work-item benchmark-sink-01 --json

# Trace markers only
python3 scripts/query_todo_archive.py --work-item benchmark-sink-01 --trace-only

# Evidence paths only
python3 scripts/query_todo_archive.py --work-item benchmark-sink-01 --evidence-only

# Spec references only
python3 scripts/query_todo_archive.py --work-item benchmark-sink-01 --spec-refs-only

# Count matching blocks
python3 scripts/query_todo_archive.py --status COMPLETE --count
```

**Key flags**:

| Flag | Description |
|------|-------------|
| `--list` | List all blocks with summaries |
| `--work-item` | Filter by work item ID |
| `--phase` | Filter by phase name |
| `--status` | Filter by `COMPLETE`, `IN_PROGRESS`, or `BLOCKED` |
| `--from-date` | Filter by start date (YYYY-MM-DD) |
| `--to-date` | Filter by end date (YYYY-MM-DD) |
| `--tag` | Filter by tag |
| `--full` | Show full content |
| `--json` | JSON output |
| `--trace-only` | Show only trace markers |
| `--evidence-only` | Show only evidence paths |
| `--spec-refs-only` | Show only spec references |
| `--count` | Show count only |
| `--archive-dir` | Custom archive directory |

## Archive Directory

Default location: `.memory-bank/TODO_ARCHIVE/`

Files are named: `YYYY-MM-DD_<work-item>_<phase>.jsonl`

## Workflow: Archiving Completed Work

1. Identify the completed block in `TODO.md`.
2. Run the archiver with `--dry-run` first to verify:
   - `axiom todo archive --work-item <ID> --phase "<phase>" --dry-run --markdown-file .memory-bank/TODO.md`
3. Run the archiver to write the JSONL file:
   - `axiom todo archive --work-item <ID> --phase "<phase>" --markdown-file .memory-bank/TODO.md`
4. Remove the archived lines from `TODO.md`.
5. Add a reference in `TODO.md` pointing to the archive (e.g., "Archived to `.memory-bank/TODO_ARCHIVE/`").
6. Verify with the query tool: `axiom todo query --list`.

## Workflow: Querying Historical Context

1. Use `--list` to see all archived blocks: `axiom todo query --list`.
2. Filter by work item, phase, date, or status: `axiom todo query --work-item <ID>`.
3. Use `--full` to see original content: `axiom todo query --work-item <ID> --full`.
4. Use `--evidence-only` or `--trace-only` for targeted lookups.
5. Use `--json` for programmatic consumption by other scripts/agents: `axiom todo query --json`.
