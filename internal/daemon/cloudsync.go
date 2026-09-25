package daemon

import (
	"errors"
	"fmt"
	"github.com/opensave/opensave/internal/ignore"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opensave/opensave/internal/cloud"
	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
	"github.com/opensave/opensave/internal/watcher"
)

// Reading the cloud mirror back.
//
// Every snapshot has long been copied to the cloud provider, and nothing ever
// read those copies except a person on the cloud screen. So the one thing a
// cloud copy is best at — carrying a save to a device while the one that made
// it is switched off — needed that person to know to go and look. GitHub
// issue #12 asked for the obvious: say so when there is a newer save up there.
//
// Each device now announces, per game, which snapshot IS its save (a "head",
// see internal/cloud/heads.go) and reads the other devices' heads. A newer
// save that carries on from the one this device has, on a device that has not
// changed its own since, is put in place without asking — the same thing
// syncing between two devices that are both on does. Anything else is asked
// about, because taking it would replace progress made here.

// CloudOffer is a newer save for one game, from another of this person's
// devices, waiting for a yes or a no.
type CloudOffer struct {
	GameID     string `json:"gameId"`
	GameName   string `json:"gameName"`
	SnapshotID string `json:"snapshotId"`
	FileName   string `json:"fileName"`
	DeviceName string `json:"deviceName"`
	SavedAt    string `json:"savedAt"`
	Provider   string `json:"provider"`
	// Diverged is true when this device's save holds progress the other one
	// does not carry — the two moved apart, or never shared a save at all —
	// so taking the other one replaces progress made here.
	Diverged bool `json:"diverged"`
}

// CloudPulled reports a save that was put in place without asking.
type CloudPulled struct {
	GameID     string `json:"gameId"`
	GameName   string `json:"gameName"`
	DeviceName string `json:"deviceName"`
	SnapshotID string `json:"snapshotId"`
}

// How soon after start the cloud is first read, and how often after that. The
// first check is what answers issue #12 — "when I open the app" — and waits
// only long enough for startup to finish. Variables so tests can drive them.
var (
	cloudCheckDelay    = 20 * time.Second
	cloudCheckInterval = 10 * time.Minute
)

// ErrCloudOfferGone is returned for an answer to an offer that is no longer
// open — already taken, dismissed, or overtaken by a newer save.
var ErrCloudOfferGone = errors.New("that save is no longer waiting — check the cloud screen for the newest one")

// cloudReader is the daemon's state for reading the mirror.
type cloudReader struct {
	// check allows one check or pull at a time: two overlapping would read
	// the same head and both restore it.
	check sync.Mutex
	// heads serialises read-modify-write of a game's cloud_heads row, which
	// snapshots, restores, uploads and answers all update from their own
	// goroutines.
	heads sync.Mutex

	offersMu sync.Mutex
	offers   []CloudOffer

	// cache holds heads already downloaded. A head file is never rewritten
	// — each announcement is a new file — so its name identifies its content.
	cacheMu sync.Mutex
	cache   map[string]cloud.Head
}

// cloudListable reports whether a provider can be read back at all. A webhook
// only receives.
func cloudListable(provider string) bool {
	return provider != "" && provider != "webhook"
}

// cloudSnapshotName is the name this device gives a snapshot in the cloud.
func cloudSnapshotName(gameID, branch, snapID string) string {
	return fmt.Sprintf("%s__%s__%s.zip", gameID, branch, snapID)
}

// snapMillis reads the moment out of a snap_<unix ms> id; 0 when it has none.
func snapMillis(snapID string) int64 {
	ms, err := strconv.ParseInt(strings.TrimPrefix(snapID, "snap_"), 10, 64)
	if err != nil {
		return 0
	}
	return ms
}

// stampMillis reads a stored timestamp; 0 when it cannot.
func stampMillis(stamp string) int64 {
	for _, layout := range []string{"2006-01-02T15:04:05.000Z", time.RFC3339Nano} {
		if t, err := time.Parse(layout, stamp); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

func nowStamp() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }

// appendChain adds a save to the end of a chain, keeping the newest
// cloud.MaxHeadChain.
func appendChain(chain []string, snapID string) []string {
	if n := len(chain); n > 0 && chain[n-1] == snapID {
		return chain
	}
	chain = append(append([]string(nil), chain...), snapID)
	if extra := len(chain) - cloud.MaxHeadChain; extra > 0 {
		chain = chain[extra:]
	}
	return chain
}

// noteSnapshotForCloud keeps the game's record current as snapshots are
// taken. Only a snapshot of the save as it stands moves it; a copy kept
// before replacing a save is history, not this device's save.
func (d *Daemon) noteSnapshotForCloud(snap store.Snapshot, current bool) {
	if !current {
		return
	}
	game, err := d.Store.GetGame(snap.GameID)
	if err != nil || snap.BranchName != game.ActiveBranch {
		return
	}
	d.cloudRd.heads.Lock()
	defer d.cloudRd.heads.Unlock()
	rec, _, err := d.Store.GetCloudHead(game.ID)
	if err != nil {
		d.Log.Log("warn", fmt.Sprintf("cloud: could not record %s as %q's save: %v", snap.ID, game.Name, err))
		return
	}
	rec.Snapshot = snap.ID
	rec.Since = snap.Timestamp
	rec.File = cloudSnapshotName(game.ID, snap.BranchName, snap.ID)
	rec.Chain = appendChain(rec.Chain, snap.ID)
	if err := d.Store.SetCloudHead(rec); err != nil {
		d.Log.Log("warn", fmt.Sprintf("cloud: could not record %s as %q's save: %v", snap.ID, game.Name, err))
	}
}

// noteRestoreForCloud records a restored snapshot as the game's save, and
// announces it.
//
// The chain starts again from it. Whatever this device did before the
// restore is not what its save carries on from any more — whether the
// snapshot came from another device or from this one's own past.
func (d *Daemon) noteRestoreForCloud(snap store.Snapshot) {
	game, err := d.Store.GetGame(snap.GameID)
	if err != nil {
		return
	}
	d.cloudRd.heads.Lock()
	rec, _, err := d.Store.GetCloudHead(game.ID)
	if err == nil {
		rec.Snapshot = snap.ID
		rec.Since = nowStamp()
		rec.File = cloudSnapshotName(game.ID, snap.BranchName, snap.ID)
		rec.Chain = []string{snap.ID}
		err = d.Store.SetCloudHead(rec)
	}
	d.cloudRd.heads.Unlock()
	if err != nil {
		d.Log.Log("warn", fmt.Sprintf("cloud: could not record the restore of %q: %v", game.Name, err))
		return
	}
	d.uploads.Add(1)
	go func() {
		defer d.uploads.Done()
		_ = d.publishHead(game.ID)
	}()
}

// publishHead announces the game's current save in the cloud.
func (d *Daemon) publishHead(gameID string) error {
	cfg, err := d.Store.GetCloudConfig()
	if err != nil || !cfg.Enabled || !cloudListable(cfg.Provider) {
		return nil
	}
	d.cloudRd.heads.Lock()
	rec, ok, err := d.Store.GetCloudHead(gameID)
	d.cloudRd.heads.Unlock()
	if err != nil || !ok || rec.Snapshot == "" {
		return err
	}
	game, err := d.Store.GetGame(gameID)
	if err != nil {
		return err
	}
	settings, err := d.Store.GetSettings()
	if err != nil {
		return err
	}
	savedAt := ""
	if ms := snapMillis(rec.Snapshot); ms > 0 {
		savedAt = time.UnixMilli(ms).UTC().Format(time.RFC3339)
	}
	head := cloud.Head{
		Version:    cloud.HeadVersion,
		DeviceID:   settings.NodeID,
		DeviceName: settings.DeviceName,
		GameID:     game.ID,
		Branch:     game.ActiveBranch,
		Snapshot:   rec.Snapshot,
		File:       rec.File,
		Chain:      rec.Chain,
		SavedAt:    savedAt,
	}
	if _, err := d.Cloud.PublishHead(head, time.Now()); err != nil {
		if !cloud.IsNotConfigured(err) {
			d.Log.Log("warn", fmt.Sprintf("cloud: could not announce %q's save: %v", game.Name, err))
		}
		return err
	}
	d.cloudRd.heads.Lock()
	defer d.cloudRd.heads.Unlock()
	if rec, _, err := d.Store.GetCloudHead(gameID); err == nil {
		rec.Published = head.Snapshot
		_ = d.Store.SetCloudHead(rec)
	}
	return nil
}

// cloudHeadFor returns the game's record, starting one from its newest
// snapshot the first time — a device upgraded from a version without records
// has history but no statement of which snapshot is its save.
func (d *Daemon) cloudHeadFor(game store.Game) (store.CloudHead, error) {
	d.cloudRd.heads.Lock()
	defer d.cloudRd.heads.Unlock()
	rec, ok, err := d.Store.GetCloudHead(game.ID)
	if err != nil || ok {
		return rec, err
	}
	snaps, err := d.Store.ListSnapshots(game.ID, game.ActiveBranch)
	if err != nil || len(snaps) == 0 {
		return rec, err
	}
	latest := snaps[0]
	rec.Snapshot = latest.ID
	rec.Since = latest.Timestamp
	rec.File = cloudSnapshotName(game.ID, latest.BranchName, latest.ID)
	rec.Chain = []string{latest.ID}
	return rec, d.Store.SetCloudHead(rec)
}

// cloudCandidate is another device's head worth acting on.
type cloudCandidate struct {
	head cloud.Head
	file string
	// follows is true when the other save carries on from this device's —
	// this device's save is in its chain — so taking it replaces nothing.
	follows bool
}

// chooseCloudCandidate picks, from other devices' heads, the one save this
// device should take or be asked about, if any.
//
// Newer is required in both cases. A save that carries on from ours but is
// older than the moment ours was set — a deliberate rollback here, say — is
// not something to put back over it.
func chooseCloudCandidate(rec store.CloudHead, activeBranch string, heads []cloud.Head,
	have func(snapID string) bool, fileFor func(snapID string) string) (cloudCandidate, bool) {

	var best cloudCandidate
	found := false
	since := stampMillis(rec.Since)
	for _, h := range heads {
		if h.Version != cloud.HeadVersion || h.Snapshot == "" || h.Branch != activeBranch {
			continue
		}
		if h.Snapshot == rec.Snapshot || h.Snapshot == rec.Dismissed || have(h.Snapshot) {
			continue
		}
		file := fileFor(h.Snapshot)
		if file == "" {
			continue // announced, but its archive has not arrived (yet)
		}
		at := snapMillis(h.Snapshot)
		if at <= since {
			continue
		}
		follows := false
		if rec.Snapshot != "" {
			for _, id := range h.Chain {
				if id == rec.Snapshot {
					follows = true
					break
				}
			}
		}
		if !found || at > snapMillis(best.head.Snapshot) {
			best = cloudCandidate{head: h, file: file, follows: follows}
			found = true
		}
	}
	return best, found
}

// saveState is how a game's save stands against its last snapshot.
type saveState int

const (
	saveUnchanged saveState = iota
	saveChanged
	saveInUse
	// saveUnknown: nothing recorded to compare with. A game tracked and never
	// changed since has no baseline — the first snapshot does not record one
	// (see the watcher's catch-up for why it must not).
	saveUnknown
)

// saveStateOf says whether the game's save is as its last automatic snapshot
// left it, has changed since, or is open in the game right now.
func (d *Daemon) saveStateOf(game store.Game) saveState {
	if watcher.AnyFileLocked(game.SavePath) {
		return saveInUse
	}
	if game.LastManifestHash == "" {
		return saveUnknown
	}
	hash, err := d.currentContentHash(game)
	if err != nil || hash != game.LastManifestHash {
		return saveChanged
	}
	return saveUnchanged
}

// neverHeldFiles reports whether this device has never had a file of this
// game's save: none on disk now, and none in any snapshot it has taken. The
// snapshots are opened and counted rather than trusted to a record, because
// versions before the file records existed wrote none — a snapshot with no
// record may still have held a save, and a save someone deleted is not a save
// that never was.
func (d *Daemon) neverHeldFiles(game store.Game) bool {
	extra, err := d.Store.GameRootPaths(game.ID)
	if err != nil {
		return false
	}
	m, _, err := delta.BuildMultiManifest(game.SavePath, extra)
	if err != nil || len(m.Files) > 0 {
		return false
	}
	for _, root := range m.Extra {
		if len(root.Files) > 0 {
			return false
		}
	}
	branches, err := d.Store.ListBranches(game.ID)
	if err != nil {
		return false
	}
	for _, b := range branches {
		snaps, err := d.Store.ListSnapshots(game.ID, b)
		if err != nil {
			return false
		}
		for _, sn := range snaps {
			n, err := snapshot.ArchiveFileCount(sn.ZipPath)
			if err != nil || n > 0 {
				return false // unreadable counts as "may have held one"
			}
		}
	}
	return true
}

// currentContentHash reads the save from disk and hashes it the way the
// watcher does when it records a snapshot.
func (d *Daemon) currentContentHash(game store.Game) (string, error) {
	extra, err := d.Store.GameRootPaths(game.ID)
	if err != nil {
		extra = nil
	}
	delta.InvalidateRoot(game.SavePath)
	for _, p := range extra {
		delta.InvalidateRoot(p)
	}
	m, _, err := delta.BuildMultiManifest(game.SavePath, extra)
	if err != nil {
		return "", err
	}
	return watcher.ContentHash(m, game.SyncIgnore), nil
}

// CheckCloud reads the other devices' heads and acts on the ones ahead of
// this device: takes what continues from its save, asks about the rest.
func (d *Daemon) CheckCloud() {
	// Paused means nothing comes from the cloud either; resuming checks.
	if d.P2P.Pause.Paused() {
		return
	}
	d.cloudRd.check.Lock()
	defer d.cloudRd.check.Unlock()

	cfg, err := d.Store.GetCloudConfig()
	if err != nil || !cfg.Enabled || !cloudListable(cfg.Provider) {
		d.setCloudOffers(nil)
		return
	}
	files, err := d.Cloud.List()
	if err != nil {
		if cloud.IsNotConfigured(err) {
			d.setCloudOffers(nil)
		} else {
			d.Log.Log("warn", fmt.Sprintf("cloud: could not check for newer saves: %v", err))
		}
		return
	}
	settings, err := d.Store.GetSettings()
	if err != nil {
		return
	}
	ownKey := cloud.DeviceKey(settings.NodeID)

	snapFiles := map[string]string{}                  // game id/branch/snapshot id -> file
	newest := map[string]map[string]cloud.CloudFile{} // game id -> device -> head
	newestAt := map[string]int64{}                    // game id/device -> ms
	ownHeads := map[string][]cloud.CloudFile{}        // game id -> this device's heads
	for _, f := range files {
		if gameID, dev, at, ok := cloud.ParseHeadFileName(f.Name); ok {
			if dev == ownKey {
				ownHeads[gameID] = append(ownHeads[gameID], f)
				continue
			}
			key := gameID + "/" + dev
			if at > newestAt[key] {
				newestAt[key] = at
				if newest[gameID] == nil {
					newest[gameID] = map[string]cloud.CloudFile{}
				}
				newest[gameID][dev] = f
			}
			continue
		}
		if gameID, branch, snapID, ok := snapshot.ParseExportEntryName(f.Name); ok {
			snapFiles[gameID+"/"+branch+"/"+snapID] = f.Name
		}
	}

	games, err := d.Store.ListGames()
	if err != nil {
		return
	}
	var offers []CloudOffer
	for _, game := range games {
		d.pruneOwnHeads(ownHeads[game.ID])
		rec, err := d.cloudHeadFor(game)
		if err != nil {
			continue
		}
		ids, err := d.Store.LinkedGameIDs(game.ID)
		if err != nil || len(ids) == 0 {
			ids = []string{game.ID}
		}
		// Found by game, branch and snapshot, never by snapshot alone. Ids are
		// milliseconds, so two devices snapshotting two different games in the
		// same one share an id — and looking it up by id alone could hand this
		// game the other one's archive to restore.
		fileFor := func(snapID string) string {
			for _, id := range ids {
				if f := snapFiles[id+"/"+game.ActiveBranch+"/"+snapID]; f != "" {
					return f
				}
			}
			return ""
		}

		// Announce this device's save if the cloud does not know it yet: a
		// device upgraded from a version without heads, or an announcement
		// that failed.
		if rec.Snapshot != "" && rec.Published != rec.Snapshot && fileFor(rec.Snapshot) != "" {
			_ = d.publishHead(game.ID)
		}
		var heads []cloud.Head
		for _, id := range ids {
			for _, f := range newest[id] {
				if h, err := d.readHead(f.Name); err == nil {
					heads = append(heads, h)
				}
			}
		}
		// This game's, for the same reason: another game's snapshot with the
		// same id says nothing about whether this one has it.
		have := func(snapID string) bool {
			snap, err := d.Store.GetSnapshot(snapID)
			return err == nil && snap.GameID == game.ID
		}
		cand, ok := chooseCloudCandidate(rec, game.ActiveBranch, heads, have, fileFor)
		if !ok {
			continue
		}

		// A save that has never held a file on this device — the folder was
		// empty when it was tracked and has been since — has nothing to lose,
		// whatever the other device's line of saves. That is a new device
		// being set up, the case the cloud is best at, and it is filled
		// without asking. One that held files and is empty now is a deletion
		// somebody made, and that is still asked about.
		state := d.saveStateOf(game)
		if state == saveInUse {
			continue // the game is running; not now, and not a question either
		}
		follows := cand.follows
		if !follows && d.neverHeldFiles(game) {
			follows, state = true, saveUnchanged
		}
		if follows && state == saveUnchanged && settings.CloudAutoPull && game.AutoSync {
			err := d.pullFromCloud(game, cand.file, cand.head.DeviceName)
			if err == nil {
				if d.OnCloudPulled != nil {
					d.OnCloudPulled(CloudPulled{GameID: game.ID, GameName: game.Name,
						DeviceName: cand.head.DeviceName, SnapshotID: cand.head.Snapshot})
				}
				continue
			}
			d.Log.Log("warn", fmt.Sprintf("cloud: could not bring %s's save for %q here: %v",
				cand.head.DeviceName, game.Name, err))
		}
		// Asked. Marked as replacing progress only when it would: the other
		// save does not carry on from this one, or this one has changed since.
		// Not when this device's state is merely unknown — a game tracked and
		// never changed has no recorded baseline, and telling that person
		// their device "has progress of its own" would be untrue.
		diverged := !follows || state == saveChanged
		offers = append(offers, CloudOffer{
			GameID:     game.ID,
			GameName:   game.Name,
			SnapshotID: cand.head.Snapshot,
			FileName:   cand.file,
			DeviceName: cand.head.DeviceName,
			SavedAt:    cand.head.SavedAt,
			Provider:   cfg.Provider,
			Diverged:   diverged,
		})
	}
	d.setCloudOffers(offers)
}

// readHead fetches a head, from the cache when it has been read before.
func (d *Daemon) readHead(name string) (cloud.Head, error) {
	d.cloudRd.cacheMu.Lock()
	h, ok := d.cloudRd.cache[name]
	d.cloudRd.cacheMu.Unlock()
	if ok {
		return h, nil
	}
	h, err := d.Cloud.FetchHead(name)
	if err != nil {
		return cloud.Head{}, err
	}
	d.cloudRd.cacheMu.Lock()
	if d.cloudRd.cache == nil {
		d.cloudRd.cache = map[string]cloud.Head{}
	}
	d.cloudRd.cache[name] = h
	d.cloudRd.cacheMu.Unlock()
	return h, nil
}

// pruneOwnHeads removes this device's older announcements for one game; only
// the newest means anything, and without this every save would leave one
// behind for good.
func (d *Daemon) pruneOwnHeads(files []cloud.CloudFile) {
	if len(files) < 2 {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		_, _, a, _ := cloud.ParseHeadFileName(files[i].Name)
		_, _, b, _ := cloud.ParseHeadFileName(files[j].Name)
		return a > b
	})
	for _, f := range files[1:] {
		_ = d.Cloud.Delete(f)
	}
}

// pullFromCloud puts another device's cloud snapshot in place as this
// device's save. The save it replaces is kept first, as every restore keeps
// it.
func (d *Daemon) pullFromCloud(game store.Game, fileName, deviceName string) error {
	_, branch, snapID, ok := snapshot.ParseExportEntryName(fileName)
	if !ok {
		return fmt.Errorf("%q is not a snapshot", fileName)
	}
	settings, err := d.Store.GetSettings()
	if err != nil {
		return err
	}
	destDir := filepath.Join(settings.BackupsDir, game.ID, branch)
	if err := os.MkdirAll(destDir, 0o777); err != nil {
		return err
	}
	destPath := filepath.Join(destDir, snapID+".zip")
	if err := d.Cloud.Download(fileName, destPath); err != nil {
		return err
	}
	info, err := os.Stat(destPath)
	if err != nil {
		return err
	}
	if err := d.EnsureImportedSnapshot(game.ID, branch, snapID, destPath, info.Size()); err != nil {
		return err
	}
	// Another device's save: the files this device keeps for itself stay
	// its own. See Manager.RestoreKeeping.
	if _, err := d.Snapshots.RestoreKeeping(game.ID, snapID, ignore.Parse(game.SyncIgnore)); err != nil {
		return err
	}
	// What is on disk now is that snapshot, so record it as the baseline.
	// Otherwise the watcher sees the restore as the game writing new data,
	// snapshots it again, and the copy goes back up as if it were new.
	if hash, err := d.currentContentHash(game); err == nil {
		_ = d.Store.SetLastManifestHash(game.ID, hash)
	}
	d.Log.Log("success", fmt.Sprintf("cloud: brought %s's save for %q here", deviceName, game.Name))
	if d.OnGameChanged != nil {
		d.OnGameChanged(game.ID)
	}
	return nil
}

// AcceptCloudOffer takes the offered save.
func (d *Daemon) AcceptCloudOffer(gameID, snapshotID string) error {
	d.cloudRd.check.Lock()
	defer d.cloudRd.check.Unlock()
	offer, ok := d.findCloudOffer(gameID, snapshotID)
	if !ok {
		return ErrCloudOfferGone
	}
	game, err := d.Store.GetGame(gameID)
	if err != nil {
		return err
	}
	if watcher.AnyFileLocked(game.SavePath) {
		return fmt.Errorf("%s has its save files open — close the game, then try again", game.Name)
	}
	if err := d.pullFromCloud(game, offer.FileName, offer.DeviceName); err != nil {
		return err
	}
	d.dropCloudOffers(gameID)
	return nil
}

// DismissCloudOffer says no to the offered save. A newer one is offered again.
func (d *Daemon) DismissCloudOffer(gameID, snapshotID string) error {
	d.cloudRd.heads.Lock()
	rec, _, err := d.Store.GetCloudHead(gameID)
	if err == nil {
		rec.Dismissed = snapshotID
		err = d.Store.SetCloudHead(rec)
	}
	d.cloudRd.heads.Unlock()
	if err != nil {
		return err
	}
	d.dropCloudOffers(gameID)
	return nil
}

// CloudOffers returns the saves waiting for an answer.
func (d *Daemon) CloudOffers() []CloudOffer {
	d.cloudRd.offersMu.Lock()
	defer d.cloudRd.offersMu.Unlock()
	return append([]CloudOffer{}, d.cloudRd.offers...)
}

func (d *Daemon) findCloudOffer(gameID, snapshotID string) (CloudOffer, bool) {
	for _, o := range d.CloudOffers() {
		if o.GameID == gameID && o.SnapshotID == snapshotID {
			return o, true
		}
	}
	return CloudOffer{}, false
}

// dropCloudOffers removes a game's offer, after it has been answered or the
// save restored some other way.
func (d *Daemon) dropCloudOffers(gameID string) {
	var kept []CloudOffer
	for _, o := range d.CloudOffers() {
		if o.GameID != gameID {
			kept = append(kept, o)
		}
	}
	d.setCloudOffers(kept)
}

// setCloudOffers replaces the open offers, telling listeners — and the log,
// which is all a headless install has — about the ones that are new.
func (d *Daemon) setCloudOffers(offers []CloudOffer) {
	d.cloudRd.offersMu.Lock()
	before := map[string]bool{}
	for _, o := range d.cloudRd.offers {
		before[o.GameID+"/"+o.SnapshotID] = true
	}
	changed := len(before) != len(offers)
	for _, o := range offers {
		if !before[o.GameID+"/"+o.SnapshotID] {
			changed = true
			d.Log.Log("info", fmt.Sprintf("cloud: %s has a newer save for %q — open OpenSave to bring it here", o.DeviceName, o.GameName))
		}
	}
	d.cloudRd.offers = append([]CloudOffer{}, offers...)
	d.cloudRd.offersMu.Unlock()
	if changed && d.OnCloudOffers != nil {
		d.OnCloudOffers(d.CloudOffers())
	}
}

// ForgetCloudOffers drops a game's offer after its save was restored by some
// other route — the cloud screen — so the banner does not keep asking about
// a question already answered.
func (d *Daemon) ForgetCloudOffers(gameID string) { d.dropCloudOffers(gameID) }
