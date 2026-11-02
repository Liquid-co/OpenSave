package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/opensave/opensave/internal/presets"
	"github.com/opensave/opensave/internal/sessions"
)

// "Track game" offers first the games running now that the game database
// knows: once each, however many of their programs run, and never a program
// of the system's own.
func TestRunningGames_TheGamesTheDatabaseKnowsRunningNow(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the game database below uses Windows paths")
	}
	d := newTestDaemon(t)
	dir := t.TempDir()
	manifest := `
Pearl Game:
  files:
    "<winLocalAppData>/Pearl/CD/save":
      tags: [save]
  installDir:
    Pearl Game: {}
  steam:
    id: 3321460
`
	if err := os.WriteFile(filepath.Join(dir, "ludusavi-manifest.yaml"), []byte(manifest), 0o666); err != nil {
		t.Fatal(err)
	}
	d.Scanner = &presets.Scanner{CacheFile: filepath.Join(dir, "steam-app-cache.json"), GOOS: "windows"}
	t.Setenv("WINDIR", `C:\Windows`)
	d.sessions.list = func() ([]sessions.Proc, error) {
		return []sessions.Proc{
			{PID: 10, Exe: `G:\Games\Pearl Game\bin64\game.exe`},
			{PID: 11, Exe: `G:\Games\Pearl Game\bin64\crashreporter.exe`},
			{PID: 12, Exe: `C:\Windows\System32\svchost.exe`},
			{PID: 13, Exe: `C:\Tools\editor.exe`},
		}, nil
	}

	got := d.RunningGames()
	if len(got) != 1 {
		t.Fatalf("running games = %+v, want Pearl Game once", got)
	}
	if got[0].Name != "Pearl Game" || got[0].AppID != "3321460" || got[0].Exe != `G:\Games\Pearl Game\bin64\game.exe` {
		t.Errorf("running game = %+v", got[0])
	}
}
