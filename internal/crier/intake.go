package crier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/wojons/consensus/internal/db"
)

// ============================================================================
// Provenance
// ============================================================================

// ProvenanceOrigin is the marker every crier-originated ledger row carries.
const ProvenanceOrigin = "crier"

// provenanceUnknown is the source recorded when neither the payload nor crier
// names the sender.
const provenanceUnknown = "unknown"

// ProvenanceContent renders the exact text stored as the ledger row's content:
// a bracketed provenance marker, then the message body verbatim.
//
// The marker is the whole provenance record, because memory_events has no
// metadata/JSON column (migrations/001_initial_schema.sql §3.1: id, type,
// content, summary_text, session_id, iteration_created, linked_memory_pages,
// embedding, created_at) and the ledger is append-only — the same constraint
// that makes internal/webhook write "[webhook <eventType>] <payload>". Marker
// shape:
//
//	[crier agent=<agent> msg=<message id> from=<source> topic=<topic>] <body>
//
// `topic` is omitted when the payload names none: a topic belongs to the relay
// (crier §4), so a producer that delivers straight into an inbox has none.
// `msg` is crier's own message id, which is the handle a later audit, ack or
// deduplication check resolves against.
func ProvenanceContent(agentName string, msg InboundMessage) string {
	parts := []string{
		ProvenanceOrigin,
		"agent=" + agentName,
		"msg=" + msg.ID,
		"from=" + msg.Source,
	}
	if msg.Topic != "" {
		parts = append(parts, "topic="+msg.Topic)
	}
	return "[" + strings.Join(parts, " ") + "] " + msg.Body
}

// ============================================================================
// Inbound message decoding
// ============================================================================

// InboundMessage is one crier inbox entry with its payload decoded.
type InboundMessage struct {
	// ID is crier's message id (provenance handle).
	ID string
	// AgentID is the inbox owner crier recorded — the consuming agent.
	AgentID string
	// Source is who sent it: the payload's own source/from/sender/agent field
	// when it carries one, else crier's recorded sender, else "unknown".
	Source string
	// Topic is the payload's topic when it names one.
	Topic string
	// Body is the agent-visible text: the payload's text/content/body/message
	// field when present, else the payload verbatim.
	Body string
	// Raw is the decoded payload, byte for byte.
	Raw []byte
}

// ParseInbound decodes one inbox entry. ok is false when the entry carries no
// agent-visible body at all — an empty or JSON-null payload — which is the one
// case the intake consumes without storing.
//
// Payload shapes, in order:
//
//   - a JSON object — the documented producer shape is
//     {"source": "...", "text": "..."} (crier integration-guide §9.2/§10.2).
//     Body comes from text/content/body/message, source from
//     source/from/sender/agent/agent_id, topic from topic.
//   - a JSON string — taken as the body.
//   - anything else (a number, an array, JSON that does not parse) — the
//     payload VERBATIM becomes the body. Nothing is dropped for having an
//     unexpected shape.
func ParseInbound(entry InboxEntry) (InboundMessage, bool) {
	msg := InboundMessage{
		ID:      entry.ID,
		AgentID: entry.AgentID,
		Source:  entry.Sender,
		Raw:     entry.Payload,
	}

	trimmed := bytes.TrimSpace(entry.Payload)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		if msg.Source == "" {
			msg.Source = provenanceUnknown
		}
		return msg, false
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err == nil && obj != nil {
		msg.Topic = stringField(obj, "topic")
		msg.Source = firstNonEmpty(
			msg.Source,
			stringField(obj, "source"),
			stringField(obj, "from"),
			stringField(obj, "sender"),
			stringField(obj, "agent"),
			stringField(obj, "agent_id"),
		)
		msg.Body = firstNonEmpty(
			stringField(obj, "text"),
			stringField(obj, "content"),
			stringField(obj, "body"),
			stringField(obj, "message"),
		)
	} else {
		var asString string
		if err := json.Unmarshal(trimmed, &asString); err == nil {
			msg.Body = asString
		}
	}

	if msg.Body == "" {
		msg.Body = string(entry.Payload)
	}
	if msg.Source == "" {
		msg.Source = provenanceUnknown
	}
	return msg, true
}

// stringField returns a JSON string field, or "" when the key is absent or
// holds a non-string value.
func stringField(obj map[string]json.RawMessage, key string) string {
	raw, ok := obj[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// firstNonEmpty returns the first non-empty argument.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ============================================================================
// Store
// ============================================================================

// Store persists one inbound message as an agent-visible ledger row.
// *DBStore is the production implementation; tests substitute a fake.
type Store interface {
	AppendUserMessage(ctx context.Context, sessionID, content string) error
}

// DBStore is the database-backed Store.
//
// It writes exactly what the two existing external-entry points write:
// api.MessageService.SendMessage and internal/webhook's routed wake both
// insert a 'user_message' memory_events row stamped for the iteration that
// will run, with a UTC RFC3339 timestamp. The harness projects those rows as
// LLM user turns (internal/harness/context.go), which is what makes an inbound
// message agent-visible.
type DBStore struct {
	DB db.DB
}

// AppendUserMessage appends one inbound message to the session ledger.
//
// iteration_created is the session's current iteration + 1: the row is
// stamped for the iteration that will run next, not the one already running,
// so a message delivered during an iteration is visible to the next one.
func (s *DBStore) AppendUserMessage(ctx context.Context, sessionID, content string) error {
	if s == nil || s.DB == nil {
		return errors.New("crier: DBStore has no database")
	}
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("crier: session id is required")
	}

	rows, err := s.DB.Query(ctx, `SELECT iteration FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		return fmt.Errorf("read session %s: %w", sessionID, err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("session %s not found", sessionID)
	}
	iteration := asInt64(rows[0]["iteration"]) + 1

	if err := s.DB.Exec(ctx,
		`INSERT INTO memory_events (type, content, session_id, iteration_created, created_at)
		 VALUES ('user_message', $1, $2, $3, $4)`,
		content, sessionID, iteration, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("append user_message: %w", err)
	}
	return nil
}

// asInt64 reads a numeric column from a driver row. The two backends can hand
// back int64, int, float64 or their text form.
func asInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case float64:
		return int64(n)
	case []byte:
		parsed, _ := strconv.ParseInt(string(n), 10, 64)
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(n, 10, 64)
		return parsed
	default:
		return 0
	}
}

// ============================================================================
// Intake
// ============================================================================

// Intake consumes an agent's durable crier inbox into a Consensus session's
// ledger.
type Intake struct {
	client *Client
	store  Store
	log    *slog.Logger
}

// NewIntake wires a crier client to a ledger store.
func NewIntake(client *Client, store Store) *Intake {
	return &Intake{client: client, store: store, log: slog.Default()}
}

// WithLogger overrides the logger used for intake diagnostics.
func (i *Intake) WithLogger(logger *slog.Logger) *Intake {
	if logger != nil {
		i.log = logger
	}
	return i
}

// Result reports what one intake pass did.
type Result struct {
	// Retrieved is how many messages crier handed back under the lease.
	Retrieved int
	// Appended is how many memory_events rows were written.
	Appended int
	// Skipped is how many retrieved messages carried no agent-visible body.
	// They are acked but not stored, so an empty payload cannot block the
	// queue forever.
	Skipped int
	// Acked is how many messages were acknowledged to crier.
	Acked int
	// MessageIDs lists the crier message ids appended, in retrieval order.
	MessageIDs []string
}

// Run performs one intake pass for agentName's inbox into sessionID: retrieve
// with a lease, append every pending message as a 'user_message' memory_events
// row carrying crier provenance, then ack the batch.
//
// An empty inbox is a no-op success: no rows, no ack, no error.
//
// If a store write fails midway the pass returns immediately and acks NOTHING,
// so crier redelivers the whole batch once the lease expires — at-least-once,
// never silent loss. Because the ledger is append-only there is no
// deduplication against it, so a redelivered batch is appended again;
// message-id-based idempotency is a separate follow-up, not a property of this
// function. Callers that must not double-append should hold the lease and
// retry with the ids from Result.MessageIDs in hand.
func (i *Intake) Run(ctx context.Context, sessionID, agentName string) (*Result, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("crier intake: session id is required")
	}
	if strings.TrimSpace(agentName) == "" {
		return nil, errors.New("crier intake: agent name is required")
	}

	leased, err := i.client.Retrieve(ctx, agentName, RetrieveOptions{})
	if err != nil {
		return nil, fmt.Errorf("crier intake: %w", err)
	}

	out := &Result{Retrieved: len(leased.Messages), MessageIDs: []string{}}
	if len(leased.Messages) == 0 {
		// A successful read with nothing to claim — an empty inbox, or every
		// queued message already held under someone else's live lease. There
		// is no lease to ack.
		return out, nil
	}

	ids := make([]string, 0, len(leased.Messages))
	for _, entry := range leased.Messages {
		ids = append(ids, entry.ID)

		msg, ok := ParseInbound(entry)
		if !ok {
			out.Skipped++
			i.log.Warn("crier intake: message carries no agent-visible body; acking without storing",
				"agent", agentName, "session_id", sessionID, "message_id", entry.ID)
			continue
		}

		if err := i.store.AppendUserMessage(ctx, sessionID, ProvenanceContent(agentName, msg)); err != nil {
			return out, fmt.Errorf("crier intake: session %s: %w", sessionID, err)
		}
		out.Appended++
		out.MessageIDs = append(out.MessageIDs, msg.ID)
	}

	if leased.LeaseID == "" {
		// crier mints a lease only when it claimed something; an empty
		// lease_id has nothing to ack and must not be sent to the ack
		// endpoint.
		return out, nil
	}
	if err := i.client.Ack(ctx, agentName, leased.LeaseID, ids); err != nil {
		return out, fmt.Errorf("crier intake: %w", err)
	}
	out.Acked = len(ids)
	return out, nil
}
