// Package web provides a minimal web admin UI for the Consensus runtime.
//
// Serves embedded HTML pages (dashboard, sessions, memory browser).
// All data operations go through the existing REST API. The server also provides
// an API proxy for the frontend to avoid CORS issues.
//
// axiom:trace work_item=polish-phase spec=specs/016-cli-interface.md plan=phase-1/task-1/step-1 impl=internal/web/server.go
package web
