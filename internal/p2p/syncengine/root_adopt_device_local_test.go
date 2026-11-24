package syncengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/ignore"
)

func TestAdoptionAllowsManifestOmittedDeviceLocalDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "device-local"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "device-local", ".state"), []byte("local"), 0600); err != nil {
		t.Fatal(err)
	}
	mf, err := delta.BuildManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := adoptionApplied(mf, delta.Manifest{Files: map[string]delta.FileEntry{}}, ignore.Rules{}, root); err != nil {
		t.Fatal(err)
	}
	if !dirKeepsDeviceLocal("device-local", mf, ignore.Rules{}, root) {
		t.Fatal("manifest-omitted entry must remain device-local")
	}
}

func TestAdoptionRejectsUnselectedOrdinaryEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "device-local"), 0700); err != nil {
		t.Fatal(err)
	}
	mf, err := delta.BuildManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := adoptionApplied(mf, delta.Manifest{Files: map[string]delta.FileEntry{}}, ignore.Rules{}, root); err == nil {
		t.Fatal("unselected empty directory must fail")
	}
}

func TestAdoptionAllowsManifestOmittedEntryUnderDecomposedDirectory(t *testing.T) {
	root := t.TempDir()
	onDisk := "cafe\u0301"
	if err := os.Mkdir(filepath.Join(root, onDisk), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, onDisk, ".state"), []byte("local"), 0600); err != nil {
		t.Fatal(err)
	}
	mf, err := delta.BuildManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := adoptionApplied(mf, delta.Manifest{Files: map[string]delta.FileEntry{}}, ignore.Rules{}, root); err != nil {
		t.Fatal(err)
	}
}
