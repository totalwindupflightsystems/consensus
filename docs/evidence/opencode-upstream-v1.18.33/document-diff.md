# Document diff — declared upstream vs served shim

| measure | declared | served | common | declared_only | served_only |
|---|---|---|---|---|---|
| paths | 162 | 69 | 28 | 134 | 41 |


## Operations added / removed / modified

| id | class | path | method | operationId | detail |
|---|---|---|---|---|---|
| DOC-OP-001 | PATH-REMOVED | /api/agent | GET | v2.agent.list | the served document declares no such path |
| DOC-OP-002 | PATH-REMOVED | /api/command | GET | v2.command.list | the served document declares no such path |
| DOC-OP-003 | PATH-REMOVED | /api/credential/{credentialID} | DELETE | v2.credential.remove | the served document declares no such path |
| DOC-OP-004 | PATH-REMOVED | /api/credential/{credentialID} | PATCH | v2.credential.update | the served document declares no such path |
| DOC-OP-005 | PATH-REMOVED | /api/event | GET | v2.event.subscribe | the served document declares no such path |
| DOC-OP-006 | PATH-REMOVED | /api/fs/find | GET | v2.fs.find | the served document declares no such path |
| DOC-OP-007 | PATH-REMOVED | /api/fs/list | GET | v2.fs.list | the served document declares no such path |
| DOC-OP-008 | PATH-REMOVED | /api/fs/read/* | GET | v2.fs.read | the served document declares no such path |
| DOC-OP-009 | PATH-REMOVED | /api/health | GET | v2.health.get | the served document declares no such path |
| DOC-OP-010 | PATH-REMOVED | /api/integration | GET | v2.integration.list | the served document declares no such path |
| DOC-OP-011 | PATH-REMOVED | /api/integration/attempt/{attemptID} | DELETE | v2.integration.attempt.cancel | the served document declares no such path |
| DOC-OP-012 | PATH-REMOVED | /api/integration/attempt/{attemptID} | GET | v2.integration.attempt.status | the served document declares no such path |
| DOC-OP-013 | PATH-REMOVED | /api/integration/attempt/{attemptID}/complete | POST | v2.integration.attempt.complete | the served document declares no such path |
| DOC-OP-014 | PATH-REMOVED | /api/integration/{integrationID} | GET | v2.integration.get | the served document declares no such path |
| DOC-OP-015 | PATH-REMOVED | /api/integration/{integrationID}/connect/key | POST | v2.integration.connect.key | the served document declares no such path |
| DOC-OP-016 | PATH-REMOVED | /api/integration/{integrationID}/connect/oauth | POST | v2.integration.connect.oauth | the served document declares no such path |
| DOC-OP-017 | PATH-REMOVED | /api/location | GET | v2.location.get | the served document declares no such path |
| DOC-OP-018 | PATH-REMOVED | /api/model | GET | v2.model.list | the served document declares no such path |
| DOC-OP-019 | PATH-REMOVED | /api/permission/request | GET | v2.permission.request.list | the served document declares no such path |
| DOC-OP-020 | PATH-REMOVED | /api/permission/saved | GET | v2.permission.saved.list | the served document declares no such path |
| DOC-OP-021 | PATH-REMOVED | /api/permission/saved/{id} | DELETE | v2.permission.saved.remove | the served document declares no such path |
| DOC-OP-022 | PATH-REMOVED | /api/provider | GET | v2.provider.list | the served document declares no such path |
| DOC-OP-023 | PATH-REMOVED | /api/provider/{providerID} | GET | v2.provider.get | the served document declares no such path |
| DOC-OP-024 | PATH-REMOVED | /api/pty | GET | v2.pty.list | the served document declares no such path |
| DOC-OP-025 | PATH-REMOVED | /api/pty | POST | v2.pty.create | the served document declares no such path |
| DOC-OP-026 | PATH-REMOVED | /api/pty/{ptyID} | DELETE | v2.pty.remove | the served document declares no such path |
| DOC-OP-027 | PATH-REMOVED | /api/pty/{ptyID} | GET | v2.pty.get | the served document declares no such path |
| DOC-OP-028 | PATH-REMOVED | /api/pty/{ptyID} | PUT | v2.pty.update | the served document declares no such path |
| DOC-OP-029 | PATH-REMOVED | /api/pty/{ptyID}/connect | GET | v2.pty.connect | the served document declares no such path |
| DOC-OP-030 | PATH-REMOVED | /api/pty/{ptyID}/connect-token | POST | v2.pty.connectToken | the served document declares no such path |
| DOC-OP-031 | PATH-REMOVED | /api/question/request | GET | v2.question.request.list | the served document declares no such path |
| DOC-OP-032 | PATH-REMOVED | /api/reference | GET | v2.reference.list | the served document declares no such path |
| DOC-OP-033 | PATH-REMOVED | /api/session | GET | v2.session.list | the served document declares no such path |
| DOC-OP-034 | PATH-REMOVED | /api/session | POST | v2.session.create | the served document declares no such path |
| DOC-OP-035 | PATH-REMOVED | /api/session/active | GET | v2.session.active | the served document declares no such path |
| DOC-OP-036 | PATH-REMOVED | /api/session/{sessionID} | GET | v2.session.get | the served document declares no such path |
| DOC-OP-037 | PATH-REMOVED | /api/session/{sessionID}/agent | POST | v2.session.switchAgent | the served document declares no such path |
| DOC-OP-038 | PATH-REMOVED | /api/session/{sessionID}/compact | POST | v2.session.compact | the served document declares no such path |
| DOC-OP-039 | PATH-REMOVED | /api/session/{sessionID}/context | GET | v2.session.context | the served document declares no such path |
| DOC-OP-040 | PATH-REMOVED | /api/session/{sessionID}/event | GET | v2.session.events | the served document declares no such path |
| DOC-OP-041 | PATH-REMOVED | /api/session/{sessionID}/history | GET | v2.session.history | the served document declares no such path |
| DOC-OP-042 | PATH-REMOVED | /api/session/{sessionID}/interrupt | POST | v2.session.interrupt | the served document declares no such path |
| DOC-OP-043 | PATH-REMOVED | /api/session/{sessionID}/message | GET | v2.session.messages | the served document declares no such path |
| DOC-OP-044 | PATH-REMOVED | /api/session/{sessionID}/message/{messageID} | GET | v2.session.message | the served document declares no such path |
| DOC-OP-045 | PATH-REMOVED | /api/session/{sessionID}/model | POST | v2.session.switchModel | the served document declares no such path |
| DOC-OP-046 | PATH-REMOVED | /api/session/{sessionID}/permission | GET | v2.session.permission.list | the served document declares no such path |
| DOC-OP-047 | PATH-REMOVED | /api/session/{sessionID}/permission | POST | v2.session.permission.create | the served document declares no such path |
| DOC-OP-048 | PATH-REMOVED | /api/session/{sessionID}/permission/{requestID} | GET | v2.session.permission.get | the served document declares no such path |
| DOC-OP-049 | PATH-REMOVED | /api/session/{sessionID}/permission/{requestID}/reply | POST | v2.session.permission.reply | the served document declares no such path |
| DOC-OP-050 | PATH-REMOVED | /api/session/{sessionID}/prompt | POST | v2.session.prompt | the served document declares no such path |
| DOC-OP-051 | PATH-REMOVED | /api/session/{sessionID}/question | GET | v2.session.question.list | the served document declares no such path |
| DOC-OP-052 | PATH-REMOVED | /api/session/{sessionID}/question/{requestID}/reject | POST | v2.session.question.reject | the served document declares no such path |
| DOC-OP-053 | PATH-REMOVED | /api/session/{sessionID}/question/{requestID}/reply | POST | v2.session.question.reply | the served document declares no such path |
| DOC-OP-054 | PATH-REMOVED | /api/session/{sessionID}/revert/clear | POST | v2.session.revert.clear | the served document declares no such path |
| DOC-OP-055 | PATH-REMOVED | /api/session/{sessionID}/revert/commit | POST | v2.session.revert.commit | the served document declares no such path |
| DOC-OP-056 | PATH-REMOVED | /api/session/{sessionID}/revert/stage | POST | v2.session.revert.stage | the served document declares no such path |
| DOC-OP-057 | PATH-REMOVED | /api/session/{sessionID}/wait | POST | v2.session.wait | the served document declares no such path |
| DOC-OP-058 | PATH-REMOVED | /api/skill | GET | v2.skill.list | the served document declares no such path |
| DOC-OP-059 | PATH-REMOVED | /command | GET | command.list | the served document declares no such path |
| DOC-OP-060 | PATH-REMOVED | /experimental/capabilities | GET | experimental.capabilities.get | the served document declares no such path |
| DOC-OP-061 | PATH-REMOVED | /experimental/console | GET | experimental.console.get | the served document declares no such path |
| DOC-OP-062 | PATH-REMOVED | /experimental/console/orgs | GET | experimental.console.listOrgs | the served document declares no such path |
| DOC-OP-063 | PATH-REMOVED | /experimental/console/switch | POST | experimental.console.switchOrg | the served document declares no such path |
| DOC-OP-064 | PATH-REMOVED | /experimental/control-plane/move-session | POST | experimental.controlPlane.moveSession | the served document declares no such path |
| DOC-OP-065 | PATH-REMOVED | /experimental/project/{projectID}/copy | DELETE | v2.projectCopy.remove | the served document declares no such path |
| DOC-OP-066 | PATH-REMOVED | /experimental/project/{projectID}/copy | POST | v2.projectCopy.create | the served document declares no such path |
| DOC-OP-067 | PATH-REMOVED | /experimental/project/{projectID}/copy/generate-name | POST | experimental.projectCopy.generateName | the served document declares no such path |
| DOC-OP-068 | PATH-REMOVED | /experimental/project/{projectID}/copy/refresh | POST | v2.projectCopy.refresh | the served document declares no such path |
| DOC-OP-069 | PATH-REMOVED | /experimental/resource | GET | experimental.resource.list | the served document declares no such path |
| DOC-OP-070 | PATH-REMOVED | /experimental/session | GET | experimental.session.list | the served document declares no such path |
| DOC-OP-071 | PATH-REMOVED | /experimental/session/{sessionID}/background | POST | experimental.session.background | the served document declares no such path |
| DOC-OP-072 | PATH-REMOVED | /experimental/workspace | GET | experimental.workspace.list | the served document declares no such path |
| DOC-OP-073 | PATH-REMOVED | /experimental/workspace | POST | experimental.workspace.create | the served document declares no such path |
| DOC-OP-074 | PATH-REMOVED | /experimental/workspace/adapter | GET | experimental.workspace.adapter.list | the served document declares no such path |
| DOC-OP-075 | PATH-REMOVED | /experimental/workspace/status | GET | experimental.workspace.status | the served document declares no such path |
| DOC-OP-076 | PATH-REMOVED | /experimental/workspace/sync-list | POST | experimental.workspace.syncList | the served document declares no such path |
| DOC-OP-077 | PATH-REMOVED | /experimental/workspace/warp | POST | experimental.workspace.warp | the served document declares no such path |
| DOC-OP-078 | PATH-REMOVED | /experimental/workspace/{id} | DELETE | experimental.workspace.remove | the served document declares no such path |
| DOC-OP-079 | PATH-REMOVED | /experimental/worktree | DELETE | worktree.remove | the served document declares no such path |
| DOC-OP-080 | PATH-REMOVED | /experimental/worktree | GET | worktree.list | the served document declares no such path |
| DOC-OP-081 | PATH-REMOVED | /experimental/worktree | POST | worktree.create | the served document declares no such path |
| DOC-OP-082 | PATH-REMOVED | /experimental/worktree/reset | POST | worktree.reset | the served document declares no such path |
| DOC-OP-083 | PATH-REMOVED | /file | GET | file.list | the served document declares no such path |
| DOC-OP-084 | PATH-REMOVED | /formatter | GET | formatter.status | the served document declares no such path |
| DOC-OP-085 | PATH-REMOVED | /global/config | GET | global.config.get | the served document declares no such path |
| DOC-OP-086 | PATH-REMOVED | /global/config | PATCH | global.config.update | the served document declares no such path |
| DOC-OP-087 | PATH-REMOVED | /global/dispose | POST | global.dispose | the served document declares no such path |
| DOC-OP-088 | PATH-REMOVED | /global/upgrade | POST | global.upgrade | the served document declares no such path |
| DOC-OP-089 | PATH-REMOVED | /instance/dispose | POST | instance.dispose | the served document declares no such path |
| DOC-OP-090 | PATH-REMOVED | /log | POST | app.log | the served document declares no such path |
| DOC-OP-091 | PATH-REMOVED | /mcp/{name}/auth | DELETE | mcp.auth.remove | the served document declares no such path |
| DOC-OP-092 | PATH-REMOVED | /mcp/{name}/auth | POST | mcp.auth.start | the served document declares no such path |
| DOC-OP-093 | PATH-REMOVED | /mcp/{name}/auth/authenticate | POST | mcp.auth.authenticate | the served document declares no such path |
| DOC-OP-094 | PATH-REMOVED | /mcp/{name}/auth/callback | POST | mcp.auth.callback | the served document declares no such path |
| DOC-OP-095 | PATH-REMOVED | /mcp/{name}/connect | POST | mcp.connect | the served document declares no such path |
| DOC-OP-096 | PATH-REMOVED | /mcp/{name}/disconnect | POST | mcp.disconnect | the served document declares no such path |
| DOC-OP-097 | PATH-REMOVED | /path | GET | path.get | the served document declares no such path |
| DOC-OP-098 | PATH-REMOVED | /permission/{requestID}/reply | POST | permission.reply | the served document declares no such path |
| DOC-OP-099 | PATH-REMOVED | /project/current | GET | project.current | the served document declares no such path |
| DOC-OP-100 | PATH-REMOVED | /project/git/init | POST | project.initGit | the served document declares no such path |
| DOC-OP-101 | PATH-REMOVED | /project/{projectID}/directories | GET | project.directories | the served document declares no such path |
| DOC-OP-102 | PATH-REMOVED | /provider/auth | GET | provider.auth | the served document declares no such path |
| DOC-OP-103 | PATH-REMOVED | /provider/{providerID}/oauth/authorize | POST | provider.oauth.authorize | the served document declares no such path |
| DOC-OP-104 | PATH-REMOVED | /provider/{providerID}/oauth/callback | POST | provider.oauth.callback | the served document declares no such path |
| DOC-OP-105 | PATH-REMOVED | /pty | GET | pty.list | the served document declares no such path |
| DOC-OP-106 | PATH-REMOVED | /pty | POST | pty.create | the served document declares no such path |
| DOC-OP-107 | PATH-REMOVED | /pty/shells | GET | pty.shells | the served document declares no such path |
| DOC-OP-108 | PATH-REMOVED | /pty/{ptyID} | DELETE | pty.remove | the served document declares no such path |
| DOC-OP-109 | PATH-REMOVED | /pty/{ptyID} | GET | pty.get | the served document declares no such path |
| DOC-OP-110 | PATH-REMOVED | /pty/{ptyID} | PUT | pty.update | the served document declares no such path |
| DOC-OP-111 | PATH-REMOVED | /pty/{ptyID}/connect | GET | pty.connect | the served document declares no such path |
| DOC-OP-112 | PATH-REMOVED | /pty/{ptyID}/connect-token | POST | pty.connectToken | the served document declares no such path |
| DOC-OP-113 | PATH-REMOVED | /question | GET | question.list | the served document declares no such path |
| DOC-OP-114 | PATH-REMOVED | /question/{requestID}/reject | POST | question.reject | the served document declares no such path |
| DOC-OP-115 | PATH-REMOVED | /question/{requestID}/reply | POST | question.reply | the served document declares no such path |
| DOC-OP-116 | PATH-REMOVED | /session/status | GET | session.status | the served document declares no such path |
| DOC-OP-117 | PATH-REMOVED | /session/{sessionID}/command | POST | session.command | the served document declares no such path |
| DOC-OP-118 | PATH-REMOVED | /session/{sessionID}/diff | GET | session.diff | the served document declares no such path |
| DOC-OP-119 | PATH-REMOVED | /session/{sessionID}/fork | POST | session.fork | the served document declares no such path |
| DOC-OP-120 | PATH-REMOVED | /session/{sessionID}/init | POST | session.init | the served document declares no such path |
| DOC-OP-121 | PATH-REMOVED | /session/{sessionID}/message/{messageID}/part/{partID} | DELETE | part.delete | the served document declares no such path |
| DOC-OP-122 | PATH-REMOVED | /session/{sessionID}/message/{messageID}/part/{partID} | PATCH | part.update | the served document declares no such path |
| DOC-OP-123 | PATH-REMOVED | /session/{sessionID}/permissions/{permissionID} | POST | permission.respond | the served document declares no such path |
| DOC-OP-124 | PATH-REMOVED | /session/{sessionID}/prompt_async | POST | session.prompt_async | the served document declares no such path |
| DOC-OP-125 | PATH-REMOVED | /session/{sessionID}/revert | POST | session.revert | the served document declares no such path |
| DOC-OP-126 | PATH-REMOVED | /session/{sessionID}/share | DELETE | session.unshare | the served document declares no such path |
| DOC-OP-127 | PATH-REMOVED | /session/{sessionID}/share | POST | session.share | the served document declares no such path |
| DOC-OP-128 | PATH-REMOVED | /session/{sessionID}/shell | POST | session.shell | the served document declares no such path |
| DOC-OP-129 | PATH-REMOVED | /session/{sessionID}/summarize | POST | session.summarize | the served document declares no such path |
| DOC-OP-130 | PATH-REMOVED | /session/{sessionID}/todo | GET | session.todo | the served document declares no such path |
| DOC-OP-131 | PATH-REMOVED | /session/{sessionID}/unrevert | POST | session.unrevert | the served document declares no such path |
| DOC-OP-132 | PATH-REMOVED | /skill | GET | app.skills | the served document declares no such path |
| DOC-OP-133 | PATH-REMOVED | /sync/history | POST | sync.history.list | the served document declares no such path |
| DOC-OP-134 | PATH-REMOVED | /sync/replay | POST | sync.replay | the served document declares no such path |
| DOC-OP-135 | PATH-REMOVED | /sync/start | POST | sync.start | the served document declares no such path |
| DOC-OP-136 | PATH-REMOVED | /sync/steal | POST | sync.steal | the served document declares no such path |
| DOC-OP-137 | PATH-REMOVED | /tui/append-prompt | POST | tui.appendPrompt | the served document declares no such path |
| DOC-OP-138 | PATH-REMOVED | /tui/clear-prompt | POST | tui.clearPrompt | the served document declares no such path |
| DOC-OP-139 | PATH-REMOVED | /tui/control/next | GET | tui.control.next | the served document declares no such path |
| DOC-OP-140 | PATH-REMOVED | /tui/control/response | POST | tui.control.response | the served document declares no such path |
| DOC-OP-141 | PATH-REMOVED | /tui/execute-command | POST | tui.executeCommand | the served document declares no such path |
| DOC-OP-142 | PATH-REMOVED | /tui/open-help | POST | tui.openHelp | the served document declares no such path |
| DOC-OP-143 | PATH-REMOVED | /tui/open-models | POST | tui.openModels | the served document declares no such path |
| DOC-OP-144 | PATH-REMOVED | /tui/open-sessions | POST | tui.openSessions | the served document declares no such path |
| DOC-OP-145 | PATH-REMOVED | /tui/open-themes | POST | tui.openThemes | the served document declares no such path |
| DOC-OP-146 | PATH-REMOVED | /tui/publish | POST | tui.publish | the served document declares no such path |
| DOC-OP-147 | PATH-REMOVED | /tui/select-session | POST | tui.selectSession | the served document declares no such path |
| DOC-OP-148 | PATH-REMOVED | /tui/show-toast | POST | tui.showToast | the served document declares no such path |
| DOC-OP-149 | PATH-REMOVED | /tui/submit-prompt | POST | tui.submitPrompt | the served document declares no such path |
| DOC-OP-150 | PATH-REMOVED | /vcs/apply | POST | vcs.apply | the served document declares no such path |
| DOC-OP-151 | PATH-REMOVED | /vcs/diff/raw | GET | vcs.diff.raw | the served document declares no such path |
| DOC-OP-152 | PATH-REMOVED | /vcs/status | GET | vcs.status | the served document declares no such path |
| DOC-OP-153 | PATH-ADDED | /api/v1/approvals | GET | listApprovals | the served document declares a path upstream does not |
| DOC-OP-154 | PATH-ADDED | /api/v1/approvals/{approvalId} | GET | getApproval | the served document declares a path upstream does not |
| DOC-OP-155 | PATH-ADDED | /api/v1/approvals/{approvalId}/review | POST | reviewApproval | the served document declares a path upstream does not |
| DOC-OP-156 | PATH-ADDED | /api/v1/auth/keys | GET | listAPIKeys | the served document declares a path upstream does not |
| DOC-OP-157 | PATH-ADDED | /api/v1/auth/keys | POST | createAPIKey | the served document declares a path upstream does not |
| DOC-OP-158 | PATH-ADDED | /api/v1/auth/keys/{keyId} | DELETE | revokeAPIKey | the served document declares a path upstream does not |
| DOC-OP-159 | PATH-ADDED | /api/v1/config | GET | getConfig | the served document declares a path upstream does not |
| DOC-OP-160 | PATH-ADDED | /api/v1/events | GET | streamEvents | the served document declares a path upstream does not |
| DOC-OP-161 | PATH-ADDED | /api/v1/health | GET | healthCheck | the served document declares a path upstream does not |
| DOC-OP-162 | PATH-ADDED | /api/v1/metrics | GET | getMetrics | the served document declares a path upstream does not |
| DOC-OP-163 | PATH-ADDED | /api/v1/quarantine | GET | listQuarantine | the served document declares a path upstream does not |
| DOC-OP-164 | PATH-ADDED | /api/v1/quarantine/{qID}/approve | POST | approveQuarantine | the served document declares a path upstream does not |
| DOC-OP-165 | PATH-ADDED | /api/v1/quarantine/{qID}/reject | POST | rejectQuarantine | the served document declares a path upstream does not |
| DOC-OP-166 | PATH-ADDED | /api/v1/sessions | GET | listSessions | the served document declares a path upstream does not |
| DOC-OP-167 | PATH-ADDED | /api/v1/sessions | POST | createSession | the served document declares a path upstream does not |
| DOC-OP-168 | PATH-ADDED | /api/v1/sessions/{sessionId} | DELETE | deleteSession | the served document declares a path upstream does not |
| DOC-OP-169 | PATH-ADDED | /api/v1/sessions/{sessionId} | GET | getSession | the served document declares a path upstream does not |
| DOC-OP-170 | PATH-ADDED | /api/v1/sessions/{sessionId} | PATCH | updateSession | the served document declares a path upstream does not |
| DOC-OP-171 | PATH-ADDED | /api/v1/sessions/{sessionId}/approvals | GET | listSessionApprovals | the served document declares a path upstream does not |
| DOC-OP-172 | PATH-ADDED | /api/v1/sessions/{sessionId}/billing | GET | getSessionBilling | the served document declares a path upstream does not |
| DOC-OP-173 | PATH-ADDED | /api/v1/sessions/{sessionId}/context | GET | getActiveContext | the served document declares a path upstream does not |
| DOC-OP-174 | PATH-ADDED | /api/v1/sessions/{sessionId}/iterations | GET | listIterations | the served document declares a path upstream does not |
| DOC-OP-175 | PATH-ADDED | /api/v1/sessions/{sessionId}/memory | GET | listMemory | the served document declares a path upstream does not |
| DOC-OP-176 | PATH-ADDED | /api/v1/sessions/{sessionId}/memory/{memoryId} | GET | getMemoryEvent | the served document declares a path upstream does not |
| DOC-OP-177 | PATH-ADDED | /api/v1/sessions/{sessionId}/message | POST | sendMessage | the served document declares a path upstream does not |
| DOC-OP-178 | PATH-ADDED | /api/v1/sessions/{sessionId}/tasks | GET | listTasks | the served document declares a path upstream does not |
| DOC-OP-179 | PATH-ADDED | /api/v1/sessions/{sessionId}/tasks | POST | createTask | the served document declares a path upstream does not |
| DOC-OP-180 | PATH-ADDED | /api/v1/sessions/{sessionId}/tool-calls | GET | listSessionToolCalls | the served document declares a path upstream does not |
| DOC-OP-181 | PATH-ADDED | /api/v1/skills | GET | listSkills | the served document declares a path upstream does not |
| DOC-OP-182 | PATH-ADDED | /api/v1/skills/{skillName} | GET | getSkill | the served document declares a path upstream does not |
| DOC-OP-183 | PATH-ADDED | /api/v1/tasks/{taskId} | GET | getTask | the served document declares a path upstream does not |
| DOC-OP-184 | PATH-ADDED | /api/v1/tasks/{taskId} | PATCH | updateTask | the served document declares a path upstream does not |
| DOC-OP-185 | PATH-ADDED | /api/v1/tasks/{taskId}/claim | POST | claimTask | the served document declares a path upstream does not |
| DOC-OP-186 | PATH-ADDED | /api/v1/tool-requests | GET | listToolRequests | the served document declares a path upstream does not |
| DOC-OP-187 | PATH-ADDED | /api/v1/tool-requests/{requestId} | GET | getToolRequest | the served document declares a path upstream does not |
| DOC-OP-188 | PATH-ADDED | /api/v1/tools | GET | listTools | the served document declares a path upstream does not |
| DOC-OP-189 | PATH-ADDED | /api/v1/tools/{toolName}/execute | POST | executeTool | the served document declares a path upstream does not |
| DOC-OP-190 | PATH-ADDED | /doc | GET | shimDoc | the served document declares a path upstream does not |
| DOC-OP-191 | PATH-ADDED | /instance | GET | shimInstanceList | the served document declares a path upstream does not |
| DOC-OP-192 | PATH-ADDED | /instance/path | GET | shimInstancePath | the served document declares a path upstream does not |
| DOC-OP-193 | PATH-ADDED | /instance/vcs | GET | shimInstanceVCS | the served document declares a path upstream does not |
| DOC-OP-194 | PATH-ADDED | /instance/vcs/diff | GET | shimInstanceVCSDiff | the served document declares a path upstream does not |
| DOC-OP-195 | PATH-ADDED | /permission/{permissionId} | GET | shimGetPermission | the served document declares a path upstream does not |
| DOC-OP-196 | PATH-ADDED | /permission/{permissionId}/resolve | POST | shimResolvePermission | the served document declares a path upstream does not |
| DOC-OP-197 | PATH-ADDED | /tui/{action} | GET | shimTUIControlStub | the served document declares a path upstream does not |
| DOC-OP-198 | PATH-ADDED | /tui/{action} | POST | shimTUIAction | the served document declares a path upstream does not |
| DOC-OP-199 | PATH-ADDED | /vcs/{vcsId} | GET | shimVCSItemStub | the served document declares a path upstream does not |
| DOC-OP-200 | PATH-ADDED | /webhooks/{source} | POST | ingestWebhook | the served document declares a path upstream does not |
| DOC-OP-201 | OPERATION-MODIFIED | /agent | GET | shimListAgents | operationId: declared=app.agents served=shimListAgents; tags: declared=["instance"] served=["Opencode Shim"]; responses: declared=["200",... |
| DOC-OP-202 | OPERATION-REMOVED | /auth/{providerID} | DELETE | auth.remove | path is declared on both sides; this method is declared upstream only |
| DOC-OP-203 | OPERATION-MODIFIED | /auth/{providerID} | PUT | shimUpdateAuth | operationId: declared=auth.set served=shimUpdateAuth; tags: declared=["control"] served=["Opencode Shim"]; responses: declared=["200", "4... |
| DOC-OP-204 | OPERATION-MODIFIED | /config | GET | shimGetConfig | operationId: declared=config.get served=shimGetConfig; tags: declared=["config"] served=["Opencode Shim"]; responses: declared=["200", "4... |
| DOC-OP-205 | OPERATION-MODIFIED | /config | PATCH | shimUpdateConfig | operationId: declared=config.update served=shimUpdateConfig; tags: declared=["config"] served=["Opencode Shim"]; responses: declared=["20... |
| DOC-OP-206 | OPERATION-MODIFIED | /config/providers | GET | shimListConfigProviders | operationId: declared=config.providers served=shimListConfigProviders; tags: declared=["config"] served=["Opencode Shim"]; responses: dec... |
| DOC-OP-207 | OPERATION-MODIFIED | /event | GET | shimEventStream | operationId: declared=event.subscribe served=shimEventStream; tags: declared=["event"] served=["Opencode Shim"]; responses: declared=["20... |
| DOC-OP-208 | OPERATION-MODIFIED | /experimental/tool | GET | shimListTools | operationId: declared=tool.list served=shimListTools; tags: declared=["experimental"] served=["Opencode Shim"]; responses: declared=["200... |
| DOC-OP-209 | OPERATION-MODIFIED | /experimental/tool/ids | GET | shimListToolIDs | operationId: declared=tool.ids served=shimListToolIDs; tags: declared=["experimental"] served=["Opencode Shim"]; responses: declared=["20... |
| DOC-OP-210 | OPERATION-MODIFIED | /file/content | GET | shimReadFile | operationId: declared=file.read served=shimReadFile; tags: declared=["file"] served=["Opencode Shim"]; responses: declared=["200", "400"]... |
| DOC-OP-211 | OPERATION-MODIFIED | /file/status | GET | shimFileStatus | operationId: declared=file.status served=shimFileStatus; tags: declared=["file"] served=["Opencode Shim"]; responses: declared=["200", "4... |
| DOC-OP-212 | OPERATION-MODIFIED | /find | GET | shimFindFiles | operationId: declared=find.text served=shimFindFiles; tags: declared=["file"] served=["Opencode Shim"]; responses: declared=["200", "400"... |
| DOC-OP-213 | OPERATION-MODIFIED | /find/file | GET | shimFindFilesSub | operationId: declared=find.files served=shimFindFilesSub; tags: declared=["file"] served=["Opencode Shim"]; responses: declared=["200", "... |
| DOC-OP-214 | OPERATION-MODIFIED | /find/symbol | GET | shimFindSymbolStub | operationId: declared=find.symbols served=shimFindSymbolStub; tags: declared=["file"] served=["Opencode Shim"]; responses: declared=["200... |
| DOC-OP-215 | OPERATION-MODIFIED | /global/event | GET | shimGlobalEvent | operationId: declared=global.event served=shimGlobalEvent; tags: declared=["global"] served=["Opencode Shim"]; responses: declared=["200"... |
| DOC-OP-216 | OPERATION-MODIFIED | /global/health | GET | shimGlobalHealth | operationId: declared=global.health served=shimGlobalHealth; tags: declared=["global"] served=["Opencode Shim"]; responses: declared=["20... |
| DOC-OP-217 | OPERATION-MODIFIED | /lsp | GET | shimLSPStatus | operationId: declared=lsp.status served=shimLSPStatus; tags: declared=["instance"] served=["Opencode Shim"]; responses: declared=["200", ... |
| DOC-OP-218 | OPERATION-MODIFIED | /mcp | GET | mcpStreamableGet | operationId: declared=mcp.status served=mcpStreamableGet; tags: declared=["mcp"] served=["MCP"]; responses: declared=["200", "400"] serve... |
| DOC-OP-219 | OPERATION-MODIFIED | /mcp | POST | mcpStreamablePost | operationId: declared=mcp.add served=mcpStreamablePost; tags: declared=["mcp"] served=["MCP"]; responses: declared=["200", "400"] served=... |
| DOC-OP-220 | OPERATION-MODIFIED | /permission | GET | shimListPermissions | operationId: declared=permission.list served=shimListPermissions; tags: declared=["permission"] served=["Opencode Shim"]; responses: decl... |
| DOC-OP-221 | OPERATION-MODIFIED | /project | GET | shimProjectList | operationId: declared=project.list served=shimProjectList; tags: declared=["project"] served=["Opencode Shim"]; responses: declared=["200... |
| DOC-OP-222 | OPERATION-REMOVED | /project/{projectID} | PATCH | project.update | path is declared on both sides; this method is declared upstream only |
| DOC-OP-223 | OPERATION-ADDED | /project/{projectId} | GET | shimProjectItem | path is declared on both sides; this method is served only |
| DOC-OP-224 | OPERATION-MODIFIED | /provider | GET | shimGetProvider | operationId: declared=provider.list served=shimGetProvider; tags: declared=["provider"] served=["Opencode Shim"]; responses: declared=["2... |
| DOC-OP-225 | OPERATION-MODIFIED | /session | GET | shimListSessions | operationId: declared=session.list served=shimListSessions; tags: declared=["session"] served=["Opencode Shim"]; responses: declared=["20... |
| DOC-OP-226 | OPERATION-MODIFIED | /session | POST | shimCreateSession | operationId: declared=session.create served=shimCreateSession; tags: declared=["session"] served=["Opencode Shim"]; responses: declared=[... |
| DOC-OP-227 | OPERATION-MODIFIED | /session/{sessionID} | DELETE | shimDeleteSession | operationId: declared=session.delete served=shimDeleteSession; tags: declared=["session"] served=["Opencode Shim"]; responses: declared=[... |
| DOC-OP-228 | OPERATION-MODIFIED | /session/{sessionID} | GET | shimGetSession | operationId: declared=session.get served=shimGetSession; tags: declared=["session"] served=["Opencode Shim"]; responses: declared=["200",... |
| DOC-OP-229 | OPERATION-MODIFIED | /session/{sessionID} | PATCH | shimUpdateSession | operationId: declared=session.update served=shimUpdateSession; tags: declared=["session"] served=["Opencode Shim"]; responses: declared=[... |
| DOC-OP-230 | OPERATION-MODIFIED | /session/{sessionID}/abort | POST | shimAbortSession | operationId: declared=session.abort served=shimAbortSession; tags: declared=["session"] served=["Opencode Shim"]; responses: declared=["2... |
| DOC-OP-231 | OPERATION-MODIFIED | /session/{sessionID}/children | GET | shimListSessionChildren | operationId: declared=session.children served=shimListSessionChildren; tags: declared=["session"] served=["Opencode Shim"]; responses: de... |
| DOC-OP-232 | OPERATION-MODIFIED | /session/{sessionID}/message | GET | shimListMessages | operationId: declared=session.messages served=shimListMessages; tags: declared=["session"] served=["Opencode Shim"]; responses: declared=... |
| DOC-OP-233 | OPERATION-MODIFIED | /session/{sessionID}/message | POST | shimSendMessage | operationId: declared=session.prompt served=shimSendMessage; tags: declared=["session"] served=["Opencode Shim"]; responses: declared=["2... |
| DOC-OP-234 | OPERATION-REMOVED | /session/{sessionID}/message/{messageID} | DELETE | session.deleteMessage | path is declared on both sides; this method is declared upstream only |
| DOC-OP-235 | OPERATION-MODIFIED | /session/{sessionID}/message/{messageID} | GET | shimGetMessage | operationId: declared=session.message served=shimGetMessage; tags: declared=["session"] served=["Opencode Shim"]; responses: declared=["2... |
| DOC-OP-236 | OPERATION-MODIFIED | /vcs | GET | shimVCS | operationId: declared=vcs.get served=shimVCS; tags: declared=["instance"] served=["Opencode Shim"]; responses: declared=["200", "400"] se... |
| DOC-OP-237 | OPERATION-MODIFIED | /vcs/diff | GET | shimVCSDiff | operationId: declared=vcs.diff served=shimVCSDiff; tags: declared=["instance"] served=["Opencode Shim"]; responses: declared=["200", "400... |


## Schemas added / removed / modified

| id | class | schema | changes | detail |
|---|---|---|---|---|
| DOC-SCH-001 | SCHEMA-REMOVED | APIError | 0 | declared upstream, absent from the served document |
| DOC-SCH-002 | SCHEMA-REMOVED | Agent | 0 | declared upstream, absent from the served document |
| DOC-SCH-003 | SCHEMA-REMOVED | AgentColor | 0 | declared upstream, absent from the served document |
| DOC-SCH-004 | SCHEMA-REMOVED | AgentConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-005 | SCHEMA-REMOVED | AgentPart | 0 | declared upstream, absent from the served document |
| DOC-SCH-006 | SCHEMA-REMOVED | AgentPartInput | 0 | declared upstream, absent from the served document |
| DOC-SCH-007 | SCHEMA-REMOVED | AgentV2Info | 0 | declared upstream, absent from the served document |
| DOC-SCH-008 | SCHEMA-REMOVED | ApiAuth | 0 | declared upstream, absent from the served document |
| DOC-SCH-009 | SCHEMA-REMOVED | AssistantMessage | 0 | declared upstream, absent from the served document |
| DOC-SCH-010 | SCHEMA-REMOVED | AttachmentConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-011 | SCHEMA-REMOVED | Auth | 0 | declared upstream, absent from the served document |
| DOC-SCH-012 | SCHEMA-REMOVED | BadRequestError | 0 | declared upstream, absent from the served document |
| DOC-SCH-013 | SCHEMA-REMOVED | CatalogUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-014 | SCHEMA-REMOVED | Command | 0 | declared upstream, absent from the served document |
| DOC-SCH-015 | SCHEMA-REMOVED | CommandExecuted | 0 | declared upstream, absent from the served document |
| DOC-SCH-016 | SCHEMA-REMOVED | CommandV2Info | 0 | declared upstream, absent from the served document |
| DOC-SCH-017 | SCHEMA-REMOVED | CompactionPart | 0 | declared upstream, absent from the served document |
| DOC-SCH-018 | SCHEMA-REMOVED | ConfigV2ExperimentalPolicy | 0 | declared upstream, absent from the served document |
| DOC-SCH-019 | SCHEMA-REMOVED | ConfigV2ReferenceGit | 0 | declared upstream, absent from the served document |
| DOC-SCH-020 | SCHEMA-REMOVED | ConfigV2ReferenceLocal | 0 | declared upstream, absent from the served document |
| DOC-SCH-021 | SCHEMA-REMOVED | ConflictError | 0 | declared upstream, absent from the served document |
| DOC-SCH-022 | SCHEMA-REMOVED | ConnectionCredentialInfo | 0 | declared upstream, absent from the served document |
| DOC-SCH-023 | SCHEMA-REMOVED | ConnectionEnvInfo | 0 | declared upstream, absent from the served document |
| DOC-SCH-024 | SCHEMA-REMOVED | ConnectionInfo | 0 | declared upstream, absent from the served document |
| DOC-SCH-025 | SCHEMA-REMOVED | ConsoleState | 0 | declared upstream, absent from the served document |
| DOC-SCH-026 | SCHEMA-REMOVED | ContentFilterError | 0 | declared upstream, absent from the served document |
| DOC-SCH-027 | SCHEMA-REMOVED | ContextOverflowError | 0 | declared upstream, absent from the served document |
| DOC-SCH-028 | SCHEMA-REMOVED | CredentialKey | 0 | declared upstream, absent from the served document |
| DOC-SCH-029 | SCHEMA-REMOVED | CredentialOAuth | 0 | declared upstream, absent from the served document |
| DOC-SCH-030 | SCHEMA-REMOVED | CredentialValue | 0 | declared upstream, absent from the served document |
| DOC-SCH-031 | SCHEMA-REMOVED | Event | 0 | declared upstream, absent from the served document |
| DOC-SCH-032 | SCHEMA-REMOVED | Event.tui.command.execute | 0 | declared upstream, absent from the served document |
| DOC-SCH-033 | SCHEMA-REMOVED | Event.tui.prompt.append | 0 | declared upstream, absent from the served document |
| DOC-SCH-034 | SCHEMA-REMOVED | Event.tui.session.select | 0 | declared upstream, absent from the served document |
| DOC-SCH-035 | SCHEMA-REMOVED | Event.tui.toast.show | 0 | declared upstream, absent from the served document |
| DOC-SCH-036 | SCHEMA-REMOVED | EventCatalogUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-037 | SCHEMA-REMOVED | EventCommandExecuted | 0 | declared upstream, absent from the served document |
| DOC-SCH-038 | SCHEMA-REMOVED | EventFileEdited | 0 | declared upstream, absent from the served document |
| DOC-SCH-039 | SCHEMA-REMOVED | EventFileWatcherUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-040 | SCHEMA-REMOVED | EventGlobalDisposed | 0 | declared upstream, absent from the served document |
| DOC-SCH-041 | SCHEMA-REMOVED | EventInstallationUpdate-available | 0 | declared upstream, absent from the served document |
| DOC-SCH-042 | SCHEMA-REMOVED | EventInstallationUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-043 | SCHEMA-REMOVED | EventIntegrationConnectionUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-044 | SCHEMA-REMOVED | EventIntegrationUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-045 | SCHEMA-REMOVED | EventLspUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-046 | SCHEMA-REMOVED | EventMcpBrowserOpenFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-047 | SCHEMA-REMOVED | EventMcpToolsChanged | 0 | declared upstream, absent from the served document |
| DOC-SCH-048 | SCHEMA-REMOVED | EventMessagePartDelta | 0 | declared upstream, absent from the served document |
| DOC-SCH-049 | SCHEMA-REMOVED | EventMessagePartRemoved | 0 | declared upstream, absent from the served document |
| DOC-SCH-050 | SCHEMA-REMOVED | EventMessagePartUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-051 | SCHEMA-REMOVED | EventMessageRemoved | 0 | declared upstream, absent from the served document |
| DOC-SCH-052 | SCHEMA-REMOVED | EventMessageUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-053 | SCHEMA-REMOVED | EventModels-devRefreshed | 0 | declared upstream, absent from the served document |
| DOC-SCH-054 | SCHEMA-REMOVED | EventPermissionAsked | 0 | declared upstream, absent from the served document |
| DOC-SCH-055 | SCHEMA-REMOVED | EventPermissionReplied | 0 | declared upstream, absent from the served document |
| DOC-SCH-056 | SCHEMA-REMOVED | EventPermissionV2Asked | 0 | declared upstream, absent from the served document |
| DOC-SCH-057 | SCHEMA-REMOVED | EventPermissionV2Replied | 0 | declared upstream, absent from the served document |
| DOC-SCH-058 | SCHEMA-REMOVED | EventPluginAdded | 0 | declared upstream, absent from the served document |
| DOC-SCH-059 | SCHEMA-REMOVED | EventProjectDirectoriesUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-060 | SCHEMA-REMOVED | EventProjectUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-061 | SCHEMA-REMOVED | EventPtyCreated | 0 | declared upstream, absent from the served document |
| DOC-SCH-062 | SCHEMA-REMOVED | EventPtyDeleted | 0 | declared upstream, absent from the served document |
| DOC-SCH-063 | SCHEMA-REMOVED | EventPtyExited | 0 | declared upstream, absent from the served document |
| DOC-SCH-064 | SCHEMA-REMOVED | EventPtyUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-065 | SCHEMA-REMOVED | EventQuestionAsked | 0 | declared upstream, absent from the served document |
| DOC-SCH-066 | SCHEMA-REMOVED | EventQuestionRejected | 0 | declared upstream, absent from the served document |
| DOC-SCH-067 | SCHEMA-REMOVED | EventQuestionReplied | 0 | declared upstream, absent from the served document |
| DOC-SCH-068 | SCHEMA-REMOVED | EventQuestionV2Asked | 0 | declared upstream, absent from the served document |
| DOC-SCH-069 | SCHEMA-REMOVED | EventQuestionV2Rejected | 0 | declared upstream, absent from the served document |
| DOC-SCH-070 | SCHEMA-REMOVED | EventQuestionV2Replied | 0 | declared upstream, absent from the served document |
| DOC-SCH-071 | SCHEMA-REMOVED | EventReferenceUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-072 | SCHEMA-REMOVED | EventServerConnected | 0 | declared upstream, absent from the served document |
| DOC-SCH-073 | SCHEMA-REMOVED | EventServerInstanceDisposed | 0 | declared upstream, absent from the served document |
| DOC-SCH-074 | SCHEMA-REMOVED | EventSessionCompacted | 0 | declared upstream, absent from the served document |
| DOC-SCH-075 | SCHEMA-REMOVED | EventSessionCreated | 0 | declared upstream, absent from the served document |
| DOC-SCH-076 | SCHEMA-REMOVED | EventSessionDeleted | 0 | declared upstream, absent from the served document |
| DOC-SCH-077 | SCHEMA-REMOVED | EventSessionDiff | 0 | declared upstream, absent from the served document |
| DOC-SCH-078 | SCHEMA-REMOVED | EventSessionError | 0 | declared upstream, absent from the served document |
| DOC-SCH-079 | SCHEMA-REMOVED | EventSessionIdle | 0 | declared upstream, absent from the served document |
| DOC-SCH-080 | SCHEMA-REMOVED | EventSessionNextAgentSwitched | 0 | declared upstream, absent from the served document |
| DOC-SCH-081 | SCHEMA-REMOVED | EventSessionNextCompactionDelta | 0 | declared upstream, absent from the served document |
| DOC-SCH-082 | SCHEMA-REMOVED | EventSessionNextCompactionEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-083 | SCHEMA-REMOVED | EventSessionNextCompactionStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-084 | SCHEMA-REMOVED | EventSessionNextContextUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-085 | SCHEMA-REMOVED | EventSessionNextModelSwitched | 0 | declared upstream, absent from the served document |
| DOC-SCH-086 | SCHEMA-REMOVED | EventSessionNextMoved | 0 | declared upstream, absent from the served document |
| DOC-SCH-087 | SCHEMA-REMOVED | EventSessionNextPromptAdmitted | 0 | declared upstream, absent from the served document |
| DOC-SCH-088 | SCHEMA-REMOVED | EventSessionNextPrompted | 0 | declared upstream, absent from the served document |
| DOC-SCH-089 | SCHEMA-REMOVED | EventSessionNextReasoningDelta | 0 | declared upstream, absent from the served document |
| DOC-SCH-090 | SCHEMA-REMOVED | EventSessionNextReasoningEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-091 | SCHEMA-REMOVED | EventSessionNextReasoningStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-092 | SCHEMA-REMOVED | EventSessionNextRetried | 0 | declared upstream, absent from the served document |
| DOC-SCH-093 | SCHEMA-REMOVED | EventSessionNextRevertCleared | 0 | declared upstream, absent from the served document |
| DOC-SCH-094 | SCHEMA-REMOVED | EventSessionNextRevertCommitted | 0 | declared upstream, absent from the served document |
| DOC-SCH-095 | SCHEMA-REMOVED | EventSessionNextRevertStaged | 0 | declared upstream, absent from the served document |
| DOC-SCH-096 | SCHEMA-REMOVED | EventSessionNextShellEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-097 | SCHEMA-REMOVED | EventSessionNextShellStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-098 | SCHEMA-REMOVED | EventSessionNextStepEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-099 | SCHEMA-REMOVED | EventSessionNextStepFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-100 | SCHEMA-REMOVED | EventSessionNextStepStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-101 | SCHEMA-REMOVED | EventSessionNextSynthetic | 0 | declared upstream, absent from the served document |
| DOC-SCH-102 | SCHEMA-REMOVED | EventSessionNextTextDelta | 0 | declared upstream, absent from the served document |
| DOC-SCH-103 | SCHEMA-REMOVED | EventSessionNextTextEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-104 | SCHEMA-REMOVED | EventSessionNextTextStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-105 | SCHEMA-REMOVED | EventSessionNextToolCalled | 0 | declared upstream, absent from the served document |
| DOC-SCH-106 | SCHEMA-REMOVED | EventSessionNextToolFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-107 | SCHEMA-REMOVED | EventSessionNextToolInputDelta | 0 | declared upstream, absent from the served document |
| DOC-SCH-108 | SCHEMA-REMOVED | EventSessionNextToolInputEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-109 | SCHEMA-REMOVED | EventSessionNextToolInputStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-110 | SCHEMA-REMOVED | EventSessionNextToolProgress | 0 | declared upstream, absent from the served document |
| DOC-SCH-111 | SCHEMA-REMOVED | EventSessionNextToolSuccess | 0 | declared upstream, absent from the served document |
| DOC-SCH-112 | SCHEMA-REMOVED | EventSessionStatus | 0 | declared upstream, absent from the served document |
| DOC-SCH-113 | SCHEMA-REMOVED | EventSessionUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-114 | SCHEMA-REMOVED | EventTodoUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-115 | SCHEMA-REMOVED | EventTuiCommandExecute | 0 | declared upstream, absent from the served document |
| DOC-SCH-116 | SCHEMA-REMOVED | EventTuiPromptAppend | 0 | declared upstream, absent from the served document |
| DOC-SCH-117 | SCHEMA-REMOVED | EventTuiSessionSelect | 0 | declared upstream, absent from the served document |
| DOC-SCH-118 | SCHEMA-REMOVED | EventTuiToastShow | 0 | declared upstream, absent from the served document |
| DOC-SCH-119 | SCHEMA-REMOVED | EventVcsBranchUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-120 | SCHEMA-REMOVED | EventWorkspaceFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-121 | SCHEMA-REMOVED | EventWorkspaceReady | 0 | declared upstream, absent from the served document |
| DOC-SCH-122 | SCHEMA-REMOVED | EventWorkspaceStatus | 0 | declared upstream, absent from the served document |
| DOC-SCH-123 | SCHEMA-REMOVED | EventWorktreeFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-124 | SCHEMA-REMOVED | EventWorktreeReady | 0 | declared upstream, absent from the served document |
| DOC-SCH-125 | SCHEMA-REMOVED | ExperimentalCapabilities | 0 | declared upstream, absent from the served document |
| DOC-SCH-126 | SCHEMA-REMOVED | File | 0 | declared upstream, absent from the served document |
| DOC-SCH-127 | SCHEMA-REMOVED | FileContent | 0 | declared upstream, absent from the served document |
| DOC-SCH-128 | SCHEMA-REMOVED | FileDiff | 0 | declared upstream, absent from the served document |
| DOC-SCH-129 | SCHEMA-REMOVED | FileEdited | 0 | declared upstream, absent from the served document |
| DOC-SCH-130 | SCHEMA-REMOVED | FileNode | 0 | declared upstream, absent from the served document |
| DOC-SCH-131 | SCHEMA-REMOVED | FilePart | 0 | declared upstream, absent from the served document |
| DOC-SCH-132 | SCHEMA-REMOVED | FilePartInput | 0 | declared upstream, absent from the served document |
| DOC-SCH-133 | SCHEMA-REMOVED | FilePartSource | 0 | declared upstream, absent from the served document |
| DOC-SCH-134 | SCHEMA-REMOVED | FilePartSourceText | 0 | declared upstream, absent from the served document |
| DOC-SCH-135 | SCHEMA-REMOVED | FileSource | 0 | declared upstream, absent from the served document |
| DOC-SCH-136 | SCHEMA-REMOVED | FileSystemEntry | 0 | declared upstream, absent from the served document |
| DOC-SCH-137 | SCHEMA-REMOVED | FileWatcherUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-138 | SCHEMA-REMOVED | ForbiddenError | 0 | declared upstream, absent from the served document |
| DOC-SCH-139 | SCHEMA-REMOVED | FormatterStatus | 0 | declared upstream, absent from the served document |
| DOC-SCH-140 | SCHEMA-REMOVED | GlobalDisposed | 0 | declared upstream, absent from the served document |
| DOC-SCH-141 | SCHEMA-REMOVED | GlobalEvent | 0 | declared upstream, absent from the served document |
| DOC-SCH-142 | SCHEMA-REMOVED | GlobalSession | 0 | declared upstream, absent from the served document |
| DOC-SCH-143 | SCHEMA-REMOVED | ImageAttachmentConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-144 | SCHEMA-REMOVED | InstallationUpdate-available | 0 | declared upstream, absent from the served document |
| DOC-SCH-145 | SCHEMA-REMOVED | InstallationUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-146 | SCHEMA-REMOVED | IntegrationAttempt | 0 | declared upstream, absent from the served document |
| DOC-SCH-147 | SCHEMA-REMOVED | IntegrationAttemptStatus | 0 | declared upstream, absent from the served document |
| DOC-SCH-148 | SCHEMA-REMOVED | IntegrationConnectionUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-149 | SCHEMA-REMOVED | IntegrationEnvMethod | 0 | declared upstream, absent from the served document |
| DOC-SCH-150 | SCHEMA-REMOVED | IntegrationInfo | 0 | declared upstream, absent from the served document |
| DOC-SCH-151 | SCHEMA-REMOVED | IntegrationInputs | 0 | declared upstream, absent from the served document |
| DOC-SCH-152 | SCHEMA-REMOVED | IntegrationKeyMethod | 0 | declared upstream, absent from the served document |
| DOC-SCH-153 | SCHEMA-REMOVED | IntegrationMethod | 0 | declared upstream, absent from the served document |
| DOC-SCH-154 | SCHEMA-REMOVED | IntegrationOAuthMethod | 0 | declared upstream, absent from the served document |
| DOC-SCH-155 | SCHEMA-REMOVED | IntegrationRef | 0 | declared upstream, absent from the served document |
| DOC-SCH-156 | SCHEMA-REMOVED | IntegrationSelectPrompt | 0 | declared upstream, absent from the served document |
| DOC-SCH-157 | SCHEMA-REMOVED | IntegrationTextPrompt | 0 | declared upstream, absent from the served document |
| DOC-SCH-158 | SCHEMA-REMOVED | IntegrationUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-159 | SCHEMA-REMOVED | IntegrationWhen | 0 | declared upstream, absent from the served document |
| DOC-SCH-160 | SCHEMA-REMOVED | InvalidCursorError | 0 | declared upstream, absent from the served document |
| DOC-SCH-161 | SCHEMA-REMOVED | InvalidRequestError | 0 | declared upstream, absent from the served document |
| DOC-SCH-162 | SCHEMA-REMOVED | JSONSchema | 0 | declared upstream, absent from the served document |
| DOC-SCH-163 | SCHEMA-REMOVED | LLMProviderMetadata | 0 | declared upstream, absent from the served document |
| DOC-SCH-164 | SCHEMA-REMOVED | LLMToolContent | 0 | declared upstream, absent from the served document |
| DOC-SCH-165 | SCHEMA-REMOVED | LSPStatus | 0 | declared upstream, absent from the served document |
| DOC-SCH-166 | SCHEMA-REMOVED | LayoutConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-167 | SCHEMA-REMOVED | LocationInfo | 0 | declared upstream, absent from the served document |
| DOC-SCH-168 | SCHEMA-REMOVED | LocationRef | 0 | declared upstream, absent from the served document |
| DOC-SCH-169 | SCHEMA-REMOVED | LogLevel | 0 | declared upstream, absent from the served document |
| DOC-SCH-170 | SCHEMA-REMOVED | LspUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-171 | SCHEMA-REMOVED | MCPStatus | 0 | declared upstream, absent from the served document |
| DOC-SCH-172 | SCHEMA-REMOVED | MCPStatusConnected | 0 | declared upstream, absent from the served document |
| DOC-SCH-173 | SCHEMA-REMOVED | MCPStatusDisabled | 0 | declared upstream, absent from the served document |
| DOC-SCH-174 | SCHEMA-REMOVED | MCPStatusFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-175 | SCHEMA-REMOVED | MCPStatusNeedsAuth | 0 | declared upstream, absent from the served document |
| DOC-SCH-176 | SCHEMA-REMOVED | MCPStatusNeedsClientRegistration | 0 | declared upstream, absent from the served document |
| DOC-SCH-177 | SCHEMA-REMOVED | McpBrowserOpenFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-178 | SCHEMA-REMOVED | McpLocalConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-179 | SCHEMA-REMOVED | McpOAuthConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-180 | SCHEMA-REMOVED | McpRemoteConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-181 | SCHEMA-REMOVED | McpResource | 0 | declared upstream, absent from the served document |
| DOC-SCH-182 | SCHEMA-REMOVED | McpServerNotFoundError | 0 | declared upstream, absent from the served document |
| DOC-SCH-183 | SCHEMA-REMOVED | McpToolsChanged | 0 | declared upstream, absent from the served document |
| DOC-SCH-184 | SCHEMA-REMOVED | McpUnsupportedOAuthError | 0 | declared upstream, absent from the served document |
| DOC-SCH-185 | SCHEMA-REMOVED | Message | 0 | declared upstream, absent from the served document |
| DOC-SCH-186 | SCHEMA-REMOVED | MessageAbortedError | 0 | declared upstream, absent from the served document |
| DOC-SCH-187 | SCHEMA-REMOVED | MessageNotFoundError | 0 | declared upstream, absent from the served document |
| DOC-SCH-188 | SCHEMA-REMOVED | MessageOutputLengthError | 0 | declared upstream, absent from the served document |
| DOC-SCH-189 | SCHEMA-REMOVED | MessagePartDelta | 0 | declared upstream, absent from the served document |
| DOC-SCH-190 | SCHEMA-REMOVED | MessagePartRemoved | 0 | declared upstream, absent from the served document |
| DOC-SCH-191 | SCHEMA-REMOVED | MessagePartUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-192 | SCHEMA-REMOVED | MessageRemoved | 0 | declared upstream, absent from the served document |
| DOC-SCH-193 | SCHEMA-REMOVED | MessageUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-194 | SCHEMA-REMOVED | Model | 0 | declared upstream, absent from the served document |
| DOC-SCH-195 | SCHEMA-REMOVED | ModelApi | 0 | declared upstream, absent from the served document |
| DOC-SCH-196 | SCHEMA-REMOVED | ModelCapabilities | 0 | declared upstream, absent from the served document |
| DOC-SCH-197 | SCHEMA-REMOVED | ModelCost | 0 | declared upstream, absent from the served document |
| DOC-SCH-198 | SCHEMA-REMOVED | ModelRef | 0 | declared upstream, absent from the served document |
| DOC-SCH-199 | SCHEMA-REMOVED | ModelV2Info | 0 | declared upstream, absent from the served document |
| DOC-SCH-200 | SCHEMA-REMOVED | Models-devRefreshed | 0 | declared upstream, absent from the served document |
| DOC-SCH-201 | SCHEMA-REMOVED | MoveSessionDestination | 0 | declared upstream, absent from the served document |
| DOC-SCH-202 | SCHEMA-REMOVED | MoveSessionError | 0 | declared upstream, absent from the served document |
| DOC-SCH-203 | SCHEMA-REMOVED | NotFoundError | 0 | declared upstream, absent from the served document |
| DOC-SCH-204 | SCHEMA-REMOVED | OAuth | 0 | declared upstream, absent from the served document |
| DOC-SCH-205 | SCHEMA-REMOVED | OutputFormat | 0 | declared upstream, absent from the served document |
| DOC-SCH-206 | SCHEMA-REMOVED | OutputFormat1 | 0 | declared upstream, absent from the served document |
| DOC-SCH-207 | SCHEMA-REMOVED | OutputFormatJsonSchema | 0 | declared upstream, absent from the served document |
| DOC-SCH-208 | SCHEMA-REMOVED | OutputFormatText | 0 | declared upstream, absent from the served document |
| DOC-SCH-209 | SCHEMA-REMOVED | Part | 0 | declared upstream, absent from the served document |
| DOC-SCH-210 | SCHEMA-REMOVED | PatchPart | 0 | declared upstream, absent from the served document |
| DOC-SCH-211 | SCHEMA-REMOVED | Path | 0 | declared upstream, absent from the served document |
| DOC-SCH-212 | SCHEMA-REMOVED | PermissionAction | 0 | declared upstream, absent from the served document |
| DOC-SCH-213 | SCHEMA-REMOVED | PermissionActionConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-214 | SCHEMA-REMOVED | PermissionAsked | 0 | declared upstream, absent from the served document |
| DOC-SCH-215 | SCHEMA-REMOVED | PermissionConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-216 | SCHEMA-REMOVED | PermissionNotFoundError | 0 | declared upstream, absent from the served document |
| DOC-SCH-217 | SCHEMA-REMOVED | PermissionObjectConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-218 | SCHEMA-REMOVED | PermissionReplied | 0 | declared upstream, absent from the served document |
| DOC-SCH-219 | SCHEMA-REMOVED | PermissionRequest | 0 | declared upstream, absent from the served document |
| DOC-SCH-220 | SCHEMA-REMOVED | PermissionRule | 0 | declared upstream, absent from the served document |
| DOC-SCH-221 | SCHEMA-REMOVED | PermissionRuleConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-222 | SCHEMA-REMOVED | PermissionRuleset | 0 | declared upstream, absent from the served document |
| DOC-SCH-223 | SCHEMA-REMOVED | PermissionSavedInfo | 0 | declared upstream, absent from the served document |
| DOC-SCH-224 | SCHEMA-REMOVED | PermissionV2Asked | 0 | declared upstream, absent from the served document |
| DOC-SCH-225 | SCHEMA-REMOVED | PermissionV2Effect | 0 | declared upstream, absent from the served document |
| DOC-SCH-226 | SCHEMA-REMOVED | PermissionV2Replied | 0 | declared upstream, absent from the served document |
| DOC-SCH-227 | SCHEMA-REMOVED | PermissionV2Reply | 0 | declared upstream, absent from the served document |
| DOC-SCH-228 | SCHEMA-REMOVED | PermissionV2Request | 0 | declared upstream, absent from the served document |
| DOC-SCH-229 | SCHEMA-REMOVED | PermissionV2Rule | 0 | declared upstream, absent from the served document |
| DOC-SCH-230 | SCHEMA-REMOVED | PermissionV2Ruleset | 0 | declared upstream, absent from the served document |
| DOC-SCH-231 | SCHEMA-REMOVED | PermissionV2Source | 0 | declared upstream, absent from the served document |
| DOC-SCH-232 | SCHEMA-REMOVED | PluginAdded | 0 | declared upstream, absent from the served document |
| DOC-SCH-233 | SCHEMA-REMOVED | PolicyEffect | 0 | declared upstream, absent from the served document |
| DOC-SCH-234 | SCHEMA-REMOVED | Project | 0 | declared upstream, absent from the served document |
| DOC-SCH-235 | SCHEMA-REMOVED | ProjectCommands | 0 | declared upstream, absent from the served document |
| DOC-SCH-236 | SCHEMA-REMOVED | ProjectCopyCopy | 0 | declared upstream, absent from the served document |
| DOC-SCH-237 | SCHEMA-REMOVED | ProjectCopyError | 0 | declared upstream, absent from the served document |
| DOC-SCH-238 | SCHEMA-REMOVED | ProjectDirectories | 0 | declared upstream, absent from the served document |
| DOC-SCH-239 | SCHEMA-REMOVED | ProjectDirectoriesUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-240 | SCHEMA-REMOVED | ProjectIcon | 0 | declared upstream, absent from the served document |
| DOC-SCH-241 | SCHEMA-REMOVED | ProjectNotFoundError | 0 | declared upstream, absent from the served document |
| DOC-SCH-242 | SCHEMA-REMOVED | ProjectSummary | 0 | declared upstream, absent from the served document |
| DOC-SCH-243 | SCHEMA-REMOVED | ProjectTime | 0 | declared upstream, absent from the served document |
| DOC-SCH-244 | SCHEMA-REMOVED | ProjectUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-245 | SCHEMA-REMOVED | ProjectVcs | 0 | declared upstream, absent from the served document |
| DOC-SCH-246 | SCHEMA-REMOVED | Prompt | 0 | declared upstream, absent from the served document |
| DOC-SCH-247 | SCHEMA-REMOVED | PromptAgentAttachment | 0 | declared upstream, absent from the served document |
| DOC-SCH-248 | SCHEMA-REMOVED | PromptFileAttachment | 0 | declared upstream, absent from the served document |
| DOC-SCH-249 | SCHEMA-REMOVED | PromptInput | 0 | declared upstream, absent from the served document |
| DOC-SCH-250 | SCHEMA-REMOVED | PromptInputFileAttachment | 0 | declared upstream, absent from the served document |
| DOC-SCH-251 | SCHEMA-REMOVED | PromptSource | 0 | declared upstream, absent from the served document |
| DOC-SCH-252 | SCHEMA-REMOVED | Provider | 0 | declared upstream, absent from the served document |
| DOC-SCH-253 | SCHEMA-REMOVED | ProviderAISDK | 0 | declared upstream, absent from the served document |
| DOC-SCH-254 | SCHEMA-REMOVED | ProviderApi | 0 | declared upstream, absent from the served document |
| DOC-SCH-255 | SCHEMA-REMOVED | ProviderAuthAuthorization | 0 | declared upstream, absent from the served document |
| DOC-SCH-256 | SCHEMA-REMOVED | ProviderAuthError | 0 | declared upstream, absent from the served document |
| DOC-SCH-257 | SCHEMA-REMOVED | ProviderAuthError1 | 0 | declared upstream, absent from the served document |
| DOC-SCH-258 | SCHEMA-REMOVED | ProviderAuthMethod | 0 | declared upstream, absent from the served document |
| DOC-SCH-259 | SCHEMA-REMOVED | ProviderConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-260 | SCHEMA-REMOVED | ProviderNative | 0 | declared upstream, absent from the served document |
| DOC-SCH-261 | SCHEMA-REMOVED | ProviderNotFoundError | 0 | declared upstream, absent from the served document |
| DOC-SCH-262 | SCHEMA-REMOVED | ProviderRequest | 0 | declared upstream, absent from the served document |
| DOC-SCH-263 | SCHEMA-REMOVED | ProviderV2Info | 0 | declared upstream, absent from the served document |
| DOC-SCH-264 | SCHEMA-REMOVED | Pty | 0 | declared upstream, absent from the served document |
| DOC-SCH-265 | SCHEMA-REMOVED | PtyCreated | 0 | declared upstream, absent from the served document |
| DOC-SCH-266 | SCHEMA-REMOVED | PtyDeleted | 0 | declared upstream, absent from the served document |
| DOC-SCH-267 | SCHEMA-REMOVED | PtyExited | 0 | declared upstream, absent from the served document |
| DOC-SCH-268 | SCHEMA-REMOVED | PtyForbiddenError | 0 | declared upstream, absent from the served document |
| DOC-SCH-269 | SCHEMA-REMOVED | PtyNotFoundError | 0 | declared upstream, absent from the served document |
| DOC-SCH-270 | SCHEMA-REMOVED | PtyTicketConnectToken | 0 | declared upstream, absent from the served document |
| DOC-SCH-271 | SCHEMA-REMOVED | PtyUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-272 | SCHEMA-REMOVED | QuestionAnswer | 0 | declared upstream, absent from the served document |
| DOC-SCH-273 | SCHEMA-REMOVED | QuestionAsked | 0 | declared upstream, absent from the served document |
| DOC-SCH-274 | SCHEMA-REMOVED | QuestionInfo | 0 | declared upstream, absent from the served document |
| DOC-SCH-275 | SCHEMA-REMOVED | QuestionNotFoundError | 0 | declared upstream, absent from the served document |
| DOC-SCH-276 | SCHEMA-REMOVED | QuestionOption | 0 | declared upstream, absent from the served document |
| DOC-SCH-277 | SCHEMA-REMOVED | QuestionRejected | 0 | declared upstream, absent from the served document |
| DOC-SCH-278 | SCHEMA-REMOVED | QuestionReplied | 0 | declared upstream, absent from the served document |
| DOC-SCH-279 | SCHEMA-REMOVED | QuestionRequest | 0 | declared upstream, absent from the served document |
| DOC-SCH-280 | SCHEMA-REMOVED | QuestionTool | 0 | declared upstream, absent from the served document |
| DOC-SCH-281 | SCHEMA-REMOVED | QuestionV2Answer | 0 | declared upstream, absent from the served document |
| DOC-SCH-282 | SCHEMA-REMOVED | QuestionV2Asked | 0 | declared upstream, absent from the served document |
| DOC-SCH-283 | SCHEMA-REMOVED | QuestionV2Info | 0 | declared upstream, absent from the served document |
| DOC-SCH-284 | SCHEMA-REMOVED | QuestionV2Option | 0 | declared upstream, absent from the served document |
| DOC-SCH-285 | SCHEMA-REMOVED | QuestionV2Rejected | 0 | declared upstream, absent from the served document |
| DOC-SCH-286 | SCHEMA-REMOVED | QuestionV2Replied | 0 | declared upstream, absent from the served document |
| DOC-SCH-287 | SCHEMA-REMOVED | QuestionV2Reply | 0 | declared upstream, absent from the served document |
| DOC-SCH-288 | SCHEMA-REMOVED | QuestionV2Request | 0 | declared upstream, absent from the served document |
| DOC-SCH-289 | SCHEMA-REMOVED | QuestionV2Tool | 0 | declared upstream, absent from the served document |
| DOC-SCH-290 | SCHEMA-REMOVED | Range | 0 | declared upstream, absent from the served document |
| DOC-SCH-291 | SCHEMA-REMOVED | ReasoningPart | 0 | declared upstream, absent from the served document |
| DOC-SCH-292 | SCHEMA-REMOVED | ReferenceGitSource | 0 | declared upstream, absent from the served document |
| DOC-SCH-293 | SCHEMA-REMOVED | ReferenceInfo | 0 | declared upstream, absent from the served document |
| DOC-SCH-294 | SCHEMA-REMOVED | ReferenceLocalSource | 0 | declared upstream, absent from the served document |
| DOC-SCH-295 | SCHEMA-REMOVED | ReferenceSource | 0 | declared upstream, absent from the served document |
| DOC-SCH-296 | SCHEMA-REMOVED | ReferenceUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-297 | SCHEMA-REMOVED | ResourceSource | 0 | declared upstream, absent from the served document |
| DOC-SCH-298 | SCHEMA-REMOVED | RetryPart | 0 | declared upstream, absent from the served document |
| DOC-SCH-299 | SCHEMA-REMOVED | RevertState | 0 | declared upstream, absent from the served document |
| DOC-SCH-300 | SCHEMA-REMOVED | ServerConfig | 0 | declared upstream, absent from the served document |
| DOC-SCH-301 | SCHEMA-REMOVED | ServerConnected | 0 | declared upstream, absent from the served document |
| DOC-SCH-302 | SCHEMA-REMOVED | ServiceUnavailableError | 0 | declared upstream, absent from the served document |
| DOC-SCH-303 | SCHEMA-REMOVED | SessionActive | 0 | declared upstream, absent from the served document |
| DOC-SCH-304 | SCHEMA-REMOVED | SessionBusyError | 0 | declared upstream, absent from the served document |
| DOC-SCH-305 | SCHEMA-REMOVED | SessionCompacted | 0 | declared upstream, absent from the served document |
| DOC-SCH-306 | SCHEMA-REMOVED | SessionCreated | 0 | declared upstream, absent from the served document |
| DOC-SCH-307 | SCHEMA-REMOVED | SessionDeleted | 0 | declared upstream, absent from the served document |
| DOC-SCH-308 | SCHEMA-REMOVED | SessionDiff | 0 | declared upstream, absent from the served document |
| DOC-SCH-309 | SCHEMA-REMOVED | SessionDurableEvent | 0 | declared upstream, absent from the served document |
| DOC-SCH-310 | SCHEMA-REMOVED | SessionDurableEventStream | 0 | declared upstream, absent from the served document |
| DOC-SCH-311 | SCHEMA-REMOVED | SessionError | 0 | declared upstream, absent from the served document |
| DOC-SCH-312 | SCHEMA-REMOVED | SessionErrorUnknown | 0 | declared upstream, absent from the served document |
| DOC-SCH-313 | SCHEMA-REMOVED | SessionHistory | 0 | declared upstream, absent from the served document |
| DOC-SCH-314 | SCHEMA-REMOVED | SessionIdle | 0 | declared upstream, absent from the served document |
| DOC-SCH-315 | SCHEMA-REMOVED | SessionInputAdmitted | 0 | declared upstream, absent from the served document |
| DOC-SCH-316 | SCHEMA-REMOVED | SessionMessage | 0 | declared upstream, absent from the served document |
| DOC-SCH-317 | SCHEMA-REMOVED | SessionMessageAgentSwitched | 0 | declared upstream, absent from the served document |
| DOC-SCH-318 | SCHEMA-REMOVED | SessionMessageAssistant | 0 | declared upstream, absent from the served document |
| DOC-SCH-319 | SCHEMA-REMOVED | SessionMessageAssistantReasoning | 0 | declared upstream, absent from the served document |
| DOC-SCH-320 | SCHEMA-REMOVED | SessionMessageAssistantText | 0 | declared upstream, absent from the served document |
| DOC-SCH-321 | SCHEMA-REMOVED | SessionMessageAssistantTool | 0 | declared upstream, absent from the served document |
| DOC-SCH-322 | SCHEMA-REMOVED | SessionMessageCompaction | 0 | declared upstream, absent from the served document |
| DOC-SCH-323 | SCHEMA-REMOVED | SessionMessageModelSwitched | 0 | declared upstream, absent from the served document |
| DOC-SCH-324 | SCHEMA-REMOVED | SessionMessageShell | 0 | declared upstream, absent from the served document |
| DOC-SCH-325 | SCHEMA-REMOVED | SessionMessageSynthetic | 0 | declared upstream, absent from the served document |
| DOC-SCH-326 | SCHEMA-REMOVED | SessionMessageSystem | 0 | declared upstream, absent from the served document |
| DOC-SCH-327 | SCHEMA-REMOVED | SessionMessageToolStateCompleted | 0 | declared upstream, absent from the served document |
| DOC-SCH-328 | SCHEMA-REMOVED | SessionMessageToolStateError | 0 | declared upstream, absent from the served document |
| DOC-SCH-329 | SCHEMA-REMOVED | SessionMessageToolStatePending | 0 | declared upstream, absent from the served document |
| DOC-SCH-330 | SCHEMA-REMOVED | SessionMessageToolStateRunning | 0 | declared upstream, absent from the served document |
| DOC-SCH-331 | SCHEMA-REMOVED | SessionMessageUser | 0 | declared upstream, absent from the served document |
| DOC-SCH-332 | SCHEMA-REMOVED | SessionMessagesResponse | 0 | declared upstream, absent from the served document |
| DOC-SCH-333 | SCHEMA-REMOVED | SessionNextAgentSwitched | 0 | declared upstream, absent from the served document |
| DOC-SCH-334 | SCHEMA-REMOVED | SessionNextCompactionDelta | 0 | declared upstream, absent from the served document |
| DOC-SCH-335 | SCHEMA-REMOVED | SessionNextCompactionEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-336 | SCHEMA-REMOVED | SessionNextCompactionStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-337 | SCHEMA-REMOVED | SessionNextContextUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-338 | SCHEMA-REMOVED | SessionNextModelSwitched | 0 | declared upstream, absent from the served document |
| DOC-SCH-339 | SCHEMA-REMOVED | SessionNextMoved | 0 | declared upstream, absent from the served document |
| DOC-SCH-340 | SCHEMA-REMOVED | SessionNextPromptAdmitted | 0 | declared upstream, absent from the served document |
| DOC-SCH-341 | SCHEMA-REMOVED | SessionNextPrompted | 0 | declared upstream, absent from the served document |
| DOC-SCH-342 | SCHEMA-REMOVED | SessionNextReasoningDelta | 0 | declared upstream, absent from the served document |
| DOC-SCH-343 | SCHEMA-REMOVED | SessionNextReasoningEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-344 | SCHEMA-REMOVED | SessionNextReasoningStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-345 | SCHEMA-REMOVED | SessionNextRetried | 0 | declared upstream, absent from the served document |
| DOC-SCH-346 | SCHEMA-REMOVED | SessionNextRetry_error | 0 | declared upstream, absent from the served document |
| DOC-SCH-347 | SCHEMA-REMOVED | SessionNextRevertCleared | 0 | declared upstream, absent from the served document |
| DOC-SCH-348 | SCHEMA-REMOVED | SessionNextRevertCommitted | 0 | declared upstream, absent from the served document |
| DOC-SCH-349 | SCHEMA-REMOVED | SessionNextRevertStaged | 0 | declared upstream, absent from the served document |
| DOC-SCH-350 | SCHEMA-REMOVED | SessionNextShellEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-351 | SCHEMA-REMOVED | SessionNextShellStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-352 | SCHEMA-REMOVED | SessionNextStepEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-353 | SCHEMA-REMOVED | SessionNextStepFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-354 | SCHEMA-REMOVED | SessionNextStepStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-355 | SCHEMA-REMOVED | SessionNextSynthetic | 0 | declared upstream, absent from the served document |
| DOC-SCH-356 | SCHEMA-REMOVED | SessionNextTextDelta | 0 | declared upstream, absent from the served document |
| DOC-SCH-357 | SCHEMA-REMOVED | SessionNextTextEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-358 | SCHEMA-REMOVED | SessionNextTextStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-359 | SCHEMA-REMOVED | SessionNextToolCalled | 0 | declared upstream, absent from the served document |
| DOC-SCH-360 | SCHEMA-REMOVED | SessionNextToolFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-361 | SCHEMA-REMOVED | SessionNextToolInputDelta | 0 | declared upstream, absent from the served document |
| DOC-SCH-362 | SCHEMA-REMOVED | SessionNextToolInputEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-363 | SCHEMA-REMOVED | SessionNextToolInputStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-364 | SCHEMA-REMOVED | SessionNextToolProgress | 0 | declared upstream, absent from the served document |
| DOC-SCH-365 | SCHEMA-REMOVED | SessionNextToolSuccess | 0 | declared upstream, absent from the served document |
| DOC-SCH-366 | SCHEMA-REMOVED | SessionNotFoundError | 0 | declared upstream, absent from the served document |
| DOC-SCH-367 | SCHEMA-REMOVED | SessionStatus | 0 | declared upstream, absent from the served document |
| DOC-SCH-368 | SCHEMA-REMOVED | SessionUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-369 | SCHEMA-REMOVED | SessionV2Info | 0 | declared upstream, absent from the served document |
| DOC-SCH-370 | SCHEMA-REMOVED | SessionsResponse | 0 | declared upstream, absent from the served document |
| DOC-SCH-371 | SCHEMA-REMOVED | SkillV2DirectorySource | 0 | declared upstream, absent from the served document |
| DOC-SCH-372 | SCHEMA-REMOVED | SkillV2EmbeddedSource | 0 | declared upstream, absent from the served document |
| DOC-SCH-373 | SCHEMA-REMOVED | SkillV2Info | 0 | declared upstream, absent from the served document |
| DOC-SCH-374 | SCHEMA-REMOVED | SkillV2Source | 0 | declared upstream, absent from the served document |
| DOC-SCH-375 | SCHEMA-REMOVED | SkillV2UrlSource | 0 | declared upstream, absent from the served document |
| DOC-SCH-376 | SCHEMA-REMOVED | SnapshotFileDiff | 0 | declared upstream, absent from the served document |
| DOC-SCH-377 | SCHEMA-REMOVED | SnapshotPart | 0 | declared upstream, absent from the served document |
| DOC-SCH-378 | SCHEMA-REMOVED | StepFinishPart | 0 | declared upstream, absent from the served document |
| DOC-SCH-379 | SCHEMA-REMOVED | StepStartPart | 0 | declared upstream, absent from the served document |
| DOC-SCH-380 | SCHEMA-REMOVED | StructuredOutputError | 0 | declared upstream, absent from the served document |
| DOC-SCH-381 | SCHEMA-REMOVED | SubtaskPart | 0 | declared upstream, absent from the served document |
| DOC-SCH-382 | SCHEMA-REMOVED | SubtaskPartInput | 0 | declared upstream, absent from the served document |
| DOC-SCH-383 | SCHEMA-REMOVED | Symbol | 0 | declared upstream, absent from the served document |
| DOC-SCH-384 | SCHEMA-REMOVED | SymbolSource | 0 | declared upstream, absent from the served document |
| DOC-SCH-385 | SCHEMA-REMOVED | SyncEventMessagePartRemoved | 0 | declared upstream, absent from the served document |
| DOC-SCH-386 | SCHEMA-REMOVED | SyncEventMessagePartUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-387 | SCHEMA-REMOVED | SyncEventMessageRemoved | 0 | declared upstream, absent from the served document |
| DOC-SCH-388 | SCHEMA-REMOVED | SyncEventMessageUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-389 | SCHEMA-REMOVED | SyncEventSessionCreated | 0 | declared upstream, absent from the served document |
| DOC-SCH-390 | SCHEMA-REMOVED | SyncEventSessionDeleted | 0 | declared upstream, absent from the served document |
| DOC-SCH-391 | SCHEMA-REMOVED | SyncEventSessionNextAgentSwitched | 0 | declared upstream, absent from the served document |
| DOC-SCH-392 | SCHEMA-REMOVED | SyncEventSessionNextCompactionEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-393 | SCHEMA-REMOVED | SyncEventSessionNextCompactionStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-394 | SCHEMA-REMOVED | SyncEventSessionNextContextUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-395 | SCHEMA-REMOVED | SyncEventSessionNextModelSwitched | 0 | declared upstream, absent from the served document |
| DOC-SCH-396 | SCHEMA-REMOVED | SyncEventSessionNextMoved | 0 | declared upstream, absent from the served document |
| DOC-SCH-397 | SCHEMA-REMOVED | SyncEventSessionNextPromptAdmitted | 0 | declared upstream, absent from the served document |
| DOC-SCH-398 | SCHEMA-REMOVED | SyncEventSessionNextPrompted | 0 | declared upstream, absent from the served document |
| DOC-SCH-399 | SCHEMA-REMOVED | SyncEventSessionNextReasoningEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-400 | SCHEMA-REMOVED | SyncEventSessionNextReasoningStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-401 | SCHEMA-REMOVED | SyncEventSessionNextRetried | 0 | declared upstream, absent from the served document |
| DOC-SCH-402 | SCHEMA-REMOVED | SyncEventSessionNextRevertCleared | 0 | declared upstream, absent from the served document |
| DOC-SCH-403 | SCHEMA-REMOVED | SyncEventSessionNextRevertCommitted | 0 | declared upstream, absent from the served document |
| DOC-SCH-404 | SCHEMA-REMOVED | SyncEventSessionNextRevertStaged | 0 | declared upstream, absent from the served document |
| DOC-SCH-405 | SCHEMA-REMOVED | SyncEventSessionNextShellEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-406 | SCHEMA-REMOVED | SyncEventSessionNextShellStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-407 | SCHEMA-REMOVED | SyncEventSessionNextStepEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-408 | SCHEMA-REMOVED | SyncEventSessionNextStepFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-409 | SCHEMA-REMOVED | SyncEventSessionNextStepStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-410 | SCHEMA-REMOVED | SyncEventSessionNextSynthetic | 0 | declared upstream, absent from the served document |
| DOC-SCH-411 | SCHEMA-REMOVED | SyncEventSessionNextTextEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-412 | SCHEMA-REMOVED | SyncEventSessionNextTextStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-413 | SCHEMA-REMOVED | SyncEventSessionNextToolCalled | 0 | declared upstream, absent from the served document |
| DOC-SCH-414 | SCHEMA-REMOVED | SyncEventSessionNextToolFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-415 | SCHEMA-REMOVED | SyncEventSessionNextToolInputEnded | 0 | declared upstream, absent from the served document |
| DOC-SCH-416 | SCHEMA-REMOVED | SyncEventSessionNextToolInputStarted | 0 | declared upstream, absent from the served document |
| DOC-SCH-417 | SCHEMA-REMOVED | SyncEventSessionNextToolProgress | 0 | declared upstream, absent from the served document |
| DOC-SCH-418 | SCHEMA-REMOVED | SyncEventSessionNextToolSuccess | 0 | declared upstream, absent from the served document |
| DOC-SCH-419 | SCHEMA-REMOVED | SyncEventSessionUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-420 | SCHEMA-REMOVED | TextPart | 0 | declared upstream, absent from the served document |
| DOC-SCH-421 | SCHEMA-REMOVED | TextPartInput | 0 | declared upstream, absent from the served document |
| DOC-SCH-422 | SCHEMA-REMOVED | Todo | 0 | declared upstream, absent from the served document |
| DOC-SCH-423 | SCHEMA-REMOVED | TodoUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-424 | SCHEMA-REMOVED | ToolFileContent | 0 | declared upstream, absent from the served document |
| DOC-SCH-425 | SCHEMA-REMOVED | ToolIDs | 0 | declared upstream, absent from the served document |
| DOC-SCH-426 | SCHEMA-REMOVED | ToolList | 0 | declared upstream, absent from the served document |
| DOC-SCH-427 | SCHEMA-REMOVED | ToolListItem | 0 | declared upstream, absent from the served document |
| DOC-SCH-428 | SCHEMA-REMOVED | ToolPart | 0 | declared upstream, absent from the served document |
| DOC-SCH-429 | SCHEMA-REMOVED | ToolState | 0 | declared upstream, absent from the served document |
| DOC-SCH-430 | SCHEMA-REMOVED | ToolStateCompleted | 0 | declared upstream, absent from the served document |
| DOC-SCH-431 | SCHEMA-REMOVED | ToolStateError | 0 | declared upstream, absent from the served document |
| DOC-SCH-432 | SCHEMA-REMOVED | ToolStatePending | 0 | declared upstream, absent from the served document |
| DOC-SCH-433 | SCHEMA-REMOVED | ToolStateRunning | 0 | declared upstream, absent from the served document |
| DOC-SCH-434 | SCHEMA-REMOVED | ToolTextContent | 0 | declared upstream, absent from the served document |
| DOC-SCH-435 | SCHEMA-REMOVED | TuiCommandExecute | 0 | declared upstream, absent from the served document |
| DOC-SCH-436 | SCHEMA-REMOVED | TuiPromptAppend | 0 | declared upstream, absent from the served document |
| DOC-SCH-437 | SCHEMA-REMOVED | TuiSessionSelect | 0 | declared upstream, absent from the served document |
| DOC-SCH-438 | SCHEMA-REMOVED | TuiToastShow | 0 | declared upstream, absent from the served document |
| DOC-SCH-439 | SCHEMA-REMOVED | UnauthorizedError | 0 | declared upstream, absent from the served document |
| DOC-SCH-440 | SCHEMA-REMOVED | UnknownError | 0 | declared upstream, absent from the served document |
| DOC-SCH-441 | SCHEMA-REMOVED | UnknownError1 | 0 | declared upstream, absent from the served document |
| DOC-SCH-442 | SCHEMA-REMOVED | UserMessage | 0 | declared upstream, absent from the served document |
| DOC-SCH-443 | SCHEMA-REMOVED | V2Event | 0 | declared upstream, absent from the served document |
| DOC-SCH-444 | SCHEMA-REMOVED | V2EventStream | 0 | declared upstream, absent from the served document |
| DOC-SCH-445 | SCHEMA-REMOVED | VcsApplyError | 0 | declared upstream, absent from the served document |
| DOC-SCH-446 | SCHEMA-REMOVED | VcsBranchUpdated | 0 | declared upstream, absent from the served document |
| DOC-SCH-447 | SCHEMA-REMOVED | VcsFileStatus | 0 | declared upstream, absent from the served document |
| DOC-SCH-448 | SCHEMA-REMOVED | WellKnownAuth | 0 | declared upstream, absent from the served document |
| DOC-SCH-449 | SCHEMA-REMOVED | Workspace | 0 | declared upstream, absent from the served document |
| DOC-SCH-450 | SCHEMA-REMOVED | WorkspaceCreateError | 0 | declared upstream, absent from the served document |
| DOC-SCH-451 | SCHEMA-REMOVED | WorkspaceEventConnectionStatus | 0 | declared upstream, absent from the served document |
| DOC-SCH-452 | SCHEMA-REMOVED | WorkspaceFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-453 | SCHEMA-REMOVED | WorkspaceReady | 0 | declared upstream, absent from the served document |
| DOC-SCH-454 | SCHEMA-REMOVED | WorkspaceStatus | 0 | declared upstream, absent from the served document |
| DOC-SCH-455 | SCHEMA-REMOVED | WorkspaceWarpError | 0 | declared upstream, absent from the served document |
| DOC-SCH-456 | SCHEMA-REMOVED | Worktree | 0 | declared upstream, absent from the served document |
| DOC-SCH-457 | SCHEMA-REMOVED | WorktreeCreateInput | 0 | declared upstream, absent from the served document |
| DOC-SCH-458 | SCHEMA-REMOVED | WorktreeError | 0 | declared upstream, absent from the served document |
| DOC-SCH-459 | SCHEMA-REMOVED | WorktreeFailed | 0 | declared upstream, absent from the served document |
| DOC-SCH-460 | SCHEMA-REMOVED | WorktreeReady | 0 | declared upstream, absent from the served document |
| DOC-SCH-461 | SCHEMA-REMOVED | WorktreeRemoveInput | 0 | declared upstream, absent from the served document |
| DOC-SCH-462 | SCHEMA-REMOVED | WorktreeResetInput | 0 | declared upstream, absent from the served document |
| DOC-SCH-463 | SCHEMA-REMOVED | effect_HttpApiError_BadRequest | 0 | declared upstream, absent from the served document |
| DOC-SCH-464 | SCHEMA-REMOVED | effect_HttpApiError_Forbidden | 0 | declared upstream, absent from the served document |
| DOC-SCH-465 | SCHEMA-REMOVED | effect_HttpApiError_InternalServerError | 0 | declared upstream, absent from the served document |
| DOC-SCH-466 | SCHEMA-REMOVED | question.rejected | 0 | declared upstream, absent from the served document |
| DOC-SCH-467 | SCHEMA-REMOVED | question.replied | 0 | declared upstream, absent from the served document |
| DOC-SCH-468 | SCHEMA-REMOVED | session.status | 0 | declared upstream, absent from the served document |
| DOC-SCH-469 | SCHEMA-ADDED | APIKeyEntry | 0 | served, not declared upstream |
| DOC-SCH-470 | SCHEMA-ADDED | ActiveContextItem | 0 | served, not declared upstream |
| DOC-SCH-471 | SCHEMA-ADDED | Approval | 0 | served, not declared upstream |
| DOC-SCH-472 | SCHEMA-ADDED | ApprovalReviewRequest | 0 | served, not declared upstream |
| DOC-SCH-473 | SCHEMA-ADDED | BillingEntry | 0 | served, not declared upstream |
| DOC-SCH-474 | SCHEMA-ADDED | CreateAPIKeyRequest | 0 | served, not declared upstream |
| DOC-SCH-475 | SCHEMA-ADDED | CreateAPIKeyResponse | 0 | served, not declared upstream |
| DOC-SCH-476 | SCHEMA-ADDED | CreateSessionRequest | 0 | served, not declared upstream |
| DOC-SCH-477 | SCHEMA-ADDED | CreateSessionResponse | 0 | served, not declared upstream |
| DOC-SCH-478 | SCHEMA-ADDED | CreateTaskRequest | 0 | served, not declared upstream |
| DOC-SCH-479 | SCHEMA-ADDED | ExecuteToolRequest | 0 | served, not declared upstream |
| DOC-SCH-480 | SCHEMA-ADDED | ExecuteToolResponse | 0 | served, not declared upstream |
| DOC-SCH-481 | SCHEMA-ADDED | HealthResponse | 0 | served, not declared upstream |
| DOC-SCH-482 | SCHEMA-ADDED | InstanceInfo | 0 | served, not declared upstream |
| DOC-SCH-483 | SCHEMA-ADDED | InstancePathInfo | 0 | served, not declared upstream |
| DOC-SCH-484 | SCHEMA-ADDED | IterationCommit | 0 | served, not declared upstream |
| DOC-SCH-485 | SCHEMA-ADDED | MCPJSONRPCResponse | 0 | served, not declared upstream |
| DOC-SCH-486 | SCHEMA-ADDED | MemoryEvent | 0 | served, not declared upstream |
| DOC-SCH-487 | SCHEMA-ADDED | MessageReceivedResponse | 0 | served, not declared upstream |
| DOC-SCH-488 | SCHEMA-ADDED | Metrics | 0 | served, not declared upstream |
| DOC-SCH-489 | SCHEMA-ADDED | QuarantineApproveRequest | 0 | served, not declared upstream |
| DOC-SCH-490 | SCHEMA-ADDED | QuarantineDecisionResponse | 0 | served, not declared upstream |
| DOC-SCH-491 | SCHEMA-ADDED | QuarantineItem | 0 | served, not declared upstream |
| DOC-SCH-492 | SCHEMA-ADDED | QuarantineListResponse | 0 | served, not declared upstream |
| DOC-SCH-493 | SCHEMA-ADDED | QuarantineRejectRequest | 0 | served, not declared upstream |
| DOC-SCH-494 | SCHEMA-ADDED | RevokeAPIKeyResponse | 0 | served, not declared upstream |
| DOC-SCH-495 | SCHEMA-ADDED | SSEEvent | 0 | served, not declared upstream |
| DOC-SCH-496 | SCHEMA-ADDED | SendMessageRequest | 0 | served, not declared upstream |
| DOC-SCH-497 | SCHEMA-ADDED | SessionBilling | 0 | served, not declared upstream |
| DOC-SCH-498 | SCHEMA-ADDED | SessionToolCall | 0 | served, not declared upstream |
| DOC-SCH-499 | SCHEMA-ADDED | ShimProject | 0 | served, not declared upstream |
| DOC-SCH-500 | SCHEMA-ADDED | ShimSymbol | 0 | served, not declared upstream |
| DOC-SCH-501 | SCHEMA-ADDED | ShimSymbolPosition | 0 | served, not declared upstream |
| DOC-SCH-502 | SCHEMA-ADDED | Skill | 0 | served, not declared upstream |
| DOC-SCH-503 | SCHEMA-ADDED | SkillDetail | 0 | served, not declared upstream |
| DOC-SCH-504 | SCHEMA-ADDED | Task | 0 | served, not declared upstream |
| DOC-SCH-505 | SCHEMA-ADDED | Tool | 0 | served, not declared upstream |
| DOC-SCH-506 | SCHEMA-ADDED | ToolRequest | 0 | served, not declared upstream |
| DOC-SCH-507 | SCHEMA-ADDED | ToolResult | 0 | served, not declared upstream |
| DOC-SCH-508 | SCHEMA-ADDED | UpdateSessionRequest | 0 | served, not declared upstream |
| DOC-SCH-509 | SCHEMA-ADDED | UpdateTaskRequest | 0 | served, not declared upstream |
| DOC-SCH-510 | SCHEMA-ADDED | schema | 0 | served, not declared upstream |
| DOC-SCH-511 | SCHEMA-ADDED | schema-2 | 0 | served, not declared upstream |
| DOC-SCH-512 | SCHEMA-ADDED | schema-3 | 0 | served, not declared upstream |
| DOC-SCH-513 | SCHEMA-MODIFIED | Config | 43 | /additionalProperties: false -> <absent> \| /properties/$schema: {"type": "string"} -> <absent> \| /properties/agent: {"additionalPropert... |
| DOC-SCH-514 | SCHEMA-MODIFIED | Session | 40 | /additionalProperties: false -> <absent> \| /properties/agent: {"type": "string"} -> <absent> \| /properties/agent_name: <absent> -> {"ty... |
| DOC-SCH-515 | SCHEMA-MODIFIED | VcsFileDiff | 9 | /additionalProperties: false -> <absent> \| /description: <absent> -> opencode Vcs.FileDiff translation — p... \| /properties/additions/d... |
| DOC-SCH-516 | SCHEMA-MODIFIED | VcsInfo | 4 | /additionalProperties: false -> <absent> \| /description: <absent> -> opencode Vcs.Info translation — both ... \| /properties/branch/desc... |

