-- What happened to each game's save that no other table remembers: files
-- received from another device or sent to one, deletions another device made
-- here, a snapshot put back, a newer save brought from the cloud, a save
-- emptied, two saves in conflict. Snapshots and play sessions keep their own
-- tables; the activity page reads all three (internal/daemon/activity.go).
--
-- A history, not a log: one row for each thing that happened, kept to the
-- newest few thousand. This device's alone. A game untracked takes its rows
-- with it.
CREATE TABLE activity_events (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    at_ms     INTEGER NOT NULL,
    game_id   TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    kind      TEXT NOT NULL,
    -- The other device's name, when another device was involved.
    device    TEXT NOT NULL DEFAULT '',
    files     INTEGER NOT NULL DEFAULT 0,
    bytes     INTEGER NOT NULL DEFAULT 0,
    detail    TEXT NOT NULL DEFAULT ''
);

CREATE INDEX activity_events_by_time ON activity_events (at_ms);
CREATE INDEX activity_events_by_game ON activity_events (game_id, at_ms);
