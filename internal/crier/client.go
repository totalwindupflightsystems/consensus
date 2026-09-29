package crier

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// Endpoint constants
// ============================================================================

const (
	// DefaultBaseURL is crier's local-development default (CRIER_PORT=8767).
	DefaultBaseURL = "http://localhost:8767"

	// EnvBaseURL overrides an empty base URL handed to NewClient. The same
	// variable backs config.crier.url via internal/config's env overrides, so
	// a deployment that configures nothing and one that uses another config
	// file both resolve to the same endpoint.
	EnvBaseURL = "CONSENSUS_CRIER_URL"

	// pathAgents is the registry collection; per-agent paths are built from it.
	pathAgents = "/agents"

	// maxResponseBytes bounds one response read. crier caps a delivery REQUEST
	// at CR_INBOX_MAX_BODY_BYTES (1 MiB default); 8 MiB leaves room for a
	// full batch of base64-encoded entries (base64 is ~4/3 of the raw size).
	maxResponseBytes = 8 << 20

	// headerAgentID, headerAgentTs and headerAgentSig are the per-agent
	// signature triple crier requires on retrieve, ack and stats whenever
	// CR_REQUIRE_AGENT_SIG is on — its default.
	headerAgentID  = "X-Agent-ID"
	headerAgentTs  = "X-Agent-Ts"
	headerAgentSig = "X-Agent-Sig"
)

// ============================================================================
// Errors
// ============================================================================

// APIError is a non-2xx answer from crier. StatusCode is crier's own HTTP
// status and Message is the server's "error" field when the body carried one —
// callers branch on StatusCode (409 means "already registered", 403 means the
// signature was presented as an agent other than the {id} in the path, 401
// means the signature headers were missing or invalid).
type APIError struct {
	Operation  string
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("crier %s: HTTP %d", e.Operation, e.StatusCode)
	}
	return fmt.Sprintf("crier %s: HTTP %d: %s", e.Operation, e.StatusCode, e.Message)
}

// ============================================================================
// Wire types
// ============================================================================

// Agent is a crier registry row.
type Agent struct {
	ID           string   `json:"id"`
	PublicKey    string   `json:"public_key,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Status       string   `json:"status,omitempty"`
}

// RegisterInput is the body of POST /agents. PublicKey is required whenever
// the server enforces per-agent signatures (CR_REQUIRE_AGENT_SIG=true, the
// default): with the key the agent's identity IS the key, so a registration
// without one is refused 400.
type RegisterInput struct {
	ID           string   `json:"id"`
	PublicKey    string   `json:"public_key,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// DeliverInput is the body of POST /agents/{id}/inbox.
type DeliverInput struct {
	// Payload is REQUIRED and must be a JSON object. crier refuses a body that
	// omits it, sends null, or sends a non-object with 400, and dispatches or
	// stores nothing.
	Payload any `json:"payload"`
	// Sender is the originating agent id. It is recorded with the message, so
	// it is the address crier reports a terminal outcome to (a message that
	// expires unacknowledged raises an MESSAGE_EXPIRED receipt in the
	// sender's inbox).
	Sender string `json:"sender,omitempty"`
	// SessionID and ThreadID are conversation context for session-aware
	// policies on the receiving side.
	SessionID string `json:"session_id,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
	// RequestID is the sender's correlation id, echoed in a blocking reply.
	RequestID string `json:"request_id,omitempty"`
	// Priority is the retrieval priority, 0..9 (higher is handed back first;
	// 0 is plain FIFO).
	Priority int `json:"priority,omitempty"`
}

// DeliverResult is crier's accept answer for a delivery.
type DeliverResult struct {
	ID        string `json:"id"`
	Transport string `json:"transport"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// InboxEntry is one leased inbox message. Payload arrives as a base64 string
// (Go's []byte JSON encoding) and decodes to the exact bytes the producer
// delivered — encoding/json decodes it into the []byte here.
type InboxEntry struct {
	ID        string  `json:"id"`
	AgentID   string  `json:"agent_id"`
	Payload   []byte  `json:"payload"`
	CreatedAt string  `json:"created_at,omitempty"`
	ExpiresAt *string `json:"expires_at,omitempty"`
	LeasedAt  string  `json:"leased_at,omitempty"`
	LeaseID   string  `json:"lease_id,omitempty"`
	Acked     bool    `json:"acked"`
	Sender    string  `json:"sender,omitempty"`
}

// RetrieveOptions are the documented retrieve query parameters. A zero field
// means "use the server's default" rather than a literal zero.
type RetrieveOptions struct {
	// Limit caps the messages claimed by one retrieve (crier default 10;
	// > 100 is a 400).
	Limit int
	// LeaseSeconds is the lease duration (crier default 30). Unacked messages
	// return under a fresh lease once it expires.
	LeaseSeconds int
	// WaitSeconds is the long-poll budget, 0..120. Zero is an immediate
	// poll-only read.
	WaitSeconds int
}

// RetrieveResult is crier's retrieve answer. Messages is always a JSON array —
// empty, never null — and LeaseID is an empty string exactly when nothing was
// claimed, in which case there is nothing to ack.
type RetrieveResult struct {
	Messages    []InboxEntry `json:"messages"`
	LeaseID     string       `json:"lease_id"`
	QueueDepth  int          `json:"queue_depth"`
	LeasedCount int          `json:"leased_count"`
}

// ackRequest is the body of POST /agents/{id}/inbox/ack. Both fields are
// required: a lease-only ack is a 400.
type ackRequest struct {
	LeaseID    string   `json:"lease_id"`
	MessageIDs []string `json:"message_ids"`
}

// ============================================================================
// Signing
// ============================================================================

// Signer produces crier's per-agent signature for one request.
//
// The signed payload is exactly "METHOD\n<path>\n<unix-seconds>", with the
// path taken WITHOUT its query string, and the timestamp within ±30s of the
// server clock (crier docs/integration-guide.md §3). The scheme is required on
// retrieve, ack and stats when the server runs with CR_REQUIRE_AGENT_SIG=true
// (its default) and the caller in X-Agent-ID must be the {id} in the path —
// any other identity is refused 403.
type Signer interface {
	Sign(method, path string, unixSeconds int64) (string, error)
}

// Ed25519Signer signs with an in-memory ed25519 private key and renders the
// hex-encoded signature the X-Agent-Sig header carries.
type Ed25519Signer struct {
	key ed25519.PrivateKey
}

// NewEd25519Signer wraps a private key (ed25519.PrivateKeySize bytes).
func NewEd25519Signer(key ed25519.PrivateKey) *Ed25519Signer {
	return &Ed25519Signer{key: key}
}

// Sign returns hex(ed25519_sign("METHOD\n<path>\n<unix-seconds>")).
func (s *Ed25519Signer) Sign(method, path string, unixSeconds int64) (string, error) {
	if len(s.key) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("crier: ed25519 private key is %d bytes, want %d",
			len(s.key), ed25519.PrivateKeySize)
	}
	sig := ed25519.Sign(s.key, []byte(SignaturePayload(method, path, unixSeconds)))
	return hex.EncodeToString(sig), nil
}

// SignaturePayload renders the exact bytes crier signs for an agent-scoped
// request. Exported so a caller (and its tests) can verify a signature without
// re-deriving the contract.
func SignaturePayload(method, path string, unixSeconds int64) string {
	return method + "\n" + path + "\n" + strconv.FormatInt(unixSeconds, 10)
}

// PublicKeyHex returns the hex-encoded ed25519 public key for a private key —
// the value POST /agents expects in public_key.
func PublicKeyHex(key ed25519.PrivateKey) string {
	if len(key) != ed25519.PrivateKeySize {
		return ""
	}
	pub, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return ""
	}
	return hex.EncodeToString(pub)
}

// ============================================================================
// Client
// ============================================================================

// Client is the crier registry/inbox HTTP client.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	bearerToken string
	agentID     string
	signer      Signer
}

// NewClient builds a client for baseURL. An empty baseURL falls back to
// CONSENSUS_CRIER_URL and then DefaultBaseURL, so an unset config key and an
// env-only deployment both resolve without the caller branching.
func NewClient(baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = strings.TrimSpace(os.Getenv(EnvBaseURL))
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// WithHTTPClient overrides the HTTP client (timeouts, transport, tests).
func (c *Client) WithHTTPClient(hc *http.Client) *Client {
	if hc != nil {
		c.httpClient = hc
	}
	return c
}

// WithBearerToken sets the shared bearer token (CR_AUTH_TOKEN) sent as
// "Authorization: Bearer <token>". Empty leaves the header off, which is what
// an auth-disabled crier expects.
func (c *Client) WithBearerToken(token string) *Client {
	c.bearerToken = token
	return c
}

// WithSigner sets the per-agent signer used on the agent-scoped requests
// (retrieve, ack). agentID is the identity the signature is presented as.
func (c *Client) WithSigner(agentID string, s Signer) *Client {
	c.agentID = agentID
	c.signer = s
	return c
}

// BaseURL returns the resolved base URL.
func (c *Client) BaseURL() string { return c.baseURL }

// ============================================================================
// Registry / inbox operations
// ============================================================================

// Register registers an agent (POST /agents).
//
// A duplicate id answers 409 with the existing row untouched; that is treated
// as success, because the caller's requirement ("this identity exists") is
// already satisfied. The returned Agent then carries only the requested id —
// the server's existing row is NOT read back, so its public key and
// capabilities are not part of the answer.
func (c *Client) Register(ctx context.Context, in RegisterInput) (*Agent, error) {
	if strings.TrimSpace(in.ID) == "" {
		return nil, fmt.Errorf("crier register: id is required")
	}
	status, body, err := c.do(ctx, http.MethodPost, pathAgents, nil, in)
	if err != nil {
		return nil, err
	}
	if status == http.StatusConflict {
		return &Agent{ID: in.ID}, nil
	}
	if err := checkStatus(status, body, "register"); err != nil {
		return nil, err
	}
	var agent Agent
	if err := json.Unmarshal(body, &agent); err != nil {
		return nil, fmt.Errorf("crier register: decode response: %w", err)
	}
	return &agent, nil
}

// Deliver delivers a message into an agent's durable inbox
// (POST /agents/{id}/inbox). No signature is required in any configuration —
// any registered producer may write into any agent's inbox, which is what
// makes it a mailbox.
func (c *Client) Deliver(ctx context.Context, agentID string, in DeliverInput) (*DeliverResult, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("crier deliver: agent id is required")
	}
	if in.Payload == nil {
		return nil, fmt.Errorf("crier deliver: payload is required (crier refuses a delivery with no payload)")
	}
	status, body, err := c.do(ctx, http.MethodPost, inboxPath(agentID), nil, in)
	if err != nil {
		return nil, err
	}
	if err := checkStatus(status, body, "deliver"); err != nil {
		return nil, err
	}
	var res DeliverResult
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("crier deliver: decode response: %w", err)
	}
	return &res, nil
}

// Retrieve leases pending inbox messages for agentID (GET /agents/{id}/inbox).
//
// An empty inbox — or one whose messages are all held under live leases — is a
// 200 with an empty messages array and an empty lease_id: a successful read
// with nothing to claim, not an error.
func (c *Client) Retrieve(ctx context.Context, agentID string, opts RetrieveOptions) (*RetrieveResult, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("crier retrieve: agent id is required")
	}
	query := url.Values{}
	if opts.Limit > 0 {
		query.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.LeaseSeconds > 0 {
		query.Set("lease_seconds", strconv.Itoa(opts.LeaseSeconds))
	}
	if opts.WaitSeconds > 0 {
		query.Set("wait_seconds", strconv.Itoa(opts.WaitSeconds))
	}

	status, body, err := c.do(ctx, http.MethodGet, inboxPath(agentID), query, nil)
	if err != nil {
		return nil, err
	}
	if err := checkStatus(status, body, "retrieve"); err != nil {
		return nil, err
	}
	var res RetrieveResult
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("crier retrieve: decode response: %w", err)
	}
	if res.Messages == nil {
		res.Messages = []InboxEntry{}
	}
	return &res, nil
}

// Ack acknowledges messages under the lease that returned them
// (POST /agents/{id}/inbox/ack) — 204 with no body on success.
//
// Both arguments are required by crier: a lease that claimed nothing is an
// empty string and must never be sent, and a lease-only ack is a 400. Only ids
// from the SAME retrieve call as leaseID may be acked; a stale lease is a 409
// and an unknown id a 404.
func (c *Client) Ack(ctx context.Context, agentID, leaseID string, messageIDs []string) error {
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("crier ack: agent id is required")
	}
	if leaseID == "" {
		return fmt.Errorf("crier ack: lease id is required (an empty lease means nothing was claimed)")
	}
	if len(messageIDs) == 0 {
		return fmt.Errorf("crier ack: at least one message id is required")
	}
	status, body, err := c.do(ctx, http.MethodPost, inboxAckPath(agentID), nil,
		ackRequest{LeaseID: leaseID, MessageIDs: messageIDs})
	if err != nil {
		return err
	}
	return checkStatus(status, body, "ack")
}

// ============================================================================
// Transport
// ============================================================================

// inboxPath is the per-agent inbox path.
func inboxPath(agentID string) string { return pathAgents + "/" + agentID + "/inbox" }

// inboxAckPath is the per-agent ack path.
func inboxAckPath(agentID string) string { return inboxPath(agentID) + "/ack" }

// do performs one request and returns the status code and the response body.
// The path is signed WITHOUT its query string when a signer is configured —
// crier excludes the query from the signed payload.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, payload any) (int, []byte, error) {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, fmt.Errorf("crier %s %s: encode request: %w", method, path, err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return 0, nil, fmt.Errorf("crier %s %s: build request: %w", method, path, err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
	if c.signer != nil {
		ts := time.Now().Unix()
		sig, err := c.signer.Sign(method, path, ts)
		if err != nil {
			return 0, nil, fmt.Errorf("crier %s %s: sign request: %w", method, path, err)
		}
		req.Header.Set(headerAgentID, c.agentID)
		req.Header.Set(headerAgentTs, strconv.FormatInt(ts, 10))
		req.Header.Set(headerAgentSig, sig)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("crier %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("crier %s %s: read response: %w", method, path, err)
	}
	return resp.StatusCode, data, nil
}

// checkStatus converts a non-2xx answer into an *APIError, carrying crier's own
// error message when the body has one.
func checkStatus(status int, body []byte, operation string) error {
	if status >= 200 && status < 300 {
		return nil
	}
	message := ""
	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		message = envelope.Error
	}
	return &APIError{Operation: operation, StatusCode: status, Message: message}
}
