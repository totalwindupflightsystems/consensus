// Package tools: sandboxed subprocess execution for external tools (WI-005).
//
// The sandbox isolates external tool execution by:
//   - Creating a temp working directory per execution
//   - Enforcing a strict environment variable whitelist
//   - Enforcing a timeout (default 30s)
//   - Limiting output size (default 1MB)
//   - Limiting concurrent executions (default max 10)
//   - Optionally blocking network access
//
// axiom:trace work_item=WI-005 spec=specs/010-tools.md plan=phase-1/task-2
package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// ============================================================================
// Constants
// ============================================================================

// DefaultTimeout is the default execution timeout for external tools.
const DefaultTimeout = 30 * time.Second

// MaxOutputBytes is the maximum output size captured from a tool execution.
const MaxOutputBytes = 1 * 1024 * 1024 // 1MB

// MaxConcurrentExecutions is the maximum number of concurrent subprocess executions.
const MaxConcurrentExecutions = 10

// AllowedEnvPrefixes are the environment variable prefixes allowed in the sandbox.
var AllowedEnvPrefixes = []string{"CONSENSUS_", "HOME", "PATH", "USER"}

// ============================================================================
// Types
// ============================================================================

// ExternalToolConfig configures sandboxed subprocess execution.
type ExternalToolConfig struct {
	// Timeout is the maximum wall-clock time for the execution.
	// Default: 30 seconds.
	Timeout time.Duration

	// MaxOutputBytes caps the captured stdout+stderr.
	// Default: 1MB. Output beyond this is truncated.
	MaxOutputBytes int

	// NoNetwork restricts network access for the tool.
	// When true, CONSENSUS_TOOL_NETWORK=none is set in the subprocess env,
	// and tools that need network should self-enforce this.
	NoNetwork bool

	// AllowedEnv is the list of environment variable names or prefixes
	// that are forwarded to the subprocess. Default: CONSENSUS_*, HOME, PATH, USER.
	AllowedEnv []string

	// ExecutableOverride overrides the executable to run.
	// If empty, the handler_ref from tools_registry is used.
	ExecutableOverride string
}

// DefaultExternalToolConfig returns a safe default configuration.
func DefaultExternalToolConfig() ExternalToolConfig {
	return ExternalToolConfig{
		Timeout:        DefaultTimeout,
		MaxOutputBytes: MaxOutputBytes,
		NoNetwork:      true,
		AllowedEnv:     append([]string{}, AllowedEnvPrefixes...),
	}
}

// ExternalToolResult holds the output of a sandboxed tool execution.
type ExternalToolResult struct {
	Output     string `json:"output"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// ============================================================================
// Concurrent Execution Semaphore
// ============================================================================

// executionSemaphore limits concurrent subprocess executions.
var executionSemaphore = make(chan struct{}, MaxConcurrentExecutions)

// activeExecutions tracks the current count for observability.
var activeExecutions atomic.Int64

// AcquireSemaphore blocks until a slot is available.
func AcquireSemaphore() {
	executionSemaphore <- struct{}{}
	activeExecutions.Add(1)
}

// ReleaseSemaphore releases a semaphore slot.
func ReleaseSemaphore() {
	<-executionSemaphore
	activeExecutions.Add(-1)
}

// ActiveExecutionCount returns the current number of concurrent executions.
func ActiveExecutionCount() int64 {
	return activeExecutions.Load()
}

// ============================================================================
// Sandbox Helpers
// ============================================================================

// createTempWorkDir creates a temporary directory for tool execution.
func createTempWorkDir(toolName string) (string, func(), error) {
	dir, err := os.MkdirTemp("", fmt.Sprintf("consensus-tool-%s-*", sanitizeName(toolName)))
	if err != nil {
		return "", nil, fmt.Errorf("sandbox: create temp dir: %w", err)
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}

// sanitizeName makes a tool name safe for filesystem use.
func sanitizeName(name string) string {
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}

// buildSandboxEnv builds the whitelisted environment for the subprocess.
func buildSandboxEnv(cfg ExternalToolConfig) []string {
	allowed := cfg.AllowedEnv
	if len(allowed) == 0 {
		allowed = AllowedEnvPrefixes
	}

	// Build a set of allowed prefixes and exact names for fast lookup
	prefixes := make([]string, 0)
	exact := make(map[string]bool)
	for _, a := range allowed {
		if strings.HasSuffix(a, "*") {
			prefixes = append(prefixes, strings.TrimSuffix(a, "*"))
		} else {
			exact[a] = true
		}
	}

	// Add CONSENSUS_TOOL_NETWORK=none if network is disabled
	var env []string
	if cfg.NoNetwork {
		env = append(env, "CONSENSUS_TOOL_NETWORK=none")
	}

	// Forward matching env vars from the parent process
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) < 1 {
			continue
		}
		name := parts[0]

		// Check exact match
		if exact[name] {
			env = append(env, e)
			continue
		}

		// Check prefix match
		for _, prefix := range prefixes {
			if strings.HasPrefix(name, prefix) {
				env = append(env, e)
				break
			}
		}
	}

	return env
}

// ============================================================================
// ExecuteExternalTool — Core Sandbox Execution
// ============================================================================

// ExecuteExternalTool runs an external tool as a sandboxed subprocess.
//
// Parameters:
//   - ctx: Context for cancellation and timeout. If ctx has no deadline,
//     cfg.Timeout is applied.
//   - executable: Path or name of the executable to run.
//   - args: Command-line arguments.
//   - cfg: Sandbox configuration (use DefaultExternalToolConfig()).
//
// Returns:
//   - *ExternalToolResult with captured output, exit code, and duration.
//   - error only if the sandbox itself fails (temp dir creation, etc.);
//     subprocess errors are captured in the result.
func ExecuteExternalTool(ctx context.Context, executable string, args []string, cfg ExternalToolConfig) (*ExternalToolResult, error) {
	if executable == "" {
		return nil, fmt.Errorf("sandbox: executable is required")
	}

	// Apply default config for zero values
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = MaxOutputBytes
	}

	// Acquire semaphore (block until a slot is available)
	AcquireSemaphore()
	defer ReleaseSemaphore()

	slog.Debug("sandbox: executing tool",
		"executable", executable,
		"args", args,
		"timeout", cfg.Timeout,
		"no_network", cfg.NoNetwork,
	)

	// Create temp working directory
	workDir, cleanup, err := createTempWorkDir(filepath.Base(executable))
	if err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	defer cleanup()

	// If no context deadline, apply our own timeout.
	execCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	// Build the command
	cmd := exec.CommandContext(execCtx, executable, args...)
	cmd.Dir = workDir
	cmd.Env = buildSandboxEnv(cfg)

	// Capture stdout and stderr via limited writer (prevent OOM from large output)
	var outputBuf bytes.Buffer
	limitedOut := &limitedWriter{
		maxBytes: cfg.MaxOutputBytes,
		w:        &outputBuf,
	}
	cmd.Stdout = limitedOut

	var stderrBuf bytes.Buffer
	limitedErr := &limitedWriter{
		maxBytes: cfg.MaxOutputBytes,
		w:        &stderrBuf,
	}
	cmd.Stderr = limitedErr

	// Track timing
	startTime := time.Now()

	// Run
	runErr := cmd.Run()
	durationMs := time.Since(startTime).Milliseconds()

	// Build result
	result := &ExternalToolResult{
		DurationMs: durationMs,
	}

	// Combine output
	output := outputBuf.String()
	if stderrBuf.Len() > 0 {
		if output != "" {
			output += "\n" + stderrBuf.String()
		} else {
			output = stderrBuf.String()
		}
	}

	// Enforce output size limit
	if len(output) > cfg.MaxOutputBytes {
		output = output[:cfg.MaxOutputBytes] + "\n... (truncated at 1MB)"
	}
	result.Output = output

	// Determine exit code and error
	if runErr != nil {
		if ctxErr := execCtx.Err(); ctxErr == context.DeadlineExceeded {
			result.ExitCode = -1
			result.Error = fmt.Sprintf("timeout after %v", cfg.Timeout)
			result.Output = result.Error
			slog.Warn("sandbox: tool timed out", "executable", executable, "timeout", cfg.Timeout)
		} else if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Error = fmt.Sprintf("exit code %d", exitErr.ExitCode())
			slog.Debug("sandbox: tool exited with code", "executable", executable, "exit_code", exitErr.ExitCode())
		} else {
			result.ExitCode = -2
			result.Error = fmt.Sprintf("execution failed: %v", runErr)
			slog.Warn("sandbox: tool execution failed", "executable", executable, "error", runErr)
		}
	} else {
		result.ExitCode = 0
	}

	slog.Debug("sandbox: tool completed",
		"executable", executable,
		"exit_code", result.ExitCode,
		"duration_ms", result.DurationMs,
		"output_bytes", len(result.Output),
	)

	return result, nil
}

// ============================================================================
// limitedWriter — Prevents unbounded output from consuming memory
// ============================================================================

type limitedWriter struct {
	maxBytes int
	written  int
	w        io.Writer
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	available := w.maxBytes - w.written
	if available <= 0 {
		return len(p), nil // silently drop excess
	}
	if len(p) > available {
		p = p[:available]
	}
	n, err := w.w.Write(p)
	w.written += n
	return len(p), err
}
