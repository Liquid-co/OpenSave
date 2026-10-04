-- What a snapshot holds, as one value: a hash over every file's name in the
-- archive and its content hash (snapshot.ContentKey). The same files give the
-- same value on every device, so two snapshots of one save — taken here, or
-- taken on another device and recorded here as "Synced from peer" — are known
-- to be the same, and the archive is kept once. '' until computed.
ALTER TABLE snapshots ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_snapshots_content ON snapshots(game_id, branch_name, content_hash);

-- Another snapshot id that names the same content as a snapshot kept here: a
-- peer's snapshot whose save this device already has archived. Known, so it is
-- not archived again, and not asked about again on every sync.
CREATE TABLE snapshot_aliases (
    alias_id    TEXT PRIMARY KEY,
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE
);
