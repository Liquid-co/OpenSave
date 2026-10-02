-- Paired devices this one lets choose folders: a game such a device syncs is
-- tracked at the folder it names, translated, even where this device cannot
-- vouch for it as a save folder (internal/p2p ensureManifestGame). The user
-- turns it on, device by device, for their own; with no row a device names
-- nothing here but known save folders, and the rest are offered
-- (CVE-2026-103398).
--
-- A table of its own rather than a column on peers, so an older version,
-- which reads peers rows strictly, still opens the database.
CREATE TABLE peer_folder_trust (
    peer_id  TEXT PRIMARY KEY,
    since_ms INTEGER NOT NULL
);
