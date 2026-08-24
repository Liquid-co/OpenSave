package winreg

import "testing"

// The manifest writes "HKEY_CURRENT_USER/Software/..."; Windows tooling writes
// "HKCU\Software\...". Both name one key, and a capture has to match back to
// the manifest entry that asked for it.
func TestNormalizePathAcceptsBothSpellings(t *testing.T) {
	want := `HKEY_CURRENT_USER\Software\Landfall Games\Rounds`
	for _, in := range []string{
		"HKEY_CURRENT_USER/Software/Landfall Games/Rounds",
		`HKCU\Software\Landfall Games\Rounds`,
		`HKCU/Software/Landfall Games/Rounds/`,
		"  HKEY_CURRENT_USER/Software/Landfall Games/Rounds  ",
	} {
		if got := NormalizePath(in); got != want {
			t.Errorf("NormalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// A path naming no hive we know is left alone rather than mangled: returning
// something that looks canonical but is not would send a restore at the wrong
// key.
func TestNormalizePathLeavesAnUnknownHiveAlone(t *testing.T) {
	in := `NOT_A_HIVE\Software\Thing`
	if got := NormalizePath(in); got != in {
		t.Errorf("NormalizePath(%q) = %q, want it unchanged", in, got)
	}
}

func TestSplitHiveRejectsWhatItCannotOpen(t *testing.T) {
	if _, _, err := splitHive(`NOT_A_HIVE\Software`); err == nil {
		t.Error("splitHive accepted an unrecognised hive")
	}
	hive, sub, err := splitHive("HKEY_CURRENT_USER/Software/Thing")
	if err != nil || hive != "HKEY_CURRENT_USER" || sub != `Software\Thing` {
		t.Errorf("splitHive = (%q, %q, %v)", hive, sub, err)
	}
}
