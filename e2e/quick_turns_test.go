package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/opensave/opensave/testutil"
)

// Two devices take turns quickly over the relay: each edits as soon as the
// other's change has arrived, before the report that it arrived has been
// acted on. Nobody edits at the same time, so no turn may be a conflict.
//
// The device that handed a change over records "we agree on this" only once
// it hears the other now holds it. It used to hear only which files were
// taken, and to check the rest by fetching the other's save afterwards — by
// which time the other had already edited again. The agreement was never
// recorded, and the next turn read as both sides having changed.
func TestQuickTurns_NoFalseConflict(t *testing.T) {
	a, b, gameID := trackBothOverRelay(t, "QuickTurns", map[string]string{"slot1.sav": "turn-0"})
	testutil.SettleSync(t, gameID, a, b)

	writer, other := b, a
	for turn := 1; turn <= 6; turn++ {
		want := fmt.Sprintf("turn-%d", turn)
		writer.WriteSave("slot1.sav", want)
		if status, _ := syncTo(writer, gameID, other.NodeID()); status == "conflict" {
			c, _ := conflictOn(writer, gameID)
			t.Fatalf("turn %d: a one-sided edit conflicted: %+v", turn, c.DiffFiles)
		}
		if !testutil.WaitFor(60*time.Second, func() bool { return other.ReadSave("slot1.sav") == want }) {
			if c, ok := conflictOn(other, gameID); ok {
				t.Fatalf("turn %d: the edit never arrived; the receiving device holds a conflict: %+v", turn, c.DiffFiles)
			}
			t.Fatalf("turn %d: the edit never arrived", turn)
		}
		if c, ok := conflictOn(writer, gameID); ok {
			t.Fatalf("turn %d: the device that edited holds a conflict: %+v", turn, c.DiffFiles)
		}
		writer, other = other, writer // the other edits at once
	}
}
