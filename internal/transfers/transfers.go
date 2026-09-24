// Package transfers keeps what is moving between this device and others right
// now, and what moved recently: which game, which device, which way, how much
// and how fast.
//
// The sync engine already reports each step — start, progress, done, failed —
// and those reports went to the screen and nowhere else, so once a sync ended
// nothing could say it had happened, and a failed one left only a line in the
// activity feed. This listens to the same reports and remembers them.
//
// In memory, and bounded: it answers "what is syncing, and did the last few
// go through", not "everything that ever moved".
package transfers

import (
	"sort"
	"sync"
	"time"
)

// Transfer is one game moving between this device and one other, one way.
type Transfer struct {
	GameID string `json:"gameId"`
	// Peer is the other device's name.
	Peer string `json:"peer"`
	// Direction is "download" (to this device) or "upload" (from it).
	Direction string `json:"direction"`
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt,omitempty"`
	// State is "running", "done" or "error".
	State            string  `json:"state"`
	BytesTransferred int64   `json:"bytesTransferred"`
	TotalBytes       int64   `json:"totalBytes"`
	Percentage       int     `json:"percentage"`
	SpeedBytesPerSec float64 `json:"speedBytesPerSec"`
	Error            string  `json:"error,omitempty"`
}

// Event is one progress report, as the sync engine gives it.
type Event struct {
	Peer             string
	Direction        string
	BytesTransferred int64
	TotalBytes       int64
	SpeedBytesPerSec float64
	Percentage       int
	Error            string
}

// Snapshot is the whole picture: what is running, newest first, and what
// finished recently, newest first.
type Snapshot struct {
	Active []Transfer `json:"active"`
	Recent []Transfer `json:"recent"`
}

// KeepRecent is how many finished transfers are remembered.
const KeepRecent = 50

// staleAfter is how long a transfer may go without a report before it is
// taken to have ended without saying so. Reports travel fire-and-forget from
// the other device for uploads, and one that never arrives must not leave a
// transfer "running" for the rest of the session.
const staleAfter = 5 * time.Minute

// Log records transfers. Safe for concurrent use; the zero value is not
// usable — use New.
type Log struct {
	mu     sync.Mutex
	active map[string]*Transfer
	seen   map[string]time.Time // last report for each active transfer
	recent []Transfer
	now    func() time.Time
}

func New() *Log {
	return &Log{active: map[string]*Transfer{}, seen: map[string]time.Time{}, now: time.Now}
}

func key(gameID string, ev Event) string {
	return gameID + "\x00" + ev.Peer + "\x00" + direction(ev)
}

// direction defaults to download: that is what the engine's own reports are,
// and a peer's report without one is older than the field.
func direction(ev Event) string {
	if ev.Direction == "" {
		return "download"
	}
	return ev.Direction
}

func (l *Log) stamp() string { return l.now().UTC().Format(time.RFC3339) }

// Started records a transfer beginning. Starting one already running starts
// it over: a retry is a new attempt.
func (l *Log) Started(gameID string, ev Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := key(gameID, ev)
	l.active[k] = &Transfer{
		GameID: gameID, Peer: ev.Peer, Direction: direction(ev),
		StartedAt: l.stamp(), State: "running",
		TotalBytes: ev.TotalBytes,
	}
	l.seen[k] = l.now()
}

// Progressed updates a running transfer, starting one if its start was
// missed — reports can arrive out of order, or the start can be lost.
func (l *Log) Progressed(gameID string, ev Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := key(gameID, ev)
	// A report with no direction (an older device's) belongs to whichever
	// transfer of this game with this device is running, rather than starting
	// a second one beside it.
	if ev.Direction == "" && l.active[k] == nil {
		for other, t := range l.active {
			if t.GameID == gameID && t.Peer == ev.Peer {
				k = other
				break
			}
		}
	}
	t := l.active[k]
	if t == nil {
		t = &Transfer{GameID: gameID, Peer: ev.Peer, Direction: direction(ev), StartedAt: l.stamp(), State: "running"}
		l.active[k] = t
	}
	t.BytesTransferred = ev.BytesTransferred
	if ev.TotalBytes > 0 {
		t.TotalBytes = ev.TotalBytes
	}
	t.Percentage = ev.Percentage
	t.SpeedBytesPerSec = ev.SpeedBytesPerSec
	l.seen[k] = l.now()
}

// Finished ends a transfer, well or not: an empty error is success.
func (l *Log) Finished(gameID string, ev Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := key(gameID, ev)
	t := l.active[k]
	if t == nil {
		// Finished without a start seen: still worth remembering that it ran.
		t = &Transfer{GameID: gameID, Peer: ev.Peer, Direction: direction(ev), StartedAt: l.stamp()}
	}
	delete(l.active, k)
	delete(l.seen, k)
	t.EndedAt = l.stamp()
	if ev.Error != "" {
		t.State = "error"
		t.Error = ev.Error
	} else {
		t.State = "done"
		t.Percentage = 100
		if t.TotalBytes > 0 {
			t.BytesTransferred = t.TotalBytes
		}
	}
	t.SpeedBytesPerSec = 0
	l.remember(*t)
}

func (l *Log) remember(t Transfer) {
	l.recent = append([]Transfer{t}, l.recent...)
	if len(l.recent) > KeepRecent {
		l.recent = l.recent[:KeepRecent]
	}
}

// Now reports running and recent transfers. A running one that has gone
// quiet for too long is moved to recent as having ended unannounced.
func (l *Log) Now() Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for k, t := range l.active {
		if now.Sub(l.seen[k]) > staleAfter {
			t.State = "error"
			t.Error = "stopped reporting progress"
			t.EndedAt = now.UTC().Format(time.RFC3339)
			t.SpeedBytesPerSec = 0
			l.remember(*t)
			delete(l.active, k)
			delete(l.seen, k)
		}
	}
	active := make([]Transfer, 0, len(l.active))
	for _, t := range l.active {
		active = append(active, *t)
	}
	sort.Slice(active, func(i, j int) bool { return active[i].StartedAt > active[j].StartedAt })
	return Snapshot{Active: active, Recent: append([]Transfer{}, l.recent...)}
}
