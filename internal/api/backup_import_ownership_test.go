package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// Reproduces the three independent native stock data-loss cases from issue
// #68 with only disposable HTTP server, SQLite, archive and save files.
func TestBackupImportCannotOverwriteAnotherGamesOwnedPath(t *testing.T) {
	cases := []struct{ name, kind string }{
		{"owned-primary-exact", "primary"},
		{"owned-named-root-exact", "named"},
		{"inside-owned-named-root", "nested"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := startTestServer(t)
			root := filepath.Join(t.TempDir(), "synthetic-owned-root")
			target := root
			if tc.kind == "nested" {
				target = filepath.Join(root, "nested-save")
			}
			if err := os.MkdirAll(target, 0700); err != nil {
				t.Fatal(err)
			}
			save := filepath.Join(target, "slot.sav")
			if err := os.WriteFile(save, []byte("ARCHIVED-OLD-SAVE"), 0600); err != nil {
				t.Fatal(err)
			}
			archive := exportV2(t, ts, []map[string]string{{"id": "untracked-imported", "name": "Untracked Imported", "savePath": target}})
			ownerPath := ts.saveDir
			if tc.kind == "primary" {
				ownerPath = root
			}
			resp, body := ts.do(t, http.MethodPost, "/api/games", map[string]string{"name": "Existing Owner", "savePath": ownerPath})
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("owner setup HTTP %d: %v", resp.StatusCode, body)
			}
			waitInitialSnapshot(t, ts, "existing-owner")
			if tc.kind != "primary" {
				resp, body = ts.do(t, http.MethodPost, "/api/games/existing-owner/roots", map[string]string{"name": "config", "path": root})
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("named-root setup HTTP %d: %v", resp.StatusCode, body)
				}
			}
			if err := os.WriteFile(save, []byte("OWNER-LIVE-PROTECTED"), 0600); err != nil {
				t.Fatal(err)
			}
			resp, body = ts.do(t, http.MethodPost, "/api/backup/restore", map[string]string{"sourcePath": archive, "mode": "overwrite"})
			after, err := os.ReadFile(save)
			if err != nil {
				t.Fatalf("owner save missing after backup import: %v", err)
			}
			t.Logf("native import scenario=%s status=%d response=%v bytes=%q", tc.name, resp.StatusCode, body, after)
			if string(after) != "OWNER-LIVE-PROTECTED" {
				t.Fatalf("import overwrote another game's live save: %q", after)
			}
			if _, err := ts.daemon.Store.GetGame("untracked-imported"); err == nil {
				t.Fatal("conflicting imported game was registered")
			}
		})
	}
}

func TestBackupImportRefusesOverwriteWhenSafetyDirectoryUnavailable(t *testing.T) {
	ts := startTestServer(t)
	target := filepath.Join(t.TempDir(), "backup-import-destination")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	slot := filepath.Join(target, "slot.sav")
	if err := os.WriteFile(slot, []byte("ARCHIVED-OLD"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := exportV2(t, ts, []map[string]string{{"id": "untracked-safety-failure", "name": "Untracked", "savePath": target}})
	if err := os.WriteFile(slot, []byte("KEEP-LIVE"), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := ts.daemon.Store.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	blockedSafety := filepath.Join(settings.BackupsDir, "_import-safety")
	if err := os.WriteFile(blockedSafety, []byte("NOT-A-DIRECTORY"), 0600); err != nil {
		t.Fatal(err)
	}
	resp, body := ts.do(t, http.MethodPost, "/api/backup/restore", map[string]string{"sourcePath": archive, "mode": "overwrite"})
	got, err := os.ReadFile(slot)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("safety folder unavailable: HTTP=%d response=%v live=%q", resp.StatusCode, body, got)
	if string(got) != "KEEP-LIVE" {
		t.Fatalf("import overwrote live save without safety backup: %q", got)
	}
	if _, err := ts.daemon.Store.GetGame("untracked-safety-failure"); err == nil {
		t.Fatal("unsafe import tracked")
	}
}

func TestRestoreEmptyImportTarget(t *testing.T) {
	t.Run("previously absent", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "new-root")
		if err := os.MkdirAll(filepath.Join(target, "nested"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "nested", "partial.sav"), []byte("partial"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := restoreEmptyImportTarget(target, false); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("new partial root remains: %v", err)
		}
	})
	t.Run("existing empty directory", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "existing-root")
		if err := os.Mkdir(target, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(target, "nested"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "nested", "partial.sav"), []byte("partial"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := restoreEmptyImportTarget(target, true); err != nil {
			t.Fatal(err)
		}
		items, err := os.ReadDir(target)
		if err != nil || len(items) != 0 {
			t.Fatalf("original empty directory not restored: %v %v", items, err)
		}
	})
}
