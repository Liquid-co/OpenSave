// Package logging provides the daemon's activity log: a bounded in-memory
// ring of recent entries (served to the dashboard's Activity Log panel on
// connect) plus a subscriber hook for live broadcast.
package logging

import (
	"sync"
	"time"
)

const historyLimit = 200

// Entry is one activity-log line.
type Entry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"` // info | warn | error | success
	Message   string `json:"message"`
}

// Logger is safe for concurrent use.
type Logger struct {
	mu          sync.Mutex
	history     []Entry
	subscribers []func(Entry)
}

// New creates a Logger.
func New() *Logger {
	return &Logger{}
}

// Log records an entry and notifies subscribers.
func (l *Logger) Log(level, message string) {
	entry := Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Message:   message,
	}

	l.mu.Lock()
	l.history = append(l.history, entry)
	if len(l.history) > historyLimit {
		l.history = l.history[len(l.history)-historyLimit:]
	}
	subs := make([]func(Entry), len(l.subscribers))
	copy(subs, l.subscribers)
	l.mu.Unlock()

	for _, sub := range subs {
		sub(entry)
	}
}

// History returns a copy of the retained entries, oldest first.
func (l *Logger) History() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Entry, len(l.history))
	copy(out, l.history)
	return out
}

// Subscribe registers a live-entry callback (e.g. the dashboard WS hub).
func (l *Logger) Subscribe(fn func(Entry)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.subscribers = append(l.subscribers, fn)
}
