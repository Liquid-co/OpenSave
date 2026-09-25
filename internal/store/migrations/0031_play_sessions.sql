-- Play sessions: when a tracked game was running on this device, as OpenSave
-- saw it — its process found and then gone (see internal/sessions), or
-- `opensave wrap` launching it. What Home and the game's page read to say
-- when it was last played and for how long in all, and what a snapshot at
-- the end of a session is named after.
--
-- This device's alone, like collections: not sent between devices.
-- A game untracked takes its sessions with it (ON DELETE CASCADE).
CREATE TABLE play_sessions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id    TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    started_ms INTEGER NOT NULL,
    ended_ms   INTEGER NOT NULL
);

CREATE INDEX play_sessions_by_game ON play_sessions (game_id, ended_ms);
