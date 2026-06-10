// Package api: event bus for real-time SSE stream (SPEC-015 §4).
//
// The EventBus allows clients to subscribe to per-session or global events.
// On Postgres, events originate from LISTEN/NOTIFY. On SQLite, events are
// pushed via a polling goroutine that checks for recent changes.
//
// axiom:trace work_item=WI-008-WI-009 spec=specs/015-api-and-mcp.md plan=phase-3/task-1 impl=internal/api/events.go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// EventBus is a pub/sub system for real-time session events.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string]map[int64]chan Event // sessionID → subID → channel
	nextID      int64
}

// Event represents a single SSE event.
type Event struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id,omitempty"`
	Type      string    `json:"type"` // "session_update", "approval_pending", "tool_result", "error", "quarantine_*"
	Data      any       `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}

// SessionUpdateEvent is emitted on session status changes.
type SessionUpdateEvent struct {
	Status    string `json:"status"`
	Iteration int64  `json:"iteration"`
}

// ApprovalPendingEvent is emitted when HITL approval is needed.
type ApprovalPendingEvent struct {
	ApprovalID  string `json:"approval_id"`
	RequestType string `json:"request_type"`
	RiskLevel   string `json:"risk_level"`
	Description string `json:"description"`
}

// NewEventBus creates a new EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string]map[int64]chan Event),
	}
}

// Subscribe registers a new subscriber channel for a session.
// sessionID can be "" for global events.
func (eb *EventBus) Subscribe(sessionID string) (int64, chan Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	id := eb.nextID
	eb.nextID++

	ch := make(chan Event, 64) // buffered to avoid blocking publishers
	if _, ok := eb.subscribers[sessionID]; !ok {
		eb.subscribers[sessionID] = make(map[int64]chan Event)
	}
	eb.subscribers[sessionID][id] = ch

	return id, ch
}

// Unsubscribe removes a subscriber by session and ID.
func (eb *EventBus) Unsubscribe(sessionID string, id int64) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if subs, ok := eb.subscribers[sessionID]; ok {
		delete(subs, id)
		if len(subs) == 0 {
			delete(eb.subscribers, sessionID)
		}
	}
}

// Publish sends an event to all subscribers for the given session.
func (eb *EventBus) Publish(sessionID string, event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	event.ID = time.Now().Format(time.RFC3339Nano)
	event.Timestamp = time.Now()

	// Send to session-specific subscribers
	if subs, ok := eb.subscribers[sessionID]; ok {
		for _, ch := range subs {
			select {
			case ch <- event:
			default:
				// buffer full, drop event (subscriber too slow)
			}
		}
	}

	// Also send to global subscribers
	if subs, ok := eb.subscribers[""]; ok {
		for _, ch := range subs {
			select {
			case ch <- event:
			default:
			}
		}
	}
}

// PublishSessionUpdate is a convenience method for session status changes.
func (eb *EventBus) PublishSessionUpdate(sessionID, status string, iteration int64) {
	eb.Publish(sessionID, Event{
		SessionID: sessionID,
		Type:      "session_update",
		Data: SessionUpdateEvent{
			Status:    status,
			Iteration: iteration,
		},
	})
}

// PublishQuarantineEvent publishes a quarantine-related event to the SSE stream.
// This satisfies the quarantine.EventPublisher interface when wrapped.
func (eb *EventBus) PublishQuarantineEvent(sessionID, eventType string, eventData any) {
	eb.Publish(sessionID, Event{
		SessionID: sessionID,
		Type:      eventType,
		Data:      eventData,
	})
}

// ============================================================================
// SSE Handler
// ============================================================================

// HandleSSE serves a Server-Sent Events stream for real-time updates.
// Clients connect with ?session_id=<id> to subscribe to a specific session,
// or without for global events.
func (s *Server) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	sessionID := r.URL.Query().Get("session_id")
	id, ch := s.events.Subscribe(sessionID)
	defer s.events.Unsubscribe(sessionID, id)

	// Send initial connection event
	initial := Event{Type: "connected", SessionID: sessionID, Data: map[string]string{"status": "connected"}}
	if data, err := json.Marshal(initial); err == nil {
		sseWrite(w, flusher, "connected", data)
	}

	// Watch context cancellation for client disconnect
	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-ch:
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if !sseWrite(w, flusher, event.Type, data) {
				return
			}
		}
	}
}

func sseWrite(w http.ResponseWriter, flusher http.Flusher, eventType string, data []byte) bool {
	_, err := w.Write([]byte("event: " + eventType + "\ndata: " + string(data) + "\n\n"))
	if err != nil {
		return false
	}
	flusher.Flush()
	return true
}

// ============================================================================
// Postgres LISTEN/NOTIFY → EventBus Bridge (SPEC-015 §4.1)
// ============================================================================

// PostgresNotificationHandler creates an EventBus-compatible handler for
// Postgres LISTEN/NOTIFY notifications. Call this with the EventBus to wire
// notifications from session_events and approval_events channels.
//
// Returns a function that handles Notification messages by parsing the JSON
// payload and publishing them as EventBus events.
func PostgresNotificationHandler(events *EventBus) func(channel string, payload string) {
	return func(channel string, payload string) {
		switch channel {
		case "session_events":
			// Parse the notification payload into an Event
			var data map[string]any
			if err := json.Unmarshal([]byte(payload), &data); err != nil {
				slog.Warn("events: failed to parse session_events notification", "error", err)
				return
			}
			sessionID, _ := data["session_id"].(string)
			status, _ := data["status"].(string)
			iteration, _ := data["iteration"].(float64)

			events.Publish(sessionID, Event{
				SessionID: sessionID,
				Type:      "session_update",
				Data: SessionUpdateEvent{
					Status:    status,
					Iteration: int64(iteration),
				},
			})

		case "approval_events":
			var data map[string]any
			if err := json.Unmarshal([]byte(payload), &data); err != nil {
				slog.Warn("events: failed to parse approval_events notification", "error", err)
				return
			}
			sessionID, _ := data["session_id"].(string)
			approvalID, _ := data["approval_id"].(string)
			requestType, _ := data["request_type"].(string)
			riskLevel, _ := data["risk_level"].(string)
			description, _ := data["description"].(string)

			events.Publish(sessionID, Event{
				SessionID: sessionID,
				Type:      "approval_pending",
				Data: ApprovalPendingEvent{
					ApprovalID:  approvalID,
					RequestType: requestType,
					RiskLevel:   riskLevel,
					Description: description,
				},
			})

		default:
			slog.Debug("events: unknown notification channel", "channel", channel)
		}
	}
}

// ============================================================================
// SQLite Event Poller (SPEC-015 §8 — Go channels)
// ============================================================================

// RowQueryFn is a function type for querying rows, matching db.DB.Query
// signature (returns []map[string]any).
type RowQueryFn func(ctx interface{}, query string, args ...any) ([]map[string]any, error)

// StartSQliteEventPoller starts a background goroutine that polls the database
// for new events and publishes them to the EventBus. This is the SQLite
// equivalent of Postgres LISTEN/NOTIFY.
//
// The poller tracks the last-seen audit_log ID and uses a 1-second tick.
// It queries audit_logs for session events and approval_requests for approvals.
//
// Returns a stop function to terminate the goroutine.
func StartSQliteEventPoller(ctx interface {
	Done() <-chan struct{}
}, events *EventBus, queryFn RowQueryFn) func() {
	stopCh := make(chan struct{})
	var lastAuditLogID int64
	var lastApprovalCheck string

	slog.Info("events: SQLite poller started", "interval", "1s")

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				pollSQliteSessionEvents(events, queryFn, &lastAuditLogID)
				pollSQliteApprovalEvents(events, queryFn, &lastApprovalCheck)
			case <-stopCh:
				slog.Info("events: SQLite poller stopped")
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return func() { close(stopCh) }
}

// pollSQliteSessionEvents checks for new session events via audit_logs.
func pollSQliteSessionEvents(events *EventBus, queryFn RowQueryFn, lastID *int64) {
	rows, err := queryFn(nil,
		`SELECT al.id, al.session_id, al.iteration, s.status
		 FROM audit_logs al
		 JOIN sessions s ON s.id = al.session_id
		 WHERE al.id > $1
		 ORDER BY al.id ASC LIMIT 50`, *lastID)
	if err != nil || len(rows) == 0 {
		return
	}
	for _, row := range rows {
		id := toInt64(row["id"])
		if id > *lastID {
			*lastID = id
		}
		sessionID := toString(row["session_id"])
		status := toString(row["status"])
		iteration := toInt64(row["iteration"])

		events.Publish(sessionID, Event{
			SessionID: sessionID,
			Type:      "session_update",
			Data: SessionUpdateEvent{
				Status:    status,
				Iteration: iteration,
			},
		})
	}
}

// pollSQliteApprovalEvents checks for new approval requests.
func pollSQliteApprovalEvents(events *EventBus, queryFn RowQueryFn, lastCheck *string) {
	rows, err := queryFn(nil,
		`SELECT id, session_id, request_type, risk_level, description, status, created_at
		 FROM approval_requests
		 WHERE created_at > $1 OR (created_at = $1 AND CAST(id AS TEXT) > $2)
		 ORDER BY created_at ASC, id ASC LIMIT 50`,
		*lastCheck, *lastCheck)
	if err != nil || len(rows) == 0 {
		return
	}
	for _, row := range rows {
		approvalID := toString(row["id"])
		createdAt := toString(row["created_at"])
		if createdAt > *lastCheck || (*lastCheck == "" && createdAt != "") {
			*lastCheck = createdAt
		}
		sessionID := toString(row["session_id"])
		requestType := toString(row["request_type"])
		riskLevel := toString(row["risk_level"])
		description := toString(row["description"])

		events.Publish(sessionID, Event{
			SessionID: sessionID,
			Type:      "approval_pending",
			Data: ApprovalPendingEvent{
				ApprovalID:  approvalID,
				RequestType: requestType,
				RiskLevel:   riskLevel,
				Description: description,
			},
		})
	}
}
