// Package quarantine implements the Cognitive Firewall for Consensus (SPEC-005).
//
// The cognitive firewall quarantines all untrusted external data before it enters
// agent memory. External payloads (webhooks, API responses, user pastes) are
// scanned for threats using a heuristic regex engine (replaceable with a fast
// local model later).
//
// Architecture:
//
//	External Payload
//	    │
//	    ▼
//	external_events table
//	    │
//	    ▼
//	ScanQuarantinedEvent() — regex/heuristic scanner
//	    │
//	    ├─ Clean → approved → data flows to agent normally
//	    │
//	    └─ Suspicious → external_quarantine table (pending scan)
//	                       │
//	                       ▼
//	                Approve via API → promoted to memory_events
//	                Reject via API  → rejected with reason in validation_notes
//
// axiom:trace work_item=WI-004 spec=specs/005-security.md,specs/013-webhooks-and-events.md plan=meta-planning
package quarantine
