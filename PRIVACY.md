# Privacy

OpenSave is designed to be private by default. It is a self-contained tool
that runs entirely on your own devices.

## What OpenSave does not do

- **No accounts.** There is no sign-up, login, or user profile.
- **No telemetry.** OpenSave does not collect analytics, usage data, or crash
  reports, and never phones home with anything about you. It does make a few
  functional network requests (an update check, and looking up game names and
  cover art while scanning) — these are listed in full under *Outbound network
  requests* below.
- **No third-party save storage.** Your saves are never uploaded anywhere
  unless you explicitly enable Cloud Backup and choose a provider.

## Where your data lives

- **Saves & snapshots** stay on your own machine, in the folders you configure
  under Settings → Storage.
- **Peer-to-peer sync** transfers save data directly between your paired
  devices — over your LAN, or over the internet through a relay.
- **The relay** (whether the default public relay or one you self-host) only
  routes WebSocket frames between paired devices. It writes no save to disk and
  keeps nothing once a device disconnects.

  Save data sent through it is sealed end-to-end, with a key derived from the
  two devices' own keys when they paired. Neither the relay operator nor
  anyone else who has your room code can read it — which matters because a
  relay passes every message to every device in the room, so "who could read
  this" was never only the operator. What a relay can still see is that two
  devices are talking, roughly how much data, and which games by id.

  Two caveats. Both devices need a recent version, and a pairing made before
  key exchange existed has no key to seal with — re-pair those two devices.
  Ours is `wss://relay.opensave.org`, running on hardware the project rents;
  self-hosting one is a single command, and LAN sync never involves a relay at
  all.

## Outbound network requests

Beyond peer-to-peer sync and any Cloud Backup you enable, OpenSave makes a
small number of functional requests. None of them include an account, your
name, or the contents of your saves. Some do send a game's Steam **App ID**,
which identifies the *game* (not you) so its name or cover art can be looked
up:

- **Update check** — queries the GitHub Releases API to see whether a newer
  version exists. Sends nothing about you.
- **Game names (Auto-scan)** — asks Steam's public store API to turn a
  detected App ID into a display name.
- **Save-path database (Auto-scan)** — downloads the community-maintained
  Ludusavi manifest (a static file hosted on GitHub) to help locate saves.
  Sends nothing about you.
- **Cover art** — fetched from Steam's image CDN by App ID and then cached on
  your device. If your network blocks Steam directly (some campus or office
  networks do), OpenSave automatically retries through a public image proxy,
  [images.weserv.nl](https://images.weserv.nl), which fetches the same public
  cover image on your behalf — so the App ID is visible to that proxy in that
  case. Cover art is not your save data.

These happen while you scan for games or view your library, not continuously
in the background, and results are cached so they aren't repeated.

## Optional cloud backup

If you turn on Cloud Backup, snapshots are uploaded to the provider you choose
(Google Drive, Dropbox, OneDrive, WebDAV, a webhook, or a local/NAS folder)
using credentials you authorize. OAuth tokens are stored locally on your
device and are never sent to the OpenSave project. Disconnecting a provider
removes its stored tokens.

## Pairing & trust

Devices only sync after an explicit pairing approval. You can unpair a device
at any time from the Devices screen, which stops all sync with it.
