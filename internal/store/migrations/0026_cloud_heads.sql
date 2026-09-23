-- Where each game's save stands on this device, as the cloud mirror sees it.
--
-- The mirror used to be write-only: every device uploaded its snapshots and
-- none read the others'. Reading them safely needs to know more than which
-- snapshot is newest, because every snapshot is uploaded — including the copy
-- a device keeps of a save it is about to replace, which is new in date and
-- old in content. So each device records here which snapshot IS its save,
-- announces that in the cloud as a "head" (see internal/cloud/heads.go), and
-- reads the other devices' heads.
--
--   snapshot   the snapshot this device's save is right now
--   since      when it became so: when it was taken, or when it was restored
--   file       that snapshot's name in the cloud
--   chain      JSON list of this device's saves since it last took one from
--              elsewhere, oldest first, ending at snapshot. Another device
--              whose own save is in this list knows taking ours continues it.
--   published  the snapshot this device last announced in the cloud, so a
--              restart does not announce every game again
--   dismissed  a snapshot from another device the person said "not now" to;
--              it is not offered again, a newer one is
CREATE TABLE cloud_heads (
    game_id   TEXT PRIMARY KEY REFERENCES games(id) ON DELETE CASCADE,
    snapshot  TEXT NOT NULL DEFAULT '',
    since     TEXT NOT NULL DEFAULT '',
    file      TEXT NOT NULL DEFAULT '',
    chain     TEXT NOT NULL DEFAULT '[]',
    published TEXT NOT NULL DEFAULT '',
    dismissed TEXT NOT NULL DEFAULT ''
);

-- Whether a newer save from another device is put in place without asking,
-- when it continues from the save this device has and this device has not
-- changed since. On by default, the way syncing between devices is.
ALTER TABLE settings ADD COLUMN cloud_auto_pull INTEGER NOT NULL DEFAULT 1;
