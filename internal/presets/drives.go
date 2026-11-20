package presets

// FixedDriveRoots is the root of every fixed drive on Windows, and nothing
// elsewhere. See fixedDriveRoots.
func FixedDriveRoots() []string { return fixedDriveRoots() }
