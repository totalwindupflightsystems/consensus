package crier

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeCrier is an in-process stand-in for the crier relay: the agent registry
// and the durable per-agent inboxes, with the real server's identity rules
// applied. A retrieve returns only the {id} in the path, an unsigned
// retrieve/ack is refused 401 when requireSig is set (CR_REQUIRE_AGENT_SIG=true
// is crier's default), a signature presented as another agent is refused 403,
// an ack releases the lease and removes the ids — and a message for a
// different agent is never handed to the caller.
//
// It is not a stub that answers whatever a test wants: the isolation assertion
// in intake_test.go is only meaningful because the fake enforces identity the
// way crier does. It binds loopback via httptest — no live network, no live
// crier server.
type fakeCrier struct {
	mu         sync.Mutex
	agents     map[string]bool
	inboxes    map[string][]InboxEntry
	seq        int
	leaseSeq   int
	requireSig bool

	retrieves int
	ackCalls  [][]string
}

func newFakeCrier() *fakeCrier {
	return &fakeCrier{
		agents:  map[string]bool{},
		inboxes: map[string][]InboxEntry{},
	}
}

// start runs the fake on a loopback httptest server.
func (f *fakeCrier) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return srv
}

// register adds an agent row directly; the client's own Register is exercised
// separately.
func (f *fakeCrier) register(agentID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agents[agentID] = true
}

// deliver stores a payload object in an agent's inbox the way
// POST /agents/{id}/inbox does and returns the message id crier minted.
func (f *fakeCrier) deliver(agentID string, payload map[string]any) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agents[agentID] = true
	return f.appendLocked(agentID, raw, "")
}

// appendLocked appends one entry, storing the payload byte for byte.
func (f *fakeCrier) appendLocked(agentID string, raw []byte, sender string) string {
	f.seq++
	entry := InboxEntry{
		ID:        fmt.Sprintf("msg-%04d", f.seq),
		AgentID:   agentID,
		Payload:   raw,
		CreatedAt: "2026-09-29T00:00:00Z",
		Sender:    sender,
	}
	f.inboxes[agentID] = append(f.inboxes[agentID], entry)
	return entry.ID
}

// pending returns the message ids still in an agent's queue (unacked).
func (f *fakeCrier) pending(agentID string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := []string{}
	for _, e := range f.inboxes[agentID] {
		ids = append(ids, e.ID)
	}
	return ids
}

// ackedIDs returns every id acknowledged so far, across all ack calls.
func (f *fakeCrier) ackedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := []string{}
	for _, call := range f.ackCalls {
		ids = append(ids, call...)
	}
	return ids
}

// retrieveCount is how many retrieve calls the fake served.
func (f *fakeCrier) retrieveCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.retrieves
}

func (f *fakeCrier) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /agents", f.handleRegister)
	mux.HandleFunc("POST /agents/{id}/inbox", f.handleDeliver)
	mux.HandleFunc("GET /agents/{id}/inbox", f.handleRetrieve)
	mux.HandleFunc("POST /agents/{id}/inbox/ack", f.handleAck)
	return mux
}

func (f *fakeCrier) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID           string   `json:"id"`
		PublicKey    string   `json:"public_key"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.agents[body.ID] {
		writeError(w, http.StatusConflict, fmt.Sprintf("agent already registered: %q", body.ID))
		return
	}
	f.agents[body.ID] = true
	writeJSON(w, http.StatusCreated, Agent{ID: body.ID, PublicKey: body.PublicKey, Capabilities: body.Capabilities, Status: "online"})
}

func (f *fakeCrier) handleDeliver(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	var body struct {
		Payload json.RawMessage `json:"payload"`
		Sender  string          `json:"sender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	trimmed := strings.TrimSpace(string(body.Payload))
	if trimmed == "" || trimmed == "null" || !strings.HasPrefix(trimmed, "{") {
		writeError(w, http.StatusBadRequest, "payload is required")
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.agents[agentID] {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	id := f.appendLocked(agentID, []byte(trimmed), body.Sender)
	writeJSON(w, http.StatusCreated, DeliverResult{ID: id, Transport: "inbox", ExpiresAt: "2026-09-30T00:00:00Z"})
}

func (f *fakeCrier) handleRetrieve(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")

	f.mu.Lock()
	defer f.mu.Unlock()
	f.retrieves++

	if !f.agents[agentID] {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	// Identity: with signatures enforced (crier's default) the caller must
	// present the triple, and X-Agent-ID must be the {id} in the path.
	if f.requireSig {
		caller := r.Header.Get(headerAgentID)
		if caller == "" || r.Header.Get(headerAgentTs) == "" || r.Header.Get(headerAgentSig) == "" {
			writeError(w, http.StatusUnauthorized, "missing agent signature headers (X-Agent-ID, X-Agent-Ts, X-Agent-Sig)")
			return
		}
		if caller != agentID {
			writeError(w, http.StatusForbidden,
				fmt.Sprintf("agent %q may only access its own resources (target %q)", caller, agentID))
			return
		}
	} else if caller := r.Header.Get(headerAgentID); caller != "" && caller != agentID {
		writeError(w, http.StatusForbidden,
			fmt.Sprintf("agent %q may only access its own resources (target %q)", caller, agentID))
		return
	}

	// Claim every entry that is not already leased; leased entries stay
	// hidden until they are acked or their lease expires (crier's §3 rule).
	queue := f.inboxes[agentID]
	claimed := make([]InboxEntry, 0, len(queue))
	remaining := make([]InboxEntry, 0, len(queue))
	leased := 0
	leaseID := ""
	for _, entry := range queue {
		if entry.LeaseID != "" {
			leased++
			remaining = append(remaining, entry)
			continue
		}
		if leaseID == "" {
			f.leaseSeq++
			leaseID = fmt.Sprintf("lease-%04d", f.leaseSeq)
		}
		entry.LeaseID = leaseID
		claimed = append(claimed, entry)
		remaining = append(remaining, entry)
	}
	if len(claimed) > 0 {
		f.inboxes[agentID] = remaining
	}

	writeJSON(w, http.StatusOK, RetrieveResult{
		Messages:    claimed,
		LeaseID:     leaseID,
		QueueDepth:  len(f.inboxes[agentID]),
		LeasedCount: leased + len(claimed),
	})
}

func (f *fakeCrier) handleAck(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	var body ackRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.LeaseID == "" || len(body.MessageIDs) == 0 {
		writeError(w, http.StatusBadRequest, "lease_id and message_ids are required")
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.agents[agentID] {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if f.requireSig {
		caller := r.Header.Get(headerAgentID)
		if caller == "" || r.Header.Get(headerAgentSig) == "" {
			writeError(w, http.StatusUnauthorized, "missing agent signature headers (X-Agent-ID, X-Agent-Ts, X-Agent-Sig)")
			return
		}
		if caller != agentID {
			writeError(w, http.StatusForbidden,
				fmt.Sprintf("agent %q may only access its own resources (target %q)", caller, agentID))
			return
		}
	}

	queue := f.inboxes[agentID]
	for _, id := range body.MessageIDs {
		idx := -1
		for i, entry := range queue {
			if entry.ID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			writeError(w, http.StatusNotFound, fmt.Sprintf("message %q not found in inbox", id))
			return
		}
		if queue[idx].LeaseID != body.LeaseID {
			writeError(w, http.StatusConflict, fmt.Sprintf("message %q is leased under %q", id, queue[idx].LeaseID))
			return
		}
	}
	kept := make([]InboxEntry, 0, len(queue))
	for _, entry := range queue {
		drop := false
		for _, id := range body.MessageIDs {
			if entry.ID == id {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, entry)
		}
	}
	f.inboxes[agentID] = kept
	f.ackCalls = append(f.ackCalls, append([]string(nil), body.MessageIDs...))

	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
