-- Whether each snapshot's archive was last found whole.
--
--   checked_ms  when it was last read back in full, every file's checksum
--               compared; 0 for never.
--   problem     what was wrong then — missing, unreadable, a file whose
--               contents no longer match its checksum — or empty when it was
--               whole.
--
-- A backup is only as good as the day it is needed, and nothing else reads a
-- snapshot until then. Checked daily in the background, and before every
-- restore (see internal/snapshot/verify.go).
ALTER TABLE snapshots ADD COLUMN checked_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE snapshots ADD COLUMN problem TEXT NOT NULL DEFAULT '';
