// Package mcp: streamable-HTTP transport (MCP-DIRECT-001).
//
// This file adds the MCP streamable-HTTP transport on top of the SAME
// JSON-RPC dispatch core the SSE transport uses (Server.handleMethod — the
// protocol logic is not forked). Per the MCP streamable-HTTP transport shape:
//
//   - POST /mcp takes a JSON-RPC request and answers a JSON-RPC response
//     (application/json).
//   - GET /mcp opens the SSE stream for server-initiated messages.
//
// It reuses HandleSSE for the GET half. The POST half is stateless per
// request: the session is addressed by the Mcp-Session-Id header (the
// streamable-HTTP session mechanism) and the mcpSession record is created by
// the initialize handshake itself, so a client needs nothing but the endpoint
// URL and an API key to attach.
//
// axiom:trace work_item=MCP-DIRECT-001 spec=specs/015-api-and-mcp.md plan=.memory-bank/work-items/MCP-DIRECT-001 impl=internal/mcp/streamable.go
package mcp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// streamableSessionHeader is the session-addressing header of the MCP
// streamable-HTTP transport ("MCP-Session-Id" per the spec; Go canonicalizes
// it to Mcp-Session-Id).
const streamableSessionHeader = "Mcp-Session-Id"

// HandleStreamable is the streamable-HTTP POST endpoint: a JSON-RPC request
// in, a JSON-RPC response out (application/json). The session is resolved
// from the Mcp-Session-Id header; a request without one is treated as the
// first message of a new streamable session (so initialize can arrive
// header-less), and every subsequent call must carry the header the server
// handed back.
func (s *Server) HandleStreamable(w http.ResponseWriter, r *http.Request) {
	tID := traceID()
	sID := spanID()
	endHTTP := spanStart("mcp.HandleStreamable", tID, sID)
	defer func() {
		endHTTP(nil)
	}()

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Session addressing: header first, query param second (the legacy
	// /mcp/message?sessionId=... spelling, so a client that only read the
	// SSE docs still attaches through this endpoint).
	sessionID := r.Header.Get(streamableSessionHeader)
	if sessionID == "" {
		sessionID = r.URL.Query().Get("sessionId")
	}

	var sess *mcpSession
	if sessionID != "" {
		s.mu.RLock()
		sess = s.sessions[sessionID]
		s.mu.RUnlock()
		if sess == nil {
			// A stale or forged session id is an authentication failure,
			// not a routing failure — match the dispatch gate's semantics
			// (DOGFOOD-101) so clients see a 401-class answer.
			s.writeHTTPError(w, http.StatusUnauthorized, nil, -32002, "Forbidden",
				"unknown or expired MCP session — re-initialize to obtain a new session")
			return
		}
	} else {
		// Header-less request: bootstrap a session record for the
		// handshake. Only initialize (and ping) can complete on it — the
		// dispatch gate rejects everything else until validateAuth marks
		// it authenticated (DOGFOOD-101).
		sessionID = generateSessionID()
		sess = &mcpSession{
			id:      sessionID,
			eventCh: make(chan string, 64),
			done:    make(chan struct{}),
		}
		s.mu.Lock()
		s.sessions[sessionID] = sess
		s.mu.Unlock()
		defer func() {
			// If the handshake did not authenticate this session, do not
			// leave the record behind (it holds no auth and cannot be
			// re-addressed by a client that did not receive the id).
			s.mu.RLock()
			_, stillThere := s.sessions[sessionID]
			s.mu.RUnlock()
			if stillThere && !sess.authenticated {
				s.mu.Lock()
				// Re-check under the write lock: a concurrent request
				// holding the same id may have authenticated meanwhile.
				if cur, ok := s.sessions[sessionID]; ok && cur == sess && !sess.authenticated {
					delete(s.sessions, sessionID)
				}
				s.mu.Unlock()
			}
		}()
		slog.Info("mcp: streamable session bootstrapped", "session", sessionID, "trace_id", tID)
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeHTTPError(w, http.StatusBadRequest, nil, -32700, "Parse error", err.Error())
		return
	}

	if req.JSONRPC != "2.0" {
		s.writeHTTPError(w, http.StatusBadRequest, req.ID, -32600, "Invalid Request", "jsonrpc must be 2.0")
		return
	}

	// Route to the shared dispatch core (same as SSE and stdio).
	result, rpcErr := s.handleMethod(&req, sess)

	// Notifications have no ID — no response body (HTTP 202 Accepted).
	if req.ID == nil || (rpcErr != nil && req.ID == nil) {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	w.Header().Set(streamableSessionHeader, sessionID)
	w.Header().Set("Content-Type", "application/json")

	if rpcErr != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(JSONRPCErrorResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   *rpcErr,
		})
		return
	}

	json.NewEncoder(w).Encode(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	})
}

// writeHTTPError writes a JSON-RPC error response with an explicit HTTP
// status. The streamable transport maps auth failures onto real HTTP status
// codes (401) while keeping the JSON-RPC error body clients already parse.
func (s *Server) writeHTTPError(w http.ResponseWriter, status int, id any, code int, message string, data any) {
	resp := JSONRPCErrorResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: JSONRPCErrObj{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// handleMCPRoot dispatches the bare /mcp mount by method: GET serves the SSE
// stream (streamable-HTTP shape, same handler as /mcp/sse), POST serves the
// streamable JSON-RPC endpoint, anything else is a 405.
func (s *Server) handleMCPRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.HandleSSE(w, r)
	case http.MethodPost:
		s.HandleStreamable(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
