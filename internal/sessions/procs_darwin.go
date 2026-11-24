//go:build darwin

package sessions

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// listProcesses asks the kernel for every process, then each one for what it
// was started with (kern.procargs2): its program, its command line, and its
// environment, read for SteamAppId as on Linux. Another user's processes
// cannot be read and are skipped, as on Linux. No cgo: the release builds
// the macOS CLI with CGO_ENABLED=0.
//
// Until this, a game played on a Mac was never seen running, so nothing held
// its sync back while it was played (syncengine/playing.go): another
// device's save could be written under a running game.
func listProcesses() ([]Proc, error) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	var out []Proc
	for _, kp := range kps {
		pid := int(kp.Proc.P_pid)
		if pid <= 1 {
			continue
		}
		raw, err := unix.SysctlRaw("kern.procargs2", pid)
		if err != nil {
			continue
		}
		p, ok := parseProcArgs(raw)
		if !ok {
			continue
		}
		p.PID = pid
		out = append(out, p)
	}
	return out, nil
}

// parseProcArgs reads kern.procargs2: the argument count, the path the
// program was started from, then the arguments and the environment, each
// NUL-terminated, with NULs padding the path out. A relative path says
// nothing about where the program is installed, so only an absolute one is
// kept as Exe.
func parseProcArgs(raw []byte) (Proc, bool) {
	var p Proc
	if len(raw) < 4 {
		return p, false
	}
	argc := int(binary.LittleEndian.Uint32(raw[:4]))
	rest := raw[4:]
	end := bytes.IndexByte(rest, 0)
	if end < 0 {
		return p, false
	}
	if exe := string(rest[:end]); filepath.IsAbs(exe) {
		p.Exe = exe
	}
	fields := bytes.Split(bytes.TrimLeft(rest[end:], "\x00"), []byte{0})
	for i, f := range fields {
		if i < argc {
			if len(f) > 0 {
				p.Args = append(p.Args, string(f))
			}
			continue
		}
		if len(f) == 0 {
			break // the end of the environment
		}
		s := string(f)
		for _, key := range []string{"SteamAppId=", "SteamGameId="} {
			if id := strings.TrimPrefix(s, key); p.SteamAppID == "" && id != s && id != "" && id != "0" {
				p.SteamAppID = id
			}
		}
	}
	if p.Exe == "" && len(p.Args) == 0 {
		return p, false
	}
	return p, true
}
