// Package webhook implements webhook ingestion and event routing (SPEC-013).
//
// Cognitive Firewall integration (WI-004): After ingesting events, the handler
// scans payloads for threats using the QuarantineScanner interface. Suspicious
// content is inserted into external_quarantine via the QuarantineInserter callback.
//
// axiom:trace work_item=repo-bootstrap-01,WI-004 spec=specs/013-webhooks-and-events.md,specs/005-security.md plan=phase-1/task-1/step-3,phase-2/task-1
package webhook
