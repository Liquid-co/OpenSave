package daemon

import (
	"testing"
	"time"
)

// The scheduled check runs when it is on and a full check finished that many
// days ago or more — counted from the last one, so a restart does not start
// the count again.
func TestVerifyDueFollowsTheSetting(t *testing.T) {
	d := newTestDaemon(t)
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	set := func(days int, last time.Time) {
		t.Helper()
		s, err := d.Store.GetSettings()
		if err != nil {
			t.Fatal(err)
		}
		s.VerifyEveryDays = days
		if err := d.Store.UpdateSettings(s); err != nil {
			t.Fatal(err)
		}
		if err := d.Store.SetLastVerify(last.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}

	set(7, time.Time{}.Add(time.Millisecond)) // never checked
	if !verifyDue(d.Store, now) {
		t.Error("never checked: not due")
	}
	set(7, now.Add(-6*24*time.Hour))
	if verifyDue(d.Store, now) {
		t.Error("checked 6 days ago on a weekly schedule: due")
	}
	set(7, now.Add(-7*24*time.Hour))
	if !verifyDue(d.Store, now) {
		t.Error("checked a week ago on a weekly schedule: not due")
	}
	set(0, now.Add(-365*24*time.Hour))
	if verifyDue(d.Store, now) {
		t.Error("turned off: due")
	}
	// A settings save does not move when the last check was.
	set(3, now.Add(-2*24*time.Hour))
	s, _ := d.Store.GetSettings()
	s.DeviceName = "renamed"
	_ = d.Store.UpdateSettings(s)
	if after, _ := d.Store.GetSettings(); after.LastVerifyMs != now.Add(-2*24*time.Hour).UnixMilli() {
		t.Errorf("a settings save changed the last check to %d", after.LastVerifyMs)
	}
}
