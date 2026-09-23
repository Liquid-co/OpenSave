package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opensave/opensave/internal/cloud"
	"github.com/opensave/opensave/internal/snapshot"
	"github.com/opensave/opensave/internal/store"
)

// Two devices sharing one cloud folder, the way two machines signed in to the
// same Google Drive share one. The daemons are built, not started: no watcher,
// no peers, and every snapshot is taken by the test, so what each check sees
// is exactly what the test arranged.

const cloudGame = "hollow-knight"

type cloudDevice struct {
	t    *testing.T
	d    *Daemon
	save string
}

func newCloudDevice(t *testing.T, name, cloudDir string) *cloudDevice {
	t.Helper()
	d := newTestDaemon(t)
	settings, err := d.Store.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.DeviceName = name
	if err := d.Store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
	cfg, err := d.Store.GetCloudConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled = true
	cfg.Provider = "local"
	cfg.URL = cloudDir
	if err := d.Store.UpdateCloudConfig(cfg); err != nil {
		t.Fatal(err)
	}
	save := t.TempDir()
	if err := d.Store.CreateGame(store.Game{
		ID: cloudGame, Name: "Hollow Knight", SavePath: save,
		ActiveBranch: "main", AutoSync: true, MaxSnapshots: 50,
	}); err != nil {
		t.Fatal(err)
	}
	return &cloudDevice{t: t, d: d, save: save}
}

// play writes the save and snapshots it the way the watcher does — the
// snapshot, then the content hash recorded against it — and waits for the
// upload and the announcement that follows it.
func (c *cloudDevice) play(content string) store.Snapshot {
	c.t.Helper()
	// Snapshot ids are milliseconds, and two devices must not share one.
	time.Sleep(5 * time.Millisecond)
	c.write(content)
	snap, err := c.d.Snapshots.Create(cloudGame, "", true)
	if err != nil {
		c.t.Fatal(err)
	}
	game, _ := c.d.Store.GetGame(cloudGame)
	hash, err := c.d.currentContentHash(game)
	if err != nil {
		c.t.Fatal(err)
	}
	if err := c.d.Store.SetLastManifestHash(cloudGame, hash); err != nil {
		c.t.Fatal(err)
	}
	c.d.uploads.Wait()
	time.Sleep(5 * time.Millisecond)
	return snap
}

func (c *cloudDevice) write(content string) {
	c.t.Helper()
	if err := os.WriteFile(filepath.Join(c.save, "user1.dat"), []byte(content), 0o666); err != nil {
		c.t.Fatal(err)
	}
}

func (c *cloudDevice) saveIs() string {
	c.t.Helper()
	b, err := os.ReadFile(filepath.Join(c.save, "user1.dat"))
	if err != nil {
		c.t.Fatal(err)
	}
	return string(b)
}

func (c *cloudDevice) check() []CloudOffer {
	c.t.Helper()
	c.d.CheckCloud()
	c.d.uploads.Wait()
	return c.d.CloudOffers()
}

// The request in issue #12: the other device has a newer save, and this one
// is told. Asked, not taken — this device has a save of its own that the
// other does not carry on from.
func TestCloudOffersANewerSaveFromAnotherDevice(t *testing.T) {
	dir := t.TempDir()
	desktop := newCloudDevice(t, "Desktop", dir)
	deck := newCloudDevice(t, "Steam Deck", dir)

	deck.play("deck's own start")
	a1 := desktop.play("act 1")

	offers := deck.check()
	if len(offers) != 1 {
		t.Fatalf("offers = %+v, want the Desktop's save", offers)
	}
	o := offers[0]
	if o.DeviceName != "Desktop" || o.SnapshotID != a1.ID || !o.Diverged {
		t.Errorf("offer = %+v, want Desktop's %s, marked as replacing progress made here", o, a1.ID)
	}
	if got := deck.saveIs(); got != "deck's own start" {
		t.Fatalf("the Deck's save changed to %q without being asked", got)
	}

	if err := deck.d.AcceptCloudOffer(cloudGame, a1.ID); err != nil {
		t.Fatal(err)
	}
	if got := deck.saveIs(); got != "act 1" {
		t.Errorf("after bringing it here the Deck's save is %q, want %q", got, "act 1")
	}
	if left := deck.d.CloudOffers(); len(left) != 0 {
		t.Errorf("the offer stayed open after it was taken: %+v", left)
	}
}

// A save that carries on from the one this device has — the other device took
// ours, then played on — is put in place without asking, the way sync
// between two devices that are both on would do it.
func TestCloudTakesASaveThatContinuesFromThisOne(t *testing.T) {
	dir := t.TempDir()
	desktop := newCloudDevice(t, "Desktop", dir)
	deck := newCloudDevice(t, "Steam Deck", dir)
	var pulled []CloudPulled
	deck.d.OnCloudPulled = func(p CloudPulled) { pulled = append(pulled, p) }

	deck.play("deck's own start")
	a1 := desktop.play("act 1")
	deck.check()
	if err := deck.d.AcceptCloudOffer(cloudGame, a1.ID); err != nil {
		t.Fatal(err)
	}
	deck.d.uploads.Wait()

	// The Desktop sees the Deck is on its own save: nothing to offer back.
	if offers := desktop.check(); len(offers) != 0 {
		t.Fatalf("the Desktop was offered its own save back: %+v", offers)
	}

	a2 := desktop.play("act 2")
	if offers := deck.check(); len(offers) != 0 {
		t.Errorf("a save that continues from the Deck's own was asked about instead of taken: %+v", offers)
	}
	if got := deck.saveIs(); got != "act 2" {
		t.Fatalf("the Deck's save is %q, want the Desktop's %q", got, "act 2")
	}
	if len(pulled) != 1 || pulled[0].SnapshotID != a2.ID || pulled[0].DeviceName != "Desktop" {
		t.Errorf("pulled = %+v, want one notice for Desktop's %s", pulled, a2.ID)
	}

	// Taking it must not bounce back: the Desktop now sees the Deck on the
	// Desktop's own save.
	deck.d.uploads.Wait()
	if offers := desktop.check(); len(offers) != 0 {
		t.Errorf("the Desktop was offered its own save back after the Deck took it: %+v", offers)
	}
}

// Both devices played since they last matched. Neither save is taken on its
// own; the one that is asked is told it would replace progress.
func TestCloudAsksWhenBothDevicesPlayed(t *testing.T) {
	dir := t.TempDir()
	desktop := newCloudDevice(t, "Desktop", dir)
	deck := newCloudDevice(t, "Steam Deck", dir)

	deck.play("start")
	a1 := desktop.play("act 1")
	deck.check()
	if err := deck.d.AcceptCloudOffer(cloudGame, a1.ID); err != nil {
		t.Fatal(err)
	}
	deck.d.uploads.Wait()

	desktop.play("desktop went left")
	b := deck.play("deck went right")

	offers := desktop.check()
	if len(offers) != 1 || offers[0].SnapshotID != b.ID || !offers[0].Diverged {
		t.Fatalf("offers = %+v, want the Deck's %s, marked as replacing progress", offers, b.ID)
	}
	if got := desktop.saveIs(); got != "desktop went left" {
		t.Errorf("the Desktop's save was replaced without asking: %q", got)
	}
}

// The hazard the heads exist for. A restore keeps a copy of the save it
// replaces, and that copy is uploaded like any snapshot — newest in date,
// older in content than what it was replaced with. Offering "the newest file"
// would hand it to the other device as if it were progress.
func TestCloudNeverOffersTheCopyKeptBeforeARestore(t *testing.T) {
	dir := t.TempDir()
	desktop := newCloudDevice(t, "Desktop", dir)
	deck := newCloudDevice(t, "Steam Deck", dir)

	deck.play("start")
	a1 := desktop.play("act 1")
	deck.check()
	if err := deck.d.AcceptCloudOffer(cloudGame, a1.ID); err != nil {
		t.Fatal(err)
	}
	deck.d.uploads.Wait()
	desktop.play("act 2")
	deck.check()
	if got := deck.saveIs(); got != "act 2" {
		t.Fatalf("setup: the Deck's save is %q, want %q", got, "act 2")
	}

	// The Desktop rolls back to act 1. Its act-2 save is kept and uploaded.
	time.Sleep(5 * time.Millisecond)
	if _, err := desktop.d.Snapshots.Restore(cloudGame, a1.ID); err != nil {
		t.Fatal(err)
	}
	desktop.d.uploads.Wait()
	if n := countSnapshotsNewerThan(t, dir, a1.ID); n < 2 {
		t.Fatalf("setup: expected the kept copy in the cloud beside act 2, found %d newer snapshots", n)
	}

	if offers := deck.check(); len(offers) != 0 {
		t.Errorf("the copy kept before a restore was offered as a newer save: %+v", offers)
	}
	if got := deck.saveIs(); got != "act 2" {
		t.Errorf("the Deck's save changed to %q", got)
	}
}

// The sharp form of the same hazard. The Desktop takes the Deck's newer save,
// and keeps its own older one first — as every restore does — so the cloud
// now holds a copy of the Desktop's act 1 dated after the Deck's act 2. The
// Deck must not be offered that copy as the Desktop's newer save.
func TestCloudNeverOffersTheCopyKeptBeforeAPull(t *testing.T) {
	dir := t.TempDir()
	desktop := newCloudDevice(t, "Desktop", dir)
	deck := newCloudDevice(t, "Steam Deck", dir)

	deck.play("start")
	a1 := desktop.play("act 1")
	deck.check()
	if err := deck.d.AcceptCloudOffer(cloudGame, a1.ID); err != nil {
		t.Fatal(err)
	}
	deck.d.uploads.Wait()
	deck.play("act 2, on the Deck")

	desktop.check()
	desktop.d.uploads.Wait()
	if got := desktop.saveIs(); got != "act 2, on the Deck" {
		t.Fatalf("setup: the Desktop did not take the Deck's save; it has %q", got)
	}

	if offers := deck.check(); len(offers) != 0 {
		t.Errorf("the Desktop's copy of its old save was offered to the Deck as newer: %+v", offers)
	}
	if got := deck.saveIs(); got != "act 2, on the Deck" {
		t.Errorf("the Deck's save changed to %q", got)
	}
}

// "Not now" means not this one. A newer save is offered again.
func TestCloudDismissedSaveIsNotOfferedAgain(t *testing.T) {
	dir := t.TempDir()
	desktop := newCloudDevice(t, "Desktop", dir)
	deck := newCloudDevice(t, "Steam Deck", dir)

	deck.play("start")
	a1 := desktop.play("act 1")
	deck.check()
	if err := deck.d.DismissCloudOffer(cloudGame, a1.ID); err != nil {
		t.Fatal(err)
	}
	if offers := deck.check(); len(offers) != 0 {
		t.Fatalf("a save the person said no to was offered again: %+v", offers)
	}

	a2 := desktop.play("act 2")
	if offers := deck.check(); len(offers) != 1 || offers[0].SnapshotID != a2.ID {
		t.Errorf("offers = %+v, want the newer %s", offers, a2.ID)
	}
}

// A save changed here since its last snapshot is not replaced without asking,
// even by one that continues from that snapshot: the change would be lost.
func TestCloudLeavesAChangedSaveAlone(t *testing.T) {
	dir := t.TempDir()
	desktop := newCloudDevice(t, "Desktop", dir)
	deck := newCloudDevice(t, "Steam Deck", dir)

	deck.play("start")
	a1 := desktop.play("act 1")
	deck.check()
	if err := deck.d.AcceptCloudOffer(cloudGame, a1.ID); err != nil {
		t.Fatal(err)
	}
	deck.d.uploads.Wait()

	deck.write("played on the Deck, not snapshotted yet")
	desktop.play("act 2")

	offers := deck.check()
	if got := deck.saveIs(); got != "played on the Deck, not snapshotted yet" {
		t.Fatalf("an unsnapshotted change was overwritten: the save is now %q", got)
	}
	if len(offers) != 1 || !offers[0].Diverged {
		t.Errorf("offers = %+v, want the Desktop's save, marked as replacing progress", offers)
	}
}

// With the setting off, even a save that continues from this one is asked.
func TestCloudAutoPullOffAsksInstead(t *testing.T) {
	dir := t.TempDir()
	desktop := newCloudDevice(t, "Desktop", dir)
	deck := newCloudDevice(t, "Steam Deck", dir)
	settings, _ := deck.d.Store.GetSettings()
	settings.CloudAutoPull = false
	if err := deck.d.Store.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}

	deck.play("start")
	a1 := desktop.play("act 1")
	deck.check()
	if err := deck.d.AcceptCloudOffer(cloudGame, a1.ID); err != nil {
		t.Fatal(err)
	}
	deck.d.uploads.Wait()
	a2 := desktop.play("act 2")

	offers := deck.check()
	if got := deck.saveIs(); got != "act 1" {
		t.Fatalf("the save was replaced with the setting off: %q", got)
	}
	if len(offers) != 1 || offers[0].SnapshotID != a2.ID || offers[0].Diverged {
		t.Errorf("offers = %+v, want %s, not marked as replacing progress", offers, a2.ID)
	}
}

// Versions without heads share the same folder. Head files must not parse as
// snapshots, or those versions would list them as backups.
func TestHeadFilesAreNotSnapshots(t *testing.T) {
	name := cloud.HeadFileName(cloudGame, "node_3f9a", time.Now())
	if _, _, _, ok := snapshot.ParseExportEntryName(name); ok {
		t.Errorf("%s parses as a snapshot name; older versions would list it as a backup", name)
	}
	if g, dev, _, ok := cloud.ParseHeadFileName(name); !ok || g != cloudGame || dev != "node3f9a" {
		t.Errorf("ParseHeadFileName(%s) = %q, %q, %v", name, g, dev, ok)
	}
	if _, _, _, ok := cloud.ParseHeadFileName(cloudGame + "__main__snap_1.zip"); ok {
		t.Error("a snapshot name parsed as a head")
	}
}

func countSnapshotsNewerThan(t *testing.T, dir, snapID string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if _, _, id, ok := snapshot.ParseExportEntryName(e.Name()); ok && strings.HasPrefix(id, "snap_") && snapMillis(id) > snapMillis(snapID) {
			n++
		}
	}
	return n
}
