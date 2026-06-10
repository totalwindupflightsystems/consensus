// Package memory implements context formatting and memory page management (SPEC-002).
//
// Provides the canonical context view formatting for LLM consumption, memory page
// resolution with deduplication, and helper types for the active context view.
//
// axiom:trace work_item=polish-phase spec=specs/002-memory.md plan=phase-1/task-1/step-1 impl=internal/memory/doc.go,internal/memory/memory.go,internal/memory/memory_test.go
package memory
