// Package mcp implements the Model Context Protocol (MCP) server for Consensus.
//
// This file implements the stdio transport for MCP (SPEC-015 §5.4).
// Unlike the SSE transport (which uses HTTP for both directions), stdio transport
// reads JSON-RPC 2.0 requests from stdin and writes responses to stdout.
// This enables MCP clients like Claude Desktop to launch the server as a
// subprocess and communicate via stdio.
//
// axiom:trace work_item=WI-015 spec=specs/015-api-and-mcp.md plan=phase-5/task-5-1/step-5-1-1 impl=internal/mcp/stdio.go
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
)

// ============================================================================
// Stdio Transport (SPEC-015 §5.4)
// ============================================================================

// ServeStdio starts an MCP stdio transport session.
// It reads JSON-RPC 2.0 requests from stdin, dispatches them to the same
// method handlers as the SSE transport, and writes responses to stdout.
//
// The transport reads line-delimited JSON: each line is a complete JSON-RPC
// request. Responses are written as line-delimited JSON to stdout.
// Stderr is reserved for logging — the MCP client reads only stdout.
//
// Unlike the SSE transport which creates per-connection sessions, stdio
// creates a single session that lasts the lifetime of the process.
// Authentication is handled via the initialize request's _meta.authorization.
func (s *Server) ServeStdio(ctx context.Context) error {
	slog.Info("mcp: starting stdio transport")

	// Create a single MCP session for the stdio connection lifetime.
	// The session is fully initialized during the initialize handshake.
	sess := &mcpSession{
		id:      "stdio",
		eventCh: make(chan string, 64),
		done:    make(chan struct{}),
	}

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	reader := bufio.NewReader(os.Stdin)
	writeMu := &sync.Mutex{} // protect stdout writes

	// Channel to report fatal errors
	errCh := make(chan error, 1)

	go func() {
		<-sigCh
		slog.Info("mcp: stdio transport interrupted, shutting down")
		close(sess.done)
	}()

	go func() {
		<-ctx.Done()
		close(sess.done)
	}()

	// Read loop: each line is a JSON-RPC request
	go func() {
		defer close(errCh)

		for {
			select {
			case <-sess.done:
				return
			default:
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					slog.Info("mcp: stdin closed")
					return
				}
				errCh <- fmt.Errorf("mcp: read stdin: %w", err)
				return
			}

			line = trimNewline(line)
			if line == "" {
				continue // skip empty lines
			}

			// Parse the JSON-RPC request
			var req JSONRPCRequest
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				slog.Warn("mcp: invalid JSON-RPC request", "error", err)
				s.writeStdioError(writeMu, nil, -32700, "Parse error", err.Error())
				continue
			}

			if req.JSONRPC != "2.0" {
				s.writeStdioError(writeMu, req.ID, -32600, "Invalid Request", "jsonrpc must be 2.0")
				continue
			}

			// Route to handler (same dispatch as SSE transport)
			result, rpcErr := s.handleMethod(&req, sess)

			// Notifications have no ID — no response needed
			if req.ID == nil {
				continue
			}

			if rpcErr != nil {
				s.writeStdioError(writeMu, req.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
				continue
			}

			// Write success response
			resp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  result,
			}
			data, err := json.Marshal(resp)
			if err != nil {
				slog.Error("mcp: failed to marshal response", "error", err)
				continue
			}

			writeMu.Lock()
			fmt.Fprintln(os.Stdout, string(data))
			writeMu.Unlock()
		}
	}()

	// Wait for done signal or error
	select {
	case err := <-errCh:
		return err
	case <-sess.done:
		return nil
	}
}

// writeStdioError writes a JSON-RPC error response to stdout.
func (s *Server) writeStdioError(mu *sync.Mutex, id any, code int, message string, data any) {
	resp := JSONRPCErrorResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: JSONRPCErrObj{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	body, err := json.Marshal(resp)
	if err != nil {
		slog.Error("mcp: failed to marshal error response", "error", err)
		return
	}

	mu.Lock()
	fmt.Fprintln(os.Stdout, string(body))
	mu.Unlock()
}

// trimNewline removes trailing \r\n or \n from a string.
func trimNewline(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '\r' {
		s = s[:len(s)-1]
	}
	return s
}


