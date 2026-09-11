-- A tombstone remembers where the game's saves were on THIS device.
--
-- Untracking a game on one device removes it on every paired device, and
-- re-tracking it brings it back — by auto-tracking from the re-tracking
-- device's manifest request, which invents a local folder by translating the
-- peer's path. That is the right thing for a game this device has never seen.
-- For one it had, it is wrong: the folder it was actually syncing, with the
-- saves in it, is left behind, and a new empty folder at a guessed path takes
-- its place. The saves stop syncing and nothing says so.
--
-- With the path remembered, a re-track restores the game where it was, as
-- long as that folder still exists. The name is kept for the same reason:
-- the record it restores needs one, and the peer's spelling may differ.
ALTER TABLE untracked_games ADD COLUMN save_path TEXT NOT NULL DEFAULT '';
ALTER TABLE untracked_games ADD COLUMN name TEXT NOT NULL DEFAULT '';
