// Package crier implements the inbound crier message path (CR-IN-001).
//
// Crier (/home/kara/crier) is the agent-to-agent message bus: a relay for live
// pub/sub, an ed25519-signed agent registry, and durable per-agent inboxes.
// Consensus consumes the DURABLE INBOX, not the relay. A delivery is stored
// until the consumer acks it, so consumer downtime costs latency and never
// messages — and there is no subscribe-before-publish window, because the
// relay DROPS a publish made while nobody is subscribed (crier
// docs/integration-guide.md §9.1, measured there on a scratch server).
//
// Two layers live here:
//
//   - Client — the HTTP client for the registry/inbox endpoints (register,
//     deliver, retrieve-with-lease, ack) against a configurable base URL: the
//     caller's value, else CONSENSUS_CRIER_URL, else DefaultBaseURL.
//   - Intake — one pass over an agent's durable inbox that appends every
//     pending message to memory_events as a 'user_message' row, stamped for
//     the iteration that will run, then acks the batch. That is the same
//     delivery convention api.MessageService.SendMessage and
//     internal/webhook's routed wake use, and it is what makes an inbound
//     message agent-visible: the harness projects 'user_message' rows as LLM
//     user turns (internal/harness/context.go).
//
// memory_events has NO metadata/JSON column (migrations/001_initial_schema.sql
// §3.1 declares id, type, content, summary_text, session_id,
// iteration_created, linked_memory_pages, embedding, created_at) and the table
// is append-only — a row is never updated, so provenance cannot be attached
// after the fact either. It therefore rides in the row's CONTENT behind a
// bracketed marker, which is the existing convention for externally-originated
// rows: internal/webhook stores "[webhook <eventType>] <payload>". See
// ProvenanceContent for the exact marker.
//
// axiom:trace work_item=CR-IN-001 spec=specs/015-api-and-mcp.md,specs/011-canonical-definitions.md plan=phase-2 impl=internal/crier/doc.go
package crier
