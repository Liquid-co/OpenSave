package syncengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/opensave/opensave/internal/delta"
	"github.com/opensave/opensave/internal/ignore"
	"github.com/opensave/opensave/internal/owntouch"
	"github.com/opensave/opensave/internal/store"
)

// Save versions.
//
// Every device knows which version of a game's save it holds, as a version
// vector: one counter per device that has ever changed the save, moved only
// when the save really changes there (the watcher saw the game or the user
// write it — never a sync). Comparing two vectors answers, exactly, the
// question the file-by-file comparison could only guess at from what each side
// happens to hold:
//
//   - one is older than the other: that device is simply behind. It takes the
//     newer save as a whole — every file, and every deletion — and only from a
//     device that holds it completely. Nothing is ever taken from it, pushed
//     by it, or deleted on its say-so, and it is a source for nobody.
//   - equal: the same save.
//   - neither: both changed it independently. Only then is anyone asked.
//
// A device also remembers the newest version it has heard of (the target)
// until it holds it. That is what keeps two outdated devices from syncing with
// each other: each knows something newer exists, so each waits for a device
// that holds it, instead of reading the other's half-finished copy as the save
// and passing on what it lacks as deletions.
//
// An empty folder holds no version at all, which is older than every version,
// so a new or emptied device fills itself from a complete one and never raises
// a conflict. A save that existed before this was introduced gets a version
// named after its content (genesisKey), so devices that already held the same
// save agree at once, and ones that did not are compared the way they always
// were, until they agree.

// VersionVector is writer key → counter.
type VersionVector map[string]int64

type versionOrder int

const (
	versionEqual versionOrder = iota
	versionOlder
	versionNewer
	versionConcurrent
)

// genesisAll is the entry "Use this save everywhere" adds (UseSaveEverywhere):
// a version holding it counts as having every save from before versions
// existed behind it, whatever its content was named.
const genesisAll = "g:*"

// holds reports whether v includes entry k at counter n.
func holds(v VersionVector, k string, n int64) bool {
	if v[k] >= n {
		return true
	}
	return k != genesisAll && strings.HasPrefix(k, "g:") && v[genesisAll] >= 1
}

// compareVersions says how a relates to b.
func compareVersions(a, b VersionVector) versionOrder {
	aBehind, bBehind := false, false
	for k, bv := range b {
		if !holds(a, k, bv) {
			aBehind = true
			break
		}
	}
	for k, av := range a {
		if !holds(b, k, av) {
			bBehind = true
			break
		}
	}
	switch {
	case !aBehind && !bBehind:
		return versionEqual
	case aBehind && !bBehind:
		return versionOlder
	case !aBehind && bBehind:
		return versionNewer
	default:
		return versionConcurrent
	}
}

// covers reports whether a is b or newer.
func covers(a, b VersionVector) bool {
	o := compareVersions(a, b)
	return o == versionEqual || o == versionNewer
}

func mergeVersions(a, b VersionVector) VersionVector {
	out := make(VersionVector, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if v > out[k] {
			out[k] = v
		}
	}
	return out
}

func (v VersionVector) clone() VersionVector { return mergeVersions(v, nil) }

// genesisOnly reports whether every entry names a save that predates version
// tracking (genesisKey), i.e. nobody has changed it since.
func (v VersionVector) genesisOnly() bool {
	if len(v) == 0 {
		return false
	}
	for k := range v {
		if !strings.HasPrefix(k, "g:") {
			return false
		}
	}
	return true
}

// String is a short, stable form for the log.
func (v VersionVector) String() string {
	if len(v) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		short := k
		if len(short) > 8 {
			short = short[:8]
		}
		parts[i] = fmt.Sprintf("%s=%d", short, v[k])
	}
	return strings.Join(parts, ",")
}

// genesisKey names the version of a save that existed before versions did,
// after its content: devices that hold the same files get the same name.
// Directories are left out — they are created by any sync, and are not what
// makes two saves the same.
func genesisKey(m delta.Manifest) string {
	return "g:" + filesKey(m, ignore.Rules{})[:16]
}

// filesKey hashes which files a save holds and what is in them: paths and
// content hashes, nothing else — not folders, not times. Excluded paths are
// left out.
func filesKey(m delta.Manifest, rules ignore.Rules) string {
	return filesKeyOf(m, func(p string) bool { return excluded(rules, p) })
}

func filesKeyOf(m delta.Manifest, leaveOut func(string) bool) string {
	paths := make([]string, 0, len(m.Files))
	for p := range m.Files {
		if !leaveOut(p) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write([]byte(m.Files[p].Hash))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// versionHashOf is what is recorded with a version: the save's files as this
// device syncs them, tagged with the exclusion rules it was taken under. A
// save that no longer hashes to it was changed here since, whether or not the
// watcher said so.
//
// Filtered, because a file nobody syncs — a device's own config — changing
// is not a new version of the save. Tagged, because writing a rule is not one
// either: it changes the filtered hash on every device at once, and each took
// that as a change of its own, so the next sync asked about a divergence
// nobody made. A hash taken under other rules is re-taken, not compared
// (sameSaveAs). The files no save is made of (delta.NeverSyncedList) are in
// the tag for the same reason.
func (e *Engine) versionHashOf(gameID string, primary delta.Manifest) string {
	text := ""
	if game, err := e.Store.GetGame(gameID); err == nil {
		text = e.IgnoreText(game)
	}
	return versionHashUnder(text, primary)
}

// versionHashUnder is versionHashOf under a given exclusion list.
func versionHashUnder(text string, primary delta.Manifest) string {
	tag := sha256.Sum256([]byte(text + "\x00" + delta.NeverSyncedList))
	return hex.EncodeToString(tag[:4]) + ":" + filesKey(primary, ignore.Parse(text))
}

// versionHashBeforeNeverSynced is versionHashOf as builds before
// delta.NeverSynced took it: the rules alone in the tag, those files counted.
func versionHashBeforeNeverSynced(rulesText string, primary delta.Manifest) string {
	tag := sha256.Sum256([]byte(rulesText))
	rules := ignore.Parse(rulesText)
	return hex.EncodeToString(tag[:4]) + ":" + filesKeyOf(primary, func(p string) bool {
		return !rules.Empty() && rules.Match(p)
	})
}

// AdoptExclusionView re-takes the hash recorded with this device's version of
// a game under what it is excluded by now (IgnoreText), when the save is the
// one it was recorded for under oldText — the files excluded then or now
// aside. Run when the exclusions change without anyone writing a rule (a new
// build's delta.NeverSyncedList, the game database naming settings files),
// before anything compares: otherwise the only difference, which files are
// counted, reads as the save changed here, and every device names the same
// save a new version of its own at once.
//
// beforeNeverSynced says the old hash was taken by a build that predates
// delta.NeverSynced, and counted those files.
func (e *Engine) AdoptExclusionView(gameID string, primary delta.Manifest, oldText string, beforeNeverSynced bool) {
	if _, err := e.Store.GetGame(gameID); err != nil {
		return
	}
	// The base agreed with each device, where it is still this save: the
	// two agreed on it, and the files now left out leave them agreeing on
	// the rest. One that no longer matches is a real change, left to count.
	oldRules := ignore.Parse(oldText)
	var oldView delta.Manifest
	if beforeNeverSynced && oldRules.Empty() {
		oldView = primary
	} else if beforeNeverSynced {
		oldView = filterManifestBy(primary, func(p string) bool { return !oldRules.Empty() && oldRules.Match(p) })
	} else {
		oldView = filterManifest(primary, oldRules)
	}
	if was, now := oldView.ManifestHash(), filterManifest(primary, e.rulesFor(gameID)).ManifestHash(); was != now {
		if peers, err := e.Store.ListPeers(); err == nil {
			for _, p := range peers {
				if e.Store.GetAgreedHash(gameID, p.ID) == was {
					_ = e.Store.SetAgreedHash(gameID, p.ID, now)
				}
			}
		}
	}

	e.versionMu.Lock()
	defer e.versionMu.Unlock()
	gv, err := e.loadVersionLocked(gameID)
	if err != nil || gv.rec.GameID == "" || gv.rec.Hash == "" {
		return
	}
	was := versionHashUnder(oldText, primary)
	if beforeNeverSynced {
		was = versionHashBeforeNeverSynced(oldText, primary)
	}
	if now := e.versionHashOf(gameID, primary); gv.rec.Hash == was && was != now {
		gv.rec.Hash = now
		_ = e.saveVersionLocked(gv)
	}
}

// sameSaveAs reports whether a hash recorded with a version still describes
// the save that now hashes to current: the same, or taken under different
// exclusion rules, which says nothing about the save.
func sameSaveAs(recorded, current string) bool {
	if recorded == current {
		return true
	}
	r, _, okR := strings.Cut(recorded, ":")
	c, _, okC := strings.Cut(current, ":")
	return okR && okC && r != c
}

// VersionInfo is a device's version of one game, as it travels in a manifest
// response. Absent from devices that predate it, which are synced the way they
// always were.
type VersionInfo struct {
	Vector VersionVector `json:"vector"`
	// Target is the newer version the device knows of and does not hold yet.
	Target VersionVector `json:"target,omitempty"`
	// HasFiles: the device holds at least one save file for the game.
	HasFiles bool `json:"hasFiles"`
	// Answered is the version someone on the device kept its own save over
	// in a conflict (Unix ms in AnsweredAt). Resolution is mutual: the device
	// holding that version is asked in turn, not overwritten.
	Answered   VersionVector `json:"answered,omitempty"`
	AnsweredAt int64         `json:"answeredAt,omitempty"`
}

// Outdated reports whether the device knows its save is behind another's.
func (v VersionInfo) Outdated() bool {
	return len(v.Target) > 0 && compareVersions(v.Vector, v.Target) == versionOlder
}

// gameVersion is the stored record with its vectors decoded.
type gameVersion struct {
	rec      store.GameVersion
	vec      VersionVector
	target   VersionVector
	answered VersionVector
}

func (e *Engine) loadVersionLocked(gameID string) (gameVersion, error) {
	rec, err := e.Store.GetGameVersion(gameID)
	if err != nil {
		return gameVersion{}, err
	}
	gv := gameVersion{rec: rec, vec: VersionVector{}}
	_ = json.Unmarshal([]byte(rec.Vector), &gv.vec)
	if gv.vec == nil {
		gv.vec = VersionVector{}
	}
	if rec.Target != "" {
		_ = json.Unmarshal([]byte(rec.Target), &gv.target)
	}
	if rec.Answered != "" {
		_ = json.Unmarshal([]byte(rec.Answered), &gv.answered)
	}
	return gv, nil
}

func (e *Engine) saveVersionLocked(gv gameVersion) error {
	if gv.rec.GameID == "" {
		return nil
	}
	raw, _ := json.Marshal(gv.vec)
	gv.rec.Vector = string(raw)
	gv.rec.Target = ""
	if len(gv.target) > 0 {
		t, _ := json.Marshal(gv.target)
		gv.rec.Target = string(t)
	}
	gv.rec.Answered = ""
	if len(gv.answered) > 0 {
		a, _ := json.Marshal(gv.answered)
		gv.rec.Answered = string(a)
	} else {
		gv.rec.AnsweredAt = 0
	}
	return e.Store.SaveGameVersion(gv.rec)
}

// ensureGenesisLocked names a save that has files and no version yet. Not
// while the device is taking a newer version: its folder is then a part-way
// copy of someone else's, and naming that would make it a version of its own.
func (gv *gameVersion) ensureGenesis(local delta.Manifest, hash string) bool {
	if len(gv.vec) > 0 || len(local.Files) == 0 || len(gv.target) > 0 || gv.rec.Pulling {
		return false
	}
	gv.vec = VersionVector{genesisKey(local): 1}
	gv.rec.Hash = hash
	return true
}

// refreshLocked brings this device's version up to date with its save before
// anyone compares it: a save that predates versions is named, and one that no
// longer hashes to what its version recorded was changed here — by the game,
// whether or not the watcher has said so yet (it waits for the game to finish
// writing, and while a game holds its files open that can be a long time). It
// is a version of its own from now. Without this, a device whose change was
// still unrecorded looked merely behind, and took the other's save over it.
//
// Not while a pull towards a newer version is unfinished: the folder is then a
// part-way copy of someone else's save, not a change made here. Reports
// whether the record changed.
func (e *Engine) refreshLocked(gv *gameVersion, game store.Game, local delta.Manifest) bool {
	hash := e.versionHashOf(game.ID, local)
	if gv.ensureGenesis(local, hash) {
		return true
	}
	if gv.rec.Pulling || len(gv.vec) == 0 || gv.rec.Hash == "" || gv.rec.Hash == hash {
		return false
	}
	if sameSaveAs(gv.rec.Hash, hash) {
		// Taken under other exclusion rules, so it cannot say whether the
		// save changed: re-taken under the new ones. A change made at the
		// same time still becomes a version — the watcher reports it
		// (NoteLocalChange).
		gv.rec.Hash = hash
		return true
	}
	e.bumpLocked(gv, game.Name, hash)
	return true
}

// learn takes in what a peer knows: a version newer than this device's
// becomes its target, unless it already knows of one newer still.
func (gv *gameVersion) learn(theirs VersionInfo) {
	for _, c := range []VersionVector{theirs.Vector, theirs.Target} {
		if len(c) == 0 || compareVersions(gv.vec, c) != versionOlder {
			continue
		}
		if len(gv.target) == 0 || compareVersions(gv.target, c) == versionOlder {
			gv.target = c.clone()
		}
	}
	gv.dropReachedTarget()
}

// dropReachedTarget forgets a target this device's version covers — or has
// gone past it sideways, by changing on its own: that is no longer "behind",
// it is a conflict, and the version comparison says so by itself.
func (gv *gameVersion) dropReachedTarget() {
	if len(gv.target) > 0 && compareVersions(gv.vec, gv.target) != versionOlder {
		gv.target = nil
	}
}

func (gv gameVersion) info(hasFiles bool) VersionInfo {
	v := VersionInfo{Vector: gv.vec.clone(), Target: gv.target.clone(), HasFiles: hasFiles}
	if len(gv.answered) > 0 {
		v.Answered, v.AnsweredAt = gv.answered.clone(), gv.rec.AnsweredAt
	}
	return v
}

// adopt makes this device hold theirs: their version, and their answer to a
// conflict if they carry one — the device holding the version they kept
// theirs over is still to be asked, whoever it meets.
func (gv *gameVersion) adopt(vec VersionVector, theirs VersionInfo) {
	gv.vec = vec.clone()
	gv.answered, gv.rec.AnsweredAt = nil, 0
	if len(theirs.Answered) > 0 {
		gv.answered, gv.rec.AnsweredAt = theirs.Answered.clone(), theirs.AnsweredAt
	}
	gv.dropReachedTarget()
}

// LocalVersion is this device's version of a game, for a manifest response.
// manifest is the save being served; a save that predates versions is named
// here if it has not been yet. Nil when the game takes no part in versions:
// one with automatic sync turned off is not watched, so nothing would ever
// move its version — the peer then syncs it the way it always has.
func (e *Engine) LocalVersion(game store.Game, manifest delta.Manifest) *VersionInfo {
	if !game.AutoSync {
		return nil
	}
	e.versionMu.Lock()
	defer e.versionMu.Unlock()
	gv, err := e.loadVersionLocked(game.ID)
	if err != nil {
		return nil
	}
	if e.refreshLocked(&gv, game, manifest) {
		_ = e.saveVersionLocked(gv)
	}
	v := gv.info(len(manifest.Files) > 0)
	return &v
}

// NoteLocalChange moves this device's version on: the watcher saw the game
// or the user change the save (never a sync — those are told apart by
// internal/owntouch). Called before the change is offered to anyone.
func (e *Engine) NoteLocalChange(gameID string) {
	game, err := e.Store.GetGame(gameID)
	if err != nil || !game.AutoSync {
		return
	}
	e.versionMu.Lock()
	defer e.versionMu.Unlock()
	gv, err := e.loadVersionLocked(gameID)
	if err != nil || gv.rec.GameID == "" {
		return
	}
	if gv.rec.Pulling {
		// Files a pull towards a newer version wrote — this run's own, still
		// arriving, or left by a run that stopped part-way (after a restart
		// the watcher reports those as a change: it cannot know who wrote
		// them). Not a version: the folder is a part-way copy of someone
		// else's, and naming it would hand that copy to every other device.
		// The pull finishing is what moves the version. Anything written here
		// meanwhile is in the snapshot taken before the pull replaces it.
		return
	}
	m, err := delta.BuildManifest(game.SavePath)
	if err != nil {
		return
	}
	hash := e.versionHashOf(gameID, m)
	if len(gv.vec) == 0 {
		// First change ever seen here, on a save that predates versions or a
		// new one. Named after its content, so a device holding the same files
		// agrees with it rather than conflicting.
		if gv.ensureGenesis(m, hash) {
			_ = e.saveVersionLocked(gv)
		}
		return
	}
	if hash == gv.rec.Hash {
		// Already a version: a sync noticed it first (refreshLocked), or the
		// change was only to files nobody syncs.
		return
	}
	// The watcher saw the game or the user write the save: a version, even if
	// the exclusion rules changed since the last one (writing a rule raises
	// no file event, so that alone never arrives here).
	e.bumpLocked(&gv, game.Name, hash)
}

// bumpLocked gives this device a new version of its own, after hash.
func (e *Engine) bumpLocked(gv *gameVersion, name, hash string) {
	gv.rec.Counter++
	gv.vec = gv.vec.clone()
	gv.vec[gv.rec.Key] = gv.rec.Counter
	gv.rec.Hash = hash
	gv.dropReachedTarget()
	if err := e.saveVersionLocked(*gv); err == nil {
		e.Log("info", fmt.Sprintf("%s changed on this device — version %s", name, gv.vec))
	}
}

// versionAfterResolution records a conflict's answer as a version.
//
// Keeping the peer's save makes this device hold what both held: newer than
// both, so the peer finds nothing to take and devices behind either take it.
//
// Keeping this device's own is a new version of its own — but deliberately
// not one that includes the peer's: that would make the peer simply behind,
// and its save would be replaced without anyone there being asked. Resolution
// is mutual. The answer is recorded instead (Answered), so this device is not
// asked again about the same version, and the peer, still independent of it,
// asks in turn. Whoever answers last decides (syncByVersion).
func (e *Engine) versionAfterResolution(gameID string, theirs *VersionInfo, keptLocal bool) {
	if theirs == nil {
		return
	}
	e.versionMu.Lock()
	defer e.versionMu.Unlock()
	gv, err := e.loadVersionLocked(gameID)
	if err != nil || gv.rec.GameID == "" {
		return
	}
	if keptLocal {
		gv.rec.Counter++
		gv.vec = gv.vec.clone()
		gv.vec[gv.rec.Key] = gv.rec.Counter
		gv.answered, gv.rec.AnsweredAt = theirs.Vector.clone(), time.Now().UnixMilli()
	} else {
		gv.vec = mergeVersions(gv.vec, theirs.Vector)
		gv.answered, gv.rec.AnsweredAt = nil, 0
	}
	gv.rec.Pulling = false // whatever was being taken, the answer replaces it
	gv.rec.Hash = ""
	if game, err := e.Store.GetGame(gameID); err == nil {
		if m, err := delta.BuildManifest(game.SavePath); err == nil {
			gv.rec.Hash = e.versionHashOf(gameID, m)
		}
	}
	gv.dropReachedTarget()
	_ = e.saveVersionLocked(gv)
}

// discardLocalVersion is for a "keep theirs" that did not finish: this
// device's own version is given up, and it takes the peer's like any outdated
// device would — whole, and only from a device that holds it.
func (e *Engine) discardLocalVersion(gameID string, theirs *VersionInfo) {
	if theirs == nil {
		return
	}
	e.versionMu.Lock()
	defer e.versionMu.Unlock()
	gv, err := e.loadVersionLocked(gameID)
	if err != nil || gv.rec.GameID == "" {
		return
	}
	gv.vec = VersionVector{}
	gv.target = theirs.Vector.clone()
	gv.rec.Hash = ""
	_ = e.saveVersionLocked(gv)
}

// versionStatusWaiting: this device or the peer is behind, and the other
// cannot give it the newer version either. Nothing happens; both wait for a
// device that holds it.
const versionStatusWaiting = "waiting"

// syncByVersion decides a sync from the two devices' versions. handled false
// leaves it to the file comparison: the two hold the same version and the
// same files, or both saves predate versions and have not met since.
//
// local and remote are filtered by the exclusion rules; unfilteredLocal is
// this device's save as it is on disk, which names a version.
func (e *Engine) syncByVersion(ctx context.Context, game store.Game, peer Peer,
	local, unfilteredLocal delta.Manifest, remote ManifestResponse) (Result, bool, error) {

	gameID := game.ID
	theirs := *remote.Version

	e.versionMu.Lock()
	gv, err := e.loadVersionLocked(gameID)
	if err != nil || gv.rec.GameID == "" {
		e.versionMu.Unlock()
		return Result{}, false, nil
	}
	before := gv.target.clone()
	changed := e.refreshLocked(&gv, game, unfilteredLocal)
	gv.learn(theirs)
	if changed || compareVersions(before, gv.target) != versionEqual || len(before) != len(gv.target) {
		_ = e.saveVersionLocked(gv)
	}
	mine := gv.info(len(local.Files) > 0)
	e.versionMu.Unlock()

	waiting := func(why string) (Result, bool, error) {
		e.logOnce(gameID+"|"+peer.ID, why)
		return Result{Status: versionStatusWaiting, PeerID: peer.ID, PeerName: peer.Name}, true, nil
	}
	pushTo := func() (Result, bool, error) {
		// Once per state: a peer still taking it is asked again on every
		// pass until it has, and that is not news each time.
		e.logOnce(gameID+"|"+peer.ID, fmt.Sprintf("%q: %s has an older version (%s, this device %s) — asking it to take this one",
			game.Name, peer.Name, theirs.Vector, mine.Vector))
		e.Transport.TriggerPeerPull(peer, gameID)
		return Result{Status: "triggered_peer_pull", Direction: "push", PeerID: peer.ID, PeerName: peer.Name}, true, nil
	}

	switch {
	case mine.Outdated():
		if theirs.Outdated() || !covers(theirs.Vector, mine.Target) {
			return waiting(fmt.Sprintf("%q: this device is behind (holds %s, newest known %s); %s does not hold that version either — waiting for a device that does",
				game.Name, mine.Vector, mine.Target, peer.Name))
		}
		return e.pullVersion(ctx, game, peer, local, remote, theirs.Vector)
	case theirs.Outdated():
		if covers(mine.Vector, theirs.Target) {
			return pushTo()
		}
		return waiting(fmt.Sprintf("%q: %s is behind and this device does not hold the version it is waiting for either",
			game.Name, peer.Name))
	}

	switch compareVersions(mine.Vector, theirs.Vector) {
	case versionNewer:
		return pushTo()
	case versionOlder:
		// learn made this device outdated above, so this is only reached if
		// the record could not be read back; the peer holds the newer version
		// completely, so take it.
		return e.pullVersion(ctx, game, peer, local, remote, theirs.Vector)
	case versionEqual:
		e.forgetLogOnce(gameID + "|" + peer.ID)
		if sameFiles(local, remote.Manifest) {
			return Result{}, false, nil
		}
		// One version, different files. Each side checks its own save against
		// its version before answering (refreshLocked), so this is a change
		// that landed after the peer answered; its sync carries it.
		return waiting(fmt.Sprintf("%q: same version as %s but the files differ — waiting for the change to be recorded",
			game.Name, peer.Name))
	}

	// Concurrent: both changed independently.
	e.forgetLogOnce(gameID + "|" + peer.ID)
	if sameFiles(local, remote.Manifest) {
		// …into the same save. Both record the union, which each computes the
		// same way, and are equal from then on.
		e.versionMu.Lock()
		if gv, err := e.loadVersionLocked(gameID); err == nil {
			gv.vec = mergeVersions(gv.vec, theirs.Vector)
			gv.rec.Hash = e.versionHashOf(gameID, unfilteredLocal)
			gv.dropReachedTarget()
			_ = e.saveVersionLocked(gv)
		}
		e.versionMu.Unlock()
		return Result{}, false, nil
	}
	if !mine.HasFiles {
		// An empty folder is never one side of a conflict: it takes the save.
		return e.pullVersion(ctx, game, peer, local, remote, mergeVersions(mine.Vector, theirs.Vector))
	}
	if !theirs.HasFiles {
		return pushTo()
	}
	if mine.Vector.genesisOnly() || theirs.Vector.genesisOnly() {
		// A save from before versions existed, never compared with this one
		// since: nothing recorded says which is newer, so the files decide
		// (transitionNewer) and the older device takes the newer save whole,
		// as any device behind does — never file by file, which is how devices
		// ended up holding mixtures of two saves that no game ever wrote. Only
		// an exact tie is asked about.
		switch transitionNewer(local, remote.Manifest, e.Store.GetAgreedHash(gameID, peer.ID)) {
		case 1:
			return pushTo()
		case -1:
			e.Log("info", fmt.Sprintf("%q: %s's save holds the more recent work — taking it whole (this device's is kept in a snapshot)",
				game.Name, peer.Name))
			return e.pullVersion(ctx, game, peer, local, remote, theirs.Vector)
		}
		e.Log("warn", fmt.Sprintf("%q differs from %s's and neither holds more recent work — asking", game.Name, peer.Name))
		e.registerConflict(gameID, peer, local, remote)
		return Result{Status: "conflict", PeerID: peer.ID, PeerName: peer.Name}, true, nil
	}

	// Someone already answered: here, keeping this device's save over the
	// peer's version (or one it has since moved on from), or there, keeping
	// theirs over this one's.
	iAnswered := len(mine.Answered) > 0 && covers(theirs.Vector, mine.Answered)
	theyAnswered := len(theirs.Answered) > 0 && covers(mine.Vector, theirs.Answered)
	switch {
	case iAnswered && theyAnswered:
		// Both were answered, each keeping its own. The later answer stands,
		// as the person who gave it saw the other one too; the earlier one's
		// save is in the snapshot taken before it is replaced.
		// Answers in the same millisecond are settled the same way on both
		// devices, or each would keep asking the other to take its own.
		if mine.AnsweredAt > theirs.AnsweredAt ||
			(mine.AnsweredAt == theirs.AnsweredAt && mine.Vector.String() > theirs.Vector.String()) {
			return pushTo()
		}
		e.Log("info", fmt.Sprintf("%q: %s's answer to the conflict came later — taking its version", game.Name, peer.Name))
		return e.pullVersion(ctx, game, peer, local, remote, mergeVersions(mine.Vector, theirs.Vector))
	case iAnswered:
		// Answered here; the peer is asked in turn when it syncs.
		return pushTo()
	}
	e.Log("warn", fmt.Sprintf("%q was changed independently on this device (%s) and on %s (%s)",
		game.Name, mine.Vector, peer.Name, theirs.Vector))
	e.registerConflict(gameID, peer, local, remote)
	return Result{Status: "conflict", PeerID: peer.ID, PeerName: peer.Name}, true, nil
}

// pullVersion makes this device's save exactly the peer's — every file it
// holds, and none it does not — and only then records the peer's version as
// this device's. Until that, the device stays behind and keeps waiting.
//
// local and remote are filtered by the exclusion rules, so excluded files are
// neither taken nor removed.
func (e *Engine) pullVersion(ctx context.Context, game store.Game, peer Peer,
	local delta.Manifest, remote ManifestResponse, adopt VersionVector) (Result, bool, error) {

	gameID := game.ID
	if until, blocked := e.unwritableUntil(gameID); blocked {
		// Said once when it happened (below); waiting is the answer until then.
		e.logOnce(gameID+"|unwritable", fmt.Sprintf("%q: waiting until %s before writing to its save folder again",
			game.Name, until.Format("15:04")))
		return Result{Status: "unwritable", PeerID: peer.ID, PeerName: peer.Name}, true, nil
	}
	if len(remote.Manifest.Files) == 0 && len(local.Files) > 0 && !remote.DeletionConfirmed {
		// The newer version is an empty folder nobody has confirmed emptying.
		e.Log("info", fmt.Sprintf("%q holds none of %q's save files now, and has not confirmed deleting them — keeping this device's copies",
			peer.Name, game.Name))
		return Result{Status: "peer_holding", PeerID: peer.ID, PeerName: peer.Name}, true, nil
	}

	var d Decision
	for p, lf := range local.Files {
		rf, ok := remote.Manifest.Files[p]
		switch {
		case !ok:
			d.FilesToDeleteLocally = append(d.FilesToDeleteLocally, p)
		case rf.Hash != lf.Hash:
			d.FilesToPull = append(d.FilesToPull, p)
		}
	}
	for p := range remote.Manifest.Files {
		if _, ok := local.Files[p]; !ok {
			d.FilesToPull = append(d.FilesToPull, p)
		}
	}
	localDirs, remoteDirs := toSet(local.Dirs), toSet(remote.Manifest.Dirs)
	for dir := range remoteDirs {
		if _, ok := localDirs[dir]; !ok {
			d.DirsToPull = append(d.DirsToPull, dir)
		}
	}
	for dir := range localDirs {
		if _, ok := remoteDirs[dir]; !ok {
			d.DirsToDeleteLocally = append(d.DirsToDeleteLocally, dir)
		}
	}
	sort.Strings(d.FilesToPull)

	e.Log("info", fmt.Sprintf("%q: taking %s's newer version (%s): %d file(s) to fetch, %d to remove",
		game.Name, peer.Name, adopt, len(d.FilesToPull), len(d.FilesToDeleteLocally)))

	// Whatever this device holds is kept in a snapshot before any of it is
	// replaced or removed.
	replacing := len(d.FilesToDeleteLocally)
	for _, p := range d.FilesToPull {
		if _, ok := local.Files[p]; ok {
			replacing++
		}
	}
	if replacing > 0 {
		if _, err := e.Snapshots.CreateBeforeReplacing(gameID, fmt.Sprintf("Before taking %s's newer version", peer.Name)); err != nil {
			return Result{}, true, fmt.Errorf("refusing to replace %q's files: they could not be snapshotted first: %w", game.Name, err)
		}
	}

	e.setPulling(gameID, true)
	defer e.setPulling(gameID, false)

	applied := e.Writing(gameID)
	started := time.Now()
	e.applyLocalDeletions(gameID, primaryRootOf(game), d)
	if n := len(d.FilesToDeleteLocally); n > 0 {
		e.RecordActivity(store.ActivityEvent{GameID: gameID, Kind: store.ActivityDeleted, Device: peer.Name, Files: n})
		e.noteEmptiedByPeer(gameID, started)
	}
	e.createPulledDirs(gameID, game, d.DirsToPull)
	if len(d.FilesToPull) > 0 {
		if err := e.pullFiles(ctx, peer, gameID, game, primaryRootOf(game), local, remote, d.FilesToPull); err != nil {
			applied()
			if errors.Is(err, fs.ErrPermission) {
				// Not something the next pass fixes: the folder needs other
				// rights (one under Program Files does). Said once, plainly,
				// and tried again later rather than on every pass.
				e.markUnwritable(gameID)
				e.Log("error", fmt.Sprintf("%q: the save folder %s cannot be written (%v). Give OpenSave write access to it, or track the game at another folder. Trying again in %s.",
					game.Name, game.SavePath, err, unwritableRetry))
			}
			return Result{}, true, fmt.Errorf("taking %s's version of %q did not finish (it is resumed on the next sync): %w", peer.Name, game.Name, err)
		}
	}
	applied()

	// Only a save that is now exactly the peer's takes its version. Read
	// straight from disk, not through the gate (settle.go): this sync is the
	// one that wrote it, and after following a peer onto a branch it still
	// holds the save — waiting for that would wait for itself until the
	// sync's time ran out.
	fresh, err := delta.BuildManifest(game.SavePath)
	if err != nil {
		return Result{}, true, err
	}
	freshHash := e.versionHashOf(gameID, fresh)
	fresh = filterManifest(fresh, e.rulesFor(gameID))
	changedSince := false
	if !sameFiles(fresh, remote.Manifest) {
		if !changedSincePulled(primaryRootOf(game).Path, fresh, remote.Manifest, d) {
			return Result{}, true, fmt.Errorf("taking %s's version of %q did not finish: the save still differs; it is resumed on the next sync", peer.Name, game.Name)
		}
		changedSince = true
	}

	e.versionMu.Lock()
	if gv, err := e.loadVersionLocked(gameID); err == nil {
		theirs := VersionInfo{}
		if remote.Version != nil {
			theirs = *remote.Version
		}
		gv.adopt(adopt, theirs)
		gv.rec.Hash = freshHash
		gv.rec.Pulling = false
		e.Log("success", fmt.Sprintf("%q now holds version %s from %s", game.Name, gv.vec, peer.Name))
		if changedSince {
			// The peer's version arrived whole, and was then changed here -
			// the game saving straight after the pull. That is a version of
			// this device's own, made from theirs, and goes back to them.
			e.bumpLocked(&gv, game.Name, freshHash)
		}
		_ = e.saveVersionLocked(gv)
	}
	e.versionMu.Unlock()
	if changedSince {
		e.Transport.TriggerPeerPull(peer, gameID)
	}

	// The file comparison's own records, so it agrees if it is ever asked.
	e.persistLineage(gameID, peer.ID, fresh, remote.Manifest)
	_ = e.Store.SetAgreedHash(gameID, peer.ID, remote.Manifest.ManifestHash())

	e.syncExtraRoots(ctx, gameID, game, peer, remote)
	if !d.HasChanges() {
		return Result{Status: "in_sync", Direction: "none", PeerID: peer.ID, PeerName: peer.Name}, true, nil
	}
	return Result{Status: "updated", Direction: "pull", PeerID: peer.ID, PeerName: peer.Name}, true, nil
}

// unwritableRetry is how long a game whose save folder could not be written
// waits before a pull into it is tried again.
const unwritableRetry = 30 * time.Minute

func (e *Engine) markUnwritable(gameID string) {
	e.versionMu.Lock()
	defer e.versionMu.Unlock()
	if e.unwritable == nil {
		e.unwritable = map[string]time.Time{}
	}
	e.unwritable[gameID] = time.Now().Add(unwritableRetry)
}

func (e *Engine) unwritableUntil(gameID string) (time.Time, bool) {
	e.versionMu.Lock()
	defer e.versionMu.Unlock()
	until, ok := e.unwritable[gameID]
	if !ok || time.Now().After(until) {
		delete(e.unwritable, gameID)
		return time.Time{}, false
	}
	return until, true
}

// setPulling marks a pull towards a newer version as running, so the files it
// writes are never taken for a change made here. In memory for this run; in
// the record from the start until the pull has completed, which only adopting
// the version clears — a pull that stopped part-way leaves a folder that is
// neither this device's version nor the peer's, and it must not be read as a
// change made here (refreshLocked, NoteLocalChange) while it waits to resume.
func (e *Engine) setPulling(gameID string, on bool) {
	e.versionMu.Lock()
	defer e.versionMu.Unlock()
	if e.pullingNow == nil {
		e.pullingNow = map[string]bool{}
	}
	if !on {
		delete(e.pullingNow, gameID)
		return
	}
	e.pullingNow[gameID] = true
	if gv, err := e.loadVersionLocked(gameID); err == nil && gv.rec.GameID != "" && !gv.rec.Pulling {
		gv.rec.Pulling = true
		_ = e.saveVersionLocked(gv)
	}
}

// quickVersionCheck is the version half of quickInSync: both saves are the
// agreed one, so the question is only whether the versions say the same.
// done reports a final answer; otherwise the full sync decides.
func (e *Engine) quickVersionCheck(game store.Game, peer Peer, local delta.Manifest, theirs VersionInfo) (res Result, done bool) {
	e.versionMu.Lock()
	gv, err := e.loadVersionLocked(game.ID)
	if err != nil || gv.rec.GameID == "" {
		e.versionMu.Unlock()
		return Result{}, false
	}
	before := gv.target.clone()
	changed := e.refreshLocked(&gv, game, local)
	gv.learn(theirs)
	if changed || compareVersions(before, gv.target) != versionEqual || len(before) != len(gv.target) {
		_ = e.saveVersionLocked(gv)
	}
	mine := gv.info(len(local.Files) > 0)
	e.versionMu.Unlock()

	if compareVersions(mine.Vector, theirs.Vector) != versionEqual {
		return Result{}, false
	}
	if mine.Outdated() || theirs.Outdated() {
		// The same save, and at least one of the two knows a newer one exists
		// that neither holds: nothing to do between these two.
		return Result{Status: versionStatusWaiting, PeerID: peer.ID, PeerName: peer.Name}, true
	}
	return Result{}, true
}

// logOnce logs a line the first time it is the answer for key, and not again
// while it stays the answer: waiting is re-decided every reconcile.
func (e *Engine) logOnce(key, msg string) {
	e.versionMu.Lock()
	if e.loggedOnce == nil {
		e.loggedOnce = map[string]string{}
	}
	same := e.loggedOnce[key] == msg
	e.loggedOnce[key] = msg
	e.versionMu.Unlock()
	if !same {
		e.Log("info", msg)
	}
}

func (e *Engine) forgetLogOnce(key string) {
	e.versionMu.Lock()
	delete(e.loggedOnce, key)
	e.versionMu.Unlock()
}

// transitionNewer says which of two saves from before versions is the newer,
// when that is beyond doubt: 1 for local, -1 for remote, 0 when it is not.
//
// Beyond doubt means one of two things. The devices last agreed on a state
// and one of them still holds exactly it: that one has not changed since, so
// the other is newer. Or one save is newer in every file that differs — each
// is newer there or missing on the other side — and the other holds no file
// of its own written after the first's newest. That is what a save someone
// played on looks like next to one nobody touched.
//
// It is deliberately not "the newer file wins, file by file": that makes a
// mixture of two saves no game wrote. And not "the newest file wins", which a
// mixture passes as easily as a real save. Here the device that gives way was
// older in every file that differs, so it never loses anything newer than
// what it takes — at worst it takes a copy that arrived only half-way, and
// then the complete save, newer than both, replaces that too when it is
// seen. States newer each in different files fail the test both ways and are
// left to a person.
func transitionNewer(local, remote delta.Manifest, agreed string) int {
	if agreed != "" {
		lh, rh := local.ManifestHash(), remote.ManifestHash()
		switch {
		case lh == agreed && rh != agreed:
			return -1
		case rh == agreed && lh != agreed:
			return 1
		}
	}
	// Two saves with no file in common are not two versions of the same
	// files — a save kept as one file named differently on each device, say
	// — and nothing about their times says one should replace the other.
	if len(local.Files) > 0 && len(remote.Files) > 0 && !shareAPath(local, remote) {
		return 0
	}
	l, r := newerInEveryFile(local, remote), newerInEveryFile(remote, local)
	switch {
	case l && !r:
		return 1
	case r && !l:
		return -1
	}
	return newestWork(local, remote)
}

// newestWork settles two saves from before versions that neither rule above
// could: the one holding the most recent work — the latest-written of the
// files where they differ — is the save, and if that is a tie, the one newer
// in more of those files. The other takes it whole, keeping a snapshot of its
// own first.
//
// Before versions, devices that never ran a game held copies relayed from the
// others at different times, and syncs file by file had left some holding
// mixtures; those were asked about, about states nobody made. Taking the one
// with the most recent work is what syncing is for — an older state is never
// what anyone wants handed on — and whatever the other held stays in its
// snapshot. Changes made since versions existed are not decided this way:
// those are versions of their own, and a real conflict between them is asked.
//
// 0 only for an exact tie, which leaves it to a person.
func newestWork(a, b delta.Manifest) int {
	var aNewest, bNewest delta.Milli
	aCount, bCount := 0, 0
	consider := func(p string) {
		af, inA := a.Files[p]
		bf, inB := b.Files[p]
		if inA && inB && af.Hash == bf.Hash {
			return
		}
		if inA && af.MtimeMs > aNewest {
			aNewest = af.MtimeMs
		}
		if inB && bf.MtimeMs > bNewest {
			bNewest = bf.MtimeMs
		}
		if inA && inB {
			switch {
			case af.MtimeMs > bf.MtimeMs:
				aCount++
			case bf.MtimeMs > af.MtimeMs:
				bCount++
			}
		}
	}
	for p := range a.Files {
		consider(p)
	}
	for p := range b.Files {
		if _, inA := a.Files[p]; !inA {
			consider(p)
		}
	}
	switch {
	case aNewest > bNewest:
		return 1
	case bNewest > aNewest:
		return -1
	case aCount > bCount:
		return 1
	case bCount > aCount:
		return -1
	}
	return 0
}

func shareAPath(a, b delta.Manifest) bool {
	for p := range a.Files {
		if _, ok := b.Files[p]; ok {
			return true
		}
	}
	return false
}

// newerInEveryFile reports whether a is newer than b wherever they differ,
// and they do differ.
func newerInEveryFile(a, b delta.Manifest) bool {
	differ := false
	var aNewest delta.Milli
	for p, af := range a.Files {
		if af.MtimeMs > aNewest {
			aNewest = af.MtimeMs
		}
		bf, ok := b.Files[p]
		if !ok {
			differ = true
			continue
		}
		if af.Hash != bf.Hash {
			differ = true
			if af.MtimeMs <= bf.MtimeMs {
				return false
			}
		}
	}
	for p, bf := range b.Files {
		if _, ok := a.Files[p]; ok {
			continue
		}
		differ = true
		if bf.MtimeMs >= aNewest {
			return false
		}
	}
	return differ
}

// StatusPeerOutdatedApp: the peer runs an OpenSave without save versions.
// Nothing is taken from it until it is updated; it can still take from here.
const StatusPeerOutdatedApp = "peer_outdated_app"

// OldBuildDeleteMessage is the answer to a deletion asked for by a device
// whose OpenSave does not keep save versions.
const OldBuildDeleteMessage = "this device keeps save versions and takes no deletions from an OpenSave that does not — update OpenSave on the asking device"

// notePeerApp records whether a peer's OpenSave keeps save versions, as its
// last manifest answer said.
func (e *Engine) notePeerApp(peerID string, versions bool) {
	e.versionMu.Lock()
	defer e.versionMu.Unlock()
	if e.peerVersions == nil {
		e.peerVersions = map[string]bool{}
	}
	e.peerVersions[peerID] = versions
}

// PeerNeedsUpdate reports whether a peer is known to run an OpenSave without
// save versions — one this device takes nothing from — so the person can be
// told to update it.
func (e *Engine) PeerNeedsUpdate(peerID string) bool {
	e.versionMu.Lock()
	defer e.versionMu.Unlock()
	v, known := e.peerVersions[peerID]
	return known && !v
}

// RefusedOldBuildDelete logs, once per game and device, that a deletion asked
// for by an OpenSave without save versions was refused.
func (e *Engine) RefusedOldBuildDelete(gameID, from string) {
	e.logOnce("refused|"+gameID+"|"+from, fmt.Sprintf(
		"%s asked to delete files of %q; refused — its OpenSave does not keep save versions and decides by the old file comparison. Update it there.",
		from, gameID))
}

// supersedes reports whether theirs is newer than this device's version and
// than the version the open conflict is with: both sides of the question are
// behind it.
func (e *Engine) supersedes(gameID string, theirs VersionInfo) bool {
	e.mu.Lock()
	c := e.activeConflicts[gameID]
	e.mu.Unlock()
	if c == nil || c.remoteVersion == nil {
		return false
	}
	e.versionMu.Lock()
	gv, err := e.loadVersionLocked(gameID)
	e.versionMu.Unlock()
	if err != nil || len(gv.vec) == 0 {
		return false
	}
	return compareVersions(gv.vec, theirs.Vector) == versionOlder &&
		compareVersions(c.remoteVersion.Vector, theirs.Vector) == versionOlder
}

// UseSaveEverywhere makes this device's save of a game the one every other
// device takes: its version becomes newer than every save from before
// versions existed, on any device, so each of those takes this one whole,
// keeping a snapshot of its own first. For when several devices hold states
// nobody can rank — copies that arrived half-way, mixtures of two saves —
// and a person knows which device has the right one.
//
// A change made on another device since versions existed is not overruled:
// that device and this one have each changed the save independently, and
// the person there is asked, as for any conflict.
func (e *Engine) UseSaveEverywhere(gameID string) (VersionVector, error) {
	game, err := e.Store.GetGame(gameID)
	if err != nil {
		return nil, err
	}
	if !game.AutoSync {
		return nil, errors.New("automatic sync is off for this game, so it does not take part in versions")
	}
	m, err := delta.BuildManifest(game.SavePath)
	if err != nil {
		return nil, err
	}
	if len(m.Files) == 0 {
		return nil, errors.New("this device holds no save files for this game")
	}
	e.versionMu.Lock()
	gv, err := e.loadVersionLocked(gameID)
	if err != nil || gv.rec.GameID == "" {
		e.versionMu.Unlock()
		if err == nil {
			err = errors.New("the game is not tracked")
		}
		return nil, err
	}
	gv.vec = mergeVersions(gv.vec, VersionVector{genesisAll: 1})
	gv.rec.Counter++
	gv.vec[gv.rec.Key] = gv.rec.Counter
	gv.rec.Hash = e.versionHashOf(gameID, m)
	gv.target, gv.answered, gv.rec.AnsweredAt = nil, nil, 0
	gv.rec.Pulling = false
	saveErr := e.saveVersionLocked(gv)
	vec := gv.vec.clone()
	e.versionMu.Unlock()
	if saveErr != nil {
		return nil, saveErr
	}
	e.mu.Lock()
	_, conflicted := e.activeConflicts[gameID]
	e.mu.Unlock()
	if conflicted {
		e.clearConflict(gameID)
	}
	e.Log("info", fmt.Sprintf("%q: this device's save is to be used everywhere — version %s", game.Name, vec))
	return vec, nil
}

// changedSincePulled reports a pull that finished and whose save was then
// changed here: every way the save now differs from the peer's is a change
// someone other than OpenSave made since the pull was planned. A game saving,
// or a person deleting a file, the moment a pull is done leaves exactly that.
//
//   - A file the pull did not touch was the same on both sides when the pull
//     was planned (anything that differed was to be fetched or removed). If
//     it differs now, it was changed here meanwhile.
//   - A file the pull wrote or removed differs because of someone else only
//     if someone other than OpenSave has written it, or put it back, since
//     (owntouch). Still as OpenSave left it, it is the pull's own unfinished
//     part: a pull that stopped part-way, resumed on the next sync as before.
//
// Without this, a save the game had just written, or a file just deleted,
// was taken for the unfinished part of the pull and replaced with the peer's
// on the next sync; only the snapshot taken before replacing it still held it.
func changedSincePulled(root string, fresh, remote delta.Manifest, d Decision) bool {
	touched := make(map[string]bool, len(d.FilesToPull)+len(d.FilesToDeleteLocally))
	for _, p := range d.FilesToPull {
		touched[p] = true
	}
	for _, p := range d.FilesToDeleteLocally {
		touched[p] = true
	}
	changedByOthers := func(p string) bool {
		return !touched[p] || !owntouch.Recent(delta.LocalNameFor(root, p))
	}
	differs := 0
	for p, lf := range fresh.Files {
		if rf, ok := remote.Files[p]; ok && rf.Hash == lf.Hash {
			continue
		}
		differs++
		if !changedByOthers(p) {
			return false
		}
	}
	for p := range remote.Files {
		if _, ok := fresh.Files[p]; ok {
			continue
		}
		differs++
		if !changedByOthers(p) {
			return false
		}
	}
	return differs > 0
}
