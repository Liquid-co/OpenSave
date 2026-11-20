-- When this device last made a backup file (.sscb) and how many games went
-- into it, for the Backups page to say "Last made". One row at most. The
-- file's path is not kept: it is the user's to move, and an old path would
-- read as a promise that the file is still there.
CREATE TABLE backup_file_made (
    id       INTEGER PRIMARY KEY CHECK (id = 1),
    made_ms  INTEGER NOT NULL,
    games    INTEGER NOT NULL
);
