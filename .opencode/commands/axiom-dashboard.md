---
description: Show a live dashboard of all active Axiom sessions with cost, tokens, model, and status.
---

Show the TUI dashboard of all active Axiom sessions.

Call the `codeops_dashboard` tool with these arguments:
- `format`: Use `$ARGUMENTS` if it contains "json", otherwise omit (defaults to table)
- `team`: Set to `true` if `$ARGUMENTS` contains "team"
- `max_rows`: If `$ARGUMENTS` contains a number, use it as max_rows

If no arguments are provided, call `codeops_dashboard` with no arguments to get the default table view.

Return the tool output directly to the user — do not summarize or reformat it.
