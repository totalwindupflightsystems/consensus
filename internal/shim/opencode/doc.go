// Package opencode implements the opencode server protocol shim (SPEC-017).
//
// Translates opencode protocol requests to native Consensus API calls.
//
// Bind constraint (DF-CONSENSUS-19): the shim has no listener of its own —
// it mounts on the main server's HTTP listener (cmd/consensus/main.go), and
// the /instance/* surface is intentionally auth-free for opencode protocol
// compatibility (SPEC-017 §3.10), answering 200 with host-layout data
// (absolute config path, home directory, workspace directory). The shim must
// therefore be served on a loopback-bound listener (server.hostname /
// CONSENSUS_HOSTNAME default 127.0.0.1). Do not expose it on a non-loopback
// interface: a non-loopback bind produces a startup warning (config
// ApplyStartupValidations), but the override is honored.
//
// axiom:trace work_item=repo-bootstrap-01 spec=specs/017-ui-adapter-layer.md plan=phase-1/task-1/step-3
package opencode
