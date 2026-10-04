-- Which version of a game's save this device holds, as a version vector, and
-- whether it knows of a newer one it has not finished taking yet (see
-- internal/p2p/syncengine/version.go).
--
-- vector is a JSON object of writer key to counter. A writer key belongs to
-- one device for one game (key below), and its counter moves only when the
-- save really changes there — the game or the user wrote it, never a sync.
-- Comparing two vectors says, without guessing from files or clocks, whether
-- one device is simply behind the other or both changed independently.
--
-- target is the newest version this device has heard of that is newer than
-- its own, '' when none. While it is set this device is outdated: it takes
-- the save only from a device holding that version, and is a source for
-- nobody — its files are not handed out, and what it lacks is not read as
-- deleted.
--
-- counter is this device's own counter for the game. It never goes back, even
-- when the device adopts another's version, so the same key and number never
-- name two different saves.
--
-- pulling is set while a pull towards target is under way, so the files it
-- writes are not mistaken for a new local version if the app stops half-way.
--
-- hash is the save's content hash when vector was last set ('' unknown). Two
-- devices on one version holding different files means a change went
-- unnoticed on one of them; this says which.
--
-- answered is the other device's version this one kept its own save over when
-- someone answered a conflict here ('' none), and answered_at when (Unix ms).
-- It travels with the version, so the other device is asked in turn rather
-- than overwritten, and this one is not asked again.
CREATE TABLE game_versions (
    game_id  TEXT PRIMARY KEY REFERENCES games(id) ON DELETE CASCADE,
    key      TEXT NOT NULL,
    counter  INTEGER NOT NULL DEFAULT 0,
    vector   TEXT NOT NULL DEFAULT '{}',
    target   TEXT NOT NULL DEFAULT '',
    pulling  INTEGER NOT NULL DEFAULT 0,
    hash     TEXT NOT NULL DEFAULT '',
    answered TEXT NOT NULL DEFAULT '',
    answered_at INTEGER NOT NULL DEFAULT 0
);
