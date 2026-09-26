package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// Checking every snapshot can still be restored (see snapshot/verify.go):
// in the background as often as Settings says — once a week unless changed —
// and on request from Settings or `opensave verify`. One found damaged is put
// back from its cloud copy straight away when there is one (repair.go).

const (
	// verifyFirst is how long after start the first look at the schedule
	// happens, out of the way of everything a start does; verifyLook how
	// often after that. A check runs when one is due, not at each look.
	verifyFirst = 20 * time.Minute
	verifyLook  = time.Hour
	// verifyPace is a pause between snapshots, so a large history is read
	// back at a pace that does not get in the way of a game.
	verifyPace = 50 * time.Millisecond
)

// VerifyReport is what a check found.
type VerifyReport struct {
	Checked int                `json:"checked"`
	Damaged []DamagedSnapshot  `json:"damaged"`
	Summary store.CheckSummary `json:"summary"`
	// Repaired is how many of the damaged were put back from the cloud as
	// part of the check; Damaged lists only those still damaged after.
	Repaired int `json:"repaired"`
}

// DamagedSnapshot is one snapshot that cannot be restored, and why.
type DamagedSnapshot struct {
	GameID     string `json:"gameId"`
	GameName   string `json:"gameName"`
	SnapshotID string `json:"snapshotId"`
	Timestamp  string `json:"timestamp"`
	Problem    string `json:"problem"`
}

// VerifySnapshots reads back every snapshot of one game, or of every game
// when gameID is empty, and records what it finds.
func (d *Daemon) VerifySnapshots(ctx context.Context, gameID string, pace time.Duration) (VerifyReport, error) {
	report := VerifyReport{Damaged: []DamagedSnapshot{}}
	games, err := d.Store.ListGames()
	if err != nil {
		return report, err
	}
	for _, g := range games {
		if gameID != "" && g.ID != gameID {
			continue
		}
		branches, _ := d.Store.ListBranches(g.ID)
		for _, b := range branches {
			snaps, _ := d.Store.ListSnapshots(g.ID, b)
			for _, snap := range snaps {
				if ctx.Err() != nil {
					return report, ctx.Err()
				}
				was := snap.Problem
				if err := d.Snapshots.Verify(snap); err != nil {
					report.Damaged = append(report.Damaged, DamagedSnapshot{
						GameID: g.ID, GameName: g.Name, SnapshotID: snap.ID, Timestamp: snap.Timestamp, Problem: err.Error(),
					})
					if was == "" {
						d.Log.Log("error", fmt.Sprintf("a snapshot of %q from %s cannot be restored: %v", g.Name, snap.Timestamp, err))
					}
				}
				report.Checked++
				if pace > 0 {
					select {
					case <-ctx.Done():
						return report, ctx.Err()
					case <-time.After(pace):
					}
				}
			}
		}
	}
	// Anything damaged that the cloud still has whole goes back now, rather
	// than waiting to be noticed and asked for.
	if len(report.Damaged) > 0 {
		if repair, err := d.RepairSnapshots(ctx); err == nil && len(repair.Repaired) > 0 {
			report.Repaired = len(repair.Repaired)
			fixed := map[string]bool{}
			for _, r := range repair.Repaired {
				fixed[r.SnapshotID] = true
			}
			still := report.Damaged[:0]
			for _, dmg := range report.Damaged {
				if !fixed[dmg.SnapshotID] {
					still = append(still, dmg)
				}
			}
			report.Damaged = still
		}
	}
	if gameID == "" {
		_ = d.Store.SetLastVerify(time.Now().UnixMilli())
	}
	report.Summary, _ = d.Store.SnapshotChecks()
	if len(report.Damaged) == 0 {
		d.Log.Log("info", fmt.Sprintf("checked %d snapshot(s): every one can be restored", report.Checked))
	}
	if d.OnGameChanged != nil {
		d.OnGameChanged(gameID)
	}
	return report, nil
}

func (d *Daemon) runVerify(ctx context.Context) {
	first := time.NewTimer(verifyFirst)
	defer first.Stop()
	select {
	case <-ctx.Done():
		return
	case <-first.C:
	}
	for {
		if verifyDue(d.Store, time.Now()) {
			_, _ = d.VerifySnapshots(ctx, "", verifyPace)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(verifyLook):
		}
	}
}

// verifyDue says whether the scheduled check should run now: it is on, and
// the last full check finished that many days ago or more.
func verifyDue(s *store.Store, now time.Time) bool {
	settings, err := s.GetSettings()
	if err != nil || settings.VerifyEveryDays <= 0 {
		return false
	}
	every := time.Duration(settings.VerifyEveryDays) * 24 * time.Hour
	return now.Sub(time.UnixMilli(settings.LastVerifyMs)) >= every
}
