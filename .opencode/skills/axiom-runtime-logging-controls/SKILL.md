---
name: axiom-runtime-logging-controls
description: Portable contract for runtime logging controls (profile/depth/categories/format/sink) across CLI and HTTP server surfaces.
version: "1.0"
license: MIT
compatibility: opencode
metadata:
  workflow: logging
  outputs: "specs/50-Runtime-Logging-Modes-And-Formats.md (contract reference); config/flags updates"
tags:
  vertical: [coding, ops, sre]
  category: operations
  core: false
---

# Axiom Runtime Logging Controls (Portable)

Authoritative source: `specs/50-Runtime-Logging-Modes-And-Formats.md`.

Use this skill when adding operator-facing controls to tune runtime logging.

## Controls (Contract)

The runtime supports:
- Profiles: `minimal`, `standard`, `verbose`, `debug`
- Depth: `0`..`3`
- Categories: include/exclude filters with non-suppressible safety behavior
- Formats: `jsonl` (default), `text`
- Sinks: `stdout` (default), `stderr`, file path

## Fail-Closed Validation

- Unknown profile/format/category MUST raise an actionable error.
- Safety categories and terminal `WARN`/`ERROR` MUST not be fully suppressible.

## Integration Surfaces

- CLI: `axiom run` and `axiom serve` expose equivalent flags.
- HTTP server: controls apply to request lifecycle, orchestration events, and SSE publication boundaries.

## Verification Minimum

1. Parser tests for valid/invalid values.
2. Category include/exclude tests.
3. Format and sink tests.
4. Tier-3 runtime proof on both `run` and `serve` with non-default controls.

## References

- `specs/50-Runtime-Logging-Modes-And-Formats.md`
- `specs/25-Structured-Logging-Events.md`
- `.memory-bank/best-practices/runtime-logging-modes.md`

axiom:trace work_item=doctrine spec=specs/50-Runtime-Logging-Modes-And-Formats.md plan= test= doc=.opencode/skills/axiom-runtime-logging-controls/SKILL.md evidence= commit=
