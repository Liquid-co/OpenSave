-- The registry keys a game keeps save data in.
--
-- 430 games in the Ludusavi manifest declare a save-tagged registry key, and
-- for 303 of them it is the only place a save exists. Those games could not be
-- backed up at all: a file-based snapshot has nothing to capture, and until now
-- they were dropped from the scan index entirely for having no file paths.
--
-- A table rather than a column on games, because a game can declare several
-- keys and because a key learned from a peer has to be storable before this
-- device has ever seen the game's manifest entry. One row per key.
--
-- The path is stored exactly as its source wrote it — full hive name, forward
-- slashes, as the manifest does — and normalised at the point of use. Rewriting
-- it on the way in would make a key learned from a peer compare unequal to the
-- same key read from the manifest, which is how a game ends up capturing the
-- same save twice under two spellings.
CREATE TABLE IF NOT EXISTS game_registry_keys (
    game_id   TEXT NOT NULL,
    key_path  TEXT NOT NULL,
    PRIMARY KEY (game_id, key_path),
    FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_game_registry_keys_game
    ON game_registry_keys(game_id);
