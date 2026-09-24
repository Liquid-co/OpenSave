package daemon

import "fmt"

// SnapshotAllResult says how "snapshot everything" went.
type SnapshotAllResult struct {
	Taken  int `json:"taken"`
	Failed []struct {
		GameID string `json:"gameId"`
		Name   string `json:"name"`
		Error  string `json:"error"`
	} `json:"failed"`
}

// SnapshotAll takes a snapshot of every tracked game — the tray's "Snapshot
// every game now", and `opensave snapshot --all`: the moment before a
// reinstall, a drive swap or an experiment, when the question is "is
// everything safe right now" and not "which game do I mean".
//
// They count as snapshots you took, so the automatic budget does not treat
// them as disposable. One game that cannot be snapshotted — a folder that is
// gone — does not stop the rest; it is reported instead.
func (d *Daemon) SnapshotAll(comment string) SnapshotAllResult {
	res := SnapshotAllResult{}
	games, err := d.Store.ListGames()
	if err != nil {
		d.Log.Log("error", fmt.Sprintf("snapshot everything: %v", err))
		return res
	}
	for _, g := range games {
		if _, err := d.Snapshots.Create(g.ID, comment, false); err != nil {
			res.Failed = append(res.Failed, struct {
				GameID string `json:"gameId"`
				Name   string `json:"name"`
				Error  string `json:"error"`
			}{g.ID, g.Name, err.Error()})
			continue
		}
		res.Taken++
	}
	msg := fmt.Sprintf("took a snapshot of %d game(s)", res.Taken)
	if len(res.Failed) > 0 {
		msg += fmt.Sprintf("; %d could not be snapshotted", len(res.Failed))
		d.Log.Log("warn", msg)
	} else {
		d.Log.Log("success", msg)
	}
	if d.OnGameChanged != nil {
		for _, g := range games {
			d.OnGameChanged(g.ID)
		}
	}
	return res
}
