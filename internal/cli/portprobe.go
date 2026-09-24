// Port occupancy probe — C-GAP-038.
//
// Classifies whatever occupies a TCP port before the server binds, so a
// startup failure on a shadowed port produces actionable guidance instead
// of a bare EADDRINUSE. The probe is purely network-based and read-only:
// it never inspects, signals, or modifies the occupant process.
//
// axiom:trace work_item=C-GAP-038 spec=specs/016-cli-interface.md impl=internal/cli/portprobe.go
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// OccupantClass classifies what ProbePort found listening on a port.
type OccupantClass string

const (
	// OccupantNone — nothing is listening; the port is free to bind.
	OccupantNone OccupantClass = "none"
	// OccupantConsensus — a Consensus server answered /api/v1/health with status ok.
	OccupantConsensus OccupantClass = "consensus"
	// OccupantForeignHTTP — an HTTP service answered, but not as Consensus
	// (e.g. the 404-on-shadowed-port stale-sidecar case).
	OccupantForeignHTTP OccupantClass = "foreign-http"
	// OccupantNonHTTP — something accepts TCP but does not speak HTTP.
	OccupantNonHTTP OccupantClass = "non-http"
)

// ProbeResult is the outcome of a ProbePort call.
type ProbeResult struct {
	Occupied bool
	Class    OccupantClass
	Addr     string
	Port     int
}

// ProbePort classifies the occupant of host:port without modifying anything.
//
// Timeout budget (C-GAP-038 prohibitions): TCP dial 500ms, HTTP probe 2s max.
// Any dial failure (including timeout) is treated as "free" — startup must
// not block on a slow-to-refuse port; a genuine conflict is caught by the
// bind's EADDRINUSE fallback in cmd/consensus/main.go instead.
func ProbePort(host string, port int) ProbeResult {
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		// Refused, unroutable, or timed out — treat as free.
		return ProbeResult{Occupied: false, Class: OccupantNone, Addr: addr, Port: port}
	}
	conn.Close()

	client := &http.Client{
		Timeout: 2 * time.Second,
		// Classify on the FIRST response — never follow redirects into a
		// different service's handlers (or a malicious occupant's lure).
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get("http://" + addr + "/api/v1/health")
	if err != nil {
		// Accepts TCP but no HTTP response: TLS terminator, raw protocol, or immediate close.
		return ProbeResult{Occupied: true, Class: OccupantNonHTTP, Addr: addr, Port: port}
	}
	defer resp.Body.Close()

	// Limited read — the occupant may not be Consensus at all.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var health struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	// Minimal health-shape check, same spirit as client.VerifyIdentity
	// (status ok + non-empty version); intentionally NOT refactored to
	// share that logic (C-GAP-038 scope).
	if json.Unmarshal(body, &health) == nil && health.Status == "ok" && health.Version != "" {
		return ProbeResult{Occupied: true, Class: OccupantConsensus, Addr: addr, Port: port}
	}
	return ProbeResult{Occupied: true, Class: OccupantForeignHTTP, Addr: addr, Port: port}
}

// Diagnostic returns an actionable stderr message naming the occupant class,
// a read-only identification hint, and both port-override forms.
func (r ProbeResult) Diagnostic() string {
	next := r.Port + 1
	class := "a non-HTTP service"
	symptom := "it accepts TCP but does not answer HTTP"
	switch r.Class {
	case OccupantConsensus:
		class = "a Consensus server"
		symptom = "it answered /api/v1/health with a Consensus payload (is another instance already running?)"
	case OccupantForeignHTTP:
		class = "a foreign HTTP service"
		symptom = fmt.Sprintf("curl http://%s/api/v1/health returns \"404 page not found\" instead of {\"status\":\"ok\",...} — the tell-tale stale-sidecar shadow symptom", r.Addr)
	}
	return fmt.Sprintf(`consensus: port %s is occupied by %s (class=%s): %s

The port is shadowed by a stale consensus-sidecar or other leftover process;
the fresh server cannot bind (bind: address already in use). Identify the
occupant read-only — do not kill host state:
    ss -tlnp | grep :%d

Start on a free port instead (either form):
    CONSENSUS_PORT=%d ./bin/consensus serve
    ./bin/consensus serve --port %d
`, r.Addr, class, r.Class, symptom, r.Port, next, next)
}
