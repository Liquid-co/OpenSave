package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/store"
)

func TestStorageReportAddsUpAndOrders(t *testing.T) {
	d := newTestDaemon(t)
	mk := func(id, name string, files map[string]int, limit int) string {
		dir := t.TempDir()
		for f, size := range files {
			// Random-ish bytes so the archive keeps roughly their size.
			b := make([]byte, size)
			for i := range b {
				b[i] = byte((i*7919 + size) % 251)
			}
			if err := os.WriteFile(filepath.Join(dir, f), b, 0o666); err != nil {
				t.Fatal(err)
			}
		}
		if err := d.Store.CreateGame(store.Game{ID: id, Name: name, SavePath: dir, ActiveBranch: "main", MaxManualSnapshots: limit}); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	mk("small", "Small Game", map[string]int{"a.sav": 100}, 0)
	mk("big", "Big Game", map[string]int{"a.sav": 200000}, 1)
	for i := 0; i < 3; i++ {
		if _, err := d.Snapshots.Create("big", "", false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Snapshots.Create("small", "", false); err != nil {
		t.Fatal(err)
	}
	// A limit of 1 manual snapshot for Big Game: the store's pruning keeps it
	// at one as they are taken, so lower it after the fact to leave work for
	// clean-up.
	game, _ := d.Store.GetGame("big")
	game.MaxManualSnapshots = 0
	d.Store.UpdateGame(game)
	for i := 0; i < 2; i++ {
		d.Snapshots.Create("big", "", false)
	}
	game.MaxManualSnapshots = 1
	d.Store.UpdateGame(game)

	r, err := d.Storage()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Games) != 2 || r.Games[0].GameID != "big" {
		t.Fatalf("games = %+v, want Big Game first", r.Games)
	}
	var sum int64
	count := 0
	for _, g := range r.Games {
		sum += g.Bytes
		count += g.Snapshots
	}
	if sum != r.TotalBytes || count != r.Snapshots || r.TotalBytes <= 0 {
		t.Errorf("totals %d bytes / %d snapshots do not add up to the games' %d / %d", r.TotalBytes, r.Snapshots, sum, count)
	}
	if len(r.Biggest) == 0 || r.Biggest[0].GameID != "big" {
		t.Errorf("biggest = %+v, want Big Game's first", r.Biggest)
	}
	for i := 1; i < len(r.Biggest); i++ {
		if r.Biggest[i].Bytes > r.Biggest[i-1].Bytes {
			t.Errorf("biggest is not in order at %d", i)
		}
	}
	// Big Game is over its limit of one, so clean-up would free all but one.
	if r.ReclaimableSnapshots != r.Games[0].Snapshots-1 || r.Reclaimable <= 0 || r.Games[0].Reclaimable != r.Reclaimable {
		t.Errorf("reclaimable = %d bytes in %d snapshots (Big Game %d of %d), want all but one of Big Game's",
			r.Reclaimable, r.ReclaimableSnapshots, r.Games[0].Reclaimable, r.Games[0].Snapshots)
	}
	if !r.FreeKnown || r.FreeBytes == 0 {
		t.Errorf("free space unknown on the test machine: %+v", r)
	}
}
