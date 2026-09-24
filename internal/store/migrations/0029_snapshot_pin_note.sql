-- A snapshot can be pinned, and can carry a note of its own.
--
--   pinned  never removed by anything automatic: not by the count limits,
--           not by the age rule, not by the sweep of old conflict branches.
--           Deleting it by hand still works — a pin protects a save point
--           from the housekeeping, not from the person who pinned it.
--   note    what someone wrote about it afterwards ("good run, before the
--           boss"). Separate from comment, which for an automatic snapshot
--           is the reason it was taken and is what the app reads to say so;
--           a note written over that would lose the reason.
--
-- Both are this device's alone: snapshots are made on each device and are
-- not sent between them, and neither are these.
ALTER TABLE snapshots ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;
ALTER TABLE snapshots ADD COLUMN note TEXT NOT NULL DEFAULT '';
