// Package syncpause holds whether this device has paused syncing, and until
// when.
//
// Paused means no save moves between this device and any other, in either
// direction, and nothing goes to or comes from the cloud backup. Snapshots are
// still taken — a pause is about not moving saves around, not about stopping
// the safety net. When it ends, by hand or when the time is up, everything
// catches up.
//
// Held in memory: a pause lasts until it is lifted, until its time is up, or
// until the app restarts, whichever comes first. A pause that outlived a
// restart would be one someone forgot about, with their saves quietly not
// syncing for days.
package syncpause

import (
	"errors"
	"sync"
	"time"
)

// ErrPaused is what an attempt to sync gets while this device is paused.
var ErrPaused = errors.New("syncing is paused on this device")

// Status is the pause as the app and the CLI show it.
type Status struct {
	Paused bool `json:"paused"`
	// UntilRestart: paused with no end time, until resumed or the app restarts.
	UntilRestart bool `json:"untilRestart,omitempty"`
	// Until is when a timed pause ends, RFC 3339.
	Until string `json:"until,omitempty"`
	// RemainingSeconds is how long a timed pause has left.
	RemainingSeconds int64 `json:"remainingSeconds,omitempty"`
}

// State is one device's pause. The zero value is not usable; use New.
type State struct {
	mu         sync.Mutex
	paused     bool
	until      time.Time // zero for a pause until restart
	timer      *time.Timer
	now        func() time.Time
	afterFunc  func(time.Duration, func()) *time.Timer
	onResume   []func()
	onChange   []func(Status)
	generation int // bumped on every change, so a stale timer does nothing
}

// New returns a device's pause state, not paused.
func New() *State {
	return &State{now: time.Now, afterFunc: time.AfterFunc}
}

// OnResume registers work to do when a pause ends, by hand or by its timer:
// the catching up. Called outside the lock, in the order registered.
func (s *State) OnResume(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onResume = append(s.onResume, fn)
}

// OnChange registers a listener told of every pause and resume.
func (s *State) OnChange(fn func(Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = append(s.onChange, fn)
}

// Pause pauses syncing for d, or until restart when d is zero or less. A pause
// while already paused replaces the old one — "for an hour" from now.
func (s *State) Pause(d time.Duration) Status {
	s.mu.Lock()
	s.stopTimerLocked()
	s.paused = true
	s.generation++
	if d > 0 {
		s.until = s.now().Add(d)
		gen := s.generation
		s.timer = s.afterFunc(d, func() { s.expire(gen) })
	} else {
		s.until = time.Time{}
	}
	st := s.statusLocked()
	listeners := append([]func(Status){}, s.onChange...)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn(st)
	}
	return st
}

// Resume ends a pause. It reports whether there was one to end; resuming when
// not paused does nothing and runs no catch-up.
func (s *State) Resume() bool {
	s.mu.Lock()
	if !s.paused {
		s.mu.Unlock()
		return false
	}
	s.endLocked()
	s.mu.Unlock()
	s.fireResumed()
	return true
}

// expire is the timer ending a timed pause — unless the pause it was set for
// has since been replaced or lifted.
func (s *State) expire(gen int) {
	s.mu.Lock()
	if !s.paused || s.generation != gen {
		s.mu.Unlock()
		return
	}
	s.endLocked()
	s.mu.Unlock()
	s.fireResumed()
}

func (s *State) endLocked() {
	s.stopTimerLocked()
	s.paused = false
	s.until = time.Time{}
	s.generation++
}

func (s *State) stopTimerLocked() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

func (s *State) fireResumed() {
	s.mu.Lock()
	resume := append([]func(){}, s.onResume...)
	change := append([]func(Status){}, s.onChange...)
	st := s.statusLocked()
	s.mu.Unlock()
	for _, fn := range change {
		fn(st)
	}
	for _, fn := range resume {
		fn()
	}
}

// Paused reports whether syncing is paused right now.
func (s *State) Paused() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

// Status reports the pause as it stands.
func (s *State) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *State) statusLocked() Status {
	if !s.paused {
		return Status{}
	}
	if s.until.IsZero() {
		return Status{Paused: true, UntilRestart: true}
	}
	remaining := s.until.Sub(s.now())
	if remaining < 0 {
		remaining = 0
	}
	return Status{
		Paused:           true,
		Until:            s.until.UTC().Format(time.RFC3339),
		RemainingSeconds: int64(remaining.Round(time.Second) / time.Second),
	}
}
