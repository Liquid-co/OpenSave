-- When each game last forgot what it agreed with its peers: at every untrack
-- (store.ForgetGameSyncState).
--
-- The lineage, the merge bases and the push records are deleted then, but the
-- game's snapshots are kept, for restoring. The sync engine also reads those
-- snapshots as proof that this device held a peer's save (syncengine,
-- HeldState) — and one taken before the untrack proved it for a folder that
-- may have changed since, outside any tracking. A peer then took this device's
-- old save as the shared starting point, read a file lost while untracked as a
-- deletion, and deleted its own copy. Snapshots from before this moment no
-- longer count as proof.
CREATE TABLE IF NOT EXISTS game_sync_forgotten (
    game_id      TEXT PRIMARY KEY,
    forgotten_ms INTEGER NOT NULL
);
