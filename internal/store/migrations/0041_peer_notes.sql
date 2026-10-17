-- A choice made on this device that a peer has not acted on yet
-- (internal/store/peer_notes.go). Until it has, this device's own next sync
-- must not read the peer's state as a change to take:
--   declined     the peer's save that "Keep mine" declined (its manifest
--                hash); not taken while the peer still holds exactly that.
--   left_branch  the branch this device was switched away from by hand; a
--                peer still on it is asked to follow, not followed back.
-- Kept across restarts. A note never outlives its reason: it is cleared once
-- the peer holds anything else, on untrack, on unpair, and on a further
-- switch by hand.
CREATE TABLE peer_notes (
    game_id TEXT NOT NULL,
    peer_id TEXT NOT NULL,
    kind    TEXT NOT NULL,
    value   TEXT NOT NULL,
    PRIMARY KEY (game_id, peer_id, kind)
);
