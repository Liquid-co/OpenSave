-- Devices this one unpaired whose goodbye may not have reached them.
--
-- Unpairing deletes the peer's row, and the key its goodbye is signed with is
-- derived from the public key in that row. The goodbye went out once; when it
-- was lost — the other device offline, the relay socket reconnecting — the
-- other device kept this one as paired for good, because the only fallback it
-- would hear was an unsigned notice it refuses from a device that has
-- authenticated before. Keeping the public key lets the goodbye be signed
-- again the next time that device turns up still acting paired.
--
-- Only public material: the key a goodbye is signed with also needs this
-- device's own private key, which is where it always was.
--
--   unpaired_ms  when; a row older than a month is dropped, since a device
--                gone that long is not coming back to be told
CREATE TABLE unpaired_peers (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '',
    address     TEXT NOT NULL DEFAULT '',
    port        INTEGER NOT NULL DEFAULT 0,
    public_key  TEXT NOT NULL,
    unpaired_ms INTEGER NOT NULL
);
