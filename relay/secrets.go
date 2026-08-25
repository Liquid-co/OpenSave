package relay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Where a relay keeps the secrets it was configured with.
//
// Environment variables were the only way to supply these, which pushes an
// operator into one of the two things that leak a secret: typing it on a
// command line, where it lands in shell history, or pasting it into a unit
// file that ends up in a repository. A file the relay owns, written once by
// `opensave-relay setup`, avoids both.
//
// The environment still wins where it is set. A container or a systemd unit
// that already injects secrets must keep working untouched, and an operator
// who has automated this should not have their automation quietly overridden
// by a file someone left behind.

// SecretsFile is the JSON the setup command writes and the relay reads.
//
// Only secrets live here. Port and room limits stay in the environment: they
// are not sensitive, they change per deployment, and mixing them in invites
// editing this file by hand — which is how the permissions get reset.
type SecretsFile struct {
	GoogleClientSecret string `json:"googleClientSecret,omitempty"`
	SteamGridDBKey     string `json:"steamGridDBKey,omitempty"`
}

// SecretsPath is where the file lives, overridable for tests and for an
// operator who keeps configuration somewhere specific.
func SecretsPath() string {
	if p := strings.TrimSpace(os.Getenv("OPENSAVE_RELAY_SECRETS")); p != "" {
		return p
	}
	if runtime.GOOS != "windows" {
		// A relay on Linux usually runs as a service account, and /etc is
		// where its operator will look. Falls back to the home directory when
		// /etc is not writable, which is the unprivileged case.
		if etc := "/etc/opensave/relay-secrets.json"; canWriteDir(filepath.Dir(etc)) {
			return etc
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "relay-secrets.json"
	}
	return filepath.Join(home, ".opensave", "relay-secrets.json")
}

func canWriteDir(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	probe := filepath.Join(dir, ".opensave-write-probe")
	if err := os.WriteFile(probe, []byte("x"), 0o600); err != nil {
		return false
	}
	_ = os.Remove(probe)
	return true
}

// LoadSecrets reads the secrets file. A missing file is not an error: a relay
// with no secrets configured runs fine, it just answers "not configured" for
// the features that need them.
func LoadSecrets(path string) (SecretsFile, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return SecretsFile{}, nil
	}
	if err != nil {
		return SecretsFile{}, err
	}
	var s SecretsFile
	if err := json.Unmarshal(raw, &s); err != nil {
		return SecretsFile{}, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	return s, nil
}

// SaveSecrets writes the file so only its owner can read it.
//
// Written to a temporary file and renamed, so an interrupted write cannot
// leave a half-file that parses as "no secrets configured" — which would
// silently disable the features rather than failing.
func SaveSecrets(path string, s SecretsFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	// 0600 before anything is written to it, not after: a chmod that follows
	// the write leaves a window where the secret is readable.
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	// Rename preserves the temp file's mode on Unix; set it again in case the
	// destination already existed with looser permissions.
	if err := os.Chmod(path, 0o600); err != nil && runtime.GOOS != "windows" {
		return err
	}
	return nil
}

// ResolveSecrets combines the environment and the file, environment first.
//
// Returns which source each value came from, so `setup` and the startup banner
// can tell an operator where a value they did not expect is coming from —
// "it is in the file but the environment is overriding it" is otherwise a
// genuinely confusing thing to debug.
func ResolveSecrets(path string) (secrets SecretsFile, sources map[string]string) {
	sources = map[string]string{}
	file, _ := LoadSecrets(path)

	pick := func(name, env, fromFile string) string {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			sources[name] = "environment (" + env + ")"
			return v
		}
		if fromFile != "" {
			sources[name] = "file (" + path + ")"
			return fromFile
		}
		sources[name] = "not set"
		return ""
	}

	secrets.GoogleClientSecret = pick("googleClientSecret", "GOOGLE_DRIVE_CLIENT_SECRET", file.GoogleClientSecret)
	secrets.SteamGridDBKey = pick("steamGridDBKey", "STEAMGRIDDB_KEY", file.SteamGridDBKey)
	return secrets, sources
}

// MaskSecret renders a secret for display without revealing it.
//
// Shows the last four characters so an operator can tell two keys apart and
// confirm they pasted the one they meant, which is the only reason to print it
// at all.
func MaskSecret(s string) string {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return "(not set)"
	case len(s) <= 4:
		return strings.Repeat("•", len(s))
	default:
		return strings.Repeat("•", 8) + s[len(s)-4:]
	}
}
