-- Collections: games grouped however someone likes — Favourites, "Playing
-- now", "Roguelikes". A way to find games in the library, and nothing more:
-- a collection changes nothing about how a game syncs, and each device keeps
-- its own (they are not sent between devices).
--
-- Favourites is built in: always there, first, and not renamed or deleted,
-- so a star on a game always has somewhere to go.
--
-- A game untracked leaves every collection with it (ON DELETE CASCADE).
CREATE TABLE collections (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    position   INTEGER NOT NULL DEFAULT 0,
    created_ms INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE collection_games (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    game_id       TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    added_ms      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (collection_id, game_id)
);

CREATE INDEX collection_games_by_game ON collection_games (game_id);

INSERT INTO collections (id, name, position, created_ms) VALUES ('favourites', 'Favourites', 0, 0);
