// Package cli implements the CLI command definitions (SPEC-016).
//
// Uses Cobra for command structure. The CLI acts as a thin REST client
// to a running Consensus server; it does not connect to the database directly.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/016-cli-interface.md plan=phase-4/task-4-1/step-4-1-1 impl=internal/cli/root.go,internal/cli/client.go,internal/cli/formatter.go,internal/cli/session.go,internal/cli/approve.go,internal/cli/migrate.go,internal/cli/config.go,internal/cli/status.go,internal/cli/memory.go,internal/cli/tool.go,internal/cli/completion.go
package cli
