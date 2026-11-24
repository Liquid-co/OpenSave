package syncengine

import (
	"testing"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/ignore"
)

func TestAdoptionAllowsDirectoryThatOnlyHoldsAnExcludedFile(t *testing.T) {
	rules := ignore.Parse("device.ini")
	remote := delta.Manifest{Files: map[string]delta.FileEntry{
		"settings.ini": {Hash: "remote"},
	}}
	got := delta.Manifest{
		Files: map[string]delta.FileEntry{
			"settings.ini":            {Hash: "remote"},
			"device-local/device.ini": {Hash: "secret"},
		},
		Dirs: []string{"device-local"},
	}
	if err := adoptionApplied(got, remote, rules, ""); err != nil {
		t.Fatal(err)
	}
}

func TestAdoptionRejectsARealLocalOnlyDirectory(t *testing.T) {
	rules := ignore.Parse("device.ini")
	remote := delta.Manifest{Files: map[string]delta.FileEntry{
		"settings.ini": {Hash: "remote"},
	}}
	got := delta.Manifest{
		Files: map[string]delta.FileEntry{
			"settings.ini": {Hash: "remote"},
		},
		Dirs: []string{"notes"},
	}
	if err := adoptionApplied(got, remote, rules, ""); err == nil {
		t.Fatal("a local-only directory with nothing excluded in it was accepted")
	}
}

func TestAdoptionRejectsAMissedRemoteFile(t *testing.T) {
	remote := delta.Manifest{Files: map[string]delta.FileEntry{
		"settings.ini": {Hash: "remote"},
	}}
	got := delta.Manifest{Files: map[string]delta.FileEntry{
		"settings.ini": {Hash: "local"},
	}}
	if err := adoptionApplied(got, remote, ignore.Rules{}, ""); err == nil {
		t.Fatal("differing file contents were accepted")
	}
}
