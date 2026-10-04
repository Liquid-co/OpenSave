package daemon

import (
	"strings"

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
	loaded bool
	db     *presets.DeviceSettings
	// patterns: the game database's, by game name, App ID and folder.
	patterns map[string][]string
	// verdicts: what was found here (detectsettings.go), by game ID.
	verdicts map[string][]store.SettingsFile
}

// devDB returns the game database for device settings, read when first
// needed. Called with devMu held.
func (d *Daemon) devDBLocked() *presets.DeviceSettings {
	if !d.dev.loaded {
		d.dev.db = d.Scanner.DeviceSettings()
		d.dev.loaded = true
	}
	if d.dev.patterns == nil {
		d.dev.patterns = map[string][]string{}
	}
	if d.dev.verdicts == nil {
		d.dev.verdicts = map[string][]store.SettingsFile{}
	}
	return d.dev.db
}

// DeviceSetting is one entry of a game's device settings, for the window.
type DeviceSetting struct {
	Pattern string `json:"pattern"`
	// Source: "database" (the game database names it), "detected" (found by
	// what it holds), or "save" (shown to change with the save, so it syncs
	// whatever named it).
	Source string `json:"source"`
	Reason string `json:"reason,omitempty"`
}

// deviceSettingsList is every entry, in the order they apply: the game
// database's, what was detected here, then what was shown to be a save —
// as "!" lines, which bring a file back whatever named it before.
func (d *Daemon) deviceSettingsList(game store.Game) []DeviceSetting {
	if d.Scanner == nil {
		return nil
	}
	key := game.Name + "\x00" + game.AppID + "\x00" + game.SavePath
	d.devMu.Lock()
	defer d.devMu.Unlock()
	db := d.devDBLocked()
	pats, ok := d.dev.patterns[key]
	if !ok {
		pats = db.Patterns(game.Name, game.AppID, []string{game.SavePath})
		d.dev.patterns[key] = pats
	}
	verdicts, ok := d.dev.verdicts[game.ID]
	if !ok {
		verdicts, _ = d.Store.SettingsFiles(game.ID)
		d.dev.verdicts[game.ID] = verdicts
	}
	var out []DeviceSetting
	for _, p := range pats {
		out = append(out, DeviceSetting{Pattern: p, Source: "database"})
	}
	for _, v := range verdicts {
		if v.Verdict == store.VerdictSettings {
			out = append(out, DeviceSetting{Pattern: "/" + v.Path, Source: "detected", Reason: v.Reason})
		}
	}
	for _, v := range verdicts {
		if v.Verdict == store.VerdictSave {
			out = append(out, DeviceSetting{Pattern: "!/" + v.Path, Source: "save", Reason: v.Reason})
		}
	}
	return out
}

func (d *Daemon) deviceSettingsFor(game store.Game) []string {
	list := d.deviceSettingsList(game)
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.Pattern)
	}
	return out
}

// forgetVerdicts drops a game's cached verdicts after they changed.
func (d *Daemon) forgetVerdicts(gameID string) {
	d.devMu.Lock()
	delete(d.dev.verdicts, gameID)
	d.devMu.Unlock()
}

// IgnoreText is the exclusion list a game is synced under (see
// syncengine.Engine.IgnoreText).
func (d *Daemon) IgnoreText(game store.Game) string {
	if d.P2P == nil || d.P2P.Sync == nil {
		return game.SyncIgnore
	}
	return d.P2P.Sync.IgnoreText(game)
}

// DeviceSettings returns a game's device settings, whether or not it is set
// to sync them — what the window lists.
func (d *Daemon) DeviceSettings(game store.Game) []DeviceSetting {
	return d.deviceSettingsList(game)
}

// exclusionsMark records, per game, the exclusion list its recorded hashes
// were last taken under, and the never-synced list of the build that took
// them.
func exclusionsMark(gameID string) string { return "exclusions:" + gameID }

func exclusionsValue(text string) string {
	return delta.NeverSyncedList + "\n--\n" + text
}

// NoteExclusions records that a game's hashes are now taken under its
// current exclusion list — after the rules were changed and the hashes
// rebased by whoever changed them.
func (d *Daemon) NoteExclusions(game store.Game) {
	_ = d.Store.SetMark(exclusionsMark(game.ID), exclusionsValue(d.IgnoreText(game)))
}

// adoptExclusionView carries every game's recorded hashes over to what the
// game is excluded by now, when that changed without anyone writing a rule:
// a build with a new delta.NeverSyncedList, or the game database naming
// settings files. Each is re-taken only when the save is the one it was
// recorded for, so a real change still counts.
//
// Without it, a build that starts excluding a file sees every save holding
// one as changed: an auto-snapshot of a game nobody played, and a new version
// of its own on every device at once, which the devices then disagree over.
func (d *Daemon) adoptExclusionView(games []store.Game) {
	neverSyncedDone := d.Store.Mark("never_synced") == delta.NeverSyncedList
	for _, game := range games {
		now := d.IgnoreText(game)
		oldText, beforeNeverSynced := game.SyncIgnore, !neverSyncedDone
		if prev := d.Store.Mark(exclusionsMark(game.ID)); prev != "" {
			list, text, _ := strings.Cut(prev, "\n--\n")
			if list == delta.NeverSyncedList && text == now {
				continue
			}
			oldText, beforeNeverSynced = text, list != delta.NeverSyncedList
		} else if !beforeNeverSynced && oldText == now {
			_ = d.Store.SetMark(exclusionsMark(game.ID), exclusionsValue(now))
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
		was := watcher.ContentHash(m, oldText)
		if beforeNeverSynced {
			was = watcher.ContentHashBeforeNeverSynced(m, oldText)
		}
		if game.LastManifestHash != "" && game.LastManifestHash == was {
			_ = d.Store.SetLastManifestHash(game.ID, watcher.ContentHash(m, now))
		}
		if d.P2P != nil && d.P2P.Sync != nil {
			primary := m
			primary.Extra = nil
			d.P2P.Sync.AdoptExclusionView(game.ID, primary, oldText, beforeNeverSynced)
		}
		_ = d.Store.SetMark(exclusionsMark(game.ID), exclusionsValue(now))
	}
	_ = d.Store.SetMark("never_synced", delta.NeverSyncedList)
}
