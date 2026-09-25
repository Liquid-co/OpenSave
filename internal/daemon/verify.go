package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// Checking every snapshot can still be restored (see snapshot/verify.go):
// daily in the background, and on request from Settings or
// `opensave verify`.

const (
	// verifyFirst is how long after start the first check runs, out of the way
	// of everything a start does; verifyEvery how often after that.
	verifyFirst = 20 * time.Minute
	verifyEvery = 24 * time.Hour
	// verifyPace is a pause between snapshots, so a large history is read
	// back at a pace that does not get in the way of a game.
	verifyPace = 50 * time.Millisecond
)

// VerifyReport is what a check found.
type VerifyReport struct {
	Checked int                `json:"checked"`
	Damaged []DamagedSnapshot  `json:"damaged"`
	Summary store.CheckSummary `json:"summary"`
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
		_, _ = d.VerifySnapshots(ctx, "", verifyPace)
		select {
		case <-ctx.Done():
			return
		case <-time.After(verifyEvery):
		}
	}
}
