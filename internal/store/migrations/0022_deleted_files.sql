-- Deletions this device made, recorded as facts rather than inferred later.
--
-- Until now a deletion was worked out by subtraction: a path present in the
-- shared lineage and absent from the save folder was taken to mean "deleted
-- here". That inference is only as good as the lineage, and the lineage is a
-- derived, mutable set — it is rebuilt from the intersection of the two
-- devices' current manifests, so a rebuild landing after a local deletion
-- REMOVES the very path that was the evidence. The deletion then reads as
-- "the peer has a file we lack", the file is pulled back, and a save the user
-- deleted reappears on the machine they deleted it from. Observed, not
-- theorised.
--
-- Syncthing does not infer. A deleted file stays in its index as a record with
-- deleted=true, an empty block list and the time of deletion, carried between
-- devices like any other entry. This table is the same idea: the deletion is
-- written down when it happens and stops depending on anything else surviving.
--
-- hash is what the file contained at the moment it was deleted, and it is the
-- safety mechanism. Syncthing pairs its records with version vectors, which
-- tell a causally-later change from a concurrent one; OpenSave has no version
-- vectors, so a record alone would be MORE dangerous than the inference it
-- replaces — it would delete on the peer with confidence and no way to know
-- whether they had meanwhile edited the file. Comparing content closes that:
-- a recorded deletion is only ever propagated to a peer whose copy still
-- hashes to exactly what was deleted. If they changed it, their bytes win and
-- we take their version instead.
--
-- That is narrower than a version vector and, for this one decision, stronger:
-- Syncthing allows a deletion to win and preserves the loser as a
-- .sync-conflict copy, whereas this can never remove content that differs from
-- what was deleted in the first place.
--
-- Keyed by (game, root, path) because a game's save can span several roots and
-- the same relative path can exist in more than one of them.
CREATE TABLE IF NOT EXISTS deleted_files (
    game_id       TEXT    NOT NULL,
    root          TEXT    NOT NULL DEFAULT '',
    path          TEXT    NOT NULL,
    -- Content hash at the moment of deletion. A peer whose copy hashes
    -- differently has edited it since, and their copy is taken instead.
    hash          TEXT    NOT NULL,
    deleted_at_ms INTEGER NOT NULL,
    PRIMARY KEY (game_id, root, path)
);

-- Expiry sweeps by age; the lookup during a sync is by game and root.
CREATE INDEX IF NOT EXISTS idx_deleted_files_age ON deleted_files (deleted_at_ms);
