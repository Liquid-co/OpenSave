package presets

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	peerProfile  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	localProfile = "8f3a1b2c4d5e6f708192a3b4c5d6e7f8"
)

func nandRoot(home, emu string) string {
	return filepath.Join(home, ".local", "share", emu, "nand", "user", "save")
}

func mkdirsAbs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// The other device's path, translated here, names its own profile. This
// device's emulator keeps the game under its own.
func TestSwitchSaveGoesUnderThisDevicesProfile(t *testing.T) {
	home := t.TempDir()
	root := nandRoot(home, "eden")
	mkdirsAbs(t, filepath.Join(root, userAccount, localProfile))
	translated := filepath.Join(root, userAccount, peerProfile, zelda)

	got := linuxScanner(t, home).SwitchSaveFolder(zelda, translated)
	if want := filepath.Join(root, userAccount, localProfile, zelda); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// When the game already has a save here, that is where it goes — in whichever
// emulator has it, not only the one the other device used, and whichever
// profile plays it.
func TestSwitchSaveGoesWhereTheGameAlreadyIs(t *testing.T) {
	home := t.TempDir()
	existing := filepath.Join(nandRoot(home, "eden"), userAccount, localProfile, zelda)
	mkdirsAbs(t, existing, filepath.Join(nandRoot(home, "eden"), userAccount, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))
	translated := filepath.Join(nandRoot(home, "citron"), userAccount, peerProfile, zelda) // no Citron here

	if got := linuxScanner(t, home).SwitchSaveFolder(zelda, translated); got != existing {
		t.Errorf("got %s, want %s", got, existing)
	}
}

// The other device's emulator is not here, and one other is.
func TestSwitchSaveGoesToTheOnlyEmulatorHere(t *testing.T) {
	home := t.TempDir()
	root := nandRoot(home, "eden")
	mkdirsAbs(t, filepath.Join(root, userAccount, localProfile), filepath.Join(root, userAccount, "00000000000000000000000000000000"))
	translated := filepath.Join(nandRoot(home, "citron"), userAccount, peerProfile, zelda)

	got := linuxScanner(t, home).SwitchSaveFolder(zelda, translated)
	if want := filepath.Join(root, userAccount, localProfile, zelda); got != want {
		t.Errorf("got %s, want %s (the all-zero folder is not a profile)", got, want)
	}
}

// Anything that would be a guess between people or emulators keeps the
// translated path, as before.
func TestSwitchSaveDoesNotGuess(t *testing.T) {
	t.Run("two profiles", func(t *testing.T) {
		home := t.TempDir()
		root := nandRoot(home, "eden")
		mkdirsAbs(t, filepath.Join(root, userAccount, localProfile), filepath.Join(root, userAccount, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))
		translated := filepath.Join(root, userAccount, peerProfile, zelda)
		if got := linuxScanner(t, home).SwitchSaveFolder(zelda, translated); got != translated {
			t.Errorf("got %s", got)
		}
	})
	t.Run("two emulators, neither the other device's", func(t *testing.T) {
		home := t.TempDir()
		mkdirsAbs(t, filepath.Join(nandRoot(home, "eden"), userAccount, localProfile), filepath.Join(nandRoot(home, "suyu"), userAccount, localProfile))
		translated := filepath.Join(nandRoot(home, "citron"), userAccount, peerProfile, zelda)
		if got := linuxScanner(t, home).SwitchSaveFolder(zelda, translated); got != translated {
			t.Errorf("got %s", got)
		}
	})
	t.Run("the game in two emulators", func(t *testing.T) {
		home := t.TempDir()
		mkdirsAbs(t, filepath.Join(nandRoot(home, "eden"), userAccount, localProfile, zelda), filepath.Join(nandRoot(home, "suyu"), userAccount, localProfile, zelda))
		translated := filepath.Join(nandRoot(home, "citron"), userAccount, peerProfile, zelda)
		if got := linuxScanner(t, home).SwitchSaveFolder(zelda, translated); got != translated {
			t.Errorf("got %s", got)
		}
	})
	t.Run("already there", func(t *testing.T) {
		home := t.TempDir()
		translated := filepath.Join(nandRoot(home, "eden"), userAccount, peerProfile, zelda)
		mkdirsAbs(t, translated, filepath.Join(nandRoot(home, "eden"), userAccount, localProfile))
		if got := linuxScanner(t, home).SwitchSaveFolder(zelda, translated); got != translated {
			t.Errorf("got %s", got)
		}
	})
}
