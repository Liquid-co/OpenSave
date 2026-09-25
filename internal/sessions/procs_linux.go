//go:build linux

package sessions

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Steam's launcher for a game: "… reaper SteamLaunch AppId=1145360 -- …".
var appIDArg = regexp.MustCompile(`^AppId=(\d+)$`)

// listProcesses reads /proc: each process's program, command line, and the
// Steam app it was started for, if Steam said. Another user's processes
// cannot be read and are skipped.
func listProcesses() ([]Proc, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var out []Proc
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 1 {
			continue
		}
		dir := filepath.Join("/proc", e.Name())
		p := Proc{PID: pid}
		p.Exe, _ = os.Readlink(filepath.Join(dir, "exe"))
		if raw, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			for _, arg := range bytes.Split(bytes.TrimRight(raw, "\x00"), []byte{0}) {
				if len(arg) > 0 {
					p.Args = append(p.Args, string(arg))
				}
			}
		}
		for _, arg := range p.Args {
			if m := appIDArg.FindStringSubmatch(arg); m != nil {
				p.SteamAppID = m[1]
			}
		}
		if p.SteamAppID == "" {
			p.SteamAppID = steamAppIDFromEnv(filepath.Join(dir, "environ"))
		}
		if p.Exe == "" && len(p.Args) == 0 {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// steamAppIDFromEnv reads SteamAppId (or SteamGameId) from a process's
// environment. Steam sets it for everything a game starts, Proton included.
func steamAppIDFromEnv(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, kv := range bytes.Split(raw, []byte{0}) {
		s := string(kv)
		for _, key := range []string{"SteamAppId=", "SteamGameId="} {
			if strings.HasPrefix(s, key) {
				if id := strings.TrimPrefix(s, key); id != "" && id != "0" {
					return id
				}
			}
		}
	}
	return ""
}
