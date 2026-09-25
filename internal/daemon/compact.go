package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/opensave/opensave/internal/snapshot"
)

// Older snapshots sharing the files they have in common (see
// snapshot/shared.go): every few hours in the background, and on request
// from Settings → Storage or `opensave storage --compact`.

const (
	// compactFirst is how long after start the first pass runs, out of the
	// way of everything a start does; compactEvery how often after that.
	compactFirst = 15 * time.Minute
	compactEvery = 3 * time.Hour
	// compactPace is a pause between snapshots, as for the daily check.
	compactPace = 50 * time.Millisecond
)

// CompactSnapshots has every snapshot that can share its large files with
// others do so. One pass at a time: a request while the background pass runs
// waits for it, and then has little left to do.
func (d *Daemon) CompactSnapshots(ctx context.Context, pace time.Duration) (snapshot.CompactResult, error) {
	d.compactMu.Lock()
	defer d.compactMu.Unlock()
	res, err := d.Snapshots.CompactAll(ctx, snapshot.CompactMinAge, pace)
	if res.Compacted > 0 {
		d.Log.Log("info", fmt.Sprintf("%d older snapshot(s) now share the files they have in common, freeing %.1f MB",
			res.Compacted, float64(res.Freed)/(1<<20)))
	}
	return res, err
}

func (d *Daemon) runCompact(ctx context.Context) {
	first := time.NewTimer(compactFirst)
	defer first.Stop()
	select {
	case <-ctx.Done():
		return
	case <-first.C:
	}
	for {
		_, _ = d.CompactSnapshots(ctx, compactPace)
		select {
		case <-ctx.Done():
			return
		case <-time.After(compactEvery):
		}
	}
}
