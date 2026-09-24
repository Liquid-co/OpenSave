package daemon

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/opensave/opensave/internal/syncpause"
)

// Pausing syncing, for this device. See package syncpause for what a pause
// covers; this file is the part that belongs to the daemon — the cloud
// backup, which the P2P engine does not know about, and the catching up when
// a pause ends.
//
// Snapshots are taken throughout. A snapshot made while paused would normally
// be copied to the cloud straight away; instead it is held here and copied
// when the pause ends, so pausing on a metered connection means nothing goes
// out, and nothing is missing from the cloud afterwards either.

type heldUpload struct {
	zipPath    string
	remoteName string
}

// PauseSync pauses syncing for d, or until the app restarts when d is zero.
func (d *Daemon) PauseSync(dur time.Duration) syncpause.Status {
	st := d.P2P.Pause.Pause(dur)
	if st.UntilRestart {
		d.Log.Log("info", "syncing paused until it is resumed or OpenSave restarts — snapshots are still taken")
	} else {
		d.Log.Log("info", fmt.Sprintf("syncing paused for %s — snapshots are still taken", dur.Round(time.Minute)))
	}
	return st
}

// ResumeSync ends a pause; the catching up runs in the background. Reports
// whether there was a pause to end.
func (d *Daemon) ResumeSync() bool {
	return d.P2P.Pause.Resume()
}

// SyncPauseStatus reports the pause as it stands.
func (d *Daemon) SyncPauseStatus() syncpause.Status {
	return d.P2P.Pause.Status()
}

// holdUpload keeps a snapshot's cloud copy for when syncing resumes.
func (d *Daemon) holdUpload(zipPath, remoteName string) {
	d.heldMu.Lock()
	defer d.heldMu.Unlock()
	d.held = append(d.held, heldUpload{zipPath: zipPath, remoteName: remoteName})
}

// catchUpAfterPause runs when a pause ends: the cloud copies held back, a
// sync of every game with every device, and a look at the cloud for newer
// saves — everything the pause stopped from happening as it came up.
func (d *Daemon) catchUpAfterPause() {
	d.heldMu.Lock()
	held := d.held
	d.held = nil
	d.heldMu.Unlock()

	sent := 0
	for _, h := range held {
		// A snapshot pruned while paused has nothing left to send.
		if _, err := os.Stat(h.zipPath); err != nil {
			continue
		}
		d.uploads.Add(1)
		go d.runCloudUpload(h.zipPath, h.remoteName, d.Log)
		sent++
	}
	if sent > 0 {
		d.Log.Log("info", fmt.Sprintf("syncing resumed; sending %d snapshot(s) held back from the cloud while paused", sent))
	} else {
		d.Log.Log("info", "syncing resumed")
	}

	d.P2P.GoSync(func(ctx context.Context) {
		d.P2P.PingPairedPeers(ctx)
		d.P2P.SyncAllGames(ctx)
	})
	d.P2P.GoSync(func(context.Context) { d.CheckCloud() })
}
