-- Games a peer syncs that this device has not been told where to keep.
--
-- When a peer asks for the manifest of a game this device does not track, the
-- default is to auto-track it: guess a local folder from the peer's own save
-- path (translated for this OS) and start syncing. That guess is what makes
-- OpenSave zero-config, and it stays the default.
--
-- It is a guess all the same, and it is wrong exactly where paths are least
-- predictable: several drives, a folder moved with a junction, a save kept in
-- a per-account directory whose name differs on every machine. For those, the
-- alternative is the one Syncthing uses — the peer tells you a game exists,
-- and you say where it lives here.
--
-- A row here is that offer, and it is deliberately NOT a game. Writing a
-- half-formed game with an empty save path would be far more invasive than it
-- looks: filepath.Clean("") is ".", so two unplaced games would normalise to
-- the same value and the "one folder, one game" check would refuse the second
-- as a duplicate of the first. The watcher, the snapshot writer and the
-- manifest builder would each need to learn to skip a game that is not really
-- there. Keeping the offer outside the games table leaves every one of those
-- invariants exactly as it was.
--
-- game_id is the PEER's id for the game, not one derived here. It is what the
-- two devices match on, and placing an offer creates the game under that same
-- id — so a game placed by hand syncs with the peer just as an auto-tracked
-- one does.
--
-- One row per (game, offering peer): several devices can offer the same game,
-- and the list is more useful saying which. Placing it satisfies all of them,
-- because what they were waiting for was a local folder.
CREATE TABLE IF NOT EXISTS offered_games (
    game_id    TEXT NOT NULL,
    peer_id    TEXT NOT NULL,
    name       TEXT NOT NULL,
    app_id     TEXT NOT NULL DEFAULT '',
    cover_url  TEXT NOT NULL DEFAULT '',
    -- The peer's own save path, kept only to show the user where the game
    -- lives on the other device. It is a hint for a human choosing a folder,
    -- never a path this device resolves or writes to.
    peer_path  TEXT NOT NULL DEFAULT '',
    first_seen TEXT NOT NULL,
    PRIMARY KEY (game_id, peer_id)
);

-- Listing offers is a per-device screen, not a per-game lookup.
CREATE INDEX IF NOT EXISTS idx_offered_games_peer ON offered_games (peer_id);

-- What to do with a game a peer syncs that this device does not track.
--
-- 'track' is the behaviour that has always existed and stays the default:
-- guess a folder from the peer's save path and start syncing immediately.
--
-- 'ask' records an offer instead and syncs nothing until a person picks the
-- folder. Worth having because the guess is least reliable exactly where a
-- wrong guess is most annoying to undo — several drives, junctioned folders,
-- per-account save directories.
--
-- TEXT rather than a boolean because this is a choice about what happens, and
-- a third answer ("ignore games from this device entirely") is a plausible
-- next one. A column named for the question survives that; one named
-- auto_track_enabled does not.
ALTER TABLE settings ADD COLUMN unknown_game_from_peer TEXT NOT NULL DEFAULT 'track';
