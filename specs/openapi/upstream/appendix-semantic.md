| id | path | method | operationId | declared_responses | served_outcome | detail |
|---|---|---|---|---|---|---|
| SHIM-SEMANTIC-001 | /mcp | GET | mcp.status | 200,400 | 200-mcp-delegate | upstream declares mcp.status (list MCP server statuses); the shim serves the MCP streamable-HTTP protocol endpoint |
| SHIM-SEMANTIC-002 | /mcp | POST | mcp.add | 200,400 | 200-mcp-delegate | upstream declares mcp.add (add an MCP server); the shim serves the MCP JSON-RPC endpoint |
