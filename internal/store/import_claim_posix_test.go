//go:build linux

package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

// A disposable POSIX lock models SQLite's database-file lock. A child process
// probes it without opening a second SQLite connection or touching live data.
func TestClaimReleasePreservesPOSIXLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.db")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lock := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 1}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lock); err != nil {
		t.Fatal(err)
	}
	probe := func() string {
		cmd := exec.Command(os.Args[0], "-test.run=^TestClaimPOSIXProbe$")
		cmd.Env = append(os.Environ(), "OPENSAVE_POSIX_PROBE="+path)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("probe: %v %s", err, out)
		}
		return string(out)
	}
	if got := probe(); got != "blocked\n" {
		t.Fatalf("initial POSIX lock not held: %q", got)
	}
	release, err := claimNativeDB(path)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if got := probe(); got != "blocked\n" {
		t.Fatalf("claim release dropped pre-existing POSIX lock: child %q", got)
	}
}

func TestClaimPOSIXProbe(t *testing.T) {
	path := os.Getenv("OPENSAVE_POSIX_PROBE")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		os.Exit(2)
	}
	lock := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 1}
	err = syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lock)
	if err == syscall.EAGAIN || err == syscall.EACCES {
		os.Stdout.WriteString("blocked\n")
		os.Exit(0)
	}
	if err != nil {
		os.Exit(2)
	}
	os.Stdout.WriteString("acquired\n")
	os.Exit(0)
}

func TestClaimRefusesHardLinkedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.db")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, path+".alias"); err != nil {
		t.Skip(err)
	}
	for _, p := range []string{path, path + ".alias"} {
		release, err := claimNativeDB(p)
		if err == nil {
			release()
			t.Fatal("hard-linked database admitted with independent sidecar")
		}
	}
}
