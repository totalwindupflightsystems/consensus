| id | path | method | class | operationId | declared_responses | served_outcome | detail |
|---|---|---|---|---|---|---|---|
| SHIM-DRIFT-001 | /auth/{providerID} | DELETE | OUTCOME-MISMATCH | auth.remove | 200,400 | 405 | declared 200,400, served 405 |
| SHIM-DRIFT-002 | /experimental/project/{projectID}/copy/generate-name | POST | NOT-SERVED | experimental.projectCopy.generateName | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-003 | /experimental/project/{projectID}/copy/refresh | POST | NOT-SERVED | v2.projectCopy.refresh | 204,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-004 | /experimental/resource | GET | NOT-SERVED | experimental.resource.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-005 | /experimental/session | GET | NOT-SERVED | experimental.session.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-006 | /experimental/session/{sessionID}/background | POST | NOT-SERVED | experimental.session.background | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-007 | /experimental/workspace/adapter | GET | NOT-SERVED | experimental.workspace.adapter.list | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-008 | /experimental/workspace/status | GET | NOT-SERVED | experimental.workspace.status | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-009 | /experimental/workspace/sync-list | POST | NOT-SERVED | experimental.workspace.syncList | 204,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-010 | /experimental/workspace/warp | POST | NOT-SERVED | experimental.workspace.warp | 204,400,404 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-011 | /experimental/workspace/{id} | DELETE | NOT-SERVED | experimental.workspace.remove | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-012 | /formatter | GET | NOT-SERVED | formatter.status | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-013 | /global/upgrade | POST | NOT-SERVED | global.upgrade | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-014 | /provider/{providerID}/oauth/authorize | POST | NOT-SERVED | provider.oauth.authorize | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-015 | /session/{sessionID}/fork | POST | OUTCOME-MISMATCH | session.fork | 200,400,404 | 501 | declared 200,400,404, served 501 |
| SHIM-DRIFT-016 | /session/{sessionID}/init | POST | OUTCOME-MISMATCH | session.init | 200,400,404 | 501 | declared 200,400,404, served 501 |
| SHIM-DRIFT-017 | /sync/steal | POST | NOT-SERVED | sync.steal | 200,400 |  | no shim route at all; net/http default 404 |
| SHIM-DRIFT-018 | /tui/submit-prompt | POST | OUTCOME-MISMATCH | tui.submitPrompt | 200,400 | 501 | declared 200,400, served 501 |
