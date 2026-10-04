package delta

import "testing"

func TestNeverSynced(t *testing.T) {
	for p, want := range map[string]bool{
		"remotecache.vdf":        true,
		"remote/RemoteCache.VDF": true,
		"remote/save.sav":        false,
		"remotecache.vdf.bak":    false,
		"myremotecache.vdf":      false,
	} {
		if got := NeverSynced(p); got != want {
			t.Errorf("NeverSynced(%q) = %v, want %v", p, got, want)
		}
	}
}
