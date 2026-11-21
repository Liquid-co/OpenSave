package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/p2p/syncengine"
	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
)

// cloudCopy is one snapshot archive in the cloud.
//
// Known says this PC holds the same snapshot, and only then are Comment,
// IsSystemAuto and Safety filled in: they are this PC's record of why it was
// taken. A cloud file carries no such record of its own, so a copy this PC
// does not hold is shown as just that, never guessed at.
type cloudCopy struct {
	Name        string `json:"name"`
	ID          string `json:"id,omitempty"`
	SizeBytes   int64  `json:"sizeBytes"`
	CreatedTime string `json:"createdTime"`
	Branch      string `json:"branch"`
	SnapshotID  string `json:"snapshotId"`
	// TakenMs is when the snapshot was taken, read from its id (snap_<unix
	// ms>); 0 when the id does not say. CreatedTime is only when the file
	// reached the cloud, which for a copy uploaded later is not the same.
	TakenMs      int64  `json:"takenMs"`
	Known        bool   `json:"known"`
	Comment      string `json:"comment,omitempty"`
	IsSystemAuto bool   `json:"isSystemAuto,omitempty"`
	Safety       bool   `json:"safety,omitempty"`
}

// cloudGame is one game on the Backups page: what the cloud holds of it, and
// whether this PC's current save is among it.
type cloudGame struct {
	GameID   string      `json:"gameId"`
	GameName string      `json:"gameName"`
	AppID    string      `json:"appId,omitempty"`
	Tracked  bool        `json:"tracked"`
	Copies   []cloudCopy `json:"copies"`
	// TotalSize is the copies' total.
	TotalSize int64 `json:"totalSize"`
	// LocalSnapshots is how many snapshots this PC holds of the game.
	LocalSnapshots int `json:"localSnapshots"`
	// Current is the snapshot this PC's save is at: the cloud head, or the
	// newest snapshot of the active branch when that came later (see
	// handleCloudOverview). Empty when it has none. CurrentUploaded says the
	// cloud holds it.
	Current         string `json:"current,omitempty"`
	CurrentUploaded bool   `json:"currentUploaded"`
	// Owed counts uploads that failed and are waiting to be tried again.
	Owed int `json:"owed"`
}

// handleCloudOverview answers the Backups page in one call: every tracked
// game, every game only the cloud holds, each game's copies, and whether the
// save each tracked game is at has reached the cloud.
func (s *Server) handleCloudOverview(w http.ResponseWriter, r *http.Request) {
	files, err := s.Daemon.Cloud.List()
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	games, err := s.Daemon.Store.ListGames()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	retries, _ := s.Daemon.Store.CloudRetries()

	byID := map[string]*cloudGame{}
	order := []string{}
	local := map[string]map[string]store.Snapshot{} // game → snapshot id → record
	for _, g := range games {
		cg := &cloudGame{GameID: g.ID, GameName: g.Name, AppID: g.AppID, Tracked: true, Copies: []cloudCopy{}}
		snaps := map[string]store.Snapshot{}
		branches, _ := s.Daemon.Store.ListBranches(g.ID)
		// The newest snapshot of the save on the active branch, and apart
		// from it the newest a sync kept. A safety copy is the save before
		// something replaced it, and an imported one is history brought in
		// from a file: neither is the save as it stands.
		var newest, newestSync store.Snapshot
		for _, b := range branches {
			list, err := s.Daemon.Store.ListSnapshots(g.ID, b)
			if err != nil {
				continue
			}
			for _, sn := range list {
				snaps[sn.ID] = sn
				if b != g.ActiveBranch || sn.Safety || sn.Comment == importedComment {
					continue
				}
				if sn.Timestamp > newest.Timestamp {
					newest = sn
				}
				if strings.HasPrefix(sn.Comment, syncengine.MirrorCommentPrefix) && sn.Timestamp > newestSync.Timestamp {
					newestSync = sn
				}
			}
		}
		cg.LocalSnapshots = len(snaps)
		// The save as it stands: the cloud record, which moves on every
		// snapshot taken and every restore, but not on the snapshot a sync
		// keeps, which the sync engine writes itself. A newer one of those
		// is the save after that sync. With no record yet, the newest.
		cg.Current = newest.ID
		if head, ok, err := s.Daemon.Store.GetCloudHead(g.ID); err == nil && ok && head.Snapshot != "" {
			cg.Current = head.Snapshot
			if newestSync.ID != "" && stampMs(newestSync.Timestamp) > stampMs(head.Since) {
				cg.Current = newestSync.ID
			}
		}
		local[g.ID] = snaps
		byID[g.ID] = cg
		order = append(order, g.ID)
	}
	for _, rt := range retries {
		if cg := byID[rt.GameID]; cg != nil {
			cg.Owed++
		}
	}

	// As the browse route does: an id linked to another folds into it.
	canonical := map[string]string{}
	resolve := func(id string) string {
		if c, done := canonical[id]; done {
			return c
		}
		c := id
		if ids, err := s.Daemon.Store.LinkedGameIDs(id); err == nil && len(ids) > 0 {
			c = ids[0]
		}
		canonical[id] = c
		return c
	}
	for _, f := range files {
		rawID, branch, snapID, ok := snapshot.ParseExportEntryName(f.Name)
		if !ok {
			continue
		}
		gameID := resolve(rawID)
		cg := byID[gameID]
		if cg == nil {
			cg = &cloudGame{GameID: gameID, GameName: gameID, Copies: []cloudCopy{}}
			if g, err := s.Daemon.Store.GetGame(gameID); err == nil && g.Name != "" {
				cg.GameName = g.Name
			}
			byID[gameID] = cg
			order = append(order, gameID)
		}
		c := cloudCopy{Name: f.Name, ID: f.ID, SizeBytes: f.SizeBytes, CreatedTime: f.CreatedTime, Branch: branch, SnapshotID: snapID, TakenMs: snapIDMillis(snapID)}
		if sn, ok := local[gameID][snapID]; ok {
			c.Known, c.Comment, c.IsSystemAuto, c.Safety = true, sn.Comment, sn.IsSystemAuto, sn.Safety
		}
		if snapID == cg.Current {
			cg.CurrentUploaded = true
		}
		cg.Copies = append(cg.Copies, c)
		cg.TotalSize += f.SizeBytes
	}

	out := make([]*cloudGame, 0, len(order))
	for _, id := range order {
		cg := byID[id]
		// Newest first.
		sort.SliceStable(cg.Copies, func(i, j int) bool {
			return copyMoment(cg.Copies[i]) > copyMoment(cg.Copies[j])
		})
		out = append(out, cg)
	}
	writeJSON(w, http.StatusOK, out)
}

// copyMoment orders copies: when the snapshot was taken where its id says,
// otherwise when the file reached the cloud.
func copyMoment(c cloudCopy) int64 {
	if c.TakenMs > 0 {
		return c.TakenMs
	}
	if t, err := time.Parse(time.RFC3339Nano, c.CreatedTime); err == nil {
		return t.UnixMilli()
	}
	return 0
}

// importedComment is what an import gives the snapshots it adds to a game's
// history (daemon.ImportArchive).
const importedComment = "Imported from backup"

// stampMs reads a stored timestamp; 0 when it cannot.
func stampMs(stamp string) int64 {
	if t, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
		return t.UnixMilli()
	}
	return 0
}

// snapIDMillis reads the moment out of a snap_<unix ms> id; 0 when it has
// none. The same reading as the daemon's snapMillis.
func snapIDMillis(id string) int64 {
	ms, err := strconv.ParseInt(strings.TrimPrefix(id, "snap_"), 10, 64)
	if err != nil || !strings.HasPrefix(id, "snap_") {
		return 0
	}
	return ms
}
