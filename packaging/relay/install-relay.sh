#!/usr/bin/env bash
#
# One-command relay install for a Linux server.
#
# Setting up a relay was: build or fetch a binary, work out where to put it,
# write a systemd unit, open a port, and — if you want the encryption the
# clients default to — put a reverse proxy in front with the two WebSocket
# settings that are easy to miss. Every one of those is the same on every
# machine, which makes it a script's job rather than a person's.
#
#   curl -fsSL https://raw.githubusercontent.com/Liquid-co/OpenSave/main/packaging/relay/install-relay.sh | sudo bash
#   sudo ./install-relay.sh --domain relay.example.com    # with automatic TLS
#   sudo ./install-relay.sh --uninstall
#
# What it will not do, because it cannot: point a DNS record at this machine.
# TLS needs a name that resolves here, and only your registrar can do that.
# Without --domain the relay speaks ws:// instead of wss://, which the app
# accepts only at a private address — a LAN, or a VPN. Saves carry no
# encryption of their own, so cleartext to a public host would put the save
# file on the wire readable, and the client refuses it. A relay meant to be
# reached over the internet needs --domain.

set -euo pipefail

REPO="Liquid-co/OpenSave"
BIN_PATH="/usr/local/bin/opensave-relay"
UNIT_PATH="/etc/systemd/system/opensave-relay.service"
SERVICE_USER="opensave-relay"
ENV_DIR="/etc/opensave-relay"
ENV_PATH="$ENV_DIR/env"
GOOGLE_SECRET_FILE=""
STEAMGRIDDB_KEY_FILE=""
# Whether we hold each secret, however it arrived — a file, or the guided
# prompts. The write block keys off these rather than the file paths so
# both routes end up in the same place.
HAVE_GOOGLE=0
HAVE_STEAMGRID=0
GUIDED=0
ASSUME_YES=0
PORT="8386"
DOMAIN=""
VERSION=""
UNINSTALL=0
SKIP_VERIFY=0
DRY=0

say()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m !\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m✗\033[0m %s\n' "$*" >&2; exit 1; }

# Everything that changes the machine goes through run() or writes via
# write_file(), so --dry-run can show the whole plan without touching
# anything. Worth the indirection for a script people are asked to pipe into
# a root shell: it means "read it first" is something you can actually do, on
# your own machine, in the state it is really in.
run() {
	if [ "$DRY" -eq 1 ]; then
		printf '   \033[2mwould run:\033[0m %s\n' "$*"
	else
		"$@"
	fi
}

# write_file <path> — content on stdin.
write_file() {
	if [ "$DRY" -eq 1 ]; then
		printf '   \033[2mwould write %s:\033[0m\n' "$1"
		sed 's/^/     | /'
	else
		cat >"$1"
	fi
}

usage() {
	cat <<'EOF'
opensave-relay installer

  --domain <host>   Serve over https/wss at this name, with a certificate
                    obtained automatically. The name must already resolve to
                    this machine.
  --port <n>        Port the relay listens on (default 8386). With --domain
                    this stays internal and only 443 is public.
  --google-secret-file <path>
                    File holding your Google OAuth client secret, so this
                    relay can complete Google Drive sign-in for clients using
                    the built-in credentials. Read once and copied to
                    /etc/opensave-relay/env, root-only. A path rather than the
                    value itself: an argument is visible in `ps` to every user
                    on the machine while the command runs, and stays in shell
                    history afterwards.

                    Not needed for sync, and not needed at all if the people
                    using this relay supply their own OAuth credentials in the
                    app — that path talks to Google directly and never reaches
                    the relay.
  --steamgriddb-key-file <path>
                    File holding a SteamGridDB API key, so this relay can look
                    up cover art for games Steam has none for — anything sold
                    only on GOG, itch or Epic, and anything found under a
                    folder name no AppID could be resolved from. Same handling
                    as the Google secret: read once, copied to
                    /etc/opensave-relay/env, root-only, never printed.

                    Get a key free at
                    https://www.steamgriddb.com/profile/preferences/api

                    Every relay needs its own. A key is rate-limited per key,
                    so sharing one across relays means the busiest of them
                    exhausts it for the rest. Without it the relay starts
                    normally and clients simply fall back to Steam's own
                    artwork.
  --guided          Ask the questions instead of expecting flags. This is
                    also what happens by default when the script is run with
                    no configuration and a terminal attached, so most people
                    never need to pass it. Piping the script into bash never
                    triggers it, because stdin is the script itself.
  --yes, -y         Skip the confirmation at the end of the guided setup.
  --version <tag>   Install a specific release (default: the latest).
  --skip-verify     Do not check the download against SHA256SUMS. Only if the
                    release genuinely has no checksums file.
  --uninstall       Stop and remove the service, the binary and the user.
  --dry-run         Print every change it would make, and make none. Needs no
                    root, and is the way to read this before trusting it.
  --help            This.
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
		--domain) DOMAIN="${2:-}"; shift 2 ;;
		--port) PORT="${2:-}"; shift 2 ;;
		--google-secret-file) GOOGLE_SECRET_FILE="${2:-}"; shift 2 ;;
		--steamgriddb-key-file) STEAMGRIDDB_KEY_FILE="${2:-}"; shift 2 ;;
		--version) VERSION="${2:-}"; shift 2 ;;
		--skip-verify) SKIP_VERIFY=1; shift ;;
		--uninstall) UNINSTALL=1; shift ;;
		--guided) GUIDED=1; shift ;;
		--yes|-y) ASSUME_YES=1; shift ;;
		--dry-run) DRY=1; shift ;;
		--help|-h) usage; exit 0 ;;
		*) die "unknown option: $1 (try --help)" ;;
	esac
done

# ── Guided setup ─────────────────────────────────────────────────────
#
# Run with no configuration and a terminal attached, this asks rather than
# expecting flags to be known in advance. Self-hosting a relay is a thing
# people do once, and "read --help, pick the right four options" is a worse
# first contact than four questions with the reasoning attached.
#
# It never triggers where it would break something: piping the script into
# bash leaves stdin pointing at the script itself, so prompting there would
# eat the script and hang. Passing any configuration flag means the caller
# already knows what they want, and --guided forces it back on for someone
# who wants the questions with a flag or two pre-set.

ask() { # ask <prompt> <default>; answer on stdout
	local prompt="$1" default="${2:-}" reply=""
	if [ -n "$default" ]; then
		printf '  %s [%s]: ' "$prompt" "$default" >&2
	else
		printf '  %s: ' "$prompt" >&2
	fi
	IFS= read -r reply || reply=""
	printf '%s' "${reply:-$default}"
}

ask_secret() { # ask_secret <prompt>; answer on stdout, never echoed
	local prompt="$1" reply=""
	printf '  %s: ' "$prompt" >&2
	stty -echo 2>/dev/null || true
	IFS= read -r reply || reply=""
	stty echo 2>/dev/null || true
	printf '
' >&2
	printf '%s' "$reply"
}

guided_setup() {
	printf '
  OpenSave relay setup
  --------------------

'
	printf '  Installs the relay as a service on this machine and starts it.

'

	printf '  1. Domain name
'
	printf '     A name pointed at this machine lets the relay use wss://, which is
'
	printf '     what OpenSave requires for syncing across the internet. Leave this
'
	printf '     blank and it serves ws://, which is accepted only on a LAN or a VPN.
'
	printf '     DNS has to already point here — this cannot do that part.
'
	DOMAIN="$(ask 'Domain (blank for LAN/VPN only)' "$DOMAIN")"
	printf '
'

	printf '  2. Port
'
	PORT="$(ask 'Port to listen on' "$PORT")"
	printf '
'

	printf '  3. Cover art (optional)
'
	printf '     A SteamGridDB key lets this relay find artwork for games Steam has
'
	printf '     none for. Free, takes a minute:
'
	printf '     https://www.steamgriddb.com/profile/preferences/api
'
	printf '     Use your own rather than sharing one — the limit is per key.
'
	# Stripped exactly as the --steamgriddb-key-file path strips: a key
	# pasted with a stray space or newline is sent as part of the bearer
	# token, and SteamGridDB answers that with a bare 401 indistinguishable
	# from a key that was never valid. Pasting is the normal way to enter
	# this, which makes stray whitespace the normal way to get it wrong.
	STEAMGRIDDB_KEY="$(ask_secret 'SteamGridDB key (blank to skip)' | tr -d '[:space:]')"
	if [ -n "$STEAMGRIDDB_KEY" ]; then HAVE_STEAMGRID=1; fi
	printf '
'

	printf '  4. Google Drive sign-in (optional)
'
	printf '     Only needed if people using this relay sign in to Drive with
'
	printf "     OpenSave's built-in credentials. Most self-hosters skip this.
"
	# Same reasoning: whitespace here reaches Google as part of the secret,
	# and the only thing it says back is "invalid_client".
	GOOGLE_SECRET="$(ask_secret 'Google client secret (blank to skip)' | tr -d '[:space:]')"
	if [ -n "$GOOGLE_SECRET" ]; then HAVE_GOOGLE=1; fi

	printf '
  ----------------------------------------
'
	if [ -n "$DOMAIN" ]; then
		printf '  Address:      wss://%s  (certificate obtained automatically)
' "$DOMAIN"
	else
		printf '  Address:      ws:// on this machine — LAN or VPN only
'
	fi
	printf '  Port:         %s
' "$PORT"
	if [ "$HAVE_STEAMGRID" -eq 1 ]; then printf '  Cover art:    key provided
'; else printf '  Cover art:    skipped
'; fi
	if [ "$HAVE_GOOGLE" -eq 1 ]; then printf '  Drive log-in: secret provided
'; else printf '  Drive log-in: skipped
'; fi
	printf '  ----------------------------------------

'

	if [ "$ASSUME_YES" -eq 0 ]; then
		local go
		go="$(ask 'Install with these settings? (y/n)' 'y')"
		case "$go" in
			[Nn]*) printf '
  Nothing was changed.

'; exit 0 ;;
		esac
	fi
	printf '
'
}

# Configuration on the command line means the caller has already decided.
CONFIGURED=0
if [ -n "$DOMAIN" ] || [ -n "$GOOGLE_SECRET_FILE" ] || [ -n "$STEAMGRIDDB_KEY_FILE" ] || [ -n "$VERSION" ]; then
	CONFIGURED=1
fi
if [ "$UNINSTALL" -eq 0 ] && { [ "$GUIDED" -eq 1 ] || { [ "$CONFIGURED" -eq 0 ] && [ -t 0 ]; }; }; then
	guided_setup
fi

if [ "$DRY" -eq 0 ] && [ "$(id -u)" -ne 0 ]; then
	die "run this as root: sudo $0 $*
Or see what it would do first, as yourself:  $0 --dry-run"
fi

if ! command -v systemctl >/dev/null 2>&1; then
	if [ "$DRY" -eq 1 ]; then
		# A dry run is for reading the plan, and should work anywhere.
		warn "no systemd here — showing the plan anyway, since this is a dry run"
	else
		die "no systemd here, so there is nothing to install into.

Run the relay directly instead:
  PORT=$PORT opensave-relay
Or as a container:
  docker run -d --restart unless-stopped -p $PORT:10000 opensave-relay"
	fi
fi

# Checked before any work is done: finding out the secret file is unreadable
# after the binary is installed and the service is running leaves a
# half-configured relay that answers sync but fails sign-in.
if [ -n "$GOOGLE_SECRET_FILE" ]; then
	[ -r "$GOOGLE_SECRET_FILE" ] || die "cannot read $GOOGLE_SECRET_FILE"
	# Read once, here, and never printed. Whitespace and the trailing newline
	# a text editor adds are stripped, because they would be sent to Google as
	# part of the secret and the failure that causes says only "invalid_client".
	GOOGLE_SECRET="$(tr -d '[:space:]' < "$GOOGLE_SECRET_FILE")"
	[ -n "$GOOGLE_SECRET" ] || die "$GOOGLE_SECRET_FILE is empty"
	HAVE_GOOGLE=1
fi

if [ -n "$STEAMGRIDDB_KEY_FILE" ]; then
	[ -r "$STEAMGRIDDB_KEY_FILE" ] || die "cannot read $STEAMGRIDDB_KEY_FILE"
	# Stripped for the same reason: a trailing newline would be sent as part of
	# the bearer token, and SteamGridDB answers that with a bare 401 that looks
	# exactly like a key that was never valid.
	STEAMGRIDDB_KEY="$(tr -d '[:space:]' < "$STEAMGRIDDB_KEY_FILE")"
	[ -n "$STEAMGRIDDB_KEY" ] || die "$STEAMGRIDDB_KEY_FILE is empty"
	HAVE_STEAMGRID=1
fi

# ── Uninstall ────────────────────────────────────────────────────────
if [ "$UNINSTALL" -eq 1 ]; then
	say "Stopping and removing the relay"
	run systemctl disable --now opensave-relay 2>/dev/null || true
	# The credential goes too. Leaving it behind means "uninstalled" left a
	# live secret on a machine somebody believes they have cleaned.
	run rm -f "$UNIT_PATH" "$BIN_PATH" "$ENV_PATH"
	run rmdir "$ENV_DIR" 2>/dev/null || true
	run systemctl daemon-reload
	run userdel "$SERVICE_USER" 2>/dev/null || true
	say "Gone. Any Caddy config was left alone — remove the OpenSave block by hand if you added one."
	exit 0
fi

# ── Which build ──────────────────────────────────────────────────────
case "$(uname -m)" in
	x86_64|amd64)  ASSET="opensave-linux-amd64.tar.gz"; INNER="opensave-linux/opensave-relay" ;;
	aarch64|arm64) ASSET="opensave-linux-arm64.tar.gz"; INNER="opensave-linux-arm64/opensave-relay" ;;
	*) die "no prebuilt relay for $(uname -m). Build it yourself: go build ./cmd/opensave-relay" ;;
esac

for tool in curl tar; do
	command -v "$tool" >/dev/null 2>&1 || die "$tool is required but not installed"
done

if [ -z "$VERSION" ]; then
	say "Finding the latest release"
	VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
		sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
	[ -n "$VERSION" ] || die "could not work out the latest version — pass --version <tag>"
fi
say "Installing $VERSION ($ASSET)"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
BASE="https://github.com/$REPO/releases/download/$VERSION"

curl -fsSL "$BASE/$ASSET" -o "$TMP/$ASSET" || die "download failed: $BASE/$ASSET"

# Verified by default. This script runs as root and puts a binary in your
# PATH; taking the download on trust would be the weakest link in the whole
# arrangement.
if [ "$SKIP_VERIFY" -eq 0 ]; then
	if curl -fsSL "$BASE/SHA256SUMS" -o "$TMP/SHA256SUMS" 2>/dev/null; then
		WANT="$(awk -v f="$ASSET" '$2 == f || $2 == "*"f {print $1}' "$TMP/SHA256SUMS" | head -1)"
		if [ -n "$WANT" ]; then
			GOT="$(sha256sum "$TMP/$ASSET" | awk '{print $1}')"
			[ "$WANT" = "$GOT" ] || die "checksum mismatch for $ASSET
  expected $WANT
  got      $GOT
Refusing to install. Try again, and if it persists say so on the issue tracker."
			say "Checksum verified"
		else
			warn "$ASSET is not listed in SHA256SUMS; continuing unverified"
		fi
	else
		warn "no SHA256SUMS published for $VERSION; continuing unverified"
	fi
fi

tar -xzf "$TMP/$ASSET" -C "$TMP" "$INNER" 2>/dev/null ||
	die "$ASSET did not contain $INNER — wrong release layout?"

run install -m 0755 "$TMP/$INNER" "$BIN_PATH"
if [ "$DRY" -eq 0 ]; then
	# Reports the tag we asked for rather than running the binary to ask it.
	#
	# Running it hung the installer outright. Every relay before the fix that
	# taught it about arguments ignored them and started a server instead —
	# so `opensave-relay --version` never returned, and the command
	# substitution here waited on it forever, with the unit file unwritten and
	# a stray relay left listening. Found by installing v2.2.1 on a real
	# machine, which is the version anyone running this today would get.
	#
	# It is also simply better: this executes a binary downloaded seconds ago,
	# as root, to learn something already known.
	say "Installed $BIN_PATH ($VERSION)"
fi

# ── Service ──────────────────────────────────────────────────────────
id -u "$SERVICE_USER" >/dev/null 2>&1 ||
	run useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"

# Hardened because it is a network-facing daemon that needs nothing from this
# machine: no files, no home, no privileges. If it is ever compromised the
# blast radius should be the socket and nothing else.
write_file "$UNIT_PATH" <<EOF
[Unit]
Description=OpenSave WAN relay
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Environment=PORT=$PORT
# Optional, hence the leading dash: a relay with no secrets configured starts
# normally and simply does not offer the features that need them — no Google
# sign-in proxy, no cover-art lookup. They live here rather than on
# Environment= lines because this unit file is world-readable and that file is
# not.
EnvironmentFile=-$ENV_PATH
ExecStart=$BIN_PATH
Restart=always
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6
MemoryMax=512M

[Install]
WantedBy=multi-user.target
EOF

# The credentials, if any were given. Deliberately not through write_file():
# that helper echoes content on a dry run, which is right for a unit file and
# wrong for these. Nothing below prints a value on any path.
#
# Both secrets share one env file, so it is truncated once and then appended
# to. Writing each with > would mean installing the second erased the first,
# and the relay would come up with the feature configured last and without the
# one configured a moment earlier — with nothing in the output to say so.
if [ "$HAVE_GOOGLE" -eq 1 ] || [ "$HAVE_STEAMGRID" -eq 1 ]; then
	if [ "$DRY" -eq 1 ]; then
		printf '   [2mwould write %s (0600, root):[0m
' "$ENV_PATH"
		if [ "$HAVE_GOOGLE" -eq 1 ]; then
			printf '     | GOOGLE_DRIVE_CLIENT_SECRET=<%s bytes, not shown>
' "${#GOOGLE_SECRET}"
		fi
		if [ "$HAVE_STEAMGRID" -eq 1 ]; then
			printf '     | STEAMGRIDDB_KEY=<%s bytes, not shown>
' "${#STEAMGRIDDB_KEY}"
		fi
	else
		install -d -m 0700 "$ENV_DIR"
		# Built beside the real file and renamed into place, so an interrupted
		# run cannot leave a half-written env file — and created at 0600
		# before a secret goes in, never briefly readable by anyone else.
		NEW_ENV="$ENV_PATH.new"
		: >"$NEW_ENV"
		chmod 0600 "$NEW_ENV"

		# Carry forward any secret this run was not given.
		#
		# This used to truncate, so installing one secret silently erased the
		# other: someone adding a SteamGridDB key to a relay that already did
		# Google Drive sign-in lost the sign-in, with nothing said. Re-running
		# the installer to add a feature is the normal way to use it, which
		# made that the normal way to hit it.
		if [ -f "$ENV_PATH" ]; then
			while IFS= read -r line || [ -n "$line" ]; do
				keep=1
				case "$line" in
					GOOGLE_DRIVE_CLIENT_SECRET=*)
						if [ "$HAVE_GOOGLE" -eq 1 ]; then keep=0; fi ;;
					STEAMGRIDDB_KEY=*)
						if [ "$HAVE_STEAMGRID" -eq 1 ]; then keep=0; fi ;;
				esac
				if [ "$keep" -eq 1 ]; then
					printf '%s
' "$line" >>"$NEW_ENV"
				fi
			done <"$ENV_PATH"
		fi

		if [ "$HAVE_GOOGLE" -eq 1 ]; then
			printf 'GOOGLE_DRIVE_CLIENT_SECRET=%s
' "$GOOGLE_SECRET" >>"$NEW_ENV"
			say "Google client secret installed to $ENV_PATH (root only)"
		fi
		if [ "$HAVE_STEAMGRID" -eq 1 ]; then
			printf 'STEAMGRIDDB_KEY=%s
' "$STEAMGRIDDB_KEY" >>"$NEW_ENV"
			say "SteamGridDB key installed to $ENV_PATH (root only)"
		fi
		mv "$NEW_ENV" "$ENV_PATH"
		chmod 0600 "$ENV_PATH"
	fi
fi

run systemctl daemon-reload

# An upgrade has to RESTART, not merely start.
#
# `enable --now` starts a stopped service and does nothing at all to a running
# one. On a first install that is right. On an upgrade — the same command, the
# same script, a machine that already has a relay — it leaves the old process
# serving while the new binary sits on disk unused. The is-active check below
# then passes, because the old process is perfectly alive, and the script says
# "Service running" and exits 0.
#
# So the one case that most needs to work is the one that silently did not,
# and nothing about the output gave it away: the only way to notice was to ask
# the running relay its version from somewhere else. Reported by a maintainer
# upgrading the public relay from 2.2.2, which went on serving 2.2.2 after a
# successful-looking install of 2.3.0.
#
# Queried on a dry run too. It reads state and changes nothing, and a dry run
# that always claimed "would start" would misreport the upgrade case — which
# is the one worth previewing.
was_running=0
if systemctl is-active --quiet opensave-relay 2>/dev/null; then
	was_running=1
fi
run systemctl enable opensave-relay
if [ "$was_running" -eq 1 ]; then
	say "Restarting the running relay to pick up the new binary"
	run systemctl restart opensave-relay
else
	run systemctl start opensave-relay
fi

if [ "$DRY" -eq 0 ]; then sleep 1; fi
if [ "$DRY" -eq 0 ]; then
	systemctl is-active --quiet opensave-relay ||
		die "the service did not start. Look at: journalctl -u opensave-relay -n 40"
	# Report the version actually being served, not the one just written to
	# disk: those were the same thing right up until they were not, and the
	# difference is the whole of the bug above.
	running_version=$(curl -fsS -m 3 "http://127.0.0.1:$PORT/health" 2>/dev/null |
		sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
	if [ -n "$running_version" ]; then
		say "Service running (serving v$running_version)"
	else
		say "Service running"
	fi
fi

# ── TLS, if a name was given ─────────────────────────────────────────
# Which address to tell people to point their devices at.
#
# This used to be the machine's PUBLIC address with a ws:// scheme, which the
# app now refuses: sync payloads carry the save file with no encryption of
# their own, so cleartext to a public host puts the save on the wire readable.
# Handing out an address the client rejects is worse than handing out none —
# the relay looks broken when it is working exactly as asked.
#
# So the rule here is the client's rule. An unencrypted relay is offered only
# at an address where the network is the trust boundary; anywhere else needs a
# name and a certificate, which is what --domain does.
is_private_addr() {
	case "$1" in
		127.*|::1|localhost) return 0 ;;
		10.*|192.168.*|169.254.*) return 0 ;;
	esac
	# 172.16-172.31 and the carrier-grade NAT range 100.64-100.127, both of
	# which need the second octet compared as a number rather than matched.
	o1="${1%%.*}"; rest="${1#*.}"; o2="${rest%%.*}"
	case "$o1" in
		172) [ "$o2" -ge 16 ] 2>/dev/null && [ "$o2" -le 31 ] 2>/dev/null && return 0 ;;
		100) [ "$o2" -ge 64 ] 2>/dev/null && [ "$o2" -le 127 ] 2>/dev/null && return 0 ;;
	esac
	return 1
}

# An address on this machine that the client would accept unencrypted, if it
# has one. A home server or a NAS does; a VPS usually does not.
LAN_ADDR=""
for candidate in $(hostname -I 2>/dev/null); do
	if is_private_addr "$candidate"; then
		LAN_ADDR="$candidate"
		break
	fi
done

PUBLIC_ONLY=0
if [ -n "$LAN_ADDR" ]; then
	PUBLIC_URL="ws://$LAN_ADDR:$PORT"
else
	PUBLIC_ONLY=1
	PUBLIC_URL=""  # nothing the client would accept; see the report below
fi

if [ -n "$DOMAIN" ]; then
	if [ "$DRY" -eq 1 ] && ! command -v caddy >/dev/null 2>&1; then
		say "Would install Caddy for automatic certificates, then serve $DOMAIN"
	elif ! command -v caddy >/dev/null 2>&1; then
		say "Installing Caddy for automatic certificates"
		if command -v apt-get >/dev/null 2>&1; then
			apt-get install -y debian-keyring debian-archive-keyring apt-transport-https curl gnupg >/dev/null
			curl -fsSL 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' |
				gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
			curl -fsSL 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
				>/etc/apt/sources.list.d/caddy-stable.list
			apt-get update -qq && apt-get install -y caddy >/dev/null
		elif command -v dnf >/dev/null 2>&1; then
			dnf install -y 'dnf-command(copr)' >/dev/null
			dnf copr enable -y @caddy/caddy >/dev/null
			dnf install -y caddy >/dev/null
		else
			die "could not install Caddy automatically on this distribution.
Install it yourself, then add:

  $DOMAIN {
      reverse_proxy localhost:$PORT
  }"
		fi
	fi

	# Appended, not overwritten: this machine may already be serving something.
	if ! grep -q "^$DOMAIN" /etc/caddy/Caddyfile 2>/dev/null; then
		if [ "$DRY" -eq 1 ]; then
			printf '   \033[2mwould append to /etc/caddy/Caddyfile:\033[0m %s { reverse_proxy localhost:%s }\n' \
				"$DOMAIN" "$PORT"
		else
			printf '\n%s {\n    reverse_proxy localhost:%s\n}\n' "$DOMAIN" "$PORT" >>/etc/caddy/Caddyfile
		fi
	fi
	run systemctl reload caddy 2>/dev/null || run systemctl restart caddy
	PUBLIC_URL="wss://$DOMAIN"
	say "Caddy serving $DOMAIN — the certificate arrives on the first request"
fi

# ── Firewall ─────────────────────────────────────────────────────────
# Only the port the outside actually needs: with a proxy in front the relay
# port stays local.
OPEN_PORT="$PORT"
[ -n "$DOMAIN" ] && OPEN_PORT="443"
if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
	run ufw allow "$OPEN_PORT/tcp" && say "Opened $OPEN_PORT/tcp in ufw"
elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
	run firewall-cmd --permanent --add-port="$OPEN_PORT/tcp"
	run firewall-cmd --reload
	say "Opened $OPEN_PORT/tcp in firewalld"
else
	warn "No active ufw/firewalld found. If your provider has its own firewall, open $OPEN_PORT/tcp there."
fi

# ── What to do next ──────────────────────────────────────────────────
HEALTH="$(printf '%s' "$PUBLIC_URL" | sed 's|^wss://|https://|; s|^ws://|http://|')/health"
# A bounded read rather than `tr </dev/urandom | head`: head closing the pipe
# early kills tr with SIGPIPE, the || fallback fires on top of the value that
# was already produced, and you get "f64rw3pick-your-own" printed as the code
# to type. Take a fixed number of bytes and filter what comes out.
#
# 512 bytes for 12 characters, because the filter throws most of them away:
# only 32 of 256 byte values survive, so a draw yields about an eighth of what
# it reads. The old 64-byte read produced roughly nine usable characters, which
# was enough for the six it took but would have come up short here — silently,
# as a weaker code that still looked right.
#
# Twelve characters of 32 symbols is ~60 bits, matching what the app generates.
# Anyone holding a room code can see your devices and ask them to pair, so it
# wants to be the length of a password rather than of a word.
ROOM="$(head -c 512 /dev/urandom 2>/dev/null | tr -dc '0123456789abcdefghjkmnpqrstvwxyz' | cut -c1-12)"
if [ "${#ROOM}" -lt 12 ]; then ROOM="pick-your-own"; fi

if [ "$PUBLIC_ONLY" -eq 1 ] && [ -z "$DOMAIN" ]; then
	cat <<EOF

$(say "Installed, but not usable yet.")

  Service     running on port $PORT
  Logs        journalctl -u opensave-relay -f

There is deliberately no address to copy here. The relay speaks ws:// until it
is given a name, this machine has only a public address, and the app refuses an
unencrypted relay at a public address — so anything printed here would be
rejected the moment it was pasted in.

EOF
else
	cat <<EOF

$(say "Done.")

  Relay URL   $PUBLIC_URL
  Health      $HEALTH
  Logs        journalctl -u opensave-relay -f

Now set this up on each device whose saves you sync — NOT on this server.
The relay never joins a room; rooms exist because clients ask for them.

  opensave config set relay-url $PUBLIC_URL
  opensave relay join $ROOM

Use the same room code on every device. In the app it is the same two fields
under Internet Sync. Treat the code like a password: anyone who has it can
send your devices a pairing request, though they still cannot sync anything
without you approving it on the device.

EOF
fi

if [ -z "$DOMAIN" ] && [ "$PUBLIC_ONLY" -eq 1 ]; then
	warn "This relay has no address the app will accept.

    It is running, but it was installed without --domain, so it speaks ws://
    rather than wss:// — and OpenSave refuses an unencrypted relay at a public
    address. Saves carry no encryption of their own, so that would put the save
    file itself on the wire in the clear.

    This machine has no private address to fall back on, which is normal for a
    hosted server. Point a name at it and re-run:

      sudo $0 --domain relay.example.com

    The certificate is obtained automatically once the name resolves here."
elif [ -z "$DOMAIN" ]; then
	warn "This relay is unencrypted (ws://), so it is reachable only from the network
    it is on — $LAN_ADDR is a private address, and the app refuses cleartext to
    a public one. That is fine for devices in the same house. To sync across
    the internet, re-run with --domain relay.example.com once a DNS record
    points here."
fi
