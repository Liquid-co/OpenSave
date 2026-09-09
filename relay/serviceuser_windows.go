//go:build windows

package relay

// Windows has no systemd unit and no service account to hand the file to: the
// relay there runs as whoever started it, and SecretsPath already keeps the
// file in that account's own profile.
func chownTo(path, username string) (changed bool, err error) { return false, nil }

// FileOwner is unavailable on Windows, where the ownership question this
// answers does not arise.
func FileOwner(path string) string { return "" }
