package daemon

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
)

// Snapshots found damaged when read back (verify.go): getting them back, or
// letting them go.
//
// An archive deleted or corrupted outside OpenSave is gone from this device,
// but not necessarily from everywhere. With cloud backup on, every snapshot
// was uploaded as it was taken, and a copy that reads back whole there is the
// same snapshot: it is fetched, checked, and put where the archive was. What
// cannot be got back can be removed from the history — its record promises a
// save that no longer exists, and the rest of each game's history is
// unaffected.

// RepairReport is what an attempt to get damaged snapshots back did.
type RepairReport struct {
	// Repaired were put back from their cloud copies.
	Repaired []DamagedSnapshot `json:"repaired"`
	// Remaining still cannot be restored: no cloud copy, or not a whole one.
	Remaining []DamagedSnapshot `json:"remaining"`
	// CloudChecked is whether the cloud was looked at; CloudError why not,
	// when it could not be.
	CloudChecked bool   `json:"cloudChecked"`
	CloudError   string `json:"cloudError,omitempty"`
}

// damagedSnapshots is every snapshot last found damaged, with its game's name.
func (d *Daemon) damagedSnapshots() ([]store.Snapshot, map[string]string, error) {
	games, err := d.Store.ListGames()
	if err != nil {
		return nil, nil, err
	}
	names := map[string]string{}
	var out []store.Snapshot
	for _, g := range games {
		names[g.ID] = g.Name
		branches, _ := d.Store.ListBranches(g.ID)
		for _, b := range branches {
			snaps, _ := d.Store.ListSnapshots(g.ID, b)
			for _, s := range snaps {
				if s.Problem != "" {
					out = append(out, s)
				}
			}
		}
	}
	return out, names, nil
}

func damagedEntry(s store.Snapshot, names map[string]string) DamagedSnapshot {
	return DamagedSnapshot{GameID: s.GameID, GameName: names[s.GameID], SnapshotID: s.ID, Timestamp: s.Timestamp, Problem: s.Problem}
}

// RepairSnapshots puts back, from the cloud, every damaged snapshot that has
// a whole copy there.
func (d *Daemon) RepairSnapshots(ctx context.Context) (RepairReport, error) {
	report := RepairReport{Repaired: []DamagedSnapshot{}, Remaining: []DamagedSnapshot{}}
	damaged, names, err := d.damagedSnapshots()
	if err != nil || len(damaged) == 0 {
		return report, err
	}

	// Snapshot ids are unique, so a copy is found by its id whatever game or
	// branch it was uploaded under — a snapshot moved by linking two games
	// kept its id and may still carry its old names in the cloud.
	inCloud := map[string]string{}
	if cfg, cErr := d.Store.GetCloudConfig(); cErr == nil && cfg.Enabled && cloudListable(cfg.Provider) {
		files, lErr := d.Cloud.List()
		if lErr != nil {
			report.CloudError = lErr.Error()
		} else {
			report.CloudChecked = true
			for _, f := range files {
				if _, _, id, ok := snapshot.ParseExportEntryName(f.Name); ok {
					inCloud[id] = f.Name
				}
			}
		}
	} else {
		report.CloudError = "cloud backup is not set up"
	}

	for _, snap := range damaged {
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		entry := damagedEntry(snap, names)
		name, ok := inCloud[snap.ID]
		if !ok {
			report.Remaining = append(report.Remaining, entry)
			continue
		}
		if err := d.fetchSnapshotCopy(snap, name); err != nil {
			entry.Problem = fmt.Sprintf("%s — and its cloud copy could not be used: %v", snap.Problem, err)
			report.Remaining = append(report.Remaining, entry)
			continue
		}
		report.Repaired = append(report.Repaired, entry)
		d.Log.Log("success", fmt.Sprintf("put back the snapshot of %q from %s from its cloud copy", names[snap.GameID], snap.Timestamp))
	}
	if len(report.Repaired) > 0 && d.OnGameChanged != nil {
		d.OnGameChanged("")
	}
	return report, nil
}

// fetchSnapshotCopy downloads a snapshot's cloud copy, checks it reads back
// whole, and puts it where the snapshot's archive belongs.
func (d *Daemon) fetchSnapshotCopy(snap store.Snapshot, cloudName string) error {
	tmp, err := os.CreateTemp("", "opensave-repair-*.zip")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)
	if err := d.Cloud.Download(cloudName, tmpPath); err != nil {
		return err
	}
	if err := snapshot.VerifyArchive(tmpPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(snap.ZipPath), 0o777); err != nil {
		return err
	}
	if err := copyFile(tmpPath, snap.ZipPath); err != nil {
		return err
	}
	return d.Store.SetSnapshotCheck(snap.ID, time.Now().UnixMilli(), "")
}

// copyFile puts src at dst whole or not at all. The part-written copy is
// named as the snapshot store's own are (.incoming-*), so one a crash leaves
// behind is swept up with theirs.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(dst), ".incoming-*")
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(out.Name(), dst)
	}
	if err != nil {
		os.Remove(out.Name())
	}
	return err
}

// ForgetDamagedSnapshots removes from the history every snapshot that cannot
// be restored. Nothing restorable is touched: only records whose archive was
// found damaged or missing.
func (d *Daemon) ForgetDamagedSnapshots() (removed []DamagedSnapshot, err error) {
	damaged, names, err := d.damagedSnapshots()
	if err != nil {
		return nil, err
	}
	removed = []DamagedSnapshot{}
	for _, snap := range damaged {
		if _, err := d.Snapshots.DeleteSnapshot(snap.GameID, snap.ID); err != nil {
			d.Log.Log("warn", fmt.Sprintf("could not remove the damaged snapshot %s of %q: %v", snap.ID, names[snap.GameID], err))
			continue
		}
		removed = append(removed, damagedEntry(snap, names))
	}
	if len(removed) > 0 {
		games := map[string]bool{}
		for _, r := range removed {
			games[names[r.GameID]] = true
		}
		list := make([]string, 0, len(games))
		for n := range games {
			list = append(list, fmt.Sprintf("%q", n))
		}
		d.Log.Log("info", fmt.Sprintf("removed %d snapshot(s) that could not be restored, of %s", len(removed), strings.Join(list, ", ")))
		if d.OnGameChanged != nil {
			d.OnGameChanged("")
		}
	}
	return removed, nil
}
