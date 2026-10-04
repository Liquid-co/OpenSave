package snapshot

import (
	"fmt"
	"testing"
)

// A snapshot copies the files that did not change from the previous one, and
// still restores to exactly the save it was taken of.
func TestSnapshotReusesUnchangedFiles(t *testing.T) {
	env := setup(t)
	for i := 0; i < 50; i++ {
		write(t, env.saveDir, fmt.Sprintf("map/chunk_%02d.bin", i), fmt.Sprintf("chunk %d %s", i, "................................................"))
	}
	first, err := env.mgr.Create("game1", "first", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := lastReused.Load(); got != 0 {
		t.Errorf("first snapshot reused %d entries from nothing", got)
	}

	// A session changes two chunks and adds one.
	write(t, env.saveDir, "map/chunk_03.bin", "changed three")
	write(t, env.saveDir, "map/chunk_40.bin", "changed forty")
	write(t, env.saveDir, "map/chunk_99.bin", "new")
	second, err := env.mgr.Create("game1", "second", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := lastReused.Load(); got != 48 {
		t.Errorf("second snapshot copied %d unchanged entries, want 48", got)
	}

	// Each snapshot restores its own state, independently of the other.
	write(t, env.saveDir, "map/chunk_03.bin", "ruined")
	if _, err := env.mgr.Restore("game1", second.ID); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"map/chunk_03.bin": "changed three",
		"map/chunk_40.bin": "changed forty",
		"map/chunk_99.bin": "new",
		"map/chunk_07.bin": "chunk 7 ................................................",
	} {
		if got := read(env.saveDir, name); got != want {
			t.Errorf("after restoring the second snapshot, %s = %q, want %q", name, got, want)
		}
	}
	if _, err := env.mgr.Restore("game1", first.ID); err != nil {
		t.Fatal(err)
	}
	if got := read(env.saveDir, "map/chunk_03.bin"); got != "chunk 3 ................................................" {
		t.Errorf("after restoring the first snapshot, chunk_03 = %q", got)
	}
	if got := read(env.saveDir, "map/chunk_99.bin"); got != "" {
		t.Errorf("restoring the first snapshot left the later file: %q", got)
	}

	// And the recorded contents of the second snapshot are right for the
	// copied entries too.
	files, err := env.store.SnapshotFiles(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 51 {
		t.Errorf("second snapshot records %d files, want 51", len(files))
	}
}
