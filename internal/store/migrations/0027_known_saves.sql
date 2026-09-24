-- Save folders the background scan has already seen.
--
-- Scanning used to happen only when someone pressed the button, so a game
-- installed since went unnoticed until they thought to. The daemon now scans
-- on its own and says when it finds a game that is new. "New" needs a memory:
-- without one, every untracked game on the machine — including the ones the
-- person looked at and chose not to track — would be announced every time.
--
--   path       the folder in the store's normalised form (see
--              normalizeLocationPath), so one folder spelled two ways is one
--              row; for comparing, never for showing
--   save_path  the folder as the scan found it, for showing
--   pending    announced and not yet looked at. Kept here rather than in
--              memory: a game found while the window was closed, and the app
--              quit before anyone opened it, would otherwise be remembered as
--              known and never mentioned at all.
CREATE TABLE known_saves (
    path       TEXT PRIMARY KEY,
    save_path  TEXT NOT NULL DEFAULT '',
    name       TEXT NOT NULL DEFAULT '',
    app_id     TEXT NOT NULL DEFAULT '',
    first_seen TEXT NOT NULL DEFAULT '',
    pending    INTEGER NOT NULL DEFAULT 0
);

-- Whether the first background scan has taken stock. Its own row, not "has
-- anything been remembered": on a machine with no saves yet the first scan
-- remembers nothing, and the first game ever installed there would then have
-- been taken for the stock-taking and never announced.
CREATE TABLE new_games_state (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    stock_taken TEXT NOT NULL
);

-- Whether the daemon looks for newly installed games in the background.
ALTER TABLE settings ADD COLUMN detect_new_games INTEGER NOT NULL DEFAULT 1;
