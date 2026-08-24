//go:build windows

package winreg

import (
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// scratchKey creates a key of our own under HKCU and removes it afterwards, so
// no test ever writes to a key a real game owns.
func scratchKey(t *testing.T) string {
	t.Helper()
	sub := `Software\OpenSaveTest\` + strconv.FormatInt(time.Now().UnixNano(), 36)
	k, _, err := registry.CreateKey(registry.CURRENT_USER, sub, registry.WRITE)
	if err != nil {
		t.Fatalf("creating scratch key: %v", err)
	}
	k.Close()
	t.Cleanup(func() {
		// Depth-first: a key with subkeys cannot be deleted.
		var del func(string)
		del = func(p string) {
			h, err := registry.OpenKey(registry.CURRENT_USER, p, registry.READ)
			if err == nil {
				names, _ := h.ReadSubKeyNames(-1)
				h.Close()
				for _, n := range names {
					del(p + `\` + n)
				}
			}
			_ = registry.DeleteKey(registry.CURRENT_USER, p)
		}
		del(sub)
	})
	return `HKEY_CURRENT_USER\` + sub
}

// Every value type a game might use has to survive capture and restore
// unchanged. A save written back with the wrong type is a save the game cannot
// read.
func TestEveryValueTypeRoundTrips(t *testing.T) {
	path := scratchKey(t)
	_, sub, _ := splitHive(path)
	h, _, err := registry.CreateKey(registry.CURRENT_USER, sub, registry.WRITE)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.SetStringValue("name", "Player One"); err != nil {
		t.Fatal(err)
	}
	if err := h.SetExpandStringValue("path", `%APPDATA%\game`); err != nil {
		t.Fatal(err)
	}
	if err := h.SetDWordValue("level", 42); err != nil {
		t.Fatal(err)
	}
	if err := h.SetQWordValue("score", 9876543210); err != nil {
		t.Fatal(err)
	}
	if err := h.SetBinaryValue("blob", []byte{0, 1, 2, 255, 254}); err != nil {
		t.Fatal(err)
	}
	if err := h.SetStringsValue("unlocks", []string{"a", "b", "c"}); err != nil {
		t.Fatal(err)
	}
	h.Close()

	captured, found, err := Capture(path)
	if err != nil || !found {
		t.Fatalf("Capture = (found %v, err %v)", found, err)
	}
	if len(captured.Values) != 6 {
		t.Fatalf("captured %d values, want 6: %+v", len(captured.Values), captured.Values)
	}

	// Wipe, then restore from the capture alone.
	hw, err := registry.OpenKey(registry.CURRENT_USER, sub, registry.WRITE)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"name", "path", "level", "score", "blob", "unlocks"} {
		if err := hw.DeleteValue(n); err != nil {
			t.Fatal(err)
		}
	}
	hw.Close()

	if err := Restore(captured); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	hr, err := registry.OpenKey(registry.CURRENT_USER, sub, registry.READ)
	if err != nil {
		t.Fatal(err)
	}
	defer hr.Close()
	if s, _, _ := hr.GetStringValue("name"); s != "Player One" {
		t.Errorf("name = %q", s)
	}
	if s, _, _ := hr.GetStringValue("path"); s != `%APPDATA%\game` {
		t.Errorf("path = %q — an EXPAND_SZ must come back unexpanded", s)
	}
	if n, _, _ := hr.GetIntegerValue("level"); n != 42 {
		t.Errorf("level = %d", n)
	}
	if n, _, _ := hr.GetIntegerValue("score"); n != 9876543210 {
		t.Errorf("score = %d — a QWORD must not be truncated to 32 bits", n)
	}
	if b, _, _ := hr.GetBinaryValue("blob"); len(b) != 5 || b[3] != 255 {
		t.Errorf("blob = %v", b)
	}
	if ss, _, _ := hr.GetStringsValue("unlocks"); len(ss) != 3 || ss[2] != "c" {
		t.Errorf("unlocks = %v", ss)
	}
}

// Saves nest: a game with a slot per character writes a subkey each.
func TestSubkeysAreCapturedAndRestored(t *testing.T) {
	path := scratchKey(t)
	_, sub, _ := splitHive(path)
	h, _, _ := registry.CreateKey(registry.CURRENT_USER, sub+`\Slot1`, registry.WRITE)
	_ = h.SetDWordValue("hp", 100)
	h.Close()

	captured, found, err := Capture(path)
	if err != nil || !found {
		t.Fatalf("Capture = (%v, %v)", found, err)
	}
	if len(captured.Subkeys) != 1 || len(captured.Subkeys[0].Values) != 1 {
		t.Fatalf("subkey not captured: %+v", captured)
	}

	var del = registry.DeleteKey(registry.CURRENT_USER, sub+`\Slot1`)
	if del != nil {
		t.Fatal(del)
	}
	if err := Restore(captured); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	hr, err := registry.OpenKey(registry.CURRENT_USER, sub+`\Slot1`, registry.READ)
	if err != nil {
		t.Fatalf("the subkey was not restored: %v", err)
	}
	defer hr.Close()
	if n, _, _ := hr.GetIntegerValue("hp"); n != 100 {
		t.Errorf("hp = %d", n)
	}
}

// A game that has never run has never written its key. That is a correct
// capture of "nothing here yet", not a failure.
func TestAMissingKeyIsNotAnError(t *testing.T) {
	_, found, err := Capture(`HKEY_CURRENT_USER\Software\OpenSaveTest\definitely-not-here`)
	if err != nil {
		t.Errorf("Capture of a missing key errored: %v", err)
	}
	if found {
		t.Error("Capture reported finding a key that does not exist")
	}
}
