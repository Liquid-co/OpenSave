package presets

import "testing"

// The manifest genuinely lists games called "Data" and "Save". A folder with
// one of those names is almost always a subfolder of a game rather than the
// game, so resolving it to an App ID hands the matcher a confident wrong
// answer — and an App ID is what merges two rows into one game.
//
// Found when the embedded index was regenerated: the previous one happened not
// to contain such a title, so nothing had ever exercised it.
func TestGenericNamesNeverResolveToAnAppID(t *testing.T) {
	idx := nameToAppIDIndex()
	for _, name := range []string{
		"Data", "data", "Save", "Saves", "SaveData", "Profile", "Profiles",
		"User", "Config", "Settings", "Backup", "Remote", "Storage", "Local",
		"Game", "Games", "userdata",
	} {
		if got := inferAppIDFromName(name, idx); got != "" {
			t.Errorf("%q resolved to App ID %q — a folder called that is almost never "+
				"the game, and an App ID is what merges two rows into one", name, got)
		}
	}
}

// The exclusion must not cost real titles that merely contain a generic word.
func TestNamesContainingAGenericWordStillResolve(t *testing.T) {
	idx := nameToAppIDIndex()
	for _, name := range []string{"Hollow Knight", "Cuphead", "Subnautica"} {
		if got := inferAppIDFromName(name, idx); got == "" {
			t.Errorf("%q no longer resolves to an App ID", name)
		}
	}
}
