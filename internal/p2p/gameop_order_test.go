package p2p

import "testing"

// An untrack and a retrack for the same game, applied in the wrong order,
// must not leave the device with the older outcome.
//
// The two travel as independent requests and, over the relay, are served on
// separate goroutines — so a pair fired close together can land in either
// order. Backwards, an untrack undoes the retrack that came after it and the
// device ends with no game and a tombstone that refuses to take it back.
func TestGameOps_AnOlderUntrackCannotUndoANewerRetrack(t *testing.T) {
	e := &Engine{Log: func(string, string) {}}
	var applied []string
	e.OnUntrackRequest = func(id string) { applied = append(applied, "untrack") }
	e.OnRetrackRequest = func(id string) { applied = append(applied, "retrack") }

	// The retrack (sent second, stamped later) arrives first.
	e.applyPeerRetrack("game", 2000)
	e.applyPeerUntrack("game", 1000)

	if len(applied) != 1 || applied[0] != "retrack" {
		t.Errorf("applied %v; the stale untrack was acted on and undid the newer retrack", applied)
	}
}

func TestGameOps_AnOlderRetrackCannotUndoANewerUntrack(t *testing.T) {
	e := &Engine{Log: func(string, string) {}}
	var applied []string
	e.OnUntrackRequest = func(id string) { applied = append(applied, "untrack") }
	e.OnRetrackRequest = func(id string) { applied = append(applied, "retrack") }

	e.applyPeerUntrack("game", 2000)
	e.applyPeerRetrack("game", 1000)

	if len(applied) != 1 || applied[0] != "untrack" {
		t.Errorf("applied %v; a stale retrack brought back a game the user had just removed", applied)
	}
}

// In order, both apply — and a stamp of zero (an older build) always does.
func TestGameOps_InOrderAndUnstampedBothApply(t *testing.T) {
	e := &Engine{Log: func(string, string) {}}
	var applied []string
	e.OnUntrackRequest = func(id string) { applied = append(applied, "untrack") }
	e.OnRetrackRequest = func(id string) { applied = append(applied, "retrack") }

	e.applyPeerUntrack("game", 1000)
	e.applyPeerRetrack("game", 2000)
	e.applyPeerUntrack("game", 0) // legacy frame, no stamp
	if len(applied) != 3 {
		t.Errorf("applied %v; in-order and unstamped operations must all apply", applied)
	}
}
