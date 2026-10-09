-- A game being configured must not sync, even after a crash or a daemon
-- restart, and even if the other device has already started syncing it.
--
-- This is not auto_sync = 0. That flag only stops this device from starting
-- a sync. A paired device that already has the game still asks for the
-- manifest and can pull or push files. Pause is in memory and dies with the
-- process. Neither is a place to leave a half-configured game.
--
-- A table, not a column on games. Older builds read games with SELECT * and
-- a new column breaks them; a table they do not know is ignored. No row
-- means the game is not held, which is every game that existed before this
-- migration.
CREATE TABLE game_provisioning_holds (
    game_id TEXT PRIMARY KEY REFERENCES games(id) ON DELETE CASCADE,
    held_at TEXT NOT NULL
);
