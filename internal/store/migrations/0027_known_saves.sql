-- Save folders the background scan has already seen.
--
-- Scanning used to happen only when someone pressed the button, so a game
-- installed since went unnoticed until they thought to. The daemon now scans
-- on its own and says when it finds a game that is new. "New" needs a memory:
-- without one, every untracked game on the machine — including the ones the
-- person looked at and chose not to track — would be announced every time.
-- The first background scan fills this without announcing anything; later
-- ones announce only what is not here yet, and add it.
--
-- Keyed by the folder in the store's normalised form (see
-- normalizeLocationPath), so the same folder spelled two ways is one row.
CREATE TABLE known_saves (
    path       TEXT PRIMARY KEY,
    name       TEXT NOT NULL DEFAULT '',
    first_seen TEXT NOT NULL DEFAULT ''
);

-- Whether the daemon looks for newly installed games in the background.
ALTER TABLE settings ADD COLUMN detect_new_games INTEGER NOT NULL DEFAULT 1;
