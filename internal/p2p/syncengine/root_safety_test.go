package syncengine

import (
	"testing"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/store"
)

// Only endangered file contents need to be recoverable. A game holding an
// unrelated log open must not indefinitely block a legitimate save update.
func TestRiskCapturedInSnapshot(t *testing.T) {
	before := delta.Manifest{Files: map[string]delta.FileEntry{
		"slot.sav":      {Hash: "before"},
		"game.log":      {Hash: "locked"},
		"caf\u00e9.ini": {Hash: "before"},
	}}
	for _, tc := range []struct {
		name      string
		got       []store.CapturedFile
		risk      []string
		wantError bool
	}{
		{"protected", []store.CapturedFile{{Root: "config", Path: "slot.sav", Hash: "before"}}, []string{"slot.sav"}, false},
		{"omitted protected data", nil, []string{"slot.sav"}, true},
		{"wrong bytes", []store.CapturedFile{{Root: "config", Path: "slot.sav", Hash: "after"}}, []string{"slot.sav"}, true},
		{"different root", []store.CapturedFile{{Root: "", Path: "slot.sav", Hash: "before"}}, []string{"slot.sav"}, true},
		{"unrelated locked file", []store.CapturedFile{{Root: "config", Path: "slot.sav", Hash: "before"}}, []string{"slot.sav"}, false},
		{"new file", nil, nil, false},
		{"nfd snapshot spelling", []store.CapturedFile{{Root: "config", Path: "cafe\u0301.ini", Hash: "before"}}, []string{"caf\u00e9.ini"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := riskCapturedInSnapshot(tc.got, "config", before, tc.risk)
			if (err != nil) != tc.wantError {
				t.Fatalf("riskCapturedInSnapshot error = %v, wantError = %v", err, tc.wantError)
			}
		})
	}
}
