// Package driver wires the database implementations and provides the Open factory.
//
// This package is separated from db to avoid import cycles: both db/postgres
// and db/sqlite import db for the interface types, so the wiring logic that
// imports both lives here.
//
// axiom:trace work_item=runtime-harness-01 spec=specs/021-repository-layout.md plan=phase-1/task-1-1/step-1-1-2
package driver
