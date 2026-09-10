# Changelog

All notable changes to OpenSave are documented here. This project adheres to
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **Every paired device now says whether what you sync with it is
  encrypted.** Encryption over a relay depends on a key the two devices
  exchange while pairing, and until this version pairing over a relay threw
  that key away — so an encrypted pairing and an unencrypted one looked exactly
  alike, on the screen that tells you the relay cannot read your saves. Each
  device in the list now carries its own state, the room panel sums it up when
  you join, and a pairing that has no key offers the one thing that fixes it.
  Devices on your local network are shown as what they are — a direct
  connection that never touches a relay — rather than as a failure to encrypt.

  The badge is computed from the same condition the sending code uses, so it
  cannot claim a protection that is not actually being applied.

- **Self-hosting a relay now asks instead of expecting flags.** Run
  `install-relay.sh` with no options on a terminal and it walks through the
  domain name, the port, cover art and Google Drive sign-in, explaining what
  each is for and why you might skip it, then shows a summary before changing
  anything. Nothing is echoed while you type a key. Every answer is still a
  flag for anyone scripting it, and piping the script into `bash` never
  triggers the questions — stdin there is the script itself.

- **Hosting a relay from the app opens the port for you.** Ticking "host a WAN
  relay" now asks the router for a port forward over UPnP and reports the
  address it got back, instead of telling you to configure the router
  yourself. Unticking it, or quitting, withdraws the mapping — a hole left
  open by a checkbox someone has since unticked is one they would never find
  again. The mapping is taken on a one-hour lease and renewed while hosting
  continues, so a crash or a reinstall cleans itself up rather than leaving
  the port open on your router indefinitely; routers that refuse leases get a
  permanent mapping as before. Where UPnP is switched off it says so and says what to do instead;
  the relay still works for devices on your own network either way. The
  port-forwarding code had been in the project since the JavaScript port,
  reachable only from a command-line verb.


- **Change where a game's save folder is, from its Manage tab.** The folder was
  fixed once a game was tracked, and could only be moved from the command line.
  It now has a Browse button beside the App ID, refuses the folders tracking has
  always refused, and asks before moving. Files sync by their position *inside*
  that folder, so two devices can point at completely different paths and still
  hold the same save — which is what makes this useful for games that keep saves
  in a folder named after your Steam or Epic account, where the name is
  different on every machine.

- **"Ask me where to keep it" for games another device syncs.** OpenSave works
  out a folder from where the game lives on the other device, which is why it
  usually needs no setup, and that stays the default. Where the guess is least
  reliable — a second drive, a folder you moved, a per-account save directory —
  Settings can now ask instead. Games waiting for a folder appear at the top of
  the Games page with the path they use on the other device, and nothing syncs
  until you choose. The other device is told plainly that yours is waiting,
  rather than being shown the same "not tracked" state as a game you removed on
  purpose.

- **Cover art on a relay you run yourself.** The relay has been able to look
  up artwork for games Steam has no cover for since the key handling landed,
  but there was no way to install the key alongside the Google secret and
  nothing said where to get one. `install-relay.sh` now takes
  `--steamgriddb-key-file`, handled exactly as the Google secret is — read
  once, copied to a root-only file, never printed, and no value-on-the-command-line
  variant, because an argument is visible in `ps` to every user on the machine.
  Both secrets now share one env file rather than the second overwriting the
  first. `docs/RELAY.md` says where to get a key, why each relay needs its own
  rather than sharing, and how to check it arrived.

### Security

- **Updates are checked against the checksums published with them.** The only
  thing between a downloaded update and a rename over the running program was
  a size check and the first two bytes of the file — anything beginning "MZ"
  passed. The connection to GitHub is encrypted, which protects the transfer
  and says nothing about the file: an altered release asset would have been
  installed and run without a murmur. Both the app and the command line now
  verify a download against the release's `SHA256SUMS`, as the relay installer
  already did, and refuse to install anything that does not match — including
  when no checksums are published at all, because "we could not check, so we
  installed it" is not a defence.

  This is not the same as signed updates. Checksums fetched from the same
  release as the file are only as trustworthy as that release. It closes the
  likelier gap — a file altered in transit or at rest — and signing with a key
  that never touches CI is the step after it.

- **Devices on your network are identified by proof, not by address.** A peer
  was recognised by the network address it connected from, which anyone on the
  same network can take — by ARP spoofing, or simply by being handed that
  address after the real device's DHCP lease expired. Requests between paired
  devices now carry proof that the sender holds the key pinned when the two
  paired, covering the route, the method and the contents, and each one is
  single-use so a captured request cannot be replayed or re-aimed.

  As with relay sync, both devices need this version: until then a pair keeps
  working on the old check, and protection latches on the first request that
  proves itself. A pairing with no key behind it has nothing to prove with —
  unpair and pair those two again to give it one.

- **Internet sync is now end-to-end encrypted and authenticated.** Saves sent
  through a relay are sealed between your two devices, so the relay passes on
  data it cannot read — not even ours — and each request now carries proof that
  it came from the device it says it did.

  Before this, the connection to the relay was encrypted but the contents were
  not, so anyone you had given your room code to was in a position to read what
  passed through. It was never reachable from the open internet: a room code is
  twelve characters from a cryptographic generator and is not guessable, so in
  practice this meant the people you had deliberately shared a room with.

  A relay can still see that two devices are talking, roughly how much data is
  moving, and which games by id.

  **Two things worth knowing.** Both devices need this version before either
  protection applies; until then a pair keeps working exactly as before, so an
  upgrade part-way through never looks like a device that stopped talking.

  And **pair your devices again over the internet once both are updated.**
  Encryption needs a key that the two devices exchange while pairing, and
  pairing over a relay never stored one — it was sent and quietly discarded, on
  every version up to this one, so no existing internet pairing has a key to
  encrypt with. Nothing is lost by leaving it: those pairs keep syncing exactly
  as they always have. Re-pairing is simply how you turn the new protection on.
  Pairings made over a local network are unaffected and always kept their key.

  LAN sync is unchanged: it never involves a relay, but it is not encrypted and
  identifies a device by its network address, so treat an untrusted network
  accordingly.

### Fixed

- **A save deleted right after it synced no longer comes back.** OpenSave
  keeps a record of which files both devices have held, and "in that record
  but missing on one side" is how it knows the missing side deleted a file
  rather than never having had it. That record was rebuilt after every
  transfer from a fresh look at the folder — and a file deleted in the moment
  after it arrived was already gone from the folder by then, so the rebuild
  erased the one entry that proved it had been shared. The next sync saw the
  other device's copy as something new and pulled it back, undoing the
  deletion with no sign anything had happened. On Linux this hit five times in
  six.

  An entry now stays in the record until the deletion has reached both sides.
  And for the case where a device deletes a file and the other device creates
  a new one under the same name before that propagates, the newer file wins
  rather than being deleted: the record of what was removed says so, and it is
  now trusted over the older evidence.

- **Typing a Steam App ID now tells you whether it's right.** As you enter
  one, the field checks with Steam and shows the store title it belongs to —
  or says that no game has that number, or that Steam couldn't be reached from
  this network. Before, the only feedback was whether a cover eventually
  appeared, and a cover that doesn't appear looks the same whether the number
  was mistyped, the game has no art, or the network blocks Steam's CDN. That
  is how "I added the App ID but nothing happens — am I doing something
  wrong?" gets asked, with no way to answer it from the app.

  Changing an App ID also clears the app's memory that art for that ID was
  missing. A cover that failed to load once was remembered as absent for six
  hours, so correcting a number a minute later — or the network coming back —
  changed nothing until then. Reported by mufaaf.

- **A save rewritten twice within one clock tick no longer keeps its old
  hash.** File size and modification time identify a file's contents only if a
  second write can't leave both unchanged, and within a filesystem's timestamp
  granularity it can — a fixed-size save written twice in quick succession
  would have the first write's hash handed back for the second, and the sync
  would see nothing to do for a save that had changed. A file is now only
  remembered once it has been still for a couple of seconds, the same rule git
  applies. Caught on Linux, where the kernel's coarser file times made it
  reproducible; it had passed on Windows by luck of the clock.

- **"Start when the computer starts" now starts in the tray, as it always said
  it would.** The setting's own description promised a launch minimised to the
  system tray, and it did not: the entry it registered launched OpenSave
  exactly as a double-click does, so every boot brought the full window up over
  whatever you sat down to do. It now launches hidden, with the tray icon as
  the way back in. Entries registered by earlier versions are repaired the
  next time OpenSave runs, so there is nothing to re-tick.

  If no tray is available — some Linux desktops have none without an
  extension — the window is shown after all rather than left unreachable.
  Reported by mufaaf.

- **A watch that lost track of a folder now finds its way back.** Watching a
  folder tells you what happens next, and everything the watcher did to correct
  itself was driven by another change arriving. When the change was the thing
  that went missing — a burst big enough to drop events, a new subfolder that
  ended up watched by nobody — the watcher was left holding a picture of the
  folder that was wrong, and nothing was ever going to correct it. It stayed
  wrong for as long as the folder stayed quiet.

  That picture is what decides whether a save has changes worth protecting
  before another device's copy is applied, so a wrong one is wrong in the
  direction that loses a save. Every watched game is now re-checked on a timer
  and put right if it has drifted, so this lasts minutes at worst instead of
  indefinitely. A game nothing has touched still produces nothing.

- **OpenSave no longer stops a game from deleting its own save (Windows).**
  While a save was being read — to hash it, to send it, or to archive it into a
  snapshot — Windows would not let anything else delete that file, and OpenSave
  reads every tracked save on a timer. A game deleting a save slot at the wrong
  moment got an error from an operation that normally cannot fail. Saves are
  now opened in a way that permits it.

  One related case is beyond our reach: Windows refuses to let any program
  replace a file that is open, whatever we ask for, and some games save by
  writing a new file and renaming it over the old one. The defence there is
  reading less — the caching added this release means unchanged saves are not
  opened at all, which is now the main reason it exists.

- **A game whose watcher failed to start was never watched again.** Starting a
  watch happened once — when a game was tracked, or when the daemon started —
  and a failure was only written to the log. A save folder on a drive that
  mounts a few seconds after login, a folder briefly held by another program,
  a passing permission error: any of those left that game watched by nobody
  for the rest of the session, with no auto-snapshots and no syncing on
  change. Nothing said so, because a watch that does not exist raises no
  events to reveal its absence. The watch set is now reconciled every minute,
  which starts anything missing and leaves existing watches alone.

  The same shape, one level down: a new subfolder is put under watch when its
  creation event arrives, and if that registration failed the folder stayed
  invisible. Failures are now remembered and retried on the next pass.

- **Memory with a large library.** Watching a save folder costs 64 KB per
  folder, and OpenSave watches every subfolder of every game it tracks, so a
  library of a few hundred games was holding hundreds of megabytes in event
  buffers before a single save had been read. That buffer is now 8 KB, which
  is still around a hundred events in flight for one folder — save folders are
  not high-event places.

  Two things made that safe to do. Dropped events used to be discarded in
  silence: every watcher error was ignored, including the one that means
  "events were lost", so a missed change stayed missed. An overflow now
  triggers a rescan, which finds whatever the lost events would have said.
  And the hash cache, which was a fixed 64 MB sized for an ordinary library,
  now scales with how many games are tracked — too small a cache evicts
  entries it is about to want and quietly goes back to re-reading saves.

  Reported by someone tracking 350+ games across three machines, whose drive
  was audibly busy at idle.

- **"Sync stalled" warnings appeared at random.** Two identical sweeps ran on
  the same thirty-second interval looking for stuck syncs, and only one of
  them raised the warning — so whichever fired first decided whether you were
  told. The duplicate is gone.

- **Configuring a self-hosted relay's secrets could silently do nothing.**
  `install-relay.sh` runs the relay as its own `opensave-relay` account, but
  `opensave-relay setup` is run with `sudo` and wrote the secrets file owned by
  root and readable only by its owner — so the service could not read it. The
  key was stored, `opensave-relay config` reported it as configured, and the
  relay went on answering "no key configured". There was nothing to pull on.

  Setup now hands the file to the account the service runs as, reading that
  account from the unit rather than assuming it, and says plainly when it
  cannot. `config` warns whenever the file's owner and the service account
  disagree.

  Separately, re-running the installer to add one secret used to erase the
  other — adding a SteamGridDB key to a relay that already did Google Drive
  sign-in silently removed the sign-in. It now replaces only the values passed
  on that run.

- **A relay could exhaust its SteamGridDB key and keep asking anyway, and its
  artwork cache never stopped growing.** Both matter for the same reason: one
  key serves everybody using that relay.

  A rate-limited key produced no backoff at all — every client's every miss
  became another request against a service already refusing them, which is how
  a brief limit becomes a long one. A relay now stops asking when SteamGridDB
  replies 429 or 503, honouring `Retry-After` where one is sent and capping it
  at fifteen minutes so a stray header cannot disable artwork for a day. A
  paused lookup is never recorded as "this game has no art", which would have
  blanked a cover for hours because of a momentary limit.

  The cache ignored expired entries on read but never removed them, so every
  distinct game name anyone ever scanned stayed resident for the life of the
  process — against a service unit that caps the relay at 512 MB. It is now
  bounded, dropping expired entries first and only then the oldest live ones.

  `/health` reports both, as `steamGridCached` and `steamGridPausedFor`, since
  neither failure is visible from the outside otherwise.

- **A scan location you added yourself was barely looked at.** Adding a folder
  under Settings and running a scan checked neither the folder itself nor
  anything below its immediate children, so the usual outcomes were "it found
  nothing" or "it offered my whole game install".

  Three causes, all in the one branch that handles user-added locations:
  the folder you added was never a candidate itself, only its children — so
  pointing straight at the folder your saves are in found nothing at all;
  children were listed exactly one level deep; and the result was never
  narrowed to where the saves actually are, which every other part of the
  scanner does. A games library therefore proposed whole installs.

  Measured against a real Steam library on a second drive: adding the library
  folder offered 17 game installs, two of them over 100 GB, and neither of the
  two real save folders inside them. It now finds
  `Batman Arkham Knight\BmGame\SaveData` and `GarrysMod\garrysmod\saves`,
  and pointing directly at either of those folders now works too.

  A folder holding other folders is still not offered as a save itself, so
  adding a games directory does not propose syncing the directory; and where
  nothing inside looks like a save folder, the game folder is still offered as
  before — a container you can correct beats nothing. The search below each
  child reuses the existing bounded walk, so it cannot turn a scan into a
  full-disk crawl.

- **Cloud backups made on one device could not be restored on the other.** A
  game's id is the slug of its display name, and a backup is stored as
  `<id>__<branch>__<snapshot>.zip`. Track the same title by auto-scan on one
  device and with **Track folder** on the other and the two names differ, so
  the ids differ, so the provider ends up holding two differently named sets of
  files — one of them shown under a bare slug, because the other device's id
  matches nothing locally. Each device would only restore its own.

  Linking the two under **Manage → Linked Copies** looked like the answer and
  did nothing, which is what made this baffling rather than merely awkward:
  links were resolved on the peer-to-peer sync path and nowhere in the cloud
  screens. They are now resolved there too. A linked game lists and restores
  the other device's backups, and the browse screen shows one game instead of
  two.

  Only ids you have actually linked are accepted — an unlinked backup is still
  refused. Removing a game's cloud copies on untrack was deliberately left
  alone: a linked id is another device's name for a title it is probably still
  tracking, and deleting its backups because this device stopped following the
  game would be silent data loss somewhere nobody was looking.

- **Constant disk activity and high memory while completely idle.** With a
  paired device online, OpenSave re-read and re-hashed *every byte of every
  save file* roughly every twenty seconds, whether or not anything had
  changed — once to answer the other device's ping, again for the sixty-second
  reconcile, and again for every manifest the peer asked for. None of it was
  visible, because the reading happens *before* the comparison that finds
  nothing to do: the app truthfully reported "no syncs running" while keeping
  a hard drive busy indefinitely. Reported as a machine slowing down for
  everything else, with the drive audibly working and hundreds of megabytes
  resident, all of it clearing the moment OpenSave was killed.

  The memory was not a leak. Each pass built a fresh block list for every file
  and dropped it moments later, and that churn keeps Go's heap target high
  while the runtime returns pages to the system lazily.

  File hashes are now remembered and reused while a file's size and
  modification time are both unchanged, so an idle folder costs a directory
  listing instead of a full read. Turning off auto-sync did not avoid any of
  this, incidentally — the ping path hashed every tracked game regardless — so
  quitting the app was the only workaround.

  Reusing a hash is only safe if nothing can change a file without the app
  noticing, so: anything OpenSave itself writes into a save folder drops that
  folder's cached hashes outright rather than reasoning about which files it
  touched; a filesystem event does the same; and every entry is re-read from
  scratch after an hour regardless, which is what catches a program that
  rewrites a file while preserving its size and timestamp. That last case is
  the one a size-and-time check cannot see, and it is why the periodic re-read
  exists rather than being optimised away.

- **A deleted save could come back, even hours later.** Deleting a save was
  never written down — it was worked out afterwards by subtraction, from a
  record of which files both devices were known to share. That record is
  rebuilt from what the two devices currently hold, so rebuilding it after a
  deletion removed the very evidence the deletion depended on. The file then
  looked like something the other device had and this one lacked, and it was
  copied back onto the machine it had just been deleted from.

  Deletions are now recorded when they happen, along with what the file
  contained at the time. A recorded deletion is only ever applied to another
  device whose copy is byte-for-byte what was deleted — if that device changed
  the file in the meantime, its version is newer and is kept instead. So a
  deletion can propagate reliably without ever being able to remove work
  somebody else did afterwards.

  There was a second way the same thing happened: a sync decides what to do and
  then does it, so deleting a save while one was already running meant the sync
  faithfully restored the file it had been told to copy. A transfer no longer
  writes back a file that was deleted here while it was running.

  Not claimed as closed. Deleting a save in the same instant it finishes
  arriving on the other device still loses the deletion roughly one time in
  twenty-five, measured over 135 attempts. The file comes back rather than
  anything being lost, and every other timing tested propagates correctly, but
  the race is narrower now rather than gone.

- **A save deleted just after syncing could come back.** A file only counts as
  deleted once both devices have recorded holding it, and that record was
  written after a round trip to the other device. Deleting inside that window
  read as "the other device has a new file" and pulled it back. The device that
  received the files now reports which ones, so the record is written
  immediately. Measured while the machine was busy: four deletions in twenty
  were lost before, none after.

- **OpenSave could refuse to close.** Stopping a save folder's watcher waited
  for it without limit, and if a game created a subfolder at that moment the
  wait never ended — the window stayed open and the process had to be killed.
  Captured from a real hang, not theorised.

- **A junction or symlink could be used to track a folder that is off limits.**
  Pointing a game at your home or Documents folder was refused; pointing it at a
  link to the same folder was not. That matters most for restoring, which
  empties its target first. The same hole let one folder be tracked as two
  separate games, giving it two watchers and duplicate snapshots.

- **One badly-named file no longer stops a whole game syncing.** Names that are
  ordinary on Linux and macOS — a `?`, a `*`, a trailing dot — cannot exist on
  Windows, and the first one encountered aborted the entire transfer, so nothing
  else arrived either. Those files are now skipped and reported by name. A colon
  was worse than an error: Windows accepted the write and put the contents
  somewhere the folder never shows.

- **macOS devices now agree with Windows and Linux about accented filenames.**
  macOS stores `café.sav` as `e` plus an accent mark; everyone else stores it as
  a single character. Neither system treats the two as the same file, so a save
  synced from a Mac never matched the copy already there and the devices could
  not converge.

## [2.3.1-beta.1] — 2026-08-22

Three things reported within a day of 2.3.0, and none of them lost anything —
which is why they were hard to spot from the outside. A save that was applied
correctly, a rule that was in force, a sign-in that was configured properly:
in each case the work happened and the screen said otherwise.

### Fixed

- **Files that shouldn't sync stay on screen after you save them.** Setting
  exclusions on a game, saving, then reopening Configuration showed an empty
  box. The rules were never lost: they were stored, and every sync applied
  them. Only the reply the screen reloads from left the field out, so it read
  as blank while working exactly as set. The natural response is to type them
  again, and each re-save wrote the same correct value while still looking
  like it had failed. Reported on SteamOS.

- **Your own Google OAuth app works with only a client ID entered.** Supplying
  your own credentials is what we recommend to anyone hitting Google's
  verification limits, and it failed in the configuration people were most
  likely to try. Entering a client ID without a secret sent that ID to our
  relay to be paired with *our* secret — the only one it has — and Google
  refused the mismatch. The failure surfaced as a token error naming nothing
  you could act on. Your own client now always goes straight to the provider,
  and a Google client ID saved without its secret says so, and where to find
  it.

- **A sync no longer fails to repair a merge-base it could have repaired.**
  After pushing, this device records what the peer was handed so a later sync
  can prove the push landed. It was recording its own save as it stood at the
  end of the sync instead — for a game that writes while it runs, a state the
  peer was never offered and could never be seen holding. Nothing broke
  visibly; a recovery simply never ran, and a merge-base stranded by a lost
  confirmation stayed stranded.

- **Upgrading a self-hosted relay actually replaces the running one.** The
  installer starts the service if it is stopped, which does nothing to one
  already running — so an upgrade wrote the new binary, reported success, and
  left the old one serving. It now restarts a running relay, and reports the
  version being served rather than the one just written to disk.

### Added

- **The app says what a release is about, not just what changed in it.** The
  What's New screen listed the bullets and dropped the summary above them, for
  every release. That summary is the part that explains what the rest is for.

## [2.3.0] — 2026-08-21

Mostly about saves that were never in one folder to begin with. A game whose
save is split across several places — a save folder here, settings under
Documents, a profile in AppData — is one game again, and every location travels
with it through sync, snapshots, restore, backups and conflicts. Alongside it,
files that should never sync can be excluded per game, written like a
`.gitignore`.

The auto-scan was rebuilt around the same problem: it now says what is in each
folder before you commit to it, hides the empty ones, and shows one tile per
game instead of one per folder. Self-hosting a relay is a single command. And
there is a guide for somebody who has never opened the app.

The other half of the release came out of going looking for what was quietly
wrong rather than waiting to be told. A save is now copied before anything
overwrites it, instead of the risk being estimated from a record that several
parts of the app were expected to keep current and none of them did. Room codes
were short enough to enumerate. A relay address that would have put save files
on the wire readable was accepted without comment. The check meant to stop two
save locations owning the same file did nothing at all on Linux. And what the
documentation said a relay could see was not what a relay can see.

### Thanks

- **u/enigmacarpc** — tested this release through its betas, and stayed with
  each problem until it was actually understood rather than merely closed.

### Security / safety

- **A save is snapshotted before anything overwrites it.** Applying a peer's
  changes could replace or delete local files with no copy kept, guarded only
  by a heuristic: before pulling, the engine asked whether the save held
  anything no snapshot had captured, and refused to sync if so.

  That question was too broad and its answer unreliable. Too broad because it
  asked whether *anything* was arriving rather than whether anything was
  *leaving* — a pull carrying only files this device had never held can destroy
  nothing, yet was refused because some unrelated folder had been edited. That
  is a config folder holding a save folder hostage, which per-location lineage
  exists to prevent. Unreliable because the record it consulted was only ever
  written by the watcher's automatic snapshot: a snapshot taken by hand did not
  update it, and neither did a sync, so it was as likely to be stale as
  accurate.

  Now the files a sync would actually destroy are identified — those a pull
  writes over, and those a peer's deletion removes — and a snapshot is taken
  before it proceeds. Deletions are included deliberately: they take the local
  copy just as thoroughly as an overwrite, and are easier to be wrong about,
  the file being gone rather than replaced with something recognisable.

  If that snapshot cannot be taken, **the sync stops** rather than guessing. A
  full disk or a missing backups folder is visible and fixable; a save
  overwritten with no copy behind it is neither.

- **Room codes are ~60 bits instead of ~19.** Two words from a list of eight
  plus four digits is 576,000 possibilities — small enough to enumerate the
  entire keyspace rather than guess at it — and they came from `Math.random()`,
  which is not a cryptographic generator. Anyone holding a room code learns
  each device's name, its type, and the games it tracks, and can send pairing
  requests; the guide already said to treat one like a password, and now that
  is true. Twelve characters from a 32-symbol alphabet, from the platform's
  secure generator, with the shapes people mistype left out. Existing codes
  keep working.

- **A relay that would carry saves in the clear is refused.** Nothing in
  OpenSave encrypts the sync payload, so `wss://` is the only thing between a
  save file and the network it crosses — which makes "ws or wss" a security
  decision rather than a preference, and nothing checked it. Refused now at the
  settings screen, at `config set relay-url`, and at the connection itself,
  that last one because `OPENSAVE_RELAY_URL` passes through neither of the
  others. Still allowed where the network is the trust boundary: a LAN, a
  private overlay such as Tailscale, or this machine.

- **What the relay can see is described accurately.** The guide said a relay
  "cannot read your saves". It can: there is no end-to-end encryption, the
  encryption ends *at* the relay, and the process handles save data in the
  clear. It stores none of it, which is the part that was true. Corrected
  wherever it appeared, alongside the honest consequence — whoever runs a relay
  is being trusted with what passes through it, which is the real argument for
  running your own.

- **One missed ping no longer means a device has gone.** A single failed probe
  marked a paired device offline, and a probe is a three-second round trip — so
  a busy machine, a brief wifi drop, or a laptop that suspended for a moment
  was enough. The device showed as offline while sitting on the same desk, and
  a sync started in that window failed with "no online peers available". Three
  consecutive failures now; a single reply restores it immediately.

- **Overlapping save locations are refused on every platform.** The check that
  stops two locations owning the same file compared paths using the host's own
  conventions, so a Windows-style path was only understood on Windows. On Linux
  it recognised no overlap at all and let every one through — and overlapping
  locations fight: the same file lands in two manifests, each sync patches it
  twice, and a deletion propagated for one is pushed back by the other.

### Changed

- **The public relay is `relay.opensave.org`, on hardware the project rents.**
  Internet sync went down twice in a fortnight. Both relays were free tiers,
  and both were switched off for exhausting an allowance: a relay holds
  connections open and never idles, so it spends a month's quota simply by
  existing. That was never a hosting accident to be waited out — it is what
  that arrangement does to this kind of service, and a third free tier would
  have ended the same way.

  The address now belongs to the project rather than to a host. Moving
  machines from here is a DNS change nobody has to notice, which is the point:
  each of the last two moves cost an emergency release and a banner asking
  every user to retype a setting by hand.

  **A relay address you set yourself is left exactly as it was.** Only installs
  still holding one of the three abandoned addresses are moved — including the
  one that was never a shipped default, but went out in a website banner during
  the second outage. Anyone who followed that banner is carried over too, since
  otherwise the people who did what we asked would have been the only ones left
  behind.

  Google Drive sign-in moves with it: the relay proxies that step, which is why
  sign-in failed during both outages alongside pairing.

### Fixed

- **The relay installer no longer hands out an address the app refuses.** It
  printed the machine's public address with a `ws://` scheme and told you to
  paste it in — which the client now declines, so a correctly installed relay
  looked broken. It offers an unencrypted address only where the network is the
  trust boundary, and on a hosted server prints none at all, saying why.

- **The daemon's port can be changed and stay changed, and a clash says what
  to do.** `daemon start --port` only ever applied to one run, the field in
  the window is no use on a headless box, and there was no `config set` key —
  so a machine where 8383 was already taken had no durable way off it. There
  is now `opensave config set port <n>`.

  `--port auto` also does what it looks like. `--port 0` used to be
  indistinguishable from not passing the flag at all, because 0 was the
  internal "not given" sentinel, so asking for any free port silently started
  on the configured one instead. And a clash printed the raw bind error and
  nothing else; it now says a second OpenSave is the usual cause and lists the
  three ways out.

- **`opensave-relay` no longer ignores its arguments.** It accepted anything
  you typed, discarded it, and started a server. Somebody self-hosting ran
  `opensave-relay config set relay-url wss://...` — reasonable, since that is
  roughly what the client command looks like — and got `bind: address already
  in use`, because the arguments went nowhere and the only thing left to do
  was start a second relay beside the one already running. The error described
  neither what they asked for nor what was wrong with it. It now recognises
  that shape, says `relay-url` is a client setting and where it belongs, and
  exits non-zero. `--help` and `--version` work too, and a port clash on
  startup now says what a port clash usually means.
- **OneDrive's setup is where you hit the problem, not three screens away.**
  It ships with no OAuth credentials — Microsoft does not allow a shared
  public app — so it needs your own before it will connect at all. A client-ID
  box did exist, under Settings → Sync, in a row of three unlabelled inputs;
  but the failure told you to look under Settings → Cloud Backup, which is a
  different tab, and nothing said what to create or where. Selecting OneDrive
  now opens the field in place, with a link to the Azure portal and the
  redirect URI to register.
- **Your own OAuth app, set beside the provider it belongs to.** The client-ID
  inputs have moved out of Settings → Sync and into Cloud Backup, under **Use
  your own OAuth app**, next to the provider you are connecting. They also now
  take the client SECRET, which the daemon has always read and nothing could
  set; and changing an id while you are signed in now signs you out, because
  the tokens were issued to the previous app and no refresh of them would be
  accepted — leaving them in place showed "Connected" over credentials that
  could not work.

  For Google Drive this is the real fix for the weekly re-login: the built-in
  credentials belong to a shared app still in testing, and consent expires on
  a timer. Your own client id does not.
- **`opensave scan --json` no longer ignores the flag.** It printed the
  formatted listing and exited 0, which is the worst way to not support
  something: a script piping it to `jq` got a parse error rather than an
  unknown-flag message, and the manual said every command accepts `--json`.
  It now emits the results in the same order the printed listing numbers them,
  so index *n* is what `add n` tracks, with the file count, size, last-written
  time and grouping each row carries.
- **Exclusions now cover a game's extra save locations, not just its main
  folder.** A rule protected the save folder and was quietly ignored
  everywhere else — so a device-specific config kept in a game's settings
  folder, which is one of the commonest reasons to have a second location at
  all, travelled to the other machine anyway. Worse than travelling: with the
  rule on one device only, that device pushed its own copy over the other's,
  destroying the very file the rule was written to protect.

  It was invisible from outside, because nothing reports a file that synced.
  The signal was internal — the guard hash has always been computed with every
  location filtered, so it left the file out while the sync carried it across.
  The two halves disagreed about whether the file existed.

  Every location now applies the rules exactly as the main folder does:
  filtered on both sides before anything is compared, filtered out of the
  lineage so a missing file is never read as a deletion to propagate, and with
  the merge base translated so adding a rule does not raise a one-off conflict.

### Added

- **The relay installer can be given a Google client secret.** A relay
  completes Google Drive's sign-in for clients using the built-in credentials,
  and the installer had no way to supply what that needs — so an installed
  relay synced correctly and failed sign-in, fixable only by knowing the
  variable's name and hand-writing a systemd override. `--google-secret-file`
  takes a path rather than the value, since an argument is visible to every
  user on the machine while the command runs, and the file is stored root-only
  and removed by `--uninstall`. Not needed for sync, and not needed at all by
  anyone using their own OAuth credentials in the app — that path talks to the
  provider directly and never reaches the relay.

- **Snapshots record what they hold, file by file.** Each one now notes the
  hash of every file it captured, written once and never revised, so "is this
  exact save recoverable?" can be answered exactly instead of inferred from a
  single whole-save value that nothing kept current. Snapshots taken before
  this have no such record and are treated as unproven, which is the cautious
  side.

- **The frontend has tests now.** It had none — not a thin suite, none — while
  the Go side had forty passing packages. That gap was not academic: of the
  frontend bugs found in this cycle, three were pure decisions sitting inside
  `.svelte` files, where the only way to exercise them was to open the app and
  look. Which folder of a game counts as the save. Whether a folder already
  tracked as its own game may be adopted as a location of another. Whether a
  truncated count reads as a floor.

  Those decisions now live in `src/lib/scan.js` and `src/lib/ignorerules.js`,
  with 31 Vitest cases against them, each one a mistake that actually reached a
  build rather than a hypothetical. They run in 8ms and are wired into CI as
  their own job, so they do not queue behind a thirty-minute race run.

  Deliberately NOT moved: anything that needs the daemon. Whether a file is
  excluded is answered by the daemon, which holds the matcher the sync engine
  itself uses — a second, nearly-right copy in the client would be wrong in the
  worst direction, telling someone a file is protected when it is not.

- **A one-command relay installer.** Self-hosting meant fetching a binary,
  writing a systemd unit, opening a port, and — for the encryption clients
  default to — a reverse proxy with the two WebSocket settings everyone
  misses. All of it identical on every machine, which makes it a script's job.
  `packaging/relay/install-relay.sh` does the lot, takes `--domain` to get a
  certificate automatically via Caddy, prints the relay URL and the exact
  client commands when it finishes, and `--uninstall` reverses it.

  It runs as root, so two things are not optional: the download is verified
  against the release's `SHA256SUMS` before anything is installed, and
  `--dry-run` prints every change it would make — the systemd unit included —
  without privileges and without touching the machine. It cannot point DNS at
  your server; that is the one step only your registrar can do, and without it
  the relay serves plain `ws://` and says so.

  Now tested on real Linux rather than only in dry run, which immediately
  found that it hung forever. It ran the freshly installed binary to report
  its version — and every relay before this release ignores its arguments and
  starts a server instead, so that call never returned. The unit file went
  unwritten and a stray relay was left listening. Installing v2.2.1, the
  version anyone running the script today would get, reproduced it exactly.
  It now reports the tag it installed, which it already knows, and does not
  execute a binary downloaded seconds earlier just to ask it a question.

  Verified end to end afterwards on Ubuntu with systemd: install completes in
  about a minute, the service is active and enabled and runs as its own
  unprivileged user, it survives a restart, two clients on that machine find
  each other through it and a save edited on one arrived on the other in about
  three seconds, and `--uninstall` leaves no service, unit, binary or user
  behind.

- **`OPENSAVE_RELAY_URL` pins the relay from the environment.** Settings live
  in SQLite, which is awkward for a machine that is provisioned rather than
  configured — a container, or an image rebuilt onto a fresh volume, where
  running a command once after first boot is not a step you get to take.
  While the variable is set it overrides the stored value on every read, so
  the window, the CLI and the sync engine all agree on which relay is in use;
  the field shows it and says where it came from, and `config set relay-url`
  refuses rather than pretending to save. Your stored setting is untouched, so
  unsetting the variable returns to it. Prefixed deliberately — a bare
  `RELAY_URL` is a name other things use, and silently redirecting someone's
  sync traffic over a collision is not worth eight characters.
- **Files that shouldn't sync can be picked from a list instead of typed.**
  The pattern box asked you to name a file you had to already know, in a
  folder you could not see, in a syntax you had to learn — and said nothing
  until the file turned up on another machine days later. **Pick from your
  save folder** now lists what is actually there, across every save location,
  each file marked *syncs* or *won't sync*.

  Ticking a file writes the pattern, anchored so it can only ever mean that
  one file. Unticking one caught by a wildcard adds an `!` exception rather
  than discarding the wildcard. The verdicts are computed by the same matcher
  the sync engine uses, on the same relative paths, and they update as you
  type — so a rule can be checked before it is trusted, rather than after.
  Patterns still matter for files that do not exist yet, like `*.log`; this
  is a way in, not a replacement.
- **A CLI guide and a relay guide.** The command reference has always listed
  what exists; neither said where a command belongs. Somebody self-hosting a
  relay had the container running and asked for the command to join the room
  from the CLI *on the relay* — a question with no answer, because the relay
  is a passive broker and joining is something gaming devices do. Both guides
  now lead with that: [`docs/CLI.md`](docs/CLI.md) opens on which machine runs
  what and which commands need a daemon, then gives worked sequences for
  pairing, internet sync, headless setup, snapshots, split saves, exclusions
  and scripting; [`docs/RELAY.md`](docs/RELAY.md) covers self-hosting, TLS,
  and the reverse-proxy settings WebSockets need.
- **A Getting Started guide**, for people who have not used OpenSave before:
  the whole thing from a fresh install, explaining each term as it arrives,
  including how to read a scan result, what to do when two devices disagree,
  and a glossary. [`GETTING_STARTED.md`](GETTING_STARTED.md). The User Guide
  stays as the reference.
- **Auto-scan says what is actually in each folder, and hides the ones holding
  nothing.** Every result now shows its file count, its size, and when it was
  last written. The last of those is the one that earns its place: the same
  game is routinely detected in three or four places at once — the Steam
  folder, wherever the launcher wrote it, and one left behind by an install
  that has moved on — and until now there was nothing on screen to say which
  was the live save. On the library this was built against, 24 of the 30 games
  found in more than one place had over two months between the freshest folder
  and the stalest.

  Folders holding no files are hidden by default, because Steam creates one
  for every game you own whether or not saves go there. That was 48 of 235
  results — a fifth of the list, all of it rows nobody could use. **Show N
  empty** in the scan toolbar brings them back, for tracking a game before it
  has saved for the first time, and `opensave scan --all` is the same thing.

  A folder that could not be read is reported as unknown, never as empty, and
  is never hidden. Measuring can fail — an unreadable subfolder, a path the
  walk chokes on — and a folder we failed to look inside is exactly the one
  that must stay on the list.
- **Auto-scan shows one tile per game, not one per folder.** A game found in
  several places used to produce several rows, scattered through the list
  rather than adjacent, because each came from a different detection pass.
  They are one tile now, with **found in N folders** underneath; opening it
  labels each folder with what it is.

  The labels matter more than the collapsing, because the duplicates are not
  one thing. A folder **inside** another is the same files seen twice, and
  cannot be tracked separately at all — two locations over one set of files
  fight over them. A folder **beside** the save folder is another piece of the
  same save, and those are now offered together as one game with extra
  locations: TrackMania's Scores, Tracks and Profiles go in with one click.
  A folder somewhere unrelated is **another copy**, usually left by an install
  you have moved on from, and is offered but never assumed.

  Two folders are only treated as one game when they share a Steam AppID, or a
  name specific enough to mean something. Rows called "Saves" or "User Data"
  are left alone: grouping on a name that could belong to anything would merge
  unrelated titles, which is the one mistake here that ends with a save in a
  folder nobody chose. When the grouping does miss a pair, ticking both and
  choosing **Track as one game** overrides it.
- **Files that shouldn't sync can be excluded per game.** Some games keep
  device-specific settings in the same folder as the save — Neva keeps
  `Config.gs` beside `Progress.gs` — and copying those to another machine can
  break the game there. The folder cannot be narrowed without losing the save,
  so the exclusion has to be per file. List them under **Configuration ->
  Files that shouldn't sync**, one per line, written like a `.gitignore`:
  names, `*` wildcards, `logs/` for a folder, `!` for an exception. Matching
  ignores case, so a rule written on a PC keeps working on a Steam Deck. Each
  device applies its own list, and a device without one is unaffected.
  **Snapshots still capture excluded files**, so a restore brings them back —
  excluding something stops it travelling, never stops it being backed up.
  From the command line, `opensave ignore <gameId> add <pattern>`, and
  `opensave ignore <gameId> test <path>` answers "would this sync?" without
  waiting to find out on the other device. Requested by RrOoSsSsOo.
  On a fleet where one device has not updated yet, the rule protects the
  updated one and leaves the other exactly as it was: the older device may
  still receive the file, because the alternative — hiding it from that
  device — would make it read the gap as a deletion and remove its own copy.
  Once both devices have the rule, the file stops travelling entirely.
- **A game whose save is split across folders is one game again.** Plenty of
  titles keep their save data in one place and their settings or mods in
  another, and the only way to cover both was to track the same game twice:
  two cards in the library, two conflicts to settle, two things to restore in
  step with each other. A game can now have as many save folders as it needs.
  Name each extra one under **Configuration -> Save locations** and it is
  synced, snapshotted, backed up and restored along with the main folder. Give
  it the same name on your other devices — the name is what the two sides
  match on, since the folder itself lives somewhere different on each machine.
  A device that knows a location's name but has no folder for it says so and
  skips it, rather than guessing where your files belong. Each folder also
  keeps its own sync history, so a settings folder both devices edited raises
  a question about the settings folder instead of holding the save hostage.
  A device on an older build is unaffected: it syncs the main save exactly as
  before and simply does not see the extra folders. Requested by tfe on
  Discord.

## [2.2.3] — 2026-08-14

The same emergency as 2.2.2, for the last time. The relay 2.2.2 moved everyone
onto was suspended a week later for the same reason, and this points the app at
one the project owns and pays for.

### Fixed

- **Internet sync works again, on `wss://relay.opensave.org`.** Both relays this
  app has shipped were free tiers, and both were switched off for exhausting a
  monthly allowance. A relay holds connections open and never idles, so it
  spends an allowance meant for services that sleep — that was never a hosting
  accident to wait out, and a third free tier would have ended the same way.

  The new address belongs to the project rather than to a host, and points at a
  rented machine. If it ever has to move again, that is a DNS record rather than
  a release: the last two moves each cost an emergency version and a banner
  asking every user to retype a setting by hand.

  Google Drive sign-in comes back with it, since the relay proxies that step —
  which is why signing in failed during both outages alongside pairing.

- **Anyone who typed in the temporary address is moved too.** During the second
  outage the website banner asked people to set the relay by hand. That address
  was never a shipped default, so it sits in settings looking like a deliberate
  choice; leaving it out of the migration would have stranded exactly the users
  who did what we asked, on a third free tier, waiting to be switched off.

  **If you set your own relay address, it is left exactly as it was.** Only
  installs still holding one of the three abandoned addresses are moved, so a
  self-hosted relay is never overwritten.

  No save was ever at risk: the relay stores nothing and writes nothing to disk,
  so anything on your machines was untouched throughout.

## [2.2.2] — 2026-08-12

An emergency release with one purpose: the public relay was suspended for
exceeding its hosting quota, and internet sync stopped for everybody using it.
This points the app at a working relay.

### Fixed

- **Internet sync works again.** The relay the app ships with,
  `opensave-relay.onrender.com`, was suspended by its host for running out of
  free quota — a relay holds connections open and so never idles, which
  consumes a month of free allowance by itself. Room-code pairing stopped
  finding devices, and Google Drive sign-in stopped too, because the same
  server proxies that step. The default is now a relay that is up.

  **If you set your own relay address, it has been left exactly as it was.**
  The change only moves installs still pointing at the suspended default, so a
  self-hosted relay is not overwritten.

  No save was ever at risk: the relay stores nothing and writes nothing to
  disk, so anything on your machines was untouched throughout.

## [2.2.1] — 2026-08-05

Mostly conflicts and backups. Three separate faults could raise a conflict on
a save the other device had never touched, a fourth could skip a conflict that
should have been raised, and a backup made from the command line could not be
restored onto a new machine at all.

### 💬 Community

- **OpenSave now has an official Discord: https://discord.gg/hvBv92DZvn** — come and say hello. It is the fastest way to get help with a save that will not sync, the right place to report a bug or ask for a feature, and where builds get discussed before they ship. If you have ever wanted to tell us what OpenSave should do next, this is where to do it.

### Added

- **Snapshots you take yourself are no longer thrown away by the ones the app
  takes for you.** Retention kept the newest few snapshots per branch
  regardless of where they came from, so in a game that saves often — Elden
  Ring, Dragonsword: Awakening — a play session's worth of automatic backups
  filled the whole allowance within minutes and pushed out the snapshot you
  took on purpose before a boss. Manual and automatic snapshots now have
  separate allowances, and **manual ones are kept forever by default**, so a
  burst of automatic backups can only ever replace other automatic backups.
  Set a limit for them per game in its Configuration tab, or for new games
  under Settings → Snapshot history; from the command line,
  `opensave game <id> set max-manual-snapshots <n>` and
  `opensave config set manual-snapshot-limit <n>`, where 0 means keep
  everything. Cloud backups follow the same rule, so your off-site copy no
  longer ends up thinner than the machine it is backing up. Existing games
  pick this up on upgrade without being reconfigured.
- **Beta builds can update to newer betas.** Installing a pre-release used to
  be a one-way door: the update check asks GitHub for the *latest release*,
  which deliberately skips pre-releases, so a beta was newer than anything it
  was offered and reported itself up to date until the final release overtook
  it — with no way forward but a manual download. Running a pre-release now
  follows the beta channel automatically, and **Settings → Updates** has a
  toggle for anyone on a stable build who wants to try what is coming. Either
  way the stable release is offered as soon as it is newer, so it is not a
  one-way door in the other direction either. From the command line,
  `opensave update` follows the same channel.
- **Tickboxes match the rest of the app.** Every checkbox and radio was the
  operating system's own square control in the operating system's own colour,
  sitting on cards built out of pills and rounded corners. They are now
  circles that fill with OpenSave's purple when you turn them on.
- **Sign in to Google Drive or Dropbox from the command line.** Cloud backup
  previously needed the desktop app to authorise it, which left a Steam Deck
  in Game Mode or a headless install unable to set it up at all. `opensave
  cloud connect`, `cloud setup`, `cloud status` and `cloud disconnect` now
  cover the whole flow, alongside pushing, pulling, listing and deleting
  cloud saves.
- **See a paired device's games, and what a conflict actually is, from the
  command line.** `opensave peers games <peerId>` lists what another device
  tracks, and `opensave conflicts` shows which files differ and on which
  side rather than only that a conflict exists.
- **Set a game's Steam App ID by hand.** Cover art and cross-device matching
  both key off the App ID, and a game the scanner could not identify had no
  way to be told. Name matching also now recognises titles whose folder name
  has lost its spaces.
- **Emulators installed on another drive are found.** The scan looked for
  each emulator's per-user data folder, which is where an *installed* one
  keeps its saves — and is on C: whatever drive the emulator itself is on.
  RetroArch, the Citra forks (Azahar, Lime3DS) and the yuzu forks (Eden,
  Suyu) are all commonly unzipped somewhere instead, and then keep their
  saves inside that folder, so anyone with their emulators on D: got nothing
  from a scan at all. Those installs are now recognised where they actually
  live: at the top of any internal drive, or inside a folder named for the
  collection ("Emulators", "Emulation", "Games"). Adding the folder as a
  custom scan path works too, and now offers the emulator's *save* folders
  rather than the whole install with its cores, BIOS and ROMs. Reported by
  Erakodo on Windows 11.
- **Dismiss a save location from the scan results.** A stale or wrong
  location can be excluded permanently instead of being offered every time
  you scan.
- **Linking a game to its copy on another device offers that device's
  games.** Linking previously meant knowing and typing the other machine's
  id for the game, which is not something anyone has to hand.
- **The Steam Deck plugin ships with every release.** It had to be built from
  source before, which is not much use on a Deck.
- **Retention limits appear in `opensave status --json`.** A headless install
  had no way to confirm what it had just set.

### Fixed

- **The conflict screen showed each save's total size instead of what differed.** The two panels reported how many files and bytes each side held altogether, which on a save where one file changed is the same number twice and settles nothing — the only account of what actually differed was the collapsed list underneath. Each panel now leads with what differs on that side, and how many files exist only there, with the whole-save totals kept underneath as context. A side with an empty save folder said "0 files · 0 B", which reads as a panel that failed to load rather than as a save that is missing; it now says so in words.
- **Conflicts on saves the other device had never touched, after sending it a
  change.** Each device remembers the last state both were known to share, and
  judges a conflict by whether both have since moved away from it. After
  sending a change, that shared point is only updated when the other device
  confirms it caught up — and that confirmation travels the network, where it
  can be lost. When it was, the shared point stayed frozen at a state neither
  device held any more, and once it is behind both of them, *every* later
  edit reads as both sides having changed. The next ordinary save prompted a
  conflict on a file the other device had not opened. The state handed over
  is now recorded, so the next sync can prove the change landed by seeing the
  other device holding exactly it, with no confirmation needed.
- **The same thing after the other device deleted a file.** A deletion is
  applied directly — the file is removed and that is the end of it — so no
  sync runs on the receiving side and nothing updated its record of the
  shared state, which went on describing a save that still contained the
  deleted file. It corrected itself whenever some later sync happened along,
  which is why this only showed up as an occasional conflict long after the
  deletion. The record is now brought up to date when the deletion is
  applied.
- **A conflict that should have been raised could be skipped, overwriting the
  other device's copy without asking.** The record of what was last sent was
  kept after it was no longer relevant, so if the other device later returned
  to that exact state — rolling back to one of its own snapshots does
  precisely that — it looked unchanged, and this device pushed over the
  rollback instead of stopping to ask. The record is now cleared as soon as
  the two devices are known to agree on something newer.
- **A backup made from the command line could not be restored onto a new
  machine.** `opensave backup export` with no games named fell back to an
  older archive format that stores snapshot files and nothing else — no
  names, no save locations — so restoring it onto a fresh install matched
  nothing, restored nothing, and said it had succeeded. It now writes the
  same archive the desktop app does. Restoring one also tracks the games it
  contains, instead of putting the files back and leaving the library empty.
- **A save folder containing a single file was restored one directory too
  high.** Restoring decides whether a save is a folder or a single file by
  looking at where it is going, and on a machine that has never held that
  game there is nothing there to look at. A folder holding one save — which
  is most of them — is indistinguishable from a save that *is* one file, and
  the file was written into the parent directory: the save came back, one
  level above where the game reads it, reported as restored. Backups now
  record which of the two it was.
- **`opensave add` never took the first snapshot it promised.** Tracking a
  game snapshots whatever is already there, in the background so the desktop
  app stays responsive. From the command line the process exited before that
  finished and took the snapshot with it, so every game tracked from the
  command line started with no history at all — the one snapshot most worth
  having, of the save before you started playing.
- **Changes made from the command line did not reach a running app.** Adding
  a game, removing one, or turning auto-sync off while the desktop app was
  open left the app unaware: the game was tracked but watched by nobody, so
  it got no automatic snapshots and no automatic sync until the app was
  restarted, with nothing on screen to say so.
- **`opensave backup export` never said what it had captured.** It read a
  count the server does not send, so every backup — full or empty — reported
  the same bare success. Importing one was equally silent about how much came
  back, or that nothing had.
- **Switching to a branch you had just created emptied the save folder.**
  Creating a branch recorded the name and nothing else, so the new branch had
  no state — and switching cleared the save location and then found nothing to
  put back. Every file disappeared, with no warning, which is indistinguishable
  from having lost the save. (It was recoverable: the outgoing branch is
  snapshotted first, so switching back restored it.) Creating a branch now
  opens a dialog asking what it should start from — **a copy of your current
  save**, which is pre-selected, or **a fresh start with no save**. Each says
  in full what it will do to your save folder, so an empty branch only clears
  it because that is what you chose. From the command line,
  `opensave branch <gameId> <name>` copies and `--empty` does not.
- **A failed backup no longer let a branch switch wipe the save anyway.**
  Switching snapshots the current save first, so the change can be undone —
  but if that snapshot failed, the failure was only written to the log and
  the switch carried on and cleared the folder regardless. The one situation
  where the backup mattered most, a full disk or a file the game still had
  open, was the one where it was skipped. A switch that cannot back up now
  stops and changes nothing.
- **A game could end up tracked twice after being linked.** Auto-tracking a
  peer's game checks whether that game is already linked to one here before
  creating anything, but the check and the creation were separate steps and a
  link written in between was seen by neither: the link found no entry to
  absorb because it did not exist yet, and the auto-track had already decided
  no link existed. The game then appeared a second time under the peer's id,
  beside the entry it had just been linked to. Both now happen as one step.
- **A snapshot could be left half-written when the app closed.** Snapshots are
  started from several places — the watcher as a game saves, a sync following
  the peer onto another branch, a safety copy before a restore — and shutdown
  did not wait for one in progress. It carried on writing into a folder that
  was going away and recorded itself against a database that had already
  closed, so the snapshot was never really taken and nothing reported it.
- **Two snapshots taken in the same millisecond lost one of them.** Snapshot
  identifiers are built from the clock, so a sync snapshotting several games
  at once, or an automatic backup landing beside a manual one, could collide.
  The second failed and the backup silently did not happen.
- **Cloud backups could be left empty or truncated.** A backup interrupted
  part-way left a partial file in the cloud that would be treated as a good
  copy; the app now finishes an upload before shutting down, and repairs a
  remote copy whose size does not match instead of skipping it.
- **Switch emulator saves were offered as one enormous entry.** The scanner
  presented the whole NAND profile tree rather than each game's own save, so
  tracking one game meant tracking all of them.
- **Beta versions past the ninth were treated as older than the ones before
  them.** Pre-release suffixes were compared as plain text, where "beta.10"
  sorts below "beta.9" because "1" precedes "9". Any beta series reaching
  double figures would have quietly stopped offering updates. They are now
  compared the way semantic versioning specifies, one identifier at a time and
  numerically where both sides are numbers.
- **Snapshot file sizes showed as 0 B, and restoring a single file did
  nothing.** Both were the command line reading fields by the wrong name —
  which produces an empty value rather than an error, so both looked like
  they had worked.
- **`opensave status --json` printed the ordinary status panel.** The flag
  was accepted and ignored, so anything scripting against it got a table of
  text where it expected JSON.
- **An update that could not be written reported success and then failed.**
  The check asked whether a temporary file could be created next to the app,
  which antivirus and Controlled Folder Access will happily allow while
  refusing the executable itself. It now attempts the exact file the update
  writes, and falls back to the installer when that is refused.

## [2.2.0] — 2026-07-28

### Added

- **Open a game's save folder from the app.** Every tracked game has a
  button that reveals its save location in Explorer, Finder or your Linux
  file manager — for checking what the scanner actually picked, or getting
  at a file by hand.
- **Match the same game across devices, even under different names.** A
  title tracked as "The First Berserker: Khazan" on one machine and a
  repack's folder name on another can now be linked, either automatically by
  Steam App ID or by linking the two entries yourself. App-ID matching is
  off by default, so two separate copies of the same game are never merged
  without you asking.
- **A completely rebuilt command line.** `opensave` is now a full headless
  client rather than a helper: auto-scan, tracking, pairing and approval,
  sync, conflict resolution, snapshots and branches, cloud backup, snapshot
  browsing and single-file restore, per-game settings, game linking, and
  running as a background service. It has styled output, a status panel on
  the bare command, shell completions for bash/zsh/fish, a man page, an
  installer on Windows and Linux, `--json` on every command for scripting,
  and `opensave update` to update itself. A Steam Deck in Game Mode or a
  headless server never needs the desktop app.
- **`opensave install` puts the CLI on your PATH from the binary itself.**
  The install scripts already did this, but the release also publishes the
  bare executable, and downloading that left you with a loose file to place
  and a PATH to edit by hand. Running `opensave install` now copies it
  somewhere permanent, sets up the `os` and `opensave-cli` aliases, and adds
  the directory to your PATH, so `opensave` works from any new terminal.
  `--dir` picks a different location.
- **Select several games at once.** Multi-select in the library for batch
  untracking, plus a "reset tracking" option that clears the library without
  touching your saves or snapshots.
- **Exclude folders from auto-scan.** Stale or wrong save locations can be
  dismissed permanently instead of being offered on every scan.
- **A Changelog page, and readable release notes.** The changelog lives
  under Settings in the sidebar, and the greeting after an update shows what
  changed since the version you were actually on.
- **Steam Deck: a rebuilt Decky plugin.** Sync, snapshot and resolve
  conflicts from Game Mode, with live sync progress and cover art in the
  panel, a daemon that starts itself, and a systemd service so syncing
  continues without opening anything.
- **Wider save detection.** Nine more emulators (PS1, PS4, Vita, Xbox,
  Dreamcast, 3DS and Switch forks), saves inside non-Steam Wine prefixes
  from Heroic, Bottles and Lutris, and save folders found by name inside a
  game's install directory.
- **Native Linux packages.** `.deb` and `.rpm` alongside the tarball and
  Flatpak.
- **A Support tab**, if you want to help fund the relay and the project.

### Changed

- **Large saves sync far faster and no longer load into memory first.**
  Blocks are written to disk as they arrive instead of being collected in
  full, so a 1 GB save no longer needs 1 GB of RAM before anything is
  written. Requests are pipelined rather than processed in fixed groups, and
  block data is compressed over the internet relay. A 40 MB save transfers
  in about a second on a LAN.
- **Auto-scan is quicker and quieter.** Cover-art fetches no longer hold up
  a scan, an unreachable Steam API costs one timeout instead of one per
  game, already-tracked saves are shown grouped below a divider rather than
  hidden, and shader caches are no longer offered as saves.
- **Cover art works on networks that block Steam.** When Steam's CDN can't
  be reached the app falls back to an image proxy automatically, and only
  warns about genuine connectivity problems rather than every game without
  published art.
- A sync interrupted by quitting the app is now cancelled cleanly instead of
  being left writing into the save folder during shutdown.

### Fixed

- **The Windows installer could rewrite unrelated parts of your PATH.** It
  set PATH through an API that hands back the *expanded* value and always
  writes a plain string back, so a PATH built from `%USERPROFILE%` or
  `%JAVA_HOME%` had those references replaced by whatever they pointed at
  during the install, and stopped following the variable afterwards. It also
  matched its own directory as a substring, so an unrelated folder with a
  similar name could convince it there was nothing to add. It now edits only
  the entry it owns and leaves the rest of the value as it found it.
- **Saves could be reported as conflicting when nothing about them
  disagreed.** A save's fingerprint covers its folders as well as its files,
  so a folder that existed on only one device was enough to make two
  otherwise identical saves look diverged — and a conflict would be raised
  asking which version to keep. There was nothing to choose between: the
  dialog had no differing file to name, so it listed none, and both sides
  showed the same file count, the same size and the same last-change time.
  A folder on one side only is now recognised for what it is and simply
  created on the other, as it always should have been. This is also why
  conflicts were appearing when the other device hadn't been touched.
- **The conflict dialog couldn't account for folders.** Where a real
  conflict involves a folder that exists on one side only, it is now listed
  alongside the differing files instead of being left out of the summary.
- **A device could stay online in the room while receiving nothing at all.**
  The relay gives every connected client a writer that drains its outbound
  queue. That writer stopped on any write error — including the send timeout
  a briefly stalled peer trips — while the reader kept accepting messages for
  it. Nothing drained the queue after that, so it filled, and every message
  bound for that device was dropped from then on. The device showed a healthy
  connection, kept sending its own heartbeats, and saw no peers and no syncs
  until something else closed the socket. The connection now closes when its
  writer stops, so the client reconnects instead of going quietly deaf.
- **A reinstalled device showed as offline forever.** Clearing a device's data
  gives it a new identity, and the pairing on the other machine still points
  at the old one — so its messages never match, and it sits at "offline"
  while plainly online and in the same room. Only unpairing and pairing again
  fixed it, with nothing on screen to suggest why. OpenSave now recognises
  this and says so, naming the device and the fix. It deliberately does not
  re-point the pairing on its own: pairing is what stops an unknown device
  reaching your saves, and adopting whatever turns up under a familiar name
  would give that away.
- **A paired device on both Wi-Fi and the internet relay could drop to
  offline while still connected.** Local discovery overwrote how the device
  was reached, quietly moving it out of the relay's care; when the local
  sighting then aged out, the device was marked offline even though the relay
  connection was live. Losing sight of a device on the local network now
  falls back to the relay instead of declaring it gone.
- **Auto-scan tracked the folder around your saves rather than the saves.**
  Steam and every Steam emulator keep the real save files in a `remote/`
  subfolder, with sync bookkeeping, achievements and playtime counters
  beside it; the scanner offered the parent. Those extra files change on
  every session independently on each device, so two machines diverged after
  every play with no save having changed — reported as both "it didn't find
  the exact save location" and "it's a bit glitchy". Containers wrapping a
  game's own save tree are unwrapped too.
- **Games matched across devices could agree what to sync, then fail to sync
  it.** App-ID matching and manual links were applied when a peer asked for a
  manifest and nowhere else, so the actual file transfer came back "Game not
  found" and no save data moved.
- **Pairing could complete on only one device.** The device starting the
  pairing tells the other which port to call back on, and a daemon that had
  fallen back to a different port advertised one nothing was listening on —
  so the approval succeeded on one machine and the other showed no paired
  devices at all. Internet sync kept working in that state, which made it
  look like a LAN-only fault. A callback that can't get through is now
  reported instead of passing silently.
- **The internet relay dropped every device every 15-30 minutes.** The host
  sleeps an idle instance and WebSocket traffic doesn't count as activity, so
  the relay went to sleep underneath live connections. Connected devices keep
  it awake now, reconnect with backoff instead of retrying in lockstep, and a
  routine reconnect no longer fills the activity log with warnings.
- **The relay could be taken down by a single stalled device.** Its outbound
  queue was bounded by message count while a message can be 16 MB, so one
  peer that stopped reading could make it buffer gigabytes.
- **A newer database no longer stops an older build from starting.**
- **Sync no longer guesses when two tracked games share an App ID.** Picking
  one could drop a peer's saves into the wrong folder, so it says so and lets
  you link the right pair yourself.
- Several save locations for the same game can be tracked separately.
- The auto-scan window no longer closes when you click into its search box —
  and the same fix applies to every other dialog.
- `opensave pair requests` shows which device is asking, instead of a bare
  id you had to approve blind.

## [2.1.1] — 2026-07-20

### Fixed

- **Conflict resolutions now stick.** Resolving a save conflict (Keep
  both / Keep mine / Keep theirs) records the agreed state so the same
  conflict can't re-appear on the next sync — fixing an endless
  re-prompting loop. "Keep mine" now also propagates your version to the
  peer instead of leaving the two devices permanently diverged.
- **Adding a game now shows up on your other devices immediately.**
  Tracking a game syncs it to paired peers right away (they auto-track
  it) instead of waiting for the periodic reconcile.
- **Untrack is now two-way and recoverable.** Untracking a game removes
  it on paired devices too and doesn't bounce back; re-tracking it on any
  device restores it on all of them.
- A peer that isn't tracking a game no longer produces an endless "Game
  not found" retry loop in the log.
- **Empty snapshots are caught, not silently backed up.** A snapshot
  with no files (almost always a mis-tracked save path) is flagged in
  Activity and never mirrored to the cloud. WebDAV uploads are verified
  after the fact, so a truncated/empty upload fails loudly.
- The in-app logo (title bar, About, boot screen) now shows the new icon.
- **Auto-scan no longer offers a game's whole install folder as its
  "save".** Games that keep their save file directly in the install
  directory (over 1,100 manifest entries — e.g. Sonic & Sega All-Stars
  Racing's `ssr_save.bin`) previously widened to the entire multi-GB
  install dir, which then got snapshotted and mirrored to cloud. The
  scanner now tracks the save file itself; single-file saves are fully
  supported by watch, snapshot, and sync.
- Save files sitting directly in broad folders like Documents are now
  offered as single-file saves instead of being skipped entirely.
- **Relay: large save transfers no longer kill the connection.** The
  relay's WebSocket message limit (32 KB by default) was far below a
  sync block (~2.7 MB), so every real transfer dropped the link and
  looped on reconnect-retry. The public relay is already fixed; this
  release carries the fix into the bundled `opensave-relay` and the
  in-app "host a relay" feature.
- **Handheld launch crashes fixed** (ROG Ally-class devices): WebKit's
  DMA-BUF renderer is disabled by default on Linux (it crashes on a
  range of GPU drivers; an explicitly set WEBKIT_DISABLE_DMABUF_RENDERER
  is respected), and the Flatpak moved to the GNOME 49 runtime, whose
  Mesa supports current handheld APUs. The tray icon now also works
  inside the Flatpak sandbox.
- Auto-scan no longer floods results with identical tiles from a busy
  Proton prefix (the "38× Persona 3 Reload" report): the precise
  manifest pass runs first, the coarse prefix listing defers to it,
  vendor/middleware folders are excluded, and entries keep their
  "(subfolder)" qualifier when renamed.
- **Content-based conflict detection.** Sync now records the manifest
  hash both devices verifiably held at each convergence (a merge-base,
  like git) and flags a conflict only when BOTH sides changed relative to
  it — replacing wall-clock mtime comparisons that had a blind window
  right after each sync under clock skew.
- Two devices that start with identical saves no longer hit a false
  "both sides modified" conflict on the first change: an in-sync check
  now records the shared state on both peers, not just the initiator.
- Unpairing a device now proactively notifies it (LAN and relay), so the
  other side stops treating you as paired immediately — no more ghost
  sync attempts or phantom "1 sync in progress" after an unpair.
- Sync progress can no longer stick at "0%" forever: a per-peer watchdog
  caps each sync pass, and the dashboard clears stalled sync indicators
  on its own (the backend retry loop still re-syncs automatically).
- Linux: the app window/taskbar icon now shows correctly, and the Linux
  tarball ships a launcher entry + icon with a one-line
  `install-desktop.sh` for app-menu integration.

### Added

- **Snapshot & branch management.** Delete individual snapshots or whole
  branches from a game's tabs, and a one-click "Clean up now" in Settings
  that prunes every game to its limit across all branches (and sweeps
  abandoned `conflict-*` branches left by resolved conflicts).
- **Snapshot retention controls.** A global default limit — now **20
  snapshots per game** — set in Settings, plus a per-game override in
  each game's Configuration tab. Retention now applies to every branch,
  not just the active one.
- **Yuzu-family Switch emulators.** Auto-scan now detects Suyu, Sudachi,
  Citron, and Eden alongside Yuzu and Ryujinx.
- **Choose-what-to-export save backups.** "Export saves…" now opens a
  picker listing every save on the machine — tracked games plus
  auto-detected ones — with select-all / tracked-only shortcuts. The
  .sscb file carries each game's current save AND where it belongs
  (paths stored in a machine-portable form, so a different PC or user
  account restores to the right place).
- **Two import modes.** "Add to snapshots" (the default) imports the
  saves into snapshot history without touching a single live file;
  "Overwrite current saves" restores everything onto disk — tracked
  games get a safety snapshot first, untracked targets get a safety zip
  in the backups folder before anything is replaced. The Activity tab
  records every game: what was restored, to which path, tracked or not.
- **Steam Deck: official Flatpak.** Every release now ships an
  `OpenSave.flatpak` that runs on stock SteamOS — no system packages, no
  lost install after SteamOS updates (the GNOME runtime provides the
  WebKit the app needs). SD-card saves (`/run/media`) are visible to the
  sandbox, and the in-app updater is Flatpak-aware (points at the new
  bundle instead of trying to self-swap the read-only install).
- **EmuDeck detection.** Auto-scan finds the `Emulation/saves` tree
  EmuDeck routes every emulator into — internal storage and SD card —
  offering each emulator as its own entry ("EmuDeck (retroarch)").
- **New app icon** — the pixel-art OS logo, across the app, installer,
  tray, and website.
- **System tray on Linux** (StatusNotifier/D-Bus): close-to-tray with
  Open / Sync all / Quit, matching Windows. On desktops without a tray
  host (stock GNOME without an extension), closing the window quits
  normally instead of stranding a hidden app.

## [2.1.0] — 2026-07-16

### Added

- **Linux & Steam Deck save detection.** Auto-scan is now platform-aware:
  - Emulator saves are found at their real Linux locations (native and
    Flatpak) — RetroArch, Dolphin, PCSX2, RPCS3, Ryujinx, yuzu, Citra,
    Cemu, PPSSPP.
  - **Proton prefixes are scanned**: games run through Proton store their
    saves in `steamapps/compatdata/<appid>/pfx`, and OpenSave now finds
    them (with the game's Steam cover art) — the bulk of Steam Deck saves.
  - The Ludusavi manifest resolves native Linux paths (`<xdgData>`,
    `<xdgConfig>`, `<home>`) and expands Windows-path entries inside each
    Proton prefix.
- The in-app updater is OS-aware: it installs the Linux tarball build on
  Linux (extracting the app binary) and the portable exe on Windows, and
  only ever applies a binary matching the running platform.


- Auto-scan now uses the community-maintained
  [Ludusavi manifest](https://github.com/mtkennerly/ludusavi-manifest)
  (sourced from PCGamingWiki): save locations for tens of thousands of
  games, detected purely by path — Steam, GOG, Epic, itch, and
  repack/cracked installs alike. A compressed snapshot (20k+ games,
  <1 MB) ships inside the binary, so scanning works instantly and fully
  offline; fresher manifest data downloads in the background at most
  once a week and takes precedence when present.
- More Steam-emulator/repack save locations detected: GSE (Goldberg
  fork), EMPRESS, Online-Fix, CPY, SmartSteamEmu, SKIDROW, and 3DM
  wrappers, alongside the existing Goldberg/CODEX/RUNE/Tenoke/FLT set.
- Large files are first-class: uploads and downloads stream from disk
  (memory use no longer scales with file size), Google Drive uses
  resumable uploads, and Dropbox/OneDrive switch to chunked upload
  sessions past their single-request limits — a 600 MB save moves
  through snapshot + cloud upload with ~1 MB of extra memory.
- Untracking a game now offers to delete its cloud snapshots too, so
  orphaned files no longer pile up in the provider.

### Fixed

- A save change made while a sync was already running is no longer lost
  until the periodic reconcile: the request queues a follow-up pass that
  runs when the active sync finishes (previously, deleting or changing a
  file mid-sync could silently skip propagation for minutes).
- Tracking a folder no longer blocks the app: path validation refuses
  nonexistent paths, drive roots, whole-profile/system folders, and
  OpenSave's own data directory with a clear message; the same folder
  can't be tracked twice; and the initial snapshot runs in the
  background (tracking a huge folder previously froze the API for
  minutes and could wedge the file watcher until restart).
- The file-watcher engine no longer holds its global lock during
  recursive directory walks — one slow watch can't freeze every other
  game's tracking operations.
- A snapshot no longer fails outright when a single file is unreadable
  (locked by the game or antivirus): unreadable files are skipped with
  a warning, and only a fully unreadable save is an error.
- Watcher auto-snapshots now push a live update to the dashboard.
- The "What's new" greeting no longer announces an update when only the
  build timestamp changed.

## [2.0.1] — 2026-07-15

First update delivered through the in-app updater. If you installed 2.0.0,
the update banner will offer this release — one click installs it.

### Fixed

- In-app update now works for installed (Program Files) copies: when the
  app can't replace its own files, it downloads the installer and launches
  it instead (UAC prompt) rather than failing with "Access is denied".
- A provider card (e.g. Local Folder) no longer shows "Connected" off the
  OAuth tokens belonging to a different provider.
- Non-app binaries (CLI, relay) report the correct version.
- GitHub releases are titled "OpenSave vX.Y.Z" instead of the bare tag.

### Notes

- Early 2.0.0 downloads predate the final 2.0.0 build; this release brings
  every install to a known-good state via the in-app updater.

## [2.0.0] — 2026-07-14

Complete rewrite of OpenSave from Node.js/Electron to **Go + Wails**: one small
native binary with no runtime to install. Wire-compatible with the original —
Go and JS devices sync together (same REST routes, P2P protocol, UDP discovery,
and relay envelope).

### Added

- Native desktop app (Wails webview) with a Hydra-style dark UI; system-tray
  background running.
- Update OpenSave from inside the app: one-click install from GitHub
  releases, or pull a newer build directly from a paired device ("Update
  from this device" on the Devices page) — no more copying the exe around.
- Release notes shown in the update banner, and the full changelog in the
  About dialog ("What's new").
- Activity log also written to `~/.opensave/opensave.log` for
  after-the-fact diagnosis.
- In-app styled confirmation dialogs replace the bare browser popups.
- Auto-scan for Steam, emulator, repack, Epic, GOG, and Unreal saves, shown as
  a browsable grid of vertical Steam cover art.
- Block-level delta sync (SHA-256, adaptive 64 KB–2 MB blocks) — only changed
  blocks transfer.
- Snapshot history with per-branch timelines, whole-save and single-file
  restore, and an automatic safety snapshot before every restore.
- Lineage-based conflict resolution (keep local / remote / both-as-branch).
- P2P over LAN (zero-config UDP discovery) and internet (relay room codes),
  with an option to self-host the relay.
- Cloud backup to Google Drive, Dropbox, OneDrive, WebDAV, webhook, or a
  local/NAS folder, with a per-game cloud snapshot browser.
- Cloud snapshot browser: cover-art tile grid (like auto-scan) with per-game
  drill-in, restore, delete, upload, live upload progress, and In
  cloud / Not uploaded filters.
- Google Drive snapshots now live in an auto-created "OpenSave" folder
  instead of the Drive root (override with a folder ID in Settings).
- Cloud mirroring is on by default; the toggle, Drive folder ID, and custom
  OAuth client IDs moved to Settings → Sync.
- In-app About dialog and an optional "update available" banner.
- First-run welcome with guided next steps.

### Fixed

- Cross-origin (CORS) preflight is handled, so tracking games from the UI no
  longer fails with "Failed to fetch".
- Cloud sync self-heals a revoked/expired OAuth token instead of falsely
  showing "connected", and prompts you to reconnect.
- Cover-art image error handling no longer risks a UI freeze; the sidebar,
  cards, and detail header fall back cleanly.
- Per-game view state no longer leaks between games in the detail view.
- A failed download could delete the original file on the sending device
  (sync lineage now only counts files verifiably present on both sides).
- Leftover `.opensave.tmp` files from interrupted transfers no longer sync
  to other devices; stale ones are cleaned up automatically.
- Antivirus briefly locking freshly-written files (especially `.exe`) no
  longer fails the sync — the final rename retries for several seconds.
- Save paths pointing at profile/system folders are refused with a clear
  message instead of failing every sync on a Windows junction.
- Resolving a save conflict no longer freezes the app during long
  transfers; progress updates during large files instead of sitting at 0%.
- Clear full-screen error (with Retry) when the window can't reach the
  background service, instead of endless "Loading…" panels.

### Security / safety

- Local and single-file restores now confirm before overwriting the current
  save (the current state is snapshotted first).
- The local API and dashboard remain loopback-only; relay traffic is limited
  to paired peers.

[2.3.0]: https://github.com/Liquid-co/OpenSave/releases/tag/v2.3.0
[2.2.3]: https://github.com/Liquid-co/OpenSave/releases/tag/v2.2.3
[2.2.2]: https://github.com/Liquid-co/OpenSave/releases/tag/v2.2.2
[2.2.1]: https://github.com/Liquid-co/OpenSave/releases/tag/v2.2.1
[2.2.0]: https://github.com/Liquid-co/OpenSave/releases/tag/v2.2.0
[2.1.1]: https://github.com/Liquid-co/OpenSave/releases/tag/v2.1.1
[2.1.0]: https://github.com/Liquid-co/OpenSave/releases/tag/v2.1.0
[2.0.1]: https://github.com/Liquid-co/OpenSave/releases/tag/v2.0.1
[2.0.0]: https://github.com/Liquid-co/OpenSave/releases/tag/v2.0.0
