package opencode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// apiPtyCreate serves upstream v2.pty.create — POST /api/pty (ROUTE-ADD-029).
//
// The pinned document (specs/openapi/upstream/openapi-1.18.33.json,
// paths./api/pty.post) declares: one optional deepObject `location` query
// selector (location[directory] / location[workspace],
// additionalProperties:false — the same selector the sibling GET arm carries),
// a REQUIRED application/json requestBody that is an object with
// additionalProperties:false and five OPTIONAL members — command (string),
// args (string[]), cwd (string), title (string) and env (object of string) —
// and responses 200 {location: LocationInfo, data: Pty} (description:
// "Create PTY session"), 400 InvalidRequestError, 401 UnauthorizedError.
//
// SERVED, in this order:
//
//  1. 401 UnauthorizedError — the /api/* auth wall runs before the mux, and
//     /api/pty is in v2EnvelopeRoutePaths, so an unauthenticated request keeps
//     the declared v2 body ({_tag:"UnauthorizedError", message}) rather than
//     the v1 UNAUTHENTICATED envelope.
//  2. 400 InvalidRequestError — the two declared input contracts are checked
//     before any answer about the operation: the `location` selector through
//     the shared apiV2LocationSelector (kind Query — the identical validation
//     the GET arm serves) and the required requestBody through
//     validateApiPtyCreateBody (kind Body for an absent/unparseable/non-object
//     body, kind Payload for a value that violates the declared schema:
//     an undeclared member, or a member of the wrong JSON type — field named).
//     The body is REQUIRED by the operation (`"required": true` on
//     requestBody, not on its members), so an absent body is the declared 400
//     here rather than a silent pass; the ROUTE-ADD-070 precedent for a
//     body-carrying operation. No declared member is required, so `{}` is a
//     well-formed body.
//  3. 501 typed not_implemented for a well-formed request — the intentional
//     narrowing ROUTE-ADD-103 established for pty.create and SPEC-017 §3.9
//     records for the whole pty.create family.
//
// WHAT IS NOT SERVED AND WHY — the declared 200 {location, data: Pty}. That
// body requires a real Pty: id (pattern ^pty), title, command, args, cwd,
// status (running|exited) and pid. This runtime keeps NO pty registry and
// spawns NO pseudo-terminals (the ROUTE-ADD-102 / ROUTE-ADD-028 truthfulness
// rule beside it: pty.list answers the empty array for the same reason), and
// Consensus exposes no raw PTY lifecycle — shell execution runs through the
// authenticated agent tool system inside a session. Emitting a Pty object
// would fabricate a spawned terminal, so the declared success stays
// unreachable and the typed 501 names the operation instead. Serving the 200
// is an owner/spec decision (SPEC-017 §3.9, SPEC-018 SHIM-NARROWED-012…):
// implementing it means giving the runtime a real PTY registry, not reshaping
// this handler.
//
// The narrowing is only reachable through a WELL-FORMED request (valid
// selectors and a body that satisfies the declared schema), which keeps the
// 501 a statement about the runtime's capability rather than a cover for
// input the shim refused to inspect.
//
// ch:trace row=ROUTE-ADD-029 spec=specs/openapi/upstream/openapi-1.18.33.json#v2.pty.create test=internal/shim/opencode/api_pty_create_test.go evidence=docs/evidence/ROUTE-ADD-029-live-probe.md witness=none:unattended-worker-session verdict=none:not-judged commit=none:awaiting-foreman-commit
func (s *Server) apiPtyCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.apiV2LocationSelector(w, r); !ok {
		return
	}
	if !validateApiPtyCreateBody(w, r) {
		return
	}
	writeNotImplemented(w, r, "v2.pty.create",
		"v2.pty.create is not implemented: spawning pseudo-terminals is opencode-specific and Consensus keeps no pty registry; use the native shell tool inside an agent session")
}

// validateApiPtyCreateBody enforces the declared v2.pty.create requestBody:
// a required application/json object, additionalProperties:false, whose only
// declared members are command (string), args (string[]), cwd (string), title
// (string) and env (object whose additionalProperties are strings). None of
// the five is required, so `{}` passes; the body itself is required by the
// operation, so an absent body is refused. A JSON null for a declared member
// is a type violation, not an absence — the pinned schema gives no member a
// null arm.
//
// Every refusal answer is the declared 400 InvalidRequestError (the v2 wire
// shape the sibling GET arm already emits): kind Body when the body is absent,
// unparseable, not a single JSON value, or not a JSON object at all; kind
// Payload with the offending field named when a value violates the schema.
func validateApiPtyCreateBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.ContentLength == 0 {
		writeV2InvalidRequest(w, "Body", "",
			"v2.pty.create declares a required application/json request body; send a JSON object (an empty object is valid)")
		return false
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var raw any
	if err := dec.Decode(&raw); err != nil {
		writeV2InvalidRequest(w, "Body", "", "malformed request body: "+err.Error())
		return false
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		writeV2InvalidRequest(w, "Body", "", "request body must contain exactly one JSON object")
		return false
	}

	obj, ok := raw.(map[string]any)
	if !ok {
		writeV2InvalidRequest(w, "Body", "", "request body must be a JSON object")
		return false
	}

	// additionalProperties:false — an undeclared member is a contract
	// violation, never an enhancement, so it is refused by name.
	for field := range obj {
		switch field {
		case "command", "args", "cwd", "title", "env":
		default:
			writeV2InvalidRequest(w, "Payload", field,
				fmt.Sprintf("field %q is not declared by v2.pty.create (additionalProperties: false)", field))
			return false
		}
	}

	for _, field := range []string{"command", "cwd", "title"} {
		value, present := obj[field]
		if !present {
			continue
		}
		if _, isString := value.(string); !isString {
			writeV2InvalidRequest(w, "Payload", field,
				fmt.Sprintf("field %q must be a string", field))
			return false
		}
	}

	if value, present := obj["args"]; present {
		items, isArray := value.([]any)
		if !isArray {
			writeV2InvalidRequest(w, "Payload", "args", `field "args" must be an array of strings`)
			return false
		}
		for i, item := range items {
			if _, isString := item.(string); !isString {
				writeV2InvalidRequest(w, "Payload", fmt.Sprintf("args[%d]", i),
					fmt.Sprintf("args[%d] must be a string", i))
				return false
			}
		}
	}

	if value, present := obj["env"]; present {
		env, isObject := value.(map[string]any)
		if !isObject {
			writeV2InvalidRequest(w, "Payload", "env", `field "env" must be an object of string values`)
			return false
		}
		for name, entry := range env {
			if _, isString := entry.(string); !isString {
				writeV2InvalidRequest(w, "Payload", "env."+name,
					fmt.Sprintf("env[%q] must be a string", name))
				return false
			}
		}
	}

	return true
}
