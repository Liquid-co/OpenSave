package store

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Two SQLite handles stand in for independently running native processes.
// The import lease must refuse new game/peer/root registrations until the
// imported path has been registered under the same admission.
func TestImportPathAdmissionSerializesIndependentRegistrations(t *testing.T) {
	db := filepath.Join(t.TempDir(), "native.db")
	a, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	owner := filepath.Join(t.TempDir(), "owner")
	nested := filepath.Join(owner, "config")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}

	admission, err := a.BeginImportPath(nested, "incoming")
	if err != nil {
		t.Fatal(err)
	}
	defer admission.Release()
	if err := b.CreateGame(Game{ID: "competing", Name: "Competing", SavePath: owner}); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("concurrent game registration escaped admission: %v", err)
	}
	if _, err := b.CreateGameUnlessAliased(Game{ID: "autocreate", Name: "Peer", SavePath: owner}); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("concurrent auto-track escaped admission: %v", err)
	}
	if err := a.CreateGameAdmitted(Game{ID: "incoming", Name: "Incoming", SavePath: nested}, admission); err != nil {
		t.Fatal(err)
	}
	admission.Release()
	if err := b.CreateGame(Game{ID: "competing", Name: "Competing", SavePath: owner}); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("owned path became available after import registered: %v", err)
	}
}

func TestImportPathAdmissionRefusesNamedRootAndNestedPath(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "native.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	path := filepath.Join(t.TempDir(), "config")
	if err := s.CreateGame(Game{ID: "owner", Name: "Owner", SavePath: filepath.Join(t.TempDir(), "primary")}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGameRoot("owner", "config", path); err != nil {
		t.Fatal(err)
	}
	if a, err := s.BeginImportPath(filepath.Join(path, "nested"), "incoming"); err == nil {
		a.Release()
		t.Fatal("import was admitted under existing mapped named root")
	} else if !errors.Is(err, ErrGamePathOwned) {
		t.Fatal(err)
	}
}

func TestImportAdmissionRefusesConcurrentRootRelocationAndWrongToken(t *testing.T) {
	db := filepath.Join(t.TempDir(), "native.db")
	a, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	original := filepath.Join(t.TempDir(), "ordinary")
	target := filepath.Join(t.TempDir(), "target")
	if err := b.CreateGame(Game{ID: "ordinary", Name: "Ordinary", SavePath: original}); err != nil {
		t.Fatal(err)
	}
	admission, err := a.BeginImportPath(target, "import-game")
	if err != nil {
		t.Fatal(err)
	}
	defer admission.Release()

	if err := b.AddGameRoot("ordinary", "new", target); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("concurrent AddRoot was not excluded: %v", err)
	}
	game, err := b.GetGame("ordinary")
	if err != nil {
		t.Fatal(err)
	}
	game.SavePath = target
	if err := b.UpdateGame(game); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("concurrent relocation was not excluded: %v", err)
	}
	if err := a.CreateGameAdmitted(Game{ID: "wrong", Name: "Wrong", SavePath: target}, admission); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("wrong ID accepted under import admission: %v", err)
	}
	if err := a.CreateGameAdmitted(Game{ID: "import-game", Name: "Incoming", SavePath: original}, admission); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("wrong path accepted under import admission: %v", err)
	}
}

// Alternate path spelling of the same DB file must contend for the same lock.
func TestImportPathAdmissionSameInodeDifferentPath(t *testing.T) {
	dir := t.TempDir()
	db1 := filepath.Join(dir, "native.db")
	db2 := filepath.Join(dir, "native-hardlink.db")
	a, err := Open(db1)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	// Create a hard link so the two paths share the same inode.
	if err := os.Link(db1, db2); err != nil {
		t.Skip("hard links not supported on this filesystem")
	}
	b, err := Open(db2)
	if err != nil {
		// SQLite WAL names its journal from the path, so two hardlink spellings
		// cannot be opened in one process. The child-process test covers the lock.
		if strings.Contains(err.Error(), "disk I/O error") {
			t.Skip(err.Error())
		}
		t.Fatal(err)
	}
	defer b.Close()

	target := filepath.Join(t.TempDir(), "claimed")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	admission, err := a.BeginImportPath(target, "incoming")
	if err != nil {
		t.Fatal(err)
	}
	defer admission.Release()
	if err := b.CreateGame(Game{ID: "other", Name: "Other", SavePath: target}); !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("hard-linked DB path escaped admission: %v", err)
	}
}

// A truly separate native process, not merely a second handle in the same cwd.
func TestImportPathClaimChildProcess(t *testing.T) {
	if os.Getenv("OPENSAVE_IMPORT_CLAIM_CHILD") != "1" {
		return
	}
	path := os.Getenv("OPENSAVE_IMPORT_CLAIM_DB")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	err = s.CreateGame(Game{ID: "competing", Name: "Competing", SavePath: filepath.Join(t.TempDir(), "save")})
	if !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("cross-process exclusion failed: %v", err)
	}
}

func TestImportPathClaimCrossProcessCWDAndHardlink(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "native.db")
	aliasDir := filepath.Join(dir, "alias")
	childDir := filepath.Join(dir, "different-working-directory")
	if err := os.MkdirAll(aliasDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(childDir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	alias := filepath.Join(aliasDir, "native-hardlink.db")
	if err := os.Link(db, alias); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	admission, err := s.BeginImportPath(filepath.Join(dir, "target"), "incoming")
	if err != nil {
		t.Fatal(err)
	}
	defer admission.Release()
	for _, dbPath := range []string{db, alias} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestImportPathClaimChildProcess$", "-test.v")
		cmd.Dir = childDir
		cmd.Env = append(os.Environ(), "OPENSAVE_IMPORT_CLAIM_CHILD=1", "OPENSAVE_IMPORT_CLAIM_DB="+dbPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("different-CWD child for %s failed: %v\n%s", dbPath, err, out)
		}
	}
}

func TestImportPathClaimChildDifferentHome(t *testing.T) {
	if os.Getenv("OPENSAVE_IMPORT_CLAIM_HOME_CHILD") != "1" {
		return
	}
	s, err := Open(os.Getenv("OPENSAVE_IMPORT_CLAIM_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	err = s.CreateGame(Game{ID: "competing", Name: "Competing", SavePath: filepath.Join(t.TempDir(), "save")})
	if !errors.Is(err, ErrGamePathOwned) {
		t.Fatalf("cross-home exclusion failed: %v", err)
	}
}

func TestImportPathClaimSameLockAcrossHome(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "native.db")
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	admission, err := s.BeginImportPath(target, "incoming")
	if err != nil {
		t.Fatal(err)
	}
	defer admission.Release()
	otherHome := filepath.Join(dir, "other-home")
	if err := os.MkdirAll(otherHome, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestImportPathClaimChildDifferentHome$", "-test.v")
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "HOME=") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env, "HOME="+otherHome, "OPENSAVE_IMPORT_CLAIM_HOME_CHILD=1", "OPENSAVE_IMPORT_CLAIM_DB="+db)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("different HOME escaped the database lock: %v\n%s", err, out)
	}
}
