# SHIM-GAP-001 Triage Report

Date: 2026-09-30
Artifact: specs/openapi/upstream/opencode-declared-vs-served-1.18.33.json
Total findings triaged: 174

## Summary

| Triage | Count |
|--------|-------|
| must-serve | 41 |
| never-serve | 1 |
| defer | 132 |

### By class within each triage

| Triage | Class | Count |
|--------|-------|-------|
| must-serve | ? | 8 |
| must-serve | OUTCOME-MISMATCH | 33 |
| never-serve | ? | 1 |
| defer | ? | 19 |
| defer | NOT-SERVED | 111 |
| defer | SEMANTIC-DIFFERENT | 2 |

## Must-serve (ordered by client-call priority)

| # | operationId | class | reason |
|---|-------------|-------|--------|
| 1 | part.delete | ? | called in first minute by opencode client |
| 2 | part.update | ? | called in first minute by opencode client |
| 3 | permission.respond | ? | called in first minute by opencode client |
| 4 | session.command | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 5 | session.deleteMessage | ? | called in first minute by opencode client |
| 6 | session.diff | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 7 | session.fork | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 8 | session.init | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 9 | session.prompt_async | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 10 | session.revert | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 11 | session.share | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 12 | session.shell | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 13 | session.status | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 14 | session.summarize | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 15 | session.todo | ? | called in first minute by opencode client |
| 16 | session.unrevert | ? | called in first minute by opencode client |
| 17 | session.unshare | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 18 | ? | ? | called in first minute by opencode client |
| 19 | project.current | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 20 | project.directories | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 21 | project.initGit | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 22 | project.list | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 23 | project.update | ? | called in first minute by opencode client |
| 24 | auth.remove | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 25 | find.symbols | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 26 | instance.dispose | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 27 | tui.appendPrompt | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 28 | tui.clearPrompt | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 29 | tui.control.next | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 30 | tui.control.response | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 31 | tui.executeCommand | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 32 | tui.openHelp | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 33 | tui.openModels | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 34 | tui.openSessions | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 35 | tui.openThemes | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 36 | tui.publish | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 37 | tui.showToast | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 38 | tui.submitPrompt | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 39 | vcs.apply | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 40 | vcs.diff.raw | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |
| 41 | vcs.status | OUTCOME-MISMATCH | already served but answers wrong; fix in-place |

## Never-serve

| operationId | class | why |
|-------------|-------|-----|
| tui.selectSession | ? | TUI-only operation, not needed by harness backend |

## Defer

(132 items — see artifact JSON for full list)

| operationId | class | reason |
|-------------|-------|--------|
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| ? | ? | needs design decision |
| app.skills | NOT-SERVED | not called in first minute; implement after core surface |
| command.list | NOT-SERVED | not called in first minute; implement after core surface |
| experimental.capabilities.get | NOT-SERVED | not called in first minute; implement after core surface |
| experimental.console.get | NOT-SERVED | not called in first minute; implement after core surface |
| experimental.console.listOrgs | NOT-SERVED | not called in first minute; implement after core surface |
| experimental.console.switchOrg | NOT-SERVED | not called in first minute; implement after core surface |
| experimental.controlPlane.moveSession | NOT-SERVED | not called in first minute; implement after core surface |
| experimental.projectCopy.generateName | NOT-SERVED | not called in first minute; implement after core surface |
| experimental.resource.list | NOT-SERVED | not called in first minute; implement after core surface |
| experimental.session.background | NOT-SERVED | not called in first minute; implement after core surface |
| experimental.session.list | NOT-SERVED | not called in first minute; implement after core surface |
| experimental.workspace.adapter.list | NOT-SERVED | not called in first minute; implement after core surface |
| experimental.workspace.create | NOT-SERVED | not called in first minute; implement after core surface |
| experimental.workspace.list | NOT-SERVED | not called in first minute; implement after core surface |
| ... | ... | (102 more in artifact JSON) |
