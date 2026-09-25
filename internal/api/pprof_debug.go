// Package api: loopback-only pprof debug listener (PERF-CONSENSUS-11).
//
// The serve binary exposes net/http/pprof handlers (registered on
// http.DefaultServeMux by the side-effect import) on a dedicated listener
// bound to 127.0.0.1. The public API router never mounts /debug/pprof —
// pprof endpoints are unauthenticated, so a public mount would repeat the
// DF-CONSENSUS-19 class of exposure.
//
// The listener is started by cmd/consensus/main.go (runServer) using the
// server.pprof_addr config value (default 127.0.0.1:8095, empty = off).
package api

import (
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* on http.DefaultServeMux
	"strings"
)

// loopbackHosts are the only hosts a pprof listener may bind to.
var loopbackHosts = map[string]bool{
	"127.0.0.1": true,
	"::1":       true,
	"localhost": true,
}

// StartPprofListener starts the loopback-only pprof debug listener serving
// http.DefaultServeMux (/debug/pprof/*). The address must resolve to a
// loopback host — anything else is refused (logged loudly, nil returned) so
// a config error can never expose unauthenticated pprof publicly.
// Returns the running listener (close it on shutdown) or nil.
func StartPprofListener(addr string) net.Listener {
	if addr == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		slog.Error("pprof: invalid debug address, listener disabled", "addr", addr, "error", err)
		return nil
	}
	// A blank host means all interfaces — refuse it explicitly.
	if host == "" || !loopbackHosts[strings.ToLower(host)] {
		slog.Error("pprof: refusing non-loopback debug address (pprof is unauthenticated); listener disabled",
			"addr", addr)
		return nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// A busy debug port must never take the server down.
		slog.Warn("pprof: debug listener failed to start", "addr", addr, "error", err)
		return nil
	}
	go func() {
		if err := http.Serve(ln, http.DefaultServeMux); err != nil {
			// ErrServerClosed only happens via ln.Close(), i.e. shutdown.
			slog.Debug("pprof: debug listener stopped", "addr", addr, "error", err)
		}
	}()
	slog.Info("pprof: debug listener started (loopback only)", "addr", ln.Addr().String(),
		"endpoints", "/debug/pprof/, /debug/pprof/heap, /debug/pprof/goroutine?debug=2")
	return ln
}
