package syncengine

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/ignore"
	"github.com/opensave/opensave/internal/store"
)

// adoptPeerRoot applies an explicit keep-remote choice to one named save
// location. Ordinary sync is still governed by ComputeWithDeletions.
//
// This is not a filesystem transaction: if an I/O operation fails after a
// partial apply, the protective snapshot is the undo and conflict stays open.
func (e *Engine) adoptPeerRoot(ctx context.Context, gameID string, game store.Game, peer Peer,
	root syncRoot, before delta.Manifest, remoteData ManifestResponse) error {
	remoteRoot, ok := remoteData.Manifest.Extra[root.Name]
	if !ok || remoteData.Proto < ProtoMultiRoot {
		return fmt.Errorf("peer no longer exposes the %q save location", root.Name)
	}
	rules := e.rulesFor(gameID)
	local := filterManifest(before, rules)
	remote := filterManifest(delta.Manifest{Files: remoteRoot.Files, Dirs: remoteRoot.Dirs}, rules)
	decision := Decision{}
	for p, f := range remote.Files {
		if !delta.IsSafePath(root.Path, p) {
			return fmt.Errorf("unsafe remote file path %q", p)
		}
		if old, has := local.Files[p]; !has || old.Hash != f.Hash {
			decision.FilesToPull = append(decision.FilesToPull, p)
		}
	}
	for p := range local.Files {
		if !delta.IsSafePath(root.Path, p) {
			return fmt.Errorf("unsafe local file path %q", p)
		}
		if _, has := remote.Files[p]; !has {
			decision.FilesToDeleteLocally = append(decision.FilesToDeleteLocally, p)
		}
	}
	for _, p := range remote.Dirs {
		if !delta.IsSafePath(root.Path, p) {
			return fmt.Errorf("unsafe remote directory path %q", p)
		}
		decision.DirsToPull = append(decision.DirsToPull, p)
	}
	for _, p := range local.Dirs {
		if !delta.IsSafePath(root.Path, p) {
			return fmt.Errorf("unsafe local directory path %q", p)
		}
		// rmdir only removes empty directories, so a directory that still
		// holds an excluded file is left in place on purpose. It is not part
		// of the adopted manifest.
		if dirKeepsDeviceLocal(p, before, rules, root.Path) {
			continue
		}
		found := false
		for _, rd := range remote.Dirs {
			if rd == p {
				found = true
				break
			}
		}
		if !found {
			decision.DirsToDeleteLocally = append(decision.DirsToDeleteLocally, p)
		}
	}
	sort.Strings(decision.FilesToPull)
	sort.Strings(decision.FilesToDeleteLocally)
	if err := ctx.Err(); err != nil {
		return err
	}

	// Writing excludes readers of this game, not other writers: holds nest on
	// purpose (settle.go). Same-root sync does not apply this location while
	// its conflict is still registered. The checks below are what reject a
	// location that changed anyway; they are not a writer mutex.
	apply := func() error {
		done := e.Writing(gameID)
		defer done()
		delta.InvalidateRoot(root.Path)
		current, err := delta.BuildManifest(root.Path)
		if err != nil {
			return err
		}
		if current.ManifestHash() != before.ManifestHash() {
			return fmt.Errorf("%q location changed after protective snapshot; retry resolution", root.Name)
		}
		// No deletion or update ever reaches the chosen peer or primary root.
		e.createPulledDirsIn(gameID, root, decision.DirsToPull, pullAdoptRemote)
		if len(decision.FilesToPull) > 0 {
			rootResp := ManifestResponse{Manifest: remote, Proto: remoteData.Proto,
				ActiveBranch: remoteData.ActiveBranch}
			if err := e.pullFiles(ctx, peer, gameID, game, root, local, rootResp,
				decision.FilesToPull, pullAdoptRemote); err != nil {
				return fmt.Errorf("pull selected peer version: %w", err)
			}
		}
		e.applyLocalDeletions(gameID, root, decision)
		// Normal pull and deletion helpers may skip files. Never claim
		// success when the resulting filtered contents differ from choice.
		delta.InvalidateRoot(root.Path)
		verified, err := delta.BuildManifest(root.Path)
		if err != nil {
			return fmt.Errorf("verify adopted location: %w", err)
		}
		if err := adoptionApplied(verified, remote, rules, root.Path); err != nil {
			return fmt.Errorf("remote selection not fully applied to %q: %w", root.Name, err)
		}
		return ctx.Err()
	}
	if err := apply(); err != nil {
		return err
	}

	// Never hold the local write gate during this network round trip.
	again, err := e.Transport.FetchManifest(ctx, peer, gameID, ManifestQuery{
		Name: game.Name, SavePath: game.SavePath, AppID: game.AppID, CoverURL: game.CoverURL,
	})
	if err != nil {
		return fmt.Errorf("verify chosen peer is unchanged: %w", err)
	}
	r, ok := again.Manifest.Extra[root.Name]
	if !ok || again.Proto < ProtoMultiRoot {
		return fmt.Errorf("peer changed %q while its version was being adopted; conflict retained", root.Name)
	}
	againFiltered := filterManifest(delta.Manifest{Files: r.Files, Dirs: r.Dirs}, rules)
	if err := adoptionApplied(againFiltered, remote, rules, ""); err != nil {
		return fmt.Errorf("peer changed %q while its version was being adopted; conflict retained: %w", root.Name, err)
	}

	// Recheck under the write gate before committing one atomic metadata
	// statement. The conflict remains active until both states are verified.
	commit := func() error {
		done := e.Writing(gameID)
		defer done()
		delta.InvalidateRoot(root.Path)
		verified, err := delta.BuildManifest(root.Path)
		if err != nil {
			return err
		}
		if err := adoptionApplied(verified, remote, rules, root.Path); err != nil {
			return fmt.Errorf("%q changed again before recording remote adoption: %w", root.Name, err)
		}
		files, dirs := IntersectLineage(remote, remote)
		return e.Store.SetResolvedRootSyncState(gameID, peer.ID, root.Name,
			remote.RootHash(delta.PrimaryRoot), files, dirs)
	}
	if err := commit(); err != nil {
		return fmt.Errorf("persist adopted root agreement: %w", err)
	}
	e.clearRootConflict(gameID, root.Name)
	if len(decision.FilesToPull) > 0 {
		e.Transport.ReportSyncEvent(peer, gameID, "sync-complete", map[string]any{
			"peerName": e.deviceName(), "direction": "upload", "pulledFiles": decision.FilesToPull, "root": root.Name,
		})
		if e.Progress.OnSyncComplete != nil {
			e.Progress.OnSyncComplete(gameID, ProgressEvent{PeerName: peer.Name, Direction: "download"})
		}
	}
	e.Log("info", fmt.Sprintf("the %q save location of %q resolved: adopted %s's verified version", root.Name, game.Name, peer.Name))
	return nil
}

// adoptionApplied reports whether the syncable contents of got are the
// selected remote manifest. A directory kept only because it still holds an
// excluded or never-synced file is not a disagreement: removing it would
// delete the device-local file, and treating it as a hash mismatch made
// keep-remote fail after it had already applied the peer's files.
func adoptionApplied(got, remote delta.Manifest, rules ignore.Rules, rootPath string) error {
	filtered := filterManifest(got, rules)
	if manifestFilesHash(filtered) != manifestFilesHash(remote) {
		return fmt.Errorf("files differ (got %s, expected %s)", manifestFilesHash(filtered), manifestFilesHash(remote))
	}
	have := map[string]struct{}{}
	for _, d := range filtered.Dirs {
		have[d] = struct{}{}
	}
	for _, d := range remote.Dirs {
		if _, ok := have[d]; !ok {
			return fmt.Errorf("remote directory %q missing", d)
		}
	}
	want := map[string]struct{}{}
	for _, d := range remote.Dirs {
		want[d] = struct{}{}
	}
	for _, d := range filtered.Dirs {
		if _, ok := want[d]; ok {
			continue
		}
		if dirKeepsDeviceLocal(d, got, rules, rootPath) {
			continue
		}
		return fmt.Errorf("local directory %q is not in the selected version", d)
	}
	return nil
}

func manifestFilesHash(m delta.Manifest) string {
	m.Dirs = nil
	return m.RootHash(delta.PrimaryRoot)
}

// dirKeepsExcluded reports whether dir, or something inside it, is excluded
// from sync. Those paths are absent from the filtered manifest, so the
// directory can look deletable when it is not.
func dirKeepsExcluded(dir string, full delta.Manifest, rules ignore.Rules) bool {
	prefix := dir + "/"
	for p := range full.Files {
		if strings.HasPrefix(p, prefix) && excluded(rules, p) {
			return true
		}
	}
	for _, d := range full.Dirs {
		if d != dir && !strings.HasPrefix(d, prefix) {
			continue
		}
		if excluded(rules, d) {
			return true
		}
	}
	return false
}

// dirKeepsDeviceLocal preserves entries intentionally omitted by the native
// manifest as well as excluded files. It never grants an exception to
// represented synced files.
// This is only consulted during explicit remote-root adoption, never ordinary
// bidirectional planning.
func dirKeepsDeviceLocal(dir string, full delta.Manifest, rules ignore.Rules, rootPath string) bool {
	if dirKeepsExcluded(dir, full, rules) {
		return true
	}
	if rootPath == "" {
		return false
	}
	actual := delta.LocalNameFor(rootPath, dir)
	start, err := filepath.Rel(rootPath, actual)
	if err != nil {
		return false
	}
	start = filepath.ToSlash(start)
	found := false
	err = fs.WalkDir(os.DirFS(rootPath), start, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		logical := delta.NormalizeRelPath(filepath.ToSlash(p))
		if logical == dir {
			return nil
		}
		if entry.IsDir() {
			for _, d := range full.Dirs {
				if d == logical {
					return nil
				}
			}
			found = true
			return fs.SkipAll
		}
		if _, represented := full.Files[logical]; !represented {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found && err == nil
}
