package daemon

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/opensave/opensave/internal/presets"
	"github.com/opensave/opensave/internal/store"
)

// Noticing newly installed games.
//
// The save scan used to run only when somebody pressed the button, so a game
// installed since sat untracked — no snapshots, nothing synced — until they
// thought to scan again, usually after losing something. The daemon now runs
// the same scan in the background and says when it finds a game that is new:
// not tracked, not seen by an earlier scan, and with save files in it.
//
// It says so; it does not track. The scan's results are good enough to put in
// front of a person and not good enough to act on unasked — a launcher's
// cache folder, a second copy of a game already tracked under another name —
// and tracking something means snapshotting it and sending it to every
// paired device.

// NewGame is a game the background scan found that nothing tracks yet.
type NewGame struct {
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
	AppID    string `json:"appId,omitempty"`
}

// How soon after start the background scan first runs, and how often after.
// The first waits out startup — a scan reads a good deal of the disk, and a
// machine that has just booted has enough to do. Variables so tests can drive
// them.
var (
	newGameFirstScan    = 3 * time.Minute
	newGameScanInterval = time.Hour
)

// newGameState serialises the background scan's writes to what is waiting.
type newGameState struct {
	mu sync.Mutex
}

// ScanForSaves runs the save scan: every location the scanner knows, less
// the excluded ones, measured and grouped into games. One at a time — the
// scanner rewrites its name cache as it goes, and the background scan and the
// Games page's button must not both be doing that at once.
func (d *Daemon) ScanForSaves() ([]presets.DiscoveredSave, error) {
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	settings, err := d.Store.GetSettings()
	if err != nil {
		return nil, err
	}
	found := d.Scanner.Scan(settings.CustomScanPaths)
	found = presets.FilterExcluded(found, settings.ExcludePaths)
	// Measured after excluding, so the budget is spent only on locations that
	// will actually be offered. Empty ones are kept: the scan screen hides
	// them behind a toggle rather than losing them.
	presets.Measure(found)
	// After measuring: which folder of a game is the one to track depends on
	// which of them hold anything and when they were last written.
	presets.Group(found)
	if found == nil {
		found = []presets.DiscoveredSave{}
	}
	return found, nil
}

// DetectNewGames runs the scan and announces the games that are new since the
// last one. The first run only takes stock.
func (d *Daemon) DetectNewGames() {
	settings, err := d.Store.GetSettings()
	if err != nil || !settings.DetectNewGames {
		return
	}
	found, err := d.ScanForSaves()
	if err != nil {
		return
	}
	tracked, err := d.trackedSavePaths()
	if err != nil {
		return
	}
	hadStock, err := d.Store.StockTaken()
	if err != nil {
		return
	}
	d.newGames.mu.Lock()
	defer d.newGames.mu.Unlock()

	var remember []store.KnownSave
	var fresh []NewGame
	for _, group := range presets.Groups(found) {
		// Taking stock, everything found is remembered, files or not. A
		// folder the scan could not measure this time — measuring shares one
		// time budget across a whole library — or one empty today is still
		// already here, and letting it through would have a later scan
		// announce an old game as new. The live app did exactly that, with
		// twenty-odd games at once.
		//
		// After that, a folder with nothing in it is not news: a launcher
		// makes one for every game you own. It becomes news when the game
		// first writes to it, so it is not remembered and the next scan looks
		// again. The same goes for one the scan could not measure.
		if hadStock && !groupHasFiles(group) {
			continue
		}
		known, isTracked := false, false
		for _, r := range group {
			if k, err := d.Store.IsKnownSave(r.SavePath); err == nil && k {
				known = true
			}
			for _, t := range tracked {
				if store.PathsOverlap(r.SavePath, t) {
					isTracked = true
				}
			}
		}
		news := !known && !isTracked && hadStock
		// Every folder of the game, so a later scan that picks another of them
		// as the one to track does not announce it a second time. The one to
		// track is marked as waiting, if this is news.
		for i, r := range group {
			remember = append(remember, store.KnownSave{Path: r.SavePath, Name: r.Name, AppID: r.AppID,
				Pending: news && i == 0})
		}
		if news {
			fresh = append(fresh, NewGame{Name: group[0].Name, SavePath: group[0].SavePath, AppID: group[0].AppID})
		}
	}
	if err := d.Store.RememberSaves(remember); err != nil {
		d.Log.Log("warn", fmt.Sprintf("could not note the saves the scan found: %v", err))
		return
	}
	if !hadStock {
		// Announcing everything untracked the first time would bury the one
		// real newcomer under every game somebody chose not to track. Marked
		// even when nothing was found: an empty machine's first game is news.
		if err := d.Store.MarkStockTaken(); err != nil {
			d.Log.Log("warn", fmt.Sprintf("could not note that the first scan ran: %v", err))
			return
		}
		d.Log.Log("info", fmt.Sprintf(
			"background scan: noted %d save folder(s) already on this machine; games installed from now on will be pointed out", len(remember)))
		return
	}
	if len(fresh) == 0 {
		return
	}
	names := make([]string, 0, len(fresh))
	for _, g := range fresh {
		names = append(names, g.Name)
	}
	d.Log.Log("info", fmt.Sprintf("found %d game(s) with saves OpenSave isn't keeping yet: %s — track them from a scan on the Games page, or `opensave scan`",
		len(names), strings.Join(names, ", ")))
	if d.OnNewGames != nil {
		d.OnNewGames(d.NewGames())
	}
}

// groupHasFiles reports whether any folder of a game holds files, as far as
// the scan could measure.
func groupHasFiles(group []presets.DiscoveredSave) bool {
	for _, r := range group {
		if r.Measured && r.FileCount > 0 {
			return true
		}
	}
	return false
}

// trackedSavePaths lists every folder a tracked game owns.
func (d *Daemon) trackedSavePaths() ([]string, error) {
	games, err := d.Store.ListGames()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, g := range games {
		out = append(out, g.SavePath)
		if roots, err := d.Store.GameRootPaths(g.ID); err == nil {
			for _, p := range roots {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

// NewGames returns the found games still waiting to be looked at. One tracked
// since it was found is no longer waiting.
func (d *Daemon) NewGames() []NewGame {
	pending, err := d.Store.PendingSaves()
	if err != nil {
		return []NewGame{}
	}
	tracked, _ := d.trackedSavePaths()
	out := []NewGame{}
	for _, k := range pending {
		isTracked := false
		for _, t := range tracked {
			if store.PathsOverlap(k.Path, t) {
				isTracked = true
				break
			}
		}
		if !isTracked {
			out = append(out, NewGame{Name: k.Name, SavePath: k.Path, AppID: k.AppID})
		}
	}
	return out
}

// DismissNewGames clears the waiting list. The games stay remembered, so they
// are not announced again.
func (d *Daemon) DismissNewGames() {
	d.newGames.mu.Lock()
	err := d.Store.ClearPendingSaves()
	d.newGames.mu.Unlock()
	if err != nil {
		d.Log.Log("warn", fmt.Sprintf("could not clear the new games: %v", err))
	}
	if d.OnNewGames != nil {
		d.OnNewGames([]NewGame{})
	}
}
