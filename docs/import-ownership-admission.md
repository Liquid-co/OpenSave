# Backup import ownership admission

Untracked imports reserve native save-path ownership before touching the
restore destination. The reservation remains held through extraction,
authoritative registration, and any returned-error recovery. Recovery does not
roll back files after registration has committed. This is not a durable
filesystem transaction or a guarantee of recovery after process or power loss.

Ordinary game creation (including peer auto-registration), primary-path updates,
and mapped-root additions/remaps cooperate with the same cross-process claim.
They wait up to 30 seconds, checking every 100 ms, before returning
`ErrPathClaimBusy`. This matches the bounded-wait policy for native topology
admission. The bound covers acquisition, not the subsequent operation. Native
ownership preparation and the SQLite commit are both inside the claim, with
filesystem resolution outside the SQLite transaction. Imports still refuse
immediately when admission is busy.

Busy is distinct from `ErrGamePathOwned`: contention does not establish an
ownership collision. Interactive registration and path-edit APIs return HTTP
409 for busy admission. Their request contexts cancel the wait before mutation;
legacy Store callers without a request context retain the bounded default.
Real ownership collisions are checked again after acquisition and still fail.

## Native lock identity

On Unix, the persistent sidecar is `<resolved-database-path>.path-claim.lock`.
It must not be unlinked on release, or later callers could lock different
inodes. Symlink and relative path spellings resolve to the same sidecar,
independently of HOME or the caller's working directory. The lock implementation
never opens/closes the SQLite file: closing a separate descriptor to that inode
can drop SQLite's process-wide POSIX locks.

Unix guarded mutations refuse databases with more than one hard link. Adjacent
sidecars cannot coordinate arbitrary hardlink spellings, and SQLite itself does
not safely support independently named journals for those aliases. This replaces
the previous hardlink admission promise with explicit refusal; it does not claim
to make opening SQLite through hardlinks safe.

Windows retains its database-file-identity named mutex and dedicated pinned
owner thread. Acquisition remains nonblocking at the native layer; ordinary
callers use the common bounded waiter. Abandoned claims fail separately from
ownership collisions and ordinary contention.

Noncooperating old binaries, external tools, concurrent alias replacement and
symlink swaps remain outside this admission contract. These claims do not
provide daemon lifecycle takeover or cross-process sync topology coordination.

## Regression coverage

- 24 simultaneous disjoint registrations across two Store handles all succeed.
- A registration resumes after an import releases its claim.
- Timeout/cancellation leave no new record and do not leak the claim.
- A waiting registration rechecks ownership after the import commits.
- The interactive handler propagates cancellation and resumes after contention.
- A child-process POSIX-lock probe remains blocked after claim release on Linux.
- Cross-process, symlink, different-cwd and different-HOME claims still exclude.
- Unix hardlinked databases fail closed; Windows mutex recursion/release tests
  remain native Windows tests.
- Existing primary/named/nested import ownership and recovery regressions remain.
