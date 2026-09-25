package e2e

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/opensave/opensave/testutil"
)

// Exporting and importing saves (Cloud Backup → Export / Import, and
// `opensave backup`) since games could be linked, emptied, and could lose
// their folders.

type exportReply struct {
	Exported int `json:"exported"`
	Skipped  []struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	} `json:"skipped"`
}

type importReply struct {
	Restored  int `json:"restored"`
	Snapshots int `json:"snapshots"`
	Skipped   int `json:"skipped"`
	Results   []struct {
		ID     string `json:"id"`
		Action string `json:"action"`
		Error  string `json:"error"`
	} `json:"results"`
}

// savedInExport is what an export holds for one game: its files by name.
func savedInExport(t *testing.T, path, gameID string) map[string]string {
	t.Helper()
	packed, err := io.ReadAll(brotli.NewReader(bytes.NewReader(readFile(t, path))))
	if err != nil {
		t.Fatal(err)
	}
	outer, err := zip.NewReader(bytes.NewReader(packed), int64(len(packed)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range outer.File {
		if f.Name != "saves/"+gameID+".zip" {
			continue
		}
		rc, _ := f.Open()
		raw, _ := io.ReadAll(rc)
		rc.Close()
		out := map[string]string{}
		for name, body := range zipContents(t, raw) {
			out[name] = string(body)
		}
		return out
	}
	return nil
}

// A game whose save folder is gone, or was emptied, still has its save in
// its snapshots: the export carries the newest of those rather than
// skipping the game or writing an empty save — which, imported with
// "overwrite" on another device, would empty that device's save too.
func TestBackup_AGameWithoutItsFilesExportsItsNewestSnapshot(t *testing.T) {
	for _, c := range []struct {
		name  string
		harm  func(t *testing.T, dir string)
	}{
		{"folder gone", func(t *testing.T, dir string) { testutil.RemoveTree(t, dir) }},
		{"folder emptied", func(t *testing.T, dir string) { emptyFolder(t, dir) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			td := testutil.NewTestDaemon(t, "BackupEmpty")
			td.WriteSave("slot1.sav", "hours of play")
			gameID := td.TrackGame("Backup Empty Game")
			td.API(http.MethodPost, "/api/games/"+gameID+"/snapshot", map[string]string{"comment": "kept"}, nil)
			c.harm(t, td.SaveDir)

			target := filepath.Join(testutil.TempDir(t), "saves.sscb")
			var res exportReply
			td.API(http.MethodPost, "/api/backup/export", map[string]any{"targetPath": target, "games": []map[string]string{{"id": gameID}}}, &res)
			if res.Exported != 1 {
				t.Fatalf("export = %+v, want the game exported", res)
			}
			if got := savedInExport(t, target, gameID); got["slot1.sav"] != "hours of play" {
				t.Errorf("the export holds %v for the game, want its newest snapshot's slot1.sav", got)
			}
		})
	}
}

// A backup made before two copies of a game were linked names the one that
// was merged away. Importing it goes to the game it was linked into — not
// skipped as untracked, and not tracked all over again as a duplicate.
func TestBackup_ImportFollowsALink(t *testing.T) {
	for _, mode := range []string{"snapshots", "overwrite"} {
		t.Run(mode, func(t *testing.T) {
			td := testutil.NewTestDaemon(t, "BackupLinked")
			td.WriteSave("slot1.sav", "from the backup")
			oldID := td.TrackGame("Linked Import Game")
			target := filepath.Join(testutil.TempDir(t), "saves.sscb")
			td.API(http.MethodPost, "/api/backup/export", map[string]any{"targetPath": target, "games": []map[string]string{{"id": oldID}}}, nil)

			// The same game at another folder, and the first linked into it.
			other := filepath.Join(testutil.TempDir(t), "steam-copy")
			if err := os.MkdirAll(other, 0o777); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(other, "slot1.sav"), []byte("the steam copy"), 0o666); err != nil {
				t.Fatal(err)
			}
			var steam struct {
				ID string `json:"id"`
			}
			td.API(http.MethodPost, "/api/games", map[string]string{"name": "Linked Import Game Steam", "savePath": other}, &steam)
			td.API(http.MethodPost, "/api/games/"+steam.ID+"/link", map[string]string{"alias": oldID}, nil)
			testutil.SettleSync(t, steam.ID, td)

			var res importReply
			td.API(http.MethodPost, "/api/backup/restore", map[string]string{"sourcePath": target, "mode": mode}, &res)
			if res.Skipped != 0 || len(res.Results) != 1 || res.Results[0].Action == "skipped" {
				t.Fatalf("import = %+v; want the backup's game taken as the one it was linked into", res)
			}
			var games map[string]struct {
				Name     string `json:"name"`
				Branches map[string]struct {
					Snapshots []struct {
						Comment string `json:"comment"`
					} `json:"snapshots"`
				} `json:"branches"`
			}
			td.API(http.MethodGet, "/api/games", nil, &games)
			if _, dup := games[oldID]; dup {
				t.Errorf("the import tracked %q again, beside the game it was linked into", oldID)
			}
			imported := 0
			for _, b := range games[steam.ID].Branches {
				for _, s := range b.Snapshots {
					if s.Comment == "Imported from backup" {
						imported++
					}
				}
			}
			if imported == 0 {
				t.Error("the linked game has no snapshot from the backup")
			}
			if mode == "overwrite" {
				if got, _ := os.ReadFile(filepath.Join(other, "slot1.sav")); string(got) != "from the backup" {
					t.Errorf("overwrite left the linked game's save as %q", got)
				}
			}
		})
	}
}
