# Input Area (§5.6) Implementation Notes

## Files to modify
- chronicle/index.html (embedded Go HTML, served by internal/chronicle/server.go)
- chronicle/css/layout.css (layout styles)

## API Endpoints Available
- POST /api/v1/sessions — Create session (body: {agent_name, goal, model_id})
- POST /api/v1/sessions/{id}/message — Send message (body: {content, type})
- GET /api/v1/sessions/{id} — Get session details (returns status, model_id, budget)
- PATCH /api/v1/sessions/{id} — Update session (body: {status: "pause"|"resume"|"cancel"})

## Existing state
The ./chronicle/ directory is embedded via Go embed and served at /chronicle/
The cron task is to build the Input Area — see full spec at specs/026-dashboard-ui.md §5.6
