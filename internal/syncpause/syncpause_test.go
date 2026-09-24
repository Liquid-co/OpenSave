package syncpause

import (
	"sync"
	"testing"
	"time"
)

// fake gives a State a clock and timers the test moves by hand.
type fake struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	at    time.Time
	fn    func()
	fired bool
}

func withFakeClock() (*State, *fake) {
	f := &fake{now: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	s := New()
	s.now = func() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.now }
	s.afterFunc = func(d time.Duration, fn func()) *time.Timer {
		f.mu.Lock()
		f.timers = append(f.timers, &fakeTimer{at: f.now.Add(d), fn: fn})
		f.mu.Unlock()
		// A real timer that never fires, so Stop has something to stop.
		return time.NewTimer(time.Hour * 24 * 365)
	}
	return s, f
}

// advance moves the clock and fires every timer now due, as time would.
func (f *fake) advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	var due []*fakeTimer
	for _, t := range f.timers {
		if !t.fired && !t.at.After(f.now) {
			t.fired = true
			due = append(due, t)
		}
	}
	f.mu.Unlock()
	for _, t := range due {
		t.fn()
	}
}

func TestATimedPauseEndsOnItsOwnAndCatchesUp(t *testing.T) {
	s, clock := withFakeClock()
	resumed := 0
	s.OnResume(func() { resumed++ })

	st := s.Pause(time.Hour)
	if !st.Paused || st.UntilRestart || st.RemainingSeconds != 3600 || st.Until != "2026-09-25T13:00:00Z" {
		t.Errorf("status = %+v", st)
	}
	clock.advance(59 * time.Minute)
	if !s.Paused() || s.Status().RemainingSeconds != 60 {
		t.Errorf("after 59 minutes: paused=%v %+v", s.Paused(), s.Status())
	}
	clock.advance(time.Minute)
	if s.Paused() {
		t.Error("still paused when the hour was up")
	}
	if resumed != 1 {
		t.Errorf("catch-up ran %d times, want once", resumed)
	}
}

func TestAPauseUntilRestartHasNoEnd(t *testing.T) {
	s, clock := withFakeClock()
	st := s.Pause(0)
	if !st.Paused || !st.UntilRestart || st.Until != "" {
		t.Errorf("status = %+v", st)
	}
	clock.advance(1000 * time.Hour)
	if !s.Paused() {
		t.Error("a pause until restart ended by itself")
	}
}

// Resuming by hand ends the pause at once, catches up once, and the old
// timer firing later does nothing — least of all a second catch-up, or ending
// a new pause started since.
func TestResumingByHandDisarmsTheTimer(t *testing.T) {
	s, clock := withFakeClock()
	resumed := 0
	s.OnResume(func() { resumed++ })

	s.Pause(time.Hour)
	if !s.Resume() {
		t.Fatal("Resume reported there was no pause")
	}
	if s.Paused() || resumed != 1 {
		t.Fatalf("after Resume: paused=%v, catch-ups=%d", s.Paused(), resumed)
	}
	s.Pause(0) // a new pause, until restart
	clock.advance(2 * time.Hour)
	if !s.Paused() {
		t.Error("the first pause's timer ended the second pause")
	}
	if resumed != 1 {
		t.Errorf("catch-ups = %d, want 1", resumed)
	}
}

// Pausing again replaces the pause: the new length counts from now, and only
// the new timer can end it.
func TestPausingAgainReplacesThePause(t *testing.T) {
	s, clock := withFakeClock()
	s.Pause(10 * time.Minute)
	clock.advance(5 * time.Minute)
	s.Pause(time.Hour)
	clock.advance(10 * time.Minute) // past the first pause's end
	if !s.Paused() {
		t.Fatal("the replaced pause's timer ended the new one")
	}
	if got := s.Status().RemainingSeconds; got != 50*60 {
		t.Errorf("remaining = %ds, want 3000", got)
	}
}

func TestResumeWhenNotPausedDoesNothing(t *testing.T) {
	s, _ := withFakeClock()
	resumed := 0
	s.OnResume(func() { resumed++ })
	if s.Resume() {
		t.Error("Resume reported ending a pause that did not exist")
	}
	if resumed != 0 {
		t.Error("catch-up ran without a pause")
	}
	if s.Status() != (Status{}) {
		t.Errorf("status = %+v, want the zero status", s.Status())
	}
}

func TestListenersHearEveryChange(t *testing.T) {
	s, clock := withFakeClock()
	var heard []Status
	s.OnChange(func(st Status) { heard = append(heard, st) })
	s.Pause(time.Minute)
	clock.advance(time.Minute)
	s.Pause(0)
	s.Resume()
	if len(heard) != 4 || !heard[0].Paused || heard[1].Paused || !heard[2].UntilRestart || heard[3].Paused {
		t.Errorf("heard %+v", heard)
	}
}

func TestANilStateIsNeverPaused(t *testing.T) {
	var s *State
	if s.Paused() {
		t.Error("a nil State reported paused")
	}
}

// The real timer, briefly, so the wiring to time.AfterFunc is exercised too.
func TestARealTimerEndsThePause(t *testing.T) {
	s := New()
	done := make(chan struct{})
	s.OnResume(func() { close(done) })
	s.Pause(20 * time.Millisecond)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the pause never ended")
	}
	if s.Paused() {
		t.Error("paused after the timer ended it")
	}
}
