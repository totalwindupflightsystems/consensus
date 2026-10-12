| id | path | method | operationId | declared_responses | served_outcome | detail |
|---|---|---|---|---|---|---|
| SHIM-NARROWED-001 | /permission/{requestID}/reply | POST | permission.reply | 200,400,404 | 404-typed | shim answers the declared 404 but never the declared success code |
| SHIM-NARROWED-002 | /project/{projectID} | PATCH | project.update | 200,400,404 | 200 | shim answers the declared 200 but never the declared success code |
| SHIM-NARROWED-003 | /question/{requestID}/reject | POST | question.reject | 200,400,404 | 404-typed | shim answers the declared 404 but never the declared success code |
| SHIM-NARROWED-004 | /question/{requestID}/reply | POST | question.reply | 200,400,404 | 404-typed | shim answers the declared 404 but never the declared success code |
| SHIM-NARROWED-005 | /session/{sessionID}/message/{messageID}/part/{partID} | DELETE | part.delete | 200,400,404 | 404 | shim answers the declared 404 but never the declared success code |
| SHIM-NARROWED-006 | /tui/select-session | POST | tui.selectSession | 200,400,404 | 404 | router catch-all answers the declared 404 (no success path) |
