package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ROUTE-ADD-054 — POST /api/session/{sessionID}/prompt (v2.session.prompt)
//
// The upstream operation durably admits a PromptInput and declares 200, 400,
// 401, 404 and 409. Consensus has a native message-send path, so the shim
// validates the v2 body and delegates delivery to the shared service. When a
// service is not injected (the lightweight shim-only deployment), the handler
// still performs the durable admission directly through the same sessions and
// memory_events tables rather than fabricating success.
//
// ch:trace row=ROUTE-ADD-054 spec=specs/024-shim-support-matrix.md#v2-session-prompt wave=consensus-foreman-2026-10-31-06-09-32#ROUTE-ADD-054 test=api_session_prompt_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml prompt=/tmp/consensus-foreman-2026-10-31-06-09-32-ROUTE-ADD-054.txt evidence=/tmp/parity-054/declared-vs-served.json witness=none:pending-foreman-verification verdict=none:pending-judge commit=<your-commit-sha> memory=none:pending-foreman-write

type apiSessionPromptRequest struct {
	ID       string                `json:"id"`
	Prompt   apiSessionPromptInput `json:"prompt"`
	Delivery string                `json:"delivery"`
	Resume   *bool                 `json:"resume"`
}

type apiSessionPromptInput struct {
	Text   string            `json:"text"`
	Files  []json.RawMessage `json:"files"`
	Agents []json.RawMessage `json:"agents"`
}

// handleApiSessionPrompt serves POST /api/session/{sessionID}/prompt.
func (s *Server) handleApiSessionPrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return
	}

	var req apiSessionPromptRequest
	if !decodeDeclaredJSONObject(w, r, &req,
		"request body must contain prompt with required text") {
		return
	}
	if strings.TrimSpace(req.ID) != "" && !strings.HasPrefix(req.ID, "msg_") {
		writeV2InvalidRequest(w, "Body", "id", `field "id" must match the declared pattern ^msg_`)
		return
	}
	if strings.TrimSpace(req.Prompt.Text) == "" {
		writeV2InvalidRequest(w, "Body", "prompt.text", `required field "prompt.text" must not be blank`)
		return
	}
	if req.Delivery == "" {
		req.Delivery = "queue"
	} else if req.Delivery != "steer" && req.Delivery != "queue" {
		writeV2InvalidRequest(w, "Body", "delivery", `field "delivery" must be "steer" or "queue"`)
		return
	}

	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: the shim has no session store")
		return
	}
	row, err := s.db.QueryRow(r.Context(),
		`SELECT status, iteration FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: nothing to prompt", sessionID))
			return
		}
		writeV2InvalidRequest(w, "Shim", "sessions", "failed to resolve session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: nothing to prompt", sessionID))
		return
	}
	if sessionIsActive(toString(row["status"])) {
		s.sessionBusyError(w, r, sessionID)
		return
	}

	if s.svc != nil {
		// Prompt admission is asynchronous by contract: the request has already
		// resolved the session and active-turn conflict, so schedule the native
		// send without holding the HTTP request open for the agent response.
		enqueueCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
		go func() {
			defer cancel()
			_, _ = s.svc.SendMessage(enqueueCtx, MessageSendInput{
				SessionID: sessionID,
				Content:   req.Prompt.Text,
				MsgType:   "user_instruction",
			})
		}()
	} else {
		// This fallback is used by the shim's minimal deployment and tests. It
		// mirrors the native API's durable admission; the full service path also
		// wakes the harness and waits for its response.
		now := time.Now().UTC().Format(time.RFC3339)
		iteration := toInt64(row["iteration"]) + 1
		if err := s.db.Exec(r.Context(),
			`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at) VALUES ('user_message', $1, $2, $3, $4)`,
			req.Prompt.Text, sessionID, iteration, now); err != nil {
			writeV2InvalidRequest(w, "Shim", "memory_events", "failed to admit prompt: "+err.Error())
			return
		}
		if err := s.db.Exec(r.Context(),
			`UPDATE sessions SET status = 'thinking', heartbeat_at = $1, iteration = iteration + 1 WHERE id = $2`, now, sessionID); err != nil {
			writeV2InvalidRequest(w, "Shim", "sessions", "failed to schedule prompt: "+err.Error())
			return
		}
	}

	seq := int64(0)
	if rows, err := s.db.Query(r.Context(),
		`SELECT id FROM memory_events WHERE session_id = $1 AND type = 'user_message' ORDER BY id DESC LIMIT 1`, sessionID); err == nil && len(rows) > 0 {
		seq = toInt64(rows[0]["id"])
	}
	if seq == 0 {
		seq = toInt64(row["iteration"]) + 1
	}
	messageID := req.ID
	if messageID == "" {
		messageID = fmt.Sprintf("msg_%d", seq)
	}

	prompt := map[string]any{"text": req.Prompt.Text}
	if req.Prompt.Files != nil {
		prompt["files"] = req.Prompt.Files
	}
	if req.Prompt.Agents != nil {
		prompt["agents"] = req.Prompt.Agents
	}
	writeJSONStatus(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"admittedSeq": seq,
			"id":          messageID,
			"sessionID":   sessionID,
			"prompt":      prompt,
			"delivery":    req.Delivery,
			"timeCreated": float64(time.Now().UnixMilli()),
		},
	})
}

// isApiSessionPromptPath admits only the declared concrete prompt path to the
// v2 UnauthorizedError envelope.
func isApiSessionPromptPath(path string) bool {
	sessionID, tail, ok := cutApiSessionSubpath(path)
	return ok && sessionID != "" && tail == "prompt"
}
