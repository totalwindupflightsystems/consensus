package opencode

import (
	"net/http"
)

// handleApiEventSubscribe serves GET /api/event — upstream v2.event.subscribe
// (ROUTE-ADD-009, SHIM-DRIFT-003, declared responses: 200
// "Event stream" text/event-stream of V2Event, 400 InvalidRequestError, 401
// UnauthorizedError; no declared parameters, no requestBody). Previously the
// path had no route at all and net/http answered the default 404 (the
// artifact's class NOT-SERVED).
//
// ch:trace row=ROUTE-ADD-009 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.event.subscribe test=internal/shim/opencode/api_event_subscribe_test.go witness=none:unattended-worker-session
//
// HONEST NARROWING (ROUTE-FIX-037 precedent): the declared 200 is NOT served
// and never fabricated. The declared success body is an SSE stream of
// V2Event — a discriminated union whose members carry a required id matching
// ^evt_, a required data.info: Message (for the message.* members), and a
// required durable aggregate envelope ({aggregateID, seq, version}). The
// runtime's only real event stream is the one backing the v1 /event route
// (handleGlobalEvent): it emits v1-shaped events (message.created,
// tool.started, permission.requested, ...) with none of the V2Event members —
// no ^evt_ id, no Message info object, no durable envelope. Re-emitting those
// frames under the v2 route would present an undeclared payload shape as the
// declared contract, which is exactly the fabrication this shim's
// truthfulness rules forbid; the runtime keeps no v2 session-event store
// (internal/db has none) that could back the declared frames truthfully.
//
// Therefore every well-formed subscribe (and there is no malformed input:
// the operation declares no parameters and no requestBody, so there is
// nothing to reject against a narrower arm) answers the DECLARED 400
// InvalidRequestError with this reason. Making the v2 event surface real is a
// design change with its own row, not a contract detail.
//
// Wave-recovery ordering (consensus-2026-09-26-00-13-38): the 400 is written
// with no streaming headers before it — this route sits inside the authed
// /api/* wall, so the unauthenticated arm stays a fast 401 (v2
// UnauthorizedError envelope via v2EnvelopeRoutePaths in server.go), never an
// SSE hang.
func (s *Server) handleApiEventSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpencodeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
		return
	}

	writeV2InvalidRequest(w, "", "",
		"v2.event.subscribe is not served by the Consensus shim: the runtime keeps no v2 session-event store, and its real event stream (v1 /event) emits v1-shaped events that cannot be truthfully re-framed as the declared V2Event schema")
}
