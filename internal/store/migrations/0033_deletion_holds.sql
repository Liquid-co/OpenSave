-- Games held back from syncing because every save file in one of their save
-- locations was deleted on this device at once, and nobody has said yet
-- whether that was meant (see internal/p2p/syncengine/hold.go).
--
-- state is 'held' while it waits for an answer; 'fetching' after the answer
-- "put them back", until every file is here again; and 'confirmed' once the
-- answer was "delete them on the other devices too" — or once another
-- device's own deletion emptied the folder here — so the same empty folder
-- is not held again. The row goes when the folder has its files again, which
-- arms the guard for next time.
--
-- This device's alone: a hold is about what happened here. A game untracked
-- takes its hold with it (ON DELETE CASCADE).
CREATE TABLE deletion_holds (
    game_id   TEXT PRIMARY KEY REFERENCES games(id) ON DELETE CASCADE,
    since_ms  INTEGER NOT NULL,
    state     TEXT NOT NULL DEFAULT 'held',
    -- What the emptied locations held before, as a JSON object of location
    -- name ("" is the main save folder) to file paths: what "delete them
    -- there too" would remove elsewhere, and what has to be back for the
    -- hold to let go.
    held      TEXT NOT NULL DEFAULT '{}'
);
