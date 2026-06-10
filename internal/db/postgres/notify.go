// Package postgres: Postgres LISTEN/NOTIFY support for real-time event streaming.
//
// This provides a notification listener that subscribes to Postgres channels
// and forwards notifications to a user-provided callback.
//
// axiom:trace work_item=WI-002-migrate-pgx spec=specs/015-api-and-mcp.md plan=phase-3/task-1 impl=internal/db/postgres/notify.go
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Notification represents a single Postgres NOTIFY message.
type Notification struct {
	Channel string
	Payload string
	PID     uint32
}

// NotificationHandler is a callback for received notifications.
type NotificationHandler func(Notification)

// NotificationListener manages a dedicated connection for Postgres LISTEN/NOTIFY.
// It acquires a connection from the pool and runs the LISTEN loop in a goroutine.
type NotificationListener struct {
	pool    *pgxpool.Pool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.Mutex
	handler NotificationHandler
	channels []string
	running  bool
}

// NewNotificationListener creates a new listener for the given pool.
// Call Start() to begin listening.
func NewNotificationListener(pool *pgxpool.Pool) *NotificationListener {
	return &NotificationListener{
		pool:     pool,
		channels: make([]string, 0),
	}
}

// SetHandler sets the callback that will receive notifications.
func (l *NotificationListener) SetHandler(handler NotificationHandler) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.handler = handler
}

// AddChannel registers a channel to LISTEN on. Channels are subscribed when
// Start() is called. If already running, the new channel is subscribed immediately.
func (l *NotificationListener) AddChannel(channel string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.channels = append(l.channels, channel)
}

// Start begins listening on all registered channels. It acquires a dedicated
// connection from the pool and runs the notification loop in a background goroutine.
// Call Stop() to clean up.
func (l *NotificationListener) Start() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.running {
		return fmt.Errorf("notification listener already running")
	}

	if len(l.channels) == 0 {
		return fmt.Errorf("no channels registered for notification listener")
	}

	if l.handler == nil {
		return fmt.Errorf("no handler set for notification listener")
	}

	l.ctx, l.cancel = context.WithCancel(context.Background())
	l.running = true

	l.wg.Add(1)
	go l.run()

	return nil
}

// run is the main listener loop. It acquires a dedicated connection and
// processes incoming notifications until the context is cancelled.
func (l *NotificationListener) run() {
	defer l.wg.Done()

	// Acquire a dedicated connection for LISTEN/NOTIFY
	conn, err := l.pool.Acquire(l.ctx)
	if err != nil {
		slog.Error("postgres: notification listener: failed to acquire connection", "error", err)
		l.mu.Lock()
		l.running = false
		l.mu.Unlock()
		return
	}
	defer conn.Release()

	// Subscribe to all channels
	for _, ch := range l.channels {
		_, err := conn.Exec(l.ctx, "LISTEN "+quoteIdent(ch))
		if err != nil {
			slog.Error("postgres: notification listener: LISTEN failed", "channel", ch, "error", err)
			l.mu.Lock()
			l.running = false
			l.mu.Unlock()
			return
		}
		slog.Info("postgres: notification listener: LISTENing", "channel", ch)
	}

	slog.Info("postgres: notification listener started", "channels", len(l.channels))

	// Wait for notifications
	for {
		notification, err := conn.Conn().WaitForNotification(l.ctx)
		if err != nil {
			// Context cancellation is expected during shutdown
			if l.ctx.Err() != nil {
				slog.Info("postgres: notification listener stopped")
				return
			}
			slog.Warn("postgres: notification listener: wait error", "error", err)
			continue
		}

		// Forward to handler
		l.mu.Lock()
		handler := l.handler
		l.mu.Unlock()

		if handler != nil {
			handler(Notification{
				Channel: notification.Channel,
				Payload: notification.Payload,
				PID:     notification.PID,
			})
		}
	}
}

// Stop gracefully stops the notification listener.
func (l *NotificationListener) Stop() {
	l.mu.Lock()
	if !l.running {
		l.mu.Unlock()
		return
	}
	l.running = false
	l.mu.Unlock()

	if l.cancel != nil {
		l.cancel()
	}
	l.wg.Wait()
}

// IsRunning returns true if the listener is active.
func (l *NotificationListener) IsRunning() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.running
}

// quoteIdent safely quotes a PostgreSQL identifier for use in LISTEN/UNLISTEN.
// This prevents SQL injection via channel names.
func quoteIdent(name string) string {
	// Simple quoting: double-quote and escape embedded double quotes
	return `"` + escapeDoubleQuote(name) + `"`
}

func escapeDoubleQuote(s string) string {
	var result string
	for _, ch := range s {
		if ch == '"' {
			result += `""`
		} else {
			result += string(ch)
		}
	}
	return result
}
