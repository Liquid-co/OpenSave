package daemon

import (
	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/presets"
	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/watcher"
)

// Device settings: the files the game database names as a game's settings
// rather than its save are left out of syncing (presets/devicesettings.go),
// through the game's exclusion list (syncengine.IgnoreText).
//
// Worked out once per run and kept: the database is read when first needed,
// and a game's answer only changes when its name, App ID or folder does —
// which makes it a different key here. A database update takes effect at the
// next start, where adoptExclusionView carries every recorded hash over.

type devSettingsCache struct {
	loaded   bool
	db       *presets.DeviceSettings
	patterns map[string][]string
}

func (d *Daemon) deviceSettingsFor(game store.Game) []string {
	if d.Scanner == nil {
		return nil
	}
	key := game.Name + "\x00" + game.AppID + "\x00" + game.SavePath
	d.devMu.Lock()
	defer d.devMu.Unlock()
	if !d.dev.loaded {
		d.dev.db = d.Scanner.DeviceSettings()
		d.dev.patterns = map[string][]string{}
		d.dev.loaded = true
	}
	if p, ok := d.dev.patterns[key]; ok {
		return p
	}
	p := d.dev.db.Patterns(game.Name, game.AppID, []string{game.SavePath})
	d.dev.patterns[key] = p
	return p
}

// IgnoreText is the exclusion list a game is synced under (see
// syncengine.Engine.IgnoreText).
func (d *Daemon) IgnoreText(game store.Game) string {
	if d.P2P == nil || d.P2P.Sync == nil {
		return game.SyncIgnore
	}
	return d.P2P.Sync.IgnoreText(game)
}

// DeviceSettings returns the patterns for a game's device settings, whether
// or not it is set to sync them — what the window lists.
func (d *Daemon) DeviceSettings(game store.Game) []string {
	return d.deviceSettingsFor(game)
}

// adoptExclusionView carries a game's last-snapshot hash over to what the game
// is excluded by now, when the game database names settings files in its save
// folder: at the first start with this build, or when the database starts
// naming them. Without it the watcher takes the new view for a change, and
// snapshots — and syncs — a game nobody played.
//
// Recognised by the hash itself: it still matches the save under the game's
// own rules alone, as it was recorded before any settings were left out. Then
// the save is the one it was recorded for, and the hash is re-taken under the
// new list; a save that did change keeps its old hash, and its change counts.
// A change of the settings list between two runs that both left some out is
// not recognised, and costs at most one automatic snapshot, the safe
// direction (as daemon.LastSnapshotHashUnder). The base agreed with each
// device needs nothing here: the sync engine translates a base recorded under
// other exclusions whenever it compares (syncengine, #52).
func (d *Daemon) adoptExclusionView(games []store.Game) {
	for _, game := range games {
		now := d.IgnoreText(game)
		if now == game.SyncIgnore || game.LastManifestHash == "" {
			continue
		}
		extra, err := d.Store.GameRootPaths(game.ID)
		if err != nil {
			extra = nil
		}
		m, failures, err := delta.BuildMultiManifest(game.SavePath, extra)
		if err != nil || len(failures) > 0 {
			continue // unreadable now: tried again at the next start
		}
		if game.LastManifestHash == watcher.ContentHash(m, game.SyncIgnore) {
			_ = d.Store.SetLastManifestHash(game.ID, watcher.ContentHash(m, now))
		}
	}
}
