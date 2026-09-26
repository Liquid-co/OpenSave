-- How often every snapshot is read back to check it can be restored
-- (internal/daemon/verify.go), in days; 0 is never. Once a week unless
-- changed. last_verify_ms is when the last full check finished, so a restart
-- does not start the count again.
ALTER TABLE settings ADD COLUMN verify_every_days INTEGER NOT NULL DEFAULT 7;
ALTER TABLE settings ADD COLUMN last_verify_ms INTEGER NOT NULL DEFAULT 0;
