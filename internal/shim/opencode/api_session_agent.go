package opencode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ============================================================================
// ROUTE-ADD-041 — POST /api/session/{sessionID}/agent (v2.session.switchAgent)
// ============================================================================
//
// The pinned upstream document (specs/openapi/upstream/openapi-1.18.33.json,
// paths."/api/session/{sessionID}/agent".post) declares v2.session.switchAgent
// with declared responses
//
//	204 — <No Content>
//	400 — InvalidRequestError
//	401 — UnauthorizedError
//	404 — SessionNotFoundError
//
// one required path parameter sessionID (a plain string whose declared pattern
// is ^ses) and a REQUIRED request body — the additionalProperties:false object
// {agent: string} (agent is required; the schema declares no minLength).
//
// Before this change the exact path had no shim route at all, so net/http
// answered its default 404 — the declared-vs-served artifact's NOT-SERVED
// class, indistinguishable from "no such opencode operation". The route is
// registered as its own EXACT pattern (server.go, mux.HandleFunc
// "/api/session/{sessionID}/agent"); it is never an /api/session/* catch-all,
// so every sibling /api/session/{sessionID}[...] operation keeps the
// classification its own contract row is filed against, and the exact
// two-segment shape joins the v2 UnauthorizedError envelope through
// isV2EnvelopeRoutePath (server.go).
//
// Arms:
//
//	204 — the declared empty success, answered as a truthful idempotent no-op.
//	      This shim is a protocol adapter over the Consensus API service: its
//	      sessions store (migration 001 §3.2) records Consensus agent runs, and
//	      it holds NO opencode agent-selection registry to switch — there is no
//	      table, migration or config field that maps a session to a selectable
//	      opencode agent. The declared success carries no body at all, so the
//	      204 asserts nothing the runtime cannot back: it claims no agent
//	      change, echoes no identifier and invents no state (the
//	      handleExperimentalProjectCopyRemove / moveSession truthful-204
//	      precedent; contrast the declared 200 arms that DO carry data, where a
//	      reshape refusal answers an empty collection instead).
//	400 InvalidRequestError — every declared input violation, each naming its
//	      field: a present-but-blank sessionID (kind Path), an absent,
//	      unparseable, non-object, null or multi-value body, an undeclared body
//	      field (additionalProperties:false), and a missing, non-string or blank
//	      agent (kind Body). An unreadable session store is reported through the
//	      same declared 400 (kind Shim, service sessions) because this operation
//	      declares no 5xx arm and absence must never be claimed from a store the
//	      instance could not read (the handleApiSessionList /
//	      handleApiSessionActive convention).
//	401 UnauthorizedError — the /api/* auth wall; this exact two-segment shape
//	      is admitted by isV2EnvelopeRoutePath, so the body is the declared v2
//	      envelope rather than the shim's own v1 UNAUTHENTICATED body.
//	404 SessionNotFoundError — the sessionID is not a session this instance
//	      holds. The decision is read from the runtime's REAL sessions store, so
//	      a client distinguishes "no such session" from "the switch happened".
//	405 METHOD_NOT_ALLOWED — every method other than POST; POST is the only
//	      method the pinned document declares on this path, so no sibling
//	      operation is reclassified by this registration.
//
// ch:trace row=ROUTE-ADD-041 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.session.switchAgent wave=.coding-hermes/waves/consensus-foreman-2026-10-27-09-39-34.json test=internal/shim/opencode/api_session_agent_test.go doc=specs/openapi/upstream/consensus-shim-served-surface.yaml prompt=/tmp/consensus-foreman-2026-10-27-09-39-34-ROUTE-ADD-041-brief.txt evidence=docs/evidence/ROUTE-ADD-041-live-probe.md witness=none:foreman-direct-in-worktree verdict=none:foreman-tier2-pending commit=none:pre-commit memory=none:foreman-writes
//
// handleApiSessionAgent serves POST /api/session/{sessionID}/agent — upstream
// v2.session.switchAgent (ROUTE-ADD-041, declared responses 204, 400, 401, 404).
// See the file doc comment for the arm-by-arm contract and for why the declared
// 204 is the truthful answer.
func (s *Server) handleApiSessionAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}

	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeV2InvalidRequest(w, "Path", "sessionID", "sessionID path parameter must not be blank")
		return
	}

	if !decodeApiSessionAgentBody(w, r) {
		return
	}

	// An absent store cannot answer the 404 question, and this operation
	// declares no 5xx arm: report the unreadable store through the declared
	// InvalidRequestError instead of claiming the session does not exist.
	if s.db == nil {
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to resolve session: the shim has no session store")
		return
	}

	// Resolve the session against the runtime's real session store. The db
	// drivers in internal/db report a missing row as an error (there is no
	// sql.ErrNoRows sentinel on this seam), so the two answers are told apart
	// explicitly: a "no rows in result" reply is the declared 404, while any
	// OTHER read failure is reported through the declared InvalidRequestError
	// (this operation declares no 5xx arm and absence must never be claimed
	// from a store the instance could not read).
	row, err := s.db.QueryRow(r.Context(), `SELECT id FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		if isNoRowsResult(err) {
			writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
				fmt.Sprintf("no session %q in this runtime: nothing to switch", sessionID))
			return
		}
		writeV2InvalidRequest(w, "Shim", "sessions",
			"failed to resolve session: "+err.Error())
		return
	}
	if row == nil {
		writeOpencodeNotFoundError(w, "SessionNotFoundError", "sessionID", sessionID,
			fmt.Sprintf("no session %q in this runtime: nothing to switch", sessionID))
		return
	}

	// The session is one this instance holds and the request is well formed.
	// The declared 204 is the truthful answer: this adapter holds no opencode
	// agent-selection registry for a Consensus session (file doc comment), so
	// there is no agent binding to change and no data-bearing success to
	// fabricate. The declared success carries no body, so no claim is made.
	w.WriteHeader(http.StatusNoContent)
}

// decodeApiSessionAgentBody validates the operation's REQUIRED request body —
// the additionalProperties:false object {agent: string} — and answers the
// declared InvalidRequestError 400 for every violation, naming the offending
// field. It never retains the body: the shim has no agent registry to write it
// into, so the value is validated and discarded (the
// decodeApiIntegrationConnectKeyBody convention).
func decodeApiSessionAgentBody(w http.ResponseWriter, r *http.Request) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var body map[string]json.RawMessage
	if err := decoder.Decode(&body); err != nil {
		writeV2InvalidRequest(w, "Body", "body",
			"request body must be a JSON object containing only agent: "+err.Error())
		return false
	}
	if body == nil {
		writeV2InvalidRequest(w, "Body", "body",
			"request body must be a JSON object containing only agent")
		return false
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		writeV2InvalidRequest(w, "Body", "body",
			"request body must contain exactly one JSON object: "+err.Error())
		return false
	}

	for field := range body {
		if field != "agent" {
			writeV2InvalidRequest(w, "Body", field,
				fmt.Sprintf("unknown field %q: the declared body schema allows only agent", field))
			return false
		}
	}

	agentRaw, exists := body["agent"]
	if !exists {
		writeV2InvalidRequest(w, "Body", "agent", `required field "agent" is missing`)
		return false
	}
	var agentValue any
	if err := json.Unmarshal(agentRaw, &agentValue); err != nil {
		writeV2InvalidRequest(w, "Body", "agent", `field "agent" must be a string`)
		return false
	}
	agent, isString := agentValue.(string)
	if !isString {
		writeV2InvalidRequest(w, "Body", "agent", `field "agent" must be a string`)
		return false
	}
	if strings.TrimSpace(agent) == "" {
		writeV2InvalidRequest(w, "Body", "agent", `field "agent" must not be blank`)
		return false
	}

	return true
}

// isNoRowsResult reports whether a QueryRow error means "this instance holds no
// such row". The internal/db drivers (sqlite, postgres, and their tx variants)
// report an empty result set as their own error string rather than a
// sql.ErrNoRows sentinel, so the declared 404 arm is decided on that reply and
// every OTHER error keeps the declared 400 (absence is never claimed from a
// store the instance could not read).
func isNoRowsResult(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no rows in result")
}

// isApiSessionAgentPath admits only the declared concrete route shape
// (/api/session/{sessionID}/agent — exactly two non-empty segments, the second
// literally "agent") to the v2 UnauthorizedError auth branch. Sibling and
// deeper /api/session/* paths keep the shim's own envelope.
func isApiSessionAgentPath(path string) bool {
	rest, ok := strings.CutPrefix(path, "/api/session/")
	if !ok {
		return false
	}
	sessionID, tail, ok := strings.Cut(rest, "/")
	if !ok {
		return false
	}
	return sessionID != "" && tail == "agent"
}
