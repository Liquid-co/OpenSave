package syncengine

import (
	"fmt"
	"time"

	"github.com/opensave/opensave/internal/store"
)

// RecordActivity keeps an event in the history the activity page reads
// (store.ActivityEvent), and says it happened through OnActivity. The history
// is a convenience, never a reason to fail what it describes: a write that
// fails is logged and the sync carries on.
func (e *Engine) RecordActivity(ev store.ActivityEvent) {
	if ev.AtMs == 0 {
		ev.AtMs = time.Now().UnixMilli()
	}
	saved, err := e.Store.RecordActivity(ev)
	if err != nil {
		e.Log("warn", fmt.Sprintf("could not note %s for %s in the activity history: %v", ev.Kind, ev.GameID, err))
		return
	}
	if e.OnActivity != nil {
		e.OnActivity(saved)
	}
}

// locationDetail is what an event says about which save location it was in:
// nothing for the main folder.
func locationDetail(root string) string {
	if root == "" {
		return ""
	}
	return fmt.Sprintf("in its %q location", root)
}
