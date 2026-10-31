| id | path | method | class | operationId | declared_responses | served_outcome | detail |
|---|---|---|---|---|---|---|---|
| SHIM-DRIFT-001 | /api/session/{sessionID}/permission/{requestID} | GET | NOT-SERVED | v2.session.permission.get | 200,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-002 | /api/session/{sessionID}/permission/{requestID}/reply | POST | NOT-SERVED | v2.session.permission.reply | 204,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-003 | /api/session/{sessionID}/prompt | POST | NOT-SERVED | v2.session.prompt | 200,400,401,404,409 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-004 | /api/session/{sessionID}/question | GET | NOT-SERVED | v2.session.question.list | 200,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-005 | /api/session/{sessionID}/question/{requestID}/reject | POST | NOT-SERVED | v2.session.question.reject | 204,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-006 | /api/session/{sessionID}/question/{requestID}/reply | POST | NOT-SERVED | v2.session.question.reply | 204,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-007 | /api/session/{sessionID}/revert/commit | POST | NOT-SERVED | v2.session.revert.commit | 204,400,401,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-008 | /api/session/{sessionID}/revert/stage | POST | NOT-SERVED | v2.session.revert.stage | 200,400,401,404,500 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-009 | /api/session/{sessionID}/wait | POST | NOT-SERVED | v2.session.wait | 204,400,401,404,503 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-010 | /auth/{providerID} | DELETE | OUTCOME-MISMATCH | auth.remove | 200,400 | 405 | declared 200,400, served 405 |
| SHIM-DRIFT-011 | /experimental/project/{projectID}/copy/generate-name | POST | NOT-SERVED | experimental.projectCopy.generateName | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-012 | /experimental/project/{projectID}/copy/refresh | POST | NOT-SERVED | v2.projectCopy.refresh | 204,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-013 | /experimental/resource | GET | NOT-SERVED | experimental.resource.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-014 | /experimental/session | GET | NOT-SERVED | experimental.session.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-015 | /experimental/session/{sessionID}/background | POST | NOT-SERVED | experimental.session.background | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-016 | /experimental/workspace | GET | NOT-SERVED | experimental.workspace.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-017 | /experimental/workspace | POST | NOT-SERVED | experimental.workspace.create | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-018 | /experimental/workspace/adapter | GET | NOT-SERVED | experimental.workspace.adapter.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-019 | /experimental/workspace/status | GET | NOT-SERVED | experimental.workspace.status | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-020 | /experimental/workspace/sync-list | POST | NOT-SERVED | experimental.workspace.syncList | 204,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-021 | /experimental/workspace/warp | POST | NOT-SERVED | experimental.workspace.warp | 204,400,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-022 | /experimental/workspace/{id} | DELETE | NOT-SERVED | experimental.workspace.remove | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-023 | /formatter | GET | NOT-SERVED | formatter.status | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-024 | /global/upgrade | POST | NOT-SERVED | global.upgrade | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-025 | /provider/{providerID}/oauth/authorize | POST | NOT-SERVED | provider.oauth.authorize | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-026 | /session/{sessionID}/fork | POST | OUTCOME-MISMATCH | session.fork | 200,400,404 | 501 | declared 200,400,404, served 501 |
| SHIM-DRIFT-027 | /session/{sessionID}/init | POST | OUTCOME-MISMATCH | session.init | 200,400,404 | 501 | declared 200,400,404, served 501 |
| SHIM-DRIFT-028 | /sync/steal | POST | NOT-SERVED | sync.steal | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-029 | /tui/submit-prompt | POST | OUTCOME-MISMATCH | tui.submitPrompt | 200,400 | 501 | declared 200,400, served 501 |
