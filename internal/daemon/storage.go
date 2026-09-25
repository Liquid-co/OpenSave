package daemon

import (
	"sort"

	"github.com/opensave/opensave/internal/fsx"
	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
)

// Where the space goes: how much each game's snapshots take, which snapshots
// are the biggest, how much room is left, and how much "Clean up now" would
// give back. Sizes are the archives' as recorded when each was taken; what
// they take on disk, once older snapshots share their files, is DiskBytes.

// GameStorage is one game's share.
type GameStorage struct {
	GameID    string `json:"gameId"`
	Name      string `json:"name"`
	Bytes     int64  `json:"bytes"`
	Snapshots int    `json:"snapshots"`
	Pinned    int    `json:"pinned"`
	// Reclaimable is what clean-up would free from this game now.
	Reclaimable int64 `json:"reclaimable"`
	// DiskBytes is what its snapshots take on disk: less than Bytes, the
	// size of each snapshot's archive, once older ones share the files they
	// have in common (snapshot/shared.go).
	DiskBytes int64 `json:"diskBytes"`
}

// SnapshotSize is one snapshot, for the list of the biggest.
type SnapshotSize struct {
	GameID       string `json:"gameId"`
	GameName     string `json:"gameName"`
	SnapshotID   string `json:"snapshotId"`
	Branch       string `json:"branch"`
	Timestamp    string `json:"timestamp"`
	Comment      string `json:"comment"`
	Note         string `json:"note,omitempty"`
	Bytes        int64  `json:"bytes"`
	Pinned       bool   `json:"pinned"`
	IsSystemAuto bool   `json:"isSystemAuto"`
}

// StorageReport is the whole picture.
type StorageReport struct {
	BackupsDir string         `json:"backupsDir"`
	TotalBytes int64          `json:"totalBytes"`
	Snapshots  int            `json:"snapshots"`
	FreeBytes  uint64         `json:"freeBytes"`
	FreeKnown  bool           `json:"freeKnown"`
	Games      []GameStorage  `json:"games"`
	Biggest    []SnapshotSize `json:"biggest"`
	// Reclaimable is what "Clean up now" would free across every game, and
	// ReclaimableSnapshots how many snapshots that is.
	Reclaimable          int64 `json:"reclaimable"`
	ReclaimableSnapshots int   `json:"reclaimableSnapshots"`
	// DiskBytes is what every snapshot takes on disk, as for each game.
	DiskBytes int64 `json:"diskBytes"`
}

// BiggestShown is how many of the largest snapshots the report lists.
const BiggestShown = 10

// Storage reports where snapshot space goes.
func (d *Daemon) Storage() (StorageReport, error) {
	settings, err := d.Store.GetSettings()
	if err != nil {
		return StorageReport{}, err
	}
	report := StorageReport{BackupsDir: settings.BackupsDir, Games: []GameStorage{}, Biggest: []SnapshotSize{}}
	report.FreeBytes, report.FreeKnown = fsx.FreeBytes(settings.BackupsDir)

	plan, err := d.Snapshots.PrunePlan()
	if err != nil {
		return StorageReport{}, err
	}
	reclaim := map[string]int64{}
	for _, s := range plan {
		reclaim[s.GameID] += s.SizeBytes
		report.Reclaimable += s.SizeBytes
	}
	report.ReclaimableSnapshots = len(plan)

	games, err := d.Store.ListGames()
	if err != nil {
		return StorageReport{}, err
	}
	var all []SnapshotSize
	var every []store.Snapshot
	for _, g := range games {
		gs := GameStorage{GameID: g.ID, Name: g.Name, Reclaimable: reclaim[g.ID]}
		branches, _ := d.Store.ListBranches(g.ID)
		var its []store.Snapshot
		for _, b := range branches {
			snaps, _ := d.Store.ListSnapshots(g.ID, b)
			its = append(its, snaps...)
			for _, s := range snaps {
				gs.Bytes += s.SizeBytes
				gs.Snapshots++
				if s.Pinned {
					gs.Pinned++
				}
				all = append(all, SnapshotSize{
					GameID: g.ID, GameName: g.Name, SnapshotID: s.ID, Branch: b,
					Timestamp: s.Timestamp, Comment: s.Comment, Note: s.Note,
					Bytes: s.SizeBytes, Pinned: s.Pinned, IsSystemAuto: s.IsSystemAuto,
				})
			}
		}
		gs.DiskBytes = snapshot.DiskBytes(its)
		every = append(every, its...)
		report.TotalBytes += gs.Bytes
		report.Snapshots += gs.Snapshots
		report.Games = append(report.Games, gs)
	}
	report.DiskBytes = snapshot.DiskBytes(every)
	sort.SliceStable(report.Games, func(i, j int) bool { return report.Games[i].Bytes > report.Games[j].Bytes })
	sort.SliceStable(all, func(i, j int) bool { return all[i].Bytes > all[j].Bytes })
	if len(all) > BiggestShown {
		all = all[:BiggestShown]
	}
	report.Biggest = append(report.Biggest, all...)
	return report, nil
}
