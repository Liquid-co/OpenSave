//go:build darwin

package sessions

import (
	"encoding/binary"
	"reflect"
	"testing"
)

// kern.procargs2 as the kernel lays it out: argc, the program path padded
// with NULs, the arguments, the environment, and then nothing.
func TestParseProcArgs(t *testing.T) {
	raw := binary.LittleEndian.AppendUint32(nil, 2)
	raw = append(raw, "/Applications/Game.app/Contents/MacOS/Game\x00\x00\x00\x00"...)
	raw = append(raw, "/Applications/Game.app/Contents/MacOS/Game\x00-windowed\x00"...)
	raw = append(raw, "HOME=/Users/me\x00SteamAppId=1145360\x00\x00\x00"...)

	p, ok := parseProcArgs(raw)
	if !ok {
		t.Fatal("not parsed")
	}
	if p.Exe != "/Applications/Game.app/Contents/MacOS/Game" {
		t.Errorf("Exe = %q", p.Exe)
	}
	if want := []string{"/Applications/Game.app/Contents/MacOS/Game", "-windowed"}; !reflect.DeepEqual(p.Args, want) {
		t.Errorf("Args = %q, want %q", p.Args, want)
	}
	if p.SteamAppID != "1145360" {
		t.Errorf("SteamAppID = %q", p.SteamAppID)
	}

	// A relative program path is not where the program is.
	raw = binary.LittleEndian.AppendUint32(nil, 1)
	raw = append(raw, "./game\x00\x00./game\x00\x00"...)
	if p, ok := parseProcArgs(raw); !ok || p.Exe != "" || len(p.Args) != 1 {
		t.Errorf("relative: %+v, %v", p, ok)
	}

	if _, ok := parseProcArgs([]byte{1}); ok {
		t.Error("a truncated record was parsed")
	}
}
