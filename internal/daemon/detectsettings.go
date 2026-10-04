package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/ignore"
	"github.com/opensave/opensave/internal/store"
)

// Finding a game's settings by what they hold, and giving back what turns out
// to be a save.
//
// Detection (presets/detectsettings.go) takes a file for settings by its
// contents. Before anything is left out, and again at every later run, the
// game's snapshots are asked how the file behaved (snapshot/history.go). And
// as long as a file is left out, how it behaves is watched: in this device's
// automatic snapshots, and in what every other device sends (observeSettings).
// A settings file is changed now and then; a save changes with the sessions
// played. One that changes along with the rest of the save in most of them is
// a save after all: it syncs again, for good, whatever named it — this
// detection or the game database — and the log says which and why.
//
// Errors cost little either way. A settings file taken for a save syncs, as
// every file did before. A save taken for settings is still in every
// snapshot, and comes back to syncing at the latest when it has been seen to
// change with the save a few times.

// detectorVersion moves when detection changes, so every game is judged again.
const detectorVersion = "1"

const (
	observeMinMoved = 3 // changes along with the save before a file is given back
	detectEvery     = 24 * time.Hour
)

func settingsDetectMark(gameID string) string { return "settings_detect:" + gameID }

// detectSettingsDue runs detection for every game not judged under this
// detector and folder yet, or not for a day. In the background, from Start.
func (d *Daemon) detectSettingsDue(ctx context.Context) {
	games, err := d.Store.ListGames()
	if err != nil {
		return
	}
	for _, game := range games {
		if ctx.Err() != nil {
			return
		}
		mark := d.Store.Mark(settingsDetectMark(game.ID))
		want := detectorVersion + "|" + game.SavePath
		if prev, at, ok := strings.Cut(mark, "@"); ok && prev == want {
			if t, err := time.Parse(time.RFC3339, at); err == nil && time.Since(t) < detectEvery {
				continue
			}
		}
		d.DetectSettings(game)
		_ = d.Store.SetMark(settingsDetectMark(game.ID), want+"@"+time.Now().UTC().Format(time.RFC3339))
	}
}

// DetectSettings judges a game's files, records what is new, and re-asks the
// snapshots about everything taken for settings so far.
func (d *Daemon) DetectSettings(game store.Game) {
	if d.Scanner == nil || game.SavePath == "" || SaveFolderMissing(game.SavePath) {
		return
	}
	d.devMu.Lock()
	db := d.devDBLocked()
	d.devMu.Unlock()

	known := map[string]store.SettingsFile{}
	if verdicts, err := d.Store.SettingsFiles(game.ID); err == nil {
		for _, v := range verdicts {
			known[v.Path] = v
		}
	}
	found := db.DetectSettings(game.Name, game.AppID, game.SavePath)
	var ask []string
	for _, f := range found {
		if _, ok := known[f.Path]; !ok {
			ask = append(ask, f.Path)
		}
	}
	for p, v := range known {
		if v.Verdict == store.VerdictSettings {
			ask = append(ask, p)
		}
	}
	saves := d.Snapshots.ChangesLikeASave(game.ID, ask)

	changed := false
	var taken, given []string
	for _, f := range found {
		if _, ok := known[f.Path]; ok {
			continue
		}
		v := store.SettingsFile{GameID: game.ID, Path: f.Path, Verdict: store.VerdictSettings, Reason: f.Reason}
		if why, isSave := saves[f.Path]; isSave {
			v.Verdict, v.Reason = store.VerdictSave, why
		} else {
			taken = append(taken, f.Path)
		}
		if d.Store.SetSettingsFile(v) == nil {
			changed = true
		}
	}
	for p, v := range known {
		if why, isSave := saves[p]; isSave && v.Verdict == store.VerdictSettings {
			v.Verdict, v.Reason, v.AtMs = store.VerdictSave, why, 0
			if d.Store.SetSettingsFile(v) == nil {
				changed = true
				given = append(given, p+" ("+why+")")
			}
		}
	}
	if !changed {
		return
	}
	d.exclusionsChanged(game)
	if len(taken) > 0 && !game.SyncDeviceSettings {
		d.Log.Log("info", fmt.Sprintf("%q: %s %s this device's settings, not the save — each device keeps its own. "+
			"Untick %s under the game's \"Files that shouldn't sync\" to sync %s after all.",
			game.Name, strings.Join(taken, ", "), isAre(len(taken)), itThem(len(taken)), itThem(len(taken))))
	}
	if len(given) > 0 {
		d.Log.Log("warn", fmt.Sprintf("%q: %s changed along with the save, so %s part of the save after all — syncing again.",
			game.Name, strings.Join(given, ", "), itThey(len(given))))
	}
}

// exclusionsChanged carries a game's recorded hashes over to its exclusion
// list as it is now (adoptExclusionView), after detection changed it.
func (d *Daemon) exclusionsChanged(game store.Game) {
	d.forgetVerdicts(game.ID)
	if fresh, err := d.Store.GetGame(game.ID); err == nil {
		game = fresh
	}
	d.adoptExclusionView([]store.Game{game})
}

// observeSettings is told what a device holds of a game — another device's
// manifest as it arrives, or this device's own at an automatic snapshot — and
// keeps count, for every file left out as this game's settings, of how often
// it changed when the rest of the save did. peerID is "" for this device.
func (d *Daemon) observeSettings(game store.Game, peerID string, m delta.Manifest) {
	if game.SyncDeviceSettings {
		return
	}
	settings := d.deviceSettingsFor(game)
	if len(settings) == 0 {
		return
	}
	asSettings := ignore.Parse(strings.Join(settings, "\n"))
	var watched []string
	rest := delta.Manifest{Files: map[string]delta.FileEntry{}}
	for p, f := range m.Files {
		if asSettings.Match(p) {
			watched = append(watched, p)
		} else {
			rest.Files[p] = f
		}
	}
	if len(watched) == 0 {
		return
	}
	user := ignore.Parse(game.SyncIgnore)
	for p := range rest.Files {
		if delta.NeverSynced(p) || (!user.Empty() && user.Match(p)) {
			delete(rest.Files, p)
		}
	}
	restHash := rest.ManifestHash()

	// The rest of the save, as last seen from this device.
	r := d.Store.GetSettingsObservation(game.ID, peerID, "")
	saveMoved := r.LastHash != "" && r.LastHash != restHash
	first := r.LastHash == ""
	r.LastHash = restHash
	_ = d.Store.SaveSettingsObservation(r)

	var given []string
	for _, p := range watched {
		o := d.Store.GetSettingsObservation(game.ID, peerID, p)
		h := m.Files[p].Hash
		if saveMoved && o.LastHash != "" {
			if o.LastHash != h {
				o.Moved++
			} else {
				o.Still++
			}
		}
		o.LastHash = h
		_ = d.Store.SaveSettingsObservation(o)
		if first || !saveMoved {
			continue
		}
		moved, still := 0, 0
		if all, err := d.Store.SettingsObservations(game.ID, p); err == nil {
			for _, x := range all {
				moved += x.Moved
				still += x.Still
			}
		}
		if moved >= observeMinMoved && moved > still {
			why := fmt.Sprintf("changed in %d of the %d times the save changed", moved, moved+still)
			if d.Store.SetSettingsFile(store.SettingsFile{GameID: game.ID, Path: p, Verdict: store.VerdictSave, Reason: why}) == nil {
				given = append(given, p+" ("+why+")")
			}
		}
	}
	if len(given) > 0 {
		d.exclusionsChanged(game)
		d.Log.Log("warn", fmt.Sprintf("%q: %s changed along with the save, so %s part of the save after all — syncing again.",
			game.Name, strings.Join(given, ", "), itThey(len(given))))
	}
}

// observeLocalSettings is observeSettings for this device's own save, at an
// automatic snapshot.
func (d *Daemon) observeLocalSettings(gameID string) {
	game, err := d.Store.GetGame(gameID)
	if err != nil {
		return
	}
	m, err := delta.BuildManifest(filepath.Clean(game.SavePath))
	if err != nil {
		return
	}
	d.observeSettings(game, "", m)
}

func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func itThey(n int) string {
	if n == 1 {
		return "it is"
	}
	return "they are"
}

func itThem(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}
