# Running your own relay

Two devices on the same network find each other by themselves. On different
networks — your PC at home and a laptop somewhere else — they need something
with a public address to introduce them. That is the relay.

OpenSave ships pointed at a free hosted one, so **you do not need this guide
to use internet sync**. Run your own if you want to not depend on someone
else's server, or want it to wake instantly instead of cold-starting.

---

## The one thing to get straight first

**The relay never joins a room, and there is no command to make it.**

This is the most common confusion, so it is worth being blunt about. The relay
is a dumb pipe. It has no configuration beyond a port, no idea which room codes
exist until clients turn up, and nothing to sign in to. There is no
`opensave relay join` to run *on the relay* — that command belongs on your
**gaming devices**, and there is nothing to install on the server beyond the
relay binary itself.

What actually happens:

1. Your PC connects to the relay and says "put me in room `purple-otter-42`".
2. The relay makes that room exist, because someone asked for it.
3. Your laptop connects and asks for the same room.
4. The relay now forwards frames between the two, and stores nothing. The
   connection to it is encrypted, but that encryption ends at the relay rather
   than at the far device — so a relay could read what it forwards. That is
   the reason to run your own.

So: **run the container, then point your devices at it.** That is the whole job.
If your VPS is not itself a machine you play games on, it never joins a room.

---

## The short way

On a Linux server, one command does the lot — binary, systemd service,
firewall, and a certificate if you have a domain pointed here:

```bash
curl -fsSL https://opensave.org/relay.sh -o install-relay.sh
sudo bash install-relay.sh
```

Run with no options and it asks: the domain name, the port, whether you want
cover art, and whether you need Google Drive sign-in — each with the reasoning
attached, and a summary to confirm before it changes anything. Nothing is
echoed while you type a key.

Every answer is also a flag, if you would rather say it all up front:

```bash
sudo bash install-relay.sh --domain relay.example.com
```

Leave `--domain` off and the relay speaks unencrypted `ws://`, which OpenSave
accepts only at a private address — a LAN or a VPN. On a hosted server, whose
only address is public, the installer will say so and print no address to copy,
because anything it printed would be refused. See [Why `ws://` is not enough](#3-put-tls-in-front-of-it).

With a name and a certificate it prints the relay URL and the two commands to
run on your gaming devices; `sudo bash install-relay.sh --uninstall` removes
everything.

**Read it before you run it as root.** It has a `--dry-run` that needs no
privileges and prints every change it would make, including the systemd unit
in full:

```bash
bash install-relay.sh --dry-run --domain relay.example.com
```

The download is checked against the release's `SHA256SUMS` before anything is
installed.

The rest of this page is the same thing done by hand, and what to do when it
does not work.

## 1. Run the relay

### Docker

```bash
docker build -f relay/Dockerfile -t opensave-relay .
docker run -d --name opensave-relay --restart unless-stopped -p 8386:10000 opensave-relay
```

The image sets `PORT=10000` internally; the `-p` maps it to 8386 on the host.
Use whatever host port you like.

### Binary

```bash
go build -o opensave-relay ./cmd/opensave-relay
./opensave-relay
```

It prints where it is listening and stays in the foreground. For a permanent
install, run it under systemd.

### Settings

Settings that are not secrets are environment variables — there are no flags:

| Variable | Default | What it does |
| --- | --- | --- |
| `PORT` | `8386` (`10000` in the image) | Port to listen on |
| `MAX_PER_ROOM` | `20` | Most devices allowed in one room |
| `OPENSAVE_RELAY_SECRETS` | see below | Where the secrets file lives |

### Secrets

Two optional features need a credential. Neither affects sync, and a relay
with neither configured starts normally and simply does not offer them:

| Secret | Enables | Section |
| --- | --- | --- |
| Google OAuth client secret | Google Drive sign-in through this relay | [3b](#3b-google-drive-sign-in-optional) |
| SteamGridDB API key | Cover art for games Steam has none for | [3c](#3c-cover-art-optional) |

The way to set them is `opensave-relay setup`, which asks for each in turn:

```bash
opensave-relay setup
```

Nothing is echoed while you type, and the file it writes is created `0600`
before a byte goes into it. That is the point of the command: a secret given
any other way tends to end up somewhere it outlives its usefulness — on a
command line your shell records, or in a unit file that gets committed.

It writes to `/etc/opensave/relay-secrets.json` where that is writable, and
`~/.opensave/relay-secrets.json` otherwise. `OPENSAVE_RELAY_SECRETS` overrides
the location.

Run it with `sudo` on a machine set up by `install-relay.sh` and it hands the
file to the `opensave-relay` account the service runs as, and says so. That
step matters more than it sounds: the file is `0600`, so a root-owned one is
unreadable to the service — the key would be stored, reported as configured by
`opensave-relay config`, and never reach the relay. If the handover cannot be
done it says so instead of reporting success, and `config` warns whenever the
owner and the service account disagree.

`GOOGLE_DRIVE_CLIENT_SECRET` and `STEAMGRIDDB_KEY` still work as environment
variables, and **take precedence over the file**, so a container or systemd
unit that already injects them keeps working untouched. `opensave-relay
config` prints what is configured and which of the two it came from — worth
running when a value you did not expect is in effect, because "it is in the
file but the environment is overriding it" is otherwise a genuinely confusing
hour. Both commands show only the last four characters of a key, enough to
tell two apart without revealing either.

`opensave-relay --help` prints all of this. Note that `relay-url` is a
*client* setting and does nothing here; see step 4.

## 2. Check it is up

```bash
curl http://your-server:8386/health
```

You should get JSON with `"status":"ok"`, plus `rooms`, `clients`,
`totalConnections` and `totalMessages`. Straight after starting, `rooms` is
**0** — that is correct, and does not mean anything is wrong. Rooms appear
when devices connect.

## 3. Put TLS in front of it

Not optional across the internet, and the app enforces it. Nothing in OpenSave
encrypts the sync payload — the save file travels gzipped inside JSON — so the
relay connection is the only thing between a save and the network it crosses.
A bare relay speaks plain `ws://`, and the app refuses that to any public
address, accepting it only where the network is itself the trust boundary
(loopback, a home LAN, or a private overlay such as Tailscale).

The usual arrangement is a reverse proxy — Caddy, nginx, Traefik — terminating
TLS on 443 and forwarding to the relay. Caddy needs two lines:

```
relay.example.com {
    reverse_proxy localhost:8386
}
```

Caddy gets a certificate automatically, and your relay URL becomes
`wss://relay.example.com`.

**Whatever proxy you use, it must pass WebSocket upgrades through.** Caddy and
Traefik do by default; nginx needs it spelled out:

```nginx
location / {
    proxy_pass http://localhost:8386;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 3600s;   # sync connections are long-lived
}
```

That last line matters: nginx closes idle connections after 60 seconds by
default, which shows up as a relay that keeps reconnecting.

Without a proxy you are limited to `ws://`, which the app accepts only at a
private address: `ws://192.168.1.50:8386` for a machine on your own network is
fine, and opening that port on the firewall is all it needs. A public address
over `ws://` is refused by the client, so a hosted server needs the proxy and
a certificate rather than an open port.

## 3b. Google Drive sign-in (optional)

Only relevant if the people using this relay sign in to Google Drive with
OpenSave's built-in credentials. Sync itself needs none of this, and neither
does anyone who enters their own OAuth client ID and secret in the app — that
path talks to Google directly and never touches the relay.

**Running your own relay? You almost certainly want to skip this section.**
The relay sends Google the client ID the app gave it, paired with the secret
configured here, and Google refuses the pair unless both halves belong to the
same OAuth app. The app sends OpenSave's built-in client ID, whose secret only
the official relay holds — so there is nothing you could put here that would
work. Anyone syncing to Drive through your relay should enter their own OAuth
client ID and secret in OpenSave instead: that path talks to Google directly,
never touches your relay, and needs nothing from you. This section is for
whoever operates the relay the built-in credentials belong to.

The relay completes the token exchange on behalf of those clients, which needs
the client secret for the OAuth app whose ID they are using. Put it in a file
and point the installer at it:

```bash
sudo bash install-relay.sh --domain relay.example.com --google-secret-file /root/gd-secret.txt
```

It is copied to `/etc/opensave-relay/env`, mode `0600`, root-owned, and the
service unit loads it with `EnvironmentFile=`. Two things that are deliberate:

- **A file, not an argument.** `--google-client-secret <value>` would be
  visible in `ps` to every user on the machine for as long as the command runs,
  and would stay in your shell history afterwards.
- **Not in the unit file.** Unit files under `/etc/systemd/system/` are
  world-readable, so a secret on an `Environment=` line can be read by any
  account on the box.

Delete your copy of the secret file afterwards; the installed one is enough.
`--uninstall` removes it along with everything else.

Without it the relay starts normally and simply does not offer the sign-in
proxy — Drive sign-in through this relay returns an error, and everything else
works.

## 3c. Cover art (optional)

OpenSave gets a game's cover from Steam by AppID. That covers most of a
library and none of the rest: a game sold only on GOG, itch or Epic has no
AppID at all, and one found under a folder name nothing could be resolved from
has none either. Those are the games that show up blank.

SteamGridDB catalogues artwork for them, searchable by name, and this relay
can look it up — if you give it a key.

**Get one free.** Sign in at [steamgriddb.com](https://www.steamgriddb.com/)
and generate a key at
[Preferences → API](https://www.steamgriddb.com/profile/preferences/api). It
is a 32-character string, and it takes about a minute.

**Then either** put it in a file and point the installer at it:

```bash
sudo bash install-relay.sh --domain relay.example.com --steamgriddb-key-file /root/sgdb-key.txt
```

**or** run setup on the server and paste it at the prompt:

```bash
opensave-relay setup
```

Both end up in the same place, root-readable only. As with the Google secret,
there is no `--steamgriddb-key <value>` flag on purpose: an argument is
visible in `ps` to every user on the machine for as long as the command runs,
and stays in your shell history afterwards. Delete your copy of the key file
once it is installed. Restart the relay for it to take effect.

Re-running the installer to add this to a relay that already has a Google
secret keeps the Google secret: it only replaces the values you pass on that
run. Adding one feature has never been a reason to lose another.

**Use your own key, not somebody else's.** SteamGridDB rate-limits per key, so
a key shared between relays is exhausted for everyone by whichever relay is
busiest. This is also why the key lives on the relay and not in the app: a key
compiled into an open-source client is a key anyone can lift out of the
binary, and the first person who does gets it limited for every user.

Without a key the relay starts normally, `/api/cover/lookup` answers `503`
with a message saying this relay has no key configured — which the app treats
differently from "this game has no art" — and covers fall back to Steam's own
artwork. Nothing else is affected.

Check it from your workstation:

```bash
curl 'https://relay.example.com/api/cover/lookup?name=Hollow%20Knight'
```

`200` with a `url` means it is working. `503` means the key did not reach the
relay; `opensave-relay config` will say whether it sees one.

## 4. Point your devices at it

**On each device**, not on the server.

In the app: **Internet Sync → Relay server (self-hostable)**, put in your URL,
then **Test relay** to confirm it answers. Then either generate a room code or
paste the one you are already using.

From the command line:

```bash
opensave config set relay-url wss://relay.example.com
opensave relay join purple-otter-42     # the same code on every device
opensave relay status                   # shows the room and the relay in use
```

Provisioning devices rather than configuring them by hand? Set
`OPENSAVE_RELAY_URL` in the environment instead and skip the `config set` — it
overrides the stored value for as long as it is set, so a container gets the
right relay without anyone running a command inside it:

```bash
OPENSAVE_RELAY_URL=wss://relay.example.com opensave daemon start
```

Note the prefix: it is `OPENSAVE_RELAY_URL`, not a bare `RELAY_URL`, so it
cannot collide with something else on the same box and quietly redirect your
sync traffic.

Use the **same room code and the same relay URL on every device**. The code is
the only thing that decides who can find whom, so treat it like a password —
anyone who has it can send your devices a pairing request. They still cannot
sync anything without you approving the pairing on the device itself.

To check they have met:

```bash
opensave peers
```

To stop using internet sync on a device: `opensave relay leave`.

## If it does not connect

**`opensave relay status` shows the room but no peers appear.** Check the other
device has the *same* code and the *same* relay URL — a typo in either produces
exactly this, silently. `curl .../health` on both machines proves they can
reach the server at all.

**It connects and then drops every minute.** Your reverse proxy is timing out
the WebSocket. See `proxy_read_timeout` above.

**"The relay connection is down", retrying.** Either the URL is wrong (`wss://`
against a relay with no TLS, or `ws://` against one behind HTTPS), or the port
is not open. `curl` the health endpoint from the client machine — not from the
server — to tell those apart.

**Health check works, WebSocket does not.** The proxy is serving HTTP fine but
not passing upgrades. That is the `Upgrade`/`Connection` headers.

**Devices found each other but nothing syncs.** The relay's job is finished at
that point — this is a pairing or tracking problem, not a relay one. Check both
sides show as paired under **Devices**, and that both are tracking the game.

## What the relay can and cannot see

It forwards frames between devices in the same room, writes no saves to disk,
and keeps nothing after a device disconnects — the room disappears when the
last member leaves. Restarting it drops live connections; clients reconnect on
their own.

What it *cannot* see is the saves. Save payloads are sealed end-to-end
between the two paired devices, with a key derived from the keys they pinned
when pairing, so a relay operator forwards ciphertext. That also closes
something less obvious: a relay hands every frame to every member of the room,
so before this, anyone you had given your room code to received your save data
whether or not you had paired with them.

What it *can* still see is that two devices are talking, roughly how much data
is moving, and which games by id. Running your own is still the better
property — not because ours is hostile, but because "you do not have to take
our word for it" beats a promise.

Both devices need a version with this. A pairing made before key exchange
existed has no key to seal with and still sends in the clear; re-pair those two
devices.

The health endpoint is public and reports counts only — how many rooms and
clients, never codes or contents.
