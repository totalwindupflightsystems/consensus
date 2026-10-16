// Command consensus-loadprobe is a Postgres backend load probe for the
// Consensus runtime (REVIEW-CONSENSUS-3).
//
// It drives N concurrent agent sessions through the real REST API against a
// live server, sends a fixed number of messages ("turns") per session, and
// measures per-turn latency, aggregate throughput, p50/p95/p99 latency and
// error rate at each concurrency level.
//
// The unit of work is one POST /api/v1/sessions/{id}/message — the same
// write path (auth lookup + rate-limit accounting + memory_events INSERT +
// sessions UPDATE) a production agent turn exercises. Every turn is sent with
// the session's own scoped API key, so the measured path includes the
// per-session auth and rate-limit accounting against the backend.
//
// This is probe infrastructure only — it is NOT product code and imports only
// the standard library.
//
// Usage:
//
//	consensus-loadprobe \
//	  --base-url http://127.0.0.1:8090 \
//	  --admin-key cs_ak_... \
//	  --concurrency 1,4,16,32 \
//	  --turns 200 \
//	  --out /tmp/loadprobe.json
//
// Output is a single JSON document: one entry per concurrency level with
// aggregate metrics plus the full raw per-turn latency sample set, so the
// committed evidence is machine-readable and reproducible rather than a
// summary.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const requestTimeout = 30 * time.Second

// createSessionResponse is the subset of the CreateSession response the probe
// needs (id + the session-scoped api_key).
type createSessionResponse struct {
	ID     string `json:"id"`
	APIKey string `json:"api_key"`
	Status string `json:"status"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// messageResponse is the message endpoint's success envelope; the error
// envelope is shared ({"error": {"code", "message"}}).
type messageResponse struct {
	Status  string `json:"status"`
	Session string `json:"session"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// levelResult is the aggregate for one concurrency level.
type levelResult struct {
	Concurrency      int            `json:"concurrency"`
	TurnsPerSession  int            `json:"turns_per_session"`
	SessionsCreated  int            `json:"sessions_created"`
	SessionErrors    int            `json:"session_create_errors"`
	SessionCreateErr string         `json:"session_create_error,omitempty"`
	TotalTurns       int            `json:"total_turns"`
	SuccessfulTurns  int            `json:"successful_turns"`
	ErrorTurns       int            `json:"error_turns"`
	ErrorRate        float64        `json:"error_rate"`
	DurationMs       int64          `json:"duration_ms"`
	ThroughputPerSec float64        `json:"throughput_turns_per_sec"`
	Latency          latencySummary `json:"latency_ms"`
	ErrorsByStatus   map[int]int    `json:"errors_by_status"`
	LatencySamples   []int64        `json:"latency_samples_ms"`
}

type latencySummary struct {
	Min  float64 `json:"min"`
	Mean float64 `json:"mean"`
	P50  float64 `json:"p50"`
	P95  float64 `json:"p95"`
	P99  float64 `json:"p99"`
	Max  float64 `json:"max"`
}

type report struct {
	Probe           string        `json:"probe"`
	GeneratedAt     string        `json:"generated_at"`
	BaseURL         string        `json:"base_url"`
	Backend         string        `json:"backend"`
	TurnsPerSession int           `json:"turns_per_session"`
	Levels          []levelResult `json:"levels"`
}

func main() {
	var (
		baseURL     = flag.String("base-url", "http://127.0.0.1:8090", "Consensus server base URL")
		adminKey    = flag.String("admin-key", "", "admin-scoped API key (cs_ak_...) used to create sessions")
		concurrency = flag.String("concurrency", "1,4,16,32", "comma-separated concurrency levels")
		turns       = flag.Int("turns", 200, "messages (turns) per session per level")
		out         = flag.String("out", "", "output JSON path (default: stdout)")
		backend     = flag.String("backend", "postgres", "backend label recorded in the report")
	)
	flag.Parse()

	if *adminKey == "" {
		fmt.Fprintln(os.Stderr, "consensus-loadprobe: --admin-key is required (cs_ak_...)")
		os.Exit(2)
	}

	levels, err := parseConcurrency(*concurrency)
	if err != nil {
		fmt.Fprintf(os.Stderr, "consensus-loadprobe: %v\n", err)
		os.Exit(2)
	}

	rep := report{
		Probe:           "consensus-loadprobe",
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		BaseURL:         strings.TrimRight(*baseURL, "/"),
		Backend:         *backend,
		TurnsPerSession: *turns,
		Levels:          []levelResult{},
	}

	client := &http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        256,
			MaxIdleConnsPerHost: 256,
		},
	}

	for _, n := range levels {
		res := runLevel(client, rep.BaseURL, *adminKey, n, *turns)
		rep.Levels = append(rep.Levels, res)
	}

	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "consensus-loadprobe: marshal report: %v\n", err)
		os.Exit(1)
	}

	if *out != "" {
		if err := os.WriteFile(*out, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "consensus-loadprobe: write %s: %v\n", *out, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "consensus-loadprobe: wrote %s\n", *out)
		return
	}
	fmt.Println(string(data))
}

func parseConcurrency(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid concurrency value %q", part)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no concurrency levels specified")
	}
	return out, nil
}

// runLevel creates N sessions, then has each session send turns messages
// concurrently. Session creation and turn sending are measured separately.
func runLevel(client *http.Client, baseURL, adminKey string, n, turns int) levelResult {
	res := levelResult{
		Concurrency:     n,
		TurnsPerSession: turns,
		ErrorsByStatus:  map[int]int{},
		LatencySamples:  []int64{},
	}

	// Phase 1 — create N sessions concurrently (admin key).
	type session struct {
		id  string
		key string
	}
	sessions := make([]session, n)
	var (
		createWg  sync.WaitGroup
		createErr sync.Mutex
	)
	createWg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer createWg.Done()
			id, key, err := createSession(client, baseURL, adminKey, i)
			if err == nil {
				sessions[i] = session{id: id, key: key}
				return
			}
			// Record the first non-nil error as a session-creation failure.
			createErr.Lock()
			if res.SessionCreateErr == "" {
				res.SessionCreateErr = err.Error()
			}
			res.SessionErrors++
			createErr.Unlock()
			fmt.Fprintf(os.Stderr, "  [level %d] create session %d failed: %v\n", n, i, err)
		}(i)
	}
	createWg.Wait()

	for _, s := range sessions {
		if s.id != "" {
			res.SessionsCreated++
		}
	}
	if res.SessionsCreated == 0 {
		res.TotalTurns = 0
		res.SuccessfulTurns = 0
		res.ErrorTurns = 0
		res.ErrorRate = 1.0
		res.DurationMs = 0
		res.ThroughputPerSec = 0
		res.Latency = latencySummary{}
		return res
	}

	// Phase 2 — each session sends `turns` messages concurrently.
	start := time.Now()
	var (
		samplesMu sync.Mutex
		okCount   int64
		errCount  int64
	)
	var turnWg sync.WaitGroup
	for si := 0; si < n; si++ {
		if sessions[si].id == "" {
			continue
		}
		turnWg.Add(1)
		go func(si int) {
			defer turnWg.Done()
			s := sessions[si]
			for k := 0; k < turns; k++ {
				latMs, status, _ := sendMessage(client, baseURL, s.id, s.key, si, k)
				samplesMu.Lock()
				res.LatencySamples = append(res.LatencySamples, latMs)
				res.ErrorsByStatus[status]++
				samplesMu.Unlock()
				if status >= 200 && status < 300 {
					atomic.AddInt64(&okCount, 1)
				} else {
					atomic.AddInt64(&errCount, 1)
				}
			}
		}(si)
	}
	turnWg.Wait()
	elapsed := time.Since(start)

	res.TotalTurns = int(okCount + errCount)
	res.SuccessfulTurns = int(okCount)
	res.ErrorTurns = int(errCount)
	res.DurationMs = elapsed.Milliseconds()
	if elapsed > 0 {
		res.ThroughputPerSec = float64(res.TotalTurns) / elapsed.Seconds()
	}
	if res.TotalTurns > 0 {
		res.ErrorRate = float64(res.ErrorTurns) / float64(res.TotalTurns)
	}
	res.Latency = summarize(res.LatencySamples)
	return res
}

func createSession(client *http.Client, baseURL, adminKey string, i int) (string, string, error) {
	body, _ := json.Marshal(map[string]any{
		"agent_name": fmt.Sprintf("loadprobe-%d", i),
		"goal":       "load probe",
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/sessions", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var parsed createSessionResponse
	_ = json.Unmarshal(raw, &parsed)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("http %d", resp.StatusCode)
		if parsed.Error != nil {
			msg = fmt.Sprintf("http %d %s: %s", resp.StatusCode, parsed.Error.Code, parsed.Error.Message)
		}
		return "", "", fmt.Errorf("%s", msg)
	}
	if parsed.ID == "" || parsed.APIKey == "" {
		return "", "", fmt.Errorf("create session returned empty id/key: %s", string(raw))
	}
	return parsed.ID, parsed.APIKey, nil
}

func sendMessage(client *http.Client, baseURL, sessionID, sessionKey string, si, k int) (int64, int, string) {
	content := fmt.Sprintf("loadprobe-%d-turn-%d-%d", si, k, time.Now().UnixNano())
	body, _ := json.Marshal(map[string]any{"content": content})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/sessions/"+sessionID+"/message", bytes.NewReader(body))
	if err != nil {
		return 0, 0, "CLIENT_ERROR"
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionKey)

	start := time.Now()
	resp, err := client.Do(req)
	lat := time.Since(start).Milliseconds()
	if err != nil {
		return lat, 0, "CLIENT_ERROR"
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var parsed messageResponse
	_ = json.Unmarshal(raw, &parsed)
	code := ""
	if parsed.Error != nil {
		code = parsed.Error.Code
	}
	return lat, resp.StatusCode, code
}

func summarize(samples []int64) latencySummary {
	if len(samples) == 0 {
		return latencySummary{}
	}
	sorted := make([]int64, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum int64
	for _, s := range sorted {
		sum += s
	}
	return latencySummary{
		Min:  float64(sorted[0]),
		Mean: float64(sum) / float64(len(sorted)),
		P50:  percentile(sorted, 0.50),
		P95:  percentile(sorted, 0.95),
		P99:  percentile(sorted, 0.99),
		Max:  float64(sorted[len(sorted)-1]),
	}
}

func percentile(sorted []int64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return float64(sorted[0])
	}
	if p >= 1 {
		return float64(sorted[len(sorted)-1])
	}
	// Nearest-rank method: the ceil(p*N)-th value (1-indexed).
	rank := int(math.Ceil(p * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return float64(sorted[rank-1])
}
