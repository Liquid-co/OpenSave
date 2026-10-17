-- How fast this device has measured its connection to each paired device:
-- from transfers it made, and from a short speed test when there is nothing
-- recent (see internal/p2p/syncengine/linkspeed.go). Syncs go to the fastest
-- connections first, so a save reaches the devices close to it before it
-- crawls over a slow relay — and those can then hand it on.
--
-- bytes_per_sec is a running average; measured_ms when it last changed;
-- probed_ms when a speed test last ran, so a slow link is not tested over and
-- over.
CREATE TABLE peer_links (
    peer_id       TEXT PRIMARY KEY,
    bytes_per_sec REAL NOT NULL DEFAULT 0,
    measured_ms   INTEGER NOT NULL DEFAULT 0,
    probed_ms     INTEGER NOT NULL DEFAULT 0
);
