-- Files in a game's main save folder judged to be its settings rather than its
-- save, by what they hold (presets/detectsettings.go), and files shown to be a
-- save after all — by changing with the save, in this device's snapshots or in
-- what another device sent (daemon/detectsettings.go).
--
-- verdict 'settings' leaves the file out of syncing, as the game database's
-- settings files are. verdict 'save' keeps it syncing for good: detection never
-- takes it again, and a game database entry that named it is overruled.
-- path is relative to the main save folder, slash-separated.
CREATE TABLE settings_files (
    game_id TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    path    TEXT NOT NULL,
    verdict TEXT NOT NULL,
    reason  TEXT NOT NULL DEFAULT '',
    at_ms   INTEGER NOT NULL,
    PRIMARY KEY (game_id, path)
);

-- What another device's copy of an excluded settings file did, sync by sync:
-- moved counts syncs in which the save changed and so did that copy, still
-- those in which the save changed and it did not. A settings file is changed
-- now and then; a save that was taken for one changes with every session.
CREATE TABLE settings_observations (
    game_id   TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    peer_id   TEXT NOT NULL,
    path      TEXT NOT NULL,
    last_hash TEXT NOT NULL DEFAULT '',
    moved     INTEGER NOT NULL DEFAULT 0,
    still     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (game_id, peer_id, path)
);
