// Package mcp implements the Model Context Protocol (MCP) server for Consensus
// per SPEC-015 §5. The MCP server exposes the agent runtime to any MCP-compatible
// client (Claude Desktop, IDE plugins, etc.) via JSON-RPC 2.0.
//
// Transport options:
//   - SSE (default): JSON-RPC over Server-Sent Events (SPEC-015 §5.4)
//   - Stdio: JSON-RPC over stdin/stdout (SPEC-015 §5.4)
//
// The server shares the same database connection and auth model as the REST API.
// There is no separate state — all operations read and write the database.
//
// axiom:trace work_item=WI-015 spec=specs/015-api-and-mcp.md plan=phase-5/task-5-1/step-5-1-1 impl=internal/mcp/
package mcp
