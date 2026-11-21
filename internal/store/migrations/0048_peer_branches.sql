-- Which branch of a game each paired device was on when this device last
-- synced that game with it: what the other device's answer to the sync said
-- (syncengine, the manifest's activeBranch). For the game page to say which
-- devices are on which branch, as of that sync, and never more than that.
--
-- One row per game and device, replaced at every sync. Unpairing a device
-- takes its rows with it.
CREATE TABLE peer_branches (
    game_id  TEXT NOT NULL,
    peer_id  TEXT NOT NULL REFERENCES peers(id) ON DELETE CASCADE,
    branch   TEXT NOT NULL,
    seen_ms  INTEGER NOT NULL,
    PRIMARY KEY (game_id, peer_id)
);
