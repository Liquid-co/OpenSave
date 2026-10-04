-- One-time steps this device has already taken, by name, with the value they
-- were taken for. A step whose value changes in a later build runs again.
-- Used for delta.NeverSyncedList: when the files no save is made of change,
-- the hashes recorded under the old list are re-taken once (daemon.go).
CREATE TABLE marks (
    name  TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
