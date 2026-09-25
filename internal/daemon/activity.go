package daemon

import (
	"sort"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// What has happened to the saves, for the activity page: one timeline of
// the syncs, snapshots and play sessions of every game, and for each game
// when and where it was last played, snapshotted and synced.
//
// Three records make it up. The activity history (store.ActivityEvent) has
// files received from or sent to another device, deletions another device
// made here, restores, cloud pulls, emptied saves and conflicts. Snapshots and
// play sessions have their own tables. Nothing is kept twice to make this.

// ActivityItem is one thing on the timeline.
type ActivityItem struct {
	AtMs   int64  `json:"atMs"`
	GameID string `json:"gameId"`
	// Kind is a store.Activity* kind, or "snapshot", or "played".
	Kind   string `json:"kind"`
	Device string `json:"device,omitempty"`
	Files  int    `json:"files,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
	Detail string `json:"detail,omitempty"`
	// A snapshot's.
	SnapshotID string `json:"snapshotId,omitempty"`
	Comment    string `json:"comment,omitempty"`
	Auto       bool   `json:"auto,omitempty"`
	// A play session's length.
	DurationMs int64 `json:"durationMs,omitempty"`
}

// GameActivity is where one game stands: when it was last played and on
// which device, and when it was last snapshotted and synced.
type GameActivity struct {
	GameID string `json:"gameId"`
	// LastPlayedAt is when its save last changed through play: a session
	// ending here, or a newer save arriving from another device, which is
	// that device having been played. LastPlayedOn names that device; empty
	// is this one.
	LastPlayedAt int64  `json:"lastPlayedAt,omitempty"`
	LastPlayedOn string `json:"lastPlayedOn,omitempty"`
	// Its newest snapshot, on any branch.
	LastSnapshotAt int64  `json:"lastSnapshotAt,omitempty"`
	LastSnapshot   string `json:"lastSnapshot,omitempty"`
	// When it last matched another device's copy, and which device.
	LastSyncedAt   int64  `json:"lastSyncedAt,omitempty"`
	LastSyncedWith string `json:"lastSyncedWith,omitempty"`
	// Played here, in all.
	PlaytimeMs int64 `json:"playtimeMs,omitempty"`
	Sessions   int   `json:"sessions,omitempty"`
}

// ActivityReport is the activity page's data.
type ActivityReport struct {
	Items []ActivityItem `json:"items"`
	Games []GameActivity `json:"games"`
	More  bool           `json:"more"`
	// Device is this device's name, for "played here".
	Device string `json:"device"`
}

// mirrorPrefix starts the comment of a snapshot kept of another device's
// save when it arrived (syncengine recordMirrorSnapshot).
const mirrorPrefix = "Synced from peer: "

// mirrorDevice is the device a mirrored snapshot came from, if it is one.
func mirrorDevice(comment string) (string, bool) {
	rest, ok := strings.CutPrefix(comment, mirrorPrefix)
	if !ok {
		return "", false
	}
	name, _, _ := strings.Cut(rest, " (")
	return name, name != ""
}

func isoMs(ts string) int64 {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

// Activity is up to limit timeline items older than beforeMs (0: from now),
// of one game or of all when gameID is empty, and every game's standing.
func (d *Daemon) Activity(beforeMs int64, gameID string, limit int) (ActivityReport, error) {
	if limit <= 0 || limit > 500 {
		limit = 150
	}
	report := ActivityReport{Items: []ActivityItem{}, Games: []GameActivity{}}
	if settings, err := d.Store.GetSettings(); err == nil {
		report.Device = settings.DeviceName
	}

	events, err := d.Store.ListActivity(beforeMs, gameID, limit+1)
	if err != nil {
		return report, err
	}
	for _, ev := range events {
		report.Items = append(report.Items, ActivityItem{AtMs: ev.AtMs, GameID: ev.GameID, Kind: ev.Kind,
			Device: ev.Device, Files: ev.Files, Bytes: ev.Bytes, Detail: ev.Detail})
	}

	before := ""
	if beforeMs > 0 {
		before = time.UnixMilli(beforeMs).UTC().Format("2006-01-02T15:04:05.000Z")
	}
	snaps, err := d.Store.RecentSnapshots(before, (limit+1)*4)
	if err != nil {
		return report, err
	}
	n := 0
	for _, s := range snaps {
		if gameID != "" && s.GameID != gameID {
			continue
		}
		item := ActivityItem{AtMs: isoMs(s.Timestamp), GameID: s.GameID, Kind: "snapshot",
			SnapshotID: s.ID, Comment: s.Comment, Auto: s.IsSystemAuto}
		if dev, ok := mirrorDevice(s.Comment); ok {
			item.Device = dev
		}
		report.Items = append(report.Items, item)
		if n++; n > limit {
			break
		}
	}

	sessions, err := d.Store.RecentPlaySessions(beforeMs, (limit+1)*4)
	if err != nil {
		return report, err
	}
	n = 0
	for _, s := range sessions {
		if gameID != "" && s.GameID != gameID {
			continue
		}
		report.Items = append(report.Items, ActivityItem{AtMs: s.EndedMs, GameID: s.GameID, Kind: "played",
			DurationMs: s.EndedMs - s.StartedMs})
		if n++; n > limit {
			break
		}
	}

	sort.SliceStable(report.Items, func(i, j int) bool { return report.Items[i].AtMs > report.Items[j].AtMs })
	if len(report.Items) > limit {
		report.Items = report.Items[:limit]
		report.More = true
	}

	games, err := d.gameActivity(gameID)
	if err != nil {
		return report, err
	}
	report.Games = games
	return report, nil
}

// gameActivity is where each game stands.
func (d *Daemon) gameActivity(only string) ([]GameActivity, error) {
	games, err := d.Store.ListGames()
	if err != nil {
		return nil, err
	}
	play, _ := d.Store.PlayStatsByGame()
	arrived, _ := d.Store.LatestActivity(store.ActivityReceived, store.ActivityCloudPulled)
	peers, _ := d.Store.ListPeers()
	names := map[string]string{}
	for _, p := range peers {
		names[p.ID] = p.Name
	}

	out := []GameActivity{}
	for _, g := range games {
		if only != "" && g.ID != only {
			continue
		}
		ga := GameActivity{GameID: g.ID}
		if st, ok := play[g.ID]; ok {
			ga.PlaytimeMs, ga.Sessions = st.PlaytimeMs, st.Sessions
			ga.LastPlayedAt = st.LastPlayedMs
		}
		// A save that came from another device later than the last session
		// here was made there.
		if ev, ok := arrived[g.ID]; ok && ev.AtMs > ga.LastPlayedAt {
			ga.LastPlayedAt, ga.LastPlayedOn = ev.AtMs, ev.Device
		}

		branches, _ := d.Store.ListBranches(g.ID)
		for _, b := range branches {
			snaps, _ := d.Store.ListSnapshots(g.ID, b) // newest first
			if len(snaps) == 0 {
				continue
			}
			s := snaps[0]
			if at := isoMs(s.Timestamp); at > ga.LastSnapshotAt {
				ga.LastSnapshotAt, ga.LastSnapshot = at, s.Comment
			}
			// Snapshots kept of another device's save, from before the
			// activity history existed: that device was played then.
			for _, m := range snaps {
				if dev, ok := mirrorDevice(m.Comment); ok {
					if at := isoMs(m.Timestamp); at > ga.LastPlayedAt {
						ga.LastPlayedAt, ga.LastPlayedOn = at, dev
					}
					break
				}
			}
		}

		synced, _ := d.Store.GameLastSynced(g.ID)
		for peerID, at := range synced {
			if ms := isoMs(at); ms > ga.LastSyncedAt {
				ga.LastSyncedAt, ga.LastSyncedWith = ms, names[peerID]
			}
		}
		out = append(out, ga)
	}
	return out, nil
}
