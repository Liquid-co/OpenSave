-- Play sessions of the devices this one is paired with: what each of them
-- told this one about its own sessions (p2p/playsessions.go), so the
-- Activity page can say how long a game was played on each device rather
-- than only here.
--
-- Each device sends only its own sessions, and only to devices it is paired
-- with; nothing is passed on. A row is the peer's session as it recorded it:
-- the game by the id the two share, and when it started and ended on the
-- peer's clock. Started-at identifies a session on its device, so a session
-- told twice is kept once.
--
-- No reference to games: a session can arrive for a game not tracked here
-- yet, and is there when it is. Unpairing a device takes its sessions with
-- it (ON DELETE CASCADE): what it played is its own to tell.
CREATE TABLE peer_play_sessions (
    peer_id    TEXT NOT NULL REFERENCES peers(id) ON DELETE CASCADE,
    game_id    TEXT NOT NULL,
    started_ms INTEGER NOT NULL,
    ended_ms   INTEGER NOT NULL,
    PRIMARY KEY (peer_id, game_id, started_ms)
);

CREATE INDEX peer_play_sessions_by_game ON peer_play_sessions (game_id, ended_ms);
CREATE INDEX peer_play_sessions_by_peer ON peer_play_sessions (peer_id, ended_ms);
