package syncengine

import (
	"testing"

	"github.com/opensave/opensave/internal/delta"
)

// The decision rule for a recorded deletion, tested exhaustively because it is
// the only place where a record can cause a file to be removed on another
// device. Every case here is "what happens to the peer's copy".

func deletionTestManifest(files map[string]string) delta.Manifest {
	m := delta.Manifest{Files: map[string]delta.FileEntry{}}
	for path, hash := range files {
		m.Files[path] = delta.FileEntry{Hash: hash, Size: 1}
	}
	return m
}

func containsPath(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// The bug this exists for. The lineage no longer holds the path — a refresh
// rebuilt it from the intersection of the two manifests after the deletion, so
// the evidence is gone. Without a record, the peer's copy reads as a new file
// and is pulled back onto the device that deleted it.
func TestARecordedDeletionPropagatesWithoutTheLineage(t *testing.T) {
	local := deletionTestManifest(map[string]string{"keep.sav": "k"})
	remote := deletionTestManifest(map[string]string{"keep.sav": "k", "drop.sav": "d"})

	d := ComputeWithDeletions(local, remote,
		map[string]struct{}{"keep.sav": {}}, // lineage WITHOUT drop.sav
		nil, "",
		map[string]DeletedRecord{"drop.sav": {Hash: "d"}})

	if !containsPath(d.FilesToDeleteOnPeer, "drop.sav") {
		t.Errorf("the deletion was not propagated: delete=%v pull=%v",
			d.FilesToDeleteOnPeer, d.FilesToPull)
	}
	if containsPath(d.FilesToPull, "drop.sav") {
		t.Error("the deleted file was pulled back — this is the resurrection")
	}
}

// The safety property, and the one that must never regress. The peer edited
// the file after this device last saw it, so their copy hashes differently.
// Their bytes win; the record must not remove them.
func TestARecordedDeletionNeverRemovesAnEditedCopy(t *testing.T) {
	local := deletionTestManifest(map[string]string{})
	remote := deletionTestManifest(map[string]string{"slot.sav": "THEIR-NEWER-CONTENT"})

	d := ComputeWithDeletions(local, remote, nil, nil, "",
		map[string]DeletedRecord{"slot.sav": {Hash: "what-we-deleted"}})

	if containsPath(d.FilesToDeleteOnPeer, "slot.sav") {
		t.Error("a recorded deletion removed a copy the peer had edited — the " +
			"content check is the only thing standing between a deletion record " +
			"and destroying someone else's work")
	}
	if !containsPath(d.FilesToPull, "slot.sav") {
		t.Errorf("the peer's newer copy was not taken: pull=%v", d.FilesToPull)
	}
}

// Without a record and without lineage, the conservative default stands: the
// peer's file is taken. Changing this would delete on suspicion.
func TestNoRecordStillMeansPull(t *testing.T) {
	local := deletionTestManifest(map[string]string{})
	remote := deletionTestManifest(map[string]string{"mystery.sav": "m"})

	d := ComputeWithDeletions(local, remote, nil, nil, "", nil)
	if containsPath(d.FilesToDeleteOnPeer, "mystery.sav") {
		t.Error("a file nobody recorded deleting was deleted on the peer")
	}
	if !containsPath(d.FilesToPull, "mystery.sav") {
		t.Error("the conservative default no longer pulls an unknown file")
	}
}

// The lineage path is untouched: a file both sides are known to have shared,
// now absent locally, still propagates as a deletion with no record involved.
func TestTheLineagePathIsUnchanged(t *testing.T) {
	local := deletionTestManifest(map[string]string{})
	remote := deletionTestManifest(map[string]string{"shared.sav": "s"})

	d := ComputeWithDeletions(local, remote,
		map[string]struct{}{"shared.sav": {}}, nil, "", nil)

	if !containsPath(d.FilesToDeleteOnPeer, "shared.sav") {
		t.Error("the existing lineage-based deletion stopped working")
	}
}

// A nil map has to behave exactly as before, or every existing caller changes
// meaning silently.
func TestNilRecordsBehaveLikeTheOldFunction(t *testing.T) {
	local := deletionTestManifest(map[string]string{"a.sav": "1"})
	remote := deletionTestManifest(map[string]string{"a.sav": "1", "b.sav": "2"})
	lineage := map[string]struct{}{"a.sav": {}}

	withNil := ComputeWithDeletions(local, remote, lineage, nil, "", nil)
	old := ComputeWithBase(local, remote, lineage, nil, "")

	if len(withNil.FilesToPull) != len(old.FilesToPull) ||
		len(withNil.FilesToDeleteOnPeer) != len(old.FilesToDeleteOnPeer) {
		t.Errorf("nil records changed the outcome: new=%+v old=%+v", withNil, old)
	}
}

// A record for a file the peer does not have is simply spent — there is
// nothing to delete, and it must not invent work.
func TestARecordForAFileThePeerLacksDoesNothing(t *testing.T) {
	local := deletionTestManifest(map[string]string{})
	remote := deletionTestManifest(map[string]string{})

	d := ComputeWithDeletions(local, remote, nil, nil, "",
		map[string]DeletedRecord{"gone.sav": {Hash: "h"}})

	if len(d.FilesToDeleteOnPeer) != 0 || len(d.FilesToPull) != 0 {
		t.Errorf("a spent record produced work: %+v", d)
	}
}

// A record must not affect a file this device still has. If it is present
// locally the record is stale, and the ordinary comparison applies.
func TestARecordIsIgnoredWhenTheFileIsBackLocally(t *testing.T) {
	local := deletionTestManifest(map[string]string{"back.sav": "same"})
	remote := deletionTestManifest(map[string]string{"back.sav": "same"})

	d := ComputeWithDeletions(local, remote, nil, nil, "",
		map[string]DeletedRecord{"back.sav": {Hash: "same"}})

	if containsPath(d.FilesToDeleteOnPeer, "back.sav") {
		t.Error("a stale record deleted the peer's copy of a file this device has")
	}
}
