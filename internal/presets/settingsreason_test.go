package presets

import "testing"

// A reason is stored as its code and read back the same, and a sentence a
// test build stored before there were codes is read as its code too.
func TestSettingsReasonRoundTrips(t *testing.T) {
	for _, r := range []SettingsReason{
		{Code: ReasonUnrealConfig},
		{Code: ReasonContents},
		{Code: ReasonContentsAndName},
		{Code: ReasonSnapshots, N: 3, Of: 5},
		{Code: ReasonChanges, N: 4, Of: 6},
	} {
		if got := ParseSettingsReason(r.Encode()); got != r {
			t.Errorf("%q read back as %+v, want %+v", r.Encode(), got, r)
		}
		if got := ParseSettingsReason(r.Text()); got != r {
			t.Errorf("the sentence %q read back as %+v, want %+v", r.Text(), got, r)
		}
	}
	if got := (SettingsReason{Code: ReasonSnapshots, N: 3, Of: 5}).Text(); got != "changed in 3 of the 5 snapshots in which the save changed" {
		t.Errorf("Text = %q", got)
	}

	for _, s := range []string{"", "something else entirely", "contents 3 5", "snapshots 3"} {
		got := ParseSettingsReason(s)
		if got.Known() || got.Text() != s || got.Encode() != s {
			t.Errorf("%q read as %+v (%q), want it kept as it is", s, got, got.Text())
		}
	}
}
